package dispatcher

import (
	"fmt"
)

func (dispatcher CommandDispatcher) UpgradeToMetasploit(args []string) {
	fmt.Println("TO BE IMPLEMENTED.")
}

func (dispatcher CommandDispatcher) UpgradeToMetasploitHelp() string {
	fmt.Println("Usage of UpgradeToMetasploit")
	fmt.Println("\tUpgradeToMetasploit [SRC] [DST]")
	return ""
}

func (dispatcher CommandDispatcher) UpgradeToMetasploitDesc() string {
	fmt.Println("UpgradeToMetasploit")
	fmt.Println("\tUpgrade Platypus session to Metasploit session")
	return ""
}
