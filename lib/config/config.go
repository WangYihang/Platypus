package config

import (
	"fmt"
	"github.com/go-ini/ini"
	"github.com/WangYihang/Platypus/lib/util/log"
)

var Cfg *ini.File

func init(){
	var err error
	var configPath = "runtime/app.ini"
	log.Info("Loading config file: %s", configPath)
	Cfg, err = ini.Load(configPath)
	if err != nil {	
        log.Error("Fail to read config file: %s, using old config info.", err)
		return
	}
}

func printConfig(cfg *ini.File) {
	fmt.Println("Batch config")
	fmt.Println("\tCommand: ", cfg.Section("Batch").Key("Command").MustString("/bin/cat /flag"))
	fmt.Println("\tRound: ", cfg.Section("Batch").Key("Round").MustInt(300))
	fmt.Println("Report config")
	fmt.Println("\tServer: ", cfg.Section("Report").Key("Server").MustString("http://report.pwnable.cn"))
	fmt.Println("\tCookie: ", cfg.Section("Report").Key("Cookie").MustString("PHPSESSID=d72ja9fsjj2i20d0ashsahjdk21"))
	fmt.Println("\tData: ", cfg.Section("Report").Key("Data").MustString("flag=__FLAG__&token=1Y3nZoOMn66FBRTLYgd5LS52x1itXoJZIDawNtEvjh2PpdDFPtrH8adGXaAcjZuP"))
	fmt.Println("\tAccept: ", cfg.Section("Report").Key("Accept").MustString("accepted"))
	fmt.Println("Crontab config")
	fmt.Println("\tHost: ", cfg.Section("Crontab").Key("Host").MustString("1.3.3.7"))
	fmt.Println("\tPort: ", cfg.Section("Crontab").Key("Port").MustInt(5555))
}