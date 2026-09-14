package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"tokenmonitor/config"
)

// TestChallenger_LoadConfig_NonExistentFile tests LoadConfig with a non-existent file path
// Requirement 1: Test LoadConfig() with non-existent file: does it return valid default config without crashing?
func TestChallenger_LoadConfig_NonExistentFile(t *testing.T) {
	nonExistentPaths := []string{
		filepath.Join(os.TempDir(), "non_existent_config_123456789.yaml"),
		"does_not_exist_anywhere_in_this_repo.yaml",
		"./path/to/definitely/missing/file.yaml",
	}

	for _, path := range nonExistentPaths {
		t.Run(path, func(t *testing.T) {
			cfg, err := config.LoadConfig(path)
			if err != nil {
				t.Fatalf("LoadConfig(%q) returned error for non-existent file: %v", path, err)
			}
			if cfg == nil {
				t.Fatalf("LoadConfig(%q) returned nil config", path)
			}

			// Verify defaults for Server
			if cfg.Server.DashboardPort != 9090 {
				t.Errorf("Server.DashboardPort = %d, expected 9090", cfg.Server.DashboardPort)
			}
			if cfg.Server.BindAddress != "127.0.0.1" {
				t.Errorf("Server.BindAddress = %q, expected '127.0.0.1'", cfg.Server.BindAddress)
			}
			if cfg.Server.ReadTimeoutSeconds != 15 {
				t.Errorf("Server.ReadTimeoutSeconds = %d, expected 15", cfg.Server.ReadTimeoutSeconds)
			}
			if cfg.Server.WriteTimeoutSeconds != 30 {
				t.Errorf("Server.WriteTimeoutSeconds = %d, expected 30", cfg.Server.WriteTimeoutSeconds)
			}

			// Verify defaults for Proxy
			if cfg.Proxy.Enabled != false {
				t.Errorf("Proxy.Enabled = %v, expected false", cfg.Proxy.Enabled)
			}
			if cfg.Proxy.ListenPort != 8080 {
				t.Errorf("Proxy.ListenPort = %d, expected 8080", cfg.Proxy.ListenPort)
			}
			if cfg.Proxy.UpstreamTarget != "https://generativelanguage.googleapis.com" {
				t.Errorf("Proxy.UpstreamTarget = %q, expected 'https://generativelanguage.googleapis.com'", cfg.Proxy.UpstreamTarget)
			}
			if cfg.Proxy.MaxConcurrentRequests != 100 {
				t.Errorf("Proxy.MaxConcurrentRequests = %d, expected 100", cfg.Proxy.MaxConcurrentRequests)
			}
			if cfg.Proxy.UpstreamTimeoutSeconds != 180 {
				t.Errorf("Proxy.UpstreamTimeoutSeconds = %d, expected 180", cfg.Proxy.UpstreamTimeoutSeconds)
			}

			// Verify defaults for Database
			if cfg.Database.SQLitePath != "./data/token_monitor.db" {
				t.Errorf("Database.SQLitePath = %q, expected './data/token_monitor.db'", cfg.Database.SQLitePath)
			}
			if cfg.Database.MaxOpenConns != 25 {
				t.Errorf("Database.MaxOpenConns = %d, expected 25", cfg.Database.MaxOpenConns)
			}
			if cfg.Database.MaxIdleConns != 10 {
				t.Errorf("Database.MaxIdleConns = %d, expected 10", cfg.Database.MaxIdleConns)
			}
			if cfg.Database.ConnMaxLifetimeMinutes != 60 {
				t.Errorf("Database.ConnMaxLifetimeMinutes = %d, expected 60", cfg.Database.ConnMaxLifetimeMinutes)
			}
			if cfg.Database.EnableWALMode != true {
				t.Errorf("Database.EnableWALMode = %v, expected true", cfg.Database.EnableWALMode)
			}
			if cfg.Database.RollupIntervalSeconds != 300 {
				t.Errorf("Database.RollupIntervalSeconds = %d, expected 300", cfg.Database.RollupIntervalSeconds)
			}

			// Verify defaults for AccountProfile
			if cfg.AccountProfile.AccountType != "Google Consumer Account (Individual)" {
				t.Errorf("AccountProfile.AccountType = %q, expected 'Google Consumer Account (Individual)'", cfg.AccountProfile.AccountType)
			}
			if cfg.AccountProfile.PlanName != "20X ULTRA PLAN" {
				t.Errorf("AccountProfile.PlanName = %q, expected '20X ULTRA PLAN'", cfg.AccountProfile.PlanName)
			}
			if cfg.AccountProfile.QuotaBandwidth != "20x Quota Bandwidth" {
				t.Errorf("AccountProfile.QuotaBandwidth = %q, expected '20x Quota Bandwidth'", cfg.AccountProfile.QuotaBandwidth)
			}
			if cfg.AccountProfile.AutoRenew != true {
				t.Errorf("AccountProfile.AutoRenew = %v, expected true", cfg.AccountProfile.AutoRenew)
			}

			// Verify defaults for LocalTailer
			if cfg.LocalTailer.Enabled != true {
				t.Errorf("LocalTailer.Enabled = %v, expected true", cfg.LocalTailer.Enabled)
			}
			if cfg.LocalTailer.PollIntervalSeconds != 10 {
				t.Errorf("LocalTailer.PollIntervalSeconds = %d, expected 10", cfg.LocalTailer.PollIntervalSeconds)
			}

			// Verify defaults for Alerting
			if cfg.Alerting.Enabled != false {
				t.Errorf("Alerting.Enabled = %v, expected false", cfg.Alerting.Enabled)
			}
			if cfg.Alerting.DailyTokenThreshold != 50000000 {
				t.Errorf("Alerting.DailyTokenThreshold = %d, expected 50000000", cfg.Alerting.DailyTokenThreshold)
			}
			if cfg.Alerting.TokenExpiryWarningMinutes != 10 {
				t.Errorf("Alerting.TokenExpiryWarningMinutes = %d, expected 10", cfg.Alerting.TokenExpiryWarningMinutes)
			}
			if cfg.Alerting.Telegram.BotToken != "" {
				t.Errorf("Alerting.Telegram.BotToken = %q, expected empty", cfg.Alerting.Telegram.BotToken)
			}
			if cfg.Alerting.Telegram.ChatID != "" {
				t.Errorf("Alerting.Telegram.ChatID = %q, expected empty", cfg.Alerting.Telegram.ChatID)
			}
		})
	}
}

// TestChallenger_LoadConfig_PartialYAML tests LoadConfig with partial YAML configurations
// Requirement 2: Test LoadConfig() with partial YAML: do omitted fields take their fallback defaults?
func TestChallenger_LoadConfig_PartialYAML(t *testing.T) {
	tempDir := t.TempDir()

	tests := []struct {
		name     string
		yamlData string
		verify   func(t *testing.T, cfg *config.Config)
	}{
		{
			name:     "Empty YAML file",
			yamlData: "",
			verify: func(t *testing.T, cfg *config.Config) {
				if cfg.Server.DashboardPort != 9090 {
					t.Errorf("DashboardPort = %d, expected 9090", cfg.Server.DashboardPort)
				}
				if cfg.Database.MaxOpenConns != 25 {
					t.Errorf("MaxOpenConns = %d, expected 25", cfg.Database.MaxOpenConns)
				}
				if cfg.AccountProfile.PlanName != "20X ULTRA PLAN" {
					t.Errorf("PlanName = %q, expected '20X ULTRA PLAN'", cfg.AccountProfile.PlanName)
				}
				if cfg.LocalTailer.PollIntervalSeconds != 10 {
					t.Errorf("PollIntervalSeconds = %d, expected 10", cfg.LocalTailer.PollIntervalSeconds)
				}
				if cfg.Alerting.DailyTokenThreshold != 50000000 {
					t.Errorf("DailyTokenThreshold = %d, expected 50000000", cfg.Alerting.DailyTokenThreshold)
				}
			},
		},
		{
			name:     "Empty YAML object",
			yamlData: "{}\n",
			verify: func(t *testing.T, cfg *config.Config) {
				if cfg.Server.DashboardPort != 9090 {
					t.Errorf("DashboardPort = %d, expected 9090", cfg.Server.DashboardPort)
				}
				if cfg.Database.SQLitePath != "./data/token_monitor.db" {
					t.Errorf("SQLitePath = %q, expected default", cfg.Database.SQLitePath)
				}
			},
		},
		{
			name: "Only custom Server port specified",
			yamlData: `
server:
  dashboard_port: 8888
`,
			verify: func(t *testing.T, cfg *config.Config) {
				// Custom value must be respected
				if cfg.Server.DashboardPort != 8888 {
					t.Errorf("DashboardPort = %d, expected 8888", cfg.Server.DashboardPort)
				}
				// Other Server fields take defaults
				if cfg.Server.BindAddress != "127.0.0.1" {
					t.Errorf("BindAddress = %q, expected '127.0.0.1'", cfg.Server.BindAddress)
				}
				if cfg.Server.ReadTimeoutSeconds != 15 {
					t.Errorf("ReadTimeoutSeconds = %d, expected 15", cfg.Server.ReadTimeoutSeconds)
				}
				if cfg.Server.WriteTimeoutSeconds != 30 {
					t.Errorf("WriteTimeoutSeconds = %d, expected 30", cfg.Server.WriteTimeoutSeconds)
				}
				// Other blocks retain defaults
				if cfg.Database.MaxOpenConns != 25 {
					t.Errorf("Database.MaxOpenConns = %d, expected 25", cfg.Database.MaxOpenConns)
				}
				if cfg.LocalTailer.PollIntervalSeconds != 10 {
					t.Errorf("LocalTailer.PollIntervalSeconds = %d, expected 10", cfg.LocalTailer.PollIntervalSeconds)
				}
			},
		},
		{
			name: "Only custom AccountProfile Email specified",
			yamlData: `
account_profile:
  email: "adversary@testcorp.com"
`,
			verify: func(t *testing.T, cfg *config.Config) {
				if cfg.AccountProfile.Email != "adversary@testcorp.com" {
					t.Errorf("Email = %q, expected 'adversary@testcorp.com'", cfg.AccountProfile.Email)
				}
				if cfg.AccountProfile.AccountType != "Google Consumer Account (Individual)" {
					t.Errorf("AccountType = %q, expected default", cfg.AccountProfile.AccountType)
				}
				if cfg.AccountProfile.PlanName != "20X ULTRA PLAN" {
					t.Errorf("PlanName = %q, expected default", cfg.AccountProfile.PlanName)
				}
				if cfg.AccountProfile.QuotaBandwidth != "20x Quota Bandwidth" {
					t.Errorf("QuotaBandwidth = %q, expected default", cfg.AccountProfile.QuotaBandwidth)
				}
				if cfg.AccountProfile.AutoRenew != true {
					t.Errorf("AutoRenew = %v, expected true", cfg.AccountProfile.AutoRenew)
				}
			},
		},
		{
			name: "Zero and negative numeric values fallback to defaults",
			yamlData: `
server:
  dashboard_port: -9090
  bind_address: ""
  read_timeout_seconds: 0
  write_timeout_seconds: -5
proxy:
  listen_port: 0
  upstream_target: ""
  max_concurrent_requests: -10
  upstream_timeout_seconds: 0
database:
  sqlite_path: ""
  max_open_conns: 0
  max_idle_conns: -2
  conn_max_lifetime_minutes: 0
  rollup_interval_seconds: -100
local_tailer:
  poll_interval_seconds: 0
alerting:
  daily_token_threshold: -50
  token_expiry_warning_minutes: 0
`,
			verify: func(t *testing.T, cfg *config.Config) {
				if cfg.Server.DashboardPort != 9090 {
					t.Errorf("DashboardPort fallback failed: %d", cfg.Server.DashboardPort)
				}
				if cfg.Server.BindAddress != "127.0.0.1" {
					t.Errorf("BindAddress fallback failed: %q", cfg.Server.BindAddress)
				}
				if cfg.Server.ReadTimeoutSeconds != 15 {
					t.Errorf("ReadTimeoutSeconds fallback failed: %d", cfg.Server.ReadTimeoutSeconds)
				}
				if cfg.Server.WriteTimeoutSeconds != 30 {
					t.Errorf("WriteTimeoutSeconds fallback failed: %d", cfg.Server.WriteTimeoutSeconds)
				}
				if cfg.Proxy.ListenPort != 8080 {
					t.Errorf("Proxy ListenPort fallback failed: %d", cfg.Proxy.ListenPort)
				}
				if cfg.Proxy.UpstreamTarget != "https://generativelanguage.googleapis.com" {
					t.Errorf("Proxy UpstreamTarget fallback failed: %q", cfg.Proxy.UpstreamTarget)
				}
				if cfg.Proxy.MaxConcurrentRequests != 100 {
					t.Errorf("Proxy MaxConcurrentRequests fallback failed: %d", cfg.Proxy.MaxConcurrentRequests)
				}
				if cfg.Proxy.UpstreamTimeoutSeconds != 180 {
					t.Errorf("Proxy UpstreamTimeoutSeconds fallback failed: %d", cfg.Proxy.UpstreamTimeoutSeconds)
				}
				if cfg.Database.SQLitePath != "./data/token_monitor.db" {
					t.Errorf("Database SQLitePath fallback failed: %q", cfg.Database.SQLitePath)
				}
				if cfg.Database.MaxOpenConns != 25 {
					t.Errorf("Database MaxOpenConns fallback failed: %d", cfg.Database.MaxOpenConns)
				}
				if cfg.Database.MaxIdleConns != 10 {
					t.Errorf("Database MaxIdleConns fallback failed: %d", cfg.Database.MaxIdleConns)
				}
				if cfg.Database.ConnMaxLifetimeMinutes != 60 {
					t.Errorf("Database ConnMaxLifetimeMinutes fallback failed: %d", cfg.Database.ConnMaxLifetimeMinutes)
				}
				if cfg.Database.RollupIntervalSeconds != 300 {
					t.Errorf("Database RollupIntervalSeconds fallback failed: %d", cfg.Database.RollupIntervalSeconds)
				}
				if cfg.LocalTailer.PollIntervalSeconds != 10 {
					t.Errorf("LocalTailer PollIntervalSeconds fallback failed: %d", cfg.LocalTailer.PollIntervalSeconds)
				}
				if cfg.Alerting.DailyTokenThreshold != 50000000 {
					t.Errorf("Alerting DailyTokenThreshold fallback failed: %d", cfg.Alerting.DailyTokenThreshold)
				}
				if cfg.Alerting.TokenExpiryWarningMinutes != 10 {
					t.Errorf("Alerting TokenExpiryWarningMinutes fallback failed: %d", cfg.Alerting.TokenExpiryWarningMinutes)
				}
			},
		},
		{
			name: "Explicit boolean false values preserved without being overwritten",
			yamlData: `
proxy:
  enabled: false
database:
  enable_wal_mode: false
account_profile:
  auto_renew: false
local_tailer:
  enabled: false
alerting:
  enabled: false
`,
			verify: func(t *testing.T, cfg *config.Config) {
				if cfg.Proxy.Enabled != false {
					t.Errorf("Proxy.Enabled = %v, expected false", cfg.Proxy.Enabled)
				}
				if cfg.Database.EnableWALMode != false {
					t.Errorf("Database.EnableWALMode = %v, expected false", cfg.Database.EnableWALMode)
				}
				if cfg.AccountProfile.AutoRenew != false {
					t.Errorf("AccountProfile.AutoRenew = %v, expected false", cfg.AccountProfile.AutoRenew)
				}
				if cfg.LocalTailer.Enabled != false {
					t.Errorf("LocalTailer.Enabled = %v, expected false", cfg.LocalTailer.Enabled)
				}
				if cfg.Alerting.Enabled != false {
					t.Errorf("Alerting.Enabled = %v, expected false", cfg.Alerting.Enabled)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filePath := filepath.Join(tempDir, "test_"+time.Now().Format("150405.000000")+".yaml")
			if err := os.WriteFile(filePath, []byte(tt.yamlData), 0644); err != nil {
				t.Fatalf("Failed to write temporary test YAML: %v", err)
			}
			defer os.Remove(filePath)

			cfg, err := config.LoadConfig(filePath)
			if err != nil {
				t.Fatalf("LoadConfig failed unexpectedly: %v", err)
			}
			tt.verify(t, cfg)
		})
	}
}

// TestChallenger_LoadConfig_InvalidSyntax tests LoadConfig error handling on malformed YAML
func TestChallenger_LoadConfig_InvalidSyntax(t *testing.T) {
	tempDir := t.TempDir()
	malformedPath := filepath.Join(tempDir, "malformed.yaml")
	malformedContent := "server:\n  dashboard_port: [unclosed list\n  bad yaml syntax :::"

	if err := os.WriteFile(malformedPath, []byte(malformedContent), 0644); err != nil {
		t.Fatalf("Failed to write malformed YAML: %v", err)
	}
	defer os.Remove(malformedPath)

	cfg, err := config.LoadConfig(malformedPath)
	if err == nil {
		t.Fatal("Expected error on malformed YAML, got nil error")
	}
	if cfg != nil {
		t.Errorf("Expected nil config on malformed YAML error, got %+v", cfg)
	}
}

// TestChallenger_ParseSubscriptionTimes_EdgeCases tests ParseSubscriptionTimes across all edge cases
// Requirement 3: Test ParseSubscriptionTimes() across edge cases: valid date "2026-09-06", datetime "2026-09-06 14:30:00",
// RFC3339 "2026-09-06T14:30:00Z", invalid string "invalid-date", empty string "".
func TestChallenger_ParseSubscriptionTimes_EdgeCases(t *testing.T) {
	tests := []struct {
		name          string
		startInput    string
		expiryInput   string
		checkStart    func(t *testing.T, start time.Time)
		checkExpiry   func(t *testing.T, expiry time.Time)
		expectedError bool
	}{
		{
			name:        "1. Valid date format (YYYY-MM-DD)",
			startInput:  "2026-09-06",
			expiryInput: "2026-10-06",
			checkStart: func(t *testing.T, start time.Time) {
				expected := time.Date(2026, 9, 6, 0, 0, 0, 0, time.UTC)
				if !start.Equal(expected) {
					t.Errorf("start = %v, expected %v", start, expected)
				}
			},
			checkExpiry: func(t *testing.T, expiry time.Time) {
				expected := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)
				if !expiry.Equal(expected) {
					t.Errorf("expiry = %v, expected %v", expiry, expected)
				}
			},
		},
		{
			name:        "2. Datetime format (YYYY-MM-DD HH:MM:SS)",
			startInput:  "2026-09-06 14:30:00",
			expiryInput: "2026-10-06 18:45:12",
			checkStart: func(t *testing.T, start time.Time) {
				expected := time.Date(2026, 9, 6, 14, 30, 0, 0, time.UTC)
				if !start.Equal(expected) {
					t.Errorf("start = %v, expected %v", start, expected)
				}
			},
			checkExpiry: func(t *testing.T, expiry time.Time) {
				expected := time.Date(2026, 10, 6, 18, 45, 12, 0, time.UTC)
				if !expiry.Equal(expected) {
					t.Errorf("expiry = %v, expected %v", expiry, expected)
				}
			},
		},
		{
			name:        "3. RFC3339 format (YYYY-MM-DDTHH:MM:SSZ)",
			startInput:  "2026-09-06T14:30:00Z",
			expiryInput: "2026-10-06T14:30:00Z",
			checkStart: func(t *testing.T, start time.Time) {
				expected := time.Date(2026, 9, 6, 14, 30, 0, 0, time.UTC)
				if !start.Equal(expected) {
					t.Errorf("start = %v, expected %v", start, expected)
				}
			},
			checkExpiry: func(t *testing.T, expiry time.Time) {
				expected := time.Date(2026, 10, 6, 14, 30, 0, 0, time.UTC)
				if !expiry.Equal(expected) {
					t.Errorf("expiry = %v, expected %v", expiry, expected)
				}
			},
		},
		{
			name:        "4. Invalid string ('invalid-date')",
			startInput:  "invalid-date",
			expiryInput: "invalid-date",
			checkStart: func(t *testing.T, start time.Time) {
				// Must fallback to approximate time.Now()
				diff := time.Since(start)
				if diff < -time.Second || diff > 5*time.Second {
					t.Errorf("start fallback %v is too far from now (%v)", start, diff)
				}
			},
			checkExpiry: func(t *testing.T, expiry time.Time) {
				// Must fallback to approximate time.Now().AddDate(0, 1, 0)
				expectedApprox := time.Now().AddDate(0, 1, 0)
				diff := expiry.Sub(expectedApprox)
				if diff < -5*time.Second || diff > 5*time.Second {
					t.Errorf("expiry fallback %v is too far from expected +1 month (%v)", expiry, diff)
				}
			},
		},
		{
			name:        "5. Empty string ('')",
			startInput:  "",
			expiryInput: "",
			checkStart: func(t *testing.T, start time.Time) {
				diff := time.Since(start)
				if diff < -time.Second || diff > 5*time.Second {
					t.Errorf("start fallback %v is too far from now (%v)", start, diff)
				}
			},
			checkExpiry: func(t *testing.T, expiry time.Time) {
				expectedApprox := time.Now().AddDate(0, 1, 0)
				diff := expiry.Sub(expectedApprox)
				if diff < -5*time.Second || diff > 5*time.Second {
					t.Errorf("expiry fallback %v is too far from expected +1 month (%v)", expiry, diff)
				}
			},
		},
		{
			name:        "6. Slash formats (YYYY/MM/DD and YYYY/MM/DD HH:MM:SS)",
			startInput:  "2026/09/06",
			expiryInput: "2026/10/06 23:59:59",
			checkStart: func(t *testing.T, start time.Time) {
				expected := time.Date(2026, 9, 6, 0, 0, 0, 0, time.UTC)
				if !start.Equal(expected) {
					t.Errorf("start = %v, expected %v", start, expected)
				}
			},
			checkExpiry: func(t *testing.T, expiry time.Time) {
				expected := time.Date(2026, 10, 6, 23, 59, 59, 0, time.UTC)
				if !expiry.Equal(expected) {
					t.Errorf("expiry = %v, expected %v", expiry, expected)
				}
			},
		},
		{
			name:        "7. RFC3339 with timezone offset (+07:00)",
			startInput:  "2026-09-06T14:30:00+07:00",
			expiryInput: "2026-10-06T14:30:00+07:00",
			checkStart: func(t *testing.T, start time.Time) {
				expected := time.Date(2026, 9, 6, 7, 30, 0, 0, time.UTC)
				if !start.Equal(expected) {
					t.Errorf("start = %v, expected %v", start.UTC(), expected)
				}
			},
			checkExpiry: func(t *testing.T, expiry time.Time) {
				expected := time.Date(2026, 10, 6, 7, 30, 0, 0, time.UTC)
				if !expiry.Equal(expected) {
					t.Errorf("expiry = %v, expected %v", expiry.UTC(), expected)
				}
			},
		},
		{
			name:        "8. RFC3339Nano format",
			startInput:  "2026-09-06T14:30:00.123456789Z",
			expiryInput: "2026-10-06T14:30:00.987654321Z",
			checkStart: func(t *testing.T, start time.Time) {
				expected := time.Date(2026, 9, 6, 14, 30, 0, 123456789, time.UTC)
				if !start.Equal(expected) {
					t.Errorf("start = %v, expected %v", start, expected)
				}
			},
			checkExpiry: func(t *testing.T, expiry time.Time) {
				expected := time.Date(2026, 10, 6, 14, 30, 0, 987654321, time.UTC)
				if !expiry.Equal(expected) {
					t.Errorf("expiry = %v, expected %v", expiry, expected)
				}
			},
		},
		{
			name:        "9. Whitespace padding trimmed before parse",
			startInput:  "  2026-09-06  ",
			expiryInput: "  2026-10-06 12:00:00  ",
			checkStart: func(t *testing.T, start time.Time) {
				expected := time.Date(2026, 9, 6, 0, 0, 0, 0, time.UTC)
				if !start.Equal(expected) {
					t.Errorf("start = %v, expected %v", start, expected)
				}
			},
			checkExpiry: func(t *testing.T, expiry time.Time) {
				expected := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
				if !expiry.Equal(expected) {
					t.Errorf("expiry = %v, expected %v", expiry, expected)
				}
			},
		},
		{
			name:        "10. Mixed: valid start with invalid expiry",
			startInput:  "2026-09-06",
			expiryInput: "garbage-expiry-string",
			checkStart: func(t *testing.T, start time.Time) {
				expected := time.Date(2026, 9, 6, 0, 0, 0, 0, time.UTC)
				if !start.Equal(expected) {
					t.Errorf("start = %v, expected %v", start, expected)
				}
			},
			checkExpiry: func(t *testing.T, expiry time.Time) {
				expectedApprox := time.Now().AddDate(0, 1, 0)
				diff := expiry.Sub(expectedApprox)
				if diff < -5*time.Second || diff > 5*time.Second {
					t.Errorf("expiry fallback %v is too far from expected +1 month (%v)", expiry, diff)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				AccountProfile: config.AccountProfileConfig{
					SubscriptionStart:  tt.startInput,
					SubscriptionExpiry: tt.expiryInput,
				},
			}

			start, expiry, err := cfg.ParseSubscriptionTimes()
			if tt.expectedError && err == nil {
				t.Fatalf("Expected error, got nil")
			}
			if !tt.expectedError && err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if tt.checkStart != nil {
				tt.checkStart(t, start)
			}
			if tt.checkExpiry != nil {
				tt.checkExpiry(t, expiry)
			}
		})
	}
}

// TestChallenger_Parity_With_RealConfigYAML tests the actual config.yaml from root
func TestChallenger_Parity_With_RealConfigYAML(t *testing.T) {
	realConfigPath := "../config.yaml"
	if _, err := os.Stat(realConfigPath); err != nil {
		t.Skipf("Root config.yaml not found at %s", realConfigPath)
	}

	cfg, err := config.LoadConfig(realConfigPath)
	if err != nil {
		t.Fatalf("Failed to load root config.yaml: %v", err)
	}

	// Verify all 6 blocks
	if cfg.Server.DashboardPort != 9090 {
		t.Errorf("Server.DashboardPort = %d, expected 9090", cfg.Server.DashboardPort)
	}
	if cfg.Proxy.ListenPort != 8080 {
		t.Errorf("Proxy.ListenPort = %d, expected 8080", cfg.Proxy.ListenPort)
	}
	if cfg.Database.SQLitePath != "./data/token_monitor.db" {
		t.Errorf("Database.SQLitePath = %q, expected './data/token_monitor.db'", cfg.Database.SQLitePath)
	}
	if cfg.AccountProfile.Email != "ethanpham671986@gmail.com" {
		t.Errorf("AccountProfile.Email = %q", cfg.AccountProfile.Email)
	}
	if cfg.LocalTailer.PollIntervalSeconds != 10 {
		t.Errorf("LocalTailer.PollIntervalSeconds = %d, expected 10", cfg.LocalTailer.PollIntervalSeconds)
	}
	// Verify telegram sub-block exists and is typed
	if cfg.Alerting.Telegram.BotToken != "" || cfg.Alerting.Telegram.ChatID != "" {
		t.Logf("Alerting.Telegram parsed correctly: %+v", cfg.Alerting.Telegram)
	}
}
