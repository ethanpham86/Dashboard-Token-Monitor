package storage_test

import (
	"math"
	"path/filepath"
	"testing"
	"time"

	"tokenmonitor/collector"
	"tokenmonitor/config"
	"tokenmonitor/storage"
)

type mockStorageCodexProvider struct {
	dashboard collector.CodexDashboardDTO
}

func (m *mockStorageCodexProvider) Dashboard(timeRange string) collector.CodexDashboardDTO {
	return m.dashboard
}

type mockStorageClaudeProvider struct {
	dashboard collector.ClaudeDashboardDTO
}

func (m *mockStorageClaudeProvider) Dashboard(timeRange string) collector.ClaudeDashboardDTO {
	return m.dashboard
}

func createStorageChallengeServer(t *testing.T) (*storage.Storage, func()) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "storage_m11_challenge.db")

	cfg := &config.Config{
		Database: config.DatabaseConfig{
			SQLitePath:    dbPath,
			EnableWALMode: true,
		},
		AccountProfile: config.AccountProfileConfig{
			Email: "challenge_m11@test.local",
		},
	}

	store, err := storage.NewStorage(cfg)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	return store, func() {
		store.Close()
	}
}

// 1. Empirical test: 10 FinOps Pricing Tiers verification
func TestChallengerM11_Storage_PricingTiers(t *testing.T) {
	testCases := []struct {
		model            string
		prompt           int64
		output           int64
		thinking         int64
		cached           int64
		expectedCostMin  float64
		expectedCostMax  float64
		expectedSavMin   float64
		expectedSavMax   float64
	}{
		// Claude Opus: $15.00 prompt, $75.00 output, $1.50 cache per 1M
		{
			model:           "claude-3-opus-20240229",
			prompt:          1000000,
			output:          1000000,
			cached:          1000000,
			expectedCostMin: 91.49,
			expectedCostMax: 91.51,
			expectedSavMin:  13.49,
			expectedSavMax:  13.51,
		},
		// Claude Sonnet: $3.00 prompt, $15.00 output, $0.30 cache per 1M
		{
			model:           "claude-3-5-sonnet-20241022",
			prompt:          1000000,
			output:          1000000,
			cached:          1000000,
			expectedCostMin: 18.29,
			expectedCostMax: 18.31,
			expectedSavMin:  2.69,
			expectedSavMax:  2.71,
		},
		// Claude Haiku: $0.80 prompt, $4.00 output, $0.08 cache per 1M
		{
			model:           "claude-3-5-haiku-20241022",
			prompt:          1000000,
			output:          1000000,
			cached:          1000000,
			expectedCostMin: 4.87,
			expectedCostMax: 4.89,
			expectedSavMin:  0.71,
			expectedSavMax:  0.73,
		},
		// OpenAI o1: $15.00 prompt, $60.00 output, $7.50 cache per 1M
		{
			model:           "o1-preview",
			prompt:          1000000,
			output:          1000000,
			cached:          1000000,
			expectedCostMin: 82.49,
			expectedCostMax: 82.51,
			expectedSavMin:  7.49,
			expectedSavMax:  7.51,
		},
		// OpenAI GPT-4o: $2.50 prompt, $10.00 output, $1.25 cache per 1M
		{
			model:           "gpt-4o",
			prompt:          1000000,
			output:          1000000,
			cached:          1000000,
			expectedCostMin: 13.74,
			expectedCostMax: 13.76,
			expectedSavMin:  1.24,
			expectedSavMax:  1.26,
		},
		// Google Ultra: $2.50 prompt, $10.00 output, $0.625 cache per 1M
		{
			model:           "gemini-2.5-ultra",
			prompt:          1000000,
			output:          1000000,
			cached:          1000000,
			expectedCostMin: 13.12,
			expectedCostMax: 13.13,
			expectedSavMin:  1.87,
			expectedSavMax:  1.88,
		},
		// Google Pro: $1.25 prompt, $5.00 output, $0.3125 cache per 1M
		{
			model:           "gemini-2.5-pro",
			prompt:          1000000,
			output:          1000000,
			cached:          1000000,
			expectedCostMin: 6.56,
			expectedCostMax: 6.57,
			expectedSavMin:  0.93,
			expectedSavMax:  0.94,
		},
		// Google Flash: $0.075 prompt, $0.30 output, $0.01875 cache per 1M
		{
			model:           "gemini-2.5-flash",
			prompt:          1000000,
			output:          1000000,
			cached:          1000000,
			expectedCostMin: 0.393,
			expectedCostMax: 0.395,
			expectedSavMin:  0.056,
			expectedSavMax:  0.057,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.model, func(t *testing.T) {
			cost, sav := storage.CalculateTokensCostUSD(tc.model, tc.prompt, tc.output, tc.thinking, tc.cached)
			if cost < tc.expectedCostMin || cost > tc.expectedCostMax {
				t.Errorf("Model %s: expected cost in [%f, %f], got %f", tc.model, tc.expectedCostMin, tc.expectedCostMax, cost)
			}
			if sav < tc.expectedSavMin || sav > tc.expectedSavMax {
				t.Errorf("Model %s: expected savings in [%f, %f], got %f", tc.model, tc.expectedSavMin, tc.expectedSavMax, sav)
			}
		})
	}
}

// 2. Empirical test: ResolveCrossLLMProject resolution and collision prevention
func TestChallengerM11_Storage_ResolveCrossLLMProject_EdgeCases(t *testing.T) {
	cases := []struct {
		input       string
		expectedID  string
		expectedNm  string
	}{
		// Fallbacks
		{"", "proj-tokenmonitor", "TokenMonitor (GoLangDev)"},
		{".", "proj-tokenmonitor", "TokenMonitor (GoLangDev)"},
		{"/", "proj-tokenmonitor", "TokenMonitor (GoLangDev)"},
		{"default", "proj-tokenmonitor", "TokenMonitor (GoLangDev)"},
		{"default workspace", "proj-tokenmonitor", "TokenMonitor (GoLangDev)"},

		// Canonical IDs
		{"proj-tokenmonitor", "proj-tokenmonitor", "TokenMonitor (GoLangDev)"},
		{"proj-mcredit", "proj-mcredit", "MCREDIT (ProjectR)"},
		{"proj-tieuchuanhardeninglinux", "proj-tieuchuanhardeninglinux", "TieuChuanHardeningLinux (Security Standards)"},
		{"proj-projectscriptos", "proj-projectscriptos", "ProjectScriptOS"},

		// Paths with Windows backslashes
		{`E:\GoogleDrive\WorkSpace\Code\ProjectGolang\GoLangDev\TokenMonitor`, "proj-tokenmonitor", "TokenMonitor (GoLangDev)"},
		{`C:\Users\Developer\WorkSpace\ProjectR\MCREDIT\data.R`, "proj-mcredit", "MCREDIT (ProjectR)"},
		{`D:\SecStandards\TieuChuanHardeningLinux\docs\cis.md`, "proj-tieuchuanhardeninglinux", "TieuChuanHardeningLinux (Security Standards)"},
		{`E:\ProjectScriptOS\scripts\copy.sh`, "proj-projectscriptos", "ProjectScriptOS"},

		// Collision guard: MCREDIT keyword inside TokenMonitor path MUST resolve to TokenMonitor
		{`E:\TokenMonitor\test_data_for_mcredit_sample.json`, "proj-tokenmonitor", "TokenMonitor (GoLangDev)"},
		{`E:\TokenMonitor\tieuchuan_hardening_benchmark.go`, "proj-tokenmonitor", "TokenMonitor (GoLangDev)"},

		// Arbitrary unknown workspace slugification
		{`/opt/my_custom_ai_app`, "proj-my_custom_ai_app", "my_custom_ai_app"},
	}

	for _, tc := range cases {
		t.Run("Resolve_"+tc.input, func(t *testing.T) {
			id, nm, _ := storage.ResolveCrossLLMProject(tc.input)
			if id != tc.expectedID {
				t.Errorf("Input %q: expected ID %q, got %q", tc.input, tc.expectedID, id)
			}
			if nm != tc.expectedNm {
				t.Errorf("Input %q: expected Name %q, got %q", tc.input, tc.expectedNm, nm)
			}
		})
	}
}

// 3. Empirical test: Inline 2-minute TTL Auto-Sweep behavior
func TestChallengerM11_Storage_TTL_AutoSweep(t *testing.T) {
	store, cleanup := createStorageChallengeServer(t)
	defer cleanup()

	now := time.Now().UTC()

	// Seed 2 tasks:
	// Task 1: Started 5 minutes ago, status RUNNING -> SHOULD be auto-swept to COMPLETED
	// Task 2: Started 30 seconds ago, status RUNNING -> SHOULD stay RUNNING
	_, err := store.DB.Exec(`
		INSERT INTO agent_fleet_telemetry (account_id, subagent_id, role_name, task_name, status, started_at, duration_ms)
		VALUES 
		(1, 'sub-stale-001', 'Self-Branch Worker', 'Stale Task', 'RUNNING', ?, 5000),
		(1, 'sub-fresh-001', 'Self-Branch Worker', 'Fresh Task', 'RUNNING', ?, 2000)
	`, now.Add(-5*time.Minute).Format("2006-01-02 15:04:05"), now.Add(-30*time.Second).Format("2006-01-02 15:04:05"))
	if err != nil {
		t.Fatalf("Failed to seed telemetry: %v", err)
	}

	// Trigger GetAntigravityProjectStats which runs the inline sweep
	_, err = store.GetAntigravityProjectStats("24h")
	if err != nil {
		t.Fatalf("GetAntigravityProjectStats failed: %v", err)
	}

	// Inspect DB to verify Task 1 is COMPLETED and Task 2 is RUNNING
	var status1, status2 string
	err = store.DB.QueryRow(`SELECT status FROM agent_fleet_telemetry WHERE subagent_id = 'sub-stale-001'`).Scan(&status1)
	if err != nil {
		t.Fatalf("Query task 1 failed: %v", err)
	}
	if status1 != "COMPLETED" {
		t.Errorf("Expected stale task to be swept to COMPLETED, got %q", status1)
	}

	err = store.DB.QueryRow(`SELECT status FROM agent_fleet_telemetry WHERE subagent_id = 'sub-fresh-001'`).Scan(&status2)
	if err != nil {
		t.Fatalf("Query task 2 failed: %v", err)
	}
	if status2 != "RUNNING" {
		t.Errorf("Expected fresh task to stay RUNNING, got %q", status2)
	}
}

// 4. Empirical test: Deterministic Tie-Breaking across all 3 sort criteria
func TestChallengerM11_Storage_DeterministicTieBreaking(t *testing.T) {
	store, cleanup := createStorageChallengeServer(t)
	defer cleanup()

	// Mock Codex with 2 sessions having identical tokens but different names and activity
	mockCodex := &mockStorageCodexProvider{
		dashboard: collector.CodexDashboardDTO{
			SourceStatus: "ACTIVE",
			Summary: collector.CodexSummaryDTO{
				TotalTokens: 200000,
				InputTokens: 160000,
			},
			Sessions: []collector.CodexSessionDTO{
				{
					SessionID:   "sess-tie-1",
					Workspace:   "/opt/project_alpha",
					Model:       "gpt-4o",
					TotalTokens: 100000,
					Turns:       10,
					ToolCalls:   5,
					Status:      "COMPLETED",
				},
				{
					SessionID:   "sess-tie-2",
					Workspace:   "/opt/project_beta",
					Model:       "gpt-4o",
					TotalTokens: 100000,
					Turns:       20,
					ToolCalls:   10,
					Status:      "COMPLETED",
				},
			},
		},
	}

	leaderboard, err := store.GetProjectsLeaderboard("24h", "tokens", mockCodex, nil)
	if err != nil {
		t.Fatalf("GetProjectsLeaderboard failed: %v", err)
	}

	if len(leaderboard.Projects) < 2 {
		t.Fatalf("Expected at least 2 projects, got %d", len(leaderboard.Projects))
	}

	// In tokens sort: total tokens equal (100k vs 100k), cost equal -> secondary is cost, tertiary is alphabetical name
	if leaderboard.Projects[0].ProjectName != "project_alpha" {
		t.Errorf("Expected tie-break to order 'project_alpha' first, got %s", leaderboard.Projects[0].ProjectName)
	}
	if leaderboard.Projects[1].ProjectName != "project_beta" {
		t.Errorf("Expected tie-break to order 'project_beta' second, got %s", leaderboard.Projects[1].ProjectName)
	}

	// In activity sort: project_beta (activity 30) > project_alpha (activity 15)
	actLeaderboard, err := store.GetProjectsLeaderboard("24h", "activity", mockCodex, nil)
	if err != nil {
		t.Fatalf("GetProjectsLeaderboard activity sort failed: %v", err)
	}
	if actLeaderboard.Projects[0].ProjectName != "project_beta" {
		t.Errorf("Expected project_beta #1 in activity sort, got %s", actLeaderboard.Projects[0].ProjectName)
	}
	if actLeaderboard.Projects[1].ProjectName != "project_alpha" {
		t.Errorf("Expected project_alpha #2 in activity sort, got %s", actLeaderboard.Projects[1].ProjectName)
	}
}

// 5. Empirical test: Float Delta Adjustment on odd division ensuring exact 100.0% sum
func TestChallengerM11_Storage_OddTokens_Exact100PercentInvariant(t *testing.T) {
	store, cleanup := createStorageChallengeServer(t)
	defer cleanup()

	now := time.Now().UTC()

	// Seed 1 token for TokenMonitor in Google Antigravity
	_, err := store.DB.Exec(`
		INSERT INTO token_usage_logs (account_id, request_type, model_name, prompt_tokens, output_tokens, thinking_tokens, cached_tokens, total_tokens, timestamp)
		VALUES (1, 'CHAT_574184f1_1', 'gemini-2.5-pro', 1, 0, 0, 0, 1, ?)
	`, now.Format("2006-01-02 15:04:05"))
	if err != nil {
		t.Fatalf("Failed to seed token: %v", err)
	}

	// Seed 1 token for TokenMonitor in OpenAI Codex
	mockCodex := &mockStorageCodexProvider{
		dashboard: collector.CodexDashboardDTO{
			SourceStatus: "ACTIVE",
			Summary: collector.CodexSummaryDTO{
				TotalTokens: 1,
				InputTokens: 1,
			},
			Sessions: []collector.CodexSessionDTO{
				{
					SessionID:   "sess-odd-1",
					Workspace:   "TokenMonitor",
					Model:       "gpt-4o",
					TotalTokens: 1,
					Turns:       1,
				},
			},
		},
	}

	// Seed 1 token for TokenMonitor in Anthropic Claude
	mockClaude := &mockStorageClaudeProvider{
		dashboard: collector.ClaudeDashboardDTO{
			SourceStatus: "ACTIVE",
			Sessions: []collector.ClaudeSessionDTO{
				{
					SessionID:   "sess-odd-2",
					Workspace:   "TokenMonitor",
					ProjectName: "TokenMonitor",
					Model:       "claude-3-5-sonnet",
					TotalTokens: 1,
					InputTokens: 1,
					Turns:       1,
				},
			},
		},
	}

	// Total tokens = 1 + 1 + 1 = 3. Division 1/3 = 33.333333% each -> sum would be 99.999999% without delta adjustment.
	leaderboard, err := store.GetProjectsLeaderboard("24h", "tokens", mockCodex, mockClaude)
	if err != nil {
		t.Fatalf("GetProjectsLeaderboard failed: %v", err)
	}

	if len(leaderboard.Projects) == 0 {
		t.Fatalf("Expected TokenMonitor project")
	}

	p := leaderboard.Projects[0]
	sumPct := p.GoogleBreakdown.Percentage + p.OpenAIBreakdown.Percentage + p.ClaudeBreakdown.Percentage

	if math.Abs(sumPct-100.0) > 0.000001 {
		t.Errorf("Expected exact 100.0%% sum with 1/3 tokens, got %f (G=%f, O=%f, C=%f)",
			sumPct, p.GoogleBreakdown.Percentage, p.OpenAIBreakdown.Percentage, p.ClaudeBreakdown.Percentage)
	}
}
