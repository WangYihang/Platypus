package main

import (
	"github.com/WangYihang/Platypus/internal/databases"
	agent_model "github.com/WangYihang/Platypus/internal/models/agent"
	binary_model "github.com/WangYihang/Platypus/internal/models/binary"
	listener_model "github.com/WangYihang/Platypus/internal/models/listener"
	record_model "github.com/WangYihang/Platypus/internal/models/record"
	template_model "github.com/WangYihang/Platypus/internal/models/template"
	user_model "github.com/WangYihang/Platypus/internal/models/user"
)

func main() {
	var users = []user_model.User{
		{Username: "admin", Password: "$2a$14$.hgpkrxfBUnio1CUdxTB6Orc77rTjEwirFMVXpnBFC4GZ3niXwpSC", Role: "admin"},
		{Username: "user", Password: "$2a$14$10zaXJd7OuM1XS.q6oCLxOmfpa3jxY1pr4uIE4z9Y4QOlzGE9UcP.", Role: "user"},
	}

	var listeners = []listener_model.Listener{
		{Host: "127.0.0.1", Port: 13337, Protocol: "plain_tcp", Enable: true},
		{Host: "127.0.0.1", Port: 13338, Protocol: "termite_tcp", Enable: true},
		{Host: "127.0.0.1", Port: 13339, Protocol: "termite_tcp", Enable: false},
	}

	var templates = []template_model.Template{
		{OS: "linux", Arch: "amd64"},
		{OS: "linux", Arch: "x86"},
		{OS: "linux", Arch: "arm"},
	}

	DB := databases.ConnectDatabase()

	DB.AutoMigrate(&user_model.User{})
	DB.AutoMigrate(&listener_model.Listener{})
	DB.AutoMigrate(&agent_model.Agent{})
	DB.AutoMigrate(&template_model.Template{})
	DB.AutoMigrate(&binary_model.Binary{})
	DB.AutoMigrate(&record_model.Record{})

	for _, user := range users {
		if !user_model.CheckUserExists(user.Username) {
			DB.Create(&user)
		}
	}

	for _, listener := range listeners {
		if !listener_model.CheckListenerExists(listener.Host, listener.Port, listener.Protocol) {
			DB.Create(&listener)
		}
	}

	for _, template := range templates {
		if !template_model.CheckTemplateExists(template.OS, template.Arch) {
			DB.Create(&template)
		}
	}
}
