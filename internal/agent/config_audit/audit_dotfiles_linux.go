package config_audit

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/WangYihang/Platypus/internal/agent/config_audit/sources"
)

func init() { Register(&dotfilesAuditor{}) }

// dotfilesAuditor scans the well-known credential-bearing dotfiles in
// every user's home: cloud SDK config (~/.aws/credentials,
// ~/.config/gcloud/...), package manager creds (~/.npmrc, ~/.pypirc),
// VCS creds (~/.git-credentials, ~/.netrc), container/kube
// (~/.docker/config.json, ~/.kube/config). These are usually
// chmod 600 by the tools that write them, but operators frequently
// loosen perms to share them across sessions or copy them into images.
type dotfilesAuditor struct{}

func (dotfilesAuditor) ID() string       { return "dotfiles.cloud_creds" }
func (dotfilesAuditor) Category() string { return "cloud" }

func (dotfilesAuditor) Metadata() AuditMetadata {
	return AuditMetadata{
		Title:       "Cloud and tool credential dotfiles",
		Description: "Scans the standard credential files written by AWS, gcloud, npm, pip, docker, kubectl, git, and curl/netrc. Flags credential-shaped values and reports world-readable permission modes.",
	}
}

func (dotfilesAuditor) Applicable(_ context.Context) bool {
	return len(sources.HomeDirs()) > 0
}

// Tuple of (relative path under home, max bytes). Files larger than
// the cap are skipped — the typical credential file is well under
// 64 KiB; anything bigger is almost certainly a different file that
// happened to land at this path.
var dotfileTargets = []struct {
	rel string
	max int64
}{
	{".aws/credentials", 64 * 1024},
	{".aws/config", 64 * 1024},
	{".config/gcloud/application_default_credentials.json", 64 * 1024},
	{".config/gcloud/legacy_credentials", 64 * 1024},
	{".azure/accessTokens.json", 256 * 1024},
	{".azure/azureProfile.json", 256 * 1024},
	{".kube/config", 256 * 1024},
	{".docker/config.json", 256 * 1024},
	{".npmrc", 64 * 1024},
	{".pypirc", 64 * 1024},
	{".netrc", 64 * 1024},
	{".git-credentials", 64 * 1024},
	{".gitconfig", 256 * 1024},
}

func (a dotfilesAuditor) Run(ctx context.Context) ([]Leak, error) {
	var leaks []Leak
	for _, home := range sources.HomeDirs() {
		if ctx.Err() != nil {
			break
		}
		for _, t := range dotfileTargets {
			path := filepath.Join(home, t.rel)
			if !sources.FileExists(path) {
				continue
			}
			data, err := sources.ReadCapped(path, t.max)
			if err != nil && !errors.Is(err, sources.ErrTooLarge) {
				continue
			}
			if len(data) == 0 {
				continue
			}
			if ls, err := ScanBytes(a.ID(), a.Category(), path, data); err == nil {
				leaks = append(leaks, ls...)
			}
			// Permission check: a credential file readable by anyone
			// other than the owner is itself a finding regardless of
			// content. AWS / gcloud / kube CLIs all default to 600 —
			// 644 means a human chmod'd it.
			if perm, ok := worldReadablePerm(path); ok {
				leaks = append(leaks, Leak{
					ID:            a.ID() + ".world_readable",
					Category:      a.Category(),
					Risk:          RiskMedium,
					Title:         "Credential file is world-readable",
					Location:      path,
					MatchRedacted: "mode=" + perm,
					Pattern:       "behavior:world-readable",
					Description:   "This credential file's permission mode allows any local user to read it. Tools like aws-cli and gcloud write these files chmod 600 by default; loosening that exposes the credentials to every process on the host.",
					Remediation:   "Restore restrictive permissions: `chmod 600 " + path + "`.",
				})
			}
		}
	}
	return leaks, nil
}
