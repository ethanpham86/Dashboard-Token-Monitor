package storage_test

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"tokenmonitor/collector"
	"tokenmonitor/storage"
)

// mockCodexProvider implements storage.CodexProvider for unit tests
type testMockCodexProvider struct {
	dashboard collector.CodexDashboardDTO
}

func (m *testMockCodexProvider) Dashboard(timeRange string) collector.CodexDashboardDTO {
	return m.dashboard
}

// mockClaudeProvider implements storage.ClaudeProvider for unit tests
type testMockClaudeProvider struct {
	dashboard collector.ClaudeDashboardDTO
}

func (m *testMockClaudeProvider) Dashboard(timeRange string) collector.ClaudeDashboardDTO {
	return m.dashboard
}

// TestLeaderboard_EmptyDatabase verifies the empty database invariants:
// returns HTTP 200 equivalent (err == nil), non-nil Projects slice (len == 0), and all KPIs zeroed.
func TestLeaderboard_EmptyDatabase(t *testing.T) {
	store, cleanup := newCanonicalTestStorage(t, "test_leaderboard_empty.db")
	defer cleanup()

	ranges := []string{"today", "24h", "7d", "30d", "all"}
	sorts := []string{"tokens", "cost", "activity"}

	for _, r := range ranges {
		for _, s := range sorts {
			t.Run(fmt.Sprintf("Range_%s_Sort_%s", r, s), func(t *testing.T) {
				var res *storage.ProjectLeaderboardResponseDTO
				var err error
				res, err = store.GetProjectsLeaderboard(r, s, nil, nil)
				if err != nil {
					t.Fatalf("Expected nil error for empty DB, got: %v", err)
				}
				if res == nil {
					t.Fatalf("Expected non-nil leaderboard response")
				}
				if res.Projects == nil {
					t.Errorf("Expected non-nil Projects slice for range=%s, sort=%s", r, s)
				}
				if len(res.Projects) != 0 {
					t.Errorf("Expected 0 projects for empty DB, got %d", len(res.Projects))
				}
				if res.TimeRange != r {
					t.Errorf("Expected TimeRange=%q, got %q", r, res.TimeRange)
				}
				if res.SortBy != s {
					t.Errorf("Expected SortBy=%q, got %q", s, res.SortBy)
				}
				if res.KPIs.TotalProjectsCount != 0 {
					t.Errorf("Expected TotalProjectsCount=0, got %d", res.KPIs.TotalProjectsCount)
				}
				if res.KPIs.ActiveProjectsCount != 0 {
					t.Errorf("Expected ActiveProjectsCount=0, got %d", res.KPIs.ActiveProjectsCount)
				}
				if res.KPIs.GrandTotalTokens != 0 {
					t.Errorf("Expected GrandTotalTokens=0, got %d", res.KPIs.GrandTotalTokens)
				}
				if res.KPIs.GrandTotalCostUSD != 0.0 {
					t.Errorf("Expected GrandTotalCostUSD=0.0, got %f", res.KPIs.GrandTotalCostUSD)
				}
				if res.KPIs.GrandTotalActivity != 0 {
					t.Errorf("Expected GrandTotalActivity=0, got %d", res.KPIs.GrandTotalActivity)
				}
				if res.KPIs.OverallCacheHitPercent != 0.0 {
					t.Errorf("Expected OverallCacheHitPercent=0.0, got %f", res.KPIs.OverallCacheHitPercent)
				}
				if res.KPIs.TopConsumerProject != "" {
					t.Errorf("Expected TopConsumerProject='', got %q", res.KPIs.TopConsumerProject)
				}
				if res.KPIs.TopActiveProject != "" {
					t.Errorf("Expected TopActiveProject='', got %q", res.KPIs.TopActiveProject)
				}
			})
		}
	}
}

// TestLeaderboard_ValidRangesAndSorts_Populated verifies sorting and ranking invariants with multi-project data
func TestLeaderboard_ValidRangesAndSorts_Populated(t *testing.T) {
	store, cleanup := newCanonicalTestStorage(t, "test_leaderboard_populated.db")
	defer cleanup()

	now := time.Now().UTC()

	// Seed Project A: Token-heavy (TokenMonitor: 2,000,000 tokens on Flash model)
	for i := 0; i < 4; i++ {
		reqType := fmt.Sprintf("CHAT_574184f1_%d", i+1)
		_, err := store.DB.Exec(`
			INSERT INTO token_usage_logs (account_id, request_type, model_name, prompt_tokens, output_tokens, thinking_tokens, cached_tokens, total_tokens, timestamp)
			VALUES (1, ?, 'gemini-2.5-flash', 400000, 100000, 0, 0, 500000, ?)
		`, reqType, now.Add(-time.Duration(i+1)*time.Minute).Format("2006-01-02 15:04:05"))
		if err != nil {
			t.Fatalf("Failed to seed TokenMonitor logs: %v", err)
		}
	}

	// Seed Project B: Cost-heavy (MCREDIT: 600,000 tokens on Ultra model -> higher cost)
	_, err := store.DB.Exec(`
		INSERT INTO token_usage_logs (account_id, request_type, model_name, prompt_tokens, output_tokens, thinking_tokens, cached_tokens, total_tokens, timestamp)
		VALUES 
		(1, 'CHAT_5fc429ff_1', 'gemini-2.5-ultra', 300000, 300000, 0, 0, 600000, ?)
	`, now.Add(-10*time.Minute).Format("2006-01-02 15:04:05"))
	if err != nil {
		t.Fatalf("Failed to seed MCREDIT logs: %v", err)
	}

	// Seed Project C: Activity-heavy (TieuChuanHardeningLinux: 15 calls + 15 tasks = 30 activity)
	for i := 0; i < 15; i++ {
		reqType := fmt.Sprintf("CHAT_227fb340_%d", i+1)
		_, err := store.DB.Exec(`
			INSERT INTO token_usage_logs (account_id, request_type, model_name, prompt_tokens, output_tokens, thinking_tokens, cached_tokens, total_tokens, timestamp)
			VALUES (1, ?, 'gemini-2.5-flash', 1000, 500, 0, 0, 1500, ?)
		`, reqType, now.Add(-time.Duration(i+1)*time.Minute).Format("2006-01-02 15:04:05"))
		if err != nil {
			t.Fatalf("Failed to seed TCH logs: %v", err)
		}
	}
	for i := 0; i < 15; i++ {
		subId := fmt.Sprintf("sub-227fb340-task-%d", i+1)
		_, err := store.DB.Exec(`
			INSERT INTO agent_fleet_telemetry (account_id, subagent_id, role_name, task_name, status, started_at, finished_at, duration_ms, tokens_used, tokens_offloaded)
			VALUES (1, ?, 'Verification Tester', 'TieuChuanHardeningLinux: Automated audit', 'COMPLETED', ?, ?, 600, 1200, 400)
		`, subId, now.Add(-time.Duration(i+1)*time.Minute).Format("2006-01-02 15:04:05"), now.Format("2006-01-02 15:04:05"))
		if err != nil {
			t.Fatalf("Failed to seed TCH telemetry: %v", err)
		}
	}

	// 1. Verify sort=tokens
	resTokens, err := store.GetProjectsLeaderboard("24h", "tokens", nil, nil)
	if err != nil {
		t.Fatalf("GetProjectsLeaderboard tokens sort failed: %v", err)
	}
	if len(resTokens.Projects) < 3 {
		t.Fatalf("Expected at least 3 projects, got %d", len(resTokens.Projects))
	}
	for i := 0; i < len(resTokens.Projects)-1; i++ {
		if resTokens.Projects[i].TotalTokens < resTokens.Projects[i+1].TotalTokens {
			t.Errorf("Sort by tokens failed: [%d] %d < [%d] %d",
				i, resTokens.Projects[i].TotalTokens, i+1, resTokens.Projects[i+1].TotalTokens)
		}
	}
	if !strings.Contains(resTokens.Projects[0].ProjectName, "TokenMonitor") {
		t.Errorf("Expected TokenMonitor to be #1 for tokens, got %q", resTokens.Projects[0].ProjectName)
	}
	if resTokens.Projects[0].PercentOfTop != 100.0 {
		t.Errorf("Rank #1 PercentOfTop must be 100.0, got %f", resTokens.Projects[0].PercentOfTop)
	}

	// 2. Verify sort=cost
	resCost, err := store.GetProjectsLeaderboard("24h", "cost", nil, nil)
	if err != nil {
		t.Fatalf("GetProjectsLeaderboard cost sort failed: %v", err)
	}
	if len(resCost.Projects) < 3 {
		t.Fatalf("Expected at least 3 projects, got %d", len(resCost.Projects))
	}
	for i := 0; i < len(resCost.Projects)-1; i++ {
		if resCost.Projects[i].EstimatedCostUSD < resCost.Projects[i+1].EstimatedCostUSD {
			t.Errorf("Sort by cost failed: [%d] $%f < [%d] $%f",
				i, resCost.Projects[i].EstimatedCostUSD, i+1, resCost.Projects[i+1].EstimatedCostUSD)
		}
	}
	if !strings.Contains(resCost.Projects[0].ProjectName, "MCREDIT") {
		t.Errorf("Expected MCREDIT to be #1 for cost, got %q", resCost.Projects[0].ProjectName)
	}

	// 3. Verify sort=activity
	resAct, err := store.GetProjectsLeaderboard("24h", "activity", nil, nil)
	if err != nil {
		t.Fatalf("GetProjectsLeaderboard activity sort failed: %v", err)
	}
	if len(resAct.Projects) < 3 {
		t.Fatalf("Expected at least 3 projects, got %d", len(resAct.Projects))
	}
	for i := 0; i < len(resAct.Projects)-1; i++ {
		if resAct.Projects[i].TotalActivity < resAct.Projects[i+1].TotalActivity {
			t.Errorf("Sort by activity failed: [%d] %d < [%d] %d",
				i, resAct.Projects[i].TotalActivity, i+1, resAct.Projects[i+1].TotalActivity)
		}
	}
	if !strings.Contains(resAct.Projects[0].ProjectName, "TieuChuanHardeningLinux") {
		t.Errorf("Expected TieuChuanHardeningLinux to be #1 for activity, got %q", resAct.Projects[0].ProjectName)
	}

	// 4. Verify all valid range values
	for _, r := range []string{"today", "24h", "7d", "30d", "all"} {
		resRange, err := store.GetProjectsLeaderboard(r, "tokens", nil, nil)
		if err != nil {
			t.Errorf("Range %s failed: %v", r, err)
		}
		if resRange.TimeRange != r {
			t.Errorf("Expected TimeRange=%s, got %s", r, resRange.TimeRange)
		}
	}
}

// TestLeaderboard_CrossLLM_Invariants verifies cross-provider math and breakdown integrity
func TestLeaderboard_CrossLLM_Invariants(t *testing.T) {
	store, cleanup := newCanonicalTestStorage(t, "test_leaderboard_cross_llm.db")
	defer cleanup()

	now := time.Now().UTC()

	// Seed Google Antigravity
	_, err := store.DB.Exec(`
		INSERT INTO token_usage_logs (account_id, request_type, model_name, prompt_tokens, output_tokens, thinking_tokens, cached_tokens, total_tokens, timestamp)
		VALUES 
		(1, 'CHAT_574184f1_cross_1', 'gemini-2.5-pro', 200000, 80000, 20000, 30000, 300000, ?)
	`, now.Add(-5*time.Minute).Format("2006-01-02 15:04:05"))
	if err != nil {
		t.Fatalf("Failed to seed Antigravity logs: %v", err)
	}

	// Mock Codex Provider
	mockCodex := &testMockCodexProvider{
		dashboard: collector.CodexDashboardDTO{
			SourceStatus: "ACTIVE",
			Summary: collector.CodexSummaryDTO{
				TotalTokens:       400000,
				InputTokens:       300000,
				CachedInputTokens: 50000,
				ReasoningTokens:   10000,
			},
			Sessions: []collector.CodexSessionDTO{
				{
					SessionID:   "codex-s1",
					Workspace:   "E:\\GoogleDrive\\WorkSpace\\Code\\ProjectGolang\\GoLangDev\\TokenMonitor",
					Model:       "gpt-5",
					TotalTokens: 250000,
					Turns:       10,
					ToolCalls:   5,
					Status:      "ACTIVE",
				},
				{
					SessionID:   "codex-s2",
					Workspace:   "/home/user/projectscriptos",
					Model:       "o3-mini",
					TotalTokens: 150000,
					Turns:       6,
					ToolCalls:   2,
					Status:      "COMPLETED",
				},
			},
		},
	}

	// Mock Claude Provider
	mockClaude := &testMockClaudeProvider{
		dashboard: collector.ClaudeDashboardDTO{
			SourceStatus: "ACTIVE",
			Sessions: []collector.ClaudeSessionDTO{
				{
					SessionID:      "claude-s1",
					Workspace:      "TokenMonitor",
					ProjectName:    "TokenMonitor",
					Model:          "claude-3-5-sonnet",
					InputTokens:    100000,
					OutputTokens:   40000,
					CacheTokens:    20000,
					ThinkingTokens: 5000,
					TotalTokens:    160000,
					Turns:          8,
					ToolCalls:      4,
					Status:         "ACTIVE",
				},
			},
		},
	}

	res, err := store.GetProjectsLeaderboard("24h", "tokens", mockCodex, mockClaude)
	if err != nil {
		t.Fatalf("GetProjectsLeaderboard failed: %v", err)
	}

	if len(res.Projects) == 0 {
		t.Fatalf("Expected projects, got 0")
	}

	var sumTokens int64
	var sumCost float64
	for _, p := range res.Projects {
		sumTokens += p.TotalTokens
		sumCost += p.EstimatedCostUSD

		// Breakdown tokens sum matches project TotalTokens
		compTokens := p.GoogleBreakdown.Tokens + p.OpenAIBreakdown.Tokens + p.ClaudeBreakdown.Tokens
		if compTokens != p.TotalTokens {
			t.Errorf("Project %q component tokens %d != TotalTokens %d", p.ProjectName, compTokens, p.TotalTokens)
		}

		// Breakdown activity sum matches TotalActivity
		compActivity := p.GoogleBreakdown.Activity + p.OpenAIBreakdown.Activity + p.ClaudeBreakdown.Activity
		if compActivity != p.TotalActivity {
			t.Errorf("Project %q component activity %d != TotalActivity %d", p.ProjectName, compActivity, p.TotalActivity)
		}

		// When TotalTokens > 0, provider percentages sum to 100.0%
		if p.TotalTokens > 0 {
			pctSum := p.GoogleBreakdown.Percentage + p.OpenAIBreakdown.Percentage + p.ClaudeBreakdown.Percentage
			if math.Abs(pctSum-100.0) > 0.001 {
				t.Errorf("Project %q provider percentages do not sum to 100.0%%: %f", p.ProjectName, pctSum)
			}
		}
	}

	if sumTokens != res.KPIs.GrandTotalTokens {
		t.Errorf("GrandTotalTokens mismatch: sum=%d, KPI=%d", sumTokens, res.KPIs.GrandTotalTokens)
	}
	if math.Abs(sumCost-res.KPIs.GrandTotalCostUSD) > 0.001 {
		t.Errorf("GrandTotalCostUSD mismatch: sum=%f, KPI=%f", sumCost, res.KPIs.GrandTotalCostUSD)
	}

	// KPI Provider Percentage sum == 100.0%
	kpiPctSum := res.KPIs.GoogleTotalPercent + res.KPIs.OpenAITotalPercent + res.KPIs.ClaudeTotalPercent
	if math.Abs(kpiPctSum-100.0) > 0.001 {
		t.Errorf("KPI Provider percentages do not sum to 100.0%%: got %f", kpiPctSum)
	}
}

// TestLeaderboard_NilMonitorsSafe verifies safe fallback when monitors are nil or typed nil
func TestLeaderboard_NilMonitorsSafe(t *testing.T) {
	store, cleanup := newCanonicalTestStorage(t, "test_leaderboard_nil_monitors.db")
	defer cleanup()

	var typedNilCodex *testMockCodexProvider
	var typedNilClaude *testMockClaudeProvider

	res, err := store.GetProjectsLeaderboard("today", "tokens", typedNilCodex, typedNilClaude)
	if err != nil {
		t.Fatalf("Unexpected error with typed nil monitors: %v", err)
	}
	if res == nil {
		t.Fatalf("Expected non-nil result")
	}
}

// TestLeaderboard_DefaultFallback verifies that empty parameters fallback to 30d and tokens
func TestLeaderboard_DefaultFallback(t *testing.T) {
	store, cleanup := newCanonicalTestStorage(t, "test_leaderboard_defaults.db")
	defer cleanup()

	res, err := store.GetProjectsLeaderboard("", "", nil, nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if res.TimeRange != "30d" {
		t.Errorf("Expected default TimeRange '30d', got %q", res.TimeRange)
	}
	if res.SortBy != "tokens" {
		t.Errorf("Expected default SortBy 'tokens', got %q", res.SortBy)
	}
}
