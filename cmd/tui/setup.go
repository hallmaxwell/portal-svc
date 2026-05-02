package tui

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"
)

func runSetupWizard() {
	// Read existing .env file if any
	envMap := make(map[string]string)
	exe, _ := os.Executable()
	baseDir := filepath.Dir(exe)
	// For testing from tree root it might be better to just use current directory or handle gracefully
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

	// Helper to get default
	getDef := func(key, def string) string {
		if val, ok := envMap[key]; ok && val != "" {
			return val
		}
		return def
	}

	var (
		doIP          = getDef("DO_IP", "")
		uuid          = getDef("UUID", "")
		publicKey     = getDef("PUBLIC_KEY", "")
		shortID       = getDef("SHORT_ID", "")
		bypassDomains = getDef("BYPASS_DOMAINS", `["example.com", "google.cn"]`)
		confirm       bool
	)

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Transit Node IP (DO_IP)").
				Value(&doIP),
			huh.NewInput().
				Title("VLESS User UUID (UUID)").
				Value(&uuid),
			huh.NewInput().
				Title("REALITY Public Key (PUBLIC_KEY)").
				Value(&publicKey),
			huh.NewInput().
				Title("REALITY Short ID (SHORT_ID)").
				Value(&shortID),
			huh.NewInput().
				Title("Bypass Domains (BYPASS_DOMAINS)").
				Description("JSON array format").
				Value(&bypassDomains),
		),
		huh.NewGroup(
			huh.NewConfirm().
				Title("Save these parameters to .env?").
				Value(&confirm),
		),
	)

	err := form.Run()
	if err != nil {
		fmt.Println("Setup canceled:", err)
		return
	}

	if !confirm {
		fmt.Println("Setup aborted by user.")
		return
	}

	// Update map with new values
	newVals := map[string]string{
		"DO_IP":          doIP,
		"UUID":           uuid,
		"PUBLIC_KEY":     publicKey,
		"SHORT_ID":       shortID,
		"BYPASS_DOMAINS": bypassDomains,
	}

	// Create new env file content by updating existing lines
	var newEnvLines []string
	updatedKeys := make(map[string]bool)

	for _, line := range envLines {
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
				// Keep other keys as is
				newEnvLines = append(newEnvLines, line)
			}
		} else {
			newEnvLines = append(newEnvLines, line)
		}
	}

	// Append any keys that weren't in the original file
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

	err = os.WriteFile(envPath, []byte(envContent), 0600)
	if err != nil {
		fmt.Printf("Error saving .env file: %v\n", err)
	} else {
		fmt.Println("Successfully saved configuration to .env")
	}
}
