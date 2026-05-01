package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"hawego/portal/util"
)

var (
	infoLogFilePath  = filepath.Join(os.TempDir(), "portal_svc_info.log")
	errorLogFilePath = filepath.Join(os.TempDir(), "portal_svc_error.log")

	infoLogger  *boundedLogger
	errorLogger *boundedLogger
	logMu       sync.Mutex
)

type boundedLogger struct {
	filePath string
	maxLines int
	lines    []string
	loaded   bool
}

func initLogFiles() {
	_ = os.WriteFile(infoLogFilePath, []byte(""), 0666)
	_ = os.WriteFile(errorLogFilePath, []byte(""), 0666)

	infoLogger = &boundedLogger{filePath: infoLogFilePath, maxLines: 1000}
	errorLogger = &boundedLogger{filePath: errorLogFilePath, maxLines: 1000}
}

func appendToLog(logger *boundedLogger, lines []string) {
	logMu.Lock()
	defer logMu.Unlock()

	if logger == nil {
		return
	}

	if !logger.loaded {
		data, err := os.ReadFile(logger.filePath)
		if err == nil {
			fileLines := strings.Split(string(data), "\n")
			for _, l := range fileLines {
				if len(strings.TrimSpace(l)) > 0 {
					logger.lines = append(logger.lines, l)
				}
			}
		}
		logger.loaded = true
	}

	logger.lines = append(logger.lines, lines...)

	if len(logger.lines) > logger.maxLines {
		logger.lines = logger.lines[len(logger.lines)-logger.maxLines:]
	}

	outData := strings.Join(logger.lines, "\n") + "\n"
	_ = os.WriteFile(logger.filePath, []byte(outData), 0666)
}

func writeLog(level, prefix, msg string) {
	if infoLogger == nil || errorLogger == nil {
		return
	}
	timestamp := time.Now().Format("2006/01/02 15:04:05")
	logLine := fmt.Sprintf("%s %s %s", timestamp, prefix, msg)

	lines := []string{logLine}

	appendToLog(infoLogger, lines)

	if level == "error" {
		appendToLog(errorLogger, lines)
	}
}

func sysLogInfo(msg string) {
	writeLog("info", "service:", msg)
	fmt.Println("service: " + msg)
}

func sysLogError(msg string) {
	writeLog("error", "service:", msg)
	os.Stderr.WriteString("service: " + msg + "\n")
}

type singBoxLogWriter struct {
	isStderr bool
}

func (w *singBoxLogWriter) Write(p []byte) (n int, err error) {
	lines := strings.Split(strings.TrimSuffix(string(p), "\n"), "\n")
	for _, l := range lines {
		if len(strings.TrimSpace(l)) > 0 {
			level := "info"
			lowerL := strings.ToLower(l)
			if strings.Contains(lowerL, "error") || strings.Contains(lowerL, "fatal") || w.isStderr {
				level = "error"
				os.Stderr.WriteString("sing-box: " + l + "\n")
			} else {
				fmt.Println("sing-box: " + l)
			}
			writeLog(level, "sing-box:", l)
		}
	}
	return len(p), nil
}

func main() {
	initLogFiles()
	data, err := os.ReadFile("/transit_config.tmpl.json")
	if err != nil {
		sysLogError(fmt.Sprintf("Failed to read config template: %v", err))
		os.Exit(1)
	}
	content := string(data)


	var replacements []string
	for _, env := range os.Environ() {
		pair := strings.SplitN(env, "=", 2)
		if len(pair) != 2 {
			continue
		}

		key, val := pair[0], strings.Trim(strings.TrimSpace(pair[1]), `"'`)

		if !strings.Contains(content, "{"+key+"}") {
			continue
		}

		if util.IsRawJSONValue(val) {
			
			content = strings.ReplaceAll(content, `"{`+key+`}"`, val)

			content = strings.ReplaceAll(content, `{`+key+`}`, val)
		} else {
			replacements = append(replacements, `{`+key+`}`, val)
		}
	}

	if len(replacements) > 0 {
		replacer := strings.NewReplacer(replacements...)
		content = replacer.Replace(content)
	}

	outPath := "/tmp/transit.config.run.json"
	os.WriteFile(outPath, []byte(content), 0600)

	cmd := exec.Command("sing-box", "run", "-c", outPath)
	cmd.Stdout = &singBoxLogWriter{isStderr: false}
	cmd.Stderr = &singBoxLogWriter{isStderr: true}

	sysLogInfo("Transit Node Launching...")

	if err := cmd.Start(); err != nil {
		sysLogError(fmt.Sprintf("Launch failed: %v", err))
		return
	}

	go func() {
		time.Sleep(2 * time.Second)
		os.Remove(outPath)
		sysLogInfo("transit.config.run.json cleared, transit node is running.")
	}()

	cmd.Wait()
}
