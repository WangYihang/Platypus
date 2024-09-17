package options

import (
	"github.com/WangYihang/Platypus/pkg/version"
	"github.com/jessevdk/go-flags"
)

// Options represents the command line options
type Options struct {
	RemoteHost  string `short:"h" long:"host" description:"Remote host" required:"true"`
	RemotePort  int    `short:"p" long:"port" description:"Remote port" required:"true"`
	Token       string `short:"t" long:"token" description:"API token" required:"true"`
	Environment string `short:"e" long:"env" description:"Environment" required:"true" choice:"development" choice:"staging" choice:"production" default:"production"`
	Version     func() `short:"v" long:"version" description:"Print version information and exit"`
}

// InitOptions initializes the command line options
func InitOptions() (*Options, error) {
	var opts = Options{
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
