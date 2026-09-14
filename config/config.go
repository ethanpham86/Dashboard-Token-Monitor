package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server         ServerConfig         `yaml:"server"`
	Proxy          ProxyConfig          `yaml:"proxy"`
	Database       DatabaseConfig       `yaml:"database"`
	AccountProfile AccountProfileConfig `yaml:"account_profile"`
	LocalTailer    LocalTailerConfig    `yaml:"local_tailer"`
	OpenAIMonitor  OpenAIMonitorConfig  `yaml:"openai_monitor"`
	ClaudeMonitor  ClaudeMonitorConfig  `yaml:"claude_monitor"`
	Alerting       AlertingConfig       `yaml:"alerting"`
	Backup         BackupConfig         `yaml:"backup"`
}

type ServerConfig struct {
	DashboardPort       int    `yaml:"dashboard_port"`
	BindAddress         string `yaml:"bind_address"`
	ReadTimeoutSeconds  int    `yaml:"read_timeout_seconds"`
	WriteTimeoutSeconds int    `yaml:"write_timeout_seconds"`
}

type ProxyConfig struct {
	Enabled                bool   `yaml:"enabled"`
	ListenPort             int    `yaml:"listen_port"`
	UpstreamTarget         string `yaml:"upstream_target"`
	MaxConcurrentRequests  int    `yaml:"max_concurrent_requests"`
	UpstreamTimeoutSeconds int    `yaml:"upstream_timeout_seconds"`
}

type DatabaseConfig struct {
	SQLitePath             string        `yaml:"sqlite_path"`
	MaxOpenConns           int           `yaml:"max_open_conns"`
	MaxIdleConns           int           `yaml:"max_idle_conns"`
	ConnMaxLifetimeMinutes int           `yaml:"conn_max_lifetime_minutes"`
	EnableWALMode          bool          `yaml:"enable_wal_mode"`
	RollupIntervalSeconds  int           `yaml:"rollup_interval_seconds"`
	Backup                 *BackupConfig `yaml:"backup,omitempty"`
}

type BackupConfig struct {
	Enabled         bool   `yaml:"enabled"`
	IntervalMinutes int    `yaml:"interval_minutes"`
	BackupDir       string `yaml:"backup_dir"`
	MaxKeep         int    `yaml:"max_keep"`
}

type AccountProfileConfig struct {
	Email                string `yaml:"email"`
	AccountType          string `yaml:"account_type"`
	PlanName             string `yaml:"plan_name"`
	QuotaBandwidth       string `yaml:"quota_bandwidth"`
	InstallationUUIDPath string `yaml:"installation_uuid_path"`
	SubscriptionStart    string `yaml:"subscription_start"`
	SubscriptionExpiry   string `yaml:"subscription_expiry"`
	AutoRenew            bool   `yaml:"auto_renew"`
}

type LocalTailerConfig struct {
	Enabled             bool   `yaml:"enabled"`
	IDEBrainDir         string `yaml:"ide_brain_dir"`
	IDEConvDBDir        string `yaml:"ide_conv_db_dir"`
	PollIntervalSeconds int    `yaml:"poll_interval_seconds"`
}

// OpenAIMonitorConfig điều khiển collector chỉ-đọc dành cho Codex/OpenAI.
// Collector chỉ quét session JSONL và tuyệt đối không đọc auth.json.
type OpenAIMonitorConfig struct {
	Enabled             bool   `yaml:"enabled"`
	SessionsDir         string `yaml:"sessions_dir"`
	PollIntervalSeconds int    `yaml:"poll_interval_seconds"`
	MaxSessionRows      int    `yaml:"max_session_rows"`
	MaxFiles            int    `yaml:"max_files"`
}

// ClaudeMonitorConfig điều khiển collector chỉ-đọc dành cho Anthropic Claude Code.
// Collector chỉ quét projects session JSONL và tuyệt đối không đọc .credentials.json.
type ClaudeMonitorConfig struct {
	Enabled             bool   `yaml:"enabled"`
	ProjectsDir         string `yaml:"projects_dir"`
	PollIntervalSeconds int    `yaml:"poll_interval_seconds"`
	MaxSessionRows      int    `yaml:"max_session_rows"`
	MaxFiles            int    `yaml:"max_files"`
}

type AlertingConfig struct {
	Enabled                   bool           `yaml:"enabled"`
	DailyTokenThreshold       int64          `yaml:"daily_token_threshold"`
	TokenExpiryWarningMinutes int            `yaml:"token_expiry_warning_minutes"`
	Telegram                  TelegramConfig `yaml:"telegram"`
}

type TelegramConfig struct {
	BotToken string `yaml:"bot_token"`
	ChatID   string `yaml:"chat_id"`
}

// NewDefaultConfig khởi tạo cấu hình chuẩn với giá trị mặc định cho toàn bộ 6 khối cấu hình
func NewDefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			DashboardPort:       9090,
			BindAddress:         "127.0.0.1",
			ReadTimeoutSeconds:  15,
			WriteTimeoutSeconds: 30,
		},
		Proxy: ProxyConfig{
			Enabled:                false,
			ListenPort:             8080,
			UpstreamTarget:         "https://generativelanguage.googleapis.com",
			MaxConcurrentRequests:  100,
			UpstreamTimeoutSeconds: 180,
		},
		Database: DatabaseConfig{
			SQLitePath:             "./data/token_monitor.db",
			MaxOpenConns:           25,
			MaxIdleConns:           10,
			ConnMaxLifetimeMinutes: 60,
			EnableWALMode:          true,
			RollupIntervalSeconds:  300,
		},
		AccountProfile: AccountProfileConfig{
			AccountType:    "Google Consumer Account (Individual)",
			PlanName:       "20X ULTRA PLAN",
			QuotaBandwidth: "20x Quota Bandwidth",
			AutoRenew:      true,
		},
		LocalTailer: LocalTailerConfig{
			Enabled:             true,
			PollIntervalSeconds: 10,
		},
		OpenAIMonitor: OpenAIMonitorConfig{
			Enabled:             true,
			PollIntervalSeconds: 10,
			MaxSessionRows:      50,
			MaxFiles:            1000,
		},
		ClaudeMonitor: ClaudeMonitorConfig{
			Enabled:             true,
			PollIntervalSeconds: 10,
			MaxSessionRows:      50,
			MaxFiles:            1000,
		},
		Alerting: AlertingConfig{
			Enabled:                   false,
			DailyTokenThreshold:       50000000,
			TokenExpiryWarningMinutes: 10,
			Telegram: TelegramConfig{
				BotToken: "",
				ChatID:   "",
			},
		},
		Backup: BackupConfig{
			Enabled:         true,
			IntervalMinutes: 60,
			BackupDir:       "./data/backup",
			MaxKeep:         7,
		},
	}
}

func LoadConfig(path string) (*Config, error) {
	cfg := NewDefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("không thể đọc file cấu hình %s: %w", path, err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("không thể parse yaml cấu hình: %w", err)
	}

	// Đặt giá trị mặc định nếu thiếu hoặc không hợp lệ
	if cfg.Server.DashboardPort <= 0 {
		cfg.Server.DashboardPort = 9090
	}
	if cfg.Server.BindAddress == "" {
		cfg.Server.BindAddress = "127.0.0.1"
	}
	if cfg.Server.ReadTimeoutSeconds <= 0 {
		cfg.Server.ReadTimeoutSeconds = 15
	}
	if cfg.Server.WriteTimeoutSeconds <= 0 {
		cfg.Server.WriteTimeoutSeconds = 30
	}

	if cfg.Proxy.ListenPort <= 0 {
		cfg.Proxy.ListenPort = 8080
	}
	if cfg.Proxy.UpstreamTarget == "" {
		cfg.Proxy.UpstreamTarget = "https://generativelanguage.googleapis.com"
	}
	if cfg.Proxy.MaxConcurrentRequests <= 0 {
		cfg.Proxy.MaxConcurrentRequests = 100
	}
	if cfg.Proxy.UpstreamTimeoutSeconds <= 0 {
		cfg.Proxy.UpstreamTimeoutSeconds = 180
	}

	if cfg.Database.SQLitePath == "" {
		cfg.Database.SQLitePath = "./data/token_monitor.db"
	}
	if cfg.Database.MaxOpenConns <= 0 {
		cfg.Database.MaxOpenConns = 25
	}
	if cfg.Database.MaxIdleConns <= 0 {
		cfg.Database.MaxIdleConns = 10
	}
	if cfg.Database.ConnMaxLifetimeMinutes <= 0 {
		cfg.Database.ConnMaxLifetimeMinutes = 60
	}
	if cfg.Database.RollupIntervalSeconds <= 0 {
		cfg.Database.RollupIntervalSeconds = 300
	}

	if cfg.AccountProfile.AccountType == "" {
		cfg.AccountProfile.AccountType = "Google Consumer Account (Individual)"
	}
	if cfg.AccountProfile.PlanName == "" {
		cfg.AccountProfile.PlanName = "20X ULTRA PLAN"
	}
	if cfg.AccountProfile.QuotaBandwidth == "" {
		cfg.AccountProfile.QuotaBandwidth = "20x Quota Bandwidth"
	}

	if cfg.LocalTailer.PollIntervalSeconds <= 0 {
		cfg.LocalTailer.PollIntervalSeconds = 10
	}
	if cfg.OpenAIMonitor.PollIntervalSeconds <= 0 {
		cfg.OpenAIMonitor.PollIntervalSeconds = 10
	}
	if cfg.OpenAIMonitor.MaxSessionRows <= 0 {
		cfg.OpenAIMonitor.MaxSessionRows = 50
	}
	if cfg.OpenAIMonitor.MaxFiles <= 0 {
		cfg.OpenAIMonitor.MaxFiles = 1000
	}

	if cfg.Alerting.DailyTokenThreshold <= 0 {
		cfg.Alerting.DailyTokenThreshold = 50000000
	}
	if cfg.Alerting.TokenExpiryWarningMinutes <= 0 {
		cfg.Alerting.TokenExpiryWarningMinutes = 10
	}

	if cfg.Database.Backup != nil {
		if cfg.Database.Backup.BackupDir != "" {
			cfg.Backup.BackupDir = cfg.Database.Backup.BackupDir
		}
		if cfg.Database.Backup.IntervalMinutes > 0 {
			cfg.Backup.IntervalMinutes = cfg.Database.Backup.IntervalMinutes
		}
		if cfg.Database.Backup.MaxKeep > 0 {
			cfg.Backup.MaxKeep = cfg.Database.Backup.MaxKeep
		}
		cfg.Backup.Enabled = cfg.Database.Backup.Enabled
	}
	if cfg.Backup.IntervalMinutes <= 0 {
		cfg.Backup.IntervalMinutes = 60
	}
	if cfg.Backup.BackupDir == "" {
		cfg.Backup.BackupDir = "./data/backup"
	}
	if cfg.Backup.MaxKeep <= 0 {
		cfg.Backup.MaxKeep = 7
	}

	return cfg, nil
}

func (c *Config) ParseSubscriptionTimes() (time.Time, time.Time, error) {
	layouts := []string{
		"2006-01-02",
		"2006-01-02 15:04:05",
		time.RFC3339,
		time.RFC3339Nano,
		"2006/01/02",
		"2006/01/02 15:04:05",
	}

	now := time.Now()
	defaultExpiry := now.AddDate(0, 1, 0)

	parseDate := func(val string, fallback time.Time) time.Time {
		val = strings.TrimSpace(val)
		if val == "" {
			return fallback
		}
		for _, layout := range layouts {
			if t, err := time.Parse(layout, val); err == nil {
				return t
			}
		}
		return fallback
	}

	start := parseDate(c.AccountProfile.SubscriptionStart, now)
	expiry := parseDate(c.AccountProfile.SubscriptionExpiry, defaultExpiry)

	return start, expiry, nil
}
