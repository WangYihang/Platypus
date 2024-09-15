package raas

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/WangYihang/Platypus/internal/utils/log"
)

func ParsePort(host string, defaultPort uint16) uint16 {
	pair := strings.Split(host, ":")
	if len(pair) < 2 {
		return defaultPort
	}
	port, err := strconv.Atoi(pair[len(pair)-1])
	if err != nil {
		return defaultPort
	}
	return uint16(port)
}

func ParseHostname(host string) string {
	return strings.Split(host, ":")[0]
}

func URI2Command(requestURI string, httpHost string) string {
	// eg:
	//     "/python"                 -> "python"              -> {"python"}
	//     "/python/"                -> "python"              -> {"python"}
	//     "/8.8.8.8/1337"           -> "8.8.8.8/1337"        -> {"8.8.8.8", "1337"}
	//     "/8.8.8.8/1337/"          -> "8.8.8.8/1337"        -> {"8.8.8.8", "1337"}
	//     "/8.8.8.8/1337/python"    -> "8.8.8.8/1337/python" -> {"8.8.8.8", "1337", "python"}
	//     "/8.8.8.8/1337/python/"   -> "8.8.8.8/1337/python" -> {"8.8.8.8", "1337", "python"}
	//     "/8.8.8.8/1337/python///" -> "8.8.8.8/1337/python" -> {"8.8.8.8", "1337", "python"}
	target := strings.Split(strings.Trim(requestURI, "/"), "/")

	// step 1: parse host and port, default set to the platypus listening port currently
	host := ParseHostname(httpHost)
	port := ParsePort(httpHost, 80)

	if strings.HasPrefix(requestURI, "/") && len(target) > 1 {
		host = target[0]
		// TODO: ensure the format of port is int16
		t, err := strconv.Atoi(target[1])
		port = uint16(t)
		if err != nil {
			log.Debug("Invalid port number: %s", target[1])
		}
	}

	// step 2: parse language
	// language is the last element of target
	language := strings.Replace(target[len(target)-1], ".", "", -1)

	// step 3: read template
	// template rendering in golang tastes like shit,
	// here we will trying to use string replace temporarily.
	// read reverse shell template file from assets
	// preread to check the language is valid or not
	templateFilename := fmt.Sprintf("assets/template/rsh/%s.tpl", language)
	_, err := os.ReadFile(templateFilename)
	if err != nil {
		templateFilename = "assets/template/rsh/bash.tpl"
	}
	templateContent, _ := os.ReadFile(templateFilename)

	// step 4: render target host and port into template
	renderedContent := string(templateContent)
	renderedContent = strings.Replace(renderedContent, "__HOST__", host, -1)
	renderedContent = strings.Replace(renderedContent, "__PORT__", strconv.Itoa(int(port)), -1)
	return renderedContent
}
