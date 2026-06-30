package util

import (
	"os"
	"os/exec"
	"runtime"
)

// OpenFileInEditor attempts to open the specified file using the default system editor/viewer.
func OpenFileInEditor(filePath string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "windows":
		// 'start' is a cmd internal command
		cmd = exec.Command("cmd", "/c", "start", filePath)
	case "darwin":
		cmd = exec.Command("open", filePath)
	default: // linux, freebsd, etc.
		cmd = exec.Command("xdg-open", filePath)
		// Fallback to nano if xdg-open is not available (common in headless/SSH environments)
		if err := cmd.Run(); err != nil {
			cmd = exec.Command("nano", filePath)
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
		} else {
			return nil
		}
	}

	return cmd.Run()
}
