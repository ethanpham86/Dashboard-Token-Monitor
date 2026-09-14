package collector

import (
	"os"
	"path/filepath"
	"testing"

	"tokenmonitor/config"
)

func TestClaudeMonitor_ParseAndDashboard(t *testing.T) {
	tempDir := t.TempDir()
	projDir := filepath.Join(tempDir, "projects", "test-project")
	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatalf("Failed to create temp project dir: %v", err)
	}

	sampleLines := []string{
		`{"type":"queue-operation","operation":"enqueue","timestamp":"2026-08-01T10:00:00.000Z","sessionId":"sess-123"}`,
		`{"type":"user","message":{"role":"user","content":"Run unit tests"},"timestamp":"2026-08-01T10:00:01.000Z","sessionId":"sess-123","cwd":"E:\\WorkSpace\\DemoProject"}`,
		`{"type":"assistant","message":{"role":"assistant","model":"claude-3-7-sonnet","content":[{"type":"thinking","thinking":"Let me check the test files."},{"type":"tool_use","name":"Bash"}],"usage":{"input_tokens":1500,"output_tokens":350,"cache_creation_input_tokens":500,"cache_read_input_tokens":1200}},"timestamp":"2026-08-01T10:00:05.000Z","sessionId":"sess-123","cwd":"E:\\WorkSpace\\DemoProject"}`,
		`{"type":"user","message":{"role":"user","content":"Fix the failing test"},"timestamp":"2026-08-01T10:01:00.000Z","sessionId":"sess-123","cwd":"E:\\WorkSpace\\DemoProject"}`,
		`{"type":"assistant","message":{"role":"assistant","model":"claude-3-7-sonnet","content":[{"type":"tool_use","name":"Edit"}],"usage":{"input_tokens":2000,"output_tokens":400,"cache_creation_input_tokens":0,"cache_read_input_tokens":1800}},"timestamp":"2026-08-01T10:01:10.000Z","sessionId":"sess-123","cwd":"E:\\WorkSpace\\DemoProject"}`,
	}

	sessionFile := filepath.Join(projDir, "session_test.jsonl")
	content := ""
	for _, l := range sampleLines {
		content += l + "\n"
	}
	if err := os.WriteFile(sessionFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write mock jsonl: %v", err)
	}

	cfg := config.ClaudeMonitorConfig{
		Enabled:             true,
		ProjectsDir:         filepath.Join(tempDir, "projects"),
		PollIntervalSeconds: 1,
		MaxSessionRows:      10,
		MaxFiles:            50,
	}

	monitor := NewClaudeMonitor(cfg)
	if err := monitor.Refresh(); err != nil {
		t.Fatalf("Refresh failed: %v", err)
	}

	dashboard := monitor.Dashboard("all")
	if dashboard.SourceStatus != "READY" {
		t.Errorf("Expected SourceStatus READY, got %s", dashboard.SourceStatus)
	}
	if dashboard.Summary.TotalTokens != (1500 + 350 + 500 + 1200 + 2000 + 400 + 1800) {
		t.Errorf("Unexpected TotalTokens: got %d", dashboard.Summary.TotalTokens)
	}
	if dashboard.Summary.InputTokens != (1500 + 2000) {
		t.Errorf("Unexpected InputTokens: got %d", dashboard.Summary.InputTokens)
	}
	if dashboard.Summary.OutputTokens != (350 + 400) {
		t.Errorf("Unexpected OutputTokens: got %d", dashboard.Summary.OutputTokens)
	}
	if dashboard.Summary.CacheWriteTokens != 500 {
		t.Errorf("Unexpected CacheWriteTokens: got %d", dashboard.Summary.CacheWriteTokens)
	}
	if dashboard.Summary.CacheReadTokens != (1200 + 1800) {
		t.Errorf("Unexpected CacheReadTokens: got %d", dashboard.Summary.CacheReadTokens)
	}
	if dashboard.Summary.ToolCalls != 2 {
		t.Errorf("Expected 2 tool calls, got %d", dashboard.Summary.ToolCalls)
	}
	if dashboard.Summary.UserTurns != 2 {
		t.Errorf("Expected 2 user turns, got %d", dashboard.Summary.UserTurns)
	}
	if len(dashboard.Sessions) != 1 {
		t.Fatalf("Expected 1 session, got %d", len(dashboard.Sessions))
	}
	if dashboard.Sessions[0].ProjectName != "DemoProject" {
		t.Errorf("Expected project DemoProject, got %s", dashboard.Sessions[0].ProjectName)
	}

	// Verify Graph generation
	graph := monitor.Graph("all")
	if graph == nil {
		t.Fatal("Expected non-nil graph")
	}
	if len(graph.Nodes) < 3 {
		t.Errorf("Expected at least 3 nodes in graph, got %d", len(graph.Nodes))
	}
	if len(graph.Links) < 2 {
		t.Errorf("Expected at least 2 links in graph, got %d", len(graph.Links))
	}
}

func TestClaudeMonitorEmptyGraph(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.ClaudeMonitorConfig{
		Enabled:     true,
		ProjectsDir: tempDir,
	}
	monitor := NewClaudeMonitor(cfg)
	_ = monitor.Refresh()
	graph := monitor.Graph("all")
	if len(graph.Nodes) != 0 || len(graph.Projects) != 0 {
		t.Fatalf("Expected 0 nodes and 0 projects for empty Claude monitor, got %d nodes", len(graph.Nodes))
	}
}
