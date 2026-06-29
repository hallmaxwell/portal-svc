package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"hawego/portal/shared"
	"hawego/portal/util"
)

const (
	defaultRemoteConfig = "templates/remote_config.tmpl.json"
	tempConfig          = "remote.config.run.json"
)

func runRemote(templatePath string) {
	if err := shared.InitLogFiles(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize log files: %v\n", err)
		os.Exit(1)
	}

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

func printRenderUsage() {
	fmt.Println(`Render a configuration template with environment variables

Usage:
  portal-remote render [flags]

Flags:
      --config string   Path to the input template file
      --out string      Path to the output JSON file
      --ci              Inject CI rules
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
		content, err = shared.ProcessRuleSets(content, srsDir)
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

func printUsage() {
	fmt.Println(`Usage:
  portal-remote [flags]
  portal-remote render [flags]

Flags:
      --config string   Path to template config (default "templates/remote_config.tmpl.json")
  -h, --help            help for portal-remote`)
}

func main() {
	if len(os.Args) > 1 {
		cmd := os.Args[1]

		if cmd == "-h" || cmd == "--help" {
			printUsage()
			os.Exit(0)
		}

		if cmd == "render" {
			handleRenderCmd(os.Args[2:])
			return
		}
	}

	// Parse flags for main runner mode
	configPath := defaultRemoteConfig
	for i := 1; i < len(os.Args); i++ {
		if os.Args[i] == "--config" && i+1 < len(os.Args) {
			configPath = os.Args[i+1]
			break
		}
	}

	runRemote(configPath)
}
