package storage_test

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"tokenmonitor/collector"
	"tokenmonitor/config"
	"tokenmonitor/storage"
)

// TestChallengerM4_Storage_CorruptDatabaseFile tests that NewStorage returns an error
// and does not panic when presented with a corrupted, non-SQLite database file.
func TestChallengerM4_Storage_CorruptDatabaseFile(t *testing.T) {
	tmpDir := t.TempDir()
	corruptFile := filepath.Join(tmpDir, "corrupted_token_monitor.db")

	// Write garbage data that does NOT have the SQLite header
	garbage := []byte("INVALID_SQLITE_HEADER_CORRUPTED_BYTES_1234567890\n\x00\xff\xfe\xfd")
	if err := os.WriteFile(corruptFile, garbage, 0644); err != nil {
		t.Fatalf("Failed to write corrupt test file: %v", err)
	}

	cfg := &config.Config{
		Database: config.DatabaseConfig{
			SQLitePath:    corruptFile,
			EnableWALMode: true,
		},
		AccountProfile: config.AccountProfileConfig{
			Email: "test@example.com",
		},
	}

	store, err := storage.NewStorage(cfg)
	if err == nil {
		if store != nil {
			store.Close()
		}
		t.Fatal("Expected NewStorage to fail on corrupted database file, but returned nil error")
	}

	t.Logf("Correctly rejected corrupt database file with error: %v", err)
}

// TestChallengerM4_Storage_ClosedDatabase_SafeErrors tests that all repository query methods
// return descriptive errors and never panic when called on a closed database connection.
func TestChallengerM4_Storage_ClosedDatabase_SafeErrors(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "closed_test.db")

	cfg := &config.Config{
		Database: config.DatabaseConfig{
			SQLitePath: dbPath,
		},
		AccountProfile: config.AccountProfileConfig{
			Email: "closed@example.com",
		},
	}

	store, err := storage.NewStorage(cfg)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	// Close database
	if err := store.Close(); err != nil {
		t.Fatalf("Failed to close storage: %v", err)
	}

	// 1. GetAccountProfile
	if _, err := store.GetAccountProfile(); err == nil {
		t.Error("Expected error from GetAccountProfile on closed DB, got nil")
	}

	// 2. GetSummaryMetrics
	if _, err := store.GetSummaryMetrics(); err == nil {
		t.Error("Expected error from GetSummaryMetrics on closed DB, got nil")
	}

	// 3. GetSummaryMetricsByRange
	if _, err := store.GetSummaryMetricsByRange("24h"); err == nil {
		t.Error("Expected error from GetSummaryMetricsByRange on closed DB, got nil")
	}

	// 4. GetTimeSeriesDataByRange
	if _, err := store.GetTimeSeriesDataByRange("24h"); err == nil {
		t.Error("Expected error from GetTimeSeriesDataByRange on closed DB, got nil")
	}

	// 5. GetDailySummariesByRange
	if _, err := store.GetDailySummariesByRange("30d"); err == nil {
		t.Error("Expected error from GetDailySummariesByRange on closed DB, got nil")
	}

	// 6. GetModelDistribution
	if _, err := store.GetModelDistribution("all"); err == nil {
		t.Error("Expected error from GetModelDistribution on closed DB, got nil")
	}

	// 7. GetModelTimeSeries
	if _, err := store.GetModelTimeSeries("30d", "all"); err == nil {
		t.Error("Expected error from GetModelTimeSeries on closed DB, got nil")
	}

	// 8. RollupHourlyMetrics
	if err := store.RollupHourlyMetrics(); err == nil {
		t.Error("Expected error from RollupHourlyMetrics on closed DB, got nil")
	}

	// 9. InsertUsageBatch
	ev := &collector.TokenUsageEvent{AccountID: 1, Timestamp: time.Now(), ModelName: "gemini"}
	if err := store.InsertUsageBatch([]*collector.TokenUsageEvent{ev}); err == nil {
		t.Error("Expected error from InsertUsageBatch on closed DB, got nil")
	}
}

// TestChallengerM4_Storage_ForeignKeyConstraintEnforcement tests that PRAGMA foreign_keys = ON
// prevents inserting records with non-existent foreign keys.
func TestChallengerM4_Storage_ForeignKeyConstraintEnforcement(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "fk_test.db")

	cfg := &config.Config{
		Database: config.DatabaseConfig{
			SQLitePath: dbPath,
		},
		AccountProfile: config.AccountProfileConfig{
			Email: "fk_test@example.com",
		},
	}

	store, err := storage.NewStorage(cfg)
	if err != nil {
		t.Fatalf("Failed to initialize storage: %v", err)
	}
	defer store.Close()

	// Attempt to insert an auth_session referencing a non-existent account ID (e.g. 999999)
	now := time.Now()
	_, err = store.DB.Exec(`
		INSERT INTO auth_sessions (account_id, token_status, token_issued_at, token_expires_at)
		VALUES (999999, 'VALID', ?, ?)
	`, now, now.Add(1*time.Hour))

	if err == nil {
		t.Fatal("Expected foreign key violation error when inserting auth_session with invalid account_id 999999, got nil")
	}
	t.Logf("Correctly enforced foreign key constraint: %v", err)

	// Attempt to insert a token_usage_logs referencing non-existent account ID
	_, err = store.DB.Exec(`
		INSERT INTO token_usage_logs (account_id, timestamp, model_name, total_tokens)
		VALUES (999999, ?, 'gemini-test', 100)
	`, now)

	if err == nil {
		t.Fatal("Expected foreign key violation error when inserting token_usage_logs with invalid account_id 999999, got nil")
	}
	t.Logf("Correctly enforced foreign key constraint on logs: %v", err)
}

// TestChallengerM4_Storage_HighConcurrencyStress stresses SQLite under concurrent reads,
// writes, and rollup aggregations to ensure WAL mode and busy_timeout handle concurrency smoothly.
func TestChallengerM4_Storage_HighConcurrencyStress(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "stress_concurrency.db")

	cfg := &config.Config{
		Database: config.DatabaseConfig{
			SQLitePath:             dbPath,
			MaxOpenConns:           25,
			MaxIdleConns:           10,
			ConnMaxLifetimeMinutes: 10,
			EnableWALMode:          true,
		},
		AccountProfile: config.AccountProfileConfig{
			Email: "stress@example.com",
		},
	}

	store, err := storage.NewStorage(cfg)
	if err != nil {
		t.Fatalf("Failed to initialize storage: %v", err)
	}
	defer store.Close()

	var wg sync.WaitGroup
	errChan := make(chan error, 100)

	// 5 writer goroutines inserting batches
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for b := 0; b < 10; b++ {
				events := make([]*collector.TokenUsageEvent, 10)
				for j := 0; j < 10; j++ {
					events[j] = &collector.TokenUsageEvent{
						AccountID:      1,
						Timestamp:      time.Now().Add(-time.Duration(b*5) * time.Minute),
						ModelName:      fmt.Sprintf("gemini-stress-%d", (workerID+j)%3),
						PromptTokens:   int64(1000 + j*10),
						OutputTokens:   int64(200 + j*5),
						ThinkingTokens: int64(50 + j*2),
						CachedTokens:   int64(500 + j*10),
						TotalTokens:    int64(1200 + j*15),
						LatencyMs:      int64(800 + j*20),
						StatusCode:     200,
						RequestType:    "CONCURRENT_TEST",
					}
				}
				if err := store.InsertUsageBatch(events); err != nil {
					errChan <- fmt.Errorf("writer %d batch %d failed: %w", workerID, b, err)
					return
				}
				time.Sleep(2 * time.Millisecond)
			}
		}(i)
	}

	// 5 reader goroutines querying metrics concurrently
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for r := 0; r < 10; r++ {
				if _, err := store.GetSummaryMetricsByRange("all"); err != nil {
					errChan <- fmt.Errorf("reader %d summary failed: %w", workerID, err)
					return
				}
				if _, err := store.GetTimeSeriesDataByRange("24h"); err != nil {
					errChan <- fmt.Errorf("reader %d timeseries failed: %w", workerID, err)
					return
				}
				if _, err := store.GetModelDistribution("all"); err != nil {
					errChan <- fmt.Errorf("reader %d models failed: %w", workerID, err)
					return
				}
				time.Sleep(3 * time.Millisecond)
			}
		}(i)
	}

	// 2 rollup goroutines running aggregations concurrently
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for u := 0; u < 5; u++ {
				if err := store.RollupHourlyMetrics(); err != nil {
					errChan <- fmt.Errorf("rollup worker %d failed: %w", workerID, err)
					return
				}
				time.Sleep(5 * time.Millisecond)
			}
		}(i)
	}

	wg.Wait()
	close(errChan)

	for err := range errChan {
		t.Errorf("Concurrency stress failure: %v", err)
	}
}
