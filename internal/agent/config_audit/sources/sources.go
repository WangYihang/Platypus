// Package sources is a tiny utility layer shared by the auditors:
// bounded file reads, home-directory enumeration, and a couple of fs
// helpers. Kept separate from the registry so auditor unit tests can
// import this without dragging the gitleaks dependency.
package sources

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ErrTooLarge is returned by ReadCapped when a target exceeds the cap.
// Callers typically log it and produce an "info" leak summarising
// "skipped, too large" rather than aborting the scan.
var ErrTooLarge = errors.New("config_audit: file exceeds size cap")

// ReadCapped reads up to max bytes from path. If the file is larger
// than max, it returns the prefix that was read AND ErrTooLarge so
// the caller can decide whether to use partial data.
//
// The cap protects the agent from being driven into OOM by a
// pathologically large config file (a 5 GB nginx access log that
// happened to be named .env, for instance).
func ReadCapped(path string, max int64) ([]byte, error) {
	if max <= 0 {
		return nil, fmt.Errorf("ReadCapped: invalid cap %d", max)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Stat first so we can short-circuit on huge files without reading.
	if st, err := f.Stat(); err == nil && st.Size() > max {
		buf := make([]byte, max)
		n, _ := io.ReadFull(f, buf)
		return buf[:n], ErrTooLarge
	}

	buf, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil {
		return buf, err
	}
	if int64(len(buf)) > max {
		return buf[:max], ErrTooLarge
	}
	return buf, nil
}

// HomeDirs returns the home directories worth auditing on a typical
// Linux box: /root plus every immediate child of /home that is itself
// a directory. We deliberately do not recurse — auditors are expected
// to look only inside the home, not under it.
//
// Containers and minimal images often only have /root; that's fine,
// the auditors get exactly one path back.
func HomeDirs() []string {
	var out []string
	if st, err := os.Stat("/root"); err == nil && st.IsDir() {
		out = append(out, "/root")
	}
	if entries, err := os.ReadDir("/home"); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			p := filepath.Join("/home", e.Name())
			if st, err := os.Stat(p); err == nil && st.IsDir() {
				out = append(out, p)
			}
		}
	}
	return out
}

// FileExists is a small ergonomic wrapper. Returns false on any error.
func FileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

// IsWorldReadable returns true if the regular-file permission mode
// allows reads from "other" (the last octal digit's read bit). We use
// it to flag credentials sitting in chmod 644 / 666 files.
func IsWorldReadable(mode fs.FileMode) bool {
	return mode.Perm()&0004 != 0
}

// LineByLine streams data into the callback once per line. It is
// strictly an iteration helper; auditors that already have a []byte
// from ReadCapped should use this rather than re-reading the file.
//
// The callback receives the 1-based line number and the line content
// (without trailing newline). Returning false halts iteration.
func LineByLine(data []byte, fn func(line int, text string) bool) {
	if len(data) == 0 {
		return
	}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20) // 1 MiB max line
	n := 0
	for scanner.Scan() {
		n++
		if !fn(n, scanner.Text()) {
			return
		}
	}
}
