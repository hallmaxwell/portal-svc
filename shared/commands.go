package shared

import (
	"errors"
	"flag"
	"fmt"
	"os"
)

// HandleFlagError handles flag parsing errors, gracefully exiting on help or throwing an error.
func HandleFlagError(err error) {
	if errors.Is(err, flag.ErrHelp) {
		os.Exit(0)
	} else if err != nil {
		os.Exit(1)
	}
}

// FatalError prints an error message to os.Stderr and exits with code 1.
func FatalError(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

// CheckError exits fatally if err is not nil.
func CheckError(err error, format string, args ...any) {
	if err != nil {
		if len(args) > 0 {
			FatalError(format, append(args, err)...)
		} else {
			FatalError(format, err)
		}
	}
}
