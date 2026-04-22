package storage_test

import (
	"context"
	"testing"
	"time"

	"github.com/WangYihang/Platypus/internal/storage"
)

// TestActivities_RecordAndList covers the happy path: every inserted row
// comes back on a broad list, newest first.
func TestActivities_RecordAndList(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	pid := "proj-1"

	now := time.Now().UTC()
	rows := []*storage.Activity{
		{At: now.Add(-3 * time.Second), ProjectID: &pid, Category: storage.CategoryFile, Action: "file.read", ActorUser: "alice", Outcome: storage.OutcomeSuccess, TargetLabel: "/etc/passwd"},
		{At: now.Add(-2 * time.Second), ProjectID: &pid, Category: storage.CategoryCommand, Action: "command.exec", ActorUser: "alice", Outcome: storage.OutcomeSuccess, TargetLabel: "ls -la"},
		{At: now.Add(-1 * time.Second), ProjectID: &pid, Category: storage.CategoryTunnel, Action: "tunnel.create", ActorUser: "bob", Outcome: storage.OutcomeError, Error: "boom"},
	}
	for _, r := range rows {
		if err := db.Activities().Record(ctx, r); err != nil {
			t.Fatalf("Record: %v", err)
		}
	}

	got, cursor, err := db.Activities().List(ctx, storage.ActivityFilter{ProjectID: &pid})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if cursor != "" {
		t.Fatalf("cursor should be empty on single page, got %q", cursor)
	}
	if len(got) != len(rows) {
		t.Fatalf("rows = %d; want %d", len(got), len(rows))
	}
	// Newest-first: tunnel.create was last inserted, so it comes first.
	if got[0].Action != "tunnel.create" {
		t.Fatalf("first = %q; want tunnel.create", got[0].Action)
	}
}

// Keyset pagination should neither double-return rows nor drop any.
func TestActivities_Cursor_ConsistencyAcrossPages(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	pid := "proj-1"

	now := time.Now().UTC()
	const total = 25
	for i := 0; i < total; i++ {
		if err := db.Activities().Record(ctx, &storage.Activity{
			At:        now.Add(time.Duration(i) * time.Millisecond),
			ProjectID: &pid,
			Category:  storage.CategoryCommand,
			Action:    "command.exec",
			Outcome:   storage.OutcomeSuccess,
		}); err != nil {
			t.Fatalf("Record: %v", err)
		}
	}

	seen := map[int64]bool{}
	f := storage.ActivityFilter{ProjectID: &pid, Limit: 7}
	pages := 0
	for {
		page, cursor, err := db.Activities().List(ctx, f)
		if err != nil {
			t.Fatalf("List page %d: %v", pages, err)
		}
		for _, a := range page {
			if seen[a.ID] {
				t.Fatalf("duplicate id %d across pages", a.ID)
			}
			seen[a.ID] = true
		}
		pages++
		if cursor == "" {
			break
		}
		f.Cursor = cursor
		if pages > 10 {
			t.Fatalf("too many pages; bailing")
		}
	}
	if len(seen) != total {
		t.Fatalf("saw %d unique rows; want %d", len(seen), total)
	}
}

// Category + actor + outcome filters combine with AND semantics.
func TestActivities_Filter_Combinations(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	pid := "proj-1"
	pid2 := "proj-2"

	type row struct {
		project  string
		actor    string
		action   string
		category string
		outcome  string
	}
	inserts := []row{
		{pid, "alice", "command.exec", storage.CategoryCommand, storage.OutcomeSuccess},
		{pid, "alice", "file.read", storage.CategoryFile, storage.OutcomeSuccess},
		{pid, "bob", "command.exec", storage.CategoryCommand, storage.OutcomeError},
		{pid, "bob", "tunnel.create", storage.CategoryTunnel, storage.OutcomeDenied},
		{pid2, "alice", "command.exec", storage.CategoryCommand, storage.OutcomeSuccess},
	}
	for _, r := range inserts {
		proj := r.project
		if err := db.Activities().Record(ctx, &storage.Activity{
			ProjectID: &proj,
			Category:  r.category,
			Action:    r.action,
			ActorUser: r.actor,
			Outcome:   r.outcome,
		}); err != nil {
			t.Fatalf("Record: %v", err)
		}
	}

	// Project + actor filter.
	got, _, _ := db.Activities().List(ctx, storage.ActivityFilter{
		ProjectID: &pid,
		ActorUser: "alice",
	})
	if len(got) != 2 {
		t.Fatalf("alice in proj-1: got %d; want 2", len(got))
	}

	// Category filter.
	got, _, _ = db.Activities().List(ctx, storage.ActivityFilter{
		ProjectID:  &pid,
		Categories: []string{storage.CategoryCommand},
	})
	if len(got) != 2 {
		t.Fatalf("commands in proj-1: got %d; want 2", len(got))
	}

	// Outcome filter.
	got, _, _ = db.Activities().List(ctx, storage.ActivityFilter{
		ProjectID: &pid,
		Outcome:   storage.OutcomeError,
	})
	if len(got) != 1 || got[0].ActorUser != "bob" {
		t.Fatalf("errors in proj-1: got %+v", got)
	}

	// include_global=false does not leak proj-2 rows.
	got, _, _ = db.Activities().List(ctx, storage.ActivityFilter{ProjectID: &pid})
	if len(got) != 4 {
		t.Fatalf("project-1 rows: got %d; want 4", len(got))
	}

	// Global-only (ProjectID=&""): only rows with NULL project_id match.
	empty := ""
	got, _, _ = db.Activities().List(ctx, storage.ActivityFilter{ProjectID: &empty})
	if len(got) != 0 {
		t.Fatalf("global rows: got %d; want 0", len(got))
	}

	// IncludeGlobal=true merges NULL rows with project rows. Insert a
	// true global event and confirm it surfaces when requested, stays
	// hidden otherwise.
	if err := db.Activities().Record(ctx, &storage.Activity{
		ProjectID: nil,
		Category:  storage.CategoryAuth,
		Action:    "user.login",
		ActorUser: "alice",
	}); err != nil {
		t.Fatalf("Record global: %v", err)
	}
	got, _, _ = db.Activities().List(ctx, storage.ActivityFilter{ProjectID: &pid, IncludeGlobal: true})
	if len(got) != 5 {
		t.Fatalf("proj-1+global rows: got %d; want 5", len(got))
	}
	got, _, _ = db.Activities().List(ctx, storage.ActivityFilter{ProjectID: &pid})
	if len(got) != 4 {
		t.Fatalf("proj-1 alone (after adding a global): got %d; want 4", len(got))
	}
}

// Time range filter enforces [From, To] inclusively.
func TestActivities_TimeRange(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	pid := "proj-1"

	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		if err := db.Activities().Record(ctx, &storage.Activity{
			At:        base.Add(time.Duration(i) * time.Hour),
			ProjectID: &pid,
			Category:  storage.CategoryCommand,
			Action:    "command.exec",
			Outcome:   storage.OutcomeSuccess,
		}); err != nil {
			t.Fatalf("Record: %v", err)
		}
	}

	got, _, _ := db.Activities().List(ctx, storage.ActivityFilter{
		ProjectID: &pid,
		From:      base.Add(1 * time.Hour),
		To:        base.Add(3 * time.Hour),
	})
	if len(got) != 3 {
		t.Fatalf("time-bounded: got %d; want 3", len(got))
	}
}

// Outcome validation lives in the schema's CHECK constraint — inserting a
// bogus value must fail.
func TestActivities_RejectsInvalidOutcome(t *testing.T) {
	db := newTestDB(t)
	err := db.Activities().Record(context.Background(), &storage.Activity{
		Category: storage.CategoryCommand,
		Action:   "command.exec",
		Outcome:  "maybe",
	})
	if err == nil {
		t.Fatal("expected CHECK constraint error on outcome=maybe; got nil")
	}
}

// Count mirrors the List filter logic without pagination.
func TestActivities_Count(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	pid := "proj-1"

	for i := 0; i < 7; i++ {
		_ = db.Activities().Record(ctx, &storage.Activity{
			ProjectID: &pid,
			Category:  storage.CategoryFile,
			Action:    "file.read",
			Outcome:   storage.OutcomeSuccess,
		})
	}
	n, err := db.Activities().Count(ctx, storage.ActivityFilter{ProjectID: &pid})
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if n != 7 {
		t.Fatalf("Count = %d; want 7", n)
	}
}
