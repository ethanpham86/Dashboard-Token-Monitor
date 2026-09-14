package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"tokenmonitor/collector"
	"tokenmonitor/config"
	"tokenmonitor/storage"
	"tokenmonitor/web"
)

func TestTokenMonitorEndToEnd(t *testing.T) {
	tmpDB := "./test_token_monitor.db"
	defer os.Remove(tmpDB)

	cfg := &config.Config{
		Server: config.ServerConfig{
			DashboardPort: 9099,
			BindAddress:   "127.0.0.1",
		},
		Database: config.DatabaseConfig{
			SQLitePath:    tmpDB,
			MaxOpenConns:  5,
			MaxIdleConns:  2,
			EnableWALMode: true,
		},
		AccountProfile: config.AccountProfileConfig{
			Email:              "testuser@gmail.com",
			AccountType:        "Google Consumer Account (Individual)",
			PlanName:           "20X ULTRA PLAN",
			QuotaBandwidth:     "20x Quota Bandwidth",
			SubscriptionStart:  "2026-09-06",
			SubscriptionExpiry: "2026-10-06",
			AutoRenew:          true,
		},
	}

	// 1. Test Khởi tạo Storage SQLite
	store, err := storage.NewStorage(cfg)
	if err != nil {
		t.Fatalf("Khởi tạo storage thất bại: %v", err)
	}
	defer store.Close()

	// 2. Test Nạp Account Profile
	acc, err := store.GetAccountProfile()
	if err != nil {
		t.Fatalf("GetAccountProfile thất bại: %v", err)
	}
	if acc.Email == "" {
		t.Errorf("Email không được để trống")
	}
	if acc.TokenStatus != "VALID" {
		t.Errorf("Kỳ vọng token status VALID nhưng nhận %s", acc.TokenStatus)
	}
	if acc.SubscriptionExpiry.IsZero() {
		t.Errorf("Hạn dùng subscription không được là IsZero")
	}

	// 3. Test Parse Gemini usageMetadata với Thinking Tokens và Cached Tokens
	rawJSON := `{
		"candidates": [{"content": {"parts": [{"text": "Hello world"}]}}],
		"usageMetadata": {
			"promptTokenCount": 150000,
			"candidatesTokenCount": 25000,
			"totalTokenCount": 175000,
			"cachedContentTokenCount": 1200000,
			"candidatesTokensDetails": [
				{"modality": "TEXT", "tokenCount": 18000},
				{"modality": "THINKING", "tokenCount": 7000}
			]
		},
		"modelVersion": "gemini-3.8-flash"
	}`

	var resp collector.GeminiResponseWrapper
	if err := json.Unmarshal([]byte(rawJSON), &resp); err != nil {
		t.Fatalf("Parse json response thất bại: %v", err)
	}
	if resp.UsageMetadata == nil {
		t.Fatal("usageMetadata bị nil")
	}
	if resp.UsageMetadata.GetThinkingTokens() != 7000 {
		t.Errorf("Kỳ vọng Thinking Tokens 7000, nhận: %d", resp.UsageMetadata.GetThinkingTokens())
	}
	if resp.UsageMetadata.CachedContentTokenCount != 1200000 {
		t.Errorf("Kỳ vọng Cached Tokens 1200000, nhận: %d", resp.UsageMetadata.CachedContentTokenCount)
	}

	// 4. Test Ring Buffer & Batch Insert
	buf := collector.NewAsyncBuffer(store, 100, 10, 50*time.Millisecond)
	buf.Start()

	event := &collector.TokenUsageEvent{
		AccountID:      acc.ID,
		Timestamp:      time.Now(),
		ModelName:      "Gemini 3.8 Flash (High)",
		PromptTokens:   resp.UsageMetadata.PromptTokenCount,
		OutputTokens:   resp.UsageMetadata.CandidatesTokenCount,
		ThinkingTokens: resp.UsageMetadata.GetThinkingTokens(),
		CachedTokens:   resp.UsageMetadata.CachedContentTokenCount,
		TotalTokens:    resp.UsageMetadata.TotalTokenCount,
		LatencyMs:      950,
		StatusCode:     200,
		RequestType:    "INTERACTIVE",
	}
	buf.Push(event)

	// Chờ buffer flush
	time.Sleep(150 * time.Millisecond)
	buf.Stop()

	// 5. Test Query Thống kê
	metrics, err := store.GetSummaryMetrics()
	if err != nil {
		t.Fatalf("GetSummaryMetrics thất bại: %v", err)
	}
	if metrics.TotalGrandTokens != 175000 {
		t.Errorf("Kỳ vọng TotalGrandTokens 175000, nhận: %d", metrics.TotalGrandTokens)
	}
	if metrics.ThinkingTokens != 7000 {
		t.Errorf("Kỳ vọng ThinkingTokens 7000, nhận: %d", metrics.ThinkingTokens)
	}
	if metrics.CachedTokens != 1200000 {
		t.Errorf("Kỳ vọng CachedTokens 1200000, nhận: %d", metrics.CachedTokens)
	}

	// 6. Test Web Handler Endpoints
	webServer := web.NewServer(store, buf, nil)
	router := webServer.Routes()

	req := httptest.NewRequest("GET", "/healthz", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("Kỳ vọng /healthz trả về 200, nhận: %d", w.Code)
	}

	reqMetrics := httptest.NewRequest("GET", "/api/metrics/summary", nil)
	wMetrics := httptest.NewRecorder()
	router.ServeHTTP(wMetrics, reqMetrics)
	if wMetrics.Code != http.StatusOK {
		t.Errorf("Kỳ vọng /api/metrics/summary trả về 200, nhận: %d", wMetrics.Code)
	}

	// 7. Test ECharts Model TimeSeries Endpoint
	reqECharts := httptest.NewRequest("GET", "/api/metrics/models/timeseries?range=30d", nil)
	wECharts := httptest.NewRecorder()
	router.ServeHTTP(wECharts, reqECharts)
	if wECharts.Code != http.StatusOK {
		t.Errorf("Kỳ vọng /api/metrics/models/timeseries trả về 200, nhận: %d", wECharts.Code)
	}

	// 7b. Test Models Distribution with range
	reqModels := httptest.NewRequest("GET", "/api/metrics/models?range=24h", nil)
	wModels := httptest.NewRecorder()
	router.ServeHTTP(wModels, reqModels)
	if wModels.Code != http.StatusOK {
		t.Errorf("Kỳ vọng /api/metrics/models?range=24h trả về 200, nhận: %d", wModels.Code)
	}

	// 7c. Test Daily History with range
	reqDaily := httptest.NewRequest("GET", "/api/metrics/daily?range=24h", nil)
	wDaily := httptest.NewRecorder()
	router.ServeHTTP(wDaily, reqDaily)
	if wDaily.Code != http.StatusOK {
		t.Errorf("Kỳ vọng /api/metrics/daily?range=24h trả về 200, nhận: %d", wDaily.Code)
	}

	// 7d. Test Agent Fleet Summary Endpoint
	reqFleetSum := httptest.NewRequest("GET", "/api/agents/summary", nil)
	wFleetSum := httptest.NewRecorder()
	router.ServeHTTP(wFleetSum, reqFleetSum)
	if wFleetSum.Code != http.StatusOK {
		t.Errorf("Kỳ vọng /api/agents/summary trả về 200, nhận: %d", wFleetSum.Code)
	}

	// 7e. Test Agent Concurrency Timeline Endpoint
	reqFleetConc := httptest.NewRequest("GET", "/api/agents/concurrency", nil)
	wFleetConc := httptest.NewRecorder()
	router.ServeHTTP(wFleetConc, reqFleetConc)
	if wFleetConc.Code != http.StatusOK {
		t.Errorf("Kỳ vọng /api/agents/concurrency trả về 200, nhận: %d", wFleetConc.Code)
	}

	// 7f. Test Agent Gantt Tasks Endpoint
	reqFleetGantt := httptest.NewRequest("GET", "/api/agents/gantt", nil)
	wFleetGantt := httptest.NewRecorder()
	router.ServeHTTP(wFleetGantt, reqFleetGantt)
	if wFleetGantt.Code != http.StatusOK {
		t.Errorf("Kỳ vọng /api/agents/gantt trả về 200, nhận: %d", wFleetGantt.Code)
	}

	// 8. Test Dynamic Model Parsing
	testCases := []struct {
		content  string
		expected string
	}{
		{
			content:  "<USER_SETTINGS_CHANGE>\nThe user changed setting `Model Selection` from None to Gemini 3.8 Flash (High).\n</USER_SETTINGS_CHANGE>",
			expected: "Gemini 3.8 Flash (High)",
		},
		{
			content:  "<USER_SETTINGS_CHANGE>\nThe user changed setting `Model Selection` from Gemini 3.8 Flash (High) to Claude Sonnet 4.6 (Thinking).\n</USER_SETTINGS_CHANGE>",
			expected: "Claude Sonnet 4.6 (Thinking)",
		},
		{
			content:  "<USER_SETTINGS_CHANGE>\nThe user changed setting `Model Selection` from None to Gemini 3.1 Pro High.\n</USER_SETTINGS_CHANGE>",
			expected: "Gemini 3.1 Pro (High)",
		},
		{
			content:  "<USER_SETTINGS_CHANGE>\nThe user changed setting `Model Selection` from None to GPT-OSS 120B (Medium).\n</USER_SETTINGS_CHANGE>",
			expected: "GPT-OSS 120B (Medium)",
		},
		{
			content:  "<USER_SETTINGS_CHANGE>\nThe user changed setting `Model Selection` from None to Gemini 3.7 Flash Medium (Fast).\n</USER_SETTINGS_CHANGE>",
			expected: "Gemini 3.7 Flash Medium",
		},
	}

	for _, tc := range testCases {
		res := collector.ExtractModelName(tc.content)
		if res != tc.expected {
			t.Errorf("ExtractModelName thất bại: kỳ vọng '%s', nhận '%s'", tc.expected, res)
		}
	}
}

func TestInspectActualDB(t *testing.T) {
	dbPath := `./data/token_monitor.db`
	if _, err := os.Stat(dbPath); err != nil {
		t.Skip("db not found")
	}

	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=ro", filepath.ToSlash(dbPath)))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	tables := []string{"accounts", "token_usage_logs", "token_usage_hourly_rollup", "agent_fleet_telemetry"}
	for _, tbl := range tables {
		var cnt int
		_ = db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", tbl)).Scan(&cnt)
		t.Logf("Table %s: %d rows", tbl, cnt)
	}

	var minDate, maxDate string
	var totalTokens int64
	var totalCalls int
	_ = db.QueryRow("SELECT MIN(timestamp), MAX(timestamp), SUM(total_tokens), COUNT(*) FROM token_usage_logs").Scan(&minDate, &maxDate, &totalTokens, &totalCalls)
	t.Logf("token_usage_logs: %d calls, %d tokens, from %s to %s", totalCalls, totalTokens, minDate, maxDate)

	var distinctModels int
	_ = db.QueryRow("SELECT COUNT(DISTINCT model_name) FROM token_usage_logs").Scan(&distinctModels)
	t.Logf("Distinct models: %d", distinctModels)
	rows, _ := db.Query("SELECT model_name, COUNT(*), SUM(total_tokens) FROM token_usage_logs GROUP BY model_name ORDER BY SUM(total_tokens) DESC")
	for rows.Next() {
		var m string
		var c int
		var tok int64
		_ = rows.Scan(&m, &c, &tok)
		t.Logf("   Model %s: %d calls, %d tokens", m, c, tok)
	}
	rows.Close()
}

