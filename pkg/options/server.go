package options

import (
	"github.com/WangYihang/Platypus/pkg/version"
	"github.com/jessevdk/go-flags"
)

// ServerOptions represents the command line options
type ServerOptions struct {
	ConfigFile string `short:"c" long:"config" description:"Path to the configuration file" required:"true"`
	Version    func() `short:"v" long:"version" description:"Print version information and exit"`
}

// InitServerOptions initializes the command line options
func InitServerOptions() (*ServerOptions, error) {
	var opts = ServerOptions{
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
