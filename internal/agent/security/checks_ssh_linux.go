//go:build linux

package security

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
)

func init() {
	Register(&sshConfigCheck{path: "/etc/ssh/sshd_config"})
}

// sshConfigCheck parses sshd_config and surfaces the small set of
// settings that frequently appear in hardening guides. It does NOT
// shell out to `sshd -T` (which would print the effective config
// after Match blocks and Include expansion) because that requires
// either root or the sshd binary on PATH; the static parse is
// good enough for "did the operator forget to flip the obvious
// switch" without any sudo cost.
type sshConfigCheck struct {
	path string
}

func (c *sshConfigCheck) ID() string                        { return "ssh.config" }
func (c *sshConfigCheck) Category() string                  { return "ssh" }
func (c *sshConfigCheck) Applicable(_ context.Context) bool { return fileExists(c.path) }
func (c *sshConfigCheck) Metadata() CheckMetadata {
	return CheckMetadata{
		Title: "SSH server configuration",
		Description: "Parses /etc/ssh/sshd_config for the directives most often cited in " +
			"hardening guides: PermitRootLogin, PasswordAuthentication, PermitEmptyPasswords, " +
			"X11Forwarding, Protocol, LoginGraceTime. Match blocks and Include directives are " +
			"NOT honored — the parse is global-scope only, which matches the typical " +
			"\"top-level posture\" question. Run `sshd -T` for the effective config including " +
			"Match expansion when a row is unclear.",
		References: []string{"CIS 5.2", "STIG SV-204576"},
	}
}

type sshExpectation struct {
	directive string
	want      string // expected lowercase value; empty = "any value other than badValues"
	badValues []string
	severity  string
	title     string
	desc      string
	fix       string
	// onMissing controls what happens when the directive is absent.
	// Some directives default to a safe value, in which case absence
	// is fine; others (PermitRootLogin) historically default to
	// "yes" or "prohibit-password", so absence is its own finding.
	onMissing missingPolicy
}

type missingPolicy int

const (
	missingOK   missingPolicy = iota // absence = safe default
	missingFlag                      // absence = report as if value is the bad default
)

var sshExpectations = []sshExpectation{
	{
		directive: "PermitRootLogin", want: "no", severity: SeverityHigh,
		title:     "Root SSH login allowed",
		desc:      "PermitRootLogin should be 'no' so direct root logins go through sudo / per-user audit trails.",
		fix:       "Set 'PermitRootLogin no' in /etc/ssh/sshd_config and restart sshd.",
		onMissing: missingFlag,
	},
	{
		directive: "PasswordAuthentication", want: "no", severity: SeverityHigh,
		title:     "Password authentication enabled",
		desc:      "Password auth exposes accounts to credential-stuffing and slow online brute force; key-based auth is the modern baseline.",
		fix:       "Set 'PasswordAuthentication no' once you've confirmed every operator has a working SSH key.",
		onMissing: missingFlag,
	},
	{
		directive: "PermitEmptyPasswords", want: "no", severity: SeverityCritical,
		title:     "Empty passwords accepted",
		desc:      "PermitEmptyPasswords=yes lets accounts without a password log in over SSH — the worst-case configuration.",
		fix:       "Set 'PermitEmptyPasswords no' immediately.",
		onMissing: missingOK,
	},
	{
		directive: "X11Forwarding", want: "no", severity: SeverityLow,
		title:     "X11 forwarding enabled",
		desc:      "X11Forwarding=yes widens the trust boundary to every X client on the remote display; rarely needed on servers.",
		fix:       "Set 'X11Forwarding no' unless an explicit workflow needs it.",
		onMissing: missingOK,
	},
	{
		directive: "Protocol", badValues: []string{"1", "1,2", "2,1"}, severity: SeverityCritical,
		title:     "SSH Protocol 1 enabled",
		desc:      "SSH protocol 1 has been broken since 2001; it must not appear in 'Protocol' lines.",
		fix:       "Remove the Protocol directive (modern sshd is v2-only) or set 'Protocol 2'.",
		onMissing: missingOK,
	},
	{
		directive: "LoginGraceTime", badValues: []string{"0"}, severity: SeverityMedium,
		title:     "LoginGraceTime disabled",
		desc:      "LoginGraceTime=0 removes the connection-establishment timeout, helping connection-exhaustion DoS.",
		fix:       "Set 'LoginGraceTime 30' (or your site standard).",
		onMissing: missingOK,
	},
}

func (c *sshConfigCheck) Run(_ context.Context) ([]Finding, error) {
	values, err := parseSSHDConfig(c.path)
	if err != nil {
		return nil, err
	}
	var findings []Finding
	for _, e := range sshExpectations {
		got, present := values[strings.ToLower(e.directive)]
		gotLower := strings.ToLower(got)

		if !present {
			if e.onMissing == missingOK {
				continue
			}
			findings = append(findings, Finding{
				ID:          "ssh." + strings.ToLower(e.directive),
				Category:    "ssh",
				Severity:    e.severity,
				Title:       e.title,
				Description: e.desc,
				Evidence:    fmt.Sprintf("%s not set in %s; relying on sshd built-in default.", e.directive, c.path),
				Remediation: e.fix,
			})
			continue
		}

		bad := false
		switch {
		case e.want != "":
			bad = gotLower != strings.ToLower(e.want)
		case len(e.badValues) > 0:
			for _, bv := range e.badValues {
				if gotLower == strings.ToLower(bv) {
					bad = true
					break
				}
			}
		}
		if !bad {
			continue
		}
		findings = append(findings, Finding{
			ID:          "ssh." + strings.ToLower(e.directive),
			Category:    "ssh",
			Severity:    e.severity,
			Title:       e.title,
			Description: e.desc,
			Evidence:    fmt.Sprintf("%s %s", e.directive, got),
			Remediation: e.fix,
		})
	}
	return findings, nil
}

// parseSSHDConfig reads the file and returns the last-seen value for
// each directive, keyed by lowercase directive name. Lines starting
// with '#' and blank lines are ignored. Match blocks are NOT honored
// — every directive reads as if it were at top level. This is a
// known limitation; it matches the typical "global posture" question
// the scan asks. Include directives are also not expanded.
func parseSSHDConfig(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	out := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := splitSSHDLine(line)
		if !ok {
			continue
		}
		out[strings.ToLower(key)] = val
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// splitSSHDLine handles both "Key Value" and "Key=Value" forms (sshd
// accepts whitespace OR a single '=' as the separator). Trailing
// inline comments after a '#' are stripped.
func splitSSHDLine(line string) (key, value string, ok bool) {
	if i := strings.Index(line, "#"); i >= 0 {
		line = strings.TrimSpace(line[:i])
		if line == "" {
			return "", "", false
		}
	}
	sep := strings.IndexAny(line, " \t=")
	if sep <= 0 {
		return "", "", false
	}
	key = line[:sep]
	value = strings.TrimSpace(strings.TrimLeft(line[sep:], " \t="))
	if value == "" {
		return "", "", false
	}
	return key, value, true
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}
