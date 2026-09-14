package storage_test

import (
	"context"
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

// TestZeroMock_UnseededDatabase_ExactZeroInvariants empirically verifies that on an unseeded,
// completely fresh database (0 rows in all tables), all queries and endpoints return exact zero
// counts: 0 nodes, 0 projects, 0 links, 0 packets, 0 timeseries points, and 0 tokens.
func TestZeroMock_UnseededDatabase_ExactZeroInvariants(t *testing.T) {
	store, cleanup := newCanonicalTestStorage(t, "unseeded_fresh_adversarial.db")
	defer cleanup()

	ctx := context.Background()

	// 1. Empirically verify that all relevant tables have exactly 0 rows
	tableChecks := []string{"agent_fleet_telemetry", "token_usage_logs", "token_usage_hourly_rollup"}
	for _, tbl := range tableChecks {
		var cnt int
		err := store.DB.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", tbl)).Scan(&cnt)
		if err != nil {
			t.Fatalf("Failed to query count of table %s: %v", tbl, err)
		}
		if cnt != 0 {
			t.Fatalf("Precondition failed: table %s has %d rows, expected 0", tbl, cnt)
		}
	}

	// 2. Test GetAgentTopologyGraph("all")
	graphAll, err := store.GetAgentTopologyGraph("all")
	if err != nil {
		t.Fatalf("GetAgentTopologyGraph('all') returned error: %v", err)
	}
	if len(graphAll.Nodes) != 0 {
		t.Errorf("Expected exactly 0 Nodes for 'all' on empty DB, got %d", len(graphAll.Nodes))
	}
	if len(graphAll.Projects) != 0 {
		t.Errorf("Expected exactly 0 Projects for 'all' on empty DB, got %d", len(graphAll.Projects))
	}
	if len(graphAll.Links) != 0 {
		t.Errorf("Expected exactly 0 Links for 'all' on empty DB, got %d", len(graphAll.Links))
	}

	// 3. Test GetAgentTopologyGraph("today")
	graphToday, err := store.GetAgentTopologyGraph("today")
	if err != nil {
		t.Fatalf("GetAgentTopologyGraph('today') returned error: %v", err)
	}
	if len(graphToday.Nodes) != 0 {
		t.Errorf("Expected exactly 0 Nodes for 'today' on empty DB, got %d", len(graphToday.Nodes))
	}
	if len(graphToday.Projects) != 0 {
		t.Errorf("Expected exactly 0 Projects for 'today' on empty DB, got %d", len(graphToday.Projects))
	}
	if len(graphToday.Links) != 0 {
		t.Errorf("Expected exactly 0 Links for 'today' on empty DB, got %d", len(graphToday.Links))
	}

	// 4. Test GetAgentTopologyGraph with explicit time range: GetAgentTopologyGraph("all", "today")
	graphAllToday, err := store.GetAgentTopologyGraph("all", "today")
	if err != nil {
		t.Fatalf("GetAgentTopologyGraph('all', 'today') returned error: %v", err)
	}
	if len(graphAllToday.Nodes) != 0 {
		t.Errorf("Expected exactly 0 Nodes for ('all', 'today') on empty DB, got %d", len(graphAllToday.Nodes))
	}
	if len(graphAllToday.Projects) != 0 {
		t.Errorf("Expected exactly 0 Projects for ('all', 'today') on empty DB, got %d", len(graphAllToday.Projects))
	}
	if len(graphAllToday.Links) != 0 {
		t.Errorf("Expected exactly 0 Links for ('all', 'today') on empty DB, got %d", len(graphAllToday.Links))
	}

	// 5. Test GetAgentGanttPackets("all") and GetAgentGanttPackets("today")
	packetsAll, err := store.GetAgentGanttPackets("all")
	if err != nil {
		t.Fatalf("GetAgentGanttPackets('all') returned error: %v", err)
	}
	if len(packetsAll) != 0 {
		t.Errorf("Expected exactly 0 Packets for 'all' on empty DB, got %d", len(packetsAll))
	}

	packetsToday, err := store.GetAgentGanttPackets("today")
	if err != nil {
		t.Fatalf("GetAgentGanttPackets('today') returned error: %v", err)
	}
	if len(packetsToday) != 0 {
		t.Errorf("Expected exactly 0 Packets for 'today' on empty DB, got %d", len(packetsToday))
	}

	// 6. Test GetTimeSeriesDataByRange("today")
	pointsToday, err := store.GetTimeSeriesDataByRange("today")
	if err != nil {
		t.Fatalf("GetTimeSeriesDataByRange('today') returned error: %v", err)
	}
	if len(pointsToday) != 0 {
		t.Errorf("Expected exactly 0 Points for 'today' on empty DB (no historical data leakage), got %d", len(pointsToday))
	}

	// Also check other ranges for complete zero-leakage
	for _, r := range []string{"24h", "7d", "30d", "all"} {
		pts, err := store.GetTimeSeriesDataByRange(r)
		if err != nil {
			t.Fatalf("GetTimeSeriesDataByRange(%q) returned error: %v", r, err)
		}
		if len(pts) != 0 {
			t.Errorf("Expected 0 Points for %q on empty DB, got %d", r, len(pts))
		}
	}

	// 7. Test GetSummaryMetricsByRange("today")
	summaryToday, err := store.GetSummaryMetricsByRange("today")
	if err != nil {
		t.Fatalf("GetSummaryMetricsByRange('today') returned error: %v", err)
	}
	if summaryToday.TotalGrandTokens != 0 {
		t.Errorf("Expected TotalGrandTokens == 0, got %d", summaryToday.TotalGrandTokens)
	}
	if summaryToday.PromptTokens != 0 {
		t.Errorf("Expected PromptTokens == 0, got %d", summaryToday.PromptTokens)
	}
	if summaryToday.OutputTokens != 0 {
		t.Errorf("Expected OutputTokens == 0, got %d", summaryToday.OutputTokens)
	}
	if summaryToday.ThinkingTokens != 0 {
		t.Errorf("Expected ThinkingTokens == 0, got %d", summaryToday.ThinkingTokens)
	}
	if summaryToday.CachedTokens != 0 {
		t.Errorf("Expected CachedTokens == 0, got %d", summaryToday.CachedTokens)
	}
	if summaryToday.TotalCalls != 0 {
		t.Errorf("Expected TotalCalls == 0, got %d", summaryToday.TotalCalls)
	}
	if summaryToday.EstimatedCostUSD != 0.0 {
		t.Errorf("Expected EstimatedCostUSD == 0.0, got %f", summaryToday.EstimatedCostUSD)
	}
	if summaryToday.EstimatedSavingsUSD != 0.0 {
		t.Errorf("Expected EstimatedSavingsUSD == 0.0, got %f", summaryToday.EstimatedSavingsUSD)
	}
	if summaryToday.EquivalentGrossUSD != 0.0 {
		t.Errorf("Expected EquivalentGrossUSD == 0.0, got %f", summaryToday.EquivalentGrossUSD)
	}

	// 8. Test GetAgentFleetSummary("today") and ("all")
	fleetToday, err := store.GetAgentFleetSummary("today")
	if err != nil {
		t.Fatalf("GetAgentFleetSummary('today') returned error: %v", err)
	}
	if fleetToday.TotalFleet != 0 {
		t.Errorf("Expected TotalFleet == 0, got %d", fleetToday.TotalFleet)
	}
	if fleetToday.OffloadedTokens != 0 {
		t.Errorf("Expected OffloadedTokens == 0, got %d", fleetToday.OffloadedTokens)
	}
	if fleetToday.ActiveConcurrency != 0 {
		t.Errorf("Expected ActiveConcurrency == 0, got %d", fleetToday.ActiveConcurrency)
	}
}

// TestZeroMock_SeededDatabase_DynamicCalculations empirically validates that when real data
// is inserted across multiple projects and roles:
// 1. Projects are discovered dynamically without hardcoded constants.
// 2. Role nodes are generated strictly from active data (no phantom roles).
// 3. Tokens and tasks are dynamically summed per role, per project, and at the root controller.
// 4. Time range filters accurately isolate records and prevent historical data leakage.
// 5. Project filtering isolates single projects cleanly without cross-contamination.
func TestZeroMock_SeededDatabase_DynamicCalculations(t *testing.T) {
	store, cleanup := newCanonicalTestStorage(t, "seeded_adversarial.db")
	defer cleanup()

	ctx := context.Background()
	now := time.Now().UTC()

	// Seed 4 real agent tasks across 3 distinct projects
	// Project 1: TokenMonitor (GoLangDev)
	// Task A: Explorer - 40,000 tokens, COMPLETED
	// Task B: Worker   - 120,000 tokens, RUNNING
	// Sum TokenMonitor = 160,000 tokens, 2 tasks, Status = RUNNING / ACTIVE

	// Project 2: MCREDIT (ProjectR)
	// Task C: Research Agent - 85,000 tokens, COMPLETED
	// Sum MCREDIT = 85,000 tokens, 1 task, Status = COMPLETED

	// Project 3: TieuChuanHardeningLinux (Security Standards)
	// Task D: Verification Tester - 250,000 tokens, COMPLETED
	// Sum Hardening = 250,000 tokens, 1 task, Status = COMPLETED

	// Grand Total Agent Tokens = 160,000 + 85,000 + 250,000 = 495,000 tokens
	// Grand Total Agent Tasks = 4 tasks

	tasksToInsert := []struct {
		SubagentID string
		RoleName   string
		TaskName   string
		Status     string
		StartedAt  string
		FinishedAt *string
		DurationMs int64
		TokensUsed int64
		TokensOff  int64
	}{
		{
			SubagentID: "sub-tm-001",
			RoleName:   "Codebase Explorer",
			TaskName:   "TokenMonitor: Inspect schema and repository",
			Status:     "COMPLETED",
			StartedAt:  now.Add(-25 * time.Minute).Format("2006-01-02 15:04:05"),
			FinishedAt: stringPtr(now.Add(-20 * time.Minute).Format("2006-01-02 15:04:05")),
			DurationMs: 1500,
			TokensUsed: 50000,
			TokensOff:  40000,
		},
		{
			SubagentID: "sub-tm-002",
			RoleName:   "Self-Branch Worker",
			TaskName:   "TokenMonitor: Implement zero-mock dynamic engine",
			Status:     "RUNNING",
			StartedAt:  now.Add(-5 * time.Minute).Format("2006-01-02 15:04:05"),
			FinishedAt: nil,
			DurationMs: 0,
			TokensUsed: 130000,
			TokensOff:  120000,
		},
		{
			SubagentID: "sub-mcr-001",
			RoleName:   "Research Agent",
			TaskName:   "ProjectR: MCREDIT scoring pipeline analysis",
			Status:     "COMPLETED",
			StartedAt:  now.Add(-40 * time.Minute).Format("2006-01-02 15:04:05"),
			FinishedAt: stringPtr(now.Add(-35 * time.Minute).Format("2006-01-02 15:04:05")),
			DurationMs: 2500,
			TokensUsed: 95000,
			TokensOff:  85000,
		},
		{
			SubagentID: "sub-tch-001",
			RoleName:   "Verification Tester",
			TaskName:   "TieuChuanHardeningLinux: Audit CIS benchmark compliance",
			Status:     "COMPLETED",
			StartedAt:  now.Add(-60 * time.Minute).Format("2006-01-02 15:04:05"),
			FinishedAt: stringPtr(now.Add(-50 * time.Minute).Format("2006-01-02 15:04:05")),
			DurationMs: 3200,
			TokensUsed: 260000,
			TokensOff:  250000,
		},
	}

	for _, tsk := range tasksToInsert {
		var finVal any
		if tsk.FinishedAt != nil {
			finVal = *tsk.FinishedAt
		}
		_, err := store.DB.ExecContext(ctx, `
			INSERT INTO agent_fleet_telemetry (account_id, subagent_id, role_name, task_name, status, started_at, finished_at, duration_ms, tokens_used, tokens_offloaded)
			VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, tsk.SubagentID, tsk.RoleName, tsk.TaskName, tsk.Status, tsk.StartedAt, finVal, tsk.DurationMs, tsk.TokensUsed, tsk.TokensOff)
		if err != nil {
			t.Fatalf("Failed to insert task %s: %v", tsk.SubagentID, err)
		}
	}

	// Seed token usage logs
	// Log 1: today, gemini-2.5-pro, total 30,000 tokens
	// Log 2: today, gemini-2.5-flash, total 50,000 tokens
	// Log 3: 4 days ago (historical), gemini-2.5-pro, total 70,000 tokens
	log1Time := now.Add(-2 * time.Hour).Format("2006-01-02 15:04:05")
	log2Time := now.Add(-30 * time.Minute).Format("2006-01-02 15:04:05")
	log3HistoricalTime := now.Add(-96 * time.Hour).Format("2006-01-02 15:04:05")

	_, err := store.DB.ExecContext(ctx, `
		INSERT INTO token_usage_logs (account_id, model_name, prompt_tokens, thinking_tokens, output_tokens, cached_tokens, total_tokens, latency_ms, timestamp)
		VALUES 
		(1, 'gemini-2.5-pro', 15000, 3000, 5000, 7000, 30000, 450, ?),
		(1, 'gemini-2.5-flash', 25000, 5000, 10000, 10000, 50000, 280, ?),
		(1, 'gemini-2.5-pro', 50000, 0, 20000, 0, 70000, 600, ?)
	`, log1Time, log2Time, log3HistoricalTime)
	if err != nil {
		t.Fatalf("Failed to insert token_usage_logs: %v", err)
	}

	// ---------------------------------------------------------
	// VERIFICATION A: GetAgentTopologyGraph Dynamic Aggregation
	// ---------------------------------------------------------
	graph, err := store.GetAgentTopologyGraph("all")
	if err != nil {
		t.Fatalf("GetAgentTopologyGraph('all') failed: %v", err)
	}

	// 1. Exactly 3 projects discovered dynamically
	if len(graph.Projects) != 3 {
		t.Fatalf("Expected exactly 3 projects in summary, got %d", len(graph.Projects))
	}

	projMap := make(map[string]storage.ProjectSummaryDTO)
	for _, p := range graph.Projects {
		projMap[p.ID] = p
	}

	// Check Project 1: TokenMonitor
	tmProj, ok := projMap["proj-tokenmonitor"]
	if !ok {
		t.Errorf("Missing proj-tokenmonitor in Projects list")
	} else {
		if tmProj.TotalTokens != 160000 {
			t.Errorf("proj-tokenmonitor expected 160,000 tokens, got %d", tmProj.TotalTokens)
		}
		if tmProj.TotalTasks != 2 {
			t.Errorf("proj-tokenmonitor expected 2 tasks, got %d", tmProj.TotalTasks)
		}
		if tmProj.Status != "ACTIVE" {
			t.Errorf("proj-tokenmonitor expected Status ACTIVE (since task is running), got %s", tmProj.Status)
		}
	}

	// Check Project 2: MCREDIT
	mcrProj, ok := projMap["proj-mcredit"]
	if !ok {
		t.Errorf("Missing proj-mcredit in Projects list")
	} else {
		if mcrProj.TotalTokens != 85000 {
			t.Errorf("proj-mcredit expected 85,000 tokens, got %d", mcrProj.TotalTokens)
		}
		if mcrProj.TotalTasks != 1 {
			t.Errorf("proj-mcredit expected 1 task, got %d", mcrProj.TotalTasks)
		}
		if mcrProj.Status != "COMPLETED" {
			t.Errorf("proj-mcredit expected Status COMPLETED, got %s", mcrProj.Status)
		}
	}

	// Check Project 3: TieuChuanHardeningLinux
	tchProj, ok := projMap["proj-tieuchuanhardeninglinux"]
	if !ok {
		t.Errorf("Missing proj-tieuchuanhardeninglinux in Projects list")
	} else {
		if tchProj.TotalTokens != 250000 {
			t.Errorf("proj-tieuchuanhardeninglinux expected 250,000 tokens, got %d", tchProj.TotalTokens)
		}
		if tchProj.TotalTasks != 1 {
			t.Errorf("proj-tieuchuanhardeninglinux expected 1 task, got %d", tchProj.TotalTasks)
		}
		if tchProj.Status != "COMPLETED" {
			t.Errorf("proj-tieuchuanhardeninglinux expected Status COMPLETED, got %s", tchProj.Status)
		}
	}

	// 2. Check Root Node ("root-account")
	// Expected grand total = 160,000 + 85,000 + 250,000 = 495,000 tokens, 4 tasks
	var rootNode *storage.AgentNodeDTO
	for i := range graph.Nodes {
		if graph.Nodes[i].ID == "root-account" {
			rootNode = &graph.Nodes[i]
			break
		}
	}
	if rootNode == nil {
		t.Fatalf("Root node 'root-account' not found in Nodes list")
	}
	if rootNode.Tokens != 495000 {
		t.Errorf("Root node expected 495,000 tokens, got %d", rootNode.Tokens)
	}
	if rootNode.TaskCount != 4 {
		t.Errorf("Root node expected 4 tasks, got %d", rootNode.TaskCount)
	}
	if rootNode.Status != "RUNNING" {
		t.Errorf("Root node expected Status RUNNING, got %s", rootNode.Status)
	}

	// 3. Check Dynamic Role Nodes (Zero Phantom Roles)
	nodeMap := make(map[string]storage.AgentNodeDTO)
	for _, n := range graph.Nodes {
		nodeMap[n.ID] = n
	}

	// TokenMonitor: Project Hub, Orchestrator, Explorer, Worker (4 nodes)
	if _, ok := nodeMap["proj-tokenmonitor"]; !ok {
		t.Errorf("Missing Hub node for TokenMonitor")
	}
	if _, ok := nodeMap["orch-proj-tokenmonitor"]; !ok {
		t.Errorf("Missing Orchestrator node for TokenMonitor")
	}
	tmExplorer, ok := nodeMap["agent-proj-tokenmonitor-codebase-explorer"]
	if !ok {
		t.Errorf("Missing Codebase Explorer node for TokenMonitor")
	} else {
		if tmExplorer.Tokens != 40000 {
			t.Errorf("TM Explorer expected 40,000 tokens, got %d", tmExplorer.Tokens)
		}
		if tmExplorer.Status != "COMPLETED" {
			t.Errorf("TM Explorer expected Status COMPLETED, got %s", tmExplorer.Status)
		}
	}
	tmWorker, ok := nodeMap["agent-proj-tokenmonitor-self-branch-worker"]
	if !ok {
		t.Errorf("Missing Self-Branch Worker node for TokenMonitor")
	} else {
		if tmWorker.Tokens != 120000 {
			t.Errorf("TM Worker expected 120,000 tokens, got %d", tmWorker.Tokens)
		}
		if tmWorker.Status != "RUNNING" {
			t.Errorf("TM Worker expected Status RUNNING, got %s", tmWorker.Status)
		}
		// SymbolSize test: tokens > 80,000 and <= 200,000 -> size 38.0
		if tmWorker.SymbolSize != 38.0 {
			t.Errorf("TM Worker expected SymbolSize 38.0, got %f", tmWorker.SymbolSize)
		}
	}

	// Ensure phantom roles are NOT present in TokenMonitor
	phantomRolesTM := []string{
		"agent-proj-tokenmonitor-verification-tester",
		"agent-proj-tokenmonitor-research-agent",
		"agent-proj-tokenmonitor-pki-auditor",
	}
	for _, pr := range phantomRolesTM {
		if _, ok := nodeMap[pr]; ok {
			t.Errorf("Phantom role %s unexpectedly created in TokenMonitor!", pr)
		}
	}

	// MCREDIT: Hub, Orchestrator, Research Agent (3 nodes)
	mcrResearch, ok := nodeMap["agent-proj-mcredit-research-agent"]
	if !ok {
		t.Errorf("Missing Research Agent node for MCREDIT")
	} else {
		if mcrResearch.Tokens != 85000 {
			t.Errorf("MCREDIT Research expected 85,000 tokens, got %d", mcrResearch.Tokens)
		}
		if mcrResearch.Status != "COMPLETED" {
			t.Errorf("MCREDIT Research expected Status COMPLETED, got %s", mcrResearch.Status)
		}
		// SymbolSize > 80,000 and <= 200,000 -> size 38.0
		if mcrResearch.SymbolSize != 38.0 {
			t.Errorf("MCREDIT Research expected SymbolSize 38.0, got %f", mcrResearch.SymbolSize)
		}
	}

	// Hardening: Hub, Orchestrator, Verification Tester (3 nodes)
	tchTester, ok := nodeMap["agent-proj-tieuchuanhardeninglinux-verification-tester"]
	if !ok {
		t.Errorf("Missing Verification Tester node for Hardening")
	} else {
		if tchTester.Tokens != 250000 {
			t.Errorf("Hardening Tester expected 250,000 tokens, got %d", tchTester.Tokens)
		}
		// SymbolSize > 200,000 and <= 500,000 -> size 42.0
		if tchTester.SymbolSize != 42.0 {
			t.Errorf("Hardening Tester expected SymbolSize 42.0, got %f", tchTester.SymbolSize)
		}
	}

	// Total Nodes in All Graph:
	// 1 (root) + 4 (TokenMonitor) + 3 (MCREDIT) + 3 (Hardening) = 11 nodes
	if len(graph.Nodes) != 11 {
		t.Errorf("Expected exactly 11 nodes in total graph, got %d", len(graph.Nodes))
	}

	// 4. Verify Project Filtering Isolation (Zero Contamination)
	graphMCR, err := store.GetAgentTopologyGraph("proj-mcredit", "all")
	if err != nil {
		t.Fatalf("GetAgentTopologyGraph('proj-mcredit', 'all') failed: %v", err)
	}
	// For MCREDIT alone: Root Node + MCR Hub + MCR Orch + MCR Research = 4 nodes
	if len(graphMCR.Nodes) != 4 {
		t.Errorf("Filtered MCREDIT expected exactly 4 nodes, got %d", len(graphMCR.Nodes))
	}
	for _, n := range graphMCR.Nodes {
		if n.ID != "root-account" && !strings.Contains(n.ID, "mcredit") {
			t.Errorf("Node %s unexpectedly leaked into MCREDIT filtered graph", n.ID)
		}
	}

	// ---------------------------------------------------------
	// VERIFICATION B: Summary Metrics Range Isolation & Dynamic SUM
	// ---------------------------------------------------------
	// Today's summary should include Log 1 (30,000) and Log 2 (50,000) = 80,000 tokens.
	// Historical Log 3 (70,000 tokens, 4 days ago) MUST BE EXCLUDED!
	summaryToday, err := store.GetSummaryMetricsByRange("today")
	if err != nil {
		t.Fatalf("GetSummaryMetricsByRange('today') failed: %v", err)
	}
	if summaryToday.TotalGrandTokens != 80000 {
		t.Errorf("Summary 'today' expected 80,000 Grand Tokens, got %d (historical data leakage!)", summaryToday.TotalGrandTokens)
	}
	if summaryToday.PromptTokens != 40000 { // 15,000 + 25,000
		t.Errorf("Summary 'today' expected 40,000 Prompt Tokens, got %d", summaryToday.PromptTokens)
	}
	if summaryToday.ThinkingTokens != 8000 { // 3,000 + 5,000
		t.Errorf("Summary 'today' expected 8,000 Thinking Tokens, got %d", summaryToday.ThinkingTokens)
	}
	if summaryToday.OutputTokens != 15000 { // 5,000 + 10,000
		t.Errorf("Summary 'today' expected 15,000 Output Tokens, got %d", summaryToday.OutputTokens)
	}
	if summaryToday.CachedTokens != 17000 { // 7,000 + 10,000
		t.Errorf("Summary 'today' expected 17,000 Cached Tokens, got %d", summaryToday.CachedTokens)
	}
	if summaryToday.TotalCalls != 2 {
		t.Errorf("Summary 'today' expected 2 TotalCalls, got %d", summaryToday.TotalCalls)
	}
	if summaryToday.AvgTokensPerCall != 40000.0 { // 80,000 / 2
		t.Errorf("Summary 'today' expected AvgTokensPerCall 40,000.0, got %f", summaryToday.AvgTokensPerCall)
	}
	if summaryToday.EstimatedCostUSD <= 0 {
		t.Errorf("Summary 'today' expected EstimatedCostUSD > 0, got %f", summaryToday.EstimatedCostUSD)
	}

	// Summary 'all' should include all 3 logs: 30,000 + 50,000 + 70,000 = 150,000 tokens, 3 calls
	summaryAll, err := store.GetSummaryMetricsByRange("all")
	if err != nil {
		t.Fatalf("GetSummaryMetricsByRange('all') failed: %v", err)
	}
	if summaryAll.TotalGrandTokens != 150000 {
		t.Errorf("Summary 'all' expected 150,000 Grand Tokens, got %d", summaryAll.TotalGrandTokens)
	}
	if summaryAll.TotalCalls != 3 {
		t.Errorf("Summary 'all' expected 3 TotalCalls, got %d", summaryAll.TotalCalls)
	}

	// ---------------------------------------------------------
	// VERIFICATION C: Timeseries Range Isolation (Zero Historical Leakage)
	// ---------------------------------------------------------
	ptsToday, err := store.GetTimeSeriesDataByRange("today")
	if err != nil {
		t.Fatalf("GetTimeSeriesDataByRange('today') failed: %v", err)
	}
	var sumPtsToday int64
	for _, p := range ptsToday {
		sumPtsToday += p.TotalTokens
	}
	if sumPtsToday != 80000 {
		t.Errorf("Timeseries 'today' sum expected 80,000 tokens, got %d", sumPtsToday)
	}

	// Timeseries '7d' should include all records within 7 days (including historical log 3)
	pts7d, err := store.GetTimeSeriesDataByRange("7d")
	if err != nil {
		t.Fatalf("GetTimeSeriesDataByRange('7d') failed: %v", err)
	}
	var sumPts7d int64
	for _, p := range pts7d {
		sumPts7d += p.TotalTokens
	}
	if sumPts7d != 150000 {
		t.Errorf("Timeseries '7d' sum expected 150,000 tokens, got %d", sumPts7d)
	}

	// ---------------------------------------------------------
	// VERIFICATION D: Gantt Packets Dynamic Generation
	// ---------------------------------------------------------
	packetsAll, err := store.GetAgentGanttPackets("all")
	if err != nil {
		t.Fatalf("GetAgentGanttPackets('all') failed: %v", err)
	}
	if len(packetsAll) == 0 {
		t.Errorf("Expected Gantt packets to be dynamically generated for multi-task telemetry, got 0")
	}
	for _, pkt := range packetsAll {
		if pkt.Tokens <= 0 {
			t.Errorf("Packet %s has 0 or negative tokens: %d", pkt.ID, pkt.Tokens)
		}
		if pkt.FromRole == pkt.ToRole {
			t.Errorf("Packet %s is an invalid self-loop between identical roles: %s", pkt.ID, pkt.FromRole)
		}
	}
}

// TestZeroMock_HTTP_API_Endpoints_FreshAndSeeded empirically asserts the full JSON serialization
// of HTTP endpoints for both an unseeded fresh database (zero mock invariants) and seeded database.
func TestZeroMock_HTTP_API_Endpoints_FreshAndSeeded(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get wd: %v", err)
	}
	if filepath.Base(wd) == "storage" {
		_ = os.Chdir("..")
		defer func() { _ = os.Chdir(wd) }()
	}

	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "http_adversarial.db")

	cfg := &config.Config{
		Database: config.DatabaseConfig{
			SQLitePath:    dbPath,
			MaxOpenConns:  5,
			MaxIdleConns:  2,
			EnableWALMode: true,
		},
		AccountProfile: config.AccountProfileConfig{
			Email: "adversarial_web@example.com",
		},
	}

	store, err := storage.NewStorage(cfg)
	if err != nil {
		t.Fatalf("NewStorage failed: %v", err)
	}
	defer store.Close()

	buf := collector.NewAsyncBuffer(store, 1000, 100, 50*time.Millisecond)
	buf.Start()
	defer buf.Stop()

	srv := web.NewServer(store, buf, nil)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	client := ts.Client()

	// PHASE 1: HTTP API on Fresh / Unseeded DB (0 rows)
	t.Run("HTTP_Fresh_Empty_Invariants", func(t *testing.T) {
		// 1. GET /api/agents/graph?range=today
		resp, err := client.Get(ts.URL + "/api/agents/graph?range=today")
		if err != nil {
			t.Fatalf("GET /api/agents/graph?range=today failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET /api/agents/graph?range=today status: %d", resp.StatusCode)
		}
		var graph storage.AgentTopologyGraphDTO
		if err := json.NewDecoder(resp.Body).Decode(&graph); err != nil {
			t.Fatalf("Failed to decode graph JSON: %v", err)
		}
		if len(graph.Nodes) != 0 {
			t.Errorf("HTTP: Expected 0 Nodes on empty DB, got %d", len(graph.Nodes))
		}
		if len(graph.Projects) != 0 {
			t.Errorf("HTTP: Expected 0 Projects on empty DB, got %d", len(graph.Projects))
		}
		if len(graph.Links) != 0 {
			t.Errorf("HTTP: Expected 0 Links on empty DB, got %d", len(graph.Links))
		}

		// 2. GET /api/agents/gantt/packets?range=all
		respPackets, err := client.Get(ts.URL + "/api/agents/gantt/packets?range=all")
		if err != nil {
			t.Fatalf("GET /api/agents/gantt/packets?range=all failed: %v", err)
		}
		defer respPackets.Body.Close()
		var packets []storage.AgentGanttPacketDTO
		if err := json.NewDecoder(respPackets.Body).Decode(&packets); err != nil {
			t.Fatalf("Failed to decode packets JSON: %v", err)
		}
		if len(packets) != 0 {
			t.Errorf("HTTP: Expected 0 Gantt packets on empty DB, got %d", len(packets))
		}

		// 3. GET /api/metrics/timeseries?range=today
		respTS, err := client.Get(ts.URL + "/api/metrics/timeseries?range=today")
		if err != nil {
			t.Fatalf("GET /api/metrics/timeseries?range=today failed: %v", err)
		}
		defer respTS.Body.Close()
		bodyBytes, _ := io.ReadAll(respTS.Body)
		bodyStr := strings.TrimSpace(string(bodyBytes))
		if bodyStr != "null" && bodyStr != "[]" {
			t.Errorf("HTTP: Expected null or [] for empty timeseries, got %s", bodyStr)
		}

		// 4. GET /api/metrics/summary?range=today
		respSum, err := client.Get(ts.URL + "/api/metrics/summary?range=today")
		if err != nil {
			t.Fatalf("GET /api/metrics/summary?range=today failed: %v", err)
		}
		defer respSum.Body.Close()
		var sumDTO storage.SummaryMetricsDTO
		if err := json.NewDecoder(respSum.Body).Decode(&sumDTO); err != nil {
			t.Fatalf("Failed to decode summary JSON: %v", err)
		}
		if sumDTO.TotalGrandTokens != 0 {
			t.Errorf("HTTP: Expected 0 TotalGrandTokens on empty DB, got %d", sumDTO.TotalGrandTokens)
		}
		if sumDTO.TotalCalls != 0 {
			t.Errorf("HTTP: Expected 0 TotalCalls on empty DB, got %d", sumDTO.TotalCalls)
		}
		if sumDTO.EstimatedCostUSD != 0.0 {
			t.Errorf("HTTP: Expected 0.0 Cost on empty DB, got %f", sumDTO.EstimatedCostUSD)
		}
	})

	// PHASE 2: Seed records and test HTTP API
	t.Run("HTTP_Seeded_Dynamic_Invariants", func(t *testing.T) {
		now := time.Now().UTC()
		// Seed 1 agent record
		_, err := store.DB.Exec(`
			INSERT INTO agent_fleet_telemetry (account_id, subagent_id, role_name, task_name, status, started_at, finished_at, duration_ms, tokens_used, tokens_offloaded)
			VALUES (1, 'sub-tm-http', 'Codebase Explorer', 'TokenMonitor: HTTP test scan', 'COMPLETED', ?, ?, 1000, 20000, 18000)
		`, now.Add(-10*time.Minute).Format("2006-01-02 15:04:05"), now.Add(-8*time.Minute).Format("2006-01-02 15:04:05"))
		if err != nil {
			t.Fatalf("Failed to seed agent record: %v", err)
		}

		// Seed 1 token usage log
		_, err = store.DB.Exec(`
			INSERT INTO token_usage_logs (account_id, model_name, prompt_tokens, thinking_tokens, output_tokens, cached_tokens, total_tokens, latency_ms, timestamp)
			VALUES (1, 'gemini-2.5-pro', 8000, 1000, 2000, 4000, 15000, 320, ?)
		`, now.Add(-5*time.Minute).Format("2006-01-02 15:04:05"))
		if err != nil {
			t.Fatalf("Failed to seed token log: %v", err)
		}

		// Verify GET /api/agents/graph?range=today reflects seeded data
		resp, err := client.Get(ts.URL + "/api/agents/graph?range=today")
		if err != nil {
			t.Fatalf("GET /api/agents/graph?range=today failed: %v", err)
		}
		defer resp.Body.Close()
		var graph storage.AgentTopologyGraphDTO
		if err := json.NewDecoder(resp.Body).Decode(&graph); err != nil {
			t.Fatalf("Failed to decode graph JSON: %v", err)
		}
		if len(graph.Projects) != 1 {
			t.Errorf("HTTP Seeded: Expected 1 project, got %d", len(graph.Projects))
		}
		// Nodes: Root (1) + Hub (1) + Orch (1) + Explorer (1) = 4
		if len(graph.Nodes) != 4 {
			t.Errorf("HTTP Seeded: Expected 4 nodes, got %d", len(graph.Nodes))
		}

		// Verify GET /api/metrics/summary?range=today reflects seeded data
		respSum, err := client.Get(ts.URL + "/api/metrics/summary?range=today")
		if err != nil {
			t.Fatalf("GET /api/metrics/summary?range=today failed: %v", err)
		}
		defer respSum.Body.Close()
		var sumDTO storage.SummaryMetricsDTO
		if err := json.NewDecoder(respSum.Body).Decode(&sumDTO); err != nil {
			t.Fatalf("Failed to decode summary JSON: %v", err)
		}
		if sumDTO.TotalGrandTokens != 15000 {
			t.Errorf("HTTP Seeded: Expected TotalGrandTokens 15,000, got %d", sumDTO.TotalGrandTokens)
		}
		if sumDTO.TotalCalls != 1 {
			t.Errorf("HTTP Seeded: Expected TotalCalls 1, got %d", sumDTO.TotalCalls)
		}
	})
}

func stringPtr(s string) *string {
	return &s
}
