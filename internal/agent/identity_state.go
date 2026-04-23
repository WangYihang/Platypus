package agent

import (
	"encoding/base32"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// MeshBootstrapState is the persisted local bootstrap material the agent can
// reuse across restarts to join the same mesh overlay before contacting the
// server again.
type MeshBootstrapState struct {
	PSKFile   string
	ProjectID string
	Peers     []string
}

type meshBootstrapMetadata struct {
	ProjectID string   `json:"project_id,omitempty"`
	Peers     []string `json:"peers,omitempty"`
}

// ResolveIdentityDir returns the effective persistent state directory.
func ResolveIdentityDir(identityDir string) string {
	if identityDir != "" {
		return identityDir
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ".platypus-agent"
	}
	return filepath.Join(home, ".platypus", "agent")
}

// MeshStateDir returns the subdirectory used for persisted mesh state.
func MeshStateDir(identityDir string) string {
	return filepath.Join(ResolveIdentityDir(identityDir), "mesh")
}

func meshPSKPath(identityDir string) string {
	return filepath.Join(MeshStateDir(identityDir), "psk")
}

func meshBootstrapMetadataPath(identityDir string) string {
	return filepath.Join(MeshStateDir(identityDir), "bootstrap.json")
}

// PersistMeshBootstrap stores mesh bootstrap material so future runs can bring
// up the overlay before talking to the server.
func PersistMeshBootstrap(identityDir string, psk []byte, projectID string, peers []string) error {
	meshDir := MeshStateDir(identityDir)
	if err := os.MkdirAll(meshDir, 0o700); err != nil {
		return err
	}
	if len(psk) > 0 {
		tmpPSK := meshPSKPath(identityDir) + ".tmp"
		encoded := base32.StdEncoding.EncodeToString(psk) + "\n"
		if err := os.WriteFile(tmpPSK, []byte(encoded), 0o600); err != nil {
			return err
		}
		if err := os.Rename(tmpPSK, meshPSKPath(identityDir)); err != nil {
			return err
		}
	}
	meta := meshBootstrapMetadata{ProjectID: projectID}
	if len(peers) > 0 {
		meta.Peers = append([]string(nil), peers...)
	}
	blob, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal mesh bootstrap metadata: %w", err)
	}
	tmpMeta := meshBootstrapMetadataPath(identityDir) + ".tmp"
	if err := os.WriteFile(tmpMeta, append(blob, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmpMeta, meshBootstrapMetadataPath(identityDir))
}

// LoadPersistedMeshBootstrap returns previously stored mesh bootstrap material.
// A nil result means no persisted mesh state was found.
func LoadPersistedMeshBootstrap(identityDir string) (*MeshBootstrapState, error) {
	pskPath := meshPSKPath(identityDir)
	if _, err := os.Stat(pskPath); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	state := &MeshBootstrapState{PSKFile: pskPath}
	metaPath := meshBootstrapMetadataPath(identityDir)
	blob, err := os.ReadFile(metaPath)
	if err == nil {
		var meta meshBootstrapMetadata
		if err := json.Unmarshal(blob, &meta); err != nil {
			return nil, fmt.Errorf("parse mesh bootstrap metadata: %w", err)
		}
		state.ProjectID = meta.ProjectID
		state.Peers = append([]string(nil), meta.Peers...)
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	return state, nil
}
