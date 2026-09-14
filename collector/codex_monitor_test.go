package collector

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tokenmonitor/config"
)

func TestCodexMonitorParsesUsageWorkflowAndRateLimits(t *testing.T) {
	root := t.TempDir()
	now := time.Now().UTC().Truncate(time.Second)
	path := filepath.Join(root, "2026", "09", "12", "rollout-test-session.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	lines := []string{
		codexTestLine(t, now, "session_meta", map[string]any{"id": "session-123", "session_id": "session-123", "cwd": filepath.Join(root, "safe-project"), "model_provider": "openai", "originator": "codex", "timestamp": now.Format(time.RFC3339Nano)}),
		codexTestLine(t, now.Add(time.Second), "turn_context", map[string]any{"model": "gpt-test-codex", "cwd": filepath.Join(root, "safe-project")}),
		codexTestLine(t, now.Add(2*time.Second), "event_msg", map[string]any{"type": "task_started"}),
		codexTestLine(t, now.Add(3*time.Second), "response_item", map[string]any{"type": "message", "content": "super-secret-prompt-must-not-leak"}),
		codexTestLine(t, now.Add(4*time.Second), "token_usage_record", map[string]any{"usage": map[string]any{"input_tokens": 100, "cached_input_tokens": 40, "cache_write_input_tokens": 5, "output_tokens": 50, "reasoning_output_tokens": 20, "total_tokens": 150}}),
		codexTestLine(t, now.Add(5*time.Second), "event_msg", map[string]any{"type": "item_completed", "item": map[string]any{"type": "CommandExecution", "exit_code": 0}}),
		codexTestLine(t, now.Add(6*time.Second), "event_msg", map[string]any{"type": "item_completed", "item": map[string]any{"type": "CommandExecution", "exit_code": 1}}),
		codexTestLine(t, now.Add(7*time.Second), "event_msg", map[string]any{"type": "item_completed", "item": map[string]any{"type": "AgentMessage"}}),
		codexTestLine(t, now.Add(8*time.Second), "event_msg", map[string]any{"type": "token_count", "rate_limits": map[string]any{"limit_id": "codex", "plan_type": "plus", "primary": map[string]any{"used_percent": 25.5, "window_minutes": 300, "resets_at": now.Add(time.Hour).Unix()}, "secondary": map[string]any{"used_percent": 10, "window_minutes": 10080, "resets_at": now.Add(7 * 24 * time.Hour).Unix()}, "credits": map[string]any{"has_credits": true, "unlimited": false, "balance": "12.50"}}}),
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	monitor := NewCodexMonitor(config.OpenAIMonitorConfig{Enabled: true, SessionsDir: root, MaxFiles: 10, MaxSessionRows: 10})
	if err := monitor.Refresh(); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	dashboard := monitor.Dashboard("all")
	if dashboard.SourceStatus != "READY" || dashboard.FilesScanned != 1 {
		t.Fatalf("unexpected source state: %+v", dashboard)
	}
	if dashboard.Summary.TotalTokens != 150 || dashboard.Summary.InputTokens != 100 || dashboard.Summary.CachedInputTokens != 40 {
		t.Fatalf("unexpected token summary: %+v", dashboard.Summary)
	}
	if dashboard.Summary.OutputTokens != 50 || dashboard.Summary.ReasoningTokens != 20 || dashboard.Summary.CacheWriteTokens != 5 {
		t.Fatalf("unexpected output summary: %+v", dashboard.Summary)
	}
	if dashboard.Summary.Turns != 1 || dashboard.Summary.ToolCalls != 2 || dashboard.Summary.ToolFailures != 1 || dashboard.Summary.AssistantResponses != 1 {
		t.Fatalf("unexpected workflow summary: %+v", dashboard.Summary)
	}
	if dashboard.Summary.CacheHitPercent != 40 || dashboard.Summary.ToolSuccessPercent != 50 {
		t.Fatalf("unexpected efficiency summary: %+v", dashboard.Summary)
	}
	if dashboard.RateLimits == nil || dashboard.RateLimits.Primary == nil || dashboard.RateLimits.Primary.UsedPercent != 25.5 {
		t.Fatalf("unexpected rate limits: %+v", dashboard.RateLimits)
	}
	if len(dashboard.Models) != 1 || dashboard.Models[0].ModelName != "gpt-test-codex" {
		t.Fatalf("unexpected models: %+v", dashboard.Models)
	}
	if len(dashboard.Sessions) != 1 || dashboard.Sessions[0].Workspace != "safe-project" {
		t.Fatalf("unexpected sessions: %+v", dashboard.Sessions)
	}
	encoded, err := json.Marshal(dashboard)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "super-secret-prompt-must-not-leak") || strings.Contains(string(encoded), root) {
		t.Fatalf("dashboard leaked content or full source path: %s", encoded)
	}
}

func TestCodexMonitorAppliesTimeRange(t *testing.T) {
	root := t.TempDir()
	now := time.Now().UTC().Truncate(time.Second)
	path := filepath.Join(root, "rollout-range.jsonl")
	lines := []string{
		codexTestLine(t, now.AddDate(0, 0, -60), "session_meta", map[string]any{"id": "range-session", "cwd": filepath.Join(root, "range-project")}),
		codexTestLine(t, now.AddDate(0, 0, -60), "turn_context", map[string]any{"model": "old-model"}),
		codexTestLine(t, now.AddDate(0, 0, -60), "token_usage_record", map[string]any{"usage": map[string]any{"input_tokens": 900, "output_tokens": 100, "total_tokens": 1000}}),
		codexTestLine(t, now, "turn_context", map[string]any{"model": "current-model"}),
		codexTestLine(t, now, "token_usage_record", map[string]any{"usage": map[string]any{"input_tokens": 90, "output_tokens": 10, "total_tokens": 100}}),
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	monitor := NewCodexMonitor(config.OpenAIMonitorConfig{Enabled: true, SessionsDir: root, MaxFiles: 10, MaxSessionRows: 10})
	if err := monitor.Refresh(); err != nil {
		t.Fatal(err)
	}
	if got := monitor.Dashboard("30d").Summary.TotalTokens; got != 100 {
		t.Fatalf("30d total = %d, want 100", got)
	}
	if got := monitor.Dashboard("all").Summary.TotalTokens; got != 1100 {
		t.Fatalf("all total = %d, want 1100", got)
	}
}

func TestCodexMonitorUnavailableDirectory(t *testing.T) {
	monitor := NewCodexMonitor(config.OpenAIMonitorConfig{Enabled: true, SessionsDir: filepath.Join(t.TempDir(), "missing")})
	if err := monitor.Refresh(); err == nil {
		t.Fatal("Refresh() error = nil, want missing directory error")
	}
	if got := monitor.Dashboard("all").SourceStatus; got != "UNAVAILABLE" {
		t.Fatalf("SourceStatus = %q, want UNAVAILABLE", got)
	}
}

func TestCodexMonitorParsesTokenCountEvents(t *testing.T) {
	root := t.TempDir()
	now := time.Now().UTC().Truncate(time.Second)
	path := filepath.Join(root, "rollout-token-count.jsonl")

	turn1Total := map[string]any{"input_tokens": 3598, "cached_input_tokens": 2944, "output_tokens": 339, "reasoning_output_tokens": 256, "total_tokens": 3937}
	turn1Last := map[string]any{"input_tokens": 3598, "cached_input_tokens": 2944, "output_tokens": 339, "reasoning_output_tokens": 256, "total_tokens": 3937}
	turn2Total := map[string]any{"input_tokens": 7570, "cached_input_tokens": 5888, "output_tokens": 440, "reasoning_output_tokens": 256, "total_tokens": 8010}
	turn2Last := map[string]any{"input_tokens": 3972, "cached_input_tokens": 2944, "output_tokens": 101, "reasoning_output_tokens": 0, "total_tokens": 4073}

	lines := []string{
		codexTestLine(t, now, "session_meta", map[string]any{"id": "session-tc-1", "session_id": "session-tc-1", "cwd": filepath.Join(root, "my-code"), "model_provider": "openai"}),
		codexTestLine(t, now.Add(time.Second), "turn_context", map[string]any{"model": "cx/gpt-5.5"}),
		// Turn 1 event
		codexTestLine(t, now.Add(2*time.Second), "event_msg", map[string]any{"type": "token_count", "info": map[string]any{"total_token_usage": turn1Total, "last_token_usage": turn1Last}}),
		// Duplicate turn 1 event (e.g. rate limits update) - should NOT double count
		codexTestLine(t, now.Add(3*time.Second), "event_msg", map[string]any{"type": "token_count", "info": map[string]any{"total_token_usage": turn1Total, "last_token_usage": turn1Last}}),
		// Turn 2 event
		codexTestLine(t, now.Add(4*time.Second), "event_msg", map[string]any{"type": "token_count", "info": map[string]any{"total_token_usage": turn2Total, "last_token_usage": turn2Last}}),
	}

	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	monitor := NewCodexMonitor(config.OpenAIMonitorConfig{Enabled: true, SessionsDir: root, MaxFiles: 10, MaxSessionRows: 10})
	if err := monitor.Refresh(); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	dashboard := monitor.Dashboard("all")
	if dashboard.Summary.TotalTokens != 8010 {
		t.Fatalf("Summary.TotalTokens = %d, want 8010", dashboard.Summary.TotalTokens)
	}
	if dashboard.Summary.InputTokens != (3598 + 3972) {
		t.Fatalf("Summary.InputTokens = %d, want %d", dashboard.Summary.InputTokens, 3598+3972)
	}
	if dashboard.Summary.CachedInputTokens != (2944 + 2944) {
		t.Fatalf("Summary.CachedInputTokens = %d, want %d", dashboard.Summary.CachedInputTokens, 2944+2944)
	}
	if dashboard.Summary.OutputTokens != (339 + 101) {
		t.Fatalf("Summary.OutputTokens = %d, want %d", dashboard.Summary.OutputTokens, 339+101)
	}
	if dashboard.Summary.ReasoningTokens != 256 {
		t.Fatalf("Summary.ReasoningTokens = %d, want 256", dashboard.Summary.ReasoningTokens)
	}
	if len(dashboard.Models) != 1 || dashboard.Models[0].ModelName != "cx/gpt-5.5" {
		t.Fatalf("Models = %+v, want cx/gpt-5.5", dashboard.Models)
	}

	// Test Graph with data
	graph := monitor.Graph("all")
	if len(graph.Nodes) == 0 {
		t.Fatal("Graph should have nodes when sessions exist")
	}

	// Test Graph with empty monitor
	emptyMonitor := NewCodexMonitor(config.OpenAIMonitorConfig{Enabled: true, SessionsDir: t.TempDir()})
	_ = emptyMonitor.Refresh()
	emptyGraph := emptyMonitor.Graph("all")
	if len(emptyGraph.Nodes) != 0 || len(emptyGraph.Projects) != 0 {
		t.Fatalf("Empty monitor graph should have 0 nodes and 0 projects, got %d nodes", len(emptyGraph.Nodes))
	}
}

func codexTestLine(t *testing.T, timestamp time.Time, recordType string, payload any) string {
	t.Helper()
	encoded, err := json.Marshal(map[string]any{"timestamp": timestamp.Format(time.RFC3339Nano), "type": recordType, "payload": payload})
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}
