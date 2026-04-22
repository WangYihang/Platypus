package config

// ListenerConfig defines a TLS ingress port that managed-host agents dial
// back to.
type ListenerConfig struct {
	Host           string `yaml:"host"            validate:"required"`
	Port           uint16 `yaml:"port"            validate:"required,min=1,max=65535"`
	HashFormat     string `yaml:"hashFormat"`
	DisableHistory bool   `yaml:"disable_history"`
	PublicIP       string `yaml:"public_ip"`
	ShellPath      string `yaml:"shell_path"`
}

type RESTfulConfig struct {
	Host              string `yaml:"host"              validate:"required_if=Enable true"`
	Port              uint16 `yaml:"port"              validate:"required_if=Enable true,min=0,max=65535"`
	Enable            bool   `yaml:"enable"`
	JWTRefreshKey     string `yaml:"JWTRefreshKey"`
	JWTAccessKey      string `yaml:"JWTAccessKey"`
	RefreshExpireTime int    `yaml:"RefreshExpireTime"` // seconds; 0 defaults to 14 days
	AccessExpireTime  int    `yaml:"AccessExpireTime"`  // seconds; 0 defaults to 15 min
	DBFile            string `yaml:"DBFile"`            // empty defaults to ./platypus.db
	Domain            string `yaml:"Domain"`
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

// DistributorConfig defines the HTTP endpoint that fronts the agent
// release artifact store. The Distributor itself serves only a signed
// manifest and redirects to presigned object-store URLs for artifact
// downloads — the binaries live in Store.
//
// Every field carries a `mapstructure:` tag because viper's Unmarshal
// reads those, not `yaml:`. Without it snake_case keys silently bind
// to the zero value.
type DistributorConfig struct {
	Host         string              `yaml:"host"          mapstructure:"host" validate:"required"`
	Port         uint16              `yaml:"port"          mapstructure:"port" validate:"required,min=1,max=65535"`
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
	PSKFile        string   `yaml:"psk_file"`
	IdentityDir    string   `yaml:"identity_dir"`
	ListenAddr     string   `yaml:"listen_addr"`
	AdvertiseAddrs []string `yaml:"advertise_addrs"`
	Peers          []string `yaml:"peers"`
}

type Config struct {
	Listeners   []ListenerConfig  `yaml:"listeners"`
	RESTful     RESTfulConfig     `yaml:"restful"`
	Distributor DistributorConfig `yaml:"distributor"`
	Mesh        MeshConfig        `yaml:"mesh"`
	OpenBrowser bool              `yaml:"openBrowser"`
}
