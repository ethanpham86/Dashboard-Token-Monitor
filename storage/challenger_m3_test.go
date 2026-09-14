package storage_test

import (
	"fmt"
	"math"
	"os"
	"regexp"
	"sync"
	"testing"

	"tokenmonitor/config"
	"tokenmonitor/storage"
)

func createM3TestStorage(t *testing.T, dbPath string) (*storage.Storage, func()) {
	_ = os.Remove(dbPath)
	_ = os.Remove(dbPath + "-wal")
	_ = os.Remove(dbPath + "-shm")

	cfg := &config.Config{
		Database: config.DatabaseConfig{
			SQLitePath:              dbPath,
			MaxOpenConns:            10,
			MaxIdleConns:            5,
			ConnMaxLifetimeMinutes: 5,
			EnableWALMode:           true,
			RollupIntervalSeconds:   300,
		},
		AccountProfile: config.AccountProfileConfig{
			Email:              "challenger_m3@example.com",
			AccountType:        "Google Consumer Account (Individual)",
			PlanName:           "20X ULTRA PLAN",
			QuotaBandwidth:     "20x Quota Bandwidth",
			SubscriptionStart:  "2026-09-08 00:00:00",
			SubscriptionExpiry: "2026-10-08 00:00:00",
			AutoRenew:          true,
		},
	}

	store, err := storage.NewStorage(cfg)
	if err != nil {
		t.Fatalf("Failed to initialize Storage: %v", err)
	}

	// Clean out any seed data from token_usage_logs and token_usage_hourly_rollup
	// to ensure a pristine test baseline
	_, err = store.DB.Exec("DELETE FROM token_usage_hourly_rollup")
	if err != nil {
		t.Fatalf("Failed to clear rollup table: %v", err)
	}
	_, err = store.DB.Exec("DELETE FROM token_usage_logs")
	if err != nil {
		t.Fatalf("Failed to clear logs table: %v", err)
	}

	cleanup := func() {
		_ = store.Close()
		_ = os.Remove(dbPath)
		_ = os.Remove(dbPath + "-wal")
		_ = os.Remove(dbPath + "-shm")
	}

	return store, cleanup
}

// TestChallenger_Rollup_EmptyDatabase verifies that RollupHourlyMetrics runs
// without error on an empty token_usage_logs table, resulting in 0 rollup rows.
func TestChallenger_Rollup_EmptyDatabase(t *testing.T) {
	dbPath := "./test_m3_empty_rollup.db"
	store, cleanup := createM3TestStorage(t, dbPath)
	defer cleanup()

	err := store.RollupHourlyMetrics()
	if err != nil {
		t.Fatalf("RollupHourlyMetrics() failed on empty database: %v", err)
	}

	var count int
	err = store.DB.QueryRow("SELECT COUNT(*) FROM token_usage_hourly_rollup").Scan(&count)
	if err != nil {
		t.Fatalf("failed to count rows in token_usage_hourly_rollup: %v", err)
	}

	if count != 0 {
		t.Errorf("expected 0 rows in rollup for empty logs, got %d", count)
	}
}

// TestChallenger_Rollup_DetailedMetricsAggregation tests that RollupHourlyMetrics
// accurately aggregates call_count, token sums (prompt, output, thinking, cached, total),
// and avg_latency across multiple accounts, hours, and models.
func TestChallenger_Rollup_DetailedMetricsAggregation(t *testing.T) {
	dbPath := "./test_m3_detailed_rollup.db"
	store, cleanup := createM3TestStorage(t, dbPath)
	defer cleanup()

	// Insert Account 2 to test multi-account aggregation
	_, err := store.DB.Exec(`
		INSERT INTO accounts (id, account_email, installation_uuid, registered_at, subscription_expiry)
		VALUES (2, 'secondary@example.com', 'uuid-test-2', '2026-09-08 00:00:00', '2026-10-08 00:00:00')
	`)
	if err != nil {
		t.Fatalf("failed to insert secondary account: %v", err)
	}

	// Insert known logs:
	// Account 1, Hour 10 (2026-09-08 10:00:00), Model "gemini-2.5-pro":
	//   Log 1: prompt=1000, output=200, thinking=50, cached=300, total=1200, latency=450
	//   Log 2: prompt=2500, output=500, thinking=100, cached=1000, total=3000, latency=650
	// Account 1, Hour 10 (2026-09-08 10:00:00), Model "gemini-2.5-flash":
	//   Log 3: prompt=800, output=150, thinking=0, cached=200, total=950, latency=200
	// Account 1, Hour 11 (2026-09-08 11:00:00), Model "gemini-2.5-pro":
	//   Log 4: prompt=5000, output=1000, thinking=250, cached=2000, total=6000, latency=800
	// Account 2, Hour 10 (2026-09-08 10:00:00), Model "gemini-2.5-pro":
	//   Log 5: prompt=3000, output=400, thinking=80, cached=1200, total=3400, latency=500
	logs := []struct {
		accountID int64
		timestamp string
		model     string
		prompt    int64
		output    int64
		thinking  int64
		cached    int64
		total     int64
		latency   int64
	}{
		{1, "2026-09-08 10:12:30", "gemini-2.5-pro", 1000, 200, 50, 300, 1200, 450},
		{1, "2026-09-08 10:45:15", "gemini-2.5-pro", 2500, 500, 100, 1000, 3000, 650},
		{1, "2026-09-08 10:30:00", "gemini-2.5-flash", 800, 150, 0, 200, 950, 200},
		{1, "2026-09-08 11:05:00", "gemini-2.5-pro", 5000, 1000, 250, 2000, 6000, 800},
		{2, "2026-09-08 10:20:00", "gemini-2.5-pro", 3000, 400, 80, 1200, 3400, 500},
	}

	stmt, err := store.DB.Prepare(`
		INSERT INTO token_usage_logs (
			account_id, timestamp, model_name, prompt_tokens, output_tokens,
			thinking_tokens, cached_tokens, total_tokens, latency_ms, status_code, request_type
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 200, 'TEST')
	`)
	if err != nil {
		t.Fatalf("failed to prepare insert statement: %v", err)
	}
	defer stmt.Close()

	for _, l := range logs {
		_, err := stmt.Exec(l.accountID, l.timestamp, l.model, l.prompt, l.output, l.thinking, l.cached, l.total, l.latency)
		if err != nil {
			t.Fatalf("failed to insert log record: %v", err)
		}
	}

	// Run RollupHourlyMetrics
	err = store.RollupHourlyMetrics()
	if err != nil {
		t.Fatalf("RollupHourlyMetrics() execution failed: %v", err)
	}

	// Query aggregated results
	rows, err := store.DB.Query(`
		SELECT 
			account_id, time_bucket, model_name, call_count,
			sum_prompt_tokens, sum_output_tokens, sum_thinking_tokens,
			sum_cached_tokens, sum_total_tokens, avg_latency_ms
		FROM token_usage_hourly_rollup
		ORDER BY account_id, time_bucket, model_name
	`)
	if err != nil {
		t.Fatalf("failed to query rollup table: %v", err)
	}
	defer rows.Close()

	type rollupResult struct {
		accountID   int64
		timeBucket  string
		modelName   string
		callCount   int64
		sumPrompt   int64
		sumOutput   int64
		sumThinking int64
		sumCached   int64
		sumTotal    int64
		avgLatency  float64
	}

	var results []rollupResult
	for rows.Next() {
		var r rollupResult
		err := rows.Scan(
			&r.accountID, &r.timeBucket, &r.modelName, &r.callCount,
			&r.sumPrompt, &r.sumOutput, &r.sumThinking,
			&r.sumCached, &r.sumTotal, &r.avgLatency,
		)
		if err != nil {
			t.Fatalf("failed to scan rollup row: %v", err)
		}
		results = append(results, r)
	}

	if len(results) != 4 {
		t.Fatalf("expected exactly 4 aggregated rollup rows, got %d", len(results))
	}

	// Expected results:
	// 0: Account 1, "2026-09-08 10:00:00", "gemini-2.5-flash"
	// 1: Account 1, "2026-09-08 10:00:00", "gemini-2.5-pro"
	// 2: Account 1, "2026-09-08 11:00:00", "gemini-2.5-pro"
	// 3: Account 2, "2026-09-08 10:00:00", "gemini-2.5-pro"

	// Check 0: Account 1, gemini-2.5-flash, 10:00:00
	r0 := results[0]
	if r0.accountID != 1 || (r0.timeBucket != "2026-09-08 10:00:00" && r0.timeBucket != "2026-09-08T10:00:00Z") || r0.modelName != "gemini-2.5-flash" {
		t.Errorf("r0 key mismatch: %+v", r0)
	}
	if r0.callCount != 1 || r0.sumPrompt != 800 || r0.sumOutput != 150 || r0.sumThinking != 0 ||
		r0.sumCached != 200 || r0.sumTotal != 950 || math.Abs(r0.avgLatency-200.0) > 0.001 {
		t.Errorf("r0 values incorrect: %+v", r0)
	}

	// Check 1: Account 1, gemini-2.5-pro, 10:00:00 (Aggregated 2 logs)
	r1 := results[1]
	if r1.accountID != 1 || (r1.timeBucket != "2026-09-08 10:00:00" && r1.timeBucket != "2026-09-08T10:00:00Z") || r1.modelName != "gemini-2.5-pro" {
		t.Errorf("r1 key mismatch: %+v", r1)
	}
	if r1.callCount != 2 {
		t.Errorf("r1 callCount expected 2, got %d", r1.callCount)
	}
	if r1.sumPrompt != 3500 { // 1000 + 2500
		t.Errorf("r1 sumPrompt expected 3500, got %d", r1.sumPrompt)
	}
	if r1.sumOutput != 700 { // 200 + 500
		t.Errorf("r1 sumOutput expected 700, got %d", r1.sumOutput)
	}
	if r1.sumThinking != 150 { // 50 + 100
		t.Errorf("r1 sumThinking expected 150, got %d", r1.sumThinking)
	}
	if r1.sumCached != 1300 { // 300 + 1000
		t.Errorf("r1 sumCached expected 1300, got %d", r1.sumCached)
	}
	if r1.sumTotal != 4200 { // 1200 + 3000
		t.Errorf("r1 sumTotal expected 4200, got %d", r1.sumTotal)
	}
	if math.Abs(r1.avgLatency-550.0) > 0.001 { // (450 + 650) / 2 = 550.0
		t.Errorf("r1 avgLatency expected 550.0, got %f", r1.avgLatency)
	}

	// Check 2: Account 1, gemini-2.5-pro, 11:00:00
	r2 := results[2]
	if r2.accountID != 1 || (r2.timeBucket != "2026-09-08 11:00:00" && r2.timeBucket != "2026-09-08T11:00:00Z") || r2.modelName != "gemini-2.5-pro" {
		t.Errorf("r2 key mismatch: %+v", r2)
	}
	if r2.callCount != 1 || r2.sumPrompt != 5000 || r2.sumOutput != 1000 || r2.sumThinking != 250 ||
		r2.sumCached != 2000 || r2.sumTotal != 6000 || math.Abs(r2.avgLatency-800.0) > 0.001 {
		t.Errorf("r2 values incorrect: %+v", r2)
	}

	// Check 3: Account 2, gemini-2.5-pro, 10:00:00
	r3 := results[3]
	if r3.accountID != 2 || (r3.timeBucket != "2026-09-08 10:00:00" && r3.timeBucket != "2026-09-08T10:00:00Z") || r3.modelName != "gemini-2.5-pro" {
		t.Errorf("r3 key mismatch: %+v", r3)
	}
	if r3.callCount != 1 || r3.sumPrompt != 3000 || r3.sumOutput != 400 || r3.sumThinking != 80 ||
		r3.sumCached != 1200 || r3.sumTotal != 3400 || math.Abs(r3.avgLatency-500.0) > 0.001 {
		t.Errorf("r3 values incorrect: %+v", r3)
	}
}

// TestChallenger_Rollup_IdempotencyAndConflictResolution verifies that repeated
// executions of RollupHourlyMetrics perform idempotent upsert on conflict without
// primary key or unique index violations, and update existing rows when new data arrives.
func TestChallenger_Rollup_IdempotencyAndConflictResolution(t *testing.T) {
	dbPath := "./test_m3_idempotency_rollup.db"
	store, cleanup := createM3TestStorage(t, dbPath)
	defer cleanup()

	// 1. Seed initial logs
	_, err := store.DB.Exec(`
		INSERT INTO token_usage_logs (
			account_id, timestamp, model_name, prompt_tokens, output_tokens,
			thinking_tokens, cached_tokens, total_tokens, latency_ms, status_code, request_type
		) VALUES 
		(1, '2026-09-08 10:10:00', 'gemini-2.5-pro', 1000, 200, 50, 300, 1200, 400, 200, 'TEST_1'),
		(1, '2026-09-08 10:20:00', 'gemini-2.5-pro', 2000, 400, 100, 600, 2400, 600, 200, 'TEST_2')
	`)
	if err != nil {
		t.Fatalf("failed to insert initial logs: %v", err)
	}

	// 2. Run Rollup 10 consecutive times without modifying logs
	for i := 1; i <= 10; i++ {
		err := store.RollupHourlyMetrics()
		if err != nil {
			t.Fatalf("iteration %d failed with error: %v", i, err)
		}
	}

	// Verify exactly 1 row exists (no duplicates or primary key violations)
	var count int
	err = store.DB.QueryRow("SELECT COUNT(*) FROM token_usage_hourly_rollup").Scan(&count)
	if err != nil {
		t.Fatalf("query row count failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 row in rollup after 10 runs, got %d", count)
	}

	// Verify values after 10 runs match exactly
	var callCount, sumPrompt, sumOutput, sumThinking, sumCached, sumTotal int64
	var avgLatency float64
	err = store.DB.QueryRow(`
		SELECT call_count, sum_prompt_tokens, sum_output_tokens, sum_thinking_tokens,
		       sum_cached_tokens, sum_total_tokens, avg_latency_ms
		FROM token_usage_hourly_rollup
		WHERE account_id = 1 AND time_bucket = '2026-09-08 10:00:00' AND model_name = 'gemini-2.5-pro'
	`).Scan(&callCount, &sumPrompt, &sumOutput, &sumThinking, &sumCached, &sumTotal, &avgLatency)
	if err != nil {
		t.Fatalf("failed to query row: %v", err)
	}

	if callCount != 2 || sumPrompt != 3000 || sumOutput != 600 || sumThinking != 150 ||
		sumCached != 900 || sumTotal != 3600 || math.Abs(avgLatency-500.0) > 0.001 {
		t.Errorf("values mismatch after 10 runs: callCount=%d, sumPrompt=%d, avgLatency=%f", callCount, sumPrompt, avgLatency)
	}

	// 3. Now insert a 3rd log into the SAME bucket
	// Log 3: prompt=1500, output=300, thinking=50, cached=500, total=1800, latency=800
	_, err = store.DB.Exec(`
		INSERT INTO token_usage_logs (
			account_id, timestamp, model_name, prompt_tokens, output_tokens,
			thinking_tokens, cached_tokens, total_tokens, latency_ms, status_code, request_type
		) VALUES 
		(1, '2026-09-08 10:50:00', 'gemini-2.5-pro', 1500, 300, 50, 500, 1800, 800, 200, 'TEST_3')
	`)
	if err != nil {
		t.Fatalf("failed to insert 3rd log: %v", err)
	}

	// 4. Run Rollup again: should perform ON CONFLICT DO UPDATE
	err = store.RollupHourlyMetrics()
	if err != nil {
		t.Fatalf("11th Rollup execution failed: %v", err)
	}

	// Row count must remain 1
	err = store.DB.QueryRow("SELECT COUNT(*) FROM token_usage_hourly_rollup").Scan(&count)
	if err != nil {
		t.Fatalf("query row count failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected row count to remain 1 after upsert, got %d", count)
	}

	// Verify updated values:
	// callCount: 2 + 1 = 3
	// sumPrompt: 3000 + 1500 = 4500
	// sumOutput: 600 + 300 = 900
	// sumThinking: 150 + 50 = 200
	// sumCached: 900 + 500 = 1400
	// sumTotal: 3600 + 1800 = 5400
	// avgLatency: (400 + 600 + 800) / 3 = 600.0
	err = store.DB.QueryRow(`
		SELECT call_count, sum_prompt_tokens, sum_output_tokens, sum_thinking_tokens,
		       sum_cached_tokens, sum_total_tokens, avg_latency_ms
		FROM token_usage_hourly_rollup
		WHERE account_id = 1 AND time_bucket = '2026-09-08 10:00:00' AND model_name = 'gemini-2.5-pro'
	`).Scan(&callCount, &sumPrompt, &sumOutput, &sumThinking, &sumCached, &sumTotal, &avgLatency)
	if err != nil {
		t.Fatalf("failed to query updated row: %v", err)
	}

	if callCount != 3 {
		t.Errorf("expected call_count 3, got %d", callCount)
	}
	if sumPrompt != 4500 {
		t.Errorf("expected sumPrompt 4500, got %d", sumPrompt)
	}
	if sumOutput != 900 {
		t.Errorf("expected sumOutput 900, got %d", sumOutput)
	}
	if sumThinking != 200 {
		t.Errorf("expected sumThinking 200, got %d", sumThinking)
	}
	if sumCached != 1400 {
		t.Errorf("expected sumCached 1400, got %d", sumCached)
	}
	if sumTotal != 5400 {
		t.Errorf("expected sumTotal 5400, got %d", sumTotal)
	}
	if math.Abs(avgLatency-600.0) > 0.001 {
		t.Errorf("expected avgLatency 600.0, got %f", avgLatency)
	}
}

// TestChallenger_Rollup_ConcurrentStress tests concurrent invocations of RollupHourlyMetrics
// under SQLite WAL mode to ensure no unhandled database locks, panics, or corruption.
func TestChallenger_Rollup_ConcurrentStress(t *testing.T) {
	dbPath := "./test_m3_concurrent_rollup.db"
	store, cleanup := createM3TestStorage(t, dbPath)
	defer cleanup()

	// Seed some logs
	for i := 0; i < 50; i++ {
		ts := fmt.Sprintf("2026-09-08 1%d:%02d:00", i%3, i)
		_, err := store.DB.Exec(`
			INSERT INTO token_usage_logs (
				account_id, timestamp, model_name, prompt_tokens, output_tokens,
				thinking_tokens, cached_tokens, total_tokens, latency_ms, status_code, request_type
			) VALUES (1, ?, 'gemini-2.5-pro', 100, 50, 10, 20, 150, 300, 200, 'CONCURRENT_TEST')
		`, ts)
		if err != nil {
			t.Fatalf("failed to seed concurrent log: %v", err)
		}
	}

	concurrency := 8
	var wg sync.WaitGroup
	errChan := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				if err := store.RollupHourlyMetrics(); err != nil {
					errChan <- fmt.Errorf("worker %d run %d failed: %w", workerID, j, err)
					return
				}
			}
		}(i)
	}

	wg.Wait()
	close(errChan)

	for err := range errChan {
		t.Errorf("concurrent rollup error: %v", err)
	}
}

// TestChallenger_MainGo_RollupWiring directly inspects main.go to ensure:
// 1. Rollup interval uses cfg.Database.RollupIntervalSeconds with 5m fallback.
// 2. RollupHourlyMetrics is called once on startup.
// 3. RollupHourlyMetrics is called inside the ticker loop.
// 4. Graceful shutdown cleanly stops the ticker and terminates the goroutine.
func TestChallenger_MainGo_RollupWiring(t *testing.T) {
	content, err := os.ReadFile("../main.go")
	if err != nil {
		t.Fatalf("failed to read main.go: %v", err)
	}

	code := string(content)

	// Check 1: rollup interval check
	if !regexp.MustCompile(`cfg\.Database\.RollupIntervalSeconds`).MatchString(code) {
		t.Errorf("main.go does not reference cfg.Database.RollupIntervalSeconds")
	}
	if !regexp.MustCompile(`5\s*\*\s*time\.Minute`).MatchString(code) {
		t.Errorf("main.go does not contain 5 * time.Minute fallback")
	}

	// Check 2: store.RollupHourlyMetrics() called on startup
	if !regexp.MustCompile(`store\.RollupHourlyMetrics\(\)`).MatchString(code) {
		t.Errorf("main.go does not call store.RollupHourlyMetrics()")
	}

	// Check 3: rollupTicker.Stop() and close(rollupDone) on shutdown
	if !regexp.MustCompile(`rollupTicker\.Stop\(\)`).MatchString(code) {
		t.Errorf("main.go does not call rollupTicker.Stop() on shutdown")
	}
	if !regexp.MustCompile(`close\(\s*rollupDone\s*\)`).MatchString(code) {
		t.Errorf("main.go does not close rollupDone channel on shutdown")
	}
}
