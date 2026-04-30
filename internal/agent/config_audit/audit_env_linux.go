package config_audit

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/WangYihang/Platypus/internal/agent/config_audit/sources"
)

func init() { Register(&envAuditor{}) }

// envAuditor walks /proc/<pid>/environ for every process the agent can
// read and feeds the NUL-separated NAME=VALUE pairs into gitleaks.
//
// Why /proc/*/environ instead of just os.Environ(): the agent's own
// environment is rarely the interesting one — the operator wants to
// see what's exported into long-running services (web servers, CI
// runners, jump-box screen sessions) where credentials get pinned for
// the lifetime of the process. Permissions naturally limit us to our
// own UID's processes when the agent isn't root, which is the right
// failure mode (we report what we can read, no panic).
type envAuditor struct{}

func (envAuditor) ID() string       { return "env.process" }
func (envAuditor) Category() string { return "env" }

func (envAuditor) Metadata() AuditMetadata {
	return AuditMetadata{
		Title:       "Process environment variables",
		Description: "Scans environment variables of every readable process for credential-shaped values. Real services often leak API keys and DB passwords this way.",
	}
}

func (envAuditor) Applicable(_ context.Context) bool {
	st, err := os.Stat("/proc")
	return err == nil && st.IsDir()
}

// totalEnvBudget caps the aggregate bytes we feed to the detector
// across all processes. A busy host may have hundreds of processes;
// without the cap, a single misbehaving service writing megabytes of
// junk into its env could pin the agent in a long scan.
const totalEnvBudget = 4 * 1024 * 1024 // 4 MiB
const perProcEnvCap = 256 * 1024       // 256 KiB

func (a envAuditor) Run(ctx context.Context) ([]Leak, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}
	var leaks []Leak
	var consumed int64
	for _, e := range entries {
		if ctx.Err() != nil {
			break
		}
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid <= 0 {
			continue
		}
		if consumed >= totalEnvBudget {
			break
		}
		path := filepath.Join("/proc", e.Name(), "environ")
		data, rerr := sources.ReadCapped(path, perProcEnvCap)
		if rerr != nil && !errors.Is(rerr, sources.ErrTooLarge) {
			// Permission denied is the common case (other UID's
			// process when we're not root). Silently skip; an empty
			// env is indistinguishable from "we couldn't read it".
			continue
		}
		if len(data) == 0 {
			continue
		}
		consumed += int64(len(data))

		// /proc/<pid>/environ is NUL-separated NAME=VALUE pairs. We
		// feed it to gitleaks one variable at a time so the StartLine
		// reported back is the index of the offending var rather
		// than always 1. We also build a compact "comm:env-name"
		// location so the UI shows exactly which variable on which
		// process is the problem.
		comm := readComm(pid)
		for i, pair := range splitNUL(data) {
			if pair == "" {
				continue
			}
			name, _, ok := cutEq(pair)
			if !ok {
				continue
			}
			loc := fmt.Sprintf("pid=%d comm=%s env=%s", pid, comm, name)
			ls, _ := ScanString(a.ID(), a.Category(), loc, pair)
			for j := range ls {
				// gitleaks reports a synthetic line number; we have
				// already encoded the variable's identity in loc, so
				// just keep loc as-is and discard the spurious line.
				ls[j].Location = loc
			}
			leaks = append(leaks, ls...)
			_ = i
		}
	}
	return leaks, nil
}

func splitNUL(b []byte) []string {
	var out []string
	start := 0
	for i, c := range b {
		if c == 0 {
			if i > start {
				out = append(out, string(b[start:i]))
			}
			start = i + 1
		}
	}
	if start < len(b) {
		out = append(out, string(b[start:]))
	}
	return out
}

func cutEq(s string) (k, v string, ok bool) {
	i := strings.IndexByte(s, '=')
	if i < 0 {
		return s, "", false
	}
	return s[:i], s[i+1:], true
}

// readComm returns the short process name from /proc/<pid>/comm, or
// "" if it can't be read. Used purely for human-readable Location
// strings — never trust this for security decisions, a process can
// change its own comm.
func readComm(pid int) string {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
	if err != nil {
		return "?"
	}
	return strings.TrimRight(string(b), "\n")
}
