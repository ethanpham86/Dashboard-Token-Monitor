package web_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tokenmonitor/collector"
	"tokenmonitor/config"
	"tokenmonitor/storage"
	"tokenmonitor/web"
)

func setupTestServer(t *testing.T) (*httptest.Server, func()) {
	t.Helper()

	// Ensure working directory is repo root
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get wd: %v", err)
	}
	// If in web subdirectory, cd to repo root
	if filepath.Base(wd) == "web" {
		_ = os.Chdir("..")
	}

	tmpDB := filepath.Join(os.TempDir(), "test_token_monitor_m3.db")
	_ = os.Remove(tmpDB)

	cfg := config.NewDefaultConfig()
	cfg.Database.SQLitePath = tmpDB

	store, err := storage.NewStorage(cfg)
	if err != nil {
		t.Fatalf("failed to init storage: %v", err)
	}

	buf := collector.NewAsyncBuffer(store, 1000, 100, 1*time.Second)
	tailer := collector.NewLocalTailer(cfg, buf)

	srv := web.NewServer(store, buf, tailer)
	ts := httptest.NewServer(srv.Routes())

	cleanup := func() {
		ts.Close()
		buf.Stop()
		store.Close()
		_ = os.Remove(tmpDB)
		if filepath.Base(wd) == "web" {
			_ = os.Chdir(wd)
		}
	}
	return ts, cleanup
}

func TestStation5_GetConfigYaml(t *testing.T) {
	ts, cleanup := setupTestServer(t)
	defer cleanup()

	resp, err := http.Get(ts.URL + "/config.yaml")
	if err != nil {
		t.Fatalf("Failed to GET /config.yaml: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected HTTP 200 for /config.yaml, got %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/plain") && !strings.Contains(ct, "text/yaml") && !strings.Contains(ct, "application/x-yaml") {
		t.Errorf("Expected text/plain or text/yaml Content-Type, got: %s", ct)
	}

	buf := make([]byte, 512)
	n, _ := resp.Body.Read(buf)
	bodySnippet := string(buf[:n])
	if !strings.Contains(bodySnippet, "server:") || !strings.Contains(bodySnippet, "dashboard_port") {
		t.Errorf("/config.yaml body missing expected config content: %s", bodySnippet)
	}
}

func TestStation5_HeadAndRangeRequests_ConfigYaml(t *testing.T) {
	ts, cleanup := setupTestServer(t)
	defer cleanup()

	// HEAD /config.yaml
	resp, err := http.Head(ts.URL + "/config.yaml")
	if err != nil {
		t.Fatalf("Failed to HEAD /config.yaml: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200 for HEAD /config.yaml, got %d", resp.StatusCode)
	}

	// Range request
	req, _ := http.NewRequest("GET", ts.URL+"/config.yaml", nil)
	req.Header.Set("Range", "bytes=0-15")
	respRange, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Range request failed: %v", err)
	}
	defer respRange.Body.Close()
	if respRange.StatusCode != http.StatusPartialContent && respRange.StatusCode != http.StatusOK {
		t.Errorf("Expected 206 or 200 for Range request, got %d", respRange.StatusCode)
	}
}

func TestStation5_DocsPortal_Files(t *testing.T) {
	ts, cleanup := setupTestServer(t)
	defer cleanup()

	urlsToCheck := []string{
		"/docs/index.html",
		"/docs/tokenmonitor-architecture.html",
		"/docs/tokenmonitor-dataflow.html",
		"/docs/tokenmonitor-database-erd.html",
		"/docs/TokenMonitor_Configuration_Guide.md",
		"/docs/TokenMonitor_Database_ERD.md",
		"/docs/README.md",
		// Runbooks
		"/docs/TokenMonitor_20260908/TokenMonitor_00-Prerequisites_20260908.md",
		"/docs/TokenMonitor_20260908/TokenMonitor_01-Architecture_20260908.md",
		"/docs/TokenMonitor_20260908/TokenMonitor_02-HA_Deployment_20260908.md",
		"/docs/TokenMonitor_20260908/TokenMonitor_03-Install_Deploy_20260908.md",
		"/docs/TokenMonitor_20260908/TokenMonitor_04-Tuning_20260908.md",
		"/docs/TokenMonitor_20260908/TokenMonitor_05-Troubleshooting_20260908.md",
		"/docs/TokenMonitor_20260908/TokenMonitor_06-Security_Policy_Compliance_20260908.md",
		// Group 4 References
		"/docs/TokenMonitor_20260908/references/TokenMonitor_Ref_001_schema.sql",
		"/docs/TokenMonitor_20260908/references/TokenMonitor_Ref_002_config_template.yaml",
		"/docs/TokenMonitor_20260908/references/TokenMonitor_Ref_003_gemini_usage_metadata_spec.json",
		"/docs/TokenMonitor_20260908/references/TokenMonitor_Ref_004_database_erd.md",
	}

	for _, u := range urlsToCheck {
		t.Run(u, func(t *testing.T) {
			resp, err := http.Get(ts.URL + u)
			if err != nil {
				t.Fatalf("Failed to GET %s: %v", u, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("Expected HTTP 200 for %s, got %d", u, resp.StatusCode)
			}
		})
	}
}

func TestStation5_DocsPortal_DirectAndTrailingSlash(t *testing.T) {
	ts, cleanup := setupTestServer(t)
	defer cleanup()

	// 1. Test /docs (without trailing slash) - client without auto-redirect
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Get(ts.URL + "/docs")
	if err != nil {
		t.Fatalf("Failed to GET /docs: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMovedPermanently && resp.StatusCode != http.StatusTemporaryRedirect && resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 301, 307 or 200 for /docs, got %d", resp.StatusCode)
	}

	// 2. Test /docs/ (with trailing slash)
	resp2, err := http.Get(ts.URL + "/docs/")
	if err != nil {
		t.Fatalf("Failed to GET /docs/: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Errorf("Expected 200 for /docs/, got %d", resp2.StatusCode)
	}
}

func TestStation5_APIEndpoints(t *testing.T) {
	ts, cleanup := setupTestServer(t)
	defer cleanup()

	endpoints := []string{
		"/api/account",
		"/api/metrics/summary",
		"/api/metrics/timeseries",
		"/api/metrics/daily",
		"/api/metrics/models",
	}

	for _, ep := range endpoints {
		t.Run(ep, func(t *testing.T) {
			resp, err := http.Get(ts.URL + ep)
			if err != nil {
				t.Fatalf("Failed to GET %s: %v", ep, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("Expected HTTP 200 for %s, got %d", ep, resp.StatusCode)
			}

			ct := resp.Header.Get("Content-Type")
			if !strings.Contains(ct, "application/json") {
				t.Errorf("Expected application/json for %s, got %s", ep, ct)
			}
		})
	}
}