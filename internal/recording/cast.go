// Package recording captures interactive shell sessions to disk in
// asciinema v2 format ("cast files"). The format is a JSON header
// line followed by newline-delimited 3-tuple events:
//
//	{"version": 2, "width": 80, "height": 24, "timestamp": 1700000000, ...}
//	[0.012, "o", "ls\r\n"]
//	[0.034, "o", "file1\nfile2\n"]
//	[1.002, "r", "120 30"]
//
// Format reference: https://docs.asciinema.org/manual/asciicast/v2/
//
// The writer is goroutine-safe (one mutex protects the file handle and
// the cumulative counters). It buffers writes through the OS file —
// callers that want stronger durability should arrange their own
// fsync schedule.
package recording

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Header is the JSON document on the first line of a v2 cast file.
// Fields beyond the four required ones are optional; we only emit the
// useful ones. omitempty on optional fields keeps the line compact for
// recordings that don't set them.
type Header struct {
	Version   int               `json:"version"`
	Width     int               `json:"width"`
	Height    int               `json:"height"`
	Timestamp int64             `json:"timestamp"`
	Title     string            `json:"title,omitempty"`
	Command   string            `json:"command,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
}

// Writer streams events into a .cast file. New starts the header line;
// every subsequent WriteOutput / WriteResize call appends an event
// with a delta computed from the writer's birth time.
//
// The writer owns its file handle. Close finalises the file (flush +
// close) and is safe to call multiple times.
type Writer struct {
	path  string
	start time.Time
	mu    sync.Mutex
	f     *os.File
	// cumulative counters surfaced via Stats so the storage row
	// can be updated periodically without re-stat'ing the file.
	bytesWritten int64
	frameCount   int64
	closed       bool
}

// NewWriter creates a new cast file at path and writes the header
// line. The parent directory is created if missing (mode 0o700 — the
// file may contain command output that should not be world-readable).
// The returned writer is ready for WriteOutput / WriteResize calls.
func NewWriter(path string, h Header) (*Writer, error) {
	if h.Version == 0 {
		h.Version = 2
	}
	if h.Width <= 0 {
		h.Width = 80
	}
	if h.Height <= 0 {
		h.Height = 24
	}
	if h.Timestamp == 0 {
		h.Timestamp = time.Now().Unix()
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("recording: mkdir parent: %w", err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, fmt.Errorf("recording: open cast file: %w", err)
	}

	headerBytes, err := json.Marshal(h)
	if err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return nil, fmt.Errorf("recording: marshal header: %w", err)
	}
	headerBytes = append(headerBytes, '\n')
	n, err := f.Write(headerBytes)
	if err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return nil, fmt.Errorf("recording: write header: %w", err)
	}

	return &Writer{
		path:         path,
		start:        time.Unix(h.Timestamp, 0),
		f:            f,
		bytesWritten: int64(n),
	}, nil
}

// WriteOutput appends an output event ("o") with the bytes the agent
// produced. delta is computed from now - start so the player paces
// playback in real time. A zero-length payload is a no-op.
func (w *Writer) WriteOutput(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	return w.writeEvent(time.Now(), "o", string(data))
}

// WriteInput appends an input event ("i"). asciinema v2 supports it
// but most players ignore input events on playback; we still capture
// it so audit consumers can replay what the operator typed without
// guessing it from the echo.
func (w *Writer) WriteInput(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	return w.writeEvent(time.Now(), "i", string(data))
}

// WriteResize appends a resize event ("r") with the new pty
// dimensions. cols/rows must be positive; zero-or-negative values are
// silently dropped.
func (w *Writer) WriteResize(cols, rows uint32) error {
	if cols == 0 || rows == 0 {
		return nil
	}
	return w.writeEvent(time.Now(), "r", fmt.Sprintf("%d %d", cols, rows))
}

// writeEvent serialises one event tuple. asciinema uses a top-level
// JSON array, not an object — `[delta, kind, payload]`. We hand-craft
// the line because json.Marshal of `[]any{f, s, p}` produces the same
// shape but allocates two intermediate slices per event; for a chatty
// shell session that adds up.
func (w *Writer) writeEvent(at time.Time, kind, payload string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return errors.New("recording: writer closed")
	}

	delta := at.Sub(w.start).Seconds()
	if delta < 0 {
		delta = 0
	}

	// Encode the payload as a JSON string so embedded control chars,
	// quotes, and unicode survive the round-trip. The kind is a
	// single-letter ASCII tag, so we hand-build that piece.
	pj, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("recording: marshal payload: %w", err)
	}

	// 0.123456 keeps six fractional digits — matches the asciinema
	// reference recorder and is well below the 1ms playback grain.
	line := fmt.Sprintf("[%.6f, %q, %s]\n", delta, kind, pj)
	n, err := io.WriteString(w.f, line)
	if err != nil {
		return fmt.Errorf("recording: write event: %w", err)
	}
	w.bytesWritten += int64(n)
	w.frameCount++
	return nil
}

// Stats returns a snapshot of the cumulative counters: bytes written
// (header + every event line), frame count (events only), and elapsed
// duration since New.
func (w *Writer) Stats() (bytes int64, frames int64, duration time.Duration) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.bytesWritten, w.frameCount, time.Since(w.start)
}

// Path returns the absolute (or caller-relative) path the writer is
// targeting.
func (w *Writer) Path() string { return w.path }

// Close flushes the underlying file and releases the handle.
// Idempotent.
func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	w.closed = true
	if w.f == nil {
		return nil
	}
	err := w.f.Close()
	w.f = nil
	return err
}
