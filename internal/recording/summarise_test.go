package recording

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/WangYihang/Platypus/internal/llm"
	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

// fakeLLM stands in for the chat-completions endpoint. Returns a
// fixed summary and records the prompt it was handed so a test can
// assert on what actually left the process.
func fakeLLM(t *testing.T, reply string, seen *string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if seen != nil && len(body.Messages) > 0 {
			*seen = body.Messages[len(body.Messages)-1].Content
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"` + reply + `"}}]}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// seedFinishedRecording writes a minimal cast file and the matching DB
// rows, returning the manager and the recording id.
func seedFinishedRecording(t *testing.T, aiEnabled bool, srvURL string) (*Manager, string, *storage.DB) {
	t.Helper()
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	if err := db.Users().Create(ctx, &user.User{
		ID: "u1", Username: "admin", PasswordHash: "hash", Role: user.RoleAdmin,
	}); err != nil {
		t.Fatalf("create user: %v", err)
	}
	proj := &storage.Project{ID: "p1", Name: "P", Slug: "p", CreatedBy: "u1"}
	if err := db.Projects().Create(ctx, proj); err != nil {
		t.Fatalf("create project: %v", err)
	}
	if aiEnabled {
		if err := db.Projects().SetAISummariesEnabled(ctx, "p1", true); err != nil {
			t.Fatalf("enable ai: %v", err)
		}
	}

	dir := t.TempDir()
	castPath := filepath.Join(dir, "rec1.cast")
	cast := `{"version":2,"width":80,"height":24}
[0.1,"o","whoami\r\n"]
[0.2,"o","root\r\n"]
[0.3,"o","export TOKEN=supersecretvalue123456\r\n"]
`
	if err := os.WriteFile(castPath, []byte(cast), 0o600); err != nil {
		t.Fatalf("write cast: %v", err)
	}

	rec := &storage.TerminalRecording{
		ID: "rec1", ProjectID: "p1", HostID: "h1", AgentID: "a1", UserID: "u1",
		Cols: 80, Rows: 24, Shell: "bash", FilePath: "rec1.cast",
		Status: storage.RecordingStatusCompleted, StartedAt: time.Now().UTC(),
	}
	if err := db.TerminalRecordings().Create(ctx, rec); err != nil {
		t.Fatalf("create recording: %v", err)
	}

	m := New(db, dir, true)
	m.llm = llm.New(srvURL, "test-key", "test-model")
	return m, "rec1", db
}

// TestSummariseOne_StoresSummary is the end-to-end path the original
// WIP branch never had: cast on disk → extract → redact → model →
// summary column.
func TestSummariseOne_StoresSummary(t *testing.T) {
	var prompt string
	srv := fakeLLM(t, "Checked identity and set an env var.", &prompt)
	m, id, db := seedFinishedRecording(t, true, srv.URL)

	if err := m.summariseOne(context.Background(), id); err != nil {
		t.Fatalf("summariseOne: %v", err)
	}

	rec, err := db.TerminalRecordings().Get(context.Background(), id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if rec.Summary != "Checked identity and set an env var." {
		t.Errorf("summary = %q, want the model's reply", rec.Summary)
	}

	// The whole point of redact.go: the token must not have left.
	if prompt == "" {
		t.Fatal("fake LLM saw no prompt")
	}
	if strings.Contains(prompt, "supersecretvalue123456") {
		t.Errorf("secret reached the model; prompt was:\n%s", prompt)
	}
	if !strings.Contains(prompt, "whoami") {
		t.Errorf("prompt lost the actual terminal text:\n%s", prompt)
	}
}

// TestSummariseOne_RespectsOptOut is the safety property: a project
// that never opted in must not have its cast file leave the process.
func TestSummariseOne_RespectsOptOut(t *testing.T) {
	var prompt string
	srv := fakeLLM(t, "should not happen", &prompt)
	m, id, db := seedFinishedRecording(t, false, srv.URL)

	if err := m.summariseOne(context.Background(), id); err != nil {
		t.Fatalf("summariseOne: %v", err)
	}
	if prompt != "" {
		t.Errorf("opted-out project still called the LLM with:\n%s", prompt)
	}
	rec, _ := db.TerminalRecordings().Get(context.Background(), id)
	if rec.Summary != "" {
		t.Errorf("summary = %q, want empty for an opted-out project", rec.Summary)
	}
}

// TestSummarise_WaitDrains covers the shutdown contract: Wait must not
// return until the async summariser has stored its result.
func TestSummarise_WaitDrains(t *testing.T) {
	srv := fakeLLM(t, "done", nil)
	m, id, db := seedFinishedRecording(t, true, srv.URL)

	m.summarise(id)
	m.Wait()

	rec, err := db.TerminalRecordings().Get(context.Background(), id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if rec.Summary != "done" {
		t.Errorf("summary = %q after Wait; the goroutine had not finished", rec.Summary)
	}
}

// TestSummarise_NoClientIsNoop guards the default deployment: with no
// API key configured nothing is spawned, so Wait returns immediately
// and no row is touched.
func TestSummarise_NoClientIsNoop(t *testing.T) {
	m, id, db := seedFinishedRecording(t, true, "")
	m.llm = llm.New("", "", "") // no API key → Available() == false

	m.summarise(id)
	m.Wait()

	rec, _ := db.TerminalRecordings().Get(context.Background(), id)
	if rec.Summary != "" {
		t.Errorf("summary = %q, want empty when no LLM is configured", rec.Summary)
	}
}

// TestFinish_DropsEmptyRecording covers the junk-row case: a terminal
// opened and closed without producing output leaves a cast holding only
// its header, which has nothing to play back and renders as an error
// tile in the Recordings list.
func TestFinish_DropsEmptyRecording(t *testing.T) {
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	if err := db.Users().Create(ctx, &user.User{
		ID: "u1", Username: "admin", PasswordHash: "hash", Role: user.RoleAdmin,
	}); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := db.Projects().Create(ctx, &storage.Project{
		ID: "p1", Name: "P", Slug: "p", CreatedBy: "u1",
	}); err != nil {
		t.Fatalf("create project: %v", err)
	}

	m := New(db, t.TempDir(), true)
	sess, err := m.Begin(ctx, BeginInput{
		ProjectID: "p1", HostID: "h1", AgentID: "a1", UserID: "u1",
		Cols: 80, Rows: 24, Shell: "bash",
	})
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	id := sess.ID()
	castPath := sess.AbsolutePath()

	// No WriteOutput calls at all — zero frames.
	sess.Finish(ctx, "")

	if _, err := db.TerminalRecordings().Get(ctx, id); !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("row still present after an empty session; got err=%v", err)
	}
	if _, err := os.Stat(castPath); !os.IsNotExist(err) {
		t.Errorf("cast file still on disk at %s", castPath)
	}
}

// TestFinish_KeepsFailedEmptyRecording is the other half: a session
// that failed before capturing anything is evidence, not junk.
func TestFinish_KeepsFailedEmptyRecording(t *testing.T) {
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	if err := db.Users().Create(ctx, &user.User{
		ID: "u1", Username: "admin", PasswordHash: "hash", Role: user.RoleAdmin,
	}); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := db.Projects().Create(ctx, &storage.Project{
		ID: "p1", Name: "P", Slug: "p", CreatedBy: "u1",
	}); err != nil {
		t.Fatalf("create project: %v", err)
	}

	m := New(db, t.TempDir(), true)
	sess, err := m.Begin(ctx, BeginInput{
		ProjectID: "p1", HostID: "h1", AgentID: "a1", UserID: "u1",
		Cols: 80, Rows: 24, Shell: "bash",
	})
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	id := sess.ID()

	sess.Finish(ctx, "agent link dropped")

	rec, err := db.TerminalRecordings().Get(ctx, id)
	if err != nil {
		t.Fatalf("failed session was dropped: %v", err)
	}
	if rec.Status != storage.RecordingStatusFailed {
		t.Errorf("status = %q, want failed", rec.Status)
	}
}
