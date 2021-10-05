package suggest

import (
	"strings"

	"github.com/WangYihang/Platypus/cmd/admin/meta"
	"github.com/WangYihang/Platypus/cmd/admin/meta/auth"
	"github.com/WangYihang/Platypus/cmd/admin/meta/connect"
	"github.com/WangYihang/Platypus/cmd/admin/meta/run"
	"github.com/c-bata/go-prompt"
	"github.com/google/shlex"
	"github.com/lithammer/fuzzysearch/fuzzy"
)

func GetMetaCommandsMap() map[string]interface{} {
	return map[string]interface{}{
		"auth":    auth.Command{},
		"connect": connect.Command{},
		"run":     run.Command{},
	}
}

func GetCommandSuggestions() []prompt.Suggest {
	var suggests []prompt.Suggest
	for name, command := range GetMetaCommandsMap() {
		suggest := prompt.Suggest{Text: name, Description: command.(meta.MetaCommand).Description()}
		suggests = append(suggests, suggest)
	}
	return suggests
}

func IsValidCommand(command string) bool {
	for name := range GetMetaCommandsMap() {
		if strings.EqualFold(command, name) {
			return true
		}
	}
	return false
}

func GetFuzzyCommandSuggestions(pattern string) []prompt.Suggest {
	var suggests []prompt.Suggest
	for name, command := range GetMetaCommandsMap() {
		if fuzzy.MatchFold(pattern, name) {
			suggest := prompt.Suggest{Text: name, Description: command.(meta.MetaCommand).Description()}
			suggests = append(suggests, suggest)
		}
	}
	return suggests
}

func GetPreconfiguredArguments(command string) []meta.Argument {
	if val, ok := GetMetaCommandsMap()[strings.ToLower(command)]; ok {
		return val.(meta.MetaCommand).Arguments()
	}
	return []meta.Argument{}
}

func GetArgumentsSuggestions(text string) []prompt.Suggest {
	var suggests []prompt.Suggest
	args, _ := shlex.Split(text)

	if strings.HasSuffix(text, " ") {
		args = append(args, "")
	}

	command := args[0]
	previousArgument := args[len(args)-1]
	preconfiguredArguments := GetPreconfiguredArguments(command)

	// Mode: Value suggestion
	if len(args) > 1 {
		previousPreviousArgument := args[len(args)-2]
		for _, a := range preconfiguredArguments {
			if "--"+a.Name == previousPreviousArgument && !a.IsFlag && a.SuggestFunc != nil {
				suggests = append(suggests, a.SuggestFunc(a.Name)...)
				return suggests
			}
		}
	}

	// Mode: Argument suggestion
	// eg: `--host 0.0.0.0 -`
	for _, a := range preconfiguredArguments {
		found := false
		for _, arg := range args[1:] {
			if "--"+a.Name == arg {
				if a.AllowRepeat {
					// Arguments which is appeared and allow repeating
					suggest := prompt.Suggest{Text: "--" + a.Name, Description: a.Desc}
					suggests = append(suggests, suggest)
				}
				found = true
			}
		}
		if !found {
			// Arguments which is not appeared
			if strings.Trim(previousArgument, " ") == "" {
				suggest := prompt.Suggest{Text: "--" + a.Name, Description: a.Desc}
				suggests = append(suggests, suggest)
			} else {
				if fuzzy.MatchFold(strings.Trim(previousArgument, "- "), a.Name) {
					suggest := prompt.Suggest{Text: "--" + a.Name, Description: a.Desc}
					suggests = append(suggests, suggest)
				}
			}
		}
	}

	return suggests
}
