package dispatcher

import (
	"fmt"

	"github.com/WangYihang/Platypus/internal/context"
	"github.com/WangYihang/Platypus/internal/util/log"
	"github.com/WangYihang/Platypus/internal/util/os"
)

func (dispatcher CommandDispatcher) Upgrade(args []string) {
	if len(args) != 1 {
		log.Error("Arguments error, use `Help Upgrade` to get more information")
		dispatcher.UpgradeHelp()
		return
	}

	connectBackAddr := args[0]
	// TODO: Check format: [Dotted Decimal Notation]:[uint16 Port]

	if context.Ctx.Current == nil {
		log.Error("The current client is not set, please use `Jump` to set the current client")
		return
	}

	if context.Ctx.Current.OS != os.Linux {
		log.Error("The operating system of the current client is supported, will be supported soon in the next few releases.")
		return
	}

	context.Ctx.Current.UpgradeToTermite(connectBackAddr)
}

func (dispatcher CommandDispatcher) UpgradeHelp() string {
	fmt.Println("Usage of Upgrade")
	fmt.Println("\tUpgrade [Connect Back Addr]")
	fmt.Println("Example")
	fmt.Println("\tUpgrade 1.3.3.7:13337")
	return ""
}

func (dispatcher CommandDispatcher) UpgradeDesc() string {
	fmt.Println("Upgrade")
	fmt.Println("\tUpgrade Platypus session to encrypted Termite session")
	return ""
}
