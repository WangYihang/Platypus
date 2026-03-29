package cmd

import (
	"io"
	"strings"

	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/log"
	"github.com/WangYihang/readline"
	"github.com/google/shlex"
	"github.com/spf13/cobra"
)

var readLineInstance *readline.Instance

var rootCmd = &cobra.Command{
	Use:   "platypus",
	Short: "Platypus reverse shell manager",
	// Silence cobra's built-in error/usage printing — we handle it in the REPL
	SilenceErrors: true,
	SilenceUsage:  true,
}

func init() {
	// Register all commands
	rootCmd.AddCommand(
		exitCmd,
		listCmd,
		jumpCmd,
		commandCmd,
		aliasCmd,
		infoCmd,
		gatherCmd,
		deleteCmd,
		ptyCmd,
		interactCmd,
		downloadCmd,
		uploadCmd,
		runCmd,
		restCmd,
		switchingCmd,
		turnCmd,
		tunnelCmd,
		upgradeCmd,
		upgradeToMetasploitCmd,
		dataDispatcherCmd,
	)
}

func filterInput(r rune) (rune, bool) {
	switch r {
	case readline.CharCtrlZ:
		return r, false
	}
	return r, true
}

// RunREPL starts the interactive command loop using cobra for dispatch.
func RunREPL() {
	// Build tab completion from cobra command tree
	completer := readline.NewPrefixCompleter()
	children := []readline.PrefixCompleterInterface{}
	for _, cmd := range rootCmd.Commands() {
		children = append(children, readline.PcItem(cmd.Name()))
	}
	completer.SetChildren(children)

	var err error
	readLineInstance, err = readline.NewEx(&readline.Config{
		Prompt:              core.Ctx.CommandPrompt,
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

	core.Ctx.RLInstance = readLineInstance
	log.Logger.SetOutput(readLineInstance.Stderr())

	for {
		line, err := readLineInstance.Readline()
		if err == readline.ErrInterrupt {
			if len(line) == 0 {
				break
			}
			continue
		} else if err == io.EOF {
			break
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		args, err := shlex.Split(line)
		if err != nil {
			log.Error("Parse error: %s", err)
			continue
		}

		rootCmd.SetArgs(args)
		if err := rootCmd.Execute(); err != nil {
			log.Error("%s", err)
		}
	}
}
