package llm

import (
	"regexp"
	"strings"
)

// Redact strips secret-shaped substrings from terminal-output text
// before it goes to a third-party LLM. Best-effort, not bulletproof:
// a determined operator pasting a credential CAN still leak it (the
// LLM might infer it from context elsewhere in the buffer). The
// goal is to catch the common shapes operators encounter without
// realising — `export TOKEN=...`, `Authorization: Bearer ...`,
// PEM blocks, raw API-key blobs.
//
// Returns (cleaned text, count of redactions). Callers log the
// count so suspiciously-high redaction can prompt the operator to
// double-check what they pasted.
func Redact(s string) (string, int) {
	count := 0
	out := make([]string, 0, 256)
	for _, line := range strings.Split(s, "\n") {
		clean, n := redactLine(line)
		count += n
		out = append(out, clean)
	}
	return strings.Join(out, "\n"), count
}

func redactLine(line string) (string, int) {
	count := 0

	// 1. PEM block bodies. We keep BEGIN/END markers so the model
	//    knows "a PEM thing was here" but the actual key bytes are
	//    gone. Only the body line (between BEGIN and END) hits
	//    here — markers are short and pass the other rules.
	if pemBodyRe.MatchString(line) {
		return "<redacted: PEM body>", 1
	}

	// 2. KEY=value / KEY: value where KEY looks credential-y.
	if m := envSecretRe.FindStringSubmatchIndex(line); m != nil {
		// keep KEY + the operator before the value, drop the value.
		head := line[:m[3]]
		return head + "<redacted>", 1
	}

	// 3. Authorization: Bearer <token>  /  Authorization: Basic <blob>
	if m := bearerRe.FindStringSubmatchIndex(line); m != nil {
		return line[:m[3]] + "<redacted>", 1
	}

	// 4. Long contiguous non-whitespace blob (≥80 chars). Catches
	//    raw base64/JWT/hex blobs that aren't behind a KEY=.
	//    80 was picked over 100 to also catch typical 88-char
	//    44-byte base64 secrets and 86-char SSH key fingerprints.
	if longBlobRe.MatchString(line) {
		replaced := longBlobRe.ReplaceAllString(line, "<redacted: long blob>")
		// Count one redaction per matched blob.
		matches := len(longBlobRe.FindAllString(line, -1))
		return replaced, matches
	}

	return line, count
}

var (
	// PEM body lines: 64 base64 chars (rfc7468) + optional padding.
	// The header / footer lines start with `-----` and skip this.
	pemBodyRe = regexp.MustCompile(`^[A-Za-z0-9+/]{60,}={0,2}$`)

	// Common secret-naming patterns followed by `=` or `:` and a
	// non-whitespace value. We deliberately do NOT require a word
	// boundary before the keyword: real-world env var names are
	// underscore-prefixed (`ANTHROPIC_API_KEY`, `DB_PASSWORD`),
	// and `\b` would treat `_` as a word char and refuse to match.
	// The trade-off is false positives on prose like "your
	// password is required" — for a redaction pass aimed at
	// keeping secrets out of LLM input, "redact too much" beats
	// "leak". Capture group 1 covers the KEY+separator so the
	// redactor can preserve it. (?i) makes it case-insensitive.
	envSecretRe = regexp.MustCompile(
		`(?i)((?:password|passwd|pwd|secret|api[_-]?key|access[_-]?key|access[_-]?token|auth[_-]?token|bearer[_-]?token|client[_-]?secret|private[_-]?key|token)\s*[:=]\s*)\S+`)

	// HTTP Authorization header in a curl trace etc.
	bearerRe = regexp.MustCompile(
		`(?i)((?:authorization|x-api-key|x-auth-token)\s*[:=]\s*(?:Bearer\s+|Basic\s+)?)\S+`)

	// 80+ contiguous non-whitespace, non-quote chars. The exclusions
	// keep us from eating quoted strings whole.
	longBlobRe = regexp.MustCompile(`[^\s'"]{80,}`)
)
