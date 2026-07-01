package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"portal-svc/shared"
	"portal-svc/ui"
	"portal-svc/util"
)

const (
	defaultRemoteConfig = "templates/remote_config.tmpl.json"
	tempConfig          = "remote.config.run.json"
)

func runRemote(templatePath string) {
	shared.CheckError(shared.InitLogFiles(), "Failed to initialize log files: %v")

	envMap := shared.ProcessEnvMap()
	content, err := util.RenderConfigTemplate(templatePath, envMap)
	if err != nil {
		shared.SysLogError(fmt.Sprintf("Failed to render config template: %v", err), true)
		os.Exit(1)
	}

	exeDir, err := shared.ExecutableDir()
	if err != nil {
		shared.SysLogError(fmt.Sprintf("Failed to get executable directory: %v", err), true)
		os.Exit(1)
	}
	srsDir := filepath.Join(exeDir, "srs")

	content, err = shared.ProcessRuleSets(content, srsDir)
	if err != nil {
		shared.SysLogError(fmt.Sprintf("Failed to process rule sets: %v", err), true)
	}

	outPath := filepath.Join(os.TempDir(), tempConfig)
	if err := os.WriteFile(outPath, []byte(content), 0600); err != nil {
		shared.SysLogError(fmt.Sprintf("Failed to write rendered config: %v", err), true)
		os.Exit(1)
	}

	cmd := exec.Command("sing-box", "run", "-c", outPath)
	cmd.Stdout = &shared.SingBoxLogWriter{PrintToStdout: true}
	cmd.Stderr = &shared.SingBoxLogWriter{PrintToStdout: true}

	shared.SysLogInfo("Remote Node Launching...", true)

	if err := cmd.Start(); err != nil {
		shared.SysLogError(fmt.Sprintf("Launch failed: %v", err), true)
		return
	}

	go func() {
		time.Sleep(2 * time.Second)
		if err := os.Remove(outPath); err != nil && !os.IsNotExist(err) {
			shared.SysLogError(fmt.Sprintf("Failed to remove %s: %v", tempConfig, err), true)
			return
		}
		shared.SysLogInfo(fmt.Sprintf("%s cleared, remote node is running.", tempConfig), true)
	}()

	if err := cmd.Wait(); err != nil {
		shared.SysLogError(fmt.Sprintf("Remote node exited with error: %v", err), true)
		os.Exit(1)
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

func printGenerateUsage() { ui.PrintHelp(p, ui.HelpConfigJSON, "generate") }

func handleGenerateCmd(args []string) {
	generateCmd := flag.NewFlagSet("generate", flag.ContinueOnError)
	generateCmd.Usage = printGenerateUsage

	err := generateCmd.Parse(args)
	shared.HandleFlagError(err)

	cwd, err := os.Getwd()
	shared.CheckError(err, "Failed to get current directory: %v", err)

	tmplDir := filepath.Join(cwd, "templates")
	shared.CheckError(os.MkdirAll(tmplDir, 0755), "Failed to create templates directory: %v")

	tmplPath := filepath.Join(tmplDir, "remote_config.tmpl.json")
	if _, err := os.Stat(tmplPath); os.IsNotExist(err) {
		// Download from GitHub
		p.Info("Downloading latest remote_config.tmpl.json...")
		url := "https://raw.githubusercontent.com/hallmaxwell/portal-svc/main/templates/remote_config.tmpl.json"
		shared.CheckError(shared.DownloadFile(url, tmplPath), "Failed to download template: %v")
		p.Success(fmt.Sprintf("Downloaded template to %s", tmplPath))
	} else {
		p.Info(fmt.Sprintf("Template already exists at %s", tmplPath))
	}

	envPath := filepath.Join(cwd, ".env")
	if _, err := os.Stat(envPath); err == nil {
		p.Info(".env file already exists. Skipping parameter generation.")
		os.Exit(0)
	}

	p.Info("Generating cryptographic parameters...")
	uuid, _ := shared.GenerateUUID()
	shortID, _ := shared.GenerateShortID()
	privKey, pubKey, _ := shared.GenerateX25519KeyPair()

	envContent := fmt.Sprintf(`UUID=%s
PRIVATE_KEY=%s
SHORT_ID=%s

# Optional Proxy Chain Parameters
PROXY_IP=
PROXY_PORT=
PROXY_USERNAME=
PROXY_PASSWORD=
`, uuid, privKey, shortID)

	shared.CheckError(os.WriteFile(envPath, []byte(envContent), 0600), "Failed to write .env file: %v")

	p.Success("Initialization Complete! Server .env file created.")
	p.Warning("!!! ACTION REQUIRED !!!")
	p.Info("Copy the following parameters to your LOCAL client's .env file:")
	p.Print(fmt.Sprintf("UUID=%s\nPUBLIC_KEY=%s\nSHORT_ID=%s", uuid, pubKey, shortID))
	p.Warning("Keep your server's PRIVATE_KEY secret and safe.")
}

func printUsage() { ui.PrintHelp(p, ui.HelpConfigJSON, "main_remote") }

var p = ui.NewPrinter()

func main() {
	if len(os.Args) < 2 {
		ui.PrintHelp(p, ui.HelpConfigJSON, "main_remote")
		os.Exit(0)
	}

	cmd := os.Args[1]

	if cmd == "-h" || cmd == "--help" {
		printUsage()
		os.Exit(0)
	}

	if cmd == "render" {
		handleRenderCmd(os.Args[2:])
		return
	}

	if cmd == "generate" {
		handleGenerateCmd(os.Args[2:])
		return
	}

	if cmd != "run" {
		p.Error(ui.NewAppError("UNKNOWN_CMD", fmt.Sprintf("Error: unknown command %q", cmd), "", ui.SeverityError, nil))
		os.Exit(1)
	}

	// Parse flags for main runner mode (run command)
	configPath := defaultRemoteConfig
	for i := 1; i < len(os.Args); i++ {
		if os.Args[i] == "--config" && i+1 < len(os.Args) {
			configPath = os.Args[i+1]
			break
		}
	}

	runRemote(configPath)
}
