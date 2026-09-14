package collector

import (
	"sync"
	"testing"
	"time"
)

// TestBuffer_CanonicalLifecycle verifies the full buffer lifecycle:
// capacity 1000, non-blocking drop, batch 100 flush, tick flush, and drain on stop.
func TestBuffer_CanonicalLifecycle(t *testing.T) {
	mock := &canonicalMockWriter{}

	// 1. Enforce defaults when non-positive values are passed
	bDefault := NewAsyncBuffer(mock, 0, 0, 0)
	if cap(bDefault.eventChan) != 1000 {
		t.Errorf("Expected default capacity 1000, got %d", cap(bDefault.eventChan))
	}
	if bDefault.batchSize != 100 {
		t.Errorf("Expected default batchSize 100, got %d", bDefault.batchSize)
	}
	if bDefault.flushTick != 1*time.Second {
		t.Errorf("Expected default flushTick 1s, got %v", bDefault.flushTick)
	}

	// 2. Capacity 1000 limit & Non-blocking drop
	b := NewAsyncBuffer(mock, 1000, 100, 50*time.Millisecond)
	// Do not start worker yet, fill buffer to exactly 1000
	for i := 0; i < 1000; i++ {
		ev := &TokenUsageEvent{AccountID: 1, TotalTokens: int64(i + 1)}
		if !b.Push(ev) {
			t.Fatalf("Push failed at index %d within 1000 capacity", i)
		}
	}

	// 1001st push MUST be dropped non-blockingly and return false
	droppedEv := &TokenUsageEvent{AccountID: 1, TotalTokens: 9999}
	if b.Push(droppedEv) {
		t.Errorf("1001st push should have returned false (dropped)")
	}

	// 3. Start worker and verify Drain on Stop
	b.Start()
	b.Stop()

	// All 1000 events must be drained into writer
	if mock.Count() != 1000 {
		t.Errorf("Expected 1000 events drained on Stop(), got %d", mock.Count())
	}
}

// TestBuffer_Batch100Flush verifies that when 100 items are pushed,
// they are immediately flushed to the writer without waiting for timer.
func TestBuffer_Batch100Flush(t *testing.T) {
	mock := &canonicalMockWriter{}
	// Large flush tick (1 hour) so flush happens ONLY due to batchSize (100)
	b := NewAsyncBuffer(mock, 1000, 100, 1*time.Hour)
	b.Start()
	defer b.Stop()

	for i := 0; i < 100; i++ {
		ev := &TokenUsageEvent{AccountID: 1, TotalTokens: int64(i + 1)}
		b.Push(ev)
	}

	// Allow short window for worker to flush the batch
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if mock.Count() >= 100 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if mock.Count() != 100 {
		t.Errorf("Expected 100 events flushed by batch threshold, got %d", mock.Count())
	}
}

// TestBuffer_TickFlush verifies that partial batches (< 100) are flushed when the periodic ticker fires.
func TestBuffer_TickFlush(t *testing.T) {
	mock := &canonicalMockWriter{}
	// 50ms tick
	b := NewAsyncBuffer(mock, 1000, 100, 50*time.Millisecond)
	b.Start()
	defer b.Stop()

	// Push 25 events (< 100)
	for i := 0; i < 25; i++ {
		ev := &TokenUsageEvent{AccountID: 1, TotalTokens: int64(i + 1)}
		b.Push(ev)
	}

	// Wait up to 300ms for ticker to trigger
	deadline := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(deadline) {
		if mock.Count() >= 25 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if mock.Count() != 25 {
		t.Errorf("Expected 25 events flushed by tick, got %d", mock.Count())
	}
}

// TestBuffer_ConcurrentPush ensures buffer handles multiple concurrent producers cleanly without race.
func TestBuffer_ConcurrentPush(t *testing.T) {
	mock := &canonicalMockWriter{}
	b := NewAsyncBuffer(mock, 1000, 50, 20*time.Millisecond)
	b.Start()

	var wg sync.WaitGroup
	goroutines := 10
	eventsPerRoutine := 50

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(routineID int) {
			defer wg.Done()
			for i := 0; i < eventsPerRoutine; i++ {
				ev := &TokenUsageEvent{
					AccountID:   1,
					TotalTokens: int64(routineID*100 + i),
				}
				b.Push(ev)
			}
		}(g)
	}

	wg.Wait()
	b.Stop()

	expectedTotal := goroutines * eventsPerRoutine
	if mock.Count() != expectedTotal {
		t.Errorf("Expected %d total concurrent events flushed, got %d", expectedTotal, mock.Count())
	}
}
