package app

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	keyring "github.com/zalando/go-keyring"
)

const listServersResponse = `{
  "status": true,
  "msg": {
    "servers": {
      "h1": {"hash":"h1","host":"0.0.0.0","port":13337,"encrypted":false,"public_ip":"1.2.3.4","interfaces":["eth0"],"clients":{"c1":{}},"termite_clients":{}},
      "h2": {"hash":"h2","host":"0.0.0.0","port":13338,"encrypted":true,"public_ip":"1.2.3.4","interfaces":["eth0"],"clients":{},"termite_clients":{"c2":{},"c3":{}}}
    },
    "distributor": {"host":"0.0.0.0","port":7331,"interfaces":["eth0"],"route":{},"url":""}
  }
}`

func startListenersServer(t *testing.T) (*httptest.Server, *http.Request) {
	t.Helper()
	var lastPost *http.Request
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"token":"tok"}`))
	})
	mux.HandleFunc("/notify", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) })
	mux.HandleFunc("/api/server", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			w.Write([]byte(listServersResponse))
		case "POST":
			lastPost = r
			body, _ := io.ReadAll(r.Body)
			r.Body = io.NopCloser(strings.NewReader(string(body)))
			r.ParseForm()
			w.Write([]byte(`{"status":true,"msg":{"hash":"new"}}`))
		}
	})
	mux.HandleFunc("/api/server/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" {
			w.Write([]byte(`{"status":true}`))
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

func TestApp_CreateListener_PostsForm(t *testing.T) {
	mux := http.NewServeMux()
	var got *http.Request
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"token":"tok"}`))
	})
	mux.HandleFunc("/notify", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) })
	mux.HandleFunc("/api/server", func(w http.ResponseWriter, r *http.Request) {
		got = r
		body, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(strings.NewReader(string(body)))
		_ = r.ParseForm()
		w.Write([]byte(`{"status":true,"msg":{"hash":"new"}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	a := freshConnectedApp(t, srv.URL)
	if err := a.CreateListener("0.0.0.0", 4444, true); err != nil {
		t.Fatalf("CreateListener: %v", err)
	}
	if got == nil {
		t.Fatal("server didn't see the POST")
	}
	if got.PostFormValue("host") != "0.0.0.0" {
		t.Errorf("host = %q", got.PostFormValue("host"))
	}
	if got.PostFormValue("port") != "4444" {
		t.Errorf("port = %q", got.PostFormValue("port"))
	}
	if got.PostFormValue("encrypted") != "true" {
		t.Errorf("encrypted = %q", got.PostFormValue("encrypted"))
	}
}

func TestApp_DeleteListener(t *testing.T) {
	srv, _ := startListenersServer(t)
	a := freshConnectedApp(t, srv.URL)
	if err := a.DeleteListener("h1"); err != nil {
		t.Fatalf("DeleteListener: %v", err)
	}
}

func TestApp_GenerateRaasOneliner(t *testing.T) {
	keyring.MockInit()
	a, _ := New(filepath.Join(t.TempDir(), "p.json"), "test-raas-"+t.Name())
	// No connection needed — generation is a pure local string op.

	cases := []struct {
		name        string
		listener    string
		lang        string
		wantSubstr  string
		wantLang    string
	}{
		{"bash", "0.0.0.0:13337", "bash", "13337", "bash"},
		{"python", "0.0.0.0:13337", "python", "13337", "python"},
		{"perl", "0.0.0.0:13337", "perl", "13337", "perl"},
		{"unknown falls back to bash", "0.0.0.0:13337", "lolnope", "13337", "bash"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := a.GenerateRaasOneliner(tc.listener, tc.lang)
			if got == "" {
				t.Fatal("empty oneliner")
			}
			if !strings.Contains(got, tc.wantSubstr) {
				t.Errorf("missing %q in %q", tc.wantSubstr, got)
			}
			if tc.wantLang != "" && !strings.Contains(strings.ToLower(got), strings.ToLower(tc.wantLang)) {
				// "bash" or "python" etc should appear somewhere in the oneliner.
				if tc.lang == "lolnope" {
					// fallback to bash → must contain "bash"
					if !strings.Contains(got, "bash") {
						t.Errorf("fallback didn't produce bash: %q", got)
					}
				} else {
					t.Errorf("lang %q not found in %q", tc.wantLang, got)
				}
			}
		})
	}
}

func TestApp_AvailableRaasLanguages(t *testing.T) {
	keyring.MockInit()
	a, _ := New(filepath.Join(t.TempDir(), "p.json"), "test-langs-"+t.Name())
	got := a.AvailableRaasLanguages()
	if len(got) < 5 {
		t.Errorf("len = %d, want at least 5; got %v", len(got), got)
	}
	// Contains expected baseline.
	for _, want := range []string{"bash", "python", "perl"} {
		found := false
		for _, l := range got {
			if l == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing %q in %v", want, got)
		}
	}
}
