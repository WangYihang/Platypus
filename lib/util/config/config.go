package config

type Config struct {
	Servers []struct {
		Host       string `yaml:"host"`
		Port       uint16 `yaml:"port"`
		HashFormat string `yaml:"hashFormat"`
		Encrypted  bool   `yaml:"encrypted"`
	}
	RESTful struct {
		Host   string `yaml:"host"`
		Port   uint16 `yaml:"port"`
		Enable bool   `yaml:"enable"`
	}
	Distributor struct {
		Host string `yaml:"host"`
		Port uint16 `yaml:"port"`
	}
	Update      bool `yaml:"update"`
	OpenBrowser bool `yaml:"openBrowser"`
}
