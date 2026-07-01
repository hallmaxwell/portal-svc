package util

import (
	"fmt"
	"os"
)

// FatalError prints the given error message to os.Stderr and exits with code 1.
// It is used to unify duplicated error handling across the codebase.
func FatalError(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
