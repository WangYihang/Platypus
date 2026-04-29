package app

import (
	"testing"

	"github.com/WangYihang/Platypus/internal/utils/config"
)

func TestNewApp(t *testing.T) {
	cfg := &config.Options{}
	a := New(cfg)

	if a.Config != cfg {
		t.Error("config not set")
	}
	if a.Sessions == nil {
		t.Error("sessions manager not initialized")
	}
}

func TestFindSessionEmpty(t *testing.T) {
	a := New(&config.Options{})
	if s := a.FindSession("anything"); s != nil {
		t.Error("expected nil for empty manager")
	}
	if s := a.FindSession(""); s != nil {
		t.Error("expected nil for empty clue")
	}
}
