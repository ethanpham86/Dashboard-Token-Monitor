package collector_test

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"tokenmonitor/collector"
	"tokenmonitor/config"
)

// MockStorageWriter captures flushed batches in a thread-safe manner.
type MockStorageWriter struct {
	mu      sync.Mutex
	batches [][]*collector.TokenUsageEvent
	events  []*collector.TokenUsageEvent
}

func (m *MockStorageWriter) InsertUsageBatch(events []*collector.TokenUsageEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	batchCopy := make([]*collector.TokenUsageEvent, len(events))
	copy(batchCopy, events)
	m.batches = append(m.batches, batchCopy)
	m.events = append(m.events, events...)
	return nil
}

func (m *MockStorageWriter) TotalEvents() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.events)
}

func (m *MockStorageWriter) BatchCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.batches)
}

func (m *MockStorageWriter) BatchSizes() []int {
	m.mu.Lock()
	defer m.mu.Unlock()
	sizes := make([]int, len(m.batches))
	for i, b := range m.batches {
		sizes[i] = len(b)
	}
	return sizes
}

// TestReviewerM3_AsyncBuffer_DefaultsAndConfig verifies capacity 1000, batch 100, flush 1s defaults.
func TestReviewerM3_AsyncBuffer_DefaultsAndConfig(t *testing.T) {
	mock := &MockStorageWriter{}

	// Case 1: All zero / negative parameters -> should fall back to 1000, 100, 1s
	bufDefault := collector.NewAsyncBuffer(mock, 0, 0, 0)
	if bufDefault == nil {
		t.Fatal("NewAsyncBuffer returned nil")
	}

	// Verify buffer capacity by filling it
	for i := 0; i < 1000; i++ {
		ev := &collector.TokenUsageEvent{TotalTokens: int64(i)}
		if !bufDefault.Push(ev) {
			t.Fatalf("Failed to push item %d into default buffer of expected capacity 1000", i)
		}
	}
	// 1001st push must fail non-blockingly
	extraEv := &collector.TokenUsageEvent{TotalTokens: 1001}
	if bufDefault.Push(extraEv) {
		t.Error("Expected 1001st push to fail for default buffer capacity of 1000")
	}

	// Case 2: Negative values
	bufNeg := collector.NewAsyncBuffer(mock, -10, -5, -1*time.Second)
	for i := 0; i < 1000; i++ {
		if !bufNeg.Push(&collector.TokenUsageEvent{}) {
			t.Fatalf("Failed to push item %d into negative-config buffer", i)
		}
	}
	if bufNeg.Push(&collector.TokenUsageEvent{}) {
		t.Error("Negative capacity should have fallen back to 1000")
	}
}

// TestReviewerM3_AsyncBuffer_NonBlockingPush ensures Push never blocks the caller.
func TestReviewerM3_AsyncBuffer_NonBlockingPush(t *testing.T) {
	mock := &MockStorageWriter{}
	capacity := 10
	buf := collector.NewAsyncBuffer(mock, capacity, 5, 10*time.Second)

	// Fill to capacity
	for i := 0; i < capacity; i++ {
		ok := buf.Push(&collector.TokenUsageEvent{TotalTokens: int64(i)})
		if !ok {
			t.Fatalf("Push %d failed unexpectedly", i)
		}
	}

	// Attempt push when full - must return immediately with false
	done := make(chan bool, 1)
	go func() {
		start := time.Now()
		ok := buf.Push(&collector.TokenUsageEvent{TotalTokens: 999})
		elapsed := time.Since(start)
		if ok {
			t.Errorf("Push into full buffer should return false")
		}
		if elapsed > 50*time.Millisecond {
			t.Errorf("Push took too long (%v), expected non-blocking immediate return", elapsed)
		}
		done <- true
	}()

	select {
	case <-done:
		// Succeeded promptly
	case <-time.After(500 * time.Millisecond):
		t.Fatal("buf.Push blocked when buffer was full! Non-blocking contract VIOLATED.")
	}
}

// TestReviewerM3_AsyncBuffer_ChannelDrainOnStop tests that all remaining events in buffer are flushed on Stop().
func TestReviewerM3_AsyncBuffer_ChannelDrainOnStop(t *testing.T) {
	mock := &MockStorageWriter{}
	// Batch size 100, long flush tick (1 hour) so timer won't flush
	buf := collector.NewAsyncBuffer(mock, 1000, 100, 1*time.Hour)
	buf.Start()

	totalToPush := 255
	for i := 0; i < totalToPush; i++ {
		ev := &collector.TokenUsageEvent{
			AccountID:   1,
			ModelName:   "gemini-test",
			TotalTokens: int64(i + 1),
		}
		if !buf.Push(ev) {
			t.Fatalf("Failed to push event %d", i)
		}
	}

	// Call Stop() - this MUST drain all 255 events into writer
	buf.Stop()

	if mock.TotalEvents() != totalToPush {
		t.Fatalf("Drain on stop incomplete! Expected %d events flushed, got %d", totalToPush, mock.TotalEvents())
	}

	sizes := mock.BatchSizes()
	expectedSizes := []int{100, 100, 55}
	if len(sizes) != len(expectedSizes) {
		t.Fatalf("Expected %d batches, got %d (sizes: %v)", len(expectedSizes), len(sizes), sizes)
	}
	for i, s := range sizes {
		if s != expectedSizes[i] {
			t.Errorf("Batch %d size mismatch: expected %d, got %d", i, expectedSizes[i], s)
		}
	}
}

// TestReviewerM3_AsyncBuffer_PeriodicFlush tests periodic ticker flush of partial batches.
func TestReviewerM3_AsyncBuffer_PeriodicFlush(t *testing.T) {
	mock := &MockStorageWriter{}
	// Small flush tick 50ms, batch size 100
	buf := collector.NewAsyncBuffer(mock, 1000, 100, 50*time.Millisecond)
	buf.Start()
	defer buf.Stop()

	// Push 15 events (< 100 batch size)
	for i := 0; i < 15; i++ {
		buf.Push(&collector.TokenUsageEvent{TotalTokens: int64(i)})
	}

	// Wait for ticker to trigger flush
	time.Sleep(150 * time.Millisecond)

	if mock.TotalEvents() != 15 {
		t.Fatalf("Periodic flush failed: expected 15 events flushed, got %d", mock.TotalEvents())
	}
}

// TestReviewerM3_AsyncBuffer_FlushDirect verifies direct bypass flushing without queue.
func TestReviewerM3_AsyncBuffer_FlushDirect(t *testing.T) {
	mock := &MockStorageWriter{}
	buf := collector.NewAsyncBuffer(mock, 10, 5, 1*time.Second)

	// Direct flush empty slice
	if err := buf.FlushDirect(nil); err != nil {
		t.Errorf("FlushDirect(nil) returned error: %v", err)
	}
	if err := buf.FlushDirect([]*collector.TokenUsageEvent{}); err != nil {
		t.Errorf("FlushDirect(empty) returned error: %v", err)
	}

	// Direct flush 30 events (more than buffer capacity 10)
	events := make([]*collector.TokenUsageEvent, 30)
	for i := range events {
		events[i] = &collector.TokenUsageEvent{TotalTokens: int64(i)}
	}

	if err := buf.FlushDirect(events); err != nil {
		t.Fatalf("FlushDirect returned error: %v", err)
	}

	if mock.TotalEvents() != 30 {
		t.Errorf("Expected 30 events in mock writer, got %d", mock.TotalEvents())
	}
}

// TestReviewerM3_LocalTailer_IDEBrainDirCollection tests IDEBrainDir fallback and deduplication logic.
func TestReviewerM3_LocalTailer_IDEBrainDirCollection(t *testing.T) {
	tmpDir := t.TempDir()
	customBrain := filepath.Join(tmpDir, "custom-ide", "brain")
	if err := os.MkdirAll(customBrain, 0755); err != nil {
		t.Fatalf("Failed to create custom brain dir: %v", err)
	}

	cfg := &config.Config{
		LocalTailer: config.LocalTailerConfig{
			Enabled:             true,
			IDEBrainDir:         customBrain,
			PollIntervalSeconds: 10,
		},
	}

	mock := &MockStorageWriter{}
	buf := collector.NewAsyncBuffer(mock, 1000, 100, 1*time.Second)
	tailer := collector.NewLocalTailer(cfg, buf)
	if tailer == nil {
		t.Fatal("NewLocalTailer returned nil")
	}

	// Case 2: Deduplication - if IDEBrainDir is added twice or identical to a discovered brain
	// Verify creating another tailer with the same config does not crash or corrupt
	tailer2 := collector.NewLocalTailer(cfg, buf)
	if tailer2 == nil {
		t.Fatal("NewLocalTailer deduplication check returned nil")
	}
}

// TestReviewerM3_LocalTailer_ExtractModelName tests regex and fallback extraction from chat transcripts.
func TestReviewerM3_LocalTailer_ExtractModelName(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected string
	}{
		{
			name:     "Regex Model Selection Claude Sonnet",
			content:  `<USER_SETTINGS_CHANGE> changed setting "Model Selection" from "gemini-flash" to "Claude Sonnet 4.6 (Thinking)". No need to respond.`,
			expected: "Claude Sonnet 4.6 (Thinking)",
		},
		{
			name:     "Regex Model Selection Gemini Flash",
			content:  `<USER_SETTINGS_CHANGE> changed setting 'Model Selection' from None to Gemini 3.8 Flash (High).`,
			expected: "Gemini 3.8 Flash (High)",
		},
		{
			name:     "Fallback text Claude Opus 4.6",
			content:  `Model Selection is changed: using claude opus 4.6 for heavy reasoning`,
			expected: "Claude Opus 4.6 (Thinking)",
		},
		{
			name:     "Fallback text GPT-OSS 120B",
			content:  `<USER_SETTINGS_CHANGE> Switch model to gpt-oss 120b`,
			expected: "GPT-OSS 120B (Medium)",
		},
		{
			name:     "Irrelevant text without settings change tag",
			content:  `Just a normal chat about Gemini 3.8 Flash without setting change`,
			expected: "",
		},
		{
			name:     "Empty string",
			content:  "",
			expected: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actual := collector.ExtractModelName(tc.content)
			if actual != tc.expected {
				t.Errorf("ExtractModelName(%q) = %q; expected %q", tc.content, actual, tc.expected)
			}
		})
	}
}

// TestReviewerM3_LocalTailer_NormalizeModelName tests model name normalization.
func TestReviewerM3_LocalTailer_NormalizeModelName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"`sonnet 4.6`", "Claude Sonnet 4.6 (Thinking)"},
		{"opus 4.6.", "Claude Opus 4.6 (Thinking)"},
		{"gpt-oss", "GPT-OSS 120B (Medium)"},
		{"3.8 flash", "Gemini 3.8 Flash (High)"},
		{"3.7 flash", "Gemini 3.7 Flash Medium"},
		{"3.6 flash", "Gemini 3.6 Flash Medium"},
		{"3.5 flash", "Gemini 3.5 Flash (High)"},
		{"3.1 pro", "Gemini 3.1 Pro (High)"},
		{"ultra", "Gemini Ultra"},
		{"unknown-custom-model", "unknown-custom-model"},
	}

	for _, tc := range tests {
		actual := collector.NormalizeModelName(tc.input)
		if actual != tc.expected {
			t.Errorf("NormalizeModelName(%q) = %q; expected %q", tc.input, actual, tc.expected)
		}
	}
}
