package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tokenmonitor/config"

	"gopkg.in/yaml.v3"
)

// TestReviewerM2_ConfigYamlAllBlocks verifies that config.yaml in the project root
// contains all 6 required blocks and all nested fields per the Configuration Guide.
func TestReviewerM2_ConfigYamlAllBlocks(t *testing.T) {
	configPath := filepath.Join("..", "config.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read config.yaml: %v", err)
	}

	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Failed to unmarshal config.yaml as generic map: %v", err)
	}

	requiredBlocks := []string{
		"server",
		"proxy",
		"database",
		"account_profile",
		"local_tailer",
		"alerting",
	}

	for _, block := range requiredBlocks {
		val, exists := raw[block]
		if !exists || val == nil {
			t.Errorf("config.yaml is missing required block: %s", block)
		}
	}

	// Verify alerting.telegram sub-block
	alertingRaw, ok := raw["alerting"].(map[string]interface{})
	if !ok {
		t.Fatalf("alerting block is not a YAML mapping")
	}

	telegramRaw, exists := alertingRaw["telegram"].(map[string]interface{})
	if !exists || telegramRaw == nil {
		t.Fatalf("config.yaml is missing alerting.telegram sub-block")
	}

	if _, ok := telegramRaw["bot_token"]; !ok {
		t.Errorf("alerting.telegram is missing bot_token")
	}
	if _, ok := telegramRaw["chat_id"]; !ok {
		t.Errorf("alerting.telegram is missing chat_id")
	}

	// Also verify LoadConfig correctly parses config.yaml into typed struct
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed to load config.yaml: %v", err)
	}

	if cfg.Server.DashboardPort != 9090 {
		t.Errorf("Expected Server.DashboardPort=9090, got %d", cfg.Server.DashboardPort)
	}
	if cfg.Database.SQLitePath != "./data/token_monitor.db" {
		t.Errorf("Expected Database.SQLitePath='./data/token_monitor.db', got %s", cfg.Database.SQLitePath)
	}
	if cfg.AccountProfile.Email != "ethanpham671986@gmail.com" {
		t.Errorf("Expected AccountProfile.Email='ethanpham671986@gmail.com', got %s", cfg.AccountProfile.Email)
	}
	if cfg.LocalTailer.PollIntervalSeconds != 10 {
		t.Errorf("Expected LocalTailer.PollIntervalSeconds=10, got %d", cfg.LocalTailer.PollIntervalSeconds)
	}
}

// TestReviewerM2_NewDefaultConfig verifies all default values across all 6 blocks.
func TestReviewerM2_NewDefaultConfig(t *testing.T) {
	cfg := config.NewDefaultConfig()
	if cfg == nil {
		t.Fatal("NewDefaultConfig returned nil")
	}

	// Block 1: Server
	if cfg.Server.DashboardPort != 9090 {
		t.Errorf("Default Server.DashboardPort expected 9090, got %d", cfg.Server.DashboardPort)
	}
	if cfg.Server.BindAddress != "127.0.0.1" {
		t.Errorf("Default Server.BindAddress expected '127.0.0.1', got %s", cfg.Server.BindAddress)
	}
	if cfg.Server.ReadTimeoutSeconds != 15 {
		t.Errorf("Default Server.ReadTimeoutSeconds expected 15, got %d", cfg.Server.ReadTimeoutSeconds)
	}
	if cfg.Server.WriteTimeoutSeconds != 30 {
		t.Errorf("Default Server.WriteTimeoutSeconds expected 30, got %d", cfg.Server.WriteTimeoutSeconds)
	}

	// Block 2: Proxy
	if cfg.Proxy.Enabled != false {
		t.Errorf("Default Proxy.Enabled expected false, got %v", cfg.Proxy.Enabled)
	}
	if cfg.Proxy.ListenPort != 8080 {
		t.Errorf("Default Proxy.ListenPort expected 8080, got %d", cfg.Proxy.ListenPort)
	}
	if cfg.Proxy.UpstreamTarget != "https://generativelanguage.googleapis.com" {
		t.Errorf("Default Proxy.UpstreamTarget expected 'https://generativelanguage.googleapis.com', got %s", cfg.Proxy.UpstreamTarget)
	}
	if cfg.Proxy.MaxConcurrentRequests != 100 {
		t.Errorf("Default Proxy.MaxConcurrentRequests expected 100, got %d", cfg.Proxy.MaxConcurrentRequests)
	}
	if cfg.Proxy.UpstreamTimeoutSeconds != 180 {
		t.Errorf("Default Proxy.UpstreamTimeoutSeconds expected 180, got %d", cfg.Proxy.UpstreamTimeoutSeconds)
	}

	// Block 3: Database
	if cfg.Database.SQLitePath != "./data/token_monitor.db" {
		t.Errorf("Default Database.SQLitePath expected './data/token_monitor.db', got %s", cfg.Database.SQLitePath)
	}
	if cfg.Database.MaxOpenConns != 25 {
		t.Errorf("Default Database.MaxOpenConns expected 25, got %d", cfg.Database.MaxOpenConns)
	}
	if cfg.Database.MaxIdleConns != 10 {
		t.Errorf("Default Database.MaxIdleConns expected 10, got %d", cfg.Database.MaxIdleConns)
	}
	if cfg.Database.ConnMaxLifetimeMinutes != 60 {
		t.Errorf("Default Database.ConnMaxLifetimeMinutes expected 60, got %d", cfg.Database.ConnMaxLifetimeMinutes)
	}
	if cfg.Database.EnableWALMode != true {
		t.Errorf("Default Database.EnableWALMode expected true, got %v", cfg.Database.EnableWALMode)
	}
	if cfg.Database.RollupIntervalSeconds != 300 {
		t.Errorf("Default Database.RollupIntervalSeconds expected 300, got %d", cfg.Database.RollupIntervalSeconds)
	}

	// Block 4: AccountProfile
	if cfg.AccountProfile.AccountType != "Google Consumer Account (Individual)" {
		t.Errorf("Default AccountProfile.AccountType unexpected: %s", cfg.AccountProfile.AccountType)
	}
	if cfg.AccountProfile.PlanName != "20X ULTRA PLAN" {
		t.Errorf("Default AccountProfile.PlanName unexpected: %s", cfg.AccountProfile.PlanName)
	}
	if cfg.AccountProfile.QuotaBandwidth != "20x Quota Bandwidth" {
		t.Errorf("Default AccountProfile.QuotaBandwidth unexpected: %s", cfg.AccountProfile.QuotaBandwidth)
	}
	if cfg.AccountProfile.AutoRenew != true {
		t.Errorf("Default AccountProfile.AutoRenew expected true, got %v", cfg.AccountProfile.AutoRenew)
	}

	// Block 5: LocalTailer
	if cfg.LocalTailer.Enabled != true {
		t.Errorf("Default LocalTailer.Enabled expected true, got %v", cfg.LocalTailer.Enabled)
	}
	if cfg.LocalTailer.PollIntervalSeconds != 10 {
		t.Errorf("Default LocalTailer.PollIntervalSeconds expected 10, got %d", cfg.LocalTailer.PollIntervalSeconds)
	}

	// Block 6: Alerting
	if cfg.Alerting.Enabled != false {
		t.Errorf("Default Alerting.Enabled expected false, got %v", cfg.Alerting.Enabled)
	}
	if cfg.Alerting.DailyTokenThreshold != 50000000 {
		t.Errorf("Default Alerting.DailyTokenThreshold expected 50000000, got %d", cfg.Alerting.DailyTokenThreshold)
	}
	if cfg.Alerting.TokenExpiryWarningMinutes != 10 {
		t.Errorf("Default Alerting.TokenExpiryWarningMinutes expected 10, got %d", cfg.Alerting.TokenExpiryWarningMinutes)
	}
	if cfg.Alerting.Telegram.BotToken != "" || cfg.Alerting.Telegram.ChatID != "" {
		t.Errorf("Default Alerting.Telegram expected empty strings, got bot_token='%s', chat_id='%s'",
			cfg.Alerting.Telegram.BotToken, cfg.Alerting.Telegram.ChatID)
	}
}

// TestReviewerM2_LoadConfig_EdgeCases stress tests LoadConfig under various edge cases.
func TestReviewerM2_LoadConfig_EdgeCases(t *testing.T) {
	// Case 1: Non-existent file -> returns default config without error
	nonExistent := filepath.Join(t.TempDir(), "non_existent_config.yaml")
	cfg, err := config.LoadConfig(nonExistent)
	if err != nil {
		t.Fatalf("LoadConfig on non-existent file returned error: %v", err)
	}
	if cfg.Server.DashboardPort != 9090 {
		t.Errorf("Expected fallback to default DashboardPort=9090, got %d", cfg.Server.DashboardPort)
	}

	// Case 2: Empty file -> returns default config
	emptyFile := filepath.Join(t.TempDir(), "empty.yaml")
	if err := os.WriteFile(emptyFile, []byte(""), 0644); err != nil {
		t.Fatalf("Failed to create empty file: %v", err)
	}
	cfgEmpty, err := config.LoadConfig(emptyFile)
	if err != nil {
		t.Fatalf("LoadConfig on empty file returned error: %v", err)
	}
	if cfgEmpty.Server.DashboardPort != 9090 || cfgEmpty.Database.SQLitePath != "./data/token_monitor.db" {
		t.Errorf("LoadConfig on empty file failed to populate defaults")
	}

	// Case 3: Invalid YAML syntax -> returns descriptive error
	badFile := filepath.Join(t.TempDir(), "corrupt.yaml")
	if err := os.WriteFile(badFile, []byte("server:\n  port: [unclosed"), 0644); err != nil {
		t.Fatalf("Failed to create corrupt file: %v", err)
	}
	_, err = config.LoadConfig(badFile)
	if err == nil {
		t.Fatal("Expected error on invalid YAML syntax, got nil")
	}
	if !strings.Contains(err.Error(), "không thể parse yaml") {
		t.Errorf("Expected error to mention YAML parsing, got: %v", err)
	}

	// Case 4: Partial YAML with non-positive / zero values
	partialFile := filepath.Join(t.TempDir(), "partial.yaml")
	partialContent := `
server:
  dashboard_port: -5
  bind_address: ""
  read_timeout_seconds: 0
  write_timeout_seconds: -10
proxy:
  listen_port: -1
  upstream_target: ""
  max_concurrent_requests: 0
  upstream_timeout_seconds: -1
database:
  sqlite_path: ""
  max_open_conns: 0
  max_idle_conns: -2
  conn_max_lifetime_minutes: 0
  rollup_interval_seconds: -100
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
	if err := os.WriteFile(partialFile, []byte(partialContent), 0644); err != nil {
		t.Fatalf("Failed to write partial YAML: %v", err)
	}
	cfgPartial, err := config.LoadConfig(partialFile)
	if err != nil {
		t.Fatalf("LoadConfig on partial YAML returned error: %v", err)
	}

	// Verify all non-positive / empty fields fell back to healthy defaults
	if cfgPartial.Server.DashboardPort != 9090 {
		t.Errorf("Expected fallback DashboardPort=9090, got %d", cfgPartial.Server.DashboardPort)
	}
	if cfgPartial.Server.BindAddress != "127.0.0.1" {
		t.Errorf("Expected fallback BindAddress='127.0.0.1', got %s", cfgPartial.Server.BindAddress)
	}
	if cfgPartial.Server.ReadTimeoutSeconds != 15 {
		t.Errorf("Expected fallback ReadTimeoutSeconds=15, got %d", cfgPartial.Server.ReadTimeoutSeconds)
	}
	if cfgPartial.Server.WriteTimeoutSeconds != 30 {
		t.Errorf("Expected fallback WriteTimeoutSeconds=30, got %d", cfgPartial.Server.WriteTimeoutSeconds)
	}
	if cfgPartial.Proxy.ListenPort != 8080 {
		t.Errorf("Expected fallback ListenPort=8080, got %d", cfgPartial.Proxy.ListenPort)
	}
	if cfgPartial.Proxy.UpstreamTarget != "https://generativelanguage.googleapis.com" {
		t.Errorf("Expected fallback UpstreamTarget, got %s", cfgPartial.Proxy.UpstreamTarget)
	}
	if cfgPartial.Proxy.MaxConcurrentRequests != 100 {
		t.Errorf("Expected fallback MaxConcurrentRequests=100, got %d", cfgPartial.Proxy.MaxConcurrentRequests)
	}
	if cfgPartial.Proxy.UpstreamTimeoutSeconds != 180 {
		t.Errorf("Expected fallback UpstreamTimeoutSeconds=180, got %d", cfgPartial.Proxy.UpstreamTimeoutSeconds)
	}
	if cfgPartial.Database.SQLitePath != "./data/token_monitor.db" {
		t.Errorf("Expected fallback SQLitePath, got %s", cfgPartial.Database.SQLitePath)
	}
	if cfgPartial.Database.MaxOpenConns != 25 {
		t.Errorf("Expected fallback MaxOpenConns=25, got %d", cfgPartial.Database.MaxOpenConns)
	}
	if cfgPartial.Database.MaxIdleConns != 10 {
		t.Errorf("Expected fallback MaxIdleConns=10, got %d", cfgPartial.Database.MaxIdleConns)
	}
	if cfgPartial.Database.ConnMaxLifetimeMinutes != 60 {
		t.Errorf("Expected fallback ConnMaxLifetimeMinutes=60, got %d", cfgPartial.Database.ConnMaxLifetimeMinutes)
	}
	if cfgPartial.Database.RollupIntervalSeconds != 300 {
		t.Errorf("Expected fallback RollupIntervalSeconds=300, got %d", cfgPartial.Database.RollupIntervalSeconds)
	}
	if cfgPartial.AccountProfile.AccountType != "Google Consumer Account (Individual)" {
		t.Errorf("Expected fallback AccountType, got %s", cfgPartial.AccountProfile.AccountType)
	}
	if cfgPartial.AccountProfile.PlanName != "20X ULTRA PLAN" {
		t.Errorf("Expected fallback PlanName, got %s", cfgPartial.AccountProfile.PlanName)
	}
	if cfgPartial.AccountProfile.QuotaBandwidth != "20x Quota Bandwidth" {
		t.Errorf("Expected fallback QuotaBandwidth, got %s", cfgPartial.AccountProfile.QuotaBandwidth)
	}
	if cfgPartial.LocalTailer.PollIntervalSeconds != 10 {
		t.Errorf("Expected fallback PollIntervalSeconds=10, got %d", cfgPartial.LocalTailer.PollIntervalSeconds)
	}
	if cfgPartial.Alerting.DailyTokenThreshold != 50000000 {
		t.Errorf("Expected fallback DailyTokenThreshold=50000000, got %d", cfgPartial.Alerting.DailyTokenThreshold)
	}
	if cfgPartial.Alerting.TokenExpiryWarningMinutes != 10 {
		t.Errorf("Expected fallback TokenExpiryWarningMinutes=10, got %d", cfgPartial.Alerting.TokenExpiryWarningMinutes)
	}
}

// TestReviewerM2_ParseSubscriptionTimes_MultiLayout tests flexible date parsing.
func TestReviewerM2_ParseSubscriptionTimes_MultiLayout(t *testing.T) {
	testCases := []struct {
		name       string
		startStr   string
		expiryStr  string
		checkStart func(t *testing.T, st time.Time)
		checkExp   func(t *testing.T, exp time.Time)
	}{
		{
			name:      "Layout YYYY-MM-DD",
			startStr:  "2026-09-06",
			expiryStr: "2026-10-06",
			checkStart: func(t *testing.T, st time.Time) {
				if st.Year() != 2026 || st.Month() != 9 || st.Day() != 6 {
					t.Errorf("Start mismatch: %v", st)
				}
			},
			checkExp: func(t *testing.T, exp time.Time) {
				if exp.Year() != 2026 || exp.Month() != 10 || exp.Day() != 6 {
					t.Errorf("Expiry mismatch: %v", exp)
				}
			},
		},
		{
			name:      "Layout YYYY-MM-DD HH:MM:SS",
			startStr:  "2026-09-06 14:30:15",
			expiryStr: "2026-10-06 14:30:15",
			checkStart: func(t *testing.T, st time.Time) {
				if st.Hour() != 14 || st.Minute() != 30 || st.Second() != 15 {
					t.Errorf("Start time mismatch: %v", st)
				}
			},
			checkExp: func(t *testing.T, exp time.Time) {
				if exp.Hour() != 14 || exp.Minute() != 30 || exp.Second() != 15 {
					t.Errorf("Expiry time mismatch: %v", exp)
				}
			},
		},
		{
			name:      "Layout RFC3339",
			startStr:  "2026-09-06T14:30:15Z",
			expiryStr: "2026-10-06T14:30:15Z",
			checkStart: func(t *testing.T, st time.Time) {
				if st.Year() != 2026 || st.Month() != 9 || st.Day() != 6 {
					t.Errorf("Start mismatch: %v", st)
				}
			},
			checkExp: func(t *testing.T, exp time.Time) {
				if exp.Year() != 2026 || exp.Month() != 10 || exp.Day() != 6 {
					t.Errorf("Expiry mismatch: %v", exp)
				}
			},
		},
		{
			name:      "Layout RFC3339Nano",
			startStr:  "2026-09-06T14:30:15.123456789Z",
			expiryStr: "2026-10-06T14:30:15.123456789Z",
			checkStart: func(t *testing.T, st time.Time) {
				if st.Nanosecond() != 123456789 {
					t.Errorf("Start nanosecond mismatch: %v", st)
				}
			},
			checkExp: func(t *testing.T, exp time.Time) {
				if exp.Nanosecond() != 123456789 {
					t.Errorf("Expiry nanosecond mismatch: %v", exp)
				}
			},
		},
		{
			name:      "Layout Slash Date YYYY/MM/DD",
			startStr:  "2026/09/06",
			expiryStr: "2026/10/06 18:00:00",
			checkStart: func(t *testing.T, st time.Time) {
				if st.Year() != 2026 || st.Month() != 9 || st.Day() != 6 {
					t.Errorf("Start mismatch: %v", st)
				}
			},
			checkExp: func(t *testing.T, exp time.Time) {
				if exp.Hour() != 18 {
					t.Errorf("Expiry hour mismatch: %v", exp)
				}
			},
		},
		{
			name:      "Empty strings fallback to now and +1 month",
			startStr:  "",
			expiryStr: "   ",
			checkStart: func(t *testing.T, st time.Time) {
				if time.Since(st) > 5*time.Second || time.Since(st) < -5*time.Second {
					t.Errorf("Start fallback should be close to now, got: %v", st)
				}
			},
			checkExp: func(t *testing.T, exp time.Time) {
				diff := exp.Sub(time.Now())
				if diff < 27*24*time.Hour || diff > 32*24*time.Hour {
					t.Errorf("Expiry fallback should be ~1 month, got diff: %v", diff)
				}
			},
		},
		{
			name:      "Corrupt/Garbage strings fallback gracefully",
			startStr:  "not-a-real-date",
			expiryStr: "garbage-value-12345",
			checkStart: func(t *testing.T, st time.Time) {
				if time.Since(st) > 5*time.Second || time.Since(st) < -5*time.Second {
					t.Errorf("Start garbage fallback should be close to now, got: %v", st)
				}
			},
			checkExp: func(t *testing.T, exp time.Time) {
				diff := exp.Sub(time.Now())
				if diff < 27*24*time.Hour || diff > 32*24*time.Hour {
					t.Errorf("Expiry garbage fallback should be ~1 month, got diff: %v", diff)
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
			start, expiry, err := cfg.ParseSubscriptionTimes()
			if err != nil {
				t.Fatalf("ParseSubscriptionTimes returned unexpected error: %v", err)
			}
			tc.checkStart(t, start)
			tc.checkExp(t, expiry)
		})
	}
}
