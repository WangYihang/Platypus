package meta

import (
	"github.com/WangYihang/Platypus/internal/util/log"
	"github.com/akamensky/argparse"
	"github.com/c-bata/go-prompt"
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

type MetaCommand interface {
	Name() string
	Help() string
	Description() string
	Arguments() []Argument
	Execute([]string)
	Suggest(name string) []prompt.Suggest
}

func ParseArguments(command MetaCommand, args []string) (map[string]interface{}, error) {
	parser := argparse.NewParser(command.Name(), command.Description())
	parser.Command.ExitOnHelp(false)
	result := make(map[string]interface{})
	for _, argument := range command.Arguments() {
		if argument.IsFlag {
			result[argument.Name] = parser.Flag("", argument.Name, &argparse.Options{Required: argument.IsRequired, Help: argument.Desc})
		} else {
			result[argument.Name] = parser.String("", argument.Name, &argparse.Options{Required: argument.IsRequired, Help: argument.Desc})
		}
	}
	err := parser.Parse(args)
	if err != nil {
		log.Error(err.Error())
		return nil, err
	}
	return result, nil
}
