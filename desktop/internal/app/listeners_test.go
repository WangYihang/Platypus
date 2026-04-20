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
    {"hash":"h1","host":"0.0.0.0","port":13337,"encrypted":false,"public_ip":"1.2.3.4","interfaces":["eth0"],"clients":{"c1":{}},"termite_clients":{}},
    {"hash":"h2","host":"0.0.0.0","port":13338,"encrypted":true,"public_ip":"1.2.3.4","interfaces":["eth0"],"clients":{},"termite_clients":{"c2":{},"c3":{}}}
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
	if h1.Encrypted || h1.NumSessions != 1 {
		t.Errorf("h1 = %+v", h1)
	}
	if !h2.Encrypted || h2.NumSessions != 2 {
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
	if err := a.CreateListener("0.0.0.0", 4444, true); err != nil {
		t.Fatalf("CreateListener: %v", err)
	}
	var decoded struct {
		Host      string `json:"host"`
		Port      int    `json:"port"`
		Encrypted bool   `json:"encrypted"`
	}
	if err := json.Unmarshal(bodyBytes, &decoded); err != nil {
		t.Fatalf("decode body: %v — raw: %q", err, bodyBytes)
	}
	if decoded.Host != "0.0.0.0" || decoded.Port != 4444 || !decoded.Encrypted {
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

// raasTestServer stubs the two v1 RaaS endpoints with controllable responses.
func raasTestServer(t *testing.T) (*httptest.Server, *http.Request) {
	t.Helper()
	var lastOnelinerReq *http.Request
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"token":"tok"}`))
	})
	mux.HandleFunc("/notify", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) })
	mux.HandleFunc("/api/v1/raas/languages", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":true,"languages":["bash","perl","python","ruby"]}`))
	})
	mux.HandleFunc("/api/v1/raas/oneliner", func(w http.ResponseWriter, r *http.Request) {
		lastOnelinerReq = r
		lang := r.URL.Query().Get("lang")
		if lang == "" {
			lang = "bash"
		}
		w.Write([]byte(`{"status":true,"oneliner":"echo ran-lang-` + lang + `"}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, lastOnelinerReq
}

func TestApp_GenerateRaasOneliner_SendsHostPortLang(t *testing.T) {
	var captured *http.Request
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"token":"tok"}`))
	})
	mux.HandleFunc("/notify", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) })
	mux.HandleFunc("/api/v1/raas/oneliner", func(w http.ResponseWriter, r *http.Request) {
		captured = r
		w.Write([]byte(`{"status":true,"oneliner":"the-oneliner"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	a := freshConnectedApp(t, srv.URL)
	got, err := a.GenerateRaasOneliner("10.0.0.1:1337", "python")
	if err != nil {
		t.Fatalf("GenerateRaasOneliner: %v", err)
	}
	if got != "the-oneliner" {
		t.Errorf("returned = %q", got)
	}
	if captured.URL.Query().Get("host") != "10.0.0.1" {
		t.Errorf("host = %q", captured.URL.Query().Get("host"))
	}
	if captured.URL.Query().Get("port") != "1337" {
		t.Errorf("port = %q", captured.URL.Query().Get("port"))
	}
	if captured.URL.Query().Get("lang") != "python" {
		t.Errorf("lang = %q", captured.URL.Query().Get("lang"))
	}
}

func TestApp_GenerateRaasOneliner_NotConnected(t *testing.T) {
	keyring.MockInit()
	a, _ := New(filepath.Join(t.TempDir(), "p.json"), "test-raas-notconn-"+t.Name())
	if _, err := a.GenerateRaasOneliner("1.2.3.4:1337", "bash"); err != ErrNotConnected {
		t.Errorf("err = %v, want ErrNotConnected", err)
	}
}

func TestApp_AvailableRaasLanguages_Remote(t *testing.T) {
	srv, _ := raasTestServer(t)
	a := freshConnectedApp(t, srv.URL)
	got, err := a.AvailableRaasLanguages()
	if err != nil {
		t.Fatalf("AvailableRaasLanguages: %v", err)
	}
	want := []string{"bash", "perl", "python", "ruby"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d (got %v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestApp_AvailableRaasLanguages_NotConnected(t *testing.T) {
	keyring.MockInit()
	a, _ := New(filepath.Join(t.TempDir(), "p.json"), "test-langs-notconn-"+t.Name())
	if _, err := a.AvailableRaasLanguages(); err != ErrNotConnected {
		t.Errorf("err = %v, want ErrNotConnected", err)
	}
}
