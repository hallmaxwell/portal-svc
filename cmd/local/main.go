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

	"hawego/portal/shared"
	"hawego/portal/templates"
	"hawego/portal/util"

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

	content, err = shared.ProcessRuleSets(content, srsDir, false)
	if err != nil {
		shared.SysLogError(fmt.Sprintf("Failed to process rule sets: %v", err), false)
		// non-fatal, continue with original content
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

func printMainUsage() {
	fmt.Println(`Usage:
  portal-local [command] [flags]

Available Commands:
  install     Install as System Service
  start       Start Service
  stop        Stop Service
  restart     Restart Service
  uninstall   Remove Service
  logs        View service logs
  generate    Generate local environment template and .env file
  render      Render configuration template with environment variables

Flags:
  -h, --help   help for portal-local`)
}

func printLogsUsage() {
	fmt.Println(`View service logs

Usage:
  portal-local logs [flags] [error|info]

Flags:
  -f, --follow          Follow log output
  -n, --lines int       Number of lines to show (default 100)
  -h, --help            help for logs`)
}

func handleLogsCmd(args []string) {
	if err := shared.InitLogPaths(); err != nil {
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

	targetLogFile := shared.InfoLogFilePath
	if logsCmd.NArg() > 0 && logsCmd.Arg(0) == "error" {
		targetLogFile = shared.ErrorLogFilePath
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
		recentLogs, err := shared.RecentNonBlankLines(targetLogFile, *nLines)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to read log file: %v\n", err)
			os.Exit(1)
		}

		if util.IsNotBlank(recentLogs) {
			fmt.Print(recentLogs)
		}
	}
}

func printGenerateUsage() {
	fmt.Println(`Generate local environment template and .env file

Usage:
  portal-local generate [flags]

Flags:
  -h, --help            help for generate`)
}

func handleGenerateCmd(args []string) {
	generateCmd := flag.NewFlagSet("generate", flag.ContinueOnError)
	generateCmd.Usage = printGenerateUsage

	err := generateCmd.Parse(args)
	if err == flag.ErrHelp {
		os.Exit(0)
	} else if err != nil {
		os.Exit(1)
	}

	baseDir, err := os.Getwd()
	if err != nil {
		baseDir = "."
	}

	fmt.Println("Generating local environment template...")

	tmplName := "local_config.tmpl.json"
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

	envPath := filepath.Join(baseDir, ".env")
	if _, err := os.Stat(envPath); err == nil {
		fmt.Println(".env file already exists. Skipping parameter generation.")
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

	if err := os.WriteFile(envPath, []byte(envContent), 0600); err != nil {
		fmt.Fprintf(os.Stderr, "Error: could not write .env file: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Successfully generated .env template at '%s'.\n", envPath)

	fmt.Println("\nOpening .env file for configuration...")
	if err := util.OpenFileInEditor(envPath); err != nil {
		fmt.Fprintf(os.Stderr, "Notice: could not automatically open .env file in editor. Please open it manually to fill in the parameters from your remote node.\n")
	} else {
		fmt.Println("Please paste the UUID, PUBLIC_KEY, and SHORT_ID from your remote node, then save the file.")
	}
}

func printRenderUsage() {
	fmt.Println(`Render a configuration template with environment variables

Usage:
  portal-local render [flags]

Flags:
      --config string   Path to the input template file
      --out string      Path to the output JSON file
      --ci              Inject CI rules (ci-direct-out, disable auto_route)
      --index-srs       Download and index .srs files to local srs/ folder
  -h, --help            help for render`)
}

func handleRenderCmd(args []string) {
	renderCmd := flag.NewFlagSet("render", flag.ContinueOnError)
	renderCmd.Usage = printRenderUsage
	configPath := renderCmd.String("config", "", "Path to the input template file")
	outPath := renderCmd.String("out", "", "Path to the output JSON file")
	ci := renderCmd.Bool("ci", false, "Inject CI rules")
	indexSrs := renderCmd.Bool("index-srs", false, "Download and index .srs files")

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

	envMap := shared.ProcessEnvMap()
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

	if *indexSrs {
		cwd, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to get current working directory: %v\n", err)
			os.Exit(1)
		}
		srsDir := filepath.Join(cwd, "srs")
		content, err = shared.ProcessRuleSets(content, srsDir, true)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to process rule sets: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Successfully indexed rule sets to %s\n", srsDir)
	}

	if err := os.WriteFile(*outPath, []byte(content), 0600); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write output file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully rendered configuration to %s\n", *outPath)
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
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to get executable path: %v\n", err)
			os.Exit(1)
		}
		if err := shared.EnsureBundledSingBox(baseDir); err != nil {
			fmt.Fprintf(os.Stderr, "Pre-flight check failed: %v\n", err)
			os.Exit(1)
		}
	}

	if serviceCommandNeedsElevation(svcCmd) && !util.IsAdmin() {
		fmt.Println("Elevated privileges required for service command. Attempting to elevate...")
		err := util.RunMeElevated()
		if err != nil {
			if strings.Contains(err.Error(), "elevated process exited with code") {
				os.Exit(1)
			}
			fmt.Fprintf(os.Stderr, "Failed to elevate privileges: %v\n", err)
			fmt.Fprintf(os.Stderr, "Permission denied: please run this command as an administrator/root.\n")
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
			fmt.Fprintf(os.Stderr, "Permission denied: please run this command as an administrator/root.\n")
			os.Exit(1)
		}

		fmt.Fprintf(os.Stderr, "Failed to execute service command '%s': %v\n", svcCmd, err)
		os.Exit(1)
	}

	if serviceCommandNeedsSingBox(svcCmd) {
		time.Sleep(2 * time.Second)
		recentErrors, err := shared.RecentNonBlankLines(shared.ErrorLogFilePath, 5)
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

func main() {
	if len(os.Args) < 2 {
		// When no args provided, start the daemon (run by the service manager)
		runDaemonMode(defaultConfig)
		return
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

	if isServiceCommand(cmd) {
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
			Arguments: []string{"--config", configPath},
		}

		prg := &program{templatePath: configPath}
		s, err := service.New(prg, svcConfig)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create service: %v\n", err)
			os.Exit(1)
		}

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

	fmt.Printf("Error: unknown command %q\n", cmd)
	printMainUsage()
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
		Arguments: []string{"--config", configPath},
	}

	prg := &program{templatePath: configPath}
	s, err := service.New(prg, svcConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create service: %v\n", err)
		os.Exit(1)
	}

	err = s.Run()
	if err != nil {
		log.Fatalf("Service runtime error: %v", err)
	}
}
