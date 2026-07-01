package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSingBoxLogWriter(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantLevel  string
		wantStderr bool
	}{
		{
			name:       "Normal info log",
			input:      "INFO[0000] sing-box started",
			wantLevel:  "info",
			wantStderr: false,
		},
		{
			name:       "Error keyword in log",
			input:      "ERROR[0001] connection failed",
			wantLevel:  "error",
			wantStderr: false,
		},
		{
			name:       "Warning keyword in log",
			input:      "WARNING[0001] connection failed",
			wantLevel:  "info",
			wantStderr: false,
		},
		{
			name:       "Fatal keyword in log",
			input:      "FATAL[0002] config invalid",
			wantLevel:  "error",
			wantStderr: false,
		},
		{
			name:       "Stderr stream",
			input:      "some unexpected output",
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

			writer := &singBoxLogWriter{printToStdout: false}

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

func TestValidateDockServiceCommands(t *testing.T) {
	if err := validateDockServiceCommands([]string{"install", "start", "restart", "stop", "uninstall"}); err != nil {
		t.Fatalf("expected valid service commands, got %v", err)
	}

	err := validateDockServiceCommands([]string{"start", "bogus"})
	if err == nil {
		t.Fatal("expected invalid service command error")
	}
	if !strings.Contains(err.Error(), "bogus") {
		t.Fatalf("expected error to mention invalid command, got %v", err)
	}
}

func TestSplitDockArgs(t *testing.T) {
	flagArgs, svcArgs := splitDockArgs([]string{"--config", "custom.json", "install", "start"})

	if strings.Join(flagArgs, " ") != "--config custom.json" {
		t.Fatalf("flagArgs = %#v", flagArgs)
	}
	if strings.Join(svcArgs, " ") != "install start" {
		t.Fatalf("svcArgs = %#v", svcArgs)
	}
}

func TestRecentNonBlankLines(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "error.log")
	content := "first\n\nsecond\nthird\nfourth\n"
	if err := os.WriteFile(logPath, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write test log: %v", err)
	}

	got, err := recentNonBlankLines(logPath, 2)
	if err != nil {
		t.Fatalf("recentNonBlankLines failed: %v", err)
	}

	want := "third\nfourth\n"
	if got != want {
		t.Fatalf("recentNonBlankLines = %q, want %q", got, want)
	}
}
