package shared

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"portal-svc/ui"
	"runtime"
	"strings"
	"sync"
	"time"

	"portal-svc/util"

	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	InfoLogFilePath  string
	ErrorLogFilePath string

	infoLogger  *lumberjack.Logger
	errorLogger *lumberjack.Logger
	logMu       sync.Mutex
)

// ExecutableDir returns the directory containing the currently executing binary.
func ExecutableDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", ui.NewAppError("EXEC_PATH_ERR", "Failed to get executable path", err.Error(), ui.SeverityFatal, err)
	}
	return filepath.Dir(exe), nil
}

// SingBoxBinaryName returns the platform-specific name for sing-box.
func SingBoxBinaryName() string {
	if runtime.GOOS == "windows" {
		return "sing-box.exe"
	}
	return "sing-box"
}

// BundledSingBoxPath returns the expected path to the bundled sing-box binary.
func BundledSingBoxPath(baseDir string) string {
	return filepath.Join(baseDir, "core", SingBoxBinaryName())
}

// ProcessEnvMap parses os.Environ into a map.
func ProcessEnvMap() map[string]string {
	envMap := make(map[string]string)
	for _, env := range os.Environ() {
		pair := strings.SplitN(env, "=", 2)
		if len(pair) == 2 {
			envMap[pair[0]] = pair[1]
		}
	}
	return envMap
}

// InitLogPaths sets up the file paths for logging.
func InitLogPaths() error {
	baseDir, err := ExecutableDir()
	if err != nil {
		return err
	}
	logsDir := filepath.Join(baseDir, "logs")

	InfoLogFilePath = filepath.Join(logsDir, "access.log")
	ErrorLogFilePath = filepath.Join(logsDir, "error.log")
	return nil
}

// InitLogFiles initializes the lumberjack loggers.
func InitLogFiles() error {
	if err := InitLogPaths(); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(InfoLogFilePath), 0700); err != nil {
		return ui.NewAppError("DIR_CREATE_ERR", "Failed to create logs directory", err.Error(), ui.SeverityFatal, err)
	}

	infoLogger = &lumberjack.Logger{
		Filename:   InfoLogFilePath,
		MaxSize:    10, // megabytes
		MaxBackups: 5,
		MaxAge:     28, // days
		Compress:   true,
	}

	errorLogger = &lumberjack.Logger{
		Filename:   ErrorLogFilePath,
		MaxSize:    10, // megabytes
		MaxBackups: 5,
		MaxAge:     28, // days
		Compress:   true,
	}

	// Always clear logs on start
	if err := infoLogger.Rotate(); err != nil {
		return ui.NewAppError("LOG_ROTATE_ERR", "Failed to rotate access log", err.Error(), ui.SeverityError, err)
	}
	if err := errorLogger.Rotate(); err != nil {
		return ui.NewAppError("LOG_ROTATE_ERR", "Failed to rotate error log", err.Error(), ui.SeverityError, err)
	}

	// Ensure empty logs exist
	if err := os.WriteFile(InfoLogFilePath, []byte(""), 0600); err != nil {
		return ui.NewAppError("LOG_INIT_ERR", "Failed to initialize access log", err.Error(), ui.SeverityFatal, err)
	}
	if err := os.WriteFile(ErrorLogFilePath, []byte(""), 0600); err != nil {
		return ui.NewAppError("LOG_INIT_ERR", "Failed to initialize error log", err.Error(), ui.SeverityFatal, err)
	}

	return nil
}

// WriteLog writes a formatted log message.
func WriteLog(level, prefix, msg string, printToStdout bool) {
	if infoLogger == nil || errorLogger == nil {
		return
	}

	logMu.Lock()
	defer logMu.Unlock()

	timestamp := time.Now().Format("2006/01/02 15:04:05")
	logLine := fmt.Sprintf("%s %s %s\n", timestamp, prefix, msg)

	if _, err := infoLogger.Write([]byte(logLine)); err != nil && printToStdout {
		fmt.Fprintf(os.Stderr, "failed to write access log: %v\n", err)
	}

	if level == "error" {
		if _, err := errorLogger.Write([]byte(logLine)); err != nil && printToStdout {
			fmt.Fprintf(os.Stderr, "failed to write error log: %v\n", err)
		}
	}

	if printToStdout {
		if level == "error" {
			if _, err := os.Stderr.WriteString(prefix + " " + msg + "\n"); err != nil {
				return
			}
		} else {

		}
	}
}

// SysLogInfo logs an info level message.
func SysLogInfo(msg string, printToStdout bool) {
	WriteLog("info", "service:", msg, printToStdout)
}

// SysLogError logs an error level message.
func SysLogError(msg string, printToStdout bool) {
	WriteLog("error", "service:", msg, printToStdout)
}

// SingBoxLogWriter is an io.Writer that processes sing-box logs.
type SingBoxLogWriter struct {
	PrintToStdout bool
}

func (w *SingBoxLogWriter) Write(p []byte) (n int, err error) {
	lines := strings.Split(strings.TrimSuffix(string(p), "\n"), "\n")
	for _, l := range lines {
		if util.IsNotBlank(l) {
			level := "info"
			if util.ContainsCaseInsensitive(l, "error") || util.ContainsCaseInsensitive(l, "fatal") {
				level = "error"
			}
			WriteLog(level, "sing-box:", l, w.PrintToStdout)
		}
	}
	return len(p), nil
}

// KillExistingSingBox tries to forcefully kill any running sing-box process.
func KillExistingSingBox() {
	var err error
	if runtime.GOOS == "windows" {
		err = exec.Command("taskkill", "/F", "/T", "/IM", "sing-box.exe").Run()
	} else {
		err = exec.Command("killall", "-9", "sing-box").Run()
	}
	if err != nil {
		SysLogInfo(fmt.Sprintf("No existing sing-box process stopped: %v", err), false)
	}
}

// EnsureBundledSingBox verifies the presence of the sing-box binary.
func EnsureBundledSingBox(baseDir string) error {
	singBoxPath := BundledSingBoxPath(baseDir)
	if _, err := os.Stat(singBoxPath); err != nil {
		return ui.NewAppError("SING_BOX_MISSING", fmt.Sprintf("sing-box executable not found at %s", singBoxPath), "", ui.SeverityFatal, nil)
	}
	return nil
}

// RecentNonBlankLines returns the last maxLines from a file, skipping blank lines.
func RecentNonBlankLines(path string, maxLines int) (string, error) {
	if maxLines <= 0 {
		return "", nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	start := 0
	if len(lines) > maxLines {
		start = len(lines) - maxLines
	}

	var b strings.Builder
	for i := start; i < len(lines); i++ {
		if util.IsNotBlank(lines[i]) {
			b.WriteString(lines[i])
			b.WriteByte('\n')
		}
	}
	return b.String(), nil
}

// RecordSystemStatus logs current network interfaces when an error occurs.
func RecordSystemStatus() {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("netsh", "interface", "show", "interface")
	} else {
		cmd = exec.Command("ip", "link", "show")
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		SysLogError(fmt.Sprintf("Failed to record system status: %v", err), false)
		return
	}
	SysLogError(fmt.Sprintf("System Status at crash:\n%s", string(out)), false)
}

// CaptureElevatedOut is used to redirect stdout/stderr if the CLI was re-invoked elevated with hidden window output redirection.
func CaptureElevatedOut() {
	for i, arg := range os.Args {
		if strings.HasPrefix(arg, "--elevated-out=") {
			outFile := strings.TrimPrefix(arg, "--elevated-out=")
			f, err := os.OpenFile(outFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
			if err == nil {
				os.Stdout = f
				os.Stderr = f
				log.SetOutput(f)
			} else {
				log.Printf("Failed to redirect elevated output to %s: %v\n", outFile, err)
			}
			os.Args = append(os.Args[:i], os.Args[i+1:]...)
			break
		}
	}
}
