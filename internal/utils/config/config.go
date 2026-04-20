package config

type ServerConfig struct {
	Host           string `yaml:"host"            validate:"required"`
	Port           uint16 `yaml:"port"            validate:"required,min=1,max=65535"`
	HashFormat     string `yaml:"hashFormat"`
	Encrypted      bool   `yaml:"encrypted"`
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
	RefreshExpireTime int    `yaml:"RefreshExpireTime"`
	AccessExpireTime  int    `yaml:"AccessExpireTime"`
	DBFile            string `yaml:"DBFile"`
	Domain            string `yaml:"Domain"` // 公网 IP
}

type DistributorConfig struct {
	Host string `yaml:"host" validate:"required"`
	Port uint16 `yaml:"port" validate:"required,min=1,max=65535"`
	Url  string `yaml:"url"`
}

type Config struct {
	Servers     []ServerConfig    `yaml:"servers"`
	RESTful     RESTfulConfig     `yaml:"restful"`
	Distributor DistributorConfig `yaml:"distributor"`
	Update      bool              `yaml:"update"`
	OpenBrowser bool              `yaml:"openBrowser"`
}
