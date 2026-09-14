package storage_test

import (
	"fmt"
	"math"
	"math/rand"
	"testing"
	"time"

	"tokenmonitor/collector"
	"tokenmonitor/storage"
)

// challenger2MockCodex implements storage.CodexProvider for Challenger M11-2 testing.
type challenger2MockCodex struct {
	dashboardDTO collector.CodexDashboardDTO
}

func (m *challenger2MockCodex) Dashboard(timeRange string) collector.CodexDashboardDTO {
	return m.dashboardDTO
}

// challenger2MockClaude implements storage.ClaudeProvider for Challenger M11-2 testing.
type challenger2MockClaude struct {
	dashboardDTO collector.ClaudeDashboardDTO
}

func (m *challenger2MockClaude) Dashboard(timeRange string) collector.ClaudeDashboardDTO {
	return m.dashboardDTO
}

// =========================================================================
// 1. PROVIDER PERCENTAGE SUM INVARIANT (100.0%)
// =========================================================================

// TestChallengerM11_2_ProviderPercentageSumInvariant_Comprehensive tests the 100.0% invariant
// across single-provider, dual-provider, tri-provider, repeating fractions, extreme asymmetries,
// and 1,000 randomized Monte Carlo token distributions.
func TestChallengerM11_2_ProviderPercentageSumInvariant_Comprehensive(t *testing.T) {
	store, cleanup := newCanonicalTestStorage(t, "m11_2_pct_invariant.db")
	defer cleanup()

	const floatEpsilon = 0.001 // Allowed tolerance per DISPATCH.md

	testCases := []struct {
		name         string
		googleTokens int64
		openaiTokens int64
		claudeTokens int64
	}{
		// Single provider
		{"Single_Google_Only", 500000, 0, 0},
		{"Single_OpenAI_Only", 0, 850000, 0},
		{"Single_Claude_Only", 0, 0, 1200000},

		// Dual provider
		{"Dual_Google_OpenAI_Equal", 500000, 500000, 0},
		{"Dual_Google_Claude_Equal", 600000, 0, 600000},
		{"Dual_OpenAI_Claude_Equal", 0, 750000, 750000},
		{"Dual_70_30", 700000, 300000, 0},
		{"Dual_99.9_0.1", 999000, 1000, 0},

		// Tri provider equal (repeating fraction: 1/3 = 33.333...%)
		{"Tri_Equal_OneThird", 100000, 100000, 100000},
		{"Tri_Equal_SingleToken", 1, 1, 1},
		{"Tri_PrimeNumbers", 1000003, 1000033, 1000037},

		// Extreme asymmetry
		{"Asymmetry_GoogleTiny", 1, 100000000, 100000000},
		{"Asymmetry_OpenAITiny", 100000000, 1, 100000000},
		{"Asymmetry_ClaudeTiny", 100000000, 100000000, 1},
		{"Asymmetry_OneVsBillion", 1, 1000000000, 0},
		{"Asymmetry_TrillionScale", 500000000000, 300000000000, 200000000000},
	}

	for idx, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sessID := fmt.Sprintf("sess-invariant-%d", idx)
			ws := "E:/GoogleDrive/WorkSpace/Code/ProjectGolang/GoLangDev/TokenMonitor"

			mockCodex := &challenger2MockCodex{
				dashboardDTO: collector.CodexDashboardDTO{
					Summary: collector.CodexSummaryDTO{
						TotalTokens:       tc.openaiTokens,
						InputTokens:       tc.openaiTokens * 8 / 10,
						CachedInputTokens: tc.openaiTokens * 2 / 10,
						OutputTokens:      tc.openaiTokens * 2 / 10,
					},
					Sessions: []collector.CodexSessionDTO{
						{
							SessionID:   sessID + "-codex",
							Workspace:   ws,
							Model:       "cx/gpt-5.5",
							TotalTokens: tc.openaiTokens,
							Turns:       1,
							Status:      "COMPLETED",
						},
					},
				},
			}

			mockClaude := &challenger2MockClaude{
				dashboardDTO: collector.ClaudeDashboardDTO{
					Sessions: []collector.ClaudeSessionDTO{
						{
							SessionID:    sessID + "-claude",
							Workspace:    "TokenMonitor",
							Model:        "claude-3-7-sonnet",
							TotalTokens:  tc.claudeTokens,
							InputTokens:  tc.claudeTokens * 7 / 10,
							OutputTokens: tc.claudeTokens * 2 / 10,
							CacheTokens:  tc.claudeTokens * 1 / 10,
							Turns:        1,
							Status:       "COMPLETED",
						},
					},
				},
			}

			// Clear logs and insert Google tokens if any
			_, _ = store.DB.Exec("DELETE FROM token_usage_logs")
			if tc.googleTokens > 0 {
				pTok := tc.googleTokens * 8 / 10
				oTok := tc.googleTokens - pTok
				cTok := tc.googleTokens * 5 / 10
				now := time.Now().UTC()
				_, err := store.DB.Exec(`
					INSERT INTO token_usage_logs (account_id, model_name, request_type, prompt_tokens, output_tokens, thinking_tokens, cached_tokens, total_tokens, latency_ms, timestamp)
					VALUES (1, 'gemini-2.5-pro', 'CHAT_convInv_001', ?, ?, 0, ?, ?, 200, ?)
				`, pTok, oTok, cTok, tc.googleTokens, now.Format("2006-01-02 15:04:05"))
				if err != nil {
					t.Fatalf("Failed to insert Google log: %v", err)
				}
			}

			res, err := store.GetProjectsLeaderboard("all", "tokens", mockCodex, mockClaude)
			if err != nil {
				t.Fatalf("GetProjectsLeaderboard failed: %v", err)
			}

			totalTokens := tc.googleTokens + tc.openaiTokens + tc.claudeTokens
			if totalTokens > 0 {
				if len(res.Projects) == 0 {
					t.Fatalf("Expected at least 1 project returned, got 0")
				}
				proj := res.Projects[0]
				gPct := proj.GoogleBreakdown.Percentage
				oPct := proj.OpenAIBreakdown.Percentage
				cPct := proj.ClaudeBreakdown.Percentage

				sum := gPct + oPct + cPct
				diff := math.Abs(100.0 - sum)

				if diff > floatEpsilon {
					t.Errorf("[%s] Invariant VIOLATED: sum = %f (diff = %f > epsilon %f) [G=%f, O=%f, C=%f]",
						tc.name, sum, diff, floatEpsilon, gPct, oPct, cPct)
				}

				// Check that individual percentages are within [0.0, 100.0 + epsilon]
				if gPct < 0.0 || gPct > 100.0+floatEpsilon {
					t.Errorf("[%s] Google pct out of bounds: %f", tc.name, gPct)
				}
				if oPct < 0.0 || oPct > 100.0+floatEpsilon {
					t.Errorf("[%s] OpenAI pct out of bounds: %f", tc.name, oPct)
				}
				if cPct < 0.0 || cPct > 100.0+floatEpsilon {
					t.Errorf("[%s] Claude pct out of bounds: %f", tc.name, cPct)
				}
			}
		})
	}

	// Monte Carlo 1,000 Random Distributions Stress Test
	t.Run("MonteCarlo_1000_RandomDistributions", func(t *testing.T) {
		r := rand.New(rand.NewSource(20260914))
		ws := "E:/GoogleDrive/WorkSpace/Code/ProjectGolang/GoLangDev/TokenMonitor"

		for iter := 0; iter < 1000; iter++ {
			gTok := r.Int63n(10000000) + 1
			oTok := r.Int63n(10000000) + 1
			cTok := r.Int63n(10000000) + 1

			mockCodex := &challenger2MockCodex{
				dashboardDTO: collector.CodexDashboardDTO{
					Summary: collector.CodexSummaryDTO{
						TotalTokens:       oTok,
						InputTokens:       oTok * 8 / 10,
						CachedInputTokens: oTok * 2 / 10,
						OutputTokens:      oTok * 2 / 10,
					},
					Sessions: []collector.CodexSessionDTO{
						{
							SessionID:   fmt.Sprintf("mc-codex-%d", iter),
							Workspace:   ws,
							Model:       "cx/gpt-5.5",
							TotalTokens: oTok,
							Turns:       1,
							Status:      "COMPLETED",
						},
					},
				},
			}

			mockClaude := &challenger2MockClaude{
				dashboardDTO: collector.ClaudeDashboardDTO{
					Sessions: []collector.ClaudeSessionDTO{
						{
							SessionID:    fmt.Sprintf("mc-claude-%d", iter),
							Workspace:    "TokenMonitor",
							Model:        "claude-3-7-sonnet",
							TotalTokens:  cTok,
							InputTokens:  cTok * 7 / 10,
							OutputTokens: cTok * 2 / 10,
							CacheTokens:  cTok * 1 / 10,
							Turns:        1,
							Status:       "COMPLETED",
						},
					},
				},
			}

			_, _ = store.DB.Exec("DELETE FROM token_usage_logs")
			pTok := gTok * 8 / 10
			outTok := gTok - pTok
			now := time.Now().UTC()
			_, err := store.DB.Exec(`
				INSERT INTO token_usage_logs (account_id, model_name, request_type, prompt_tokens, output_tokens, thinking_tokens, cached_tokens, total_tokens, latency_ms, timestamp)
				VALUES (1, 'gemini-2.5-pro', 'CHAT_convMC_001', ?, ?, 0, 0, ?, 200, ?)
			`, pTok, outTok, gTok, now.Format("2006-01-02 15:04:05"))
			if err != nil {
				t.Fatalf("Failed to insert MC Google log: %v", err)
			}

			res, err := store.GetProjectsLeaderboard("all", "tokens", mockCodex, mockClaude)
			if err != nil {
				t.Fatalf("Monte Carlo iter %d: GetProjectsLeaderboard failed: %v", iter, err)
			}

			if len(res.Projects) == 0 {
				t.Fatalf("Monte Carlo iter %d: expected 1 project, got 0", iter)
			}

			p := res.Projects[0]
			sum := p.GoogleBreakdown.Percentage + p.OpenAIBreakdown.Percentage + p.ClaudeBreakdown.Percentage
			diff := math.Abs(100.0 - sum)
			if diff > floatEpsilon {
				t.Fatalf("Monte Carlo iter %d: sum invariant failed: %f (diff %f > %f) [G=%d, O=%d, C=%d]",
					iter, sum, diff, floatEpsilon, gTok, oTok, cTok)
			}
		}
	})
}

// =========================================================================
// 2. TOKEN BREAKDOWN CONSERVATION
// =========================================================================

// TestChallengerM11_2_TokenBreakdownConservation verifies:
// 1. Cross-provider additive conservation in ProjectLeaderboardItemDTO
// 2. Grand total conservation in ProjectLeaderboardKPIsDTO
// 3. Activity conservation (TotalActivity == TotalCalls + AgentTasks)
func TestChallengerM11_2_TokenBreakdownConservation(t *testing.T) {
	store, cleanup := newCanonicalTestStorage(t, "m11_2_conservation.db")
	defer cleanup()

	now := time.Now().UTC()

	// Seed Google tokens and agent tasks
	// Google tokens: prompt 60,000, output 15,000, thinking 5,000, cached 25,000 -> total 75,000 (Prompt + Output)
	_, err := store.DB.Exec(`
		INSERT INTO token_usage_logs (account_id, model_name, request_type, prompt_tokens, output_tokens, thinking_tokens, cached_tokens, total_tokens, latency_ms, timestamp)
		VALUES (1, 'gemini-2.5-pro', 'CHAT_convCons_001', 60000, 15000, 5000, 25000, 75000, 250, ?)
	`, now.Add(-10*time.Minute).Format("2006-01-02 15:04:05"))
	if err != nil {
		t.Fatalf("Failed to insert Google log: %v", err)
	}

	_, err = store.DB.Exec(`
		INSERT INTO agent_fleet_telemetry (account_id, subagent_id, role_name, task_name, status, started_at, finished_at, duration_ms, tokens_used, tokens_offloaded)
		VALUES (1, 'sub-convCons_001', 'Verification Tester', 'Test Conservation', 'COMPLETED', ?, ?, 3000, 75000, 25000)
	`, now.Add(-10*time.Minute).Format("2006-01-02 15:04:05"), now.Add(-9*time.Minute).Format("2006-01-02 15:04:05"))
	if err != nil {
		t.Fatalf("Failed to insert fleet telemetry: %v", err)
	}

	// Seed OpenAI Codex: total 100,000 (input 80,000, cached 20,000, reasoning 10,000, output 20,000), turns 6, tools 4
	mockCodex := &challenger2MockCodex{
		dashboardDTO: collector.CodexDashboardDTO{
			Summary: collector.CodexSummaryDTO{
				TotalTokens:       100000,
				InputTokens:       80000,
				CachedInputTokens: 20000,
				OutputTokens:      20000,
				ReasoningTokens:   10000,
				ModelCalls:        6,
			},
			Sessions: []collector.CodexSessionDTO{
				{
					SessionID:   "sess-cons-codex",
					Workspace:   "E:/GoogleDrive/WorkSpace/Code/ProjectGolang/GoLangDev/TokenMonitor",
					Model:       "cx/gpt-5.5",
					TotalTokens: 100000,
					Turns:       6,
					ToolCalls:   4,
					Status:      "COMPLETED",
				},
			},
		},
	}

	// Seed Claude: total 150,000 (input 90,000, output 30,000, cache 30,000, thinking 15,000), turns 8, tools 5
	mockClaude := &challenger2MockClaude{
		dashboardDTO: collector.ClaudeDashboardDTO{
			Sessions: []collector.ClaudeSessionDTO{
				{
					SessionID:      "sess-cons-claude",
					Workspace:      "TokenMonitor",
					Model:          "claude-3-7-sonnet",
					TotalTokens:    150000,
					InputTokens:    90000,
					OutputTokens:   30000,
					CacheTokens:    30000,
					ThinkingTokens: 15000,
					Turns:          8,
					ToolCalls:      5,
					Status:         "COMPLETED",
				},
			},
		},
	}

	res, err := store.GetProjectsLeaderboard("all", "tokens", mockCodex, mockClaude)
	if err != nil {
		t.Fatalf("GetProjectsLeaderboard failed: %v", err)
	}

	if len(res.Projects) != 1 {
		t.Fatalf("Expected 1 unified project, got %d", len(res.Projects))
	}

	item := res.Projects[0]

	// 1. Total tokens additive conservation
	expectedTotalTokens := item.GoogleBreakdown.Tokens + item.OpenAIBreakdown.Tokens + item.ClaudeBreakdown.Tokens
	if item.TotalTokens != expectedTotalTokens {
		t.Errorf("TotalTokens conservation violated: item=%d, sum of breakdowns=%d",
			item.TotalTokens, expectedTotalTokens)
	}

	// 2. Prompt tokens conservation
	expectedPromptTokens := item.GoogleBreakdown.PromptTokens + item.OpenAIBreakdown.PromptTokens + item.ClaudeBreakdown.PromptTokens
	if item.PromptTokens != expectedPromptTokens {
		t.Errorf("PromptTokens conservation violated: item=%d, sum=%d", item.PromptTokens, expectedPromptTokens)
	}

	// 3. Output tokens conservation
	expectedOutputTokens := item.GoogleBreakdown.OutputTokens + item.OpenAIBreakdown.OutputTokens + item.ClaudeBreakdown.OutputTokens
	if item.OutputTokens != expectedOutputTokens {
		t.Errorf("OutputTokens conservation violated: item=%d, sum=%d", item.OutputTokens, expectedOutputTokens)
	}

	// 4. Cached tokens conservation
	expectedCachedTokens := item.GoogleBreakdown.CachedTokens + item.OpenAIBreakdown.CachedTokens + item.ClaudeBreakdown.CachedTokens
	if item.CachedTokens != expectedCachedTokens {
		t.Errorf("CachedTokens conservation violated: item=%d, sum=%d", item.CachedTokens, expectedCachedTokens)
	}

	// 5. Thinking tokens conservation
	expectedThinkingTokens := item.GoogleBreakdown.ThinkingTokens + item.OpenAIBreakdown.ThinkingTokens + item.ClaudeBreakdown.ThinkingTokens
	if item.ThinkingTokens != expectedThinkingTokens {
		t.Errorf("ThinkingTokens conservation violated: item=%d, sum=%d", item.ThinkingTokens, expectedThinkingTokens)
	}

	// 6. Calls & Tasks & Activity conservation
	expectedCalls := item.GoogleBreakdown.Calls + item.OpenAIBreakdown.Calls + item.ClaudeBreakdown.Calls
	if item.TotalCalls != expectedCalls {
		t.Errorf("TotalCalls conservation violated: item=%d, sum=%d", item.TotalCalls, expectedCalls)
	}
	expectedTasks := item.GoogleBreakdown.Tasks + item.OpenAIBreakdown.Tasks + item.ClaudeBreakdown.Tasks
	if item.AgentTasks != expectedTasks {
		t.Errorf("AgentTasks conservation violated: item=%d, sum=%d", item.AgentTasks, expectedTasks)
	}
	if item.TotalActivity != item.TotalCalls+item.AgentTasks {
		t.Errorf("TotalActivity conservation violated: TotalActivity=%d != TotalCalls(%d) + AgentTasks(%d)",
			item.TotalActivity, item.TotalCalls, item.AgentTasks)
	}
	expectedActivity := item.GoogleBreakdown.Activity + item.OpenAIBreakdown.Activity + item.ClaudeBreakdown.Activity
	if item.TotalActivity != expectedActivity {
		t.Errorf("TotalActivity breakdown conservation violated: item=%d, sum=%d", item.TotalActivity, expectedActivity)
	}

	// 7. Cost USD conservation
	expectedCost := item.GoogleBreakdown.CostUSD + item.OpenAIBreakdown.CostUSD + item.ClaudeBreakdown.CostUSD
	if math.Abs(item.EstimatedCostUSD-expectedCost) > 1e-4 {
		t.Errorf("EstimatedCostUSD conservation violated: item=%f, sum=%f", item.EstimatedCostUSD, expectedCost)
	}

	// 8. Savings USD conservation
	expectedSavings := item.GoogleBreakdown.SavingsUSD + item.OpenAIBreakdown.SavingsUSD + item.ClaudeBreakdown.SavingsUSD
	if math.Abs(item.EstimatedSavingsUSD-expectedSavings) > 1e-4 {
		t.Errorf("EstimatedSavingsUSD conservation violated: item=%f, sum=%f", item.EstimatedSavingsUSD, expectedSavings)
	}

	// 9. Grand KPI conservation
	k := res.KPIs
	if k.GrandTotalTokens != item.TotalTokens {
		t.Errorf("KPI GrandTotalTokens conservation violated: KPI=%d, Project=%d", k.GrandTotalTokens, item.TotalTokens)
	}
	if math.Abs(k.GrandTotalCostUSD-item.EstimatedCostUSD) > 1e-4 {
		t.Errorf("KPI GrandTotalCostUSD conservation violated: KPI=%f, Project=%f", k.GrandTotalCostUSD, item.EstimatedCostUSD)
	}
	if math.Abs(k.GrandTotalSavingsUSD-item.EstimatedSavingsUSD) > 1e-4 {
		t.Errorf("KPI GrandTotalSavingsUSD conservation violated: KPI=%f, Project=%f", k.GrandTotalSavingsUSD, item.EstimatedSavingsUSD)
	}
	if k.GrandTotalCalls != item.TotalCalls {
		t.Errorf("KPI GrandTotalCalls conservation violated: KPI=%d, Project=%d", k.GrandTotalCalls, item.TotalCalls)
	}
	if k.GrandTotalTasks != item.AgentTasks {
		t.Errorf("KPI GrandTotalTasks conservation violated: KPI=%d, Project=%d", k.GrandTotalTasks, item.AgentTasks)
	}
	if k.GrandTotalActivity != k.GrandTotalCalls+k.GrandTotalTasks {
		t.Errorf("KPI GrandTotalActivity conservation violated: Activity=%d != Calls(%d) + Tasks(%d)",
			k.GrandTotalActivity, k.GrandTotalCalls, k.GrandTotalTasks)
	}
	if k.GoogleTotalTokens != item.GoogleBreakdown.Tokens {
		t.Errorf("KPI GoogleTotalTokens conservation violated: KPI=%d, Item=%d", k.GoogleTotalTokens, item.GoogleBreakdown.Tokens)
	}
	if k.OpenAITotalTokens != item.OpenAIBreakdown.Tokens {
		t.Errorf("KPI OpenAITotalTokens conservation violated: KPI=%d, Item=%d", k.OpenAITotalTokens, item.OpenAIBreakdown.Tokens)
	}
	if k.ClaudeTotalTokens != item.ClaudeBreakdown.Tokens {
		t.Errorf("KPI ClaudeTotalTokens conservation violated: KPI=%d, Item=%d", k.ClaudeTotalTokens, item.ClaudeBreakdown.Tokens)
	}
	if k.GrandTotalTokens != k.GoogleTotalTokens+k.OpenAITotalTokens+k.ClaudeTotalTokens {
		t.Errorf("KPI Provider token sum conservation violated: Grand=%d, Sum=%d",
			k.GrandTotalTokens, k.GoogleTotalTokens+k.OpenAITotalTokens+k.ClaudeTotalTokens)
	}
	kpiPctSum := k.GoogleTotalPercent + k.OpenAITotalPercent + k.ClaudeTotalPercent
	if math.Abs(100.0-kpiPctSum) > 0.001 {
		t.Errorf("KPI provider percentages sum violated: sum=%f", kpiPctSum)
	}
}

// =========================================================================
// 3. UNIVERSAL FINOPS PRICING & NON-CACHED PROMPT FORMULA
// =========================================================================

// TestChallengerM11_2_FinOpsPricing_UniversalMatrix tests CalculateTokensCostUSD
// across all 10 model tiers against official rate sheets.
func TestChallengerM11_2_FinOpsPricing_UniversalMatrix(t *testing.T) {
	tiers := []struct {
		modelName    string
		promptRate1M float64
		outputRate1M float64
		cachedRate1M float64
		expectedTier string
	}{
		// 1. Anthropic Opus
		{"claude-3-opus-20240229", 15.00, 75.00, 1.50, "Opus"},
		{"claude-opus-4.6", 15.00, 75.00, 1.50, "Opus"},

		// 2. OpenAI o1
		{"o1-preview", 15.00, 60.00, 7.50, "o1"},
		{"o1-2024-12-17", 15.00, 60.00, 7.50, "o1"},

		// 3. Anthropic Sonnet
		{"claude-3-5-sonnet", 3.00, 15.00, 0.30, "Sonnet"},
		{"claude-3-7-sonnet", 3.00, 15.00, 0.30, "Sonnet"},

		// 4. Google Ultra
		{"gemini-ultra", 2.50, 10.00, 0.625, "Ultra"},
		{"google-ai-ultra-20x", 2.50, 10.00, 0.625, "Ultra"},

		// 5. OpenAI Flagship (gpt-4o)
		{"gpt-4o", 2.50, 10.00, 1.25, "GPT-4o"},
		{"gpt-4o-2024-11-20", 2.50, 10.00, 1.25, "GPT-4o"},

		// 6. Anthropic Haiku
		{"claude-3-5-haiku", 0.80, 4.00, 0.08, "Haiku"},
		{"claude-haiku-4", 0.80, 4.00, 0.08, "Haiku"},

		// 7. OpenAI o3-mini / o1-mini
		{"o3-mini", 1.10, 4.40, 0.55, "o3-mini"},
		{"o1-mini", 1.10, 4.40, 0.55, "o1-mini"},

		// 8. OpenAI gpt-4o-mini
		{"gpt-4o-mini", 0.15, 0.60, 0.075, "gpt-4o-mini"},

		// 9. Google Pro / Frontier Codex
		{"gemini-2.5-pro", 1.25, 5.00, 0.3125, "Pro/Codex"},
		{"cx/gpt-5.5", 1.25, 5.00, 0.3125, "Pro/Codex"},
		{"gpt-5.1-codex", 1.25, 5.00, 0.3125, "Pro/Codex"},

		// 10. Flash Tier / Default Fallback
		{"gemini-2.0-flash", 0.075, 0.30, 0.01875, "Flash"},
		{"gemini-3.8-flash-high", 0.075, 0.30, 0.01875, "Flash"},
		{"gpt-oss-120b", 0.075, 0.30, 0.01875, "Flash"},
		{"unknown-model-fallback", 0.075, 0.30, 0.01875, "Flash"},
	}

	for _, tt := range tiers {
		t.Run(tt.modelName, func(t *testing.T) {
			const (
				promptTok   = int64(1000000)
				outputTok   = int64(500000)
				thinkingTok = int64(200000)
				cachedTok   = int64(400000)
			)

			costUSD, savingsUSD := storage.CalculateTokensCostUSD(tt.modelName, promptTok, outputTok, thinkingTok, cachedTok)

			expectedCost := (float64(promptTok) * tt.promptRate1M / 1e6) +
				(float64(cachedTok) * tt.cachedRate1M / 1e6) +
				(float64(outputTok+thinkingTok) * tt.outputRate1M / 1e6)

			expectedSavings := float64(cachedTok) * (tt.promptRate1M - tt.cachedRate1M) / 1e6

			if math.Abs(costUSD-expectedCost) > 1e-6 {
				t.Errorf("[%s (%s)] Cost mismatch: expected %f, got %f",
					tt.modelName, tt.expectedTier, expectedCost, costUSD)
			}
			if math.Abs(savingsUSD-expectedSavings) > 1e-6 {
				t.Errorf("[%s (%s)] Savings mismatch: expected %f, got %f",
					tt.modelName, tt.expectedTier, expectedSavings, savingsUSD)
			}
			if savingsUSD < 0 {
				t.Errorf("[%s] Savings must be non-negative, got %f", tt.modelName, savingsUSD)
			}

			// Gross equivalent value verification: Gross == Cost + Savings
			gross := costUSD + savingsUSD
			expectedGross := (float64(promptTok+cachedTok) * tt.promptRate1M / 1e6) +
				(float64(outputTok+thinkingTok) * tt.outputRate1M / 1e6)

			if math.Abs(gross-expectedGross) > 1e-6 {
				t.Errorf("[%s] Gross identity failed: gross=%f, expectedGross=%f", tt.modelName, gross, expectedGross)
			}
		})
	}
}

// TestChallengerM11_2_NonCachedPromptCalculation_PreventsDoubleCharging empirically
// demonstrates and verifies the non-cached prompt formula:
//
//	nonCachedPrompt := promptTokens - cachedTokens
//	if nonCachedPrompt < 0 { nonCachedPrompt = 0 }
//
// If promptTokens was used directly when it contains cached tokens,
// the cached tokens would be billed twice: once at promptRate and once at cachedRate.
func TestChallengerM11_2_NonCachedPromptCalculation_PreventsDoubleCharging(t *testing.T) {
	const model = "cx/gpt-5.5" // Codex tier: $1.25 prompt, $5.00 output, $0.3125 cache per 1M
	const (
		totalInputTok = int64(1000000) // Total input tokens (includes cached)
		cachedTok     = int64(400000)  // Cached input tokens (40% cache hit)
		outputTok     = int64(200000)
	)

	// 1. Correct calculation: subtract cached tokens from total input to get non-cached
	nonCachedPrompt := totalInputTok - cachedTok
	if nonCachedPrompt < 0 {
		nonCachedPrompt = 0
	}
	if nonCachedPrompt != 600000 {
		t.Fatalf("Expected nonCachedPrompt = 600,000, got %d", nonCachedPrompt)
	}

	correctCost, correctSavings := storage.CalculateTokensCostUSD(model, nonCachedPrompt, outputTok, 0, cachedTok)

	// Expected with non-cached:
	// 600k * $1.25/1M = $0.75
	// 400k * $0.3125/1M = $0.125
	// 200k * $5.00/1M = $1.00
	// Total = $1.875
	expectedCorrectCost := (600000.0 * 1.25 / 1e6) + (400000.0 * 0.3125 / 1e6) + (200000.0 * 5.00 / 1e6)
	if math.Abs(correctCost-expectedCorrectCost) > 1e-6 {
		t.Errorf("Correct cost calculation mismatch: expected %f, got %f", expectedCorrectCost, correctCost)
	}

	// 2. Erroneous calculation without subtraction (double charging)
	doubleChargedCost, _ := storage.CalculateTokensCostUSD(model, totalInputTok, outputTok, 0, cachedTok)
	overchargeAmount := doubleChargedCost - correctCost
	expectedOvercharge := float64(cachedTok) * 1.25 / 1e6 // 400k * $1.25/1M = $0.50

	if math.Abs(overchargeAmount-expectedOvercharge) > 1e-6 {
		t.Errorf("Overcharge calculation mismatch: expected %f, got %f", expectedOvercharge, overchargeAmount)
	}

	// 3. Verify that repository.go GetProjectsLeaderboard implements the correct formula for Codex
	store, cleanup := newCanonicalTestStorage(t, "m11_2_double_charge.db")
	defer cleanup()

	mockCodex := &challenger2MockCodex{
		dashboardDTO: collector.CodexDashboardDTO{
			Summary: collector.CodexSummaryDTO{
				TotalTokens:       1000000,
				InputTokens:       800000,
				CachedInputTokens: 400000,
				OutputTokens:      200000,
				ReasoningTokens:   0,
			},
			Sessions: []collector.CodexSessionDTO{
				{
					SessionID:   "sess-dc-1",
					Workspace:   "E:/GoogleDrive/WorkSpace/Code/ProjectGolang/GoLangDev/TokenMonitor",
					Model:       model,
					TotalTokens: 1000000,
					Turns:       1,
					ToolCalls:   0,
					Status:      "COMPLETED",
				},
			},
		},
	}

	res, err := store.GetProjectsLeaderboard("all", "tokens", mockCodex, nil)
	if err != nil {
		t.Fatalf("GetProjectsLeaderboard failed: %v", err)
	}

	if len(res.Projects) != 1 {
		t.Fatalf("Expected 1 project, got %d", len(res.Projects))
	}

	leaderboardCost := res.Projects[0].EstimatedCostUSD
	// pTok = 800k, oTok = 200k, cTok = 400k
	// nonCachedPrompt = 800k - 400k = 400k
	// Cost = 400k * 1.25/1e6 + 400k * 0.3125/1e6 + 200k * 5.00/1e6
	// = 0.50 + 0.125 + 1.00 = $1.625
	expectedLeaderboardCost := (400000.0 * 1.25 / 1e6) + (400000.0 * 0.3125 / 1e6) + (200000.0 * 5.00 / 1e6)
	if math.Abs(leaderboardCost-expectedLeaderboardCost) > 1e-4 {
		t.Errorf("Leaderboard cost reflects double-charging: got %f, expected %f (without double charging)",
			leaderboardCost, expectedLeaderboardCost)
	}

	// 4. Verify Edge Case: cachedTokens > promptTokens
	negCached := int64(900000)
	smallerPrompt := int64(500000)
	clampedNonCached := smallerPrompt - negCached
	if clampedNonCached < 0 {
		clampedNonCached = 0
	}
	if clampedNonCached != 0 {
		t.Errorf("Expected clamped nonCachedPrompt = 0, got %d", clampedNonCached)
	}

	safeCost, _ := storage.CalculateTokensCostUSD(model, clampedNonCached, outputTok, 0, negCached)
	if safeCost < 0 {
		t.Errorf("Cost cannot be negative: got %f", safeCost)
	}

	_ = correctSavings
}

// TestChallengerM11_2_ZeroTokenBoundaryConditions tests that 0 tokens produces
// exactly 0 cost, 0 savings, 0% percentages, and no NaN or Inf anywhere in the response.
func TestChallengerM11_2_ZeroTokenBoundaryConditions(t *testing.T) {
	cost, savings := storage.CalculateTokensCostUSD("gemini-2.5-pro", 0, 0, 0, 0)
	if cost != 0.0 || savings != 0.0 {
		t.Errorf("Expected 0 tokens to produce cost=0, savings=0; got cost=%f, savings=%f", cost, savings)
	}

	if math.IsNaN(cost) || math.IsInf(cost, 0) {
		t.Errorf("Cost is NaN or Inf for 0 tokens")
	}
	if math.IsNaN(savings) || math.IsInf(savings, 0) {
		t.Errorf("Savings is NaN or Inf for 0 tokens")
	}

	store, cleanup := newCanonicalTestStorage(t, "m11_2_zero_tokens.db")
	defer cleanup()

	res, err := store.GetProjectsLeaderboard("today", "tokens", nil, nil)
	if err != nil {
		t.Fatalf("GetProjectsLeaderboard failed: %v", err)
	}

	if math.IsNaN(res.KPIs.GrandTotalCostUSD) || math.IsInf(res.KPIs.GrandTotalCostUSD, 0) {
		t.Errorf("GrandTotalCostUSD is NaN or Inf")
	}
	if math.IsNaN(res.KPIs.OverallCacheHitPercent) || math.IsInf(res.KPIs.OverallCacheHitPercent, 0) {
		t.Errorf("OverallCacheHitPercent is NaN or Inf")
	}
	if math.IsNaN(res.KPIs.GoogleTotalPercent) || math.IsInf(res.KPIs.GoogleTotalPercent, 0) {
		t.Errorf("GoogleTotalPercent is NaN or Inf")
	}
	if math.IsNaN(res.KPIs.OpenAITotalPercent) || math.IsInf(res.KPIs.OpenAITotalPercent, 0) {
		t.Errorf("OpenAITotalPercent is NaN or Inf")
	}
	if math.IsNaN(res.KPIs.ClaudeTotalPercent) || math.IsInf(res.KPIs.ClaudeTotalPercent, 0) {
		t.Errorf("ClaudeTotalPercent is NaN or Inf")
	}
}