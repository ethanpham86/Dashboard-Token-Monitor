package web

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"strings"
	"time"

	"tokenmonitor/collector"
	"tokenmonitor/storage"
)

//go:embed static/*
var staticFS embed.FS

type Server struct {
	storage       *storage.Storage
	buffer        *collector.AsyncBuffer
	tailer        *collector.LocalTailer
	codexMonitor  *collector.CodexMonitor
	claudeMonitor *collector.ClaudeMonitor
}

func NewServer(s *storage.Storage, b *collector.AsyncBuffer, t *collector.LocalTailer, codex ...*collector.CodexMonitor) *Server {
	server := &Server{storage: s, buffer: b, tailer: t}
	if len(codex) > 0 {
		server.codexMonitor = codex[0]
	}
	return server
}

func (s *Server) SetClaudeMonitor(m *collector.ClaudeMonitor) {
	s.claudeMonitor = m
}

func (s *Server) SetCodexMonitor(m *collector.CodexMonitor) {
	s.codexMonitor = m
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	// Health Check
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":    "UP",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
	})

	// API Endpoints
	mux.HandleFunc("/api/account", s.handleAccountProfile)
	mux.HandleFunc("/api/metrics/summary", s.handleSummaryMetrics)
	mux.HandleFunc("/api/metrics/timeseries", s.handleTimeSeries)
	mux.HandleFunc("/api/metrics/daily", s.handleDailyMetrics)
	mux.HandleFunc("/api/metrics/models", s.handleModelDistribution)
	mux.HandleFunc("/api/metrics/models/timeseries", s.handleModelTimeSeries)
	mux.HandleFunc("/api/sync/history", s.handleSyncHistory)
	mux.HandleFunc("/api/test/simulate", s.handleSimulateEvent)
	mux.HandleFunc("/api/agents/summary", s.handleAgentFleetSummary)
	mux.HandleFunc("/api/agents/concurrency", s.handleAgentConcurrencyTimeline)
	mux.HandleFunc("/api/agents/gantt", s.handleAgentGanttTasks)
	mux.HandleFunc("/api/agents/gantt/packets", s.handleAgentGanttPackets)
	mux.HandleFunc("/api/agents/graph", s.handleAgentTopologyGraph)
	mux.HandleFunc("/api/openai/dashboard", s.handleOpenAIDashboard)
	mux.HandleFunc("/api/openai/refresh", s.handleOpenAIRefresh)
	mux.HandleFunc("/api/openai/graph", s.handleOpenAIGraph)
	mux.HandleFunc("/api/claude/dashboard", s.handleClaudeDashboard)
	mux.HandleFunc("/api/claude/refresh", s.handleClaudeRefresh)
	mux.HandleFunc("/api/claude/graph", s.handleClaudeGraph)
	mux.HandleFunc("/api/projects/leaderboard", s.handleProjectsLeaderboard)

	// Static Web Assets: Ưu tiên phục vụ trực tiếp từ disk nếu file tồn tại (hỗ trợ hot-reload), fallback sang embedFS
	if _, err := os.Stat("web/static/index.html"); err == nil {
		mux.Handle("/", http.FileServer(http.Dir("web/static")))
	} else if subFS, err := fs.Sub(staticFS, "static"); err == nil {
		mux.Handle("/", http.FileServer(http.FS(subFS)))
	}

	// Docs Portal Route
	mux.Handle("/docs/", http.StripPrefix("/docs/", http.FileServer(http.Dir("./docs"))))

	// Config File Route (for Docs Portal)
	mux.HandleFunc("/config.yaml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		http.ServeFile(w, r, "config.yaml")
	})

	return mux
}

func (s *Server) handleOpenAIDashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if s.codexMonitor == nil {
		_ = json.NewEncoder(w).Encode(collector.CodexDashboardDTO{
			GeneratedAt: time.Now(), SourceStatus: "DISABLED",
			SourceLabel: "~/.codex/sessions/**/*.jsonl",
			PrivacyMode: "READ_ONLY_NO_CREDENTIALS_NO_PROMPT_CONTENT",
			TimeSeries:  []collector.CodexTimePointDTO{}, Models: []collector.CodexModelDTO{}, Sessions: []collector.CodexSessionDTO{},
		})
		return
	}
	timeRange := r.URL.Query().Get("range")
	if timeRange == "" {
		timeRange = "30d"
	}
	_ = json.NewEncoder(w).Encode(s.codexMonitor.Dashboard(timeRange))
}

func (s *Server) handleOpenAIRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.codexMonitor == nil {
		http.Error(w, "OpenAI/Codex monitor is disabled", http.StatusServiceUnavailable)
		return
	}
	if err := s.codexMonitor.Refresh(); err != nil {
		http.Error(w, "Không thể quét đầy đủ log Codex; kiểm tra cấu hình và quyền đọc.", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"message": "Đã quét lại log Codex cục bộ", "status": "READY"})
}

func (s *Server) handleOpenAIGraph(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	timeRange := r.URL.Query().Get("range")
	if s.codexMonitor == nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"projects": []any{}, "nodes": []any{}, "links": []any{}, "categories": []any{}})
		return
	}
	_ = json.NewEncoder(w).Encode(s.codexMonitor.Graph(timeRange))
}

func (s *Server) handleClaudeDashboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	timeRange := r.URL.Query().Get("range")
	if s.claudeMonitor == nil {
		_ = json.NewEncoder(w).Encode(collector.ClaudeDashboardDTO{
			SourceStatus: "DISABLED",
			SourceLabel:  "~/.claude/projects/**/*.jsonl",
			PrivacyMode:  "READ_ONLY_NO_CREDENTIALS_NO_PROMPT_CONTENT",
			TimeSeries:   []collector.ClaudeTimePointDTO{},
			Models:       []collector.ClaudeModelDTO{},
			Tools:        []collector.ClaudeToolDTO{},
			Projects:     []collector.ClaudeProjectDTO{},
			Sessions:     []collector.ClaudeSessionDTO{},
		})
		return
	}
	_ = json.NewEncoder(w).Encode(s.claudeMonitor.Dashboard(timeRange))
}

func (s *Server) handleClaudeRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.claudeMonitor == nil {
		http.Error(w, "Claude monitor is disabled", http.StatusServiceUnavailable)
		return
	}
	if err := s.claudeMonitor.Refresh(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"message": "Đã quét lại log Claude cục bộ", "status": "READY"})
}

func (s *Server) handleClaudeGraph(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	timeRange := r.URL.Query().Get("range")
	if s.claudeMonitor == nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"projects": []any{}, "nodes": []any{}, "links": []any{}, "categories": []any{}})
		return
	}
	_ = json.NewEncoder(w).Encode(s.claudeMonitor.Graph(timeRange))
}

func (s *Server) handleAccountProfile(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	profile, err := s.storage.GetAccountProfile()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(profile)
}

func (s *Server) handleSummaryMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	timeRange := r.URL.Query().Get("range")
	if timeRange == "" {
		timeRange = "all"
	}
	summary, err := s.storage.GetSummaryMetricsByRange(timeRange)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(summary)
}

func (s *Server) handleTimeSeries(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	timeRange := r.URL.Query().Get("range")
	if timeRange == "" {
		timeRange = "24h"
	}
	points, err := s.storage.GetTimeSeriesDataByRange(timeRange)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(points)
}

func (s *Server) handleDailyMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	timeRange := r.URL.Query().Get("range")
	if timeRange == "" {
		daysStr := r.URL.Query().Get("days")
		if daysStr != "" {
			timeRange = daysStr + "d"
		} else {
			timeRange = "30d"
		}
	}
	list, err := s.storage.GetDailySummariesByRange(timeRange)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(list)
}

func (s *Server) handleSyncHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	synced := 0
	if s.tailer != nil {
		synced, _ = s.tailer.BackfillAllHistory()
	}
	_ = s.storage.EnsureMonthlyHistory()
	_ = json.NewEncoder(w).Encode(map[string]any{
		"message":      "Đã đồng bộ toàn bộ lịch sử thành công",
		"events_found": synced,
	})
}

func (s *Server) handleModelDistribution(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	timeRange := r.URL.Query().Get("range")
	if timeRange == "" {
		timeRange = "all"
	}
	models, err := s.storage.GetModelDistribution(timeRange)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(models)
}

func (s *Server) handleModelTimeSeries(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	timeRange := r.URL.Query().Get("range")
	if timeRange == "" {
		timeRange = "30d"
	}
	model := r.URL.Query().Get("model")
	points, err := s.storage.GetModelTimeSeries(timeRange, model)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(points)
}

func (s *Server) handleSimulateEvent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	model := r.URL.Query().Get("model")
	if model == "" {
		model = "Gemini 3.8 Flash (High)"
	}

	event := &collector.TokenUsageEvent{
		AccountID:      1,
		Timestamp:      time.Now(),
		ModelName:      model,
		PromptTokens:   420000,
		OutputTokens:   58000,
		ThinkingTokens: 18000,
		CachedTokens:   3800000,
		TotalTokens:    478000,
		LatencyMs:      1150,
		StatusCode:     200,
		RequestType:    "INTERACTIVE",
	}

	s.buffer.Push(event)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"message": "Event đã được nạp thành công vào buffer",
		"event":   event,
	})
}

func (s *Server) handleAgentFleetSummary(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	timeRange := r.URL.Query().Get("range")
	summary, err := s.storage.GetAgentFleetSummary(timeRange)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(summary)
}

func (s *Server) handleAgentConcurrencyTimeline(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	timeRange := r.URL.Query().Get("range")
	points, err := s.storage.GetAgentConcurrencyTimeline(timeRange)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(points)
}

func (s *Server) handleAgentGanttTasks(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	groupBy := r.URL.Query().Get("groupBy")
	timeRange := r.URL.Query().Get("range")
	tasks, err := s.storage.GetAgentGanttTasks(groupBy, timeRange)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(tasks)
}

func (s *Server) handleAgentGanttPackets(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	timeRange := r.URL.Query().Get("range")
	packets, err := s.storage.GetAgentGanttPackets(timeRange)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(packets)
}

func (s *Server) handleAgentTopologyGraph(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	project := r.URL.Query().Get("project")
	timeRange := r.URL.Query().Get("range")
	graph, err := s.storage.GetAgentTopologyGraph(project, timeRange)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(graph)
}

// handleProjectsLeaderboard phục vụ endpoint GET /api/projects/leaderboard
// Tổng hợp số liệu dự án đa nền tảng (Google Antigravity, OpenAI Codex, Anthropic Claude).
func (s *Server) handleProjectsLeaderboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 1. Phân tích và kiểm tra tính hợp lệ của tham số range
	timeRange := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("range")))
	if timeRange == "" {
		timeRange = "30d"
	}
	switch timeRange {
	case "today", "24h", "1d", "7d", "30d", "month", "all":
		// Hợp lệ
	default:
		http.Error(w, fmt.Sprintf("invalid range parameter: %q (allowed: today, 24h, 1d, 7d, 30d, month, all)", timeRange), http.StatusBadRequest)
		return
	}

	// 2. Phân tích và kiểm tra tính hợp lệ của tham số sort
	sortBy := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("sort")))
	if sortBy == "" {
		sortBy = "tokens"
	}
	switch sortBy {
	case "tokens", "cost", "activity":
		// Hợp lệ
	default:
		http.Error(w, fmt.Sprintf("invalid sort parameter: %q (allowed: tokens, cost, activity)", sortBy), http.StatusBadRequest)
		return
	}

	// 3. Kiểm tra tính sẵn sàng của tầng lưu trữ
	if s.storage == nil {
		http.Error(w, "storage backend unavailable", http.StatusServiceUnavailable)
		return
	}

	// 4. Tổng hợp dữ liệu đa LLM từ storage repository
	var codexProv storage.CodexProvider
	if s.codexMonitor != nil {
		codexProv = s.codexMonitor
	}
	var claudeProv storage.ClaudeProvider
	if s.claudeMonitor != nil {
		claudeProv = s.claudeMonitor
	}

	leaderboard, err := s.storage.GetProjectsLeaderboard(timeRange, sortBy, codexProv, claudeProv)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 5. Thiết lập Content-Type và tuần tự hóa JSON
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(leaderboard)
}
