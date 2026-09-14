package storage_test

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"tokenmonitor/config"
	"tokenmonitor/storage"

	_ "modernc.org/sqlite"
)

func TestBackup_CreateBackup_Integrity(t *testing.T) {
	tempDir := t.TempDir()
	t.Cleanup(func() {
		runtime.GC()
		time.Sleep(25 * time.Millisecond)
	})
	dbPath := filepath.Join(tempDir, "source.db")
	backupDest := filepath.Join(tempDir, "backups", "snapshot.db")

	cfg := &config.Config{
		Database: config.DatabaseConfig{
			SQLitePath:              dbPath,
			MaxOpenConns:            5,
			MaxIdleConns:            2,
			ConnMaxLifetimeMinutes: 10,
			EnableWALMode:           true,
			RollupIntervalSeconds:   300,
		},
		Backup: config.BackupConfig{
			Enabled:         true,
			IntervalMinutes: 60,
			BackupDir:       filepath.Join(tempDir, "backups"),
			MaxKeep:         3,
		},
	}

	store, err := storage.NewStorage(cfg)
	if err != nil {
		t.Fatalf("Failed to initialize Storage: %v", err)
	}
	defer store.Close()

	// Insert test telemetry record directly
	now := time.Now()
	_, err = store.DB.Exec(`
		INSERT INTO agent_fleet_telemetry 
		(account_id, subagent_id, role_name, task_name, status, started_at, finished_at, duration_ms, tokens_used, tokens_offloaded)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		1, "subagent-backup-test-1", "BackupTester", "RunSnapshotTest", "COMPLETED", now, now.Add(100*time.Millisecond), 100, 5000, 2000,
	)
	if err != nil {
		t.Fatalf("Failed to insert telemetry: %v", err)
	}

	// Create backup
	if err := os.MkdirAll(filepath.Dir(backupDest), 0755); err != nil {
		t.Fatalf("Failed to create backup dir: %v", err)
	}
	if err := store.CreateBackup(backupDest); err != nil {
		t.Fatalf("CreateBackup failed: %v", err)
	}

	// Verify backup file exists and has size
	info, err := os.Stat(backupDest)
	if err != nil {
		t.Fatalf("Backup file not found: %v", err)
	}
	if info.Size() == 0 {
		t.Fatalf("Backup file is empty")
	}

	// Verify no loose -wal or -shm was generated for backup
	if _, err := os.Stat(backupDest + "-wal"); !os.IsNotExist(err) {
		t.Errorf("Unexpected -wal file found next to backup")
	}
	if _, err := os.Stat(backupDest + "-shm"); !os.IsNotExist(err) {
		t.Errorf("Unexpected -shm file found next to backup")
	}

	// Open backup database directly and verify content
	bDB, err := sql.Open("sqlite", backupDest)
	if err != nil {
		t.Fatalf("Failed to open backup database: %v", err)
	}
	bDB.SetMaxIdleConns(0)
	defer bDB.Close()

	var count int
	row := bDB.QueryRow("SELECT COUNT(*) FROM agent_fleet_telemetry WHERE subagent_id = 'subagent-backup-test-1'")
	if err := row.Scan(&count); err != nil {
		t.Fatalf("Failed to query backup database: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 row in backup DB, got %d", count)
	}
}

func TestBackup_PerformAutoBackup_And_RetentionPrune(t *testing.T) {
	tempDir := t.TempDir()
	t.Cleanup(func() {
		runtime.GC()
		time.Sleep(25 * time.Millisecond)
	})
	dbPath := filepath.Join(tempDir, "source.db")
	backupDir := filepath.Join(tempDir, "backups")

	maxKeep := 3
	cfg := &config.Config{
		Database: config.DatabaseConfig{
			SQLitePath:              dbPath,
			MaxOpenConns:            5,
			MaxIdleConns:            2,
			ConnMaxLifetimeMinutes: 10,
			EnableWALMode:           true,
			RollupIntervalSeconds:   300,
		},
		Backup: config.BackupConfig{
			Enabled:         true,
			IntervalMinutes: 60,
			BackupDir:       backupDir,
			MaxKeep:         maxKeep,
		},
	}

	store, err := storage.NewStorage(cfg)
	if err != nil {
		t.Fatalf("Failed to initialize Storage: %v", err)
	}
	defer store.Close()

	// Simulate 5 backup files
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		t.Fatalf("Failed to create backup dir: %v", err)
	}
	for i := 1; i <= 5; i++ {
		fname := filepath.Join(backupDir, fmt.Sprintf("token_monitor_backup_2026090%d_120000.db", i))
		if err := store.CreateBackup(fname); err != nil {
			t.Fatalf("Failed to create mock backup file %s: %v", fname, err)
		}
	}

	// Verify 5 files exist
	entries, _ := os.ReadDir(backupDir)
	var initialCount int
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".db" && e.Name() != "token_monitor.db" {
			initialCount++
		}
	}
	if initialCount != 5 {
		t.Fatalf("Expected 5 initial backup files, got %d", initialCount)
	}

	// Run PruneOldBackups
	if err := store.PruneOldBackups(backupDir, maxKeep); err != nil {
		t.Fatalf("PruneOldBackups failed: %v", err)
	}

	// Verify only 3 files remain
	entries, _ = os.ReadDir(backupDir)
	var prunedCount int
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".db" && e.Name() != "token_monitor.db" {
			prunedCount++
		}
	}
	if prunedCount != maxKeep {
		t.Fatalf("Expected %d backup files after pruning, got %d", maxKeep, prunedCount)
	}

	// Verify the remaining 3 are the newest ones (03, 04, 05)
	for _, day := range []int{3, 4, 5} {
		expectedFile := filepath.Join(backupDir, fmt.Sprintf("token_monitor_backup_2026090%d_120000.db", day))
		if _, err := os.Stat(expectedFile); os.IsNotExist(err) {
			t.Errorf("Expected recent file %s to be kept, but it was deleted", expectedFile)
		}
	}

	// Also verify older ones (01, 02) were deleted
	for _, day := range []int{1, 2} {
		deletedFile := filepath.Join(backupDir, fmt.Sprintf("token_monitor_backup_2026090%d_120000.db", day))
		if _, err := os.Stat(deletedFile); !os.IsNotExist(err) {
			t.Errorf("Expected older file %s to be pruned, but it still exists", deletedFile)
		}
	}
}

func TestBackup_PerformAutoBackup_EndToEnd(t *testing.T) {
	tempDir := t.TempDir()
	t.Cleanup(func() {
		runtime.GC()
		time.Sleep(25 * time.Millisecond)
	})
	dbPath := filepath.Join(tempDir, "source.db")
	backupDir := filepath.Join(tempDir, "backups")

	cfg := &config.Config{
		Database: config.DatabaseConfig{
			SQLitePath:              dbPath,
			MaxOpenConns:            5,
			MaxIdleConns:            2,
			ConnMaxLifetimeMinutes: 10,
			EnableWALMode:           true,
			RollupIntervalSeconds:   300,
		},
		Backup: config.BackupConfig{
			Enabled:         true,
			IntervalMinutes: 60,
			BackupDir:       backupDir,
			MaxKeep:         2,
		},
	}

	store, err := storage.NewStorage(cfg)
	if err != nil {
		t.Fatalf("Failed to initialize Storage: %v", err)
	}
	defer store.Close()

	destPath, err := store.PerformAutoBackup()
	if err != nil {
		t.Fatalf("PerformAutoBackup failed: %v", err)
	}
	if destPath == "" {
		t.Fatalf("Expected non-empty backup path")
	}

	// Verify timestamp file and canonical token_monitor.db in backup
	if _, err := os.Stat(destPath); err != nil {
		t.Errorf("Timestamped backup not created: %v", err)
	}

	latestDB := filepath.Join(backupDir, "token_monitor.db")
	if _, err := os.Stat(latestDB); err != nil {
		t.Errorf("token_monitor.db not created in backup dir: %v", err)
	}
}

