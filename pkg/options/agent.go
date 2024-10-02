package options

import (
	"github.com/WangYihang/Platypus/pkg/version"
	"github.com/jessevdk/go-flags"
)

// AgentOptions represents the command line options
type AgentOptions struct {
	RemoteHost  string `short:"h" long:"host" description:"Remote host" required:"true"`
	RemotePort  int    `short:"p" long:"port" description:"Remote port" required:"true"`
	Token       string `short:"t" long:"token" description:"API token" required:"true"`
	Environment string `short:"e" long:"env" description:"Environment" required:"true" choice:"development" choice:"staging" choice:"production" default:"production"`
	Version     func() `short:"v" long:"version" description:"Print version information and exit"`
}

// InitAgentOptions initializes the command line options
func InitAgentOptions() (*AgentOptions, error) {
	var opts = AgentOptions{
		Version: func() {
			version.PrintVersion()
		},
	}
	_, err := flags.Parse(&opts)
	if err != nil {
		return nil, err
	}
	return &opts, nil
}
