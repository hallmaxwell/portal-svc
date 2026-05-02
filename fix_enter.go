package main

import (
	"fmt"
	"strings"
	"io/ioutil"
)

func main() {
	content, err := ioutil.ReadFile("cmd/tui/tui.go")
	if err != nil {
		fmt.Println("Error reading file:", err)
		return
	}
	// Let's strip any actual carriage returns that might be embedded inside the file itself just in case
	newContent := strings.ReplaceAll(string(content), "\r", "")
	ioutil.WriteFile("cmd/tui/tui.go", []byte(newContent), 0644)
}
