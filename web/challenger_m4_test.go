package web_test

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"tokenmonitor/collector"
	"tokenmonitor/config"
	"tokenmonitor/storage"
	"tokenmonitor/web"
)

type localMockStorageWriter struct {
	mu     sync.Mutex
	events []*collector.TokenUsageEvent
}

func (m *localMockStorageWriter) InsertUsageBatch(events []*collector.TokenUsageEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, events...)
	return nil
}

// TestChallengerM4_Web_ClosedDB_HTTP500Responses verifies that all API endpoints gracefully
// return HTTP 500 Internal Server Error when database fails, without crashing or panicking.
func TestChallengerM4_Web_ClosedDB_HTTP500Responses(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "closed_web.db")

	cfg := &config.Config{
		Database: config.DatabaseConfig{
			SQLitePath: dbPath,
		},
		AccountProfile: config.AccountProfileConfig{
			Email: "closed_web@example.com",
		},
	}

	store, err := storage.NewStorage(cfg)
	if err != nil {
		t.Fatalf("NewStorage failed: %v", err)
	}

	// Close database to simulate total DB connection loss
	store.Close()

	server := web.NewServer(store, nil, nil)
	router := server.Routes()

	endpoints := []string{
		"/api/account",
		"/api/metrics/summary",
		"/api/metrics/timeseries",
		"/api/metrics/daily",
		"/api/metrics/models",
		"/api/metrics/models/timeseries",
	}

	for _, ep := range endpoints {
		t.Run(ep, func(t *testing.T) {
			req := httptest.NewRequest("GET", ep, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusInternalServerError {
				t.Errorf("Endpoint %s: expected HTTP 500 on closed DB, got %d (body: %s)",
					ep, rec.Code, rec.Body.String())
			}
		})
	}
}

// TestChallengerM4_Web_SQLInjectionAndAdversarialParams tests resilience against SQL injection
// payloads in query parameters.
func TestChallengerM4_Web_SQLInjectionAndAdversarialParams(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "sqli_test.db")

	cfg := &config.Config{
		Database: config.DatabaseConfig{
			SQLitePath: dbPath,
		},
		AccountProfile: config.AccountProfileConfig{
			Email: "sqli_user@example.com",
		},
	}

	store, err := storage.NewStorage(cfg)
	if err != nil {
		t.Fatalf("NewStorage failed: %v", err)
	}
	defer store.Close()

	server := web.NewServer(store, nil, nil)
	router := server.Routes()

	adversarialURLs := []string{
		"/api/metrics/summary?range=1';+DROP+TABLE+accounts;--",
		"/api/metrics/timeseries?range=1'+UNION+SELECT+1,2,3,4,5,6,7,8,9,10,11,12;--",
		"/api/metrics/daily?range='--",
		"/api/metrics/models?range=1'+OR+'1'='1",
		"/api/metrics/models/timeseries?range=24h&model=gemini'+OR+1=1;--",
	}

	for _, u := range adversarialURLs {
		req := httptest.NewRequest("GET", u, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		// Must not panic, must respond with valid HTTP status (200 or 500)
		if rec.Code != http.StatusOK && rec.Code != http.StatusInternalServerError {
			t.Errorf("Unexpected status code %d for adversarial URL: %s", rec.Code, u)
		}
	}

	// Verify accounts table still exists and is not dropped
	var count int
	err = store.DB.QueryRow("SELECT COUNT(*) FROM accounts").Scan(&count)
	if err != nil {
		t.Fatalf("CRITICAL VULNERABILITY: accounts table was dropped or corrupted via SQL injection! %v", err)
	}
	if count == 0 {
		t.Fatalf("CRITICAL: accounts table was emptied via SQL injection!")
	}
}

// TestChallengerM4_Web_DocsPortal_AdversarialPathTraversals tests that path traversal
// attempts cannot escape docs directory and never leak source code files.
func TestChallengerM4_Web_DocsPortal_AdversarialPathTraversals(t *testing.T) {
	server := web.NewServer(nil, nil, nil)
	router := server.Routes()

	traversals := []string{
		"/docs/../main.go",
		"/docs/../../main.go",
		"/docs/..%2f..%2fmain.go",
		"/docs/%2e%2e%2fmain.go",
		"/docs/nonexistent_file_12345.html",
	}

	for _, path := range traversals {
		req := httptest.NewRequest("GET", path, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		// Must be rejected, sanitized or 404 (never 200 leaking source code)
		if rec.Code == http.StatusOK {
			t.Errorf("Path traversal %s returned HTTP 200! Possible file leakage.", path)
		}
		if strings.Contains(rec.Body.String(), "package main") {
			t.Fatalf("CRITICAL SECURITY LEAK: main.go source code leaked via %s!", path)
		}
	}
}

// TestChallengerM4_Web_SimulateEvent_AdversarialInputs tests method enforcement and
// special characters in simulation endpoint.
func TestChallengerM4_Web_SimulateEvent_AdversarialInputs(t *testing.T) {
	mock := &localMockStorageWriter{}
	buf := collector.NewAsyncBuffer(mock, 100, 10, 100*time.Millisecond)
	buf.Start()
	defer buf.Stop()

	server := web.NewServer(nil, buf, nil)
	router := server.Routes()

	// 1. GET method should be rejected with 405 Method Not Allowed
	reqGet := httptest.NewRequest("GET", "/api/test/simulate", nil)
	recGet := httptest.NewRecorder()
	router.ServeHTTP(recGet, reqGet)

	if recGet.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected 405 Method Not Allowed for GET /api/test/simulate, got %d", recGet.Code)
	}

	// 2. DELETE method should be rejected with 405
	reqDel := httptest.NewRequest("DELETE", "/api/test/simulate", nil)
	recDel := httptest.NewRecorder()
	router.ServeHTTP(recDel, reqDel)

	if recDel.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected 405 Method Not Allowed for DELETE /api/test/simulate, got %d", recDel.Code)
	}

	// 3. POST with XSS payload in model name should succeed safely
	reqPost := httptest.NewRequest("POST", "/api/test/simulate?model=<script>alert('xss')</script>", nil)
	recPost := httptest.NewRecorder()
	router.ServeHTTP(recPost, reqPost)

	if recPost.Code != http.StatusOK {
		t.Errorf("Expected 200 for POST /api/test/simulate, got %d", recPost.Code)
	}
}
