//go:build !windows

package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/charmbracelet/huh/spinner"
)

func hasAdminPrivileges() bool {
	return os.Geteuid() == 0
}

func executeWithElevation(action string) {
	if !hasAdminPrivileges() {
		fmt.Println("[ ERROR ] Root privileges are required for this action.")
		fmt.Println("Please run this tool using 'sudo' to perform this action.")
		return
	}

	runServiceCommand(action)
}

func runServiceCommand(action string) {
	exe, err := os.Executable()
	if err != nil {
		fmt.Printf("[ ERROR ] %v\n", err)
		return
	}

	cmd := exec.Command(exe, "dock", action)
	cmd.Dir = filepath.Dir(exe)

	var runErr error
	var cmdOut []byte

	err = spinner.New().
		Type(spinner.Dots).
		Title(fmt.Sprintf("Executing 'dock %s'...", action)).
		Action(func() {
			cmdOut, runErr = cmd.CombinedOutput()
		}).
		Run()

	if err != nil {
		fmt.Printf("[ ERROR ] Failed to render spinner: %v\n", err)
		return
	}

	if runErr != nil {
		fmt.Printf("[ ERROR ] Failed to execute %s: %v\n", action, runErr)
		if len(cmdOut) > 0 {
			fmt.Printf("\nCommand output:\n%s\n", string(cmdOut))
		}
	} else {
		fmt.Printf("[ SUCCESS ] Successfully executed '%s'.\n", action)
		if len(cmdOut) > 0 {
			fmt.Printf("\nOutput:\n%s\n", string(cmdOut))
		}
	}
}
