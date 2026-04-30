package config_audit

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/WangYihang/Platypus/internal/agent/config_audit/sources"
)

func init() { Register(&sshKeysAuditor{}) }

// sshKeysAuditor inspects each user's ~/.ssh directory. It is the
// only auditor that does not rely on gitleaks at all — gitleaks does
// have a "private-key" rule but it cannot tell an *encrypted* private
// key (fine to leave at rest) from an *unencrypted* one (the actual
// finding). We inspect the PEM header ourselves to make that
// distinction, then add behavioural checks on file modes.
type sshKeysAuditor struct{}

func (sshKeysAuditor) ID() string       { return "ssh.keys" }
func (sshKeysAuditor) Category() string { return "ssh" }

func (sshKeysAuditor) Metadata() AuditMetadata {
	return AuditMetadata{
		Title:       "SSH key material",
		Description: "Inspects ~/.ssh for unencrypted private keys, world-readable key files, and authorized_keys entries that grant unrestricted access.",
	}
}

func (sshKeysAuditor) Applicable(_ context.Context) bool {
	return len(sources.HomeDirs()) > 0
}

func (a sshKeysAuditor) Run(ctx context.Context) ([]Leak, error) {
	var leaks []Leak
	for _, home := range sources.HomeDirs() {
		if ctx.Err() != nil {
			break
		}
		dir := filepath.Join(home, ".ssh")
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			path := filepath.Join(dir, e.Name())
			data, err := sources.ReadCapped(path, 256*1024)
			if err != nil && !errors.Is(err, sources.ErrTooLarge) {
				continue
			}
			leaks = append(leaks, a.classify(path, e.Name(), data)...)
		}
	}
	return leaks, nil
}

func (a sshKeysAuditor) classify(path, name string, data []byte) []Leak {
	var out []Leak
	lower := strings.ToLower(name)

	// Private-key inspection. PEM has either:
	//   -----BEGIN RSA PRIVATE KEY----- ... Proc-Type: 4,ENCRYPTED   (encrypted, ok)
	//   -----BEGIN OPENSSH PRIVATE KEY----- ... bcrypt/aes256-ctr     (encrypted, ok)
	//   -----BEGIN ENCRYPTED PRIVATE KEY-----                         (encrypted, ok)
	//   anything else with PRIVATE KEY                                (unencrypted)
	if bytes.Contains(data, []byte("PRIVATE KEY-----")) {
		encrypted := bytes.Contains(data, []byte("BEGIN ENCRYPTED PRIVATE KEY")) ||
			bytes.Contains(data, []byte("Proc-Type: 4,ENCRYPTED")) ||
			isOpensshEncrypted(data)
		if !encrypted {
			out = append(out, Leak{
				ID:            a.ID() + ".unencrypted_private",
				Category:      a.Category(),
				Risk:          RiskHigh,
				Title:         "Unencrypted SSH private key on disk",
				Location:      path,
				MatchRedacted: "PEM PRIVATE KEY without passphrase",
				Pattern:       "behavior:ssh-key-unencrypted",
				Description:   "This file contains a PEM-encoded SSH private key with no passphrase. Anyone who can read the file can use it to authenticate as its owner.",
				Remediation:   "Add a passphrase: `ssh-keygen -p -f " + path + "`. Better, store the key in an agent (`ssh-add`) and rotate any keys that may have been exposed.",
			})
		}
		// Permissions on a private key.
		if perm, ok := worldReadablePerm(path); ok {
			out = append(out, Leak{
				ID:            a.ID() + ".private_world_readable",
				Category:      a.Category(),
				Risk:          RiskHigh,
				Title:         "SSH private key is world-readable",
				Location:      path,
				MatchRedacted: "mode=" + perm,
				Pattern:       "behavior:ssh-key-world-readable",
				Description:   "OpenSSH refuses to use a private key whose mode allows reads from anyone other than the owner — the file is also a leak via every other process on the host.",
				Remediation:   "Restore restrictive permissions: `chmod 600 " + path + "`.",
			})
		}
	}

	// authorized_keys: flag entries that allow no-restriction shell.
	if lower == "authorized_keys" {
		out = append(out, a.checkAuthorizedKeys(path, data)...)
	}
	return out
}

func (a sshKeysAuditor) checkAuthorizedKeys(path string, data []byte) []Leak {
	var out []Leak
	sources.LineByLine(data, func(n int, text string) bool {
		t := strings.TrimSpace(text)
		if t == "" || strings.HasPrefix(t, "#") {
			return true
		}
		// Lines starting with options use commas before the keytype.
		// We crudely detect "no options" by checking if the first
		// space-separated token is a known key type.
		first := t
		if i := strings.IndexByte(t, ' '); i >= 0 {
			first = t[:i]
		}
		switch first {
		case "ssh-rsa", "ssh-ed25519", "ssh-dss", "ecdsa-sha2-nistp256",
			"ecdsa-sha2-nistp384", "ecdsa-sha2-nistp521",
			"sk-ecdsa-sha2-nistp256@openssh.com", "sk-ssh-ed25519@openssh.com":
			// No restriction options. Flag as low risk —
			// authorized_keys without options is the default and not
			// inherently broken, but worth surfacing so the operator
			// can decide whether `from=`/`command=` would be
			// appropriate for this key's use case.
			out = append(out, Leak{
				ID:            a.ID() + ".authorized_no_options",
				Category:      a.Category(),
				Risk:          RiskInfo,
				Title:         "authorized_keys entry has no restrictions",
				Location:      path + ":" + strconv.Itoa(n),
				MatchRedacted: first + " <fingerprint hidden>",
				Pattern:       "behavior:authorized-no-options",
				Description:   "This authorized_keys entry permits full interactive login. If the key is used for an automated task, consider adding `from=`, `command=`, or `restrict` options to limit blast radius.",
				Remediation:   "Prefix the entry with options like `from=\"10.0.0.0/8\",no-pty,no-port-forwarding,command=\"<binary>\"`.",
			})
		}
		return true
	})
	return out
}

// isOpensshEncrypted parses the OpenSSH wire-format private key
// (BEGIN OPENSSH PRIVATE KEY ... END) to determine whether the cipher
// field is "none" (unencrypted) or anything else (encrypted). Strict
// parsing would decode base64 and read the cipher length-prefixed
// string at offset 15; for our purposes a textual heuristic is
// sufficient: the unencrypted form decodes to bytes containing the
// literal "none" cipher tag right after the magic header. Practically,
// looking for `aes` / `bcrypt` substrings in the base64 body is too
// loose; instead we rely on a base64-decode pass and check the cipher.
//
// Implementation detail: we keep this simple and just check for a
// known base64 prefix that corresponds to `openssh-key-v1\x00\x00\x00\x00\x04none`.
// That prefix decodes to "b3BlbnNzaC1rZXktdjEAAAAABG5vbmU" — its
// presence in the body means the key is unencrypted.
func isOpensshEncrypted(data []byte) bool {
	if !bytes.Contains(data, []byte("BEGIN OPENSSH PRIVATE KEY")) {
		// Not an OpenSSH key — caller already handled the PEM path.
		return false
	}
	const unencryptedMarker = "b3BlbnNzaC1rZXktdjEAAAAABG5vbmU"
	return !bytes.Contains(data, []byte(unencryptedMarker))
}
