package interact

import (
	"fmt"
	"net/http"
	"net/url"
	"os"

	server_api "github.com/WangYihang/Platypus/cmd/admin/api/server"
	"github.com/WangYihang/Platypus/cmd/admin/ctx"
	"github.com/WangYihang/Platypus/cmd/admin/meta"
	"github.com/WangYihang/Platypus/internal/util/log"
	"github.com/c-bata/go-prompt"
	"github.com/gorilla/websocket"
	"golang.org/x/term"
)

type Command struct{}

func (command Command) Name() string {
	return "Interact"
}

func (command Command) Help() string {
	return "Interact"
}

func (command Command) Description() string {
	return "Interact"
}

func (command Command) Arguments() []meta.Argument {
	return []meta.Argument{
		{Name: "hash", Desc: "hash of a client / termite to interact with", IsFlag: false, IsRequired: true, AllowRepeat: false, Default: nil, SuggestFunc: command.Suggest},
	}
}

func interact(hash string) {
	u := url.URL{Scheme: "ws", Host: fmt.Sprintf("%s:%d", ctx.Ctx.Host, ctx.Ctx.Port), Path: fmt.Sprintf("/ws/tty/%s", hash)}
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

	isClosed := false
	c.SetCloseHandler(func(code int, text string) error {
		isClosed = true
		return nil
	})

	// Set stdin in raw mode.
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		log.Error(err.Error())
		return
	}

	defer func() {
		term.Restore(int(os.Stdin.Fd()), oldState)
	}()

	go func() {
		for {
			if isClosed {
				break
			}
			_, message, err := c.ReadMessage()
			// opcode := message[0]
			if err != nil {
				log.Info(err.Error())
				continue
			}
			body := message[1:]
			os.Stdout.Write(body)
		}
	}()

	for {
		if isClosed {
			break
		}
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

func (command Command) Execute(args []string) {
	if !ctx.IsValidToken(ctx.Ctx.Token) {
		log.Error("Invalid token: %s", ctx.Ctx.Token)
		return
	}
	result, err := meta.ParseArguments(command, args)
	if err != nil {
		log.Error(err.Error())
		return
	}
	hash := *result["hash"].(*string)
	interact(hash)
}

func (command Command) Suggest(name string, typed string) []prompt.Suggest {
	if !ctx.IsValidToken(ctx.Ctx.Token) {
		return []prompt.Suggest{}
	}
	switch name {
	case "hash":
		suggests := []prompt.Suggest{}
		for _, server := range server_api.GetServers().Servers {
			if server.Encrypted {
				for _, termite := range server.TermiteClients {
					suggest := prompt.Suggest{
						Text:        termite.Hash,
						Description: termite.OnelineDesc(),
					}
					suggests = append(suggests, suggest)
				}
			} else {
				for _, client := range server.Clients {
					suggest := prompt.Suggest{
						Text:        client.Hash,
						Description: client.OnelineDesc(),
					}
					suggests = append(suggests, suggest)
				}
			}
		}
		return suggests
	default:
		return []prompt.Suggest{}
	}
}
