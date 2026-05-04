package sysplugins

import (
	"os"
	"path/filepath"
)

// writeFile is a test helper that mkdirs the parent and writes
// content to path. Kept here so the test file stays focused on
// assertions rather than fs scaffolding.
func writeFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}
