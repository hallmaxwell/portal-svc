package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"hawego/portal/internal/template"
)

func main() {
	data, err := os.ReadFile("/config.template.json")
	if err != nil {
		panic(err)
	}
	content := string(data)

	envMap := make(map[string]string)
	for _, env := range os.Environ() {
		pair := strings.SplitN(env, "=", 2)
		if len(pair) != 2 {
			continue
		}
		key, val := pair[0], strings.Trim(strings.TrimSpace(pair[1]), `"'`)
		envMap[key] = val
	}

	content = template.Substitute(content, envMap)

	outPath := "/tmp/transit.config.run.json"
	os.WriteFile(outPath,[]byte(content), 0644)

	cmd := exec.Command("sing-box", "run", "-c", outPath)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr

	fmt.Println("Transit Node Launching...")
	
	if err := cmd.Start(); err != nil {
		fmt.Println("Launch failed: ", err)
		return
	}
	
	go func() {
		time.Sleep(2 * time.Second)
		os.Remove(outPath)
		fmt.Println("transit.config.run.json cleared, transit node is running.")
	}()
	
	cmd.Wait() 
}