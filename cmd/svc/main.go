package main

import (
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
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	infoLogFilePath  string
	errorLogFilePath string

	infoLogger  *lumberjack.Logger
	errorLogger *lumberjack.Logger
	logMu       sync.Mutex
)

func initLogFiles() {
	exe, err := os.Executable()
	if err != nil {
		exe = "."
	}
	baseDir := filepath.Dir(exe)
	logsDir := filepath.Join(baseDir, "logs")
	os.MkdirAll(logsDir, 0700)

	infoLogFilePath = filepath.Join(logsDir, "access.log")
	errorLogFilePath = filepath.Join(logsDir, "error.log")

	infoLogger = &lumberjack.Logger{
		Filename:   infoLogFilePath,
		MaxSize:    10, // megabytes
		MaxBackups: 5,
		MaxAge:     28, // days
		Compress:   true,
	}

	errorLogger = &lumberjack.Logger{
		Filename:   errorLogFilePath,
		MaxSize:    10, // megabytes
		MaxBackups: 5,
		MaxAge:     28, // days
		Compress:   true,
	}

	// Always clear logs on start
	_ = infoLogger.Rotate()
	_ = errorLogger.Rotate()

	// Ensure empty logs exist
	os.WriteFile(infoLogFilePath, []byte(""), 0600)
	os.WriteFile(errorLogFilePath, []byte(""), 0600)
}

func writeLog(level, prefix, msg string, printToStdout bool) {
	if infoLogger == nil || errorLogger == nil {
		return
	}

	logMu.Lock()
	defer logMu.Unlock()

	timestamp := time.Now().Format("2006/01/02 15:04:05")
	logLine := fmt.Sprintf("%s %s %s\n", timestamp, prefix, msg)

	infoLogger.Write([]byte(logLine))

	if level == "error" {
		errorLogger.Write([]byte(logLine))
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
		if util.IsNotBlank(l) {
			level := "info"
			if util.ContainsCaseInsensitive(l, "error") || util.ContainsCaseInsensitive(l, "fatal") {
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

func recordSystemStatus() {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("netsh", "interface", "show", "interface")
	} else {
		cmd = exec.Command("ip", "link", "show")
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		sysLogError(fmt.Sprintf("Failed to record system status: %v", err), false)
		return
	}
	sysLogError(fmt.Sprintf("System Status at crash:\n%s", string(out)), false)
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

	envMap, err := util.LoadEnvMap(envPath)
	if err != nil {
		sysLogError(fmt.Sprintf("Failed to open environment file: %v", err), false)
		return
	}

	var tplPath = p.templatePath
	if !filepath.IsAbs(tplPath) {
		tplPath = filepath.Join(baseDir, tplPath)
	}

	content, err := util.RenderConfigTemplate(tplPath, envMap)
	if err != nil {
		sysLogError(fmt.Sprintf("Failed to render config template: %v", err), false)
		return
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
		recordSystemStatus()
		os.Exit(1)
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
					sysLogError("Network health check failed, triggering restart", false)
					recordSystemStatus()
					p.cleanup()
					os.Exit(1)
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

	envMap := make(map[string]string)
	for _, env := range os.Environ() {
		pair := strings.SplitN(env, "=", 2)
		if len(pair) == 2 {
			envMap[pair[0]] = pair[1]
		}
	}

	content, err := util.RenderConfigTemplate(templatePath, envMap)
	if err != nil {
		sysLogError(fmt.Sprintf("Failed to render config template: %v", err), true)
		os.Exit(1)
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

func printMainUsage() {
	fmt.Println(`Usage:
  portal-svc [command]

Available Commands:
  dock        Sing-Box background service with auto-recovery
  transit     Launch a transit node
  logs        View service logs
  render      Render a configuration template with environment variables

Flags:
  -h, --help   help for portal-svc

Use "portal-svc [command] --help" for more information about a command.`)
}

func printDockUsage() {
	fmt.Println(`Sing-Box background service with auto-recovery

Usage:
  portal-svc dock [flags] [service-command]

Service Commands:
  install     Install as System Service
  start       Start Service
  stop        Stop Service
  restart     Restart Service
  uninstall   Remove Service

Flags:
      --config string   Path to dock template config (default "templates/dock_config.tmpl.json")
  -h, --help            help for dock`)
}

func printTransitUsage() {
	fmt.Println(`Launch a transit node

Usage:
  portal-svc transit [flags]

Flags:
      --config string   Path to transit template config (default "templates/transit_config.tmpl.json")
  -h, --help            help for transit`)
}

func printLogsUsage() {
	fmt.Println(`View service logs

Usage:
  portal-svc logs [flags] [error|info]

Flags:
  -f, --follow          Follow log output
  -n, --lines int       Number of lines to show (default 100)
  -h, --help            help for logs`)
}

func printRenderUsage() {
	fmt.Println(`Render a configuration template with environment variables

Usage:
  portal-svc render [flags]

Flags:
      --config string   Path to the input template file
      --out string      Path to the output JSON file
      --ci              Inject CI rules (ci-direct-out, disable auto_route)
  -h, --help            help for render`)
}

func handleRenderCmd(args []string) {
	renderCmd := flag.NewFlagSet("render", flag.ContinueOnError)
	renderCmd.Usage = printRenderUsage
	configPath := renderCmd.String("config", "", "Path to the input template file")
	outPath := renderCmd.String("out", "", "Path to the output JSON file")
	ci := renderCmd.Bool("ci", false, "Inject CI rules")

	err := renderCmd.Parse(args)
	if err == flag.ErrHelp {
		os.Exit(0)
	} else if err != nil {
		os.Exit(1)
	}

	if *configPath == "" || *outPath == "" {
		fmt.Println("Error: --config and --out are required.")
		printRenderUsage()
		os.Exit(1)
	}

	envMap := make(map[string]string)
	for _, env := range os.Environ() {
		pair := strings.SplitN(env, "=", 2)
		if len(pair) == 2 {
			envMap[pair[0]] = pair[1]
		}
	}

	content, err := util.RenderConfigTemplate(*configPath, envMap)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to render template: %v\n", err)
		os.Exit(1)
	}

	if *ci {
		content, err = util.InjectCIRules(content)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to inject CI rules: %v\n", err)
			os.Exit(1)
		}
	}

	if err := os.WriteFile(*outPath, []byte(content), 0600); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write output file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully rendered configuration to %s\n", *outPath)
}

func handleLogsCmd(args []string) {
	logsCmd := flag.NewFlagSet("logs", flag.ContinueOnError)
	logsCmd.Usage = printLogsUsage
	nLines := logsCmd.Int("n", 100, "")
	follow := logsCmd.Bool("f", false, "")

	err := logsCmd.Parse(args)
	if err == flag.ErrHelp {
		os.Exit(0)
	} else if err != nil {
		os.Exit(1)
	}

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
		printMainUsage()
		os.Exit(0)
	}

	cmd := os.Args[1]

	if cmd == "-h" || cmd == "--help" {
		printMainUsage()
		os.Exit(0)
	}

	if cmd == "logs" {
		handleLogsCmd(os.Args[2:])
		return
	}

	if cmd == "render" {
		handleRenderCmd(os.Args[2:])
		return
	}

	if cmd == "transit" {
		transitCmd := flag.NewFlagSet("transit", flag.ContinueOnError)
		transitCmd.Usage = printTransitUsage
		configPath := transitCmd.String("config", "templates/transit_config.tmpl.json", "Path to transit template config")

		err := transitCmd.Parse(os.Args[2:])
		if err == flag.ErrHelp {
			os.Exit(0)
		} else if err != nil {
			os.Exit(1)
		}

		runTransit(*configPath)
		return
	}

	if cmd == "dock" {
		dockCmd := flag.NewFlagSet("dock", flag.ContinueOnError)
		dockCmd.Usage = printDockUsage
		configPath := dockCmd.String("config", "templates/dock_config.tmpl.json", "Path to dock template config")

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

		err := dockCmd.Parse(flagArgs)
		if err == flag.ErrHelp {
			os.Exit(0)
		} else if err != nil {
			os.Exit(1)
		}

		svcConfig := &service.Config{
			Name:        "PortalDaemon",
			DisplayName: "Portal Daemon",
			Description: "Portal Daemon background service with auto-recovery",
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
			// Pre-flight check: see if ANY command requires elevation
			needsElevation := false
			for _, svcCmd := range svcArgs {
				if svcCmd == "install" || svcCmd == "start" || svcCmd == "stop" || svcCmd == "uninstall" || svcCmd == "restart" {
					needsElevation = true
					break
				}
			}

			if needsElevation && !util.IsAdmin() {
				fmt.Println("Elevated privileges required for service command(s). Attempting to elevate...")
				err := util.RunMeElevated()
				if err != nil {
					if strings.Contains(err.Error(), "elevated process exited with code") {
						// The elevated child process failed, and it likely already printed its own error.
						os.Exit(1)
					}
					fmt.Fprintf(os.Stderr, "Failed to elevate privileges: %v\n", err)
					fmt.Fprintf(os.Stderr, "Permission denied: please run this command as an administrator/root.\n")
					os.Exit(1)
				}
				// The elevated child process executed everything successfully.
				return
			}

			for _, svcCmd := range svcArgs {
				if svcCmd == "install" || svcCmd == "start" {
					exe, _ := os.Executable()
					baseDir := filepath.Dir(exe)

					// Initialize paths for log reading
					logsDir := filepath.Join(baseDir, "logs")
					errorLogFilePath = filepath.Join(logsDir, "error.log")

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
					if strings.Contains(err.Error(), "Access is denied") || strings.Contains(err.Error(), "permission denied") || strings.Contains(err.Error(), "requires elevation") {
						fmt.Fprintf(os.Stderr, "Permission denied: please run this command as an administrator/root.\n")
						os.Exit(1)
					}

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
			}
			return
		}

		err = s.Run()
		if err != nil {
			log.Fatalf("Service runtime error: %v", err)
		}
		return
	}

	fmt.Printf("Error: unknown command %q for \"portal-svc\"\n", cmd)
	printMainUsage()
	os.Exit(1)
}
