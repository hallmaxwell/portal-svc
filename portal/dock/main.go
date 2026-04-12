package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

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

func main() {
	baseDir, err := os.Getwd()
	if err != nil {
		baseDir = "."
	}

	envPath := filepath.Join(baseDir, ".env")
	templatePath := filepath.Join(baseDir, "config.template.json")
	outPath := filepath.Join(os.TempDir(), "dock.config.run.json")

	if _, err := os.Stat(envPath); os.IsNotExist(err) {
		fmt.Printf("Can't find .env file in %s\n", baseDir)
		return
	}

	envMap := make(map[string]string)
	envFile, _ := os.Open(envPath)
	defer envFile.Close()
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

	tempData, _ := os.ReadFile(templatePath)
	content := string(tempData)
	for key, val := range envMap {
		if isRawJSONValue(val) {
			content = strings.ReplaceAll(content, `"{`+key+`}"`, val)
			content = strings.ReplaceAll(content, `{`+key+`}`, val)
		} else {
			content = strings.ReplaceAll(content, `{`+key+`}`, val)
		}
	}

	os.WriteFile(outPath, []byte(content), 0644)
	cmd := exec.Command("sing-box", "run", "-c", outPath)
	cmd.Dir = baseDir
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Println("Starting sing-box with the generated configuration...")
	if err := cmd.Run(); err != nil {
		fmt.Println("Error running sing-box:", err)
	}

	os.Remove(outPath)
	fmt.Println("sing-box has exited. Temporary configuration file removed.")
}