package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/optoken"
	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

// /api/v1/projects/:pid/transfers (per-project list)
// /api/v1/projects/:pid/hosts/:hid/transfers (per-host list)
// /api/v1/transfers (global list — admin only)
// /api/v1/projects/:pid/transfers/:id/cancel (cancel an in-flight transfer)

// transferRouteFixture seeds an admin user, project, and host so we
// can exercise the project-scoped transfers endpoints with a real
// DB and a real RBAC verifier.
type transferRouteFixture struct {
	DB        *storage.DB
	RBAC      *RBAC
	Token     string
	UserID    string
	ProjectID string
	HostID    string
	Cancels   *TransferCancelRegistry
}

func newTransferRouteFixture(t *testing.T, hostID string) *transferRouteFixture {
	t.Helper()
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	cache := optoken.NewCache(64, 30*time.Second)
	verifier := NewTokenVerifier(db, cache)
	admin := seedUserForAPITest(t, db, "tx-admin", user.RoleAdmin)
	proj := seedProjectForAPITest(t, db, "tx-prod", admin)
	if _, err := db.Hosts().Upsert(context.Background(), &storage.HostIdentity{
		ProjectID:   proj.ID,
		MachineID:   "m-" + hostID,
		Fingerprint: "fp-" + hostID,
		Hostname:    "host-" + hostID,
		OS:          "linux",
		SeenAt:      time.Now().UTC(),
		AgentID:     hostID,
	}); err != nil {
		t.Fatalf("seed host: %v", err)
	}
	tok := mintSessionForTest(t, db, admin)

	return &transferRouteFixture{
		DB:        db,
		RBAC:      NewRBAC(db, verifier),
		Token:     tok,
		UserID:    admin.ID,
		ProjectID: proj.ID,
		HostID:    hostID,
		Cancels:   NewTransferCancelRegistry(),
	}
}

func (f *transferRouteFixture) authedGet(t *testing.T, srv *httptest.Server, path string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, srv.URL+path, nil)
	req.Header.Set("Authorization", "Bearer "+f.Token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	return resp
}

func (f *transferRouteFixture) authedPost(t *testing.T, srv *httptest.Server, path string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, srv.URL+path, nil)
	req.Header.Set("Authorization", "Bearer "+f.Token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	return resp
}

func seedTransfer(t *testing.T, db *storage.DB, id, projectID, hostID, userID, status string) *storage.FileTransfer {
	t.Helper()
	ft := &storage.FileTransfer{
		ID:        id,
		ProjectID: projectID,
		HostID:    hostID,
		UserID:    userID,
		Direction: storage.TransferDirectionDownload,
		Kind:      storage.TransferKindArchive,
		Format:    "tar.gz",
		PathsJSON: `["/etc"]`,
		Status:    status,
		StartedAt: time.Now().UTC(),
	}
	if err := db.FileTransfers().Create(context.Background(), ft); err != nil {
		t.Fatalf("seed transfer: %v", err)
	}
	return ft
}

// Per-project list returns transfers scoped to that project.
func TestTransfers_ListProject(t *testing.T) {
	f := newTransferRouteFixture(t, "h-1")
	otherProj := seedProjectForAPITest(t, f.DB, "tx-other",
		seedUserForAPITest(t, f.DB, "other-admin", user.RoleAdmin))

	seedTransfer(t, f.DB, "ft-1", f.ProjectID, f.HostID, f.UserID, storage.TransferStatusRunning)
	seedTransfer(t, f.DB, "ft-2", f.ProjectID, f.HostID, f.UserID, storage.TransferStatusDone)
	seedTransfer(t, f.DB, "ft-3", otherProj.ID, "h-other", f.UserID, storage.TransferStatusDone)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterV1TransferRoutes(r, TransferRoutesDeps{
		DB: f.DB, RBAC: f.RBAC, Cancels: f.Cancels,
	})
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp := f.authedGet(t, srv, "/api/v1/projects/"+f.ProjectID+"/transfers")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d; body=%s", resp.StatusCode, b)
	}
	var got struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Items) != 2 {
		t.Errorf("len(items) = %d; want 2 (ft-1, ft-2)", len(got.Items))
	}
}

// Per-host list further filters to a single host.
func TestTransfers_ListPerHost(t *testing.T) {
	f := newTransferRouteFixture(t, "h-target")
	// Seed a second host in the same project.
	if _, err := f.DB.Hosts().Upsert(context.Background(), &storage.HostIdentity{
		ProjectID:   f.ProjectID,
		MachineID:   "m-other",
		Fingerprint: "fp-other",
		Hostname:    "host-other",
		OS:          "linux",
		SeenAt:      time.Now().UTC(),
		AgentID:     "h-other",
	}); err != nil {
		t.Fatalf("seed second host: %v", err)
	}
	seedTransfer(t, f.DB, "ft-1", f.ProjectID, "h-target", f.UserID, storage.TransferStatusDone)
	seedTransfer(t, f.DB, "ft-2", f.ProjectID, "h-other", f.UserID, storage.TransferStatusDone)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterV1TransferRoutes(r, TransferRoutesDeps{
		DB: f.DB, RBAC: f.RBAC, Cancels: f.Cancels,
	})
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp := f.authedGet(t, srv,
		"/api/v1/projects/"+f.ProjectID+"/hosts/h-target/transfers")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d; body=%s", resp.StatusCode, b)
	}
	var got struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Items) != 1 || got.Items[0]["id"] != "ft-1" {
		t.Fatalf("got %+v; want only ft-1", got.Items)
	}
}

// Cancel of a registered transfer flips Active() to 0.
func TestTransfers_Cancel(t *testing.T) {
	f := newTransferRouteFixture(t, "h-cancel")
	seedTransfer(t, f.DB, "ft-cancel", f.ProjectID, f.HostID, f.UserID, storage.TransferStatusRunning)

	canceled := make(chan struct{})
	cancelFn := func() { close(canceled) }
	f.Cancels.Register("ft-cancel", cancelFn)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterV1TransferRoutes(r, TransferRoutesDeps{
		DB: f.DB, RBAC: f.RBAC, Cancels: f.Cancels,
	})
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp := f.authedPost(t, srv,
		"/api/v1/projects/"+f.ProjectID+"/transfers/ft-cancel/cancel")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d; want 202; body=%s", resp.StatusCode, b)
	}
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("Cancel registry did not fire")
	}
}

// Cancel of an unknown transfer ID returns 404.
func TestTransfers_CancelUnknown(t *testing.T) {
	f := newTransferRouteFixture(t, "h-x")

	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterV1TransferRoutes(r, TransferRoutesDeps{
		DB: f.DB, RBAC: f.RBAC, Cancels: f.Cancels,
	})
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp := f.authedPost(t, srv,
		"/api/v1/projects/"+f.ProjectID+"/transfers/missing/cancel")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d; want 404; body=%s", resp.StatusCode, b)
	}
}

// List honors a limit query param.
func TestTransfers_ListLimit(t *testing.T) {
	f := newTransferRouteFixture(t, "h-l")
	for i := 0; i < 5; i++ {
		seedTransfer(t, f.DB, "ft-"+strconv.Itoa(i), f.ProjectID, f.HostID, f.UserID, storage.TransferStatusDone)
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterV1TransferRoutes(r, TransferRoutesDeps{
		DB: f.DB, RBAC: f.RBAC, Cancels: f.Cancels,
	})
	srv := httptest.NewServer(r)
	defer srv.Close()
	resp := f.authedGet(t, srv,
		"/api/v1/projects/"+f.ProjectID+"/transfers?limit=2")
	defer resp.Body.Close()
	var got struct {
		Items []map[string]any `json:"items"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&got)
	if len(got.Items) != 2 {
		t.Fatalf("len = %d; want 2", len(got.Items))
	}
}
