package tui

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"
)

type SetupData struct {
	DO_IP          string
	UUID           string
	PUBLIC_KEY     string
	SHORT_ID       string
	BYPASS_DOMAINS string
	Confirm        bool
	EnvMap         map[string]string
	EnvLines       []string
	EnvPath        string
}

func GetSetupData() *SetupData {
	envMap := make(map[string]string)
	exe, _ := os.Executable()
	baseDir := filepath.Dir(exe)
	if strings.Contains(exe, "go-build") {
		cwd, _ := os.Getwd()
		baseDir = cwd
	}
	envPath := filepath.Join(baseDir, ".env")

	var envLines []string

	if envFile, err := os.Open(envPath); err == nil {
		scanner := bufio.NewScanner(envFile)
		for scanner.Scan() {
			line := scanner.Text()
			envLines = append(envLines, line)
			trimmedLine := strings.TrimSpace(line)
			if trimmedLine == "" || strings.HasPrefix(trimmedLine, "#") {
				continue
			}
			parts := strings.SplitN(trimmedLine, "=", 2)
			if len(parts) == 2 {
				envMap[strings.TrimSpace(parts[0])] = strings.Trim(strings.TrimSpace(parts[1]), `"'`)
			}
		}
		envFile.Close()
	}

	getDef := func(key, def string) string {
		if val, ok := envMap[key]; ok && val != "" {
			return val
		}
		return def
	}

	return &SetupData{
		DO_IP:          getDef("DO_IP", ""),
		UUID:           getDef("UUID", ""),
		PUBLIC_KEY:     getDef("PUBLIC_KEY", ""),
		SHORT_ID:       getDef("SHORT_ID", ""),
		BYPASS_DOMAINS: getDef("BYPASS_DOMAINS", `["example.com", "google.cn"]`),
		EnvMap:         envMap,
		EnvLines:       envLines,
		EnvPath:        envPath,
	}
}

func BuildSetupForm(data *SetupData) *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Transit Node IP (DO_IP)").
				Value(&data.DO_IP),
			huh.NewInput().
				Title("VLESS User UUID (UUID)").
				Value(&data.UUID),
			huh.NewInput().
				Title("REALITY Public Key (PUBLIC_KEY)").
				Value(&data.PUBLIC_KEY),
			huh.NewInput().
				Title("REALITY Short ID (SHORT_ID)").
				Value(&data.SHORT_ID),
			huh.NewInput().
				Title("Bypass Domains (BYPASS_DOMAINS)").
				Description("JSON array format").
				Value(&data.BYPASS_DOMAINS),
		),
		huh.NewGroup(
			huh.NewConfirm().
				Title("Save these parameters to .env?").
				Value(&data.Confirm),
		),
	)
}

func SaveSetup(data *SetupData) error {
	newVals := map[string]string{
		"DO_IP":          data.DO_IP,
		"UUID":           data.UUID,
		"PUBLIC_KEY":     data.PUBLIC_KEY,
		"SHORT_ID":       data.SHORT_ID,
		"BYPASS_DOMAINS": data.BYPASS_DOMAINS,
	}

	var newEnvLines []string
	updatedKeys := make(map[string]bool)

	for _, line := range data.EnvLines {
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine == "" || strings.HasPrefix(trimmedLine, "#") {
			newEnvLines = append(newEnvLines, line)
			continue
		}
		parts := strings.SplitN(trimmedLine, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			if newVal, ok := newVals[key]; ok {
				if key == "BYPASS_DOMAINS" {
					newEnvLines = append(newEnvLines, fmt.Sprintf("%s=%s", key, newVal))
				} else {
					newEnvLines = append(newEnvLines, fmt.Sprintf(`%s="%s"`, key, newVal))
				}
				updatedKeys[key] = true
			} else {
				newEnvLines = append(newEnvLines, line)
			}
		} else {
			newEnvLines = append(newEnvLines, line)
		}
	}

	for key, val := range newVals {
		if !updatedKeys[key] {
			if key == "BYPASS_DOMAINS" {
				newEnvLines = append(newEnvLines, fmt.Sprintf("%s=%s", key, val))
			} else {
				newEnvLines = append(newEnvLines, fmt.Sprintf(`%s="%s"`, key, val))
			}
		}
	}

	envContent := strings.Join(newEnvLines, "\n") + "\n"
	return os.WriteFile(data.EnvPath, []byte(envContent), 0600)
}

func runSetupWizard() {
    // Keep this for when called directly not from TUI loops
	data := GetSetupData()
	form := BuildSetupForm(data)
	if err := form.Run(); err != nil {
		fmt.Println("Setup canceled:", err)
		return
	}
	if !data.Confirm {
		fmt.Println("Setup aborted by user.")
		return
	}
	if err := SaveSetup(data); err != nil {
		fmt.Printf("Error saving .env file: %v\n", err)
	} else {
		fmt.Println("Successfully saved configuration to .env")
	}
}
