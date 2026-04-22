package storage

import (
	"context"
	"testing"
	"time"
)

// newMeshStatsDB opens an in-memory DB with all migrations applied
// and returns a MeshStatsRepo for it.
func newMeshStatsDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// TestMeshStatsInsertAndQuery writes five samples and reads them
// back in the expected window. Also exercises canonical pair
// ordering (query is case-insensitive to the a,b argument order).
func TestMeshStatsInsertAndQuery(t *testing.T) {
	db := newMeshStatsDB(t)
	ctx := context.Background()
	start := time.Date(2026, 4, 22, 10, 0, 0, 0, time.UTC)

	var rows []MeshLinkStat
	for i := 0; i < 5; i++ {
		rows = append(rows, MeshLinkStat{
			At:        start.Add(time.Duration(i) * time.Second),
			ProjectID: "proj1",
			NodeA:     "zzz", NodeB: "aaa", // reversed to test canonical
			BytesIn: int64(i * 100), BytesOut: int64(i * 200),
			MsgsIn: int64(i), MsgsOut: int64(i * 2),
		})
	}
	if err := db.MeshStats().InsertLinkStats(ctx, rows); err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Query with arguments in their "natural" order; canonical
	// pair logic must still match.
	got, err := db.MeshStats().ListLinkHistory(ctx, "proj1", "aaa", "zzz",
		LinkHistoryOpts{Since: start, Until: start.Add(10 * time.Second)})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 5 {
		t.Fatalf("want 5, got %d", len(got))
	}
	if got[0].NodeA != "aaa" || got[0].NodeB != "zzz" {
		t.Fatalf("canonical pair mismatch: %+v", got[0])
	}
	if got[4].BytesIn != 400 {
		t.Fatalf("last sample BytesIn = %d, want 400", got[4].BytesIn)
	}
}

// TestMeshStatsDownsample ensures MaxPoints thins to first/last.
func TestMeshStatsDownsample(t *testing.T) {
	db := newMeshStatsDB(t)
	ctx := context.Background()
	start := time.Date(2026, 4, 22, 10, 0, 0, 0, time.UTC)

	var rows []MeshLinkStat
	for i := 0; i < 100; i++ {
		rows = append(rows, MeshLinkStat{
			At:        start.Add(time.Duration(i) * time.Second),
			ProjectID: "p", NodeA: "a", NodeB: "b",
			BytesIn: int64(i),
		})
	}
	if err := db.MeshStats().InsertLinkStats(ctx, rows); err != nil {
		t.Fatalf("insert: %v", err)
	}

	got, err := db.MeshStats().ListLinkHistory(ctx, "p", "a", "b",
		LinkHistoryOpts{Since: start, Until: start.Add(200 * time.Second), MaxPoints: 10})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 10 {
		t.Fatalf("thinning: want 10, got %d", len(got))
	}
	if got[0].BytesIn != 0 {
		t.Fatalf("first point should be 0, got %d", got[0].BytesIn)
	}
	if got[9].BytesIn != 99 {
		t.Fatalf("last point should be 99, got %d", got[9].BytesIn)
	}
}

// TestMeshStatsGC deletes rows older than cutoff across both tables.
func TestMeshStatsGC(t *testing.T) {
	db := newMeshStatsDB(t)
	ctx := context.Background()
	start := time.Date(2026, 4, 22, 10, 0, 0, 0, time.UTC)

	// 3 link rows: two old, one fresh.
	if err := db.MeshStats().InsertLinkStats(ctx, []MeshLinkStat{
		{At: start, ProjectID: "p", NodeA: "a", NodeB: "b", BytesIn: 1},
		{At: start.Add(time.Second), ProjectID: "p", NodeA: "a", NodeB: "b", BytesIn: 2},
		{At: start.Add(time.Hour), ProjectID: "p", NodeA: "a", NodeB: "b", BytesIn: 3},
	}); err != nil {
		t.Fatalf("insert link: %v", err)
	}
	cpu := 40.0
	if err := db.MeshStats().InsertMachineStats(ctx, []MachineStat{
		{At: start, HostID: "h1", ProjectID: "p", CPUPercent: &cpu},
	}); err != nil {
		t.Fatalf("insert machine: %v", err)
	}

	n, err := db.MeshStats().GCOlderThan(ctx, start.Add(30*time.Minute))
	if err != nil {
		t.Fatalf("gc: %v", err)
	}
	if n != 3 { // 2 link + 1 machine
		t.Fatalf("gc removed %d rows, want 3", n)
	}
	got, _ := db.MeshStats().ListLinkHistory(ctx, "p", "a", "b",
		LinkHistoryOpts{Since: start.Add(-time.Hour), Until: start.Add(2 * time.Hour)})
	if len(got) != 1 || got[0].BytesIn != 3 {
		t.Fatalf("remaining rows wrong: %+v", got)
	}
}
