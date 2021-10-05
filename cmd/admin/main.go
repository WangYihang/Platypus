package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/WangYihang/Platypus/cmd/admin/ctx"
	"github.com/WangYihang/Platypus/cmd/admin/meta"
	"github.com/WangYihang/Platypus/internal/util/fs"
	"github.com/WangYihang/Platypus/internal/util/suggest"
	prompt "github.com/c-bata/go-prompt"
	"github.com/c-bata/go-prompt/completer"
	"github.com/google/shlex"
)

type Completer struct {
	namespace string
}

func NewCompleter() (*Completer, error) {
	return &Completer{
		namespace: "admin",
	}, nil
}

func (c *Completer) Complete(d prompt.Document) []prompt.Suggest {
	// User did not type anything
	// eg: ``
	text := d.TextBeforeCursor()
	if strings.TrimSpace(text) == "" {
		return suggest.GetCommandSuggestions()
	}
	// User typed something
	// eg: `prox`
	args := strings.Split(text, " ")
	cmd := args[0]
	if len(args) <= 1 {
		return suggest.GetFuzzyCommandSuggestions(cmd)
	} else {
		// Ensure cmd is a valid cmd
		if !suggest.IsValidCommand(cmd) {
			return []prompt.Suggest{}
		}
		return suggest.GetArgumentsSuggestions(text)
	}
}

func Executor(text string) {
	// Save into history
	AppendHistory(ctx.GetHistoryFilepath(), text)
	// Execute
	arguments, _ := shlex.Split(text)
	if len(arguments) > 0 {
		command := arguments[0]
		if val, ok := suggest.GetMetaCommandsMap()[strings.ToLower(command)]; ok {
			val.(meta.MetaCommand).Execute(arguments)
		}
	}
}

func AppendHistory(path string, content string) {
	fs.AppendFile(path, []byte(content+"\n"))
}

func LoadHistory(path string) []string {
	file, err := os.Open(path)
	if err != nil {
		return []string{}
	}
	scanner := bufio.NewScanner(file)
	scanner.Split(bufio.ScanLines)
	var text []string
	for scanner.Scan() {
		text = append(text, scanner.Text())
	}
	defer file.Close()
	return text
}

func StartCli() {
	c, err := NewCompleter()
	if err != nil {
		fmt.Println("error", err)
		os.Exit(1)
	}
	p := prompt.New(
		Executor,
		c.Complete,
		prompt.OptionTitle("platypus-admin: interactive platypus client"),
		prompt.OptionPrefix(">> "),
		prompt.OptionHistory(LoadHistory(ctx.GetHistoryFilepath())),
		prompt.OptionInputTextColor(prompt.Yellow),
		prompt.OptionCompletionWordSeparator(completer.FilePathCompletionSeparator),
	)
	p.Run()
}

func main() {
	StartCli()
}
