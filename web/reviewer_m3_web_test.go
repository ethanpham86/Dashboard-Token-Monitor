package web_test

import (
	"encoding/json"
	"io"
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

func setupTestWebServer(t *testing.T) (*httptest.Server, *storage.Storage, func()) {
	t.Helper()

	// Ensure working directory is repository root so relative paths "./docs" and "config.yaml" resolve
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get wd: %v", err)
	}
	if filepath.Base(wd) == "web" {
		_ = os.Chdir("..")
	}

	tmpDir, err := os.MkdirTemp("", "tokenmonitor_web_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := &config.Config{
		Database: config.DatabaseConfig{
			SQLitePath:    dbPath,
			MaxOpenConns:  5,
			MaxIdleConns:  2,
			EnableWALMode: true,
		},
		AccountProfile: config.AccountProfileConfig{
			Email:              "web_test@example.com",
			AccountType:        "Google Consumer Account (Individual)",
			PlanName:           "20X ULTRA PLAN",
			QuotaBandwidth:     "20x Quota Bandwidth",
			SubscriptionStart:  "2026-09-01",
			SubscriptionExpiry: "2026-10-01",
			AutoRenew:          true,
		},
	}

	store, err := storage.NewStorage(cfg)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to create storage: %v", err)
	}

	buf := collector.NewAsyncBuffer(store, 100, 10, 100*time.Millisecond)
	server := web.NewServer(store, buf, nil)
	ts := httptest.NewServer(server.Routes())

	cleanup := func() {
		ts.Close()
		buf.Stop()
		store.Close()
		os.RemoveAll(tmpDir)
		if filepath.Base(wd) == "web" {
			_ = os.Chdir(wd)
		}
	}

	return ts, store, cleanup
}

// TestReviewerM3_Web_ConfigYamlRoute verifies that /config.yaml serves the config.yaml file correctly.
func TestReviewerM3_Web_ConfigYamlRoute(t *testing.T) {
	ts, _, cleanup := setupTestWebServer(t)
	defer cleanup()

	resp, err := http.Get(ts.URL + "/config.yaml")
	if err != nil {
		t.Fatalf("Failed to GET /config.yaml: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected HTTP 200 for /config.yaml, got %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/plain") {
		t.Errorf("Expected Content-Type text/plain, got %s", ct)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read body: %v", err)
	}

	bodyStr := string(body)
	if !strings.Contains(bodyStr, "server:") || !strings.Contains(bodyStr, "database:") {
		t.Errorf("Expected config.yaml contents, got: %s", bodyStr)
	}
}

// TestReviewerM3_Web_DocsRoute verifies that /docs/ routes serve documentation files.
func TestReviewerM3_Web_DocsRoute(t *testing.T) {
	ts, _, cleanup := setupTestWebServer(t)
	defer cleanup()

	testDocs := []struct {
		path         string
		expectedCode int
		expectedText string
	}{
		{"/docs/", http.StatusOK, "TokenMonitor"},
		{"/docs/tokenmonitor-dataflow.html", http.StatusOK, "TokenMonitor"},
		{"/docs/TokenMonitor_Configuration_Guide.md", http.StatusOK, "config.yaml"},
		{"/docs/TokenMonitor_Database_ERD.md", http.StatusOK, "token_usage_hourly_rollup"},
		{"/docs/non_existent_doc_12345.md", http.StatusNotFound, "404 page not found"},
	}

	for _, td := range testDocs {
		t.Run(td.path, func(t *testing.T) {
			resp, err := http.Get(ts.URL + td.path)
			if err != nil {
				t.Fatalf("Failed to GET %s: %v", td.path, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != td.expectedCode {
				t.Errorf("GET %s expected status %d, got %d", td.path, td.expectedCode, resp.StatusCode)
			}

			body, _ := io.ReadAll(resp.Body)
			if !strings.Contains(string(body), td.expectedText) {
				t.Errorf("GET %s body missing expected text '%s'", td.path, td.expectedText)
			}
		})
	}
}

// TestReviewerM3_Web_DocsPathTraversal verifies that path traversal outside ./docs is prevented.
func TestReviewerM3_Web_DocsPathTraversal(t *testing.T) {
	ts, _, cleanup := setupTestWebServer(t)
	defer cleanup()

	// Attempting to access ../main.go via /docs/
	resp, err := http.Get(ts.URL + "/docs/../main.go")
	if err != nil {
		t.Fatalf("Failed to GET /docs/../main.go: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	if strings.Contains(bodyStr, "func main()") {
		t.Errorf("Security vulnerability: path traversal leaked main.go through /docs/")
	}
}

// TestReviewerM3_Web_REST_APIs verifies all REST API endpoints for Station 5.
func TestReviewerM3_Web_REST_APIs(t *testing.T) {
	ts, _, cleanup := setupTestWebServer(t)
	defer cleanup()

	endpoints := []struct {
		method       string
		path         string
		expectedCode int
	}{
		{http.MethodGet, "/healthz", http.StatusOK},
		{http.MethodGet, "/api/account", http.StatusOK},
		{http.MethodGet, "/api/metrics/summary?range=all", http.StatusOK},
		{http.MethodGet, "/api/metrics/timeseries?range=24h", http.StatusOK},
		{http.MethodGet, "/api/metrics/daily?range=30d", http.StatusOK},
		{http.MethodGet, "/api/metrics/daily?days=7", http.StatusOK},
		{http.MethodGet, "/api/metrics/models?range=all", http.StatusOK},
		{http.MethodGet, "/api/metrics/models/timeseries?range=30d&model=gemini-2.5-pro", http.StatusOK},
		{http.MethodGet, "/api/sync/history", http.StatusOK},
		{http.MethodPost, "/api/test/simulate?model=gemini-2.5-pro", http.StatusOK},
		{http.MethodGet, "/api/test/simulate", http.StatusMethodNotAllowed},
		{http.MethodGet, "/", http.StatusOK},
	}

	client := &http.Client{}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			req, err := http.NewRequest(ep.method, ts.URL+ep.path, nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != ep.expectedCode {
				t.Errorf("%s %s expected status %d, got %d", ep.method, ep.path, ep.expectedCode, resp.StatusCode)
			}

			// For 200 OK JSON responses, verify valid JSON
			if resp.StatusCode == http.StatusOK && strings.Contains(resp.Header.Get("Content-Type"), "application/json") {
				var js map[string]any
				var jsArr []any
				body, _ := io.ReadAll(resp.Body)
				errMap := json.Unmarshal(body, &js)
				errArr := json.Unmarshal(body, &jsArr)
				if errMap != nil && errArr != nil {
					t.Errorf("%s %s returned invalid JSON: %v", ep.method, ep.path, string(body))
				}
			}
		})
	}
}
