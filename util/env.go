package util

import (
	"bufio"
	"os"
	"strings"
)

// LoadEnvMap reads a file and parses key=value pairs into a map.
func LoadEnvMap(envPath string) (map[string]string, error) {
	envMap := make(map[string]string)
	envFile, err := os.Open(envPath)
	if err != nil {
		return nil, err
	}
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
	return envMap, nil
}
