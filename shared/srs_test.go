package shared

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProcessRuleSets(t *testing.T) {
	// Simple config with one remote rule set
	configJSONFail := `{"route": {"rule_set": [{"tag": "geosite-test", "type": "remote", "format": "binary", "url": "http://invalid.local/test.srs"}]}}`

	tmpDir := t.TempDir()
	srsDir := filepath.Join(tmpDir, "srs")

	os.MkdirAll(srsDir, 0755)
	testFile := filepath.Join(srsDir, "geosite-test.srs")
	os.WriteFile(testFile, []byte("dummy data"), 0644)

	newConfig, err := ProcessRuleSets(configJSONFail, srsDir, false)
	if err != nil {
		t.Fatalf("ProcessRuleSets failed: %v", err)
	}

	if !strings.Contains(newConfig, `"type": "local"`) {
		t.Errorf("Expected config to be rewritten to type local")
	}

	testFileEscaped := strings.ReplaceAll(testFile, "\\", "\\\\")

	if !strings.Contains(newConfig, testFileEscaped) {
		t.Errorf("Expected config to contain path to local file, got: %s", newConfig)
	}
}
