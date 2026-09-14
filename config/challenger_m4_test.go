package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"tokenmonitor/config"
)

// TestChallengerM4_Config_CorruptYAMLSyntax tests that LoadConfig cleanly returns
// an error on malformed YAML syntax without panicking.
func TestChallengerM4_Config_CorruptYAMLSyntax(t *testing.T) {
	tmpDir := t.TempDir()
	badYamlFile := filepath.Join(tmpDir, "syntax_error.yaml")

	badContent := `
server:
  dashboard_port: [unclosed_list
  bind_address: "127.0.0.1
`
	if err := os.WriteFile(badYamlFile, []byte(badContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	cfg, err := config.LoadConfig(badYamlFile)
	if err == nil {
		t.Fatal("Expected error on invalid YAML syntax, got nil")
	}
	if cfg != nil {
		t.Error("Expected nil config on syntax error, got non-nil")
	}
	t.Logf("Correctly rejected malformed YAML: %v", err)
}

// TestChallengerM4_Config_CorruptAndBoundaryValues tests that negative or zero values
// in configuration files are safely sanitized to default values.
func TestChallengerM4_Config_CorruptAndBoundaryValues(t *testing.T) {
	tmpDir := t.TempDir()
	boundaryYamlFile := filepath.Join(tmpDir, "boundary.yaml")

	boundaryContent := `
server:
  dashboard_port: -8080
  bind_address: ""
  read_timeout_seconds: -10
  write_timeout_seconds: 0
proxy:
  listen_port: -1
  upstream_target: ""
  max_concurrent_requests: -100
  upstream_timeout_seconds: 0
database:
  sqlite_path: ""
  max_open_conns: -5
  max_idle_conns: 0
  conn_max_lifetime_minutes: -1
  rollup_interval_seconds: -300
account_profile:
  account_type: ""
  plan_name: ""
  quota_bandwidth: ""
local_tailer:
  poll_interval_seconds: -1
alerting:
  daily_token_threshold: -500
  token_expiry_warning_minutes: 0
`
	if err := os.WriteFile(boundaryYamlFile, []byte(boundaryContent), 0644); err != nil {
		t.Fatalf("Failed to create boundary file: %v", err)
	}

	cfg, err := config.LoadConfig(boundaryYamlFile)
	if err != nil {
		t.Fatalf("LoadConfig returned unexpected error: %v", err)
	}

	// Verify defaults applied
	if cfg.Server.DashboardPort != 9090 {
		t.Errorf("Expected DashboardPort 9090, got %d", cfg.Server.DashboardPort)
	}
	if cfg.Server.BindAddress != "127.0.0.1" {
		t.Errorf("Expected BindAddress '127.0.0.1', got %s", cfg.Server.BindAddress)
	}
	if cfg.Proxy.ListenPort != 8080 {
		t.Errorf("Expected Proxy.ListenPort 8080, got %d", cfg.Proxy.ListenPort)
	}
	if cfg.Database.SQLitePath != "./data/token_monitor.db" {
		t.Errorf("Expected SQLitePath default, got %s", cfg.Database.SQLitePath)
	}
	if cfg.Database.MaxOpenConns != 25 {
		t.Errorf("Expected MaxOpenConns 25, got %d", cfg.Database.MaxOpenConns)
	}
	if cfg.Database.RollupIntervalSeconds != 300 {
		t.Errorf("Expected RollupIntervalSeconds 300, got %d", cfg.Database.RollupIntervalSeconds)
	}
	if cfg.LocalTailer.PollIntervalSeconds != 10 {
		t.Errorf("Expected PollIntervalSeconds 10, got %d", cfg.LocalTailer.PollIntervalSeconds)
	}
}

// TestChallengerM4_Config_SubscriptionDate_AdversarialInputs tests parsing resilience
// when date strings are corrupted, invalid, or formatted unexpectedly.
func TestChallengerM4_Config_SubscriptionDate_AdversarialInputs(t *testing.T) {
	testCases := []struct {
		name        string
		startStr    string
		expiryStr   string
		shouldPanic bool
	}{
		{"Completely random string", "not_a_date_at_all", "gibberish_12345", false},
		{"Out of range month/day", "2026-99-99", "2026-13-45", false},
		{"Empty strings", "", "", false},
		{"SQL injection string", "' OR '1'='1", "DROP TABLE accounts;", false},
		{"Unix timestamp string", "1725800000", "1728400000", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{
				AccountProfile: config.AccountProfileConfig{
					SubscriptionStart:  tc.startStr,
					SubscriptionExpiry: tc.expiryStr,
				},
			}

			start, expiry, err := cfg.ParseSubscriptionTimes()
			if err != nil {
				t.Errorf("ParseSubscriptionTimes returned unexpected error: %v", err)
			}
			if start.IsZero() {
				t.Errorf("Expected non-zero start time for input %q", tc.startStr)
			}
			if expiry.IsZero() {
				t.Errorf("Expected non-zero expiry time for input %q", tc.expiryStr)
			}
			if !expiry.After(start) {
				t.Errorf("Expected expiry (%v) to be after start (%v) for input %q", expiry, start, tc.expiryStr)
			}
		})
	}
}

// TestChallengerM4_Config_EmptyYAMLDocument tests loading an empty YAML file.
func TestChallengerM4_Config_EmptyYAMLDocument(t *testing.T) {
	tmpDir := t.TempDir()
	emptyFile := filepath.Join(tmpDir, "empty.yaml")
	if err := os.WriteFile(emptyFile, []byte("{}"), 0644); err != nil {
		t.Fatalf("Failed to create empty YAML file: %v", err)
	}

	cfg, err := config.LoadConfig(emptyFile)
	if err != nil {
		t.Fatalf("LoadConfig failed on empty YAML document: %v", err)
	}

	if cfg.Server.DashboardPort != 9090 {
		t.Errorf("Expected default DashboardPort 9090, got %d", cfg.Server.DashboardPort)
	}
	if cfg.AccountProfile.PlanName != "20X ULTRA PLAN" {
		t.Errorf("Expected default PlanName, got %s", cfg.AccountProfile.PlanName)
	}
}
