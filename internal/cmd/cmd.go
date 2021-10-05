package cmd

import (
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
	Help() string
	Description() string
	Arguments() []Argument
	Execute([]string)
	Suggest(name string) []prompt.Suggest
}
