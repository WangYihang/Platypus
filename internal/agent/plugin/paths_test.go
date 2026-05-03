package plugin

import (
	"path/filepath"
	"testing"
)

func TestPaths_Layout(t *testing.T) {
	root := "/tmp/agent"
	p := NewPaths(root)
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"Root", p.Root(), filepath.Join(root, "plugins")},
		{"CatalogFile", p.CatalogFile(), filepath.Join(root, "plugins", "catalog.json")},
		{"PublishersDir", p.PublishersDir(), filepath.Join(root, "plugins", "publishers")},
		{"PublisherKeyFile", p.PublisherKeyFile("abc"), filepath.Join(root, "plugins", "publishers", "abc.pub")},
		{"InstalledDir", p.InstalledDir(), filepath.Join(root, "plugins", "installed")},
		{"PluginDir", p.PluginDir("com.example.foo"), filepath.Join(root, "plugins", "installed", "com.example.foo")},
		{"VersionDir", p.VersionDir("com.example.foo", "1.0.0"),
			filepath.Join(root, "plugins", "installed", "com.example.foo", "1.0.0")},
		{"ManifestFile", p.ManifestFile("com.example.foo", "1.0.0"),
			filepath.Join(root, "plugins", "installed", "com.example.foo", "1.0.0", "plugin.yaml")},
		{"WasmFile", p.WasmFile("com.example.foo", "1.0.0", "foo.wasm"),
			filepath.Join(root, "plugins", "installed", "com.example.foo", "1.0.0", "foo.wasm")},
		{"SignatureFile", p.SignatureFile("com.example.foo", "1.0.0", "foo.wasm"),
			filepath.Join(root, "plugins", "installed", "com.example.foo", "1.0.0", "foo.wasm.minisig")},
		{"StateDir", p.StateDir("com.example.foo"),
			filepath.Join(root, "plugins", "installed", "com.example.foo", "state")},
		{"QuarantineDir", p.QuarantineDir("com.example.foo"),
			filepath.Join(root, "plugins", "quarantine", "com.example.foo")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("got %q, want %q", tc.got, tc.want)
			}
		})
	}
}
