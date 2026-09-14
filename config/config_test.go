package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tokenmonitor/config"
)

// TestConfig_NewDefaultConfig asserts all default values across all 6 blocks
func TestConfig_NewDefaultConfig(t *testing.T) {
	cfg := config.NewDefaultConfig()
	if cfg == nil {
		t.Fatal("NewDefaultConfig returned nil")
	}

	// 1. ServerConfig
	if cfg.Server.DashboardPort != 9090 {
		t.Errorf("Expected Server.DashboardPort=9090, got %d", cfg.Server.DashboardPort)
	}
	if cfg.Server.BindAddress != "127.0.0.1" {
		t.Errorf("Expected Server.BindAddress='127.0.0.1', got %q", cfg.Server.BindAddress)
	}
	if cfg.Server.ReadTimeoutSeconds != 15 {
		t.Errorf("Expected Server.ReadTimeoutSeconds=15, got %d", cfg.Server.ReadTimeoutSeconds)
	}
	if cfg.Server.WriteTimeoutSeconds != 30 {
		t.Errorf("Expected Server.WriteTimeoutSeconds=30, got %d", cfg.Server.WriteTimeoutSeconds)
	}

	// 2. ProxyConfig
	if cfg.Proxy.Enabled {
		t.Errorf("Expected Proxy.Enabled=false, got true")
	}
	if cfg.Proxy.ListenPort != 8080 {
		t.Errorf("Expected Proxy.ListenPort=8080, got %d", cfg.Proxy.ListenPort)
	}
	if cfg.Proxy.UpstreamTarget != "https://generativelanguage.googleapis.com" {
		t.Errorf("Expected Proxy.UpstreamTarget='https://generativelanguage.googleapis.com', got %q", cfg.Proxy.UpstreamTarget)
	}
	if cfg.Proxy.MaxConcurrentRequests != 100 {
		t.Errorf("Expected Proxy.MaxConcurrentRequests=100, got %d", cfg.Proxy.MaxConcurrentRequests)
	}
	if cfg.Proxy.UpstreamTimeoutSeconds != 180 {
		t.Errorf("Expected Proxy.UpstreamTimeoutSeconds=180, got %d", cfg.Proxy.UpstreamTimeoutSeconds)
	}

	// 3. DatabaseConfig
	if cfg.Database.SQLitePath != "./data/token_monitor.db" {
		t.Errorf("Expected Database.SQLitePath='./data/token_monitor.db', got %q", cfg.Database.SQLitePath)
	}
	if cfg.Database.MaxOpenConns != 25 {
		t.Errorf("Expected Database.MaxOpenConns=25, got %d", cfg.Database.MaxOpenConns)
	}
	if cfg.Database.MaxIdleConns != 10 {
		t.Errorf("Expected Database.MaxIdleConns=10, got %d", cfg.Database.MaxIdleConns)
	}
	if cfg.Database.ConnMaxLifetimeMinutes != 60 {
		t.Errorf("Expected Database.ConnMaxLifetimeMinutes=60, got %d", cfg.Database.ConnMaxLifetimeMinutes)
	}
	if !cfg.Database.EnableWALMode {
		t.Errorf("Expected Database.EnableWALMode=true, got false")
	}
	if cfg.Database.RollupIntervalSeconds != 300 {
		t.Errorf("Expected Database.RollupIntervalSeconds=300, got %d", cfg.Database.RollupIntervalSeconds)
	}

	// 4. AccountProfileConfig
	if cfg.AccountProfile.AccountType != "Google Consumer Account (Individual)" {
		t.Errorf("Expected AccountProfile.AccountType='Google Consumer Account (Individual)', got %q", cfg.AccountProfile.AccountType)
	}
	if cfg.AccountProfile.PlanName != "20X ULTRA PLAN" {
		t.Errorf("Expected AccountProfile.PlanName='20X ULTRA PLAN', got %q", cfg.AccountProfile.PlanName)
	}
	if cfg.AccountProfile.QuotaBandwidth != "20x Quota Bandwidth" {
		t.Errorf("Expected AccountProfile.QuotaBandwidth='20x Quota Bandwidth', got %q", cfg.AccountProfile.QuotaBandwidth)
	}
	if !cfg.AccountProfile.AutoRenew {
		t.Errorf("Expected AccountProfile.AutoRenew=true, got false")
	}

	// 5. LocalTailerConfig
	if !cfg.LocalTailer.Enabled {
		t.Errorf("Expected LocalTailer.Enabled=true, got false")
	}
	if cfg.LocalTailer.PollIntervalSeconds != 10 {
		t.Errorf("Expected LocalTailer.PollIntervalSeconds=10, got %d", cfg.LocalTailer.PollIntervalSeconds)
	}

	// 6. AlertingConfig
	if cfg.Alerting.Enabled {
		t.Errorf("Expected Alerting.Enabled=false, got true")
	}
	if cfg.Alerting.DailyTokenThreshold != 50000000 {
		t.Errorf("Expected Alerting.DailyTokenThreshold=50000000, got %d", cfg.Alerting.DailyTokenThreshold)
	}
	if cfg.Alerting.TokenExpiryWarningMinutes != 10 {
		t.Errorf("Expected Alerting.TokenExpiryWarningMinutes=10, got %d", cfg.Alerting.TokenExpiryWarningMinutes)
	}
	if cfg.Alerting.Telegram.BotToken != "" || cfg.Alerting.Telegram.ChatID != "" {
		t.Errorf("Expected Alerting.Telegram to be empty, got %+v", cfg.Alerting.Telegram)
	}
}

// TestConfig_LoadConfig_Comprehensive tests normal file load, missing file fallback,
// partial YAML with invalid/non-positive inputs, and malformed syntax.
func TestConfig_LoadConfig_Comprehensive(t *testing.T) {
	tempDir := t.TempDir()

	// Sub-test 1: Normal valid config
	t.Run("NormalValidFile", func(t *testing.T) {
		validPath := filepath.Join(tempDir, "valid_config.yaml")
		content := `
server:
  dashboard_port: 8000
  bind_address: "0.0.0.0"
  read_timeout_seconds: 20
  write_timeout_seconds: 40
proxy:
  enabled: true
  listen_port: 8081
  upstream_target: "https://custom.target.com"
  max_concurrent_requests: 200
  upstream_timeout_seconds: 300
database:
  sqlite_path: "./custom/db.sqlite"
  max_open_conns: 50
  max_idle_conns: 20
  conn_max_lifetime_minutes: 120
  enable_wal_mode: false
  rollup_interval_seconds: 600
account_profile:
  email: "custom@example.com"
  account_type: "Workspace Enterprise"
  plan_name: "Enterprise Plus"
  quota_bandwidth: "100x"
  subscription_start: "2026-01-01"
  subscription_expiry: "2026-12-31"
  auto_renew: false
local_tailer:
  enabled: false
  poll_interval_seconds: 30
alerting:
  enabled: true
  daily_token_threshold: 100000000
  token_expiry_warning_minutes: 30
  telegram:
    bot_token: "token123"
    chat_id: "chat456"
`
		if err := os.WriteFile(validPath, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to write valid config: %v", err)
		}

		cfg, err := config.LoadConfig(validPath)
		if err != nil {
			t.Fatalf("LoadConfig failed on valid config: %v", err)
		}

		if cfg.Server.DashboardPort != 8000 || cfg.Server.BindAddress != "0.0.0.0" {
			t.Errorf("Server mismatch: %+v", cfg.Server)
		}
		if !cfg.Proxy.Enabled || cfg.Proxy.ListenPort != 8081 || cfg.Proxy.UpstreamTarget != "https://custom.target.com" {
			t.Errorf("Proxy mismatch: %+v", cfg.Proxy)
		}
		if cfg.Database.SQLitePath != "./custom/db.sqlite" || cfg.Database.EnableWALMode != false {
			t.Errorf("Database mismatch: %+v", cfg.Database)
		}
		if cfg.AccountProfile.Email != "custom@example.com" || cfg.AccountProfile.AutoRenew != false {
			t.Errorf("AccountProfile mismatch: %+v", cfg.AccountProfile)
		}
		if cfg.LocalTailer.Enabled != false || cfg.LocalTailer.PollIntervalSeconds != 30 {
			t.Errorf("LocalTailer mismatch: %+v", cfg.LocalTailer)
		}
		if !cfg.Alerting.Enabled || cfg.Alerting.DailyTokenThreshold != 100000000 || cfg.Alerting.Telegram.BotToken != "token123" {
			t.Errorf("Alerting mismatch: %+v", cfg.Alerting)
		}
	})

	// Sub-test 2: Missing file fallback
	t.Run("MissingFileFallback", func(t *testing.T) {
		missingPath := filepath.Join(tempDir, "does_not_exist.yaml")
		cfg, err := config.LoadConfig(missingPath)
		if err != nil {
			t.Fatalf("LoadConfig on missing file should return nil error, got: %v", err)
		}
		if cfg == nil {
			t.Fatal("Expected default config on missing file, got nil")
		}
		if cfg.Server.DashboardPort != 9090 {
			t.Errorf("Expected fallback DashboardPort 9090, got %d", cfg.Server.DashboardPort)
		}
	})

	// Sub-test 3: Partial YAML and non-positive value overrides
	t.Run("PartialAndInvalidInputsFallback", func(t *testing.T) {
		partialPath := filepath.Join(tempDir, "partial_config.yaml")
		content := `
server:
  dashboard_port: -1
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
  max_idle_conns: -1
  conn_max_lifetime_minutes: 0
  rollup_interval_seconds: -10
account_profile:
  account_type: ""
  plan_name: ""
  quota_bandwidth: ""
local_tailer:
  poll_interval_seconds: 0
alerting:
  daily_token_threshold: -1
  token_expiry_warning_minutes: 0
`
		if err := os.WriteFile(partialPath, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to write partial config: %v", err)
		}

		cfg, err := config.LoadConfig(partialPath)
		if err != nil {
			t.Fatalf("LoadConfig failed on partial config: %v", err)
		}

		// Verify non-positive and empty values fallback to healthy defaults
		if cfg.Server.DashboardPort != 9090 {
			t.Errorf("Expected Server.DashboardPort=9090 fallback, got %d", cfg.Server.DashboardPort)
		}
		if cfg.Server.BindAddress != "127.0.0.1" {
			t.Errorf("Expected Server.BindAddress='127.0.0.1' fallback, got %q", cfg.Server.BindAddress)
		}
		if cfg.Proxy.ListenPort != 8080 {
			t.Errorf("Expected Proxy.ListenPort=8080 fallback, got %d", cfg.Proxy.ListenPort)
		}
		if cfg.Database.SQLitePath != "./data/token_monitor.db" {
			t.Errorf("Expected Database.SQLitePath fallback, got %q", cfg.Database.SQLitePath)
		}
		if cfg.AccountProfile.PlanName != "20X ULTRA PLAN" {
			t.Errorf("Expected AccountProfile.PlanName fallback, got %q", cfg.AccountProfile.PlanName)
		}
		if cfg.LocalTailer.PollIntervalSeconds != 10 {
			t.Errorf("Expected LocalTailer.PollIntervalSeconds fallback, got %d", cfg.LocalTailer.PollIntervalSeconds)
		}
	})

	// Sub-test 4: Malformed YAML
	t.Run("MalformedYAML", func(t *testing.T) {
		corruptPath := filepath.Join(tempDir, "corrupt.yaml")
		if err := os.WriteFile(corruptPath, []byte("server: [unclosed"), 0644); err != nil {
			t.Fatalf("Failed to write corrupt config: %v", err)
		}

		_, err := config.LoadConfig(corruptPath)
		if err == nil {
			t.Fatal("Expected error on malformed YAML, got nil")
		}
		if !strings.Contains(err.Error(), "không thể parse yaml") {
			t.Errorf("Expected error to mention YAML parse failure, got: %v", err)
		}
	})
}

// TestConfig_ParseSubscriptionTimes tests all supported date layouts and fallback behavior
func TestConfig_ParseSubscriptionTimes(t *testing.T) {
	testCases := []struct {
		name        string
		startStr    string
		expiryStr   string
		expectStart func(t *testing.T, st time.Time)
		expectExp   func(t *testing.T, exp time.Time)
	}{
		{
			name:      "Layout 2006-01-02",
			startStr:  "2026-09-08",
			expiryStr: "2026-10-08",
			expectStart: func(t *testing.T, st time.Time) {
				if st.Year() != 2026 || st.Month() != 9 || st.Day() != 8 {
					t.Errorf("Start mismatch: %v", st)
				}
			},
			expectExp: func(t *testing.T, exp time.Time) {
				if exp.Year() != 2026 || exp.Month() != 10 || exp.Day() != 8 {
					t.Errorf("Expiry mismatch: %v", exp)
				}
			},
		},
		{
			name:      "Layout 2006-01-02 15:04:05",
			startStr:  "2026-09-08 12:30:45",
			expiryStr: "2026-10-08 18:45:00",
			expectStart: func(t *testing.T, st time.Time) {
				if st.Hour() != 12 || st.Minute() != 30 || st.Second() != 45 {
					t.Errorf("Start time mismatch: %v", st)
				}
			},
			expectExp: func(t *testing.T, exp time.Time) {
				if exp.Hour() != 18 || exp.Minute() != 45 {
					t.Errorf("Expiry time mismatch: %v", exp)
				}
			},
		},
		{
			name:      "Layout RFC3339",
			startStr:  "2026-09-08T15:00:00Z",
			expiryStr: "2026-10-08T15:00:00Z",
			expectStart: func(t *testing.T, st time.Time) {
				if st.Year() != 2026 || st.Month() != 9 || st.Day() != 8 || st.Hour() != 15 {
					t.Errorf("Start mismatch: %v", st)
				}
			},
			expectExp: func(t *testing.T, exp time.Time) {
				if exp.Year() != 2026 || exp.Month() != 10 || exp.Day() != 8 || exp.Hour() != 15 {
					t.Errorf("Expiry mismatch: %v", exp)
				}
			},
		},
		{
			name:      "Layout RFC3339Nano",
			startStr:  "2026-09-08T15:00:00.123456789Z",
			expiryStr: "2026-10-08T15:00:00.987654321Z",
			expectStart: func(t *testing.T, st time.Time) {
				if st.Nanosecond() != 123456789 {
					t.Errorf("Start nano mismatch: %d", st.Nanosecond())
				}
			},
			expectExp: func(t *testing.T, exp time.Time) {
				if exp.Nanosecond() != 987654321 {
					t.Errorf("Expiry nano mismatch: %d", exp.Nanosecond())
				}
			},
		},
		{
			name:      "Layout 2006/01/02",
			startStr:  "2026/09/08",
			expiryStr: "2026/10/08",
			expectStart: func(t *testing.T, st time.Time) {
				if st.Year() != 2026 || st.Month() != 9 || st.Day() != 8 {
					t.Errorf("Start mismatch: %v", st)
				}
			},
			expectExp: func(t *testing.T, exp time.Time) {
				if exp.Year() != 2026 || exp.Month() != 10 || exp.Day() != 8 {
					t.Errorf("Expiry mismatch: %v", exp)
				}
			},
		},
		{
			name:      "Layout 2006/01/02 15:04:05",
			startStr:  "2026/09/08 09:15:30",
			expiryStr: "2026/10/08 21:00:00",
			expectStart: func(t *testing.T, st time.Time) {
				if st.Hour() != 9 || st.Minute() != 15 || st.Second() != 30 {
					t.Errorf("Start time mismatch: %v", st)
				}
			},
			expectExp: func(t *testing.T, exp time.Time) {
				if exp.Hour() != 21 {
					t.Errorf("Expiry time mismatch: %v", exp)
				}
			},
		},
		{
			name:      "Empty string fallback",
			startStr:  "",
			expiryStr: "",
			expectStart: func(t *testing.T, st time.Time) {
				if time.Since(st) > 5*time.Second || time.Since(st) < -5*time.Second {
					t.Errorf("Expected start fallback near now, got: %v", st)
				}
			},
			expectExp: func(t *testing.T, exp time.Time) {
				diff := exp.Sub(time.Now())
				if diff < 27*24*time.Hour || diff > 32*24*time.Hour {
					t.Errorf("Expected expiry fallback ~1 month, got diff: %v", diff)
				}
			},
		},
		{
			name:      "Invalid/Garbage format fallback",
			startStr:  "invalid-date-string",
			expiryStr: "not-a-valid-timestamp",
			expectStart: func(t *testing.T, st time.Time) {
				if time.Since(st) > 5*time.Second || time.Since(st) < -5*time.Second {
					t.Errorf("Expected start fallback near now on invalid string, got: %v", st)
				}
			},
			expectExp: func(t *testing.T, exp time.Time) {
				diff := exp.Sub(time.Now())
				if diff < 27*24*time.Hour || diff > 32*24*time.Hour {
					t.Errorf("Expected expiry fallback ~1 month on invalid string, got diff: %v", diff)
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{
				AccountProfile: config.AccountProfileConfig{
					SubscriptionStart:  tc.startStr,
					SubscriptionExpiry: tc.expiryStr,
				},
			}
			st, exp, err := cfg.ParseSubscriptionTimes()
			if err != nil {
				t.Fatalf("ParseSubscriptionTimes returned error: %v", err)
			}
			tc.expectStart(t, st)
			tc.expectExp(t, exp)
		})
	}
}
