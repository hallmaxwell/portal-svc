package tweak

import (
	"encoding/json"
	"fmt"
	"os"
	"portal-svc/ui"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	OverrideFileName = "user_override.json"
)

// LoadOverrides reads the user_override.json file and returns a map of JSON paths to values.
func LoadOverrides(filePath string) (map[string]interface{}, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]interface{}), nil
		}
		return nil, ui.NewAppError("OVERRIDE_READ_ERR", "Failed to read override file", err.Error(), ui.SeverityError, err)
	}

	var overrides map[string]interface{}
	if err := json.Unmarshal(data, &overrides); err != nil {
		return nil, ui.NewAppError("OVERRIDE_PARSE_ERR", "Failed to parse override file", err.Error(), ui.SeverityError, err)
	}

	return overrides, nil
}

// SaveOverrides writes the map of overrides back to user_override.json.
func SaveOverrides(filePath string, overrides map[string]interface{}) error {
	data, err := json.MarshalIndent(overrides, "", "  ")
	if err != nil {
		return ui.NewAppError("OVERRIDE_ENCODE_ERR", "Failed to encode overrides", err.Error(), ui.SeverityError, err)
	}

	if err := os.WriteFile(filePath, data, 0600); err != nil {
		return ui.NewAppError("OVERRIDE_WRITE_ERR", "Failed to write override file", err.Error(), ui.SeverityError, err)
	}

	return nil
}

// ApplyOverrides takes a base JSON string and applies the overrides map to it using sjson.
func ApplyOverrides(baseJSON string, overrides map[string]interface{}) (string, error) {
	result := baseJSON
	var err error

	for path, value := range overrides {
		result, err = sjson.Set(result, path, value)
		if err != nil {
			return baseJSON, ui.NewAppError("OVERRIDE_SET_ERR", fmt.Sprintf("Failed to set path '%s'", path), err.Error(), ui.SeverityError, err)
		}
	}

	return result, nil
}

// GetValue queries a JSON string for a specific path using gjson.
func GetValue(jsonStr string, path string) gjson.Result {
	return gjson.Get(jsonStr, path)
}
