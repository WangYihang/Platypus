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

	HashFormat     string `yaml:"hash_format"     mapstructure:"hash_format"`
	ShellPath      string `yaml:"shell_path"      mapstructure:"shell_path"`
	DisableHistory bool   `yaml:"disable_history" mapstructure:"disable_history"`
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

// RESTfulConfig configures the REST/WebSocket admin surface. Since
// the unified ingress merged every port into cfg.Ingress, only the
// JWT/storage knobs remain here — the REST engine is mounted onto
// the ingress dispatcher's virtual HTTP listener.
type RESTfulConfig struct {
	Enable            bool   `yaml:"enable"            mapstructure:"enable"`
	JWTRefreshKey     string `yaml:"JWTRefreshKey"     mapstructure:"JWTRefreshKey"`
	JWTAccessKey      string `yaml:"JWTAccessKey"      mapstructure:"JWTAccessKey"`
	RefreshExpireTime int    `yaml:"RefreshExpireTime" mapstructure:"RefreshExpireTime"` // seconds; 0 defaults to 14 days
	AccessExpireTime  int    `yaml:"AccessExpireTime"  mapstructure:"AccessExpireTime"`  // seconds; 0 defaults to 15 min
	DBFile            string `yaml:"DBFile"            mapstructure:"DBFile"`            // empty defaults to ./platypus.db
}

// AccessTTLOrDefault returns the configured access token lifetime in
// seconds, or a sensible default (15 minutes) when unset.
func (c RESTfulConfig) AccessTTLOrDefault() int {
	if c.AccessExpireTime > 0 {
		return c.AccessExpireTime
	}
	return 15 * 60
}

// RefreshTTLOrDefault returns the configured refresh token lifetime in
// seconds, or a sensible default (14 days) when unset.
func (c RESTfulConfig) RefreshTTLOrDefault() int {
	if c.RefreshExpireTime > 0 {
		return c.RefreshExpireTime
	}
	return 14 * 24 * 60 * 60
}

// DBFileOrDefault returns the configured SQLite path, or "./platypus.db"
// when unset. Unix paths only for now.
func (c RESTfulConfig) DBFileOrDefault() string {
	if c.DBFile != "" {
		return c.DBFile
	}
	return "./platypus.db"
}

// DistributorConfig defines the routes that front the agent release
// artifact store. The distributor itself serves only a signed manifest
// and redirects to presigned object-store URLs for artifact
// downloads — the binaries live in Store. The HTTP routes are mounted
// on the same gin engine the REST API runs on; no dedicated port.
type DistributorConfig struct {
	Url          string              `yaml:"url"           mapstructure:"url"`
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

// ChannelOrDefault returns the configured release channel, defaulting
// to "stable" when unset.
func (c DistributorConfig) ChannelOrDefault() string {
	if c.Channel != "" {
		return c.Channel
	}
	return "stable"
}

// MeshConfig opts the server into the agent overlay. When PSKFile is
// empty the server stays in pure hub-and-spoke mode; otherwise it
// generates a persistent mesh identity, accepts inbound mesh links, and
// can route admin traffic to any reachable agent by NodeID.
type MeshConfig struct {
	PSKFile        string   `yaml:"psk_file"        mapstructure:"psk_file"`
	IdentityDir    string   `yaml:"identity_dir"    mapstructure:"identity_dir"`
	ListenAddr     string   `yaml:"listen_addr"     mapstructure:"listen_addr"`
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
	OpenBrowser bool              `yaml:"openBrowser"`
}
