package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/WangYihang/Platypus/internal/activity"
	"github.com/WangYihang/Platypus/internal/optoken"
	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

// activitiesTestSetup stands up a minimal router that exercises both the
// project-scoped and global activity endpoints, plus a token issuer so we
// can mint Bearer tokens for the RBAC middleware.
func activitiesTestSetup(t *testing.T) (*gin.Engine, *storage.DB, string, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	cache := optoken.NewCache(64, 30*time.Second)
	verifier := NewTokenVerifier(db, cache)

	// Admin user + one project, so the project-scoped route resolves.
	ctx := context.Background()
	adminID := uuid.NewString()
	if err := db.Users().Create(ctx, &user.User{
		ID: adminID, Username: "admin", PasswordHash: "x", Role: user.RoleAdmin,
	}); err != nil {
		t.Fatalf("Users.Create: %v", err)
	}
	projectID := uuid.NewString()
	if err := db.Projects().Create(ctx, &storage.Project{
		ID: projectID, Name: "P1", Slug: "p1", CreatedBy: adminID,
	}); err != nil {
		t.Fatalf("Projects.Create: %v", err)
	}

	rbac := NewRBAC(db, verifier)
	h := NewActivitiesHandler(db)
	r := gin.New()
	RegisterV1ActivitiesRoutes(r, h, rbac)
	return r, db, adminID, projectID
}

// TestActivities_ListProject exercises the happy path of the project-scoped
// list endpoint: the handler surfaces seeded rows, newest first.
func TestActivities_ListProject(t *testing.T) {
	r, db, adminID, pid := activitiesTestSetup(t)
	ctx := context.Background()

	now := time.Now().UTC()
	for i, act := range []string{"command.exec", "file.read", "tunnel.create"} {
		if err := db.Activities().Record(ctx, &storage.Activity{
			At:        now.Add(time.Duration(i) * time.Second),
			ProjectID: &pid,
			ActorType: storage.ActorTypeUser,
			ActorUser: adminID,
			Category:  "test",
			Action:    act,
			Outcome:   storage.OutcomeSuccess,
		}); err != nil {
			t.Fatalf("Record: %v", err)
		}
	}

	token := mintBearerForUserID(t, db, adminID, user.RoleAdmin)

	w := probeReqWithPath(r, "GET", "/api/v1/projects/"+pid+"/activities", token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("List status=%d body=%s", w.Code, w.Body.String())
	}
	var resp listActivitiesResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Items) != 3 {
		t.Fatalf("items=%d; want 3", len(resp.Items))
	}
	if resp.Items[0].Action != "tunnel.create" {
		t.Fatalf("newest item action=%q; want tunnel.create", resp.Items[0].Action)
	}
}

// RBAC: a caller without a project membership and without global-admin is
// 403'd on the project-scoped endpoint.
func TestActivities_ListProject_DeniesNonMember(t *testing.T) {
	r, db, _, pid := activitiesTestSetup(t)
	ctx := context.Background()

	viewerID := uuid.NewString()
	if err := db.Users().Create(ctx, &user.User{
		ID: viewerID, Username: "viewer", PasswordHash: "x", Role: user.RoleViewer,
	}); err != nil {
		t.Fatalf("Users.Create: %v", err)
	}
	token := mintBearerForUserID(t, db, viewerID, user.RoleViewer)

	w := probeReqWithPath(r, "GET", "/api/v1/projects/"+pid+"/activities", token, nil)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d; want 403 body=%s", w.Code, w.Body.String())
	}
}

// TestActivities_Recorder_FillsContext confirms that RecordActivity
// auto-populates actor_ip / actor_ua / project id / actor_user from the
// gin context when the caller omits them.
func TestActivities_Recorder_FillsContext(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	activity.SetRecorder(activity.New(db))
	t.Cleanup(func() { activity.SetRecorder(nil) })

	r := gin.New()
	r.POST("/api/v1/projects/:pid/thing", func(c *gin.Context) {
		c.Set(claimsCtxKey, &AccessClaims{UserID: "user-1", Username: "alice", Role: user.RoleAdmin})
		RecordActivity(c, ActivityInput{
			Category: storage.CategoryCommand,
			Action:   "command.exec",
		})
		c.Status(http.StatusNoContent)
	})

	w := probeReqWithPath(r, "POST", "/api/v1/projects/proj-xyz/thing", "", nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status=%d", w.Code)
	}

	// Recorder writes async — poll briefly.
	deadline := time.Now().Add(2 * time.Second)
	var got []*storage.Activity
	for {
		got, _, err = db.Activities().List(context.Background(), storage.ActivityFilter{})
		if err != nil {
			t.Fatalf("Activities.List: %v", err)
		}
		if len(got) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for activity row")
		}
		time.Sleep(10 * time.Millisecond)
	}
	a := got[0]
	if a.ProjectID == nil || *a.ProjectID != "proj-xyz" {
		t.Fatalf("project_id = %v; want proj-xyz", a.ProjectID)
	}
	if a.ActorUser != "user-1" {
		t.Fatalf("actor_user = %q; want user-1", a.ActorUser)
	}
	if a.ActorIP == "" {
		t.Fatalf("actor_ip was not auto-filled")
	}
}

// TestActivities_ListProject_SourceFilter covers the source/actor_type
// segment that powers the activities page: ?source=human surfaces user
// + api_token rows; ?source=agent surfaces agent rows only; the
// explicit ?actor_type= override hits the raw column. Catches drift
// between expandSourceAlias and the storage filter.
func TestActivities_ListProject_SourceFilter(t *testing.T) {
	r, db, adminID, pid := activitiesTestSetup(t)
	ctx := context.Background()

	rows := []struct {
		actorType string
		actor     string
		action    string
	}{
		{storage.ActorTypeUser, adminID, "file.delete"},
		{storage.ActorTypeAPIToken, "tok-1", "command.exec"},
		{storage.ActorTypeAgent, "agent-1", "session.start"},
		{storage.ActorTypeSystem, "", "server.start"},
	}
	for i, r := range rows {
		if err := db.Activities().Record(ctx, &storage.Activity{
			At:        time.Now().UTC().Add(time.Duration(i) * time.Second),
			ProjectID: &pid,
			ActorType: r.actorType,
			ActorUser: r.actor,
			Category:  storage.CategoryCommand,
			Action:    r.action,
			Outcome:   storage.OutcomeSuccess,
		}); err != nil {
			t.Fatalf("Record: %v", err)
		}
	}

	token := mintBearerForUserID(t, db, adminID, user.RoleAdmin)

	cases := []struct {
		name  string
		query string
		want  int
	}{
		{"source=human", "?source=human", 2},
		{"source=agent", "?source=agent", 1},
		{"source=system", "?source=system", 1},
		{"actor_type=user", "?actor_type=user", 1},
		{"actor_type=user,agent", "?actor_type=user,agent", 2},
		{"unknown source falls through to all", "?source=mystery", 4},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := probeReqWithPath(r, "GET", "/api/v1/projects/"+pid+"/activities"+tc.query, token, nil)
			if w.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
			var resp listActivitiesResponse
			if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if len(resp.Items) != tc.want {
				t.Fatalf("items=%d; want %d (rows=%+v)", len(resp.Items), tc.want, resp.Items)
			}
		})
	}
}
