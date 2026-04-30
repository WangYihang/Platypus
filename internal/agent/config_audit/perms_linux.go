package config_audit

import (
	"fmt"
	"os"
)

// worldReadablePerm returns the file's permission octal as a string
// (e.g. "0644") and true if the "other" read bit is set. Returns "",
// false for missing/unreadable files or for paths that aren't regular
// files (we don't want to treat a 0755 directory as a leak).
func worldReadablePerm(path string) (string, bool) {
	st, err := os.Stat(path)
	if err != nil {
		return "", false
	}
	if !st.Mode().IsRegular() {
		return "", false
	}
	perm := st.Mode().Perm()
	return fmt.Sprintf("%#o", perm), perm&0004 != 0
}
