package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	_ "embed"
	"portal-svc/shared"
	"portal-svc/templates"
	"portal-svc/ui"
	"portal-svc/util"
	"portal-svc/util/tweak"

	"github.com/kardianos/service"
	"github.com/nxadm/tail"
)

const (
	defaultConfig = "templates/local_config.tmpl.json"
	tempConfig    = "local.config.run.json"

	serviceName        = "PortalDaemon"
	serviceDisplayName = "Portal Daemon"
	serviceDescription = "Portal Daemon background service with auto-recovery"
)

var p = ui.NewPrinter()

func init() {
	shared.CaptureElevatedOut()
}

func isServiceCommand(cmd string) bool {
	switch cmd {
	case "install", "start", "stop", "restart", "uninstall":
		return true
	default:
		return false
	}
}

func serviceCommandNeedsElevation(cmd string) bool {
	return isServiceCommand(cmd)
}

func serviceCommandNeedsSingBox(cmd string) bool {
	return cmd == "start" || cmd == "restart"
}

// ==========================================
// Program Logic
// ==========================================
type program struct {
	cmd          *exec.Cmd
	outPath      string
	exit         chan struct{}
	stopping     atomic.Bool
	stopOnce     sync.Once
	templatePath string
}

func (p *program) Start(_ service.Service) error {
	p.exit = make(chan struct{})
	go p.run()
	go p.monitorNetwork()
	return nil
}

func (p *program) run() {
	if err := shared.InitLogFiles(); err != nil {
		log.Printf("Failed to initialize log files: %v", err)
		os.Exit(1)
	}
	shared.SysLogInfo("Starting service run loop...", false)

	baseDir, err := shared.ExecutableDir()
	if err != nil {
		shared.SysLogError(fmt.Sprintf("Failed to get executable path: %v", err), false)
		os.Exit(1)
	}

	singBoxPath := shared.BundledSingBoxPath(baseDir)
	if err := shared.EnsureBundledSingBox(baseDir); err != nil {
		shared.SysLogError(fmt.Sprintf("Dependencies not found: %v", err), false)
		os.Exit(1)
	}

	envPath := filepath.Join(baseDir, ".env")
	p.outPath = filepath.Join(os.TempDir(), tempConfig)
	srsDir := filepath.Join(baseDir, "srs")

	if _, err := os.Stat(envPath); err != nil {
		shared.SysLogError("Environment file not found", false)
		os.Exit(1)
	}

	envMap, err := util.LoadEnvMap(envPath)
	if err != nil {
		shared.SysLogError(fmt.Sprintf("Failed to open environment file: %v", err), false)
		os.Exit(1)
	}

	var tplPath = p.templatePath
	if !filepath.IsAbs(tplPath) {
		tplPath = filepath.Join(baseDir, tplPath)
	}

	content, err := util.RenderConfigTemplate(tplPath, envMap)
	if err != nil {
		shared.SysLogError(fmt.Sprintf("Failed to render config template: %v", err), false)
		os.Exit(1)
	}

	content, err = shared.ProcessRuleSets(content, srsDir)
	if err != nil {
		shared.SysLogError(fmt.Sprintf("Failed to process rule sets: %v", err), false)
		// non-fatal, continue with original content
	}

	// Apply user overrides
	overridePath := filepath.Join(baseDir, tweak.OverrideFileName)
	overrides, err := tweak.LoadOverrides(overridePath)
	if err != nil {
		shared.SysLogError(fmt.Sprintf("Failed to load user overrides: %v", err), false)
	} else if len(overrides) > 0 {
		content, err = tweak.ApplyOverrides(content, overrides)
		if err != nil {
			shared.SysLogError(fmt.Sprintf("Failed to apply user overrides: %v", err), false)
		} else {
			shared.SysLogInfo("Applied user configuration overrides", false)
		}
	}

	if err := os.WriteFile(p.outPath, []byte(content), 0600); err != nil {
		shared.SysLogError(fmt.Sprintf("Failed to write rendered config: %v", err), false)
		os.Exit(1)
	}

	shared.KillExistingSingBox()

	p.cmd = exec.Command(singBoxPath, "run", "-c", p.outPath)
	p.cmd.Dir = baseDir
	p.cmd.Stdout = &shared.SingBoxLogWriter{PrintToStdout: false}
	p.cmd.Stderr = &shared.SingBoxLogWriter{PrintToStdout: false}
	if err := p.cmd.Start(); err != nil {
		shared.SysLogError(fmt.Sprintf("Failed to start sing-box: %v", err), false)
		p.cleanup()
		os.Exit(1)
	}

	waitErr := p.cmd.Wait()

	p.cleanup()

	if !p.stopping.Load() {
		if waitErr != nil {
			shared.SysLogError(fmt.Sprintf("Sing-box process exited unexpectedly: %v", waitErr), false)
		} else {
			shared.SysLogError("Sing-box process exited unexpectedly", false)
		}
		shared.RecordSystemStatus()
		os.Exit(1)
	}
}

func (p *program) cleanup() {
	if p.cmd != nil && p.cmd.Process != nil {
		if err := p.cmd.Process.Kill(); err != nil && !strings.Contains(strings.ToLower(err.Error()), "process already finished") {
			shared.SysLogError(fmt.Sprintf("Failed to kill sing-box process during cleanup: %v", err), false)
		}
	}
	if p.outPath != "" {
		if err := os.Remove(p.outPath); err != nil && !os.IsNotExist(err) {
			shared.SysLogError(fmt.Sprintf("Failed to remove temporary config: %v", err), false)
		}
	}
}

func (p *program) monitorNetwork() {
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
					shared.SysLogError("Network health check failed, triggering restart", false)
					shared.RecordSystemStatus()
					p.cleanup()
					os.Exit(1)
				}
			} else {
				failCount = 0
				if err := conn.Close(); err != nil {
					shared.SysLogError(fmt.Sprintf("Failed to close network health check connection: %v", err), false)
				}
			}
		}
	}
}

func (p *program) Stop(_ service.Service) error {
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
// Command Handlers
// ==========================================

func printMainUsage() { ui.PrintHelp(p, ui.HelpConfigJSON, "main_local") }

func printLogsUsage() { ui.PrintHelp(p, ui.HelpConfigJSON, "logs") }

func handleLogsCmd(args []string) {
	shared.CheckError(shared.InitLogPaths(), "Failed to initialize log paths: %v")

	logsCmd := flag.NewFlagSet("logs", flag.ContinueOnError)
	logsCmd.Usage = printLogsUsage
	nLines := logsCmd.Int("n", 100, "")
	follow := logsCmd.Bool("f", false, "")

	err := logsCmd.Parse(args)
	shared.HandleFlagError(err)
	if *nLines < 0 {
		p.Error(ui.NewAppError("FLAG_ERR", "Error: --lines must be greater than or equal to 0", "", ui.SeverityError, nil))
		os.Exit(1)
	}

	targetLogFile := shared.InfoLogFilePath
	if logsCmd.NArg() > 0 && logsCmd.Arg(0) == "error" {
		targetLogFile = shared.ErrorLogFilePath
	}

	if _, err := os.Stat(targetLogFile); os.IsNotExist(err) {
		p.Error(ui.NewAppError("LOG_MISSING", fmt.Sprintf("Log file does not exist: %s", targetLogFile), "", ui.SeverityWarning, nil))
		return
	}

	if *follow {
		t, err := tail.TailFile(targetLogFile, tail.Config{
			Follow:    true,
			ReOpen:    true,
			MustExist: false,
			Logger:    tail.DiscardingLogger,
		})
		shared.CheckError(err, "Failed to tail log file: %v", err)
		for line := range t.Lines {
			p.Print(line.Text)
		}
	} else {
		recentLogs, err := shared.RecentNonBlankLines(targetLogFile, *nLines)
		shared.CheckError(err, "Failed to read log file: %v", err)

		if util.IsNotBlank(recentLogs) {
			p.Print(recentLogs)
		}
	}
}

func printGenerateUsage() { ui.PrintHelp(p, ui.HelpConfigJSON, "generate") }

func handleGenerateCmd(args []string) {
	generateCmd := flag.NewFlagSet("generate", flag.ContinueOnError)
	generateCmd.Usage = printGenerateUsage

	err := generateCmd.Parse(args)
	shared.HandleFlagError(err)

	baseDir, err := os.Getwd()
	if err != nil {
		baseDir = "."
	}

	p.Info("Generating local environment template...")

	tmplName := "local_config.tmpl.json"
	tmplData, err := templates.FS.ReadFile(tmplName)
	shared.CheckError(err, "Error: could not find embedded template '%s': %v", tmplName, err)

	tmplDir := filepath.Join(baseDir, "templates")
	shared.CheckError(os.MkdirAll(tmplDir, 0755), "Error: could not create templates directory: %v")

	outPath := filepath.Join(tmplDir, tmplName)
	if _, err := os.Stat(outPath); err == nil {
		p.Info(fmt.Sprintf("Template '%s' already exists in '%s'. Skipping.", tmplName, tmplDir))
	} else if !os.IsNotExist(err) {
		p.Error(ui.NewAppError("TMPL_INSPECT_ERR", fmt.Sprintf("Error: could not inspect template file '%s'", outPath), err.Error(), ui.SeverityError, err))
		os.Exit(1)
	}
	shared.CheckError(os.WriteFile(outPath, tmplData, 0644), "Error: could not write template file: %v")
	p.Success(fmt.Sprintf("Successfully released '%s' to '%s'.", tmplName, tmplDir))

	envPath := filepath.Join(baseDir, ".env")
	if _, err := os.Stat(envPath); err == nil {
		p.Info(".env file already exists. Skipping parameter generation.")
		os.Exit(0)
	}

	envContent := `DO_IP=
UUID=
PUBLIC_KEY=
SHORT_ID=

# List of domain suffixes to bypass proxy, formatted as JSON array elements
# E.g., [".local", ".lan", ".company.internal"]
BYPASS_DOMAINS=[".local", ".lan"]
`

	shared.CheckError(os.WriteFile(envPath, []byte(envContent), 0600), "Error: could not write .env file: %v")
	p.Success(fmt.Sprintf("Successfully generated .env template at '%s'.", envPath))

	p.Info("Opening .env file for configuration...")
	if err := util.OpenFileInEditor(envPath); err != nil {
		p.Warning("Notice: could not automatically open .env file in editor. Please open it manually to fill in the parameters from your remote node.")
	} else {
		p.Warning("Please paste the UUID, PUBLIC_KEY, and SHORT_ID from your remote node, then save the file.")
	}
}

func printRenderUsage() { ui.PrintHelp(p, ui.HelpConfigJSON, "render") }

func handleRenderCmd(args []string) {
	renderCmd := flag.NewFlagSet("render", flag.ContinueOnError)
	renderCmd.Usage = printRenderUsage
	configPath := renderCmd.String("config", "", "Path to the input template file")
	outPath := renderCmd.String("out", "", "Path to the output JSON file")
	ci := renderCmd.Bool("ci", false, "Inject CI rules")
	indexSrs := renderCmd.Bool("index-srs", false, "Download and index .srs files")

	err := renderCmd.Parse(args)
	shared.HandleFlagError(err)

	if *configPath == "" || *outPath == "" {
		p.Error(ui.NewAppError("ARGS_MISSING", "--config and --out are required.", "", ui.SeverityError, nil))
		printRenderUsage()
		os.Exit(1)
	}

	envMap := shared.ProcessEnvMap()
	content, err := util.RenderConfigTemplate(*configPath, envMap)
	shared.CheckError(err, "Failed to render template: %v", err)

	if *ci {
		content, err = util.InjectCIRules(content)
		shared.CheckError(err, "Failed to inject CI rules: %v", err)
	}

	if *indexSrs {
		cwd, err := os.Getwd()
		shared.CheckError(err, "Failed to get current working directory: %v", err)
		srsDir := filepath.Join(cwd, "srs")
		content, err = shared.ProcessRuleSets(content, srsDir)
		shared.CheckError(err, "Failed to process rule sets: %v", err)
		p.Success(fmt.Sprintf("Successfully indexed rule sets to %s", srsDir))
	}

	shared.CheckError(os.WriteFile(*outPath, []byte(content), 0600), "Failed to write output file: %v")

	p.Success(fmt.Sprintf("Successfully rendered configuration to %s", *outPath))
}

func printTweakUsage() { ui.PrintHelp(p, ui.HelpConfigJSON, "tweak") }

func handleTweakCmd(args []string) {
	tweakCmd := flag.NewFlagSet("tweak", flag.ContinueOnError)
	tweakCmd.Usage = printTweakUsage
	configPath := tweakCmd.String("config", defaultConfig, "Path to the input template file")

	err := tweakCmd.Parse(args)
	shared.HandleFlagError(err)

	baseDir, err := shared.ExecutableDir()
	shared.CheckError(err, "Failed to get executable path: %v", err)

	var tplPath = *configPath
	if !filepath.IsAbs(tplPath) {
		tplPath = filepath.Join(baseDir, tplPath)
	}

	envPath := filepath.Join(baseDir, ".env")
	envMap, err := util.LoadEnvMap(envPath)
	if err != nil {
		p.Warning(fmt.Sprintf("Notice: could not load environment file: %v", err))
		envMap = make(map[string]string)
	}

	content, err := util.RenderConfigTemplate(tplPath, envMap)
	shared.CheckError(err, "Failed to render config template: %v", err)

	overridePath := filepath.Join(baseDir, tweak.OverrideFileName)
	err = tweak.RunTUI(content, overridePath)
	shared.CheckError(err, "Failed to run TUI: %v", err)
}

func splitServiceArgs(args []string) (flagArgs []string, configPath string) {
	configPath = defaultConfig
	for i := 0; i < len(args); i++ {
		if args[i] == "--config" && i+1 < len(args) {
			configPath = args[i+1]
			i++
		} else {
			flagArgs = append(flagArgs, args[i])
		}
	}
	return flagArgs, configPath
}

func runServiceCommand(s service.Service, svcCmd string) {
	var baseDir string
	var err error

	if serviceCommandNeedsSingBox(svcCmd) {
		baseDir, err = shared.ExecutableDir()
		shared.CheckError(err, "Failed to get executable path: %v", err)
		shared.CheckError(shared.EnsureBundledSingBox(baseDir), "Pre-flight check failed: %v")
	}

	if serviceCommandNeedsElevation(svcCmd) && !util.IsAdmin() {
		p.Info("Elevated privileges required for service command. Attempting to elevate...")
		err := util.RunMeElevated()
		if err != nil {
			if strings.Contains(err.Error(), "elevated process exited with code") {
				os.Exit(1)
			}
			p.Error(ui.NewAppError("ELEVATE_FAILED", "Permission denied: please run this command as an administrator/root.", err.Error(), ui.SeverityError, err))
			os.Exit(1)
		}
		return
	}

	if serviceCommandNeedsSingBox(svcCmd) {
		logsDir := filepath.Join(baseDir, "logs")
		shared.ErrorLogFilePath = filepath.Join(logsDir, "error.log")
	}

	err = service.Control(s, svcCmd)
	if err != nil {
		if strings.Contains(err.Error(), "Access is denied") || strings.Contains(err.Error(), "permission denied") || strings.Contains(err.Error(), "requires elevation") {
			p.Error(ui.NewAppError("PERM_DENIED", "Permission denied: please run this command as an administrator/root.", "", ui.SeverityError, nil))
			os.Exit(1)
		}

		p.Error(ui.NewAppError("SVC_CMD_ERR", fmt.Sprintf("Failed to execute service command '%s'", svcCmd), err.Error(), ui.SeverityError, err))
		os.Exit(1)
	}

	if serviceCommandNeedsSingBox(svcCmd) {
		time.Sleep(2 * time.Second)
		recentErrors, err := shared.RecentNonBlankLines(shared.ErrorLogFilePath, 5)
		if err != nil && !os.IsNotExist(err) {
			p.Warning(fmt.Sprintf("Warning: failed to read startup error log: %v", err))
		}
		if err == nil && util.IsNotBlank(recentErrors) {
			p.Warning(fmt.Sprintf("Service command '%s' executed, but errors occurred shortly after:\n%s", svcCmd, recentErrors))
			os.Exit(1)
		}
	}

	p.Success(fmt.Sprintf("Service command '%s' executed successfully.", svcCmd))
}

func main() {
	if len(os.Args) < 2 {
		ui.PrintHelp(p, ui.HelpConfigJSON, "main_local")
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

	if cmd == "generate" {
		handleGenerateCmd(os.Args[2:])
		return
	}

	if cmd == "render" {
		handleRenderCmd(os.Args[2:])
		return
	}

	if cmd == "tweak" {
		handleTweakCmd(os.Args[2:])
		return
	}

	if cmd == "run" {
		for _, arg := range os.Args[2:] {
			if arg == "-h" || arg == "--help" {
				ui.PrintHelp(p, ui.HelpConfigJSON, "run")
				os.Exit(0)
			}
		}
		configPath := defaultConfig
		// Quick parse for --config
		for i, arg := range os.Args {
			if arg == "--config" && i+1 < len(os.Args) {
				configPath = os.Args[i+1]
			}
		}
		runDaemonMode(configPath)
		return
	}

	if isServiceCommand(cmd) {
		// Check for help flag
		for _, arg := range os.Args[2:] {
			if arg == "-h" || arg == "--help" {
				ui.PrintHelp(p, ui.HelpConfigJSON, cmd)
				os.Exit(0)
			}
		}

		_, configPath := splitServiceArgs(os.Args[2:])

		svcConfig := &service.Config{
			Name:        serviceName,
			DisplayName: serviceDisplayName,
			Description: serviceDescription,
			Option: service.KeyValue{
				"OnFailure":              "restart",
				"OnFailureDelayDuration": "10s",
				"OnFailureResetPeriod":   600,
			},
			Arguments: []string{"run", "--config", configPath},
		}

		prg := &program{templatePath: configPath}
		s, err := service.New(prg, svcConfig)
		shared.CheckError(err, "Failed to create service: %v", err)

		runServiceCommand(s, cmd)
		return
	}

	// For any other arguments or daemon launch logic checking
	if len(os.Args) >= 2 && os.Args[1] == "--config" {
		configPath := defaultConfig
		if len(os.Args) > 2 {
			configPath = os.Args[2]
		}
		runDaemonMode(configPath)
		return
	}

	p.Error(ui.NewAppError("UNKNOWN_CMD", fmt.Sprintf("Error: unknown command %q", cmd), "", ui.SeverityError, nil))
	ui.PrintHelp(p, ui.HelpConfigJSON, "main_local")
	os.Exit(1)
}

func runDaemonMode(configPath string) {
	svcConfig := &service.Config{
		Name:        serviceName,
		DisplayName: serviceDisplayName,
		Description: serviceDescription,
		Option: service.KeyValue{
			"OnFailure":              "restart",
			"OnFailureDelayDuration": "10s",
			"OnFailureResetPeriod":   600,
		},
		Arguments: []string{"run", "--config", configPath},
	}

	prg := &program{templatePath: configPath}
	s, err := service.New(prg, svcConfig)
	shared.CheckError(err, "Failed to create service: %v", err)

	err = s.Run()
	if err != nil {
		log.Fatalf("Service runtime error: %v", err)
	}
}
