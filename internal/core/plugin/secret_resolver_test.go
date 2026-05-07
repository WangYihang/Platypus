package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// stubSecretReader is the minimum interface ResolveSecrets needs from
// storage. Tests use this in-memory implementation so the resolver
// can be exercised without standing up a SQLite database. The real
// implementation in storage.ProjectSecretRepo will satisfy the same
// interface (verified by an interface-assertion test under
// internal/api once the wire-up lands).
type stubSecretReader struct {
	values  map[string][]byte // secretID → plaintext
	missing map[string]bool   // secretID → "doesn't exist"
	revoked map[string]bool   // secretID → "exists but revoked"
}

func (s *stubSecretReader) Reveal(_ context.Context, id string) ([]byte, error) {
	if s.missing[id] {
		return nil, ErrSecretNotFound
	}
	if s.revoked[id] {
		return nil, ErrSecretRevoked
	}
	v, ok := s.values[id]
	if !ok {
		return nil, ErrSecretNotFound
	}
	return v, nil
}

// TestResolveSecrets_NoRefsRoundTrips: a config that contains no
// {"$secret":...} placeholder must come back byte-identical. This is
// the most common path — most plugin configs have no secrets at all,
// and the resolver shouldn't reformat or re-encode them.
func TestResolveSecrets_NoRefsRoundTrips(t *testing.T) {
	in := json.RawMessage(`{"region":"us-east-1","port":514,"tls":true}`)
	out, err := ResolveSecrets(context.Background(), &stubSecretReader{}, in)
	if err != nil {
		t.Fatalf("ResolveSecrets: %v", err)
	}
	if !jsonEqual(out, in) {
		t.Fatalf("round-trip drift:\n  in:  %s\n  out: %s", in, out)
	}
}

// jsonEqual normalises whitespace and key ordering so byte-level
// comparisons don't fail on cosmetic reformat.
func jsonEqual(a, b json.RawMessage) bool {
	var av, bv interface{}
	if err := json.Unmarshal(a, &av); err != nil {
		return false
	}
	if err := json.Unmarshal(b, &bv); err != nil {
		return false
	}
	ab, _ := json.Marshal(av)
	bb, _ := json.Marshal(bv)
	return string(ab) == string(bb)
}

// errorContains is a tiny helper for "the error message names this
// thing" assertions. Tests stay readable when the assertion focuses
// on the operator-facing identifier (secret_id, field path) rather
// than the full string.
func errorContains(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("err = %q, want substring %q", err.Error(), want)
	}
}

// errorIs checks errors.Is and reports the actual chain on failure.
// Useful for sentinel-error assertions where the wrapped message
// would obscure the test intent.
func errorIs(t *testing.T, err, target error) {
	t.Helper()
	if !errors.Is(err, target) {
		t.Fatalf("err = %v, want errors.Is(%v)", err, target)
	}
}

// TestResolveSecrets_SubstitutesTopLevelString: the simplest "real"
// case — a top-level field whose value is a SecretRef gets replaced
// with the secret's plaintext. Nested cases are covered separately;
// this pins the mechanic at the simplest depth so a regression
// shows up as an obvious failure rather than buried under a tree
// walk.
func TestResolveSecrets_SubstitutesTopLevelString(t *testing.T) {
	reader := &stubSecretReader{
		values: map[string][]byte{"sec_db": []byte("hunter2")},
	}
	in := json.RawMessage(`{"db_password":{"$secret":"sec_db"},"port":5432}`)
	out, err := ResolveSecrets(context.Background(), reader, in)
	if err != nil {
		t.Fatalf("ResolveSecrets: %v", err)
	}
	want := json.RawMessage(`{"db_password":"hunter2","port":5432}`)
	if !jsonEqual(out, want) {
		t.Fatalf("substitution drift:\n  out:  %s\n  want: %s", out, want)
	}
}

// TestResolveSecrets_StrictShapeOnly: an object with a $secret key
// AND extra keys is NOT a SecretRef — it passes through verbatim.
// This is the property that lets a plugin author legitimately
// define a `$secret` key in their schema (e.g., as a discriminator)
// without surprising substitution.
func TestResolveSecrets_StrictShapeOnly(t *testing.T) {
	reader := &stubSecretReader{
		values: map[string][]byte{"sec_db": []byte("hunter2")},
	}
	in := json.RawMessage(`{"weird":{"$secret":"sec_db","fallback":"ignore"}}`)
	out, err := ResolveSecrets(context.Background(), reader, in)
	if err != nil {
		t.Fatalf("ResolveSecrets: %v", err)
	}
	if !jsonEqual(out, in) {
		t.Fatalf("non-strict ref should pass through:\n  in:  %s\n  out: %s", in, out)
	}
}

// TestResolveSecrets_NestedObject: refs at any depth get
// substituted. The recursive walk is the whole point of nested
// JSON over flat KV — pin it explicitly so a future "optimisation"
// that flattens or short-circuits the walk gets caught.
func TestResolveSecrets_NestedObject(t *testing.T) {
	reader := &stubSecretReader{
		values: map[string][]byte{
			"sec_outer": []byte("outer-value"),
			"sec_inner": []byte("inner-value"),
		},
	}
	in := json.RawMessage(`{
		"top": {"$secret": "sec_outer"},
		"db": {
			"host": "10.0.0.1",
			"auth": {
				"password": {"$secret": "sec_inner"},
				"username": "svc"
			}
		}
	}`)
	out, err := ResolveSecrets(context.Background(), reader, in)
	if err != nil {
		t.Fatalf("ResolveSecrets: %v", err)
	}
	want := json.RawMessage(`{
		"top": "outer-value",
		"db": {
			"host": "10.0.0.1",
			"auth": {"password": "inner-value", "username": "svc"}
		}
	}`)
	if !jsonEqual(out, want) {
		t.Fatalf("nested substitution:\n  out:  %s\n  want: %s", out, want)
	}
}

// TestResolveSecrets_ArrayOfRefs: refs inside arrays substitute
// element-by-element. This is the case that breaks flat-KV models
// (you'd need ad-hoc index encoding); the resolver handles it as
// just another shape of recursion.
func TestResolveSecrets_ArrayOfRefs(t *testing.T) {
	reader := &stubSecretReader{
		values: map[string][]byte{
			"sec_a": []byte("alpha"),
			"sec_b": []byte("beta"),
		},
	}
	in := json.RawMessage(`{
		"keys": [
			{"$secret": "sec_a"},
			{"$secret": "sec_b"},
			"plain-string-keep"
		]
	}`)
	out, err := ResolveSecrets(context.Background(), reader, in)
	if err != nil {
		t.Fatalf("ResolveSecrets: %v", err)
	}
	want := json.RawMessage(`{"keys": ["alpha","beta","plain-string-keep"]}`)
	if !jsonEqual(out, want) {
		t.Fatalf("array substitution:\n  out:  %s\n  want: %s", out, want)
	}
}

// TestResolveSecrets_MissingSecret: a ref to a nonexistent secret
// fails with an error that names the JSON Pointer of the offending
// field plus the secret_id. Both are needed: operators see the
// path so they know WHERE to fix, and the id so they know WHAT to
// fix (or whether it's a typo, a revoked secret they need to
// rotate, or a stale config from a deleted secret).
func TestResolveSecrets_MissingSecret(t *testing.T) {
	reader := &stubSecretReader{
		missing: map[string]bool{"sec_gone": true},
	}
	in := json.RawMessage(`{"db":{"password":{"$secret":"sec_gone"}}}`)
	_, err := ResolveSecrets(context.Background(), reader, in)
	errorIs(t, err, ErrSecretNotFound)
	errorContains(t, err, "/db/password")
	errorContains(t, err, "sec_gone")
}

// TestResolveSecrets_RevokedSecret: distinct sentinel from
// not-found so callers can render "rotate this config" rather than
// "secret never existed". Same path-naming requirement.
func TestResolveSecrets_RevokedSecret(t *testing.T) {
	reader := &stubSecretReader{
		revoked: map[string]bool{"sec_rotated": true},
	}
	in := json.RawMessage(`{"api_key":{"$secret":"sec_rotated"}}`)
	_, err := ResolveSecrets(context.Background(), reader, in)
	errorIs(t, err, ErrSecretRevoked)
	errorContains(t, err, "/api_key")
	errorContains(t, err, "sec_rotated")
}

// TestResolveSecrets_RootLevelRef: a config that itself IS a
// SecretRef (rare but legal — a plugin whose entire config is a
// single string). The error path for missing root-level refs
// should still produce a readable message; "(root)" is the
// convention for the empty JSON Pointer.
func TestResolveSecrets_RootLevelRefMissing(t *testing.T) {
	reader := &stubSecretReader{missing: map[string]bool{"sec_root": true}}
	in := json.RawMessage(`{"$secret":"sec_root"}`)
	_, err := ResolveSecrets(context.Background(), reader, in)
	errorIs(t, err, ErrSecretNotFound)
	errorContains(t, err, "(root)")
}

// TestResolveSecrets_PointerEscaping: keys containing "/" or "~"
// are legal JSON but pathological for paths. Verify the error
// message preserves the escaped form so the operator can copy it
// into a JSON Pointer query (e.g., to grep their config file).
func TestResolveSecrets_PointerEscaping(t *testing.T) {
	reader := &stubSecretReader{missing: map[string]bool{"sec_x": true}}
	in := json.RawMessage(`{"weird/key":{"$secret":"sec_x"}}`)
	_, err := ResolveSecrets(context.Background(), reader, in)
	errorIs(t, err, ErrSecretNotFound)
	// "/" inside the key escapes to "~1" per RFC 6901.
	errorContains(t, err, "/weird~1key")
}

// TestResolveSecrets_ArrayOfObjectsWithRefs: the "destinations"
// pattern — array of objects each with one secret-marked field.
// Mirrors the syslog-forwarder shape called out in the design
// discussion as the canonical case where flat-KV breaks down.
func TestResolveSecrets_ArrayOfObjectsWithRefs(t *testing.T) {
	reader := &stubSecretReader{
		values: map[string][]byte{
			"sec_dd": []byte("dd-token"),
			"sec_lf": []byte("lf-token"),
		},
	}
	in := json.RawMessage(`{
		"destinations": [
			{"name": "datadog", "auth_token": {"$secret": "sec_dd"}, "tls": true},
			{"name": "logflare", "auth_token": {"$secret": "sec_lf"}, "tls": true}
		]
	}`)
	out, err := ResolveSecrets(context.Background(), reader, in)
	if err != nil {
		t.Fatalf("ResolveSecrets: %v", err)
	}
	want := json.RawMessage(`{
		"destinations": [
			{"name": "datadog", "auth_token": "dd-token", "tls": true},
			{"name": "logflare", "auth_token": "lf-token", "tls": true}
		]
	}`)
	if !jsonEqual(out, want) {
		t.Fatalf("array-of-objects substitution:\n  out:  %s\n  want: %s", out, want)
	}
}
