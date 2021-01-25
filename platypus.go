package main

import (
	"io/ioutil"
	"time"

	"github.com/WangYihang/Platypus/lib/cli/dispatcher"
	"github.com/WangYihang/Platypus/lib/context"
	"github.com/WangYihang/Platypus/lib/util/fs"
	"github.com/WangYihang/Platypus/lib/util/resource"
	"gopkg.in/yaml.v2"
)

type Config struct {
	Servers []struct {
		Host string `yaml:"host"`
		Port int16  `yaml:"port"`
	}
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

	// Init servers from config file
	for _, s := range config.Servers {
		server := context.CreateTCPServer(s.Host, uint16(s.Port))
		// avoid terminal being disrupted
		time.Sleep(0x100 * time.Millisecond)
		go (*server).Run()
		context.Ctx.AddServer(server)
	}

	// Run main loop
	dispatcher.Run()
}
