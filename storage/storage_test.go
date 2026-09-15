package storage_test

import (
	"context"
	"fmt"
	"math"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"tokenmonitor/config"
	"tokenmonitor/storage"
)

func newCanonicalTestStorage(t *testing.T, dbName string) (*storage.Storage, func()) {
	t.Helper()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, dbName)

	cfg := &config.Config{
		Database: config.DatabaseConfig{
			SQLitePath:              dbPath,
			MaxOpenConns:            10,
			MaxIdleConns:            5,
			ConnMaxLifetimeMinutes: 10,
			EnableWALMode:           true,
			RollupIntervalSeconds:   300,
		},
		AccountProfile: config.AccountProfileConfig{
			Email:              "canonical_test@example.com",
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
		t.Fatalf("Failed to initialize Storage: %v", err)
	}

	cleanup := func() {
		_ = store.Close()
	}
	return store, cleanup
}

// TestStorage_Canonical_Pragmas verifies that all 6 required PRAGMAs are active
func TestStorage_Canonical_Pragmas(t *testing.T) {
	store, cleanup := newCanonicalTestStorage(t, "test_pragmas.db")
	defer cleanup()

	ctx := context.Background()
	conn, err := store.DB.Conn(ctx)
	if err != nil {
		t.Fatalf("Failed to get connection: %v", err)
	}
	defer conn.Close()

	// 1. foreign_keys = 1 (ON)
	var fk int
	if err := conn.QueryRowContext(ctx, "PRAGMA foreign_keys;").Scan(&fk); err != nil {
		t.Fatalf("PRAGMA foreign_keys query failed: %v", err)
	}
	if fk != 1 {
		t.Errorf("Expected foreign_keys=1, got %d", fk)
	}

	// 2. journal_mode = wal
	var jm string
	if err := conn.QueryRowContext(ctx, "PRAGMA journal_mode;").Scan(&jm); err != nil {
		t.Fatalf("PRAGMA journal_mode query failed: %v", err)
	}
	if strings.ToLower(jm) != "wal" {
		t.Errorf("Expected journal_mode=wal, got %s", jm)
	}

	// 3. synchronous = 1 (NORMAL)
	var syncMode int
	if err := conn.QueryRowContext(ctx, "PRAGMA synchronous;").Scan(&syncMode); err != nil {
		t.Fatalf("PRAGMA synchronous query failed: %v", err)
	}
	if syncMode != 1 {
		t.Errorf("Expected synchronous=1 (NORMAL), got %d", syncMode)
	}

	// 4. cache_size = -64000
	var cacheSize int
	if err := conn.QueryRowContext(ctx, "PRAGMA cache_size;").Scan(&cacheSize); err != nil {
		t.Fatalf("PRAGMA cache_size query failed: %v", err)
	}
	if cacheSize != -64000 {
		t.Errorf("Expected cache_size=-64000, got %d", cacheSize)
	}

	// 5. temp_store = 2 (MEMORY)
	var tempStore int
	if err := conn.QueryRowContext(ctx, "PRAGMA temp_store;").Scan(&tempStore); err != nil {
		t.Fatalf("PRAGMA temp_store query failed: %v", err)
	}
	if tempStore != 2 {
		t.Errorf("Expected temp_store=2 (MEMORY), got %d", tempStore)
	}

	// 6. busy_timeout = 5000
	var busyTimeout int
	if err := conn.QueryRowContext(ctx, "PRAGMA busy_timeout;").Scan(&busyTimeout); err != nil {
		t.Fatalf("PRAGMA busy_timeout query failed: %v", err)
	}
	if busyTimeout != 5000 {
		t.Errorf("Expected busy_timeout=5000, got %d", busyTimeout)
	}
}

// TestStorage_Canonical_SchemaTablesAndIndexes verifies that 4 tables and 4 indexes exist
func TestStorage_Canonical_SchemaTablesAndIndexes(t *testing.T) {
	store, cleanup := newCanonicalTestStorage(t, "test_schema.db")
	defer cleanup()

	expectedTables := []string{
		"accounts",
		"auth_sessions",
		"token_usage_logs",
		"token_usage_hourly_rollup",
	}

	for _, table := range expectedTables {
		var count int
		query := "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?"
		if err := store.DB.QueryRow(query, table).Scan(&count); err != nil {
			t.Fatalf("Query table %s failed: %v", table, err)
		}
		if count != 1 {
			t.Errorf("Expected table %s to exist in sqlite_master", table)
		}
	}

	expectedIndexes := []string{
		"idx_token_usage_timestamp",
		"idx_token_usage_account_model",
		"idx_hourly_bucket",
		"idx_token_dedup_chat",
	}

	for _, idx := range expectedIndexes {
		var count int
		query := "SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?"
		if err := store.DB.QueryRow(query, idx).Scan(&count); err != nil {
			t.Fatalf("Query index %s failed: %v", idx, err)
		}
		if count != 1 {
			t.Errorf("Expected index %s to exist in sqlite_master", idx)
		}
	}
}

// TestStorage_Canonical_FKCascade verifies ON DELETE CASCADE across child tables
func TestStorage_Canonical_FKCascade(t *testing.T) {
	store, cleanup := newCanonicalTestStorage(t, "test_fk_cascade.db")
	defer cleanup()

	accountID := int64(888)
	_, err := store.DB.Exec(`
		INSERT INTO accounts (id, account_email, installation_uuid, registered_at, subscription_expiry)
		VALUES (?, 'cascade_test@example.com', 'uuid-cascade', '2026-09-08 00:00:00', '2026-10-08 00:00:00')
	`, accountID)
	if err != nil {
		t.Fatalf("Failed to insert parent account: %v", err)
	}

	// Insert child rows in auth_sessions, token_usage_logs, token_usage_hourly_rollup
	_, err = store.DB.Exec(`
		INSERT INTO auth_sessions (account_id, token_status, token_issued_at, token_expires_at)
		VALUES (?, 'VALID', '2026-09-08 00:00:00', '2026-09-08 01:00:00')
	`, accountID)
	if err != nil {
		t.Fatalf("Failed to insert auth_session: %v", err)
	}

	_, err = store.DB.Exec(`
		INSERT INTO token_usage_logs (account_id, timestamp, model_name, prompt_tokens, output_tokens, total_tokens)
		VALUES (?, '2026-09-08 00:10:00', 'gemini-test', 100, 50, 150)
	`, accountID)
	if err != nil {
		t.Fatalf("Failed to insert token_usage_log: %v", err)
	}

	_, err = store.DB.Exec(`
		INSERT INTO token_usage_hourly_rollup (account_id, time_bucket, model_name, call_count, sum_total_tokens)
		VALUES (?, '2026-09-08 00:00:00', 'gemini-test', 1, 150)
	`, accountID)
	if err != nil {
		t.Fatalf("Failed to insert token_usage_hourly_rollup: %v", err)
	}

	// Delete parent account
	_, err = store.DB.Exec("DELETE FROM accounts WHERE id = ?", accountID)
	if err != nil {
		t.Fatalf("Delete parent account failed: %v", err)
	}

	// Assert children were cascaded
	childTables := []string{"auth_sessions", "token_usage_logs", "token_usage_hourly_rollup"}
	for _, table := range childTables {
		var count int
		query := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE account_id = ?", table)
		if err := store.DB.QueryRow(query, accountID).Scan(&count); err != nil {
			t.Fatalf("Query %s failed: %v", table, err)
		}
		if count != 0 {
			t.Errorf("CASCADE failed for %s: expected 0 rows, got %d", table, count)
		}
	}
}

// TestStorage_Canonical_RollupHourlyMetrics_MathAndIdempotency verifies aggregation math
// and idempotent repeated runs
func TestStorage_Canonical_RollupHourlyMetrics_MathAndIdempotency(t *testing.T) {
	store, cleanup := newCanonicalTestStorage(t, "test_rollup.db")
	defer cleanup()

	// Clear out tables for pristine baseline
	store.DB.Exec("DELETE FROM token_usage_hourly_rollup")
	store.DB.Exec("DELETE FROM token_usage_logs")

	accountID := int64(1)

	// Insert 3 records in hour 2026-09-08 14:00:00 for model gemini-2.5-pro
	records := []struct {
		ts       string
		prompt   int64
		output   int64
		thinking int64
		cached   int64
		latency  int
	}{
		{"2026-09-08 14:05:00", 1000, 200, 50, 800, 1000},
		{"2026-09-08 14:20:00", 2000, 400, 100, 1600, 1500},
		{"2026-09-08 14:50:00", 3000, 600, 150, 2400, 2000},
	}

	for _, r := range records {
		total := r.prompt + r.output
		_, err := store.DB.Exec(`
			INSERT INTO token_usage_logs (
				account_id, timestamp, model_name, prompt_tokens, output_tokens,
				thinking_tokens, cached_tokens, total_tokens, latency_ms, status_code, request_type
			) VALUES (?, ?, 'gemini-2.5-pro', ?, ?, ?, ?, ?, ?, 200, 'TEST')
		`, accountID, r.ts, r.prompt, r.output, r.thinking, r.cached, total, r.latency)
		if err != nil {
			t.Fatalf("Failed to insert log: %v", err)
		}
	}

	// 1. Run Rollup
	if err := store.RollupHourlyMetrics(); err != nil {
		t.Fatalf("RollupHourlyMetrics failed: %v", err)
	}

	// Verify Rollup row
	var callCount, sumPrompt, sumOutput, sumThinking, sumCached, sumTotal int64
	var avgLatency float64
	query := `
		SELECT call_count, sum_prompt_tokens, sum_output_tokens, sum_thinking_tokens,
		       sum_cached_tokens, sum_total_tokens, avg_latency_ms
		FROM token_usage_hourly_rollup
		WHERE account_id = ? AND time_bucket = '2026-09-08 14:00:00' AND model_name = 'gemini-2.5-pro'
	`
	err := store.DB.QueryRow(query, accountID).Scan(
		&callCount, &sumPrompt, &sumOutput, &sumThinking, &sumCached, &sumTotal, &avgLatency,
	)
	if err != nil {
		t.Fatalf("Failed to query rollup row: %v", err)
	}

	if callCount != 3 {
		t.Errorf("Expected call_count=3, got %d", callCount)
	}
	if sumPrompt != 6000 {
		t.Errorf("Expected sumPrompt=6000, got %d", sumPrompt)
	}
	if sumOutput != 1200 {
		t.Errorf("Expected sumOutput=1200, got %d", sumOutput)
	}
	if sumThinking != 300 {
		t.Errorf("Expected sumThinking=300, got %d", sumThinking)
	}
	if sumCached != 4800 {
		t.Errorf("Expected sumCached=4800, got %d", sumCached)
	}
	if sumTotal != 7200 {
		t.Errorf("Expected sumTotal=7200, got %d", sumTotal)
	}
	expectedAvgLatency := 1500.0 // (1000 + 1500 + 2000) / 3
	if math.Abs(avgLatency-expectedAvgLatency) > 0.01 {
		t.Errorf("Expected avgLatency=%.2f, got %.2f", expectedAvgLatency, avgLatency)
	}

	// 2. Idempotency test: Re-run rollup 3 more times
	for run := 0; run < 3; run++ {
		if err := store.RollupHourlyMetrics(); err != nil {
			t.Fatalf("Repeat RollupHourlyMetrics run %d failed: %v", run+1, err)
		}
	}

	var totalRollupRows int
	store.DB.QueryRow("SELECT COUNT(*) FROM token_usage_hourly_rollup").Scan(&totalRollupRows)
	if totalRollupRows != 1 {
		t.Errorf("Idempotency violation: expected exactly 1 rollup row, got %d", totalRollupRows)
	}
}

// TestStorage_Canonical_RangeQueries verifies range query APIs
func TestStorage_Canonical_RangeQueries(t *testing.T) {
	store, cleanup := newCanonicalTestStorage(t, "test_ranges.db")
	defer cleanup()

	// Insert events at current time
	now := time.Now()
	_, err := store.DB.Exec(`
		INSERT INTO token_usage_logs (account_id, timestamp, model_name, prompt_tokens, output_tokens, total_tokens, latency_ms)
		VALUES (1, ?, 'gemini-range-test', 500, 250, 750, 400)
	`, now.Format("2006-01-02 15:04:05"))
	if err != nil {
		t.Fatalf("Failed to insert log: %v", err)
	}

	// 1. GetSummaryMetricsByRange
	ranges := []string{"24h", "7d", "30d", "all"}
	for _, r := range ranges {
		m, err := store.GetSummaryMetricsByRange(r)
		if err != nil {
			t.Errorf("GetSummaryMetricsByRange(%q) failed: %v", r, err)
		} else if m == nil {
			t.Errorf("GetSummaryMetricsByRange(%q) returned nil", r)
		}
	}

	// 2. GetTimeSeriesDataByRange
	for _, r := range []string{"24h", "7d", "30d"} {
		pts, err := store.GetTimeSeriesDataByRange(r)
		if err != nil {
			t.Errorf("GetTimeSeriesDataByRange(%q) failed: %v", r, err)
		}
		if pts == nil {
			t.Errorf("GetTimeSeriesDataByRange(%q) returned nil", r)
		}
	}

	// 3. GetDailySummariesByRange
	for _, r := range []string{"7d", "30d"} {
		daily, err := store.GetDailySummariesByRange(r)
		if err != nil {
			t.Errorf("GetDailySummariesByRange(%q) failed: %v", r, err)
		}
		if daily == nil {
			t.Errorf("GetDailySummariesByRange(%q) returned nil", r)
		}
	}

	// 4. GetModelDistribution
	dist, err := store.GetModelDistribution("all")
	if err != nil {
		t.Fatalf("GetModelDistribution failed: %v", err)
	}
	if len(dist) == 0 {
		t.Errorf("Expected model distribution to contain at least 1 entry, got 0")
	}

	// 5. GetModelTimeSeries
	mts, err := store.GetModelTimeSeries("30d", "gemini-range-test")
	if err != nil {
		t.Fatalf("GetModelTimeSeries failed: %v", err)
	}
	if mts == nil {
		t.Errorf("Expected non-nil model timeseries")
	}
}

// TestStorage_Canonical_DetectActiveAntigravityAccount tests safe execution of detector
func TestStorage_Canonical_DetectActiveAntigravityAccount(t *testing.T) {
	name, email, plan, err := storage.DetectActiveAntigravityAccount()
	if err != nil {
		t.Logf("DetectActiveAntigravityAccount returned error (expected in isolated test environment): %v", err)
	} else {
		t.Logf("Detected account: name=%s, email=%s, plan=%s", name, email, plan)
	}
}

// TestAgentTopologyGraph tests GetAgentTopologyGraph functionality and project filtering
func TestAgentTopologyGraph(t *testing.T) {
	tempDir := t.TempDir()
	t.Cleanup(func() {
		runtime.GC()
		time.Sleep(25 * time.Millisecond)
	})
	dbPath := filepath.Join(tempDir, "test_topo_graph.db")

	cfg := &config.Config{
		Database: config.DatabaseConfig{
			SQLitePath:    dbPath,
			MaxOpenConns:  2,
			MaxIdleConns:  1,
			EnableWALMode: true,
		},
	}

	store, err := storage.NewStorage(cfg)
	if err != nil {
		t.Fatalf("NewStorage failed: %v", err)
	}
	defer store.Close()

	// 0. Verify genuine empty structure on unseeded database
	emptyGraph, err := store.GetAgentTopologyGraph("all")
	if err != nil {
		t.Fatalf("GetAgentTopologyGraph('all') on empty DB failed: %v", err)
	}
	if len(emptyGraph.Nodes) != 0 || len(emptyGraph.Links) != 0 || len(emptyGraph.Projects) != 0 {
		t.Errorf("Expected 0 nodes, links, projects on empty DB, got nodes=%d, links=%d, projects=%d",
			len(emptyGraph.Nodes), len(emptyGraph.Links), len(emptyGraph.Projects))
	}
	if len(emptyGraph.Categories) != 8 {
		t.Errorf("Expected 8 categories, got %d", len(emptyGraph.Categories))
	}

	// Seed genuine test telemetry records
	now := time.Now().UTC()
	_, err = store.DB.Exec(`
		INSERT INTO agent_fleet_telemetry (account_id, subagent_id, role_name, task_name, status, started_at, finished_at, duration_ms, tokens_used, tokens_offloaded)
		VALUES 
		(1, 'sub-tm-001', 'Codebase Explorer', 'Analyze storage repository', 'COMPLETED', ?, ?, 1500, 20000, 10000),
		(1, 'sub-tm-002', 'Self-Branch Worker', 'Implement zero-mock telemetry', 'RUNNING', ?, NULL, 0, 30000, 15000),
		(1, 'sub-tm-003', 'Verification Tester', 'Run unit tests', 'COMPLETED', ?, ?, 800, 10000, 5000),
		(1, 'sub-tch-001', 'Codebase Explorer', 'TieuChuanHardeningLinux: Inspect Huong_dan_cau_hinh_may_chu_Linux_v1.md', 'COMPLETED', ?, ?, 2000, 50000, 25000)
	`, now.Add(-30*time.Minute).Format("2006-01-02 15:04:05"),
		now.Add(-28*time.Minute).Format("2006-01-02 15:04:05"),
		now.Add(-20*time.Minute).Format("2006-01-02 15:04:05"),
		now.Add(-15*time.Minute).Format("2006-01-02 15:04:05"),
		now.Add(-14*time.Minute).Format("2006-01-02 15:04:05"),
		now.Add(-10*time.Minute).Format("2006-01-02 15:04:05"),
		now.Add(-8*time.Minute).Format("2006-01-02 15:04:05"))
	if err != nil {
		t.Fatalf("Failed to seed test telemetry: %v", err)
	}

	// 1. Test default graph (all projects)
	graph, err := store.GetAgentTopologyGraph("all")
	if err != nil {
		t.Fatalf("GetAgentTopologyGraph('all') failed: %v", err)
	}
	if graph == nil {
		t.Fatalf("Expected non-nil graph")
	}
	if len(graph.Nodes) == 0 {
		t.Errorf("Expected nodes in graph, got 0")
	}
	if len(graph.Links) == 0 {
		t.Errorf("Expected links in graph, got 0")
	}
	if len(graph.Projects) == 0 {
		t.Errorf("Expected projects in graph, got 0")
	}
	if len(graph.Categories) == 0 {
		t.Errorf("Expected categories in graph, got 0")
	}

	// 2. Test filtered graph
	filtered, err := store.GetAgentTopologyGraph("proj-tokenmonitor")
	if err != nil {
		t.Fatalf("GetAgentTopologyGraph('proj-tokenmonitor') failed: %v", err)
	}
	if len(filtered.Nodes) == 0 {
		t.Errorf("Expected nodes in filtered graph, got 0")
	}
	for _, node := range filtered.Nodes {
		if node.ProjectID != "proj-tokenmonitor" && node.ID != "root-account" {
			t.Errorf("Expected node ProjectID to be 'proj-tokenmonitor', got %s", node.ProjectID)
		}
	}

	// 2b. Test filtered graph for TieuChuanHardeningLinux
	hardeningFiltered, err := store.GetAgentTopologyGraph("proj-tieuchuanhardeninglinux")
	if err != nil {
		t.Fatalf("GetAgentTopologyGraph('proj-tieuchuanhardeninglinux') failed: %v", err)
	}
	if len(hardeningFiltered.Nodes) == 0 {
		t.Errorf("Expected nodes in hardeningFiltered graph, got 0")
	}
	for _, node := range hardeningFiltered.Nodes {
		if node.ProjectID != "proj-tieuchuanhardeninglinux" && node.ID != "root-account" {
			t.Errorf("Expected node ProjectID to be 'proj-tieuchuanhardeninglinux', got %s", node.ProjectID)
		}
	}

	// 3. Test today range
	todayGraph, err := store.GetAgentTopologyGraph("", "today")
	if err != nil {
		t.Fatalf("GetAgentTopologyGraph('', 'today') failed: %v", err)
	}
	if todayGraph == nil {
		t.Fatalf("Expected non-nil todayGraph")
	}
	if len(todayGraph.Nodes) == 0 {
		t.Errorf("Expected nodes in todayGraph, got 0")
	}
}

// TestAgentGanttPackets tests GetAgentGanttPackets functionality
func TestAgentGanttPackets(t *testing.T) {
	tempDir := t.TempDir()
	t.Cleanup(func() {
		runtime.GC()
		time.Sleep(25 * time.Millisecond)
	})
	dbPath := filepath.Join(tempDir, "test_gantt_packets.db")

	cfg := &config.Config{
		Database: config.DatabaseConfig{
			SQLitePath:    dbPath,
			MaxOpenConns:  2,
			MaxIdleConns:  1,
			EnableWALMode: true,
		},
	}

	store, err := storage.NewStorage(cfg)
	if err != nil {
		t.Fatalf("NewStorage failed: %v", err)
	}
	defer store.Close()

	// 0. Verify genuine empty slice on unseeded database
	emptyPackets, err := store.GetAgentGanttPackets("all")
	if err != nil {
		t.Fatalf("GetAgentGanttPackets('all') on empty DB failed: %v", err)
	}
	if len(emptyPackets) != 0 {
		t.Errorf("Expected 0 packets on empty DB, got %d", len(emptyPackets))
	}

	// Seed sequential subagent tasks with different roles under the same session
	now := time.Now().UTC()
	_, err = store.DB.Exec(`
		INSERT INTO agent_fleet_telemetry (account_id, subagent_id, role_name, task_name, status, started_at, finished_at, duration_ms, tokens_used, tokens_offloaded)
		VALUES 
		(1, 'sub-session1-001', 'Codebase Explorer', 'Analyze storage repository', 'COMPLETED', ?, ?, 1500, 20000, 10000),
		(1, 'sub-session1-002', 'Self-Branch Worker', 'Implement zero-mock telemetry', 'COMPLETED', ?, ?, 2000, 30000, 15000),
		(1, 'sub-session1-003', 'Verification Tester', 'Run unit tests', 'COMPLETED', ?, ?, 800, 10000, 5000)
	`, now.Add(-30*time.Minute).Format("2006-01-02 15:04:05"),
		now.Add(-28*time.Minute).Format("2006-01-02 15:04:05"),
		now.Add(-25*time.Minute).Format("2006-01-02 15:04:05"),
		now.Add(-20*time.Minute).Format("2006-01-02 15:04:05"),
		now.Add(-15*time.Minute).Format("2006-01-02 15:04:05"),
		now.Add(-10*time.Minute).Format("2006-01-02 15:04:05"))
	if err != nil {
		t.Fatalf("Failed to seed test telemetry: %v", err)
	}

	// 1. Test GetAgentGanttPackets with seeded records
	packets, err := store.GetAgentGanttPackets("all")
	if err != nil {
		t.Fatalf("GetAgentGanttPackets('all') failed: %v", err)
	}
	if len(packets) == 0 {
		t.Errorf("Expected packets in result, got 0")
	}

	// Verify required packet properties (colors, types, labels) and no fake demo packets
	typesFound := make(map[string]bool)
	for _, p := range packets {
		if strings.HasPrefix(p.ID, "pkt-demo-") {
			t.Errorf("Detected fake demo packet ID %s in result", p.ID)
		}
		if p.Color == "" {
			t.Errorf("Packet %s has empty color", p.ID)
		}
		if p.PacketType == "" {
			t.Errorf("Packet %s has empty type", p.ID)
		}
		typesFound[p.PacketType] = true
	}
	if len(typesFound) == 0 {
		t.Errorf("Expected distinct packet types")
	}
}

// TestExtractProjectFromPathString_DynamicNewProjects verifies that new projects created in any directory
// (including subfolders of GoLangDev or arbitrary drives) are dynamically resolved to their own project ID and name
// rather than being hijacked by TokenMonitor.
func TestExtractProjectFromPathString_DynamicNewProjects(t *testing.T) {
	testCases := []struct {
		inputPath    string
		expectedID   string
		expectedName string
	}{
		{
			inputPath:    `e:\GoogleDrive\WorkSpace\Code\ProjectGolang\GoLangDev\MyNewPaymentGateway`,
			expectedID:   "proj-mynewpaymentgateway",
			expectedName: "MyNewPaymentGateway",
		},
		{
			inputPath:    `D:\Projects\EcommerceBackend`,
			expectedID:   "proj-ecommercebackend",
			expectedName: "EcommerceBackend",
		},
		{
			inputPath:    `C:\Users\John\Desktop\AI_Research_Bot`,
			expectedID:   "proj-ai_research_bot",
			expectedName: "AI_Research_Bot",
		},
		{
			inputPath:    `e:/WorkSpace/FintechApp`,
			expectedID:   "proj-fintechapp",
			expectedName: "FintechApp",
		},
		{
			inputPath:    `e:\GoogleDrive\WorkSpace\Code\ProjectGolang\GoLangDev\TokenMonitor`,
			expectedID:   "proj-tokenmonitor",
			expectedName: "TokenMonitor (GoLangDev)",
		},
		{
			inputPath:    `e:\GoogleDrive\WorkSpace\Code\ProjectGolang\GoLangDev\WebDownloadSKT24\saoke-downloader\.agents\explorer_survey_ui\survey_report.md`,
			expectedID:   "proj-webdownloadskt24",
			expectedName: "WebDownloadSKT24 (UI/UX Portal)",
		},
		{
			inputPath:    `e:\GoogleDrive\WorkSpace\Code\ProjectGolang\GoLangDev\WebDownloadSKT24\saoke-downloader\.agents\worker_m0_baseline\main.go`,
			expectedID:   "proj-webdownloadskt24",
			expectedName: "WebDownloadSKT24 (UI/UX Portal)",
		},
		{
			inputPath:    `e:\GoogleDrive\WorkSpace\Code\ProjectGolang\GoLangDev\WebDownloadSKT24\ui-ux-pro-max-skill-main`,
			expectedID:   "proj-webdownloadskt24",
			expectedName: "WebDownloadSKT24 (UI/UX Portal)",
		},
		{
			inputPath:    `e:\GoogleDrive\WorkSpace\Code\ProjectGolang\GoLangDev\WebDownloadSKT24\saoke-downloader`,
			expectedID:   "proj-webdownloadskt24",
			expectedName: "WebDownloadSKT24 (UI/UX Portal)",
		},
	}

	for _, tc := range testCases {
		id, name := storage.ExtractProjectFromPathString(tc.inputPath)
		if id != tc.expectedID {
			t.Errorf("Path %s: expected ID %q, got %q", tc.inputPath, tc.expectedID, id)
		}
		if name != tc.expectedName {
			t.Errorf("Path %s: expected Name %q, got %q", tc.inputPath, tc.expectedName, name)
		}
	}
}

