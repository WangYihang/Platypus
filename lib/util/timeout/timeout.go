package timeout

import (
	"time"
)

func GenerateTimeout() time.Duration {
	return time.Microsecond * 0x10
}
