package api

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

// fillFromContext is the heart of audit attribution. These tests pin
// the AAT-vs-human attribution rules so a future refactor can't
// silently reintroduce ActorUser=TokenID (which would corrupt every
// per-user dashboard / filter).

func freshGinCtx() *gin.Context {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/", nil)
	return c
}

func TestFillFromContext_HumanPrincipal(t *testing.T) {
	c := freshGinCtx()
	SetPrincipal(c, &Principal{
		Kind:   PrincipalUser,
		UserID: "u-alice",
		Role:   user.RoleOperator,
	})
	in := ActivityInput{}
	fillFromContext(c, &in)
	if in.ActorUser != "u-alice" {
		t.Errorf("ActorUser = %q, want u-alice", in.ActorUser)
	}
	if in.ActorType != storage.ActorTypeUser {
		t.Errorf("ActorType = %q, want %q", in.ActorType, storage.ActorTypeUser)
	}
	if in.ActorTokenID != "" {
		t.Errorf("ActorTokenID = %q on human request, want empty", in.ActorTokenID)
	}
}

func TestFillFromContext_AATPrincipal(t *testing.T) {
	c := freshGinCtx()
	SetPrincipal(c, &Principal{
		Kind:    PrincipalAATKind,
		TokenID: "aat_xyz",
		UserID:  "u-issuer",
		Role:    user.RoleViewer,
	})
	in := ActivityInput{}
	fillFromContext(c, &in)
	if in.ActorType != storage.ActorTypeAPIToken {
		t.Errorf("ActorType = %q, want %q", in.ActorType, storage.ActorTypeAPIToken)
	}
	if in.ActorTokenID != "aat_xyz" {
		t.Errorf("ActorTokenID = %q, want aat_xyz", in.ActorTokenID)
	}
	if in.ActorUser != "" {
		t.Errorf("ActorUser = %q on AAT request, want empty (issuer is not the actor)", in.ActorUser)
	}
}

func TestFillFromContext_NoPrincipal_Anonymous(t *testing.T) {
	c := freshGinCtx()
	in := ActivityInput{}
	fillFromContext(c, &in)
	if in.ActorType != storage.ActorTypeAnonymous {
		t.Errorf("ActorType = %q, want anonymous", in.ActorType)
	}
}

func TestFillFromContext_PreservesExplicitFields(t *testing.T) {
	c := freshGinCtx()
	SetPrincipal(c, &Principal{Kind: PrincipalUser, UserID: "u-alice"})
	in := ActivityInput{
		ActorUser: "explicit-override",
		ActorType: storage.ActorTypeAgent,
	}
	fillFromContext(c, &in)
	if in.ActorUser != "explicit-override" {
		t.Errorf("ActorUser = %q, want override preserved", in.ActorUser)
	}
	if in.ActorType != storage.ActorTypeAgent {
		t.Errorf("ActorType = %q, want override preserved", in.ActorType)
	}
}
