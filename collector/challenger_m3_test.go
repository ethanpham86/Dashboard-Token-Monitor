package collector

import (
	"os"
	"regexp"
	"sync"
	"testing"
	"time"
)

// mockStorageWriter implements StorageWriter for testing AsyncBuffer behavior
type mockStorageWriter struct {
	mu          sync.Mutex
	batches     [][]*TokenUsageEvent
	totalEvents int
}

func (m *mockStorageWriter) InsertUsageBatch(events []*TokenUsageEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Make a copy of events slice
	cp := make([]*TokenUsageEvent, len(events))
	copy(cp, events)
	m.batches = append(m.batches, cp)
	m.totalEvents += len(events)
	return nil
}

func (m *mockStorageWriter) GetTotalEvents() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.totalEvents
}

func (m *mockStorageWriter) GetBatchCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.batches)
}

// TestChallenger_Buffer_DefaultConfigValues tests that NewAsyncBuffer enforces
// capacity 1000, batchSize 100, and flushTick 1s when non-positive parameters are provided.
func TestChallenger_Buffer_DefaultConfigValues(t *testing.T) {
	mock := &mockStorageWriter{}

	testCases := []struct {
		name      string
		capacity  int
		batchSize int
		flushTick time.Duration
	}{
		{"All zeros", 0, 0, 0},
		{"All negative", -10, -100, -time.Second},
		{"Zero capacity only", 0, 50, 500 * time.Millisecond},
		{"Zero batch size only", 500, 0, 500 * time.Millisecond},
		{"Zero flush tick only", 500, 50, 0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			b := NewAsyncBuffer(mock, tc.capacity, tc.batchSize, tc.flushTick)

			expectedCap := tc.capacity
			if expectedCap <= 0 {
				expectedCap = 1000
			}
			expectedBatch := tc.batchSize
			if expectedBatch <= 0 {
				expectedBatch = 100
			}
			expectedTick := tc.flushTick
			if expectedTick <= 0 {
				expectedTick = 1 * time.Second
			}

			if cap(b.eventChan) != expectedCap {
				t.Errorf("expected channel capacity %d, got %d", expectedCap, cap(b.eventChan))
			}
			if b.batchSize != expectedBatch {
				t.Errorf("expected batchSize %d, got %d", expectedBatch, b.batchSize)
			}
			if b.flushTick != expectedTick {
				t.Errorf("expected flushTick %v, got %v", expectedTick, b.flushTick)
			}
		})
	}
}

// TestChallenger_Buffer_CapacityLimit_And_NonBlockingDrop verifies that the buffer
// can hold exactly 1000 items, and the 1001st push is dropped without blocking or panic.
func TestChallenger_Buffer_CapacityLimit_And_NonBlockingDrop(t *testing.T) {
	mock := &mockStorageWriter{}
	b := NewAsyncBuffer(mock, 1000, 100, 1*time.Second)
	// Do NOT call b.Start() so worker does not consume from channel

	if cap(b.eventChan) != 1000 {
		t.Fatalf("expected capacity 1000, got %d", cap(b.eventChan))
	}

	// Push 1000 events: all must succeed
	for i := 0; i < 1000; i++ {
		ev := &TokenUsageEvent{
			AccountID:   1,
			Timestamp:   time.Now(),
			ModelName:   "gemini-2.5-pro",
			TotalTokens: int64(i + 1),
		}
		accepted := b.Push(ev)
		if !accepted {
			t.Fatalf("expected push %d to be accepted, but was rejected", i)
		}
	}

	if len(b.eventChan) != 1000 {
		t.Fatalf("expected channel length 1000, got %d", len(b.eventChan))
	}

	// 1001st push: must return false (dropped non-blocking)
	overflowEvent := &TokenUsageEvent{
		AccountID:   1,
		Timestamp:   time.Now(),
		ModelName:   "gemini-2.5-pro",
		TotalTokens: 99999,
	}
	accepted := b.Push(overflowEvent)
	if accepted {
		t.Errorf("expected 1001st push to be rejected (buffer full), but returned true")
	}

	if len(b.eventChan) != 1000 {
		t.Errorf("channel length should remain 1000, got %d", len(b.eventChan))
	}
}

// TestChallenger_Buffer_BatchFlush_OnThreshold verifies that when batchSize (100) events
// are pushed, the worker flushes them immediately without waiting for the ticker.
func TestChallenger_Buffer_BatchFlush_OnThreshold(t *testing.T) {
	mock := &mockStorageWriter{}
	// Set very long flush tick so ticker cannot be responsible for the flush
	b := NewAsyncBuffer(mock, 1000, 100, 1*time.Hour)
	b.Start()
	defer b.Stop()

	// Push exactly 100 events
	for i := 0; i < 100; i++ {
		ev := &TokenUsageEvent{
			AccountID:   1,
			Timestamp:   time.Now(),
			ModelName:   "gemini-2.5-flash",
			TotalTokens: int64(i + 1),
		}
		if !b.Push(ev) {
			t.Fatalf("push %d failed", i)
		}
	}

	// Wait up to 500ms for worker to process the full batch
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if mock.GetTotalEvents() >= 100 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if mock.GetTotalEvents() != 100 {
		t.Errorf("expected 100 events flushed by batch threshold, got %d", mock.GetTotalEvents())
	}
	if mock.GetBatchCount() != 1 {
		t.Errorf("expected 1 batch flushed, got %d", mock.GetBatchCount())
	}
}

// TestChallenger_Buffer_FlushOnTicker verifies that partial batches (< 100) are
// flushed when the periodic flush ticker triggers.
func TestChallenger_Buffer_FlushOnTicker(t *testing.T) {
	mock := &mockStorageWriter{}
	// 50ms tick
	b := NewAsyncBuffer(mock, 1000, 100, 50*time.Millisecond)
	b.Start()
	defer b.Stop()

	// Push 35 events (< 100)
	for i := 0; i < 35; i++ {
		ev := &TokenUsageEvent{
			AccountID:   1,
			Timestamp:   time.Now(),
			ModelName:   "claude-sonnet-4.6",
			TotalTokens: int64(i + 1),
		}
		b.Push(ev)
	}

	// Wait for ticker (at least 150ms for 3 ticks)
	deadline := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(deadline) {
		if mock.GetTotalEvents() >= 35 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if mock.GetTotalEvents() != 35 {
		t.Errorf("expected 35 events flushed by ticker, got %d", mock.GetTotalEvents())
	}
}

// TestChallenger_Buffer_StopDrainsRemainingEvents verifies that calling Stop()
// drains and flushes any pending events remaining in the channel.
func TestChallenger_Buffer_StopDrainsRemainingEvents(t *testing.T) {
	mock := &mockStorageWriter{}
	// Very long tick
	b := NewAsyncBuffer(mock, 1000, 100, 1*time.Hour)
	b.Start()

	// Push 42 events
	for i := 0; i < 42; i++ {
		ev := &TokenUsageEvent{
			AccountID:   1,
			Timestamp:   time.Now(),
			ModelName:   "gemini-ultra",
			TotalTokens: int64(i + 1),
		}
		b.Push(ev)
	}

	// Stop buffer: should drain and flush all 42
	b.Stop()

	if mock.GetTotalEvents() != 42 {
		t.Errorf("expected 42 events drained on Stop(), got %d", mock.GetTotalEvents())
	}
}

// TestChallenger_MainGo_BufferParametersParity directly inspects main.go to ensure
// collector.NewAsyncBuffer is called with capacity 1000 and batchSize 100.
func TestChallenger_MainGo_BufferParametersParity(t *testing.T) {
	content, err := os.ReadFile("../main.go")
	if err != nil {
		t.Fatalf("failed to read main.go: %v", err)
	}

	// Look for: NewAsyncBuffer(store, 1000, 100, 1*time.Second)
	pattern := `collector\.NewAsyncBuffer\s*\(\s*store\s*,\s*1000\s*,\s*100\s*,\s*1\s*\*\s*time\.Second\s*\)`
	matched, err := regexp.Match(pattern, content)
	if err != nil {
		t.Fatalf("regex failed: %v", err)
	}
	if !matched {
		t.Errorf("main.go does not instantiate NewAsyncBuffer with exact parameters (store, 1000, 100, 1*time.Second)")
	}
}
