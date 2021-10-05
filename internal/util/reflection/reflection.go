package reflection

import (
	"reflect"
	"strings"

	"github.com/WangYihang/Platypus/internal/cli/dispatcher"
	"github.com/c-bata/go-prompt"
	"github.com/google/shlex"
	"github.com/lithammer/fuzzysearch/fuzzy"
)

func Invoke(any interface{}, name string, args ...interface{}) []reflect.Value {
	params := make([]reflect.Value, len(args))
	for i, _ := range args {
		params[i] = reflect.ValueOf(args[i])
	}
	return reflect.ValueOf(any).MethodByName(name).Call(params)
}

func GetAllMethods(any interface{}) []string {
	var methods []string
	anyType := reflect.TypeOf(any)
	for i := 0; i < anyType.NumMethod(); i++ {
		method := anyType.Method(i)
		methods = append(methods, method.Name)
	}
	return methods
}

func GetCommandSuggestions(any interface{}) []prompt.Suggest {
	var suggests []prompt.Suggest
	anyType := reflect.TypeOf(any)
	for i := 0; i < anyType.NumMethod(); i++ {
		method := anyType.Method(i)
		methodName := method.Name
		descMethodName := methodName + "Desc"
		descMethod := reflect.ValueOf(any).MethodByName(descMethodName)
		if descMethod.Kind() != reflect.Func {
			continue
		}
		result := descMethod.Call(nil)
		if len(result) == 1 && result[0].Kind() == reflect.String {
			suggest := prompt.Suggest{Text: methodName, Description: result[0].String()}
			suggests = append(suggests, suggest)
		}
	}
	return suggests
}

func contains(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}
	return false
}

func IsValidCmd(any interface{}, cmd string) bool {
	return contains(GetAllMethods(any), cmd)
}

func GetFuzzyCommandSuggestions(any interface{}, pattern string) []prompt.Suggest {
	var suggests []prompt.Suggest

	commands := GetAllMethods(any)
	matches := fuzzy.FindFold(pattern, commands)

	anyType := reflect.TypeOf(any)
	for i := 0; i < anyType.NumMethod(); i++ {
		method := anyType.Method(i)
		methodName := method.Name
		if contains(matches, methodName) {
			descMethodName := methodName + "Desc"
			descMethod := reflect.ValueOf(any).MethodByName(descMethodName)
			if descMethod.Kind() != reflect.Func {
				continue
			}
			result := descMethod.Call(nil)
			if len(result) == 1 && result[0].Kind() == reflect.String {
				suggest := prompt.Suggest{Text: methodName, Description: result[0].String()}
				suggests = append(suggests, suggest)
			}
		}
	}
	return suggests
}
func GetPreconfiguredArguments(any interface{}, cmd string) []dispatcher.Argument {
	arguments := []dispatcher.Argument{}
	argumentsMethodName := cmd + "Arguments"
	descMethod := reflect.ValueOf(any).MethodByName(argumentsMethodName)
	if descMethod.Kind() == reflect.Func {
		result := descMethod.Call(nil)
		if len(result) == 1 && result[0].Kind() == reflect.Slice {
			elements := result[0]
			for i := 0; i < elements.Len(); i++ {
				element := elements.Index(i)
				argument := dispatcher.Argument{
					Name:        element.FieldByName("Name").String(),
					Desc:        element.FieldByName("Desc").String(),
					IsFlag:      element.FieldByName("IsFlag").Bool(),
					AllowRepeat: element.FieldByName("AllowRepeat").Bool(),
					IsRequired:  element.FieldByName("IsRequired").Bool(),
					Default:     element.FieldByName("Default").Interface(),
					SuggestFunc: element.FieldByName("SuggestFunc").Interface().(func(name string) []prompt.Suggest),
				}
				arguments = append(arguments, argument)
			}
		}
	}
	return arguments
}

func GetArgumentsSuggestions(any interface{}, text string) []prompt.Suggest {
	var suggests []prompt.Suggest
	args, _ := shlex.Split(text)

	if strings.HasSuffix(text, " ") {
		args = append(args, "")
	}

	cmd := args[0]
	previousArgument := args[len(args)-1]
	preconfiguredArguments := GetPreconfiguredArguments(any, cmd)

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

func Contains(target interface{}, obj interface{}) bool {
	targetValue := reflect.ValueOf(target)
	switch reflect.TypeOf(target).Kind() {
	case reflect.Slice, reflect.Array:
		for i := 0; i < targetValue.Len(); i++ {
			if targetValue.Index(i).Interface() == obj {
				return true
			}
		}
	case reflect.Map:
		if targetValue.MapIndex(reflect.ValueOf(obj)).IsValid() {
			return true
		}
	}
	return false
}

func IContains(target interface{}, obj interface{}) bool {
	targetValue := reflect.ValueOf(target)
	switch reflect.TypeOf(target).Kind() {
	case reflect.Slice, reflect.Array:
		for i := 0; i < targetValue.Len(); i++ {
			if targetValue.Index(i).Interface() == obj {
				return true
			}
		}
	case reflect.Map:
		if targetValue.MapIndex(reflect.ValueOf(obj)).IsValid() {
			return true
		}
	}
	return false
}
