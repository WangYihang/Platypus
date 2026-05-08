package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"github.com/charmbracelet/huh"

	agentplugin "github.com/WangYihang/Platypus/internal/agent/plugin"

	tmpls "github.com/WangYihang/Platypus/cmd/platypus-cli/templates"
)

// parseCapabilities lifts the kong []string into typed
// []CapabilityID, refusing unknown families before any file is
// written. The parser is the single place a typo or removed
// capability surfaces an error to the operator — every
// downstream consumer (the YAML renderer, the README list, the
// hint blocks) operates on the already-typed slice.
func parseCapabilities(in []string) ([]agentplugin.CapabilityID, error) {
	out := make([]agentplugin.CapabilityID, 0, len(in))
	for _, raw := range in {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		c, ok := agentplugin.ParseCapabilityID(raw)
		if !ok {
			valid := make([]string, len(agentplugin.AllCapabilityIDs))
			for i, c := range agentplugin.AllCapabilityIDs {
				valid[i] = string(c)
			}
			return nil, fmt.Errorf("unknown capability %q (valid: %s)", raw, strings.Join(valid, ", "))
		}
		out = append(out, c)
	}
	return out, nil
}

// pluginNewCmd scaffolds a fresh Rust plugin project. Two paths share
// the same renderer:
//
//   - non-interactive: every required field arrives via flags. Tests
//     and CI use this path. Skips huh entirely.
//   - interactive: missing flags trigger a huh form. The answered
//     struct then drives the same renderer.
type pluginNewCmd struct {
	Dir          string   `arg:"" optional:"" help:"Output directory. Default: ./<rightmost-id-segment>."`
	ID           string   `help:"Plugin id, reverse-DNS style (com.example.my-plugin)."`
	Name         string   `help:"Display name."`
	Version      string   `name:"plugin-version" default:"1.0.0" help:"Strict semver MAJOR.MINOR.PATCH for the new plugin."`
	AuthorName   string   `name:"author-name"`
	AuthorEmail  string   `name:"author-email"`
	License      string   `default:"Apache-2.0"`
	Description  string   `default:"A Platypus plugin scaffolded by platypus-cli."`
	Capabilities []string `help:"Capability families: log, kv, sysinfo, exec, fs.read, fs.write, net.http, process, net.dial, net.listen."`
	WithConfig   bool     `name:"with-config" default:"true" help:"Include a starter config: block."`
	Force        bool     `help:"Add files into a non-empty output directory."`
}

// templateContext is the single struct every template renders
// against. Pre-derives the fields the templates can't compute
// inline (CrateName from the id, EntryWasm filename, the rendered
// YAML / hint blocks) so each .tmpl stays declarative.
type templateContext struct {
	ID               string
	Name             string
	Version          string
	AuthorName       string
	AuthorEmail      string
	License          string
	Description      string
	WithConfig       bool
	CrateName        string // hyphenated lowercase, derived from the rightmost id segment
	EntryWasm        string // "<crate-name>.wasm" — matches the cargo crate output
	CapabilitiesYAML string
	CapabilityHints  string
	CapabilityList   string
}

// Run is the kong entry point. When required fields are missing
// AND stdin is a tty, drops into the interactive huh wizard. When
// stdin isn't a tty (CI / scripts) and required fields are
// missing, errors out — never silently picks defaults the author
// can't see.
func (c *pluginNewCmd) Run(_ *runContext) error {
	if c.needsWizard() {
		if !isInteractive() {
			return errors.New("required flags missing (--id, --name) and stdin is not a tty; pass them explicitly or run interactively")
		}
		if err := c.runWizard(); err != nil {
			return err
		}
	}
	caps, err := parseCapabilities(c.Capabilities)
	if err != nil {
		return err
	}
	if err := agentplugin.ValidatePluginID(c.ID); err != nil {
		return err
	}
	if err := agentplugin.ValidateSemver(c.Version); err != nil {
		return err
	}
	if strings.TrimSpace(c.Name) == "" {
		return errors.New("--name is required")
	}

	// Default output dir = rightmost id segment (cargo new-style).
	dir := c.Dir
	if dir == "" {
		dir = "./" + rightmostSegment(c.ID)
	}

	// Refuse to write into a non-empty directory unless --force.
	// Authors who pass an existing project dir on accident shouldn't
	// have files clobbered; --force makes the opt-in explicit.
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 && !c.Force {
		return fmt.Errorf("refusing to write into non-empty %q (pass --force to override)", dir)
	}

	ctx := c.buildContext(caps)
	return render(dir, ctx)
}

// buildContext fills the templateContext from the kong struct +
// the typed capabilities parsed in Run. Pure function: every input
// is on the receiver or passed in by the caller.
func (c *pluginNewCmd) buildContext(caps []agentplugin.CapabilityID) templateContext {
	crate := strings.ReplaceAll(rightmostSegment(c.ID), ".", "-")
	authorName := c.AuthorName
	if authorName == "" {
		authorName = "Anonymous"
	}
	authorEmail := c.AuthorEmail
	if authorEmail == "" {
		authorEmail = "you@example.com"
	}

	return templateContext{
		ID:               c.ID,
		Name:             c.Name,
		Version:          c.Version,
		AuthorName:       authorName,
		AuthorEmail:      authorEmail,
		License:          c.License,
		Description:      c.Description,
		WithConfig:       c.WithConfig,
		CrateName:        crate,
		EntryWasm:        crate + ".wasm",
		CapabilitiesYAML: renderCapabilitiesYAML(caps),
		CapabilityHints:  renderCapHints(caps),
		CapabilityList:   renderCapabilityList(caps),
	}
}

// render walks the embedded rust/ template tree and writes each file
// to the output directory with the .tmpl suffix stripped. Existing
// non-template files in dir are preserved (the --force gate already
// let the author through).
func render(dir string, ctx templateContext) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}

	const root = "rust"
	written := []string{}
	walkErr := fs.WalkDir(tmpls.FS, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		// Strip the language-root prefix so paths come out
		// relative to the output dir.
		rel := strings.TrimPrefix(path, root+"/")
		out := filepath.Join(dir, strings.TrimSuffix(rel, ".tmpl"))

		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", filepath.Dir(out), err)
		}
		raw, err := tmpls.FS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read template %s: %w", path, err)
		}
		// One-shot text/template per file. Template name doesn't
		// matter to the user; using the path keeps error messages
		// (which include the template name) clear.
		t, err := template.New(path).Option("missingkey=error").Parse(string(raw))
		if err != nil {
			return fmt.Errorf("parse template %s: %w", path, err)
		}
		f, err := os.Create(out)
		if err != nil {
			return fmt.Errorf("create %s: %w", out, err)
		}
		if err := t.Execute(f, ctx); err != nil {
			_ = f.Close()
			return fmt.Errorf("render %s: %w", out, err)
		}
		if err := f.Close(); err != nil {
			return err
		}
		written = append(written, strings.TrimSuffix(rel, ".tmpl"))
		return nil
	})
	if walkErr != nil {
		return walkErr
	}

	sort.Strings(written)
	printSummary(dir, ctx, written)
	return nil
}

// printSummary emits the post-scaffold success block. Plain text
// (no styling) so test output stays clean and the message works
// over a non-tty pipe; the interactive wizard gets its own coloured
// preamble.
func printSummary(dir string, ctx templateContext, written []string) {
	fmt.Printf("Scaffolded %s in %s\n\n", ctx.ID, dir)
	for _, f := range written {
		fmt.Printf("  %s\n", f)
	}
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Printf("  cd %s\n", dir)
	fmt.Println("  cargo build --release --target wasm32-unknown-unknown")
	fmt.Printf("  cp target/wasm32-unknown-unknown/release/%s.wasm %s\n", ctx.CrateName, ctx.EntryWasm)
	fmt.Println("  platypus-cli plugin keygen --out-secret ~/.platypus/publisher.secret \\")
	fmt.Println("                              --out-public ~/.platypus/publisher.pub")
	fmt.Printf("  platypus-cli plugin sign --key ~/.platypus/publisher.secret --wasm %s\n", ctx.EntryWasm)
	fmt.Println("  platypus-cli plugin validate-manifest plugin.yaml")
	fmt.Println()
	fmt.Println("See docs/plugins/AUTHORS.md for the deeper guide.")
}

// rightmostSegment plucks the rightmost dot-separated piece of a
// reverse-DNS id. "com.example.my-plugin" → "my-plugin".
func rightmostSegment(id string) string {
	i := strings.LastIndex(id, ".")
	if i < 0 {
		return id
	}
	return id[i+1:]
}

// gitConfigOrDefault returns `git config --get <key>` if available,
// else the supplied default. Used by the wizard to seed
// author-name / author-email fields with the operator's git
// identity, mirroring `cargo new`'s ergonomics.
func gitConfigOrDefault(key, def string) string {
	out, err := exec.Command("git", "config", "--get", key).Output()
	if err != nil {
		return def
	}
	v := strings.TrimSpace(string(out))
	if v == "" {
		return def
	}
	return v
}

// needsWizard reports whether the kong-supplied flags carry enough
// to scaffold without prompting. The minimum non-interactive
// trigger set is {ID, Name} — everything else has a sensible
// default (Version=1.0.0, License=Apache-2.0, etc.).
func (c *pluginNewCmd) needsWizard() bool {
	return c.ID == "" || c.Name == ""
}

// isInteractive reports whether stdin is a tty. Goes through the
// stat path so we don't pull in a tty lib for one boolean. The
// wizard refuses to run when this is false — CI must pass flags.
func isInteractive() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// runWizard mutates the receiver in-place with answers from a
// huh form. Each prompt is gated behind "field is empty" so an
// operator who supplied --id (but not --name, say) only gets
// prompted for the missing pieces.
func (c *pluginNewCmd) runWizard() error {
	// Defaults seeded from git config so the form already has
	// reasonable starting values for author-* and from the kong
	// defaults for License / Version / Description.
	if c.AuthorName == "" {
		c.AuthorName = gitConfigOrDefault("user.name", "Anonymous")
	}
	if c.AuthorEmail == "" {
		c.AuthorEmail = gitConfigOrDefault("user.email", "you@example.com")
	}
	if c.Version == "" {
		c.Version = "1.0.0"
	}
	if c.License == "" {
		c.License = "Apache-2.0"
	}

	// huh's MultiSelect needs the value type to match the option
	// type. We use string for capability families because it
	// flows directly into Capabilities []string with no
	// conversion.
	capOpts := []huh.Option[string]{
		huh.NewOption("log (implicit; always granted)", "log").Selected(true),
		huh.NewOption("kv — key-value store (plugin's own scope)", "kv"),
		huh.NewOption("sysinfo — read-only host snapshot", "sysinfo"),
		huh.NewOption("exec — run commands from an allowlist", "exec"),
		huh.NewOption("fs.read — read files from a path allowlist", "fs.read"),
		huh.NewOption("fs.write — write files in a path allowlist", "fs.write"),
		huh.NewOption("net.http — outbound HTTP to host allowlist", "net.http"),
		huh.NewOption("process — interactive PTY spawn", "process"),
		huh.NewOption("net.dial — outbound TCP to target allowlist", "net.dial"),
		huh.NewOption("net.listen — inbound TCP listen", "net.listen"),
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Plugin id").
				Description("Reverse-DNS, lowercase, hyphen-segments OK.").
				Placeholder("com.example.my-plugin").
				Validate(agentplugin.ValidatePluginID).
				Value(&c.ID),
			huh.NewInput().
				Title("Display name").
				Description("Human-readable; shown in the install dialog.").
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return errors.New("name is required")
					}
					return nil
				}).
				Value(&c.Name),
			huh.NewInput().
				Title("Version").
				Description("Strict semver MAJOR.MINOR.PATCH.").
				Validate(agentplugin.ValidateSemver).
				Value(&c.Version),
		),
		huh.NewGroup(
			huh.NewInput().
				Title("Author name").
				Value(&c.AuthorName),
			huh.NewInput().
				Title("Author email").
				Value(&c.AuthorEmail),
			huh.NewInput().
				Title("License").
				Description("SPDX id (Apache-2.0, MIT, BSD-3-Clause, ...).").
				Value(&c.License),
			huh.NewText().
				Title("Description").
				Description("One sentence about what the plugin does.").
				CharLimit(280).
				Value(&c.Description),
		),
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Capabilities to request").
				Description("The operator approves the requested set at install time. log is implicit.").
				Options(capOpts...).
				Value(&c.Capabilities),
		),
		huh.NewGroup(
			huh.NewConfirm().
				Title("Include a starter config: block?").
				Description("Adds the canonical greeting+shout schema (see docs/plugins/CONFIG_AUTHORING.md).").
				Affirmative("Yes").
				Negative("No").
				Value(&c.WithConfig),
		),
	)
	return form.Run()
}
