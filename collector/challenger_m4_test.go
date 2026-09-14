package collector

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"tokenmonitor/config"
)

// failingStorageWriter simulates storage failures (e.g. disk full, connection drop, lock)
type failingStorageWriter struct {
	mu        sync.Mutex
	failCount int
	callCount int
}

func (f *failingStorageWriter) InsertUsageBatch(events []*TokenUsageEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.callCount++
	f.failCount++
	return fmt.Errorf("simulated database failure: disk full or database locked (call %d)", f.callCount)
}

// TestChallengerM4_Buffer_GoroutineLeak tests that starting and stopping AsyncBuffer
// does not leak goroutines.
func TestChallengerM4_Buffer_GoroutineLeak(t *testing.T) {
	runtime.GC()
	time.Sleep(50 * time.Millisecond)
	baseGoroutines := runtime.NumGoroutine()

	const numBuffers = 10
	mock := &canonicalMockWriter{}
	buffers := make([]*AsyncBuffer, numBuffers)

	for i := 0; i < numBuffers; i++ {
		b := NewAsyncBuffer(mock, 100, 10, 50*time.Millisecond)
		b.Start()
		buffers[i] = b

		for j := 0; j < 5; j++ {
			b.Push(&TokenUsageEvent{TotalTokens: int64(j)})
		}
	}

	for _, b := range buffers {
		b.Stop()
	}

	runtime.GC()
	time.Sleep(100 * time.Millisecond)

	finalGoroutines := runtime.NumGoroutine()
	if finalGoroutines > baseGoroutines+2 {
		t.Errorf("Leaked goroutines detected! Base: %d, Final: %d (leak count: %d)",
			baseGoroutines, finalGoroutines, finalGoroutines-baseGoroutines)
	}
}

// TestChallengerM4_Buffer_WriterErrorInjection tests buffer resilience when the underlying
// StorageWriter consistently fails. The buffer must log and discard without panicking or hanging.
func TestChallengerM4_Buffer_WriterErrorInjection(t *testing.T) {
	failingWriter := &failingStorageWriter{}
	b := NewAsyncBuffer(failingWriter, 500, 20, 30*time.Millisecond)
	b.Start()

	for i := 0; i < 100; i++ {
		ev := &TokenUsageEvent{
			AccountID:   1,
			ModelName:   "gemini-test",
			TotalTokens: int64(i + 1),
		}
		if !b.Push(ev) {
			t.Fatalf("Push %d failed unexpectedly", i)
		}
	}

	time.Sleep(150 * time.Millisecond)
	b.Stop()

	failingWriter.mu.Lock()
	calls := failingWriter.callCount
	failingWriter.mu.Unlock()

	if calls == 0 {
		t.Errorf("Expected writer to be called despite errors, got 0 calls")
	}
}

// TestChallengerM4_Buffer_ConcurrentPushAndStopRace stresses concurrent pushes while
// stopping the buffer, ensuring no data race panic occurs.
func TestChallengerM4_Buffer_ConcurrentPushAndStopRace(t *testing.T) {
	mock := &canonicalMockWriter{}
	b := NewAsyncBuffer(mock, 1000, 50, 10*time.Millisecond)
	b.Start()

	var wg sync.WaitGroup
	const producers = 10
	const eventsPerProducer = 100

	for i := 0; i < producers; i++ {
		wg.Add(1)
		go func(pid int) {
			defer wg.Done()
			for j := 0; j < eventsPerProducer; j++ {
				ev := &TokenUsageEvent{
					AccountID:   1,
					ModelName:   "gemini-stress",
					TotalTokens: int64(pid*1000 + j),
				}
				b.Push(ev)
				time.Sleep(100 * time.Microsecond)
			}
		}(i)
	}

	time.Sleep(5 * time.Millisecond)
	b.Stop()
	wg.Wait()
}

// TestChallengerM4_Tailer_CorruptJSONLines tests that LocalTailer skips corrupted,
// truncated, or non-JSON lines in transcript.jsonl without panicking or halting.
func TestChallengerM4_Tailer_CorruptJSONLines(t *testing.T) {
	tmpDir := t.TempDir()
	convDir := filepath.Join(tmpDir, "conv_corrupt", ".system_generated", "logs")
	if err := os.MkdirAll(convDir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	transcriptPath := filepath.Join(convDir, "transcript.jsonl")

	corruptContent := "" +
		"{\"step_index\": 0, \"source\": \"USER\", \"content\": \"hello\"}\n" +
		"MALFORMED_NON_JSON_CONTENT_HERE\n" +
		"{truncated_json: true,\n" +
		"\n" +
		"{\"step_index\": 1, \"source\": \"MODEL\", \"type\": \"PLANNER_RESPONSE\", \"content\": \"Valid response!\"}\n" +
		"{}\n" +
		"{\"step_index\": 2, \"source\": \"MODEL\", \"type\": \"PLANNER_RESPONSE\", \"content\": \"Second valid!\"}\n"

	if err := os.WriteFile(transcriptPath, []byte(corruptContent), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	mock := &canonicalMockWriter{}
	buf := NewAsyncBuffer(mock, 100, 10, 10*time.Millisecond)
	buf.Start()
	defer buf.Stop()

	cfg := &config.Config{
		LocalTailer: config.LocalTailerConfig{
			Enabled:     true,
			IDEBrainDir: tmpDir,
		},
	}
	tailer := NewLocalTailer(cfg, buf)
	// Explicitly isolate tailer roots to the test directory to avoid scanning real user files
	tailer.rootDirs = []string{tmpDir}

	count, err := tailer.BackfillAllHistory()
	if err != nil {
		t.Fatalf("BackfillAllHistory failed with error on corrupt lines: %v", err)
	}

	if count != 2 {
		t.Errorf("Expected exactly 2 valid MODEL events ingested, got %d", count)
	}
}

// TestChallengerM4_Tailer_GoroutineLifecycle tests that LocalTailer Start and Stop
// cleanly terminates its polling goroutines.
func TestChallengerM4_Tailer_GoroutineLifecycle(t *testing.T) {
	runtime.GC()
	time.Sleep(50 * time.Millisecond)
	baseGoroutines := runtime.NumGoroutine()

	tmpDir := t.TempDir()
	mock := &canonicalMockWriter{}
	buf := NewAsyncBuffer(mock, 100, 10, 10*time.Millisecond)
	buf.Start()
	defer buf.Stop()

	cfg := &config.Config{
		LocalTailer: config.LocalTailerConfig{
			Enabled:             true,
			IDEBrainDir:         tmpDir,
			PollIntervalSeconds: 1,
		},
	}
	tailer := NewLocalTailer(cfg, buf)
	tailer.rootDirs = []string{tmpDir}

	tailer.Start()

	time.Sleep(1300 * time.Millisecond)

	tailer.Stop()

	runtime.GC()
	time.Sleep(100 * time.Millisecond)

	finalGoroutines := runtime.NumGoroutine()
	if finalGoroutines > baseGoroutines+2 {
		t.Errorf("LocalTailer leaked goroutines! Base: %d, Final: %d", baseGoroutines, finalGoroutines)
	}
}