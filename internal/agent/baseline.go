package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// baselineFileName is the per-CA file that records the operator's
// system-plugin allowlist captured at first enroll. Lives next to the
// identity files (client.key / client.crt / project_ca.crt) inside the
// per-CA subdir so a re-enroll to a different server lands in a fresh
// directory and its own baseline.
const baselineFileName = "baseline.json"

// ErrBaselineNotFound is returned by LoadBaseline when no baseline
// file exists in the directory. Callers treat this as "first boot,
// fall back to whatever the install bundle / CLI flag says".
var ErrBaselineNotFound = errors.New("agent: baseline not found on disk")

// baselineFile is the on-disk shape. Plain JSON object so an operator
// inspecting the file by hand can audit what the agent decided to
// install. AppliedAt is the wall clock at write time; nothing reads
// it programmatically — it's there for forensic purposes.
type baselineFile struct {
	PluginIDs []string  `json:"plugin_ids"`
	AppliedAt time.Time `json:"applied_at"`
}

// SaveBaseline atomically writes the operator-chosen system-plugin
// allowlist into <dir>/baseline.json. Empty / nil ids is a valid
// state ("operator picked no plugins; agent runs with mandatory core
// only"); the file is still written so we don't keep re-evaluating
// the install bundle on every boot.
func SaveBaseline(dir string, ids []string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("agent: SaveBaseline mkdir %s: %w", dir, err)
	}
	data, err := json.MarshalIndent(baselineFile{
		PluginIDs: append([]string(nil), ids...),
		AppliedAt: time.Now().UTC(),
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("agent: SaveBaseline marshal: %w", err)
	}
	// Two-step write so a crash mid-write doesn't leave a half-formed
	// file: write to a sibling temp, then rename. Same trick the
	// identity files use, just inlined here because we don't need the
	// PEM-marshalling that SaveIdentity does.
	tmp := filepath.Join(dir, baselineFileName+".tmp")
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("agent: SaveBaseline write %s: %w", tmp, err)
	}
	final := filepath.Join(dir, baselineFileName)
	if err := os.Rename(tmp, final); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("agent: SaveBaseline rename: %w", err)
	}
	return nil
}

// LoadBaseline reads <dir>/baseline.json and returns the persisted
// allowlist. ErrBaselineNotFound if the file doesn't exist (typical on
// first boot before SaveBaseline has run); a wrapped JSON error if the
// file is malformed.
//
// A persisted-but-empty allowlist returns an empty slice (not nil) so
// callers can distinguish "operator explicitly picked the empty set"
// from "no baseline file present". The two cases behave the same
// downstream (both leave only the mandatory core installed) but the
// distinction matters for log lines.
func LoadBaseline(dir string) ([]string, error) {
	path := filepath.Join(dir, baselineFileName)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrBaselineNotFound
		}
		return nil, fmt.Errorf("agent: LoadBaseline read: %w", err)
	}
	var bf baselineFile
	if err := json.Unmarshal(raw, &bf); err != nil {
		return nil, fmt.Errorf("agent: LoadBaseline parse %s: %w", path, err)
	}
	if bf.PluginIDs == nil {
		// Distinguish from absent file via the empty slice; the agent
		// cares about presence/absence of the file, not nil-ness of
		// the slice inside it.
		return []string{}, nil
	}
	return bf.PluginIDs, nil
}
