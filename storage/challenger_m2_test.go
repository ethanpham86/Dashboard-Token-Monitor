package storage_test

import (
	"database/sql"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"tokenmonitor/storage"

	_ "modernc.org/sqlite"
)

// TestChallenger_M2_WorkstationActualDetection tests detection against the workstation's actual Antigravity IDE state DB
func TestChallenger_M2_WorkstationActualDetection(t *testing.T) {
	appData := os.Getenv("APPDATA")
	actualPath := filepath.Join(appData, "Antigravity IDE", "User", "globalStorage", "state.vscdb")
	if _, err := os.Stat(actualPath); os.IsNotExist(err) {
		t.Skipf("Actual Antigravity state.vscdb not found at %s; skipping live workstation check", actualPath)
	}

	name, email, plan, err := storage.DetectActiveAntigravityAccount()
	if err != nil {
		t.Fatalf("DetectActiveAntigravityAccount failed on workstation database: %v", err)
	}

	t.Logf("Live Antigravity Account Detected: Name='%s', Email='%s', Plan='%s'", name, email, plan)

	if email == "" {
		t.Errorf("Expected detected email to be non-empty")
	}
	if !strings.Contains(email, "@") {
		t.Errorf("Expected valid email with '@', got: '%s'", email)
	}
	if plan == "" {
		t.Errorf("Expected detected plan to be non-empty")
	}
	if name == "" {
		t.Errorf("Expected detected name to be non-empty")
	}

	// Verify plan is either Pro or Ultra
	if !strings.Contains(plan, "Google AI") {
		t.Errorf("Expected plan to contain 'Google AI', got: '%s'", plan)
	}
}

// TestChallenger_M2_MissingStateDB_Fallback verifies graceful handling when state.vscdb is missing
func TestChallenger_M2_MissingStateDB_Fallback(t *testing.T) {
	origEnv := os.Getenv("ANTIGRAVITY_STATE_DB")
	origAppData := os.Getenv("APPDATA")
	defer func() {
		os.Setenv("ANTIGRAVITY_STATE_DB", origEnv)
		os.Setenv("APPDATA", origAppData)
	}()

	tempEmptyDir := t.TempDir()
	os.Setenv("ANTIGRAVITY_STATE_DB", "")
	os.Setenv("APPDATA", tempEmptyDir)

	// Must not panic and must return error
	name, email, plan, err := storage.DetectActiveAntigravityAccount()
	if err == nil {
		t.Fatalf("Expected error when state.vscdb is missing, got nil (name='%s', email='%s', plan='%s')", name, email, plan)
	}

	if name != "" || email != "" || plan != "" {
		t.Errorf("Expected empty returns on error, got name='%s', email='%s', plan='%s'", name, email, plan)
	}

	if !strings.Contains(err.Error(), "không tìm thấy file state.vscdb") {
		t.Errorf("Expected error to indicate missing state.vscdb, got: %v", err)
	}
}

// TestChallenger_M2_DualMode_JSON_And_Protobuf tests both Primary (JSON) and Secondary (Protobuf) modes
func TestChallenger_M2_DualMode_JSON_And_Protobuf(t *testing.T) {
	origEnv := os.Getenv("ANTIGRAVITY_STATE_DB")
	defer os.Setenv("ANTIGRAVITY_STATE_DB", origEnv)

	t.Run("Mode1_JSON_Basic", func(t *testing.T) {
		tempDir := t.TempDir()
		t.Cleanup(func() {
			runtime.GC()
			time.Sleep(20 * time.Millisecond)
		})
		mockDBPath := filepath.Join(tempDir, "state.vscdb")

		// Create mock database with ItemTable
		db, err := sql.Open("sqlite", mockDBPath)
		if err != nil {
			t.Fatalf("Failed to create mock db: %v", err)
		}
		db.SetMaxIdleConns(0)
		_, _ = db.Exec("PRAGMA journal_mode = DELETE;")
		_, err = db.Exec(`
			CREATE TABLE ItemTable (key TEXT PRIMARY KEY, value TEXT);
			INSERT INTO ItemTable (key, value) VALUES ('antigravityAuthStatus', '{"name":"John Doe","email":"john.doe@gmail.com","apiKey":"test-key","userStatusProtoBinaryBase64":""}');
		`)
		if err != nil {
			db.Close()
			t.Fatalf("Failed to populate mock db: %v", err)
		}
		db.Close()

		os.Setenv("ANTIGRAVITY_STATE_DB", mockDBPath)
		name, email, plan, err := storage.DetectActiveAntigravityAccount()
		if err != nil {
			t.Fatalf("Mode 1 detection failed: %v", err)
		}

		if name != "John Doe" {
			t.Errorf("Expected name 'John Doe', got '%s'", name)
		}
		if email != "john.doe@gmail.com" {
			t.Errorf("Expected email 'john.doe@gmail.com', got '%s'", email)
		}
		if plan != "Google AI Pro" {
			t.Errorf("Expected plan 'Google AI Pro', got '%s'", plan)
		}
	})

	t.Run("Mode1_JSON_With_Ultra_Proto_Base64", func(t *testing.T) {
		tempDir := t.TempDir()
		t.Cleanup(func() {
			runtime.GC()
			time.Sleep(20 * time.Millisecond)
		})
		mockDBPath := filepath.Join(tempDir, "state.vscdb")

		protoPayload := base64.StdEncoding.EncodeToString([]byte("dummy protobuf header g1-ultra-tier Google AI Ultra trailer"))

		db, err := sql.Open("sqlite", mockDBPath)
		if err != nil {
			t.Fatalf("Failed to create mock db: %v", err)
		}
		db.SetMaxIdleConns(0)
		_, _ = db.Exec("PRAGMA journal_mode = DELETE;")
		valJSON := fmt.Sprintf(`{"name":"Jane Smith","email":"jane@example.com","apiKey":"k","userStatusProtoBinaryBase64":"%s"}`, protoPayload)
		_, err = db.Exec(`
			CREATE TABLE ItemTable (key TEXT PRIMARY KEY, value TEXT);
			INSERT INTO ItemTable (key, value) VALUES ('antigravityAuthStatus', ?);
		`, valJSON)
		if err != nil {
			db.Close()
			t.Fatalf("Failed to populate mock db: %v", err)
		}
		db.Close()

		os.Setenv("ANTIGRAVITY_STATE_DB", mockDBPath)
		name, email, plan, err := storage.DetectActiveAntigravityAccount()
		if err != nil {
			t.Fatalf("Mode 1 Ultra detection failed: %v", err)
		}

		if name != "Jane Smith" {
			t.Errorf("Expected name 'Jane Smith', got '%s'", name)
		}
		if email != "jane@example.com" {
			t.Errorf("Expected email 'jane@example.com', got '%s'", email)
		}
		if plan != "Google AI Ultra (20X Ultra Tier)" {
			t.Errorf("Expected plan 'Google AI Ultra (20X Ultra Tier)', got '%s'", plan)
		}
	})

	t.Run("Mode2_Secondary_Protobuf_Direct", func(t *testing.T) {
		tempDir := t.TempDir()
		t.Cleanup(func() {
			runtime.GC()
			time.Sleep(20 * time.Millisecond)
		})
		mockDBPath := filepath.Join(tempDir, "state.vscdb")

		// Create raw protobuf payload matching Antigravity IDE format:
		// [bytes prefix]: [Name] [separator] [Email] [separator] [Plan/Tier]
		rawProto := "\x08\x01\x12\x1aAuthor Name: Alice Wonderland:alice.wonderland@gmail.com\x22\x30tier:g1-ultra-tier Google AI Ultra Plan"
		rawB64 := base64.StdEncoding.EncodeToString([]byte(rawProto))

		db, err := sql.Open("sqlite", mockDBPath)
		if err != nil {
			t.Fatalf("Failed to create mock db: %v", err)
		}
		db.SetMaxIdleConns(0)
		_, _ = db.Exec("PRAGMA journal_mode = DELETE;")
		_, err = db.Exec(`
			CREATE TABLE ItemTable (key TEXT PRIMARY KEY, value TEXT);
			INSERT INTO ItemTable (key, value) VALUES ('antigravityUnifiedStateSync.userStatus', ?);
		`, rawB64)
		if err != nil {
			db.Close()
			t.Fatalf("Failed to populate mock db: %v", err)
		}
		db.Close()

		os.Setenv("ANTIGRAVITY_STATE_DB", mockDBPath)
		name, email, plan, err := storage.DetectActiveAntigravityAccount()
		if err != nil {
			t.Fatalf("Mode 2 Protobuf detection failed: %v", err)
		}

		if email != "alice.wonderland@gmail.com" {
			t.Errorf("Expected email 'alice.wonderland@gmail.com', got '%s'", email)
		}
		if plan != "Google AI Ultra (20X Ultra Tier)" {
			t.Errorf("Expected plan 'Google AI Ultra (20X Ultra Tier)', got '%s'", plan)
		}
		if !strings.Contains(name, "Alice") {
			t.Errorf("Expected name to contain 'Alice', got '%s'", name)
		}
	})

	t.Run("Mode2_Secondary_Protobuf_NestedB64", func(t *testing.T) {
		tempDir := t.TempDir()
		t.Cleanup(func() {
			runtime.GC()
			time.Sleep(20 * time.Millisecond)
		})
		mockDBPath := filepath.Join(tempDir, "state.vscdb")

		// Inner payload long enough to exceed 100 base64 chars (nestedB64Re pattern)
		innerPayload := "User Profile: Bob Builder bob.builder@google.com with active subscription tier Google AI Pro and extra padding characters to make the inner base64 string exceed one hundred characters long easily for regex matching."
		innerB64 := base64.StdEncoding.EncodeToString([]byte(innerPayload))
		outerPayload := fmt.Sprintf("HEADER_PREFIX___%s___TRAILER_SUFFIX", innerB64)
		outerB64 := base64.StdEncoding.EncodeToString([]byte(outerPayload))

		db, err := sql.Open("sqlite", mockDBPath)
		if err != nil {
			t.Fatalf("Failed to create mock db: %v", err)
		}
		db.SetMaxIdleConns(0)
		_, _ = db.Exec("PRAGMA journal_mode = DELETE;")
		_, err = db.Exec(`
			CREATE TABLE ItemTable (key TEXT PRIMARY KEY, value TEXT);
			INSERT INTO ItemTable (key, value) VALUES ('antigravityUnifiedStateSync.userStatus', ?);
		`, outerB64)
		if err != nil {
			db.Close()
			t.Fatalf("Failed to populate mock db: %v", err)
		}
		db.Close()

		os.Setenv("ANTIGRAVITY_STATE_DB", mockDBPath)
		name, email, plan, err := storage.DetectActiveAntigravityAccount()
		if err != nil {
			t.Fatalf("Mode 2 Nested Protobuf detection failed: %v", err)
		}

		if email != "bob.builder@google.com" {
			t.Errorf("Expected email 'bob.builder@google.com', got '%s'", email)
		}
		if plan != "Google AI Pro" {
			t.Errorf("Expected plan 'Google AI Pro', got '%s'", plan)
		}
		if !strings.Contains(name, "Bob") {
			t.Errorf("Expected name to contain 'Bob', got '%s'", name)
		}
	})

	t.Run("Missing_Both_Keys_Error", func(t *testing.T) {
		tempDir := t.TempDir()
		t.Cleanup(func() {
			runtime.GC()
			time.Sleep(20 * time.Millisecond)
		})
		mockDBPath := filepath.Join(tempDir, "state.vscdb")

		db, err := sql.Open("sqlite", mockDBPath)
		if err != nil {
			t.Fatalf("Failed to create mock db: %v", err)
		}
		db.SetMaxIdleConns(0)
		_, _ = db.Exec("PRAGMA journal_mode = DELETE;")
		_, err = db.Exec(`
			CREATE TABLE ItemTable (key TEXT PRIMARY KEY, value TEXT);
			INSERT INTO ItemTable (key, value) VALUES ('otherKey', 'otherValue');
		`)
		if err != nil {
			db.Close()
			t.Fatalf("Failed to populate mock db: %v", err)
		}
		db.Close()

		os.Setenv("ANTIGRAVITY_STATE_DB", mockDBPath)
		_, _, _, err = storage.DetectActiveAntigravityAccount()
		if err == nil {
			t.Fatalf("Expected error when neither auth key exists, got nil")
		}
		if !strings.Contains(err.Error(), "không tìm thấy key auth") {
			t.Errorf("Expected error to mention auth keys, got: %v", err)
		}
	})
}

// TestChallenger_M2_ReadOnlyLockIsolation verifies that sqlite_detector mode=ro operates
// without write permissions, does NOT generate wal/shm files, and bypasses the global WAL hook.
func TestChallenger_M2_ReadOnlyLockIsolation(t *testing.T) {
	tempDir := t.TempDir()
	t.Cleanup(func() {
		runtime.GC()
		time.Sleep(25 * time.Millisecond)
	})
	dbPath := filepath.Join(tempDir, "readonly_test.db")

	// 1. Create a plain SQLite database without WAL mode
	setupDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to create setup db: %v", err)
	}
	setupDB.SetMaxIdleConns(0)
	_, err = setupDB.Exec(`
		PRAGMA journal_mode = DELETE;
		CREATE TABLE test_data (id INTEGER PRIMARY KEY, msg TEXT);
		INSERT INTO test_data (msg) VALUES ('immutable');
	`)
	if err != nil {
		setupDB.Close()
		t.Fatalf("Failed to initialize test table: %v", err)
	}
	setupDB.Close()

	// Ensure no -wal or -shm files exist before opening read-only
	walPath := dbPath + "-wal"
	shmPath := dbPath + "-shm"
	_ = os.Remove(walPath)
	_ = os.Remove(shmPath)

	if _, err := os.Stat(walPath); !os.IsNotExist(err) {
		t.Fatalf("wal file exists before test")
	}
	if _, err := os.Stat(shmPath); !os.IsNotExist(err) {
		t.Fatalf("shm file exists before test")
	}

	// 2. Open using the dedicated "sqlite_detector" driver with mode=ro
	roDSN := fmt.Sprintf("file:%s?mode=ro", filepath.ToSlash(dbPath))
	roDB, err := sql.Open("sqlite_detector", roDSN)
	if err != nil {
		t.Fatalf("Failed to open db with sqlite_detector: %v", err)
	}
	defer roDB.Close()

	// 3. Test Read Operation succeeds
	var msg string
	err = roDB.QueryRow("SELECT msg FROM test_data WHERE id = 1").Scan(&msg)
	if err != nil {
		t.Fatalf("Read operation failed on read-only database: %v", err)
	}
	if msg != "immutable" {
		t.Errorf("Expected 'immutable', got '%s'", msg)
	}

	// 4. Test Write Operation FAILS with read-only error
	_, err = roDB.Exec("INSERT INTO test_data (msg) VALUES ('forbidden')")
	if err == nil {
		t.Fatalf("Expected write operation to FAIL on mode=ro database, but it succeeded!")
	}
	if !strings.Contains(err.Error(), "readonly") && !strings.Contains(err.Error(), "attempt to write") {
		t.Errorf("Expected read-only violation error, got: %v", err)
	}

	// 5. Test Schema Modification FAILS with read-only error
	_, err = roDB.Exec("CREATE TABLE evil (x INT)")
	if err == nil {
		t.Fatalf("Expected CREATE TABLE to FAIL on mode=ro database, but it succeeded!")
	}

	// 6. Test WAL / SHM isolation: opening with mode=ro MUST NOT create -wal or -shm files
	if _, err := os.Stat(walPath); !os.IsNotExist(err) {
		t.Errorf("CRITICAL ISOLATION BREACH: mode=ro opened database created a -wal file: %s", walPath)
	}
	if _, err := os.Stat(shmPath); !os.IsNotExist(err) {
		t.Errorf("CRITICAL ISOLATION BREACH: mode=ro opened database created a -shm file: %s", shmPath)
	}

	t.Log("Empirical verification PASSED: mode=ro with sqlite_detector achieves complete lock and write isolation.")
}

// TestChallenger_M2_Stress_ConcurrentDetection verifies high-concurrency read operations
// without lock exhaustion, data races, or panics.
func TestChallenger_M2_Stress_ConcurrentDetection(t *testing.T) {
	appData := os.Getenv("APPDATA")
	actualPath := filepath.Join(appData, "Antigravity IDE", "User", "globalStorage", "state.vscdb")
	if _, err := os.Stat(actualPath); os.IsNotExist(err) {
		t.Skip("Skipping live concurrent test: actual state.vscdb not found")
	}

	concurrency := 20
	errChan := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		go func(id int) {
			name, email, plan, err := storage.DetectActiveAntigravityAccount()
			if err != nil {
				errChan <- fmt.Errorf("goroutine %d failed: %w", id, err)
				return
			}
			if email == "" || name == "" || plan == "" {
				errChan <- fmt.Errorf("goroutine %d returned empty fields: name='%s', email='%s', plan='%s'", id, name, email, plan)
				return
			}
			errChan <- nil
		}(i)
	}

	for i := 0; i < concurrency; i++ {
		if err := <-errChan; err != nil {
			t.Errorf("Concurrent detection failure: %v", err)
		}
	}
}

// TestChallenger_M2_Stress_CorruptedFiles tests resilience against corrupted or abnormal files
func TestChallenger_M2_Stress_CorruptedFiles(t *testing.T) {
	origEnv := os.Getenv("ANTIGRAVITY_STATE_DB")
	defer os.Setenv("ANTIGRAVITY_STATE_DB", origEnv)

	t.Run("ZeroByteFile", func(t *testing.T) {
		tempDir := t.TempDir()
		emptyFile := filepath.Join(tempDir, "empty.vscdb")
		if err := os.WriteFile(emptyFile, []byte{}, 0644); err != nil {
			t.Fatalf("Failed to write 0-byte file: %v", err)
		}

		os.Setenv("ANTIGRAVITY_STATE_DB", emptyFile)
		_, _, _, err := storage.DetectActiveAntigravityAccount()
		if err == nil {
			t.Errorf("Expected error when reading 0-byte file, got nil")
		}
	})

	t.Run("GarbageDataFile", func(t *testing.T) {
		tempDir := t.TempDir()
		garbageFile := filepath.Join(tempDir, "garbage.vscdb")
		if err := os.WriteFile(garbageFile, []byte("THIS_IS_NOT_A_SQLITE_DATABASE_HEADER_AT_ALL"), 0644); err != nil {
			t.Fatalf("Failed to write garbage file: %v", err)
		}

		os.Setenv("ANTIGRAVITY_STATE_DB", garbageFile)
		_, _, _, err := storage.DetectActiveAntigravityAccount()
		if err == nil {
			t.Errorf("Expected error when reading non-sqlite file, got nil")
		}
	})

	t.Run("NoItemTableInDB", func(t *testing.T) {
		tempDir := t.TempDir()
		t.Cleanup(func() {
			runtime.GC()
			time.Sleep(25 * time.Millisecond)
		})
		noTableDB := filepath.Join(tempDir, "notable.vscdb")
		db, err := sql.Open("sqlite", noTableDB)
		if err != nil {
			t.Fatalf("Failed to create db: %v", err)
		}
		db.SetMaxIdleConns(0)
		_, _ = db.Exec("PRAGMA journal_mode = DELETE;")
		_, _ = db.Exec("CREATE TABLE some_other_table (x INT);")
		db.Close()

		os.Setenv("ANTIGRAVITY_STATE_DB", noTableDB)
		_, _, _, err = storage.DetectActiveAntigravityAccount()
		if err == nil {
			t.Errorf("Expected error when ItemTable is missing, got nil")
		}
	})

	t.Run("InvalidBase64InUserStatus", func(t *testing.T) {
		tempDir := t.TempDir()
		t.Cleanup(func() {
			runtime.GC()
			time.Sleep(25 * time.Millisecond)
		})
		invalidB64DB := filepath.Join(tempDir, "invalidb64.vscdb")
		db, err := sql.Open("sqlite", invalidB64DB)
		if err != nil {
			t.Fatalf("Failed to create db: %v", err)
		}
		db.SetMaxIdleConns(0)
		_, _ = db.Exec("PRAGMA journal_mode = DELETE;")
		_, _ = db.Exec(`
			CREATE TABLE ItemTable (key TEXT PRIMARY KEY, value TEXT);
			INSERT INTO ItemTable (key, value) VALUES ('antigravityUnifiedStateSync.userStatus', '!!!NOT_BASE64_AT_ALL???');
		`)
		db.Close()

		os.Setenv("ANTIGRAVITY_STATE_DB", invalidB64DB)
		_, _, _, err = storage.DetectActiveAntigravityAccount()
		if err == nil {
			t.Errorf("Expected error for invalid base64, got nil")
		}
		if !strings.Contains(err.Error(), "giải mã base64") {
			t.Errorf("Expected base64 decode error, got: %v", err)
		}
	})
}

// TestChallenger_M2_Stress_DirectoryAsDB tests pointing ANTIGRAVITY_STATE_DB to a folder
func TestChallenger_M2_Stress_DirectoryAsDB(t *testing.T) {
	origEnv := os.Getenv("ANTIGRAVITY_STATE_DB")
	defer os.Setenv("ANTIGRAVITY_STATE_DB", origEnv)

	tempDir := t.TempDir()
	os.Setenv("ANTIGRAVITY_STATE_DB", tempDir)

	_, _, _, err := storage.DetectActiveAntigravityAccount()
	if err == nil {
		t.Errorf("Expected error when ANTIGRAVITY_STATE_DB is a directory, got nil")
	}
}
