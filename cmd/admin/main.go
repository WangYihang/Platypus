package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/WangYihang/Platypus/cmd/admin/ctx"
	"github.com/WangYihang/Platypus/cmd/admin/meta"
	"github.com/WangYihang/Platypus/internal/util/fs"
	"github.com/WangYihang/Platypus/internal/util/log"
	"github.com/WangYihang/Platypus/internal/util/suggest"
	prompt "github.com/c-bata/go-prompt"
	"github.com/google/shlex"
)

type Completer struct{}

func NewCompleter() (*Completer, error) {
	return &Completer{}, nil
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
		return suggest.GetArgumentsSuggestions(text, d.TextAfterCursor())
	}
}

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

func Executor(text string) {
	// Save into history
	AppendHistory(ctx.GetHistoryFilepath(), text)
	// Execute
	arguments, _ := shlex.Split(text)
	if len(arguments) > 0 {
		command := arguments[0]
		if val, ok := suggest.GetMetaCommandsMap()[strings.ToLower(command)]; ok {
			val.(meta.MetaCommand).Execute(arguments)
		} else {
			log.Error("No such command, use <TAB> to auto complete available commands")
			stdout, stderr, _ := system(text)
			log.Info("Executing locally: %s", text)
			fmt.Printf("%s", stdout)
			fmt.Printf("%s", stderr)
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
	ctx.SaveTermState()
	c, err := NewCompleter()
	if err != nil {
		fmt.Println("error", err)
		os.Exit(1)
	}
	p := prompt.New(
		Executor,
		c.Complete,
		prompt.OptionHistory(LoadHistory(ctx.GetHistoryFilepath())),
	)
	p.Run()
}

func main() {
	StartCli()
}
