package storage_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tokenmonitor/config"
	"tokenmonitor/storage"
)

// safeTempDB creates a temporary SQLite file with safe cleanup on Windows
func safeTempDB(t *testing.T, prefix string) (string, func()) {
	t.Helper()
	dir, err := os.MkdirTemp("", prefix+"_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	dbPath := filepath.Join(dir, prefix+".db")
	cleanup := func() {
		// Small delay to allow Windows file handles to completely close
		time.Sleep(20 * time.Millisecond)
		_ = os.RemoveAll(dir)
	}
	return dbPath, cleanup
}

// TestChallengerRun4_Storage_Schema5TablesAnd7Indexes verifies all 5 tables and 7 indexes
// match docs/TokenMonitor_Database_ERD.md exactly on fresh initialization.
func TestChallengerRun4_Storage_Schema5TablesAnd7Indexes(t *testing.T) {
	dbPath, cleanup := safeTempDB(t, "challenger_r4_schema")
	defer cleanup()

	cfg := &config.Config{
		Database: config.DatabaseConfig{
			SQLitePath:    dbPath,
			MaxOpenConns:  2,
			MaxIdleConns:  1,
			EnableWALMode: true,
		},
		AccountProfile: config.AccountProfileConfig{
			Email: "challenger_r4@example.com",
		},
	}

	store, err := storage.NewStorage(cfg)
	if err != nil {
		t.Fatalf("NewStorage failed: %v", err)
	}
	defer store.Close()

	// 1. Verify 5 tables
	expectedTables := []string{
		"accounts",
		"auth_sessions",
		"token_usage_logs",
		"token_usage_hourly_rollup",
		"agent_fleet_telemetry",
	}

	for _, tbl := range expectedTables {
		var cnt int
		err := store.DB.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", tbl).Scan(&cnt)
		if err != nil {
			t.Fatalf("Failed to query table %s: %v", tbl, err)
		}
		if cnt != 1 {
			t.Errorf("CRITICAL: Expected table '%s' to exist (got count=%d)", tbl, cnt)
		}
	}

	// 2. Verify 7 strategic indexes
	expectedIndexes := []string{
		"idx_token_usage_timestamp",
		"idx_token_usage_account_model",
		"idx_hourly_bucket",
		"idx_token_dedup_chat",
		"idx_agent_fleet_started",
		"idx_agent_fleet_role",
		"idx_agent_fleet_subagent",
	}

	for _, idx := range expectedIndexes {
		var cnt int
		err := store.DB.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?", idx).Scan(&cnt)
		if err != nil {
			t.Fatalf("Failed to query index %s: %v", idx, err)
		}
		if cnt != 1 {
			t.Errorf("CRITICAL: Expected index '%s' to exist (got count=%d)", idx, cnt)
		}
	}

	// 3. Verify PRAGMAs on live connection
	ctx := context.Background()
	conn, err := store.DB.Conn(ctx)
	if err != nil {
		t.Fatalf("Failed to get connection: %v", err)
	}
	defer conn.Close()

	var fk int
	_ = conn.QueryRowContext(ctx, "PRAGMA foreign_keys;").Scan(&fk)
	if fk != 1 {
		t.Errorf("Expected PRAGMA foreign_keys=1, got %d", fk)
	}

	var jm string
	_ = conn.QueryRowContext(ctx, "PRAGMA journal_mode;").Scan(&jm)
	if strings.ToLower(jm) != "wal" {
		t.Errorf("Expected PRAGMA journal_mode=wal, got %s", jm)
	}

	var syncMode int
	_ = conn.QueryRowContext(ctx, "PRAGMA synchronous;").Scan(&syncMode)
	if syncMode != 1 {
		t.Errorf("Expected PRAGMA synchronous=1 (NORMAL), got %d", syncMode)
	}

	var cacheSize int
	_ = conn.QueryRowContext(ctx, "PRAGMA cache_size;").Scan(&cacheSize)
	if cacheSize != -64000 {
		t.Errorf("Expected PRAGMA cache_size=-64000, got %d", cacheSize)
	}

	var tempStore int
	_ = conn.QueryRowContext(ctx, "PRAGMA temp_store;").Scan(&tempStore)
	if tempStore != 2 {
		t.Errorf("Expected PRAGMA temp_store=2 (MEMORY), got %d", tempStore)
	}

	var busyTimeout int
	_ = conn.QueryRowContext(ctx, "PRAGMA busy_timeout;").Scan(&busyTimeout)
	if busyTimeout != 5000 {
		t.Errorf("Expected PRAGMA busy_timeout=5000, got %d", busyTimeout)
	}
}

// TestChallengerRun4_Storage_ActualLiveDBVerification inspects actual ./data/token_monitor.db
func TestChallengerRun4_Storage_ActualLiveDBVerification(t *testing.T) {
	liveDBPath := filepath.Join("..", "data", "token_monitor.db")
	if _, err := os.Stat(liveDBPath); err != nil {
		liveDBPath = filepath.Join("data", "token_monitor.db")
		if _, err := os.Stat(liveDBPath); err != nil {
			t.Skip("Live database ./data/token_monitor.db not found, skipping inspection")
		}
	}

	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=ro", filepath.ToSlash(liveDBPath)))
	if err != nil {
		t.Fatalf("Failed to open live database in mode=ro: %v", err)
	}
	db.SetMaxIdleConns(0)
	defer db.Close()

	// Verify all 5 tables exist in live DB
	tables := []string{
		"accounts",
		"auth_sessions",
		"token_usage_logs",
		"token_usage_hourly_rollup",
		"agent_fleet_telemetry",
	}

	for _, tbl := range tables {
		var cnt int
		err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", tbl).Scan(&cnt)
		if err != nil || cnt != 1 {
			t.Errorf("Live DB missing table: %s (err: %v, count: %d)", tbl, err, cnt)
		}

		var rowCount int
		_ = db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", tbl)).Scan(&rowCount)
		t.Logf("Live DB Table %s: %d rows", tbl, rowCount)
	}

	// Verify all 7 indexes exist in live DB
	indexes := []string{
		"idx_token_usage_timestamp",
		"idx_token_usage_account_model",
		"idx_hourly_bucket",
		"idx_token_dedup_chat",
		"idx_agent_fleet_started",
		"idx_agent_fleet_role",
		"idx_agent_fleet_subagent",
	}

	for _, idx := range indexes {
		var cnt int
		err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?", idx).Scan(&cnt)
		if err != nil || cnt != 1 {
			t.Errorf("Live DB missing index: %s (err: %v, count: %d)", idx, err, cnt)
		}
	}
}

// TestChallengerRun4_Storage_SchemaIntegrityAndAdversarialConstraints tests DB constraint enforcement
func TestChallengerRun4_Storage_SchemaIntegrityAndAdversarialConstraints(t *testing.T) {
	dbPath, cleanup := safeTempDB(t, "challenger_r4_constraints")
	defer cleanup()

	cfg := &config.Config{
		Database: config.DatabaseConfig{
			SQLitePath:    dbPath,
			MaxOpenConns:  1,
			MaxIdleConns:  1,
			EnableWALMode: true,
		},
		AccountProfile: config.AccountProfileConfig{
			Email: "constraints_tester@example.com",
		},
	}

	store, err := storage.NewStorage(cfg)
	if err != nil {
		t.Fatalf("NewStorage failed: %v", err)
	}
	defer store.Close()

	// 1. Test CHECK constraint on auth_sessions.token_status:
	// Allowed values: ('VALID', 'EXPIRED', 'REFRESHING', 'REVOKED')
	for _, validStatus := range []string{"VALID", "EXPIRED", "REFRESHING", "REVOKED"} {
		_, err := store.DB.Exec(`
			INSERT INTO auth_sessions (account_id, token_status, token_issued_at, token_expires_at)
			VALUES (1, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		`, validStatus)
		if err != nil {
			t.Errorf("Valid status %s was rejected: %v", validStatus, err)
		}
	}

	// Invalid status must be rejected
	_, err = store.DB.Exec(`
		INSERT INTO auth_sessions (account_id, token_status, token_issued_at, token_expires_at)
		VALUES (1, 'HACKED_STATUS', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`)
	if err == nil {
		t.Errorf("Expected CHECK constraint violation for invalid token_status, got nil")
	}

	// 2. Test Partial Unique Index idx_token_dedup_chat on token_usage_logs(request_type) WHERE request_type LIKE 'CHAT_%'
	_, err = store.DB.Exec(`
		INSERT INTO token_usage_logs (account_id, model_name, request_type)
		VALUES (1, 'gemini-pro', 'CHAT_conv1_step1')
	`)
	if err != nil {
		t.Fatalf("Failed to insert initial CHAT event: %v", err)
	}

	// Duplicate CHAT event must fail unique constraint
	_, err = store.DB.Exec(`
		INSERT INTO token_usage_logs (account_id, model_name, request_type)
		VALUES (1, 'gemini-pro', 'CHAT_conv1_step1')
	`)
	if err == nil {
		t.Errorf("Expected UNIQUE constraint violation for duplicate CHAT request_type, got nil")
	}

	// Non-CHAT request_type can have duplicates
	_, err = store.DB.Exec(`
		INSERT INTO token_usage_logs (account_id, model_name, request_type)
		VALUES (1, 'gemini-pro', 'INTERACTIVE')
	`)
	if err != nil {
		t.Fatalf("First INTERACTIVE failed: %v", err)
	}
	_, err = store.DB.Exec(`
		INSERT INTO token_usage_logs (account_id, model_name, request_type)
		VALUES (1, 'gemini-pro', 'INTERACTIVE')
	`)
	if err != nil {
		t.Errorf("Second INTERACTIVE should NOT violate index, got: %v", err)
	}

	// 3. Test idx_agent_fleet_subagent UNIQUE constraint
	_, err = store.DB.Exec(`
		INSERT INTO agent_fleet_telemetry (account_id, subagent_id, role_name, task_name, started_at)
		VALUES (1, 'subagent-unique-123', 'Explorer', 'Inspect code', CURRENT_TIMESTAMP)
	`)
	if err != nil {
		t.Fatalf("Failed to insert first subagent: %v", err)
	}

	_, err = store.DB.Exec(`
		INSERT INTO agent_fleet_telemetry (account_id, subagent_id, role_name, task_name, started_at)
		VALUES (1, 'subagent-unique-123', 'Worker', 'Duplicate ID', CURRENT_TIMESTAMP)
	`)
	if err == nil {
		t.Errorf("Expected UNIQUE constraint violation on idx_agent_fleet_subagent, got nil")
	}

	// 4. Test Composite UNIQUE key on token_usage_hourly_rollup(account_id, time_bucket, model_name)
	timeBucket := "2026-09-11 10:00:00"
	_, err = store.DB.Exec(`
		INSERT INTO token_usage_hourly_rollup (account_id, time_bucket, model_name, call_count)
		VALUES (1, ?, 'gemini-pro', 10)
	`, timeBucket)
	if err != nil {
		t.Fatalf("Failed to insert initial rollup: %v", err)
	}

	_, err = store.DB.Exec(`
		INSERT INTO token_usage_hourly_rollup (account_id, time_bucket, model_name, call_count)
		VALUES (1, ?, 'gemini-pro', 20)
	`, timeBucket)
	if err == nil {
		t.Errorf("Expected composite UNIQUE violation on hourly rollup, got nil")
	}
}
