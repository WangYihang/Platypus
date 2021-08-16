package dispatcher

import (
	"fmt"
)

func (dispatcher commandDispatcher) UpgradeToMetasploit(args []string) {
	fmt.Println("TO BE IMPLEMENTED.")
}

func (dispatcher commandDispatcher) UpgradeToMetasploitHelp(args []string) {
	fmt.Println("Usage of UpgradeToMetasploit")
	fmt.Println("\tUpgradeToMetasploit [SRC] [DST]")
}

func (dispatcher commandDispatcher) UpgradeToMetasploitDesc(args []string) {
	fmt.Println("UpgradeToMetasploit")
	fmt.Println("\tUpgrade Platypus session to Metasploit session")
}
