package config_audit

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/WangYihang/Platypus/internal/agent/config_audit/sources"
)

func init() { Register(&webAppAuditor{}) }

// webAppAuditor walks a small set of well-known web-app deployment
// roots looking for config files framework conventions tell us will
// hold credentials. The walk is bounded on three axes — the prefix
// list itself, max depth, and a hard ceiling on the number of files
// fed to the detector — so this never degrades into a full-disk
// scan on a host with a noisy filesystem.
type webAppAuditor struct{}

func (webAppAuditor) ID() string       { return "webapp.config" }
func (webAppAuditor) Category() string { return "webapp" }

func (webAppAuditor) Metadata() AuditMetadata {
	return AuditMetadata{
		Title:       "Web application configuration files",
		Description: "Walks /var/www, /srv, /opt, and each user home for known framework config files (.env, wp-config.php, settings.py, application.yml, appsettings.json, database.yml, secrets.yml). Each candidate is scanned for credential-shaped strings.",
	}
}

func (webAppAuditor) Applicable(_ context.Context) bool { return true }

// Roots we walk. We deliberately do NOT walk / — too noisy and slow.
var webAppRoots = []string{
	"/var/www",
	"/srv",
	"/opt",
}

// Configuration filenames worth scanning. Lower-cased; comparison is
// case-insensitive.
var webAppFilenames = map[string]struct{}{
	".env":                   {},
	".env.local":             {},
	".env.production":        {},
	".env.staging":           {},
	"wp-config.php":          {},
	"settings.py":            {},
	"local_settings.py":      {},
	"application.yml":        {},
	"application.yaml":       {},
	"application.properties": {},
	"appsettings.json":       {},
	"config.php":             {},
	"database.yml":           {},
	"secrets.yml":            {},
	"credentials.yml.enc":    {}, // rails encrypted creds; might still contain dev creds
}

const (
	webAppMaxDepth = 4
	webAppMaxFiles = 200
	webAppMaxBytes = 1 * 1024 * 1024 // 1 MiB per file
)

func (a webAppAuditor) Run(ctx context.Context) ([]Leak, error) {
	var (
		leaks   []Leak
		visited int
	)
	roots := append([]string{}, webAppRoots...)
	// First-level subdirectories of each home count as web-app
	// candidates too — `/home/alice/myapp/.env` is a common shape.
	for _, h := range sources.HomeDirs() {
		roots = append(roots, h)
	}

	for _, root := range roots {
		if ctx.Err() != nil || visited >= webAppMaxFiles {
			break
		}
		st, err := os.Stat(root)
		if err != nil || !st.IsDir() {
			continue
		}
		baseDepth := strings.Count(filepath.Clean(root), string(filepath.Separator))
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, werr error) error {
			if werr != nil {
				return nil // tolerate permission errors mid-walk
			}
			if ctx.Err() != nil || visited >= webAppMaxFiles {
				return filepath.SkipAll
			}
			depth := strings.Count(path, string(filepath.Separator)) - baseDepth
			if depth > webAppMaxDepth {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if d.IsDir() {
				// Skip a few obvious noise directories.
				name := d.Name()
				switch name {
				case "node_modules", "vendor", ".git", ".cache", "dist", "build":
					return filepath.SkipDir
				}
				return nil
			}
			lower := strings.ToLower(d.Name())
			if _, ok := webAppFilenames[lower]; !ok {
				// `appsettings.<env>.json` is common in .NET; admit
				// it via prefix so we don't have to enumerate every
				// possible environment name.
				if !(strings.HasPrefix(lower, "appsettings.") && strings.HasSuffix(lower, ".json")) {
					return nil
				}
			}
			visited++
			data, err := sources.ReadCapped(path, webAppMaxBytes)
			if err != nil && !errors.Is(err, sources.ErrTooLarge) {
				return nil
			}
			if len(data) == 0 {
				return nil
			}
			ls, _ := ScanBytes(a.ID(), a.Category(), path, data)
			leaks = append(leaks, ls...)
			if perm, ok := worldReadablePerm(path); ok {
				leaks = append(leaks, Leak{
					ID:            a.ID() + ".world_readable",
					Category:      a.Category(),
					Risk:          RiskLow,
					Title:         "Web app config file is world-readable",
					Location:      path,
					MatchRedacted: "mode=" + perm,
					Pattern:       "behavior:world-readable",
					Description:   "This config file's permission mode allows any local user to read it. If it contains secrets the blast radius is the entire host, not just the web-app's own UID.",
					Remediation:   "Tighten with `chmod 640 " + path + "` (or 600) and ensure it's owned by the web-app user.",
				})
			}
			return nil
		})
	}
	return leaks, nil
}
