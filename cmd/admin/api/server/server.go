package server

import (
	"fmt"

	"github.com/WangYihang/Platypus/cmd/admin/api"
	"github.com/WangYihang/Platypus/cmd/admin/ctx"
	server_controller "github.com/WangYihang/Platypus/internal/controller/server"
	"github.com/imroc/req"
)

type ServersResponse struct {
	api.Response
	server_controller.ServersWithDistributorAddress `json:"msg"`
}

func GetServers() ServersResponse {
	authedHeader := req.Header{
		"Accept":        "application/json",
		"Authorization": fmt.Sprintf("Bearer %s", ctx.Ctx.Token),
	}
	r, _ := req.Get(fmt.Sprintf("http://%s:%d/api/v1/servers", ctx.Ctx.Host, ctx.Ctx.Port), authedHeader)
	sr := ServersResponse{}
	r.ToJSON(&sr)
	return sr
}
