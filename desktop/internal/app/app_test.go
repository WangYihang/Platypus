package app

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	keyring "github.com/zalando/go-keyring"

	"github.com/WangYihang/Platypus/desktop/internal/profile"
)

func init() {
	keyring.MockInit()
}

func newTestApp(t *testing.T) *App {
	t.Helper()
	keyring.MockInit()
	a, err := New(filepath.Join(t.TempDir(), "profiles.json"), "platypus-desktop-test-"+t.Name())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return a
}

func TestApp_ListProfiles_EmptyByDefault(t *testing.T) {
	a := newTestApp(t)
	got, err := a.ListProfiles()
	if err != nil {
		t.Fatalf("ListProfiles: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}

func TestApp_AddProfile_PersistsBothLayers(t *testing.T) {
	a := newTestApp(t)

	if err := a.AddProfile("local", "http://127.0.0.1:7331", "the-secret"); err != nil {
		t.Fatalf("AddProfile: %v", err)
	}
	got, err := a.ListProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "local" || got[0].URL != "http://127.0.0.1:7331" {
		t.Errorf("ListProfiles = %+v", got)
	}
	// Secret in keychain.
	sec, err := a.keychain.Load("local")
	if err != nil {
		t.Fatalf("keychain.Load: %v", err)
	}
	if sec != "the-secret" {
		t.Errorf("secret = %q", sec)
	}
}

func TestApp_AddProfile_DuplicateRejected(t *testing.T) {
	a := newTestApp(t)
	a.AddProfile("p", "http://x", "s")

	err := a.AddProfile("p", "http://y", "s2")
	if !errors.Is(err, profile.ErrAlreadyExists) {
		t.Errorf("err = %v, want ErrAlreadyExists", err)
	}
}

func TestApp_AddProfile_InvalidURLRejected(t *testing.T) {
	a := newTestApp(t)
	if err := a.AddProfile("p", "not-a-url", "s"); err == nil {
		t.Error("expected validation error")
	}
}

func TestApp_RemoveProfile_RemovesFromBothLayers(t *testing.T) {
	a := newTestApp(t)
	a.AddProfile("p", "http://x", "s")

	if err := a.RemoveProfile("p"); err != nil {
		t.Fatalf("RemoveProfile: %v", err)
	}
	if got, _ := a.ListProfiles(); len(got) != 0 {
		t.Errorf("after Remove, ListProfiles = %+v", got)
	}
	if _, err := a.keychain.Load("p"); err == nil {
		t.Error("secret still in keychain after RemoveProfile")
	}
}

func TestApp_RemoveProfile_Missing(t *testing.T) {
	a := newTestApp(t)
	err := a.RemoveProfile("nope")
	if !errors.Is(err, profile.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestApp_Connect_FetchesTokenAndMarksConnected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/notify" {
			w.WriteHeader(404) // connect spawns a notifier dial; irrelevant here
			return
		}
		if r.URL.Path != "/api/v1/auth/token" {
			t.Errorf("path = %q", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"secret":"the-secret"}` {
			t.Errorf("body = %q", body)
		}
		w.Write([]byte(`{"token":"abc-123"}`))
	}))
	defer srv.Close()

	a := newTestApp(t)
	a.AddProfile("local", srv.URL, "the-secret")

	if err := a.Connect("local"); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	st := a.ConnectionStatus()
	if !st.Connected {
		t.Error("Connected = false, want true")
	}
	if st.ProfileName != "local" {
		t.Errorf("ProfileName = %q", st.ProfileName)
	}
}

func TestApp_Connect_BadSecret(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte(`{"error":"invalid secret"}`))
	}))
	defer srv.Close()

	a := newTestApp(t)
	a.AddProfile("p", srv.URL, "wrong")

	err := a.Connect("p")
	if err == nil {
		t.Fatal("expected error on bad secret")
	}
	if a.ConnectionStatus().Connected {
		t.Error("Connected should be false on failed Connect")
	}
}

func TestApp_Connect_MissingProfile(t *testing.T) {
	a := newTestApp(t)
	err := a.Connect("never-added")
	if !errors.Is(err, profile.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestApp_Disconnect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"token":"x"}`))
	}))
	defer srv.Close()

	a := newTestApp(t)
	a.AddProfile("p", srv.URL, "s")
	a.Connect("p")

	a.Disconnect()
	if a.ConnectionStatus().Connected {
		t.Error("Connected = true after Disconnect")
	}
}

// Profiles survive across App restart (simulated by re-Newing on the same path).
func TestApp_ProfilesPersistAcrossRestart(t *testing.T) {
	keyring.MockInit()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "profiles.json")
	svc := "platypus-desktop-restart-test"

	a1, err := New(cfgPath, svc)
	if err != nil {
		t.Fatal(err)
	}
	a1.AddProfile("p", "http://example", "s")

	a2, err := New(cfgPath, svc)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := a2.ListProfiles()
	if len(got) != 1 || got[0].Name != "p" {
		t.Errorf("after restart, ListProfiles = %+v", got)
	}
	// Secret survives too.
	sec, err := a2.keychain.Load("p")
	if err != nil || sec != "s" {
		t.Errorf("secret after restart: %q, err=%v", sec, err)
	}
}
