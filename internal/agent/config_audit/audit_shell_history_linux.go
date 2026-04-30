package config_audit

import (
	"context"
	"errors"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/WangYihang/Platypus/internal/agent/config_audit/sources"
)

func init() { Register(&shellHistoryAuditor{}) }

// shellHistoryAuditor inspects per-user history files. Two layered
// detections run on each line:
//
//  1. The full file is fed to gitleaks — high-confidence credential
//     hits surface here (AKIA…, ghp_…, JWT, …).
//  2. A small set of behavioural regexes flag *commands* that put a
//     credential on the command line even if no obvious cloud key is
//     present (mysql -p<inline>, curl -u user:pass, export FOO_KEY=…).
//     These are reported as medium risk: the command line itself is
//     visible in /proc, in shell history, and frequently in `ps`.
//
// History files are user-readable, so the agent without root will
// only see its own UID's homes — that's fine, we report what we can.
type shellHistoryAuditor struct{}

func (shellHistoryAuditor) ID() string       { return "shell.history" }
func (shellHistoryAuditor) Category() string { return "shell" }

func (shellHistoryAuditor) Metadata() AuditMetadata {
	return AuditMetadata{
		Title:       "Shell and REPL history files",
		Description: "Reads .bash_history, .zsh_history, .mysql_history, .psql_history, .python_history, .redis_history, .node_repl_history for each readable user. Flags both credential-shaped strings and commands that pass secrets on the command line.",
	}
}

func (shellHistoryAuditor) Applicable(_ context.Context) bool {
	return len(sources.HomeDirs()) > 0
}

var historyFiles = []string{
	".bash_history",
	".zsh_history",
	".sh_history",
	".python_history",
	".mysql_history",
	".psql_history",
	".redis_history",
	".node_repl_history",
	".lesshst",
}

// historyFileMaxBytes caps a single history file at 4 MiB. Even an
// extreme power-user keeps history well under that; the cap protects
// against a corrupted file growing without bound.
const historyFileMaxBytes = 4 * 1024 * 1024

// behavioural patterns. Each entry pairs a compiled regex with the
// finding metadata; matches surface as Pattern="behavior:<id>".
var historyBehavioural = []struct {
	id          string
	re          *regexp.Regexp
	title       string
	description string
	remediation string
}{
	{
		id:          "mysql-inline-password",
		re:          regexp.MustCompile(`(?:^|\s)mysql\b[^\n]*\s-p[^\s]`),
		title:       "Inline MySQL password on command line",
		description: "`mysql -p<password>` puts the credential in shell history, in /proc/<pid>/cmdline, and in any `ps` output. Use a defaults file (~/.my.cnf with [client] password=...) or prompt for the password instead.",
		remediation: "Replace the inline password with a configured client section or rely on the interactive prompt (`mysql -u user -p`).",
	},
	{
		id:          "curl-basic-auth",
		re:          regexp.MustCompile(`(?:^|\s)curl\b[^\n]*\s(?:-u|--user)\s+\S+:\S`),
		title:       "Inline HTTP Basic auth in curl",
		description: "`curl -u user:pass` exposes the credential in shell history and in the process command line. Read it from a credentials file (`-K`, `--netrc`) or pass it via stdin.",
		remediation: "Move the credential to ~/.netrc (chmod 600) or a curl config file passed with `-K`.",
	},
	{
		id:          "export-secret",
		re:          regexp.MustCompile(`(?i)(?:^|\s|;)export\s+([A-Z][A-Z0-9_]*(?:KEY|TOKEN|SECRET|PASSWORD|PASSWD|PWD))\s*=`),
		title:       "Secret exported into shell environment",
		description: "Exporting a credential-named variable from the interactive shell leaves it in history and in the env of every child process. Use a sourced .env file (chmod 600) or a secret manager client instead.",
		remediation: "Replace with `set -a; . ./.env; set +a` from a 600-perm file, or use direnv/age/sops/vault.",
	},
}

func (a shellHistoryAuditor) Run(ctx context.Context) ([]Leak, error) {
	var leaks []Leak
	for _, home := range sources.HomeDirs() {
		if ctx.Err() != nil {
			break
		}
		for _, name := range historyFiles {
			path := filepath.Join(home, name)
			if !sources.FileExists(path) {
				continue
			}
			data, err := sources.ReadCapped(path, historyFileMaxBytes)
			if err != nil && !errors.Is(err, sources.ErrTooLarge) {
				continue
			}
			if len(data) == 0 {
				continue
			}
			// Gitleaks pass.
			if ls, err := ScanBytes(a.ID(), a.Category(), path, data); err == nil {
				leaks = append(leaks, ls...)
			}
			// Behavioural pass.
			leaks = append(leaks, a.behaviouralScan(path, data)...)
		}
	}
	return leaks, nil
}

func (a shellHistoryAuditor) behaviouralScan(path string, data []byte) []Leak {
	var out []Leak
	sources.LineByLine(data, func(n int, text string) bool {
		// Strip zsh extended-history timestamps (": 1700000000:0;cmd").
		if strings.HasPrefix(text, ": ") {
			if i := strings.Index(text, ";"); i >= 0 {
				text = text[i+1:]
			}
		}
		for _, p := range historyBehavioural {
			if p.re.MatchString(text) {
				out = append(out, Leak{
					ID:            a.ID() + ".behavior." + p.id,
					Category:      a.Category(),
					Risk:          RiskMedium,
					Title:         p.title,
					Location:      formatLoc(path, n),
					MatchRedacted: redactCommandLine(text),
					Pattern:       "behavior:" + p.id,
					Description:   p.description,
					Remediation:   p.remediation,
				})
			}
		}
		return true
	})
	return out
}

// redactCommandLine returns the command with anything after the first
// '=' or after a space-separated argv token preserved up to its first
// 8 characters. Just enough for an operator to recognise their own
// command without leaking a literal credential to the UI.
func redactCommandLine(s string) string {
	const max = 96
	if len(s) > max {
		s = s[:max] + "…"
	}
	// Mask the most common credential-bearing patterns inline.
	s = reMaskKV.ReplaceAllString(s, "$1=****")
	s = reMaskBasicAuth.ReplaceAllString(s, "$1:****")
	return s
}

var (
	reMaskKV        = regexp.MustCompile(`(?i)([A-Z0-9_]*(?:KEY|TOKEN|SECRET|PASSWORD|PASSWD|PWD))\s*=\s*\S+`)
	reMaskBasicAuth = regexp.MustCompile(`(?:-u|--user)\s+([^:\s]+):[^\s]+`)
)

func formatLoc(path string, line int) string {
	if line <= 0 {
		return path
	}
	return path + ":" + strconv.Itoa(line)
}
