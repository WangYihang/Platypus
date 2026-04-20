package raas

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"

	"github.com/WangYihang/Platypus/internal/log"
)

//go:embed templates/*.tpl
var templates embed.FS

const fallbackLanguage = "bash"

// Languages returns every language key backed by a template in templates/*.tpl,
// sorted alphabetically. Used by the v1 API so desktop + web clients don't
// have to hard-code the list.
func Languages() []string {
	entries, err := templates.ReadDir("templates")
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		name := strings.TrimSuffix(e.Name(), ".tpl")
		if name != "" {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// Render returns the one-liner for a given host:port and language. Unknown
// languages fall back to bash so callers never have to handle the miss case.
func Render(host string, port uint16, language string) string {
	content, err := readTemplate(language)
	if err != nil {
		content, _ = readTemplate(fallbackLanguage)
	}
	rendered := strings.ReplaceAll(string(content), "__HOST__", host)
	rendered = strings.ReplaceAll(rendered, "__PORT__", strconv.Itoa(int(port)))
	return rendered
}

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
	language := strings.ReplaceAll(target[len(target)-1], ".", "")

	// step 3: read template (fall back to bash if language is unknown)
	templateContent, err := readTemplate(language)
	if err != nil {
		templateContent, _ = readTemplate(fallbackLanguage)
	}

	// step 4: render target host and port into template
	rendered := string(templateContent)
	rendered = strings.ReplaceAll(rendered, "__HOST__", host)
	rendered = strings.ReplaceAll(rendered, "__PORT__", strconv.Itoa(int(port)))
	return rendered
}
