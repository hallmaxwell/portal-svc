package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func TestSingBoxLogWriter(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		isStderr      bool
		printToStdout bool
		wantLevel     string
		wantStdoutStr string
		wantStderrStr string
	}{
		{
			name:          "Normal info log",
			input:         "INFO[0000] sing-box started",
			isStderr:      false,
			printToStdout: true,
			wantLevel:     "info",
			wantStdoutStr: "sing-box: INFO[0000] sing-box started\n",
			wantStderrStr: "",
		},
		{
			name:          "Error keyword in log",
			input:         "ERROR[0001] connection failed",
			isStderr:      false,
			printToStdout: true,
			wantLevel:     "error",
			wantStdoutStr: "",
			wantStderrStr: "sing-box: ERROR[0001] connection failed\n",
		},
		{
			name:          "Fatal keyword in log",
			input:         "FATAL[0002] config invalid",
			isStderr:      false,
			printToStdout: true,
			wantLevel:     "error",
			wantStdoutStr: "",
			wantStderrStr: "sing-box: FATAL[0002] config invalid\n",
		},
		{
			name:          "Stderr stream ignores source stream when no keyword",
			input:         "some unexpected output without keywords",
			isStderr:      true,
			printToStdout: true,
			wantLevel:     "info", // memory override: strictly ignores the stream source when there is no keyword in the log
			wantStdoutStr: "sing-box: some unexpected output without keywords\n",
			wantStderrStr: "",
		},
		{
			name:          "Stderr stream with error keyword",
			input:         "some unexpected error output",
			isStderr:      true,
			printToStdout: true,
			wantLevel:     "error",
			wantStdoutStr: "",
			wantStderrStr: "sing-box: some unexpected error output\n",
		},
		{
			name:          "Multiple lines",
			input:         "INFO[0000] line 1\nERROR[0001] line 2\n\nINFO[0002] line 3",
			isStderr:      false,
			printToStdout: true,
			wantLevel:     "mixed",
			wantStdoutStr: "sing-box: INFO[0000] line 1\nsing-box: INFO[0002] line 3\n",
			wantStderrStr: "sing-box: ERROR[0001] line 2\n",
		},
		{
			name:          "Only empty lines",
			input:         "\n\n  \n",
			isStderr:      false,
			printToStdout: true,
			wantLevel:     "none",
			wantStdoutStr: "",
			wantStderrStr: "",
		},
		{
			name:          "printToStdout is false",
			input:         "INFO[0000] hidden log",
			isStderr:      false,
			printToStdout: false,
			wantLevel:     "info",
			wantStdoutStr: "",
			wantStderrStr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldStdout := os.Stdout
			oldStderr := os.Stderr

			rOut, wOut, _ := os.Pipe()
			rErr, wErr, _ := os.Pipe()

			os.Stdout = wOut
			os.Stderr = wErr

			// Restore global streams on exit to prevent side-effects on other tests
			defer func() {
				os.Stdout = oldStdout
				os.Stderr = oldStderr
				wOut.Close()
				wErr.Close()
			}()

			writer := &singBoxLogWriter{isStderr: tt.isStderr, printToStdout: tt.printToStdout}

			tempDir := t.TempDir()
			initLogFiles(tempDir)

			n, err := writer.Write([]byte(tt.input))
			if err != nil {
				t.Fatalf("Write failed: %v", err)
			}

			if n != len([]byte(tt.input)) {
				t.Errorf("Expected Write to return %d, got %d", len([]byte(tt.input)), n)
			}

			wOut.Close()
			wErr.Close()
			os.Stdout = oldStdout
			os.Stderr = oldStderr

			var bufOut bytes.Buffer
			bufOut.ReadFrom(rOut)

			var bufErr bytes.Buffer
			bufErr.ReadFrom(rErr)

			if bufOut.String() != tt.wantStdoutStr {
				t.Errorf("Expected stdout %q, got %q", tt.wantStdoutStr, bufOut.String())
			}

			if bufErr.String() != tt.wantStderrStr {
				t.Errorf("Expected stderr %q, got %q", tt.wantStderrStr, bufErr.String())
			}

			dataInfo, _ := os.ReadFile(infoLogFilePath)
			dataError, _ := os.ReadFile(errorLogFilePath)

			if tt.wantLevel == "info" {
				if !strings.Contains(string(dataInfo), "sing-box: "+tt.input) {
					t.Errorf("Expected info log to contain %q, got %q", tt.input, string(dataInfo))
				}
				if strings.Contains(string(dataError), "sing-box: "+tt.input) {
					t.Errorf("Did not expect error log to contain %q", tt.input)
				}
			} else if tt.wantLevel == "error" {
				if !strings.Contains(string(dataInfo), "sing-box: "+tt.input) {
					t.Errorf("Expected info log to contain %q, got %q", tt.input, string(dataInfo))
				}
				if !strings.Contains(string(dataError), "sing-box: "+tt.input) {
					t.Errorf("Expected error log to contain %q, got %q", tt.input, string(dataError))
				}
			} else if tt.wantLevel == "mixed" {
				if !strings.Contains(string(dataInfo), "sing-box: INFO[0000] line 1") ||
					!strings.Contains(string(dataInfo), "sing-box: ERROR[0001] line 2") ||
					!strings.Contains(string(dataInfo), "sing-box: INFO[0002] line 3") {
					t.Errorf("Missing lines in info log: %q", string(dataInfo))
				}
				if !strings.Contains(string(dataError), "sing-box: ERROR[0001] line 2") {
					t.Errorf("Expected error log to contain line 2: %q", string(dataError))
				}
			} else if tt.wantLevel == "none" {
				if len(strings.TrimSpace(string(dataInfo))) > 0 {
					t.Errorf("Expected empty info log, got %q", string(dataInfo))
				}
				if len(strings.TrimSpace(string(dataError))) > 0 {
					t.Errorf("Expected empty error log, got %q", string(dataError))
				}
			}
		})
	}
}

func TestDockProgram_Cleanup(t *testing.T) {
	// Test file deletion
	tempFile, err := os.CreateTemp("", "dock_test_cleanup_*.json")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	outPath := tempFile.Name()
	tempFile.Close() // Close immediately as per best practices

	p := &dockProgram{outPath: outPath}
	p.cleanup()

	// Verify file is deleted
	if _, err := os.Stat(outPath); !os.IsNotExist(err) {
		t.Errorf("Expected file %s to be deleted, but it still exists or error: %v", outPath, err)
	}

	// Test process killing
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("timeout", "/T", "10")
	} else {
		cmd = exec.Command("sleep", "10")
	}

	err = cmd.Start()
	if err != nil {
		t.Fatalf("Failed to start dummy process: %v", err)
	}

	p2 := &dockProgram{cmd: cmd}
	p2.cleanup()

	// Wait for process to exit and verify it was killed
	err = cmd.Wait()
	if err == nil {
		t.Errorf("Expected process to be killed, but it exited normally")
	}
}

func BenchmarkBoundedLogWriter(b *testing.B) {
	tmpDir := b.TempDir()
	filePath := filepath.Join(tmpDir, "test.log")

	os.WriteFile(filePath, []byte(strings.Repeat("existing log line\n", 100)), 0600)

	logger := &boundedLogger{filePath: filePath, maxLines: 100}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		appendToLog(logger, []string{"test log line"})
	}
}

func TestBoundedLogWriter_Write(t *testing.T) {
	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "test.log")
	logger := &boundedLogger{filePath: logFile, maxLines: 3}

	// Write 1 line
	appendToLog(logger, []string{"line 1"})

	// Write 2 more lines
	appendToLog(logger, []string{"line 2", "line 3"})

	// Verify we have 3 lines
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}
	content := strings.TrimSpace(string(data))
	lines := strings.Split(content, "\n")
	if len(lines) != 3 {
		t.Errorf("Expected 3 lines, got %d: %q", len(lines), lines)
	}
	if lines[0] != "line 1" || lines[2] != "line 3" {
		t.Errorf("Unexpected content: %v", lines)
	}

	// Write 2 more lines, should push out the first two
	appendToLog(logger, []string{"line 4", "line 5"})

	data, err = os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}
	content = strings.TrimSpace(string(data))
	lines = strings.Split(content, "\n")
	if len(lines) != 3 {
		t.Errorf("Expected 3 lines, got %d: %q", len(lines), lines)
	}
	if lines[0] != "line 3" || lines[1] != "line 4" || lines[2] != "line 5" {
		t.Errorf("Unexpected content: %v", lines)
	}
}

func TestWriteLog(t *testing.T) {
	tempDir := t.TempDir()
	initLogFiles(tempDir)

	// Test info routing
	writeLog("info", "prefix_info", "info message", false)

	dataInfo, _ := os.ReadFile(infoLogFilePath)
	dataError, _ := os.ReadFile(errorLogFilePath)

	if !strings.Contains(string(dataInfo), "prefix_info info message") {
		t.Errorf("Expected info log to contain info message")
	}
	if strings.Contains(string(dataError), "prefix_info info message") {
		t.Errorf("Did not expect error log to contain info message")
	}

	// Test error routing
	writeLog("error", "prefix_error", "error message", false)

	dataInfo, _ = os.ReadFile(infoLogFilePath)
	dataError, _ = os.ReadFile(errorLogFilePath)

	if !strings.Contains(string(dataInfo), "prefix_error error message") {
		t.Errorf("Expected info log to contain error message")
	}
	if !strings.Contains(string(dataError), "prefix_error error message") {
		t.Errorf("Expected error log to contain error message")
	}
}

func TestBoundedLogWriter_Concurrency(t *testing.T) {
	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "test_concurrent.log")
	logger := &boundedLogger{filePath: logFile, maxLines: 100}

	var wg sync.WaitGroup
	numGoroutines := 10
	writesPerGoroutine := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < writesPerGoroutine; j++ {
				appendToLog(logger, []string{"log entry"})
			}
		}(i)
	}

	wg.Wait()

	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}
	content := strings.TrimSpace(string(data))
	lines := strings.Split(content, "\n")

	// Total writes is 100, maxLines is 100.
	if len(lines) != 100 {
		t.Errorf("Expected 100 lines, got %d", len(lines))
	}
}

func TestDockProgram_Cleanup_NoCmd(t *testing.T) {
	// Create a temporary file to act as the outPath
	tempFile, err := os.CreateTemp(t.TempDir(), "test_cleanup_*.json")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	outPath := tempFile.Name()
	tempFile.Close() // Close it so it can be deleted

	// Verify the file exists initially
	if _, err := os.Stat(outPath); os.IsNotExist(err) {
		t.Fatalf("Expected temp file to exist, but it doesn't: %s", outPath)
	}

	// Create dockProgram instance with the temp file as outPath
	p := &dockProgram{
		outPath: outPath,
	}

	// Case 1: p.cmd is nil. cleanup() should handle it without panicking
	// and should remove the file.
	p.cleanup()

	// Verify the file was deleted
	if _, err := os.Stat(outPath); !os.IsNotExist(err) {
		t.Errorf("Expected temp file to be deleted by cleanup(), but it still exists: %s", outPath)
	}

}

func TestDockProgram_Cleanup_ProcessNotNil(t *testing.T) {
	// Case 2: p.cmd is not nil, but p.cmd.Process is nil
	p2 := &dockProgram{
		cmd: &exec.Cmd{},
	}
	// This should not panic
	p2.cleanup()
}
