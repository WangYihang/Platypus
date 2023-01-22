package main

import (
	_ "github.com/WangYihang/Platypus/api"
	"github.com/WangYihang/Platypus/internal/backend/restful"
	"github.com/WangYihang/Platypus/internal/databases"
	"github.com/WangYihang/Platypus/internal/models/agent"
	"github.com/WangYihang/Platypus/internal/models/listener"
	"github.com/WangYihang/Platypus/internal/utils/apm"
	"github.com/WangYihang/Platypus/internal/utils/monitor"
	"github.com/getsentry/sentry-go"
)

//	@title			Platypus API
//	@version		2.0.0
//	@description	This is a sample listener celler listener.
//	@termsOfService	http://swagger.io/terms/

//	@contact.name	Yihang Wang
//	@contact.url	http://www.swagger.io/support
//	@contact.email	wangyihanger@gmail.com

//	@license.name	Apache 2.0
//	@license.url	http://www.apache.org/licenses/LICENSE-2.0.html

//	@host		localhost:8080
//	@BasePath	/api/v1

// @securityDefinitions.basic	BasicAuth
func main() {
	defer sentry.Recover()
	databases.ConnectDatabase()
	agent.Init()
	listener.Init()
	listener.ResumeAllListeners()
	apm.SetupSenery()
	go monitor.Monitor(8)
	restful.StartEndpoint()
}
