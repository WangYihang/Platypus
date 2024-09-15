package hash

import (
	"crypto/md5"
	"encoding/hex"
)

func MD5(data string) string {
	m := md5.New()
	m.Write([]byte(data))
	return hex.EncodeToString(m.Sum(nil))
}
