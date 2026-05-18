package util

import (
	"bufio"
	"os"
	"strings"
)

// LoadEnvMap reads an environment file and overrides with os.Environ().
func LoadEnvMap(envPath string) (map[string]string, error) {
	envMap := make(map[string]string)

	// Optional: read .env if it exists
	envFile, err := os.Open(envPath)
	if err == nil {
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
	}

	// Override with actual terminal environment variables
	for _, env := range os.Environ() {
		pair := strings.SplitN(env, "=", 2)
		if len(pair) == 2 {
			envMap[pair[0]] = pair[1]
		}
	}

	return envMap, nil
}
