package profile

import (
	"errors"
	"path/filepath"
	"sort"
	"testing"
)

func TestRegistry_NewOnMissingFile(t *testing.T) {
	r, err := NewRegistry(filepath.Join(t.TempDir(), "profiles.json"))
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	if got := r.List(); len(got) != 0 {
		t.Errorf("List = %v, want empty", got)
	}
}

func TestRegistry_AddGetList(t *testing.T) {
	r, _ := NewRegistry(filepath.Join(t.TempDir(), "profiles.json"))

	if err := r.Add(Profile{Name: "local", URL: "http://127.0.0.1:7331"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := r.Add(Profile{Name: "prod", URL: "https://example.com:7331"}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	got, ok := r.Get("local")
	if !ok {
		t.Fatal("Get(local) not found")
	}
	if got.URL != "http://127.0.0.1:7331" {
		t.Errorf("URL = %q", got.URL)
	}

	list := r.List()
	if len(list) != 2 {
		t.Fatalf("List len = %d", len(list))
	}
	// List is sorted by Name.
	names := []string{list[0].Name, list[1].Name}
	if !sort.StringsAreSorted(names) {
		t.Errorf("List not sorted: %v", names)
	}
}

func TestRegistry_AddDuplicate(t *testing.T) {
	r, _ := NewRegistry(filepath.Join(t.TempDir(), "profiles.json"))
	r.Add(Profile{Name: "x", URL: "http://1"})

	err := r.Add(Profile{Name: "x", URL: "http://2"})
	if !errors.Is(err, ErrAlreadyExists) {
		t.Errorf("err = %v, want ErrAlreadyExists", err)
	}
	// Original unchanged.
	got, _ := r.Get("x")
	if got.URL != "http://1" {
		t.Errorf("URL after duplicate Add = %q, want http://1", got.URL)
	}
}

func TestRegistry_Update(t *testing.T) {
	r, _ := NewRegistry(filepath.Join(t.TempDir(), "profiles.json"))
	r.Add(Profile{Name: "x", URL: "http://1"})

	if err := r.Update(Profile{Name: "x", URL: "http://2"}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, _ := r.Get("x")
	if got.URL != "http://2" {
		t.Errorf("URL = %q", got.URL)
	}
}

func TestRegistry_UpdateMissing(t *testing.T) {
	r, _ := NewRegistry(filepath.Join(t.TempDir(), "profiles.json"))
	err := r.Update(Profile{Name: "missing", URL: "http://x"})
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestRegistry_Remove(t *testing.T) {
	r, _ := NewRegistry(filepath.Join(t.TempDir(), "profiles.json"))
	r.Add(Profile{Name: "x", URL: "http://1"})

	if err := r.Remove("x"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, ok := r.Get("x"); ok {
		t.Error("Get after Remove still returns ok")
	}
}

func TestRegistry_RemoveMissing(t *testing.T) {
	r, _ := NewRegistry(filepath.Join(t.TempDir(), "profiles.json"))
	err := r.Remove("missing")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestRegistry_PersistenceRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profiles.json")

	r1, _ := NewRegistry(path)
	r1.Add(Profile{Name: "local", URL: "http://127.0.0.1:7331"})
	r1.Add(Profile{Name: "prod", URL: "https://example.com:7331"})
	if err := r1.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	r2, err := NewRegistry(path)
	if err != nil {
		t.Fatalf("re-NewRegistry: %v", err)
	}
	got := r2.List()
	if len(got) != 2 {
		t.Fatalf("after reload, List len = %d", len(got))
	}
	if got[0].Name != "local" || got[1].Name != "prod" {
		t.Errorf("after reload, list = %v", got)
	}
}

func TestRegistry_SaveCreatesParentDir(t *testing.T) {
	// User config dirs may not pre-exist; Save should mkdir -p.
	path := filepath.Join(t.TempDir(), "nested", "platypus", "profiles.json")
	r, err := NewRegistry(path)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r.Add(Profile{Name: "x", URL: "http://1"})
	if err := r.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
}

func TestProfile_ValidationOnAdd(t *testing.T) {
	r, _ := NewRegistry(filepath.Join(t.TempDir(), "profiles.json"))

	cases := []struct {
		name string
		p    Profile
	}{
		{"empty name", Profile{Name: "", URL: "http://x"}},
		{"empty url", Profile{Name: "x", URL: ""}},
		{"bad url scheme", Profile{Name: "x", URL: "not-a-url"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := r.Add(tc.p); err == nil {
				t.Errorf("Add(%+v) returned nil, want validation error", tc.p)
			}
		})
	}
}
