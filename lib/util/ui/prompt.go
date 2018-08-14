package ui

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func PromptYesNo(message string) bool {
	for {
		fmt.Print(fmt.Sprintf("%s [Y/N] ", message))
		inputReader := bufio.NewReader(os.Stdin)
		input, err := inputReader.ReadString('\n')
		if err != nil {
			fmt.Println()
			continue
		}
		if strings.HasPrefix(strings.ToLower(input), "y") {
			return true
		} else if strings.HasPrefix(strings.ToLower(input), "n") {
			return false
		} else {
			continue
		}
	}
}
