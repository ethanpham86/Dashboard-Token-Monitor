package collector

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"os"
	"time"
)

// CodexScanStats describes the most recent successful scan, not lifetime totals.
type CodexScanStats struct {
	FilesReused      int     `json:"files_reused"`
	FilesIncremental int     `json:"files_incremental"`
	FilesFull        int     `json:"files_full"`
	BytesParsed      int64   `json:"bytes_parsed"`
	BytesVerified    int64   `json:"bytes_verified"`
	DurationMS       float64 `json:"duration_ms"`
}

// Only counters, IDs, activity metadata and hashes survive a scan. No raw log
// bytes are retained. Published parsed sessions are immutable to dashboard readers.
type codexStreamState struct {
	offset             int64
	fileInfo           os.FileInfo
	prefixHash         [sha256.Size]byte
	tailHash           [sha256.Size]byte
	previous           CodexTokenUsage
	seenResponses      map[string]bool
	responseActivities []codexActivitySample
	itemActivities     []codexActivitySample
	baseActivityCount  int
}

// A bounded prefix and cursor boundary detect common rewrite/rotation cases.
// Arbitrary edits in the middle of an append-only file require RefreshFull.
func codexBoundaryHashes(f *os.File, offset int64) ([sha256.Size]byte, [sha256.Size]byte, int64, error) {
	var zero [sha256.Size]byte
	n := min(offset, int64(4096))
	buf := make([]byte, int(n))
	if _, err := f.ReadAt(buf, 0); err != nil {
		return zero, zero, 0, err
	}
	prefix := sha256.Sum256(buf)
	if offset <= n {
		return prefix, prefix, n, nil
	}
	if _, err := f.ReadAt(buf, offset-n); err != nil {
		return zero, zero, n, err
	}
	return prefix, sha256.Sum256(buf), 2 * n, nil
}

func cloneCodexSession(cached *codexParsedSession) *codexParsedSession {
	p := *cached
	p.Usage = append([]codexUsageSample(nil), cached.Usage...)
	p.Activities = append([]codexActivitySample(nil), cached.Activities[:cached.stream.baseActivityCount]...)
	p.stream.seenResponses = make(map[string]bool, len(cached.stream.seenResponses))
	for id := range cached.stream.seenResponses {
		p.stream.seenResponses[id] = true
	}
	p.stream.responseActivities = append([]codexActivitySample(nil), cached.stream.responseActivities...)
	p.stream.itemActivities = append([]codexActivitySample(nil), cached.stream.itemActivities...)
	return &p
}

// Kept for parser callers/tests; the opened file determines the bounded snapshot.
func parseCodexSessionFile(path string, _ int64, _ time.Time) (*codexParsedSession, error) {
	p, _, err := readCodexSession(context.Background(), path, nil)
	return p, err
}

func readCodexSession(ctx context.Context, path string, cached *codexParsedSession) (*codexParsedSession, CodexScanStats, error) {
	stats := CodexScanStats{}
	if err := ctx.Err(); err != nil {
		return nil, stats, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, stats, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, stats, err
	}
	if !info.Mode().IsRegular() {
		return nil, stats, errors.New("Codex source is not a regular file")
	}
	var parsed *codexParsedSession
	if cached != nil && cached.stream.fileInfo != nil && info.Size() > cached.Size && os.SameFile(cached.stream.fileInfo, info) {
		prefix, tail, n, hashErr := codexBoundaryHashes(f, cached.stream.offset)
		stats.BytesVerified += n
		if hashErr == nil && prefix == cached.stream.prefixHash && tail == cached.stream.tailHash {
			parsed = cloneCodexSession(cached)
			stats.FilesIncremental = 1
		}
	}
	if parsed == nil {
		parsed = &codexParsedSession{Path: path, Model: "OpenAI (unknown)", stream: codexStreamState{seenResponses: make(map[string]bool)}}
		stats.FilesFull = 1
	}
	parsed.Size, parsed.ModTime = info.Size(), info.ModTime()
	parsed.stream.fileInfo = info
	startOffset := parsed.stream.offset
	if _, err := f.Seek(startOffset, io.SeekStart); err != nil {
		return nil, stats, err
	}
	scanner := bufio.NewScanner(io.LimitReader(f, info.Size()-startOffset))
	scanner.Buffer(make([]byte, 64*1024), codexScannerMaxToken)
	// Only commit complete lines. An incomplete final write will be retried from
	// the same offset on the next append. A valid final JSON record needs no LF.
	scanner.Split(func(data []byte, atEOF bool) (advance int, token []byte, splitErr error) {
		if atEOF && !bytes.Contains(data, []byte{'\n'}) && !json.Valid(bytes.TrimSpace(data)) {
			return 0, nil, nil
		}
		advance, token, splitErr = bufio.ScanLines(data, atEOF)
		parsed.stream.offset += int64(advance)
		return
	})
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return nil, stats, err
		}
		if len(bytes.TrimSpace(scanner.Bytes())) == 0 {
			continue
		}
		parseCodexLine(parsed, scanner.Bytes())
	}
	stats.BytesParsed = info.Size() - startOffset
	if err := scanner.Err(); err != nil {
		return nil, stats, err
	}
	endInfo, err := f.Stat()
	if err != nil {
		return nil, stats, err
	}
	if endInfo.Size() < info.Size() || (endInfo.Size() == info.Size() && !endInfo.ModTime().Equal(info.ModTime())) {
		return nil, stats, errors.New("Codex source changed during scan; retry required")
	}
	prefix, tail, n, err := codexBoundaryHashes(f, parsed.stream.offset)
	stats.BytesVerified += n
	if err != nil {
		return nil, stats, err
	}
	parsed.stream.prefixHash, parsed.stream.tailHash = prefix, tail
	parsed.stream.baseActivityCount = len(parsed.Activities)
	if len(parsed.stream.responseActivities) > 0 {
		parsed.Activities = append(parsed.Activities, parsed.stream.responseActivities...)
	} else {
		parsed.Activities = append(parsed.Activities, parsed.stream.itemActivities...)
	}
	if parsed.SessionID == "" {
		parsed.SessionID = sessionIDFromFilename(path)
	}
	if parsed.Workspace == "" {
		parsed.Workspace = "Không rõ workspace"
	}
	if parsed.UpdatedAt.IsZero() {
		parsed.UpdatedAt = info.ModTime()
	}
	return parsed, stats, nil
}
