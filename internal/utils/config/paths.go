package config

import (
	"os"
	"path/filepath"
)

// joinPath is a thin wrapper around filepath.Join that exists to
// keep the Options method bodies one line each — they otherwise
// invite drift (one call site uses filepath.Join, another raw
// string concatenation, etc.).
func joinPath(dir, name string) string {
	return filepath.Join(dir, name)
}

// readFile is a test seam around os.ReadFile so unit tests can
// override secret-file loading without touching the filesystem.
// Production code calls this through the package-private wrapper so
// the tag-based "type:existingfile" kong validator still runs first
// (it validates the path exists at parse time; readFile then
// actually reads it during PostParse).
var readFile = os.ReadFile
