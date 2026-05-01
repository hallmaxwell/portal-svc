package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"hawego/portal/util"

	"github.com/kardianos/service"
	"github.com/nxadm/tail"
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

func writeLog(level, prefix, msg string, printToStdout bool) {
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

	if printToStdout {
		if level == "error" {
			os.Stderr.WriteString(prefix + " " + msg + "\n")
		} else {
			fmt.Println(prefix + " " + msg)
		}
	}
}

func sysLogInfo(msg string, printToStdout bool) {
	writeLog("info", "service:", msg, printToStdout)
}

func sysLogError(msg string, printToStdout bool) {
	writeLog("error", "service:", msg, printToStdout)
}

type singBoxLogWriter struct {
	isStderr      bool
	printToStdout bool
}

func (w *singBoxLogWriter) Write(p []byte) (n int, err error) {
	lines := strings.Split(strings.TrimSuffix(string(p), "\n"), "\n")
	for _, l := range lines {
		if len(strings.TrimSpace(l)) > 0 {
			level := "info"
			lowerL := strings.ToLower(l)
			if strings.Contains(lowerL, "error") || strings.Contains(lowerL, "fatal") || w.isStderr {
				level = "error"
			}
			writeLog(level, "sing-box:", l, w.printToStdout)
		}
	}
	return len(p), nil
}

func killExistingSingBox() {
	if runtime.GOOS == "windows" {
		_ = exec.Command("taskkill", "/F", "/T", "/IM", "sing-box.exe").Run()
	} else {
		_ = exec.Command("killall", "-9", "sing-box").Run()
	}
}

// ==========================================
// Dock Logic
// ==========================================
type dockProgram struct {
	cmd          *exec.Cmd
	outPath      string
	exit         chan struct{}
	stopping     bool
	templatePath string
}

func (p *dockProgram) Start(s service.Service) error {
	p.exit = make(chan struct{})
	go p.run()
	go p.monitorNetwork()
	return nil
}

func (p *dockProgram) run() {
	initLogFiles()
	sysLogInfo("Starting service run loop...", false)

	exe, err := os.Executable()
	if err != nil {
		sysLogError(fmt.Sprintf("Failed to get executable path: %v", err), false)
		return
	}
	baseDir := filepath.Dir(exe)

	singBoxBin := "sing-box"
	if runtime.GOOS == "windows" {
		singBoxBin = "sing-box.exe"
	}
	singBoxPath := filepath.Join(baseDir, "core", singBoxBin)

	if _, err := os.Stat(singBoxPath); err != nil {
		sysLogError(fmt.Sprintf("Dependencies not found: %s", singBoxPath), false)
		return
	}

	envPath := filepath.Join(baseDir, ".env")
	p.outPath = filepath.Join(os.TempDir(), "dock.config.run.json")

	if _, err := os.Stat(envPath); err != nil {
		sysLogError("Environment file not found", false)
		return
	}

	envMap := make(map[string]string)
	envFile, err := os.Open(envPath)
	if err != nil {
		sysLogError(fmt.Sprintf("Failed to open environment file: %v", err), false)
		return
	}

	scanner := bufio.NewScanner(envFile)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			envMap[strings.TrimSpace(parts[0])] = strings.Trim(strings.TrimSpace(parts[1]), `"'`)
		}
	}
	envFile.Close()

	var tplPath = p.templatePath
	if !filepath.IsAbs(tplPath) {
		tplPath = filepath.Join(baseDir, tplPath)
	}

	tempData, err := os.ReadFile(tplPath)
	if err != nil {
		sysLogError(fmt.Sprintf("Failed to read config template: %v", err), false)
		return
	}

	content := string(tempData)
	for key, val := range envMap {
		if util.IsRawJSONValue(val) {
			content = strings.ReplaceAll(content, `"{`+key+`}"`, val)
			content = strings.ReplaceAll(content, `{`+key+`}`, val)
		} else {
			content = strings.ReplaceAll(content, `{`+key+`}`, val)
		}
	}

	os.WriteFile(p.outPath, []byte(content), 0600)

	killExistingSingBox()

	p.cmd = exec.Command(singBoxPath, "run", "-c", p.outPath)
	p.cmd.Dir = baseDir
	p.cmd.Stdout = &singBoxLogWriter{isStderr: false, printToStdout: false}
	p.cmd.Stderr = &singBoxLogWriter{isStderr: true, printToStdout: false}
	p.cmd.Start()
	p.cmd.Wait()

	p.cleanup()

	if !p.stopping {
		sysLogError("Sing-box process exited unexpectedly", false)
	}
}

func (p *dockProgram) cleanup() {
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
	if p.outPath != "" {
		_ = os.Remove(p.outPath)
	}
}

func (p *dockProgram) monitorNetwork() {
	failCount := 0
	for {
		select {
		case <-p.exit:
			return
		case <-time.After(10 * time.Second):
			conn, err := net.DialTimeout("tcp", "8.8.8.8:53", 3*time.Second)
			if err != nil {
				failCount++
				if failCount >= 3 {
					p.cleanup()
					sysLogError("Network health check failed, triggering restart", false)
					return
				}
			} else {
				failCount = 0
				conn.Close()
			}
		}
	}
}

func (p *dockProgram) Stop(s service.Service) error {
	p.stopping = true
	close(p.exit)
	p.cleanup()
	return nil
}

// ==========================================
// Transit Logic
// ==========================================
func runTransit(templatePath string) {
	initLogFiles()

	data, err := os.ReadFile(templatePath)
	if err != nil {
		sysLogError(fmt.Sprintf("Failed to read config template: %v", err), true)
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

	outPath := filepath.Join(os.TempDir(), "transit.config.run.json")
	os.WriteFile(outPath, []byte(content), 0600)

	cmd := exec.Command("sing-box", "run", "-c", outPath)
	cmd.Stdout = &singBoxLogWriter{isStderr: false, printToStdout: true}
	cmd.Stderr = &singBoxLogWriter{isStderr: true, printToStdout: true}

	sysLogInfo("Transit Node Launching...", true)

	if err := cmd.Start(); err != nil {
		sysLogError(fmt.Sprintf("Launch failed: %v", err), true)
		return
	}

	go func() {
		time.Sleep(2 * time.Second)
		os.Remove(outPath)
		sysLogInfo("transit.config.run.json cleared, transit node is running.", true)
	}()

	cmd.Wait()
}

// ==========================================
// Main & CLI Logic
// ==========================================
func handleLogsCmd(args []string) {
	logsCmd := flag.NewFlagSet("logs", flag.ExitOnError)
	nLines := logsCmd.Int("n", 100, "")
	follow := logsCmd.Bool("f", false, "")

	logsCmd.Parse(args)

	targetLogFile := infoLogFilePath
	if logsCmd.NArg() > 0 && logsCmd.Arg(0) == "error" {
		targetLogFile = errorLogFilePath
	}

	if _, err := os.Stat(targetLogFile); os.IsNotExist(err) {
		fmt.Printf("Log file does not exist: %s\n", targetLogFile)
		return
	}

	if *follow {
		t, err := tail.TailFile(targetLogFile, tail.Config{
			Follow:    true,
			ReOpen:    true,
			MustExist: false,
			Logger:    tail.DiscardingLogger,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to tail log file: %v\n", err)
			os.Exit(1)
		}
		for line := range t.Lines {
			fmt.Println(line.Text)
		}
	} else {
		data, err := os.ReadFile(targetLogFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to read log file: %v\n", err)
			os.Exit(1)
		}

		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		start := 0
		if len(lines) > *nLines {
			start = len(lines) - *nLines
		}

		for i := start; i < len(lines); i++ {
			if len(strings.TrimSpace(lines[i])) > 0 {
				fmt.Println(lines[i])
			}
		}
	}
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: portal-svc <dock|transit|logs> [args]")
		os.Exit(1)
	}

	cmd := os.Args[1]

	if cmd == "logs" {
		handleLogsCmd(os.Args[2:])
		return
	}

	if cmd == "transit" {
		transitCmd := flag.NewFlagSet("transit", flag.ExitOnError)
		configPath := transitCmd.String("config", "/transit_config.tmpl.json", "Path to transit template config")
		transitCmd.Parse(os.Args[2:])

		runTransit(*configPath)
		return
	}

	if cmd == "dock" {
		dockCmd := flag.NewFlagSet("dock", flag.ContinueOnError)
		configPath := dockCmd.String("config", "dock_config.tmpl.json", "Path to dock template config")

		// Extract flag args vs service args
		var flagArgs []string
		var svcArgs []string

		for i := 2; i < len(os.Args); i++ {
			arg := os.Args[i]
			if strings.HasPrefix(arg, "-") {
				flagArgs = append(flagArgs, arg)
				// Consume value if not boolean flag
				if i+1 < len(os.Args) && !strings.HasPrefix(os.Args[i+1], "-") {
					flagArgs = append(flagArgs, os.Args[i+1])
					i++
				}
			} else {
				svcArgs = append(svcArgs, arg)
			}
		}

		dockCmd.Parse(flagArgs)

		svcConfig := &service.Config{
			Name:        "SingBoxWrapper",
			DisplayName: "Sing-Box Wrapper Service",
			Description: "Sing-Box background service with auto-recovery",
			Option: service.KeyValue{
				"OnFailure":              "restart",
				"OnFailureDelayDuration": "10s",
				"OnFailureResetPeriod":   600,
			},
			Arguments: append([]string{"dock"}, flagArgs...),
		}

		prg := &dockProgram{templatePath: *configPath}
		s, err := service.New(prg, svcConfig)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create service: %v\n", err)
			os.Exit(1)
		}

		if len(svcArgs) > 0 {
			svcCmd := svcArgs[0]

			if svcCmd == "install" || svcCmd == "start" {
				exe, _ := os.Executable()
				baseDir := filepath.Dir(exe)

				singBoxBin := "sing-box"
				if runtime.GOOS == "windows" {
					singBoxBin = "sing-box.exe"
				}
				singBoxPath := filepath.Join(baseDir, "core", singBoxBin)

				if _, err := os.Stat(singBoxPath); err != nil {
					fmt.Printf("Pre-flight check failed: Sing-box executable not found at %s\n", singBoxPath)
					os.Exit(1)
				}

				envPath := filepath.Join(baseDir, ".env")
				if _, err := os.Stat(envPath); err != nil {
					fmt.Printf("Pre-flight check failed: Environment file not found at %s\n", envPath)
					os.Exit(1)
				}

				var tplPath = *configPath
				if !filepath.IsAbs(tplPath) {
					tplPath = filepath.Join(baseDir, tplPath)
				}
				if _, err := os.Stat(tplPath); err != nil {
					fmt.Printf("Pre-flight check failed: Template file not found at %s\n", tplPath)
					os.Exit(1)
				}
			}

			err = service.Control(s, svcCmd)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to execute service command '%s': %v\n", svcCmd, err)
				os.Exit(1)
			}

			if svcCmd == "start" {
				time.Sleep(2 * time.Second)
				data, err := os.ReadFile(errorLogFilePath)
				if err == nil && len(data) > 0 {
					lines := strings.Split(strings.TrimSpace(string(data)), "\n")
					recentErrors := ""
					for i := len(lines) - 1; i >= 0 && i >= len(lines)-5; i-- {
						recentErrors = lines[i] + "\n" + recentErrors
					}
					if len(strings.TrimSpace(recentErrors)) > 0 {
						fmt.Printf("Service command 'start' executed, but errors occurred shortly after:\n%s\n", recentErrors)
						os.Exit(1)
					}
				}
			}

			fmt.Printf("Service command '%s' executed successfully.\n", svcCmd)
			return
		}

		err = s.Run()
		if err != nil {
			log.Fatalf("Service runtime error: %v", err)
		}
		return
	}

	fmt.Printf("Unknown command: %s\n", cmd)
	os.Exit(1)
}
