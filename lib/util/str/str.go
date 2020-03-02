package str

import (
	"math/rand"
	"strings"
)

func UpperCaseFirstChar(str string) string {
	if len(str) == 0 {
		return str
	}
	return strings.ToUpper(str[0:1]) + str[1:]
}

func RandomString(length int) string {
	charset := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}
