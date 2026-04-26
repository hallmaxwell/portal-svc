package main

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestIsRawJSONValue(t *testing.T) {
	tests := []struct {
		name     string
		val      string
		expected bool
	}{
		{"Integer", "123", true},
		{"Negative Integer", "-456", true},
		{"Float", "3.14", true},
		{"Negative Float", "-0.99", true},
		{"Boolean True", "true", true},
		{"Boolean False", "false", true},
		{"Array", "[1, 2, 3]", true},
		{"Object", `{"key": "value"}`, true},
		{"String", "hello", false},
		{"String with quotes", `"hello"`, false},
		{"Empty String", "", false},
		{"Null", "null", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isRawJSONValue(tt.val)
			if result != tt.expected {
				t.Errorf("isRawJSONValue(%q) = %v, want %v", tt.val, result, tt.expected)
			}
		})
	}
}

func TestBoundedLogWriter_Write(t *testing.T) {
	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "test.log")
	writer := &boundedLogWriter{
		filePath: logFile,
		maxLines: 3,
	}

	// Write 1 line
	_, err := writer.Write([]byte("line 1\n"))
	if err != nil {
		t.Fatalf("Failed to write: %v", err)
	}

	// Write 2 more lines
	_, err = writer.Write([]byte("line 2\nline 3\n"))
	if err != nil {
		t.Fatalf("Failed to write: %v", err)
	}

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
	_, err = writer.Write([]byte("line 4\nline 5\n"))
	if err != nil {
		t.Fatalf("Failed to write: %v", err)
	}

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
	writer := &boundedLogWriter{
		filePath: logFile,
		maxLines: 100,
	}

	var wg sync.WaitGroup
	numGoroutines := 10
	writesPerGoroutine := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < writesPerGoroutine; j++ {
				_, _ = writer.Write([]byte("log entry\n"))
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
