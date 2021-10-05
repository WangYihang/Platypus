package auth

import (
	"fmt"

	"github.com/WangYihang/Platypus/cmd/admin/ctx"
	"github.com/WangYihang/Platypus/cmd/admin/meta"
	"github.com/WangYihang/Platypus/internal/util/log"
	"github.com/c-bata/go-prompt"
	"github.com/imroc/req"
)

type Command struct{}

func (command Command) Name() string {
	return "Auth"
}

func (command Command) Help() string {
	return "Auth"
}

func (command Command) Description() string {
	return "Auth"
}

func (command Command) Arguments() []meta.Argument {
	return []meta.Argument{
		{Name: "username", Desc: "platypus username", IsFlag: false, IsRequired: true, AllowRepeat: false, Default: nil, SuggestFunc: command.Suggest},
		{Name: "password", Desc: "platypus password", IsFlag: false, IsRequired: true, AllowRepeat: false, Default: nil, SuggestFunc: command.Suggest},
	}
}

type LoginResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Expire  string `json:"expire"`
	Token   string `json:"token"`
}

func login(username string, password string) (*LoginResponse, error) {
	header := req.Header{
		"Accept": "application/json",
	}
	param := req.Param{
		"username": username,
		"password": password,
	}
	if ctx.Ctx.Host == "" {
		return nil, fmt.Errorf("host is not set yet, please use `connect` to set host and port")
	}
	r, err := req.Post(fmt.Sprintf("http://%s:%d/login", ctx.Ctx.Host, ctx.Ctx.Port), header, param)
	if err != nil {
		return nil, err
	}
	responseData := LoginResponse{}
	r.ToJSON(&responseData)
	return &responseData, nil
}

func (command Command) Execute(args []string) {
	// Check if the current context is already authed
	// TODO

	// Parse arguments
	result, err := meta.ParseArguments(command, args)
	if err != nil {
		log.Error(err.Error())
		return
	}
	username := *result["username"].(*string)
	password := *result["password"].(*string)

	// Login
	log.Info("Logging in as %s...", username)
	loginResponse, err := login(username, password)
	if err != nil {
		log.Error(err.Error())
		return
	}
	if loginResponse.Code == 200 {
		// Login succeed, save token
		ctx.Ctx.Token = loginResponse.Token
		log.Success("Token: %s", ctx.Ctx.Token)
	} else {
		log.Error("Login failed: %s", loginResponse.Message)
	}
}

func (command Command) Suggest(name string) []prompt.Suggest {
	return []prompt.Suggest{}
}
