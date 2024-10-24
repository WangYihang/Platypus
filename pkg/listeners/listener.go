package listeners

type commonListener struct {
	BindHost string `json:"bind_host" yaml:"bind_host" toml:"bind_host"`
	BindPort uint16 `json:"bind_port" yaml:"bind_port" toml:"bind_port"`
}
