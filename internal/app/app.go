package app

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
	"sync/atomic"
	"time"

	"portal-svc/templates"
	"portal-svc/util"

	"github.com/kardianos/service"
	"github.com/nxadm/tail"
	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	defaultDockConfig    = "templates/dock_config.tmpl.json"
	defaultTransitConfig = "templates/transit_config.tmpl.json"

	dockTempConfig    = "dock.config.run.json"
	transitTempConfig = "transit.config.run.json"

	serviceName        = "PortalDaemon"
	serviceDisplayName = "Portal Daemon"
	serviceDescription = "Portal Daemon background service with auto-recovery"
)

var (
	infoLogFilePath  string
	errorLogFilePath string

	infoLogger  *lumberjack.Logger
	errorLogger *lumberjack.Logger
	logMu       sync.Mutex
)

func executableDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to get executable path: %w", err)
	}
	return filepath.Dir(exe), nil
}

func singBoxBinaryName() string {
	if runtime.GOOS == "windows" {
		return "sing-box.exe"
	}
	return "sing-box"
}

func bundledSingBoxPath(baseDir string) string {
	return filepath.Join(baseDir, "core", singBoxBinaryName())
}

func processEnvMap() map[string]string {
	envMap := make(map[string]string)
	for _, env := range os.Environ() {
		pair := strings.SplitN(env, "=", 2)
		if len(pair) == 2 {
			envMap[pair[0]] = pair[1]
		}
	}
	return envMap
}

func initLogPaths() error {
	baseDir, err := executableDir()
	if err != nil {
		return err
	}
	logsDir := filepath.Join(baseDir, "logs")

	infoLogFilePath = filepath.Join(logsDir, "access.log")
	errorLogFilePath = filepath.Join(logsDir, "error.log")
	return nil
}

func initLogFiles() error {
	if err := initLogPaths(); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(infoLogFilePath), 0700); err != nil {
		return fmt.Errorf("failed to create logs directory: %w", err)
	}

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
	if err := infoLogger.Rotate(); err != nil {
		return fmt.Errorf("failed to rotate access log: %w", err)
	}
	if err := errorLogger.Rotate(); err != nil {
		return fmt.Errorf("failed to rotate error log: %w", err)
	}

	// Ensure empty logs exist
	if err := os.WriteFile(infoLogFilePath, []byte(""), 0600); err != nil {
		return fmt.Errorf("failed to initialize access log: %w", err)
	}
	if err := os.WriteFile(errorLogFilePath, []byte(""), 0600); err != nil {
		return fmt.Errorf("failed to initialize error log: %w", err)
	}

	return nil
}

func writeLog(level, prefix, msg string, printToStdout bool) {
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
	var err error
	if runtime.GOOS == "windows" {
		err = exec.Command("taskkill", "/F", "/T", "/IM", "sing-box.exe").Run()
	} else {
		err = exec.Command("killall", "-9", "sing-box").Run()
	}
	if err != nil {
		sysLogInfo(fmt.Sprintf("No existing sing-box process stopped: %v", err), false)
	}
}

func isDockServiceCommand(cmd string) bool {
	switch cmd {
	case "install", "start", "stop", "restart", "uninstall":
		return true
	default:
		return false
	}
}

func serviceCommandNeedsElevation(cmd string) bool {
	return isDockServiceCommand(cmd)
}

func serviceCommandNeedsSingBox(cmd string) bool {
	return cmd == "start" || cmd == "restart"
}

func validateDockServiceCommands(commands []string) error {
	for _, cmd := range commands {
		if !isDockServiceCommand(cmd) {
			return fmt.Errorf("invalid service command %q", cmd)
		}
	}
	return nil
}

func anyDockServiceCommand(commands []string, predicate func(string) bool) bool {
	for _, cmd := range commands {
		if predicate(cmd) {
			return true
		}
	}
	return false
}

func ensureBundledSingBox(baseDir string) error {
	singBoxPath := bundledSingBoxPath(baseDir)
	if _, err := os.Stat(singBoxPath); err != nil {
		return fmt.Errorf("sing-box executable not found at %s", singBoxPath)
	}
	return nil
}

func recentNonBlankLines(path string, maxLines int) (string, error) {
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
	stopping     atomic.Bool
	stopOnce     sync.Once
	templatePath string
}

func (p *dockProgram) Start(_ service.Service) error {
	p.exit = make(chan struct{})
	go p.run()
	go p.monitorNetwork()
	return nil
}

func (p *dockProgram) run() {
	if err := initLogFiles(); err != nil {
		log.Printf("Failed to initialize log files: %v", err)
		os.Exit(1)
	}
	sysLogInfo("Starting service run loop...", false)

	baseDir, err := executableDir()
	if err != nil {
		sysLogError(fmt.Sprintf("Failed to get executable path: %v", err), false)
		os.Exit(1)
	}

	singBoxPath := bundledSingBoxPath(baseDir)
	if err := ensureBundledSingBox(baseDir); err != nil {
		sysLogError(fmt.Sprintf("Dependencies not found: %v", err), false)
		os.Exit(1)
	}

	envPath := filepath.Join(baseDir, ".env")
	p.outPath = filepath.Join(os.TempDir(), dockTempConfig)

	if _, err := os.Stat(envPath); err != nil {
		sysLogError("Environment file not found", false)
		os.Exit(1)
	}

	envMap, err := util.LoadEnvMap(envPath)
	if err != nil {
		sysLogError(fmt.Sprintf("Failed to open environment file: %v", err), false)
		os.Exit(1)
	}

	var tplPath = p.templatePath
	if !filepath.IsAbs(tplPath) {
		tplPath = filepath.Join(baseDir, tplPath)
	}

	content, err := util.RenderConfigTemplate(tplPath, envMap)
	if err != nil {
		sysLogError(fmt.Sprintf("Failed to render config template: %v", err), false)
		os.Exit(1)
	}

	if err := os.WriteFile(p.outPath, []byte(content), 0600); err != nil {
		sysLogError(fmt.Sprintf("Failed to write rendered config: %v", err), false)
		os.Exit(1)
	}

	killExistingSingBox()

	p.cmd = exec.Command(singBoxPath, "run", "-c", p.outPath)
	p.cmd.Dir = baseDir
	p.cmd.Stdout = &singBoxLogWriter{printToStdout: false}
	p.cmd.Stderr = &singBoxLogWriter{printToStdout: false}
	if err := p.cmd.Start(); err != nil {
		sysLogError(fmt.Sprintf("Failed to start sing-box: %v", err), false)
		p.cleanup()
		os.Exit(1)
	}

	waitErr := p.cmd.Wait()

	p.cleanup()

	if !p.stopping.Load() {
		if waitErr != nil {
			sysLogError(fmt.Sprintf("Sing-box process exited unexpectedly: %v", waitErr), false)
		} else {
			sysLogError("Sing-box process exited unexpectedly", false)
		}
		recordSystemStatus()
		os.Exit(1)
	}
}

func (p *dockProgram) cleanup() {
	if p.cmd != nil && p.cmd.Process != nil {
		if err := p.cmd.Process.Kill(); err != nil && !strings.Contains(strings.ToLower(err.Error()), "process already finished") {
			sysLogError(fmt.Sprintf("Failed to kill sing-box process during cleanup: %v", err), false)
		}
	}
	if p.outPath != "" {
		if err := os.Remove(p.outPath); err != nil && !os.IsNotExist(err) {
			sysLogError(fmt.Sprintf("Failed to remove temporary config: %v", err), false)
		}
	}
}

func (p *dockProgram) monitorNetwork() {
	failCount := 0
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-p.exit:
			return
		case <-ticker.C:
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
				if err := conn.Close(); err != nil {
					sysLogError(fmt.Sprintf("Failed to close network health check connection: %v", err), false)
				}
			}
		}
	}
}

func (p *dockProgram) Stop(_ service.Service) error {
	p.stopping.Store(true)
	p.stopOnce.Do(func() {
		if p.exit != nil {
			close(p.exit)
		}
	})
	p.cleanup()
	return nil
}

// ==========================================
// Transit Logic
// ==========================================
func runTransit(templatePath string) {
	if err := initLogFiles(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize log files: %v\n", err)
		os.Exit(1)
	}

	envMap := processEnvMap()
	content, err := util.RenderConfigTemplate(templatePath, envMap)
	if err != nil {
		sysLogError(fmt.Sprintf("Failed to render config template: %v", err), true)
		os.Exit(1)
	}

	outPath := filepath.Join(os.TempDir(), transitTempConfig)
	if err := os.WriteFile(outPath, []byte(content), 0600); err != nil {
		sysLogError(fmt.Sprintf("Failed to write rendered config: %v", err), true)
		os.Exit(1)
	}

	cmd := exec.Command("sing-box", "run", "-c", outPath)
	cmd.Stdout = &singBoxLogWriter{printToStdout: true}
	cmd.Stderr = &singBoxLogWriter{printToStdout: true}

	sysLogInfo("Transit Node Launching...", true)

	if err := cmd.Start(); err != nil {
		sysLogError(fmt.Sprintf("Launch failed: %v", err), true)
		return
	}

	go func() {
		time.Sleep(2 * time.Second)
		if err := os.Remove(outPath); err != nil && !os.IsNotExist(err) {
			sysLogError(fmt.Sprintf("Failed to remove %s: %v", transitTempConfig, err), true)
			return
		}
		sysLogInfo(fmt.Sprintf("%s cleared, transit node is running.", transitTempConfig), true)
	}()

	if err := cmd.Wait(); err != nil {
		sysLogError(fmt.Sprintf("Transit node exited with error: %v", err), true)
		os.Exit(1)
	}
}

// ==========================================
// Main & CLI Logic
// ==========================================

func printMainUsage() {
	fmt.Println(`Usage:
  portal-svc [command]

Available Commands:
  init        Initialize the portal-svc environment and release default templates
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

func printInitUsage() {
	fmt.Println(`Initialize the portal-svc environment and release default templates

Usage:
  portal-svc init [dock|transit] [flags]

Flags:
  -h, --help            help for init`)
}

func handleInitCmd(args []string) {
	initCmd := flag.NewFlagSet("init", flag.ContinueOnError)
	initCmd.Usage = printInitUsage

	err := initCmd.Parse(args)
	if err == flag.ErrHelp {
		os.Exit(0)
	} else if err != nil {
		os.Exit(1)
	}

	if initCmd.NArg() < 1 {
		fmt.Println("Error: missing required argument [dock|transit]")
		printInitUsage()
		os.Exit(1)
	}

	target := initCmd.Arg(0)
	if target != "dock" && target != "transit" {
		fmt.Printf("Error: invalid target '%s', expected 'dock' or 'transit'\n", target)
		os.Exit(1)
	}

	baseDir, err := os.Getwd()
	if err != nil {
		baseDir = "."
	}
	envPath := filepath.Join(baseDir, ".env")
	envMap, err := util.LoadEnvMap(envPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to load environment file: %v\n", err)
		envMap = make(map[string]string)
	}

	portalEnv := envMap["PORTAL_ENV"]
	if portalEnv == "" {
		if target == "dock" {
			portalEnv = "local"
		} else {
			portalEnv = "cloud"
		}
	}

	fmt.Printf("Initializing for %s in %s environment...\n", target, portalEnv)

	tmplName := target + "_config.tmpl.json"
	tmplData, err := templates.FS.ReadFile(tmplName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: could not find embedded template '%s': %v\n", tmplName, err)
		os.Exit(1)
	}

	tmplDir := filepath.Join(baseDir, "templates")
	if err := os.MkdirAll(tmplDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error: could not create templates directory: %v\n", err)
		os.Exit(1)
	}

	outPath := filepath.Join(tmplDir, tmplName)
	if _, err := os.Stat(outPath); err == nil {
		fmt.Printf("Template '%s' already exists in '%s'. Skipping.\n", tmplName, tmplDir)
	} else if !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: could not inspect template file '%s': %v\n", outPath, err)
		os.Exit(1)
	} else {
		if err := os.WriteFile(outPath, tmplData, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error: could not write template file: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Successfully released '%s' to '%s'.\n", tmplName, tmplDir)
	}
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

	envMap := processEnvMap()
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
	if err := initLogPaths(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize log paths: %v\n", err)
		os.Exit(1)
	}

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
	if *nLines < 0 {
		fmt.Fprintln(os.Stderr, "Error: --lines must be greater than or equal to 0")
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
		recentLogs, err := recentNonBlankLines(targetLogFile, *nLines)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to read log file: %v\n", err)
			os.Exit(1)
		}

		if util.IsNotBlank(recentLogs) {
			fmt.Print(recentLogs)
		}
	}
}

func handleTransitCmd(args []string) {
	transitCmd := flag.NewFlagSet("transit", flag.ContinueOnError)
	transitCmd.Usage = printTransitUsage
	configPath := transitCmd.String("config", defaultTransitConfig, "Path to transit template config")

	err := transitCmd.Parse(args)
	if err == flag.ErrHelp {
		os.Exit(0)
	} else if err != nil {
		os.Exit(1)
	}

	runTransit(*configPath)
}

func splitDockArgs(args []string) (flagArgs []string, svcArgs []string) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			flagArgs = append(flagArgs, arg)
			// Consume value if not boolean flag.
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				flagArgs = append(flagArgs, args[i+1])
				i++
			}
		} else {
			svcArgs = append(svcArgs, arg)
		}
	}
	return flagArgs, svcArgs
}

func handleDockCmd(args []string) {
	dockCmd := flag.NewFlagSet("dock", flag.ContinueOnError)
	dockCmd.Usage = printDockUsage
	configPath := dockCmd.String("config", defaultDockConfig, "Path to dock template config")

	flagArgs, svcArgs := splitDockArgs(args)

	err := dockCmd.Parse(flagArgs)
	if err == flag.ErrHelp {
		os.Exit(0)
	} else if err != nil {
		os.Exit(1)
	}

	if err := validateDockServiceCommands(svcArgs); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		printDockUsage()
		os.Exit(1)
	}

	svcConfig := &service.Config{
		Name:        serviceName,
		DisplayName: serviceDisplayName,
		Description: serviceDescription,
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
		runDockServiceCommands(s, svcArgs)
		return
	}

	err = s.Run()
	if err != nil {
		log.Fatalf("Service runtime error: %v", err)
	}
}

func runDockServiceCommands(s service.Service, svcArgs []string) {
	var baseDir string
	var err error

	if anyDockServiceCommand(svcArgs, serviceCommandNeedsSingBox) {
		baseDir, err = executableDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to get executable path: %v\n", err)
			os.Exit(1)
		}
	}

	// Do pre-flight checks before attempting elevation so errors are visible in the current console.
	if anyDockServiceCommand(svcArgs, serviceCommandNeedsSingBox) {
		if err := ensureBundledSingBox(baseDir); err != nil {
			fmt.Fprintf(os.Stderr, "Pre-flight check failed: %v\n", err)
			os.Exit(1)
		}
	}

	if anyDockServiceCommand(svcArgs, serviceCommandNeedsElevation) && !util.IsAdmin() {
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
		if serviceCommandNeedsSingBox(svcCmd) {
			logsDir := filepath.Join(baseDir, "logs")
			errorLogFilePath = filepath.Join(logsDir, "error.log")
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

		if serviceCommandNeedsSingBox(svcCmd) {
			time.Sleep(2 * time.Second)
			recentErrors, err := recentNonBlankLines(errorLogFilePath, 5)
			if err != nil && !os.IsNotExist(err) {
				fmt.Fprintf(os.Stderr, "Warning: failed to read startup error log: %v\n", err)
			}
			if err == nil && util.IsNotBlank(recentErrors) {
				fmt.Printf("Service command '%s' executed, but errors occurred shortly after:\n%s\n", svcCmd, recentErrors)
				os.Exit(1)
			}
		}

		fmt.Printf("Service command '%s' executed successfully.\n", svcCmd)
	}
}

func init() {
	for i, arg := range os.Args {
		if strings.HasPrefix(arg, "--elevated-out=") {
			outFile := strings.TrimPrefix(arg, "--elevated-out=")
			f, err := os.OpenFile(outFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
			if err == nil {
				os.Stdout = f
				os.Stderr = f
				log.SetOutput(f)
				// We don't close it, intentionally letting it remain open
			} else {
				fmt.Fprintf(os.Stderr, "Failed to redirect elevated output to %s: %v\n", outFile, err)
			}
			os.Args = append(os.Args[:i], os.Args[i+1:]...)
			break
		}
	}
}

func Run() {
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

	if cmd == "init" {
		handleInitCmd(os.Args[2:])
		return
	}

	if cmd == "render" {
		handleRenderCmd(os.Args[2:])
		return
	}

	if cmd == "transit" {
		handleTransitCmd(os.Args[2:])
		return
	}

	if cmd == "dock" {
		handleDockCmd(os.Args[2:])
		return
	}

	fmt.Printf("Error: unknown command %q for \"portal-svc\"\n", cmd)
	printMainUsage()
	os.Exit(1)
}
