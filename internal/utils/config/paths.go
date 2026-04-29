package config

import "path/filepath"

// joinPath is a thin wrapper around filepath.Join that exists to
// keep the Options method bodies one line each — they otherwise
// invite drift (one call site uses filepath.Join, another raw
// string concatenation, etc.).
func joinPath(dir, name string) string {
	return filepath.Join(dir, name)
}
