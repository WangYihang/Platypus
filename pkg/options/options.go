package options

import (
	"github.com/jessevdk/go-flags"

	"github.com/WangYihang/Platypus/pkg/version"
)

// Options represents the command line options
type Options struct {
	RemoteHost string `short:"h" long:"host" description:"Remote host" required:"true"`
	RemotePort int    `short:"p" long:"port" description:"Remote port" required:"true"`
	Token      string `short:"t" long:"token" description:"API token" required:"true"`

	// Mesh overlay (optional — leaving MeshPSKFile empty keeps the agent
	// in plain hub-and-spoke mode).
	MeshListen      string   `long:"mesh-listen" description:"Address to accept inbound mesh links on, e.g. :17777. Empty = no listener, dial-only."`
	MeshPeers       []string `long:"peers" description:"Bootstrap mesh peer in host:port form. Repeatable."`
	MeshPSKFile     string   `long:"psk-file" description:"Path to mesh pre-shared key file. Enables mesh mode."`
	MeshIdentityDir string   `long:"identity-dir" description:"Directory for persistent Ed25519 mesh identity. Default: ~/.platypus/mesh/agent"`
	MeshAdvertise   []string `long:"mesh-advertise" description:"Override advertised mesh listen address(es). Repeatable."`

	MeshDiscoveryLAN      bool   `long:"mesh-discovery" description:"Enable automatic LAN peer discovery via mDNS."`
	MeshDiscoveryInterval int    `long:"mesh-discovery-interval" description:"mDNS scan interval in seconds. Default: 30."`
	MeshProjectID         string `long:"project-id" description:"Project ID for isolating mesh discovery on LAN."`

	Version func() `short:"v" long:"version" description:"Print version information and exit"`
}

// InitOptions initializes the command line options
func InitOptions() (*Options, error) {
	var opts = Options{
		MeshDiscoveryLAN:      true,
		MeshDiscoveryInterval: 30,
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
