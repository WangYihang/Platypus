package Conf

import (
	"fmt"
	"github.com/WangYihang/Platypus/internal/util/config"
	"os"

	"gopkg.in/yaml.v2"
)

type Conf struct {
	//	Jwt struct {
	JWTRefreshKey     string `yaml:"JWTRefreshKey"`
	JWTAccessKey      string `yaml:"JWTAccessKey"`
	RefreshExpireTime int    `yaml:"RefreshExpireTime"`
	AccessExpireTime  int    `yaml:"AccessExpireTime"`
	DBFile            string `yaml:"DBFile"`
	IP                string `yaml:"IP"` // 公网IP
	//	}
}

var ConfData *Conf
var MainConf config.Config

func init() {
	config := new(Conf)
	yamlFile, readErr := os.ReadFile("./internal/context/Conf/config.yaml")
	if readErr != nil {
		fmt.Println(readErr)
		return
	}
	unmarshalErr := yaml.Unmarshal(yamlFile, config)
	if unmarshalErr != nil {
		fmt.Println(unmarshalErr)
		return
	}

	ConfData = config
}
