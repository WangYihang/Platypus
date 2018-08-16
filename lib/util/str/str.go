package str

import (
	"math/rand"
	"strings"
	"time"
)

func UpperCaseFirstChar(str string) string {
	return strings.ToUpper(str[0:1]) + str[1:]
}

func RandomString(length int) string {
	charset := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, length)
	for i := range result {
		result[i] = charset[rand.New(rand.NewSource(time.Now().UnixNano())).Intn(len(charset))]
	}
	return string(result)
}
