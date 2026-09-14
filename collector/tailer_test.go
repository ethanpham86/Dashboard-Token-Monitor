package collector

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"tokenmonitor/config"
)

// canonicalMockWriter records flushed batches and events in a thread-safe manner
type canonicalMockWriter struct {
	mu     sync.Mutex
	events []*TokenUsageEvent
}

func (m *canonicalMockWriter) InsertUsageBatch(events []*TokenUsageEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range events {
		cp := *e
		m.events = append(m.events, &cp)
	}
	return nil
}

func (m *canonicalMockWriter) GetEvents() []*TokenUsageEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]*TokenUsageEvent, len(m.events))
	copy(result, m.events)
	return result
}

func (m *canonicalMockWriter) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.events)
}

// TestTailer_SeekDelta tests that LocalTailer tracks fileOffsets and reads only
// newly appended lines (delta) via Seek without re-processing previously read lines.
func TestTailer_SeekDelta(t *testing.T) {
	tmpDir := t.TempDir()
	convDir := filepath.Join(tmpDir, "conv_test_delta", ".system_generated", "logs")
	if err := os.MkdirAll(convDir, 0755); err != nil {
		t.Fatalf("Failed to create mock conversation logs directory: %v", err)
	}

	transcriptPath := filepath.Join(convDir, "transcript.jsonl")

	// 1. Write initial transcript containing 1 USER step and 1 MODEL step
	step1 := TranscriptStep{
		StepIndex: 0,
		Source:    "USER",
		Type:      "USER_PROMPT",
		CreatedAt: time.Now().Format(time.RFC3339),
		Content:   "Hello from user",
	}
	step2 := TranscriptStep{
		StepIndex: 1,
		Source:    "MODEL",
		Type:      "PLANNER_RESPONSE",
		CreatedAt: time.Now().Format(time.RFC3339),
		Content:   "Hello! I am Gemini 3.8 Flash.",
	}

	f, err := os.Create(transcriptPath)
	if err != nil {
		t.Fatalf("Failed to create transcript file: %v", err)
	}

	data1, _ := json.Marshal(step1)
	f.Write(append(data1, '\n'))
	data2, _ := json.Marshal(step2)
	f.Write(append(data2, '\n'))
	f.Close()

	fi, err := os.Stat(transcriptPath)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	initialSize := fi.Size()

	// 2. Setup Tailer with custom brain dir pointing to tmpDir
	mock := &canonicalMockWriter{}
	buf := NewAsyncBuffer(mock, 1000, 100, 10*time.Millisecond)
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
	// Point tailer roots explicitly to tmpDir
	tailer.rootDirs = []string{tmpDir}

	// Ingest existing content
	tailer.scanAndIngest()

	// Wait for buffer to flush
	time.Sleep(50 * time.Millisecond)

	if mock.Count() != 1 {
		t.Fatalf("Expected 1 event after initial scan, got %d", mock.Count())
	}

	tailer.mu.Lock()
	savedOffset := tailer.fileOffsets[transcriptPath]
	tailer.mu.Unlock()

	if savedOffset != initialSize {
		t.Fatalf("Expected fileOffset %d, got %d", initialSize, savedOffset)
	}

	// 3. Append new entries (delta): 1 USER step and 1 MODEL step with thinking tokens
	fAppend, err := os.OpenFile(transcriptPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("Failed to open transcript for appending: %v", err)
	}

	step3 := TranscriptStep{
		StepIndex: 2,
		Source:    "USER",
		Type:      "USER_PROMPT",
		CreatedAt: time.Now().Format(time.RFC3339),
		Content:   "Explain quantum entanglement.",
	}
	step4 := TranscriptStep{
		StepIndex: 3,
		Source:    "MODEL",
		Type:      "PLANNER_RESPONSE",
		CreatedAt: time.Now().Format(time.RFC3339),
		Content:   "Quantum entanglement is a phenomenon...",
		Thinking:  "Let's break down Bell states and non-locality thoroughly.",
	}

	data3, _ := json.Marshal(step3)
	fAppend.Write(append(data3, '\n'))
	data4, _ := json.Marshal(step4)
	fAppend.Write(append(data4, '\n'))
	fAppend.Close()

	fiAfter, _ := os.Stat(transcriptPath)
	newSize := fiAfter.Size()

	// 4. Ingest again - Tailer MUST seek from savedOffset and read ONLY the delta
	tailer.scanAndIngest()
	time.Sleep(50 * time.Millisecond)

	if mock.Count() != 2 {
		t.Fatalf("Expected 2 total events after delta ingest (1 initial + 1 delta), got %d", mock.Count())
	}

	tailer.mu.Lock()
	updatedOffset := tailer.fileOffsets[transcriptPath]
	tailer.mu.Unlock()

	if updatedOffset != newSize {
		t.Fatalf("Expected fileOffset updated to %d, got %d", newSize, updatedOffset)
	}

	events := mock.GetEvents()
	secondEvent := events[1]
	if secondEvent.ThinkingTokens <= 0 {
		t.Errorf("Expected delta event to have thinking tokens > 0, got %d", secondEvent.ThinkingTokens)
	}

	// 5. Ingest again without any file change - MUST NOT re-read or duplicate events
	tailer.scanAndIngest()
	time.Sleep(50 * time.Millisecond)

	if mock.Count() != 2 {
		t.Errorf("Expected event count to remain 2 when file is unchanged, got %d", mock.Count())
	}
}

// TestTailer_ModelExtraction verifies regex and fallback model extraction from user settings changes.
func TestTailer_ModelExtraction(t *testing.T) {
	testCases := []struct {
		name     string
		content  string
		expected string
	}{
		{
			name:     "Regex Claude Sonnet",
			content:  "<USER_SETTINGS_CHANGE>\nThe user changed setting `Model Selection` from None to Claude Sonnet 4.6 (Thinking).\n</USER_SETTINGS_CHANGE>",
			expected: "Claude Sonnet 4.6 (Thinking)",
		},
		{
			name:     "Regex Claude Opus",
			content:  "<USER_SETTINGS_CHANGE>\nThe user changed setting `Model Selection` from None to Claude Opus 4.6.\n</USER_SETTINGS_CHANGE>",
			expected: "Claude Opus 4.6 (Thinking)",
		},
		{
			name:     "Regex GPT-OSS 120B",
			content:  `<USER_SETTINGS_CHANGE> changed setting "Model Selection" from None to GPT-OSS 120B (Medium). </USER_SETTINGS_CHANGE>`,
			expected: "GPT-OSS 120B (Medium)",
		},
		{
			name:     "Regex Gemini 3.8 Flash High",
			content:  "<USER_SETTINGS_CHANGE>\nThe user changed setting `Model Selection` from None to Gemini 3.8 Flash (High).\n</USER_SETTINGS_CHANGE>",
			expected: "Gemini 3.8 Flash (High)",
		},
		{
			name:     "Regex Gemini 3.1 Pro High",
			content:  "<USER_SETTINGS_CHANGE>\nThe user changed setting `Model Selection` from None to Gemini 3.1 Pro High.\n</USER_SETTINGS_CHANGE>",
			expected: "Gemini 3.1 Pro (High)",
		},
		{
			name:     "Regex Gemini 3.7 Flash Medium",
			content:  "<USER_SETTINGS_CHANGE>\nThe user changed setting `Model Selection` from None to Gemini 3.7 Flash Medium.\n</USER_SETTINGS_CHANGE>",
			expected: "Gemini 3.7 Flash Medium",
		},
		{
			name:     "Fallback text gemini ultra",
			content:  "<USER_SETTINGS_CHANGE> Switching to gemini ultra model </USER_SETTINGS_CHANGE>",
			expected: "Gemini Ultra",
		},
		{
			name:     "Ignore None selection",
			content:  "<USER_SETTINGS_CHANGE>\nThe user changed setting `Model Selection` from Gemini 3.8 to None.\n</USER_SETTINGS_CHANGE>",
			expected: "",
		},
		{
			name:     "Normal chat message without tag",
			content:  "Let's discuss Gemini 3.8 Flash architecture.",
			expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := ExtractModelName(tc.content)
			if got != tc.expected {
				t.Errorf("ExtractModelName() = %q, expected %q", got, tc.expected)
			}
		})
	}
}

// TestTailer_BackfillAllHistory tests that BackfillAllHistory scans all conversations
// in brain directories and successfully ingests historical MODEL steps into storage buffer.
func TestTailer_BackfillAllHistory(t *testing.T) {
	tmpBrain := t.TempDir()

	// Create conversation 1 with 2 MODEL steps
	conv1Dir := filepath.Join(tmpBrain, "conv_alpha", ".system_generated", "logs")
	os.MkdirAll(conv1Dir, 0755)
	f1, _ := os.Create(filepath.Join(conv1Dir, "transcript.jsonl"))

	steps1 := []TranscriptStep{
		{StepIndex: 0, Source: "USER", Type: "USER_PROMPT", Content: "Prompt 1"},
		{StepIndex: 1, Source: "MODEL", Type: "PLANNER_RESPONSE", Content: "Response 1"},
		{StepIndex: 2, Source: "USER", Type: "USER_PROMPT", Content: "Prompt 2"},
		{StepIndex: 3, Source: "MODEL", Type: "PLANNER_RESPONSE", Content: "Response 2", Thinking: "Deep thought"},
	}
	for _, s := range steps1 {
		d, _ := json.Marshal(s)
		f1.Write(append(d, '\n'))
	}
	f1.Close()

	// Create conversation 2 with 3 MODEL steps
	conv2Dir := filepath.Join(tmpBrain, "conv_beta", ".system_generated", "logs")
	os.MkdirAll(conv2Dir, 0755)
	f2, _ := os.Create(filepath.Join(conv2Dir, "transcript.jsonl"))

	steps2 := []TranscriptStep{
		{StepIndex: 0, Source: "USER", Type: "USER_PROMPT", Content: "<USER_SETTINGS_CHANGE>\nThe user changed setting `Model Selection` from None to Claude Sonnet 4.6 (Thinking).\n</USER_SETTINGS_CHANGE>"},
		{StepIndex: 1, Source: "MODEL", Type: "PLANNER_RESPONSE", Content: "Response Beta 1"},
		{StepIndex: 2, Source: "MODEL", Type: "PLANNER_RESPONSE", Content: "Response Beta 2"},
		{StepIndex: 3, Source: "MODEL", Type: "PLANNER_RESPONSE", Content: "Response Beta 3"},
	}
	for _, s := range steps2 {
		d, _ := json.Marshal(s)
		f2.Write(append(d, '\n'))
	}
	f2.Close()

	mock := &canonicalMockWriter{}
	buf := NewAsyncBuffer(mock, 1000, 100, 100*time.Millisecond)

	cfg := &config.Config{
		LocalTailer: config.LocalTailerConfig{
			Enabled:     true,
			IDEBrainDir: tmpBrain,
		},
	}
	tailer := NewLocalTailer(cfg, buf)
	tailer.rootDirs = []string{tmpBrain}

	count, err := tailer.BackfillAllHistory()
	if err != nil {
		t.Fatalf("BackfillAllHistory failed: %v", err)
	}

	// Total MODEL steps = 2 (conv1) + 3 (conv2) = 5
	if count != 5 {
		t.Fatalf("Expected BackfillAllHistory to return count 5, got %d", count)
	}

	if mock.Count() != 5 {
		t.Fatalf("Expected mock writer to have received 5 flushed events, got %d", mock.Count())
	}

	// Verify model detection inside conversation 2 backfill
	events := mock.GetEvents()
	var sonnetCount int
	for _, e := range events {
		if e.ModelName == "Claude Sonnet 4.6 (Thinking)" {
			sonnetCount++
		}
	}
	if sonnetCount != 3 {
		t.Errorf("Expected 3 events with Claude Sonnet 4.6 model name from conv_beta, got %d", sonnetCount)
	}
}

// TestTailer_IgnoresToolGenericSteps ensures that steps with type "GENERIC" (tool results)
// are strictly ignored and NOT double-counted as LLM calls.
func TestTailer_IgnoresToolGenericSteps(t *testing.T) {
	tmpBrain := t.TempDir()
	convDir := filepath.Join(tmpBrain, "conv_gamma", ".system_generated", "logs")
	os.MkdirAll(convDir, 0755)
	f, _ := os.Create(filepath.Join(convDir, "transcript.jsonl"))

	steps := []TranscriptStep{
		{StepIndex: 0, Source: "USER", Type: "USER_PROMPT", Content: "Run command please"},
		{StepIndex: 1, Source: "MODEL", Type: "PLANNER_RESPONSE", Content: "Running command..."},
		// Tool execution result step: source is MODEL but type is GENERIC
		{StepIndex: 2, Source: "MODEL", Type: "GENERIC", Content: "Tool execution output: file content 123456"},
		// Tool execution step: type is RUN_COMMAND
		{StepIndex: 3, Source: "MODEL", Type: "RUN_COMMAND", Content: "exit code 0"},
		// Final model response to user
		{StepIndex: 4, Source: "MODEL", Type: "PLANNER_RESPONSE", Content: "Here is your completed result."},
	}
	for _, s := range steps {
		d, _ := json.Marshal(s)
		f.Write(append(d, '\n'))
	}
	f.Close()

	mock := &canonicalMockWriter{}
	buf := NewAsyncBuffer(mock, 1000, 100, 100*time.Millisecond)

	cfg := &config.Config{
		LocalTailer: config.LocalTailerConfig{
			Enabled:     true,
			IDEBrainDir: tmpBrain,
		},
	}
	tailer := NewLocalTailer(cfg, buf)
	tailer.rootDirs = []string{tmpBrain}

	count, err := tailer.BackfillAllHistory()
	if err != nil {
		t.Fatalf("BackfillAllHistory failed: %v", err)
	}

	// Only steps 1 and 4 are PLANNER_RESPONSE -> count must be 2, NOT 4!
	if count != 2 {
		t.Fatalf("Expected BackfillAllHistory to count exactly 2 PLANNER_RESPONSE steps, got %d", count)
	}
}

// TestTailer_EstimatePromptTokens verifies the bounded context window curve.
func TestTailer_EstimatePromptTokens(t *testing.T) {
	// Base context
	if val := EstimatePromptTokens(0); val != 16000 {
		t.Errorf("Expected 16000 for step 0, got %d", val)
	}
	// Step 10
	if val := EstimatePromptTokens(10); val != 28000 {
		t.Errorf("Expected 28000 for step 10, got %d", val)
	}
	// Step 30
	if val := EstimatePromptTokens(30); val != 52000 {
		t.Errorf("Expected 52000 for step 30, got %d", val)
	}
	// Step 70
	if val := EstimatePromptTokens(70); val != 68000 {
		t.Errorf("Expected 68000 for step 70, got %d", val)
	}
	// Step 800 (must be bounded by ceiling 85000, never unbounded millions)
	if val := EstimatePromptTokens(800); val != 85000 {
		t.Errorf("Expected 85000 for step 800, got %d", val)
	}
}

