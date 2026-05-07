package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// ErrSecretNotFound is returned by SecretReader.Reveal when no row
// with the given secret_id exists. Callers (the resolver, the install
// validator) wrap this with the offending JSON pointer so operators
// see "config field /db/password references unknown secret sec_xyz"
// rather than a bare "not found".
var ErrSecretNotFound = errors.New("plugin: secret not found")

// ErrSecretRevoked is returned by SecretReader.Reveal when the row
// exists but has been marked revoked. Surfacing this distinct from
// not-found lets the UI render a more helpful "rotate the plugin
// config to use the new secret" message.
var ErrSecretRevoked = errors.New("plugin: secret revoked")

// SecretReader is the narrow capability the resolver needs from the
// secret store: by id, give me the plaintext (or a typed error). The
// production implementation in internal/storage.ProjectSecretRepo
// satisfies this interface; tests use an in-memory stub. Defined
// here (rather than re-using *storage.ProjectSecretRepo) so the
// core/plugin package keeps its no-storage-dependency posture.
type SecretReader interface {
	Reveal(ctx context.Context, secretID string) ([]byte, error)
}

// ResolveSecrets walks a config blob and replaces every
// {"$secret":"sec_<id>"} placeholder with the secret's plaintext
// value. Configs that don't contain any placeholder come back
// unchanged (modulo encoding/json's normalisation of whitespace).
// Configs that reference unknown or revoked secrets fail with an
// error naming the offending JSON Pointer path so the operator sees
// exactly which field to fix.
//
// The resolver does not validate the resulting config against any
// schema — that's manifest_validate.go's job. Compose the two via
// ResolveAndValidate when you need both steps.
//
// Implementation note: this is a generic walk over `interface{}`.
// We deliberately avoid a code-generated path or struct tags
// because plugin configs are arbitrary JSON shapes, declared by the
// plugin author's own JSON Schema; baking any structure into the
// resolver would defeat the open-ended nature of the config model.
func ResolveSecrets(ctx context.Context, reader SecretReader, config json.RawMessage) (json.RawMessage, error) {
	if len(config) == 0 {
		return config, nil
	}
	var v interface{}
	if err := json.Unmarshal(config, &v); err != nil {
		return nil, fmt.Errorf("plugin: parse config JSON: %w", err)
	}
	resolved, err := resolveValue(ctx, reader, v, "")
	if err != nil {
		return nil, err
	}
	out, err := json.Marshal(resolved)
	if err != nil {
		return nil, fmt.Errorf("plugin: re-encode resolved config: %w", err)
	}
	return out, nil
}

// resolveValue is the recursive worker. ptr accumulates the JSON
// Pointer (RFC 6901) for the value being processed, so error
// messages can name the failing field's path.
func resolveValue(ctx context.Context, reader SecretReader, v interface{}, ptr string) (interface{}, error) {
	// At every position in the tree we ask: "is this a SecretRef
	// shape?" first. If yes, we substitute. Only objects can be
	// SecretRefs (the shape is {"$secret":"..."}), so primitives /
	// arrays fall through to their type-specific recursion.
	switch t := v.(type) {
	case map[string]interface{}:
		if id, ok := matchSecretRef(t); ok {
			plaintext, err := reader.Reveal(ctx, id)
			if err != nil {
				return nil, fmt.Errorf("plugin: resolve secret at %s (id=%s): %w", displayPtr(ptr), id, err)
			}
			// Plaintext is bytes; for strings we store it as a JSON
			// string. Future binary-secret use cases (e.g., a
			// downloaded private key) can extend this — but for now
			// plugin secrets are textual.
			return string(plaintext), nil
		}
		out := make(map[string]interface{}, len(t))
		for k, child := range t {
			resolved, err := resolveValue(ctx, reader, child, ptr+"/"+escapePtrToken(k))
			if err != nil {
				return nil, err
			}
			out[k] = resolved
		}
		return out, nil
	case []interface{}:
		out := make([]interface{}, len(t))
		for i, child := range t {
			resolved, err := resolveValue(ctx, reader, child, fmt.Sprintf("%s/%d", ptr, i))
			if err != nil {
				return nil, err
			}
			out[i] = resolved
		}
		return out, nil
	default:
		return v, nil
	}
}

// matchSecretRef identifies a SecretRef-shaped object: exactly one
// key, named "$secret", whose value is a non-empty string. Any
// extra keys disqualify the match — "{$secret: x, fallback: y}" is
// not a ref and will pass through unchanged. The strict shape
// prevents a config like {"$secret":"sec_x", "ttl":3600} from
// confusing the resolver about whether to substitute or recurse.
func matchSecretRef(m map[string]interface{}) (string, bool) {
	if len(m) != 1 {
		return "", false
	}
	v, ok := m["$secret"]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	if !ok || s == "" {
		return "", false
	}
	return s, true
}

// displayPtr returns "(root)" for the empty pointer so error
// messages stay readable at top level. Real JSON Pointers start
// with "/" otherwise.
func displayPtr(ptr string) string {
	if ptr == "" {
		return "(root)"
	}
	return ptr
}

// escapePtrToken escapes "/" and "~" per RFC 6901. Plugin config
// keys that contain these characters are pathological but legal in
// JSON; the escape keeps error messages copy-pasteable into JSON
// Pointer queries.
func escapePtrToken(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '~':
			out = append(out, '~', '0')
		case '/':
			out = append(out, '~', '1')
		default:
			out = append(out, s[i])
		}
	}
	return string(out)
}
