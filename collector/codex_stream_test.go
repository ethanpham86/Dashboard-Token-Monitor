package collector

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCodexIncrementalMatchesFullAcrossEveryRecord(t *testing.T) {
	now := time.Now().Add(-time.Hour)
	u := CodexTokenUsage{InputTokens: 100, OutputTokens: 20, TotalTokens: 120}
	lines := []string{
		codexTestLine(t, now, "session_meta", map[string]any{"id": "stream", "cwd": "/work/project"}),
		codexTestLine(t, now, "turn_context", map[string]any{"model": "model-a"}),
		codexTestLine(t, now, "event_msg", map[string]any{"type": "task_started"}),
		codexTestLine(t, now, "token_usage_record", map[string]any{"response_id": "r1", "usage": u, "thread_token_usage": u}),
		codexTestLine(t, now, "event_msg", map[string]any{"type": "token_count", "info": map[string]any{"total_token_usage": u}}),
		codexTestLine(t, now, "event_msg", map[string]any{"type": "item_completed", "item": map[string]any{"type": "CommandExecution", "exit_code": 0}}),
		codexTestLine(t, now, "response_item", map[string]any{"type": "function_call", "arguments": "private-command"}),
		codexTestLine(t, now, "event_msg", map[string]any{"type": "item_completed", "item": map[string]any{"type": "AgentMessage"}}),
		codexTestLine(t, now, "turn_context", map[string]any{"model": "model-b"}),
		codexTestLine(t, now, "event_msg", map[string]any{"type": "token_count", "info": map[string]any{"total_token_usage": CodexTokenUsage{InputTokens: 150, OutputTokens: 30, TotalTokens: 180}}}),
		codexTestLine(t, now, "event_msg", map[string]any{"type": "task_complete"}),
	}
	path := filepath.Join(t.TempDir(), "stream.jsonl")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var cached *codexParsedSession
	for i, line := range lines {
		if _, err := f.WriteString(line + "\n"); err != nil {
			t.Fatal(err)
		}
		inc, stats, err := readCodexSession(context.Background(), path, cached)
		if err != nil {
			t.Fatal(err)
		}
		full, _, err := readCodexSession(context.Background(), path, nil)
		if err != nil {
			t.Fatal(err)
		}
		incCompare, fullCompare := *inc, *full
		incCompare.stream.fileInfo, fullCompare.stream.fileInfo = nil, nil
		if !reflect.DeepEqual(incCompare, fullCompare) {
			t.Fatalf("incremental differs at record %d", i)
		}

		if i > 0 && (stats.FilesIncremental != 1 || stats.BytesParsed != int64(len(line)+1)) {
			t.Fatalf("history reread at %d: %+v", i, stats)
		}
		cached = inc
	}
}

func TestCodexIncrementalRewriteRotationAndFullVerification(t *testing.T) {
	ctx := context.Background()
	now := time.Now().Add(-time.Minute)
	line := func(n int) string {
		return codexTestLine(t, now, "token_usage_record", map[string]any{"usage": CodexTokenUsage{InputTokens: int64(n), TotalTokens: int64(n)}}) + "\n"
	}
	m := auditCodexMonitor(t, []string{strings.TrimSpace(line(100))})
	path := filepath.Join(m.sessionsDir, "rollout.jsonl")
	old := m.files[path]
	if err := os.WriteFile(path, []byte(line(200)+line(300)), 0600); err != nil {
		t.Fatal(err)
	}
	p, stats, err := readCodexSession(ctx, path, old)
	if err != nil {
		t.Fatal(err)
	}
	if stats.FilesFull != 1 || p.Usage[0].Usage.TotalTokens != 200 {
		t.Fatalf("rewrite not detected: %+v", stats)
	}
	if err := os.Rename(path, path+".old"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(line(100)+line(100)+line(100)), 0600); err != nil {
		t.Fatal(err)
	}
	_, stats, err = readCodexSession(ctx, path, p)
	if err != nil {
		t.Fatal(err)
	}
	if stats.FilesFull != 1 {
		t.Fatal("rotation reused old cursor")
	}
	if err := m.RefreshFull(); err != nil {
		t.Fatal(err)
	}
	if d := m.Dashboard("all"); d.ScanStats.FilesFull != 1 || d.Summary.TotalTokens != 300 {
		t.Fatalf("bad full refresh: %+v", d)
	}
	if err := m.Refresh(); err != nil {
		t.Fatal(err)
	}
	if stats := m.Dashboard("all").ScanStats; stats.FilesReused != 1 || stats.BytesParsed != 0 {
		t.Fatalf("unchanged file reread: %+v", stats)
	}
}

func TestCodexConcurrentRefreshAndDashboard(t *testing.T) {
	now := time.Now().Add(-time.Minute)
	line := codexTestLine(t, now, "token_usage_record", map[string]any{"usage": CodexTokenUsage{InputTokens: 1, TotalTokens: 1}}) + "\n"
	m := auditCodexMonitor(t, []string{strings.TrimSpace(line)})
	f, err := os.OpenFile(filepath.Join(m.sessionsDir, "rollout.jsonl"), os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for range 30 {
				_ = m.Dashboard("all")
				_ = m.Graph("all")
			}
		})
	}
	for range 10 {
		if _, err := f.WriteString(line); err != nil {
			t.Fatal(err)
		}
		if err := m.Refresh(); err != nil {
			t.Fatal(err)
		}
	}
	wg.Wait()
	if got := m.Dashboard("all").Summary.TotalTokens; got != 11 {
		t.Fatalf("total=%d", got)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := readCodexSession(canceled, f.Name(), nil); err != context.Canceled {
		t.Fatalf("cancel ignored: %v", err)
	}
}

func BenchmarkCodexSessionRead(b *testing.B) {
	path := filepath.Join(b.TempDir(), "large.jsonl")
	// Representative ignored content makes rescanning old history expensive.
	noise, _ := json.Marshal(map[string]any{"timestamp": "2026-09-15T00:00:00Z", "type": "response_item", "payload": map[string]any{"type": "message", "content": strings.Repeat("x", 2048)}})
	data := []byte(strings.Repeat(string(noise)+"\n", 8192))
	if err := os.WriteFile(path, data, 0600); err != nil {
		b.Fatal(err)
	}
	base, _, err := readCodexSession(context.Background(), path, nil)
	if err != nil {
		b.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		b.Fatal(err)
	}
	_, err = f.WriteString(`{"timestamp":"2026-09-15T00:01:00Z","type":"token_usage_record","payload":{"usage":{"input_tokens":100,"total_tokens":100}}}` + "\n")
	f.Close()
	if err != nil {
		b.Fatal(err)
	}
	for _, mode := range []string{"Full", "Append"} {
		b.Run(mode, func(b *testing.B) {
			b.ReportAllocs()
			var stats CodexScanStats
			for b.Loop() {
				var cached *codexParsedSession
				if mode == "Append" {
					cached = base
				}
				_, stats, err = readCodexSession(context.Background(), path, cached)
				if err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(stats.BytesParsed), "parsed-bytes/op")
			b.ReportMetric(float64(stats.BytesVerified), "verified-bytes/op")
		})
	}
}
