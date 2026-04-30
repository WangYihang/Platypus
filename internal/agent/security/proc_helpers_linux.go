//go:build linux

package security

import (
	"os"
	"path/filepath"
	"strings"
)

// processNamed returns true when at least one process under /proc has
// `name` as the bare comm (basename of the executable). Walks
// /proc/<pid>/comm rather than /proc/<pid>/cmdline because comm is a
// stable 16-byte field set by the kernel, while cmdline can be
// rewritten at runtime (e.g. nginx workers often masquerade as
// "nginx: worker process").
//
// Used by the auditd / time-sync checkers to decide "is this daemon
// actually running?" without shelling out to `systemctl` or `ps`.
func processNamed(name string) bool {
	matches, err := filepath.Glob("/proc/[0-9]*/comm")
	if err != nil {
		return false
	}
	for _, p := range matches {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if strings.TrimSpace(string(b)) == name {
			return true
		}
	}
	return false
}

// anyProcessNamed is the multi-name convenience: returns true on the
// first hit. Order matters only insofar as the first match wins —
// callers that want to know which one matched should call
// processNamed individually.
func anyProcessNamed(names ...string) bool {
	for _, n := range names {
		if processNamed(n) {
			return true
		}
	}
	return false
}
