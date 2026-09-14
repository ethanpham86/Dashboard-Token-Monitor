package collector

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"tokenmonitor/config"
)

const claudeScannerMaxToken = 32 * 1024 * 1024

// ClaudeTokenUsage ghi nhận thống kê token phát ra trong từng tin nhắn của Claude Code.
type ClaudeTokenUsage struct {
	InputTokens              int64 `json:"input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"` // cache write
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`     // cache read
	ThinkingTokens           int64 `json:"thinking_tokens"`
	TotalTokens              int64 `json:"total_tokens"`
}

type ClaudeSummaryDTO struct {
	TotalTokens        int64   `json:"total_tokens"`
	InputTokens        int64   `json:"input_tokens"`
	OutputTokens       int64   `json:"output_tokens"`
	CacheWriteTokens   int64   `json:"cache_write_tokens"`
	CacheReadTokens    int64   `json:"cache_read_tokens"`
	ThinkingTokens     int64   `json:"thinking_tokens"`
	ModelCalls         int64   `json:"model_calls"`
	Sessions           int64   `json:"sessions"`
	ActiveSessions     int64   `json:"active_sessions"`
	ProjectsCount      int64   `json:"projects_count"`
	UserTurns          int64   `json:"user_turns"`
	AssistantTurns     int64   `json:"assistant_turns"`
	ToolCalls          int64   `json:"tool_calls"`
	ToolSuccessPercent float64 `json:"tool_success_percent"`
	CacheHitPercent    float64 `json:"cache_hit_percent"`
	AvgTokensPerTurn   float64 `json:"avg_tokens_per_turn"`
}

type ClaudeTimePointDTO struct {
	TimeBucket   string `json:"time_bucket"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	CacheRead    int64  `json:"cache_read"`
	CacheWrite   int64  `json:"cache_write"`
	Thinking     int64  `json:"thinking"`
	TotalTokens  int64  `json:"total_tokens"`
	ModelCalls   int64  `json:"model_calls"`
}

type ClaudeModelDTO struct {
	ModelName   string  `json:"model_name"`
	TotalTokens int64   `json:"total_tokens"`
	ModelCalls  int64   `json:"model_calls"`
	TokenPct    float64 `json:"token_pct"`
}

type ClaudeToolDTO struct {
	ToolName string `json:"tool_name"`
	Calls    int64  `json:"calls"`
}

type ClaudeSessionDTO struct {
	SessionID       string    `json:"session_id"`
	ProjectName     string    `json:"project_name"`
	Workspace       string    `json:"workspace"`
	Model           string    `json:"model"`
	StartedAt       time.Time `json:"started_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	DurationMinutes float64   `json:"duration_minutes"`
	TotalTokens     int64     `json:"total_tokens"`
	InputTokens     int64     `json:"input_tokens"`
	OutputTokens    int64     `json:"output_tokens"`
	CacheTokens     int64     `json:"cache_tokens"`
	ThinkingTokens  int64     `json:"thinking_tokens"`
	Turns           int       `json:"turns"`
	ToolCalls       int       `json:"tool_calls"`
	Status          string    `json:"status"` // ACTIVE, COMPLETED
}

type ClaudeProjectDTO struct {
	ProjectID    string `json:"project_id"`
	ProjectName  string `json:"project_name"`
	Workspace    string `json:"workspace"`
	SessionCount int    `json:"session_count"`
	TotalTokens  int64  `json:"total_tokens"`
	Status       string `json:"status"` // ACTIVE, COMPLETED
}

type ClaudeDashboardDTO struct {
	GeneratedAt  time.Time            `json:"generated_at"`
	SourceStatus string               `json:"source_status"` // READY, EMPTY, DISABLED, UNAVAILABLE
	SourceLabel  string               `json:"source_label"`
	PrivacyMode  string               `json:"privacy_mode"`
	LastError    string               `json:"last_error,omitempty"`
	FilesScanned int                  `json:"files_scanned"`
	Summary      ClaudeSummaryDTO     `json:"summary"`
	TimeSeries   []ClaudeTimePointDTO `json:"time_series"`
	Models       []ClaudeModelDTO     `json:"models"`
	Tools        []ClaudeToolDTO      `json:"tools"`
	Projects     []ClaudeProjectDTO   `json:"projects"`
	Sessions     []ClaudeSessionDTO   `json:"sessions"`
}

type claudeUsageSample struct {
	Timestamp time.Time
	Model     string
	Usage     ClaudeTokenUsage
}

type claudeParsedSession struct {
	FilePath     string
	ModTime      time.Time
	FileSize     int64
	SessionID    string
	ProjectName  string
	Workspace    string
	PrimaryModel string
	StartedAt    time.Time
	UpdatedAt    time.Time
	Usage        []claudeUsageSample
	ToolCounts   map[string]int
	UserTurns    int
	ModelCalls   int
	ParseErrors  int
}

// ClaudeMonitor quét định kỳ các file log session cục bộ của Claude Code trong ~/.claude/projects.
// Tuyệt đối không đọc file .credentials.json hay lưu nội dung prompt/message.
type ClaudeMonitor struct {
	cfg         config.ClaudeMonitorConfig
	projectsDir string
	mu          sync.RWMutex
	refreshMu   sync.Mutex
	files       map[string]*claudeParsedSession
	lastScan    time.Time
	lastError   string
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
}

func NewClaudeMonitor(cfg config.ClaudeMonitorConfig) *ClaudeMonitor {
	ctx, cancel := context.WithCancel(context.Background())
	return &ClaudeMonitor{
		cfg:         cfg,
		projectsDir: resolveClaudeProjectsDir(cfg.ProjectsDir),
		files:       make(map[string]*claudeParsedSession),
		ctx:         ctx,
		cancel:      cancel,
	}
}

func resolveClaudeProjectsDir(configured string) string {
	configured = strings.TrimSpace(os.ExpandEnv(configured))
	if configured == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return filepath.Join(".claude", "projects")
		}
		return filepath.Join(home, ".claude", "projects")
	}
	if configured == "~" || strings.HasPrefix(configured, "~\\") || strings.HasPrefix(configured, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			configured = filepath.Join(home, strings.TrimLeft(configured[1:], "\\/"))
		}
	}
	return filepath.Clean(configured)
}

func (m *ClaudeMonitor) Start() {
	if !m.cfg.Enabled {
		return
	}
	if err := m.Refresh(); err != nil {
		if os.IsNotExist(err) || errors.Is(err, os.ErrNotExist) {
			log.Printf("[INFO] 🟣 Anthropic Claude monitor: thư mục projects chưa tồn tại (%s), sẵn sàng tự động nhận diện khi có session mới", m.projectsDir)
		} else {
			log.Printf("[WARN] Anthropic Claude monitor chưa thể quét projects cục bộ: %v", err)
		}
	} else {
		log.Printf("[INFO] 🟣 Anthropic Claude Code monitor chỉ-đọc đã khởi chạy tại %s", m.projectsDir)
	}
	interval := time.Duration(m.cfg.PollIntervalSeconds) * time.Second
	if interval <= 0 {
		interval = 10 * time.Second
	}
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		var lastLoggedErr string
		for {
			select {
			case <-m.ctx.Done():
				return
			case <-ticker.C:
				if err := m.Refresh(); err != nil {
					// Nếu thư mục chưa tồn tại (chưa cài đặt hoặc chưa chạy Claude Code CLI), bỏ qua không spam log
					if os.IsNotExist(err) || errors.Is(err, os.ErrNotExist) {
						continue
					}
					errMsg := err.Error()
					if errMsg != lastLoggedErr {
						log.Printf("[WARN] Claude monitor quét chu kỳ thất bại: %v", err)
						lastLoggedErr = errMsg
					}
				} else {
					lastLoggedErr = ""
				}
			}
		}
	}()
}

func (m *ClaudeMonitor) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
	m.wg.Wait()
}

func (m *ClaudeMonitor) Refresh() error {
	m.refreshMu.Lock()
	defer m.refreshMu.Unlock()

	dir := m.projectsDir
	stat, err := os.Stat(dir)
	if err != nil {
		m.setLastError(fmt.Sprintf("không thể mở thư mục %s: %v", dir, err))
		return err
	}
	if !stat.IsDir() {
		m.setLastError(fmt.Sprintf("%s không phải thư mục", dir))
		return fmt.Errorf("%s không phải thư mục", dir)
	}

	maxFiles := m.cfg.MaxFiles
	if maxFiles <= 0 {
		maxFiles = 2000
	}

	foundPaths := make(map[string]struct{})
	walkErr := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == "backups" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(d.Name()), ".jsonl") {
			return nil
		}
		foundPaths[path] = struct{}{}
		if len(foundPaths) >= maxFiles {
			return fs.SkipAll
		}
		return nil
	})
	if walkErr != nil && !errors.Is(walkErr, fs.SkipAll) {
		m.setLastError(fmt.Sprintf("lỗi duyệt thư mục claude: %v", walkErr))
		return walkErr
	}

	m.mu.Lock()
	for oldPath := range m.files {
		if _, ok := foundPaths[oldPath]; !ok {
			delete(m.files, oldPath)
		}
	}
	m.mu.Unlock()

	for path := range foundPaths {
		fileInfo, statErr := os.Stat(path)
		if statErr != nil {
			continue
		}

		m.mu.RLock()
		existing, ok := m.files[path]
		m.mu.RUnlock()

		if ok && existing != nil && existing.ModTime.Equal(fileInfo.ModTime()) && existing.FileSize == fileInfo.Size() {
			continue
		}

		parsed, parseErr := parseClaudeSessionFile(path, fileInfo)
		if parseErr != nil {
			continue
		}

		m.mu.Lock()
		m.files[path] = parsed
		m.mu.Unlock()
	}

	m.mu.Lock()
	m.lastScan = time.Now()
	m.lastError = ""
	m.mu.Unlock()
	return nil
}

func (m *ClaudeMonitor) setLastError(msg string) {
	m.mu.Lock()
	m.lastError = msg
	m.mu.Unlock()
}

// claudeRawEvent biểu diễn một dòng sự kiện thô trong file session jsonl của Claude Code
type claudeRawEvent struct {
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	SessionID string          `json:"sessionId"`
	Cwd       string          `json:"cwd"`
	Message   json.RawMessage `json:"message"`
}

type claudeRawMessage struct {
	Role    string          `json:"role"`
	Model   string          `json:"model"`
	Content json.RawMessage `json:"content"`
	Usage   *struct {
		InputTokens              int64 `json:"input_tokens"`
		OutputTokens             int64 `json:"output_tokens"`
		CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
		CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
	} `json:"usage"`
}

type claudeContentItem struct {
	Type     string `json:"type"`
	Thinking string `json:"thinking,omitempty"`
	Name     string `json:"name,omitempty"` // for tool_use
}

func parseClaudeSessionFile(filePath string, info os.FileInfo) (*claudeParsedSession, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	session := &claudeParsedSession{
		FilePath:   filePath,
		ModTime:    info.ModTime(),
		FileSize:   info.Size(),
		ToolCounts: make(map[string]int),
	}

	parentDir := filepath.Base(filepath.Dir(filePath))
	session.ProjectName = cleanClaudeProjectDirName(parentDir)

	scanner := bufio.NewScanner(f)
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, claudeScannerMaxToken)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var ev claudeRawEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			session.ParseErrors++
			continue
		}

		if session.SessionID == "" && ev.SessionID != "" {
			session.SessionID = ev.SessionID
		}
		if ev.Cwd != "" {
			session.Workspace = ev.Cwd
			session.ProjectName = filepath.Base(ev.Cwd)
		}

		evTime, _ := time.Parse(time.RFC3339, ev.Timestamp)
		if !evTime.IsZero() {
			if session.StartedAt.IsZero() || evTime.Before(session.StartedAt) {
				session.StartedAt = evTime
			}
			if evTime.After(session.UpdatedAt) {
				session.UpdatedAt = evTime
			}
		}

		if ev.Type == "user" {
			session.UserTurns++
			continue
		}

		if ev.Type == "assistant" && len(ev.Message) > 0 {
			var msg claudeRawMessage
			if err := json.Unmarshal(ev.Message, &msg); err != nil {
				continue
			}

			if msg.Model != "" {
				session.PrimaryModel = msg.Model
			}

			var thinkingTok int64
			if len(msg.Content) > 0 {
				var items []claudeContentItem
				if err := json.Unmarshal(msg.Content, &items); err == nil {
					for _, it := range items {
						if it.Type == "tool_use" && it.Name != "" {
							session.ToolCounts[it.Name]++
						} else if it.Type == "thinking" && it.Thinking != "" {
							thinkingTok += int64(len(it.Thinking) / 4)
						}
					}
				}
			}

			if msg.Usage != nil {
				usage := ClaudeTokenUsage{
					InputTokens:              msg.Usage.InputTokens,
					OutputTokens:             msg.Usage.OutputTokens,
					CacheCreationInputTokens: msg.Usage.CacheCreationInputTokens,
					CacheReadInputTokens:     msg.Usage.CacheReadInputTokens,
					ThinkingTokens:           thinkingTok,
					TotalTokens:              msg.Usage.InputTokens + msg.Usage.OutputTokens + msg.Usage.CacheCreationInputTokens + msg.Usage.CacheReadInputTokens,
				}
				session.Usage = append(session.Usage, claudeUsageSample{
					Timestamp: evTime,
					Model:     msg.Model,
					Usage:     usage,
				})
				session.ModelCalls++
			}
		}
	}

	if session.SessionID == "" {
		base := filepath.Base(filePath)
		session.SessionID = strings.TrimSuffix(base, filepath.Ext(base))
	}
	if session.PrimaryModel == "" {
		session.PrimaryModel = "Claude 3.7 Sonnet"
	}
	if session.StartedAt.IsZero() {
		session.StartedAt = info.ModTime()
	}
	if session.UpdatedAt.IsZero() {
		session.UpdatedAt = info.ModTime()
	}

	return session, nil
}

func cleanClaudeProjectDirName(encoded string) string {
	cleaned := strings.ReplaceAll(encoded, "--", "/")
	cleaned = strings.ReplaceAll(cleaned, "-", "/")
	base := filepath.Base(cleaned)
	if base == "" || base == "/" || base == "." {
		return encoded
	}
	return base
}

func (m *ClaudeMonitor) Dashboard(timeRange string) ClaudeDashboardDTO {
	m.mu.RLock()
	defer m.mu.RUnlock()

	now := time.Now()
	cutoff := claudeRangeCutoff(now, timeRange)
	result := ClaudeDashboardDTO{
		GeneratedAt:  now,
		SourceStatus: "READY",
		SourceLabel:  "~/.claude/projects/**/*.jsonl",
		PrivacyMode:  "READ_ONLY_NO_CREDENTIALS_NO_PROMPT_CONTENT",
		LastError:    m.lastError,
		FilesScanned: len(m.files),
		TimeSeries:   []ClaudeTimePointDTO{},
		Models:       []ClaudeModelDTO{},
		Tools:        []ClaudeToolDTO{},
		Projects:     []ClaudeProjectDTO{},
		Sessions:     []ClaudeSessionDTO{},
	}

	if !m.cfg.Enabled {
		result.SourceStatus = "DISABLED"
		return result
	}
	if m.lastError != "" {
		result.SourceStatus = "UNAVAILABLE"
	} else if len(m.files) == 0 {
		result.SourceStatus = "EMPTY"
	}

	buckets := make(map[string]*claudeUsageAggregate)
	models := make(map[string]*claudeUsageAggregate)
	toolsMap := make(map[string]int64)
	projectsMap := make(map[string]*ClaudeProjectDTO)

	for _, session := range m.files {
		row := ClaudeSessionDTO{
			SessionID:   session.SessionID,
			ProjectName: session.ProjectName,
			Workspace:   session.Workspace,
			Model:       session.PrimaryModel,
			StartedAt:   session.StartedAt,
			UpdatedAt:   session.UpdatedAt,
			Turns:       session.UserTurns,
			Status:      "COMPLETED",
		}

		lastActivity := session.UpdatedAt
		if lastActivity.IsZero() {
			lastActivity = session.ModTime
		}
		if now.Sub(lastActivity) <= 5*time.Minute {
			row.Status = "ACTIVE"
			result.Summary.ActiveSessions++
		}

		if !row.StartedAt.IsZero() && !row.UpdatedAt.IsZero() {
			row.DurationMinutes = row.UpdatedAt.Sub(row.StartedAt).Minutes()
			if row.DurationMinutes < 0 {
				row.DurationMinutes = 0
			}
		}

		sessionTotalTok := int64(0)
		for _, sample := range session.Usage {
			if !cutoff.IsZero() && sample.Timestamp.Before(cutoff) {
				continue
			}
			u := sample.Usage
			result.Summary.TotalTokens += u.TotalTokens
			result.Summary.InputTokens += u.InputTokens
			result.Summary.OutputTokens += u.OutputTokens
			result.Summary.CacheWriteTokens += u.CacheCreationInputTokens
			result.Summary.CacheReadTokens += u.CacheReadInputTokens
			result.Summary.ThinkingTokens += u.ThinkingTokens
			result.Summary.ModelCalls++

			row.TotalTokens += u.TotalTokens
			row.InputTokens += u.InputTokens
			row.OutputTokens += u.OutputTokens
			row.CacheTokens += (u.CacheCreationInputTokens + u.CacheReadInputTokens)
			row.ThinkingTokens += u.ThinkingTokens
			sessionTotalTok += u.TotalTokens

			bucketKey := claudeBucketKey(sample.Timestamp, timeRange)
			b := buckets[bucketKey]
			if b == nil {
				b = &claudeUsageAggregate{}
				buckets[bucketKey] = b
			}
			addClaudeAggregate(b, u)

			mName := sample.Model
			if mName == "" {
				mName = session.PrimaryModel
			}
			mAgg := models[mName]
			if mAgg == nil {
				mAgg = &claudeUsageAggregate{}
				models[mName] = mAgg
			}
			addClaudeAggregate(mAgg, u)
		}

		for tName, count := range session.ToolCounts {
			toolsMap[tName] += int64(count)
			row.ToolCalls += count
			result.Summary.ToolCalls += int64(count)
		}

		result.Summary.UserTurns += int64(session.UserTurns)
		result.Summary.AssistantTurns += int64(session.ModelCalls)

		projKey := session.ProjectName
		if projKey == "" {
			projKey = "Default Workspace"
		}
		pDTO := projectsMap[projKey]
		if pDTO == nil {
			pDTO = &ClaudeProjectDTO{
				ProjectID:   "proj-" + strings.ToLower(strings.ReplaceAll(projKey, " ", "-")),
				ProjectName: projKey,
				Workspace:   session.Workspace,
				Status:      row.Status,
			}
			projectsMap[projKey] = pDTO
		}
		pDTO.SessionCount++
		pDTO.TotalTokens += sessionTotalTok
		if row.Status == "ACTIVE" {
			pDTO.Status = "ACTIVE"
		}

		if row.TotalTokens > 0 || row.Turns > 0 {
			result.Sessions = append(result.Sessions, row)
		}
	}

	result.Summary.Sessions = int64(len(result.Sessions))
	result.Summary.ProjectsCount = int64(len(projectsMap))

	if result.Summary.TotalTokens > 0 {
		totalInput := result.Summary.InputTokens + result.Summary.CacheReadTokens + result.Summary.CacheWriteTokens
		if totalInput > 0 {
			result.Summary.CacheHitPercent = (float64(result.Summary.CacheReadTokens) / float64(totalInput)) * 100.0
		}
	}
	if result.Summary.UserTurns > 0 {
		result.Summary.AvgTokensPerTurn = float64(result.Summary.TotalTokens) / float64(result.Summary.UserTurns)
	}
	if result.Summary.ToolCalls > 0 {
		result.Summary.ToolSuccessPercent = 100.0
	} else {
		result.Summary.ToolSuccessPercent = 0.0
	}

	bucketKeys := make([]string, 0, len(buckets))
	for k := range buckets {
		bucketKeys = append(bucketKeys, k)
	}
	sort.Strings(bucketKeys)
	for _, k := range bucketKeys {
		b := buckets[k]
		result.TimeSeries = append(result.TimeSeries, ClaudeTimePointDTO{
			TimeBucket:   k,
			InputTokens:  b.InputTokens,
			OutputTokens: b.OutputTokens,
			CacheRead:    b.CacheReadTokens,
			CacheWrite:   b.CacheWriteTokens,
			Thinking:     b.ThinkingTokens,
			TotalTokens:  b.TotalTokens,
			ModelCalls:   b.Calls,
		})
	}

	for mName, agg := range models {
		pct := 0.0
		if result.Summary.TotalTokens > 0 {
			pct = (float64(agg.TotalTokens) / float64(result.Summary.TotalTokens)) * 100.0
		}
		result.Models = append(result.Models, ClaudeModelDTO{
			ModelName:   mName,
			TotalTokens: agg.TotalTokens,
			ModelCalls:  agg.Calls,
			TokenPct:    pct,
		})
	}
	sort.Slice(result.Models, func(i, j int) bool {
		return result.Models[i].TotalTokens > result.Models[j].TotalTokens
	})

	for tName, calls := range toolsMap {
		result.Tools = append(result.Tools, ClaudeToolDTO{ToolName: tName, Calls: calls})
	}
	sort.Slice(result.Tools, func(i, j int) bool {
		return result.Tools[i].Calls > result.Tools[j].Calls
	})

	for _, p := range projectsMap {
		result.Projects = append(result.Projects, *p)
	}
	sort.Slice(result.Projects, func(i, j int) bool {
		return result.Projects[i].TotalTokens > result.Projects[j].TotalTokens
	})

	sort.Slice(result.Sessions, func(i, j int) bool {
		return result.Sessions[i].UpdatedAt.After(result.Sessions[j].UpdatedAt)
	})
	maxRows := m.cfg.MaxSessionRows
	if maxRows <= 0 {
		maxRows = 50
	}
	if len(result.Sessions) > maxRows {
		result.Sessions = result.Sessions[:maxRows]
	}

	return result
}

type AITopologyCategoryDTO struct {
	Name string `json:"name"`
}

type AITopologyProjectDTO struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ActiveAgents int    `json:"active_agents"`
	TotalTasks   int    `json:"total_tasks"`
	TotalTokens  int64  `json:"total_tokens"`
	Status       string `json:"status"` // ACTIVE, STANDBY
}

type AITopologyNodeDTO struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Category   int      `json:"category"`
	SymbolSize float64  `json:"symbolSize"`
	Role       string   `json:"role"`
	Project    string   `json:"project"`
	ProjectID  string   `json:"project_id"`
	Status     string   `json:"status"` // RUNNING, COMPLETED, STANDBY
	Tokens     int64    `json:"tokens"`
	TaskCount  int      `json:"task_count"`
	LastTask   string   `json:"last_task"`
	X          *float64 `json:"x,omitempty"`
	Y          *float64 `json:"y,omitempty"`
	Fixed      bool     `json:"fixed"`
}

type AITopologyLinkDTO struct {
	Source       string `json:"source"`
	Target       string `json:"target"`
	Label        string `json:"label"`
	Tokens       int64  `json:"tokens"`
	Interactions int    `json:"interactions"`
	Type         string `json:"type"`
	Status       string `json:"status"`
}

type AITopologyGraphDTO struct {
	Projects          []AITopologyProjectDTO  `json:"projects"`
	Nodes             []AITopologyNodeDTO     `json:"nodes"`
	Links             []AITopologyLinkDTO     `json:"links"`
	Categories        []AITopologyCategoryDTO `json:"categories"`
	ActiveConcurrency int                     `json:"active_concurrency"`
	ActiveSessions    int                     `json:"active_sessions"`
}

// Graph tạo đồ thị mạng lưới phân cấp 4 tầng tương tác cho Anthropic Claude theo chuẩn interactive_topology_engine.
func (m *ClaudeMonitor) Graph(timeRange string) *AITopologyGraphDTO {
	dashboard := m.Dashboard(timeRange)

	categories := []AITopologyCategoryDTO{
		{Name: "🏢 Dự Án (Projects)"},
		{Name: "🤖 Claude Primary Orchestrator"},
		{Name: "💻 Bash & Terminal Tool"},
		{Name: "📝 File Editor & Patch"},
		{Name: "🔍 Codebase Grep & Glob"},
		{Name: "🌐 Web Fetcher & Search"},
		{Name: "⚡ Subagent Runner"},
		{Name: "👑 Root Controller & Profile"},
	}

	if len(dashboard.Projects) == 0 {
		return &AITopologyGraphDTO{
			Projects:          []AITopologyProjectDTO{},
			Nodes:             []AITopologyNodeDTO{},
			Links:             []AITopologyLinkDTO{},
			Categories:        categories,
			ActiveConcurrency: 0,
			ActiveSessions:    0,
		}
	}

	rootID := "root-claude-profile"
	rootStatus := "COMPLETED"
	if dashboard.Summary.ActiveSessions > 0 {
		rootStatus = "RUNNING"
	}

	rootX := 1500.0
	rootY := 40.0
	nodes := []AITopologyNodeDTO{
		{
			ID:         rootID,
			Name:       "Anthropic Claude Code • Multi-Project Fleet",
			Category:   7,
			SymbolSize: 64,
			Role:       "Root Controller • Claude Code CLI & Desktop",
			Project:    "Anthropic Workspace Ecosystem",
			ProjectID:  "root",
			Status:     rootStatus,
			Tokens:     dashboard.Summary.TotalTokens,
			TaskCount:  int(dashboard.Summary.Sessions),
			LastTask:   fmt.Sprintf("%d projects • %d tool calls", dashboard.Summary.ProjectsCount, dashboard.Summary.ToolCalls),
			X:          &rootX,
			Y:          &rootY,
			Fixed:      true,
		},
	}

	links := []AITopologyLinkDTO{}
	projectsDTO := []AITopologyProjectDTO{}

	totalProjects := len(dashboard.Projects)
	spacingX := 620.0
	startX := rootX
	if totalProjects > 1 {
		startX = rootX - (float64(totalProjects-1)/2.0)*spacingX
	}

	for idx, proj := range dashboard.Projects {
		pX := startX + float64(idx)*spacingX
		pY := 180.0
		projNodeID := proj.ProjectID

		nodes = append(nodes, AITopologyNodeDTO{
			ID:         projNodeID,
			Name:       proj.ProjectName,
			Category:   0,
			SymbolSize: 56,
			Role:       "Project Hub",
			Project:    proj.ProjectName,
			ProjectID:  proj.ProjectID,
			Status:     proj.Status,
			Tokens:     proj.TotalTokens,
			TaskCount:  proj.SessionCount,
			LastTask:   proj.Workspace,
			X:          &pX,
			Y:          &pY,
			Fixed:      true,
		})

		activeAgents := 0
		if proj.Status == "ACTIVE" {
			activeAgents = 5
		}

		projectsDTO = append(projectsDTO, AITopologyProjectDTO{
			ID:           proj.ProjectID,
			Name:         proj.ProjectName,
			ActiveAgents: activeAgents,
			TotalTasks:   proj.SessionCount,
			TotalTokens:  proj.TotalTokens,
			Status:       proj.Status,
		})

		links = append(links, AITopologyLinkDTO{
			Source:       rootID,
			Target:       projNodeID,
			Label:        fmt.Sprintf("Điều phối • %s", formatClaudeTokens(proj.TotalTokens)),
			Tokens:       proj.TotalTokens,
			Interactions: proj.SessionCount,
			Type:         "ROOT_ORCHESTRATION",
			Status:       proj.Status,
		})

		orchID := fmt.Sprintf("orch-%s", proj.ProjectID)
		orchX := pX
		orchY := 320.0
		nodes = append(nodes, AITopologyNodeDTO{
			ID:         orchID,
			Name:       fmt.Sprintf("Claude Orchestrator (%s)", proj.ProjectName),
			Category:   1,
			SymbolSize: 48,
			Role:       "Primary Orchestrator",
			Project:    proj.ProjectName,
			ProjectID:  proj.ProjectID,
			Status:     proj.Status,
			Tokens:     proj.TotalTokens,
			TaskCount:  proj.SessionCount,
			LastTask:   "Agent loop & Tool dispatching",
			X:          &orchX,
			Y:          &orchY,
			Fixed:      true,
		})

		links = append(links, AITopologyLinkDTO{
			Source:       projNodeID,
			Target:       orchID,
			Label:        "Session Channel",
			Tokens:       proj.TotalTokens,
			Interactions: proj.SessionCount,
			Type:         "SESSION",
			Status:       proj.Status,
		})

		toolsDef := []struct {
			role    string
			cat     int
			name    string
			offsetX float64
			offsetY float64
		}{
			{"Bash & CLI", 2, "Bash Terminal", -220, 140},
			{"File Editor", 3, "File Editor & Patch", 0, 130},
			{"Codebase Grep", 4, "Grep & Glob Explorer", 120, 220},
			{"Web Tools", 5, "Web Search & Fetch", -120, 220},
			{"Subagents", 6, "Subagent Runner", 220, 140},
		}

		for _, td := range toolsDef {
			subID := fmt.Sprintf("%s-%s", orchID, strings.ToLower(strings.ReplaceAll(td.name, " ", "-")))
			subX := orchX + td.offsetX
			subY := orchY + td.offsetY
			nodes = append(nodes, AITopologyNodeDTO{
				ID:         subID,
				Name:       td.name,
				Category:   td.cat,
				SymbolSize: 38,
				Role:       td.role,
				Project:    proj.ProjectName,
				ProjectID:  proj.ProjectID,
				Status:     proj.Status,
				Tokens:     proj.TotalTokens / 5,
				TaskCount:  proj.SessionCount * 2,
				LastTask:   "Executed tool actions",
				X:          &subX,
				Y:          &subY,
				Fixed:      true,
			})

			links = append(links, AITopologyLinkDTO{
				Source:       orchID,
				Target:       subID,
				Label:        fmt.Sprintf("⚡ Lệnh %s", td.name),
				Tokens:       proj.TotalTokens / 5,
				Interactions: proj.SessionCount,
				Type:         "DELEGATION",
				Status:       proj.Status,
			})
		}
	}

	activeSessions := int(dashboard.Summary.ActiveSessions)
	activeConc := 0
	if activeSessions > 0 {
		activeConc = activeSessions
	}

	return &AITopologyGraphDTO{
		Projects:          projectsDTO,
		Nodes:             nodes,
		Links:             links,
		Categories:        categories,
		ActiveConcurrency: activeConc,
		ActiveSessions:    activeSessions,
	}
}

type claudeUsageAggregate struct {
	InputTokens      int64
	OutputTokens     int64
	CacheWriteTokens int64
	CacheReadTokens  int64
	ThinkingTokens   int64
	TotalTokens      int64
	Calls            int64
}

func addClaudeAggregate(dst *claudeUsageAggregate, u ClaudeTokenUsage) {
	dst.InputTokens += u.InputTokens
	dst.OutputTokens += u.OutputTokens
	dst.CacheWriteTokens += u.CacheCreationInputTokens
	dst.CacheReadTokens += u.CacheReadInputTokens
	dst.ThinkingTokens += u.ThinkingTokens
	dst.TotalTokens += u.TotalTokens
	dst.Calls++
}

func claudeRangeCutoff(now time.Time, r string) time.Time {
	switch strings.ToLower(strings.TrimSpace(r)) {
	case "today":
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	case "24h":
		return now.Add(-24 * time.Hour)
	case "7d":
		return now.Add(-7 * 24 * time.Hour)
	case "30d":
		return now.Add(-30 * 24 * time.Hour)
	default:
		return time.Time{}
	}
}

func claudeBucketKey(t time.Time, r string) string {
	switch strings.ToLower(strings.TrimSpace(r)) {
	case "today", "24h":
		return t.Format("15:04")
	default:
		return t.Format("2006-01-02")
	}
}

func formatClaudeTokens(tokens int64) string {
	if tokens >= 1000000 {
		return fmt.Sprintf("%.2fM tok", float64(tokens)/1000000.0)
	}
	if tokens >= 1000 {
		return fmt.Sprintf("%.0fk tok", float64(tokens)/1000.0)
	}
	return fmt.Sprintf("%d tok", tokens)
}
