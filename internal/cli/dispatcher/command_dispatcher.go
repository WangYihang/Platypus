package dispatcher

import (
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"

	"github.com/WangYihang/Platypus/internal/context"
	"github.com/WangYihang/Platypus/internal/util/log"
	"github.com/WangYihang/readline"
	"github.com/c-bata/go-prompt"
	"github.com/google/shlex"
)

type Argument struct {
	Name        string
	Desc        string
	IsFlag      bool
	AllowRepeat bool
	IsRequired  bool
	Default     interface{}
	SuggestFunc func(name string) []prompt.Suggest
}

type CommandDispatcher struct{}

// Provide tab autocompletion features
var readLineInstance *readline.Instance

func system(command string) (string, string, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	os := runtime.GOOS
	switch os {
	case "windows":
		cmd := exec.Command("cmd", "/C", command)
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		return stdout.String(), stderr.String(), err
	case "darwin":
		cmd := exec.Command("/bin/sh", "-c", command)
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		return stdout.String(), stderr.String(), err
	case "linux":
		cmd := exec.Command("/bin/sh", "-c", command)
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		return stdout.String(), stderr.String(), err
	default:
		return "", "", fmt.Errorf("unsupported OS type: %s", os)
	}
}

func parseInput(input string) (string, []string) {
	args, err := shlex.Split(input)

	if err != nil {
		log.Error(err.Error())
		return "", []string{}
	}

	if len(args) == 0 {
		return "", []string{}
	}

	target := ""
	// methods := reflection.GetAllMethods(CommandDispatcher{})
	// for _, method := range methods {
	// 	if strings.EqualFold(method, args[0]) {
	// 		target = method
	// 		break
	// 	}
	// }

	if target == "" {
		log.Error("No such command, use `Help` to get more information")
		stdout, stderr, _ := system(input)
		log.Info("Executing locally: %s", input)
		fmt.Printf("%s", stdout)
		fmt.Printf("%s", stderr)
		return "", []string{}
	}
	return target, args[1:]
}

func filterInput(r rune) (rune, bool) {
	switch r {
	case readline.CharCtrlZ:
		return r, false
	}
	return r, true
}

// REPL performs read / evaluate / print repeat
func REPL() {
	// Register all commands
	completer := readline.NewPrefixCompleter()
	// children := []readline.PrefixCompleterInterface{}
	// methods := reflection.GetAllMethods(CommandDispatcher{})
	// for _, method := range methods {
	// 	if (strings.HasSuffix(method, "Help") && !strings.HasPrefix(method, "Help")) || strings.HasSuffix(method, "Desc") {
	// 		continue
	// 	}
	// 	children = append(children, readline.PcItem(method))
	// }
	// completer.SetChildren(children)

	// Construct the IO
	var err error
	readLineInstance, err = readline.NewEx(&readline.Config{
		Prompt:              context.Ctx.CommandPrompt,
		HistoryFile:         "/tmp/platypus.history",
		AutoComplete:        completer,
		InterruptPrompt:     "^C",
		EOFPrompt:           "exit",
		HistorySearchFold:   true,
		FuncFilterInputRune: filterInput,
	})

	if err != nil {
		log.Error(err.Error())
		return
	}

	context.Ctx.RLInstance = readLineInstance

	log.Logger.SetOutput(readLineInstance.Stderr())

	// Command loop
	for {
		line, err := readLineInstance.Readline()
		if err == readline.ErrInterrupt {
			if len(line) == 0 {
				break
			} else {
				continue
			}
		} else if err == io.EOF {
			break
		}
		line = strings.TrimSpace(line)
		method, args := parseInput(line)
		if method == "" {
			continue
		}
		log.Info("%v %v %v", CommandDispatcher{}, method, args)
	}
}
