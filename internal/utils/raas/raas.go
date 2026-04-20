package raas

import (
	"embed"
	"fmt"
	"io/fs"
	"strconv"
	"strings"

	"github.com/WangYihang/Platypus/internal/log"
)

//go:embed templates/*.tpl
var templates embed.FS

const fallbackLanguage = "bash"

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

func readTemplate(language string) ([]byte, error) {
	return fs.ReadFile(templates, fmt.Sprintf("templates/%s.tpl", language))
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
		t, err := strconv.Atoi(target[1])
		port = uint16(t)
		if err != nil {
			log.Debug("Invalid port number: %s", target[1])
		}
	}

	// step 2: parse language (last path element)
	language := strings.Replace(target[len(target)-1], ".", "", -1)

	// step 3: read template (fall back to bash if language is unknown)
	templateContent, err := readTemplate(language)
	if err != nil {
		templateContent, _ = readTemplate(fallbackLanguage)
	}

	// step 4: render target host and port into template
	rendered := string(templateContent)
	rendered = strings.Replace(rendered, "__HOST__", host, -1)
	rendered = strings.Replace(rendered, "__PORT__", strconv.Itoa(int(port)), -1)
	return rendered
}
