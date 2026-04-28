package recording

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestWriter_HeaderAndEvents covers the cast file shape: a single
// JSON header line with the four required v2 fields, then one event
// line per WriteOutput / WriteResize call.
func TestWriter_HeaderAndEvents(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.cast")
	w, err := NewWriter(path, Header{
		Version:   2,
		Width:     120,
		Height:    30,
		Timestamp: time.Now().Unix(),
		Title:     "test session",
		Command:   "/bin/bash",
	})
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}

	if err := w.WriteOutput([]byte("hello")); err != nil {
		t.Fatalf("WriteOutput: %v", err)
	}
	if err := w.WriteInput([]byte("ls\n")); err != nil {
		t.Fatalf("WriteInput: %v", err)
	}
	if err := w.WriteResize(80, 24); err != nil {
		t.Fatalf("WriteResize: %v", err)
	}
	// Empty payload should be a no-op (no extra event line written).
	if err := w.WriteOutput(nil); err != nil {
		t.Fatalf("WriteOutput empty: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Re-open and inspect.
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	if !scanner.Scan() {
		t.Fatalf("expected header line")
	}
	var hdr map[string]any
	if err := json.Unmarshal(scanner.Bytes(), &hdr); err != nil {
		t.Fatalf("header not JSON: %v", err)
	}
	if int(hdr["version"].(float64)) != 2 {
		t.Errorf("version = %v, want 2", hdr["version"])
	}
	if int(hdr["width"].(float64)) != 120 {
		t.Errorf("width = %v, want 120", hdr["width"])
	}

	var events [][]any
	for scanner.Scan() {
		var ev []any
		if err := json.Unmarshal(scanner.Bytes(), &ev); err != nil {
			t.Fatalf("event not JSON tuple: %v", err)
		}
		events = append(events, ev)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan: %v", err)
	}

	if len(events) != 3 {
		t.Fatalf("got %d events, want 3 (output, input, resize); empty payload should be ignored", len(events))
	}

	wantKinds := []string{"o", "i", "r"}
	for i, ev := range events {
		if len(ev) != 3 {
			t.Errorf("event %d: len=%d, want 3", i, len(ev))
			continue
		}
		gotKind, _ := ev[1].(string)
		if gotKind != wantKinds[i] {
			t.Errorf("event %d: kind=%q, want %q", i, gotKind, wantKinds[i])
		}
	}

	// Resize payload must be `<cols>x<rows>` per asciicast v2 spec —
	// space-separated dimensions trip "invalid size value in resize
	// event" in the asciinema CLI.
	resizeData, _ := events[2][2].(string)
	if resizeData != "80x24" {
		t.Errorf("resize data = %q, want '80x24'", resizeData)
	}
}

func TestWriter_WriteAfterCloseFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.cast")
	w, err := NewWriter(path, Header{Version: 2, Width: 80, Height: 24, Timestamp: time.Now().Unix()})
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// Second close is a no-op.
	if err := w.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
	// Writes after close return an error.
	if err := w.WriteOutput([]byte("x")); err == nil {
		t.Errorf("WriteOutput after Close: want error, got nil")
	}
}
