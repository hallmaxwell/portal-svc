//go:build windows

package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/charmbracelet/huh/spinner"
	"golang.org/x/sys/windows"
)

func hasAdminPrivileges() bool {
	var sid *windows.SID
	err := windows.AllocateAndInitializeSid(
		&windows.SECURITY_NT_AUTHORITY,
		2,
		windows.SECURITY_BUILTIN_DOMAIN_RID,
		windows.DOMAIN_ALIAS_RID_ADMINS,
		0, 0, 0, 0, 0, 0,
		&sid,
	)
	if err != nil {
		return false
	}
	defer windows.FreeSid(sid)

	token := windows.Token(0)
	member, err := token.IsMember(sid)
	if err != nil {
		return false
	}
	return member
}

func runElevated(args []string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}

	// Because windows.ShellExecuteEx is not available directly in x/sys/windows
	// we will use powershell Start-Process -Wait as an alternative
	// or fallback to ShellExecute which is asynchronous.

	// Create arguments string
	var argsStr string
	for _, arg := range args {
		argsStr += fmt.Sprintf(`"%s" `, arg)
	}

	psCmd := fmt.Sprintf(`Start-Process -FilePath "%s" -ArgumentList '%s' -Verb RunAs -Wait`, exe, argsStr)

	var execErr error
	err = spinner.New().
		Type(spinner.Dots).
		Title("Requesting administrative privileges...").
		Action(func() {
			cmd := exec.Command("powershell.exe", "-NoProfile", "-WindowStyle", "Hidden", "-Command", psCmd)
			execErr = cmd.Run()
		}).
		Run()

	if err != nil {
		return err
	}
	return execErr
}

func executeWithElevation(action string) {
    if !hasAdminPrivileges() {
        fmt.Println("Administrative privileges are required for this action.")
        err := runElevated([]string{"dock", action})
        if err != nil {
            fmt.Printf("Elevation failed: %v\n", err)
        } else {
            fmt.Println("Launched elevated process successfully.")
        }
        return
    }

    // Already admin, run locally
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
