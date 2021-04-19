package main

import (
	"fmt"
	"io/ioutil"
	"time"

	"github.com/WangYihang/Platypus/lib/cli/dispatcher"
	"github.com/WangYihang/Platypus/lib/context"
	"github.com/WangYihang/Platypus/lib/util/fs"
	"github.com/WangYihang/Platypus/lib/util/log"
	"github.com/WangYihang/Platypus/lib/util/resource"
	"github.com/WangYihang/Platypus/lib/util/update"
	"gopkg.in/yaml.v2"
)

type Config struct {
	Servers []struct {
		Host       string `yaml:"host"`
		Port       uint16 `yaml:"port"`
		HashFormat string `yaml:"hashFormat"`
	}
	RESTful struct {
		Host   string `yaml:"host"`
		Port   uint16 `yaml:"port"`
		Enable bool   `yaml:"enable"`
	}
	Update bool
}

func main() {
	// Detect and create config file
	configFilename := "config.yml"
	if !fs.FileExists(configFilename) {
		content, _ := resource.Asset("lib/runtime/config.example.yml")
		ioutil.WriteFile(configFilename, content, 0644)
	}

	// Read config file
	var config Config
	content, _ := ioutil.ReadFile("config.yml")
	yaml.Unmarshal(content, &config)

	// Create context
	context.CreateContext()

	// Detect new version
	if config.Update {
		update.ConfirmAndSelfUpdate()
	}

	// Init servers from config file
	for _, s := range config.Servers {
		server := context.CreateTCPServer(s.Host, uint16(s.Port), s.HashFormat)
		// avoid terminal being disrupted
		time.Sleep(0x100 * time.Millisecond)
		go (*server).Run()
		context.Ctx.AddServer(server)
	}

	// Init RESTful Server from config file
	if config.RESTful.Enable {
		rh := config.RESTful.Host
		rp := config.RESTful.Port
		rest := context.CreateRESTfulAPIServer()
		go rest.Run(fmt.Sprintf("%s:%d", rh, rp))
		log.Success("Web FrontEnd started at: http://%s:%d/", rh, rp)
		log.Success("You can use Web FrontEnd to manager all your clients with any web browser.")
		log.Success("RESTful API EndPoint at: http://%s:%d/api/", rh, rp)
		log.Success("You can use PythonSDK to manager all your clients automatically.")
	}

	// Run main loop
	dispatcher.Run()
}
