package network

import (
	"encoding/json"
	"io/ioutil"
	"net"
	"net/http"
)

// GatherInterfacesList returns a list of interfaces
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

// IP represents the IP address
type IP struct {
	Query string
}

// GetPublicIP returns the public IP address
func GetPublicIP() (string, error) {
	req, err := http.Get("http://ip-api.com/json/")
	if err != nil {
		return "", err
	}
	defer req.Body.Close()

	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		return "", err
	}

	var ip IP

	err = json.Unmarshal(body, &ip)
	if err != nil {
		return "", err
	}

	return ip.Query, nil
}
