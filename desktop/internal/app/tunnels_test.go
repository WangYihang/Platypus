package app

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestApp_ListTunnels_ParsesResponse(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"token":"tok"}`))
	})
	mux.HandleFunc("/notify", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) })
	mux.HandleFunc("/api/v1/sessions/sid/tunnels", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{
            "status": true,
            "tunnels": [
                {"type":"socks5","address":"127.0.0.1:33445"},
                {"type":"push","address":"local:1080→remote:80"}
            ]
        }`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	a := freshConnectedApp(t, srv.URL)
	got, err := a.ListTunnels("sid")
	if err != nil {
		t.Fatalf("ListTunnels: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d, want 2", len(got))
	}
	if got[0].Type != "socks5" || got[0].Address != "127.0.0.1:33445" {
		t.Errorf("got[0] = %+v", got[0])
	}
}

func TestApp_CreateTunnel_PostsJSON(t *testing.T) {
	var got map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"token":"tok"}`))
	})
	mux.HandleFunc("/notify", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) })
	mux.HandleFunc("/api/v1/sessions/sid/tunnels", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &got)
		w.Write([]byte(`{"status":true,"msg":"ok"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	a := freshConnectedApp(t, srv.URL)
	if err := a.CreateTunnel("sid", "internet", "0.0.0.0:1080", "127.0.0.1:80"); err != nil {
		t.Fatalf("CreateTunnel: %v", err)
	}
	if got["mode"] != "internet" {
		t.Errorf("mode = %v", got["mode"])
	}
	if got["src_address"] != "0.0.0.0:1080" {
		t.Errorf("src = %v", got["src_address"])
	}
	if got["dst_address"] != "127.0.0.1:80" {
		t.Errorf("dst = %v", got["dst_address"])
	}
}

func TestApp_CreateTunnel_ServerError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"token":"tok"}`))
	})
	mux.HandleFunc("/notify", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) })
	mux.HandleFunc("/api/v1/sessions/sid/tunnels", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(409)
		w.Write([]byte(`{"error":"socks5 server already exists at 127.0.0.1:1080"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	a := freshConnectedApp(t, srv.URL)
	err := a.CreateTunnel("sid", "internet", "0.0.0.0:1080", "127.0.0.1:80")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "socks5") {
		t.Errorf("err missing server text: %v", err)
	}
}

func TestApp_TunnelOps_NotConnected(t *testing.T) {
	a := newTestApp(t)
	if _, err := a.ListTunnels("sid"); err != ErrNotConnected {
		t.Errorf("ListTunnels err = %v", err)
	}
	if err := a.CreateTunnel("sid", "dynamic", "", ""); err != ErrNotConnected {
		t.Errorf("CreateTunnel err = %v", err)
	}
}
