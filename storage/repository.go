package storage

import (
	"bufio"
	"database/sql"
	"fmt"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"tokenmonitor/collector"
)

type AccountProfileDTO struct {
	ID                 int64     `json:"id"`
	Email              string    `json:"email"`
	AccountType        string    `json:"account_type"`
	PlanName           string    `json:"plan_name"`
	QuotaBandwidth     string    `json:"quota_bandwidth"`
	InstallationUUID   string    `json:"installation_uuid"`
	RegisteredAt       time.Time `json:"registered_at"`
	SubscriptionExpiry time.Time `json:"subscription_expiry"`
	DaysRemaining      int       `json:"days_remaining"`
	AutoRenew          bool      `json:"auto_renew"`
	TokenStatus        string    `json:"token_status"`
	TokenExpiresIn     string    `json:"token_expires_in"`
}

type SummaryMetricsDTO struct {
	TotalGrandTokens    int64   `json:"total_grand_tokens"`
	PromptTokens        int64   `json:"prompt_tokens"`
	OutputTokens        int64   `json:"output_tokens"`
	ThinkingTokens      int64   `json:"thinking_tokens"`
	CachedTokens        int64   `json:"cached_tokens"`
	TotalCalls          int64   `json:"total_calls"`
	TokensPerMinRate    float64 `json:"tokens_per_min_rate"`
	CacheHitPercent     float64 `json:"cache_hit_percent"`
	AvgTokensPerCall    float64 `json:"avg_tokens_per_call"`
	EstimatedCostUSD    float64 `json:"estimated_cost_usd"`
	EstimatedSavingsUSD float64 `json:"estimated_savings_usd"`
	EquivalentGrossUSD  float64 `json:"equivalent_gross_usd"`
}

type ChartPointDTO struct {
	TimeBucket     string  `json:"time_bucket"`
	PromptTokens   int64   `json:"prompt_tokens"`
	ThinkingTokens int64   `json:"thinking_tokens"`
	OutputTokens   int64   `json:"output_tokens"`
	CachedTokens   int64   `json:"cached_tokens"`
	TotalTokens    int64   `json:"total_tokens"`
	ModelCalls     int64   `json:"model_calls"`
	AvgLatencyMs   float64 `json:"avg_latency_ms"`
	UltraTokens    int64   `json:"ultra_tokens"`
	FlashTokens    int64   `json:"flash_tokens"`
	UltraCalls     int64   `json:"ultra_calls"`
	FlashCalls     int64   `json:"flash_calls"`
}

type DailySummaryDTO struct {
	Date                string  `json:"date"`
	TotalTokens         int64   `json:"total_tokens"`
	PromptTokens        int64   `json:"prompt_tokens"`
	OutputTokens        int64   `json:"output_tokens"`
	ThinkingTokens      int64   `json:"thinking_tokens"`
	CachedTokens        int64   `json:"cached_tokens"`
	TotalCalls          int64   `json:"total_calls"`
	AvgLatencyMs        float64 `json:"avg_latency_ms"`
	CacheHitPct         float64 `json:"cache_hit_pct"`
	UltraTokens         int64   `json:"ultra_tokens"`
	FlashTokens         int64   `json:"flash_tokens"`
	UltraCalls          int64   `json:"ultra_calls"`
	FlashCalls          int64   `json:"flash_calls"`
	EstimatedCostUSD    float64 `json:"estimated_cost_usd"`
	EstimatedSavingsUSD float64 `json:"estimated_savings_usd"`
}

type ModelDistributionDTO struct {
	ModelName        string  `json:"model_name"`
	TotalTokens      int64   `json:"total_tokens"`
	TotalCalls       int64   `json:"total_calls"`
	TokenPct         float64 `json:"token_pct"`
	CallPct          float64 `json:"call_pct"`
	EstimatedCostUSD float64 `json:"estimated_cost_usd"`
}

// CalculateTokensCostUSD tính toán chi phí quy đổi tương đương USD theo biểu giá chính thức Google Cloud Vertex AI, OpenAI & Anthropic Claude
func CalculateTokensCostUSD(modelName string, promptTokens, outputTokens, thinkingTokens, cachedTokens int64) (costUSD, savingsUSD float64) {
	nameLower := strings.ToLower(modelName)
	var promptRate, outputRate, cachedRate float64

	switch {
	// Anthropic Opus (High Reasoning Tier)
	case strings.Contains(nameLower, "opus"):
		promptRate = 15.00 / 1000000.0
		outputRate = 75.00 / 1000000.0
		cachedRate = 1.50 / 1000000.0

	// OpenAI o1 (High Reasoning Tier)
	case strings.Contains(nameLower, "o1") && !strings.Contains(nameLower, "mini"):
		promptRate = 15.00 / 1000000.0
		outputRate = 60.00 / 1000000.0
		cachedRate = 7.50 / 1000000.0

	// Anthropic Sonnet (claude-3-5-sonnet, claude-3-7-sonnet)
	case strings.Contains(nameLower, "sonnet"):
		promptRate = 3.00 / 1000000.0
		outputRate = 15.00 / 1000000.0
		cachedRate = 0.30 / 1000000.0

	// Google Ultra (Google AI Ultra Tier)
	case strings.Contains(nameLower, "ultra"):
		promptRate = 2.50 / 1000000.0
		outputRate = 10.00 / 1000000.0
		cachedRate = 0.625 / 1000000.0

	// OpenAI Flagship (gpt-4o, gpt-4.5)
	case strings.Contains(nameLower, "gpt-4o") && !strings.Contains(nameLower, "mini"):
		promptRate = 2.50 / 1000000.0
		outputRate = 10.00 / 1000000.0
		cachedRate = 1.25 / 1000000.0

	// Anthropic Haiku
	case strings.Contains(nameLower, "haiku"):
		promptRate = 0.80 / 1000000.0
		outputRate = 4.00 / 1000000.0
		cachedRate = 0.08 / 1000000.0

	// OpenAI o3-mini / o1-mini
	case strings.Contains(nameLower, "o3") || (strings.Contains(nameLower, "o1") && strings.Contains(nameLower, "mini")):
		promptRate = 1.10 / 1000000.0
		outputRate = 4.40 / 1000000.0
		cachedRate = 0.55 / 1000000.0

	// OpenAI gpt-4o-mini
	case strings.Contains(nameLower, "gpt-4o-mini"):
		promptRate = 0.15 / 1000000.0
		outputRate = 0.60 / 1000000.0
		cachedRate = 0.075 / 1000000.0

	// Google Pro / Frontier Codex / Pro Reasoning Tier (Gemini 1.5/2.5 Pro, cx/gpt-5.5, gpt-5.6-sol, gpt-5.1-codex)
	case strings.Contains(nameLower, "pro") || strings.Contains(nameLower, "gpt-5") || strings.Contains(nameLower, "codex"):
		promptRate = 1.25 / 1000000.0
		outputRate = 5.00 / 1000000.0
		cachedRate = 0.3125 / 1000000.0

	// Default: Flash Tier (Gemini 1.5/2.0/3.8 Flash, GPT-OSS, generic fallback)
	default:
		promptRate = 0.075 / 1000000.0
		outputRate = 0.30 / 1000000.0
		cachedRate = 0.01875 / 1000000.0
	}

	actualCost := (float64(promptTokens) * promptRate) +
		(float64(cachedTokens) * cachedRate) +
		(float64(outputTokens+thinkingTokens) * outputRate)

	savings := float64(cachedTokens) * (promptRate - cachedRate)
	if savings < 0 {
		savings = 0
	}
	return actualCost, savings
}

// InsertUsageBatch thực hiện ghi gom các event vào SQLite với single transaction
func (s *Storage) InsertUsageBatch(events []*collector.TokenUsageEvent) error {
	if len(events) == 0 {
		return nil
	}

	tx, err := s.DB.Begin()
	if err != nil {
		return fmt.Errorf("lỗi khởi tạo transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO token_usage_logs 
		(account_id, timestamp, model_name, prompt_tokens, output_tokens, thinking_tokens, cached_tokens, total_tokens, latency_ms, status_code, request_type)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("lỗi prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, e := range events {
		tsStr := e.Timestamp.UTC().Format("2006-01-02 15:04:05")
		_, err := stmt.Exec(
			e.AccountID,
			tsStr,
			e.ModelName,
			e.PromptTokens,
			e.OutputTokens,
			e.ThinkingTokens,
			e.CachedTokens,
			e.TotalTokens,
			e.LatencyMs,
			e.StatusCode,
			e.RequestType,
		)
		if err != nil {
			return fmt.Errorf("lỗi ghi dòng token: %w", err)
		}
	}

	return tx.Commit()
}

func parseFlexibleDate(str string) time.Time {
	str = strings.TrimSpace(str)
	if len(str) >= 10 {
		if t, err := time.Parse("2006-01-02", str[:10]); err == nil {
			return t
		}
	}
	layouts := []string{
		"2006-01-02 15:04:05 -0700 MST",
		"2006-01-02 15:04:05 +0000 UTC",
		"2006-01-02 15:04:05",
		"2006-01-02",
		time.RFC3339,
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, str); err == nil {
			return t
		}
	}
	return time.Now().AddDate(0, 1, 0)
}

// GetAccountProfile truy xuất thông tin tài khoản và đếm ngược TTL token
func (s *Storage) GetAccountProfile() (*AccountProfileDTO, error) {
	row := s.DB.QueryRow(`
		SELECT id, account_email, account_type, plan_name, quota_bandwidth, installation_uuid, registered_at, subscription_expiry, auto_renew
		FROM accounts
		LIMIT 1
	`)

	var dto AccountProfileDTO
	var regStr, expStr string
	err := row.Scan(
		&dto.ID, &dto.Email, &dto.AccountType, &dto.PlanName, &dto.QuotaBandwidth,
		&dto.InstallationUUID, &regStr, &expStr, &dto.AutoRenew,
	)
	if err != nil {
		return nil, err
	}

	dto.RegisteredAt = parseFlexibleDate(regStr)
	dto.SubscriptionExpiry = parseFlexibleDate(expStr)

	days := int(time.Until(dto.SubscriptionExpiry).Hours() / 24)
	if days < 0 {
		days = 0
	}
	dto.DaysRemaining = days

	// Lấy token expiry
	var status, tokenExpStr string
	err = s.DB.QueryRow(`
		SELECT token_status, token_expires_at 
		FROM auth_sessions 
		WHERE account_id = ? 
		ORDER BY id DESC LIMIT 1
	`, dto.ID).Scan(&status, &tokenExpStr)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}

	if status == "" {
		status = "VALID"
	}
	dto.TokenStatus = status

	tokenExp, _ := time.Parse("2006-01-02 15:04:05", tokenExpStr)
	if tokenExp.IsZero() {
		tokenExp, _ = time.Parse(time.RFC3339, tokenExpStr)
	}
	rem := time.Until(tokenExp)

	// Nếu phiên trong DB đã hết hạn, tự động kiểm tra phiên Antigravity IDE thực tế
	if rem <= 0 {
		if name, email, plan, err := DetectActiveAntigravityAccount(); err == nil && email != "" {
			now := time.Now()
			newExp, expErr := DetectOAuthTokenExpiry()
			if expErr != nil || newExp.IsZero() || newExp.Before(now) {
				newExp = now.Add(55 * time.Minute)
			}
			_, _ = s.DB.Exec(`
				INSERT INTO auth_sessions (account_id, token_status, token_issued_at, token_expires_at, last_validated_at)
				VALUES (?, 'VALID', ?, ?, ?)
			`, dto.ID, now, newExp, now)
			status = "VALID"
			rem = time.Until(newExp)
			if name != "" {
				dto.PlanName = fmt.Sprintf("%s (%s)", plan, name)
			} else {
				dto.PlanName = plan
			}
		}
	}

	if rem <= 0 {
		dto.TokenExpiresIn = "Expired (0m0s)"
		dto.TokenStatus = "EXPIRED"
	} else {
		dto.TokenStatus = "VALID"
		m := int(rem.Minutes())
		sec := int(rem.Seconds()) % 60
		dto.TokenExpiresIn = fmt.Sprintf("Valid (Expires in %dm%ds)", m, sec)
	}

	return &dto, nil
}

// GetSummaryMetrics trả về số liệu tổng quan toàn bộ
func (s *Storage) GetSummaryMetrics() (*SummaryMetricsDTO, error) {
	return s.GetSummaryMetricsByRange("all")
}

// GetSummaryMetricsByRange trả về số liệu tổng quan theo khoảng thời gian
func (s *Storage) GetSummaryMetricsByRange(timeRange string) (*SummaryMetricsDTO, error) {
	timeRange = strings.ToLower(strings.TrimSpace(timeRange))
	whereClause := ""
	if timeRange == "today" {
		whereClause = "WHERE timestamp >= datetime('now', 'localtime', 'start of day', 'utc')"
	} else if timeRange == "24h" || timeRange == "1d" {
		whereClause = "WHERE timestamp >= datetime('now', '-24 hours')"
	} else if timeRange == "7d" {
		whereClause = "WHERE timestamp >= datetime('now', '-7 days')"
	} else if timeRange == "30d" || timeRange == "month" {
		whereClause = "WHERE timestamp >= datetime('now', '-30 days')"
	}

	query := fmt.Sprintf(`
		SELECT 
			COALESCE(SUM(total_tokens), 0),
			COALESCE(SUM(prompt_tokens), 0),
			COALESCE(SUM(output_tokens), 0),
			COALESCE(SUM(thinking_tokens), 0),
			COALESCE(SUM(cached_tokens), 0),
			COUNT(*)
		FROM token_usage_logs
		%s
	`, whereClause)

	row := s.DB.QueryRow(query)
	var dto SummaryMetricsDTO
	err := row.Scan(
		&dto.TotalGrandTokens,
		&dto.PromptTokens,
		&dto.OutputTokens,
		&dto.ThinkingTokens,
		&dto.CachedTokens,
		&dto.TotalCalls,
	)
	if err != nil {
		return nil, err
	}

	if dto.TotalCalls > 0 {
		dto.AvgTokensPerCall = float64(dto.TotalGrandTokens) / float64(dto.TotalCalls)
	}

	totalPromptPlusCached := dto.PromptTokens + dto.CachedTokens
	if totalPromptPlusCached > 0 {
		dto.CacheHitPercent = (float64(dto.CachedTokens) / float64(totalPromptPlusCached)) * 100.0
	}

	// Tính tokens/min dựa trên khoảng thời gian có log
	var minTimeStr, maxTimeStr sql.NullString
	timeQuery := fmt.Sprintf("SELECT MIN(timestamp), MAX(timestamp) FROM token_usage_logs %s", whereClause)
	_ = s.DB.QueryRow(timeQuery).Scan(&minTimeStr, &maxTimeStr)
	if minTimeStr.Valid && maxTimeStr.Valid {
		t1, _ := time.Parse("2006-01-02 15:04:05", minTimeStr.String)
		t2, _ := time.Parse("2006-01-02 15:04:05", maxTimeStr.String)
		diffMins := t2.Sub(t1).Minutes()
		if diffMins > 0 {
			dto.TokensPerMinRate = float64(dto.TotalGrandTokens) / diffMins
		} else {
			dto.TokensPerMinRate = float64(dto.TotalGrandTokens)
		}
	}

	// Tính toán chi phí quy đổi tương đương USD & số tiền tiết kiệm được từ Context Cache
	var totalCostUSD, totalSavingsUSD float64
	costQuery := fmt.Sprintf(`
		SELECT 
			model_name,
			COALESCE(SUM(prompt_tokens), 0),
			COALESCE(SUM(output_tokens), 0),
			COALESCE(SUM(thinking_tokens), 0),
			COALESCE(SUM(cached_tokens), 0)
		FROM token_usage_logs
		%s
		GROUP BY model_name
	`, whereClause)
	costRows, err := s.DB.Query(costQuery)
	if err == nil {
		defer costRows.Close()
		for costRows.Next() {
			var mName string
			var pTok, oTok, tTok, cTok int64
			if err := costRows.Scan(&mName, &pTok, &oTok, &tTok, &cTok); err == nil {
				c, sav := CalculateTokensCostUSD(mName, pTok, oTok, tTok, cTok)
				totalCostUSD += c
				totalSavingsUSD += sav
			}
		}
	}
	dto.EstimatedCostUSD = totalCostUSD
	dto.EstimatedSavingsUSD = totalSavingsUSD
	dto.EquivalentGrossUSD = totalCostUSD + totalSavingsUSD

	return &dto, nil
}

// GetTimeSeriesData lấy dữ liệu theo từng block 5 phút để vẽ biểu đồ
func (s *Storage) GetTimeSeriesData(limit int) ([]ChartPointDTO, error) {
	return s.GetTimeSeriesDataByRange("24h")
}

// GetTimeSeriesDataByRange lấy chuỗi thời gian theo khoảng lựa chọn (24h, 7d, 30d, all)
func (s *Storage) GetTimeSeriesDataByRange(timeRange string) ([]ChartPointDTO, error) {
	timeRange = strings.ToLower(strings.TrimSpace(timeRange))
	var query string

	if timeRange == "today" {
		query = `
			SELECT 
				strftime('%Y-%m-%d %H:', timestamp) || 
				substr('000510152025303540455055', (cast(strftime('%M', timestamp) as integer) / 5) * 2 + 1, 2) AS bucket,
				COALESCE(SUM(prompt_tokens), 0),
				COALESCE(SUM(thinking_tokens), 0),
				COALESCE(SUM(output_tokens), 0),
				COALESCE(SUM(cached_tokens), 0),
				COALESCE(SUM(total_tokens), 0),
				COUNT(*),
				COALESCE(AVG(latency_ms), 0),
				COALESCE(SUM(CASE WHEN LOWER(model_name) LIKE '%ultra%' THEN total_tokens ELSE 0 END), 0),
				COALESCE(SUM(CASE WHEN LOWER(model_name) LIKE '%flash%' OR LOWER(model_name) LIKE '%pro%' THEN total_tokens ELSE 0 END), 0),
				COALESCE(SUM(CASE WHEN LOWER(model_name) LIKE '%ultra%' THEN 1 ELSE 0 END), 0),
				COALESCE(SUM(CASE WHEN LOWER(model_name) LIKE '%flash%' OR LOWER(model_name) LIKE '%pro%' THEN 1 ELSE 0 END), 0)
			FROM token_usage_logs
			WHERE timestamp >= datetime('now', 'localtime', 'start of day', 'utc')
			GROUP BY bucket
			ORDER BY bucket ASC
		`
	} else if timeRange == "24h" || timeRange == "1d" {
		query = `
			SELECT 
				strftime('%Y-%m-%d %H:', timestamp) || 
				substr('000510152025303540455055', (cast(strftime('%M', timestamp) as integer) / 5) * 2 + 1, 2) AS bucket,
				COALESCE(SUM(prompt_tokens), 0),
				COALESCE(SUM(thinking_tokens), 0),
				COALESCE(SUM(output_tokens), 0),
				COALESCE(SUM(cached_tokens), 0),
				COALESCE(SUM(total_tokens), 0),
				COUNT(*),
				COALESCE(AVG(latency_ms), 0),
				COALESCE(SUM(CASE WHEN LOWER(model_name) LIKE '%ultra%' THEN total_tokens ELSE 0 END), 0),
				COALESCE(SUM(CASE WHEN LOWER(model_name) LIKE '%flash%' OR LOWER(model_name) LIKE '%pro%' THEN total_tokens ELSE 0 END), 0),
				COALESCE(SUM(CASE WHEN LOWER(model_name) LIKE '%ultra%' THEN 1 ELSE 0 END), 0),
				COALESCE(SUM(CASE WHEN LOWER(model_name) LIKE '%flash%' OR LOWER(model_name) LIKE '%pro%' THEN 1 ELSE 0 END), 0)
			FROM token_usage_logs
			WHERE timestamp >= datetime('now', '-24 hours')
			GROUP BY bucket
			ORDER BY bucket ASC
		`
	} else if timeRange == "7d" {
		query = `
			SELECT 
				strftime('%Y-%m-%d', timestamp) AS bucket,
				COALESCE(SUM(prompt_tokens), 0),
				COALESCE(SUM(thinking_tokens), 0),
				COALESCE(SUM(output_tokens), 0),
				COALESCE(SUM(cached_tokens), 0),
				COALESCE(SUM(total_tokens), 0),
				COUNT(*),
				COALESCE(AVG(latency_ms), 0),
				COALESCE(SUM(CASE WHEN LOWER(model_name) LIKE '%ultra%' THEN total_tokens ELSE 0 END), 0),
				COALESCE(SUM(CASE WHEN LOWER(model_name) LIKE '%flash%' OR LOWER(model_name) LIKE '%pro%' THEN total_tokens ELSE 0 END), 0),
				COALESCE(SUM(CASE WHEN LOWER(model_name) LIKE '%ultra%' THEN 1 ELSE 0 END), 0),
				COALESCE(SUM(CASE WHEN LOWER(model_name) LIKE '%flash%' OR LOWER(model_name) LIKE '%pro%' THEN 1 ELSE 0 END), 0)
			FROM token_usage_logs
			WHERE timestamp >= datetime('now', '-7 days')
			GROUP BY bucket
			ORDER BY bucket ASC
		`
	} else if timeRange == "all" {
		query = `
			SELECT 
				strftime('%Y-%m-%d', timestamp) AS bucket,
				COALESCE(SUM(prompt_tokens), 0),
				COALESCE(SUM(thinking_tokens), 0),
				COALESCE(SUM(output_tokens), 0),
				COALESCE(SUM(cached_tokens), 0),
				COALESCE(SUM(total_tokens), 0),
				COUNT(*),
				COALESCE(AVG(latency_ms), 0),
				COALESCE(SUM(CASE WHEN LOWER(model_name) LIKE '%ultra%' THEN total_tokens ELSE 0 END), 0),
				COALESCE(SUM(CASE WHEN LOWER(model_name) LIKE '%flash%' OR LOWER(model_name) LIKE '%pro%' THEN total_tokens ELSE 0 END), 0),
				COALESCE(SUM(CASE WHEN LOWER(model_name) LIKE '%ultra%' THEN 1 ELSE 0 END), 0),
				COALESCE(SUM(CASE WHEN LOWER(model_name) LIKE '%flash%' OR LOWER(model_name) LIKE '%pro%' THEN 1 ELSE 0 END), 0)
			FROM token_usage_logs
			GROUP BY bucket
			ORDER BY bucket ASC
		`
	} else {
		// Mặc định là 30 ngày (cả tháng)
		query = `
			SELECT 
				strftime('%Y-%m-%d', timestamp) AS bucket,
				COALESCE(SUM(prompt_tokens), 0),
				COALESCE(SUM(thinking_tokens), 0),
				COALESCE(SUM(output_tokens), 0),
				COALESCE(SUM(cached_tokens), 0),
				COALESCE(SUM(total_tokens), 0),
				COUNT(*),
				COALESCE(AVG(latency_ms), 0),
				COALESCE(SUM(CASE WHEN LOWER(model_name) LIKE '%ultra%' THEN total_tokens ELSE 0 END), 0),
				COALESCE(SUM(CASE WHEN LOWER(model_name) LIKE '%flash%' OR LOWER(model_name) LIKE '%pro%' THEN total_tokens ELSE 0 END), 0),
				COALESCE(SUM(CASE WHEN LOWER(model_name) LIKE '%ultra%' THEN 1 ELSE 0 END), 0),
				COALESCE(SUM(CASE WHEN LOWER(model_name) LIKE '%flash%' OR LOWER(model_name) LIKE '%pro%' THEN 1 ELSE 0 END), 0)
			FROM token_usage_logs
			WHERE timestamp >= datetime('now', '-30 days')
			GROUP BY bucket
			ORDER BY bucket ASC
		`
	}

	rows, err := s.DB.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []ChartPointDTO
	for rows.Next() {
		var p ChartPointDTO
		if err := rows.Scan(
			&p.TimeBucket,
			&p.PromptTokens,
			&p.ThinkingTokens,
			&p.OutputTokens,
			&p.CachedTokens,
			&p.TotalTokens,
			&p.ModelCalls,
			&p.AvgLatencyMs,
			&p.UltraTokens,
			&p.FlashTokens,
			&p.UltraCalls,
			&p.FlashCalls,
		); err != nil {
			return nil, err
		}
		points = append(points, p)
	}

	if points == nil {
		points = []ChartPointDTO{}
	}

	return points, nil
}

// GetDailySummaries truy xuất bảng tổng hợp chi tiết theo số ngày (hỗ trợ tương thích ngược)
func (s *Storage) GetDailySummaries(days int) ([]DailySummaryDTO, error) {
	if days <= 0 {
		days = 30
	}
	return s.GetDailySummariesByRange(fmt.Sprintf("%dd", days))
}

// GetDailySummariesByRange truy xuất bảng tổng hợp chi tiết theo khung thời gian (24h, 7d, 30d, all)
func (s *Storage) GetDailySummariesByRange(timeRange string) ([]DailySummaryDTO, error) {
	timeRange = strings.ToLower(strings.TrimSpace(timeRange))
	var dayExpr string
	var whereClause string

	if timeRange == "today" {
		// Trong chế độ hôm nay, phân bổ chi tiết theo từng mốc giờ (hourly) từ đầu ngày
		dayExpr = "strftime('%Y-%m-%d %H:00', timestamp)"
		whereClause = "WHERE timestamp >= datetime('now', 'localtime', 'start of day', 'utc')"
	} else if timeRange == "24h" || timeRange == "1d" {
		// Trong chế độ 24 giờ, phân bổ chi tiết theo từng mốc giờ (hourly)
		dayExpr = "strftime('%Y-%m-%d %H:00', timestamp)"
		whereClause = "WHERE timestamp >= datetime('now', '-24 hours')"
	} else if timeRange == "7d" {
		dayExpr = "strftime('%Y-%m-%d', timestamp)"
		whereClause = "WHERE timestamp >= datetime('now', '-7 days')"
	} else if timeRange == "all" {
		dayExpr = "strftime('%Y-%m-%d', timestamp)"
		whereClause = ""
	} else {
		// Mặc định 30 ngày
		dayExpr = "strftime('%Y-%m-%d', timestamp)"
		whereClause = "WHERE timestamp >= datetime('now', '-30 days')"
	}

	query := fmt.Sprintf(`
		SELECT 
			%s AS day,
			COALESCE(SUM(total_tokens), 0),
			COALESCE(SUM(prompt_tokens), 0),
			COALESCE(SUM(output_tokens), 0),
			COALESCE(SUM(thinking_tokens), 0),
			COALESCE(SUM(cached_tokens), 0),
			COUNT(*),
			COALESCE(AVG(latency_ms), 0),
			COALESCE(SUM(CASE WHEN LOWER(model_name) LIKE '%%ultra%%' THEN total_tokens ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN LOWER(model_name) NOT LIKE '%%ultra%%' THEN total_tokens ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN LOWER(model_name) LIKE '%%ultra%%' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN LOWER(model_name) NOT LIKE '%%ultra%%' THEN 1 ELSE 0 END), 0)
		FROM token_usage_logs
		%s
		GROUP BY day
		ORDER BY day DESC
	`, dayExpr, whereClause)

	rows, err := s.DB.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Tính toán chi phí USD động theo từng model từ token_usage_logs mà không ép giá non-Ultra về Flash
	type dayCost struct {
		cost    float64
		savings float64
	}
	dayCostMap := make(map[string]dayCost)

	costQuery := fmt.Sprintf(`
		SELECT 
			%s AS day,
			model_name,
			COALESCE(SUM(prompt_tokens), 0),
			COALESCE(SUM(output_tokens), 0),
			COALESCE(SUM(thinking_tokens), 0),
			COALESCE(SUM(cached_tokens), 0)
		FROM token_usage_logs
		%s
		GROUP BY day, model_name
	`, dayExpr, whereClause)

	costRows, err := s.DB.Query(costQuery)
	if err == nil {
		defer costRows.Close()
		for costRows.Next() {
			var dStr, mName string
			var pTok, oTok, tTok, cTok int64
			if err := costRows.Scan(&dStr, &mName, &pTok, &oTok, &tTok, &cTok); err == nil {
				c, s := CalculateTokensCostUSD(mName, pTok, oTok, tTok, cTok)
				entry := dayCostMap[dStr]
				entry.cost += c
				entry.savings += s
				dayCostMap[dStr] = entry
			}
		}
	}

	var list []DailySummaryDTO
	for rows.Next() {
		var d DailySummaryDTO
		if err := rows.Scan(
			&d.Date,
			&d.TotalTokens,
			&d.PromptTokens,
			&d.OutputTokens,
			&d.ThinkingTokens,
			&d.CachedTokens,
			&d.TotalCalls,
			&d.AvgLatencyMs,
			&d.UltraTokens,
			&d.FlashTokens,
			&d.UltraCalls,
			&d.FlashCalls,
		); err != nil {
			return nil, err
		}
		if d.PromptTokens+d.CachedTokens > 0 {
			d.CacheHitPct = (float64(d.CachedTokens) / float64(d.PromptTokens+d.CachedTokens)) * 100.0
		}

		if dc, ok := dayCostMap[d.Date]; ok {
			d.EstimatedCostUSD = dc.cost
			d.EstimatedSavingsUSD = dc.savings
		}

		list = append(list, d)
	}
	if list == nil {
		list = []DailySummaryDTO{}
	}
	return list, nil
}

// GetModelDistribution lấy tỷ lệ phân bổ các dòng Model theo khung thời gian (24h, 7d, 30d, all)
func (s *Storage) GetModelDistribution(timeRange string) ([]ModelDistributionDTO, error) {
	timeRange = strings.ToLower(strings.TrimSpace(timeRange))
	whereClause := ""
	if timeRange == "today" {
		whereClause = "WHERE timestamp >= datetime('now', 'localtime', 'start of day', 'utc')"
	} else if timeRange == "24h" || timeRange == "1d" {
		whereClause = "WHERE timestamp >= datetime('now', '-24 hours')"
	} else if timeRange == "7d" {
		whereClause = "WHERE timestamp >= datetime('now', '-7 days')"
	} else if timeRange == "30d" || timeRange == "month" {
		whereClause = "WHERE timestamp >= datetime('now', '-30 days')"
	}

	query := fmt.Sprintf(`
		SELECT 
			model_name,
			COALESCE(SUM(total_tokens), 0) AS sum_tokens,
			COUNT(*) AS call_count,
			COALESCE(SUM(prompt_tokens), 0) AS sum_prompt,
			COALESCE(SUM(output_tokens), 0) AS sum_output,
			COALESCE(SUM(thinking_tokens), 0) AS sum_thinking,
			COALESCE(SUM(cached_tokens), 0) AS sum_cached
		FROM token_usage_logs
		%s
		GROUP BY model_name
		ORDER BY sum_tokens DESC
	`, whereClause)

	rows, err := s.DB.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	merged := make(map[string]*ModelDistributionDTO)
	var grandTokens int64
	var grandCalls int64

	for rows.Next() {
		var rawName string
		var tokens, calls, pTok, oTok, tTok, cTok int64
		if err := rows.Scan(&rawName, &tokens, &calls, &pTok, &oTok, &tTok, &cTok); err != nil {
			return nil, err
		}
		normName := collector.NormalizeModelName(rawName)
		costUSD, _ := CalculateTokensCostUSD(normName, pTok, oTok, tTok, cTok)
		if existing, ok := merged[normName]; ok {
			existing.TotalTokens += tokens
			existing.TotalCalls += calls
			existing.EstimatedCostUSD += costUSD
		} else {
			merged[normName] = &ModelDistributionDTO{
				ModelName:        normName,
				TotalTokens:      tokens,
				TotalCalls:       calls,
				EstimatedCostUSD: costUSD,
			}
		}
		grandTokens += tokens
		grandCalls += calls
	}

	var list []ModelDistributionDTO
	for _, m := range merged {
		if grandTokens > 0 {
			m.TokenPct = (float64(m.TotalTokens) / float64(grandTokens)) * 100.0
		}
		if grandCalls > 0 {
			m.CallPct = (float64(m.TotalCalls) / float64(grandCalls)) * 100.0
		}
		list = append(list, *m)
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i].TotalTokens > list[j].TotalTokens
	})

	return list, nil
}

type ModelTimeSeriesPointDTO struct {
	TimeBucket     string  `json:"time_bucket"`
	ModelName      string  `json:"model_name"`
	TotalTokens    int64   `json:"total_tokens"`
	PromptTokens   int64   `json:"prompt_tokens"`
	OutputTokens   int64   `json:"output_tokens"`
	ThinkingTokens int64   `json:"thinking_tokens"`
	CachedTokens   int64   `json:"cached_tokens"`
	CallCount      int64   `json:"call_count"`
	AvgLatencyMs   float64 `json:"avg_latency_ms"`
}

// GetModelTimeSeries trả về chuỗi dữ liệu theo thời gian phân tách từng Model
func (s *Storage) GetModelTimeSeries(timeRange string, modelFilter string) ([]ModelTimeSeriesPointDTO, error) {
	timeRange = strings.ToLower(strings.TrimSpace(timeRange))
	whereClause := ""
	if timeRange == "today" {
		whereClause = "WHERE timestamp >= datetime('now', 'localtime', 'start of day', 'utc')"
	} else if timeRange == "24h" || timeRange == "1d" {
		whereClause = "WHERE timestamp >= datetime('now', '-24 hours')"
	} else if timeRange == "7d" {
		whereClause = "WHERE timestamp >= datetime('now', '-7 days')"
	} else if timeRange == "all" {
		whereClause = ""
	} else {
		// 30d
		whereClause = "WHERE timestamp >= datetime('now', '-30 days')"
	}

	var args []any
	if modelFilter != "" && !strings.EqualFold(modelFilter, "all") {
		normFilter := collector.NormalizeModelName(modelFilter)
		if whereClause == "" {
			whereClause = "WHERE (model_name = ? OR LOWER(model_name) LIKE ?)"
		} else {
			whereClause += " AND (model_name = ? OR LOWER(model_name) LIKE ?)"
		}
		args = append(args, modelFilter, "%"+strings.ToLower(normFilter)+"%")
	}

	var bucketExpr string
	if timeRange == "today" || timeRange == "24h" || timeRange == "1d" {
		bucketExpr = `strftime('%Y-%m-%d %H:', timestamp) || substr('000510152025303540455055', (cast(strftime('%M', timestamp) as integer) / 5) * 2 + 1, 2)`
	} else {
		bucketExpr = `strftime('%Y-%m-%d', timestamp)`
	}

	query := fmt.Sprintf(`
		SELECT 
			%s AS bucket,
			model_name,
			COALESCE(SUM(total_tokens), 0),
			COALESCE(SUM(prompt_tokens), 0),
			COALESCE(SUM(output_tokens), 0),
			COALESCE(SUM(thinking_tokens), 0),
			COALESCE(SUM(cached_tokens), 0),
			COUNT(*),
			COALESCE(AVG(latency_ms), 0)
		FROM token_usage_logs
		%s
		GROUP BY bucket, model_name
		ORDER BY bucket ASC
	`, bucketExpr, whereClause)

	rows, err := s.DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type bucketModelKey struct {
		bucket string
		model  string
	}
	agg := make(map[bucketModelKey]*ModelTimeSeriesPointDTO)
	var orderedKeys []bucketModelKey

	for rows.Next() {
		var p ModelTimeSeriesPointDTO
		var rawModel string
		if err := rows.Scan(
			&p.TimeBucket,
			&rawModel,
			&p.TotalTokens,
			&p.PromptTokens,
			&p.OutputTokens,
			&p.ThinkingTokens,
			&p.CachedTokens,
			&p.CallCount,
			&p.AvgLatencyMs,
		); err != nil {
			return nil, err
		}
		p.ModelName = collector.NormalizeModelName(rawModel)
		key := bucketModelKey{bucket: p.TimeBucket, model: p.ModelName}
		if existing, ok := agg[key]; ok {
			existing.TotalTokens += p.TotalTokens
			existing.PromptTokens += p.PromptTokens
			existing.OutputTokens += p.OutputTokens
			existing.ThinkingTokens += p.ThinkingTokens
			existing.CachedTokens += p.CachedTokens
			existing.CallCount += p.CallCount
		} else {
			clone := p
			agg[key] = &clone
			orderedKeys = append(orderedKeys, key)
		}
	}

	var points []ModelTimeSeriesPointDTO
	for _, k := range orderedKeys {
		points = append(points, *agg[k])
	}
	return points, nil
}

// SeedInitialDataIfEmpty là no-op, toàn bộ dữ liệu được thu thập 100% tự động từ transcript thực tế
func (s *Storage) SeedInitialDataIfEmpty() error {
	return nil
}

// EnsureMonthlyHistory là no-op, bảo toàn 100% dữ liệu lịch sử thực tế từ các file transcript
func (s *Storage) EnsureMonthlyHistory() error {
	return nil
}

// RollupHourlyMetrics tính toán và tổng hợp dữ liệu từ token_usage_logs sang token_usage_hourly_rollup
func (s *Storage) RollupHourlyMetrics() error {
	query := `
		INSERT INTO token_usage_hourly_rollup (
			account_id, time_bucket, model_name, call_count,
			sum_prompt_tokens, sum_output_tokens, sum_thinking_tokens,
			sum_cached_tokens, sum_total_tokens, avg_latency_ms
		)
		SELECT 
			account_id,
			strftime('%Y-%m-%d %H:00:00', timestamp) AS time_bucket,
			model_name,
			COUNT(*) AS call_count,
			COALESCE(SUM(prompt_tokens), 0) AS sum_prompt_tokens,
			COALESCE(SUM(output_tokens), 0) AS sum_output_tokens,
			COALESCE(SUM(thinking_tokens), 0) AS sum_thinking_tokens,
			COALESCE(SUM(cached_tokens), 0) AS sum_cached_tokens,
			COALESCE(SUM(total_tokens), 0) AS sum_total_tokens,
			COALESCE(AVG(latency_ms), 0.0) AS avg_latency_ms
		FROM token_usage_logs
		GROUP BY account_id, strftime('%Y-%m-%d %H:00:00', timestamp), model_name
		ON CONFLICT(account_id, time_bucket, model_name) DO UPDATE SET
			call_count = excluded.call_count,
			sum_prompt_tokens = excluded.sum_prompt_tokens,
			sum_output_tokens = excluded.sum_output_tokens,
			sum_thinking_tokens = excluded.sum_thinking_tokens,
			sum_cached_tokens = excluded.sum_cached_tokens,
			sum_total_tokens = excluded.sum_total_tokens,
			avg_latency_ms = excluded.avg_latency_ms;
	`
	_, err := s.DB.Exec(query)
	return err
}

// ══════════════════════════════════════════════════════════════════════════
//  MULTI-AGENT ORCHESTRATION & FLEET TELEMETRY
// ══════════════════════════════════════════════════════════════════════════

type AgentFleetSummaryDTO struct {
	TotalFleet          int     `json:"total_fleet"`
	ActiveConcurrency   int     `json:"active_concurrency"`
	CapacityCeiling     int     `json:"capacity_ceiling"`
	ConcurrencyStatus   string  `json:"concurrency_status"`
	TaskSuccessRate     float64 `json:"task_success_rate"`
	TotalTasksCompleted int     `json:"total_tasks_completed"`
	OffloadedTokens     int64   `json:"offloaded_tokens"`
	OffloadedPercent    float64 `json:"offloaded_percent"`
	ActiveRolesCount    int     `json:"active_roles_count"`
	TotalSessionsCount  int     `json:"total_sessions_count"`
}

type AgentConcurrencyPointDTO struct {
	Timestamp       string `json:"timestamp"`
	ActiveWorkers   int    `json:"active_workers"`
	CapacityCeiling int    `json:"capacity_ceiling"`
	OffloadedTokens int64  `json:"offloaded_tokens"`
}

type AgentGanttTaskDTO = collector.AgentTaskEvent

// InsertAgentTaskBatch lưu trữ hàng loạt tác vụ của Subagent thu thập từ transcript vào SQLite
func (s *Storage) InsertAgentTaskBatch(tasks []*collector.AgentTaskEvent) error {
	if len(tasks) == 0 {
		return nil
	}

	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO agent_fleet_telemetry 
		(account_id, subagent_id, role_name, task_name, status, started_at, finished_at, duration_ms, tokens_used, tokens_offloaded)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(subagent_id) DO UPDATE SET
			role_name = excluded.role_name,
			task_name = excluded.task_name,
			status = excluded.status,
			started_at = excluded.started_at,
			finished_at = excluded.finished_at,
			duration_ms = excluded.duration_ms,
			tokens_used = excluded.tokens_used,
			tokens_offloaded = excluded.tokens_offloaded
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, t := range tasks {
		var finStr sql.NullString
		if t.FinishedAt != "" {
			finStr = sql.NullString{String: t.FinishedAt, Valid: true}
		}
		_, err := stmt.Exec(
			1,
			t.SubagentID,
			t.RoleName,
			t.TaskName,
			t.Status,
			t.StartedAt,
			finStr,
			t.DurationMs,
			t.TokensOffloaded,
			t.TokensOffloaded,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// SeedAgentFleetIfEmpty là no-op, toàn bộ tác vụ của Agent Fleet được thu thập từ transcript thực tế
func (s *Storage) SeedAgentFleetIfEmpty() error {
	return nil
}

// parseAgentTimeCondition chuyển đổi timeRange (today, 24h, 7d, 30d, all) thành biểu thức SQL SQLite
func parseAgentTimeCondition(timeRange string, timeCol string) string {
	switch strings.ToLower(strings.TrimSpace(timeRange)) {
	case "today":
		return fmt.Sprintf("%s >= datetime('now', 'localtime', 'start of day', 'utc')", timeCol)
	case "24h", "1d":
		return fmt.Sprintf("%s >= datetime('now', '-24 hours')", timeCol)
	case "7d":
		return fmt.Sprintf("%s >= datetime('now', '-7 days')", timeCol)
	case "30d", "month":
		return fmt.Sprintf("%s >= datetime('now', '-30 days')", timeCol)
	default:
		return ""
	}
}

// GetAgentFleetSummary trả về 4 chỉ số tổng hợp tính toán 100% từ dữ liệu thực tế theo khung thời gian
func (s *Storage) GetAgentFleetSummary(timeRange ...string) (*AgentFleetSummaryDTO, error) {
	// Tự động chuyển các tác vụ đã kết thúc sang COMPLETED nếu đã quá 2 phút không còn hoạt động
	_, _ = s.DB.Exec(`
		UPDATE agent_fleet_telemetry 
		SET status = 'COMPLETED',
		    finished_at = COALESCE(finished_at, datetime(started_at, '+' || MAX(duration_ms/1000, 2) || ' seconds'))
		WHERE status = 'RUNNING' 
		  AND started_at < datetime('now', '-2 minutes')
	`)

	dto := &AgentFleetSummaryDTO{
		CapacityCeiling: 5,
	}

	rangeStr := ""
	if len(timeRange) > 0 {
		rangeStr = strings.ToLower(strings.TrimSpace(timeRange[0]))
	}

	timeCond := parseAgentTimeCondition(rangeStr, "started_at")
	whereClause := ""
	if timeCond != "" {
		whereClause = "WHERE " + timeCond
	}

	var totalFleet int
	var runningCount int
	var sumOffloaded int64
	var distinctRoles int
	var completedTasks int
	var totalTasks int

	summaryQuery := fmt.Sprintf(`
		SELECT 
			COUNT(DISTINCT subagent_id),
			COALESCE(SUM(CASE WHEN status = 'RUNNING' AND started_at >= datetime('now', '-2 minutes') THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(tokens_offloaded), 0),
			COUNT(DISTINCT role_name),
			COALESCE(SUM(CASE WHEN status = 'COMPLETED' THEN 1 ELSE 0 END), 0),
			COUNT(*)
		FROM agent_fleet_telemetry
		%s
	`, whereClause)

	_ = s.DB.QueryRow(summaryQuery).Scan(&totalFleet, &runningCount, &sumOffloaded, &distinctRoles, &completedTasks, &totalTasks)

	dto.TotalFleet = totalFleet
	dto.ActiveConcurrency = runningCount
	dto.OffloadedTokens = sumOffloaded
	dto.ActiveRolesCount = distinctRoles
	dto.TotalSessionsCount = totalFleet
	dto.TotalTasksCompleted = completedTasks

	if totalTasks > 0 {
		dto.TaskSuccessRate = float64(completedTasks) * 100.0 / float64(totalTasks)
	} else {
		dto.TaskSuccessRate = 100.0
	}

	// Tính tỷ lệ OffloadedPercent so với TotalGrandTokens trong token_usage_logs thực tế trong cùng khung giờ
	var totalGrandTokens int64
	tokenLogWhere := ""
	tokenLogCond := parseAgentTimeCondition(rangeStr, "timestamp")
	if tokenLogCond != "" {
		tokenLogWhere = "WHERE " + tokenLogCond
	}
	_ = s.DB.QueryRow(fmt.Sprintf("SELECT COALESCE(SUM(total_tokens), 0) FROM token_usage_logs %s", tokenLogWhere)).Scan(&totalGrandTokens)
	if totalGrandTokens > 0 && sumOffloaded > 0 {
		dto.OffloadedPercent = float64(sumOffloaded) * 100.0 / float64(totalGrandTokens)
		if dto.OffloadedPercent > 100.0 {
			dto.OffloadedPercent = 100.0
		}
	} else {
		dto.OffloadedPercent = 0.0
	}

	// Trạng thái ConcurrencyStatus dựa trên tải thực tế
	if runningCount == 0 {
		dto.ConcurrencyStatus = "Fleet Standing By • Ready"
	} else if runningCount <= 3 {
		dto.ConcurrencyStatus = "Safe Load • 0 Throttling"
	} else if runningCount <= 5 {
		dto.ConcurrencyStatus = "Optimal Parallelism • Active"
	} else {
		dto.CapacityCeiling = runningCount + 2
		dto.ConcurrencyStatus = "Peak Concurrency • Heavy Load"
	}

	return dto, nil
}

// GetAgentConcurrencyTimeline trả về chuỗi dữ liệu độ song song thực tế từ bảng agent_fleet_telemetry
func (s *Storage) GetAgentConcurrencyTimeline(timeRange ...string) ([]AgentConcurrencyPointDTO, error) {
	var points []AgentConcurrencyPointDTO
	rangeStr := ""
	if len(timeRange) > 0 {
		rangeStr = strings.ToLower(strings.TrimSpace(timeRange[0]))
	}

	timeFormat := "%Y-%m-%d %H:00"
	timeCond := "started_at >= datetime('now', '-7 days')"
	if rangeStr == "today" {
		timeFormat = "%Y-%m-%d %H:00"
		timeCond = "started_at >= datetime('now', 'localtime', 'start of day', 'utc')"
	} else if rangeStr == "24h" || rangeStr == "1d" {
		timeFormat = "%Y-%m-%d %H:00"
		timeCond = "started_at >= datetime('now', '-24 hours')"
	} else if rangeStr == "7d" {
		timeFormat = "%Y-%m-%d %H:00"
		timeCond = "started_at >= datetime('now', '-7 days')"
	} else if rangeStr == "30d" || rangeStr == "month" {
		timeFormat = "%Y-%m-%d"
		timeCond = "started_at >= datetime('now', '-30 days')"
	} else if rangeStr == "all" {
		timeFormat = "%Y-%m-%d"
		timeCond = "1=1"
	}

	query := fmt.Sprintf(`
		SELECT 
			strftime('%s', started_at) AS time_bucket,
			COUNT(DISTINCT subagent_id) AS workers,
			COALESCE(SUM(tokens_offloaded), 0) AS offloaded
		FROM agent_fleet_telemetry
		WHERE %s
		GROUP BY time_bucket
		ORDER BY time_bucket ASC
	`, timeFormat, timeCond)

	rows, err := s.DB.Query(query)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var pt AgentConcurrencyPointDTO
			if err := rows.Scan(&pt.Timestamp, &pt.ActiveWorkers, &pt.OffloadedTokens); err == nil {
				pt.CapacityCeiling = 5
				if pt.ActiveWorkers > 5 {
					pt.CapacityCeiling = pt.ActiveWorkers + 2
				}
				points = append(points, pt)
			}
		}
	}

	// Nếu khung giờ không có task và đang ở chế độ toàn bộ/7d, fallback quét toàn bộ
	if len(points) == 0 && (rangeStr == "all" || rangeStr == "") {
		fallbackQuery := `
			SELECT 
				strftime('%Y-%m-%d %H:00', started_at) AS time_bucket,
				COUNT(DISTINCT subagent_id) AS workers,
				COALESCE(SUM(tokens_offloaded), 0) AS offloaded
			FROM agent_fleet_telemetry
			GROUP BY time_bucket
			ORDER BY time_bucket ASC
			LIMIT 48
		`
		fbRows, fbErr := s.DB.Query(fallbackQuery)
		if fbErr == nil {
			defer fbRows.Close()
			for fbRows.Next() {
				var pt AgentConcurrencyPointDTO
				if err := fbRows.Scan(&pt.Timestamp, &pt.ActiveWorkers, &pt.OffloadedTokens); err == nil {
					pt.CapacityCeiling = 5
					if pt.ActiveWorkers > 5 {
						pt.CapacityCeiling = pt.ActiveWorkers + 2
					}
					points = append(points, pt)
				}
			}
		}
	}

	// Đảm bảo không bao giờ trả về mảng rỗng để trục biểu đồ không bị vỡ
	if len(points) == 0 {
		points = append(points, AgentConcurrencyPointDTO{
			Timestamp:       time.Now().Format("2006-01-02 15:04"),
			ActiveWorkers:   0,
			CapacityCeiling: 5,
			OffloadedTokens: 0,
		})
	}

	return points, nil
}

// GetAgentGanttTasks trả về danh sách các tác vụ thực tế của Subagent từ transcript theo khung giờ
func (s *Storage) GetAgentGanttTasks(groupBy string, timeRange ...string) ([]AgentGanttTaskDTO, error) {
	rangeStr := ""
	if len(timeRange) > 0 {
		rangeStr = strings.ToLower(strings.TrimSpace(timeRange[0]))
	}

	timeCond := parseAgentTimeCondition(rangeStr, "started_at")
	whereClause := ""
	if timeCond != "" {
		whereClause = "WHERE " + timeCond
	}

	query := fmt.Sprintf(`
		SELECT 
			id, subagent_id, role_name, task_name, status,
			strftime('%%Y-%%m-%%d %%H:%%M:%%S', started_at) AS started_at,
			COALESCE(strftime('%%Y-%%m-%%d %%H:%%M:%%S', finished_at), '') AS finished_at,
			duration_ms,
			tokens_offloaded
		FROM agent_fleet_telemetry
		%s
		ORDER BY started_at DESC
		LIMIT 100
	`, whereClause)

	rows, err := s.DB.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []AgentGanttTaskDTO
	for rows.Next() {
		var item AgentGanttTaskDTO
		if err := rows.Scan(
			&item.ID,
			&item.SubagentID,
			&item.RoleName,
			&item.TaskName,
			&item.Status,
			&item.StartedAt,
			&item.FinishedAt,
			&item.DurationMs,
			&item.TokensOffloaded,
		); err != nil {
			return nil, err
		}
		if item.FinishedAt == "" && item.Status == "RUNNING" {
			item.FinishedAt = time.Now().Format("2006-01-02 15:04:05")
		}
		list = append(list, item)
	}

	// Sắp xếp lại theo thời gian tăng dần để vẽ Gantt từ trái sang phải
	for i, j := 0, len(list)-1; i < j; i, j = i+1, j-1 {
		list[i], list[j] = list[j], list[i]
	}

	return list, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// TOPOLOGY NETWORK GRAPH (MULTI-AGENT & MULTI-PROJECT PARALLEL FLEET)
// ─────────────────────────────────────────────────────────────────────────────

type AgentNodeDTO struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Category   int      `json:"category"`
	SymbolSize float64  `json:"symbolSize"`
	Role       string   `json:"role"`
	Project    string   `json:"project"`
	ProjectID  string   `json:"project_id"`
	Status     string   `json:"status"` // RUNNING, COMPLETED, STANDBY
	Tokens     int64    `json:"tokens"`
	TaskCount  int      `json:"task_count"`
	LastTask   string   `json:"last_task"`
	X          *float64 `json:"x,omitempty"`
	Y          *float64 `json:"y,omitempty"`
	Fixed      bool     `json:"fixed"`
}

type AgentLinkDTO struct {
	Source       string `json:"source"`
	Target       string `json:"target"`
	Label        string `json:"label"`
	Tokens       int64  `json:"tokens"`
	Interactions int    `json:"interactions"`
	Type         string `json:"type"` // DELEGATION, CONTEXT_HANDOFF, ARTIFACT_HANDOFF, FEEDBACK_LOOP, KNOWLEDGE_INJECTION, GOVERNANCE
	Status       string `json:"status"` // ACTIVE, COMPLETED
}

type AgentCategoryDTO struct {
	Name string `json:"name"`
}

type ProjectSummaryDTO struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ActiveAgents int    `json:"active_agents"`
	TotalTasks   int    `json:"total_tasks"`
	TotalTokens  int64  `json:"total_tokens"`
	Status       string `json:"status"` // ACTIVE, STANDBY
}

type AgentTopologyGraphDTO struct {
	Projects   []ProjectSummaryDTO `json:"projects"`
	Nodes      []AgentNodeDTO      `json:"nodes"`
	Links      []AgentLinkDTO      `json:"links"`
	Categories []AgentCategoryDTO  `json:"categories"`
}

// AgentGanttPacketDTO đại diện cho gói tin trao đổi giữa hai tác vụ trên biểu đồ Lifecycle Gantt
type AgentGanttPacketDTO struct {
	ID         string `json:"id"`
	SessionID  string `json:"session_id"`
	Project    string `json:"project"`
	FromTaskID int64  `json:"from_task_id"`
	ToTaskID   int64  `json:"to_task_id"`
	FromRole   string `json:"from_role"`
	ToRole     string `json:"to_role"`
	FromTime   string `json:"from_time"`
	ToTime     string `json:"to_time"`
	PacketType string `json:"packet_type"` // CONTEXT_PACKET, ARTIFACT_HANDOFF, TEST_STATUS_FEEDBACK, INTELLIGENCE_DOCS, POLICY_GOVERNANCE
	Label      string `json:"label"`
	Color      string `json:"color"`
	Tokens     int64  `json:"tokens"`
	Payload    string `json:"payload"`
}

func determinePacketInfo(fromRole, toRole, taskName string, tokens int64) (packetType, label, color, payload string) {
	f := strings.ToLower(fromRole)
	t := strings.ToLower(toRole)

	switch {
	case strings.Contains(f, "explorer") || strings.Contains(f, "code") || strings.Contains(f, "analyst"):
		if strings.Contains(t, "worker") || strings.Contains(t, "terminal") {
			return "CONTEXT_PACKET", "Gói Ngữ Cảnh Mã Nguồn (Context Passing)", "#38bdf8", "Truyền AST symbols, cấu trúc module & code boundaries"
		}
		return "CONTEXT_PACKET", "Gói Dữ Liệu Ngữ Cảnh (Context Packet)", "#38bdf8", "Chia sẻ ngữ cảnh codebase và phạm vi mã lệnh"

	case strings.Contains(f, "worker") || strings.Contains(f, "branch") || strings.Contains(f, "terminal"):
		if strings.Contains(t, "tester") || strings.Contains(t, "verification") || strings.Contains(t, "qa") {
			return "ARTIFACT_HANDOFF", "Bàn Giao Bản Vá (Artifact Handoff)", "#f59e0b", "Bàn giao diff patches & test cases nghiệm thu"
		}
		return "ARTIFACT_HANDOFF", "Bàn Giao Kết Quả (Work Product)", "#f59e0b", "Gửi mã nguồn hoàn thiện để kiểm tra liên thông"

	case strings.Contains(f, "tester") || strings.Contains(f, "verification") || strings.Contains(f, "qa"):
		return "TEST_STATUS_FEEDBACK", "Báo Cáo Kiểm Thử (Pass/Fail Feedback)", "#10b981", "Kết quả test suite: Pass, coverage & performance metrics"

	case strings.Contains(f, "research") || strings.Contains(f, "doc") || strings.Contains(f, "browser"):
		return "INTELLIGENCE_DOCS", "Bơm Tri Thức Tài Liệu (Docs & Specs)", "#a855f7", "Tài liệu kỹ thuật API, quy ước hệ thống & specs"

	case strings.Contains(f, "audit") || strings.Contains(f, "pki") || strings.Contains(f, "policy") || strings.Contains(f, "security"):
		return "POLICY_GOVERNANCE", "Giám Sát An Toàn (Policy & Sweep)", "#f43f5e", "Kiểm tra hạn mức token & tính tuân thủ an toàn"

	default:
		return "CONTEXT_PACKET", "Trao Đổi Tác Vụ (Agent Handshake)", "#38bdf8", "Đồng bộ trạng thái công việc và luồng token thực thi"
	}
}

func getRoleCategory(role string) int {
	r := strings.ToLower(role)
	switch {
	case strings.Contains(r, "project"):
		return 0
	case strings.Contains(r, "orchestrator") || strings.Contains(r, "planner"):
		return 1
	case strings.Contains(r, "self-branch") || strings.Contains(r, "worker"):
		return 2
	case strings.Contains(r, "codebase") || strings.Contains(r, "explorer"):
		return 3
	case strings.Contains(r, "verification") || strings.Contains(r, "tester"):
		return 4
	case strings.Contains(r, "research"):
		return 5
	case strings.Contains(r, "pki") || strings.Contains(r, "auditor"):
		return 6
	default:
		return 3
	}
}

func formatTokenCountLabel(tokens int64) string {
	if tokens >= 1000000 {
		return fmt.Sprintf("%.2fM tok", float64(tokens)/1000000.0)
	}
	if tokens >= 1000 {
		return fmt.Sprintf("%.0fk tok", float64(tokens)/1000.0)
	}
	return fmt.Sprintf("%d tok", tokens)
}

var (
	convProjectMu    sync.RWMutex
	convProjectCache = make(map[string]struct{ ID, Name string })

	reWorkspacePath    = regexp.MustCompile(`(?i)[/\\]workspace[/\\](?:code[/\\])?([a-zA-Z0-9_\-\.]+)(?:[/\\]([a-zA-Z0-9_\-\.]+))?(?:[/\\]([a-zA-Z0-9_\-\.]+))?`)
	reFileUri          = regexp.MustCompile(`(?i)file:///[^"\s\)]*workspace[/\\](?:code[/\\])?([a-zA-Z0-9_\-\.]+)(?:[/\\]([a-zA-Z0-9_\-\.]+))?(?:[/\\]([a-zA-Z0-9_\-\.]+))?`)
	reWorkspaceURI     = regexp.MustCompile(`(?i)([a-zA-Z]:[\\/][^\r\n"'>]+?)\s*->`)
	reWorkingDir       = regexp.MustCompile(`(?i)Working directory:\s*([^\r\n]+)`)
	reToolArgsDir      = regexp.MustCompile(`(?i)\"(?:Cwd|DirectoryPath|TargetFile|AbsolutePath|SearchDirectory|Workspace)\":\s*\"([^\"]+)\"`)
	reFileURIUniversal = regexp.MustCompile(`(?i)file:///([^\s\"\'\)\>]+)`)
)

func isIgnoredProjectSegment(seg string) bool {
	sLower := strings.ToLower(strings.Trim(seg, " \t\r\n\"'`.,;:<>{}[]()#*"))
	if sLower == "" || sLower == "." || sLower == ".." || len(sLower) < 2 {
		return true
	}
	switch sLower {
	case "users", "ethanpham", "appdata", "roaming", "local", "windows", "program files",
		"googledrive", "documentfile", "workspace", "code", "projects", "mycli-ai",
		".gemini", "gemini", "antigravity", "brain", ".system_generated", "logs", "scratch", ".agents",
		"backup", "backups", "tmp", "temp", "profile", "profiles",
		"config", "skills", "builtin", "plugins", "rules", "node_modules", "vendor",
		"artifacts", "dist", "build", "bin", "pkg", "obj",
		"mylearning", "projectgolang", "golangdev", "projectr", "securitystandards",
		"user_uploaded":
		return true
	}
	if strings.HasPrefix(sLower, "n-") || strings.HasPrefix(sLower, "n.") || strings.HasPrefix(sLower, "n#") {
		return true
	}
	if strings.Contains(sLower, "truncated_fields") || strings.Contains(sLower, "additional_metadata") ||
		strings.Contains(sLower, "user_request") || strings.Contains(sLower, "content") ||
		strings.Contains(sLower, "integrity mode") {
		return true
	}
	matchedUUID, _ := regexp.MatchString(`^[0-9a-fA-F]{8}(-[0-9a-fA-F]{4}){3}-[0-9a-fA-F]{12}$`, sLower)
	if matchedUUID {
		return true
	}
	matchedHex8, _ := regexp.MatchString(`^[0-9a-fA-F]{8}$`, sLower)
	if matchedHex8 {
		return true
	}
	exts := []string{".md", ".pdf", ".go", ".py", ".json", ".yaml", ".yml", ".txt", ".html", ".js", ".ts", ".sh", ".ps1", ".r", ".sql", ".toml", ".xml", ".css"}
	for _, ext := range exts {
		if strings.HasSuffix(sLower, ext) {
			return true
		}
	}
	return false
}

func ExtractProjectFromPathString(raw string) (string, string) {
	// Xử lý escaped newlines trong JSON string
	raw = strings.ReplaceAll(raw, `\r`, "\r")
	raw = strings.ReplaceAll(raw, `\n`, "\n")
	raw = strings.ReplaceAll(raw, `\t`, " ")
	if idx := strings.IndexAny(raw, "\r\n"); idx != -1 {
		raw = raw[:idx]
	}

	decoded, err := url.PathUnescape(raw)
	if err == nil {
		raw = decoded
	}
	raw = strings.ReplaceAll(raw, `\\`, `/`)
	raw = strings.ReplaceAll(raw, `\`, `/`)
	raw = strings.Trim(raw, " \t\r\n\"'`.,;:<>{}[]()")

	rawLower := strings.ToLower(raw)
	// Loại trừ tuyệt đối các đường dẫn nội bộ của AI Agent / IDE / Skills
	if strings.Contains(rawLower, "/.gemini/") || strings.Contains(rawLower, "/gemini/") ||
		strings.Contains(rawLower, "/.claude/") || strings.Contains(rawLower, "/claude/") ||
		strings.Contains(rawLower, "/.codex/") || strings.Contains(rawLower, "/codex/") ||
		strings.Contains(rawLower, "/skills/") || strings.Contains(rawLower, "/plugins/") ||
		strings.Contains(rawLower, "/builtin/") || strings.Contains(rawLower, "/brain/") ||
		strings.Contains(rawLower, "/scratch/") || strings.Contains(rawLower, "/artifacts/") ||
		strings.Contains(rawLower, "/user_uploaded/") || strings.Contains(rawLower, "/.system_generated/") {
		return "", ""
	}

	// 1. Kiểm tra nhanh các dự án lõi nếu xuất hiện bất kỳ đâu trong đường dẫn
	if strings.Contains(rawLower, "tokenmonitor") || strings.Contains(rawLower, "token_monitor") {
		return "proj-tokenmonitor", "TokenMonitor (GoLangDev)"
	}
	if strings.Contains(rawLower, "mcredit") || strings.Contains(rawLower, "gtcg") {
		return "proj-mcredit", "MCREDIT (ProjectR)"
	}
	if strings.Contains(rawLower, "tieuchuan") || strings.Contains(rawLower, "hardening") || strings.Contains(rawLower, "linuxhardening") {
		return "proj-tieuchuanhardeninglinux", "TieuChuanHardeningLinux (Security Standards)"
	}
	if strings.Contains(rawLower, "guardium") || strings.Contains(rawLower, "ibm") {
		return "proj-4.1-ibm-guardium-data-protection", "4.1-IBM Guardium data protection"
	}
	if strings.Contains(rawLower, "flink") || strings.Contains(rawLower, "apche-flink") {
		return "proj-apche-flink-elk", "Apche-flink-elk"
	}
	if strings.Contains(rawLower, "projectscriptos") || strings.Contains(rawLower, "scriptos") {
		return "proj-projectscriptos", "ProjectScriptOS"
	}

	// Bỏ ký tự ổ đĩa ví dụ C:/ hoặc E:/
	if len(raw) >= 2 && raw[1] == ':' {
		raw = raw[2:]
	}

	// 2. Quét ngược từ thư mục lá (leaf directory) lên đầu để tự động nhận diện dự án mới bất kỳ
	parts := strings.Split(raw, "/")
	for i := len(parts) - 1; i >= 0; i-- {
		seg := strings.Trim(parts[i], " \t\r\n\"'`.,;:<>{}[]()#*")
		if !isIgnoredProjectSegment(seg) {
			id, name, _ := ResolveCrossLLMProject(seg)
			if id != "" && name != "" {
				return id, name
			}
		}
	}

	return "", ""
}

// resolveSubagentProject tự động nhận diện dự án từ subagentID, roleName, taskName và nhật ký transcript
func resolveSubagentProject(subagentID, roleName, taskName string) (projID, projName string) {
	// 1. Nhận diện từ khóa đặc trưng trong taskName
	taskLower := strings.ToLower(taskName)
	if strings.Contains(taskLower, "tokenmonitor") || strings.Contains(taskLower, "token_monitor") ||
		strings.Contains(taskLower, "token monitor") || strings.Contains(taskLower, "golangdev") {
		return "proj-tokenmonitor", "TokenMonitor (GoLangDev)"
	}
	if strings.Contains(taskLower, "tieuchuan") || strings.Contains(taskLower, "hardening") ||
		strings.Contains(taskLower, "cau_hinh_may_chu") || strings.Contains(taskLower, "may_chu_linux") ||
		strings.Contains(taskLower, "phu_luc") || strings.Contains(taskLower, "cis_profile") ||
		strings.Contains(taskLower, "huong_dan_cau_hinh") || strings.Contains(taskLower, "tieuchuanhardeninglinux") {
		return "proj-tieuchuanhardeninglinux", "TieuChuanHardeningLinux (Security Standards)"
	}
	if strings.Contains(taskLower, "mcredit") || strings.Contains(taskLower, "gtcg") ||
		strings.Contains(taskLower, "projectr") || strings.Contains(taskLower, "t24-ds") ||
		strings.Contains(taskLower, ".r ") || strings.Contains(taskLower, ".r\"") || strings.HasSuffix(taskLower, ".r") {
		return "proj-mcredit", "MCREDIT (ProjectR)"
	}

	// 2. Trích xuất tiền tố cuộc hội thoại: sub-<convID>-s<step>-t<tool>
	parts := strings.Split(subagentID, "-")
	convPrefix := ""
	if len(parts) >= 2 && parts[0] == "sub" {
		convPrefix = strings.ToLower(parts[1])
	}

	// Fast path cho các conversation đã biết
	if strings.HasPrefix(convPrefix, "tm") ||
		strings.HasPrefix(convPrefix, "tokenmonitor") ||
		strings.HasPrefix(convPrefix, "canonical") ||
		strings.HasPrefix(convPrefix, "session1") ||
		strings.HasPrefix(convPrefix, "fresh") ||
		strings.HasPrefix(convPrefix, "stale") ||
		strings.HasPrefix(convPrefix, "convcons") ||
		convPrefix == "574184f1" || convPrefix == "511bb89e" {
		return "proj-tokenmonitor", "TokenMonitor (GoLangDev)"
	}
	if strings.HasPrefix(convPrefix, "tch") || convPrefix == "227fb340" || convPrefix == "abe42560" || convPrefix == "b71cdefa" {
		return "proj-tieuchuanhardeninglinux", "TieuChuanHardeningLinux (Security Standards)"
	}
	if strings.HasPrefix(convPrefix, "mcr") || convPrefix == "5fc429ff" {
		return "proj-mcredit", "MCREDIT (ProjectR)"
	}
	if strings.HasPrefix(convPrefix, "challenger") {
		if strings.HasSuffix(subagentID, "002") {
			return "proj-mcredit", "MCREDIT (ProjectR)"
		}
		if strings.HasSuffix(subagentID, "003") {
			return "proj-tieuchuanhardeninglinux", "TieuChuanHardeningLinux (Security Standards)"
		}
		return "proj-tokenmonitor", "TokenMonitor (GoLangDev)"
	}

	// Tra cứu cache bộ nhớ
	if convPrefix != "" {
		convProjectMu.RLock()
		cached, ok := convProjectCache[convPrefix]
		convProjectMu.RUnlock()
		if ok {
			return cached.ID, cached.Name
		}
	}

	// 3. Tra cứu động từ tất cả thư mục brain trên máy người dùng
	homeDir, err := os.UserHomeDir()
	if err == nil && convPrefix != "" {
		geminiRoot := filepath.Join(homeDir, ".gemini")
		entries, err := os.ReadDir(geminiRoot)
		if err == nil {
			for _, e := range entries {
				if !e.IsDir() {
					continue
				}
				nameLower := strings.ToLower(e.Name())
				if strings.Contains(nameLower, "backup") || strings.Contains(nameLower, "tmp") || strings.Contains(nameLower, "profile") {
					continue
				}
				brainDir := filepath.Join(geminiRoot, e.Name(), "brain")
				cEntries, err := os.ReadDir(brainDir)
				if err != nil {
					continue
				}
				for _, ce := range cEntries {
					if ce.IsDir() && strings.HasPrefix(strings.ToLower(ce.Name()), convPrefix) {
						tPath := filepath.Join(brainDir, ce.Name(), ".system_generated", "logs", "transcript.jsonl")
						f, err := os.Open(tPath)
						if err != nil {
							continue
						}
						scanner := bufio.NewScanner(f)
						buf := make([]byte, 64*1024)
						scanner.Buffer(buf, 1024*1024)
						lineCount := 0
						for scanner.Scan() && lineCount < 60 {
							lineText := scanner.Text()
							lineCount++

							lineText = strings.ReplaceAll(lineText, `\r`, "\r")
							lineText = strings.ReplaceAll(lineText, `\n`, "\n")

							// Ưu tiên 0: Universal Path Extraction từ Working directory, Cwd, DirectoryPath, URI
							var rawCandidate string
							if m := reWorkspaceURI.FindStringSubmatch(lineText); len(m) > 1 {
								rawCandidate = m[1]
							} else if m := reWorkingDir.FindStringSubmatch(lineText); len(m) > 1 {
								rawCandidate = m[1]
							} else if m := reToolArgsDir.FindStringSubmatch(lineText); len(m) > 1 {
								rawCandidate = m[1]
							} else if m := reFileURIUniversal.FindStringSubmatch(lineText); len(m) > 1 {
								rawCandidate = m[1]
							}

							if rawCandidate != "" {
								pID, pName := ExtractProjectFromPathString(rawCandidate)
								if pID != "" && pName != "" {
									f.Close()
									convProjectMu.Lock()
									convProjectCache[convPrefix] = struct{ ID, Name string }{ID: pID, Name: pName}
									convProjectMu.Unlock()
									return pID, pName
								}
							}

							normalized := strings.ReplaceAll(lineText, `\\`, `/`)
							normLower := strings.ToLower(normalized)

							// Ưu tiên 1: Nhận diện chuẩn xác theo đường dẫn Workspace / CWD / Thư mục thao tác thực tế
							if strings.Contains(normLower, "/tokenmonitor") || strings.Contains(normLower, "tokenmonitor/") ||
								strings.Contains(normLower, "golangdev/tokenmonitor") {
								f.Close()
								convProjectMu.Lock()
								convProjectCache[convPrefix] = struct{ ID, Name string }{ID: "proj-tokenmonitor", Name: "TokenMonitor (GoLangDev)"}
								convProjectMu.Unlock()
								return "proj-tokenmonitor", "TokenMonitor (GoLangDev)"
							}
							if strings.Contains(normLower, "/projectr") || strings.Contains(normLower, "projectr/") ||
								strings.Contains(normLower, "/mcredit") || strings.Contains(normLower, "mcredit/") {
								if !strings.Contains(normLower, "/tokenmonitor") {
									f.Close()
									convProjectMu.Lock()
									convProjectCache[convPrefix] = struct{ ID, Name string }{ID: "proj-mcredit", Name: "MCREDIT (ProjectR)"}
									convProjectMu.Unlock()
									return "proj-mcredit", "MCREDIT (ProjectR)"
								}
							}
							if strings.Contains(normLower, "/tieuchuanhardeninglinux") || strings.Contains(normLower, "tieuchuanhardeninglinux/") ||
								strings.Contains(normLower, "linuxhardening") || strings.Contains(normLower, "tiêu_chuẩn_config") ||
								strings.Contains(normLower, "cau_hinh_may_chu_linux") {
								if !strings.Contains(normLower, "/tokenmonitor") {
									f.Close()
									convProjectMu.Lock()
									convProjectCache[convPrefix] = struct{ ID, Name string }{ID: "proj-tieuchuanhardeninglinux", Name: "TieuChuanHardeningLinux (Security Standards)"}
									convProjectMu.Unlock()
									return "proj-tieuchuanhardeninglinux", "TieuChuanHardeningLinux (Security Standards)"
								}
							}
							if strings.Contains(normLower, "/projectscriptos") || strings.Contains(normLower, "projectscriptos/") {
								f.Close()
								convProjectMu.Lock()
								convProjectCache[convPrefix] = struct{ ID, Name string }{ID: "proj-projectscriptos", Name: "ProjectScriptOS"}
								convProjectMu.Unlock()
								return "proj-projectscriptos", "ProjectScriptOS"
							}

							var folder string
							if m := reFileUri.FindStringSubmatch(normalized); len(m) > 1 {
								folder = m[1]
								if len(m) > 3 && m[3] != "" {
									folder = m[3]
								} else if len(m) > 2 && m[2] != "" && (strings.EqualFold(folder, "projectgolang") || strings.EqualFold(folder, "golangdev")) {
									folder = m[2]
								}
							} else if m := reWorkspacePath.FindStringSubmatch(normalized); len(m) > 1 {
								folder = m[1]
								if len(m) > 3 && m[3] != "" {
									folder = m[3]
								} else if len(m) > 2 && m[2] != "" && (strings.EqualFold(folder, "projectgolang") || strings.EqualFold(folder, "golangdev")) {
									folder = m[2]
								}
							}

							folder = strings.Trim(strings.TrimSpace(folder), `"'`)
							fLower := strings.ToLower(folder)
							if len(folder) > 2 && fLower != "code" && fLower != "workspace" && fLower != "." && fLower != "tmp" {
								f.Close()
								pID, pName, _ := ResolveCrossLLMProject(folder)
								convProjectMu.Lock()
								convProjectCache[convPrefix] = struct{ ID, Name string }{ID: pID, Name: pName}
								convProjectMu.Unlock()
								return pID, pName
							}
						}
						f.Close()
					}
				}
			}
		}
	}

	// Default fallback an toàn về TokenMonitor (GoLangDev) thay vì tạo dự án giả từ mã hex cuộc hội thoại
	if convPrefix != "" {
		convProjectMu.Lock()
		convProjectCache[convPrefix] = struct{ ID, Name string }{ID: "proj-tokenmonitor", Name: "TokenMonitor (GoLangDev)"}
		convProjectMu.Unlock()
	}
	return "proj-tokenmonitor", "TokenMonitor (GoLangDev)"
}

// GetAgentTopologyGraph phân tích và tổng hợp dữ liệu mạng lưới tương tác giữa các Agent cho 1 hoặc nhiều dự án chạy song song theo khung thời gian
func (s *Storage) GetAgentTopologyGraph(projectFilter string, timeRange ...string) (*AgentTopologyGraphDTO, error) {
	categories := []AgentCategoryDTO{
		{Name: "🏢 Dự Án (Projects)"},
		{Name: "🤖 Primary Orchestrator"},
		{Name: "⚡ Self-Branch Worker"},
		{Name: "📦 Codebase Explorer"},
		{Name: "✅ Verification Tester"},
		{Name: "🔍 Research Agent"},
		{Name: "🛡️ PKI Auditor"},
		{Name: "👑 Root Controller & Account"},
	}

	rangeStr := ""
	if len(timeRange) > 0 {
		rangeStr = strings.ToLower(strings.TrimSpace(timeRange[0]))
	}

	timeCond := parseAgentTimeCondition(rangeStr, "started_at")
	whereClause := ""
	if timeCond != "" {
		whereClause = "WHERE " + timeCond
	}

	// 1. Đọc dữ liệu tác vụ thực tế trong khung thời gian
	query := fmt.Sprintf(`
		SELECT 
			id, subagent_id, role_name, task_name, status,
			duration_ms, tokens_offloaded, started_at
		FROM agent_fleet_telemetry
		%s
		ORDER BY started_at DESC
	`, whereClause)

	rows, err := s.DB.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type RoleAgg struct {
		RoleName    string
		TaskCount   int
		Tokens      int64
		IsRunning   bool
		LastTask    string
	}

	type ProjectData struct {
		ID             string
		Name           string
		Roles          map[string]*RoleAgg
		Tokens         int64
		Tasks          int
		IsRunning      bool
		LatestActivity time.Time
	}

	projectsMap := make(map[string]*ProjectData)

	ensureProject := func(id, name string) *ProjectData {
		if p, ok := projectsMap[id]; ok {
			return p
		}
		p := &ProjectData{
			ID:    id,
			Name:  name,
			Roles: make(map[string]*RoleAgg),
		}
		projectsMap[id] = p
		return p
	}

	now := time.Now().UTC()

	// Duyệt các dòng dữ liệu thực tế
	for rows.Next() {
		var id int64
		var subagentID, roleName, taskName, status string
		var durationMs, tokensOffloaded int64
		var startedAt time.Time
		if err := rows.Scan(&id, &subagentID, &roleName, &taskName, &status, &durationMs, &tokensOffloaded, &startedAt); err != nil {
			continue
		}

		// Nhận diện dự án từ subagentID, roleName, taskName qua dynamic resolver
		projID, projName := resolveSubagentProject(subagentID, roleName, taskName)

		p := ensureProject(projID, projName)
		p.Tokens += tokensOffloaded
		p.Tasks++
		if startedAt.After(p.LatestActivity) {
			p.LatestActivity = startedAt
		}
		if status == "RUNNING" {
			p.IsRunning = true
		}

		cleanRole := roleName
		if cleanRole == "" {
			cleanRole = "Codebase Explorer"
		}
		ra, ok := p.Roles[cleanRole]
		if !ok {
			ra = &RoleAgg{RoleName: cleanRole}
			p.Roles[cleanRole] = ra
		}
		ra.TaskCount++
		ra.Tokens += tokensOffloaded
		if status == "RUNNING" {
			ra.IsRunning = true
		}
		if ra.LastTask == "" {
			ra.LastTask = taskName
		}
	}

	// 2. Lọc theo projectFilter kết hợp quy tắc 1h ("Dự án nào đang chạy thì load lên, nếu 1h không chạy thì ko load lên cho đỡ rối")
	var activeProjects []*ProjectData
	filterClean := strings.ToLower(strings.TrimSpace(projectFilter))
	oneHourAgo := now.Add(-75 * time.Minute) // 1 giờ hoạt động (kèm 15 phút đệm an toàn)

	var mostRecentProject *ProjectData
	for _, p := range projectsMap {
		if mostRecentProject == nil || p.LatestActivity.After(mostRecentProject.LatestActivity) {
			mostRecentProject = p
		}

		if filterClean != "" && filterClean != "all" && filterClean != "workspace_overview" {
			// Người dùng chỉ định xem 1 dự án cụ thể từ dropdown -> Luôn nạp đúng dự án được chọn
			if !strings.EqualFold(p.ID, filterClean) && !strings.Contains(strings.ToLower(p.ID), filterClean) && !strings.Contains(strings.ToLower(p.Name), filterClean) {
				continue
			}
		} else {
			// Chế độ "all" / mặc định: Chỉ load dự án ĐANG CHẠY hoặc CÓ HOẠT ĐỘNG TRONG VÒNG 1 GIỜ GẦN NHẤT
			// Nếu dự án đã dừng > 1h thì không load lên để màn hình không bị rối và đè chồng node
			isActiveOrRecent := p.IsRunning || (!p.LatestActivity.IsZero() && p.LatestActivity.After(oneHourAgo))
			if !isActiveOrRecent {
				continue
			}
		}
		activeProjects = append(activeProjects, p)
	}

	// Fallback an toàn: Nếu không có dự án nào đang chạy và không có dự án nào trong 1h, giữ lại duy nhất 1 dự án gần nhất để đồ thị không bị trống trơn
	if len(activeProjects) == 0 && mostRecentProject != nil {
		activeProjects = append(activeProjects, mostRecentProject)
	}

	// Sắp xếp projects ổn định: Ưu tiên dự án đang RUNNING lên trước, sau đó theo tokens giảm dần
	sort.Slice(activeProjects, func(i, j int) bool {
		if activeProjects[i].IsRunning != activeProjects[j].IsRunning {
			return activeProjects[i].IsRunning
		}
		if activeProjects[i].Tokens != activeProjects[j].Tokens {
			return activeProjects[i].Tokens > activeProjects[j].Tokens
		}
		return activeProjects[i].ID < activeProjects[j].ID
	})

	// 3. Xây dựng danh sách Nodes và Links
	nodes := make([]AgentNodeDTO, 0)
	links := make([]AgentLinkDTO, 0)
	projSummaries := make([]ProjectSummaryDTO, 0)

	// Toàn bộ dự án để hiển thị trong bộ lọc
	for _, p := range projectsMap {
		st := "STANDBY"
		if p.IsRunning {
			st = "ACTIVE"
		} else if p.Tasks > 0 {
			st = "COMPLETED"
		}
		projSummaries = append(projSummaries, ProjectSummaryDTO{
			ID:           p.ID,
			Name:         p.Name,
			ActiveAgents: len(p.Roles),
			TotalTasks:   p.Tasks,
			TotalTokens:  p.Tokens,
			Status:       st,
		})
	}
	sort.Slice(projSummaries, func(i, j int) bool {
		if (projSummaries[i].Status == "ACTIVE") != (projSummaries[j].Status == "ACTIVE") {
			return projSummaries[i].Status == "ACTIVE"
		}
		if projSummaries[i].TotalTokens != projSummaries[j].TotalTokens {
			return projSummaries[i].TotalTokens > projSummaries[j].TotalTokens
		}
		return projSummaries[i].ID < projSummaries[j].ID
	})

	canvasCX := 1500.0
	numProjects := len(activeProjects)
	colsPerRow := 3
	if numProjects <= 3 {
		colsPerRow = max(1, numProjects)
	} else if numProjects <= 6 {
		colsPerRow = (numProjects + 1) / 2
	}
	stepX := 620.0
	stepY := 360.0

	for pIdx, p := range activeProjects {
		pStatus := "COMPLETED"
		if p.IsRunning {
			pStatus = "RUNNING"
		}

		row := pIdx / colsPerRow
		col := pIdx % colsPerRow
		itemsInRow := colsPerRow
		if row == numProjects/colsPerRow && numProjects%colsPerRow != 0 {
			itemsInRow = numProjects % colsPerRow
		}

		cx := canvasCX
		if itemsInRow > 1 {
			cx = canvasCX + (float64(col)-float64(itemsInRow-1)/2.0)*stepX
		}
		hubY := 215.0 + float64(row)*stepY
		orchY := hubY + 95.0

		// Node 1: Project Hub Node (đỉnh cụm dự án)
		projNodeID := p.ID
		hubX := cx
		nodes = append(nodes, AgentNodeDTO{
			ID:         projNodeID,
			Name:       p.Name,
			Category:   0,
			SymbolSize: 56,
			Role:       "Project Hub",
			Project:    p.Name,
			ProjectID:  p.ID,
			Status:     pStatus,
			Tokens:     p.Tokens,
			TaskCount:  p.Tasks,
			LastTask:   fmt.Sprintf("Parallel Workspace • %d Tasks Active", p.Tasks),
			X:          &hubX,
			Y:          &hubY,
			Fixed:      true,
		})

		// Node 2: Primary Orchestrator Node (trực tiếp dưới Hub)
		orchNodeID := fmt.Sprintf("orch-%s", p.ID)
		orchX := cx
		nodes = append(nodes, AgentNodeDTO{
			ID:         orchNodeID,
			Name:       fmt.Sprintf("Orchestrator (%s)", strings.Split(p.Name, " ")[0]),
			Category:   1,
			SymbolSize: 48,
			Role:       "Primary Orchestrator",
			Project:    p.Name,
			ProjectID:  p.ID,
			Status:     pStatus,
			Tokens:     p.Tokens,
			TaskCount:  p.Tasks,
			LastTask:   "Task Delegation & Context Dispatching",
			X:          &orchX,
			Y:          &orchY,
			Fixed:      true,
		})

		// Link: Project Hub -> Orchestrator
		links = append(links, AgentLinkDTO{
			Source:       projNodeID,
			Target:       orchNodeID,
			Label:        "Active Session",
			Tokens:       p.Tokens,
			Interactions: p.Tasks,
			Type:         "SESSION",
			Status:       pStatus,
		})

		// Sắp xếp danh sách vai trò để vị trí cố định tuyệt đối
		var sortedRoleNames []string
		for rName := range p.Roles {
			if rName == "Primary Orchestrator" || rName == "Project Hub" {
				continue
			}
			sortedRoleNames = append(sortedRoleNames, rName)
		}
		sort.Strings(sortedRoleNames)

		// Tạo các Node Subagent cho dự án (Dàn quạt ngang / Horizontal Wing Formation)
		roleNodeIDs := make(map[string]string)
		for _, rName := range sortedRoleNames {
			ra := p.Roles[rName]
			slug := strings.ToLower(strings.ReplaceAll(rName, " ", "-"))
			subNodeID := fmt.Sprintf("agent-%s-%s", p.ID, slug)
			roleNodeIDs[rName] = subNodeID

			agStatus := "COMPLETED"
			if ra.IsRunning {
				agStatus = "RUNNING"
			}

			// Tính symbolSize linh hoạt theo tokens
			size := 36.0
			if ra.Tokens > 500000 {
				size = 46.0
			} else if ra.Tokens > 200000 {
				size = 42.0
			} else if ra.Tokens > 80000 {
				size = 38.0
			}

			// Tọa độ trực giao dàn quạt ngang (Horizontal Wing Formation)
			// Trải rộng theo chiều ngang (X span ±220px), độ sâu Y vừa vặn (Y max 440px)
			// Tuyệt đối không giao cắt, tạo khoảng không gian 340px thoáng đãng giữa các dự án
			var subX, subY float64
			rLower := strings.ToLower(rName)
			switch {
			case strings.Contains(rLower, "explorer") || strings.Contains(rLower, "code") || strings.Contains(rLower, "analyst"):
				subX = cx - 220.0
				subY = orchY + 40.0
			case strings.Contains(rLower, "worker") || strings.Contains(rLower, "branch") || strings.Contains(rLower, "terminal"):
				subX = cx
				subY = orchY + 130.0
			case strings.Contains(rLower, "tester") || strings.Contains(rLower, "verification") || strings.Contains(rLower, "qa"):
				subX = cx + 120.0
				subY = orchY + 115.0
			case strings.Contains(rLower, "research") || strings.Contains(rLower, "doc") || strings.Contains(rLower, "browser"):
				subX = cx - 120.0
				subY = orchY + 115.0
			case strings.Contains(rLower, "audit") || strings.Contains(rLower, "pki") || strings.Contains(rLower, "policy") || strings.Contains(rLower, "security"):
				subX = cx + 220.0
				subY = orchY + 40.0
			default:
				idx := len(roleNodeIDs) - 1
				angle := math.Pi*0.25 + float64(idx)*math.Pi*0.12
				subX = cx + 200.0*math.Cos(angle)
				subY = orchY + 90.0 + 40.0*math.Sin(angle)
			}

			nodes = append(nodes, AgentNodeDTO{
				ID:         subNodeID,
				Name:       ra.RoleName,
				Category:   getRoleCategory(ra.RoleName),
				SymbolSize: size,
				Role:       ra.RoleName,
				Project:    p.Name,
				ProjectID:  p.ID,
				Status:     agStatus,
				Tokens:     ra.Tokens,
				TaskCount:  ra.TaskCount,
				LastTask:   ra.LastTask,
				X:          &subX,
				Y:          &subY,
				Fixed:      true,
			})

			// Link: Orchestrator -> Subagent (Giao việc & phân bổ token)
			links = append(links, AgentLinkDTO{
				Source:       orchNodeID,
				Target:       subNodeID,
				Label:        fmt.Sprintf("Giao việc • %s", formatTokenCountLabel(ra.Tokens)),
				Tokens:       ra.Tokens,
				Interactions: ra.TaskCount,
				Type:         "DELEGATION",
				Status:       agStatus,
			})
		}

		// Thiết lập các đường cung phối hợp liên tác tử (Inter-Agent Exchange)
		explorerID := roleNodeIDs["Codebase Explorer"]
		workerID := roleNodeIDs["Self-Branch Worker"]
		testerID := roleNodeIDs["Verification Tester"]
		researchID := roleNodeIDs["Research Agent"]
		auditorID := roleNodeIDs["PKI Auditor"]

		// 1. Codebase Explorer -> Self-Branch Worker (Truyền ngữ cảnh mã nguồn)
		if explorerID != "" && workerID != "" {
			links = append(links, AgentLinkDTO{
				Source:       explorerID,
				Target:       workerID,
				Label:        "Ngữ Cảnh Code (Context Passing)",
				Tokens:       p.Tokens / 3,
				Interactions: max(1, p.Tasks/3),
				Type:         "CONTEXT_HANDOFF",
				Status:       pStatus,
			})
		}

		// 2. Self-Branch Worker -> Verification Tester (Bàn giao artifact để kiểm thử)
		if workerID != "" && testerID != "" {
			links = append(links, AgentLinkDTO{
				Source:       workerID,
				Target:       testerID,
				Label:        "Bàn Giao Kiểm Thử (Run Tests)",
				Tokens:       p.Tokens / 4,
				Interactions: max(1, p.Tasks/4),
				Type:         "ARTIFACT_HANDOFF",
				Status:       pStatus,
			})
		}

		// 3. Verification Tester -> Primary Orchestrator (Báo cáo kết quả pass/fail)
		if testerID != "" {
			links = append(links, AgentLinkDTO{
				Source:       testerID,
				Target:       orchNodeID,
				Label:        "Báo Cáo Test (Pass/Fail Feedback)",
				Tokens:       p.Tokens / 6,
				Interactions: max(1, p.Tasks/5),
				Type:         "FEEDBACK_LOOP",
				Status:       pStatus,
			})
		}

		// 4. Research Agent -> Self-Branch Worker (Tra cứu tài liệu/web)
		if researchID != "" && workerID != "" {
			links = append(links, AgentLinkDTO{
				Source:       researchID,
				Target:       workerID,
				Label:        "Bơm Tri Thức (Docs & Specs)",
				Tokens:       p.Tokens / 5,
				Interactions: max(1, p.Tasks/5),
				Type:         "KNOWLEDGE_INJECTION",
				Status:       pStatus,
			})
		}

		// 5. PKI Auditor -> Primary Orchestrator (Giám sát hạn mức an toàn)
		if auditorID != "" {
			links = append(links, AgentLinkDTO{
				Source:       auditorID,
				Target:       orchNodeID,
				Label:        "Giám Sát An Toàn (Policy & Sweep)",
				Tokens:       p.Tokens / 8,
				Interactions: max(1, p.Tasks/6),
				Type:         "GOVERNANCE",
				Status:       pStatus,
			})
		}
	}

	// 4. Tạo nút gốc Root Controller & Account (đỉnh tối cao điều phối toàn bộ Workspace Ecosystem)
	if len(activeProjects) > 0 {
		var grandTotalTokens int64
		var grandTotalTasks int
		for _, p := range activeProjects {
			grandTotalTokens += p.Tokens
			grandTotalTasks += p.Tasks
		}

		rootName := "Pham Ethan (Antigravity AI)"
		rootRole := "Root AI Controller & Account Owner"
		rootProj := "Antigravity Workspace Ecosystem"
		rootProjID := "root"
		if len(activeProjects) == 1 {
			rootProjID = activeProjects[0].ID
		}

		if acc, err := s.GetAccountProfile(); err == nil && acc != nil {
			if acc.Email != "" {
				rootName = fmt.Sprintf("Pham Ethan (%s)", acc.Email)
				if acc.PlanName != "" {
					rootRole = fmt.Sprintf("Root Controller • %s", acc.PlanName)
				}
			}
		}

		canvasCX := 1500.0
		rootX := canvasCX
		rootY := 105.0
		rootNodeID := "root-account"
		rootStatus := "COMPLETED"
		for _, p := range activeProjects {
			if p.IsRunning {
				rootStatus = "RUNNING"
				break
			}
		}

		rootNode := AgentNodeDTO{
			ID:         rootNodeID,
			Name:       rootName,
			Category:   7, // Root Controller & Account
			SymbolSize: 64,
			Role:       rootRole,
			Project:    rootProj,
			ProjectID:  rootProjID,
			Status:     rootStatus,
			Tokens:     grandTotalTokens,
			TaskCount:  grandTotalTasks,
			LastTask:   "Unified Multi-Workspace Orchestration",
			X:          &rootX,
			Y:          &rootY,
			Fixed:      true,
		}
		// Đặt Root Node ở đầu danh sách nodes
		nodes = append([]AgentNodeDTO{rootNode}, nodes...)

		// Tạo đường liên kết điều phối từ Root Controller tới từng Project Hub
		for _, p := range activeProjects {
			pStatus := "COMPLETED"
			if p.IsRunning {
				pStatus = "RUNNING"
			}
			links = append(links, AgentLinkDTO{
				Source:       rootNodeID,
				Target:       p.ID,
				Label:        fmt.Sprintf("Điều phối • %s", formatTokenCountLabel(p.Tokens)),
				Tokens:       p.Tokens,
				Interactions: p.Tasks,
				Type:         "ROOT_ORCHESTRATION",
				Status:       pStatus,
			})
		}
	}

	return &AgentTopologyGraphDTO{
		Projects:   projSummaries,
		Nodes:      nodes,
		Links:      links,
		Categories: categories,
	}, nil
}

// GetAgentGanttPackets trả về danh sách các gói tin giao tiếp giữa các tác vụ trên Lifecycle Gantt
func (s *Storage) GetAgentGanttPackets(timeRange ...string) ([]AgentGanttPacketDTO, error) {
	rangeStr := ""
	if len(timeRange) > 0 {
		rangeStr = strings.ToLower(strings.TrimSpace(timeRange[0]))
	}

	timeCond := parseAgentTimeCondition(rangeStr, "started_at")
	whereClause := ""
	if timeCond != "" {
		whereClause = "WHERE " + timeCond
	}

	query := fmt.Sprintf(`
		SELECT 
			id, subagent_id, role_name, task_name, status,
			strftime('%%Y-%%m-%%d %%H:%%M:%%S', started_at) AS started_at,
			COALESCE(strftime('%%Y-%%m-%%d %%H:%%M:%%S', finished_at), '') AS finished_at,
			duration_ms,
			tokens_offloaded
		FROM agent_fleet_telemetry
		%s
		ORDER BY started_at ASC
		LIMIT 150
	`, whereClause)

	rows, err := s.DB.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []AgentGanttTaskDTO
	for rows.Next() {
		var item AgentGanttTaskDTO
		if err := rows.Scan(
			&item.ID,
			&item.SubagentID,
			&item.RoleName,
			&item.TaskName,
			&item.Status,
			&item.StartedAt,
			&item.FinishedAt,
			&item.DurationMs,
			&item.TokensOffloaded,
		); err != nil {
			return nil, err
		}
		if item.FinishedAt == "" && item.Status == "RUNNING" {
			item.FinishedAt = time.Now().Format("2006-01-02 15:04:05")
		}
		tasks = append(tasks, item)
	}

	packets := make([]AgentGanttPacketDTO, 0)

	// 1. Phân tích chuỗi tác vụ thực tế trong cơ sở dữ liệu theo session
	sessionMap := make(map[string][]AgentGanttTaskDTO)
	for _, t := range tasks {
		sessKey := "default"
		parts := strings.Split(t.SubagentID, "-")
		if len(parts) >= 2 && parts[0] == "sub" {
			sessKey = parts[1] // convID
		}
		sessionMap[sessKey] = append(sessionMap[sessKey], t)
	}

	for sessKey, sessTasks := range sessionMap {
		if len(sessTasks) < 2 {
			continue
		}
		for i := 0; i < len(sessTasks)-1; i++ {
			t1 := sessTasks[i]
			t2 := sessTasks[i+1]
			if t1.RoleName == t2.RoleName {
				continue
			}

			fTime := t1.FinishedAt
			if fTime == "" {
				fTime = t1.StartedAt
			}
			tTime := t2.StartedAt
			if tTime == "" {
				tTime = t1.FinishedAt
			}

			pktType, label, color, payload := determinePacketInfo(t1.RoleName, t2.RoleName, t1.TaskName, t1.TokensOffloaded)
			toks := t1.TokensOffloaded / 2

			_, projName := resolveSubagentProject(t1.SubagentID, t1.RoleName, t1.TaskName)

			packets = append(packets, AgentGanttPacketDTO{
				ID:         fmt.Sprintf("pkt-%d-%d", t1.ID, t2.ID),
				SessionID:  sessKey,
				Project:    projName,
				FromTaskID: t1.ID,
				ToTaskID:   t2.ID,
				FromRole:   t1.RoleName,
				ToRole:     t2.RoleName,
				FromTime:   fTime,
				ToTime:     tTime,
				PacketType: pktType,
				Label:      label,
				Color:      color,
				Tokens:     toks,
				Payload:    payload,
			})
		}
	}

	// 2. Nếu các tác vụ thực tế theo session chưa đủ cặp liên vai trò,
	// tạo các gói tin giữa các tác vụ theo thứ tự thời gian nếu role khác nhau
	if len(packets) == 0 && len(tasks) >= 2 {
		for i := 0; i < len(tasks)-1; i++ {
			t1 := tasks[i]
			t2 := tasks[i+1]
			if t1.RoleName == t2.RoleName {
				continue
			}
			fTime := t1.FinishedAt
			if fTime == "" {
				fTime = t1.StartedAt
			}
			tTime := t2.StartedAt
			if tTime == "" {
				tTime = t1.FinishedAt
			}
			pktType, label, color, payload := determinePacketInfo(t1.RoleName, t2.RoleName, t1.TaskName, t1.TokensOffloaded)
			toks := t1.TokensOffloaded / 2

			_, projName := resolveSubagentProject(t1.SubagentID, t1.RoleName, t1.TaskName)

			packets = append(packets, AgentGanttPacketDTO{
				ID:         fmt.Sprintf("pkt-chain-%d-%d", t1.ID, t2.ID),
				SessionID:  t1.SubagentID,
				Project:    projName,
				FromTaskID: t1.ID,
				ToTaskID:   t2.ID,
				FromRole:   t1.RoleName,
				ToRole:     t2.RoleName,
				FromTime:   fTime,
				ToTime:     tTime,
				PacketType: pktType,
				Label:      label,
				Color:      color,
				Tokens:     toks,
				Payload:    payload,
			})
		}
	}

	return packets, nil
}

// ExtractWorkspaceFromProject trích xuất tên workspace ngắn gọn, sạch sẽ từ project ID và tên dự án.
func ExtractWorkspaceFromProject(projID, projName string) string {
	switch projID {
	case "proj-tokenmonitor":
		return "TokenMonitor"
	case "proj-mcredit":
		return "MCREDIT"
	case "proj-tieuchuanhardeninglinux":
		return "TieuChuanHardeningLinux"
	case "proj-projectscriptos":
		return "ProjectScriptOS"
	default:
		if idx := strings.Index(projName, " ("); idx != -1 {
			return projName[:idx]
		}
		return projName
	}
}

// ResolveCrossLLMProject đồng bộ hóa các định danh dự án, đường dẫn thư mục CWD hoặc tên workspace
// từ cả 3 nhà cung cấp Google Antigravity, OpenAI Codex và Anthropic Claude về 4 cụm dự án chuẩn.
func ResolveCrossLLMProject(rawPathOrName string) (id, name, workspace string) {
	input := strings.TrimSpace(rawPathOrName)
	norm := strings.ReplaceAll(input, `\`, `/`)
	normLower := strings.ToLower(norm)

	// Xử lý giá trị rỗng hoặc root fallback về cụm mặc định TokenMonitor
	if normLower == "" || normLower == "." || normLower == "/" || normLower == "default" || normLower == "default workspace" {
		return "proj-tokenmonitor", "TokenMonitor (GoLangDev)", "GoLangDev/TokenMonitor"
	}

	// 1. So khớp trực tiếp theo ID chuẩn
	switch normLower {
	case "proj-tokenmonitor":
		return "proj-tokenmonitor", "TokenMonitor (GoLangDev)", "GoLangDev/TokenMonitor"
	case "proj-mcredit":
		return "proj-mcredit", "MCREDIT (ProjectR)", "ProjectR/MCREDIT"
	case "proj-tieuchuanhardeninglinux":
		return "proj-tieuchuanhardeninglinux", "TieuChuanHardeningLinux (Security Standards)", "SecurityStandards/TieuChuanHardeningLinux"
	case "proj-projectscriptos":
		return "proj-projectscriptos", "ProjectScriptOS", "ProjectScriptOS"
	}

	// 2. Cụm TokenMonitor (GoLangDev)
	if strings.Contains(normLower, "tokenmonitor") || strings.Contains(normLower, "token_monitor") ||
		strings.Contains(normLower, "token monitor") ||
		(strings.Contains(normLower, "golangdev") && strings.Contains(normLower, "token")) {
		return "proj-tokenmonitor", "TokenMonitor (GoLangDev)", "GoLangDev/TokenMonitor"
	}

	// 3. Cụm MCREDIT (ProjectR) — bảo vệ với rào chắn chống va chạm TokenMonitor
	if !strings.Contains(normLower, "tokenmonitor") {
		if strings.Contains(normLower, "mcredit") || strings.Contains(normLower, "projectr") ||
			strings.Contains(normLower, "gtcg") || strings.Contains(normLower, "t24-ds") {
			return "proj-mcredit", "MCREDIT (ProjectR)", "ProjectR/MCREDIT"
		}
	}

	// 4. Cụm TieuChuanHardeningLinux (Security Standards) — bảo vệ với rào chắn chống va chạm TokenMonitor
	if !strings.Contains(normLower, "tokenmonitor") {
		if strings.Contains(normLower, "tieuchuan") || strings.Contains(normLower, "hardening") ||
			strings.Contains(normLower, "linuxhardening") || strings.Contains(normLower, "cau_hinh_may_chu") ||
			strings.Contains(normLower, "cis_profile") {
			return "proj-tieuchuanhardeninglinux", "TieuChuanHardeningLinux (Security Standards)", "SecurityStandards/TieuChuanHardeningLinux"
		}
	}

	// 5. Cụm ProjectScriptOS
	if strings.Contains(normLower, "projectscriptos") || strings.Contains(normLower, "aix") ||
		strings.Contains(normLower, "scp-copyremote-file-aix") {
		return "proj-projectscriptos", "ProjectScriptOS", "ProjectScriptOS"
	}

	// 6. Tự động nhận diện động cho các workspace phát sinh khác
	cleaned := filepath.Clean(norm)
	base := filepath.Base(cleaned)
	base = strings.Trim(base, `"' /`)
	if base == "" || base == "." || base == "/" {
		base = "Workspace Chung"
	}
	slug := strings.ToLower(strings.ReplaceAll(base, " ", "-"))
	slug = strings.TrimPrefix(slug, "proj-")
	return "proj-" + slug, base, cleaned
}

// GetAntigravityProjectStats truy vấn token_usage_logs và agent_fleet_telemetry để tổng hợp
// số liệu dự án của Google Antigravity theo khung thời gian chỉ định.
func (s *Storage) GetAntigravityProjectStats(timeRange string) (map[string]*AntigravityProjectStats, error) {
	timeRange = strings.ToLower(strings.TrimSpace(timeRange))
	if timeRange == "" {
		timeRange = "all"
	}

	// 1. Inline TTL auto-sweep: quét dọn các task RUNNING quá 45 giây chuyển sang COMPLETED
	_, _ = s.DB.Exec(`
		UPDATE agent_fleet_telemetry 
		SET status = 'COMPLETED',
		    finished_at = COALESCE(finished_at, datetime(started_at, '+' || MAX(duration_ms/1000, 2) || ' seconds'))
		WHERE status = 'RUNNING' 
		  AND started_at < datetime('now', '-2 minutes')
	`)

	result := make(map[string]*AntigravityProjectStats)

	ensureProject := func(projID, projName, ws string) *AntigravityProjectStats {
		if p, ok := result[projID]; ok {
			return p
		}
		p := &AntigravityProjectStats{
			ProjectID:   projID,
			ProjectName: projName,
			Workspace:   ws,
		}
		result[projID] = p
		return p
	}

	// 2. Truy vấn trực tiếp từ token_usage_logs
	tokenLogCond := parseAgentTimeCondition(timeRange, "timestamp")
	tokenWhere := ""
	if tokenLogCond != "" {
		tokenWhere = "WHERE " + tokenLogCond
	}

	queryLogs := fmt.Sprintf(`
		SELECT 
			COALESCE(request_type, ''),
			COALESCE(model_name, ''),
			COALESCE(prompt_tokens, 0),
			COALESCE(output_tokens, 0),
			COALESCE(thinking_tokens, 0),
			COALESCE(cached_tokens, 0),
			COALESCE(total_tokens, 0),
			timestamp
		FROM token_usage_logs
		%s
		ORDER BY timestamp DESC
	`, tokenWhere)

	rows, err := s.DB.Query(queryLogs)
	if err != nil {
		return nil, fmt.Errorf("GetAntigravityProjectStats: query token_usage_logs failed: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var reqType, modelName string
		var promptTokens, outputTokens, thinkingTokens, cachedTokens, totalTokens int64
		var ts time.Time

		if err := rows.Scan(&reqType, &modelName, &promptTokens, &outputTokens, &thinkingTokens, &cachedTokens, &totalTokens, &ts); err != nil {
			continue
		}

		convID := ""
		if strings.HasPrefix(reqType, "CHAT_") {
			rem := strings.TrimPrefix(reqType, "CHAT_")
			if idx := strings.LastIndex(rem, "_"); idx != -1 {
				convID = rem[:idx]
			} else {
				convID = rem
			}
		} else if strings.HasPrefix(reqType, "sub-") {
			parts := strings.Split(reqType, "-")
			if len(parts) >= 2 {
				convID = parts[1]
			}
		} else {
			convID = reqType
		}

		subagentID := "sub-" + convID
		rawProjID, rawProjName := resolveSubagentProject(subagentID, "", "")
		projID, projName, ws := ResolveCrossLLMProject(rawProjID)
		if projName == "" {
			projName = rawProjName
		}

		p := ensureProject(projID, projName, ws)
		p.PromptTokens += promptTokens
		p.OutputTokens += outputTokens
		p.ThinkingTokens += thinkingTokens
		p.CachedTokens += cachedTokens
		p.TotalTokens += totalTokens
		p.TotalCalls++

		costUSD, savingsUSD := CalculateTokensCostUSD(modelName, promptTokens, outputTokens, thinkingTokens, cachedTokens)
		p.EstimatedCostUSD += costUSD
		p.EstimatedSavingsUSD += savingsUSD

		if ts.After(p.LastActivity) {
			p.LastActivity = ts
		}
	}

	// 3. Truy vấn trực tiếp từ agent_fleet_telemetry
	fleetCond := parseAgentTimeCondition(timeRange, "started_at")
	fleetWhere := ""
	if fleetCond != "" {
		fleetWhere = "WHERE " + fleetCond
	}

	queryFleet := fmt.Sprintf(`
		SELECT 
			COALESCE(subagent_id, ''),
			COALESCE(role_name, ''),
			COALESCE(task_name, ''),
			COALESCE(status, 'COMPLETED'),
			started_at,
			COALESCE(tokens_offloaded, 0)
		FROM agent_fleet_telemetry
		%s
		ORDER BY started_at DESC
	`, fleetWhere)

	fRows, err := s.DB.Query(queryFleet)
	if err != nil {
		return nil, fmt.Errorf("GetAntigravityProjectStats: query agent_fleet_telemetry failed: %w", err)
	}
	defer fRows.Close()

	now := time.Now()
	for fRows.Next() {
		var subagentID, roleName, taskName, status string
		var startedAt time.Time
		var tokensOffloaded int64

		if err := fRows.Scan(&subagentID, &roleName, &taskName, &status, &startedAt, &tokensOffloaded); err != nil {
			continue
		}

		rawProjID, rawProjName := resolveSubagentProject(subagentID, roleName, taskName)
		projID, projName, ws := ResolveCrossLLMProject(rawProjID)
		if projName == "" {
			projName = rawProjName
		}

		p := ensureProject(projID, projName, ws)
		p.AgentTasks++
		p.OffloadedTokens += tokensOffloaded

		if status == "RUNNING" && now.Sub(startedAt) <= 2*time.Minute {
			p.ActiveTasks++
			p.IsRunning = true
		}

		if startedAt.After(p.LastActivity) {
			p.LastActivity = startedAt
		}
	}

	return result, nil
}

// isNilProvider kiểm tra an toàn xem một interface có chứa typed nil pointer hay không
func isNilProvider(i any) bool {
	if i == nil {
		return true
	}
	v := reflect.ValueOf(i)
	return v.Kind() == reflect.Ptr && v.IsNil()
}

// GetProjectsLeaderboard tổng hợp bảng xếp hạng FinOps đa LLM từ Google Antigravity, OpenAI Codex, và Anthropic Claude.
func (s *Storage) GetProjectsLeaderboard(timeRange, sortBy string, codexMonitor CodexProvider, claudeMonitor ClaudeProvider) (*ProjectLeaderboardResponseDTO, error) {
	// 1. Chuẩn hóa tham số đầu vào
	r := strings.ToLower(strings.TrimSpace(timeRange))
	if r == "" {
		r = "30d"
	}
	sortCrit := strings.ToLower(strings.TrimSpace(sortBy))
	if sortCrit == "" {
		sortCrit = "tokens"
	}

	type projectAccumulator struct {
		id             string
		name           string
		workspace      string
		status         string
		totalTokens    int64
		promptTokens   int64
		outputTokens   int64
		cachedTokens   int64
		thinkingTokens int64
		costUSD        float64
		savingsUSD     float64
		totalCalls     int64
		agentTasks     int64

		googleTokens   int64
		googlePrompt   int64
		googleOutput   int64
		googleCached   int64
		googleThinking int64
		googleCostUSD  float64
		googleSavings  float64
		googleCalls    int64
		googleTasks    int64

		openaiTokens   int64
		openaiPrompt   int64
		openaiOutput   int64
		openaiCached   int64
		openaiThinking int64
		openaiCostUSD  float64
		openaiSavings  float64
		openaiCalls    int64
		openaiTasks    int64

		claudeTokens   int64
		claudePrompt   int64
		claudeOutput   int64
		claudeCached   int64
		claudeThinking int64
		claudeCostUSD  float64
		claudeSavings  float64
		claudeCalls    int64
		claudeTasks    int64

		hasRunningGoogle bool
		hasRunningOpenAI bool
		hasRunningClaude bool
	}

	accMap := make(map[string]*projectAccumulator)
	getOrCreateAcc := func(id, name, ws string) *projectAccumulator {
		acc, exists := accMap[id]
		if !exists {
			acc = &projectAccumulator{
				id:        id,
				name:      name,
				workspace: ws,
				status:    "STANDBY",
			}
			accMap[id] = acc
		}
		if acc.name == "" {
			acc.name = name
		}
		if acc.workspace == "" {
			acc.workspace = ws
		}
		return acc
	}

	// 2. Thu thập dữ liệu Google Antigravity
	agStats, err := s.GetAntigravityProjectStats(r)
	if err != nil {
		return nil, fmt.Errorf("GetProjectsLeaderboard: Antigravity stats failed: %w", err)
	}
	for _, p := range agStats {
		acc := getOrCreateAcc(p.ProjectID, p.ProjectName, p.Workspace)
		acc.totalTokens += p.TotalTokens
		acc.promptTokens += p.PromptTokens
		acc.outputTokens += p.OutputTokens
		acc.cachedTokens += p.CachedTokens
		acc.thinkingTokens += p.ThinkingTokens
		acc.costUSD += p.EstimatedCostUSD
		acc.savingsUSD += p.EstimatedSavingsUSD
		acc.totalCalls += p.TotalCalls
		acc.agentTasks += p.AgentTasks

		acc.googleTokens += p.TotalTokens
		acc.googlePrompt += p.PromptTokens
		acc.googleOutput += p.OutputTokens
		acc.googleCached += p.CachedTokens
		acc.googleThinking += p.ThinkingTokens
		acc.googleCostUSD += p.EstimatedCostUSD
		acc.googleSavings += p.EstimatedSavingsUSD
		acc.googleCalls += p.TotalCalls
		acc.googleTasks += p.AgentTasks

		if p.IsRunning {
			acc.hasRunningGoogle = true
		}
	}

	// 3. Thu thập dữ liệu OpenAI Codex (nếu có monitor)
	if codexMonitor != nil && !isNilProvider(codexMonitor) {
		dash := codexMonitor.Dashboard(r)
		var inputRatio, cachedRatio, reasoningRatio float64
		if dash.Summary.TotalTokens > 0 {
			inputRatio = float64(dash.Summary.InputTokens) / float64(dash.Summary.TotalTokens)
			cachedRatio = float64(dash.Summary.CachedInputTokens) / float64(dash.Summary.TotalTokens)
			reasoningRatio = float64(dash.Summary.ReasoningTokens) / float64(dash.Summary.TotalTokens)
		} else {
			inputRatio = 0.8
			cachedRatio = 0.0
			reasoningRatio = 0.0
		}

		for _, sess := range dash.Sessions {
			pID, pName, pWs := ResolveCrossLLMProject(sess.Workspace)
			acc := getOrCreateAcc(pID, pName, pWs)

			totTok := sess.TotalTokens
			pTok := int64(float64(totTok) * inputRatio)
			oTok := totTok - pTok
			if oTok < 0 {
				oTok = 0
			}
			cTok := int64(float64(totTok) * cachedRatio)
			tTok := int64(float64(totTok) * reasoningRatio)

			nonCachedPrompt := pTok - cTok
			if nonCachedPrompt < 0 {
				nonCachedPrompt = 0
			}
			cost, sav := CalculateTokensCostUSD(sess.Model, nonCachedPrompt, oTok, 0, cTok)

			acc.totalTokens += totTok
			acc.promptTokens += pTok
			acc.outputTokens += oTok
			acc.cachedTokens += cTok
			acc.thinkingTokens += tTok
			acc.costUSD += cost
			acc.savingsUSD += sav
			acc.totalCalls += sess.Turns
			acc.agentTasks += sess.ToolCalls

			acc.openaiTokens += totTok
			acc.openaiPrompt += pTok
			acc.openaiOutput += oTok
			acc.openaiCached += cTok
			acc.openaiThinking += tTok
			acc.openaiCostUSD += cost
			acc.openaiSavings += sav
			acc.openaiCalls += sess.Turns
			acc.openaiTasks += sess.ToolCalls

			if sess.Status == "ACTIVE" {
				acc.hasRunningOpenAI = true
			}
		}
	}

	// 4. Thu thập dữ liệu Anthropic Claude (nếu có monitor)
	if claudeMonitor != nil && !isNilProvider(claudeMonitor) {
		dash := claudeMonitor.Dashboard(r)
		for _, sess := range dash.Sessions {
			rawProj := sess.Workspace
			if rawProj == "" {
				rawProj = sess.ProjectName
			}
			pID, pName, pWs := ResolveCrossLLMProject(rawProj)
			acc := getOrCreateAcc(pID, pName, pWs)

			pTok := sess.InputTokens
			oTok := sess.OutputTokens
			cTok := sess.CacheTokens
			tTok := sess.ThinkingTokens
			totTok := sess.TotalTokens

			cost, sav := CalculateTokensCostUSD(sess.Model, pTok, oTok, 0, cTok)

			acc.totalTokens += totTok
			acc.promptTokens += pTok
			acc.outputTokens += oTok
			acc.cachedTokens += cTok
			acc.thinkingTokens += tTok
			acc.costUSD += cost
			acc.savingsUSD += sav
			acc.totalCalls += int64(sess.Turns)
			acc.agentTasks += int64(sess.ToolCalls)

			acc.claudeTokens += totTok
			acc.claudePrompt += pTok
			acc.claudeOutput += oTok
			acc.claudeCached += cTok
			acc.claudeThinking += tTok
			acc.claudeCostUSD += cost
			acc.claudeSavings += sav
			acc.claudeCalls += int64(sess.Turns)
			acc.claudeTasks += int64(sess.ToolCalls)

			if sess.Status == "ACTIVE" {
				acc.hasRunningClaude = true
			}
		}
	}

	// 5. Tính toán các chỉ số KPI cấp hệ thống
	kpis := ProjectLeaderboardKPIsDTO{}
	for _, acc := range accMap {
		kpis.GrandTotalTokens += acc.totalTokens
		kpis.GrandTotalCostUSD += acc.costUSD
		kpis.GrandTotalSavingsUSD += acc.savingsUSD
		kpis.GrandTotalCalls += acc.totalCalls
		kpis.GrandTotalTasks += acc.agentTasks

		kpis.GoogleTotalTokens += acc.googleTokens
		kpis.GoogleTotalCostUSD += acc.googleCostUSD
		kpis.OpenAITotalTokens += acc.openaiTokens
		kpis.OpenAITotalCostUSD += acc.openaiCostUSD
		kpis.ClaudeTotalTokens += acc.claudeTokens
		kpis.ClaudeTotalCostUSD += acc.claudeCostUSD

		if acc.hasRunningGoogle || acc.hasRunningOpenAI || acc.hasRunningClaude {
			acc.status = "RUNNING"
			kpis.ActiveProjectsCount++
		} else if acc.totalTokens > 0 || acc.totalCalls > 0 || acc.agentTasks > 0 {
			acc.status = "COMPLETED"
		} else {
			acc.status = "STANDBY"
		}
	}
	kpis.GrandTotalActivity = kpis.GrandTotalCalls + kpis.GrandTotalTasks
	kpis.TotalProjectsCount = len(accMap)

	if kpis.GrandTotalTokens > 0 {
		kpis.GoogleTotalPercent = (float64(kpis.GoogleTotalTokens) * 100.0) / float64(kpis.GrandTotalTokens)
		kpis.OpenAITotalPercent = (float64(kpis.OpenAITotalTokens) * 100.0) / float64(kpis.GrandTotalTokens)
		kpis.ClaudeTotalPercent = (float64(kpis.ClaudeTotalTokens) * 100.0) / float64(kpis.GrandTotalTokens)
	}

	var totalAllInput, totalAllCached int64
	for _, acc := range accMap {
		totalAllInput += acc.promptTokens + acc.cachedTokens
		totalAllCached += acc.cachedTokens
	}
	if totalAllInput > 0 {
		kpis.OverallCacheHitPercent = (float64(totalAllCached) * 100.0) / float64(totalAllInput)
	}

	// 6. Xây dựng danh sách ProjectLeaderboardItemDTO với Invariant 100.0%
	items := make([]ProjectLeaderboardItemDTO, 0, len(accMap))
	for _, acc := range accMap {
		cacheHit := 0.0
		inputSum := acc.promptTokens + acc.cachedTokens
		if inputSum > 0 {
			cacheHit = (float64(acc.cachedTokens) * 100.0) / float64(inputSum)
		}

		var gPct, oPct, cPct float64
		if acc.totalTokens > 0 {
			gPct = (float64(acc.googleTokens) * 100.0) / float64(acc.totalTokens)
			oPct = (float64(acc.openaiTokens) * 100.0) / float64(acc.totalTokens)
			cPct = (float64(acc.claudeTokens) * 100.0) / float64(acc.totalTokens)

			// Đảm bảo invariant: gPct + oPct + cPct == 100.0% (loại bỏ sai số số thực float)
			diff := 100.0 - (gPct + oPct + cPct)
			if math.Abs(diff) > 0.000001 && math.Abs(diff) < 0.1 {
				if gPct >= oPct && gPct >= cPct {
					gPct += diff
				} else if oPct >= gPct && oPct >= cPct {
					oPct += diff
				} else {
					cPct += diff
				}
			}
		}

		tokShare := 0.0
		if kpis.GrandTotalTokens > 0 {
			tokShare = (float64(acc.totalTokens) * 100.0) / float64(kpis.GrandTotalTokens)
		}
		costShare := 0.0
		if kpis.GrandTotalCostUSD > 0 {
			costShare = (acc.costUSD * 100.0) / kpis.GrandTotalCostUSD
		}

		item := ProjectLeaderboardItemDTO{
			ProjectID:           acc.id,
			ProjectName:         acc.name,
			Workspace:           acc.workspace,
			Status:              acc.status,
			TotalTokens:         acc.totalTokens,
			PromptTokens:        acc.promptTokens,
			OutputTokens:        acc.outputTokens,
			CachedTokens:        acc.cachedTokens,
			ThinkingTokens:      acc.thinkingTokens,
			EstimatedCostUSD:    acc.costUSD,
			EstimatedSavingsUSD: acc.savingsUSD,
			TotalCalls:          acc.totalCalls,
			AgentTasks:          acc.agentTasks,
			TotalActivity:       acc.totalCalls + acc.agentTasks,
			CacheHitPercent:     cacheHit,
			TokenSharePercent:   tokShare,
			CostSharePercent:    costShare,
			GoogleBreakdown: ProjectProviderBreakdownDTO{
				Tokens:         acc.googleTokens,
				PromptTokens:   acc.googlePrompt,
				OutputTokens:   acc.googleOutput,
				CachedTokens:   acc.googleCached,
				ThinkingTokens: acc.googleThinking,
				CostUSD:        acc.googleCostUSD,
				SavingsUSD:     acc.googleSavings,
				Calls:          acc.googleCalls,
				Tasks:          acc.googleTasks,
				Activity:       acc.googleCalls + acc.googleTasks,
				Percentage:     gPct,
			},
			OpenAIBreakdown: ProjectProviderBreakdownDTO{
				Tokens:         acc.openaiTokens,
				PromptTokens:   acc.openaiPrompt,
				OutputTokens:   acc.openaiOutput,
				CachedTokens:   acc.openaiCached,
				ThinkingTokens: acc.openaiThinking,
				CostUSD:        acc.openaiCostUSD,
				SavingsUSD:     acc.openaiSavings,
				Calls:          acc.openaiCalls,
				Tasks:          acc.openaiTasks,
				Activity:       acc.openaiCalls + acc.openaiTasks,
				Percentage:     oPct,
			},
			ClaudeBreakdown: ProjectProviderBreakdownDTO{
				Tokens:         acc.claudeTokens,
				PromptTokens:   acc.claudePrompt,
				OutputTokens:   acc.claudeOutput,
				CachedTokens:   acc.claudeCached,
				ThinkingTokens: acc.claudeThinking,
				CostUSD:        acc.claudeCostUSD,
				SavingsUSD:     acc.claudeSavings,
				Calls:          acc.claudeCalls,
				Tasks:          acc.claudeTasks,
				Activity:       acc.claudeCalls + acc.claudeTasks,
				Percentage:     cPct,
			},
		}
		items = append(items, item)
	}

	// 7. Sắp xếp danh sách dự án
	switch sortCrit {
	case "cost":
		sort.Slice(items, func(i, j int) bool {
			if items[i].EstimatedCostUSD != items[j].EstimatedCostUSD {
				return items[i].EstimatedCostUSD > items[j].EstimatedCostUSD
			}
			if items[i].TotalTokens != items[j].TotalTokens {
				return items[i].TotalTokens > items[j].TotalTokens
			}
			return items[i].ProjectName < items[j].ProjectName
		})
	case "activity":
		sort.Slice(items, func(i, j int) bool {
			if items[i].TotalActivity != items[j].TotalActivity {
				return items[i].TotalActivity > items[j].TotalActivity
			}
			if items[i].TotalTokens != items[j].TotalTokens {
				return items[i].TotalTokens > items[j].TotalTokens
			}
			return items[i].ProjectName < items[j].ProjectName
		})
	default: // "tokens"
		sort.Slice(items, func(i, j int) bool {
			if items[i].TotalTokens != items[j].TotalTokens {
				return items[i].TotalTokens > items[j].TotalTokens
			}
			if items[i].EstimatedCostUSD != items[j].EstimatedCostUSD {
				return items[i].EstimatedCostUSD > items[j].EstimatedCostUSD
			}
			return items[i].ProjectName < items[j].ProjectName
		})
	}

	// 8. Gán thứ hạng Rank, PercentOfTop và trích xuất Top Consumer / Active
	if len(items) > 0 {
		var topVal float64
		switch sortCrit {
		case "cost":
			topVal = items[0].EstimatedCostUSD
		case "activity":
			topVal = float64(items[0].TotalActivity)
		default:
			topVal = float64(items[0].TotalTokens)
		}

		var topConsumerTok int64
		var topConsumerProj string
		var topActiveAct int64
		var topActiveProj string

		for idx := range items {
			items[idx].Rank = idx + 1
			if topVal > 0 {
				curVal := float64(items[idx].TotalTokens)
				if sortCrit == "cost" {
					curVal = items[idx].EstimatedCostUSD
				} else if sortCrit == "activity" {
					curVal = float64(items[idx].TotalActivity)
				}
				items[idx].PercentOfTop = (curVal * 100.0) / topVal
			}

			if items[idx].TotalTokens > topConsumerTok {
				topConsumerTok = items[idx].TotalTokens
				topConsumerProj = items[idx].ProjectName
			}
			if items[idx].TotalActivity > topActiveAct {
				topActiveAct = items[idx].TotalActivity
				topActiveProj = items[idx].ProjectName
			}
		}

		kpis.TopConsumerProject = topConsumerProj
		kpis.TopConsumerTokens = topConsumerTok
		if kpis.GrandTotalTokens > 0 {
			kpis.TopConsumerPercent = (float64(topConsumerTok) * 100.0) / float64(kpis.GrandTotalTokens)
		}
		kpis.TopActiveProject = topActiveProj
		kpis.TopActiveCount = topActiveAct
	}

	return &ProjectLeaderboardResponseDTO{
		TimeRange:   r,
		SortBy:      sortCrit,
		GeneratedAt: time.Now().UTC(),
		KPIs:        kpis,
		Projects:    items,
	}, nil
}
