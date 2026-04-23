package main

import (
	"testing"

	"github.com/WangYihang/Platypus/internal/utils/config"
)

func TestDeriveBootstrapTarget(t *testing.T) {
	tests := []struct {
		name string
		cfg  *config.Config
		want string
	}{
		{
			name: "explicit mesh bootstrap target wins",
			cfg: &config.Config{
				Mesh:      config.MeshConfig{BootstrapTarget: "10.0.0.1:7443"},
				Listeners: []config.ListenerConfig{{Host: "0.0.0.0", Port: 9001}},
			},
			want: "10.0.0.1:7443",
		},
		{
			name: "single listener inferred",
			cfg: &config.Config{
				Listeners: []config.ListenerConfig{{Host: "127.0.0.1", Port: 13337}},
			},
			want: "127.0.0.1:13337",
		},
		{
			name: "multiple listeners require explicit target",
			cfg: &config.Config{
				Listeners: []config.ListenerConfig{
					{Host: "127.0.0.1", Port: 13337},
					{Host: "127.0.0.1", Port: 13338},
				},
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deriveBootstrapTarget(tt.cfg); got != tt.want {
				t.Fatalf("deriveBootstrapTarget() = %q, want %q", got, tt.want)
			}
		})
	}
}
