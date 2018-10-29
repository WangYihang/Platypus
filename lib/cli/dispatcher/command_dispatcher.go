package dispatcher

import (
	"io"
	"strings"

	"github.com/WangYihang/Platypus/lib/context"
	"github.com/WangYihang/Platypus/lib/util/log"
	"github.com/WangYihang/Platypus/lib/util/reflection"
	"github.com/WangYihang/Platypus/lib/util/str"
	"github.com/chzyer/readline"
)

type Dispatcher struct{}

func parseInput(input string) (string, []string) {
	methods := reflection.GetAllMethods(Dispatcher{})
	args := strings.Split(strings.TrimSpace(input), " ")
	if len(args[0]) == 0 {
		return "", []string{}
	}
	arg0 := str.UpperCaseFirstChar(args[0])
	if !reflection.Contains(methods, arg0) {
		log.Error("No such command, use `Help` to get more information")
		return "", []string{}
	}
	return str.UpperCaseFirstChar(arg0), args[1:]
}

func filterInput(r rune) (rune, bool) {
	switch r {
	case readline.CharCtrlZ:
		return r, false
	}
	return r, true
}

func Run() {
	// Register all commands
	var completer = readline.NewPrefixCompleter()
	children := []readline.PrefixCompleterInterface{}
	methods := reflection.GetAllMethods(Dispatcher{})
	for _, method := range methods {
		if (strings.HasSuffix(method, "Help") && !strings.HasPrefix(method, "Help")) || strings.HasSuffix(method, "Desc") {
			continue
		}
		children = append(children, readline.PcItem(method))
	}
	completer.SetChildren(children)

	// Construct the IO
	l, err := readline.NewEx(&readline.Config{
		Prompt:          context.Ctx.CommandPrompt,
		HistoryFile:     "~/.platypus.history",
		AutoComplete:    completer,
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",
		HistorySearchFold:   true,
		FuncFilterInputRune: filterInput,
	})

	if err != nil {
		panic(err)
	}
	defer l.Close()

	// Command loop
	for {
		line, err := l.Readline()
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
		reflection.Invoke(Dispatcher{}, method, args)
	}
}
