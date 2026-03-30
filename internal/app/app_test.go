package app

import (
	"testing"

	"github.com/WangYihang/Platypus/internal/utils/config"
)

func TestNewApp(t *testing.T) {
	cfg := &config.Config{}
	a := New(cfg)

	if a.Config != cfg {
		t.Error("config not set")
	}
	if a.Sessions == nil {
		t.Error("sessions manager not initialized")
	}
	if a.Listeners == nil {
		t.Error("listeners manager not initialized")
	}
	if a.Servers == nil {
		t.Error("servers map not initialized")
	}
	if a.Interacting == nil {
		t.Error("interacting mutex not initialized")
	}
	if a.EnvelopeQueue == nil {
		t.Error("envelope queue not initialized")
	}
	if a.Socks5Servers == nil {
		t.Error("socks5 servers map not initialized")
	}
}

func TestFindSessionEmpty(t *testing.T) {
	a := New(&config.Config{})
	if s := a.FindSession("anything"); s != nil {
		t.Error("expected nil for empty manager")
	}
	if s := a.FindSession(""); s != nil {
		t.Error("expected nil for empty clue")
	}
}

func TestFindListenerEmpty(t *testing.T) {
	a := New(&config.Config{})
	if l := a.FindListener("anything"); l != nil {
		t.Error("expected nil for empty manager")
	}
	if l := a.FindListener(""); l != nil {
		t.Error("expected nil for empty clue")
	}
}
