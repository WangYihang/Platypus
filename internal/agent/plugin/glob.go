package plugin

// glob.go — small pattern matcher used by host_exec / host_net /
// host_process / host_fs to extend their per-call allowlist check
// from "exact string equality (or unconditional `*`)" to a real
// glob.
//
// Syntax (rsync / fnmatch subset):
//
//	*   any sequence of chars EXCEPT pathSep
//	**  any sequence of chars INCLUDING pathSep (crosses dirs)
//	?   any single char EXCEPT pathSep
//
// `pathSep` is byte 0 for non-path inputs (commands, host:port pairs)
// and '/' for filesystem paths. With pathSep=0 the distinction
// between * and ** vanishes — both match anything.
//
// Patterns without any of the three metacharacters fall through to
// literal == comparison; this keeps the common case — exact path /
// command — fast and lets us drop the helper in next to the existing
// loops without changing semantics for already-staged manifests.

// matchGlob reports whether value matches pattern under the syntax
// described in the file header. matchGlob with pattern == "*" + any
// pathSep matches everything except strings containing pathSep when
// pathSep != 0; this is intentional — operators who want "any path
// recursively" should write "**" or just "/".
func matchGlob(pattern, value string, pathSep byte) bool {
	// Fast path: no metacharacters → literal compare. Avoids the
	// backtracking machinery for the (overwhelmingly common today)
	// exact-string allowlist entry.
	if !hasGlobMeta(pattern) {
		return pattern == value
	}
	return globRec(pattern, value, pathSep)
}

func hasGlobMeta(s string) bool {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '*', '?':
			return true
		}
	}
	return false
}

// globRec is the recursive matcher. The classic two-pointer iterative
// version is faster but obscures the semantics around `**`; this
// recursive form maps directly onto the syntax and our patterns are
// short (allowlist entries, not large file globs) so the depth never
// matters.
//
// Subtlety around `**`: it matches zero or more chars INCLUDING the
// surrounding path separators. Concretely, `/foo/**/bar` should match
// `/foo/bar` (** = zero chars + the trailing / is elided) and
// `/etc/**` should match `/etc` (** = zero chars + the leading / is
// elided). We bake this into the matcher by:
//   - allowing `**` followed by pathSep to also try matching with
//     the trailing pathSep elided,
//   - allowing pathSep followed by `**` (at pattern start) to try
//     matching with the leading pathSep elided.
func globRec(pattern, value string, pathSep byte) bool {
	for len(pattern) > 0 {
		// /** at start (or after a literal-slash position): the leading
		// / is optional, so /etc/** matches /etc and /foo/**/bar
		// matches /foo/bar. We handle this BEFORE the default literal
		// branch so it triggers even when value is empty (which the
		// default branch would reject).
		if pathSep != 0 && len(pattern) >= 3 &&
			pattern[0] == pathSep && pattern[1] == '*' && pattern[2] == '*' {
			rest := pattern[3:]
			if globRec(rest, value, pathSep) {
				return true
			}
			// Fall through into ** handling below by stripping the /.
		}

		switch pattern[0] {
		case '*':
			if len(pattern) > 1 && pattern[1] == '*' {
				rest := pattern[2:]
				// **/X — the trailing pathSep is also elidable
				// (so **/x matches x).
				if pathSep != 0 && len(rest) > 0 && rest[0] == pathSep {
					if globRec(rest[1:], value, pathSep) {
						return true
					}
				}
				// ** matches zero or more chars (including pathSep).
				for i := 0; i <= len(value); i++ {
					if globRec(rest, value[i:], pathSep) {
						return true
					}
				}
				return false
			}
			// Single star: greedy but does not cross pathSep when
			// pathSep != 0.
			rest := pattern[1:]
			for i := 0; i <= len(value); i++ {
				if globRec(rest, value[i:], pathSep) {
					return true
				}
				if pathSep != 0 && i < len(value) && value[i] == pathSep {
					// Reached the separator without matching the
					// remainder — single * cannot consume it, so
					// give up.
					break
				}
			}
			return false

		case '?':
			if len(value) == 0 {
				return false
			}
			if pathSep != 0 && value[0] == pathSep {
				return false
			}
			pattern = pattern[1:]
			value = value[1:]

		default:
			if len(value) == 0 || pattern[0] != value[0] {
				return false
			}
			pattern = pattern[1:]
			value = value[1:]
		}
	}
	return len(value) == 0
}

// matchAny reports whether any pattern in `patterns` matches value.
// "*" alone is the legacy "unrestricted" marker — preserved as a
// fast path so existing manifests with `commands: ["*"]` keep their
// semantics under matchGlob (a `*` literal in matchGlob with
// pathSep=0 already matches everything, but tests assert this
// explicitly and the fast path is cheaper than walking the matcher).
func matchAny(patterns []string, value string, pathSep byte) bool {
	for _, p := range patterns {
		if p == "*" {
			return true
		}
		if matchGlob(p, value, pathSep) {
			return true
		}
	}
	return false
}
