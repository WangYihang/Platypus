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

func init() { Register(&databaseAuditor{}) }

// databaseAuditor mixes two kinds of checks: gitleaks against the
// credential-bearing files (my.cnf, .pgpass) and structural rules
// against the server-side config files (pg_hba.conf "trust" rows,
// redis.conf missing `requirepass`, mongod.conf without auth). The
// structural rules catch a class of leak gitleaks cannot — "the
// daemon is configured to require no credentials at all".
type databaseAuditor struct{}

func (databaseAuditor) ID() string       { return "db.config" }
func (databaseAuditor) Category() string { return "database" }

func (databaseAuditor) Metadata() AuditMetadata {
	return AuditMetadata{
		Title:       "Database server configuration",
		Description: "Reads MySQL/MariaDB/PostgreSQL/Redis/MongoDB config files. Flags inline passwords, trust-mode authentication, missing requirepass, and disabled MongoDB authorization.",
	}
}

func (databaseAuditor) Applicable(_ context.Context) bool { return true }

// credential-bearing files: gitleaks pass.
var dbCredFiles = []string{
	"/etc/mysql/my.cnf",
	"/etc/my.cnf",
}

// per-home credential files (resolved against each home dir).
var dbCredPerHome = []string{
	".my.cnf",
	".pgpass",
}

func (a databaseAuditor) Run(ctx context.Context) ([]Leak, error) {
	var leaks []Leak

	for _, p := range dbCredFiles {
		if ctx.Err() != nil {
			break
		}
		leaks = append(leaks, a.scanCreds(p)...)
	}
	for _, home := range sources.HomeDirs() {
		for _, rel := range dbCredPerHome {
			p := filepath.Join(home, rel)
			leaks = append(leaks, a.scanCreds(p)...)
		}
	}

	// Structural pg_hba.conf — match common installs. Any "trust"
	// auth method on a non-localhost connection is high risk.
	pgHba := []string{
		"/etc/postgresql/main/pg_hba.conf",
	}
	if matches, err := filepath.Glob("/etc/postgresql/*/main/pg_hba.conf"); err == nil {
		pgHba = append(pgHba, matches...)
	}
	for _, p := range pgHba {
		leaks = append(leaks, a.checkPgHba(p)...)
	}

	for _, p := range []string{"/etc/redis/redis.conf", "/etc/redis.conf"} {
		leaks = append(leaks, a.checkRedis(p)...)
	}

	for _, p := range []string{"/etc/mongod.conf", "/etc/mongodb.conf"} {
		leaks = append(leaks, a.checkMongo(p)...)
	}

	return leaks, nil
}

func (a databaseAuditor) scanCreds(path string) []Leak {
	if !sources.FileExists(path) {
		return nil
	}
	data, err := sources.ReadCapped(path, 256*1024)
	if err != nil && !errors.Is(err, sources.ErrTooLarge) {
		return nil
	}
	if len(data) == 0 {
		return nil
	}
	out, _ := ScanBytes(a.ID(), a.Category(), path, data)

	// Bonus: my.cnf / .pgpass with `password=` lines that gitleaks
	// might miss because the value is a low-entropy human password.
	// We surface those as low-risk so they appear in the UI but
	// don't drown the high-confidence hits.
	for _, m := range rePasswordKV.FindAllStringSubmatchIndex(string(data), -1) {
		full := string(data)[m[0]:m[1]]
		if isAlreadyReported(out, full) {
			continue
		}
		out = append(out, Leak{
			ID:            a.ID() + ".password_kv",
			Category:      a.Category(),
			Risk:          RiskLow,
			Title:         "Database password configured in clear text",
			Location:      path,
			MatchRedacted: redactKV(full),
			Pattern:       "behavior:password-kv",
			Description:   "A `password=` line in this database config means the credential is on disk in plaintext. Anyone who can read the file can authenticate.",
			Remediation:   "Use a credential helper, an AUTH socket, or restrict file mode to 600 and document the threat model.",
		})
	}
	return out
}

func (a databaseAuditor) checkPgHba(path string) []Leak {
	if !sources.FileExists(path) {
		return nil
	}
	data, err := sources.ReadCapped(path, 256*1024)
	if err != nil && !errors.Is(err, sources.ErrTooLarge) {
		return nil
	}
	var out []Leak
	sources.LineByLine(data, func(n int, text string) bool {
		t := strings.TrimSpace(text)
		if t == "" || strings.HasPrefix(t, "#") {
			return true
		}
		// pg_hba columns: type db user address method
		fields := strings.Fields(t)
		if len(fields) < 4 {
			return true
		}
		method := fields[len(fields)-1]
		if !strings.EqualFold(method, "trust") {
			return true
		}
		// "local" or address starting with 127./::1 is fine; any
		// other CIDR with trust is the actual problem.
		isLocal := strings.EqualFold(fields[0], "local")
		addr := ""
		if !isLocal && len(fields) >= 5 {
			addr = fields[3]
		}
		if isLocal || addr == "127.0.0.1/32" || addr == "::1/128" {
			return true
		}
		out = append(out, Leak{
			ID:            a.ID() + ".pg_hba_trust",
			Category:      a.Category(),
			Risk:          RiskHigh,
			Title:         "PostgreSQL accepts trust authentication from a network client",
			Location:      path + ":" + strconv.Itoa(n),
			MatchRedacted: t,
			Pattern:       "behavior:pg-hba-trust",
			Description:   "A pg_hba.conf entry with method `trust` lets any client matching the address authenticate as any database user with no credential check. On a non-loopback address this is a remote-exposure issue.",
			Remediation:   "Replace `trust` with `md5`, `scram-sha-256`, or `cert`, then `pg_ctl reload`.",
		})
		return true
	})
	return out
}

func (a databaseAuditor) checkRedis(path string) []Leak {
	if !sources.FileExists(path) {
		return nil
	}
	data, err := sources.ReadCapped(path, 256*1024)
	if err != nil && !errors.Is(err, sources.ErrTooLarge) {
		return nil
	}
	hasRequirePass := false
	hasBindLocal := false
	protectedModeNo := false
	sources.LineByLine(data, func(_ int, text string) bool {
		t := strings.TrimSpace(text)
		if t == "" || strings.HasPrefix(t, "#") {
			return true
		}
		switch {
		case strings.HasPrefix(t, "requirepass "):
			hasRequirePass = true
		case strings.HasPrefix(t, "bind "):
			rest := strings.TrimPrefix(t, "bind ")
			if strings.Contains(rest, "127.0.0.1") || strings.Contains(rest, "::1") {
				hasBindLocal = true
			}
		case t == "protected-mode no":
			protectedModeNo = true
		}
		return true
	})
	var out []Leak
	if !hasRequirePass && !hasBindLocal {
		out = append(out, Leak{
			ID:            a.ID() + ".redis_no_auth",
			Category:      a.Category(),
			Risk:          RiskHigh,
			Title:         "Redis accepts unauthenticated connections",
			Location:      path,
			MatchRedacted: "no requirepass; not bound to loopback",
			Pattern:       "behavior:redis-no-auth",
			Description:   "Redis with no `requirepass` and no loopback `bind` is reachable by any host that can route to it, with full database access.",
			Remediation:   "Set `requirepass <strong-secret>` and `bind 127.0.0.1 ::1`, then restart redis.",
		})
	}
	if protectedModeNo {
		out = append(out, Leak{
			ID:            a.ID() + ".redis_protected_mode_off",
			Category:      a.Category(),
			Risk:          RiskHigh,
			Title:         "Redis protected-mode is disabled",
			Location:      path,
			MatchRedacted: "protected-mode no",
			Pattern:       "behavior:redis-protected-mode-off",
			Description:   "`protected-mode no` removes redis's last-line guard against accepting unauthenticated connections from non-loopback addresses.",
			Remediation:   "Set `protected-mode yes` and ensure either `bind` is loopback-only or `requirepass` is set.",
		})
	}
	return out
}

func (a databaseAuditor) checkMongo(path string) []Leak {
	if !sources.FileExists(path) {
		return nil
	}
	data, err := sources.ReadCapped(path, 256*1024)
	if err != nil && !errors.Is(err, sources.ErrTooLarge) {
		return nil
	}
	// mongod.conf is YAML; we don't ship a YAML parser into the audit
	// package, so use a textual heuristic: the file should contain
	// "authorization: enabled" somewhere under "security:". Absence
	// or "disabled" is the finding.
	s := string(data)
	hasAuthEnabled := reMongoAuthEnabled.MatchString(s)
	if hasAuthEnabled {
		return nil
	}
	return []Leak{{
		ID:            a.ID() + ".mongo_no_auth",
		Category:      a.Category(),
		Risk:          RiskHigh,
		Title:         "MongoDB authorization is not enabled",
		Location:      path,
		MatchRedacted: "missing security.authorization=enabled",
		Pattern:       "behavior:mongo-no-auth",
		Description:   "Without `security.authorization: enabled`, mongod accepts unauthenticated clients with full access to all databases.",
		Remediation:   "Add the `security:` block with `authorization: enabled`, create an admin user, and restart mongod.",
	}}
}

var (
	rePasswordKV       = regexp.MustCompile(`(?im)^[ \t]*password\s*=\s*\S+`)
	reMongoAuthEnabled = regexp.MustCompile(`(?ims)security\s*:[^#]*authorization\s*:\s*enabled`)
)

func redactKV(s string) string {
	if i := strings.IndexByte(s, '='); i >= 0 {
		return s[:i+1] + "****"
	}
	return "****"
}

func isAlreadyReported(out []Leak, snippet string) bool {
	for _, l := range out {
		if strings.Contains(l.MatchRedacted, snippet[:min(len(snippet), 8)]) {
			return true
		}
	}
	return false
}

