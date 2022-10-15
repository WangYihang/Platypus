package Conf

type RESTful struct {
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

var RestfulConf RESTful
