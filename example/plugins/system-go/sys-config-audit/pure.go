// pure.go — decision-layer logic for sys-config-audit-go. No SDK
// imports, no //go:build constraint, so `go test ./...` against the
// host triple compiles and runs these functions directly.

package main

import (
	"regexp"
	"strconv"
	"strings"
)

// ---- response shapes (mirrors v2pb encodings) -------------------

type ConfigLeak struct {
	ID            string   `json:"id"`
	AuditorID     string   `json:"auditorId"`
	Category      string   `json:"category"`
	Risk          string   `json:"risk"`
	Title         string   `json:"title"`
	Location      string   `json:"location"`
	MatchRedacted string   `json:"match"`
	Pattern       string   `json:"pattern"`
	Description   string   `json:"description"`
	Remediation   string   `json:"remediation"`
	References    []string `json:"references,omitempty"`
}

type AuditorResult struct {
	ID        string `json:"id"`
	Category  string `json:"category"`
	Status    string `json:"status"`
	Error     string `json:"error,omitempty"`
	ElapsedMs uint64 `json:"elapsedMs,omitempty"`
	LeakCount uint32 `json:"leakCount,omitempty"`
}

type AuditResponse struct {
	Leaks         []ConfigLeak    `json:"leaks"`
	Auditors      []AuditorResult `json:"auditors"`
	StartedAtUnix int64           `json:"startedAtUnix,omitempty"`
	ElapsedMs     uint64          `json:"elapsedMs,omitempty"`
	Error         string          `json:"error,omitempty"`
}

type AvailableAuditor struct {
	ID          string   `json:"id"`
	Category    string   `json:"category"`
	Applicable  bool     `json:"applicable"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	References  []string `json:"references,omitempty"`
}

type ListResponse struct {
	Auditors []AvailableAuditor `json:"auditors"`
	Error    string             `json:"error,omitempty"`
}

// AuditRequest accepts both `auditor_ids` (snake) and `auditorIds`
// (camel) keys.
type AuditRequest struct {
	AuditorIDsCamel []string `json:"auditorIds"`
	AuditorIDsSnake []string `json:"auditor_ids"`
	Categories      []string `json:"categories"`
}

// ============================================================
// Rule pack — mirror of example/plugins/system/sys-config-audit/src/rules.rs
// ============================================================

// Rule defines one credential-pattern entry. The regex is compiled
// lazily on first use; rules with malformed patterns compile to nil
// and are silently skipped. Order matters — earlier entries take
// precedence on a line.
type Rule struct {
	ID      string
	Pattern string
	Risk    string
	Title   string
}

// Rules is the curated rule pack. ~50 high-signal regex rules
// covering AWS / GCP / Azure / GitHub / GitLab / Slack / Stripe /
// Twilio / SendGrid / OpenAI / Anthropic / npm / PyPI / Docker Hub
// / Vault / JWT / generic Bearer / private-key block markers, etc.
// Mirrors the Rust crate's RULES table verbatim.
var Rules = []Rule{
	// cloud providers
	{ID: "aws-access-token", Pattern: `\b(AKIA|ASIA|AGPA|AIDA|AROA|AIPA|ANPA|ANVA|ASCA)[0-9A-Z]{16}\b`, Risk: "high", Title: "AWS access key id"},
	{ID: "aws-secret-access-key", Pattern: `(?i)aws[_\-\.]?(secret[_\-\.]?access[_\-\.]?key|secret)["']?\s*[:=]\s*["']?([A-Za-z0-9/+=]{40})["']?`, Risk: "high", Title: "AWS secret access key"},
	{ID: "gcp-api-key", Pattern: `\bAIza[0-9A-Za-z\-_]{35}\b`, Risk: "high", Title: "Google Cloud API key"},
	{ID: "gcp-service-account", Pattern: `"type"\s*:\s*"service_account"`, Risk: "high", Title: "GCP service account JSON key"},
	{ID: "azure-sas-token", Pattern: `\bsig=[A-Za-z0-9%]{32,}&se=[0-9TZ\-\:]+&sp=[a-z]+\b`, Risk: "high", Title: "Azure shared-access-signature token"},
	{ID: "azure-storage-account-key", Pattern: `(?i)account[_\-]?key["']?\s*[:=]\s*["']?([A-Za-z0-9+/=]{86}==)`, Risk: "high", Title: "Azure storage account key"},

	// payment processors
	{ID: "stripe-live-key", Pattern: `\bsk_live_[0-9A-Za-z]{24,}\b`, Risk: "high", Title: "Stripe live secret key"},
	{ID: "stripe-restricted-key", Pattern: `\brk_live_[0-9A-Za-z]{24,}\b`, Risk: "high", Title: "Stripe restricted live key"},
	{ID: "stripe-test-key", Pattern: `\bsk_test_[0-9A-Za-z]{24,}\b`, Risk: "medium", Title: "Stripe test secret key"},
	{ID: "square-access-token", Pattern: `\bsq0atp-[0-9A-Za-z\-_]{22}\b`, Risk: "high", Title: "Square access token"},
	{ID: "shopify-access-token", Pattern: `\bshpat_[0-9a-fA-F]{32}\b`, Risk: "high", Title: "Shopify private app access token"},

	// VCS / dev platforms
	{ID: "github-pat", Pattern: `\bghp_[0-9A-Za-z]{36,}\b`, Risk: "high", Title: "GitHub personal-access-token"},
	{ID: "github-oauth", Pattern: `\bgho_[0-9A-Za-z]{36,}\b`, Risk: "high", Title: "GitHub OAuth access token"},
	{ID: "github-server", Pattern: `\bghs_[0-9A-Za-z]{36,}\b`, Risk: "high", Title: "GitHub user-to-server token"},
	{ID: "github-user", Pattern: `\bghu_[0-9A-Za-z]{36,}\b`, Risk: "high", Title: "GitHub user-to-user token"},
	{ID: "github-refresh", Pattern: `\bghr_[0-9A-Za-z]{36,}\b`, Risk: "high", Title: "GitHub refresh token"},
	{ID: "github-fine-grained", Pattern: `\bgithub_pat_[0-9A-Za-z_]{82,}\b`, Risk: "high", Title: "GitHub fine-grained PAT"},
	{ID: "gitlab-pat", Pattern: `\bglpat-[0-9A-Za-z\-_]{20}\b`, Risk: "high", Title: "GitLab personal-access-token"},

	// chat / collaboration
	{ID: "slack-bot-token", Pattern: `\bxoxb-[0-9]{10,}-[0-9]{10,}-[0-9A-Za-z]{20,}\b`, Risk: "high", Title: "Slack bot token"},
	{ID: "slack-user-token", Pattern: `\bxoxp-[0-9]{10,}-[0-9]{10,}-[0-9]{10,}-[0-9A-Za-z]{20,}\b`, Risk: "high", Title: "Slack user token"},
	{ID: "slack-app-token", Pattern: `\bxoxa-[0-9A-Za-z\-]{20,}\b`, Risk: "high", Title: "Slack app-level token"},
	{ID: "slack-webhook", Pattern: `https://hooks\.slack\.com/services/T[A-Z0-9]+/B[A-Z0-9]+/[0-9A-Za-z]{20,}`, Risk: "medium", Title: "Slack incoming webhook URL"},
	{ID: "discord-bot-token", Pattern: `\b[MNO][A-Za-z0-9_\-]{23,28}\.[A-Za-z0-9_\-]{6,7}\.[A-Za-z0-9_\-]{27,38}\b`, Risk: "high", Title: "Discord bot token"},
	{ID: "discord-webhook", Pattern: `https://discord(?:app)?\.com/api/webhooks/[0-9]+/[A-Za-z0-9_\-]+`, Risk: "medium", Title: "Discord webhook URL"},
	{ID: "telegram-bot-token", Pattern: `\b[0-9]{8,10}:[A-Za-z0-9_\-]{35}\b`, Risk: "high", Title: "Telegram bot token"},

	// comms providers
	{ID: "twilio-api-key", Pattern: `\bSK[0-9a-fA-F]{32}\b`, Risk: "high", Title: "Twilio API key"},
	{ID: "twilio-account-sid", Pattern: `\bAC[0-9a-fA-F]{32}\b`, Risk: "medium", Title: "Twilio account SID"},
	{ID: "sendgrid-api-key", Pattern: `\bSG\.[A-Za-z0-9_\-]{22}\.[A-Za-z0-9_\-]{43}\b`, Risk: "high", Title: "SendGrid API key"},
	{ID: "mailgun-api-key", Pattern: `\bkey-[0-9a-zA-Z]{32}\b`, Risk: "high", Title: "Mailgun API key"},
	{ID: "mailchimp-api-key", Pattern: `\b[0-9a-f]{32}-us[0-9]{1,2}\b`, Risk: "high", Title: "Mailchimp API key"},
	{ID: "postmark-server-token", Pattern: `(?i)x-postmark-server-token["']?\s*[:=]\s*["']?([0-9a-f\-]{36})`, Risk: "high", Title: "Postmark server token"},

	// LLM / AI providers
	{ID: "openai-api-key", Pattern: `\bsk-[A-Za-z0-9_\-]{20,}T3BlbkFJ[A-Za-z0-9_\-]{20,}\b`, Risk: "high", Title: "OpenAI API key"},
	{ID: "anthropic-api-key", Pattern: `\bsk-ant-[A-Za-z0-9_\-]{40,}\b`, Risk: "high", Title: "Anthropic API key"},

	// package registries
	{ID: "npm-token", Pattern: `\bnpm_[A-Za-z0-9]{36,}\b`, Risk: "high", Title: "npm access token"},
	{ID: "pypi-upload-token", Pattern: `\bpypi-AgEIcHlwaS5vcmc[A-Za-z0-9_\-]{50,}\b`, Risk: "high", Title: "PyPI upload token"},
	{ID: "docker-hub-pat", Pattern: `\bdckr_pat_[A-Za-z0-9_\-]{20,}\b`, Risk: "high", Title: "Docker Hub personal-access-token"},

	// infra / monitoring
	{ID: "digitalocean-pat", Pattern: `\bdop_v1_[a-f0-9]{64}\b`, Risk: "high", Title: "DigitalOcean personal-access-token"},
	{ID: "cloudflare-api-token", Pattern: `(?i)cf[_\-]?(api[_\-]?)?token["']?\s*[:=]\s*["']?([A-Za-z0-9_\-]{40})`, Risk: "high", Title: "Cloudflare API token"},
	{ID: "heroku-api-key", Pattern: `(?i)heroku[_\-]?(api[_\-]?)?key["']?\s*[:=]\s*["']?([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})`, Risk: "high", Title: "Heroku API key"},
	{ID: "datadog-api-key", Pattern: `(?i)dd[_\-]?(api[_\-]?)?key["']?\s*[:=]\s*["']?([a-f0-9]{32})`, Risk: "high", Title: "Datadog API key"},
	{ID: "newrelic-license-key", Pattern: `\b[a-f0-9]{40}NRAL\b`, Risk: "high", Title: "New Relic license key"},
	{ID: "sentry-dsn", Pattern: `https://[0-9a-f]{32}@[a-zA-Z0-9.-]+sentry\.io/[0-9]+`, Risk: "medium", Title: "Sentry DSN with embedded credential"},
	{ID: "pagerduty-api-key", Pattern: `(?i)pagerduty[_\-]?(api[_\-]?)?key["']?\s*[:=]\s*["']?([0-9A-Za-z\-_+]{20,})`, Risk: "high", Title: "PagerDuty API key"},
	{ID: "circleci-token", Pattern: `\bCCIPAT_[A-Za-z0-9]{40}\b`, Risk: "high", Title: "CircleCI personal-access-token"},

	// secret managers / identity
	{ID: "vault-token", Pattern: `\bhvs\.[A-Za-z0-9_\-]{24,}\b`, Risk: "high", Title: "HashiCorp Vault service token"},
	{ID: "okta-api-token", Pattern: `\b00[A-Za-z0-9_\-]{40}\b`, Risk: "high", Title: "Okta API token (heuristic)"},
	{ID: "auth0-client-secret", Pattern: `(?i)auth0[_\-]?client[_\-]?secret["']?\s*[:=]\s*["']?([A-Za-z0-9_\-]{40,})`, Risk: "high", Title: "Auth0 client secret"},

	// private keys
	{ID: "private-key-rsa", Pattern: `-----BEGIN RSA PRIVATE KEY-----`, Risk: "high", Title: "RSA private key block"},
	{ID: "private-key-openssh", Pattern: `-----BEGIN OPENSSH PRIVATE KEY-----`, Risk: "high", Title: "OpenSSH private key block"},
	{ID: "private-key-encrypted", Pattern: `-----BEGIN ENCRYPTED PRIVATE KEY-----`, Risk: "medium", Title: "Encrypted private key block"},
	{ID: "private-key-ec", Pattern: `-----BEGIN EC PRIVATE KEY-----`, Risk: "high", Title: "EC private key block"},
	{ID: "private-key-dsa", Pattern: `-----BEGIN DSA PRIVATE KEY-----`, Risk: "high", Title: "DSA private key block"},
	{ID: "private-key-pgp", Pattern: `-----BEGIN PGP PRIVATE KEY BLOCK-----`, Risk: "high", Title: "PGP private key block"},

	// generic / heuristic
	{ID: "jwt", Pattern: `\beyJ[A-Za-z0-9_\-]{6,}\.eyJ[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}\b`, Risk: "high", Title: "JSON Web Token"},
	{ID: "bearer-token", Pattern: `(?i)\bBearer\s+[A-Za-z0-9_\-\.=:/+]{20,}\b`, Risk: "medium", Title: "Authorization Bearer header"},
	{ID: "basic-auth-url", Pattern: `https?://[^\s:@/]+:[^\s@/]+@[A-Za-z0-9\.\-]+`, Risk: "medium", Title: "URL with embedded basic-auth credentials"},
	{ID: "generic-api-key", Pattern: `(?i)\b(api[_\-]?key|apikey|x[_\-]api[_\-]key)["']?\s*[:=]\s*["']([A-Za-z0-9_\-]{16,})["']`, Risk: "medium", Title: "Generic API key assignment"},
	{ID: "generic-secret", Pattern: `(?i)\b(secret|client[_\-]?secret|app[_\-]?secret)["']?\s*[:=]\s*["']([A-Za-z0-9_\-]{16,})["']`, Risk: "medium", Title: "Generic secret assignment"},
}

// compiledRules holds the compiled regex per rule, parallel to Rules.
// Computed lazily on first scan.
var compiledRules []*regexp.Regexp

func compiledFor(i int) *regexp.Regexp {
	if compiledRules == nil {
		compiledRules = make([]*regexp.Regexp, len(Rules))
		for j, r := range Rules {
			compiledRules[j], _ = regexp.Compile(r.Pattern)
		}
	}
	if i < 0 || i >= len(compiledRules) {
		return nil
	}
	return compiledRules[i]
}

// ScanLine returns the index of the first matching rule and the
// matched substring, or (-1, "") if no rule matches.
func ScanLine(line string) (int, string) {
	for i := range Rules {
		re := compiledFor(i)
		if re == nil {
			continue
		}
		if loc := re.FindStringIndex(line); loc != nil {
			return i, line[loc[0]:loc[1]]
		}
	}
	return -1, ""
}

// ScanText walks `text` line-by-line (1-indexed line numbers) and
// yields one (rule_idx, line_no, matched_text) tuple per matching
// line. Stops at the first rule per line.
type ScanHit struct {
	RuleIdx  int
	LineNo   int
	Matched  string
}

func ScanText(text string) []ScanHit {
	var out []ScanHit
	for i, line := range strings.Split(text, "\n") {
		if idx, m := ScanLine(line); idx >= 0 {
			out = append(out, ScanHit{RuleIdx: idx, LineNo: i + 1, Matched: m})
		}
	}
	return out
}

// ============================================================
// shell.history pure helpers
// ============================================================

// NormalizeZshHistory strips the ": <epoch>:<elapsed>;" prefix zsh
// adds to extended-history entries. Bash and the others write raw
// commands; this is a no-op for them. Mirrors Rust's `.lines()`
// iterator which drops the trailing empty element after a final
// newline.
func NormalizeZshHistory(raw string) string {
	var b strings.Builder
	b.Grow(len(raw))
	lines := strings.Split(raw, "\n")
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	for _, line := range lines {
		body := line
		if rest, ok := strings.CutPrefix(line, ": "); ok {
			if idx := strings.IndexByte(rest, ';'); idx >= 0 {
				body = rest[idx+1:]
			} else {
				body = rest
			}
		}
		b.WriteString(body)
		b.WriteByte('\n')
	}
	return b.String()
}

type BehaviouralRule struct {
	ID          string
	Title       string
	Description string
	Remediation string
}

var Behavioural = []BehaviouralRule{
	{
		ID:          "mysql-inline-password",
		Title:       "Inline MySQL password on command line",
		Description: "`mysql -p<password>` puts the credential in shell history, in /proc/<pid>/cmdline, and in any `ps` output. Use a defaults file (~/.my.cnf with [client] password=...) or prompt for the password instead.",
		Remediation: "Replace the inline password with a configured client section or rely on the interactive prompt (`mysql -u user -p`).",
	},
	{
		ID:          "curl-basic-auth",
		Title:       "Inline HTTP Basic auth in curl",
		Description: "`curl -u user:pass` exposes the credential in shell history and in the process command line. Read it from a credentials file (--netrc) or pass it via stdin.",
		Remediation: "Move the credential to ~/.netrc (chmod 600) or a curl config file passed with `-K`.",
	},
	{
		ID:          "export-secret",
		Title:       "Secret exported into shell environment",
		Description: "Exporting a credential-named variable from the interactive shell leaves it in history and in the env of every child process. Use a sourced .env file (chmod 600) or a secret manager client instead.",
		Remediation: "Replace with `set -a; . ./.env; set +a` from a 600-perm file, or use direnv/age/sops/vault.",
	},
}

func MatchBehavioural(line string) (BehaviouralRule, bool) {
	if matchesMysqlInline(line) {
		return Behavioural[0], true
	}
	if matchesCurlBasicAuth(line) {
		return Behavioural[1], true
	}
	if matchesExportSecret(line) {
		return Behavioural[2], true
	}
	return BehaviouralRule{}, false
}

func matchesMysqlInline(line string) bool {
	hasMysql := false
	for _, tok := range strings.Fields(line) {
		if tok == "mysql" {
			hasMysql = true
			break
		}
	}
	if !hasMysql {
		return false
	}
	for i := 0; i+2 < len(line); i++ {
		precededBySpace := i == 0 || line[i-1] == ' ' || line[i-1] == '\t'
		if !precededBySpace || line[i] != '-' || line[i+1] != 'p' {
			continue
		}
		next := line[i+2]
		if next != ' ' && next != '\t' && next != '\n' && next != '-' {
			return true
		}
	}
	return false
}

func matchesCurlBasicAuth(line string) bool {
	tokens := strings.Fields(line)
	hasCurl := false
	for _, tok := range tokens {
		if tok == "curl" {
			hasCurl = true
			break
		}
	}
	if !hasCurl {
		return false
	}
	for i := 0; i < len(tokens)-1; i++ {
		if tokens[i] == "-u" || tokens[i] == "--user" {
			val := tokens[i+1]
			if strings.Contains(val, ":") && !strings.HasSuffix(val, ":") {
				return true
			}
		}
	}
	return false
}

func matchesExportSecret(line string) bool {
	stripped := strings.TrimLeft(line, " \t;")
	rest := ""
	for _, prefix := range []string{"export ", "EXPORT "} {
		if r, ok := strings.CutPrefix(stripped, prefix); ok {
			rest = r
			break
		}
	}
	if rest == "" {
		return false
	}
	rest = strings.TrimLeft(rest, " \t")
	eq := strings.IndexByte(rest, '=')
	if eq <= 0 {
		return false
	}
	name := strings.TrimSpace(rest[:eq])
	if name == "" {
		return false
	}
	upper := strings.ToUpper(name)
	for _, suffix := range []string{"KEY", "TOKEN", "SECRET", "PASSWORD", "PASSWD", "PWD"} {
		if strings.HasSuffix(upper, suffix) {
			return true
		}
	}
	return false
}

func RedactCommandLine(line string) string {
	const max = 96
	truncated := line
	if len(line) > max {
		truncated = line[:max] + "…"
	}
	masked := maskKV(truncated)
	return maskBasicAuth(masked)
}

// maskKV replaces VAR=value where VAR ends in KEY/TOKEN/SECRET/...
// with VAR=****. Operates on whitespace-separated tokens to stay
// UTF-8 safe.
func maskKV(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	first := true
	for _, tok := range strings.Split(s, " ") {
		if !first {
			b.WriteByte(' ')
		}
		first = false
		// Strip leading ; , | & punctuation.
		leadEnd := 0
		for leadEnd < len(tok) {
			c := tok[leadEnd]
			if c != ';' && c != ',' && c != '|' && c != '&' {
				break
			}
			leadEnd++
		}
		b.WriteString(tok[:leadEnd])
		body := tok[leadEnd:]
		eq := strings.IndexByte(body, '=')
		if eq < 0 {
			b.WriteString(body)
			continue
		}
		name := body[:eq]
		upper := strings.ToUpper(name)
		credShaped := false
		for _, suffix := range []string{"KEY", "TOKEN", "SECRET", "PASSWORD", "PASSWD", "PWD"} {
			if strings.HasSuffix(upper, suffix) {
				credShaped = true
				break
			}
		}
		if credShaped {
			b.WriteString(name)
			b.WriteString("=****")
		} else {
			b.WriteString(body)
		}
	}
	return b.String()
}

func maskBasicAuth(s string) string {
	tokens := strings.Split(s, " ")
	for i := 0; i+1 < len(tokens); i++ {
		if tokens[i] != "-u" && tokens[i] != "--user" {
			continue
		}
		idx := strings.IndexByte(tokens[i+1], ':')
		if idx < 0 {
			continue
		}
		tokens[i+1] = tokens[i+1][:idx+1] + "****"
	}
	return strings.Join(tokens, " ")
}

// ============================================================
// ssh.private_keys pure helpers
// ============================================================

func IsOpensshEncrypted(body string) bool {
	if !strings.Contains(body, "BEGIN OPENSSH PRIVATE KEY") {
		return false
	}
	// "b3BlbnNzaC1rZXktdjEAAAAABG5vbmU" is the base64 prefix that
	// appears in unencrypted OpenSSH keys (decodes to
	// "openssh-key-v1\x00\x00\x00\x00\x04none").
	const unencryptedMarker = "b3BlbnNzaC1rZXktdjEAAAAABG5vbmU"
	return !strings.Contains(body, unencryptedMarker)
}

// AuthorizedKeyTypes are the SSH key-algorithm tokens that, when
// they appear at the start of an authorized_keys line, indicate the
// entry has no restriction options prepended.
var AuthorizedKeyTypes = map[string]struct{}{
	"ssh-rsa":                              {},
	"ssh-ed25519":                          {},
	"ssh-dss":                              {},
	"ecdsa-sha2-nistp256":                  {},
	"ecdsa-sha2-nistp384":                  {},
	"ecdsa-sha2-nistp521":                  {},
	"sk-ecdsa-sha2-nistp256@openssh.com":   {},
	"sk-ssh-ed25519@openssh.com":           {},
}

// IsBareAuthorizedKeyEntry reports whether a single authorized_keys
// line carries a key with no restriction options (i.e. starts with a
// known key type, not "from=" / "command=" / "restrict").
func IsBareAuthorizedKeyEntry(line string) bool {
	t := strings.TrimSpace(line)
	if t == "" || strings.HasPrefix(t, "#") {
		return false
	}
	first := t
	if i := strings.IndexByte(t, ' '); i >= 0 {
		first = t[:i]
	}
	_, ok := AuthorizedKeyTypes[first]
	return ok
}

// ============================================================
// db.config pure helpers
// ============================================================

// PgHbaTrustFinding inspects one pg_hba.conf line. Returns the
// trimmed offending line if it grants `trust` from a non-loopback
// address; "" otherwise.
func PgHbaTrustFinding(line string) string {
	t := strings.TrimSpace(line)
	if t == "" || strings.HasPrefix(t, "#") {
		return ""
	}
	fields := strings.Fields(t)
	if len(fields) < 4 {
		return ""
	}
	method := fields[len(fields)-1]
	if !strings.EqualFold(method, "trust") {
		return ""
	}
	isLocal := strings.EqualFold(fields[0], "local")
	addr := ""
	if !isLocal && len(fields) >= 5 {
		addr = fields[3]
	}
	if isLocal || addr == "127.0.0.1/32" || addr == "::1/128" {
		return ""
	}
	return t
}

// RedisCheck inspects a redis.conf body. Returns:
//   - noAuth=true  if no `requirepass` and no loopback `bind`
//   - protOff=true if `protected-mode no` is set
func RedisCheck(body string) (noAuth, protOff bool) {
	hasRequirePass := false
	hasBindLocal := false
	for _, line := range strings.Split(body, "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		if strings.HasPrefix(t, "requirepass ") {
			hasRequirePass = true
		} else if rest, ok := strings.CutPrefix(t, "bind "); ok {
			if strings.Contains(rest, "127.0.0.1") || strings.Contains(rest, "::1") {
				hasBindLocal = true
			}
		} else if t == "protected-mode no" {
			protOff = true
		}
	}
	noAuth = !hasRequirePass && !hasBindLocal
	return
}

// MongoAuthEnabled is a YAML-ish heuristic — looks for "security:"
// followed by "authorization:" + "enabled" before the next non-
// indented top-level key.
func MongoAuthEnabled(body string) bool {
	lower := strings.ToLower(body)
	sec := strings.Index(lower, "security:")
	if sec < 0 {
		return false
	}
	after := lower[sec:]
	windowEnd := len(after)
	lines := strings.SplitAfter(after, "\n")
	off := 0
	for i, line := range lines {
		if i == 0 {
			off += len(line)
			continue
		}
		// Non-indented line that contains a colon is the next
		// top-level key — stop the window there.
		if len(line) > 0 && line[0] != ' ' && line[0] != '\t' && strings.Contains(line, ":") {
			windowEnd = off
			break
		}
		off += len(line)
	}
	window := after[:windowEnd]
	return strings.Contains(window, "authorization:") && strings.Contains(window, "enabled")
}

// ============================================================
// webapp.config pure helpers
// ============================================================

var WebappFilenames = map[string]struct{}{
	".env":                   {},
	".env.local":             {},
	".env.production":        {},
	".env.staging":           {},
	"wp-config.php":          {},
	"settings.py":            {},
	"local_settings.py":      {},
	"application.yml":        {},
	"application.yaml":       {},
	"application.properties": {},
	"appsettings.json":       {},
	"config.php":             {},
	"database.yml":           {},
	"secrets.yml":            {},
	"credentials.yml.enc":    {},
}

var WebappSkipDirs = map[string]struct{}{
	"node_modules": {},
	"vendor":       {},
	".git":         {},
	".cache":       {},
	"dist":         {},
	"build":        {},
}

func IsWebappConfigFile(name string) bool {
	lower := strings.ToLower(name)
	if _, ok := WebappFilenames[lower]; ok {
		return true
	}
	return strings.HasPrefix(lower, "appsettings.") && strings.HasSuffix(lower, ".json")
}

// ============================================================
// generic helpers
// ============================================================

// SafeID sanitises a rule id into a dotted-leak-id-safe segment.
func SafeID(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '-' || r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}

// Redact returns first 4 + last 4 chars of s with **** between them.
// Strings of 8 chars or fewer get fully starred. Mirrors the Rust
// crate's redact() byte-for-byte.
func Redact(s string) string {
	r := []rune(s)
	if len(r) <= 8 {
		return strings.Repeat("*", len(r))
	}
	return string(r[:4]) + "****" + string(r[len(r)-4:])
}

func SplitParent(path string) (parent, name string, ok bool) {
	trimmed := strings.TrimRight(path, "/")
	if trimmed == "" || trimmed == "/" {
		return "", "", false
	}
	idx := strings.LastIndex(trimmed, "/")
	if idx < 0 {
		return "", "", false
	}
	parent = "/"
	if idx > 0 {
		parent = trimmed[:idx]
	}
	name = trimmed[idx+1:]
	if name == "" {
		return "", "", false
	}
	return parent, name, true
}

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}

// formatLineLoc returns "<path>:<lineNo>".
func formatLineLoc(path string, lineNo int) string {
	return path + ":" + strconv.Itoa(lineNo)
}
