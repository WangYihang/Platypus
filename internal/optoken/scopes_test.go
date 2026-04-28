package optoken_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/WangYihang/Platypus/internal/optoken"
	"github.com/WangYihang/Platypus/internal/user"
)

func TestScopeConstants_Format(t *testing.T) {
	t.Parallel()
	all := optoken.AllScopes()
	if len(all) == 0 {
		t.Fatal("AllScopes() empty — expected at least the documented set")
	}
	for _, s := range all {
		if !strings.Contains(s, ":") {
			t.Errorf("scope %q missing ':' separator", s)
		}
		if strings.Contains(s, " ") {
			t.Errorf("scope %q contains space — would break space-delimited storage", s)
		}
		if s != strings.ToLower(s) {
			t.Errorf("scope %q has uppercase — convention is lowercase", s)
		}
	}
}

func TestScopeConstants_NoDuplicates(t *testing.T) {
	t.Parallel()
	all := optoken.AllScopes()
	seen := make(map[string]struct{}, len(all))
	for _, s := range all {
		if _, dup := seen[s]; dup {
			t.Errorf("duplicate scope in AllScopes: %q", s)
		}
		seen[s] = struct{}{}
	}
}

func TestKindForPrefix(t *testing.T) {
	t.Parallel()
	cases := []struct {
		prefix   string
		wantKind optoken.Kind
		wantOK   bool
	}{
		{"aat_", optoken.KindAAT, true},
		{"pst_", optoken.KindUserSession, true},
		{"plt_", optoken.KindEnrollmentToken, true},
		{"dl_", optoken.KindInstall, true},
		{"foo_", "", false},
		{"", "", false},
		{"aat", "", false}, // missing trailing underscore — must be exact
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.prefix, func(t *testing.T) {
			t.Parallel()
			gotKind, gotOK := optoken.KindForPrefix(tc.prefix)
			if gotOK != tc.wantOK {
				t.Errorf("KindForPrefix(%q) ok = %v, want %v", tc.prefix, gotOK, tc.wantOK)
			}
			if gotKind != tc.wantKind {
				t.Errorf("KindForPrefix(%q) kind = %q, want %q", tc.prefix, gotKind, tc.wantKind)
			}
		})
	}
}

func TestDetectKind(t *testing.T) {
	t.Parallel()
	// Generate real tokens of each kind so we don't hand-craft prefixes.
	type sample struct {
		raw      string
		wantKind optoken.Kind
		wantPfx  string
	}
	mk := func(prefix string, kind optoken.Kind) sample {
		_, _, _, plaintext, err := optoken.Generate(prefix)
		if err != nil {
			t.Fatalf("Generate(%q): %v", prefix, err)
		}
		return sample{raw: plaintext, wantKind: kind, wantPfx: prefix}
	}
	cases := []sample{
		mk("aat_", optoken.KindAAT),
		mk("pst_", optoken.KindUserSession),
		mk("plt_", optoken.KindEnrollmentToken),
		mk("dl_", optoken.KindInstall),
	}
	for _, c := range cases {
		c := c
		t.Run(c.wantPfx, func(t *testing.T) {
			t.Parallel()
			gotKind, gotPfx, ok := optoken.DetectKind(c.raw)
			if !ok {
				t.Fatalf("DetectKind(%q) = !ok", c.raw)
			}
			if gotKind != c.wantKind {
				t.Errorf("kind = %q, want %q", gotKind, c.wantKind)
			}
			if gotPfx != c.wantPfx {
				t.Errorf("prefix = %q, want %q", gotPfx, c.wantPfx)
			}
		})
	}

	t.Run("unknown_prefix", func(t *testing.T) {
		t.Parallel()
		_, _, ok := optoken.DetectKind("xyz_abc.def")
		if ok {
			t.Error("DetectKind(xyz_...) = ok, want !ok")
		}
	})
	t.Run("empty", func(t *testing.T) {
		t.Parallel()
		_, _, ok := optoken.DetectKind("")
		if ok {
			t.Error("DetectKind('') = ok, want !ok")
		}
	})
	t.Run("legacy_jwt", func(t *testing.T) {
		t.Parallel()
		// Legacy JWT must NOT match any opaque prefix — verifier needs
		// to be able to reject it as unknown.
		_, _, ok := optoken.DetectKind("eyJhbGciOiJIUzI1NiJ9.foo.bar")
		if ok {
			t.Error("DetectKind(jwt) = ok, want !ok")
		}
	})
}

func TestScopesFromRole(t *testing.T) {
	t.Parallel()
	cases := []struct {
		role         user.Role
		mustHave     []string
		mustNotHave  []string
		wantNonEmpty bool
	}{
		{
			role:         user.RoleAdmin,
			mustHave:     []string{optoken.ScopeHostsRead, optoken.ScopeHostsExec, optoken.ScopeFilesWrite, optoken.ScopeRPCInvoke},
			wantNonEmpty: true,
		},
		{
			role:         user.RoleOperator,
			mustHave:     []string{optoken.ScopeHostsRead, optoken.ScopeHostsExec, optoken.ScopeFilesWrite, optoken.ScopeRPCInvoke},
			wantNonEmpty: true,
		},
		{
			role:         user.RoleViewer,
			mustHave:     []string{optoken.ScopeHostsRead, optoken.ScopeFilesRead, optoken.ScopeProjectsRead, optoken.ScopeActivityRead},
			mustNotHave:  []string{optoken.ScopeHostsExec, optoken.ScopeFilesWrite, optoken.ScopeRPCInvoke},
			wantNonEmpty: true,
		},
		{
			role:         user.Role(""),
			wantNonEmpty: false,
		},
		{
			role:         user.Role("garbage"),
			wantNonEmpty: false,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(string(tc.role), func(t *testing.T) {
			t.Parallel()
			got := optoken.ScopesFromRole(tc.role)
			if (len(got) > 0) != tc.wantNonEmpty {
				t.Fatalf("ScopesFromRole(%q) non-empty = %v, want %v (got %v)", tc.role, len(got) > 0, tc.wantNonEmpty, got)
			}
			for _, want := range tc.mustHave {
				if !slices.Contains(got, want) {
					t.Errorf("ScopesFromRole(%q) missing %q (got %v)", tc.role, want, got)
				}
			}
			for _, forbid := range tc.mustNotHave {
				if slices.Contains(got, forbid) {
					t.Errorf("ScopesFromRole(%q) unexpectedly contains %q (got %v)", tc.role, forbid, got)
				}
			}
		})
	}
}

func TestScopesFromRole_AdminIsSuperset(t *testing.T) {
	t.Parallel()
	admin := optoken.ScopesFromRole(user.RoleAdmin)
	for _, lower := range []user.Role{user.RoleOperator, user.RoleViewer} {
		got := optoken.ScopesFromRole(lower)
		for _, s := range got {
			if !slices.Contains(admin, s) {
				t.Errorf("admin missing %q from %s — admin should be superset", s, lower)
			}
		}
	}
}

func TestHasScope(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		granted []string
		want    string
		ok      bool
	}{
		{"empty_granted", nil, "hosts:read", false},
		{"single_match", []string{"hosts:read"}, "hosts:read", true},
		{"single_miss", []string{"hosts:read"}, "hosts:exec", false},
		{"first_in_list", []string{"a:b", "c:d", "e:f"}, "a:b", true},
		{"middle_in_list", []string{"a:b", "c:d", "e:f"}, "c:d", true},
		{"last_in_list", []string{"a:b", "c:d", "e:f"}, "e:f", true},
		{"case_sensitive", []string{"Hosts:Read"}, "hosts:read", false},
		{"empty_want", []string{"hosts:read"}, "", false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := optoken.HasScope(tc.granted, tc.want); got != tc.ok {
				t.Errorf("HasScope(%v, %q) = %v, want %v", tc.granted, tc.want, got, tc.ok)
			}
		})
	}
}

func TestParseList_FormatList_Roundtrip(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   []string
	}{
		{"empty", nil},
		{"single", []string{"hosts:read"}},
		{"multiple", []string{"hosts:read", "hosts:exec", "files:write"}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := optoken.FormatList(tc.in)
			parsed := optoken.ParseList(s)
			// Empty roundtrips as nil-or-empty equivalence.
			if len(tc.in) == 0 {
				if len(parsed) != 0 {
					t.Errorf("empty round-trip got %v, want empty", parsed)
				}
				return
			}
			if !slices.Equal(parsed, tc.in) {
				t.Errorf("round-trip got %v, want %v", parsed, tc.in)
			}
		})
	}
}

func TestParseList_Tolerant(t *testing.T) {
	t.Parallel()
	// Storage layer uses space-delimited; tolerant parser should
	// collapse extra whitespace and ignore empties so a manually-edited
	// row doesn't blow up auth.
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"   ", nil},
		{"hosts:read", []string{"hosts:read"}},
		{"hosts:read hosts:exec", []string{"hosts:read", "hosts:exec"}},
		{"  hosts:read   hosts:exec  ", []string{"hosts:read", "hosts:exec"}},
		{"hosts:read\thosts:exec\nfiles:write", []string{"hosts:read", "hosts:exec", "files:write"}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			got := optoken.ParseList(tc.in)
			if len(tc.want) == 0 && len(got) == 0 {
				return
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("ParseList(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
