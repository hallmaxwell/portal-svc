package main

import (
	"os"
	"os/exec"
	"strings"
	"strconv"
)

func main() {
	data, err := os.ReadFile("/config.json")
	if err != nil { panic(err) }
	content := string(data)

	for _, env := range os.Environ() {
		pair := strings.SplitN(env, "=", 2)
		if len(pair) != 2 { continue }
		key, val := pair[0], strings.Trim(strings.TrimSpace(pair[1]), `"'`)

		if _, err := strconv.Atoi(val); err == nil {
			content = strings.ReplaceAll(content, `"{`+key+`}"`, val)
		} else {
			content = strings.ReplaceAll(content, `{`+key+`}`, val)
		}
	}

	os.WriteFile("/tmp/config_run.json", []byte(content), 0644)
	cmd := exec.Command("sing-box", "run", "-c", "/tmp/config_run.json")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	cmd.Run()
}