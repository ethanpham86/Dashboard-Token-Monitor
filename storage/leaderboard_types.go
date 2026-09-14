package storage

import (
	"time"

	"tokenmonitor/collector"
)

// ProjectProviderBreakdownDTO chứa chi tiết các chỉ số được đóng góp bởi từng nhà cung cấp LLM cụ thể trong 1 dự án.
type ProjectProviderBreakdownDTO struct {
	Tokens         int64   `json:"tokens"`
	PromptTokens   int64   `json:"prompt_tokens"`
	OutputTokens   int64   `json:"output_tokens"`
	CachedTokens   int64   `json:"cached_tokens"`
	ThinkingTokens int64   `json:"thinking_tokens"`
	CostUSD        float64 `json:"cost_usd"`
	SavingsUSD     float64 `json:"savings_usd"`
	Calls          int64   `json:"calls"`
	Tasks          int64   `json:"tasks"`
	Activity       int64   `json:"activity"`
	Percentage     float64 `json:"percentage"` // % đóng góp vào tổng tokens của dự án này (0.0 - 100.0)
}

// ProjectLeaderboardItemDTO đại diện cho 1 dòng dự án được xếp hạng trong bảng xếp hạng FinOps hợp nhất.
type ProjectLeaderboardItemDTO struct {
	Rank                int                         `json:"rank"`
	ProjectID           string                      `json:"project_id"`
	ProjectName         string                      `json:"project_name"`
	Workspace           string                      `json:"workspace"`
	Status              string                      `json:"status"` // "RUNNING", "COMPLETED", "STANDBY"
	TotalTokens         int64                       `json:"total_tokens"`
	PromptTokens        int64                       `json:"prompt_tokens"`
	OutputTokens        int64                       `json:"output_tokens"`
	CachedTokens        int64                       `json:"cached_tokens"`
	ThinkingTokens      int64                       `json:"thinking_tokens"`
	EstimatedCostUSD    float64                     `json:"estimated_cost_usd"`
	EstimatedSavingsUSD float64                     `json:"estimated_savings_usd"`
	TotalCalls          int64                       `json:"total_calls"`
	AgentTasks          int64                       `json:"agent_tasks"`
	TotalActivity       int64                       `json:"total_activity"`
	CacheHitPercent     float64                     `json:"cache_hit_percent"`
	TokenSharePercent   float64                     `json:"token_share_percent"` // % trên Grand Total Tokens toàn hệ thống
	CostSharePercent    float64                     `json:"cost_share_percent"`  // % trên Grand Total Cost toàn hệ thống
	PercentOfTop        float64                     `json:"percent_of_top"`      // % so với dự án dẫn đầu (#1) theo tiêu chí sắp xếp hiện tại
	GoogleBreakdown     ProjectProviderBreakdownDTO `json:"google_breakdown"`
	OpenAIBreakdown     ProjectProviderBreakdownDTO `json:"openai_breakdown"`
	ClaudeBreakdown     ProjectProviderBreakdownDTO `json:"claude_breakdown"`
}

// ProjectLeaderboardKPIsDTO tổng hợp các chỉ số điều hành cấp cao cho các thẻ KPI Header.
type ProjectLeaderboardKPIsDTO struct {
	GrandTotalTokens       int64   `json:"grand_total_tokens"`
	GrandTotalCostUSD      float64 `json:"grand_total_cost_usd"`
	GrandTotalSavingsUSD   float64 `json:"grand_total_savings_usd"`
	GrandTotalCalls        int64   `json:"grand_total_calls"`
	GrandTotalTasks        int64   `json:"grand_total_tasks"`
	GrandTotalActivity     int64   `json:"grand_total_activity"`
	TotalProjectsCount     int     `json:"total_projects_count"`
	ActiveProjectsCount    int     `json:"active_projects_count"`
	OverallCacheHitPercent float64 `json:"overall_cache_hit_percent"`
	TopConsumerProject     string  `json:"top_consumer_project"`
	TopConsumerTokens      int64   `json:"top_consumer_tokens"`
	TopConsumerPercent     float64 `json:"top_consumer_percent"`
	TopActiveProject       string  `json:"top_active_project"`
	TopActiveCount         int64   `json:"top_active_count"`
	GoogleTotalTokens      int64   `json:"google_total_tokens"`
	GoogleTotalCostUSD     float64 `json:"google_total_cost_usd"`
	GoogleTotalPercent     float64 `json:"google_total_percent"`
	OpenAITotalTokens      int64   `json:"openai_total_tokens"`
	OpenAITotalCostUSD     float64 `json:"openai_total_cost_usd"`
	OpenAITotalPercent     float64 `json:"openai_total_percent"`
	ClaudeTotalTokens      int64   `json:"claude_total_tokens"`
	ClaudeTotalCostUSD     float64 `json:"claude_total_cost_usd"`
	ClaudeTotalPercent     float64 `json:"claude_total_percent"`
}

// ProjectLeaderboardResponseDTO là cấu trúc dữ liệu trả về hoàn chỉnh cho endpoint GET /api/projects/leaderboard.
type ProjectLeaderboardResponseDTO struct {
	TimeRange   string                      `json:"time_range"`
	SortBy      string                      `json:"sort_by"` // "tokens", "cost", "activity"
	GeneratedAt time.Time                   `json:"generated_at"`
	KPIs        ProjectLeaderboardKPIsDTO   `json:"kpis"`
	Projects    []ProjectLeaderboardItemDTO `json:"projects"`
}

// AntigravityProjectStats lưu trữ số liệu tổng hợp của Google Antigravity cho 1 dự án.
type AntigravityProjectStats struct {
	ProjectID           string    `json:"project_id"`
	ProjectName         string    `json:"project_name"`
	Workspace           string    `json:"workspace"`
	TotalTokens         int64     `json:"total_tokens"`
	PromptTokens        int64     `json:"prompt_tokens"`
	OutputTokens        int64     `json:"output_tokens"`
	CachedTokens        int64     `json:"cached_tokens"`
	ThinkingTokens      int64     `json:"thinking_tokens"`
	EstimatedCostUSD    float64   `json:"estimated_cost_usd"`
	EstimatedSavingsUSD float64   `json:"estimated_savings_usd"`
	TotalCalls          int64     `json:"total_calls"`
	AgentTasks          int64     `json:"agent_tasks"`
	OffloadedTokens     int64     `json:"offloaded_tokens"`
	ActiveTasks         int       `json:"active_tasks"`
	IsRunning           bool      `json:"is_running"`
	LastActivity        time.Time `json:"last_activity"`
}

// CodexProvider giao diện trừu tượng để tách rời Storage khỏi collector.CodexMonitor.
type CodexProvider interface {
	Dashboard(timeRange string) collector.CodexDashboardDTO
}

// ClaudeProvider giao diện trừu tượng để tách rời Storage khỏi collector.ClaudeMonitor.
type ClaudeProvider interface {
	Dashboard(timeRange string) collector.ClaudeDashboardDTO
}
