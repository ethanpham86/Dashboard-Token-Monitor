-- ==================================================================================
-- File Name    : TokenMonitor_Ref_001_schema.sql
-- Description  : Database Schema chuẩn cho SQLite Time-Series Token & Quota Monitoring
-- System       : TokenMonitor (Production Observability & FinOps)
-- Date         : 2026-09-08
-- ==================================================================================

PRAGMA foreign_keys = ON;
PRAGMA journal_mode = WAL;
PRAGMA synchronous = NORMAL;
PRAGMA cache_size = -64000;
PRAGMA temp_store = MEMORY;
PRAGMA busy_timeout = 5000;

-- Bảng 1: Quản lý thông tin tài khoản và gói thuê bao
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

-- Bảng 2: Quản lý phiên làm việc & trạng thái Token (OAuth / Bearer)
CREATE TABLE IF NOT EXISTS auth_sessions (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id          INTEGER NOT NULL,
    token_status        TEXT NOT NULL CHECK(token_status IN ('VALID', 'EXPIRED', 'REFRESHING', 'REVOKED')),
    token_issued_at     DATETIME NOT NULL,
    token_expires_at    DATETIME NOT NULL,
    last_validated_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(account_id) REFERENCES accounts(id) ON DELETE CASCADE
);

-- Bảng 3: Nhật ký tiêu thụ Token theo từng cuộc gọi Model (Raw Time-Series)
CREATE TABLE IF NOT EXISTS token_usage_logs (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id          INTEGER NOT NULL,
    timestamp           DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    model_name          TEXT NOT NULL,               -- vd: 'Gemini Ultra', 'Gemini 3.8 Flash (High)'
    prompt_tokens       INTEGER NOT NULL DEFAULT 0,  -- Input context window
    output_tokens       INTEGER NOT NULL DEFAULT 0,  -- Candidates generation
    thinking_tokens     INTEGER NOT NULL DEFAULT 0,  -- Chain-of-Thought (CoT) reasoning
    cached_tokens       INTEGER NOT NULL DEFAULT 0,  -- Prompt Context Caching
    total_tokens        INTEGER NOT NULL DEFAULT 0,  -- Grand total tokens
    latency_ms          INTEGER NOT NULL DEFAULT 0,  -- Response latency
    status_code         INTEGER NOT NULL DEFAULT 200,
    request_type        TEXT NOT NULL DEFAULT 'INTERACTIVE', -- 'INTERACTIVE', 'BACKGROUND', 'BATCH'
    FOREIGN KEY(account_id) REFERENCES accounts(id) ON DELETE CASCADE
);

-- Bảng 4: Bảng rollup tổng hợp dữ liệu theo giờ để tối ưu tốc độ render biểu đồ
CREATE TABLE IF NOT EXISTS token_usage_hourly_rollup (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id          INTEGER NOT NULL,
    time_bucket         DATETIME NOT NULL,           -- Khung giờ, vd: '2026-09-08 12:00:00'
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

-- Indexes tối ưu hóa truy vấn Dashboard
CREATE INDEX IF NOT EXISTS idx_token_usage_timestamp 
    ON token_usage_logs(timestamp DESC);

CREATE INDEX IF NOT EXISTS idx_token_usage_account_model 
    ON token_usage_logs(account_id, model_name, timestamp DESC);

CREATE INDEX IF NOT EXISTS idx_hourly_bucket 
    ON token_usage_hourly_rollup(account_id, time_bucket DESC);

CREATE UNIQUE INDEX IF NOT EXISTS idx_token_dedup_chat 
    ON token_usage_logs(request_type) 
    WHERE request_type LIKE 'CHAT_%';

-- Bảng 5: Quản lý vòng đời tác vụ và telemetry của Multi-Agent Fleet
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


