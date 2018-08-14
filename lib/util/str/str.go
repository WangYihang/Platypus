package str

import "strings"

func UpperCaseFirstChar(str string) string {
	return strings.ToUpper(str[0:1]) + str[1:]
}
