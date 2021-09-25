package main

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	model_server "github.com/WangYihang/Platypus/internal/model/server"
	"github.com/WangYihang/Platypus/internal/util/log"
	prompt "github.com/c-bata/go-prompt"
	"github.com/c-bata/go-prompt/completer"
	"github.com/google/shlex"
	"github.com/gorilla/websocket"
	"github.com/imroc/req"
	"golang.org/x/term"
)

type Completer struct {
	namespace string
}

func NewCompleter() (*Completer, error) {
	return &Completer{
		namespace: "admin",
	}, nil
}

func (c *Completer) Complete(d prompt.Document) []prompt.Suggest {
	// fmt.Println("Complete is clalled")
	if d.TextBeforeCursor() == "" {
		return []prompt.Suggest{
			{Text: "run", Description: "start server"},
			{Text: "server", Description: "list servers"},
		}
	}
	args, _ := shlex.Split(d.TextBeforeCursor())
	cmd := args[0]
	switch cmd {
	case "run":
		return []prompt.Suggest{
			{Text: "--host", Description: "host to bind"},
			{Text: "--port", Description: "port to listen on"},
		}
	case "server":
		x := []prompt.Suggest{}
		servers := GetServers()
		for key, element := range servers.ServersWithDistributorAddress.Servers {
			x = append(x,
				prompt.Suggest{Text: key, Description: fmt.Sprintf("%s:%d", element.Host, element.Port)},
			)
		}
		return x
	}

	// // If PIPE is in text before the cursor, returns empty suggestions.
	// for i := range args {
	// 	if args[i] == "|" {
	// 		return []prompt.Suggest{}
	// 	}
	// }

	// // If word before the cursor starts with "-", returns CLI flag options.
	// if strings.HasPrefix(w, "-") {
	// 	return optionCompleter(args, strings.HasPrefix(w, "--"))
	// }

	// Return suggestions for option
	suggests, _ := c.completeOptionArguments(d)
	return suggests

	// namespace := checkNamespaceArg(d)
	// if namespace == "" {
	// 	namespace = c.namespace
	// }
	// commandArgs, skipNext := excludeOptions(args)
	// if skipNext {
	// 	// when type 'get pod -o ', we don't want to complete pods. we want to type 'json' or other.
	// 	// So we need to skip argumentCompleter.
	// 	return []prompt.Suggest{}
	// }
	// return c.argumentsCompleter(namespace, commandArgs)
}

func getPreviousOption(d prompt.Document) (cmd, option string, found bool) {
	shlex.Split(d.Text)
	args := strings.Split(d.TextBeforeCursor(), " ")
	l := len(args)
	if l >= 2 {
		option = args[l-2]
	}
	if strings.HasPrefix(option, "-") {
		return args[0], option, true
	}
	return "", "", false
}

func (c *Completer) completeOptionArguments(d prompt.Document) ([]prompt.Suggest, bool) {
	cmd, option, found := getPreviousOption(d)
	fmt.Printf("\n%v %v %v\n", cmd, option, found)
	if !found {
		return []prompt.Suggest{}, false
	}

	// shlex.Split(d.Text)
	// container
	switch cmd {
	case "exec", "logs", "run", "attach", "port-forward", "cp":
		if option == "-c" || option == "--container" {
			var suggestions []prompt.Suggest
			return prompt.FilterHasPrefix(
				suggestions,
				d.GetWordBeforeCursor(),
				true,
			), true
		}
	}
	return []prompt.Suggest{}, false
}

func Executor(s string) {
	fmt.Println(s)
}

type LoginResponse struct {
	Code   int    `json:"code"`
	Expire string `json:"expire"`
	Token  string `json:"token"`
}

type Response struct {
	Status bool `json:"status"`
}

type ServersResponse struct {
	Response
	model_server.ServersWithDistributorAddress `json:"msg"`
}
type Runtime struct {
	Token string
}

var rt Runtime

func Init() {
	header := req.Header{
		"Accept": "application/json",
		// "Authorization": "Basic YWRtaW46YWRtaW4=",
	}
	param := req.Param{
		"username": "admin",
		"password": "admin",
	}
	// only url is required, others are optional.
	r, err := req.Post("http://127.0.0.1:7331/login", header, param)
	if err != nil {
		log.Error(err.Error())
	}
	responseData := LoginResponse{}
	r.ToJSON(&responseData) // response => struct/map
	rt.Token = responseData.Token
}

func GetServers() ServersResponse {
	authedHeader := req.Header{
		"Accept":        "application/json",
		"Authorization": fmt.Sprintf("Bearer %s", rt.Token),
	}
	r, _ := req.Get("http://127.0.0.1:7331/api/v1/servers", authedHeader)
	xr := ServersResponse{}
	r.ToJSON(&xr) // response => struct/map
	log.Info("%+v", xr)
	return xr
}

func StartCli() {
	c, err := NewCompleter()
	if err != nil {
		fmt.Println("error", err)
		os.Exit(1)
	}

	p := prompt.New(
		Executor,
		c.Complete,
		prompt.OptionTitle("kube-prompt: interactive kubernetes client"),
		prompt.OptionPrefix(">>> "),
		prompt.OptionInputTextColor(prompt.Yellow),
		prompt.OptionCompletionWordSeparator(completer.FilePathCompletionSeparator),
	)
	p.Run()
}

func main() {
	Init()

	u := url.URL{Scheme: "ws", Host: "127.0.0.1:7331", Path: "/ws/tty/5bd4097dd23e96b691fe7bd676975176"}
	log.Info("connecting to %s", u.String())
	authedHeader := http.Header{}
	authedHeader.Add("Accept", "application/json")
	authedHeader.Add("Authorization", fmt.Sprintf("Bearer %s", rt.Token))

	fmt.Println(u.String())
	c, _, err := websocket.DefaultDialer.Dial(u.String(), authedHeader)
	if err != nil {
		log.Error("dial:", err)
	}
	defer c.Close()

	done := make(chan struct{})

	// Set stdin in raw mode.
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		panic(err)
	}
	defer func() {
		term.Restore(int(os.Stdin.Fd()), oldState)
		log.Info("Restoreing...")
	}()

	go func() {
		defer close(done)
		for {
			_, message, err := c.ReadMessage()
			// opcode := message[0]
			if err != nil {
				log.Info("read:", err)
				return
			}
			body := message[1:]
			os.Stdout.Write(body)
		}
	}()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		buffer := make([]byte, 0x10)
		n, _ := os.Stdin.Read(buffer)
		if n > 0 {
			message := make([]byte, 0)
			message = append(message, []byte("0")...)
			message = append(message, buffer[0:n]...)
			err := c.WriteMessage(websocket.BinaryMessage, message)
			if err != nil {
				break
			}
		}
	}
}
