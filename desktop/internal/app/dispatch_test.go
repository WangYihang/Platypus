package app

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	keyring "github.com/zalando/go-keyring"
)

func newDispatchTestServer(t *testing.T, dispatchHandler http.HandlerFunc, patchHandler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/auth/token":
			w.Write([]byte(`{"token":"tok"}`))
		case r.URL.Path == "/notify":
			w.WriteHeader(404)
		case r.URL.Path == "/api/v1/sessions/dispatch" && r.Method == http.MethodPost:
			if dispatchHandler != nil {
				dispatchHandler(w, r)
			}
		case strings.HasPrefix(r.URL.Path, "/api/v1/sessions/") && r.Method == http.MethodPatch:
			if patchHandler != nil {
				patchHandler(w, r)
			}
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
}

func connectedApp(t *testing.T, serverURL string, name string) *App {
	t.Helper()
	keyring.MockInit()
	a, err := New(filepath.Join(t.TempDir(), "p.json"), "test-dispatch-"+name)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := a.AddProfile("p", serverURL, "secret"); err != nil {
		t.Fatalf("AddProfile: %v", err)
	}
	if err := a.Connect("p"); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	return a
}

func TestApp_SetGroupDispatch_SendsPatchWithBody(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	patch := func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Write([]byte(`{"status":true}`))
	}
	srv := newDispatchTestServer(t, nil, patch)
	defer srv.Close()

	a := connectedApp(t, srv.URL, t.Name())
	if err := a.SetGroupDispatch("deadbeef", true); err != nil {
		t.Fatalf("SetGroupDispatch: %v", err)
	}
	if gotPath != "/api/v1/sessions/deadbeef" {
		t.Errorf("path = %q", gotPath)
	}
	if v, ok := gotBody["group_dispatch"].(bool); !ok || !v {
		t.Errorf("body = %v, want {group_dispatch: true}", gotBody)
	}
}

func TestApp_SetGroupDispatch_FalseRoundTrip(t *testing.T) {
	var gotBody map[string]any
	patch := func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Write([]byte(`{"status":true}`))
	}
	srv := newDispatchTestServer(t, nil, patch)
	defer srv.Close()

	a := connectedApp(t, srv.URL, t.Name())
	if err := a.SetGroupDispatch("h", false); err != nil {
		t.Fatalf("SetGroupDispatch: %v", err)
	}
	if v, ok := gotBody["group_dispatch"].(bool); !ok || v {
		t.Errorf("body = %v, want {group_dispatch: false}", gotBody)
	}
}

func TestApp_SetGroupDispatch_NotConnected(t *testing.T) {
	keyring.MockInit()
	a, _ := New(filepath.Join(t.TempDir(), "p.json"), "test-dispatch-not-conn-"+t.Name())
	err := a.SetGroupDispatch("h", true)
	if err != ErrNotConnected {
		t.Errorf("err = %v, want ErrNotConnected", err)
	}
}

func TestApp_DispatchCommand_HappyPath(t *testing.T) {
	var gotBody map[string]any
	dispatch := func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Write([]byte(`{"status":true,"count":2,"results":[
			{"session_hash":"a","output":"uid=0\n"},
			{"session_hash":"b","error":"timeout"}
		]}`))
	}
	srv := newDispatchTestServer(t, dispatch, nil)
	defer srv.Close()

	a := connectedApp(t, srv.URL, t.Name())
	results, err := a.DispatchCommand("id", 5)
	if err != nil {
		t.Fatalf("DispatchCommand: %v", err)
	}
	if gotBody["command"] != "id" {
		t.Errorf("command = %v", gotBody["command"])
	}
	if gotBody["timeout"].(float64) != 5 {
		t.Errorf("timeout = %v", gotBody["timeout"])
	}
	if len(results) != 2 {
		t.Fatalf("len = %d, want 2", len(results))
	}
	if results[0].SessionHash != "a" || !strings.Contains(results[0].Output, "uid=0") {
		t.Errorf("result[0] = %+v", results[0])
	}
	if results[1].SessionHash != "b" || results[1].Error != "timeout" {
		t.Errorf("result[1] = %+v", results[1])
	}
}

func TestApp_DispatchCommand_NotConnected(t *testing.T) {
	keyring.MockInit()
	a, _ := New(filepath.Join(t.TempDir(), "p.json"), "test-dispatch-notconn-"+t.Name())
	_, err := a.DispatchCommand("id", 3)
	if err != ErrNotConnected {
		t.Errorf("err = %v, want ErrNotConnected", err)
	}
}

func TestApp_DispatchCommand_ServerError400(t *testing.T) {
	dispatch := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		w.Write([]byte(`{"error":"command is required"}`))
	}
	srv := newDispatchTestServer(t, dispatch, nil)
	defer srv.Close()

	a := connectedApp(t, srv.URL, t.Name())
	_, err := a.DispatchCommand("", 3)
	if err == nil {
		t.Fatal("expected error from 400")
	}
}

func TestApp_DispatchCommand_EmptyResults(t *testing.T) {
	dispatch := func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":true,"count":0,"results":null}`))
	}
	srv := newDispatchTestServer(t, dispatch, nil)
	defer srv.Close()

	a := connectedApp(t, srv.URL, t.Name())
	results, err := a.DispatchCommand("id", 3)
	if err != nil {
		t.Fatalf("DispatchCommand: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("len = %d, want 0", len(results))
	}
}
