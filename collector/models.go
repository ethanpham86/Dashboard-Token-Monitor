package collector

import (
	"time"
)

// Gemini API response usageMetadata structures
type GeminiResponseWrapper struct {
	Candidates    []GeminiCandidate `json:"candidates,omitempty"`
	UsageMetadata *UsageMetadata    `json:"usageMetadata,omitempty"`
	ModelVersion  string            `json:"modelVersion,omitempty"`
}

type GeminiCandidate struct {
	FinishReason string `json:"finishReason,omitempty"`
	Index        int    `json:"index,omitempty"`
}

type UsageMetadata struct {
	PromptTokenCount        int64                 `json:"promptTokenCount"`
	CandidatesTokenCount    int64                 `json:"candidatesTokenCount"`
	TotalTokenCount         int64                 `json:"totalTokenCount"`
	CachedContentTokenCount int64                 `json:"cachedContentTokenCount"`
	CandidatesTokensDetails []CandidateTokenDetail `json:"candidatesTokensDetails,omitempty"`
	PromptTokensDetails     []PromptTokenDetail   `json:"promptTokensDetails,omitempty"`
}

type CandidateTokenDetail struct {
	Modality   string `json:"modality"`
	TokenCount int64  `json:"tokenCount"`
}

type PromptTokenDetail struct {
	Modality   string `json:"modality"`
	TokenCount int64  `json:"tokenCount"`
}

// TokenUsageEvent là bản ghi trung gian để đưa vào Ring Buffer và lưu vào SQLite
type TokenUsageEvent struct {
	AccountID      int64
	Timestamp      time.Time
	ModelName      string
	PromptTokens   int64
	OutputTokens   int64
	ThinkingTokens int64
	CachedTokens   int64
	TotalTokens    int64
	LatencyMs      int64
	StatusCode     int
	RequestType    string
}

// Trích xuất số lượng Thinking Tokens từ candidate details
func (u *UsageMetadata) GetThinkingTokens() int64 {
	if u == nil {
		return 0
	}
	for _, detail := range u.CandidatesTokensDetails {
		if detail.Modality == "THINKING" {
			return detail.TokenCount
		}
	}
	return 0
}

// AgentTaskEvent đại diện cho một tác vụ thực tế của Subagent được trích xuất từ transcript
type AgentTaskEvent struct {
	ID              int64  `json:"id"`
	SubagentID      string `json:"subagent_id"`
	RoleName        string `json:"role_name"`
	TaskName        string `json:"task_name"`
	Status          string `json:"status"` // RUNNING, COMPLETED, IDLE, ERROR
	StartedAt       string `json:"started_at"`
	FinishedAt      string `json:"finished_at"`
	DurationMs      int64  `json:"duration_ms"`
	TokensOffloaded int64  `json:"tokens_offloaded"`
}
