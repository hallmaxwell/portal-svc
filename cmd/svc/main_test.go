package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"
)

func TestSingBoxLogWriter(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		isStderr   bool
		wantLevel  string
		wantStderr bool
	}{
		{
			name:       "Normal info log",
			input:      "INFO[0000] sing-box started",
			isStderr:   false,
			wantLevel:  "info",
			wantStderr: false,
		},
		{
			name:       "Error keyword in log",
			input:      "ERROR[0001] connection failed",
			isStderr:   false,
			wantLevel:  "error",
			wantStderr: false,
		},
		{
			name:       "Warning keyword in log",
			input:      "WARNING[0001] connection failed",
			isStderr:   false,
			wantLevel:  "info",
			wantStderr: false,
		},
		{
			name:       "Fatal keyword in log",
			input:      "FATAL[0002] config invalid",
			isStderr:   false,
			wantLevel:  "error",
			wantStderr: false,
		},
		{
			name:       "Stderr stream",
			input:      "some unexpected output",
			isStderr:   true,
			wantLevel:  "info",
			wantStderr: false,
		},
	}

	// Change directory to a temp dir so logs don't pollute the project
	tempDir := t.TempDir()
	oldWd, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(oldWd)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock stderr
			oldStderr := os.Stderr
			r, w, _ := os.Pipe()
			os.Stderr = w

			writer := &singBoxLogWriter{isStderr: tt.isStderr, printToStdout: false}

			initLogFiles()

			_, err := writer.Write([]byte(tt.input + "\n"))
			if err != nil {
				t.Fatalf("Write failed: %v", err)
			}

			// Allow time for file sync since it's an async potential or disk IO
			time.Sleep(100 * time.Millisecond)

			w.Close()
			os.Stderr = oldStderr

			var buf bytes.Buffer
			buf.ReadFrom(r)

			dataInfo, _ := os.ReadFile(infoLogFilePath)
			dataError, _ := os.ReadFile(errorLogFilePath)

			if !strings.Contains(string(dataInfo), "sing-box: "+tt.input) {
				t.Errorf("Expected info log to contain %q, got %q", tt.input, string(dataInfo))
			}

			if tt.wantLevel == "error" {
				if !strings.Contains(string(dataError), "sing-box: "+tt.input) {
					t.Errorf("Expected error log to contain %q, got %q", tt.input, string(dataError))
				}
			} else {
				if strings.Contains(string(dataError), "sing-box: "+tt.input) {
					t.Errorf("Did not expect error log to contain %q", tt.input)
				}
			}
		})
	}
}

func TestWriteLog(t *testing.T) {
	tempDir := t.TempDir()
	oldWd, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(oldWd)

	initLogFiles()

	writeLog("info", "test:", "this is an info message", false)
	writeLog("error", "test:", "this is an error message", false)

	time.Sleep(100 * time.Millisecond)

	dataInfo, _ := os.ReadFile(infoLogFilePath)
	dataError, _ := os.ReadFile(errorLogFilePath)

	if !strings.Contains(string(dataInfo), "this is an info message") {
		t.Errorf("Expected info log to contain 'this is an info message'")
	}
	if !strings.Contains(string(dataInfo), "this is an error message") {
		t.Errorf("Expected info log to contain 'this is an error message'")
	}

	if strings.Contains(string(dataError), "this is an info message") {
		t.Errorf("Did not expect error log to contain 'this is an info message'")
	}
	if !strings.Contains(string(dataError), "this is an error message") {
		t.Errorf("Expected error log to contain 'this is an error message'")
	}
}
