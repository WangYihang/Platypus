package config

// IngressConfig binds the unified TLS ingress the server accepts all
// agent, mesh, and admin HTTPS traffic on. The ALPN dispatcher fans
// the accepted connections out to the relevant handlers:
//
//	ptps-agent → core.AgentService.Handle
//	ptps-mesh  → mesh.Node.AcceptRaw
//	h2/http    → gin via ingress.Dispatcher.HTTPListener()
//
// PublicAddr is the reachable host:port the installer template and
// mesh auto-bootstrap advertise to remote agents. When empty it
// falls back to Addr.
type IngressConfig struct {
	Addr       string `yaml:"addr"        mapstructure:"addr"`
	Cert       string `yaml:"cert"        mapstructure:"cert"`
	Key        string `yaml:"key"         mapstructure:"key"`
	PublicAddr string `yaml:"public_addr" mapstructure:"public_addr"`
}

// PublicAddrOrAddr returns PublicAddr when set, otherwise Addr. This
// is the host:port template callers should hand to the installer,
// enrollment-response mesh peers, and /api/v1/info.
func (c IngressConfig) PublicAddrOrAddr() string {
	if c.PublicAddr != "" {
		return c.PublicAddr
	}
	return c.Addr
}

// RESTfulConfig configures the storage knobs of the REST surface.
// The REST engine itself is mounted onto the unified ingress
// dispatcher's virtual HTTP listener — there is no per-protocol
// enable flag because v2 has no other control-plane mode.
//
// JWT-related fields (jwt_access_key / jwt_refresh_key /
// access_expire_time / refresh_expire_time) were retired alongside
// Phase-2 auth. Operators who still have them in their YAML can
// leave them — viper ignores unknown keys.
type RESTfulConfig struct {
	DBFile string `yaml:"db_file" mapstructure:"db_file"` // empty defaults to ./platypus.db
}

// DBFileOrDefault returns the configured SQLite path, or "./platypus.db"
// when unset. Unix paths only for now.
//
// TTL defaults moved to internal/settings so the runtime override layer
// and the YAML defaults share a single source of truth; legacy
// AccessTTLOrDefault / RefreshTTLOrDefault helpers were dropped.
func (c RESTfulConfig) DBFileOrDefault() string {
	if c.DBFile != "" {
		return c.DBFile
	}
	return "./platypus.db"
}

// DistributorConfig defines the routes that front the agent release
// artifact store. The HTTP routes are mounted on the unified ingress;
// no dedicated port. The public base URL is derived from
// IngressConfig.PublicAddrOrAddr at runtime — operators don't need
// to repeat it here.
type DistributorConfig struct {
	Channel      string              `yaml:"channel"       mapstructure:"channel"`       // default release channel; "stable" if empty
	PresignedTTL string              `yaml:"presigned_ttl" mapstructure:"presigned_ttl"` // duration, e.g. "5m"; defaults to 5 minutes
	Store        ArtifactStoreConfig `yaml:"store"         mapstructure:"store"`
}

// ArtifactStoreConfig is the S3/MinIO backend for the agent release
// artifacts and manifest.
type ArtifactStoreConfig struct {
	Endpoint        string `yaml:"endpoint"          mapstructure:"endpoint"`
	Region          string `yaml:"region"            mapstructure:"region"`
	Bucket          string `yaml:"bucket"            mapstructure:"bucket"`
	Prefix          string `yaml:"prefix"            mapstructure:"prefix"`
	AccessKeyID     string `yaml:"access_key_id"     mapstructure:"access_key_id"`
	SecretAccessKey string `yaml:"secret_access_key" mapstructure:"secret_access_key"`
	UseSSL          bool   `yaml:"use_ssl"           mapstructure:"use_ssl"`
}

// MeshConfig opts the server into the agent overlay. Identity is
// self-issued at startup against the named project's CA (see
// cmd/platypus-server/main.go::tryStartServerMesh); the listener is
// the unified ingress, so neither identity-on-disk nor a separate
// mesh listen address belongs in this struct.
type MeshConfig struct {
	PSKFile        string   `yaml:"psk_file"        mapstructure:"psk_file"`
	AdvertiseAddrs []string `yaml:"advertise_addrs" mapstructure:"advertise_addrs"`
	Peers          []string `yaml:"peers"           mapstructure:"peers"`

	DiscoveryLAN      bool   `yaml:"discovery_lan"      mapstructure:"discovery_lan"`
	DiscoveryInterval int    `yaml:"discovery_interval" mapstructure:"discovery_interval"`
	ProjectID         string `yaml:"project_id"         mapstructure:"project_id"`
	BootstrapTarget   string `yaml:"bootstrap_target"   mapstructure:"bootstrap_target"`
}

type Config struct {
	Ingress     IngressConfig     `yaml:"ingress"`
	RESTful     RESTfulConfig     `yaml:"restful"`
	Distributor DistributorConfig `yaml:"distributor"`
	Mesh        MeshConfig        `yaml:"mesh"`
}
