package main

import (
	"fmt"
	"time"

	"github.com/WangYihang/Platypus/internal/cli/dispatcher"
	"github.com/WangYihang/Platypus/internal/context"
	"github.com/WangYihang/Platypus/internal/utils/config"
	"github.com/WangYihang/Platypus/internal/utils/log"
	"github.com/WangYihang/Platypus/internal/utils/update"
	"github.com/pkg/browser"
	"github.com/spf13/viper"
)

var cfg config.Config
var v = viper.New()

func init() {
	// Configure Viper
	v.SetConfigName("config")
	v.SetConfigType("yml")
	v.AddConfigPath(".")
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			log.Error("Config file not found")
		} else {
			log.Error("Failed to read config file: %v", err)
		}
		return
	}
	if err := v.Unmarshal(&cfg); err != nil {
		log.Error("Failed to unmarshal config: %v", err)
		return
	}
}

func main() {

	// Display platypus information
	log.Success("Platypus %s is starting...", update.Version)
	log.Success("Using configuration file: %s", v.ConfigFileUsed())

	// Create context
	context.CreateContext()
	context.Ctx.Config = &cfg

	// Detect new version
	if cfg.Update {
		update.ConfirmAndSelfUpdate()
	}

	// Init distributor server from config file
	rh := cfg.Distributor.Host
	rp := cfg.Distributor.Port
	distributor := context.CreateDistributorServer(rh, rp, cfg.Distributor.Url)

	go distributor.Run(fmt.Sprintf("%s:%d", rh, rp))

	// Init RESTful Server from config file
	if cfg.RESTful.Enable {
		rh := cfg.RESTful.Host
		rp := cfg.RESTful.Port
		rest := context.CreateRESTfulAPIServer()
		go rest.Run(fmt.Sprintf("%s:%d", rh, rp))
		log.Success("Web FrontEnd started at: http://%s:%d/", rh, rp)
		log.Success("You can use Web FrontEnd to manager all your clients with any web browser.")
		log.Success("RESTful API EndPoint at: http://%s:%d/api/", rh, rp)
		log.Success("You can use PythonSDK to manager all your clients automatically.")
		context.Ctx.RESTful = rest
	}

	// Init servers from config file
	for _, s := range cfg.Servers {
		server := context.CreateTCPServer(s.Host, uint16(s.Port), s.HashFormat, s.Encrypted, s.DisableHistory, s.PublicIP, s.ShellPath)
		if server != nil {
			// avoid terminal being disrupted
			time.Sleep(0x100 * time.Millisecond)
			go (*server).Run()
		}
	}

	if cfg.OpenBrowser {
		browser.OpenURL(fmt.Sprintf("http://%s:%d/", cfg.RESTful.Host, cfg.RESTful.Port))
	}

	// Run main loop
	dispatcher.REPL()
}
