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
	CacheHitPercent    float64   `json:"cache_hit_percent"`
}

type CodexDashboardDTO struct {
	GeneratedAt  time.Time           `json:"generated_at"`
	SourceStatus string              `json:"source_status"`
	SourceLabel  string              `json:"source_label"`
	PrivacyMode  string              `json:"privacy_mode"`
	LastError    string              `json:"last_error,omitempty"`
	FilesScanned int                 `json:"files_scanned"`
	ParseErrors  int                 `json:"parse_errors"`
	Summary      CodexSummaryDTO     `json:"summary"`
	RateLimits   *CodexRateLimits    `json:"rate_limits,omitempty"`
	TimeSeries   []CodexTimePointDTO `json:"time_series"`
	Models       []CodexModelDTO     `json:"models"`
	Sessions     []CodexSessionDTO   `json:"sessions"`
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
	Type          string               `json:"type"`
	ID            string               `json:"id"`
	SessionID     string               `json:"session_id"`
	Timestamp     string               `json:"timestamp"`
	CWD           string               `json:"cwd"`
	Model         string               `json:"model"`
	ModelProvider string               `json:"model_provider"`
	Originator    string               `json:"originator"`
	Usage         *CodexTokenUsage     `json:"usage"`
	Item          *codexEventItem      `json:"item"`
	RateLimits    json.RawMessage      `json:"rate_limits"`
	Info          *codexTokenCountInfo `json:"info"`
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
	cfg         config.OpenAIMonitorConfig
	sessionsDir string
	mu          sync.RWMutex
	refreshMu   sync.Mutex
	files       map[string]*codexParsedSession
	lastScan    time.Time
	lastError   string
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
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

	entries, err := collectCodexSessionFiles(m.sessionsDir, m.cfg.MaxFiles)
	if err != nil {
		m.mu.Lock()
		m.lastScan = time.Now()
		m.lastError = err.Error()
		m.mu.Unlock()
		return err
	}

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
	m.lastScan = time.Now()
	m.lastError = ""
	m.mu.Unlock()
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
			return nil
		}
		if d.IsDir() || strings.ToLower(filepath.Ext(d.Name())) != ".jsonl" {
			return nil
		}
		if strings.EqualFold(d.Name(), "auth.json") {
			return nil
		}
		fileInfo, infoErr := d.Info()
		if infoErr != nil || !fileInfo.Mode().IsRegular() {
			return nil
		}
		entries = append(entries, codexFileEntry{path: path, size: fileInfo.Size(), modTime: fileInfo.ModTime()})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].modTime.After(entries[j].modTime) })
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
	scanner.Buffer(make([]byte, 64*1024), codexScannerMaxToken)
	currentModel := parsed.Model
	var prevTotalTokens int64
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

		var payload codexPayload
		if len(record.Payload) > 0 && json.Unmarshal(record.Payload, &payload) != nil {
			parsed.ParseErrors++
			continue
		}
		switch record.Type {
		case "session_meta":
			parsed.SessionID = firstNonEmpty(payload.SessionID, payload.ID, parsed.SessionID)
			parsed.Workspace = safeWorkspaceName(payload.CWD)
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
			if payload.Usage != nil {
				parsed.Usage = append(parsed.Usage, codexUsageSample{Timestamp: ts, Model: currentModel, Usage: *payload.Usage})
			}
		case "event_msg":
			parseCodexEvent(parsed, payload, ts, currentModel, &prevTotalTokens)
		}
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

func parseCodexEvent(parsed *codexParsedSession, payload codexPayload, ts time.Time, currentModel string, prevTotalTokens *int64) {
	switch payload.Type {
	case "task_started":
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
			parsed.Activities = append(parsed.Activities, codexActivitySample{Timestamp: ts, Kind: "tool", Success: success})
		}
	case "token_count":
		if payload.Info != nil {
			tot := payload.Info.TotalTokenUsage
			last := payload.Info.LastTokenUsage
			if tot != nil && last != nil && prevTotalTokens != nil {
				if tot.TotalTokens > *prevTotalTokens {
					*prevTotalTokens = tot.TotalTokens
					parsed.Usage = append(parsed.Usage, codexUsageSample{
						Timestamp: ts,
						Model:     currentModel,
						Usage:     *last,
					})
				}
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
		parsed.RateLimits = rate
	}
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
	m.mu.RLock()
	files := make([]*codexParsedSession, 0, len(m.files))
	for _, session := range m.files {
		files = append(files, session)
	}
	lastScan := m.lastScan
	lastError := m.lastError
	m.mu.RUnlock()

	now := time.Now()
	cutoff := codexRangeCutoff(now, timeRange)
	result := CodexDashboardDTO{
		GeneratedAt:  now,
		SourceStatus: "READY",
		SourceLabel:  "~/.codex/sessions/**/*.jsonl",
		PrivacyMode:  "READ_ONLY_NO_CREDENTIALS_NO_PROMPT_CONTENT",
		LastError:    lastError,
		FilesScanned: len(files),
		TimeSeries:   []CodexTimePointDTO{},
		Models:       []CodexModelDTO{},
		Sessions:     []CodexSessionDTO{},
	}
	if !m.cfg.Enabled {
		result.SourceStatus = "DISABLED"
		return result
	}
	if lastError != "" {
		result.SourceStatus = "UNAVAILABLE"
	} else if len(files) == 0 {
		result.SourceStatus = "EMPTY"
	} else if lastScan.IsZero() {
		result.SourceStatus = "STARTING"
	}

	buckets := make(map[string]*codexUsageAggregate)
	models := make(map[string]*codexUsageAggregate)
	var firstUsage, lastUsage time.Time
	var latestRate *CodexRateLimits

	for _, session := range files {
		row := CodexSessionDTO{
			SessionID: session.SessionID, Workspace: session.Workspace, Model: session.Model,
			ModelProvider: session.ModelProvider, Originator: session.Originator,
			StartedAt: session.StartedAt, UpdatedAt: session.UpdatedAt, Status: "COMPLETED",
		}
		lastActivity := session.UpdatedAt
		if lastActivity.IsZero() {
			lastActivity = session.ModTime
		}
		activityAge := now.Sub(lastActivity)
		if activityAge >= 0 && activityAge <= 2*time.Minute {
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
			latestRate = &copyRate
		}
		for _, sample := range session.Usage {
			if !cutoff.IsZero() && sample.Timestamp.Before(cutoff) {
				continue
			}
			addCodexUsage(&result.Summary, sample.Usage)
			row.TotalTokens += sample.Usage.TotalTokens
			row.CacheHitPercent += float64(sample.Usage.CachedInputTokens)
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
			if firstUsage.IsZero() || sample.Timestamp.Before(firstUsage) {
				firstUsage = sample.Timestamp
			}
			if lastUsage.IsZero() || sample.Timestamp.After(lastUsage) {
				lastUsage = sample.Timestamp
			}
		}
		var rowInput, rowCached int64
		for _, sample := range session.Usage {
			if !cutoff.IsZero() && sample.Timestamp.Before(cutoff) {
				continue
			}
			rowInput += sample.Usage.InputTokens
			rowCached += sample.Usage.CachedInputTokens
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
				if !activity.Success {
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
	if result.Summary.ToolCalls > 0 {
		result.Summary.ToolSuccessPercent = float64(result.Summary.ToolCalls-result.Summary.ToolFailures) * 100 / float64(result.Summary.ToolCalls)
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
	for name, a := range models {
		pct := 0.0
		if result.Summary.TotalTokens > 0 {
			pct = float64(a.TotalTokens) * 100 / float64(result.Summary.TotalTokens)
		}
		result.Models = append(result.Models, CodexModelDTO{ModelName: name, TotalTokens: a.TotalTokens, ModelCalls: a.Calls, TokenPct: pct})
	}
	sort.Slice(result.Models, func(i, j int) bool { return result.Models[i].TotalTokens > result.Models[j].TotalTokens })
	sort.Slice(result.Sessions, func(i, j int) bool { return result.Sessions[i].UpdatedAt.After(result.Sessions[j].UpdatedAt) })
	if m.cfg.MaxSessionRows > 0 && len(result.Sessions) > m.cfg.MaxSessionRows {
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

// Graph tạo đồ thị mạng lưới phân cấp 4 tầng tương tác cho OpenAI/Codex theo chuẩn interactive_topology_engine.
func (m *CodexMonitor) Graph(timeRange string) *AITopologyGraphDTO {
	dashboard := m.Dashboard(timeRange)

	categories := []AITopologyCategoryDTO{
		{Name: "🏢 Dự Án (Workspaces)"},
		{Name: "🤖 Codex Primary Engine"},
		{Name: "💻 Shell / CLI Executor"},
		{Name: "📝 File Patcher & Diff"},
		{Name: "🔍 Semantic AST Search"},
		{Name: "🌐 External Connector"},
		{Name: "⚡ Task Subworker"},
		{Name: "👑 Root Controller & Profile"},
	}

	// Nhóm các session theo Workspace
	workspaceMap := make(map[string]*struct {
		Name     string
		Tokens   int64
		Sessions int
		Status   string
	})

	for _, s := range dashboard.Sessions {
		wName := s.Workspace
		if wName == "" {
			wName = "Codex Local Workspace"
		} else {
			wName = filepath.Base(wName)
		}
		item := workspaceMap[wName]
		if item == nil {
			item = &struct {
				Name     string
				Tokens   int64
				Sessions int
				Status   string
			}{Name: wName, Status: s.Status}
			workspaceMap[wName] = item
		}
		item.Tokens += s.TotalTokens
		item.Sessions++
		if s.Status == "ACTIVE" {
			item.Status = "ACTIVE"
		}
	}

	if len(workspaceMap) == 0 {
		return &AITopologyGraphDTO{
			Projects:          []AITopologyProjectDTO{},
			Nodes:             []AITopologyNodeDTO{},
			Links:             []AITopologyLinkDTO{},
			Categories:        categories,
			ActiveConcurrency: 0,
			ActiveSessions:    0,
		}
	}

	rootID := "root-openai-profile"
	rootStatus := "COMPLETED"
	if dashboard.Summary.ActiveSessions > 0 {
		rootStatus = "RUNNING"
	}

	rootX := 1500.0
	rootY := 40.0
	nodes := []AITopologyNodeDTO{
		{
			ID:         rootID,
			Name:       "OpenAI Codex • Local Session Fleet",
			Category:   7,
			SymbolSize: 64,
			Role:       "Root Controller • OpenAI / Codex CLI Engine",
			Project:    "OpenAI Workspace Ecosystem",
			ProjectID:  "root",
			Status:     rootStatus,
			Tokens:     dashboard.Summary.TotalTokens,
			TaskCount:  int(dashboard.Summary.Sessions),
			LastTask:   fmt.Sprintf("%d sessions • %d model calls", dashboard.Summary.Sessions, dashboard.Summary.ModelCalls),
			X:          &rootX,
			Y:          &rootY,
			Fixed:      true,
		},
	}

	links := []AITopologyLinkDTO{}
	projectsDTO := []AITopologyProjectDTO{}

	totalWorkspaces := len(workspaceMap)
	spacingX := 620.0
	startX := rootX
	if totalWorkspaces > 1 {
		startX = rootX - (float64(totalWorkspaces-1)/2.0)*spacingX
	}

	idx := 0
	for wName, ws := range workspaceMap {
		pX := startX + float64(idx)*spacingX
		pY := 180.0
		projID := "proj-codex-" + strings.ToLower(strings.ReplaceAll(wName, " ", "-"))

		nodes = append(nodes, AITopologyNodeDTO{
			ID:         projID,
			Name:       wName,
			Category:   0,
			SymbolSize: 56,
			Role:       "Project Hub",
			Project:    wName,
			ProjectID:  projID,
			Status:     ws.Status,
			Tokens:     ws.Tokens,
			TaskCount:  ws.Sessions,
			LastTask:   "Active workspace",
			X:          &pX,
			Y:          &pY,
			Fixed:      true,
		})

		activeAgents := 0
		if ws.Status == "ACTIVE" {
			activeAgents = 5
		}

		projectsDTO = append(projectsDTO, AITopologyProjectDTO{
			ID:           projID,
			Name:         wName,
			ActiveAgents: activeAgents,
			TotalTasks:   ws.Sessions,
			TotalTokens:  ws.Tokens,
			Status:       ws.Status,
		})

		links = append(links, AITopologyLinkDTO{
			Source:       rootID,
			Target:       projID,
			Label:        fmt.Sprintf("Điều phối • %s", formatCodexTokens(ws.Tokens)),
			Tokens:       ws.Tokens,
			Interactions: ws.Sessions,
			Type:         "ROOT_ORCHESTRATION",
			Status:       ws.Status,
		})

		orchID := fmt.Sprintf("orch-%s", projID)
		orchX := pX
		orchY := 320.0
		nodes = append(nodes, AITopologyNodeDTO{
			ID:         orchID,
			Name:       fmt.Sprintf("Codex Engine (%s)", wName),
			Category:   1,
			SymbolSize: 48,
			Role:       "Primary Orchestrator",
			Project:    wName,
			ProjectID:  projID,
			Status:     ws.Status,
			Tokens:     ws.Tokens,
			TaskCount:  ws.Sessions,
			LastTask:   "Dispatching Codex actions & model inference",
			X:          &orchX,
			Y:          &orchY,
			Fixed:      true,
		})

		links = append(links, AITopologyLinkDTO{
			Source:       projID,
			Target:       orchID,
			Label:        "Session Channel",
			Tokens:       ws.Tokens,
			Interactions: ws.Sessions,
			Type:         "SESSION",
			Status:       ws.Status,
		})

		toolsDef := []struct {
			role    string
			cat     int
			name    string
			offsetX float64
			offsetY float64
		}{
			{"Shell Exec", 2, "Shell Terminal", -220, 140},
			{"File Patch", 3, "File Patcher", 0, 130},
			{"AST Analysis", 4, "Semantic Search", 120, 220},
			{"Connectors", 5, "External Tooling", -120, 220},
			{"Subtasks", 6, "Subtask Runner", 220, 140},
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
				Project:    wName,
				ProjectID:  projID,
				Status:     ws.Status,
				Tokens:     ws.Tokens / 5,
				TaskCount:  ws.Sessions * 2,
				LastTask:   "Tool action execution",
				X:          &subX,
				Y:          &subY,
				Fixed:      true,
			})

			links = append(links, AITopologyLinkDTO{
				Source:       orchID,
				Target:       subID,
				Label:        fmt.Sprintf("⚡ %s", td.name),
				Tokens:       ws.Tokens / 5,
				Interactions: ws.Sessions,
				Type:         "DELEGATION",
				Status:       ws.Status,
			})
		}
		idx++
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

func formatCodexTokens(tokens int64) string {
	if tokens >= 1_000_000 {
		return fmt.Sprintf("%.2fM tok", float64(tokens)/1_000_000.0)
	}
	if tokens >= 1_000 {
		return fmt.Sprintf("%dk tok", tokens/1_000)
	}
	return fmt.Sprintf("%d tok", tokens)
}
