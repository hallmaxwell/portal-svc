//go:build !windows

package util

import (
	"os"
	"os/exec"
)

// IsAdmin checks if the current user is root.
func IsAdmin() bool {
	return os.Geteuid() == 0
}

// RunMeElevated attempts to re-run the current executable with sudo.
func RunMeElevated() error {
	cmd := exec.Command("sudo", os.Args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
