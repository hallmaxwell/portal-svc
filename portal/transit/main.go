package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
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
	data, err := os.ReadFile("/config.template.json")
	if err != nil {
		panic(err)
	}
	content := string(data)


	for _, env := range os.Environ() {
		pair := strings.SplitN(env, "=", 2)
		if len(pair) != 2 {
			continue
		}
		

		key, val := pair[0], strings.Trim(strings.TrimSpace(pair[1]), `"'`)


		if isRawJSONValue(val) {
			
			content = strings.ReplaceAll(content, `"{`+key+`}"`, val)
		
			content = strings.ReplaceAll(content, `{`+key+`}`, val)
		} else {

			content = strings.ReplaceAll(content, `{`+key+`}`, val)
		}
	}

	outPath := "/tmp/transit.config.run.json"
	os.WriteFile(outPath, []byte(content), 0600)

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