package config

type Config struct {
	Servers []struct {
		Host           string `yaml:"host"`
		Port           uint16 `yaml:"port"`
		HashFormat     string `yaml:"hashFormat"`
		Encrypted      bool   `yaml:"encrypted"`
		DisableHistory bool   `yaml:"disable_history"`
		PublicIP       string `yaml:"public_ip"`
		ShellPath      string `yaml:"shell_path"`
	}
	RESTful struct {
		Host              string `yaml:"host"`
		Port              uint16 `yaml:"port"`
		Enable            bool   `yaml:"enable"`
		JWTRefreshKey     string `yaml:"JWTRefreshKey"`
		JWTAccessKey      string `yaml:"JWTAccessKey"`
		RefreshExpireTime int    `yaml:"RefreshExpireTime"`
		AccessExpireTime  int    `yaml:"AccessExpireTime"`
		DBFile            string `yaml:"DBFile"`
		Domain            string `yaml:"Domain"` // 公网IP
	}
	Distributor struct {
		Host string `yaml:"host"`
		Port uint16 `yaml:"port"`
		Url  string `yaml:"url"`
	}
	Update      bool `yaml:"update"`
	OpenBrowser bool `yaml:"openBrowser"`
}
