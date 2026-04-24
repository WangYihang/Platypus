package api

import (
	"context"
	"crypto/x509"

	"github.com/WangYihang/Platypus/internal/log"
	"github.com/WangYihang/Platypus/internal/storage"
)

// ProjectsCAPool returns the closure AgentLinkHandler uses to
// validate agent client certificates. The closure rebuilds the
// x509.CertPool on every call so that newly-created projects — and
// rotated CAs — become trust anchors without a server restart.
//
// Implementation detail: each invocation lists all projects and
// fetches each project's CA row. For Platypus's scale (tens of
// projects at most) this is cheap; if it ever stops being cheap,
// the right fix is a cache that invalidates on project / CA write.
func ProjectsCAPool(db *storage.DB) CertPoolFunc {
	return func() *x509.CertPool {
		pool := x509.NewCertPool()
		if db == nil {
			return pool
		}
		ctx := context.Background()
		projects, err := db.Projects().List(ctx)
		if err != nil {
			log.Warn("agent link: list projects for CA pool: %v", err)
			return pool
		}
		for _, p := range projects {
			ca, err := db.ProjectCA().Get(ctx, p.ID)
			if err != nil {
				// Normal for projects that haven't had a CA
				// initialised yet; only log at debug so startup
				// isn't noisy.
				log.Debug("agent link: no CA for project %s: %v", p.ID, err)
				continue
			}
			if ok := pool.AppendCertsFromPEM([]byte(ca.CertPEM)); !ok {
				log.Warn("agent link: failed to parse CA PEM for project %s", p.ID)
			}
		}
		return pool
	}
}
