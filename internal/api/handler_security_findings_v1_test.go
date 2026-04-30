package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/optoken"
	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

func findingsTestSetup(t *testing.T) (*gin.Engine, *storage.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	cache := optoken.NewCache(64, 30*time.Second)
	verifier := NewTokenVerifier(db, cache)
	rbac := NewRBAC(db, verifier)
	r := gin.New()
	RegisterV1SecurityFindingsRoutes(r, NewSecurityFindingsHandler(db), rbac)
	return r, db
}

func TestSecurityFindings_FilterBySeverity(t *testing.T) {
	r, db := findingsTestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	proj := seedProjectForAPITest(t, db, "p", admin)
	host := seedHostForSecurityTest(t, db, proj, "alpha")
	saveScan(t, db, proj.ID, host.ID, 100,
		storage.SeverityCritical, storage.SeverityHigh, storage.SeverityHigh, storage.SeverityMedium)

	tok := mintBearerForUserID(t, db, admin.ID, user.RoleAdmin)
	w := probeReqWithPath(r, "GET",
		"/api/v1/projects/"+proj.ID+"/security-findings?severity=high", tok, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var resp findingsPageResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp.Total != 2 || len(resp.Findings) != 2 {
		t.Fatalf("severity=high → total=%d items=%d", resp.Total, len(resp.Findings))
	}
	for _, f := range resp.Findings {
		if f.Severity != "high" {
			t.Fatalf("non-high finding leaked: %+v", f)
		}
	}
}

func TestSecurityFindings_FilterByHostAndCategory(t *testing.T) {
	r, db := findingsTestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	proj := seedProjectForAPITest(t, db, "p", admin)
	hostA := seedHostForSecurityTest(t, db, proj, "a")
	hostB := seedHostForSecurityTest(t, db, proj, "b")
	saveScan(t, db, proj.ID, hostA.ID, 100, storage.SeverityHigh, storage.SeverityMedium)
	saveScan(t, db, proj.ID, hostB.ID, 200, storage.SeverityHigh)

	tok := mintBearerForUserID(t, db, admin.ID, user.RoleAdmin)
	w := probeReqWithPath(r, "GET",
		"/api/v1/projects/"+proj.ID+"/security-findings?host_id="+hostA.ID, tok, nil)
	var resp findingsPageResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp.Total != 2 {
		t.Fatalf("host filter: total=%d; want 2", resp.Total)
	}
	for _, f := range resp.Findings {
		if f.HostID != hostA.ID {
			t.Fatalf("foreign host leaked: %+v", f)
		}
	}
}

func TestSecurityFindings_Pagination(t *testing.T) {
	r, db := findingsTestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	proj := seedProjectForAPITest(t, db, "p", admin)
	host := seedHostForSecurityTest(t, db, proj, "alpha")
	saveScan(t, db, proj.ID, host.ID, 100,
		storage.SeverityCritical, storage.SeverityHigh, storage.SeverityHigh,
		storage.SeverityMedium, storage.SeverityLow)

	tok := mintBearerForUserID(t, db, admin.ID, user.RoleAdmin)
	w := probeReqWithPath(r, "GET",
		"/api/v1/projects/"+proj.ID+"/security-findings?page=2&page_size=2", tok, nil)
	var resp findingsPageResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp.Total != 5 {
		t.Fatalf("total=%d; want 5", resp.Total)
	}
	if resp.Page != 2 || resp.PageSize != 2 || len(resp.Findings) != 2 {
		t.Fatalf("page=%d size=%d items=%d; want page=2 size=2 items=2",
			resp.Page, resp.PageSize, len(resp.Findings))
	}
}

func TestSecurityFindings_ProjectIsolated(t *testing.T) {
	r, db := findingsTestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	prod := seedProjectForAPITest(t, db, "prod", admin)
	stag := seedProjectForAPITest(t, db, "stag", admin)
	hostP := seedHostForSecurityTest(t, db, prod, "p1")
	hostS := seedHostForSecurityTest(t, db, stag, "s1")
	saveScan(t, db, prod.ID, hostP.ID, 100, storage.SeverityCritical)
	saveScan(t, db, stag.ID, hostS.ID, 100, storage.SeverityHigh)

	tok := mintBearerForUserID(t, db, admin.ID, user.RoleAdmin)
	w := probeReqWithPath(r, "GET",
		"/api/v1/projects/"+prod.ID+"/security-findings", tok, nil)
	var resp findingsPageResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp.Total != 1 {
		t.Fatalf("project leak: total=%d", resp.Total)
	}
	if resp.Findings[0].Severity != "critical" {
		t.Fatalf("returned wrong project's finding: %+v", resp.Findings[0])
	}
}

func TestSecurityFindings_NonMemberForbidden(t *testing.T) {
	r, db := findingsTestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	stranger := seedUserForAPITest(t, db, "stranger", user.RoleViewer)
	proj := seedProjectForAPITest(t, db, "p", admin)

	tok := mintBearerForUserID(t, db, stranger.ID, user.RoleViewer)
	w := probeReqWithPath(r, "GET",
		"/api/v1/projects/"+proj.ID+"/security-findings", tok, nil)
	if w.Code != http.StatusForbidden {
		t.Fatalf("non-member: status=%d; want 403", w.Code)
	}
}
