package main

import (
	"bufio"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/WangYihang/Platypus/cmd/admin/ctx"
	"github.com/WangYihang/Platypus/cmd/admin/meta"
	"github.com/WangYihang/Platypus/internal/util/fs"
	"github.com/WangYihang/Platypus/internal/util/log"
	"github.com/WangYihang/Platypus/internal/util/suggest"
	prompt "github.com/c-bata/go-prompt"
	"github.com/c-bata/go-prompt/completer"
	"github.com/google/shlex"
	"github.com/gorilla/websocket"
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
	// User did not type anything
	// eg: ``
	text := d.TextBeforeCursor()
	if strings.TrimSpace(text) == "" {
		return suggest.GetCommandSuggestions()
	}
	// User typed something
	// eg: `prox`
	args := strings.Split(text, " ")
	cmd := args[0]
	if len(args) <= 1 {
		return suggest.GetFuzzyCommandSuggestions(cmd)
	} else {
		// Ensure cmd is a valid cmd
		if !suggest.IsValidCommand(cmd) {
			return []prompt.Suggest{}
		}
		return suggest.GetArgumentsSuggestions(text)
	}
}

func Executor(text string) {
	// Save into history
	AppendHistory(ctx.GetHistoryFilepath(), text)
	// Execute
	arguments, _ := shlex.Split(text)
	if len(arguments) > 0 {
		command := arguments[0]
		if val, ok := suggest.GetMetaCommandsMap()[strings.ToLower(command)]; ok {
			val.(meta.MetaCommand).Execute(arguments)
		}
	}
}

func AppendHistory(path string, content string) {
	fs.AppendFile(path, []byte(content+"\n"))
}

func LoadHistory(path string) []string {
	file, err := os.Open(path)
	if err != nil {
		return []string{}
	}
	scanner := bufio.NewScanner(file)
	scanner.Split(bufio.ScanLines)
	var text []string
	for scanner.Scan() {
		text = append(text, scanner.Text())
	}
	defer file.Close()
	return text
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
		prompt.OptionTitle("platypus-admin: interactive platypus client"),
		prompt.OptionPrefix(">> "),
		prompt.OptionHistory(LoadHistory(ctx.GetHistoryFilepath())),
		prompt.OptionInputTextColor(prompt.Yellow),
		prompt.OptionCompletionWordSeparator(completer.FilePathCompletionSeparator),
	)
	p.Run()
}

func Interact(hash string) {
	u := url.URL{Scheme: "ws", Host: "127.0.0.1:7331", Path: fmt.Sprintf("/ws/tty/%s", hash)}
	log.Info("connecting to %s", u.String())
	authedHeader := http.Header{}
	authedHeader.Add("Accept", "application/json")
	authedHeader.Add("Authorization", fmt.Sprintf("Bearer %s", ctx.Ctx.Token))

	fmt.Println(u.String())
	c, _, err := websocket.DefaultDialer.Dial(u.String(), authedHeader)
	if err != nil {
		log.Error(err.Error())
		return
	}
	defer c.Close()

	// Set stdin in raw mode.
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		log.Error(err.Error())
		return
	}

	defer func() {
		term.Restore(int(os.Stdin.Fd()), oldState)
		log.Info("Restoring...")
	}()

	go func() {
		for {
			_, message, err := c.ReadMessage()
			// opcode := message[0]
			if err != nil {
				log.Info("read:", err)
				continue
			}
			body := message[1:]
			os.Stdout.Write(body)
		}
	}()

	for {
		buffer := make([]byte, 0x10)
		n, err := os.Stdin.Read(buffer)
		if err != nil {
			log.Error(err.Error())
			continue
		}
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

func main() {
	StartCli()
}
