package storage_test

import (
	"database/sql"
	"encoding/base64"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"tokenmonitor/storage"
)

// TestChallenger_Detector_MissingDB tests DetectActiveAntigravityAccount when no DB exists
func TestChallenger_Detector_MissingDB(t *testing.T) {
	tempDir := t.TempDir()
	t.Cleanup(func() {
		runtime.GC()
		time.Sleep(25 * time.Millisecond)
	})
	fakeDB := filepath.Join(tempDir, "missing_state.vscdb")

	origEnv := os.Getenv("ANTIGRAVITY_STATE_DB")
	origAppData := os.Getenv("APPDATA")
	defer func() {
		_ = os.Setenv("ANTIGRAVITY_STATE_DB", origEnv)
		_ = os.Setenv("APPDATA", origAppData)
	}()

	_ = os.Setenv("ANTIGRAVITY_STATE_DB", fakeDB)
	_ = os.Setenv("APPDATA", tempDir) // so fallback paths also don't exist

	name, email, plan, err := storage.DetectActiveAntigravityAccount()
	if err == nil {
		t.Fatalf("Expected error when state.vscdb is missing, got nil (name=%q, email=%q, plan=%q)", name, email, plan)
	}
}

// TestChallenger_Detector_PrimaryJSONMode tests key antigravityAuthStatus (JSON format)
func TestChallenger_Detector_PrimaryJSONMode(t *testing.T) {
	tempDir := t.TempDir()
	t.Cleanup(func() {
		runtime.GC()
		time.Sleep(25 * time.Millisecond)
	})
	dbPath := filepath.Join(tempDir, "state.vscdb")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open sqlite: %v", err)
	}
	db.SetMaxIdleConns(0)
	_, _ = db.Exec("PRAGMA journal_mode = DELETE;")
	_, err = db.Exec(`
		CREATE TABLE ItemTable (key TEXT PRIMARY KEY, value TEXT);
	`)
	if err != nil {
		db.Close()
		t.Fatalf("Failed to create ItemTable: %v", err)
	}

	// Payload with Ultra tier encoded in base64
	protoContent := base64.StdEncoding.EncodeToString([]byte("dummy prefix Google AI Ultra (20X Ultra Tier) dummy suffix"))
	jsonVal := `{"name":"Alice Tester","email":"alice@testcorp.com","apiKey":"xyz123","userStatusProtoBinaryBase64":"` + protoContent + `"}`

	_, err = db.Exec("INSERT INTO ItemTable (key, value) VALUES ('antigravityAuthStatus', ?)", jsonVal)
	if err != nil {
		db.Close()
		t.Fatalf("Failed to insert antigravityAuthStatus: %v", err)
	}
	db.Close()

	origEnv := os.Getenv("ANTIGRAVITY_STATE_DB")
	defer func() { _ = os.Setenv("ANTIGRAVITY_STATE_DB", origEnv) }()
	_ = os.Setenv("ANTIGRAVITY_STATE_DB", dbPath)

	name, email, plan, err := storage.DetectActiveAntigravityAccount()
	if err != nil {
		t.Fatalf("DetectActiveAntigravityAccount failed: %v", err)
	}
	if name != "Alice Tester" {
		t.Errorf("name = %q, expected 'Alice Tester'", name)
	}
	if email != "alice@testcorp.com" {
		t.Errorf("email = %q, expected 'alice@testcorp.com'", email)
	}
	if plan != "Google AI Ultra (20X Ultra Tier)" {
		t.Errorf("plan = %q, expected 'Google AI Ultra (20X Ultra Tier)'", plan)
	}
}

// TestChallenger_Detector_SecondaryProtobufMode tests key antigravityUnifiedStateSync.userStatus (Base64 Protobuf format)
func TestChallenger_Detector_SecondaryProtobufMode(t *testing.T) {
	tempDir := t.TempDir()
	t.Cleanup(func() {
		runtime.GC()
		time.Sleep(25 * time.Millisecond)
	})
	dbPath := filepath.Join(tempDir, "state.vscdb")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open sqlite: %v", err)
	}
	db.SetMaxIdleConns(0)
	_, _ = db.Exec("PRAGMA journal_mode = DELETE;")
	_, err = db.Exec(`
		CREATE TABLE ItemTable (key TEXT PRIMARY KEY, value TEXT);
	`)
	if err != nil {
		db.Close()
		t.Fatalf("Failed to create ItemTable: %v", err)
	}

	// Inner simulated protobuf stream containing Name, Email, and Ultra tier
	innerProtobuf := "binary\x01\x02User: Bob The Builder\x03\x04email: bob.builder@example.com\x05tier: g1-ultra-tier Google AI Ultra"
	encodedInner := base64.StdEncoding.EncodeToString([]byte(innerProtobuf))

	// Outer base64 wrapper as observed in actual state.vscdb
	outerProtobuf := "wrapper_header_" + encodedInner + "_wrapper_footer"
	encodedOuter := base64.StdEncoding.EncodeToString([]byte(outerProtobuf))

	_, err = db.Exec("INSERT INTO ItemTable (key, value) VALUES ('antigravityUnifiedStateSync.userStatus', ?)", encodedOuter)
	if err != nil {
		db.Close()
		t.Fatalf("Failed to insert userStatus: %v", err)
	}
	db.Close()

	origEnv := os.Getenv("ANTIGRAVITY_STATE_DB")
	defer func() { _ = os.Setenv("ANTIGRAVITY_STATE_DB", origEnv) }()
	_ = os.Setenv("ANTIGRAVITY_STATE_DB", dbPath)

	name, email, plan, err := storage.DetectActiveAntigravityAccount()
	if err != nil {
		t.Fatalf("DetectActiveAntigravityAccount failed: %v", err)
	}
	if email != "bob.builder@example.com" {
		t.Errorf("email = %q, expected 'bob.builder@example.com'", email)
	}
	if plan != "Google AI Ultra (20X Ultra Tier)" {
		t.Errorf("plan = %q, expected 'Google AI Ultra (20X Ultra Tier)'", plan)
	}
	if name == "" {
		t.Errorf("name should not be empty, got %q", name)
	}
}

// TestChallenger_Detector_ReadOnlyEnforcement verifies that opening a read-only file does NOT fail
// with SQLite write attempt errors (due to driver isolation from WAL connection hooks).
func TestChallenger_Detector_ReadOnlyEnforcement(t *testing.T) {
	tempDir := t.TempDir()
	t.Cleanup(func() {
		runtime.GC()
		time.Sleep(25 * time.Millisecond)
	})
	dbPath := filepath.Join(tempDir, "readonly_state.vscdb")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to create test db: %v", err)
	}
	db.SetMaxIdleConns(0)
	_, _ = db.Exec("PRAGMA journal_mode = DELETE;")
	_, err = db.Exec(`
		CREATE TABLE ItemTable (key TEXT PRIMARY KEY, value TEXT);
		INSERT INTO ItemTable (key, value) VALUES ('antigravityAuthStatus', '{"name":"Readonly User","email":"ro@example.com"}');
	`)
	if err != nil {
		db.Close()
		t.Fatalf("Failed to populate test db: %v", err)
	}
	db.Close()

	// Make the file read-only on disk
	if err := os.Chmod(dbPath, 0400); err != nil {
		t.Logf("Chmod warning: %v", err)
	}
	defer func() { _ = os.Chmod(dbPath, 0666) }()

	origEnv := os.Getenv("ANTIGRAVITY_STATE_DB")
	defer func() { _ = os.Setenv("ANTIGRAVITY_STATE_DB", origEnv) }()
	_ = os.Setenv("ANTIGRAVITY_STATE_DB", dbPath)

	name, email, plan, err := storage.DetectActiveAntigravityAccount()
	if err != nil {
		t.Fatalf("DetectActiveAntigravityAccount failed on read-only file: %v", err)
	}
	if name != "Readonly User" || email != "ro@example.com" || plan != "Google AI Pro" {
		t.Errorf("Unexpected result on read-only db: name=%q, email=%q, plan=%q", name, email, plan)
	}
}

// TestChallenger_Detector_CorruptedData tests graceful degradation on invalid or unparseable payloads
func TestChallenger_Detector_CorruptedData(t *testing.T) {
	tempDir := t.TempDir()
	t.Cleanup(func() {
		runtime.GC()
		time.Sleep(25 * time.Millisecond)
	})
	dbPath := filepath.Join(tempDir, "corrupted_state.vscdb")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open sqlite: %v", err)
	}
	db.SetMaxIdleConns(0)
	_, _ = db.Exec("PRAGMA journal_mode = DELETE;")
	_, err = db.Exec(`
		CREATE TABLE ItemTable (key TEXT PRIMARY KEY, value TEXT);
		INSERT INTO ItemTable (key, value) VALUES ('antigravityUnifiedStateSync.userStatus', '!!!NOT-VALID-BASE64!!!');
	`)
	if err != nil {
		db.Close()
		t.Fatalf("Failed to populate test db: %v", err)
	}
	db.Close()

	origEnv := os.Getenv("ANTIGRAVITY_STATE_DB")
	defer func() { _ = os.Setenv("ANTIGRAVITY_STATE_DB", origEnv) }()
	_ = os.Setenv("ANTIGRAVITY_STATE_DB", dbPath)

	name, email, plan, err := storage.DetectActiveAntigravityAccount()
	if err == nil {
		t.Errorf("Expected error for corrupted base64 data, got name=%q, email=%q, plan=%q", name, email, plan)
	}
}
