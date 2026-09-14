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

// setupChallengerRun4Server initializes an isolated test server with populated seed data
func setupChallengerRun4Server(t *testing.T) (*httptest.Server, *storage.Storage, *collector.AsyncBuffer, func()) {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	if filepath.Base(wd) == "web" {
		_ = os.Chdir("..")
	}

	tempDir, err := os.MkdirTemp("", "challenger_r4_web_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	dbPath := filepath.Join(tempDir, "challenger_r4_web.db")

	cfg := &config.Config{
		Database: config.DatabaseConfig{
			SQLitePath:    dbPath,
			MaxOpenConns:  5,
			MaxIdleConns:  2,
			EnableWALMode: true,
		},
		AccountProfile: config.AccountProfileConfig{
			Email:              "challenger_r4_api@example.com",
			AccountType:        "Google Consumer Account (Individual)",
			PlanName:           "20X ULTRA PLAN",
			QuotaBandwidth:     "20x Quota Bandwidth",
			SubscriptionStart:  "2026-09-01",
			SubscriptionExpiry: "2026-10-01",
			AutoRenew:          true,
		},
	}

	store, err := storage.NewStorage(cfg)
	if err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("Failed to create storage: %v", err)
	}

	// Insert test telemetry records for Agent Fleet endpoints
	now := time.Now().UTC()
	_, _ = store.DB.Exec(`
		INSERT INTO agent_fleet_telemetry (account_id, subagent_id, role_name, task_name, status, started_at, finished_at, duration_ms, tokens_used, tokens_offloaded)
		VALUES 
		(1, 'sub-challenger-001', 'Explorer', 'Stress-testing storage schema', 'COMPLETED', ?, ?, 1200, 15000, 3000),
		(1, 'sub-challenger-002', 'Worker', 'Validating REST APIs', 'RUNNING', ?, NULL, 0, 5000, 0),
		(1, 'sub-challenger-003', 'Reviewer', 'Compiling report', 'COMPLETED', ?, ?, 800, 8000, 2000)
	`, now.Add(-30*time.Minute).Format("2006-01-02 15:04:05"),
		now.Add(-28*time.Minute).Format("2006-01-02 15:04:05"),
		now.Add(-10*time.Minute).Format("2006-01-02 15:04:05"),
		now.Add(-45*time.Minute).Format("2006-01-02 15:04:05"),
		now.Add(-44*time.Minute).Format("2006-01-02 15:04:05"))

	// Insert test token usage logs
	_, _ = store.DB.Exec(`
		INSERT INTO token_usage_logs (account_id, timestamp, model_name, prompt_tokens, output_tokens, thinking_tokens, cached_tokens, total_tokens, latency_ms, status_code, request_type)
		VALUES 
		(1, ?, 'gemini-2.5-pro', 1000, 500, 200, 800, 1500, 320, 200, 'INTERACTIVE'),
		(1, ?, 'gemini-3.8-flash', 2000, 800, 300, 1500, 2800, 150, 200, 'INTERACTIVE')
	`, now.Add(-1*time.Hour).Format("2006-01-02 15:04:05"),
		now.Add(-2*time.Hour).Format("2006-01-02 15:04:05"))

	buf := collector.NewAsyncBuffer(store, 1000, 100, 50*time.Millisecond)
	buf.Start()

	server := web.NewServer(store, buf, nil)
	ts := httptest.NewServer(server.Routes())

	cleanup := func() {
		ts.Close()
		buf.Stop()
		store.Close()
		time.Sleep(25 * time.Millisecond)
		_ = os.RemoveAll(tempDir)
		if filepath.Base(wd) == "web" {
			_ = os.Chdir(wd)
		}
	}

	return ts, store, buf, cleanup
}

// TestChallengerRun4_Web_All13EndpointsEmpirical systematically tests all 13 REST API endpoints
func TestChallengerRun4_Web_All13EndpointsEmpirical(t *testing.T) {
	ts, _, _, cleanup := setupChallengerRun4Server(t)
	defer cleanup()

	client := ts.Client()

	type apiTestCase struct {
		id           int
		name         string
		method       string
		path         string
		expectedCode int
		validateJSON func(t *testing.T, body []byte)
	}

	testCases := []apiTestCase{
		// 1. GET /api/account
		{
			id:           1,
			name:         "Endpoint 1: GET /api/account",
			method:       http.MethodGet,
			path:         "/api/account",
			expectedCode: http.StatusOK,
			validateJSON: func(t *testing.T, body []byte) {
				var dto storage.AccountProfileDTO
				if err := json.Unmarshal(body, &dto); err != nil {
					t.Fatalf("Unmarshal failed: %v", err)
				}
				if dto.Email == "" {
					t.Errorf("Expected non-empty Email")
				}
				if dto.TokenStatus == "" {
					t.Errorf("Expected non-empty TokenStatus")
				}
			},
		},

		// 2. GET /api/metrics/summary
		{
			id:           2,
			name:         "Endpoint 2: GET /api/metrics/summary",
			method:       http.MethodGet,
			path:         "/api/metrics/summary?range=all",
			expectedCode: http.StatusOK,
			validateJSON: func(t *testing.T, body []byte) {
				var dto storage.SummaryMetricsDTO
				if err := json.Unmarshal(body, &dto); err != nil {
					t.Fatalf("Unmarshal failed: %v", err)
				}
			},
		},

		// 3. GET /api/metrics/timeseries
		{
			id:           3,
			name:         "Endpoint 3: GET /api/metrics/timeseries",
			method:       http.MethodGet,
			path:         "/api/metrics/timeseries?range=24h",
			expectedCode: http.StatusOK,
			validateJSON: func(t *testing.T, body []byte) {
				var points []storage.ChartPointDTO
				if err := json.Unmarshal(body, &points); err != nil {
					t.Fatalf("Unmarshal failed: %v", err)
				}
			},
		},

		// 4. GET /api/metrics/daily
		{
			id:           4,
			name:         "Endpoint 4: GET /api/metrics/daily",
			method:       http.MethodGet,
			path:         "/api/metrics/daily?range=30d",
			expectedCode: http.StatusOK,
			validateJSON: func(t *testing.T, body []byte) {
				var daily []storage.DailySummaryDTO
				if err := json.Unmarshal(body, &daily); err != nil {
					t.Fatalf("Unmarshal failed: %v", err)
				}
			},
		},

		// 5. GET /api/metrics/models
		{
			id:           5,
			name:         "Endpoint 5: GET /api/metrics/models",
			method:       http.MethodGet,
			path:         "/api/metrics/models?range=all",
			expectedCode: http.StatusOK,
			validateJSON: func(t *testing.T, body []byte) {
				var models []storage.ModelDistributionDTO
				if err := json.Unmarshal(body, &models); err != nil {
					t.Fatalf("Unmarshal failed: %v", err)
				}
				if len(models) == 0 {
					t.Errorf("Expected model distribution items")
				}
			},
		},

		// 6. GET /api/metrics/models/timeseries
		{
			id:           6,
			name:         "Endpoint 6: GET /api/metrics/models/timeseries",
			method:       http.MethodGet,
			path:         "/api/metrics/models/timeseries?range=30d&model=gemini-2.5-pro",
			expectedCode: http.StatusOK,
			validateJSON: func(t *testing.T, body []byte) {
				var points []storage.ChartPointDTO
				if err := json.Unmarshal(body, &points); err != nil {
					t.Fatalf("Unmarshal failed: %v", err)
				}
			},
		},

		// 7. GET /api/sync/history
		{
			id:           7,
			name:         "Endpoint 7: GET /api/sync/history",
			method:       http.MethodGet,
			path:         "/api/sync/history",
			expectedCode: http.StatusOK,
			validateJSON: func(t *testing.T, body []byte) {
				var res map[string]any
				if err := json.Unmarshal(body, &res); err != nil {
					t.Fatalf("Unmarshal failed: %v", err)
				}
				if _, ok := res["events_found"]; !ok {
					t.Errorf("Expected 'events_found' in sync history response")
				}
			},
		},

		// 8. POST /api/test/simulate
		{
			id:           8,
			name:         "Endpoint 8: POST /api/test/simulate",
			method:       http.MethodPost,
			path:         "/api/test/simulate?model=Gemini+3.8+Flash+(High)",
			expectedCode: http.StatusOK,
			validateJSON: func(t *testing.T, body []byte) {
				var res map[string]any
				if err := json.Unmarshal(body, &res); err != nil {
					t.Fatalf("Unmarshal failed: %v", err)
				}
				if _, ok := res["event"]; !ok {
					t.Errorf("Expected 'event' in simulation response")
				}
			},
		},

		// 9. GET /api/agents/summary
		{
			id:           9,
			name:         "Endpoint 9: GET /api/agents/summary",
			method:       http.MethodGet,
			path:         "/api/agents/summary?range=all",
			expectedCode: http.StatusOK,
			validateJSON: func(t *testing.T, body []byte) {
				var summary storage.AgentFleetSummaryDTO
				if err := json.Unmarshal(body, &summary); err != nil {
					t.Fatalf("Unmarshal failed: %v", err)
				}
				if summary.TotalFleet == 0 {
					t.Errorf("Expected TotalFleet > 0 in seeded fleet summary")
				}
			},
		},

		// 10. GET /api/agents/concurrency
		{
			id:           10,
			name:         "Endpoint 10: GET /api/agents/concurrency",
			method:       http.MethodGet,
			path:         "/api/agents/concurrency?range=24h",
			expectedCode: http.StatusOK,
			validateJSON: func(t *testing.T, body []byte) {
				var points []storage.AgentConcurrencyPointDTO
				if err := json.Unmarshal(body, &points); err != nil {
					t.Fatalf("Unmarshal failed: %v", err)
				}
				if len(points) == 0 {
					t.Errorf("Expected points in concurrency timeline")
				}
			},
		},

		// 11. GET /api/agents/gantt
		{
			id:           11,
			name:         "Endpoint 11: GET /api/agents/gantt",
			method:       http.MethodGet,
			path:         "/api/agents/gantt?groupBy=roles&range=all",
			expectedCode: http.StatusOK,
			validateJSON: func(t *testing.T, body []byte) {
				var tasks []storage.AgentGanttTaskDTO
				if err := json.Unmarshal(body, &tasks); err != nil {
					t.Fatalf("Unmarshal failed: %v", err)
				}
				if len(tasks) == 0 {
					t.Errorf("Expected tasks in Gantt response")
				}
			},
		},

		// 12. GET /api/agents/gantt/packets (Crucial requirement from prompt!)
		{
			id:           12,
			name:         "Endpoint 12: GET /api/agents/gantt/packets",
			method:       http.MethodGet,
			path:         "/api/agents/gantt/packets?range=all",
			expectedCode: http.StatusOK,
			validateJSON: func(t *testing.T, body []byte) {
				var packets []storage.AgentGanttPacketDTO
				if err := json.Unmarshal(body, &packets); err != nil {
					t.Fatalf("Unmarshal failed: %v", err)
				}
				if len(packets) == 0 {
					t.Errorf("Expected packets in Gantt packets response, got 0")
				}
				for _, p := range packets {
					if p.ID == "" || p.Color == "" || p.PacketType == "" || p.Label == "" {
						t.Errorf("Packet missing required properties: %+v", p)
					}
				}
			},
		},

		// 13. GET /api/agents/graph
		{
			id:           13,
			name:         "Endpoint 13: GET /api/agents/graph",
			method:       http.MethodGet,
			path:         "/api/agents/graph?project=all&range=all",
			expectedCode: http.StatusOK,
			validateJSON: func(t *testing.T, body []byte) {
				var graph storage.AgentTopologyGraphDTO
				if err := json.Unmarshal(body, &graph); err != nil {
					t.Fatalf("Unmarshal failed: %v", err)
				}
				if len(graph.Nodes) == 0 {
					t.Errorf("Expected nodes in topology graph, got 0")
				}
				if len(graph.Links) == 0 {
					t.Errorf("Expected links in topology graph, got 0")
				}
				if len(graph.Projects) == 0 {
					t.Errorf("Expected projects in topology graph, got 0")
				}
				// Verify 3 multi-project clusters are present
				foundTokenMonitor := false
				foundMCredit := false
				foundHardening := false
				for _, p := range graph.Projects {
					if p.ID == "proj-tokenmonitor" {
						foundTokenMonitor = true
					}
					if p.ID == "proj-mcredit" {
						foundMCredit = true
					}
					if p.ID == "proj-tieuchuanhardeninglinux" {
						foundHardening = true
					}
				}
				if !foundTokenMonitor || !foundMCredit || !foundHardening {
					t.Errorf("Expected 3 project clusters, found: TM=%v, MC=%v, Hardening=%v",
						foundTokenMonitor, foundMCredit, foundHardening)
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(tc.method, ts.URL+tc.path, nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("Request %s failed: %v", tc.path, err)
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("Failed to read body: %v", err)
			}

			if resp.StatusCode != tc.expectedCode {
				t.Errorf("%s expected HTTP %d, got %d (body: %s)", tc.path, tc.expectedCode, resp.StatusCode, string(body))
			}

			if tc.validateJSON != nil {
				tc.validateJSON(t, body)
			}
		})
	}
}

// TestChallengerRun4_Web_AdversarialMethodViolations tests method rejection on endpoints
func TestChallengerRun4_Web_AdversarialMethodViolations(t *testing.T) {
	ts, _, _, cleanup := setupChallengerRun4Server(t)
	defer cleanup()

	client := ts.Client()

	// 1. GET on POST-only endpoint /api/test/simulate must return 405 Method Not Allowed
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/test/simulate", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET /api/test/simulate: expected 405 Method Not Allowed, got %d", resp.StatusCode)
	}

	// 2. DELETE on /api/test/simulate must return 405
	reqDel, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/test/simulate", nil)
	respDel, err := client.Do(reqDel)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer respDel.Body.Close()
	if respDel.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("DELETE /api/test/simulate: expected 405 Method Not Allowed, got %d", respDel.StatusCode)
	}
}

// TestChallengerRun4_Web_GanttPackets_DeepInspection specifically stress-tests GET /api/agents/gantt/packets
func TestChallengerRun4_Web_GanttPackets_DeepInspection(t *testing.T) {
	ts, _, _, cleanup := setupChallengerRun4Server(t)
	defer cleanup()

	client := ts.Client()

	ranges := []string{"all", "24h", "7d", "today", "invalid_range_fuzz"}
	for _, r := range ranges {
		t.Run(fmt.Sprintf("range=%s", r), func(t *testing.T) {
			url := fmt.Sprintf("%s/api/agents/gantt/packets?range=%s", ts.URL, r)
			resp, err := client.Get(url)
			if err != nil {
				t.Fatalf("Failed to GET %s: %v", url, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("Expected 200 OK for %s, got %d", url, resp.StatusCode)
			}

			var packets []storage.AgentGanttPacketDTO
			body, _ := io.ReadAll(resp.Body)
			if err := json.Unmarshal(body, &packets); err != nil {
				t.Fatalf("Invalid JSON returned for %s: %v", url, err)
			}

			// Validate packet schema invariants
			for _, p := range packets {
				if !strings.HasPrefix(p.Color, "#") {
					t.Errorf("Packet Color must be hex code starting with '#', got %q", p.Color)
				}
				if p.FromRole == "" || p.ToRole == "" {
					t.Errorf("Packet must specify FromRole and ToRole: %+v", p)
				}
			}
		})
	}
}

// TestTopology_EdgeLabels_And_ProviderTabs verifies that index.html includes the standardized
// edgeLabel configuration, curve midpoint alignment, label toggle modes, and provider controls.
func TestTopology_EdgeLabels_And_ProviderTabs(t *testing.T) {
	ts, _, _, cleanup := setupChallengerRun4Server(t)
	defer cleanup()

	client := ts.Client()
	resp, err := client.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("Failed to GET /: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200 for index.html, got %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read body: %v", err)
	}
	html := string(bodyBytes)

	checks := []struct {
		desc    string
		pattern string
	}{
		{"Smart label button ID", `id="btn-topo-labels-smart"`},
		{"All labels button ID", `id="btn-topo-labels-all"`},
		{"Smart label button text", `🏷️ Gọn Gàng`},
		{"All labels button text", `📑 Hiện Tất Cả`},
		{"edgeLabel property in links", `edgeLabel:`},
		{"edgeLabel position middle", `position: 'middle'`},
		{"transformCoordToGlobal used for canvas overlay", `transformCoordToGlobal`},
		{"syncAgentFleetControlsForProvider function", `function syncAgentFleetControlsForProvider`},
		{"Codex header dynamic hints", `updateCodexHeader`},
		{"Claude header dynamic hints", `updateClaudeHeader`},
		{"syncTopologyOverlaySize function", `function syncTopologyOverlaySize`},
		{"Coordinate validity guard", `!p0 || !p1 || !cp || isNaN(p0[0])`},
		{"Edge coordinate Y-axis validity guard", `isNaN(p0[1]) || isNaN(p1[0])`},
		{"Node coordinate validity guard", `nx === undefined || ny === undefined || isNaN(nx) || isNaN(ny)`},
		{"syncTopologyOverlaySize resize invalidation", `_topoNeedsResize = true;`},
	}

	for _, c := range checks {
		t.Run(c.desc, func(t *testing.T) {
			if !strings.Contains(html, c.pattern) {
				t.Errorf("Expected index.html to contain %q (%s), but was missing", c.pattern, c.desc)
			}
		})
	}

	// Verify no duplicate function declarations
	t.Run("No duplicate setTopologySpeed", func(t *testing.T) {
		count := strings.Count(html, "function setTopologySpeed")
		if count != 1 {
			t.Errorf("Expected exactly 1 declaration of function setTopologySpeed, found %d", count)
		}
	})

	t.Run("No duplicate toggleTopologyPause", func(t *testing.T) {
		count := strings.Count(html, "function toggleTopologyPause")
		if count != 1 {
			t.Errorf("Expected exactly 1 declaration of function toggleTopologyPause, found %d", count)
		}
	})
}

