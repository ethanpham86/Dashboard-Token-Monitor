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

func auditCodexMonitor(t *testing.T, lines []string) *CodexMonitor {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "rollout.jsonl"), []byte(strings.Join(lines, "\n")+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	m := NewCodexMonitor(config.OpenAIMonitorConfig{Enabled: true, SessionsDir: root})
	if err := m.Refresh(); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestCodexAuditMixedFormatsAndResponseReplay(t *testing.T) {
	now := time.Now().Add(-time.Minute)
	u := CodexTokenUsage{InputTokens: 100, CachedInputTokens: 40, OutputTokens: 20, ReasoningOutputTokens: 10, TotalTokens: 120}
	for _, reverse := range []bool{false, true} {
		direct := codexTestLine(t, now, "token_usage_record", map[string]any{"response_id": "response-1", "usage": u, "thread_token_usage": u})
		event := codexTestLine(t, now, "event_msg", map[string]any{"type": "token_count", "info": map[string]any{"total_token_usage": u, "last_token_usage": u}})
		lines := []string{direct, event, direct, event}
		if reverse {
			lines = []string{event, direct, event, direct}
		}
		d := auditCodexMonitor(t, lines).Dashboard("all")
		if d.Summary.TotalTokens != 120 || d.Summary.ModelCalls != 1 || d.Summary.CachedInputTokens != 40 || d.Summary.ReasoningTokens != 10 {
			t.Fatalf("reverse=%v: %+v", reverse, d.Summary)
		}
	}
}

func TestCodexAuditCumulativeGapsResetAndModels(t *testing.T) {
	now := time.Now().Add(-time.Minute)
	line := func(n int64) string {
		return codexTestLine(t, now, "event_msg", map[string]any{"type": "token_count", "info": map[string]any{"total_token_usage": CodexTokenUsage{InputTokens: n, TotalTokens: n}}})
	}
	m := auditCodexMonitor(t, []string{
		codexTestLine(t, now, "turn_context", map[string]any{"model": "model-a"}), line(100), line(100), line(300),
		codexTestLine(t, now, "turn_context", map[string]any{"model": "model-b"}), line(50), line(80),
	})
	d := m.Dashboard("all")
	if d.Summary.TotalTokens != 330 || d.Summary.ModelCalls != 3 || d.ParseErrors != 1 || d.SourceStatus != "PARTIAL" {
		t.Fatalf("bad cumulative accounting: %+v", d)
	}
	if len(d.ModelTimeSeries) != 2 || d.ModelTimeSeries[0].TotalTokens != 300 || d.ModelTimeSeries[1].TotalTokens != 30 {
		t.Fatalf("bad model buckets: %+v", d.ModelTimeSeries)
	}
}

func TestCodexAuditCompletedTurnAndOpaqueTools(t *testing.T) {
	now := time.Now().Add(-time.Second)
	m := auditCodexMonitor(t, []string{
		codexTestLine(t, now, "event_msg", map[string]any{"type": "task_started"}),
		codexTestLine(t, now, "response_item", map[string]any{"type": "custom_tool_call", "input": "PRIVATE COMMAND"}),
		codexTestLine(t, now, "response_item", map[string]any{"type": "custom_tool_call_output", "output": "PRIVATE OUTPUT"}),
		codexTestLine(t, now, "event_msg", map[string]any{"type": "task_complete", "last_agent_message": "PRIVATE RESPONSE"}),
	})
	d := m.Dashboard("all")
	if d.Summary.ActiveSessions != 0 || d.Sessions[0].Status != "COMPLETED" || d.Summary.ToolCalls != 1 || d.Summary.ToolResultsKnown != 0 || d.Summary.ToolSuccessPercent != 0 {
		t.Fatalf("bad activity: %+v", d)
	}
	b, _ := json.Marshal(d)
	if strings.Contains(string(b), "PRIVATE") {
		t.Fatal("content leaked")
	}
}

func TestCodexAuditCoverageGraphAndDuplicateFiles(t *testing.T) {
	now := time.Now().Add(-time.Minute)
	m := auditCodexMonitor(t, []string{codexTestLine(t, now, "session_meta", map[string]any{"id": "one", "cwd": "/a/project"}), codexTestLine(t, now, "token_usage_record", map[string]any{"usage": CodexTokenUsage{InputTokens: 100, TotalTokens: 100}})})
	copyData, err := os.ReadFile(filepath.Join(m.sessionsDir, "rollout.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(m.sessionsDir, "copy.jsonl"), copyData, 0600); err != nil {
		t.Fatal(err)
	}
	other := codexTestLine(t, now, "session_meta", map[string]any{"id": "two", "cwd": "/b/project"}) + "\n" + codexTestLine(t, now, "token_usage_record", map[string]any{"usage": CodexTokenUsage{InputTokens: 200, TotalTokens: 200}}) + "\n"
	if err := os.WriteFile(filepath.Join(m.sessionsDir, "other.jsonl"), []byte(other), 0600); err != nil {
		t.Fatal(err)
	}
	m.cfg.MaxSessionRows = 1
	if err := m.Refresh(); err != nil {
		t.Fatal(err)
	}
	d := m.Dashboard("all")
	if d.Summary.TotalTokens != 300 || d.Summary.Sessions != 2 || len(d.Sessions) != 1 || !d.SessionsTruncated {
		t.Fatalf("bad dedup/row limit: %+v", d)
	}
	g := m.Graph("all")
	if len(g.Projects) != 2 || len(g.Nodes) != 5 || g.Nodes[0].Tokens != 300 {
		t.Fatalf("graph has fabricated or missing nodes: %+v", g)
	}
	m.cfg.MaxFiles = 1
	if err := m.Refresh(); err != nil {
		t.Fatal(err)
	}
	d = m.Dashboard("all")
	if !d.FilesTruncated || d.FilesDiscovered != 3 || d.SourceStatus != "PARTIAL" {
		t.Fatalf("missing coverage warning: %+v", d)
	}
}

func TestCodexAuditHomeQuotaAndErrorPrivacy(t *testing.T) {
	t.Setenv("CODEX_HOME", filepath.Join(t.TempDir(), "custom-home"))
	if got := resolveCodexSessionsDir(""); got != filepath.Join(os.Getenv("CODEX_HOME"), "sessions") {
		t.Fatalf("CODEX_HOME ignored: %s", got)
	}
	now := time.Now().Add(-time.Hour)
	rate := func(id string, n float64) string {
		return codexTestLine(t, now, "event_msg", map[string]any{"type": "token_count", "rate_limits": map[string]any{"limit_id": id, "primary": map[string]any{"used_percent": n, "window_minutes": 15}}})
	}
	d := auditCodexMonitor(t, []string{rate("codex", 20), rate("codex_other", 90)}).Dashboard("all")
	if d.RateLimits == nil || d.RateLimits.Primary.UsedPercent != 20 || !d.RateLimits.Stale {
		t.Fatalf("quota conflated: %+v", d.RateLimits)
	}
	m := NewCodexMonitor(config.OpenAIMonitorConfig{Enabled: true, SessionsDir: filepath.Join(t.TempDir(), "private-user-path")})
	_ = m.Refresh()
	b, _ := json.Marshal(m.Dashboard("all"))
	if strings.Contains(string(b), "private-user-path") {
		t.Fatal("source path leaked through API error")
	}
}

func TestCodexAuditAppendAndPartialLine(t *testing.T) {
	now := time.Now().Add(-time.Minute)
	line := codexTestLine(t, now, "token_usage_record", map[string]any{"usage": CodexTokenUsage{InputTokens: 100, TotalTokens: 100}})
	m := auditCodexMonitor(t, []string{line})
	path := filepath.Join(m.sessionsDir, "rollout.jsonl")
	if err := os.WriteFile(path, []byte(line+"\n"+line[:len(line)/2]), 0600); err != nil {
		t.Fatal(err)
	}
	if err := m.Refresh(); err != nil {
		t.Fatal(err)
	}
	if d := m.Dashboard("all"); d.Summary.TotalTokens != 100 || d.ParseErrors != 0 {
		t.Fatalf("partial write counted: %+v", d)
	}
	if err := os.WriteFile(path, []byte(line+"\n"+line+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := m.Refresh(); err != nil {
		t.Fatal(err)
	}
	if got := m.Dashboard("all").Summary.TotalTokens; got != 200 {
		t.Fatalf("append total = %d", got)
	}
	if err := os.WriteFile(path, []byte(line+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := m.Refresh(); err != nil {
		t.Fatal(err)
	}
	if got := m.Dashboard("all").Summary.TotalTokens; got != 100 {
		t.Fatalf("truncate total = %d", got)
	}
}
