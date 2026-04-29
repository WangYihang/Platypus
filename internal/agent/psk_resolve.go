package agent

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// systemPSKPath is the package-level fallback consulted last in the
// PSK resolution chain. Mirrors the convention `/etc/<service>/<file>`
// that systemd / sysv units have used for decades, so an admin who
// dropped the PSK there via Ansible (or a service unit's
// LoadCredential=) doesn't have to thread anything through the
// binary's invocation.
const systemPSKPath = "/etc/platypus/mesh.psk"

// PSKFileName is the basename mesh.psk lives under inside the
// data-dir. Exposed so tests / install scripts can refer to one
// constant.
const PSKFileName = "mesh.psk"

// PSKResolveOptions bundles the four input layers ResolvePSKFile
// considers. CLIPath / EnvFile / EnvInline / DataDir get filled by
// the agent main.go from flags + env + the resolved data dir.
type PSKResolveOptions struct {
	CLIPath   string // --psk-file flag
	EnvFile   string // PLATYPUS_MESH_PSK_FILE
	EnvInline string // PLATYPUS_MESH_PSK (raw base32 / hex contents)
	DataDir   string // <data-dir>/mesh.psk default

	// EphemeralDir is where the inline PSK gets materialised when
	// EnvInline wins resolution. Defaults to os.TempDir(); tests
	// override it for isolation.
	EphemeralDir string
}

// ResolvePSKFile walks the documented resolution chain (see
// pkg/options.PSKResolutionOrder) and returns the absolute path to
// the file the agent should read at startup. Returns "" without an
// error when no PSK is configured anywhere — that's a valid state
// (mesh participation is then disabled) and the caller decides
// whether to surface a hint.
//
// When EnvInline supplies the secret directly we materialise it to
// a 0600 file under EphemeralDir so the rest of the agent (and the
// mesh handshake codec, which expects bytes from disk) doesn't need
// to special-case "PSK lives in memory".
func ResolvePSKFile(in PSKResolveOptions) (string, error) {
	if in.CLIPath != "" {
		return in.CLIPath, nil
	}
	if in.EnvFile != "" {
		return in.EnvFile, nil
	}
	if in.EnvInline != "" {
		path, err := materialiseInlinePSK(in.EnvInline, in.EphemeralDir)
		if err != nil {
			return "", fmt.Errorf("materialise inline PSK: %w", err)
		}
		return path, nil
	}
	if in.DataDir != "" {
		candidate := filepath.Join(in.DataDir, PSKFileName)
		if fileExists(candidate) {
			return candidate, nil
		}
	}
	if fileExists(systemPSKPath) {
		return systemPSKPath, nil
	}
	return "", nil
}

// InstallPSK writes the supplied PSK string to the given target path
// with 0600 permissions, creating parent dirs as needed. The string
// is normalised first: surrounding whitespace stripped, internal
// whitespace collapsed (so a paste-from-Slack with trailing newlines
// or wrapped lines lands as one tidy token).
//
// Empty input is a parse error — accidentally clearing the PSK by
// piping an empty buffer would be very surprising. Use os.Remove +
// re-install if rotation is intended.
func InstallPSK(target, raw string) error {
	cleaned := strings.Join(strings.Fields(raw), "")
	if cleaned == "" {
		return errors.New("agent: InstallPSK: empty PSK input")
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return fmt.Errorf("create psk dir: %w", err)
	}
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, []byte(cleaned+"\n"), 0o600); err != nil {
		return fmt.Errorf("write psk: %w", err)
	}
	if err := os.Rename(tmp, target); err != nil {
		return fmt.Errorf("rename psk: %w", err)
	}
	return nil
}

// DefaultPSKTarget is where `psk install <psk>` writes when the user
// didn't override --psk-file or --data-dir. Mirrors the resolution
// chain: agents under the same data-dir auto-discover the file we
// just wrote.
func DefaultPSKTarget(dataDir string) string {
	return filepath.Join(dataDir, PSKFileName)
}

// materialiseInlinePSK writes the inline contents to a tmp file under
// dir (defaulting to os.TempDir()) and returns its path. The file is
// world-unreadable (0600) and is not auto-removed — the agent
// process owns the file for its lifetime.
func materialiseInlinePSK(contents, dir string) (string, error) {
	if dir == "" {
		dir = os.TempDir()
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(dir, "platypus-mesh-psk-*.psk")
	if err != nil {
		return "", err
	}
	if _, err := f.WriteString(strings.TrimSpace(contents) + "\n"); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return "", err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(f.Name())
		return "", err
	}
	if err := os.Chmod(f.Name(), 0o600); err != nil {
		return "", err
	}
	return f.Name(), nil
}

func fileExists(p string) bool {
	if p == "" {
		return false
	}
	_, err := os.Stat(p)
	return err == nil
}
