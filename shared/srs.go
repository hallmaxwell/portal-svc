package shared

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"portal-svc/ui"
	"strings"
	"time"
)

// ProcessRuleSets checks the parsed configuration for remote .srs rule sets,
// downloads them to srsDir if possible, and modifies the config to use the local files.
func ProcessRuleSets(configJSON string, srsDir string) (string, error) {
	var config map[string]interface{}
	if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
		return "", ui.NewAppError("CONFIG_PARSE_ERR", "Failed to parse config JSON", err.Error(), ui.SeverityError, err)
	}

	routeVal, ok := config["route"]
	if !ok {
		return configJSON, nil // No route block
	}
	routeMap, ok := routeVal.(map[string]interface{})
	if !ok {
		return configJSON, nil
	}

	ruleSetsVal, ok := routeMap["rule_set"]
	if !ok {
		return configJSON, nil // No rule_set block
	}

	ruleSets, ok := ruleSetsVal.([]interface{})
	if !ok {
		return configJSON, nil
	}

	if err := os.MkdirAll(srsDir, 0755); err != nil {
		return "", ui.NewAppError("DIR_CREATE_ERR", fmt.Sprintf("Failed to create srs directory %s", srsDir), err.Error(), ui.SeverityError, err)
	}

	modified := false
	for i, rs := range ruleSets {
		rsMap, ok := rs.(map[string]interface{})
		if !ok {
			continue
		}

		rsType, _ := rsMap["type"].(string)
		rsFormat, _ := rsMap["format"].(string)
		rsURL, _ := rsMap["url"].(string)
		rsTag, _ := rsMap["tag"].(string)

		if rsType == "remote" && rsFormat == "binary" && strings.HasSuffix(rsURL, ".srs") && rsTag != "" {
			localFileName := rsTag + ".srs"
			localFilePath := filepath.Join(srsDir, localFileName)

			// Attempt to download the file
			downloadErr := DownloadFile(rsURL, localFilePath)
			if downloadErr != nil {
				msg := fmt.Sprintf("Warning: failed to download rule set %s from %s: %v", rsTag, rsURL, downloadErr)
				SysLogError(msg, true)
			} else {
				msg := fmt.Sprintf("Successfully downloaded rule set %s to %s", rsTag, localFilePath)
				SysLogInfo(msg, true)
			}

			// Check if local file exists (either just downloaded or cached)
			if _, err := os.Stat(localFilePath); err == nil {
				// Local file exists, rewrite the rule set configuration
				rsMap["type"] = "local"
				rsMap["path"] = localFilePath
				delete(rsMap, "url")
				delete(rsMap, "download_detour")
				ruleSets[i] = rsMap
				modified = true
			} else {
				msg := fmt.Sprintf("Warning: no local cache found for rule set %s, sing-box might fail if network is unreachable.", rsTag)
				SysLogError(msg, true)
			}
		}
	}

	if !modified {
		return configJSON, nil
	}

	// Re-marshal the modified configuration
	newConfigBytes, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return "", ui.NewAppError("CONFIG_MARSHAL_ERR", "Failed to re-marshal modified config", err.Error(), ui.SeverityError, err)
	}

	return string(newConfigBytes), nil
}

func DownloadFile(url string, dest string) error {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ui.NewAppError("HTTP_ERR", fmt.Sprintf("Failed to download SRS file (bad status: %s)", resp.Status), "", ui.SeverityError, nil)
	}

	tmpDest := dest + ".tmp"
	out, err := os.Create(tmpDest)
	if err != nil {
		return err
	}

	_, err = io.Copy(out, resp.Body)
	out.Close()
	if err != nil {
		os.Remove(tmpDest)
		return err
	}

	// Rename temp file to actual file
	return os.Rename(tmpDest, dest)
}
