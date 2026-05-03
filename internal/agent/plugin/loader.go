package plugin

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	extism "github.com/extism/go-sdk"
	"github.com/jedisct1/go-minisign"

	"github.com/WangYihang/Platypus/internal/log"
)

// loaded is the live representation of one installed plugin: its
// manifest, granted capabilities, the constructed extism.Plugin (lazy
// — instantiated on first call), and the log buffer host_log writes
// into.
//
// instance is loaded behind a sync.Once to keep cold-start cheap. The
// trade-off: the first invocation of any method on a freshly-installed
// plugin pays the wazero compile cost. For chatty plugins that's
// amortised; for one-shot plugins it's <50ms.
type loaded struct {
	id        string
	manifest  *Manifest
	entry     CatalogEntry
	pubKey    minisign.PublicKey
	stateDir  string
	wasmPath  string // resolved at load time, never changes
	sigPath   string
	logs      *logBuffer

	pctx        *pluginCtx // built once, reused by every instance
	currentCorr atomic.Pointer[string]

	mu       sync.Mutex
	instance *extism.Plugin
}

// instanceOf returns the (cached) extism plugin for `l`. The first
// caller pays the build/compile cost; subsequent callers share the
// instance. extism.Plugin is documented as not goroutine-safe, so the
// caller must hold l.mu while calling p.Call() — Invoke does this.
func (l *loaded) instanceOf(ctx context.Context) (*extism.Plugin, error) {
	if l.instance != nil {
		return l.instance, nil
	}

	// Re-verify the on-disk binary. Cheap (<1ms for a typical 1MB
	// .wasm) and catches tamper-after-install: a host operator who
	// trusts the agent must also trust that nothing edited the .wasm
	// out from under it between installs.
	if err := VerifyWasmFile(l.pubKey, l.wasmPath, l.sigPath); err != nil {
		return nil, fmt.Errorf("plugin: re-verify on load: %w", err)
	}
	wasm, err := os.ReadFile(l.wasmPath)
	if err != nil {
		return nil, fmt.Errorf("plugin: read wasm: %w", err)
	}

	// Build the manifest as extism understands it. We deliberately do
	// NOT pass AllowedHosts / AllowedPaths — every IO goes through our
	// host functions so the capability check is centralised in
	// host_funcs.go rather than split across two enforcement layers.
	emanifest := extism.Manifest{
		Wasm: []extism.Wasm{extism.WasmData{Data: wasm, Name: l.id}},
		Memory: &extism.ManifestMemory{
			MaxPages: maxPagesForMB(l.manifest.Resources.MaxMemoryMB),
		},
		Timeout: l.manifest.Resources.MaxInvocationMS,
	}

	// EnableWasi=true gives us extism's stdlib (printf via WASI is
	// useful for plugin development) but leaves filesystem / sockets
	// inaccessible because AllowedPaths/AllowedHosts are empty.
	cfg := extism.PluginConfig{
		EnableWasi: true,
	}

	inst, err := extism.NewPlugin(ctx, emanifest, cfg, l.pctx.buildHostFunctions())
	if err != nil {
		return nil, fmt.Errorf("plugin: instantiate: %w", err)
	}
	inst.SetLogger(func(level extism.LogLevel, msg string) {
		// Anything extism's own runtime emits (panic recovery, OOM,
		// etc.) lands in the host log under the plugin id so an
		// operator can grep for it.
		log.L.Info("plugin.runtime", "plugin_id", l.id, "level", level.String(), "msg", msg)
	})
	l.instance = inst
	return inst, nil
}

// close releases the wazero runtime backing this plugin. Idempotent.
// Also reaps any orphaned child processes — a plugin that crashed
// mid-host_process_relay would otherwise leak a long-lived child
// owned by the now-defunct wasm instance.
func (l *loaded) close(ctx context.Context) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.pctx != nil {
		l.pctx.reapProcessHandles()
	}
	if l.instance != nil {
		_ = l.instance.Close(ctx)
		l.instance = nil
	}
}

// LoadAll discovers every installed plugin under paths.InstalledDir
// and returns the constructed loaded set. Errors for individual
// plugins are logged and skipped — a single broken plugin must not
// prevent the agent from booting.
func LoadAll(ctx context.Context, paths Paths, cat *Catalog, pubResolver PublisherResolver, opts LoadOptions) []*loaded {
	out := make([]*loaded, 0, 8)
	for _, e := range cat.All() {
		l, err := loadOne(ctx, paths, e, pubResolver, opts)
		if err != nil {
			log.L.Warn("plugin.load.failed", "plugin_id", e.ID, "version", e.Version, "error", err.Error())
			continue
		}
		out = append(out, l)
		log.L.Info("plugin.load.ok", "plugin_id", e.ID, "version", e.Version,
			"granted_capabilities", e.GrantedCapabilities)
	}
	return out
}

// LoadOptions tunes per-plugin runtime caps. Sensible defaults are
// applied via fillDefaults so callers can pass zero-value LoadOptions{}.
type LoadOptions struct {
	MaxFileReadBytes int64
	MaxKVValueBytes  int64
	LogBufferLines   int
	Now              func() time.Time
}

func (o *LoadOptions) fillDefaults() {
	if o.MaxFileReadBytes <= 0 {
		o.MaxFileReadBytes = 4 * 1024 * 1024
	}
	if o.MaxKVValueBytes <= 0 {
		o.MaxKVValueBytes = 256 * 1024
	}
	if o.LogBufferLines <= 0 {
		o.LogBufferLines = 256
	}
	if o.Now == nil {
		o.Now = time.Now
	}
}

// PublisherResolver returns the trusted minisign public key for the
// given key id. The agent backs this with the publishers/ directory;
// tests can inject an in-memory map.
type PublisherResolver func(keyID string) (minisign.PublicKey, error)

// FilesystemPublisherResolver loads pubkeys from <paths>/publishers/.
// Returns os.ErrNotExist when the key file is absent so callers can
// distinguish "no such trusted publisher" from "I/O failure".
func FilesystemPublisherResolver(paths Paths) PublisherResolver {
	return func(keyID string) (minisign.PublicKey, error) {
		pk, _, err := LoadPublicKey(paths.PublisherKeyFile(keyID))
		if errors.Is(err, os.ErrNotExist) {
			return minisign.PublicKey{}, os.ErrNotExist
		}
		return pk, err
	}
}

func loadOne(ctx context.Context, paths Paths, e CatalogEntry, resolve PublisherResolver, opts LoadOptions) (*loaded, error) {
	opts.fillDefaults()

	manifestBytes, err := os.ReadFile(paths.ManifestFile(e.ID, e.Version))
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	m, err := ParseManifest(manifestBytes)
	if err != nil {
		return nil, err
	}
	pk, err := resolve(e.PublisherKeyID)
	if err != nil {
		return nil, fmt.Errorf("resolve publisher %q: %w", e.PublisherKeyID, err)
	}

	wasmPath := paths.WasmFile(e.ID, e.Version, m.Runtime.Entry)
	sigPath := paths.SignatureFile(e.ID, e.Version, m.Runtime.Entry)
	if err := VerifyWasmFile(pk, wasmPath, sigPath); err != nil {
		return nil, err
	}

	stateDir := paths.StateDir(e.ID)
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return nil, fmt.Errorf("mkdir state: %w", err)
	}

	granted := map[CapabilityID]bool{}
	for _, g := range e.GrantedCapabilities {
		granted[CapabilityID(g)] = true
	}
	granted[CapLog] = true // always

	l := &loaded{
		id:       e.ID,
		manifest: m,
		entry:    e,
		pubKey:   pk,
		stateDir: stateDir,
		wasmPath: wasmPath,
		sigPath:  sigPath,
		logs:     newLogBuffer(opts.LogBufferLines),
	}
	emptyCorr := ""
	l.currentCorr.Store(&emptyCorr)
	l.pctx = &pluginCtx{
		id:       e.ID,
		manifest: m,
		granted:  granted,
		stateDir: stateDir,
		logSink:  l.logs,
		now:      opts.Now,
		correlationID: func() string {
			v := l.currentCorr.Load()
			if v == nil {
				return ""
			}
			return *v
		},
		maxFileReadSize: opts.MaxFileReadBytes,
		maxKVValueSize:  opts.MaxKVValueBytes,
	}
	return l, nil
}

// maxPagesForMB converts the manifest's max_memory_mb to wazero memory
// pages (1 page = 64 KiB). Capped at uint32 max; the manifest validator
// already rejects values above 1024 MiB so the cap is just a safety
// net.
func maxPagesForMB(mb uint32) uint32 {
	const bytesPerPage = 64 * 1024
	pages := uint64(mb) * 1024 * 1024 / bytesPerPage
	if pages > 0xFFFFFFFF {
		return 0xFFFFFFFF
	}
	return uint32(pages)
}
