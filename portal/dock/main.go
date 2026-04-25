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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kardianos/service"
	"github.com/nxadm/tail"
)

var logFilePath = filepath.Join(os.TempDir(), "dock.portal.svc.log")

type boundedLogWriter struct {
	filePath string
	maxLines int
	mu       sync.Mutex
}

func (w *boundedLogWriter) Write(p[]byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	var validLines[]string
	data, err := os.ReadFile(w.filePath)
	if err == nil {
		lines := strings.Split(string(data), "\n")
		for _, l := range lines {
			if len(strings.TrimSpace(l)) > 0 {
				validLines = append(validLines, l)
			}
		}
	}

	newLines := strings.Split(strings.TrimSuffix(string(p), "\n"), "\n")
	for _, l := range newLines {
		if len(strings.TrimSpace(l)) > 0 {
			validLines = append(validLines, l)
		}
	}

	if len(validLines) > w.maxLines {
		validLines = validLines[len(validLines)-w.maxLines:]
	}

	outData := strings.Join(validLines, "\n") + "\n"
	_ = os.WriteFile(w.filePath,[]byte(outData), 0666)

	return len(p), nil
}

func setupBackgroundLogger() {
	writer := &boundedLogWriter{filePath: logFilePath, maxLines: 100}
	log.SetOutput(writer)
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
}

func killExistingSingBox() {
	if runtime.GOOS == "windows" {
		_ = exec.Command("taskkill", "/F", "/T", "/IM", "sing-box.exe").Run()
	} else {
		_ = exec.Command("killall", "-9", "sing-box").Run()
	}
}

func isRawJSONValue(val string) bool {
	if _, err := strconv.Atoi(val); err == nil {
		return true
	}
	if _, err := strconv.ParseFloat(val, 64); err == nil {
		return true
	}
	if val == "true" || val == "false" {
		return true
	}
	if strings.HasPrefix(val, "[") && strings.HasSuffix(val, "]") {
		return true
	}
	if strings.HasPrefix(val, "{") && strings.HasSuffix(val, "}") {
		return true
	}
	return false
}

type program struct {
	cmd      *exec.Cmd
	outPath  string
	exit     chan struct{}
	stopping bool
}

func (p *program) Start(s service.Service) error {
	p.exit = make(chan struct{})
	go p.run()
	go p.monitorNetwork()
	return nil
}

func (p *program) run() {
	exe, err := os.Executable()
	if err != nil {
		log.Fatalf("Failed to get executable path: %v", err)
	}
	baseDir := filepath.Dir(exe)

	singBoxBin := "sing-box"
	if runtime.GOOS == "windows" {
		singBoxBin = "sing-box.exe"
	}
	singBoxPath := filepath.Join(baseDir, "core", singBoxBin)

	if _, err := os.Stat(singBoxPath); os.IsNotExist(err) {
		log.Fatalf("Dependencies not found: %s", singBoxPath)
	}

	envPath := filepath.Join(baseDir, ".env")
	templatePath := filepath.Join(baseDir, "config.template.json")
	p.outPath = filepath.Join(os.TempDir(), "dock.config.run.json")

	if _, err := os.Stat(envPath); os.IsNotExist(err) {
		log.Fatalf("Environment file not found")
	}

	envMap := make(map[string]string)
	envFile, err := os.Open(envPath)
	if err != nil {
		log.Fatalf("Failed to open environment file: %v", err)
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

	tempData, err := os.ReadFile(templatePath)
	if err != nil {
		log.Fatalf("Failed to read config template: %v", err)
	}

	content := string(tempData)
	for key, val := range envMap {
		if isRawJSONValue(val) {
			content = strings.ReplaceAll(content, `"{`+key+`}"`, val)
			content = strings.ReplaceAll(content, `{`+key+`}`, val)
		} else {
			content = strings.ReplaceAll(content, `{`+key+`}`, val)
		}
	}

	os.WriteFile(p.outPath,[]byte(content), 0644)

	killExistingSingBox()

	p.cmd = exec.Command(singBoxPath, "run", "-c", p.outPath)
	p.cmd.Dir = baseDir
	p.cmd.Stdout = log.Writer()
	p.cmd.Stderr = log.Writer()
	p.cmd.Start()
	p.cmd.Wait()

	p.cleanup()

	if !p.stopping {
		log.Fatalf("Sing-box process exited unexpectedly")
	}
}

func (p *program) cleanup() {
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
	if p.outPath != "" {
		_ = os.Remove(p.outPath)
	}
}

func (p *program) monitorNetwork() {
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
					log.Fatalf("Network health check failed, triggering restart")
				}
			} else {
				failCount = 0
				conn.Close()
			}
		}
	}
}

func (p *program) Stop(s service.Service) error {
	p.stopping = true
	close(p.exit)
	p.cleanup()
	return nil
}

func handleLogsCmd(args[]string) {
	logsCmd := flag.NewFlagSet("logs", flag.ExitOnError)
	nLines := logsCmd.Int("n", 100, "")
	follow := logsCmd.Bool("f", false, "")

	logsCmd.Parse(args)

	if _, err := os.Stat(logFilePath); os.IsNotExist(err) {
		fmt.Printf("Log file does not exist: %s\n", logFilePath)
		return
	}

	if *follow {
		t, err := tail.TailFile(logFilePath, tail.Config{
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
		data, err := os.ReadFile(logFilePath)
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
	svcConfig := &service.Config{
		Name:        "SingBoxWrapper",
		DisplayName: "Sing-Box Wrapper Service",
		Description: "Sing-Box background service with auto-recovery",
		Option: service.KeyValue{
			"OnFailure":              "restart",
			"OnFailureDelayDuration": "10s",
			"OnFailureResetPeriod":   600,
		},
	}

	prg := &program{}
	s, err := service.New(prg, svcConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create service: %v\n", err)
		os.Exit(1)
	}

	if len(os.Args) > 1 {
		cmd := os.Args[1]

		if cmd == "logs" {
			handleLogsCmd(os.Args[2:])
			return
		}

		err = service.Control(s, cmd)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to execute service command '%s': %v\n", cmd, err)
			os.Exit(1)
		}
		fmt.Printf("Service command '%s' executed successfully.\n", cmd)
		return
	}

	setupBackgroundLogger()

	err = s.Run()
	if err != nil {
		log.Fatalf("Service runtime error: %v", err)
	}
}