package ui

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/WangYihang/Platypus/lib/util/log"
)

func PromptYesNo(message string) bool {
	fmt.Println(fmt.Sprintf("%s [Y/N]", message))
	inputReader := bufio.NewReader(os.Stdin)
	input, err := inputReader.ReadString('\n')
	if err != nil {
		log.Error("Read from stdin failed")
	}
	if strings.HasPrefix(strings.ToLower(input), "y") {
		return true
	} else if strings.HasPrefix(strings.ToLower(input), "n") {
		return false
	} else {
		return PromptYesNo(message)
	}
}
