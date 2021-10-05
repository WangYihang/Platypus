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

type CompileResponse struct {
	api.Response
	Message string `json:"msg"`
}

func Compile(host string, port uint16, os string, arch string, upx int) (string, error) {
	authedHeader := req.Header{
		"Accept":        "application/json",
		"Authorization": fmt.Sprintf("Bearer %s", ctx.Ctx.Token),
	}
	param := req.Param{
		"host": host,
		"port": port,
		"os":   os,
		"arch": arch,
		"upx":  upx,
	}
	r, _ := req.Post(fmt.Sprintf("http://%s:%d/api/v1/compile", ctx.Ctx.Host, ctx.Ctx.Port), authedHeader, param)
	sr := CompileResponse{}
	r.ToJSON(&sr)
	if sr.Status {
		return sr.Message, nil
	} else {
		return "", fmt.Errorf(sr.Message)
	}
}

type DistributorPortResponse struct {
	api.Response
	Message uint16 `json:"msg"`
}

func GetDistribuorPort() uint16 {
	authedHeader := req.Header{
		"Accept":        "application/json",
		"Authorization": fmt.Sprintf("Bearer %s", ctx.Ctx.Token),
	}
	r, _ := req.Get(fmt.Sprintf("http://%s:%d/api/v1/distport", ctx.Ctx.Host, ctx.Ctx.Port), authedHeader)
	sr := DistributorPortResponse{}
	r.ToJSON(&sr)
	return sr.Message
}
