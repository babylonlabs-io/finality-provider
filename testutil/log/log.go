package log

import (
	"time"
)

// Add a date prefix to the format string. example usage:
// t.Logf(log.FormatWithDate("Fp home dir: %s"), fpHomeDir)
func Prefix(format string) string {
	currentTime := time.Now().Format("[15:04:05.000]")
	return currentTime + " " + format
}
