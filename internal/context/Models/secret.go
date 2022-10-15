package Models

import (
	"crypto/md5"
	"encoding/hex"
)

func EncryptAlg(origData string) string {
	return MD5(origData)
}

// MD5 MD5加密
func MD5(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}
