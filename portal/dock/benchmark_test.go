package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func BenchmarkBoundedLogWriter(b *testing.B) {
	tmpDir := b.TempDir()
	filePath := filepath.Join(tmpDir, "test.log")

	// Pre-fill the log file to test the append behavior
	os.WriteFile(filePath, []byte(strings.Repeat("existing log line\n", 100)), 0666)

	logger := &boundedLogger{filePath: filePath, maxLines: 100}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		appendToLog(logger, []string{"test log line"})
	}
}
