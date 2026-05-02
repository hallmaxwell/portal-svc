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
			wantStderr: false, // In unified main.go, singBoxLogWriter itself doesn't directly write to os.Stderr unless in writeLog, but let's check
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
			input:      "some unexpected error output",
			isStderr:   true,
			wantLevel:  "error",
			wantStderr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock stderr
			oldStderr := os.Stderr
			r, w, _ := os.Pipe()
			os.Stderr = w

			writer := &singBoxLogWriter{isStderr: tt.isStderr, printToStdout: false}

			// We can't easily capture infoLogger/errorLogger since they are globals,
			// but we can at least test that Write processes without crashing.
			// Ideally we'd reset the globals and check them.

			tempDir := t.TempDir()
			initLogFiles(tempDir)

			_, err := writer.Write([]byte(tt.input + "\n"))
			if err != nil {
				t.Fatalf("Write failed: %v", err)
			}

			w.Close()
			os.Stderr = oldStderr

			var buf bytes.Buffer
			buf.ReadFrom(r)

			// read log files to check level
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

	os.WriteFile(filePath, []byte(strings.Repeat("existing log line\n", 100)), 0666)

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
