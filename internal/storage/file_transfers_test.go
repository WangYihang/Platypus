package storage_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

// FileTransfersRepo persists rows describing file-transfer tasks
// (downloads + uploads) so the UI can list both in-flight and
// historical transfers with filtering by project and host. Status
// transitions: pending → running → done/failed/canceled.

func newRunningTransfer(projectID, hostID, userID string) *storage.FileTransfer {
	return &storage.FileTransfer{
		ID:               "ft-" + hostID + "-" + projectID,
		ProjectID:        projectID,
		HostID:           hostID,
		UserID:           userID,
		Direction:        storage.TransferDirectionDownload,
		Kind:             storage.TransferKindArchive,
		Format:           "tar.gz",
		PathsJSON:        `["/etc/hosts","/var/log"]`,
		Status:           storage.TransferStatusRunning,
		BytesTransferred: 0,
		TotalBytes:       1024,
		StartedAt:        time.Now().UTC().Truncate(time.Millisecond),
	}
}

func TestFileTransfers_CreateAndGet(t *testing.T) {
	db := newTestDB(t)
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "prod", "Production", admin)

	tr := newRunningTransfer(proj.ID, "host-1", admin.ID)
	ctx := context.Background()
	if err := db.FileTransfers().Create(ctx, tr); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := db.FileTransfers().Get(ctx, tr.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.HostID != "host-1" || got.Direction != storage.TransferDirectionDownload {
		t.Fatalf("got %+v", got)
	}
	if got.PathsJSON != tr.PathsJSON {
		t.Fatalf("PathsJSON = %q; want %q", got.PathsJSON, tr.PathsJSON)
	}
	if got.Status != storage.TransferStatusRunning {
		t.Fatalf("Status = %q; want running", got.Status)
	}
}

func TestFileTransfers_GetMissingReturnsNotFound(t *testing.T) {
	db := newTestDB(t)
	_, err := db.FileTransfers().Get(context.Background(), "no-such-id")
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("Get missing = %v; want ErrNotFound", err)
	}
}

func TestFileTransfers_UpdateProgress(t *testing.T) {
	db := newTestDB(t)
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "prod", "Production", admin)
	tr := newRunningTransfer(proj.ID, "host-1", admin.ID)
	ctx := context.Background()
	if err := db.FileTransfers().Create(ctx, tr); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := db.FileTransfers().UpdateProgress(ctx, tr.ID, 512, 320, 2048); err != nil {
		t.Fatalf("UpdateProgress: %v", err)
	}
	got, err := db.FileTransfers().Get(ctx, tr.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.BytesTransferred != 512 || got.WireBytes != 320 || got.TotalBytes != 2048 {
		t.Fatalf("progress = %d/%d (wire %d); want 512/2048 (wire 320)",
			got.BytesTransferred, got.TotalBytes, got.WireBytes)
	}
	if got.Status != storage.TransferStatusRunning {
		t.Fatalf("Status changed to %q; want still running", got.Status)
	}
}

func TestFileTransfers_Finish(t *testing.T) {
	db := newTestDB(t)
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "prod", "Production", admin)
	tr := newRunningTransfer(proj.ID, "host-1", admin.ID)
	ctx := context.Background()
	if err := db.FileTransfers().Create(ctx, tr); err != nil {
		t.Fatalf("Create: %v", err)
	}

	finishedAt := time.Now().UTC().Truncate(time.Millisecond)
	if err := db.FileTransfers().Finish(ctx, tr.ID,
		storage.TransferStatusDone, 4096, 1234, "", finishedAt); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	got, err := db.FileTransfers().Get(ctx, tr.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != storage.TransferStatusDone {
		t.Fatalf("Status = %q; want done", got.Status)
	}
	if got.BytesTransferred != 4096 {
		t.Fatalf("BytesTransferred = %d; want 4096", got.BytesTransferred)
	}
	if got.WireBytes != 1234 {
		t.Fatalf("WireBytes = %d; want 1234", got.WireBytes)
	}
	if got.FinishedAt == nil || !got.FinishedAt.Equal(finishedAt) {
		t.Fatalf("FinishedAt = %v; want %v", got.FinishedAt, finishedAt)
	}
}

func TestFileTransfers_FinishWithError(t *testing.T) {
	db := newTestDB(t)
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "prod", "Production", admin)
	tr := newRunningTransfer(proj.ID, "host-1", admin.ID)
	ctx := context.Background()
	if err := db.FileTransfers().Create(ctx, tr); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := db.FileTransfers().Finish(ctx, tr.ID,
		storage.TransferStatusFailed, 100, 100, "permission denied", time.Now().UTC()); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	got, err := db.FileTransfers().Get(ctx, tr.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != storage.TransferStatusFailed || got.ErrorMessage != "permission denied" {
		t.Fatalf("got status=%q err=%q", got.Status, got.ErrorMessage)
	}
}

// List with no filters returns rows newest-first.
func TestFileTransfers_ListOrderedByStartedAtDesc(t *testing.T) {
	db := newTestDB(t)
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "prod", "Production", admin)

	ctx := context.Background()
	t0 := time.Now().UTC().Truncate(time.Millisecond)
	for i, hid := range []string{"h-a", "h-b", "h-c"} {
		tr := newRunningTransfer(proj.ID, hid, admin.ID)
		tr.ID = "ft-" + hid
		tr.StartedAt = t0.Add(time.Duration(i) * time.Second)
		if err := db.FileTransfers().Create(ctx, tr); err != nil {
			t.Fatalf("Create %s: %v", hid, err)
		}
	}

	rows, err := db.FileTransfers().List(ctx, storage.FileTransferFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("len = %d; want 3", len(rows))
	}
	// Newest first: h-c, h-b, h-a.
	want := []string{"h-c", "h-b", "h-a"}
	for i, r := range rows {
		if r.HostID != want[i] {
			t.Fatalf("rows[%d].HostID = %q; want %q", i, r.HostID, want[i])
		}
	}
}

func TestFileTransfers_ListByProject(t *testing.T) {
	db := newTestDB(t)
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	prod := seedProject(t, db, "prod", "Prod", admin)
	stag := seedProject(t, db, "stag", "Stag", admin)

	ctx := context.Background()
	for i, p := range []*storage.Project{prod, prod, stag} {
		tr := newRunningTransfer(p.ID, "h", admin.ID)
		tr.ID = "ft-" + p.ID + "-" + itoaInt(i)
		if err := db.FileTransfers().Create(ctx, tr); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	rows, err := db.FileTransfers().List(ctx, storage.FileTransferFilter{
		ProjectID: prod.ID,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("List(project=prod) = %d; want 2", len(rows))
	}
	for _, r := range rows {
		if r.ProjectID != prod.ID {
			t.Errorf("got row from project %q", r.ProjectID)
		}
	}
}

func TestFileTransfers_ListByHost(t *testing.T) {
	db := newTestDB(t)
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "prod", "Prod", admin)

	ctx := context.Background()
	for i, hid := range []string{"a", "a", "b"} {
		tr := newRunningTransfer(proj.ID, hid, admin.ID)
		tr.ID = "ft-" + hid + "-" + itoaInt(i)
		if err := db.FileTransfers().Create(ctx, tr); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	rows, err := db.FileTransfers().List(ctx, storage.FileTransferFilter{
		HostID: "a",
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("List(host=a) = %d; want 2", len(rows))
	}
}

// Limit clamps the result set size.
func TestFileTransfers_ListLimit(t *testing.T) {
	db := newTestDB(t)
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "prod", "Prod", admin)

	ctx := context.Background()
	for i := 0; i < 5; i++ {
		tr := newRunningTransfer(proj.ID, "h", admin.ID)
		tr.ID = "ft-" + itoaInt(i)
		if err := db.FileTransfers().Create(ctx, tr); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}
	rows, err := db.FileTransfers().List(ctx, storage.FileTransferFilter{Limit: 2})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("len = %d; want 2", len(rows))
	}
}

// CountActive returns the number of running transfers (used by the
// "active count" badge in the UI).
func TestFileTransfers_CountActive(t *testing.T) {
	db := newTestDB(t)
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "prod", "Prod", admin)

	ctx := context.Background()
	tr1 := newRunningTransfer(proj.ID, "h", admin.ID)
	tr1.ID = "ft-r1"
	if err := db.FileTransfers().Create(ctx, tr1); err != nil {
		t.Fatalf("Create r1: %v", err)
	}
	tr2 := newRunningTransfer(proj.ID, "h", admin.ID)
	tr2.ID = "ft-d1"
	if err := db.FileTransfers().Create(ctx, tr2); err != nil {
		t.Fatalf("Create d1: %v", err)
	}
	if err := db.FileTransfers().Finish(ctx, tr2.ID,
		storage.TransferStatusDone, 0, 0, "", time.Now().UTC()); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	n, err := db.FileTransfers().CountActive(ctx, storage.FileTransferFilter{})
	if err != nil {
		t.Fatalf("CountActive: %v", err)
	}
	if n != 1 {
		t.Fatalf("CountActive = %d; want 1", n)
	}
}

// Cancellation is a transition: a running transfer becomes
// canceled. After cancellation the row keeps its final state so
// the UI can still see the row in the history.
func TestFileTransfers_Cancel(t *testing.T) {
	db := newTestDB(t)
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "prod", "Prod", admin)
	tr := newRunningTransfer(proj.ID, "h", admin.ID)
	ctx := context.Background()
	if err := db.FileTransfers().Create(ctx, tr); err != nil {
		t.Fatalf("Create: %v", err)
	}

	at := time.Now().UTC().Truncate(time.Millisecond)
	if err := db.FileTransfers().Finish(ctx, tr.ID,
		storage.TransferStatusCanceled, 0, 0, "", at); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	got, err := db.FileTransfers().Get(ctx, tr.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != storage.TransferStatusCanceled {
		t.Fatalf("Status = %q; want canceled", got.Status)
	}
}

func itoaInt(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
