package collector

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
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

const codexScannerMaxToken = 32 * 1024 * 1024

// CodexTokenUsage mirrors the non-secret usage counters emitted in Codex session logs.
// cached_input_tokens and reasoning_output_tokens are subsets of input/output respectively.
type CodexTokenUsage struct {
	InputTokens           int64 `json:"input_tokens"`
	CachedInputTokens     int64 `json:"cached_input_tokens"`
	CacheWriteInputTokens int64 `json:"cache_write_input_tokens"`
	OutputTokens          int64 `json:"output_tokens"`
	ReasoningOutputTokens int64 `json:"reasoning_output_tokens"`
	TotalTokens           int64 `json:"total_tokens"`
}

type CodexRateLimitWindow struct {
	UsedPercent   float64 `json:"used_percent"`
	WindowMinutes int64   `json:"window_minutes"`
	ResetsAt      int64   `json:"resets_at"`
}

type CodexCredits struct {
	HasCredits bool   `json:"has_credits"`
	Unlimited  bool   `json:"unlimited"`
	Balance    string `json:"balance,omitempty"`
}

type CodexRateLimits struct {
	LimitID    string                `json:"limit_id,omitempty"`
	LimitName  string                `json:"limit_name,omitempty"`
	PlanType   string                `json:"plan_type,omitempty"`
	Primary    *CodexRateLimitWindow `json:"primary,omitempty"`
	Secondary  *CodexRateLimitWindow `json:"secondary,omitempty"`
	Credits    *CodexCredits         `json:"credits,omitempty"`
	ObservedAt time.Time             `json:"observed_at"`
	SessionID  string                `json:"session_id,omitempty"`
	Stale      bool                  `json:"stale"`
}

type CodexSummaryDTO struct {
	TotalTokens        int64   `json:"total_tokens"`
	InputTokens        int64   `json:"input_tokens"`
	CachedInputTokens  int64   `json:"cached_input_tokens"`
	CacheWriteTokens   int64   `json:"cache_write_tokens"`
	OutputTokens       int64   `json:"output_tokens"`
	ReasoningTokens    int64   `json:"reasoning_tokens"`
	ModelCalls         int64   `json:"model_calls"`
	Sessions           int64   `json:"sessions"`
	ActiveSessions     int64   `json:"active_sessions"`
	Turns              int64   `json:"turns"`
	AssistantResponses int64   `json:"assistant_responses"`
	ToolCalls          int64   `json:"tool_calls"`
	ToolFailures       int64   `json:"tool_failures"`
	ToolResultsKnown   int64   `json:"tool_results_known"`
	ToolSuccessPercent float64 `json:"tool_success_percent"`
	CacheHitPercent    float64 `json:"cache_hit_percent"`
	AvgTokensPerTurn   float64 `json:"avg_tokens_per_turn"`
	TokensPerMinute    float64 `json:"tokens_per_minute"`
}

type CodexTimePointDTO struct {
	TimeBucket      string `json:"time_bucket"`
	InputTokens     int64  `json:"input_tokens"`
	CachedTokens    int64  `json:"cached_tokens"`
	OutputTokens    int64  `json:"output_tokens"`
	ReasoningTokens int64  `json:"reasoning_tokens"`
	TotalTokens     int64  `json:"total_tokens"`
	ModelCalls      int64  `json:"model_calls"`
}

type CodexModelDTO struct {
	ModelName   string  `json:"model_name"`
	TotalTokens int64   `json:"total_tokens"`
	ModelCalls  int64   `json:"model_calls"`
	TokenPct    float64 `json:"token_pct"`
}

type CodexSessionDTO struct {
	SessionID          string    `json:"session_id"`
	Workspace          string    `json:"workspace"`
	WorkspaceID        string    `json:"workspace_id"`
	Model              string    `json:"model"`
	ModelProvider      string    `json:"model_provider"`
	Originator         string    `json:"originator"`
	StartedAt          time.Time `json:"started_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	DurationMinutes    float64   `json:"duration_minutes"`
	Status             string    `json:"status"`
	Turns              int64     `json:"turns"`
	AssistantResponses int64     `json:"assistant_responses"`
	ToolCalls          int64     `json:"tool_calls"`
	ToolFailures       int64     `json:"tool_failures"`
	TotalTokens        int64     `json:"total_tokens"`
	InputTokens        int64     `json:"input_tokens"`
	CachedInputTokens  int64     `json:"cached_input_tokens"`
	OutputTokens       int64     `json:"output_tokens"`
	ReasoningTokens    int64     `json:"reasoning_tokens"`
	ModelCalls         int64     `json:"model_calls"`
	CacheHitPercent    float64   `json:"cache_hit_percent"`
}

type CodexDashboardDTO struct {
	LastScanAt        time.Time                `json:"last_scan_at"`
	Coverage          string                   `json:"coverage"`
	FilesDiscovered   int                      `json:"files_discovered"`
	FilesTruncated    bool                     `json:"files_truncated"`
	SessionsTruncated bool                     `json:"sessions_truncated"`
	ModelTimeSeries   []CodexModelTimePointDTO `json:"model_time_series"`
	GeneratedAt       time.Time                `json:"generated_at"`
	SourceStatus      string                   `json:"source_status"`
	SourceLabel       string                   `json:"source_label"`
	PrivacyMode       string                   `json:"privacy_mode"`
	LastError         string                   `json:"last_error,omitempty"`
	FilesScanned      int                      `json:"files_scanned"`
	ParseErrors       int                      `json:"parse_errors"`
	Summary           CodexSummaryDTO          `json:"summary"`
	RateLimits        *CodexRateLimits         `json:"rate_limits,omitempty"`
	TimeSeries        []CodexTimePointDTO      `json:"time_series"`
	Models            []CodexModelDTO          `json:"models"`
	Sessions          []CodexSessionDTO        `json:"sessions"`
}

type CodexModelTimePointDTO struct {
	CodexTimePointDTO
	ModelName string `json:"model_name"`
}

type codexUsageSample struct {
	Timestamp time.Time
	Model     string
	Usage     CodexTokenUsage
}

type codexActivitySample struct {
	Timestamp time.Time
	Kind      string
	Success   bool
	Known     bool
}

type codexUsageAggregate struct {
	CodexTokenUsage
	Calls int64
}

type codexParsedSession struct {
	Path          string
	Size          int64
	ModTime       time.Time
	SessionID     string
	Workspace     string
	Model         string
	ModelProvider string
	Originator    string
	StartedAt     time.Time
	UpdatedAt     time.Time
	Usage         []codexUsageSample
	Activities    []codexActivitySample
	RateLimits    *CodexRateLimits
	ParseErrors   int
	TurnEnded     bool
	WorkspaceID   string
}

type codexLogRecord struct {
	Timestamp string          `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

type codexTokenCountInfo struct {
	TotalTokenUsage    *CodexTokenUsage `json:"total_token_usage"`
	LastTokenUsage     *CodexTokenUsage `json:"last_token_usage"`
	ModelContextWindow int64            `json:"model_context_window"`
}

type codexPayload struct {
	Role             string               `json:"role"`
	ResponseID       string               `json:"response_id"`
	ThreadTokenUsage *CodexTokenUsage     `json:"thread_token_usage"`
	Type             string               `json:"type"`
	ID               string               `json:"id"`
	SessionID        string               `json:"session_id"`
	Timestamp        string               `json:"timestamp"`
	CWD              string               `json:"cwd"`
	Model            string               `json:"model"`
	ModelProvider    string               `json:"model_provider"`
	Originator       string               `json:"originator"`
	Usage            *CodexTokenUsage     `json:"usage"`
	Item             *codexEventItem      `json:"item"`
	RateLimits       json.RawMessage      `json:"rate_limits"`
	Info             *codexTokenCountInfo `json:"info"`
}

type codexEventItem struct {
	Type     string `json:"type"`
	ExitCode *int   `json:"exit_code"`
	Status   string `json:"status"`
}

type codexRateLimitPayload struct {
	LimitID   string                `json:"limit_id"`
	LimitName string                `json:"limit_name"`
	PlanType  string                `json:"plan_type"`
	Primary   *CodexRateLimitWindow `json:"primary"`
	Secondary *CodexRateLimitWindow `json:"secondary"`
	Credits   *struct {
		HasCredits bool            `json:"has_credits"`
		Unlimited  bool            `json:"unlimited"`
		Balance    json.RawMessage `json:"balance"`
	} `json:"credits"`
}

type CodexMonitor struct {
	cfg             config.OpenAIMonitorConfig
	sessionsDir     string
	mu              sync.RWMutex
	refreshMu       sync.Mutex
	files           map[string]*codexParsedSession
	filesDiscovered int
	lastScan        time.Time
	lastError       string
	ctx             context.Context
	cancel          context.CancelFunc
	wg              sync.WaitGroup
}

func NewCodexMonitor(cfg config.OpenAIMonitorConfig) *CodexMonitor {
	ctx, cancel := context.WithCancel(context.Background())
	return &CodexMonitor{
		cfg:         cfg,
		sessionsDir: resolveCodexSessionsDir(cfg.SessionsDir),
		files:       make(map[string]*codexParsedSession),
		ctx:         ctx,
		cancel:      cancel,
	}
}

func resolveCodexSessionsDir(configured string) string {
	configured = strings.TrimSpace(os.ExpandEnv(configured))
	if configured == "" {
		if codexHome := strings.TrimSpace(os.Getenv("CODEX_HOME")); codexHome != "" {
			return filepath.Join(codexHome, "sessions")
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return filepath.Join(".codex", "sessions")
		}
		return filepath.Join(home, ".codex", "sessions")
	}
	if configured == "~" || strings.HasPrefix(configured, "~\\") || strings.HasPrefix(configured, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			configured = filepath.Join(home, strings.TrimLeft(configured[1:], "\\/"))
		}
	}
	return filepath.Clean(configured)
}

func (m *CodexMonitor) Start() {
	if !m.cfg.Enabled {
		return
	}
	if err := m.Refresh(); err != nil {
		if os.IsNotExist(err) || errors.Is(err, os.ErrNotExist) {
			log.Printf("[INFO] 🤖 OpenAI/Codex monitor: thư mục sessions chưa tồn tại (%s), sẵn sàng tự động nhận diện khi có session mới", m.sessionsDir)
		} else {
			log.Printf("[WARN] OpenAI/Codex monitor chưa thể quét session cục bộ: %v", err)
		}
	} else {
		log.Printf("[INFO] 🤖 OpenAI/Codex monitor chỉ-đọc đã khởi chạy tại %s", m.sessionsDir)
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
					// Nếu thư mục chưa tồn tại, bỏ qua không spam log
					if os.IsNotExist(err) || errors.Is(err, os.ErrNotExist) {
						continue
					}
					errMsg := err.Error()
					if errMsg != lastLoggedErr {
						log.Printf("[WARN] OpenAI/Codex monitor refresh thất bại: %v", err)
						lastLoggedErr = errMsg
					}
				} else {
					lastLoggedErr = ""
				}
			}
		}
	}()
}

func (m *CodexMonitor) Stop() {
	m.cancel()
	m.wg.Wait()
}

// Refresh scans only *.jsonl files below the configured sessions directory.
// It never traverses ~/.codex/auth.json or credential stores. Prompt/message fields pass through
// the JSONL scanner but are never decoded into application structs, retained, or emitted.
func (m *CodexMonitor) Refresh() error {
	if !m.cfg.Enabled {
		return nil
	}
	m.refreshMu.Lock()
	defer m.refreshMu.Unlock()

	entries, err := collectCodexSessionFiles(m.sessionsDir, 0)
	if err != nil {
		m.mu.Lock()
		m.lastScan = time.Now()
		m.lastError = "Không thể đọc thư mục session Codex; kiểm tra đường dẫn và quyền truy cập."
		m.mu.Unlock()
		return err
	}

	discovered := len(entries)
	if m.cfg.MaxFiles > 0 && len(entries) > m.cfg.MaxFiles {
		entries = entries[:m.cfg.MaxFiles]
	}
	scanFailures := 0
	m.mu.RLock()
	old := make(map[string]*codexParsedSession, len(m.files))
	for path, parsed := range m.files {
		old[path] = parsed
	}
	m.mu.RUnlock()

	next := make(map[string]*codexParsedSession, len(entries))
	for _, entry := range entries {
		if cached, ok := old[entry.path]; ok && cached.Size == entry.size && cached.ModTime.Equal(entry.modTime) {
			next[entry.path] = cached
			continue
		}
		parsed, parseErr := parseCodexSessionFile(entry.path, entry.size, entry.modTime)
		if parseErr != nil {
			scanFailures++
			if cached, ok := old[entry.path]; ok {
				cachedCopy := *cached
				cachedCopy.ParseErrors++
				next[entry.path] = &cachedCopy
			}
			continue
		}
		next[entry.path] = parsed
	}

	m.mu.Lock()
	m.files = next
	m.filesDiscovered = discovered
	m.lastScan = time.Now()
	m.lastError = ""
	if scanFailures > 0 {
		m.lastError = fmt.Sprintf("Không đọc được %d file session Codex.", scanFailures)
	}
	m.mu.Unlock()
	if scanFailures > 0 {
		return errors.New("Codex session scan incomplete")
	}
	return nil
}

type codexFileEntry struct {
	path    string
	size    int64
	modTime time.Time
}

func collectCodexSessionFiles(root string, maxFiles int) ([]codexFileEntry, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("không thể truy cập thư mục Codex sessions: %w", err)
	}
	if !info.IsDir() {
		return nil, errors.New("đường dẫn Codex sessions không phải thư mục")
	}

	entries := make([]codexFileEntry, 0, 64)
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || strings.ToLower(filepath.Ext(d.Name())) != ".jsonl" {
			return nil
		}
		if strings.EqualFold(d.Name(), "auth.json") {
			return nil
		}
		fileInfo, infoErr := d.Info()
		if infoErr != nil {
			return infoErr
		}
		if !fileInfo.Mode().IsRegular() {
			return nil
		}
		entries = append(entries, codexFileEntry{path: path, size: fileInfo.Size(), modTime: fileInfo.ModTime()})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].modTime.Equal(entries[j].modTime) {
			return entries[i].path < entries[j].path
		}
		return entries[i].modTime.After(entries[j].modTime)
	})
	if maxFiles > 0 && len(entries) > maxFiles {
		entries = entries[:maxFiles]
	}
	return entries, nil
}

func parseCodexSessionFile(path string, size int64, modTime time.Time) (*codexParsedSession, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	parsed := &codexParsedSession{
		Path:    path,
		Size:    size,
		ModTime: modTime,
		Model:   "OpenAI (unknown)",
	}
	scanner := bufio.NewScanner(f)
	scanner.Split(func(data []byte, atEOF bool) (int, []byte, error) {
		if atEOF && !bytes.Contains(data, []byte("\n")) && !json.Valid(bytes.TrimSpace(data)) {
			return len(data), nil, nil
		}
		return bufio.ScanLines(data, atEOF)
	})
	scanner.Buffer(make([]byte, 64*1024), codexScannerMaxToken)
	currentModel := parsed.Model
	var previous CodexTokenUsage
	seenResponses := make(map[string]bool)
	var responseActivities, itemActivities []codexActivitySample
	for scanner.Scan() {
		var record codexLogRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			parsed.ParseErrors++
			continue
		}
		ts := parseCodexTimestamp(record.Timestamp)
		if ts.IsZero() {
			ts = modTime
		}
		if parsed.StartedAt.IsZero() || ts.Before(parsed.StartedAt) {
			parsed.StartedAt = ts
		}
		if parsed.UpdatedAt.IsZero() || ts.After(parsed.UpdatedAt) {
			parsed.UpdatedAt = ts
		}

		if record.Type != "session_meta" && record.Type != "turn_context" && record.Type != "token_usage_record" && record.Type != "event_msg" && record.Type != "response_item" {
			continue
		}
		var payload codexPayload
		if len(record.Payload) > 0 && json.Unmarshal(record.Payload, &payload) != nil {
			parsed.ParseErrors++
			continue
		}
		switch record.Type {
		case "session_meta":
			parsed.SessionID = firstNonEmpty(payload.SessionID, payload.ID, parsed.SessionID)
			parsed.Workspace = safeWorkspaceName(payload.CWD)
			parsed.WorkspaceID = fmt.Sprintf("%x", sha256.Sum256([]byte(payload.CWD)))
			parsed.ModelProvider = payload.ModelProvider
			parsed.Originator = payload.Originator
			if metaTime := parseCodexTimestamp(payload.Timestamp); !metaTime.IsZero() {
				parsed.StartedAt = metaTime
			}
		case "turn_context":
			if strings.TrimSpace(payload.Model) != "" {
				currentModel = strings.TrimSpace(payload.Model)
				parsed.Model = currentModel
			}
			if parsed.Workspace == "" {
				parsed.Workspace = safeWorkspaceName(payload.CWD)
			}
		case "token_usage_record":
			if payload.Usage != nil && (payload.ResponseID == "" || !seenResponses[payload.ResponseID]) {
				if payload.ResponseID != "" {
					seenResponses[payload.ResponseID] = true
				}
				if payload.ThreadTokenUsage != nil {
					appendCodexCumulative(parsed, *payload.ThreadTokenUsage, &previous, ts, currentModel)
				} else {
					parsed.Usage = append(parsed.Usage, codexUsageSample{Timestamp: ts, Model: currentModel, Usage: *payload.Usage})
					addCodexCounters(&previous, *payload.Usage)
				}
			}
		case "response_item":
			if payload.Type == "function_call" || payload.Type == "custom_tool_call" || payload.Type == "local_shell_call" {
				responseActivities = append(responseActivities, codexActivitySample{Timestamp: ts, Kind: "tool"})
			}
		case "event_msg":
			before := len(parsed.Activities)
			parseCodexEvent(parsed, payload, ts, currentModel, &previous)
			if payload.Type == "item_completed" && len(parsed.Activities) > before && parsed.Activities[len(parsed.Activities)-1].Kind == "tool" {
				itemActivities = append(itemActivities, parsed.Activities[len(parsed.Activities)-1])
				parsed.Activities = parsed.Activities[:before]
			}
		}
	}
	// Rollouts expose tool invocations separately; opaque outputs do not establish success.
	if len(responseActivities) > 0 {
		parsed.Activities = append(parsed.Activities, responseActivities...)
	} else {
		parsed.Activities = append(parsed.Activities, itemActivities...)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if parsed.SessionID == "" {
		parsed.SessionID = sessionIDFromFilename(path)
	}
	if parsed.Workspace == "" {
		parsed.Workspace = "Không rõ workspace"
	}
	if parsed.UpdatedAt.IsZero() {
		parsed.UpdatedAt = modTime
	}
	return parsed, nil
}

func parseCodexEvent(parsed *codexParsedSession, payload codexPayload, ts time.Time, currentModel string, previous *CodexTokenUsage) {
	switch payload.Type {
	case "task_complete", "turn_aborted":
		parsed.TurnEnded = true
	case "task_started":
		parsed.TurnEnded = false
		parsed.Activities = append(parsed.Activities, codexActivitySample{Timestamp: ts, Kind: "turn", Success: true})
	case "item_completed":
		if payload.Item == nil {
			return
		}
		switch payload.Item.Type {
		case "AgentMessage":
			parsed.Activities = append(parsed.Activities, codexActivitySample{Timestamp: ts, Kind: "assistant", Success: true})
		case "CommandExecution", "Extension":
			success := true
			if payload.Item.ExitCode != nil {
				success = *payload.Item.ExitCode == 0
			}
			if strings.EqualFold(payload.Item.Status, "failed") || strings.EqualFold(payload.Item.Status, "error") {
				success = false
			}
			parsed.Activities = append(parsed.Activities, codexActivitySample{Timestamp: ts, Kind: "tool", Success: success, Known: payload.Item.ExitCode != nil || payload.Item.Status == "completed" || payload.Item.Status == "failed" || payload.Item.Status == "error"})
		}
	case "token_count":
		if payload.Info != nil {
			tot := payload.Info.TotalTokenUsage
			if tot != nil {
				appendCodexCumulative(parsed, *tot, previous, ts, currentModel)
			}
		}
		if len(payload.RateLimits) == 0 || string(payload.RateLimits) == "null" {
			return
		}
		var source codexRateLimitPayload
		if err := json.Unmarshal(payload.RateLimits, &source); err != nil {
			parsed.ParseErrors++
			return
		}
		rate := &CodexRateLimits{
			LimitID: source.LimitID, LimitName: source.LimitName, PlanType: source.PlanType,
			Primary: source.Primary, Secondary: source.Secondary, ObservedAt: ts,
		}
		if source.Credits != nil {
			rate.Credits = &CodexCredits{
				HasCredits: source.Credits.HasCredits,
				Unlimited:  source.Credits.Unlimited,
				Balance:    rawScalarString(source.Credits.Balance),
			}
		}
		if rate.LimitID == "" || rate.LimitID == "codex" {
			parsed.RateLimits = rate
		}
	}
}

// Both record formats describe the same thread counters. Use component deltas once.
func appendCodexCumulative(parsed *codexParsedSession, total CodexTokenUsage, previous *CodexTokenUsage, ts time.Time, model string) {
	if total == *previous {
		return
	}
	delta := CodexTokenUsage{InputTokens: total.InputTokens - previous.InputTokens, CachedInputTokens: total.CachedInputTokens - previous.CachedInputTokens, CacheWriteInputTokens: total.CacheWriteInputTokens - previous.CacheWriteInputTokens, OutputTokens: total.OutputTokens - previous.OutputTokens, ReasoningOutputTokens: total.ReasoningOutputTokens - previous.ReasoningOutputTokens, TotalTokens: total.TotalTokens - previous.TotalTokens}
	*previous = total
	if delta.TotalTokens < 0 || delta.InputTokens < 0 || delta.OutputTokens < 0 || delta.CachedInputTokens < 0 || delta.ReasoningOutputTokens < 0 || delta.CacheWriteInputTokens < 0 {
		// A compaction/reset changes the baseline; it is not new billed usage.
		parsed.ParseErrors++
		return
	}
	if delta.TotalTokens > 0 {
		parsed.Usage = append(parsed.Usage, codexUsageSample{Timestamp: ts, Model: model, Usage: delta})
	}
}

func addCodexCounters(target *CodexTokenUsage, usage CodexTokenUsage) {
	target.InputTokens += usage.InputTokens
	target.CachedInputTokens += usage.CachedInputTokens
	target.CacheWriteInputTokens += usage.CacheWriteInputTokens
	target.OutputTokens += usage.OutputTokens
	target.ReasoningOutputTokens += usage.ReasoningOutputTokens
	target.TotalTokens += usage.TotalTokens
}

func rawScalarString(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	return strings.Trim(string(raw), "\"")
}

func parseCodexTimestamp(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05.999999999Z07:00", "2006-01-02 15:04:05"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

func safeWorkspaceName(cwd string) string {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		return ""
	}
	clean := filepath.Clean(cwd)
	name := filepath.Base(clean)
	if name == "." || name == string(filepath.Separator) || name == "" {
		return "Local workspace"
	}
	return name
}

func sessionIDFromFilename(path string) string {
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	parts := strings.Split(name, "-")
	if len(parts) >= 5 {
		return strings.Join(parts[len(parts)-5:], "-")
	}
	return name
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func (m *CodexMonitor) Dashboard(timeRange string) CodexDashboardDTO {
	return m.dashboard(timeRange, true)
}

// DashboardForAggregation returns every monitored session, without the UI row limit.
func (m *CodexMonitor) DashboardForAggregation(timeRange string) CodexDashboardDTO {
	return m.dashboard(timeRange, false)
}

func (m *CodexMonitor) dashboard(timeRange string, limitRows bool) CodexDashboardDTO {
	timeRange = strings.ToLower(strings.TrimSpace(timeRange))
	if timeRange == "1d" {
		timeRange = "24h"
	}
	m.mu.RLock()
	files := make([]*codexParsedSession, 0, len(m.files))
	for _, session := range m.files {
		files = append(files, session)
	}
	lastScan := m.lastScan
	lastError := m.lastError
	discovered := m.filesDiscovered
	m.mu.RUnlock()

	now := time.Now()
	cutoff := codexRangeCutoff(now, timeRange)
	result := CodexDashboardDTO{
		LastScanAt:      lastScan,
		Coverage:        "LOCAL_SESSION_LOGS_ONLY_NOT_ACCOUNT_USAGE",
		FilesDiscovered: discovered,
		FilesTruncated:  m.cfg.MaxFiles > 0 && discovered > m.cfg.MaxFiles,
		ModelTimeSeries: []CodexModelTimePointDTO{},
		GeneratedAt:     now,
		SourceStatus:    "READY",
		SourceLabel:     "Configured Codex sessions/**/*.jsonl",
		PrivacyMode:     "READ_ONLY_NO_CREDENTIALS_NO_PROMPT_CONTENT",
		LastError:       lastError,
		FilesScanned:    len(files),
		TimeSeries:      []CodexTimePointDTO{},
		Models:          []CodexModelDTO{},
		Sessions:        []CodexSessionDTO{},
	}
	if !m.cfg.Enabled {
		result.SourceStatus = "DISABLED"
		return result
	}
	if lastError != "" {
		result.SourceStatus = "UNAVAILABLE"
		if len(files) > 0 {
			result.SourceStatus = "PARTIAL"
		}
	} else if len(files) == 0 {
		result.SourceStatus = "EMPTY"
	} else if lastScan.IsZero() {
		result.SourceStatus = "STARTING"
	}

	buckets := make(map[string]*codexUsageAggregate)
	models := make(map[string]*codexUsageAggregate)
	modelBuckets := make(map[string]map[string]*codexUsageAggregate)
	var firstUsage, lastUsage time.Time
	var latestRate *CodexRateLimits

	sort.Slice(files, func(i, j int) bool {
		if files[i].ModTime.Equal(files[j].ModTime) {
			return files[i].Path < files[j].Path
		}
		return files[i].ModTime.After(files[j].ModTime)
	})
	seenSessions := make(map[string]bool)
	for _, session := range files {
		if seenSessions[session.SessionID] {
			continue
		}
		seenSessions[session.SessionID] = true
		row := CodexSessionDTO{
			SessionID: session.SessionID, Workspace: session.Workspace, WorkspaceID: session.WorkspaceID, Model: session.Model,
			ModelProvider: session.ModelProvider, Originator: session.Originator,
			StartedAt: session.StartedAt, UpdatedAt: session.UpdatedAt, Status: "IDLE",
		}
		if session.TurnEnded {
			row.Status = "COMPLETED"
		}
		lastActivity := session.UpdatedAt
		if lastActivity.IsZero() {
			lastActivity = session.ModTime
		}
		activityAge := now.Sub(lastActivity)
		if !session.TurnEnded && activityAge >= 0 && activityAge <= 2*time.Minute {
			row.Status = "ACTIVE"
		}
		if !row.StartedAt.IsZero() && !row.UpdatedAt.IsZero() {
			row.DurationMinutes = row.UpdatedAt.Sub(row.StartedAt).Minutes()
			if row.DurationMinutes < 0 {
				row.DurationMinutes = 0
			}
		}
		if session.RateLimits != nil && (latestRate == nil || session.RateLimits.ObservedAt.After(latestRate.ObservedAt)) {
			copyRate := *session.RateLimits
			copyRate.SessionID = session.SessionID
			copyRate.Stale = now.Sub(copyRate.ObservedAt) > 2*time.Minute || (copyRate.Primary != nil && copyRate.Primary.ResetsAt > 0 && copyRate.Primary.ResetsAt <= now.Unix())
			latestRate = &copyRate
		}
		var rowInput, rowCached int64
		for _, sample := range session.Usage {
			if !cutoff.IsZero() && sample.Timestamp.Before(cutoff) {
				continue
			}
			addCodexUsage(&result.Summary, sample.Usage)
			row.TotalTokens += sample.Usage.TotalTokens
			row.InputTokens += sample.Usage.InputTokens
			row.CachedInputTokens += sample.Usage.CachedInputTokens
			row.OutputTokens += sample.Usage.OutputTokens
			row.ReasoningTokens += sample.Usage.ReasoningOutputTokens
			row.ModelCalls++
			rowInput += sample.Usage.InputTokens
			rowCached += sample.Usage.CachedInputTokens
			result.Summary.ModelCalls++
			bucketKey := codexBucketKey(sample.Timestamp, timeRange)
			bucket := buckets[bucketKey]
			if bucket == nil {
				bucket = &codexUsageAggregate{}
				buckets[bucketKey] = bucket
			}
			addCodexAggregate(bucket, sample.Usage)
			modelName := firstNonEmpty(sample.Model, session.Model, "OpenAI (unknown)")
			modelAgg := models[modelName]
			if modelAgg == nil {
				modelAgg = &codexUsageAggregate{}
				models[modelName] = modelAgg
			}
			addCodexAggregate(modelAgg, sample.Usage)
			if modelBuckets[bucketKey] == nil {
				modelBuckets[bucketKey] = make(map[string]*codexUsageAggregate)
			}
			if modelBuckets[bucketKey][modelName] == nil {
				modelBuckets[bucketKey][modelName] = &codexUsageAggregate{}
			}
			addCodexAggregate(modelBuckets[bucketKey][modelName], sample.Usage)
			if firstUsage.IsZero() || sample.Timestamp.Before(firstUsage) {
				firstUsage = sample.Timestamp
			}
			if lastUsage.IsZero() || sample.Timestamp.After(lastUsage) {
				lastUsage = sample.Timestamp
			}
		}
		if rowInput > 0 {
			row.CacheHitPercent = float64(rowCached) * 100 / float64(rowInput)
		}
		for _, activity := range session.Activities {
			if !cutoff.IsZero() && activity.Timestamp.Before(cutoff) {
				continue
			}
			switch activity.Kind {
			case "turn":
				row.Turns++
				result.Summary.Turns++
			case "assistant":
				row.AssistantResponses++
				result.Summary.AssistantResponses++
			case "tool":
				row.ToolCalls++
				result.Summary.ToolCalls++
				if activity.Known {
					result.Summary.ToolResultsKnown++
				}
				if activity.Known && !activity.Success {
					row.ToolFailures++
					result.Summary.ToolFailures++
				}
			}
		}
		if row.TotalTokens > 0 || row.Turns > 0 || row.ToolCalls > 0 || cutoff.IsZero() || !session.UpdatedAt.Before(cutoff) {
			result.Summary.Sessions++
			if row.Status == "ACTIVE" {
				result.Summary.ActiveSessions++
			}
			result.Sessions = append(result.Sessions, row)
		}
		result.ParseErrors += session.ParseErrors
	}

	result.RateLimits = latestRate
	if result.Summary.InputTokens > 0 {
		result.Summary.CacheHitPercent = float64(result.Summary.CachedInputTokens) * 100 / float64(result.Summary.InputTokens)
	}
	if result.Summary.ToolResultsKnown > 0 {
		result.Summary.ToolSuccessPercent = float64(result.Summary.ToolResultsKnown-result.Summary.ToolFailures) * 100 / float64(result.Summary.ToolResultsKnown)
	}
	if result.Summary.Turns > 0 {
		result.Summary.AvgTokensPerTurn = float64(result.Summary.TotalTokens) / float64(result.Summary.Turns)
	}
	if !firstUsage.IsZero() && !lastUsage.IsZero() {
		minutes := lastUsage.Sub(firstUsage).Minutes()
		if minutes < 1 {
			minutes = 1
		}
		result.Summary.TokensPerMinute = float64(result.Summary.TotalTokens) / minutes
	}

	bucketKeys := make([]string, 0, len(buckets))
	for key := range buckets {
		bucketKeys = append(bucketKeys, key)
	}
	sort.Strings(bucketKeys)
	for _, key := range bucketKeys {
		a := buckets[key]
		result.TimeSeries = append(result.TimeSeries, CodexTimePointDTO{
			TimeBucket: key, InputTokens: a.InputTokens, CachedTokens: a.CachedInputTokens,
			OutputTokens: a.OutputTokens, ReasoningTokens: a.ReasoningOutputTokens,
			TotalTokens: a.TotalTokens, ModelCalls: a.Calls,
		})
	}
	for _, key := range bucketKeys {
		names := make([]string, 0, len(modelBuckets[key]))
		for name := range modelBuckets[key] {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			a := modelBuckets[key][name]
			result.ModelTimeSeries = append(result.ModelTimeSeries, CodexModelTimePointDTO{ModelName: name, CodexTimePointDTO: CodexTimePointDTO{TimeBucket: key, InputTokens: a.InputTokens, CachedTokens: a.CachedInputTokens, OutputTokens: a.OutputTokens, ReasoningTokens: a.ReasoningOutputTokens, TotalTokens: a.TotalTokens, ModelCalls: a.Calls}})
		}
	}
	if result.ParseErrors > 0 || result.FilesTruncated {
		result.SourceStatus = "PARTIAL"
	}
	for name, a := range models {
		pct := 0.0
		if result.Summary.TotalTokens > 0 {
			pct = float64(a.TotalTokens) * 100 / float64(result.Summary.TotalTokens)
		}
		result.Models = append(result.Models, CodexModelDTO{ModelName: name, TotalTokens: a.TotalTokens, ModelCalls: a.Calls, TokenPct: pct})
	}
	sort.Slice(result.Models, func(i, j int) bool { return result.Models[i].TotalTokens > result.Models[j].TotalTokens })
	sort.Slice(result.Sessions, func(i, j int) bool { return result.Sessions[i].UpdatedAt.After(result.Sessions[j].UpdatedAt) })
	if limitRows && m.cfg.MaxSessionRows > 0 && len(result.Sessions) > m.cfg.MaxSessionRows {
		result.SessionsTruncated = true
		result.Sessions = result.Sessions[:m.cfg.MaxSessionRows]
	}
	return result
}

func addCodexUsage(summary *CodexSummaryDTO, usage CodexTokenUsage) {
	summary.InputTokens += usage.InputTokens
	summary.CachedInputTokens += usage.CachedInputTokens
	summary.CacheWriteTokens += usage.CacheWriteInputTokens
	summary.OutputTokens += usage.OutputTokens
	summary.ReasoningTokens += usage.ReasoningOutputTokens
	summary.TotalTokens += usage.TotalTokens
}

func addCodexAggregate(target *codexUsageAggregate, usage CodexTokenUsage) {
	target.InputTokens += usage.InputTokens
	target.CachedInputTokens += usage.CachedInputTokens
	target.CacheWriteInputTokens += usage.CacheWriteInputTokens
	target.OutputTokens += usage.OutputTokens
	target.ReasoningOutputTokens += usage.ReasoningOutputTokens
	target.TotalTokens += usage.TotalTokens
	target.Calls++
}

func codexRangeCutoff(now time.Time, timeRange string) time.Time {
	switch strings.ToLower(strings.TrimSpace(timeRange)) {
	case "today":
		year, month, day := now.Date()
		return time.Date(year, month, day, 0, 0, 0, 0, now.Location())
	case "24h":
		return now.Add(-24 * time.Hour)
	case "7d":
		return now.AddDate(0, 0, -7)
	case "all":
		return time.Time{}
	default:
		return now.AddDate(0, 0, -30)
	}
}

func codexBucketKey(ts time.Time, timeRange string) string {
	local := ts.Local()
	if timeRange == "today" || timeRange == "24h" {
		return local.Format("2006-01-02 15:00")
	}
	return local.Format("2006-01-02")
}

// Graph contains observed workspaces and sessions only. ACTIVE means recent log activity.
func (m *CodexMonitor) Graph(timeRange string) *AITopologyGraphDTO {
	d := m.dashboard(timeRange, false)
	g := &AITopologyGraphDTO{Projects: []AITopologyProjectDTO{}, Nodes: []AITopologyNodeDTO{}, Links: []AITopologyLinkDTO{}, Categories: []AITopologyCategoryDTO{{Name: "Workspaces"}, {Name: "Codex sessions"}, {Name: ""}, {Name: ""}, {Name: ""}, {Name: ""}, {Name: ""}, {Name: "Local logs"}}, ActiveConcurrency: int(d.Summary.ActiveSessions), ActiveSessions: int(d.Summary.ActiveSessions)}
	if len(d.Sessions) == 0 {
		return g
	}
	rootID := "root-openai-profile"
	g.Nodes = append(g.Nodes, AITopologyNodeDTO{ID: rootID, Name: "Codex local sessions", Category: 7, SymbolSize: 64, Tokens: d.Summary.TotalTokens, TaskCount: int(d.Summary.Sessions), Status: "IDLE", Role: "Local log aggregate"})
	groups := make(map[string][]CodexSessionDTO)
	for _, row := range d.Sessions {
		key := firstNonEmpty(row.WorkspaceID, row.Workspace)
		groups[key] = append(groups[key], row)
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		rows := groups[key]
		name := rows[0].Workspace
		id := fmt.Sprintf("proj-codex-%x", sha256.Sum256([]byte(key)))
		var tokens int64
		active := 0
		for _, row := range rows {
			tokens += row.TotalTokens
			if row.Status == "ACTIVE" {
				active++
			}
		}
		status := "IDLE"
		if active > 0 {
			status = "ACTIVE"
			g.Nodes[0].Status = "RUNNING"
		}
		g.Projects = append(g.Projects, AITopologyProjectDTO{ID: id, Name: name, TotalTokens: tokens, TotalTasks: len(rows), ActiveAgents: active, Status: status})
		g.Nodes = append(g.Nodes, AITopologyNodeDTO{ID: id, Name: name, Category: 0, SymbolSize: 48, Project: name, ProjectID: id, Tokens: tokens, TaskCount: len(rows), Status: status, Role: "Workspace aggregate"})
		g.Links = append(g.Links, AITopologyLinkDTO{Source: rootID, Target: id, Type: "WORKSPACE", Tokens: tokens, Interactions: len(rows), Status: status})
		for _, row := range rows {
			sid := "session-codex-" + row.SessionID
			g.Nodes = append(g.Nodes, AITopologyNodeDTO{ID: sid, Name: row.Model, Category: 1, SymbolSize: 32, Project: name, ProjectID: id, Tokens: row.TotalTokens, TaskCount: int(row.Turns), Status: row.Status, Role: "Observed session", LastTask: row.SessionID})
			g.Links = append(g.Links, AITopologyLinkDTO{Source: id, Target: sid, Type: "SESSION", Tokens: row.TotalTokens, Interactions: int(row.Turns), Status: row.Status})
		}
	}
	return g
}

func formatCodexTokens(tokens int64) string {
	if tokens >= 1_000_000 {
		return fmt.Sprintf("%.2fM tok", float64(tokens)/1_000_000.0)
	}
	if tokens >= 1_000 {
		return fmt.Sprintf("%dk tok", tokens/1_000)
	}
	return fmt.Sprintf("%d tok", tokens)
}
