package config

type Config struct {
	Servers []struct {
		Host           string `yaml:"host"`
		Port           uint16 `yaml:"port"`
		HashFormat     string `yaml:"hashFormat"`
		Encrypted      bool   `yaml:"encrypted"`
		DisableHistory bool   `yaml:"disable_history"`
		PublicIP       string `yaml:"public_ip"`
	}
	RESTful struct {
		Host   string `yaml:"host"`
		Port   uint16 `yaml:"port"`
		Enable bool   `yaml:"enable"`
	}
	Distributor struct {
		Host string `yaml:"host"`
		Port uint16 `yaml:"port"`
		Url  string `yaml:"url"`
	}
	Update      bool `yaml:"update"`
	OpenBrowser bool `yaml:"openBrowser"`
}

var RemoteAddrPlaceHolder = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA" +
	"BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB" +
	"CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC" +
	"DDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDD" +
	":" + "65535"
