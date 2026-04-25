package optoken_test

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"strings"
	"testing"

	"github.com/WangYihang/Platypus/internal/optoken"
)

func TestGenerate_Shape(t *testing.T) {
	t.Parallel()
	for _, prefix := range []string{"aat_", "pst_", "plt_", "dl_"} {
		prefix := prefix
		t.Run(prefix, func(t *testing.T) {
			t.Parallel()
			id, secret, hash, plaintext, err := optoken.Generate(prefix)
			if err != nil {
				t.Fatalf("Generate(%q) returned err: %v", prefix, err)
			}
			if !strings.HasPrefix(id, prefix) {
				t.Errorf("id %q missing prefix %q", id, prefix)
			}
			if got := strings.TrimPrefix(id, prefix); len(got) != 20 {
				t.Errorf("id body length = %d, want 20 (id=%q)", len(got), id)
			}
			if len(secret) == 0 {
				t.Errorf("secret bytes empty")
			}
			if len(hash) != sha256.Size {
				t.Errorf("hash length = %d, want %d", len(hash), sha256.Size)
			}
			// plaintext must be "id.<secret_b32>", with the dot separator.
			parts := strings.SplitN(plaintext, ".", 2)
			if len(parts) != 2 {
				t.Fatalf("plaintext %q missing dot separator", plaintext)
			}
			if parts[0] != id {
				t.Errorf("plaintext id half = %q, want %q", parts[0], id)
			}
			if len(parts[1]) != 20 {
				t.Errorf("plaintext secret half length = %d, want 20 (got %q)", len(parts[1]), parts[1])
			}
		})
	}
}

func TestGenerate_HashMatchesSecret(t *testing.T) {
	t.Parallel()
	_, secret, hash, _, err := optoken.Generate("aat_")
	if err != nil {
		t.Fatalf("Generate err: %v", err)
	}
	want := sha256.Sum256(secret)
	if !bytes.Equal(hash, want[:]) {
		t.Errorf("hash != sha256(secret)\n got: %x\nwant: %x", hash, want[:])
	}
}

func TestGenerate_Randomness(t *testing.T) {
	t.Parallel()
	seen := make(map[string]struct{}, 32)
	for i := 0; i < 32; i++ {
		id, _, _, plaintext, err := optoken.Generate("aat_")
		if err != nil {
			t.Fatalf("Generate err: %v", err)
		}
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate id across 32 Generate calls: %q", id)
		}
		seen[id] = struct{}{}
		if !strings.HasPrefix(plaintext, id+".") {
			t.Errorf("plaintext %q does not start with %q.", plaintext, id)
		}
	}
}

func TestParse_Roundtrip(t *testing.T) {
	t.Parallel()
	for i := 0; i < 16; i++ {
		genID, genSecret, _, plaintext, err := optoken.Generate("aat_")
		if err != nil {
			t.Fatalf("Generate err: %v", err)
		}
		id, secret, err := optoken.Parse(plaintext, "aat_")
		if err != nil {
			t.Fatalf("Parse(%q) err: %v", plaintext, err)
		}
		if id != genID {
			t.Errorf("Parse id = %q, want %q", id, genID)
		}
		if !bytes.Equal(secret, genSecret) {
			t.Errorf("Parse secret bytes mismatch\n got: %x\nwant: %x", secret, genSecret)
		}
	}
}

func TestParse_Errors(t *testing.T) {
	t.Parallel()
	// Build a known-good token to derive malformed variants from.
	id, _, _, plaintext, err := optoken.Generate("aat_")
	if err != nil {
		t.Fatalf("Generate err: %v", err)
	}

	cases := []struct {
		name           string
		raw            string
		expectedPrefix string
	}{
		{"empty", "", "aat_"},
		{"prefix_only", "aat_", "aat_"},
		{"wrong_prefix", plaintext, "pst_"},
		{"no_dot", id + "abcdefghij", "aat_"},
		{"empty_id_half", "aat_." + "abcdefghijklmnopqrst", "aat_"},
		{"empty_secret_half", id + ".", "aat_"},
		{"non_base32_secret", id + ".!!!notb32!!!", "aat_"},
		{"prefix_only_with_dot", "aat_.", "aat_"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gotID, gotSecret, err := optoken.Parse(tc.raw, tc.expectedPrefix)
			if err == nil {
				t.Fatalf("Parse(%q, %q) = (%q, %x, nil), want ErrMalformed", tc.raw, tc.expectedPrefix, gotID, gotSecret)
			}
			if !errors.Is(err, optoken.ErrMalformed) {
				t.Errorf("err = %v, want ErrMalformed", err)
			}
		})
	}
}

func TestParse_CaseInsensitiveSecret(t *testing.T) {
	t.Parallel()
	// Generated secrets are lowercase; Parse must accept uppercase too
	// because base32 is case-insensitive and operators may copy-paste
	// after a font that uppercases.
	id, secret, _, plaintext, err := optoken.Generate("aat_")
	if err != nil {
		t.Fatalf("Generate err: %v", err)
	}
	parts := strings.SplitN(plaintext, ".", 2)
	if len(parts) != 2 {
		t.Fatalf("bad plaintext")
	}
	upper := parts[0] + "." + strings.ToUpper(parts[1])

	gotID, gotSecret, err := optoken.Parse(upper, "aat_")
	if err != nil {
		t.Fatalf("Parse(uppercase) err: %v", err)
	}
	if gotID != id {
		t.Errorf("id = %q, want %q", gotID, id)
	}
	if !bytes.Equal(gotSecret, secret) {
		t.Errorf("secret mismatch under uppercase input")
	}
}

func TestHash(t *testing.T) {
	t.Parallel()
	cases := [][]byte{
		nil,
		{},
		{0x00},
		[]byte("hello"),
		bytes.Repeat([]byte{0xab}, 1024),
	}
	for _, in := range cases {
		got := optoken.Hash(in)
		want := sha256.Sum256(in)
		if !bytes.Equal(got, want[:]) {
			t.Errorf("Hash(%x) = %x, want %x", in, got, want[:])
		}
		if len(got) != 32 {
			t.Errorf("Hash returned length %d, want 32", len(got))
		}
	}
}

func TestEqual(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		a, b []byte
		want bool
	}{
		{"both_nil", nil, nil, true},
		{"both_empty", []byte{}, []byte{}, true},
		{"nil_vs_empty", nil, []byte{}, true},
		{"equal_short", []byte{1, 2, 3}, []byte{1, 2, 3}, true},
		{"differ_first_byte", []byte{1, 2, 3}, []byte{9, 2, 3}, false},
		{"differ_last_byte", []byte{1, 2, 3}, []byte{1, 2, 9}, false},
		{"length_mismatch", []byte{1, 2, 3}, []byte{1, 2}, false},
		{"empty_vs_nonempty", nil, []byte{1}, false},
		{"equal_long", bytes.Repeat([]byte{0x5a}, 64), bytes.Repeat([]byte{0x5a}, 64), true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := optoken.Equal(tc.a, tc.b); got != tc.want {
				t.Errorf("Equal(%x, %x) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}
