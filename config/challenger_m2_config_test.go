package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"tokenmonitor/config"
)

// TestChallenger_M2_ConfigDefaults verifies NewDefaultConfig sets all 6 blocks with expected values
func TestChallenger_M2_ConfigDefaults(t *testing.T) {
	cfg := config.NewDefaultConfig()
	if cfg == nil {
		t.Fatal("NewDefaultConfig returned nil")
	}

	// 1. Server
	if cfg.Server.DashboardPort != 9090 {
		t.Errorf("Expected DashboardPort 9090, got %d", cfg.Server.DashboardPort)
	}
	if cfg.Server.BindAddress != "127.0.0.1" {
		t.Errorf("Expected BindAddress '127.0.0.1', got '%s'", cfg.Server.BindAddress)
	}
	if cfg.Server.ReadTimeoutSeconds != 15 {
		t.Errorf("Expected ReadTimeoutSeconds 15, got %d", cfg.Server.ReadTimeoutSeconds)
	}
	if cfg.Server.WriteTimeoutSeconds != 30 {
		t.Errorf("Expected WriteTimeoutSeconds 30, got %d", cfg.Server.WriteTimeoutSeconds)
	}

	// 2. Proxy
	if cfg.Proxy.Enabled {
		t.Errorf("Expected Proxy.Enabled false, got true")
	}
	if cfg.Proxy.ListenPort != 8080 {
		t.Errorf("Expected ListenPort 8080, got %d", cfg.Proxy.ListenPort)
	}
	if cfg.Proxy.UpstreamTarget != "https://generativelanguage.googleapis.com" {
		t.Errorf("Expected UpstreamTarget 'https://generativelanguage.googleapis.com', got '%s'", cfg.Proxy.UpstreamTarget)
	}
	if cfg.Proxy.MaxConcurrentRequests != 100 {
		t.Errorf("Expected MaxConcurrentRequests 100, got %d", cfg.Proxy.MaxConcurrentRequests)
	}
	if cfg.Proxy.UpstreamTimeoutSeconds != 180 {
		t.Errorf("Expected UpstreamTimeoutSeconds 180, got %d", cfg.Proxy.UpstreamTimeoutSeconds)
	}

	// 3. Database
	if cfg.Database.SQLitePath != "./data/token_monitor.db" {
		t.Errorf("Expected SQLitePath './data/token_monitor.db', got '%s'", cfg.Database.SQLitePath)
	}
	if cfg.Database.MaxOpenConns != 25 {
		t.Errorf("Expected MaxOpenConns 25, got %d", cfg.Database.MaxOpenConns)
	}
	if cfg.Database.MaxIdleConns != 10 {
		t.Errorf("Expected MaxIdleConns 10, got %d", cfg.Database.MaxIdleConns)
	}
	if cfg.Database.ConnMaxLifetimeMinutes != 60 {
		t.Errorf("Expected ConnMaxLifetimeMinutes 60, got %d", cfg.Database.ConnMaxLifetimeMinutes)
	}
	if !cfg.Database.EnableWALMode {
		t.Errorf("Expected EnableWALMode true, got false")
	}
	if cfg.Database.RollupIntervalSeconds != 300 {
		t.Errorf("Expected RollupIntervalSeconds 300, got %d", cfg.Database.RollupIntervalSeconds)
	}

	// 4. AccountProfile
	if cfg.AccountProfile.AccountType != "Google Consumer Account (Individual)" {
		t.Errorf("Expected AccountType 'Google Consumer Account (Individual)', got '%s'", cfg.AccountProfile.AccountType)
	}
	if cfg.AccountProfile.PlanName != "20X ULTRA PLAN" {
		t.Errorf("Expected PlanName '20X ULTRA PLAN', got '%s'", cfg.AccountProfile.PlanName)
	}
	if cfg.AccountProfile.QuotaBandwidth != "20x Quota Bandwidth" {
		t.Errorf("Expected QuotaBandwidth '20x Quota Bandwidth', got '%s'", cfg.AccountProfile.QuotaBandwidth)
	}
	if !cfg.AccountProfile.AutoRenew {
		t.Errorf("Expected AutoRenew true, got false")
	}

	// 5. LocalTailer
	if !cfg.LocalTailer.Enabled {
		t.Errorf("Expected LocalTailer.Enabled true, got false")
	}
	if cfg.LocalTailer.PollIntervalSeconds != 10 {
		t.Errorf("Expected PollIntervalSeconds 10, got %d", cfg.LocalTailer.PollIntervalSeconds)
	}

	// 6. Alerting
	if cfg.Alerting.Enabled {
		t.Errorf("Expected Alerting.Enabled false, got true")
	}
	if cfg.Alerting.DailyTokenThreshold != 50000000 {
		t.Errorf("Expected DailyTokenThreshold 50000000, got %d", cfg.Alerting.DailyTokenThreshold)
	}
	if cfg.Alerting.TokenExpiryWarningMinutes != 10 {
		t.Errorf("Expected TokenExpiryWarningMinutes 10, got %d", cfg.Alerting.TokenExpiryWarningMinutes)
	}
}

// TestChallenger_M2_LoadConfig_MissingFile verifies fallback when config file is absent
func TestChallenger_M2_LoadConfig_MissingFile(t *testing.T) {
	tempDir := t.TempDir()
	nonExistent := filepath.Join(tempDir, "absent_config.yaml")

	cfg, err := config.LoadConfig(nonExistent)
	if err != nil {
		t.Fatalf("LoadConfig failed on non-existent file: %v", err)
	}
	if cfg == nil {
		t.Fatal("Expected default config on missing file, got nil")
	}
	if cfg.Server.DashboardPort != 9090 {
		t.Errorf("Expected fallback port 9090, got %d", cfg.Server.DashboardPort)
	}
}

// TestChallenger_M2_LoadConfig_ActualConfigFile verifies loading the repository's real config.yaml
func TestChallenger_M2_LoadConfig_ActualConfigFile(t *testing.T) {
	cfgPath := "../config.yaml"
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		t.Skip("config.yaml not found at root")
	}

	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("Failed to load root config.yaml: %v", err)
	}

	if cfg.Server.DashboardPort == 0 {
		t.Errorf("DashboardPort is zero")
	}
	if cfg.Database.SQLitePath == "" {
		t.Errorf("SQLitePath is empty")
	}
	// Verify telegram fields are present in struct without unmarshal errors
	t.Logf("Loaded config.yaml successfully: Server Port=%d, DB=%s, Alerting Telegram BotTokenLen=%d",
		cfg.Server.DashboardPort, cfg.Database.SQLitePath, len(cfg.Alerting.Telegram.BotToken))
}

// TestChallenger_M2_ParseSubscriptionTimes verifies multi-layout and fallback parsing
func TestChallenger_M2_ParseSubscriptionTimes(t *testing.T) {
	cases := []struct {
		name        string
		startStr    string
		expiryStr   string
		expectYear  int
		expectMonth time.Month
		expectDay   int
	}{
		{
			name:        "YYYY-MM-DD",
			startStr:    "2026-09-06",
			expiryStr:   "2026-10-06",
			expectYear:  2026,
			expectMonth: time.October,
			expectDay:   6,
		},
		{
			name:        "YYYY-MM-DD HH:MM:SS",
			startStr:    "2026-09-06 14:30:00",
			expiryStr:   "2026-10-06 18:45:00",
			expectYear:  2026,
			expectMonth: time.October,
			expectDay:   6,
		},
		{
			name:        "RFC3339",
			startStr:    "2026-09-06T00:00:00Z",
			expiryStr:   "2026-10-06T23:59:59Z",
			expectYear:  2026,
			expectMonth: time.October,
			expectDay:   6,
		},
		{
			name:        "Slash_Format",
			startStr:    "2026/09/06",
			expiryStr:   "2026/10/06",
			expectYear:  2026,
			expectMonth: time.October,
			expectDay:   6,
		},
		{
			name:        "Slash_With_Time",
			startStr:    "2026/09/06 10:00:00",
			expiryStr:   "2026/10/06 20:00:00",
			expectYear:  2026,
			expectMonth: time.October,
			expectDay:   6,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{
				AccountProfile: config.AccountProfileConfig{
					SubscriptionStart:  tc.startStr,
					SubscriptionExpiry: tc.expiryStr,
				},
			}
			start, expiry, err := cfg.ParseSubscriptionTimes()
			if err != nil {
				t.Fatalf("ParseSubscriptionTimes failed: %v", err)
			}
			if expiry.Year() != tc.expectYear || expiry.Month() != tc.expectMonth || expiry.Day() != tc.expectDay {
				t.Errorf("Expected expiry %04d-%02d-%02d, got %v", tc.expectYear, tc.expectMonth, tc.expectDay, expiry)
			}
			if start.IsZero() {
				t.Errorf("Start time should not be zero")
			}
		})
	}

	t.Run("Empty_And_Invalid_Fallback", func(t *testing.T) {
		cfg := &config.Config{
			AccountProfile: config.AccountProfileConfig{
				SubscriptionStart:  "",
				SubscriptionExpiry: "INVALID_DATE_STRING",
			},
		}
		start, expiry, err := cfg.ParseSubscriptionTimes()
		if err != nil {
			t.Fatalf("ParseSubscriptionTimes unexpected error: %v", err)
		}
		now := time.Now()
		if start.IsZero() || start.Year() != now.Year() {
			t.Errorf("Expected fallback start around now, got %v", start)
		}
		if expiry.IsZero() || expiry.Before(now) {
			t.Errorf("Expected fallback expiry in future, got %v", expiry)
		}
	})
}
