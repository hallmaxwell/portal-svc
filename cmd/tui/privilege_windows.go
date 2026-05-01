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

	verb := windows.StringToUTF16Ptr("runas")
	file := windows.StringToUTF16Ptr(exe)

	// Create arguments string
	var argsStr string
	for _, arg := range args {
		argsStr += fmt.Sprintf(`"%s" `, arg)
	}
	argsPtr := windows.StringToUTF16Ptr(argsStr)

	cwd, _ := os.Getwd()
	cwdPtr := windows.StringToUTF16Ptr(cwd)

	var execErr error

	err = spinner.New().
		Title("Requesting administrative privileges...").
		Action(func() {
			var sei windows.SHELLEXECUTEINFO
			sei.CbSize = uint32(windows.SizeofSHELLEXECUTEINFO)
			sei.FMask = windows.SEE_MASK_NOCLOSEPROCESS
			sei.Hwnd = 0
			sei.LpVerb = verb
			sei.LpFile = file
			sei.LpParameters = argsPtr
			sei.LpDirectory = cwdPtr
			sei.NShow = windows.SW_NORMAL

			execErr = windows.ShellExecuteEx(&sei)
			if execErr != nil {
				return
			}
			if sei.HProcess != 0 {
				windows.WaitForSingleObject(sei.HProcess, windows.INFINITE)
				windows.CloseHandle(sei.HProcess)
			}
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
		fmt.Printf("Error: %v\n", err)
		return
	}

	cmd := exec.Command(exe, "dock", action)
	cmd.Dir = filepath.Dir(exe)

	var runErr error
	var cmdOut []byte

	err = spinner.New().
		Title(fmt.Sprintf("Executing 'dock %s'...", action)).
		Action(func() {
			cmdOut, runErr = cmd.CombinedOutput()
		}).
		Run()

	if err != nil {
		fmt.Printf("Failed to render spinner: %v\n", err)
		return
	}

	if runErr != nil {
		fmt.Printf("Failed to execute %s: %v\n", action, runErr)
		if len(cmdOut) > 0 {
			fmt.Printf("\nCommand output:\n%s\n", string(cmdOut))
		}
	} else {
		fmt.Printf("Successfully executed '%s'.\n", action)
		if len(cmdOut) > 0 {
			fmt.Printf("\nOutput:\n%s\n", string(cmdOut))
		}
	}
}
