package plugin

import (
	"context"
	"sync"

	"github.com/WangYihang/Platypus/internal/log"
)

// Registry is the agent's process-wide plugin runtime. One instance is
// constructed at agent startup, wired into AgentRPCHandlers.PluginCall
// and AgentHandlerDeps.PluginMgmt, and lives until the agent exits.
//
// Concurrency model: Registry's RWMutex protects only the plugins map
// and the catalog handle. Per-plugin invocation serialisation lives on
// the loaded.mu inside each *loaded — extism.Plugin is documented as
// not goroutine-safe, so two server-driven RPCs targeting the same
// plugin id run sequentially. RPCs against different plugin ids run
// in parallel.
//
// Method placement across files:
//   registry.go              this file: ctor, lifecycle, types
//   registry_invoke.go       Invoke + audit + exportDeclared
//   registry_lifecycle.go    List, Tail, SetEnabled, Remove
//   mgmt_stream.go           HandleMgmt + per-op write helpers
//   install.go               handleInstall (entry point)
//   install_receive.go       receive + chunk parsing + sha256
//   install_persist.go       persistInstall + hotLoad
type Registry struct {
	paths   Paths
	catalog *Catalog
	resolve PublisherResolver
	options LoadOptions
	auditor Auditor

	mu      sync.RWMutex
	plugins map[string]*loaded
}

// Auditor is the hook for emitting per-call audit records into the
// agent's existing structured-log stream. The default
// (DefaultAuditor) goes to log.L; tests inject a fake to assert that
// fields are populated correctly.
type Auditor func(record AuditRecord)

// AuditRecord is one per-invocation row. Populated by Registry.audit
// and handed to the configured Auditor.
type AuditRecord struct {
	PluginID            string
	Method              string
	CorrelationID       string
	GrantedCapabilities []string
	FuelUsed            uint64
	MemPeakBytes        uint64
	ElapsedMS           int64
	Error               string
}

// DefaultAuditor emits AuditRecord as a single structured log line on
// log.L.
func DefaultAuditor(rec AuditRecord) {
	log.L.Info("plugin.invoke",
		"plugin_id", rec.PluginID,
		"method", rec.Method,
		"correlation_id", rec.CorrelationID,
		"granted_capabilities", rec.GrantedCapabilities,
		"fuel_used", rec.FuelUsed,
		"mem_peak_bytes", rec.MemPeakBytes,
		"elapsed_ms", rec.ElapsedMS,
		"error", rec.Error,
	)
}

// Options configures the Registry. Zero-value fields fall back to
// sensible defaults inside New (PublisherResolver →
// FilesystemPublisherResolver(Paths); Auditor → DefaultAuditor).
type Options struct {
	Paths    Paths
	Resolver PublisherResolver
	Load     LoadOptions
	Auditor  Auditor
}

// New constructs an empty Registry. Call Load(ctx) to populate it from
// disk; tests that don't need disk state can skip Load and inject
// plugins directly via the package-internal map.
func New(opts Options) (*Registry, error) {
	if opts.Resolver == nil {
		opts.Resolver = FilesystemPublisherResolver(opts.Paths)
	}
	if opts.Auditor == nil {
		opts.Auditor = DefaultAuditor
	}
	cat, err := LoadCatalog(opts.Paths.CatalogFile())
	if err != nil {
		return nil, err
	}
	return &Registry{
		paths:   opts.Paths,
		catalog: cat,
		resolve: opts.Resolver,
		options: opts.Load,
		auditor: opts.Auditor,
		plugins: map[string]*loaded{},
	}, nil
}

// Load discovers and loads every plugin recorded in the catalog. Errors
// for individual plugins are logged and skipped; a single broken
// plugin must not prevent the agent from booting.
func (r *Registry) Load(ctx context.Context) {
	loaded := LoadAll(ctx, r.paths, r.catalog, r.resolve, r.options)
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, l := range loaded {
		r.plugins[l.id] = l
	}
}

// Close releases every running wasm instance. Best-effort; errors are
// logged. Used by the agent's reconnect / shutdown paths.
func (r *Registry) Close(ctx context.Context) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, l := range r.plugins {
		l.close(ctx)
	}
	r.plugins = map[string]*loaded{}
}
