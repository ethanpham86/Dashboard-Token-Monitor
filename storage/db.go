package storage

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"tokenmonitor/config"

	"modernc.org/sqlite"
)

func init() {
	sqlite.RegisterConnectionHook(func(conn sqlite.ExecQuerierContext, dsn string) error {
		pragmas := []string{
			"PRAGMA foreign_keys = ON;",
			"PRAGMA journal_mode = WAL;",
			"PRAGMA synchronous = NORMAL;",
			"PRAGMA cache_size = -64000;",
			"PRAGMA temp_store = MEMORY;",
			"PRAGMA busy_timeout = 5000;",
		}
		for _, p := range pragmas {
			if _, err := conn.ExecContext(context.Background(), p, nil); err != nil {
				return fmt.Errorf("lỗi khởi tạo pragma '%s' trên kết nối mới: %w", p, err)
			}
		}
		return nil
	})
}


type Storage struct {
	DB  *sql.DB
	cfg *config.Config
}

func NewStorage(cfg *config.Config) (*Storage, error) {
	dbDir := filepath.Dir(cfg.Database.SQLitePath)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return nil, fmt.Errorf("không thể tạo thư mục database %s: %w", dbDir, err)
	}

	db, err := sql.Open("sqlite", cfg.Database.SQLitePath)
	if err != nil {
		return nil, fmt.Errorf("không thể mở database sqlite: %w", err)
	}

	// Cấu hình Connection Pooling theo chuẩn golang-expert-guidelines
	maxOpen := cfg.Database.MaxOpenConns
	if maxOpen <= 0 {
		maxOpen = 25
	}
	maxIdle := cfg.Database.MaxIdleConns
	if maxIdle <= 0 {
		maxIdle = 10
	}
	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)
	db.SetConnMaxLifetime(time.Duration(cfg.Database.ConnMaxLifetimeMinutes) * time.Minute)

	s := &Storage{DB: db, cfg: cfg}
	if err := s.applyPragmas(); err != nil {
		return nil, fmt.Errorf("lỗi thiết lập sqlite pragma: %w", err)
	}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("lỗi khởi tạo migration: %w", err)
	}
	if err := s.ensureDefaultAccount(); err != nil {
		return nil, fmt.Errorf("lỗi khởi tạo tài khoản mặc định: %w", err)
	}

	return s, nil
}

func (s *Storage) applyPragmas() error {
	pragmas := []string{
		"PRAGMA foreign_keys = ON;",
		"PRAGMA journal_mode = WAL;",
		"PRAGMA synchronous = NORMAL;",
		"PRAGMA cache_size = -64000;",
		"PRAGMA temp_store = MEMORY;",
		"PRAGMA busy_timeout = 5000;",
	}
	for _, p := range pragmas {
		if _, err := s.DB.Exec(p); err != nil {
			return fmt.Errorf("lỗi chạy pragma '%s': %w", p, err)
		}
	}
	return nil
}

func (s *Storage) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS accounts (
		id                  INTEGER PRIMARY KEY AUTOINCREMENT,
		account_email       TEXT NOT NULL UNIQUE,
		account_type        TEXT NOT NULL DEFAULT 'Google Consumer Account (Individual)',
		plan_name           TEXT NOT NULL DEFAULT '20X ULTRA PLAN',
		quota_bandwidth     TEXT NOT NULL DEFAULT '20x Quota Bandwidth',
		installation_uuid   TEXT NOT NULL,
		registered_at       DATETIME NOT NULL,
		subscription_expiry DATETIME NOT NULL,
		auto_renew          BOOLEAN NOT NULL DEFAULT 1,
		created_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS auth_sessions (
		id                  INTEGER PRIMARY KEY AUTOINCREMENT,
		account_id          INTEGER NOT NULL,
		token_status        TEXT NOT NULL CHECK(token_status IN ('VALID', 'EXPIRED', 'REFRESHING', 'REVOKED')),
		token_issued_at     DATETIME NOT NULL,
		token_expires_at    DATETIME NOT NULL,
		last_validated_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(account_id) REFERENCES accounts(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS token_usage_logs (
		id                  INTEGER PRIMARY KEY AUTOINCREMENT,
		account_id          INTEGER NOT NULL,
		timestamp           DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		model_name          TEXT NOT NULL,
		prompt_tokens       INTEGER NOT NULL DEFAULT 0,
		output_tokens       INTEGER NOT NULL DEFAULT 0,
		thinking_tokens     INTEGER NOT NULL DEFAULT 0,
		cached_tokens       INTEGER NOT NULL DEFAULT 0,
		total_tokens        INTEGER NOT NULL DEFAULT 0,
		latency_ms          INTEGER NOT NULL DEFAULT 0,
		status_code         INTEGER NOT NULL DEFAULT 200,
		request_type        TEXT NOT NULL DEFAULT 'INTERACTIVE',
		FOREIGN KEY(account_id) REFERENCES accounts(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS token_usage_hourly_rollup (
		id                  INTEGER PRIMARY KEY AUTOINCREMENT,
		account_id          INTEGER NOT NULL,
		time_bucket         DATETIME NOT NULL,
		model_name          TEXT NOT NULL,
		call_count          INTEGER NOT NULL DEFAULT 0,
		sum_prompt_tokens   INTEGER NOT NULL DEFAULT 0,
		sum_output_tokens   INTEGER NOT NULL DEFAULT 0,
		sum_thinking_tokens INTEGER NOT NULL DEFAULT 0,
		sum_cached_tokens   INTEGER NOT NULL DEFAULT 0,
		sum_total_tokens    INTEGER NOT NULL DEFAULT 0,
		avg_latency_ms      REAL NOT NULL DEFAULT 0.0,
		UNIQUE(account_id, time_bucket, model_name),
		FOREIGN KEY(account_id) REFERENCES accounts(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_token_usage_timestamp 
		ON token_usage_logs(timestamp DESC);

	CREATE INDEX IF NOT EXISTS idx_token_usage_account_model 
		ON token_usage_logs(account_id, model_name, timestamp DESC);

	CREATE INDEX IF NOT EXISTS idx_hourly_bucket 
		ON token_usage_hourly_rollup(account_id, time_bucket DESC);

	CREATE UNIQUE INDEX IF NOT EXISTS idx_token_dedup_chat 
		ON token_usage_logs(request_type) 
		WHERE request_type LIKE 'CHAT_%';

	CREATE TABLE IF NOT EXISTS agent_fleet_telemetry (
		id                  INTEGER PRIMARY KEY AUTOINCREMENT,
		account_id          INTEGER NOT NULL DEFAULT 1,
		subagent_id         TEXT NOT NULL,
		role_name           TEXT NOT NULL,
		task_name           TEXT NOT NULL,
		status              TEXT NOT NULL DEFAULT 'COMPLETED',
		started_at          DATETIME NOT NULL,
		finished_at         DATETIME,
		duration_ms         INTEGER DEFAULT 0,
		tokens_used         INTEGER DEFAULT 0,
		tokens_offloaded    INTEGER DEFAULT 0,
		created_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_agent_fleet_started 
		ON agent_fleet_telemetry(started_at DESC);

	CREATE INDEX IF NOT EXISTS idx_agent_fleet_role 
		ON agent_fleet_telemetry(role_name, started_at DESC);

	CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_fleet_subagent 
		ON agent_fleet_telemetry(subagent_id);
	`
	if _, err := s.DB.Exec(schema); err != nil {
		return err
	}

	// Tự động dọn dẹp các bản ghi mock/seed cũ nếu còn tồn tại
	_, _ = s.DB.Exec("DELETE FROM agent_fleet_telemetry WHERE subagent_id LIKE 'subagent-task-%'")
	return nil
}

func (s *Storage) ensureDefaultAccount() error {
	// Tự động phát hiện tài khoản thực tế đang đăng nhập trong Antigravity IDE
	name, email, plan, err := DetectActiveAntigravityAccount()
	if err == nil && email != "" {
		s.cfg.AccountProfile.Email = email
		if name != "" {
			s.cfg.AccountProfile.PlanName = fmt.Sprintf("%s (%s)", plan, name)
		} else {
			s.cfg.AccountProfile.PlanName = plan
		}
	}

	uuidStr := ""
	if detectedUUID, err := DetectInstallationUUID(); err == nil && detectedUUID != "" {
		uuidStr = detectedUUID
	} else if s.cfg.AccountProfile.InstallationUUIDPath != "" {
		if data, err := os.ReadFile(s.cfg.AccountProfile.InstallationUUIDPath); err == nil {
			uuidStr = strings.TrimSpace(string(data))
		}
	}
	if uuidStr == "" {
		uuidStr = "unknown-installation-uuid"
	}

	start, expiry, _ := s.cfg.ParseSubscriptionTimes()

	// Trích xuất hạn token OAuth thực tế từ state.vscdb
	tokenExpiry, err := DetectOAuthTokenExpiry()
	now := time.Now()
	if err != nil || tokenExpiry.IsZero() {
		tokenExpiry = now.Add(58*time.Minute + 30*time.Second)
	}

	var totalAccounts int
	_ = s.DB.QueryRow("SELECT COUNT(*) FROM accounts").Scan(&totalAccounts)
	if totalAccounts > 0 {
		_, err = s.DB.Exec(`
			UPDATE accounts SET 
				account_email = ?,
				account_type = ?,
				plan_name = ?,
				quota_bandwidth = ?,
				installation_uuid = ?,
				registered_at = ?,
				subscription_expiry = ?,
				updated_at = CURRENT_TIMESTAMP
			WHERE id = 1
		`, s.cfg.AccountProfile.Email, s.cfg.AccountProfile.AccountType, s.cfg.AccountProfile.PlanName, s.cfg.AccountProfile.QuotaBandwidth, uuidStr, start.Format("2006-01-02 15:04:05"), expiry.Format("2006-01-02 15:04:05"))
		if err != nil {
			return err
		}

		var sessionCount int
		_ = s.DB.QueryRow("SELECT COUNT(*) FROM auth_sessions WHERE account_id = 1").Scan(&sessionCount)
		if sessionCount == 0 {
			_, _ = s.DB.Exec(`
				INSERT INTO auth_sessions (account_id, token_status, token_issued_at, token_expires_at)
				VALUES (1, 'VALID', ?, ?)
			`, now, tokenExpiry)
		}
		return nil
	}

	_, err = s.DB.Exec(`
		INSERT INTO accounts (account_email, account_type, plan_name, quota_bandwidth, installation_uuid, registered_at, subscription_expiry, auto_renew)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, s.cfg.AccountProfile.Email, s.cfg.AccountProfile.AccountType, s.cfg.AccountProfile.PlanName, s.cfg.AccountProfile.QuotaBandwidth, uuidStr, start.Format("2006-01-02 15:04:05"), expiry.Format("2006-01-02 15:04:05"), s.cfg.AccountProfile.AutoRenew)
	if err != nil {
		return err
	}

	// Khởi tạo phiên token hợp lệ ban đầu
	var accountID int64
	_ = s.DB.QueryRow("SELECT id FROM accounts WHERE account_email = ?", s.cfg.AccountProfile.Email).Scan(&accountID)
	_, _ = s.DB.Exec(`
		INSERT INTO auth_sessions (account_id, token_status, token_issued_at, token_expires_at)
		VALUES (?, 'VALID', ?, ?)
	`, accountID, now, tokenExpiry)

	return nil
}

func (s *Storage) Close() error {
	if s.DB != nil {
		return s.DB.Close()
	}
	return nil
}
