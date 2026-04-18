package main

import (
	"bufio"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"log"
	"github.com/kardianos/service"
)

func initLogger() *os.File {
    exe, _ := os.Executable()
    baseDir := filepath.Dir(exe)
    logPath := filepath.Join(baseDir, "portal_service.log")
    
    f, err := os.OpenFile(logPath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
    if err != nil {
        return nil
    }
    log.SetOutput(f)
    log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
    return f
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
		log.Fatalf("Read template flie failed: %v", err)
	}
	baseDir := filepath.Dir(exe)

	envPath := filepath.Join(baseDir, ".env")
	templatePath := filepath.Join(baseDir, "config.template.json")
	p.outPath = filepath.Join(os.TempDir(), "dock.config.run.json")

	if _, err := os.Stat(envPath); os.IsNotExist(err) {
		log.Fatalf("Read template flie failed: %v", err)
	}

	envMap := make(map[string]string)
	envFile, err := os.Open(envPath)
	if err != nil {
		log.Fatalf("Read template flie failed: %v", err)
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
		log.Fatalf("Read template flie failed: %v", err)
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

	p.cmd = exec.Command("sing-box", "run", "-c", p.outPath)
	p.cmd.Dir = baseDir
	p.cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	p.cmd.Start()
	p.cmd.Wait()

	p.cleanup()

	if !p.stopping {
		log.Fatalf("Read template flie failed: %v", err)
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
					log.Fatalf("Read template flie failed: %v", err)
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

func main() {
	logFile := initLogger()
    if logFile != nil {
        defer logFile.Close()
	}
	
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
		log.Fatalf("Read template flie failed: %v", err)
	}

	if len(os.Args) > 1 {
		err = service.Control(s, os.Args[1])
		if err != nil {
			log.Fatalf("Read template flie failed: %v", err)
		}
		return
	}

	err = s.Run()
	if err != nil {
		log.Fatalf("Read template flie failed: %v", err)
	}
}