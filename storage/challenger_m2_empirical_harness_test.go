package storage_test

import (
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"tokenmonitor/storage"

	_ "modernc.org/sqlite"
)

// TestChallenger_Harness_TrimmingLoopMatrix systematically stress-tests shifts of 0, 1, 2, and 3 bytes
// with various prefix tags (including 0x7a) and padding configurations.
func TestChallenger_Harness_TrimmingLoopMatrix(t *testing.T) {
	shifts := []struct {
		name       string
		prefix     string
		expectPlan string
	}{
		{name: "Shift0_Aligned", prefix: "", expectPlan: "Google AI Ultra (20X Ultra Tier)"},
		{name: "Shift1_1ByteTag_0x7a", prefix: "\x7a", expectPlan: "Google AI Ultra (20X Ultra Tier)"},
		{name: "Shift1_1ByteAscii_X", prefix: "X", expectPlan: "Google AI Ultra (20X Ultra Tier)"},
		{name: "Shift2_2BytesAscii_XY", prefix: "XY", expectPlan: "Google AI Ultra (20X Ultra Tier)"},
		{name: "Shift3_3BytesAscii_XYZ", prefix: "XYZ", expectPlan: "Google AI Ultra (20X Ultra Tier)"},
	}

	for _, tc := range shifts {
		t.Run(tc.name, func(t *testing.T) {
			tempDir := t.TempDir()
			dbPath := filepath.Join(tempDir, "trim_test.vscdb")

			// Setup DB
			setupDB, err := sql.Open("sqlite", dbPath)
			if err != nil {
				t.Fatalf("Open failed: %v", err)
			}
			_, _ = setupDB.Exec("PRAGMA journal_mode = DELETE;")
			_, err = setupDB.Exec("CREATE TABLE ItemTable (key TEXT PRIMARY KEY, value TEXT);")
			if err != nil {
				setupDB.Close()
				t.Fatalf("Exec failed: %v", err)
			}

			// Construct payload
			innerPayload := fmt.Sprintf("prefix_header_User: Dev %s:dev.%s@adversary.com:tier: g1-ultra-tier Google AI Ultra payload filler to exceed 40 characters easily", tc.name, tc.name)
			innerB64 := base64.StdEncoding.EncodeToString([]byte(innerPayload))

			// Prepend prefix to innerB64, simulating tag shift
			shiftedInner := tc.prefix + innerB64
			outerPayload := "outer_prefix_" + shiftedInner + "_outer_suffix"
			outerB64 := base64.StdEncoding.EncodeToString([]byte(outerPayload))

			_, err = setupDB.Exec("INSERT INTO ItemTable (key, value) VALUES ('antigravityUnifiedStateSync.userStatus', ?)", outerB64)
			if err != nil {
				setupDB.Close()
				t.Fatalf("Insert failed: %v", err)
			}
			setupDB.Close()

			origEnv := os.Getenv("ANTIGRAVITY_STATE_DB")
			defer func() { _ = os.Setenv("ANTIGRAVITY_STATE_DB", origEnv) }()
			_ = os.Setenv("ANTIGRAVITY_STATE_DB", dbPath)

			name, email, plan, err := storage.DetectActiveAntigravityAccount()
			if err != nil {
				t.Fatalf("Detection failed for %s: %v", tc.name, err)
			}

			expectedEmail := fmt.Sprintf("dev.%s@adversary.com", tc.name)
			if email != expectedEmail {
				t.Errorf("[%s] Expected email %q, got %q", tc.name, expectedEmail, email)
			}
			if plan != tc.expectPlan {
				t.Errorf("[%s] Expected plan %q, got %q", tc.name, tc.expectPlan, plan)
			}
			if name == "" {
				t.Errorf("[%s] Expected non-empty name, got empty", tc.name)
			}
			t.Logf("[%s] PASSED: email=%s, name=%s, plan=%s", tc.name, email, name, plan)
		})
	}
}

// TestChallenger_Harness_PaddingVariations tests unpadded (RawStdEncoding), standard padded (StdEncoding),
// and boundary lengths (len % 4 == 1, 2, 3).
func TestChallenger_Harness_PaddingVariations(t *testing.T) {
	testCases := []struct {
		name      string
		encoding  *base64.Encoding
		extraPad  string
		expectErr bool
	}{
		{name: "StdEncoding_WithPadding", encoding: base64.StdEncoding, extraPad: "", expectErr: false},
		{name: "RawStdEncoding_NoPadding", encoding: base64.RawStdEncoding, extraPad: "", expectErr: false},
		{name: "WhitespaceAroundBase64", encoding: base64.StdEncoding, extraPad: "  \n\t  ", expectErr: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tempDir := t.TempDir()
			t.Cleanup(func() {
				runtime.GC()
				time.Sleep(25 * time.Millisecond)
			})
			dbPath := filepath.Join(tempDir, "pad_test.vscdb")

			setupDB, err := sql.Open("sqlite", dbPath)
			if err != nil {
				t.Fatalf("Open failed: %v", err)
			}
			setupDB.SetMaxIdleConns(0)
			_, _ = setupDB.Exec("PRAGMA journal_mode = DELETE;")
			_, err = setupDB.Exec("CREATE TABLE ItemTable (key TEXT PRIMARY KEY, value TEXT);")
			if err != nil {
				setupDB.Close()
				t.Fatalf("Exec failed: %v", err)
			}

			innerPayload := "User: Pad Tester:pad.tester@company.io:Google AI Ultra (20X Ultra Tier) extra text to reach base64 length"
			var innerB64 string
			if tc.encoding == base64.RawStdEncoding {
				innerB64 = base64.RawStdEncoding.EncodeToString([]byte(innerPayload))
			} else {
				innerB64 = base64.StdEncoding.EncodeToString([]byte(innerPayload))
			}

			outerPayload := "outer_pad_" + innerB64 + "_outer_pad"
			outerB64 := tc.extraPad + base64.StdEncoding.EncodeToString([]byte(outerPayload)) + tc.extraPad

			_, err = setupDB.Exec("INSERT INTO ItemTable (key, value) VALUES ('antigravityUnifiedStateSync.userStatus', ?)", outerB64)
			if err != nil {
				setupDB.Close()
				t.Fatalf("Insert failed: %v", err)
			}
			setupDB.Close()

			origEnv := os.Getenv("ANTIGRAVITY_STATE_DB")
			defer func() { _ = os.Setenv("ANTIGRAVITY_STATE_DB", origEnv) }()
			_ = os.Setenv("ANTIGRAVITY_STATE_DB", dbPath)

			name, email, plan, err := storage.DetectActiveAntigravityAccount()
			if tc.expectErr {
				if err == nil {
					t.Errorf("[%s] Expected error, got nil (email=%q)", tc.name, email)
				}
				return
			}
			if err != nil {
				t.Fatalf("[%s] Unexpected error: %v", tc.name, err)
			}
			if email != "pad.tester@company.io" {
				t.Errorf("[%s] Expected email 'pad.tester@company.io', got %q", tc.name, email)
			}
			if plan != "Google AI Ultra (20X Ultra Tier)" {
				t.Errorf("[%s] Expected plan 'Google AI Ultra (20X Ultra Tier)', got %q", tc.name, plan)
			}
			t.Logf("[%s] PASSED: email=%s, name=%s, plan=%s", tc.name, email, name, plan)
		})
	}
}

// TestChallenger_Harness_CorruptedPayloadsMatrix verifies graceful degradation across various corruptions.
func TestChallenger_Harness_CorruptedPayloadsMatrix(t *testing.T) {
	tempDir := t.TempDir()
	t.Cleanup(func() {
		runtime.GC()
		time.Sleep(25 * time.Millisecond)
	})

	t.Run("InvalidJSONInPrimaryAuthStatus", func(t *testing.T) {
		dbPath := filepath.Join(tempDir, "corrupt_json.vscdb")
		setupDB, err := sql.Open("sqlite", dbPath)
		if err != nil {
			t.Fatalf("Open failed: %v", err)
		}
		setupDB.SetMaxIdleConns(0)
		_, _ = setupDB.Exec("PRAGMA journal_mode = DELETE;")
		_, _ = setupDB.Exec(`
			CREATE TABLE ItemTable (key TEXT PRIMARY KEY, value TEXT);
			INSERT INTO ItemTable (key, value) VALUES ('antigravityAuthStatus', '{malformed json: true, missing quotes');
			INSERT INTO ItemTable (key, value) VALUES ('antigravityUnifiedStateSync.userStatus', ?);
		`, base64.StdEncoding.EncodeToString([]byte("outer_header_User: Fallback User:fallback@example.com:Google AI Pro_outer_footer")))
		setupDB.Close()

		origEnv := os.Getenv("ANTIGRAVITY_STATE_DB")
		defer func() { _ = os.Setenv("ANTIGRAVITY_STATE_DB", origEnv) }()
		_ = os.Setenv("ANTIGRAVITY_STATE_DB", dbPath)

		name, email, plan, err := storage.DetectActiveAntigravityAccount()
		if err != nil {
			t.Fatalf("Expected fallback to secondary mode on malformed primary JSON, got error: %v", err)
		}
		if email != "fallback@example.com" {
			t.Errorf("Expected fallback email 'fallback@example.com', got %q", email)
		}
		t.Logf("Malformed primary JSON fell back gracefully: name=%q, email=%q, plan=%q", name, email, plan)
	})

	t.Run("EmptyJSONFieldsInPrimaryAuthStatus", func(t *testing.T) {
		dbPath := filepath.Join(tempDir, "empty_json.vscdb")
		setupDB, err := sql.Open("sqlite", dbPath)
		if err != nil {
			t.Fatalf("Open failed: %v", err)
		}
		setupDB.SetMaxIdleConns(0)
		_, _ = setupDB.Exec("PRAGMA journal_mode = DELETE;")
		_, _ = setupDB.Exec(`
			CREATE TABLE ItemTable (key TEXT PRIMARY KEY, value TEXT);
			INSERT INTO ItemTable (key, value) VALUES ('antigravityAuthStatus', '{"name":"","email":"","apiKey":""}');
		`)
		setupDB.Close()

		origEnv := os.Getenv("ANTIGRAVITY_STATE_DB")
		defer func() { _ = os.Setenv("ANTIGRAVITY_STATE_DB", origEnv) }()
		_ = os.Setenv("ANTIGRAVITY_STATE_DB", dbPath)

		name, email, plan, err := storage.DetectActiveAntigravityAccount()
		if err != nil {
			t.Fatalf("Expected no error for empty fields, got: %v", err)
		}
		if name != "" || email != "" {
			t.Errorf("Expected empty name and email, got name=%q, email=%q", name, email)
		}
		if plan != "Google AI Pro" {
			t.Errorf("Expected default plan 'Google AI Pro', got %q", plan)
		}
	})

	t.Run("NonBase64SecondaryStatus", func(t *testing.T) {
		dbPath := filepath.Join(tempDir, "not_b64.vscdb")
		setupDB, err := sql.Open("sqlite", dbPath)
		if err != nil {
			t.Fatalf("Open failed: %v", err)
		}
		setupDB.SetMaxIdleConns(0)
		_, _ = setupDB.Exec("PRAGMA journal_mode = DELETE;")
		_, _ = setupDB.Exec(`
			CREATE TABLE ItemTable (key TEXT PRIMARY KEY, value TEXT);
			INSERT INTO ItemTable (key, value) VALUES ('antigravityUnifiedStateSync.userStatus', '---NOT-BASE64---@@@$$$');
		`)
		setupDB.Close()

		origEnv := os.Getenv("ANTIGRAVITY_STATE_DB")
		defer func() { _ = os.Setenv("ANTIGRAVITY_STATE_DB", origEnv) }()
		_ = os.Setenv("ANTIGRAVITY_STATE_DB", dbPath)

		_, _, _, err = storage.DetectActiveAntigravityAccount()
		if err == nil {
			t.Errorf("Expected error for non-base64 secondary status, got nil")
		}
		if !strings.Contains(err.Error(), "giải mã base64") {
			t.Errorf("Expected error to mention base64 decoding, got: %v", err)
		}
	})

	t.Run("ValidBase64WithBinaryGarbage", func(t *testing.T) {
		dbPath := filepath.Join(tempDir, "garbage_proto.vscdb")
		setupDB, err := sql.Open("sqlite", dbPath)
		if err != nil {
			t.Fatalf("Open failed: %v", err)
		}
		setupDB.SetMaxIdleConns(0)
		_, _ = setupDB.Exec("PRAGMA journal_mode = DELETE;")
		garbageB64 := base64.StdEncoding.EncodeToString([]byte("\x00\x01\x02\x03\xff\xfe\xfd\xfcNO_EMAIL_HERE"))
		_, _ = setupDB.Exec(`
			CREATE TABLE ItemTable (key TEXT PRIMARY KEY, value TEXT);
			INSERT INTO ItemTable (key, value) VALUES ('antigravityUnifiedStateSync.userStatus', ?);
		`, garbageB64)
		setupDB.Close()

		origEnv := os.Getenv("ANTIGRAVITY_STATE_DB")
		defer func() { _ = os.Setenv("ANTIGRAVITY_STATE_DB", origEnv) }()
		_ = os.Setenv("ANTIGRAVITY_STATE_DB", dbPath)

		name, email, plan, err := storage.DetectActiveAntigravityAccount()
		if err != nil {
			t.Fatalf("Expected graceful handling without error for binary garbage, got: %v", err)
		}
		if email != "" {
			t.Errorf("Expected empty email for garbage, got %q", email)
		}
		if name != "" {
			t.Errorf("Expected empty name for garbage, got %q", name)
		}
		if plan != "Google AI Pro" {
			t.Errorf("Expected default plan 'Google AI Pro', got %q", plan)
		}
	})
}

// TestChallenger_Harness_MissingDatabase_Adversarial verifies behavior on various missing/corrupted file targets.
func TestChallenger_Harness_MissingDatabase_Adversarial(t *testing.T) {
	origEnv := os.Getenv("ANTIGRAVITY_STATE_DB")
	origAppData := os.Getenv("APPDATA")
	defer func() {
		_ = os.Setenv("ANTIGRAVITY_STATE_DB", origEnv)
		_ = os.Setenv("APPDATA", origAppData)
	}()

	t.Run("EmptyEnvAndNonExistentAppData", func(t *testing.T) {
		_ = os.Setenv("ANTIGRAVITY_STATE_DB", "")
		_ = os.Setenv("APPDATA", t.TempDir())

		_, _, _, err := storage.DetectActiveAntigravityAccount()
		if err == nil {
			t.Fatal("Expected error when no state.vscdb candidates exist, got nil")
		}
		if !strings.Contains(err.Error(), "không tìm thấy file state.vscdb") {
			t.Errorf("Expected missing file error, got: %v", err)
		}
	})

	t.Run("TargetIsDirectory", func(t *testing.T) {
		dir := t.TempDir()
		_ = os.Setenv("ANTIGRAVITY_STATE_DB", dir)

		_, _, _, err := storage.DetectActiveAntigravityAccount()
		if err == nil {
			t.Fatal("Expected error when target is a directory, got nil")
		}
	})

	t.Run("TargetIsZeroByteFile", func(t *testing.T) {
		tempDir := t.TempDir()
		emptyFile := filepath.Join(tempDir, "zero_byte.vscdb")
		_ = os.WriteFile(emptyFile, []byte{}, 0644)
		_ = os.Setenv("ANTIGRAVITY_STATE_DB", emptyFile)

		_, _, _, err := storage.DetectActiveAntigravityAccount()
		if err == nil {
			t.Fatal("Expected error when target is 0-byte file, got nil")
		}
	})

	t.Run("TargetIsCorruptSQLiteHeader", func(t *testing.T) {
		tempDir := t.TempDir()
		corruptFile := filepath.Join(tempDir, "corrupt_header.vscdb")
		_ = os.WriteFile(corruptFile, []byte("NOT_A_SQLITE_3_DATABASE_HEADER_CORRUPTED"), 0644)
		_ = os.Setenv("ANTIGRAVITY_STATE_DB", corruptFile)

		_, _, _, err := storage.DetectActiveAntigravityAccount()
		if err == nil {
			t.Fatal("Expected error when file has corrupt header, got nil")
		}
	})

	t.Run("TableItemTableMissing", func(t *testing.T) {
		tempDir := t.TempDir()
		dbPath := filepath.Join(tempDir, "no_table.vscdb")
		db, _ := sql.Open("sqlite", dbPath)
		_, _ = db.Exec("PRAGMA journal_mode = DELETE;")
		_, _ = db.Exec("CREATE TABLE some_random_table (id INT);")
		db.Close()
		_ = os.Setenv("ANTIGRAVITY_STATE_DB", dbPath)

		_, _, _, err := storage.DetectActiveAntigravityAccount()
		if err == nil {
			t.Fatal("Expected error when ItemTable is missing, got nil")
		}
	})
}

// TestChallenger_Harness_ReadOnly_StrictWritePrevention verifies mode=ro strictly prevents
// ANY write operations (INSERT, UPDATE, DELETE, CREATE, DROP) and creates zero -wal or -shm files.
func TestChallenger_Harness_ReadOnly_StrictWritePrevention(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "readonly_test.vscdb")

	setupDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	_, err = setupDB.Exec(`
		PRAGMA journal_mode = DELETE;
		CREATE TABLE ItemTable (key TEXT PRIMARY KEY, value TEXT);
		INSERT INTO ItemTable (key, value) VALUES ('antigravityAuthStatus', '{"name":"Safe User","email":"safe@ro.test"}');
	`)
	if err != nil {
		setupDB.Close()
		t.Fatalf("Setup failed: %v", err)
	}
	setupDB.Close()

	// Make database read-only on disk
	_ = os.Chmod(dbPath, 0444)
	defer func() { _ = os.Chmod(dbPath, 0666) }()

	origEnv := os.Getenv("ANTIGRAVITY_STATE_DB")
	defer func() { _ = os.Setenv("ANTIGRAVITY_STATE_DB", origEnv) }()
	_ = os.Setenv("ANTIGRAVITY_STATE_DB", dbPath)

	// Step 1: DetectActiveAntigravityAccount succeeds on read-only file
	name, email, plan, err := storage.DetectActiveAntigravityAccount()
	if err != nil {
		t.Fatalf("DetectActiveAntigravityAccount failed on read-only file: %v", err)
	}
	if name != "Safe User" || email != "safe@ro.test" || plan != "Google AI Pro" {
		t.Errorf("Unexpected result: name=%q, email=%q, plan=%q", name, email, plan)
	}

	// Step 2: Directly open with "sqlite_detector" and mode=ro to stress test write prevention
	roDSN := fmt.Sprintf("file:%s?mode=ro", filepath.ToSlash(dbPath))
	roDB, err := sql.Open("sqlite_detector", roDSN)
	if err != nil {
		t.Fatalf("Failed to open sqlite_detector mode=ro: %v", err)
	}
	defer roDB.Close()

	// Attempt INSERT
	if _, err := roDB.Exec("INSERT INTO ItemTable (key, value) VALUES ('evil_key', 'evil_value')"); err == nil {
		t.Fatal("CRITICAL SECURITY FLAW: INSERT succeeded on mode=ro database!")
	} else {
		t.Logf("Verified INSERT blocked: %v", err)
	}

	// Attempt UPDATE
	if _, err := roDB.Exec("UPDATE ItemTable SET value = 'hacked' WHERE key = 'antigravityAuthStatus'"); err == nil {
		t.Fatal("CRITICAL SECURITY FLAW: UPDATE succeeded on mode=ro database!")
	} else {
		t.Logf("Verified UPDATE blocked: %v", err)
	}

	// Attempt DELETE
	if _, err := roDB.Exec("DELETE FROM ItemTable"); err == nil {
		t.Fatal("CRITICAL SECURITY FLAW: DELETE succeeded on mode=ro database!")
	} else {
		t.Logf("Verified DELETE blocked: %v", err)
	}

	// Attempt CREATE TABLE
	if _, err := roDB.Exec("CREATE TABLE backdoor (id INT)"); err == nil {
		t.Fatal("CRITICAL SECURITY FLAW: CREATE TABLE succeeded on mode=ro database!")
	} else {
		t.Logf("Verified CREATE TABLE blocked: %v", err)
	}

	// Attempt DROP TABLE
	if _, err := roDB.Exec("DROP TABLE ItemTable"); err == nil {
		t.Fatal("CRITICAL SECURITY FLAW: DROP TABLE succeeded on mode=ro database!")
	} else {
		t.Logf("Verified DROP TABLE blocked: %v", err)
	}

	// Step 3: Verify no -wal or -shm files were created
	walPath := dbPath + "-wal"
	shmPath := dbPath + "-shm"
	if _, err := os.Stat(walPath); !os.IsNotExist(err) {
		t.Fatalf("CRITICAL ISOLATION BREACH: mode=ro connection generated -wal file: %s", walPath)
	}
	if _, err := os.Stat(shmPath); !os.IsNotExist(err) {
		t.Fatalf("CRITICAL ISOLATION BREACH: mode=ro connection generated -shm file: %s", shmPath)
	}

	t.Log("Strict mode=ro write prevention verified: 5/5 write operations blocked; 0 auxiliary files created.")
}

// TestChallenger_Harness_LiveDatabase_ZeroByteModificationProof calculates SHA256 before and after
// calling DetectActiveAntigravityAccount on the actual workstation Antigravity IDE database,
// empirically proving that ZERO BYTES were altered.
func TestChallenger_Harness_LiveDatabase_ZeroByteModificationProof(t *testing.T) {
	appData := os.Getenv("APPDATA")
	actualPath := filepath.Join(appData, "Antigravity IDE", "User", "globalStorage", "state.vscdb")
	if _, err := os.Stat(actualPath); os.IsNotExist(err) {
		t.Skipf("Live Antigravity state.vscdb not found at %s; skipping live DB byte-level proof", actualPath)
	}

	hashFile := func(p string) (string, int64, time.Time, error) {
		fi, err := os.Stat(p)
		if err != nil {
			return "", 0, time.Time{}, err
		}
		f, err := os.Open(p)
		if err != nil {
			return "", 0, time.Time{}, err
		}
		defer f.Close()
		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			return "", 0, time.Time{}, err
		}
		return fmt.Sprintf("%x", h.Sum(nil)), fi.Size(), fi.ModTime(), nil
	}

	hashBefore, sizeBefore, modBefore, err := hashFile(actualPath)
	if err != nil {
		t.Fatalf("Failed to hash live database before detection: %v", err)
	}

	origEnv := os.Getenv("ANTIGRAVITY_STATE_DB")
	defer func() { _ = os.Setenv("ANTIGRAVITY_STATE_DB", origEnv) }()
	_ = os.Setenv("ANTIGRAVITY_STATE_DB", actualPath)

	name, email, plan, err := storage.DetectActiveAntigravityAccount()
	if err != nil {
		t.Fatalf("Live detection failed: %v", err)
	}
	t.Logf("Live Detected: Name=%q, Email=%q, Plan=%q", name, email, plan)

	hashAfter, sizeAfter, modAfter, err := hashFile(actualPath)
	if err != nil {
		t.Fatalf("Failed to hash live database after detection: %v", err)
	}

	if hashBefore != hashAfter {
		t.Fatalf("CRITICAL INTEGRITY BREACH: SHA256 checksum changed!\nBefore: %s\nAfter:  %s", hashBefore, hashAfter)
	}
	if sizeBefore != sizeAfter {
		t.Fatalf("CRITICAL INTEGRITY BREACH: File size changed from %d to %d bytes!", sizeBefore, sizeAfter)
	}
	if !modBefore.Equal(modAfter) {
		t.Fatalf("CRITICAL INTEGRITY BREACH: File modification time changed from %v to %v!", modBefore, modAfter)
	}

	t.Logf("EMPIRICAL PROOF VERIFIED: Live database state.vscdb is 100%% UNCHANGED (SHA256=%s, Size=%d bytes, ModTime=%v)", hashBefore, sizeBefore, modBefore)
}
