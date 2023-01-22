package config

import (
	"github.com/golobby/config/v3"
	"github.com/golobby/config/v3/pkg/feeder"
)

type myConfig struct {
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
	JWTSecretKey string `yaml:"jwt_secret_key"`
	Update       bool   `yaml:"update"`
	OpenBrowser  bool   `yaml:"openBrowser"`
}

var RemoteAddrPlaceHolder = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA" +
	"BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB" +
	"CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC" +
	"DDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDD" +
	":" + "65535"

// Create an instance of the configuration struct
var MyConfig *myConfig

func LoadConfig() *myConfig {
	if MyConfig == nil {
		MyConfig = &myConfig{}
		jsonFeeder := feeder.Yaml{Path: "config.yaml"}
		c := config.New()
		c.AddFeeder(jsonFeeder)
		c.AddStruct(MyConfig)
		c.Feed()
	}
	return MyConfig
}
