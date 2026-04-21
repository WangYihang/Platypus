package app

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	keyring "github.com/zalando/go-keyring"
)

const listListenersV1Fixture = `{
  "listeners": [
    {"hash":"h1","host":"0.0.0.0","port":13337,"public_ip":"1.2.3.4","interfaces":["eth0"],"termite_clients":{"c1":{}}},
    {"hash":"h2","host":"0.0.0.0","port":13338,"public_ip":"1.2.3.4","interfaces":["eth0"],"termite_clients":{"c2":{},"c3":{}}}
  ]
}`

func startListenersServer(t *testing.T) (*httptest.Server, *http.Request) {
	t.Helper()
	var lastPost *http.Request
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"token":"tok"}`))
	})
	mux.HandleFunc("/notify", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) })
	mux.HandleFunc("/api/v1/listeners", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			w.Write([]byte(listListenersV1Fixture))
		case "POST":
			lastPost = r
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"hash":"new"}`))
		}
	})
	mux.HandleFunc("/api/v1/listeners/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" {
			w.WriteHeader(http.StatusNoContent)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, lastPost
}

func freshConnectedApp(t *testing.T, baseURL string) *App {
	t.Helper()
	keyring.MockInit()
	a, _ := New(filepath.Join(t.TempDir(), "p.json"), "test-listener-"+t.Name())
	a.emitFn = func(string, any) {}
	a.AddProfile("p", baseURL, "s")
	if err := a.Connect("p"); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(a.Disconnect)
	return a
}

func TestApp_ListListeners(t *testing.T) {
	srv, _ := startListenersServer(t)
	a := freshConnectedApp(t, srv.URL)

	got, err := a.ListListeners()
	if err != nil {
		t.Fatalf("ListListeners: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d, want 2", len(got))
	}
	byHash := map[string]int{got[0].Hash: 0, got[1].Hash: 1}
	h1 := got[byHash["h1"]]
	h2 := got[byHash["h2"]]
	if h1.NumSessions != 1 {
		t.Errorf("h1 = %+v", h1)
	}
	if h2.NumSessions != 2 {
		t.Errorf("h2 = %+v", h2)
	}
}

func TestApp_CreateListener_PostsJSON(t *testing.T) {
	mux := http.NewServeMux()
	var bodyBytes []byte
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"token":"tok"}`))
	})
	mux.HandleFunc("/notify", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) })
	mux.HandleFunc("/api/v1/listeners", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.NotFound(w, r)
			return
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		bodyBytes, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"hash":"new"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	a := freshConnectedApp(t, srv.URL)
	if err := a.CreateListener("0.0.0.0", 4444); err != nil {
		t.Fatalf("CreateListener: %v", err)
	}
	var decoded struct {
		Host string `json:"host"`
		Port int    `json:"port"`
	}
	if err := json.Unmarshal(bodyBytes, &decoded); err != nil {
		t.Fatalf("decode body: %v — raw: %q", err, bodyBytes)
	}
	if decoded.Host != "0.0.0.0" || decoded.Port != 4444 {
		t.Errorf("body = %+v", decoded)
	}
}

func TestApp_DeleteListener(t *testing.T) {
	srv, _ := startListenersServer(t)
	a := freshConnectedApp(t, srv.URL)
	if err := a.DeleteListener("h1"); err != nil {
		t.Fatalf("DeleteListener: %v", err)
	}
}
