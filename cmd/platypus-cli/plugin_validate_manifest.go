package main

import (
	"fmt"
	"os"

	"github.com/WangYihang/Platypus/internal/agent/plugin"
)

// pluginValidateManifestCmd parses + validates a plugin.yaml without
// touching the agent or producing any artefacts. Authors run this
// during development to surface errors early; CI on the
// platypus-plugins index repo uses the same code path so authoring
// errors caught locally match what the index will refuse.
type pluginValidateManifestCmd struct {
	File string `arg:"" required:"" help:"Path to the plugin.yaml file to validate."`
}

func (c *pluginValidateManifestCmd) Run(_ *runContext) error {
	data, err := os.ReadFile(c.File)
	if err != nil {
		return fmt.Errorf("read manifest: %w", err)
	}
	m, err := plugin.ParseManifest(data)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "OK: %s (%s v%s)\n", c.File, m.ID, m.Version)
	caps := m.DeclaredCapabilities()
	if len(caps) > 0 {
		fmt.Fprintln(os.Stdout, "Declared capabilities:")
		for _, cap := range caps {
			fmt.Fprintf(os.Stdout, "  - %s\n", cap)
		}
	}
	return nil
}
