//go:build wasip1

// sys-config-audit-go is the TinyGo / wasip1 port of
// example/plugins/system/sys-config-audit. Same v3 auditor set
// (shell.history, cloud.aws, ssh.private_keys, env.process,
// db.config, webapp.config), same wire output (protojson
// ConfigAuditResponse / ListConfigAuditorsResponse).
//
// Build:
//   tinygo build -target wasi -o sys_config_audit.wasm .   # preferred
//   GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared \
//       -o sys_config_audit.wasm .                          # fallback
//
// The pure decision-layer logic lives in pure.go (no build tag,
// host-testable); main.go owns the host-fn wrappers and entry
// points.
package main

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/extism/go-pdk"

	platypus "github.com/WangYihang/Platypus/sdk/go/platypus-plugin"
)

// ---- registered auditors ----------------------------------------

type auditor struct {
	id, category, title, description string
	run                              func() []ConfigLeak
}

func registry() []auditor {
	return []auditor{
		{
			id:          "shell.history",
			category:    "shell",
			title:       "Shell and REPL history files",
			description: "Reads .bash_history, .zsh_history, .sh_history, .python_history, .mysql_history, .psql_history, .redis_history, .node_repl_history, .lesshst for each readable user. Flags credential-shaped strings via the rule pack and three behavioural patterns.",
			run:         auditShellHistory,
		},
		{
			id:          "cloud.aws",
			category:    "cloud",
			title:       "Cloud and tool credential dotfiles",
			description: "Scans AWS, GCP, Azure, k8s, Docker, npm, pip, netrc, and git credential files in every user's home. Flags credential-shaped values and reports world-readable permission modes.",
			run:         auditCloudDotfiles,
		},
		{
			id:          "ssh.private_keys",
			category:    "ssh",
			title:       "SSH key material",
			description: "Inspects ~/.ssh for unencrypted private keys, world-readable key files, and authorized_keys entries that grant unrestricted access.",
			run:         auditSSHKeys,
		},
		{
			id:          "env.process",
			category:    "env",
			title:       "Process environment variables",
			description: "Reads /proc/<pid>/environ for every readable process and scans NAME=VALUE pairs for credential-shaped values. Per-proc cap 256 KiB; aggregate budget 4 MiB.",
			run:         auditEnvProcess,
		},
		{
			id:          "db.config",
			category:    "database",
			title:       "Database server configuration",
			description: "Reads MySQL/MariaDB/PostgreSQL/Redis/MongoDB config files. Flags inline passwords, trust-mode pg_hba.conf rows, missing redis requirepass/protected-mode, and mongod.conf without authorization.",
			run:         auditDBConfig,
		},
		{
			id:          "webapp.config",
			category:    "webapp",
			title:       "Web application configuration files",
			description: "Walks /var/www, /srv, /opt, and each user home for known framework config files. Capped at 200 files visited and 4 levels deep.",
			run:         auditWebappConfig,
		},
	}
}

// ---- entry points -----------------------------------------------

//go:wasmexport list_config_auditors
func listConfigAuditors() int32 {
	auds := registry()
	out := ListResponse{Auditors: make([]AvailableAuditor, 0, len(auds))}
	for _, a := range auds {
		out.Auditors = append(out.Auditors, AvailableAuditor{
			ID:          a.id,
			Category:    a.category,
			Applicable:  true,
			Title:       a.title,
			Description: a.description,
		})
	}
	body, err := json.Marshal(out)
	if err != nil {
		platypus.LogErrorf("sys-config-audit-go: marshal: %s", err.Error())
		return 1
	}
	pdk.OutputString(string(body))
	return 0
}

//go:wasmexport config_audit
func configAudit() int32 {
	var req AuditRequest
	if input := pdk.Input(); len(input) > 0 {
		_ = json.Unmarshal(input, &req)
	}
	want := append([]string(nil), req.AuditorIDsCamel...)
	want = append(want, req.AuditorIDsSnake...)

	resp := AuditResponse{
		Leaks:    make([]ConfigLeak, 0),
		Auditors: make([]AuditorResult, 0),
	}
	for _, a := range registry() {
		if len(want) > 0 && !contains(want, a.id) {
			continue
		}
		if len(req.Categories) > 0 && !contains(req.Categories, a.category) {
			continue
		}
		found := a.run()
		resp.Auditors = append(resp.Auditors, AuditorResult{
			ID:        a.id,
			Category:  a.category,
			Status:    "ok",
			LeakCount: uint32(len(found)),
		})
		resp.Leaks = append(resp.Leaks, found...)
	}
	resp.Leaks, resp.TotalCount, resp.HasMore = paginateLeaks(resp.Leaks, req.Offset, req.Limit)
	body, err := json.Marshal(resp)
	if err != nil {
		platypus.LogErrorf("sys-config-audit-go: marshal: %s", err.Error())
		return 1
	}
	pdk.OutputString(string(body))
	return 0
}

func main() {}

// ============================================================
// candidateHomeDirs (host-side)
// ============================================================

func candidateHomeDirs() []string {
	out := []string{"/root"}
	if entries, err := platypus.HostFSListDir("/home"); err == nil {
		for _, e := range entries {
			if e.IsDir {
				out = append(out, "/home/"+e.Name)
			}
		}
	}
	return out
}

// statViaParent fetches a directory entry from its parent listing.
func statViaParent(path string) (platypus.FSListEntry, bool) {
	parent, name, ok := SplitParent(path)
	if !ok {
		return platypus.FSListEntry{}, false
	}
	entries, err := platypus.HostFSListDir(parent)
	if err != nil {
		return platypus.FSListEntry{}, false
	}
	for _, e := range entries {
		if e.Name == name {
			return e, true
		}
	}
	return platypus.FSListEntry{}, false
}

// ============================================================
// shell.history
// ============================================================

var shellHistoryFiles = []string{
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

func auditShellHistory() []ConfigLeak {
	var out []ConfigLeak
	for _, home := range candidateHomeDirs() {
		for _, name := range shellHistoryFiles {
			path := home + "/" + name
			raw, err := platypus.HostFSReadString(path)
			if err != nil {
				continue
			}
			normalized := NormalizeZshHistory(raw)
			// Rule-pack pass.
			for _, hit := range ScanText(normalized) {
				r := Rules[hit.RuleIdx]
				out = append(out, ConfigLeak{
					ID:            "shell.history.gitleaks." + SafeID(r.ID),
					AuditorID:     "shell.history",
					Category:      "shell",
					Risk:          r.Risk,
					Title:         r.Title,
					Location:      formatLineLoc(path, hit.LineNo),
					MatchRedacted: Redact(hit.Matched),
					Pattern:       r.ID,
					Description:   "A " + strings.ToLower(r.Title) + " value was found in shell history. Plaintext credentials in history files leak via .bash_history backups, screen-sharing, and anyone with read access to the user's home directory.",
					Remediation:   "Rotate the credential. Clear the relevant lines from " + path + " (or the whole file). Set HISTCONTROL=ignorespace and prefix sensitive commands with a leading space.",
				})
			}
			// Behavioural pass.
			for i, line := range strings.Split(normalized, "\n") {
				if b, ok := MatchBehavioural(line); ok {
					out = append(out, ConfigLeak{
						ID:            "shell.history.behavior." + b.ID,
						AuditorID:     "shell.history",
						Category:      "shell",
						Risk:          "medium",
						Title:         b.Title,
						Location:      formatLineLoc(path, i+1),
						MatchRedacted: RedactCommandLine(line),
						Pattern:       "behavior:" + b.ID,
						Description:   b.Description,
						Remediation:   b.Remediation,
					})
				}
			}
		}
	}
	return out
}

// ============================================================
// cloud.aws (broadened to all credential dotfiles)
// ============================================================

type dotfileTarget struct {
	rel      string
	maxBytes int
}

var dotfileTargets = []dotfileTarget{
	{".aws/credentials", 64 * 1024},
	{".aws/config", 64 * 1024},
	{".config/gcloud/application_default_credentials.json", 64 * 1024},
	{".azure/accessTokens.json", 256 * 1024},
	{".azure/azureProfile.json", 256 * 1024},
	{".kube/config", 256 * 1024},
	{".docker/config.json", 256 * 1024},
	{".npmrc", 64 * 1024},
	{".pypirc", 64 * 1024},
	{".netrc", 64 * 1024},
	{".git-credentials", 64 * 1024},
	{".gitconfig", 256 * 1024},
}

func auditCloudDotfiles() []ConfigLeak {
	var out []ConfigLeak
	for _, home := range candidateHomeDirs() {
		for _, t := range dotfileTargets {
			path := home + "/" + t.rel
			raw, err := platypus.HostFSReadString(path)
			if err != nil {
				continue
			}
			if len(raw) > t.maxBytes {
				continue
			}
			for _, hit := range ScanText(raw) {
				r := Rules[hit.RuleIdx]
				out = append(out, ConfigLeak{
					ID:            "cloud.aws.gitleaks." + SafeID(r.ID),
					AuditorID:     "cloud.aws",
					Category:      "cloud",
					Risk:          r.Risk,
					Title:         r.Title,
					Location:      formatLineLoc(path, hit.LineNo),
					MatchRedacted: Redact(hit.Matched),
					Pattern:       r.ID,
					Description:   "A " + strings.ToLower(r.Title) + " value was found in this credential dotfile. Tools like aws-cli and gcloud write these files with restrictive perms by default; loosening them or copying them into images is the most common cloud-account hijack vector.",
					Remediation:   "Rotate the credential at its issuer. Prefer instance/role credentials (IAM role, GKE service account, managed identity) over long-lived keys on disk.",
				})
			}
			if entry, ok := statViaParent(path); ok {
				if entry.Mode&0o004 != 0 {
					out = append(out, ConfigLeak{
						ID:            "cloud.aws.world_readable",
						AuditorID:     "cloud.aws",
						Category:      "cloud",
						Risk:          "medium",
						Title:         "Credential file is world-readable",
						Location:      path,
						MatchRedacted: "mode=" + octal4(entry.Mode&0o7777),
						Pattern:       "behavior:world-readable",
						Description:   "This credential file's permission mode allows any local user to read it. AWS / gcloud / kube CLIs default to 600 — loosening that exposes the credentials to every process on the host.",
						Remediation:   "Restore restrictive permissions: chmod 600 " + path + ".",
					})
				}
			}
		}
	}
	return out
}

// octal4 renders mode as a 4-digit octal string.
func octal4(mode uint32) string {
	return string([]byte{
		'0' + byte((mode>>9)&0o7),
		'0' + byte((mode>>6)&0o7),
		'0' + byte((mode>>3)&0o7),
		'0' + byte(mode&0o7),
	})
}

// ============================================================
// ssh.private_keys
// ============================================================

func auditSSHKeys() []ConfigLeak {
	var out []ConfigLeak
	for _, home := range candidateHomeDirs() {
		dir := home + "/.ssh"
		entries, err := platypus.HostFSListDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir {
				continue
			}
			path := dir + "/" + e.Name
			body, err := platypus.HostFSReadString(path)
			if err != nil {
				continue
			}
			if strings.Contains(body, "PRIVATE KEY-----") {
				encrypted := strings.Contains(body, "BEGIN ENCRYPTED PRIVATE KEY") ||
					strings.Contains(body, "Proc-Type: 4,ENCRYPTED") ||
					IsOpensshEncrypted(body)
				if !encrypted {
					out = append(out, ConfigLeak{
						ID:            "ssh.private_keys.unencrypted_private",
						AuditorID:     "ssh.private_keys",
						Category:      "ssh",
						Risk:          "high",
						Title:         "Unencrypted SSH private key on disk",
						Location:      path,
						MatchRedacted: "PEM PRIVATE KEY without passphrase",
						Pattern:       "behavior:ssh-key-unencrypted",
						Description:   "This file contains a PEM-encoded SSH private key with no passphrase. Anyone who can read the file can use it to authenticate as its owner.",
						Remediation:   "Add a passphrase: `ssh-keygen -p -f " + path + "`. Better, store the key in an agent (`ssh-add`) and rotate any keys that may have been exposed.",
					})
				}
				if e.Mode&0o004 != 0 {
					out = append(out, ConfigLeak{
						ID:            "ssh.private_keys.private_world_readable",
						AuditorID:     "ssh.private_keys",
						Category:      "ssh",
						Risk:          "high",
						Title:         "SSH private key is world-readable",
						Location:      path,
						MatchRedacted: "mode=" + octal4(e.Mode&0o7777),
						Pattern:       "behavior:ssh-key-world-readable",
						Description:   "OpenSSH refuses to use a private key whose mode allows reads from anyone other than the owner — the file is also a leak via every other process on the host.",
						Remediation:   "Restore restrictive permissions: chmod 600 " + path + ".",
					})
				}
			}
			if strings.EqualFold(e.Name, "authorized_keys") {
				for i, line := range strings.Split(body, "\n") {
					if !IsBareAuthorizedKeyEntry(line) {
						continue
					}
					first := strings.Fields(strings.TrimSpace(line))[0]
					out = append(out, ConfigLeak{
						ID:            "ssh.private_keys.authorized_no_options",
						AuditorID:     "ssh.private_keys",
						Category:      "ssh",
						Risk:          "info",
						Title:         "authorized_keys entry has no restrictions",
						Location:      formatLineLoc(path, i+1),
						MatchRedacted: first + " <fingerprint hidden>",
						Pattern:       "behavior:authorized-no-options",
						Description:   "This authorized_keys entry permits full interactive login. If the key is used for an automated task, consider adding `from=`, `command=`, or `restrict` options to limit blast radius.",
						Remediation:   `Prefix the entry with options like "from=10.0.0.0/8,no-pty,no-port-forwarding,command=<binary>".`,
					})
				}
			}
		}
	}
	return out
}

// ============================================================
// env.process
// ============================================================

const (
	perProcEnvCap   = 256 * 1024
	totalEnvBudget  = 4 * 1024 * 1024
)

func auditEnvProcess() []ConfigLeak {
	entries, err := platypus.HostFSListDir("/proc")
	if err != nil {
		return nil
	}
	var out []ConfigLeak
	consumed := 0
	for _, e := range entries {
		if !e.IsDir {
			continue
		}
		pid, err := strconv.Atoi(e.Name)
		if err != nil || pid <= 0 {
			continue
		}
		if consumed >= totalEnvBudget {
			break
		}
		environPath := "/proc/" + e.Name + "/environ"
		environ, err := platypus.HostFSReadString(environPath)
		if err != nil || environ == "" {
			continue
		}
		bytes := len(environ)
		if bytes > perProcEnvCap {
			bytes = perProcEnvCap
		}
		consumed += bytes
		comm := "?"
		if c, err := platypus.HostFSReadString("/proc/" + e.Name + "/comm"); err == nil {
			comm = strings.TrimSpace(c)
		}
		// Walk NUL-separated NAME=VALUE pairs.
		for _, pair := range strings.Split(environ[:bytes], "\x00") {
			if pair == "" {
				continue
			}
			name := pair
			if eq := strings.IndexByte(pair, '='); eq >= 0 {
				name = pair[:eq]
			}
			loc := "pid=" + strconv.Itoa(pid) + " comm=" + comm + " env=" + name
			if idx, m := ScanLine(pair); idx >= 0 {
				r := Rules[idx]
				out = append(out, ConfigLeak{
					ID:            "env.process.gitleaks." + SafeID(r.ID),
					AuditorID:     "env.process",
					Category:      "env",
					Risk:          r.Risk,
					Title:         r.Title,
					Location:      loc,
					MatchRedacted: Redact(m),
					Pattern:       r.ID,
					Description:   "Process " + comm + " (pid " + strconv.Itoa(pid) + ") has a " + strings.ToLower(r.Title) + " value in its environment. Env vars are visible to every child process and to anyone with /proc read access for that uid.",
					Remediation:   "Restart the process after rotating the credential. Prefer reading secrets from a 600-perm file or a secret manager rather than leaving them in the env.",
				})
			}
		}
	}
	return out
}

// ============================================================
// db.config
// ============================================================

var dbCredFiles = []string{"/etc/mysql/my.cnf", "/etc/my.cnf"}
var dbCredPerHome = []string{".my.cnf", ".pgpass"}

func auditDBConfig() []ConfigLeak {
	var out []ConfigLeak
	for _, p := range dbCredFiles {
		out = append(out, scanDBCred(p)...)
	}
	for _, home := range candidateHomeDirs() {
		for _, rel := range dbCredPerHome {
			out = append(out, scanDBCred(home+"/"+rel)...)
		}
	}
	out = append(out, checkPgHba("/etc/postgresql/main/pg_hba.conf")...)
	if entries, err := platypus.HostFSListDir("/etc/postgresql"); err == nil {
		for _, d := range entries {
			if !d.IsDir {
				continue
			}
			out = append(out, checkPgHba("/etc/postgresql/"+d.Name+"/main/pg_hba.conf")...)
		}
	}
	out = append(out, checkRedis("/etc/redis/redis.conf")...)
	out = append(out, checkRedis("/etc/redis.conf")...)
	out = append(out, checkMongo("/etc/mongod.conf")...)
	out = append(out, checkMongo("/etc/mongodb.conf")...)
	return out
}

func scanDBCred(path string) []ConfigLeak {
	raw, err := platypus.HostFSReadString(path)
	if err != nil {
		return nil
	}
	var out []ConfigLeak
	for _, hit := range ScanText(raw) {
		r := Rules[hit.RuleIdx]
		out = append(out, ConfigLeak{
			ID:            "db.config.gitleaks." + SafeID(r.ID),
			AuditorID:     "db.config",
			Category:      "database",
			Risk:          r.Risk,
			Title:         r.Title,
			Location:      formatLineLoc(path, hit.LineNo),
			MatchRedacted: Redact(hit.Matched),
			Pattern:       r.ID,
			Description:   "A " + strings.ToLower(r.Title) + " value was found in this database config. Anyone who can read the file can authenticate.",
			Remediation:   "Use a credential helper (mysql-config-editor, .my.cnf [client] section with chmod 600), an AUTH socket, or a secret manager.",
		})
	}
	for i, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimLeft(line, " \t")
		lower := strings.ToLower(trimmed)
		if !strings.HasPrefix(lower, "password") {
			continue
		}
		afterPass := strings.TrimLeft(trimmed[8:], " \t")
		afterEq, ok := strings.CutPrefix(afterPass, "=")
		if !ok {
			continue
		}
		if strings.TrimSpace(afterEq) == "" {
			continue
		}
		out = append(out, ConfigLeak{
			ID:            "db.config.password_kv",
			AuditorID:     "db.config",
			Category:      "database",
			Risk:          "low",
			Title:         "Database password configured in clear text",
			Location:      formatLineLoc(path, i+1),
			MatchRedacted: "password=****",
			Pattern:       "behavior:password-kv",
			Description:   "A `password=` line in this database config means the credential is on disk in plaintext.",
			Remediation:   "Use a credential helper, an AUTH socket, or restrict file mode to 600 and document the threat model.",
		})
	}
	return out
}

func checkPgHba(path string) []ConfigLeak {
	raw, err := platypus.HostFSReadString(path)
	if err != nil {
		return nil
	}
	var out []ConfigLeak
	for i, line := range strings.Split(raw, "\n") {
		if t := PgHbaTrustFinding(line); t != "" {
			out = append(out, ConfigLeak{
				ID:            "db.config.pg_hba_trust",
				AuditorID:     "db.config",
				Category:      "database",
				Risk:          "high",
				Title:         "PostgreSQL accepts trust authentication from a network client",
				Location:      formatLineLoc(path, i+1),
				MatchRedacted: t,
				Pattern:       "behavior:pg-hba-trust",
				Description:   "A pg_hba.conf entry with method `trust` lets any client matching the address authenticate as any database user with no credential check. On a non-loopback address this is a remote-exposure issue.",
				Remediation:   "Replace `trust` with `md5`, `scram-sha-256`, or `cert`, then `pg_ctl reload`.",
			})
		}
	}
	return out
}

func checkRedis(path string) []ConfigLeak {
	raw, err := platypus.HostFSReadString(path)
	if err != nil {
		return nil
	}
	noAuth, protOff := RedisCheck(raw)
	var out []ConfigLeak
	if noAuth {
		out = append(out, ConfigLeak{
			ID:            "db.config.redis_no_auth",
			AuditorID:     "db.config",
			Category:      "database",
			Risk:          "high",
			Title:         "Redis accepts unauthenticated connections",
			Location:      path,
			MatchRedacted: "no requirepass; not bound to loopback",
			Pattern:       "behavior:redis-no-auth",
			Description:   "Redis with no `requirepass` and no loopback `bind` is reachable by any host that can route to it, with full database access.",
			Remediation:   "Set `requirepass <strong-secret>` and `bind 127.0.0.1 ::1`, then restart redis.",
		})
	}
	if protOff {
		out = append(out, ConfigLeak{
			ID:            "db.config.redis_protected_mode_off",
			AuditorID:     "db.config",
			Category:      "database",
			Risk:          "high",
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

func checkMongo(path string) []ConfigLeak {
	raw, err := platypus.HostFSReadString(path)
	if err != nil || MongoAuthEnabled(raw) {
		return nil
	}
	return []ConfigLeak{{
		ID:            "db.config.mongo_no_auth",
		AuditorID:     "db.config",
		Category:      "database",
		Risk:          "high",
		Title:         "MongoDB authorization is not enabled",
		Location:      path,
		MatchRedacted: "missing security.authorization=enabled",
		Pattern:       "behavior:mongo-no-auth",
		Description:   "Without `security.authorization: enabled`, mongod accepts unauthenticated clients with full access to all databases.",
		Remediation:   "Add the `security:` block with `authorization: enabled`, create an admin user, and restart mongod.",
	}}
}

// ============================================================
// webapp.config
// ============================================================

var webappRoots = []string{"/var/www", "/srv", "/opt"}

const (
	webappMaxDepth = 4
	webappMaxFiles = 200
	webappMaxBytes = 1 * 1024 * 1024
)

func auditWebappConfig() []ConfigLeak {
	roots := append([]string{}, webappRoots...)
	roots = append(roots, candidateHomeDirs()...)

	var out []ConfigLeak
	visited := 0

	for _, root := range roots {
		if visited >= webappMaxFiles {
			return out
		}
		if _, err := platypus.HostFSListDir(root); err != nil {
			continue
		}
		type frame struct {
			path  string
			depth int
		}
		stack := []frame{{root, 0}}
		for len(stack) > 0 {
			if visited >= webappMaxFiles {
				return out
			}
			top := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			entries, err := platypus.HostFSListDir(top.path)
			if err != nil {
				continue
			}
			for _, e := range entries {
				if visited >= webappMaxFiles {
					return out
				}
				path := top.path + "/" + e.Name
				if e.IsDir {
					if _, skip := WebappSkipDirs[e.Name]; skip {
						continue
					}
					if top.depth+1 <= webappMaxDepth {
						stack = append(stack, frame{path, top.depth + 1})
					}
					continue
				}
				if !IsWebappConfigFile(e.Name) {
					continue
				}
				visited++
				raw, err := platypus.HostFSReadString(path)
				if err != nil || raw == "" || len(raw) > webappMaxBytes {
					continue
				}
				for _, hit := range ScanText(raw) {
					r := Rules[hit.RuleIdx]
					out = append(out, ConfigLeak{
						ID:            "webapp.config.gitleaks." + SafeID(r.ID),
						AuditorID:     "webapp.config",
						Category:      "webapp",
						Risk:          r.Risk,
						Title:         r.Title,
						Location:      formatLineLoc(path, hit.LineNo),
						MatchRedacted: Redact(hit.Matched),
						Pattern:       r.ID,
						Description:   "A " + strings.ToLower(r.Title) + " value was found in this web-app config. If the file is shipped into a container image or readable by other UIDs, the credential's blast radius is the entire host.",
						Remediation:   "Read secrets from a sourced .env file outside the deploy tree (chmod 600), or inject via a secret manager / orchestrator.",
					})
				}
				if e.Mode&0o004 != 0 {
					out = append(out, ConfigLeak{
						ID:            "webapp.config.world_readable",
						AuditorID:     "webapp.config",
						Category:      "webapp",
						Risk:          "low",
						Title:         "Web app config file is world-readable",
						Location:      path,
						MatchRedacted: "mode=" + octal4(e.Mode&0o7777),
						Pattern:       "behavior:world-readable",
						Description:   "This config file's permission mode allows any local user to read it. If it contains secrets the blast radius is the entire host, not just the web-app's own UID.",
						Remediation:   "Tighten with `chmod 640 " + path + "` (or 600) and ensure it's owned by the web-app user.",
					})
				}
			}
		}
	}
	return out
}
