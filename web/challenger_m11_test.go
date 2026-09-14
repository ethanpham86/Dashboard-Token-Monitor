package web_test

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"tokenmonitor/collector"
	"tokenmonitor/config"
	"tokenmonitor/storage"
	"tokenmonitor/web"
)

// mockCodexLeaderboardProvider implements storage.CodexProvider for empirical testing
type mockCodexLeaderboardProvider struct {
	dashboard collector.CodexDashboardDTO
}

func (m *mockCodexLeaderboardProvider) Dashboard(timeRange string) collector.CodexDashboardDTO {
	return m.dashboard
}

// mockClaudeLeaderboardProvider implements storage.ClaudeProvider for empirical testing
type mockClaudeLeaderboardProvider struct {
	dashboard collector.ClaudeDashboardDTO
}

func (m *mockClaudeLeaderboardProvider) Dashboard(timeRange string) collector.ClaudeDashboardDTO {
	return m.dashboard
}

// createTestLeaderboardServer sets up a temporary database and web server
func createTestLeaderboardServer(t *testing.T) (*storage.Storage, *httptest.Server, func()) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "challenger_m11_test.db")

	cfg := &config.Config{
		Database: config.DatabaseConfig{
			SQLitePath:    dbPath,
			MaxOpenConns:  5,
			MaxIdleConns:  2,
			EnableWALMode: true,
		},
		AccountProfile: config.AccountProfileConfig{
			Email:    "challenger_m11@test.local",
			PlanName: "Google AI Ultra (20X Ultra Tier)",
		},
	}

	store, err := storage.NewStorage(cfg)
	if err != nil {
		t.Fatalf("Failed to initialize storage: %v", err)
	}

	buf := collector.NewAsyncBuffer(store, 1000, 100, 50*time.Millisecond)
	buf.Start()

	server := web.NewServer(store, buf, nil)
	ts := httptest.NewServer(server.Routes())

	cleanup := func() {
		ts.Close()
		buf.Stop()
		store.Close()
	}

	return store, ts, cleanup
}

// 1. Empirically verify empty database invariants
func TestChallengerM11_Leaderboard_EmptyDatabaseInvariants(t *testing.T) {
	_, ts, cleanup := createTestLeaderboardServer(t)
	defer cleanup()

	client := ts.Client()
	ranges := []string{"today", "24h", "1d", "7d", "30d", "month", "all"}
	sorts := []string{"tokens", "cost", "activity"}

	for _, r := range ranges {
		for _, s := range sorts {
			t.Run(fmt.Sprintf("EmptyDB_range=%s_sort=%s", r, s), func(t *testing.T) {
				url := fmt.Sprintf("%s/api/projects/leaderboard?range=%s&sort=%s", ts.URL, r, s)
				resp, err := client.Get(url)
				if err != nil {
					t.Fatalf("HTTP GET failed: %v", err)
				}
				defer resp.Body.Close()

				if resp.StatusCode != http.StatusOK {
					t.Fatalf("Expected HTTP 200, got %d", resp.StatusCode)
				}

				bodyBytes, err := io.ReadAll(resp.Body)
				if err != nil {
					t.Fatalf("Failed to read response body: %v", err)
				}

				// Verify JSON contains `"projects":[]` instead of `"projects":null`
				bodyStr := string(bodyBytes)
				if !strings.Contains(bodyStr, `"projects":[]`) {
					t.Errorf("Expected raw JSON to serialize empty slice as [] but got: %s", bodyStr)
				}

				var respDTO storage.ProjectLeaderboardResponseDTO
				if err := json.Unmarshal(bodyBytes, &respDTO); err != nil {
					t.Fatalf("Failed to unmarshal JSON: %v", err)
				}

				if respDTO.TimeRange != r {
					t.Errorf("Expected time_range %q, got %q", r, respDTO.TimeRange)
				}
				if respDTO.SortBy != s {
					t.Errorf("Expected sort_by %q, got %q", s, respDTO.SortBy)
				}
				if respDTO.Projects == nil {
					t.Errorf("Projects slice must not be nil")
				}
				if len(respDTO.Projects) != 0 {
					t.Errorf("Expected 0 projects, got %d", len(respDTO.Projects))
				}
				if respDTO.KPIs.TotalProjectsCount != 0 {
					t.Errorf("Expected TotalProjectsCount=0, got %d", respDTO.KPIs.TotalProjectsCount)
				}
				if respDTO.KPIs.ActiveProjectsCount != 0 {
					t.Errorf("Expected ActiveProjectsCount=0, got %d", respDTO.KPIs.ActiveProjectsCount)
				}
				if respDTO.KPIs.GrandTotalTokens != 0 {
					t.Errorf("Expected GrandTotalTokens=0, got %d", respDTO.KPIs.GrandTotalTokens)
				}
				if respDTO.KPIs.GrandTotalCostUSD != 0.0 {
					t.Errorf("Expected GrandTotalCostUSD=0.0, got %f", respDTO.KPIs.GrandTotalCostUSD)
				}
				if respDTO.KPIs.GrandTotalActivity != 0 {
					t.Errorf("Expected GrandTotalActivity=0, got %d", respDTO.KPIs.GrandTotalActivity)
				}
				if respDTO.KPIs.OverallCacheHitPercent != 0.0 {
					t.Errorf("Expected OverallCacheHitPercent=0.0, got %f", respDTO.KPIs.OverallCacheHitPercent)
				}
				if respDTO.KPIs.TopConsumerProject != "" {
					t.Errorf("Expected TopConsumerProject='', got %q", respDTO.KPIs.TopConsumerProject)
				}
				if respDTO.KPIs.TopActiveProject != "" {
					t.Errorf("Expected TopActiveProject='', got %q", respDTO.KPIs.TopActiveProject)
				}
			})
		}
	}
}

// 2. Empirically verify parameter validation (HTTP 400 Bad Request)
func TestChallengerM11_Leaderboard_ParameterValidation_HTTP400(t *testing.T) {
	_, ts, cleanup := createTestLeaderboardServer(t)
	defer cleanup()

	client := ts.Client()

	invalidRanges := []string{"invalid", "100d", "year", "yesterday", "-1d", "d7", "24", "today_extra", "all!"}
	for _, invRange := range invalidRanges {
		t.Run("InvalidRange_"+invRange, func(t *testing.T) {
			url := fmt.Sprintf("%s/api/projects/leaderboard?range=%s", ts.URL, invRange)
			resp, err := client.Get(url)
			if err != nil {
				t.Fatalf("GET failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("Expected HTTP 400 for range=%q, got %d", invRange, resp.StatusCode)
			}
			body, _ := io.ReadAll(resp.Body)
			if !strings.Contains(string(body), "invalid range parameter") {
				t.Errorf("Expected error message to mention 'invalid range parameter', got: %s", string(body))
			}
		})
	}

	invalidSorts := []string{"invalid", "name", "id", "rank", "tokenss", "costt", "tasks", "duration", "drop_table"}
	for _, invSort := range invalidSorts {
		t.Run("InvalidSort_"+invSort, func(t *testing.T) {
			url := fmt.Sprintf("%s/api/projects/leaderboard?sort=%s", ts.URL, invSort)
			resp, err := client.Get(url)
			if err != nil {
				t.Fatalf("GET failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("Expected HTTP 400 for sort=%q, got %d", invSort, resp.StatusCode)
			}
			body, _ := io.ReadAll(resp.Body)
			if !strings.Contains(string(body), "invalid sort parameter") {
				t.Errorf("Expected error message to mention 'invalid sort parameter', got: %s", string(body))
			}
		})
	}
}

// 3. Empirically verify HTTP method guard (HTTP 405 Method Not Allowed)
func TestChallengerM11_Leaderboard_HTTPMethodGuard_HTTP405(t *testing.T) {
	_, ts, cleanup := createTestLeaderboardServer(t)
	defer cleanup()

	client := ts.Client()
	disallowedMethods := []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch}

	for _, method := range disallowedMethods {
		t.Run("DisallowedMethod_"+method, func(t *testing.T) {
			req, err := http.NewRequest(method, ts.URL+"/api/projects/leaderboard", nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusMethodNotAllowed {
				t.Errorf("Expected HTTP 405 for %s, got %d", method, resp.StatusCode)
			}
		})
	}
}

// 4. Empirically verify case-insensitivity, trimming, and default fallback parameters
func TestChallengerM11_Leaderboard_CaseAndDefaultParameters(t *testing.T) {
	_, ts, cleanup := createTestLeaderboardServer(t)
	defer cleanup()

	client := ts.Client()

	cases := []struct {
		name          string
		query         string
		expectedRange string
		expectedSort  string
	}{
		{
			name:          "NoParams_DefaultFallback",
			query:         "",
			expectedRange: "30d",
			expectedSort:  "tokens",
		},
		{
			name:          "UpperCaseRangeAndSort",
			query:         "?range=TODAY&sort=COST",
			expectedRange: "today",
			expectedSort:  "cost",
		},
		{
			name:          "WhitespacePaddedParameters",
			query:         "?range=%2024h%20&sort=%20activity%20",
			expectedRange: "24h",
			expectedSort:  "activity",
		},
		{
			name:          "AliasRange1d",
			query:         "?range=1d&sort=tokens",
			expectedRange: "1d",
			expectedSort:  "tokens",
		},
		{
			name:          "AliasRangeMonth",
			query:         "?range=month&sort=cost",
			expectedRange: "month",
			expectedSort:  "cost",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := client.Get(ts.URL + "/api/projects/leaderboard" + tc.query)
			if err != nil {
				t.Fatalf("GET failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("Expected HTTP 200, got %d", resp.StatusCode)
			}

			var dto storage.ProjectLeaderboardResponseDTO
			if err := json.NewDecoder(resp.Body).Decode(&dto); err != nil {
				t.Fatalf("Failed to decode JSON: %v", err)
			}

			if dto.TimeRange != tc.expectedRange {
				t.Errorf("Expected time_range=%q, got %q", tc.expectedRange, dto.TimeRange)
			}
			if dto.SortBy != tc.expectedSort {
				t.Errorf("Expected sort_by=%q, got %q", tc.expectedSort, dto.SortBy)
			}
		})
	}
}

// 5. Empirically verify nil storage and nil monitor handling
func TestChallengerM11_Leaderboard_NilStorageAndTypedNilMonitors(t *testing.T) {
	// Case A: Storage is nil -> returns HTTP 503
	t.Run("NilStorage_Returns503", func(t *testing.T) {
		nilStorageServer := web.NewServer(nil, nil, nil)
		ts := httptest.NewServer(nilStorageServer.Routes())
		defer ts.Close()

		resp, err := http.Get(ts.URL + "/api/projects/leaderboard")
		if err != nil {
			t.Fatalf("GET failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("Expected HTTP 503 when storage is nil, got %d", resp.StatusCode)
		}
	})

	// Case B: Monitors are nil or typed nil -> returns HTTP 200 without panic
	t.Run("TypedNilMonitors_SafeNoPanic", func(t *testing.T) {
		store, ts, cleanup := createTestLeaderboardServer(t)
		defer cleanup()

		resp, err := http.Get(ts.URL + "/api/projects/leaderboard")
		if err != nil {
			t.Fatalf("GET failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected HTTP 200 with nil monitors, got %d", resp.StatusCode)
		}
		_ = store
	})
}

// 6. Empirically verify sorting invariants across tokens, cost, and activity
func TestChallengerM11_Leaderboard_SortingInvariants(t *testing.T) {
	store, ts, cleanup := createTestLeaderboardServer(t)
	defer cleanup()

	client := ts.Client()
	now := time.Now().UTC()

	// Seed Project A: Token Heavy (TokenMonitor)
	// 5,000,000 tokens with Flash model -> Cost is low (~$0.375), 10 calls, 5 tasks (Activity=15)
	for i := 0; i < 5; i++ {
		reqType := fmt.Sprintf("CHAT_574184f1_%d", i+1)
		_, err := store.DB.Exec(`
			INSERT INTO token_usage_logs (account_id, request_type, model_name, prompt_tokens, output_tokens, thinking_tokens, cached_tokens, total_tokens, timestamp)
			VALUES (1, ?, 'gemini-2.5-flash', 800000, 200000, 0, 0, 1000000, ?)
		`, reqType, now.Add(-time.Duration(i)*time.Minute).Format("2006-01-02 15:04:05"))
		if err != nil {
			t.Fatalf("Failed to seed TokenMonitor logs: %v", err)
		}
	}
	// Seed Project A agent tasks (5 tasks)
	for i := 0; i < 5; i++ {
		subId := fmt.Sprintf("sub-574184f1-task-%d", i+1)
		_, err := store.DB.Exec(`
			INSERT INTO agent_fleet_telemetry (account_id, subagent_id, role_name, task_name, status, started_at, finished_at, duration_ms, tokens_used, tokens_offloaded)
			VALUES (1, ?, 'Self-Branch Worker', 'TokenMonitor: Build feature', 'COMPLETED', ?, ?, 1000, 50000, 20000)
		`, subId, now.Add(-time.Duration(i)*time.Minute).Format("2006-01-02 15:04:05"), now.Format("2006-01-02 15:04:05"))
		if err != nil {
			t.Fatalf("Failed to seed TokenMonitor telemetry: %v", err)
		}
	}

	// Seed Project B: Cost Heavy (MCREDIT)
	// 1,000,000 tokens with Ultra model -> Cost is high ($2.50 prompt + $10.00 output = ~$10.00), 2 calls, 1 task (Activity=3)
	_, err := store.DB.Exec(`
		INSERT INTO token_usage_logs (account_id, request_type, model_name, prompt_tokens, output_tokens, thinking_tokens, cached_tokens, total_tokens, timestamp)
		VALUES 
		(1, 'CHAT_5fc429ff_1', 'gemini-2.5-ultra', 500000, 500000, 0, 0, 1000000, ?),
		(1, 'CHAT_5fc429ff_2', 'gemini-2.5-ultra', 0, 0, 0, 0, 0, ?)
	`, now.Add(-10*time.Minute).Format("2006-01-02 15:04:05"), now.Add(-8*time.Minute).Format("2006-01-02 15:04:05"))
	if err != nil {
		t.Fatalf("Failed to seed MCREDIT logs: %v", err)
	}
	_, err = store.DB.Exec(`
		INSERT INTO agent_fleet_telemetry (account_id, subagent_id, role_name, task_name, status, started_at, finished_at, duration_ms, tokens_used, tokens_offloaded)
		VALUES (1, 'sub-5fc429ff-task-1', 'Research Agent', 'MCREDIT: Explore portfolio', 'COMPLETED', ?, ?, 1000, 10000, 5000)
	`, now.Add(-10*time.Minute).Format("2006-01-02 15:04:05"), now.Format("2006-01-02 15:04:05"))
	if err != nil {
		t.Fatalf("Failed to seed MCREDIT telemetry: %v", err)
	}

	// Seed Project C: Activity Heavy (TieuChuanHardeningLinux)
	// 50,000 tokens with Flash model (Cost ~$0.004), but 20 calls and 30 tasks (Activity=50)
	for i := 0; i < 20; i++ {
		reqType := fmt.Sprintf("CHAT_227fb340_%d", i+1)
		_, err := store.DB.Exec(`
			INSERT INTO token_usage_logs (account_id, request_type, model_name, prompt_tokens, output_tokens, thinking_tokens, cached_tokens, total_tokens, timestamp)
			VALUES (1, ?, 'gemini-2.5-flash', 2000, 500, 0, 0, 2500, ?)
		`, reqType, now.Add(-time.Duration(i+1)*time.Minute).Format("2006-01-02 15:04:05"))
		if err != nil {
			t.Fatalf("Failed to seed TCH logs: %v", err)
		}
	}
	for i := 0; i < 30; i++ {
		subId := fmt.Sprintf("sub-227fb340-task-%d", i+1)
		_, err := store.DB.Exec(`
			INSERT INTO agent_fleet_telemetry (account_id, subagent_id, role_name, task_name, status, started_at, finished_at, duration_ms, tokens_used, tokens_offloaded)
			VALUES (1, ?, 'Verification Tester', 'TieuChuanHardeningLinux: CIS check', 'COMPLETED', ?, ?, 500, 1000, 500)
		`, subId, now.Add(-time.Duration(i+1)*time.Minute).Format("2006-01-02 15:04:05"), now.Format("2006-01-02 15:04:05"))
		if err != nil {
			t.Fatalf("Failed to seed TCH telemetry: %v", err)
		}
	}

	// SUBTEST A: sort=tokens
	t.Run("SortByTokens_Invariant", func(t *testing.T) {
		resp, err := client.Get(ts.URL + "/api/projects/leaderboard?range=24h&sort=tokens")
		if err != nil {
			t.Fatalf("GET failed: %v", err)
		}
		defer resp.Body.Close()

		var dto storage.ProjectLeaderboardResponseDTO
		if err := json.NewDecoder(resp.Body).Decode(&dto); err != nil {
			t.Fatalf("Decode failed: %v", err)
		}

		if len(dto.Projects) < 3 {
			t.Fatalf("Expected at least 3 projects, got %d", len(dto.Projects))
		}

		// Verify invariant: Project #1 TotalTokens >= Project #2 TotalTokens >= Project #3 TotalTokens
		for i := 0; i < len(dto.Projects)-1; i++ {
			if dto.Projects[i].TotalTokens < dto.Projects[i+1].TotalTokens {
				t.Errorf("Tokens sort invariant failed at index %d: Project %s (%d) < Project %s (%d)",
					i, dto.Projects[i].ProjectName, dto.Projects[i].TotalTokens,
					dto.Projects[i+1].ProjectName, dto.Projects[i+1].TotalTokens)
			}
		}

		// Rank and PercentOfTop invariants
		for i, proj := range dto.Projects {
			if proj.Rank != i+1 {
				t.Errorf("Expected Rank %d, got %d", i+1, proj.Rank)
			}
			if i == 0 && proj.PercentOfTop != 100.0 {
				t.Errorf("Rank #1 PercentOfTop must be 100.0, got %f", proj.PercentOfTop)
			}
			if proj.PercentOfTop < 0.0 || proj.PercentOfTop > 100.0 {
				t.Errorf("PercentOfTop out of bounds [0, 100]: %f", proj.PercentOfTop)
			}
		}

		// Project #1 must be TokenMonitor (5M tokens)
		if !strings.Contains(dto.Projects[0].ProjectName, "TokenMonitor") {
			t.Errorf("Expected Project #1 to be TokenMonitor, got %s", dto.Projects[0].ProjectName)
		}
		if dto.KPIs.TopConsumerProject != dto.Projects[0].ProjectName {
			t.Errorf("KPI TopConsumerProject %q does not match rank #1 project %q",
				dto.KPIs.TopConsumerProject, dto.Projects[0].ProjectName)
		}
	})

	// SUBTEST B: sort=cost
	t.Run("SortByCost_Invariant", func(t *testing.T) {
		resp, err := client.Get(ts.URL + "/api/projects/leaderboard?range=24h&sort=cost")
		if err != nil {
			t.Fatalf("GET failed: %v", err)
		}
		defer resp.Body.Close()

		var dto storage.ProjectLeaderboardResponseDTO
		if err := json.NewDecoder(resp.Body).Decode(&dto); err != nil {
			t.Fatalf("Decode failed: %v", err)
		}

		if len(dto.Projects) < 3 {
			t.Fatalf("Expected at least 3 projects, got %d", len(dto.Projects))
		}

		for idx, p := range dto.Projects {
			t.Logf("Cost sort [%d] %s: cost=$%f, tokens=%d, act=%d", idx, p.ProjectName, p.EstimatedCostUSD, p.TotalTokens, p.TotalActivity)
		}

		// Verify invariant: Project #1 EstimatedCostUSD >= Project #2 EstimatedCostUSD >= Project #3 EstimatedCostUSD
		for i := 0; i < len(dto.Projects)-1; i++ {
			if dto.Projects[i].EstimatedCostUSD < dto.Projects[i+1].EstimatedCostUSD {
				t.Errorf("Cost sort invariant failed at index %d: Project %s ($%f) < Project %s ($%f)",
					i, dto.Projects[i].ProjectName, dto.Projects[i].EstimatedCostUSD,
					dto.Projects[i+1].ProjectName, dto.Projects[i+1].EstimatedCostUSD)
			}
		}

		// Project #1 must be MCREDIT (Ultra model cost is highest)
		if !strings.Contains(dto.Projects[0].ProjectName, "MCREDIT") {
			t.Errorf("Expected Project #1 by cost to be MCREDIT, got %s", dto.Projects[0].ProjectName)
		}
		if dto.Projects[0].PercentOfTop != 100.0 {
			t.Errorf("Rank #1 PercentOfTop must be 100.0, got %f", dto.Projects[0].PercentOfTop)
		}
	})

	// SUBTEST C: sort=activity
	t.Run("SortByActivity_Invariant", func(t *testing.T) {
		resp, err := client.Get(ts.URL + "/api/projects/leaderboard?range=24h&sort=activity")
		if err != nil {
			t.Fatalf("GET failed: %v", err)
		}
		defer resp.Body.Close()

		var dto storage.ProjectLeaderboardResponseDTO
		if err := json.NewDecoder(resp.Body).Decode(&dto); err != nil {
			t.Fatalf("Decode failed: %v", err)
		}

		if len(dto.Projects) < 3 {
			t.Fatalf("Expected at least 3 projects, got %d", len(dto.Projects))
		}

		for idx, p := range dto.Projects {
			t.Logf("Activity sort [%d] %s: act=%d, tokens=%d, cost=$%f", idx, p.ProjectName, p.TotalActivity, p.TotalTokens, p.EstimatedCostUSD)
		}

		// Verify invariant: Project #1 TotalActivity >= Project #2 TotalActivity >= Project #3 TotalActivity
		for i := 0; i < len(dto.Projects)-1; i++ {
			if dto.Projects[i].TotalActivity < dto.Projects[i+1].TotalActivity {
				t.Errorf("Activity sort invariant failed at index %d: Project %s (%d) < Project %s (%d)",
					i, dto.Projects[i].ProjectName, dto.Projects[i].TotalActivity,
					dto.Projects[i+1].ProjectName, dto.Projects[i+1].TotalActivity)
			}
		}

		// Project #1 must be TieuChuanHardeningLinux (Activity = 20 + 30 = 50)
		if !strings.Contains(dto.Projects[0].ProjectName, "TieuChuanHardeningLinux") {
			t.Errorf("Expected Project #1 by activity to be TieuChuanHardeningLinux, got %s", dto.Projects[0].ProjectName)
		}
		if dto.KPIs.TopActiveProject != dto.Projects[0].ProjectName {
			t.Errorf("KPI TopActiveProject %q does not match rank #1 project %q",
				dto.KPIs.TopActiveProject, dto.Projects[0].ProjectName)
		}
		if dto.KPIs.TopActiveCount != dto.Projects[0].TotalActivity {
			t.Errorf("KPI TopActiveCount %d does not match rank #1 TotalActivity %d",
				dto.KPIs.TopActiveCount, dto.Projects[0].TotalActivity)
		}
	})
}

// 7. Empirically verify Cross-LLM aggregation and mathematical invariants
func TestChallengerM11_Leaderboard_CrossLLM_MathematicalInvariants(t *testing.T) {
	store, _, cleanup := createTestLeaderboardServer(t)
	defer cleanup()

	now := time.Now().UTC()

	// Seed Google Antigravity logs for TokenMonitor and MCREDIT
	_, err := store.DB.Exec(`
		INSERT INTO token_usage_logs (account_id, request_type, model_name, prompt_tokens, output_tokens, thinking_tokens, cached_tokens, total_tokens, timestamp)
		VALUES 
		(1, 'CHAT_574184f1_step_1', 'gemini-2.5-pro', 300000, 100000, 50000, 50000, 450000, ?),
		(1, 'CHAT_5fc429ff_step_1', 'gemini-2.5-ultra', 200000, 50000, 0, 10000, 260000, ?)
	`, now.Add(-5*time.Minute).Format("2006-01-02 15:04:05"), now.Add(-4*time.Minute).Format("2006-01-02 15:04:05"))
	if err != nil {
		t.Fatalf("Failed to seed Antigravity logs: %v", err)
	}

	// Mock Codex Provider with sessions contributing to TokenMonitor and ProjectScriptOS
	mockCodex := &mockCodexLeaderboardProvider{
		dashboard: collector.CodexDashboardDTO{
			SourceStatus: "ACTIVE",
			Summary: collector.CodexSummaryDTO{
				TotalTokens:       500000,
				InputTokens:       350000,
				CachedInputTokens: 50000,
				ReasoningTokens:   20000,
			},
			Sessions: []collector.CodexSessionDTO{
				{
					SessionID:   "codex-sess-1",
					Workspace:   "E:\\GoogleDrive\\WorkSpace\\Code\\ProjectGolang\\GoLangDev\\TokenMonitor",
					Model:       "cx/gpt-5.5",
					TotalTokens: 300000,
					Turns:       15,
					ToolCalls:   10,
					Status:      "COMPLETED",
				},
				{
					SessionID:   "codex-sess-2",
					Workspace:   "/opt/scripts/projectscriptos",
					Model:       "o3-mini",
					TotalTokens: 200000,
					Turns:       8,
					ToolCalls:   4,
					Status:      "ACTIVE",
				},
			},
		},
	}

	// Mock Claude Provider with sessions contributing to TokenMonitor and MCREDIT
	mockClaude := &mockClaudeLeaderboardProvider{
		dashboard: collector.ClaudeDashboardDTO{
			SourceStatus: "ACTIVE",
			Sessions: []collector.ClaudeSessionDTO{
				{
					SessionID:      "claude-sess-1",
					Workspace:      "TokenMonitor",
					ProjectName:    "TokenMonitor",
					Model:          "claude-3-5-sonnet",
					InputTokens:    150000,
					OutputTokens:   50000,
					CacheTokens:    30000,
					ThinkingTokens: 10000,
					TotalTokens:    240000,
					Turns:          12,
					ToolCalls:      6,
					Status:         "ACTIVE",
				},
				{
					SessionID:      "claude-sess-2",
					Workspace:      "/data/projectr/mcredit",
					ProjectName:    "mcredit",
					Model:          "claude-3-5-haiku",
					InputTokens:    80000,
					OutputTokens:   20000,
					CacheTokens:    10000,
					ThinkingTokens: 0,
					TotalTokens:    110000,
					Turns:          5,
					ToolCalls:      2,
					Status:         "COMPLETED",
				},
			},
		},
	}

	// Execute GetProjectsLeaderboard directly
	leaderboard, err := store.GetProjectsLeaderboard("24h", "tokens", mockCodex, mockClaude)
	if err != nil {
		t.Fatalf("GetProjectsLeaderboard failed: %v", err)
	}

	if len(leaderboard.Projects) == 0 {
		t.Fatalf("Expected aggregated projects, got 0")
	}

	// Grand totals check
	var sumTokens int64
	var sumCost float64
	for _, proj := range leaderboard.Projects {
		sumTokens += proj.TotalTokens
		sumCost += proj.EstimatedCostUSD

		// INVARIANT: When TotalTokens > 0, Google + OpenAI + Claude percentages == 100.0%
		if proj.TotalTokens > 0 {
			pctSum := proj.GoogleBreakdown.Percentage + proj.OpenAIBreakdown.Percentage + proj.ClaudeBreakdown.Percentage
			if math.Abs(pctSum-100.0) > 0.0001 {
				t.Errorf("Project %q provider percentages do not sum to 100.0%% (sum=%f, G=%f, O=%f, C=%f)",
					proj.ProjectName, pctSum, proj.GoogleBreakdown.Percentage, proj.OpenAIBreakdown.Percentage, proj.ClaudeBreakdown.Percentage)
			}
		}

		// INVARIANT: Provider token components sum to project totals
		componentTokenSum := proj.GoogleBreakdown.Tokens + proj.OpenAIBreakdown.Tokens + proj.ClaudeBreakdown.Tokens
		if componentTokenSum != proj.TotalTokens {
			t.Errorf("Project %q token sum mismatch: components sum to %d, TotalTokens=%d",
				proj.ProjectName, componentTokenSum, proj.TotalTokens)
		}

		// INVARIANT: Activity sum matches calls + tasks
		expectedActivity := proj.TotalCalls + proj.AgentTasks
		if proj.TotalActivity != expectedActivity {
			t.Errorf("Project %q TotalActivity mismatch: expected %d, got %d",
				proj.ProjectName, expectedActivity, proj.TotalActivity)
		}
		componentActivitySum := proj.GoogleBreakdown.Activity + proj.OpenAIBreakdown.Activity + proj.ClaudeBreakdown.Activity
		if componentActivitySum != proj.TotalActivity {
			t.Errorf("Project %q provider activity sum mismatch: components sum to %d, TotalActivity=%d",
				proj.ProjectName, componentActivitySum, proj.TotalActivity)
		}

		// INVARIANT: Cost breakdown sum matches project total
		componentCostSum := proj.GoogleBreakdown.CostUSD + proj.OpenAIBreakdown.CostUSD + proj.ClaudeBreakdown.CostUSD
		if math.Abs(componentCostSum-proj.EstimatedCostUSD) > 0.001 {
			t.Errorf("Project %q cost sum mismatch: components sum to %f, EstimatedCostUSD=%f",
				proj.ProjectName, componentCostSum, proj.EstimatedCostUSD)
		}

		// INVARIANT: TokenSharePercent matches project share of grand total
		expectedShare := (float64(proj.TotalTokens) * 100.0) / float64(leaderboard.KPIs.GrandTotalTokens)
		if math.Abs(proj.TokenSharePercent-expectedShare) > 0.001 {
			t.Errorf("Project %q TokenSharePercent mismatch: expected %f, got %f",
				proj.ProjectName, expectedShare, proj.TokenSharePercent)
		}
	}

	if sumTokens != leaderboard.KPIs.GrandTotalTokens {
		t.Errorf("GrandTotalTokens mismatch: sum of projects is %d, KPI says %d",
			sumTokens, leaderboard.KPIs.GrandTotalTokens)
	}
	if math.Abs(sumCost-leaderboard.KPIs.GrandTotalCostUSD) > 0.001 {
		t.Errorf("GrandTotalCostUSD mismatch: sum of projects is %f, KPI says %f",
			sumCost, leaderboard.KPIs.GrandTotalCostUSD)
	}

	// KPI Provider Percentage sum == 100.0%
	kpiPctSum := leaderboard.KPIs.GoogleTotalPercent + leaderboard.KPIs.OpenAITotalPercent + leaderboard.KPIs.ClaudeTotalPercent
	if math.Abs(kpiPctSum-100.0) > 0.001 {
		t.Errorf("KPI Provider percentages do not sum to 100.0%%: got %f", kpiPctSum)
	}
}

// 8. Empirically verify time window filtering monotonicity and boundaries
func TestChallengerM11_Leaderboard_TimeWindowFiltering_Monotonicity(t *testing.T) {
	store, ts, cleanup := createTestLeaderboardServer(t)
	defer cleanup()

	client := ts.Client()
	now := time.Now().UTC()

	// Seed events at distinct intervals:
	// Event 1: 5 minutes ago (today, 24h, 7d, 30d, all)
	// Event 2: 18 hours ago (24h, 7d, 30d, all - may or may not be today depending on UTC date)
	// Event 3: 3 days ago (7d, 30d, all)
	// Event 4: 15 days ago (30d, all)
	// Event 5: 60 days ago (all only)
	intervals := []struct {
		offset time.Duration
		tokens int64
	}{
		{offset: 5 * time.Minute, tokens: 10000},
		{offset: 18 * time.Hour, tokens: 20000},
		{offset: 3 * 24 * time.Hour, tokens: 40000},
		{offset: 15 * 24 * time.Hour, tokens: 80000},
		{offset: 60 * 24 * time.Hour, tokens: 160000},
	}

	for i, it := range intervals {
		reqType := fmt.Sprintf("CHAT_574184f1_step_%d", i+1)
		_, err := store.DB.Exec(`
			INSERT INTO token_usage_logs (account_id, request_type, model_name, prompt_tokens, output_tokens, thinking_tokens, cached_tokens, total_tokens, timestamp)
			VALUES (1, ?, 'gemini-2.5-flash', ?, 0, 0, 0, ?, ?)
		`, reqType, it.tokens, it.tokens, now.Add(-it.offset).Format("2006-01-02 15:04:05"))
		if err != nil {
			t.Fatalf("Failed to seed event %d: %v", i, err)
		}
	}

	getTokensForRange := func(r string) int64 {
		resp, err := client.Get(fmt.Sprintf("%s/api/projects/leaderboard?range=%s", ts.URL, r))
		if err != nil {
			t.Fatalf("GET failed for range %s: %v", r, err)
		}
		defer resp.Body.Close()

		var dto storage.ProjectLeaderboardResponseDTO
		if err := json.NewDecoder(resp.Body).Decode(&dto); err != nil {
			t.Fatalf("Decode failed: %v", err)
		}
		return dto.KPIs.GrandTotalTokens
	}

	tokToday := getTokensForRange("today")
	tok24h := getTokensForRange("24h")
	tok7d := getTokensForRange("7d")
	tok30d := getTokensForRange("30d")
	tokAll := getTokensForRange("all")

	t.Logf("Tokens by range: today=%d, 24h=%d, 7d=%d, 30d=%d, all=%d",
		tokToday, tok24h, tok7d, tok30d, tokAll)

	// Invariant: GrandTotalTokens must be monotonically non-decreasing as window expands
	if tok24h > tok7d {
		t.Errorf("Monotonicity violation: 24h tokens (%d) > 7d tokens (%d)", tok24h, tok7d)
	}
	if tok7d > tok30d {
		t.Errorf("Monotonicity violation: 7d tokens (%d) > 30d tokens (%d)", tok7d, tok30d)
	}
	if tok30d > tokAll {
		t.Errorf("Monotonicity violation: 30d tokens (%d) > all tokens (%d)", tok30d, tokAll)
	}

	// Invariant: 60-day-old event (160,000 tokens) must ONLY appear in 'all'
	if tok30d >= tokAll {
		t.Errorf("Expected 30d tokens (%d) to be strictly less than all tokens (%d)", tok30d, tokAll)
	}
	if tokAll < 310000 {
		t.Errorf("Expected all tokens >= 310000, got %d", tokAll)
	}
}

// 9. Empirically verify concurrent query stress testing (zero data races, zero panics)
func TestChallengerM11_Leaderboard_ConcurrentStressTest(t *testing.T) {
	_, ts, cleanup := createTestLeaderboardServer(t)
	defer cleanup()

	client := ts.Client()
	concurrency := 20
	iterations := 10

	var wg sync.WaitGroup
	errCh := make(chan error, concurrency*iterations)

	ranges := []string{"today", "24h", "7d", "30d", "all"}
	sorts := []string{"tokens", "cost", "activity"}

	for worker := 0; worker < concurrency; worker++ {
		wg.Add(1)
		go func(wId int) {
			defer wg.Done()
			for it := 0; it < iterations; it++ {
				r := ranges[(wId+it)%len(ranges)]
				s := sorts[(wId+it)%len(sorts)]
				url := fmt.Sprintf("%s/api/projects/leaderboard?range=%s&sort=%s", ts.URL, r, s)

				resp, err := client.Get(url)
				if err != nil {
					errCh <- fmt.Errorf("Worker %d GET %s failed: %w", wId, url, err)
					return
				}
				if resp.StatusCode != http.StatusOK {
					resp.Body.Close()
					errCh <- fmt.Errorf("Worker %d GET %s returned status %d", wId, url, resp.StatusCode)
					return
				}

				var dto storage.ProjectLeaderboardResponseDTO
				if err := json.NewDecoder(resp.Body).Decode(&dto); err != nil {
					resp.Body.Close()
					errCh <- fmt.Errorf("Worker %d decode JSON failed: %w", wId, err)
					return
				}
				resp.Body.Close()

				if dto.TimeRange != r || dto.SortBy != s {
					errCh <- fmt.Errorf("Worker %d expected range=%s sort=%s, got range=%s sort=%s",
						wId, r, s, dto.TimeRange, dto.SortBy)
					return
				}
			}
		}(worker)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("Concurrent stress error: %v", err)
	}
}
