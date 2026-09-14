package storage_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"tokenmonitor/config"
	"tokenmonitor/storage"
)

func createTestStorage(t *testing.T, dbPath string) (*storage.Storage, func()) {
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
		},
		AccountProfile: config.AccountProfileConfig{
			Email:              "challenger@example.com",
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

	cleanup := func() {
		_ = store.Close()
		_ = os.Remove(dbPath)
		_ = os.Remove(dbPath + "-wal")
		_ = os.Remove(dbPath + "-shm")
	}

	return store, cleanup
}

// TestChallenger_PRAGMAs verifies that newly opened connections in the connection pool
// execute with foreign_keys = ON and busy_timeout = 5000, along with other required PRAGMAs.
func TestChallenger_PRAGMAs(t *testing.T) {
	dbPath := "./test_challenger_pragmas.db"
	store, cleanup := createTestStorage(t, dbPath)
	defer cleanup()

	ctx := context.Background()
	connsCount := 8
	var conns []*sql.Conn
	defer func() {
		for _, c := range conns {
			if c != nil {
				_ = c.Close()
			}
		}
	}()

	// Open multiple concurrent dedicated connections to force the pool to create new underlying connections
	for i := 0; i < connsCount; i++ {
		conn, err := store.DB.Conn(ctx)
		if err != nil {
			t.Fatalf("Failed to obtain dedicated connection #%d from pool: %v", i+1, err)
		}
		conns = append(conns, conn)
	}

	var wg sync.WaitGroup
	for i, conn := range conns {
		wg.Add(1)
		go func(idx int, c *sql.Conn) {
			defer wg.Done()

			// 1. Verify foreign_keys = ON (1)
			var fk int
			if err := c.QueryRowContext(ctx, "PRAGMA foreign_keys;").Scan(&fk); err != nil {
				t.Errorf("Conn #%d: Query PRAGMA foreign_keys failed: %v", idx, err)
			} else if fk != 1 {
				t.Errorf("Conn #%d: Expected foreign_keys=1, got %d", idx, fk)
			}

			// 2. Verify busy_timeout = 5000
			var busyTimeout int
			if err := c.QueryRowContext(ctx, "PRAGMA busy_timeout;").Scan(&busyTimeout); err != nil {
				t.Errorf("Conn #%d: Query PRAGMA busy_timeout failed: %v", idx, err)
			} else if busyTimeout != 5000 {
				t.Errorf("Conn #%d: Expected busy_timeout=5000, got %d", idx, busyTimeout)
			}

			// 3. Verify synchronous = NORMAL (1)
			var syncMode int
			if err := c.QueryRowContext(ctx, "PRAGMA synchronous;").Scan(&syncMode); err != nil {
				t.Errorf("Conn #%d: Query PRAGMA synchronous failed: %v", idx, err)
			} else if syncMode != 1 {
				t.Errorf("Conn #%d: Expected synchronous=1 (NORMAL), got %d", idx, syncMode)
			}

			// 4. Verify temp_store = MEMORY (2)
			var tempStore int
			if err := c.QueryRowContext(ctx, "PRAGMA temp_store;").Scan(&tempStore); err != nil {
				t.Errorf("Conn #%d: Query PRAGMA temp_store failed: %v", idx, err)
			} else if tempStore != 2 {
				t.Errorf("Conn #%d: Expected temp_store=2 (MEMORY), got %d", idx, tempStore)
			}

			// 5. Verify cache_size = -64000
			var cacheSize int
			if err := c.QueryRowContext(ctx, "PRAGMA cache_size;").Scan(&cacheSize); err != nil {
				t.Errorf("Conn #%d: Query PRAGMA cache_size failed: %v", idx, err)
			} else if cacheSize != -64000 {
				t.Errorf("Conn #%d: Expected cache_size=-64000, got %d", idx, cacheSize)
			}

			// 6. Verify journal_mode = WAL
			var journalMode string
			if err := c.QueryRowContext(ctx, "PRAGMA journal_mode;").Scan(&journalMode); err != nil {
				t.Errorf("Conn #%d: Query PRAGMA journal_mode failed: %v", idx, err)
			} else if strings.ToLower(journalMode) != "wal" {
				t.Errorf("Conn #%d: Expected journal_mode=wal, got %s", idx, journalMode)
			}
		}(i, conn)
	}
	wg.Wait()

	// Now close all connections, wait, and acquire a brand new connection after pool churn
	for _, c := range conns {
		_ = c.Close()
	}
	conns = nil

	freshConn, err := store.DB.Conn(ctx)
	if err != nil {
		t.Fatalf("Failed to obtain fresh connection after churn: %v", err)
	}
	defer freshConn.Close()

	var fk int
	var bt int
	if err := freshConn.QueryRowContext(ctx, "PRAGMA foreign_keys;").Scan(&fk); err != nil || fk != 1 {
		t.Fatalf("Fresh connection after churn foreign_keys expected 1, got %d (err: %v)", fk, err)
	}
	if err := freshConn.QueryRowContext(ctx, "PRAGMA busy_timeout;").Scan(&bt); err != nil || bt != 5000 {
		t.Fatalf("Fresh connection after churn busy_timeout expected 5000, got %d (err: %v)", bt, err)
	}
}

// TestChallenger_ForeignKeyCascade verifies that deleting an account from accounts
// cascades and deletes associated rows in auth_sessions, token_usage_logs, and token_usage_hourly_rollup,
// and that inserting child rows referencing a non-existent account fails.
func TestChallenger_ForeignKeyCascade(t *testing.T) {
	dbPath := "./test_challenger_fk_cascade.db"
	store, cleanup := createTestStorage(t, dbPath)
	defer cleanup()

	// 1. Verify foreign key violation on inserting orphan records
	_, err := store.DB.Exec(`
		INSERT INTO auth_sessions (account_id, token_status, token_issued_at, token_expires_at)
		VALUES (999999, 'VALID', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`)
	if err == nil {
		t.Fatal("Expected foreign key constraint failure when inserting auth_sessions with non-existent account_id, but got nil")
	}

	_, err = store.DB.Exec(`
		INSERT INTO token_usage_logs (account_id, model_name, request_type)
		VALUES (999999, 'gemini-2.5-pro', 'INTERACTIVE')
	`)
	if err == nil {
		t.Fatal("Expected foreign key constraint failure when inserting token_usage_logs with non-existent account_id, but got nil")
	}

	_, err = store.DB.Exec(`
		INSERT INTO token_usage_hourly_rollup (account_id, time_bucket, model_name)
		VALUES (999999, '2026-09-08 12:00:00', 'gemini-2.5-pro')
	`)
	if err == nil {
		t.Fatal("Expected foreign key constraint failure when inserting token_usage_hourly_rollup with non-existent account_id, but got nil")
	}

	// 2. Create a specific test account to test CASCADE deletion
	res, err := store.DB.Exec(`
		INSERT INTO accounts (account_email, installation_uuid, registered_at, subscription_expiry)
		VALUES ('cascade_target@example.com', 'test-uuid-cascade', '2026-09-08 00:00:00', '2026-10-08 00:00:00')
	`)
	if err != nil {
		t.Fatalf("Failed to insert test account: %v", err)
	}
	targetAccountID, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("Failed to get targetAccountID: %v", err)
	}

	// 3. Insert child records in auth_sessions, token_usage_logs, token_usage_hourly_rollup
	for i := 0; i < 3; i++ {
		_, err = store.DB.Exec(`
			INSERT INTO auth_sessions (account_id, token_status, token_issued_at, token_expires_at)
			VALUES (?, 'VALID', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		`, targetAccountID)
		if err != nil {
			t.Fatalf("Failed to insert auth_session #%d: %v", i+1, err)
		}
	}

	for i := 0; i < 5; i++ {
		_, err = store.DB.Exec(`
			INSERT INTO token_usage_logs (account_id, model_name, request_type, prompt_tokens, output_tokens, total_tokens)
			VALUES (?, 'gemini-2.5-flash', 'INTERACTIVE', 100, 50, 150)
		`, targetAccountID)
		if err != nil {
			t.Fatalf("Failed to insert token_usage_logs #%d: %v", i+1, err)
		}
	}

	for i := 0; i < 2; i++ {
		timeBucket := fmt.Sprintf("2026-09-08 %02d:00:00", 10+i)
		_, err = store.DB.Exec(`
			INSERT INTO token_usage_hourly_rollup (account_id, time_bucket, model_name, call_count, sum_total_tokens)
			VALUES (?, ?, 'gemini-2.5-flash', 10, 1500)
		`, targetAccountID, timeBucket)
		if err != nil {
			t.Fatalf("Failed to insert token_usage_hourly_rollup #%d: %v", i+1, err)
		}
	}

	// 4. Verify that child records are present before deletion
	var countSessions, countLogs, countRollups int
	_ = store.DB.QueryRow("SELECT COUNT(*) FROM auth_sessions WHERE account_id = ?", targetAccountID).Scan(&countSessions)
	_ = store.DB.QueryRow("SELECT COUNT(*) FROM token_usage_logs WHERE account_id = ?", targetAccountID).Scan(&countLogs)
	_ = store.DB.QueryRow("SELECT COUNT(*) FROM token_usage_hourly_rollup WHERE account_id = ?", targetAccountID).Scan(&countRollups)

	if countSessions != 3 || countLogs != 5 || countRollups != 2 {
		t.Fatalf("Pre-deletion counts mismatch: sessions=%d (expected 3), logs=%d (expected 5), rollups=%d (expected 2)",
			countSessions, countLogs, countRollups)
	}

	// 5. Delete the parent account
	delRes, err := store.DB.Exec("DELETE FROM accounts WHERE id = ?", targetAccountID)
	if err != nil {
		t.Fatalf("Failed to delete account id=%d: %v", targetAccountID, err)
	}
	affected, _ := delRes.RowsAffected()
	if affected != 1 {
		t.Fatalf("Expected 1 row deleted from accounts, got %d", affected)
	}

	// 6. Verify that CASCADE deleted all child records across all 3 child tables
	_ = store.DB.QueryRow("SELECT COUNT(*) FROM auth_sessions WHERE account_id = ?", targetAccountID).Scan(&countSessions)
	_ = store.DB.QueryRow("SELECT COUNT(*) FROM token_usage_logs WHERE account_id = ?", targetAccountID).Scan(&countLogs)
	_ = store.DB.QueryRow("SELECT COUNT(*) FROM token_usage_hourly_rollup WHERE account_id = ?", targetAccountID).Scan(&countRollups)

	if countSessions != 0 {
		t.Errorf("CASCADE failure: expected 0 auth_sessions after deleting account, found %d", countSessions)
	}
	if countLogs != 0 {
		t.Errorf("CASCADE failure: expected 0 token_usage_logs after deleting account, found %d", countLogs)
	}
	if countRollups != 0 {
		t.Errorf("CASCADE failure: expected 0 token_usage_hourly_rollup after deleting account, found %d", countRollups)
	}
}

// TestChallenger_UniquePartialIndexChat verifies that idx_token_dedup_chat enforces uniqueness
// for identical CHAT_% request_types, allows different CHAT_% types, and does NOT enforce
// uniqueness on non-CHAT request types like INTERACTIVE or BACKGROUND.
func TestChallenger_UniquePartialIndexChat(t *testing.T) {
	dbPath := "./test_challenger_idx_chat.db"
	store, cleanup := createTestStorage(t, dbPath)
	defer cleanup()

	var accountID int64
	err := store.DB.QueryRow("SELECT id FROM accounts LIMIT 1").Scan(&accountID)
	if err != nil {
		t.Fatalf("Failed to find account: %v", err)
	}

	// 1. Insert first CHAT_% request_type
	chatReqType := "CHAT_conv123_step001"
	_, err = store.DB.Exec(`
		INSERT INTO token_usage_logs (account_id, model_name, request_type, prompt_tokens, output_tokens, total_tokens)
		VALUES (?, 'gemini-2.5-pro', ?, 100, 200, 300)
	`, accountID, chatReqType)
	if err != nil {
		t.Fatalf("First insert of %s failed: %v", chatReqType, err)
	}

	// 2. Insert duplicate CHAT_% request_type -> MUST FAIL with UNIQUE constraint error
	_, err = store.DB.Exec(`
		INSERT INTO token_usage_logs (account_id, model_name, request_type, prompt_tokens, output_tokens, total_tokens)
		VALUES (?, 'gemini-2.5-pro', ?, 150, 250, 400)
	`, accountID, chatReqType)
	if err == nil {
		t.Fatalf("Expected UNIQUE constraint violation on duplicate request_type '%s', but insert succeeded!", chatReqType)
	}
	if !strings.Contains(err.Error(), "UNIQUE") && !strings.Contains(err.Error(), "constraint failed") {
		t.Errorf("Expected error to mention UNIQUE constraint, got: %v", err)
	}

	// 3. Insert distinct CHAT_% request_type -> MUST SUCCEED
	chatReqType2 := "CHAT_conv123_step002"
	_, err = store.DB.Exec(`
		INSERT INTO token_usage_logs (account_id, model_name, request_type, prompt_tokens, output_tokens, total_tokens)
		VALUES (?, 'gemini-2.5-pro', ?, 100, 200, 300)
	`, accountID, chatReqType2)
	if err != nil {
		t.Fatalf("Insert of distinct chat request_type %s failed: %v", chatReqType2, err)
	}

	// 4. Insert multiple non-CHAT records with identical request_type ('INTERACTIVE') -> MUST SUCCEED
	for i := 0; i < 5; i++ {
		_, err = store.DB.Exec(`
			INSERT INTO token_usage_logs (account_id, model_name, request_type, prompt_tokens, output_tokens, total_tokens)
			VALUES (?, 'gemini-2.5-pro', 'INTERACTIVE', 50, 50, 100)
		`, accountID)
		if err != nil {
			t.Fatalf("Insert #%d of non-chat request_type 'INTERACTIVE' failed unexpectedly: %v", i+1, err)
		}
	}

	// 5. Insert multiple non-CHAT records with identical request_type ('BACKGROUND') -> MUST SUCCEED
	for i := 0; i < 3; i++ {
		_, err = store.DB.Exec(`
			INSERT INTO token_usage_logs (account_id, model_name, request_type, prompt_tokens, output_tokens, total_tokens)
			VALUES (?, 'gemini-2.5-pro', 'BACKGROUND', 20, 20, 40)
		`, accountID)
		if err != nil {
			t.Fatalf("Insert #%d of non-chat request_type 'BACKGROUND' failed unexpectedly: %v", i+1, err)
		}
	}

	// 6. Adversarial edge case: check case behavior with lower-case 'chat_%'
	// In SQLite LIKE is case-insensitive for ASCII characters.
	// So 'chat_abc' matches `request_type LIKE 'CHAT_%'`.
	// What happens if we insert 'chat_conv999' then 'CHAT_conv999'?
	// The unique index is on (request_type). By default SQLite TEXT collation is BINARY unless specified NOCASE.
	// So 'chat_conv999' and 'CHAT_conv999' are distinct binary strings, but both enter the partial index.
	// Let's verify this behavior empirically.
	_, err = store.DB.Exec(`
		INSERT INTO token_usage_logs (account_id, model_name, request_type)
		VALUES (?, 'gemini-2.5-pro', 'chat_conv999_step1')
	`, accountID)
	if err != nil {
		t.Fatalf("Insert of 'chat_conv999_step1' failed: %v", err)
	}

	// Duplicate of 'chat_conv999_step1'
	_, err = store.DB.Exec(`
		INSERT INTO token_usage_logs (account_id, model_name, request_type)
		VALUES (?, 'gemini-2.5-pro', 'chat_conv999_step1')
	`, accountID)
	if err == nil {
		t.Fatal("Expected UNIQUE constraint violation on duplicate lower-case 'chat_conv999_step1', but insert succeeded!")
	}
}

// TestChallenger_Stress_BusyTimeout verifies that busy_timeout = 5000 actually causes a concurrent
// writer to wait and eventually succeed when a lock is held temporarily, rather than failing immediately.
func TestChallenger_Stress_BusyTimeout(t *testing.T) {
	dbPath := "./test_challenger_busy_timeout.db"
	store, cleanup := createTestStorage(t, dbPath)
	defer cleanup()

	ctx := context.Background()
	conn1, err := store.DB.Conn(ctx)
	if err != nil {
		t.Fatalf("Failed to get conn1: %v", err)
	}
	defer conn1.Close()

	conn2, err := store.DB.Conn(ctx)
	if err != nil {
		t.Fatalf("Failed to get conn2: %v", err)
	}
	defer conn2.Close()

	// 1. Conn1 begins exclusive write lock
	_, err = conn1.ExecContext(ctx, "BEGIN IMMEDIATE;")
	if err != nil {
		t.Fatalf("conn1 failed to begin immediate transaction: %v", err)
	}

	start := time.Now()
	lockDuration := 350 * time.Millisecond

	// In background, release conn1 lock after lockDuration
	go func() {
		time.Sleep(lockDuration)
		_, _ = conn1.ExecContext(ctx, "COMMIT;")
	}()

	// Conn2 attempts to begin immediate transaction while conn1 holds the lock.
	// Because busy_timeout = 5000ms, conn2 will WAIT and succeed when conn1 commits!
	// If busy_timeout was 0, it would fail immediately with "database is locked".
	_, err = conn2.ExecContext(ctx, "BEGIN IMMEDIATE;")
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("conn2 failed to acquire write lock with busy_timeout=5000: %v", err)
	}
	_, _ = conn2.ExecContext(ctx, "COMMIT;")

	if elapsed < 200*time.Millisecond {
		t.Errorf("Expected conn2 to wait for conn1 lock release (at least 200ms), but finished in %v", elapsed)
	}
	t.Logf("Empirical proof of busy_timeout: conn2 waited %v and succeeded after conn1 committed!", elapsed)
}

// TestChallenger_Cascade_IndexCleanup verifies that cascading deletion of an account cleanly
// frees up partial index slots in idx_token_dedup_chat, permitting immediate reuse.
func TestChallenger_Cascade_IndexCleanup(t *testing.T) {
	dbPath := "./test_challenger_cascade_index.db"
	store, cleanup := createTestStorage(t, dbPath)
	defer cleanup()

	// Insert Account A
	resA, err := store.DB.Exec(`
		INSERT INTO accounts (account_email, installation_uuid, registered_at, subscription_expiry)
		VALUES ('account_a@example.com', 'uuid-a', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`)
	if err != nil {
		t.Fatalf("Failed to insert account A: %v", err)
	}
	idA, _ := resA.LastInsertId()

	// Insert Account B
	resB, err := store.DB.Exec(`
		INSERT INTO accounts (account_email, installation_uuid, registered_at, subscription_expiry)
		VALUES ('account_b@example.com', 'uuid-b', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`)
	if err != nil {
		t.Fatalf("Failed to insert account B: %v", err)
	}
	idB, _ := resB.LastInsertId()

	sharedChatType := "CHAT_session_reused_001"

	// Insert chat log under Account A
	_, err = store.DB.Exec(`
		INSERT INTO token_usage_logs (account_id, model_name, request_type)
		VALUES (?, 'gemini-2.5-pro', ?)
	`, idA, sharedChatType)
	if err != nil {
		t.Fatalf("Insert under Account A failed: %v", err)
	}

	// Attempting to insert same chat log under Account B must fail before deletion
	_, err = store.DB.Exec(`
		INSERT INTO token_usage_logs (account_id, model_name, request_type)
		VALUES (?, 'gemini-2.5-pro', ?)
	`, idB, sharedChatType)
	if err == nil {
		t.Fatal("Expected unique index failure before cascade delete, but insert succeeded")
	}

	// Delete Account A -> must cascade delete token_usage_logs for Account A
	_, err = store.DB.Exec("DELETE FROM accounts WHERE id = ?", idA)
	if err != nil {
		t.Fatalf("Failed to delete Account A: %v", err)
	}

	// Now inserting the same chat log under Account B MUST SUCCEED because index entry was purged by cascade
	_, err = store.DB.Exec(`
		INSERT INTO token_usage_logs (account_id, model_name, request_type)
		VALUES (?, 'gemini-2.5-pro', ?)
	`, idB, sharedChatType)
	if err != nil {
		t.Fatalf("Insert under Account B failed after cascade delete of Account A: %v", err)
	}
}

// TestChallenger_ExistingDatabaseFile verifies the real runtime database ./data/token_monitor.db
func TestChallenger_ExistingDatabaseFile(t *testing.T) {
	dbPath := "../data/token_monitor.db"
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Skip("Existing runtime database ./data/token_monitor.db does not exist yet")
	}

	cfg := &config.Config{
		Database: config.DatabaseConfig{
			SQLitePath:    dbPath,
			MaxOpenConns:  2,
			EnableWALMode: true,
		},
	}

	store, err := storage.NewStorage(cfg)
	if err != nil {
		t.Fatalf("Failed to open existing database: %v", err)
	}
	defer store.Close()

	// 1. Verify 4 tables exist
	expectedTables := map[string]bool{
		"accounts":                  false,
		"auth_sessions":             false,
		"token_usage_logs":          false,
		"token_usage_hourly_rollup": false,
	}

	rows, err := store.DB.Query("SELECT name FROM sqlite_master WHERE type='table'")
	if err != nil {
		t.Fatalf("Failed to query tables from sqlite_master: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		_ = rows.Scan(&name)
		if _, exists := expectedTables[name]; exists {
			expectedTables[name] = true
		}
	}
	for tbl, found := range expectedTables {
		if !found {
			t.Errorf("Existing DB missing required table: %s", tbl)
		}
	}

	// 2. Verify 4 strategic indexes exist
	expectedIndexes := map[string]bool{
		"idx_token_usage_timestamp":     false,
		"idx_token_usage_account_model": false,
		"idx_hourly_bucket":             false,
		"idx_token_dedup_chat":          false,
	}

	idxRows, err := store.DB.Query("SELECT name FROM sqlite_master WHERE type='index'")
	if err != nil {
		t.Fatalf("Failed to query indexes from sqlite_master: %v", err)
	}
	defer idxRows.Close()

	for idxRows.Next() {
		var name string
		_ = idxRows.Scan(&name)
		if _, exists := expectedIndexes[name]; exists {
			expectedIndexes[name] = true
		}
	}
	for idx, found := range expectedIndexes {
		if !found {
			t.Errorf("Existing DB missing required index: %s", idx)
		}
	}

	// 3. Verify ON DELETE CASCADE on all 3 child tables
	childTables := []string{"auth_sessions", "token_usage_logs", "token_usage_hourly_rollup"}
	for _, ct := range childTables {
		fkQuery := fmt.Sprintf("PRAGMA foreign_key_list(%s);", ct)
		fkRows, err := store.DB.Query(fkQuery)
		if err != nil {
			t.Errorf("Failed to query foreign_key_list for %s: %v", ct, err)
			continue
		}
		var foundCascade bool
		for fkRows.Next() {
			var id, seq int
			var table, from, to, onUpdate, onDelete, match string
			_ = fkRows.Scan(&id, &seq, &table, &from, &to, &onUpdate, &onDelete, &match)
			if table == "accounts" && from == "account_id" && onDelete == "CASCADE" {
				foundCascade = true
			}
		}
		fkRows.Close()
		if !foundCascade {
			t.Errorf("Child table %s is missing ON DELETE CASCADE referencing accounts(id)", ct)
		}
	}
}
