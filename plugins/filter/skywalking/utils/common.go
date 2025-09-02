package utils

import "time"

func CurrentTimeMillis() int64 {
	// Returns the current time in milliseconds since epoch
	return time.Now().UnixNano() / 1e6
}
