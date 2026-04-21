package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"net"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
)

// MachineID returns a stable, per-machine identifier computed once at first
// call and cached for the lifetime of the process. Preferred sources, in
// order:
//
//  1. Linux:   /etc/machine-id
//  2. macOS:   IOPlatformUUID via `ioreg -rd1 -c IOPlatformExpertDevice`
//  3. Windows: HKLM\SOFTWARE\Microsoft\Cryptography\MachineGuid via `reg`
//  4. Fallback: sha256(hostname + sorted MACs). Lossy — a container
//     recreated with a new MAC looks like a different host — but stable
//     enough for the merge-by-machine-id flow to degrade gracefully.
//
// The empty string is returned if nothing works; the server treats that
// as "use fingerprint fallback only" without prompting the user.
func MachineID() string {
	return machineIDOnce()
}

var (
	machineIDCache string
	machineIDMu    sync.Mutex
	machineIDDone  bool
)

func machineIDOnce() string {
	machineIDMu.Lock()
	defer machineIDMu.Unlock()
	if machineIDDone {
		return machineIDCache
	}
	machineIDCache = readMachineID()
	machineIDDone = true
	return machineIDCache
}

func readMachineID() string {
	switch runtime.GOOS {
	case "linux":
		if v := readFileTrimmed("/etc/machine-id"); v != "" {
			return v
		}
		// Some distros use /var/lib/dbus/machine-id.
		if v := readFileTrimmed("/var/lib/dbus/machine-id"); v != "" {
			return v
		}
	case "darwin":
		if v := readIOPlatformUUID(); v != "" {
			return v
		}
	case "windows":
		if v := readWindowsMachineGUID(); v != "" {
			return v
		}
	}
	return fallbackFingerprint()
}

// readFileTrimmed reads a file and returns its trimmed contents, or empty
// if anything fails. No propagation of errors — callers interpret empty
// as "unavailable".
func readFileTrimmed(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// ioregUUIDPattern picks the hex UUID out of `ioreg` output. Keeping it
// lenient — braces optional, case insensitive — because different macOS
// versions have formatted it differently in the past.
var ioregUUIDPattern = regexp.MustCompile(`"IOPlatformUUID"\s*=\s*"?([0-9A-Fa-f-]+)"?`)

func readIOPlatformUUID() string {
	out, err := exec.Command("ioreg", "-rd1", "-c", "IOPlatformExpertDevice").Output()
	if err != nil {
		return ""
	}
	m := ioregUUIDPattern.FindSubmatch(out)
	if len(m) < 2 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(string(m[1])))
}

// machineGUIDPattern parses a line like:
//
//	MachineGuid    REG_SZ    12345678-abcd-...
//
// from the Windows `reg query` output.
var machineGUIDPattern = regexp.MustCompile(`MachineGuid\s+REG_SZ\s+([0-9A-Fa-f-]+)`)

func readWindowsMachineGUID() string {
	out, err := exec.Command("reg", "query",
		`HKLM\SOFTWARE\Microsoft\Cryptography`, "/v", "MachineGuid").Output()
	if err != nil {
		return ""
	}
	m := machineGUIDPattern.FindSubmatch(out)
	if len(m) < 2 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(string(m[1])))
}

// fallbackFingerprint returns a deterministic sha256 of hostname + sorted
// MAC addresses. Prefixed with "fp-" so the server can tell at a glance
// that this is the weaker identifier.
func fallbackFingerprint() string {
	hostname, _ := os.Hostname()

	macs := []string{}
	if ifs, err := net.Interfaces(); err == nil {
		for _, i := range ifs {
			if i.HardwareAddr != nil && len(i.HardwareAddr) > 0 {
				macs = append(macs, i.HardwareAddr.String())
			}
		}
	}
	sort.Strings(macs)

	h := sha256.New()
	h.Write([]byte(hostname))
	h.Write([]byte{0})
	for _, m := range macs {
		h.Write([]byte(m))
		h.Write([]byte{0})
	}
	return "fp-" + hex.EncodeToString(h.Sum(nil))
}
