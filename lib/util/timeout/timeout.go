package timeout

import (
	"time"
)

func GenerateTimeout() time.Duration {
	return time.Millisecond * 0x100
}
