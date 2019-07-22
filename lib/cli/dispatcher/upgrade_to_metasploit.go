package dispatcher

import (
	"fmt"
)

func (dispatcher Dispatcher) UpgradeToMetasploit(args []string) {
	fmt.Println("TO BE IMPLEMENTED.")
}

func (dispatcher Dispatcher) UpgradeToMetasploitHelp(args []string) {
	fmt.Println("Usage of UpgradeToMetasploit")
	fmt.Println("\tUpgradeToMetasploit [SRC] [DST]")
}

func (dispatcher Dispatcher) UpgradeToMetasploitDesc(args []string) {
	fmt.Println("UpgradeToMetasploit")
	fmt.Println("\tUpgrade Platypus session to Metasploit session")
}
