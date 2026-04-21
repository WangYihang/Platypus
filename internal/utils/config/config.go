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

// DistributorConfig defines the HTTP endpoint that serves agent binaries for
// admins to download and install on managed hosts.
type DistributorConfig struct {
	Host string `yaml:"host" validate:"required"`
	Port uint16 `yaml:"port" validate:"required,min=1,max=65535"`
	Url  string `yaml:"url"`
}

type Config struct {
	Listeners   []ListenerConfig  `yaml:"listeners"`
	RESTful     RESTfulConfig     `yaml:"restful"`
	Distributor DistributorConfig `yaml:"distributor"`
	Update      bool              `yaml:"update"`
	OpenBrowser bool              `yaml:"openBrowser"`
}
