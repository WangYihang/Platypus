// sys-config-audit-go is the TinyGo port of
// example/plugins/system/sys-config-audit. Same v2 auditor set
// (shell.history, cloud.aws, ssh.private_keys), same scan strategy
// (substring scanners — no regex — to keep the wasm artefact small
// and the runtime panic-free), same wire output (protojson
// ConfigAuditResponse / ListConfigAuditorsResponse).
//
// Build: tinygo build -target wasi -o sys_config_audit.wasm .
package main

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/extism/go-pdk"

	platypus "github.com/WangYihang/Platypus/sdk/go/platypus-plugin"
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
// (camel) — operators may hand-craft requests via the REST API in
// either form.
type AuditRequest struct {
	AuditorIDsCamel []string `json:"auditorIds"`
	AuditorIDsSnake []string `json:"auditor_ids"`
	Categories      []string `json:"categories"`
}

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
			title:       "Shell history credential scan",
			description: "Scans ~/.bash_history and ~/.zsh_history for embedded AWS access keys, GitHub tokens, and Authorization headers.",
			run:         auditShellHistory,
		},
		{
			id:          "cloud.aws",
			category:    "cloud",
			title:       "AWS credentials file",
			description: "Flags the presence of ~/.aws/credentials with non-empty key fields. Sensitive even when filesystem permissions are correct because the file should ideally not exist on a server.",
			run:         auditAWSCredentials,
		},
		{
			id:          "ssh.private_keys",
			category:    "ssh",
			title:       "Unencrypted SSH private keys",
			description: "Lists ~/.ssh/id_* files and flags any whose body lacks the 'ENCRYPTED' marker — i.e. keys not protected by a passphrase.",
			run:         auditSSHPrivateKeys,
		},
	}
}

//export list_config_auditors
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

//export config_audit
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
	body, err := json.Marshal(resp)
	if err != nil {
		platypus.LogErrorf("sys-config-audit-go: marshal: %s", err.Error())
		return 1
	}
	pdk.OutputString(string(body))
	return 0
}

func main() {}

// ---- auditor implementations ------------------------------------

func auditShellHistory() []ConfigLeak {
	var out []ConfigLeak
	for _, path := range candidateHistoryFiles() {
		raw, err := platypus.HostFSReadString(path)
		if err != nil {
			continue
		}
		for i, line := range strings.Split(raw, "\n") {
			lineNo := i + 1
			if m, ok := scanAWSKey(line); ok {
				out = append(out, makeLeak("aws-access-key-id", "high",
					"AWS access key id (AKIA...) embedded in shell history",
					path, lineNo, m))
				continue // one finding per line
			}
			if m, ok := scanGitHubToken(line); ok {
				out = append(out, makeLeak("github-token", "high",
					"GitHub personal-access-token (ghp_/gho_/ghs_/ghu_) in shell history",
					path, lineNo, m))
				continue
			}
			if m, ok := scanBearer(line); ok {
				out = append(out, makeLeak("bearer-token", "medium",
					"Authorization: Bearer header in shell history",
					path, lineNo, m))
			}
		}
	}
	return out
}

func makeLeak(patID, risk, title, path string, lineNo int, m string) ConfigLeak {
	return ConfigLeak{
		ID:            "shell.history." + patID,
		AuditorID:     "shell.history",
		Category:      "shell",
		Risk:          risk,
		Title:         title,
		Location:      path + ":" + strconv.Itoa(lineNo),
		MatchRedacted: redact(m),
		Pattern:       "substring:" + patID,
		Description:   "Plaintext credentials in shell history can be exfiltrated via .bash_history backup, screen-sharing, or anyone with read access to the operator's home directory.",
		Remediation:   "Rotate the credential. Clear the relevant lines from " + path + " (or the whole file). For long-term defence: set HISTCONTROL=ignorespace + prefix sensitive commands with a leading space.",
	}
}

// scanAWSKey looks for "AKIA" + 16 [A-Z0-9] chars. Returns the
// full 20-char match.
func scanAWSKey(line string) (string, bool) {
	const pat = "AKIA"
	for i := 0; i+len(pat)+16 <= len(line); i++ {
		if line[i:i+len(pat)] != pat {
			continue
		}
		key := line[i : i+len(pat)+16]
		ok := true
		for _, c := range key[len(pat):] {
			if !((c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
				ok = false
				break
			}
		}
		if ok {
			return key, true
		}
	}
	return "", false
}

// scanGitHubToken: gh[opsu]_ + 20+ alphanumeric/underscore.
func scanGitHubToken(line string) (string, bool) {
	for i := 0; i+4 <= len(line); i++ {
		if line[i] != 'g' || line[i+1] != 'h' || line[i+3] != '_' {
			continue
		}
		switch line[i+2] {
		case 'p', 'o', 's', 'u':
		default:
			continue
		}
		j := i + 4
		for j < len(line) {
			c := line[j]
			if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
				break
			}
			j++
		}
		if j-(i+4) >= 20 {
			return line[i:j], true
		}
	}
	return "", false
}

// scanBearer: case-insensitive "Bearer " + 20+ token chars.
func scanBearer(line string) (string, bool) {
	lower := strings.ToLower(line)
	idx := strings.Index(lower, "bearer ")
	if idx < 0 {
		return "", false
	}
	tokenStart := idx + len("bearer ")
	j := tokenStart
	for j < len(line) {
		c := line[j]
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '.' || c == '_' || c == '-') {
			break
		}
		j++
	}
	if j-tokenStart >= 20 {
		return line[idx:j], true
	}
	return "", false
}

func candidateHistoryFiles() []string {
	out := []string{
		"/root/.bash_history",
		"/root/.zsh_history",
	}
	if entries, err := platypus.HostFSListDir("/home"); err == nil {
		for _, e := range entries {
			if !e.IsDir {
				continue
			}
			out = append(out,
				"/home/"+e.Name+"/.bash_history",
				"/home/"+e.Name+"/.zsh_history",
			)
		}
	}
	return out
}

func auditAWSCredentials() []ConfigLeak {
	var out []ConfigLeak
	for _, path := range candidateAWSFiles() {
		raw, err := platypus.HostFSReadString(path)
		if err != nil {
			continue
		}
		for i, line := range strings.Split(raw, "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				continue
			}
			if !strings.HasPrefix(strings.ToLower(trimmed), "aws_access_key_id") {
				continue
			}
			eq := strings.IndexByte(trimmed, '=')
			if eq < 0 {
				continue
			}
			key := strings.TrimSpace(trimmed[eq+1:])
			if key == "" {
				continue
			}
			out = append(out, ConfigLeak{
				ID:            "cloud.aws.credentials_present",
				AuditorID:     "cloud.aws",
				Category:      "cloud",
				Risk:          "high",
				Title:         "AWS credentials file present on host",
				Location:      path + ":" + strconv.Itoa(i+1),
				MatchRedacted: redact(key),
				Pattern:       "behavior:aws-credentials-file",
				Description:   "An AWS access key in ~/.aws/credentials is the most common cloud-account hijack vector. Servers should use IAM-role credentials (instance metadata) instead.",
				Remediation:   "Remove the credentials file. Switch to IAM-role-based auth (EC2 instance profile, EKS service account, or similar). Rotate the leaked key in AWS console.",
			})
		}
	}
	return out
}

func candidateAWSFiles() []string {
	out := []string{"/root/.aws/credentials"}
	if entries, err := platypus.HostFSListDir("/home"); err == nil {
		for _, e := range entries {
			if !e.IsDir {
				continue
			}
			out = append(out, "/home/"+e.Name+"/.aws/credentials")
		}
	}
	return out
}

func auditSSHPrivateKeys() []ConfigLeak {
	var out []ConfigLeak
	for _, dir := range candidateSSHDirs() {
		entries, err := platypus.HostFSListDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir || !strings.HasPrefix(e.Name, "id_") || strings.HasSuffix(e.Name, ".pub") {
				continue
			}
			path := dir + "/" + e.Name
			raw, err := platypus.HostFSReadString(path)
			if err != nil {
				continue
			}
			if strings.Contains(raw, "ENCRYPTED") {
				continue
			}
			out = append(out, ConfigLeak{
				ID:            "ssh.private_key.unencrypted",
				AuditorID:     "ssh.private_keys",
				Category:      "ssh",
				Risk:          "medium",
				Title:         "Unencrypted SSH private key at " + path,
				Location:      path,
				MatchRedacted: e.Name + " (no ENCRYPTED marker in body)",
				Pattern:       "behavior:unencrypted-ssh-private-key",
				Description:   "An SSH private key without a passphrase grants full access to whoever can read the file. ssh-add can cache the passphrase to avoid re-typing.",
				Remediation:   "Add a passphrase: ssh-keygen -p -f " + path + ". Use ssh-agent / ssh-add to avoid prompts every connection.",
			})
		}
	}
	return out
}

func candidateSSHDirs() []string {
	out := []string{"/root/.ssh"}
	if entries, err := platypus.HostFSListDir("/home"); err == nil {
		for _, e := range entries {
			if !e.IsDir {
				continue
			}
			out = append(out, "/home/"+e.Name+"/.ssh")
		}
	}
	return out
}

// ---- helpers ----------------------------------------------------

// redact matches the agent-side pattern: first 4 + last 4 chars of
// the matched string, with "****" between, when the string is >8
// chars; "*" repeated otherwise. Mirrors the Rust crate's redact.
func redact(s string) string {
	r := []rune(s)
	if len(r) <= 8 {
		return strings.Repeat("*", len(r))
	}
	return string(r[:4]) + "****" + string(r[len(r)-4:])
}

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}
