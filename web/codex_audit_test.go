package web_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"tokenmonitor/collector"
	"tokenmonitor/config"
	"tokenmonitor/web"
)

func TestCodexSnapshotAPIAndFullRefresh(t *testing.T) {
	root := t.TempDir()
	for _, id := range []string{"a", "b"} {
		line := `{"timestamp":"2026-09-15T00:00:00Z","type":"session_meta","payload":{"id":"` + id + `","cwd":"/project"}}` + "\n" + `{"timestamp":"2026-09-15T00:00:01Z","type":"token_usage_record","payload":{"usage":{"input_tokens":100,"total_tokens":100}}}` + "\n"
		if err := os.WriteFile(filepath.Join(root, id+".jsonl"), []byte(line), 0600); err != nil {
			t.Fatal(err)
		}
	}
	m := collector.NewCodexMonitor(config.OpenAIMonitorConfig{Enabled: true, SessionsDir: root, MaxSessionRows: 1})
	if err := m.Refresh(); err != nil {
		t.Fatal(err)
	}
	h := web.NewServer(nil, nil, nil, m).Routes()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/openai/dashboard?range=all&include_graph=1", nil))
	var d collector.CodexDashboardDTO
	if err := json.Unmarshal(w.Body.Bytes(), &d); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || d.Graph == nil || d.Graph.Nodes[0].Tokens != 200 || len(d.Sessions) != 1 || len(d.Graph.Nodes) != 4 {
		t.Fatalf("bad bundled snapshot: %s", w.Body.String())
	}
	for _, path := range []string{"/api/openai/dashboard?range=invalid", "/api/openai/graph?range=invalid"} {
		w = httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("invalid range accepted: %s", path)
		}
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/api/openai/graph", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatal("graph accepted mutation method")
	}
	if err := m.Refresh(); err != nil {
		t.Fatal(err)
	}
	if m.Dashboard("all").ScanStats.FilesReused != 2 {
		t.Fatal("scan cache not used")
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/api/openai/refresh", nil))
	if w.Code != 200 || m.Dashboard("all").ScanStats.FilesFull != 2 {
		t.Fatal("explicit refresh did not verify full source")
	}
}
