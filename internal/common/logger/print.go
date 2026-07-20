package logger

import (
	"fmt"
	"time"
)

const printTSFormat = "2006-01-02 15:04:05"

// Tprintf writes a timestamped line to stdout — use in place of fmt.Printf.
func Tprintf(format string, args ...any) {
	fmt.Printf("["+time.Now().Format(printTSFormat)+"] "+format, args...)
}

// Tprintln writes a timestamped line to stdout — use in place of fmt.Println.
// With no args it emits a blank line (no timestamp), preserving visual spacing.
func Tprintln(args ...any) {
	if len(args) == 0 {
		fmt.Println()
		return
	}
	fmt.Printf("[%s] %s\n", time.Now().Format(printTSFormat), fmt.Sprint(args...))
}
