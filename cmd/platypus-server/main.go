package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"runtime"
	"time"

	"github.com/WangYihang/Platypus/internal/cli/dispatcher"
	"github.com/WangYihang/Platypus/internal/context"
	"github.com/WangYihang/Platypus/internal/utils/config"
	"github.com/WangYihang/Platypus/internal/utils/fs"
	"github.com/WangYihang/Platypus/internal/utils/log"
	"github.com/WangYihang/Platypus/internal/utils/update"
	"github.com/pkg/browser"
	"gopkg.in/yaml.v2"
)

func main() {
	// Detect and create config file
	configFilenameWithVersion := fmt.Sprintf("config-v%s.yml", update.Version)
	if !fs.FileExists(configFilenameWithVersion) {
		content, _ := os.ReadFile("assets/config.example.yml")
		ioutil.WriteFile(configFilenameWithVersion, content, 0644)
	}

	var configFilename string
	if runtime.GOOS == "windows" {
		configFilename = configFilenameWithVersion
	} else {
		configFilename = "config.yml"
		if fs.FileExists(configFilename) {
			os.Remove(configFilename)
		}
		os.Symlink(configFilenameWithVersion, configFilename)
	}
	// Read config file
	var config config.Config
	content, _ := ioutil.ReadFile(configFilename)
	err := yaml.Unmarshal(content, &config)
	if err != nil {
		log.Error("Read config file failed, please check syntax of file `%s`, or just delete the `%s` to force regenerate config file", configFilename, configFilename)
		return
	}

	// Display platypus information
	log.Success("Platypus %s is starting...", update.Version)

	// Create context
	context.CreateContext()
	context.Ctx.Config = &config

	// Detect new version
	if config.Update {
		update.ConfirmAndSelfUpdate()
	}

	// Init distributor server from config file
	rh := config.Distributor.Host
	rp := config.Distributor.Port
	distributor := context.CreateDistributorServer(rh, rp, config.Distributor.Url)

	go distributor.Run(fmt.Sprintf("%s:%d", rh, rp))

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
		context.Ctx.RESTful = rest
	}

	// Init servers from config file
	for _, s := range config.Servers {
		server := context.CreateTCPServer(s.Host, uint16(s.Port), s.HashFormat, s.Encrypted, s.DisableHistory, s.PublicIP, s.ShellPath)
		if server != nil {
			// avoid terminal being disrupted
			time.Sleep(0x100 * time.Millisecond)
			go (*server).Run()
		}
	}

	if config.OpenBrowser {
		browser.OpenURL(fmt.Sprintf("http://%s:%d/", config.RESTful.Host, config.RESTful.Port))
	}

	// Run main loop
	dispatcher.REPL()
}
