package storage_test

import (
	"os"
	"path/filepath"
	"testing"

	"tokenmonitor/config"
	"tokenmonitor/storage"
)

func setupTestStorage(t *testing.T) (*storage.Storage, func()) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "tokenmonitor_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := &config.Config{
		Database: config.DatabaseConfig{
			SQLitePath:    dbPath,
			MaxOpenConns:  5,
			MaxIdleConns:  2,
			EnableWALMode: true,
		},
		AccountProfile: config.AccountProfileConfig{
			Email:              "reviewer_m3@example.com",
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
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to create storage: %v", err)
	}

	cleanup := func() {
		store.Close()
		os.RemoveAll(tmpDir)
	}

	return store, cleanup
}

// TestReviewerM3_RollupHourlyMetrics_EmptyTable verifies Rollup on an empty logs table.
func TestReviewerM3_RollupHourlyMetrics_EmptyTable(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()

	// Clear any seeded logs
	_, err := store.DB.Exec("DELETE FROM token_usage_logs")
	if err != nil {
		t.Fatalf("Failed to clear logs: %v", err)
	}
	_, err = store.DB.Exec("DELETE FROM token_usage_hourly_rollup")
	if err != nil {
		t.Fatalf("Failed to clear rollup: %v", err)
	}

	err = store.RollupHourlyMetrics()
	if err != nil {
		t.Fatalf("RollupHourlyMetrics failed on empty table: %v", err)
	}

	var count int
	err = store.DB.QueryRow("SELECT COUNT(*) FROM token_usage_hourly_rollup").Scan(&count)
	if err != nil {
		t.Fatalf("Query count failed: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0 rollup rows on empty logs table, got %d", count)
	}
}

// TestReviewerM3_RollupHourlyMetrics_AggregationMath verifies calculation of SUMs, COUNT, and AVG.
func TestReviewerM3_RollupHourlyMetrics_AggregationMath(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()

	// Clear logs & rollup
	_, _ = store.DB.Exec("DELETE FROM token_usage_logs")
	_, _ = store.DB.Exec("DELETE FROM token_usage_hourly_rollup")

	stmt, err := store.DB.Prepare(`
		INSERT INTO token_usage_logs 
		(account_id, timestamp, model_name, prompt_tokens, output_tokens, thinking_tokens, cached_tokens, total_tokens, latency_ms, status_code, request_type)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		t.Fatalf("Prepare failed: %v", err)
	}
	defer stmt.Close()

	records := []struct {
		accID    int64
		ts       string
		model    string
		prompt   int64
		output   int64
		thinking int64
		cached   int64
		total    int64
		latency  int
	}{
		{1, "2026-09-08 14:10:00", "gemini-2.5-pro", 100, 50, 20, 10, 150, 200},
		{1, "2026-09-08 14:35:00", "gemini-2.5-pro", 200, 100, 40, 30, 300, 400},
		{1, "2026-09-08 14:50:00", "gemini-2.5-flash", 50, 20, 0, 5, 70, 100},
		{1, "2026-09-08 15:10:00", "gemini-2.5-pro", 300, 150, 60, 50, 450, 300},
	}

	for _, r := range records {
		_, err := stmt.Exec(r.accID, r.ts, r.model, r.prompt, r.output, r.thinking, r.cached, r.total, r.latency, 200, "INTERACTIVE")
		if err != nil {
			t.Fatalf("Insert failed: %v", err)
		}
	}

	// Execute Rollup
	if err := store.RollupHourlyMetrics(); err != nil {
		t.Fatalf("RollupHourlyMetrics failed: %v", err)
	}

	var totalRollups int
	_ = store.DB.QueryRow("SELECT COUNT(*) FROM token_usage_hourly_rollup").Scan(&totalRollups)
	if totalRollups != 3 {
		t.Fatalf("Expected 3 rollup rows, got %d", totalRollups)
	}

	var count, sumP, sumO, sumTh, sumCa, sumTot int64
	var avgLat float64
	err = store.DB.QueryRow(`
		SELECT call_count, sum_prompt_tokens, sum_output_tokens, sum_thinking_tokens, sum_cached_tokens, sum_total_tokens, avg_latency_ms
		FROM token_usage_hourly_rollup
		WHERE account_id = 1 AND time_bucket = '2026-09-08 14:00:00' AND model_name = 'gemini-2.5-pro'
	`).Scan(&count, &sumP, &sumO, &sumTh, &sumCa, &sumTot, &avgLat)
	if err != nil {
		t.Fatalf("Failed to query rollup row: %v", err)
	}

	if count != 2 {
		t.Errorf("Expected call_count=2, got %d", count)
	}
	if sumP != 300 {
		t.Errorf("Expected sum_prompt=300, got %d", sumP)
	}
	if sumO != 150 {
		t.Errorf("Expected sum_output=150, got %d", sumO)
	}
	if sumTh != 60 {
		t.Errorf("Expected sum_thinking=60, got %d", sumTh)
	}
	if sumCa != 40 {
		t.Errorf("Expected sum_cached=40, got %d", sumCa)
	}
	if sumTot != 450 {
		t.Errorf("Expected sum_total=450, got %d", sumTot)
	}
	if avgLat != 300.0 {
		t.Errorf("Expected avg_latency=300.0, got %f", avgLat)
	}
}

// TestReviewerM3_RollupHourlyMetrics_IdempotencyAndUpsert tests running Rollup twice and after adding more logs.
func TestReviewerM3_RollupHourlyMetrics_IdempotencyAndUpsert(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()

	_, _ = store.DB.Exec("DELETE FROM token_usage_logs")
	_, _ = store.DB.Exec("DELETE FROM token_usage_hourly_rollup")

	// Insert 1 log
	_, err := store.DB.Exec(`
		INSERT INTO token_usage_logs 
		(account_id, timestamp, model_name, prompt_tokens, output_tokens, thinking_tokens, cached_tokens, total_tokens, latency_ms, status_code, request_type)
		VALUES (1, '2026-09-08 10:15:00', 'gemini-2.5-pro', 100, 50, 10, 0, 150, 250, 200, 'INTERACTIVE')
	`)
	if err != nil {
		t.Fatalf("Insert log failed: %v", err)
	}

	// 1. First rollup
	if err := store.RollupHourlyMetrics(); err != nil {
		t.Fatalf("First Rollup failed: %v", err)
	}

	var count1, totalTokens1 int64
	_ = store.DB.QueryRow("SELECT call_count, sum_total_tokens FROM token_usage_hourly_rollup WHERE time_bucket = '2026-09-08 10:00:00'").Scan(&count1, &totalTokens1)
	if count1 != 1 || totalTokens1 != 150 {
		t.Fatalf("Expected count=1, total=150; got count=%d, total=%d", count1, totalTokens1)
	}

	// 2. Second rollup with NO new data -> Idempotency check: row count and values must NOT change
	if err := store.RollupHourlyMetrics(); err != nil {
		t.Fatalf("Second Rollup failed: %v", err)
	}

	var count2, totalTokens2 int64
	var rowCount int
	_ = store.DB.QueryRow("SELECT COUNT(*) FROM token_usage_hourly_rollup").Scan(&rowCount)
	if rowCount != 1 {
		t.Errorf("Idempotency violation: expected exactly 1 rollup row, got %d", rowCount)
	}

	_ = store.DB.QueryRow("SELECT call_count, sum_total_tokens FROM token_usage_hourly_rollup WHERE time_bucket = '2026-09-08 10:00:00'").Scan(&count2, &totalTokens2)
	if count2 != 1 || totalTokens2 != 150 {
		t.Errorf("Idempotency violation: values changed on re-run: count=%d, total=%d", count2, totalTokens2)
	}

	// 3. Insert additional log in same bucket: prompt=200, output=100, latency=150
	_, err = store.DB.Exec(`
		INSERT INTO token_usage_logs 
		(account_id, timestamp, model_name, prompt_tokens, output_tokens, thinking_tokens, cached_tokens, total_tokens, latency_ms, status_code, request_type)
		VALUES (1, '2026-09-08 10:45:00', 'gemini-2.5-pro', 200, 100, 20, 0, 300, 150, 200, 'INTERACTIVE')
	`)
	if err != nil {
		t.Fatalf("Insert 2nd log failed: %v", err)
	}

	// 4. Third rollup -> ON CONFLICT DO UPDATE SET must update call_count=2, sum_total_tokens=450, avg_latency=200.0
	if err := store.RollupHourlyMetrics(); err != nil {
		t.Fatalf("Third Rollup failed: %v", err)
	}

	_ = store.DB.QueryRow("SELECT COUNT(*) FROM token_usage_hourly_rollup").Scan(&rowCount)
	if rowCount != 1 {
		t.Errorf("UPSERT violation: expected still 1 row, got %d", rowCount)
	}

	var count3, totalTokens3 int64
	var avgLat3 float64
	_ = store.DB.QueryRow(`
		SELECT call_count, sum_total_tokens, avg_latency_ms 
		FROM token_usage_hourly_rollup 
		WHERE time_bucket = '2026-09-08 10:00:00'
	`).Scan(&count3, &totalTokens3, &avgLat3)

	if count3 != 2 {
		t.Errorf("UPSERT expected call_count=2, got %d", count3)
	}
	if totalTokens3 != 450 {
		t.Errorf("UPSERT expected sum_total_tokens=450, got %d", totalTokens3)
	}
	if avgLat3 != 200.0 {
		t.Errorf("UPSERT expected avg_latency_ms=200.0, got %f", avgLat3)
	}
}

// TestReviewerM3_RollupHourlyMetrics_MultiAccountIsolation tests that different accounts are rolled up separately.
func TestReviewerM3_RollupHourlyMetrics_MultiAccountIsolation(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()

	// Insert second account
	_, err := store.DB.Exec(`
		INSERT INTO accounts (id, account_email, account_type, plan_name, quota_bandwidth, installation_uuid, registered_at, subscription_expiry)
		VALUES (2, 'account2@example.com', 'Consumer', 'Pro Plan', '1x Quota', 'uuid-2', '2026-09-01 00:00:00', '2026-10-01 00:00:00')
	`)
	if err != nil {
		t.Fatalf("Failed to insert account 2: %v", err)
	}

	// Insert log for Account 1 and Account 2 at same hour and model
	ts := "2026-09-08 12:10:00"
	_, _ = store.DB.Exec(`
		INSERT INTO token_usage_logs (account_id, timestamp, model_name, prompt_tokens, output_tokens, total_tokens, latency_ms)
		VALUES (1, ?, 'gemini-2.5-pro', 100, 50, 150, 200),
		       (2, ?, 'gemini-2.5-pro', 500, 250, 750, 600)
	`, ts, ts)

	if err := store.RollupHourlyMetrics(); err != nil {
		t.Fatalf("Rollup failed: %v", err)
	}

	var acc1Tot, acc2Tot int64
	err1 := store.DB.QueryRow("SELECT sum_total_tokens FROM token_usage_hourly_rollup WHERE account_id = 1").Scan(&acc1Tot)
	err2 := store.DB.QueryRow("SELECT sum_total_tokens FROM token_usage_hourly_rollup WHERE account_id = 2").Scan(&acc2Tot)

	if err1 != nil || err2 != nil {
		t.Fatalf("Failed to query rollup for accounts: %v, %v", err1, err2)
	}
	if acc1Tot != 150 {
		t.Errorf("Expected acc 1 total=150, got %d", acc1Tot)
	}
	if acc2Tot != 750 {
		t.Errorf("Expected acc 2 total=750, got %d", acc2Tot)
	}
}
