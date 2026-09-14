package web_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tokenmonitor/collector"
	"tokenmonitor/config"
	"tokenmonitor/storage"
	"tokenmonitor/web"
)

func newCanonicalWebServer(t *testing.T) (*httptest.Server, func()) {
	t.Helper()

	// Ensure working directory is repository root so relative paths like config.yaml and ./docs resolve
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	if filepath.Base(wd) == "web" {
		_ = os.Chdir("..")
	}

	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "canonical_web_test.db")

	cfg := &config.Config{
		Database: config.DatabaseConfig{
			SQLitePath:    dbPath,
			MaxOpenConns:  5,
			MaxIdleConns:  2,
			EnableWALMode: true,
		},
		AccountProfile: config.AccountProfileConfig{
			Email:              "web_canonical@example.com",
			AccountType:        "Google Consumer Account (Individual)",
			PlanName:           "20X ULTRA PLAN",
			QuotaBandwidth:     "20x Quota Bandwidth",
			SubscriptionStart:  "2026-09-08",
			SubscriptionExpiry: "2026-10-08",
			AutoRenew:          true,
		},
	}

	store, err := storage.NewStorage(cfg)
	if err != nil {
		t.Fatalf("NewStorage failed: %v", err)
	}

	// Seed genuine test telemetry records for agent topology and gantt packet tests
	now := time.Now().UTC()
	if _, err := store.DB.Exec(`
		INSERT INTO agent_fleet_telemetry (account_id, subagent_id, role_name, task_name, status, started_at, finished_at, duration_ms, tokens_used, tokens_offloaded)
		VALUES 
		(1, 'sub-canonical-001', 'Codebase Explorer', 'Analyze storage schema', 'COMPLETED', ?, ?, 1200, 15000, 10000),
		(1, 'sub-canonical-002', 'Self-Branch Worker', 'Implement zero-mock architecture', 'RUNNING', ?, NULL, 0, 25000, 12000),
		(1, 'sub-canonical-003', 'Verification Tester', 'Run unit tests and verification', 'COMPLETED', ?, ?, 800, 8000, 4000),
		(1, 'sub-tch-001', 'Codebase Explorer', 'TieuChuanHardeningLinux: Inspect Huong_dan_cau_hinh_may_chu_Linux_v1.md', 'COMPLETED', ?, ?, 1600, 35000, 20000),
		(1, 'sub-tch-002', 'Self-Branch Worker', 'TieuChuanHardeningLinux: CIS RHEL8 profile automation', 'COMPLETED', ?, ?, 1400, 40000, 22000)
	`, now.Add(-30*time.Minute).Format("2006-01-02 15:04:05"),
		now.Add(-28*time.Minute).Format("2006-01-02 15:04:05"),
		now.Add(-20*time.Minute).Format("2006-01-02 15:04:05"),
		now.Add(-15*time.Minute).Format("2006-01-02 15:04:05"),
		now.Add(-14*time.Minute).Format("2006-01-02 15:04:05"),
		now.Add(-10*time.Minute).Format("2006-01-02 15:04:05"),
		now.Add(-8*time.Minute).Format("2006-01-02 15:04:05"),
		now.Add(-5*time.Minute).Format("2006-01-02 15:04:05"),
		now.Add(-3*time.Minute).Format("2006-01-02 15:04:05")); err != nil {
		t.Fatalf("Failed to seed test telemetry: %v", err)
	}

	buf := collector.NewAsyncBuffer(store, 1000, 100, 50*time.Millisecond)
	buf.Start()

	srv := web.NewServer(store, buf, nil)
	ts := httptest.NewServer(srv.Routes())

	cleanup := func() {
		ts.Close()
		buf.Stop()
		store.Close()
		if filepath.Base(wd) == "web" {
			_ = os.Chdir(wd)
		}
	}

	return ts, cleanup
}

// TestWebHandler_Canonical_AllRoutes verifies all required routes and API endpoints
func TestWebHandler_Canonical_AllRoutes(t *testing.T) {
	ts, cleanup := newCanonicalWebServer(t)
	defer cleanup()

	client := ts.Client()

	routes := []struct {
		name         string
		method       string
		path         string
		expectedCode int
		validate     func(t *testing.T, resp *http.Response, body []byte)
	}{
		{
			name:         "HealthCheck",
			method:       http.MethodGet,
			path:         "/healthz",
			expectedCode: http.StatusOK,
			validate: func(t *testing.T, resp *http.Response, body []byte) {
				var res map[string]any
				if err := json.Unmarshal(body, &res); err != nil {
					t.Fatalf("Failed to parse JSON from /healthz: %v", err)
				}
				if res["status"] != "UP" {
					t.Errorf("Expected status='UP', got %v", res["status"])
				}
			},
		},
		{
			name:         "OpenAIDashboard_DisabledSafePayload",
			method:       http.MethodGet,
			path:         "/api/openai/dashboard?range=24h",
			expectedCode: http.StatusOK,
			validate: func(t *testing.T, resp *http.Response, body []byte) {
				var dashboard collector.CodexDashboardDTO
				if err := json.Unmarshal(body, &dashboard); err != nil {
					t.Fatalf("Failed to parse JSON from /api/openai/dashboard: %v", err)
				}
				if dashboard.SourceStatus != "DISABLED" {
					t.Errorf("Expected disabled monitor payload, got %q", dashboard.SourceStatus)
				}
				if dashboard.PrivacyMode != "READ_ONLY_NO_CREDENTIALS_NO_PROMPT_CONTENT" {
					t.Errorf("Unexpected privacy mode: %q", dashboard.PrivacyMode)
				}
			},
		},
		{
			name:         "OpenAIGraph_SafePayload",
			method:       http.MethodGet,
			path:         "/api/openai/graph?range=all",
			expectedCode: http.StatusOK,
			validate: func(t *testing.T, resp *http.Response, body []byte) {
				var graph collector.AITopologyGraphDTO
				if err := json.Unmarshal(body, &graph); err != nil {
					t.Fatalf("Failed to parse JSON from /api/openai/graph: %v", err)
				}
			},
		},
		{
			name:         "ClaudeDashboard_DisabledSafePayload",
			method:       http.MethodGet,
			path:         "/api/claude/dashboard?range=today",
			expectedCode: http.StatusOK,
			validate: func(t *testing.T, resp *http.Response, body []byte) {
				var dashboard collector.ClaudeDashboardDTO
				if err := json.Unmarshal(body, &dashboard); err != nil {
					t.Fatalf("Failed to parse JSON from /api/claude/dashboard: %v", err)
				}
				if dashboard.SourceStatus != "DISABLED" {
					t.Errorf("Expected disabled monitor payload, got %q", dashboard.SourceStatus)
				}
				if dashboard.PrivacyMode != "READ_ONLY_NO_CREDENTIALS_NO_PROMPT_CONTENT" {
					t.Errorf("Unexpected privacy mode: %q", dashboard.PrivacyMode)
				}
			},
		},
		{
			name:         "ClaudeGraph_SafePayload",
			method:       http.MethodGet,
			path:         "/api/claude/graph?range=all",
			expectedCode: http.StatusOK,
			validate: func(t *testing.T, resp *http.Response, body []byte) {
				var graph collector.AITopologyGraphDTO
				if err := json.Unmarshal(body, &graph); err != nil {
					t.Fatalf("Failed to parse JSON from /api/claude/graph: %v", err)
				}
			},
		},
		{
			name:         "AccountProfile",
			method:       http.MethodGet,
			path:         "/api/account",
			expectedCode: http.StatusOK,
			validate: func(t *testing.T, resp *http.Response, body []byte) {
				var acc storage.AccountProfileDTO
				if err := json.Unmarshal(body, &acc); err != nil {
					t.Fatalf("Failed to parse JSON from /api/account: %v", err)
				}
				if acc.Email == "" {
					t.Errorf("Expected non-empty account email")
				}
				if acc.PlanName == "" {
					t.Errorf("Expected non-empty plan name")
				}
				if acc.TokenStatus != "VALID" {
					t.Errorf("Expected TokenStatus='VALID', got %q", acc.TokenStatus)
				}
			},
		},
		{
			name:         "SummaryMetrics_Default",
			method:       http.MethodGet,
			path:         "/api/metrics/summary",
			expectedCode: http.StatusOK,
			validate: func(t *testing.T, resp *http.Response, body []byte) {
				var sum storage.SummaryMetricsDTO
				if err := json.Unmarshal(body, &sum); err != nil {
					t.Fatalf("Failed to parse JSON from /api/metrics/summary: %v", err)
				}
			},
		},
		{
			name:         "SummaryMetrics_Range24h",
			method:       http.MethodGet,
			path:         "/api/metrics/summary?range=24h",
			expectedCode: http.StatusOK,
			validate: func(t *testing.T, resp *http.Response, body []byte) {
				var sum storage.SummaryMetricsDTO
				if err := json.Unmarshal(body, &sum); err != nil {
					t.Fatalf("Failed to parse JSON from /api/metrics/summary: %v", err)
				}
			},
		},
		{
			name:         "TimeSeries_24h",
			method:       http.MethodGet,
			path:         "/api/metrics/timeseries?range=24h",
			expectedCode: http.StatusOK,
			validate: func(t *testing.T, resp *http.Response, body []byte) {
				var pts []storage.ChartPointDTO
				if err := json.Unmarshal(body, &pts); err != nil {
					t.Fatalf("Failed to parse JSON from /api/metrics/timeseries: %v", err)
				}
			},
		},
		{
			name:         "DailyMetrics_30d",
			method:       http.MethodGet,
			path:         "/api/metrics/daily?range=30d",
			expectedCode: http.StatusOK,
			validate: func(t *testing.T, resp *http.Response, body []byte) {
				var daily []storage.DailySummaryDTO
				if err := json.Unmarshal(body, &daily); err != nil {
					t.Fatalf("Failed to parse JSON from /api/metrics/daily: %v", err)
				}
			},
		},
		{
			name:         "DailyMetrics_Days7",
			method:       http.MethodGet,
			path:         "/api/metrics/daily?days=7",
			expectedCode: http.StatusOK,
			validate: func(t *testing.T, resp *http.Response, body []byte) {
				var daily []storage.DailySummaryDTO
				if err := json.Unmarshal(body, &daily); err != nil {
					t.Fatalf("Failed to parse JSON from /api/metrics/daily: %v", err)
				}
			},
		},
		{
			name:         "ModelDistribution_All",
			method:       http.MethodGet,
			path:         "/api/metrics/models?range=all",
			expectedCode: http.StatusOK,
			validate: func(t *testing.T, resp *http.Response, body []byte) {
				var dist []storage.ModelDistributionDTO
				if err := json.Unmarshal(body, &dist); err != nil {
					t.Fatalf("Failed to parse JSON from /api/metrics/models: %v", err)
				}
			},
		},
		{
			name:         "ModelTimeSeries",
			method:       http.MethodGet,
			path:         "/api/metrics/models/timeseries?range=30d&model=gemini-2.5-pro",
			expectedCode: http.StatusOK,
			validate: func(t *testing.T, resp *http.Response, body []byte) {
				var pts []storage.ChartPointDTO
				if err := json.Unmarshal(body, &pts); err != nil {
					t.Fatalf("Failed to parse JSON from /api/metrics/models/timeseries: %v", err)
				}
			},
		},
		{
			name:         "ConfigFile_Yaml",
			method:       http.MethodGet,
			path:         "/config.yaml",
			expectedCode: http.StatusOK,
			validate: func(t *testing.T, resp *http.Response, body []byte) {
				ct := resp.Header.Get("Content-Type")
				if !strings.Contains(ct, "text/plain") {
					t.Errorf("Expected Content-Type text/plain, got %q", ct)
				}
				if !strings.Contains(string(body), "server:") {
					t.Errorf("Body missing expected 'server:' block")
				}
			},
		},
		{
			name:         "DocsPortal_Index",
			method:       http.MethodGet,
			path:         "/docs/",
			expectedCode: http.StatusOK,
			validate: func(t *testing.T, resp *http.Response, body []byte) {
				if !strings.Contains(string(body), "TokenMonitor") {
					t.Errorf("Expected /docs/ to contain 'TokenMonitor'")
				}
			},
		},
		{
			name:         "DocsPortal_ArchifyDiagram",
			method:       http.MethodGet,
			path:         "/docs/tokenmonitor-architecture.html",
			expectedCode: http.StatusOK,
			validate: func(t *testing.T, resp *http.Response, body []byte) {
				if !strings.Contains(string(body), "TokenMonitor") {
					t.Errorf("Expected diagram to contain 'TokenMonitor'")
				}
			},
		},
		{
			name:         "DocsPortal_PathTraversalBlocked",
			method:       http.MethodGet,
			path:         "/docs/../main.go",
			expectedCode: http.StatusNotFound, // Rejected safely by FileServer
			validate: func(t *testing.T, resp *http.Response, body []byte) {
				// Path must NOT leak main.go source code
				if strings.Contains(string(body), "package main") && strings.Contains(string(body), "func main()") {
					t.Errorf("Security violation: path traversal leaked main.go")
				}
			},
		},
		{
			name:         "AgentTopologyGraph_Default",
			method:       http.MethodGet,
			path:         "/api/agents/graph",
			expectedCode: http.StatusOK,
			validate: func(t *testing.T, resp *http.Response, body []byte) {
				var graph storage.AgentTopologyGraphDTO
				if err := json.Unmarshal(body, &graph); err != nil {
					t.Fatalf("Failed to parse JSON from /api/agents/graph: %v", err)
				}
				if len(graph.Nodes) == 0 {
					t.Errorf("Expected non-empty nodes in agent topology graph")
				}
				if len(graph.Links) == 0 {
					t.Errorf("Expected non-empty links in agent topology graph")
				}
				if len(graph.Projects) == 0 {
					t.Errorf("Expected non-empty projects in agent topology graph")
				}
				for _, node := range graph.Nodes {
					if node.X == nil || node.Y == nil {
						t.Errorf("Expected node %s to have deterministic X, Y coordinates", node.ID)
					}
					if !node.Fixed {
						t.Errorf("Expected node %s to be fixed", node.ID)
					}
				}
			},
		},
		{
			name:         "AgentTopologyGraph_FilterProject",
			method:       http.MethodGet,
			path:         "/api/agents/graph?project=proj-tokenmonitor",
			expectedCode: http.StatusOK,
			validate: func(t *testing.T, resp *http.Response, body []byte) {
				var graph storage.AgentTopologyGraphDTO
				if err := json.Unmarshal(body, &graph); err != nil {
					t.Fatalf("Failed to parse JSON from filtered /api/agents/graph: %v", err)
				}
				for _, node := range graph.Nodes {
					if node.ProjectID != "proj-tokenmonitor" {
						t.Errorf("Expected all nodes to belong to proj-tokenmonitor, got %s", node.ProjectID)
					}
				}
			},
		},
		{
			name:         "AgentTopologyGraph_FilterTieuChuanHardeningLinux",
			method:       http.MethodGet,
			path:         "/api/agents/graph?project=proj-tieuchuanhardeninglinux",
			expectedCode: http.StatusOK,
			validate: func(t *testing.T, resp *http.Response, body []byte) {
				var graph storage.AgentTopologyGraphDTO
				if err := json.Unmarshal(body, &graph); err != nil {
					t.Fatalf("Failed to parse JSON from /api/agents/graph?project=proj-tieuchuanhardeninglinux: %v", err)
				}
				if len(graph.Nodes) == 0 {
					t.Errorf("Expected nodes for proj-tieuchuanhardeninglinux, got 0")
				}
				for _, node := range graph.Nodes {
					if node.ProjectID != "proj-tieuchuanhardeninglinux" {
						t.Errorf("Expected all nodes to belong to proj-tieuchuanhardeninglinux, got %s", node.ProjectID)
					}
				}
			},
		},
		{
			name:         "AgentTopologyGraph_Range24h",
			method:       http.MethodGet,
			path:         "/api/agents/graph?range=24h",
			expectedCode: http.StatusOK,
			validate: func(t *testing.T, resp *http.Response, body []byte) {
				var graph storage.AgentTopologyGraphDTO
				if err := json.Unmarshal(body, &graph); err != nil {
					t.Fatalf("Failed to parse JSON from /api/agents/graph?range=24h: %v", err)
				}
				if len(graph.Nodes) == 0 {
					t.Errorf("Expected non-empty nodes in 24h topology graph")
				}
			},
		},
		{
			name:         "AgentTopologyGraph_Range7d",
			method:       http.MethodGet,
			path:         "/api/agents/graph?range=7d&project=proj-tokenmonitor",
			expectedCode: http.StatusOK,
			validate: func(t *testing.T, resp *http.Response, body []byte) {
				var graph storage.AgentTopologyGraphDTO
				if err := json.Unmarshal(body, &graph); err != nil {
					t.Fatalf("Failed to parse JSON from /api/agents/graph?range=7d: %v", err)
				}
				if len(graph.Nodes) == 0 {
					t.Errorf("Expected non-empty nodes in 7d filtered topology graph")
				}
			},
		},
		{
			name:         "AgentTopologyGraph_RangeToday",
			method:       http.MethodGet,
			path:         "/api/agents/graph?range=today",
			expectedCode: http.StatusOK,
			validate: func(t *testing.T, resp *http.Response, body []byte) {
				var graph storage.AgentTopologyGraphDTO
				if err := json.Unmarshal(body, &graph); err != nil {
					t.Fatalf("Failed to parse JSON from /api/agents/graph?range=today: %v", err)
				}
				if len(graph.Nodes) == 0 {
					t.Errorf("Expected non-empty nodes in today topology graph")
				}
			},
		},
		{
			name:         "AgentFleetSummary_RangeToday",
			method:       http.MethodGet,
			path:         "/api/agents/summary?range=today",
			expectedCode: http.StatusOK,
			validate: func(t *testing.T, resp *http.Response, body []byte) {
				var summary storage.AgentFleetSummaryDTO
				if err := json.Unmarshal(body, &summary); err != nil {
					t.Fatalf("Failed to parse JSON from /api/agents/summary?range=today: %v", err)
				}
			},
		},
		{
			name:         "AgentFleetSummary_Range24h",
			method:       http.MethodGet,
			path:         "/api/agents/summary?range=24h",
			expectedCode: http.StatusOK,
			validate: func(t *testing.T, resp *http.Response, body []byte) {
				var summary storage.AgentFleetSummaryDTO
				if err := json.Unmarshal(body, &summary); err != nil {
					t.Fatalf("Failed to parse JSON from /api/agents/summary?range=24h: %v", err)
				}
			},
		},
		{
			name:         "AgentConcurrency_Range24h",
			method:       http.MethodGet,
			path:         "/api/agents/concurrency?range=24h",
			expectedCode: http.StatusOK,
			validate: func(t *testing.T, resp *http.Response, body []byte) {
				var points []storage.AgentConcurrencyPointDTO
				if err := json.Unmarshal(body, &points); err != nil {
					t.Fatalf("Failed to parse JSON from /api/agents/concurrency?range=24h: %v", err)
				}
				if len(points) == 0 {
					t.Errorf("Expected at least 1 point in concurrency timeline")
				}
			},
		},
		{
			name:         "AgentGanttTasks_Range24h",
			method:       http.MethodGet,
			path:         "/api/agents/gantt?groupBy=roles&range=24h",
			expectedCode: http.StatusOK,
			validate: func(t *testing.T, resp *http.Response, body []byte) {
				var tasks []storage.AgentGanttTaskDTO
				if err := json.Unmarshal(body, &tasks); err != nil {
					t.Fatalf("Failed to parse JSON from /api/agents/gantt?range=24h: %v", err)
				}
			},
		},
		{
			name:         "AgentGanttPackets_All",
			method:       http.MethodGet,
			path:         "/api/agents/gantt/packets?range=all",
			expectedCode: http.StatusOK,
			validate: func(t *testing.T, resp *http.Response, body []byte) {
				var packets []storage.AgentGanttPacketDTO
				if err := json.Unmarshal(body, &packets); err != nil {
					t.Fatalf("Failed to parse JSON from /api/agents/gantt/packets?range=all: %v", err)
				}
				if len(packets) == 0 {
					t.Errorf("Expected packets from /api/agents/gantt/packets, got 0")
				}
				for _, p := range packets {
					if p.Color == "" || p.PacketType == "" || p.Label == "" {
						t.Errorf("Packet missing required properties: %+v", p)
					}
				}
			},
		},
		{
			name:         "AgentGanttPackets_24h",
			method:       http.MethodGet,
			path:         "/api/agents/gantt/packets?range=24h",
			expectedCode: http.StatusOK,
			validate: func(t *testing.T, resp *http.Response, body []byte) {
				var packets []storage.AgentGanttPacketDTO
				if err := json.Unmarshal(body, &packets); err != nil {
					t.Fatalf("Failed to parse JSON from /api/agents/gantt/packets?range=24h: %v", err)
				}
			},
		},
		{
			name:         "ProjectsLeaderboard_Default",
			method:       http.MethodGet,
			path:         "/api/projects/leaderboard",
			expectedCode: http.StatusOK,
			validate: func(t *testing.T, resp *http.Response, body []byte) {
				var dto storage.ProjectLeaderboardResponseDTO
				if err := json.Unmarshal(body, &dto); err != nil {
					t.Fatalf("Failed to parse JSON from /api/projects/leaderboard: %v", err)
				}
				if dto.TimeRange != "30d" {
					t.Errorf("Expected default time_range '30d', got %q", dto.TimeRange)
				}
				if dto.SortBy != "tokens" {
					t.Errorf("Expected default sort_by 'tokens', got %q", dto.SortBy)
				}
			},
		},
		{
			name:         "ProjectsLeaderboard_RangeToday",
			method:       http.MethodGet,
			path:         "/api/projects/leaderboard?range=today",
			expectedCode: http.StatusOK,
			validate: func(t *testing.T, resp *http.Response, body []byte) {
				var dto storage.ProjectLeaderboardResponseDTO
				if err := json.Unmarshal(body, &dto); err != nil {
					t.Fatalf("Failed to parse JSON from /api/projects/leaderboard?range=today: %v", err)
				}
				if dto.TimeRange != "today" {
					t.Errorf("Expected time_range 'today', got %q", dto.TimeRange)
				}
			},
		},
		{
			name:         "ProjectsLeaderboard_Range24h",
			method:       http.MethodGet,
			path:         "/api/projects/leaderboard?range=24h",
			expectedCode: http.StatusOK,
			validate: func(t *testing.T, resp *http.Response, body []byte) {
				var dto storage.ProjectLeaderboardResponseDTO
				if err := json.Unmarshal(body, &dto); err != nil {
					t.Fatalf("Failed to parse JSON from /api/projects/leaderboard?range=24h: %v", err)
				}
				if dto.TimeRange != "24h" {
					t.Errorf("Expected time_range '24h', got %q", dto.TimeRange)
				}
			},
		},
		{
			name:         "ProjectsLeaderboard_Range7d",
			method:       http.MethodGet,
			path:         "/api/projects/leaderboard?range=7d",
			expectedCode: http.StatusOK,
			validate: func(t *testing.T, resp *http.Response, body []byte) {
				var dto storage.ProjectLeaderboardResponseDTO
				if err := json.Unmarshal(body, &dto); err != nil {
					t.Fatalf("Failed to parse JSON from /api/projects/leaderboard?range=7d: %v", err)
				}
				if dto.TimeRange != "7d" {
					t.Errorf("Expected time_range '7d', got %q", dto.TimeRange)
				}
			},
		},
		{
			name:         "ProjectsLeaderboard_Range30d",
			method:       http.MethodGet,
			path:         "/api/projects/leaderboard?range=30d",
			expectedCode: http.StatusOK,
			validate: func(t *testing.T, resp *http.Response, body []byte) {
				var dto storage.ProjectLeaderboardResponseDTO
				if err := json.Unmarshal(body, &dto); err != nil {
					t.Fatalf("Failed to parse JSON from /api/projects/leaderboard?range=30d: %v", err)
				}
				if dto.TimeRange != "30d" {
					t.Errorf("Expected time_range '30d', got %q", dto.TimeRange)
				}
			},
		},
		{
			name:         "ProjectsLeaderboard_RangeAll",
			method:       http.MethodGet,
			path:         "/api/projects/leaderboard?range=all",
			expectedCode: http.StatusOK,
			validate: func(t *testing.T, resp *http.Response, body []byte) {
				var dto storage.ProjectLeaderboardResponseDTO
				if err := json.Unmarshal(body, &dto); err != nil {
					t.Fatalf("Failed to parse JSON from /api/projects/leaderboard?range=all: %v", err)
				}
				if dto.TimeRange != "all" {
					t.Errorf("Expected time_range 'all', got %q", dto.TimeRange)
				}
			},
		},
		{
			name:         "ProjectsLeaderboard_SortTokens",
			method:       http.MethodGet,
			path:         "/api/projects/leaderboard?sort=tokens",
			expectedCode: http.StatusOK,
			validate: func(t *testing.T, resp *http.Response, body []byte) {
				var dto storage.ProjectLeaderboardResponseDTO
				if err := json.Unmarshal(body, &dto); err != nil {
					t.Fatalf("Failed to parse JSON from /api/projects/leaderboard?sort=tokens: %v", err)
				}
				if dto.SortBy != "tokens" {
					t.Errorf("Expected sort_by 'tokens', got %q", dto.SortBy)
				}
			},
		},
		{
			name:         "ProjectsLeaderboard_SortCost",
			method:       http.MethodGet,
			path:         "/api/projects/leaderboard?sort=cost",
			expectedCode: http.StatusOK,
			validate: func(t *testing.T, resp *http.Response, body []byte) {
				var dto storage.ProjectLeaderboardResponseDTO
				if err := json.Unmarshal(body, &dto); err != nil {
					t.Fatalf("Failed to parse JSON from /api/projects/leaderboard?sort=cost: %v", err)
				}
				if dto.SortBy != "cost" {
					t.Errorf("Expected sort_by 'cost', got %q", dto.SortBy)
				}
			},
		},
		{
			name:         "ProjectsLeaderboard_SortActivity",
			method:       http.MethodGet,
			path:         "/api/projects/leaderboard?sort=activity",
			expectedCode: http.StatusOK,
			validate: func(t *testing.T, resp *http.Response, body []byte) {
				var dto storage.ProjectLeaderboardResponseDTO
				if err := json.Unmarshal(body, &dto); err != nil {
					t.Fatalf("Failed to parse JSON from /api/projects/leaderboard?sort=activity: %v", err)
				}
				if dto.SortBy != "activity" {
					t.Errorf("Expected sort_by 'activity', got %q", dto.SortBy)
				}
			},
		},
	}

	for _, tc := range routes {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(tc.method, ts.URL+tc.path, nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tc.expectedCode {
				t.Errorf("Expected status %d, got %d", tc.expectedCode, resp.StatusCode)
			}

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("Failed to read response body: %v", err)
			}

			if tc.validate != nil {
				tc.validate(t, resp, body)
			}
		})
	}
}

// TestWebHandler_ProjectsLeaderboard_EmptyDatabase verifies that an empty database
// returns 200 OK with "projects": [] and zeroed KPIs across all ranges and sorts.
func TestWebHandler_ProjectsLeaderboard_EmptyDatabase(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "empty_leaderboard_test.db")
	cfg := &config.Config{
		Database: config.DatabaseConfig{
			SQLitePath:    dbPath,
			MaxOpenConns:  5,
			MaxIdleConns:  2,
			EnableWALMode: true,
		},
	}
	store, err := storage.NewStorage(cfg)
	if err != nil {
		t.Fatalf("NewStorage failed: %v", err)
	}
	defer store.Close()

	srv := web.NewServer(store, nil, nil)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	client := ts.Client()
	ranges := []string{"today", "24h", "7d", "30d", "all"}
	sorts := []string{"tokens", "cost", "activity"}

	for _, r := range ranges {
		for _, s := range sorts {
			t.Run(fmt.Sprintf("EmptyDB_%s_%s", r, s), func(t *testing.T) {
				resp, err := client.Get(fmt.Sprintf("%s/api/projects/leaderboard?range=%s&sort=%s", ts.URL, r, s))
				if err != nil {
					t.Fatalf("GET failed: %v", err)
				}
				defer resp.Body.Close()

				if resp.StatusCode != http.StatusOK {
					t.Errorf("Expected HTTP 200, got %d", resp.StatusCode)
				}

				body, err := io.ReadAll(resp.Body)
				if err != nil {
					t.Fatalf("ReadAll failed: %v", err)
				}

				// Verify JSON contains `"projects":[]`
				if !strings.Contains(string(body), `"projects":[]`) {
					t.Errorf("Expected body to contain '\"projects\":[]', got: %s", string(body))
				}

				var dto storage.ProjectLeaderboardResponseDTO
				if err := json.Unmarshal(body, &dto); err != nil {
					t.Fatalf("Unmarshal failed: %v", err)
				}

				if dto.Projects == nil || len(dto.Projects) != 0 {
					t.Errorf("Expected 0 projects, got %d", len(dto.Projects))
				}
				if dto.KPIs.GrandTotalTokens != 0 {
					t.Errorf("Expected GrandTotalTokens=0, got %d", dto.KPIs.GrandTotalTokens)
				}
				if dto.KPIs.GrandTotalCostUSD != 0.0 {
					t.Errorf("Expected GrandTotalCostUSD=0.0, got %f", dto.KPIs.GrandTotalCostUSD)
				}
				if dto.KPIs.TotalProjectsCount != 0 {
					t.Errorf("Expected TotalProjectsCount=0, got %d", dto.KPIs.TotalProjectsCount)
				}
			})
		}
	}
}

// TestWebHandler_ProjectsLeaderboard_InvalidParameters verifies HTTP 400 rejection
// on invalid range or invalid sort query parameters.
func TestWebHandler_ProjectsLeaderboard_InvalidParameters(t *testing.T) {
	ts, cleanup := newCanonicalWebServer(t)
	defer cleanup()

	client := ts.Client()

	invalidRanges := []string{"invalid", "100d", "year", "yesterday", "-1d", "today!"}
	for _, invRange := range invalidRanges {
		t.Run("InvalidRange_"+invRange, func(t *testing.T) {
			resp, err := client.Get(ts.URL + "/api/projects/leaderboard?range=" + invRange)
			if err != nil {
				t.Fatalf("GET failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("Expected HTTP 400 for range=%q, got %d", invRange, resp.StatusCode)
			}
			body, _ := io.ReadAll(resp.Body)
			if !strings.Contains(string(body), "invalid range parameter") {
				t.Errorf("Expected error to mention 'invalid range parameter', got: %s", string(body))
			}
		})
	}

	invalidSorts := []string{"invalid", "name", "id", "rank", "tokenss", "tasks", "drop_table"}
	for _, invSort := range invalidSorts {
		t.Run("InvalidSort_"+invSort, func(t *testing.T) {
			resp, err := client.Get(ts.URL + "/api/projects/leaderboard?sort=" + invSort)
			if err != nil {
				t.Fatalf("GET failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("Expected HTTP 400 for sort=%q, got %d", invSort, resp.StatusCode)
			}
			body, _ := io.ReadAll(resp.Body)
			if !strings.Contains(string(body), "invalid sort parameter") {
				t.Errorf("Expected error to mention 'invalid sort parameter', got: %s", string(body))
			}
		})
	}
}

// TestWebHandler_ProjectsLeaderboard_MethodNotAllowed verifies HTTP 405 rejection
// on non-GET HTTP methods.
func TestWebHandler_ProjectsLeaderboard_MethodNotAllowed(t *testing.T) {
	ts, cleanup := newCanonicalWebServer(t)
	defer cleanup()

	client := ts.Client()
	disallowedMethods := []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch}

	for _, m := range disallowedMethods {
		t.Run("DisallowedMethod_"+m, func(t *testing.T) {
			req, err := http.NewRequest(m, ts.URL+"/api/projects/leaderboard", nil)
			if err != nil {
				t.Fatalf("NewRequest failed: %v", err)
			}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusMethodNotAllowed {
				t.Errorf("Expected HTTP 405 for %s, got %d", m, resp.StatusCode)
			}
		})
	}
}

