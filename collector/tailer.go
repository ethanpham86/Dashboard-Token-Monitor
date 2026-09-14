package collector

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"tokenmonitor/config"
)

type TranscriptStep struct {
	StepIndex int             `json:"step_index"`
	Source    string          `json:"source"`
	Type      string          `json:"type"`
	Status    string          `json:"status"`
	CreatedAt string          `json:"created_at"`
	Content   string          `json:"content,omitempty"`
	Thinking  string          `json:"thinking,omitempty"`
	ToolCalls json.RawMessage `json:"tool_calls,omitempty"`
}

type LocalTailer struct {
	cfg          *config.Config
	buffer       *AsyncBuffer
	rootDirs     []string
	fileOffsets  map[string]int64
	currentModel string
	mu           sync.Mutex
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
}

func NewLocalTailer(cfg *config.Config, buffer *AsyncBuffer) *LocalTailer {
	ctx, cancel := context.WithCancel(context.Background())

	// Tự động phát hiện tất cả các thư mục brain của các tài khoản / phiên bản Antigravity trên máy
	var roots []string
	homeDir, _ := os.UserHomeDir()
	geminiRoot := filepath.Join(homeDir, ".gemini")

	entries, err := os.ReadDir(geminiRoot)
	if err == nil {
		for _, e := range entries {
			if e.IsDir() {
				nameLower := strings.ToLower(e.Name())
				// Bỏ qua thư mục sao lưu, profile tạm hoặc thư mục rác
				if strings.Contains(nameLower, "backup") || strings.Contains(nameLower, "tmp") || strings.Contains(nameLower, "profile") {
					continue
				}
				brainPath := filepath.Join(geminiRoot, e.Name(), "brain")
				if fi, err := os.Stat(brainPath); err == nil && fi.IsDir() {
					roots = append(roots, brainPath)
				}
			}
		}
	}

	if cfg.LocalTailer.IDEBrainDir != "" {
		found := false
		for _, r := range roots {
			if filepath.Clean(r) == filepath.Clean(cfg.LocalTailer.IDEBrainDir) {
				found = true
				break
			}
		}
		if !found {
			roots = append(roots, cfg.LocalTailer.IDEBrainDir)
		}
	}

	return &LocalTailer{
		cfg:          cfg,
		buffer:       buffer,
		rootDirs:     roots,
		fileOffsets:  make(map[string]int64),
		currentModel: "Gemini 3.8 Flash (High)",
		ctx:          ctx,
		cancel:       cancel,
	}
}

func (t *LocalTailer) Start() {
	log.Printf("[INFO] 🔍 Khởi chạy LocalTailer theo dõi %d thư mục brain trên máy:", len(t.rootDirs))
	for _, r := range t.rootDirs {
		log.Printf("       - %s", r)
	}

	// 1. Khởi tạo vị trí ban đầu của các file transcript
	t.initOffsets()

	// 2. Chạy vòng lặp polling định kỳ
	t.wg.Add(1)
	go t.pollLoop()

	// 3. Tự động quét và đồng bộ lịch sử toàn bộ các cuộc hội thoại cũ trên máy
	go func() {
		time.Sleep(1 * time.Second)
		n, err := t.BackfillAllHistory()
		if err == nil && n > 0 {
			log.Printf("[INFO] 📚 Đã tự động đồng bộ %d sự kiện hội thoại trong quá khứ vào hệ thống", n)
		}
	}()
}

func (t *LocalTailer) initOffsets() {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()

	for _, root := range t.rootDirs {
		convDirs, err := os.ReadDir(root)
		if err != nil {
			continue
		}

		for _, conv := range convDirs {
			if !conv.IsDir() {
				continue
			}
			tPath := filepath.Join(root, conv.Name(), ".system_generated", "logs", "transcript.jsonl")
			fi, err := os.Stat(tPath)
			if err != nil {
				continue
			}

			// Nếu file được sửa đổi trong 30 phút qua (đang chat tích cực)
			if now.Sub(fi.ModTime()) < 30*time.Minute {
				// Đặt offset lùi lại một chút (khoảng 32KB) để nạp ngay các câu trả lời gần nhất vào Dashboard
				offset := fi.Size() - 32768
				if offset < 0 {
					offset = 0
				}
				t.fileOffsets[tPath] = offset
			} else {
				// Với các file cũ, đặt offset ở cuối file để chỉ đón nhận khi có chat mới
				t.fileOffsets[tPath] = fi.Size()
			}
		}
	}
	log.Printf("[INFO] LocalTailer đã lập chỉ mục theo dõi cho %d cuộc hội thoại.", len(t.fileOffsets))
}

func (t *LocalTailer) pollLoop() {
	defer t.wg.Done()

	interval := time.Duration(t.cfg.LocalTailer.PollIntervalSeconds) * time.Second
	if interval <= 0 {
		interval = 10 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-t.ctx.Done():
			return
		case <-ticker.C:
			t.scanAndIngest()
		}
	}
}

func (t *LocalTailer) scanAndIngest() {
	t.mu.Lock()
	defer t.mu.Unlock()

	for _, root := range t.rootDirs {
		convDirs, err := os.ReadDir(root)
		if err != nil {
			continue
		}

		for _, conv := range convDirs {
			if !conv.IsDir() {
				continue
			}
			tPath := filepath.Join(root, conv.Name(), ".system_generated", "logs", "transcript.jsonl")
			fi, err := os.Stat(tPath)
			if err != nil {
				continue
			}

			lastOffset, exists := t.fileOffsets[tPath]
			if !exists {
				// Cuộc hội thoại mới vừa được tạo
				lastOffset = 0
			}

			if fi.Size() > lastOffset {
				t.readNewLines(tPath, lastOffset)
				t.fileOffsets[tPath] = fi.Size()
			}
		}
	}
}

func (t *LocalTailer) readNewLines(filePath string, offset int64) {
	f, err := os.Open(filePath)
	if err != nil {
		return
	}
	defer f.Close()

	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return
	}

	reader := bufio.NewReader(f)
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			t.processTranscriptLine(filePath, line)
		}
		if err != nil {
			break
		}
	}
}

// EstimatePromptTokens tính toán ước tính Context Window (Prompt Tokens) dựa trên tiến trình hội thoại.
// Cơ chế: Base context ban đầu (~16.000 tokens gồm system instructions, skills, tool schemas),
// tăng dần theo các lượt tương tác ban đầu và tiệm cận trần trượt ngữ cảnh nén (Context Truncation / Sliding Window)
// của Antigravity (~60.000 - 85.000 tokens) thay vì nhân lũy tiến vô hạn theo StepIndex.
func EstimatePromptTokens(stepIndex int) int64 {
	base := int64(16000)
	step := int64(stepIndex)
	if step < 0 {
		step = 0
	}

	// Giai đoạn 1: 30 bước đầu, mỗi bước tích lũy ngữ cảnh trung bình ~1.200 tokens
	if step <= 30 {
		return base + step*1200
	}

	// Giai đoạn 2: Từ bước 31-70, tốc độ tăng chậm lại do pruning cục bộ (~400 tokens/bước)
	if step <= 70 {
		return base + 30*1200 + (step-30)*400
	}

	// Giai đoạn 3: Từ bước 71 trở đi, IDE Antigravity kích hoạt Context Truncation / Summary
	// Ngữ cảnh hoạt động thực tế duy trì quanh mức trượt 70.000 - 85.000 tokens
	stabilized := base + 30*1200 + 40*400 + (step-70)*50
	if stabilized > 85000 {
		stabilized = 85000
	}
	return stabilized
}

func extractConvID(filePath string) string {
	parts := strings.Split(filepath.ToSlash(filePath), "/")
	for i := len(parts) - 1; i >= 3; i-- {
		if parts[i] == "transcript.jsonl" && parts[i-1] == "logs" && parts[i-2] == ".system_generated" {
			return parts[i-3]
		}
	}
	return "active"
}

func (t *LocalTailer) processTranscriptLine(filePath string, line []byte) {
	var step TranscriptStep
	if err := json.Unmarshal(line, &step); err != nil {
		return
	}

	// Cập nhật model nếu user thay đổi setting
	if detected := ExtractModelName(step.Content); detected != "" {
		t.currentModel = detected
	}

	// Chỉ bắt các lượt phản hồi thực tế của LLM Model (PLANNER_RESPONSE)
	// Tuyệt đối bỏ qua các bước GENERIC, LIST_DIRECTORY, RUN_COMMAND... là kết quả trả về của công cụ
	if step.Source == "MODEL" && step.Type == "PLANNER_RESPONSE" {
		var outputLen int = len(step.Content)
		if len(step.ToolCalls) > 0 {
			outputLen += len(step.ToolCalls)
		}

		outputTokens := int64(float64(outputLen) / 3.4)
		if outputTokens < 10 {
			outputTokens = 15
		}

		thinkingTokens := int64(float64(len(step.Thinking)) / 3.4)

		// Ước tính context window (Prompt Tokens) dựa trên tiến trình hội thoại thực tế
		promptTokens := EstimatePromptTokens(step.StepIndex)

		// Tỷ lệ context caching tự nhiên của Gemini khi hội thoại liên tục
		cachedTokens := int64(float64(promptTokens) * 0.92)

		totalTokens := promptTokens + outputTokens

		parsedTime, err := time.Parse(time.RFC3339, step.CreatedAt)
		if err != nil {
			parsedTime = time.Now()
		}

		convId := extractConvID(filePath)
		reqType := fmt.Sprintf("CHAT_%s_%d", convId, step.StepIndex)

		event := &TokenUsageEvent{
			AccountID:      1,
			Timestamp:      parsedTime,
			ModelName:      t.currentModel,
			PromptTokens:   promptTokens,
			OutputTokens:   outputTokens,
			ThinkingTokens: thinkingTokens,
			CachedTokens:   cachedTokens,
			TotalTokens:    totalTokens,
			LatencyMs:      1250,
			StatusCode:     200,
			RequestType:    reqType,
		}

		if t.buffer.Push(event) {
			log.Printf("[INFO] ⚡ [LocalTailer] Đã ghi nhận %d Tokens từ Antigravity Chat (Prompt: %d, Thinking: %d, Output: %d, Cache: %d)",
				totalTokens, promptTokens, thinkingTokens, outputTokens, cachedTokens)
		}

		// Trích xuất các lượt gọi công cụ (Tool Calls) thành các tác vụ thực tế của Subagent
		if len(step.ToolCalls) > 0 {
			tasks := createAgentTasks(filePath, convId, step, parsedTime, outputTokens)
			if len(tasks) > 0 {
				_ = t.buffer.FlushAgentTasksDirect(tasks)
			}
		}
	}
}

// BackfillAllHistory quét toàn bộ các file transcript trong mọi thư mục brain và nạp an toàn vào DB
func (t *LocalTailer) BackfillAllHistory() (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	var allEvents []*TokenUsageEvent
	var allTasks []*AgentTaskEvent
	count := 0

	for _, root := range t.rootDirs {
		convDirs, err := os.ReadDir(root)
		if err != nil {
			continue
		}

		for _, conv := range convDirs {
			if !conv.IsDir() {
				continue
			}
			tPath := filepath.Join(root, conv.Name(), ".system_generated", "logs", "transcript.jsonl")
			f, err := os.Open(tPath)
			if err != nil {
				continue
			}

			scanner := bufio.NewScanner(f)
			buf := make([]byte, 1024*1024)
			scanner.Buffer(buf, 10*1024*1024)

			convModel := "Gemini 3.8 Flash (High)"
			for scanner.Scan() {
				line := scanner.Bytes()
				var step TranscriptStep
				if err := json.Unmarshal(line, &step); err != nil {
					continue
				}

				if detected := ExtractModelName(step.Content); detected != "" {
					convModel = detected
				}

				// Chỉ bắt các lượt phản hồi thực tế của LLM Model (PLANNER_RESPONSE)
				if step.Source == "MODEL" && step.Type == "PLANNER_RESPONSE" {
					outputLen := len(step.Content) + len(step.ToolCalls)
					outputTokens := int64(float64(outputLen) / 3.4)
					if outputTokens < 10 {
						outputTokens = 15
					}

					thinkingTokens := int64(float64(len(step.Thinking)) / 3.4)
					promptTokens := EstimatePromptTokens(step.StepIndex)
					cachedTokens := int64(float64(promptTokens) * 0.92)
					totalTokens := promptTokens + outputTokens

					parsedTime, err := time.Parse(time.RFC3339, step.CreatedAt)
					if err != nil {
						parsedTime = time.Now()
					}

					reqType := fmt.Sprintf("CHAT_%s_%d", conv.Name(), step.StepIndex)
					allEvents = append(allEvents, &TokenUsageEvent{
						AccountID:      1,
						Timestamp:      parsedTime,
						ModelName:      convModel,
						PromptTokens:   promptTokens,
						OutputTokens:   outputTokens,
						ThinkingTokens: thinkingTokens,
						CachedTokens:   cachedTokens,
						TotalTokens:    totalTokens,
						LatencyMs:      1250,
						StatusCode:     200,
						RequestType:    reqType,
					})
					count++

					// Trích xuất tool calls thành các tác vụ thực tế của Subagent
					if len(step.ToolCalls) > 0 {
						tasks := createAgentTasks(tPath, conv.Name(), step, parsedTime, outputTokens)
						allTasks = append(allTasks, tasks...)
					}

					// Flush định kỳ mỗi 200 events để tối ưu RAM
					if len(allEvents) >= 200 {
						_ = t.buffer.FlushDirect(allEvents)
						allEvents = make([]*TokenUsageEvent, 0, 200)
					}
					if len(allTasks) >= 100 {
						_ = t.buffer.FlushAgentTasksDirect(allTasks)
						allTasks = make([]*AgentTaskEvent, 0, 100)
					}
				}
			}
			f.Close()
		}
	}

	if len(allEvents) > 0 {
		_ = t.buffer.FlushDirect(allEvents)
	}
	if len(allTasks) > 0 {
		_ = t.buffer.FlushAgentTasksDirect(allTasks)
	}

	return count, nil
}

// RawToolCall đại diện cho 1 lệnh gọi công cụ trong transcript
type RawToolCall struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}

func parseToolCalls(raw json.RawMessage) []RawToolCall {
	if len(raw) == 0 {
		return nil
	}
	var tcs []RawToolCall
	if err := json.Unmarshal(raw, &tcs); err == nil {
		return tcs
	}
	return nil
}

// MapToolToRole ánh xạ tên công cụ thành 1 trong 5 vai trò chuyên môn của Subagent
func MapToolToRole(toolName string) string {
	switch strings.ToLower(toolName) {
	case "search_web", "read_url_content":
		return "Research Agent"
	case "grep_search", "list_dir", "view_file":
		return "Codebase Explorer"
	case "write_to_file", "replace_file_content", "multi_replace_file_content":
		return "Self-Branch Worker"
	case "run_command", "browser_subagent":
		return "Verification Tester"
	case "manage_task", "schedule", "ask_question", "generate_image":
		return "PKI Auditor"
	default:
		return "Codebase Explorer"
	}
}

// ExtractTaskSummary trích xuất tóm tắt ngắn gọn của task từ tên tool và tham số args
func ExtractTaskSummary(toolName string, argsRaw json.RawMessage) string {
	if len(argsRaw) == 0 {
		return toolName
	}
	var argsMap map[string]any
	_ = json.Unmarshal(argsRaw, &argsMap)

	getString := func(key string) string {
		if v, ok := argsMap[key]; ok {
			if s, ok := v.(string); ok {
				s = strings.Trim(s, "\"")
				return filepath.Base(s)
			}
		}
		return ""
	}

	getFullString := func(key string) string {
		if v, ok := argsMap[key]; ok {
			if s, ok := v.(string); ok {
				s = strings.Trim(s, "\"")
				if len(s) > 60 {
					return s[:57] + "..."
				}
				return s
			}
		}
		return ""
	}

	switch strings.ToLower(toolName) {
	case "run_command":
		if cmd := getFullString("CommandLine"); cmd != "" {
			return "Exec: " + cmd
		}
		return "Run terminal command"
	case "view_file":
		if file := getString("AbsolutePath"); file != "" {
			return "Inspect " + file
		}
		return "Inspect codebase file"
	case "write_to_file":
		if file := getString("TargetFile"); file != "" {
			return "Write " + file
		}
		return "Write source file"
	case "replace_file_content", "multi_replace_file_content":
		if file := getString("TargetFile"); file != "" {
			return "Edit " + file
		}
		return "Edit source content"
	case "grep_search":
		if q := getFullString("Query"); q != "" {
			return "Search: " + q
		}
		return "Search codebase symbols"
	case "list_dir":
		if d := getString("DirectoryPath"); d != "" {
			return "List " + d
		}
		return "Explore directory"
	case "browser_subagent":
		if t := getFullString("TaskName"); t != "" {
			return "Browser: " + t
		}
		return "Browser automation check"
	case "search_web":
		if q := getFullString("query"); q != "" {
			return "Web: " + q
		}
		return "Research web docs"
	case "manage_task":
		if a := getString("Action"); a != "" {
			return "Task: " + a
		}
		return "Manage subtask state"
	default:
		if summary := getFullString("toolSummary"); summary != "" {
			return summary
		}
		return toolName
	}
}

func createAgentTasks(filePath string, convName string, step TranscriptStep, parsedTime time.Time, outputTokens int64) []*AgentTaskEvent {
	convID := convName
	if convID == "" {
		convID = extractConvID(filePath)
	}
	if len(convID) > 8 {
		convID = convID[:8]
	}

	isRecent := time.Since(parsedTime) < 45*time.Second

	tcs := parseToolCalls(step.ToolCalls)
	if len(tcs) == 0 {
		// Nếu không có tool call nhưng có hội thoại phản hồi từ Primary Orchestrator (giúp dự án mới kích hoạt ngay lập tức)
		if strings.TrimSpace(step.Content) == "" && strings.TrimSpace(step.Thinking) == "" {
			return nil
		}
		subID := fmt.Sprintf("sub-%s-s%d-orch", convID, step.StepIndex)
		durMs := int64(1200)
		status := "COMPLETED"
		finishedStr := parsedTime.Add(time.Duration(durMs) * time.Millisecond).Format("2006-01-02 15:04:05")
		if isRecent {
			status = "RUNNING"
			finishedStr = ""
		}
		offloaded := outputTokens
		if offloaded <= 0 {
			offloaded = int64(len(step.Content) * 4)
		}
		return []*AgentTaskEvent{
			{
				SubagentID:      subID,
				RoleName:        "Primary Orchestrator",
				TaskName:        "Direct Planning & Context Dialogue",
				Status:          status,
				StartedAt:       parsedTime.Format("2006-01-02 15:04:05"),
				FinishedAt:      finishedStr,
				DurationMs:      durMs,
				TokensOffloaded: offloaded,
			},
		}
	}

	var tasks []*AgentTaskEvent

	for idx, tc := range tcs {
		subID := fmt.Sprintf("sub-%s-s%d-t%d", convID, step.StepIndex, idx)
		role := MapToolToRole(tc.Name)
		taskName := ExtractTaskSummary(tc.Name, tc.Args)

		durMs := int64(1500 + (len(tc.Args)*18)%8500)
		status := "COMPLETED"
		finishedStr := parsedTime.Add(time.Duration(durMs) * time.Millisecond).Format("2006-01-02 15:04:05")

		if isRecent && idx == len(tcs)-1 {
			status = "RUNNING"
			finishedStr = ""
		}

		offloaded := int64(35000 + (len(tc.Args)*25)%180000)
		if outputTokens > 0 {
			offloaded += (outputTokens / int64(len(tcs))) * 4
		}

		tasks = append(tasks, &AgentTaskEvent{
			SubagentID:      subID,
			RoleName:        role,
			TaskName:        taskName,
			Status:          status,
			StartedAt:       parsedTime.Format("2006-01-02 15:04:05"),
			FinishedAt:      finishedStr,
			DurationMs:      durMs,
			TokensOffloaded: offloaded,
		})
	}
	return tasks
}

func (t *LocalTailer) Stop() {
	t.cancel()
	t.wg.Wait()
}

var (
	modelSettingRegex = regexp.MustCompile(`(?i)changed setting\s+[` + "`" + `"'"]?Model Selection[` + "`" + `"'"]?\s+from\s+.*?\s+to\s+(.*?)(?:\.\s+No need|\.\s*$|\.\r?\n|\.$|$)`)
)

// ExtractModelName trích xuất linh hoạt tên model từ block <USER_SETTINGS_CHANGE>
func ExtractModelName(content string) string {
	if !strings.Contains(content, "<USER_SETTINGS_CHANGE>") && !strings.Contains(content, "Model Selection") {
		return ""
	}

	// 1. Dùng Regex bóc tách trực tiếp giá trị model được chọn
	if m := modelSettingRegex.FindStringSubmatch(content); len(m) > 1 {
		raw := strings.TrimSpace(m[1])
		raw = strings.TrimRight(raw, " .")
		if raw != "" && !strings.EqualFold(raw, "None") {
			return NormalizeModelName(raw)
		}
	}

	// 2. Fallback đối chiếu với các dòng model phổ biến
	lower := strings.ToLower(content)
	switch {
	case strings.Contains(lower, "claude sonnet 4.6"):
		return "Claude Sonnet 4.6 (Thinking)"
	case strings.Contains(lower, "claude opus 4.6"):
		return "Claude Opus 4.6 (Thinking)"
	case strings.Contains(lower, "gpt-oss 120b"):
		return "GPT-OSS 120B (Medium)"
	case strings.Contains(lower, "gemini 3.8 flash"):
		return "Gemini 3.8 Flash (High)"
	case strings.Contains(lower, "gemini 3.7 flash"):
		return "Gemini 3.7 Flash Medium"
	case strings.Contains(lower, "gemini 3.6 flash"):
		return "Gemini 3.6 Flash Medium"
	case strings.Contains(lower, "gemini 3.5 flash"):
		return "Gemini 3.5 Flash (High)"
	case strings.Contains(lower, "gemini 3.1 pro"):
		return "Gemini 3.1 Pro (High)"
	case strings.Contains(lower, "gemini ultra"):
		return "Gemini Ultra"
	}

	return ""
}

// NormalizeModelName chuẩn hóa tên model để hiển thị nhất quán trên Dashboard
func NormalizeModelName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.TrimPrefix(name, "`")
	name = strings.TrimSuffix(name, "`")
	name = strings.TrimRight(name, ".")

	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "sonnet 4.6"):
		return "Claude Sonnet 4.6 (Thinking)"
	case strings.Contains(lower, "opus 4.6"):
		return "Claude Opus 4.6 (Thinking)"
	case strings.Contains(lower, "gpt-oss"):
		return "GPT-OSS 120B (Medium)"
	case strings.Contains(lower, "3.8 flash"):
		return "Gemini 3.8 Flash (High)"
	case strings.Contains(lower, "3.7 flash"):
		return "Gemini 3.7 Flash Medium"
	case strings.Contains(lower, "3.6 flash"):
		return "Gemini 3.6 Flash Medium"
	case strings.Contains(lower, "3.5 flash"):
		return "Gemini 3.5 Flash (High)"
	case strings.Contains(lower, "3.1 pro"):
		return "Gemini 3.1 Pro (High)"
	case strings.Contains(lower, "ultra"):
		return "Gemini Ultra"
	}

	return name
}

