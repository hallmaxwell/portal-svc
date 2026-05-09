//go:build windows

package util

import (
	"fmt"
	"os"
	"strings"
	"syscall"
	"unsafe"
	"golang.org/x/sys/windows"
)

// IsAdmin checks if the current user has Administrator privileges.
func IsAdmin() bool {
	var sid *windows.SID
	err := windows.AllocateAndInitializeSid(
		&windows.SECURITY_NT_AUTHORITY,
		2,
		windows.SECURITY_BUILTIN_DOMAIN_RID,
		windows.DOMAIN_ALIAS_RID_ADMINS,
		0, 0, 0, 0, 0, 0,
		&sid)
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

const (
	SEE_MASK_NOCLOSEPROCESS = 0x00000040
	SW_HIDE                 = 0
)

// SHELLEXECUTEINFO corresponds to the Windows SHELLEXECUTEINFOW structure.
type SHELLEXECUTEINFO struct {
	CbSize       uint32
	FMask        uint32
	Hwnd         syscall.Handle
	LpVerb       *uint16
	LpFile       *uint16
	LpParameters *uint16
	LpDirectory  *uint16
	NShow        int32
	HInstApp     syscall.Handle
	LpIDList     uintptr
	LpClass      *uint16
	HkeyClass    syscall.Handle
	DwHotKey     uint32
	HIcon        syscall.Handle
	HProcess     syscall.Handle
}

var (
	shell32             = syscall.NewLazyDLL("shell32.dll")
	shellExecuteExWProc = shell32.NewProc("ShellExecuteExW")
)

// RunMeElevated attempts to re-run the current executable with Administrator privileges via UAC.
func RunMeElevated() error {
	verbPtr, err := syscall.UTF16PtrFromString("runas")
	if err != nil {
		return err
	}

	exePath, err := os.Executable()
	if err != nil {
		return err
	}
	exePtr, err := syscall.UTF16PtrFromString(exePath)
	if err != nil {
		return err
	}

	var args string
	if len(os.Args) > 1 {
		var quotedArgs []string
		for _, arg := range os.Args[1:] {
			quotedArgs = append(quotedArgs, "\""+arg+"\"")
		}
		args = strings.Join(quotedArgs, " ")
	}

	var argsPtr *uint16
	if args != "" {
		argsPtr, err = syscall.UTF16PtrFromString(args)
		if err != nil {
			return err
		}
	}

	cwd, _ := os.Getwd()
	var cwdPtr *uint16
	if cwd != "" {
		cwdPtr, _ = syscall.UTF16PtrFromString(cwd)
	}

	sei := SHELLEXECUTEINFO{
		FMask:        SEE_MASK_NOCLOSEPROCESS,
		LpVerb:       verbPtr,
		LpFile:       exePtr,
		LpParameters: argsPtr,
		LpDirectory:  cwdPtr,
		NShow:        SW_HIDE,
	}
	sei.CbSize = uint32(unsafe.Sizeof(sei))

	ret, _, errCode := shellExecuteExWProc.Call(uintptr(unsafe.Pointer(&sei)))
	if ret == 0 {
		return fmt.Errorf("ShellExecuteExW failed: %v", errCode)
	}

	if sei.HProcess != 0 {
		syscall.WaitForSingleObject(sei.HProcess, syscall.INFINITE)

		var exitCode uint32
		err = syscall.GetExitCodeProcess(sei.HProcess, &exitCode)
		syscall.CloseHandle(sei.HProcess)
		if err != nil {
			return fmt.Errorf("failed to get exit code: %v", err)
		}

		if exitCode != 0 {
			return fmt.Errorf("elevated process exited with code %d", exitCode)
		}
	}

	return nil
}
