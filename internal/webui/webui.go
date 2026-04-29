// Package webui serves the embedded React frontend bundle at the root
// of the platypus-server gin engine. The bundle is produced by
// `pnpm run build:web` (under desktop/frontend/) and staged into
// internal/webui/dist/ by `make web-ui-embed` before `go build`. A
// committed stub index.html lives in dist/ so a fresh checkout still
// compiles without a Node toolchain — the stub renders a "UI not
// embedded" message that points contributors at the right Make target.
//
// Routing layout (Gin matches explicit routes first, NoRoute is the
// last-resort fallback):
//
//	GET /                     → dist/index.html (no-cache)
//	GET /favicon.ico          → dist/favicon.ico
//	GET /assets/*filepath     → dist/assets/...    (immutable, 1y cache)
//	NoRoute                   → JSON 404 under /api/, /swagger/, etc.;
//	                            otherwise try a real top-level file
//	                            (manifest.webmanifest, robots.txt) and
//	                            finally fall back to index.html so
//	                            React Router deep links survive refresh.
package webui

import (
	"embed"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"

	"github.com/gin-gonic/gin"
)

//go:embed all:dist
var distFS embed.FS

// apiPrefixes lists the path prefixes owned by the API/WebSocket layer.
// A NoRoute miss under any of these returns a JSON 404 so curl clients
// don't get an HTML body for a missing /api/v1/foo. Keep this list in
// sync with the route registrations in cmd/platypus-server/main.go.
var apiPrefixes = []string{
	"/api/",
	"/swagger/",
	"/v1/manifest/",
	"/notify",
	"/ws/",
	"/install/",
}

// RegisterRoutes wires the embedded frontend onto engine. Must be
// called AFTER all API route registrations so explicit API paths win
// first-match and only true misses hit the SPA fallback.
func RegisterRoutes(engine *gin.Engine) {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		// Build-time invariant: //go:embed all:dist guarantees the dist
		// directory and at least the committed stub exist. A failure
		// here means the package was built in a way that broke the
		// embed (e.g. dist/ was deleted between codegen and `go build`).
		panic(err)
	}

	// vite emits content-hashed filenames under /assets/, so a long
	// immutable cache is safe — a new build produces new hashes and
	// the index.html (which references them, served with no-cache)
	// updates the manifest in lockstep.
	engine.GET("/assets/*filepath", func(c *gin.Context) {
		c.Header("Cache-Control", "public, max-age=31536000, immutable")
		serveFile(c, sub, "assets"+c.Param("filepath"))
	})

	engine.GET("/", spaIndex(sub))
	engine.GET("/favicon.ico", func(c *gin.Context) {
		serveFile(c, sub, "favicon.ico")
	})

	engine.NoRoute(func(c *gin.Context) {
		p := c.Request.URL.Path
		for _, pre := range apiPrefixes {
			if strings.HasPrefix(p, pre) {
				c.JSON(http.StatusNotFound, gin.H{"error": "not found", "path": p})
				return
			}
		}
		// Top-level real files first (manifest.webmanifest, robots.txt,
		// etc.) so PWA-style assets keep working; React Router deep
		// links fall through to index.html.
		clean := strings.TrimPrefix(path.Clean("/"+p), "/")
		if clean != "" && clean != "." {
			if data, err := fs.ReadFile(sub, clean); err == nil {
				ct := mime.TypeByExtension(path.Ext(clean))
				if ct == "" {
					ct = "application/octet-stream"
				}
				c.Data(http.StatusOK, ct, data)
				return
			}
		}
		spaIndex(sub)(c)
	})
}

func spaIndex(sub fs.FS) gin.HandlerFunc {
	return func(c *gin.Context) {
		data, err := fs.ReadFile(sub, "index.html")
		if err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.Header("Cache-Control", "no-cache")
		c.Data(http.StatusOK, "text/html; charset=utf-8", data)
	}
}

func serveFile(c *gin.Context, sub fs.FS, p string) {
	data, err := fs.ReadFile(sub, strings.TrimPrefix(p, "/"))
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	ct := mime.TypeByExtension(path.Ext(p))
	if ct == "" {
		ct = "application/octet-stream"
	}
	c.Data(http.StatusOK, ct, data)
}
