package util

import (
	"encoding/json"
	"os"
	"path/filepath"
	"portal-svc/ui"
	"strings"

	"portal-svc/templates"
)

// RenderConfigTemplate reads a template file, replaces placeholders with values
// from the provided env map, and returns the rendered string.
func RenderConfigTemplate(templatePath string, envMap map[string]string) (string, error) {
	tempData, err := os.ReadFile(templatePath)
	if err != nil {
		// Fallback to embedded templates
		baseName := filepath.Base(templatePath)
		tempData, err = templates.FS.ReadFile(baseName)
		if err != nil {
			return "", ui.NewAppError("TMPL_READ_ERR", "Failed to read config template", err.Error(), ui.SeverityError, err)
		}
	}

	content := string(tempData)
	for key, val := range envMap {
		// In case val has quotes
		val = strings.Trim(val, `"'`)

		if IsRawJSONValue(val) {
			content = strings.ReplaceAll(content, `"{`+key+`}"`, val)
			content = strings.ReplaceAll(content, `{`+key+`}`, val)
		} else {
			content = strings.ReplaceAll(content, `{`+key+`}`, val)
		}
	}

	return content, nil
}

// InjectCIRules parses a JSON configuration and injects testing overrides
// similar to what was done in render_config.py:
// 1. Inbounds of type 'tun' have auto_route=false and strict_route=false
// 2. Adds a 'ci-direct-out' outbound
// 3. For route.rule_set, sets download_detour to 'ci-direct-out'
func InjectCIRules(jsonContent string) (string, error) {
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(jsonContent), &data); err != nil {
		return "", ui.NewAppError("JSON_PARSE_ERR", "Failed to parse JSON", err.Error(), ui.SeverityError, err)
	}

	// 1. tun auto_route=false, strict_route=false
	if inboundsRaw, ok := data["inbounds"]; ok {
		if inbounds, isArray := inboundsRaw.([]interface{}); isArray {
			for _, inboundRaw := range inbounds {
				if inbound, isMap := inboundRaw.(map[string]interface{}); isMap {
					if typeVal, ok := inbound["type"]; ok && typeVal == "tun" {
						inbound["auto_route"] = false
						inbound["strict_route"] = false
					}
				}
			}
		}
	}

	// 2. Add 'ci-direct-out' outbound
	outboundsRaw, ok := data["outbounds"]
	var outbounds []interface{}
	if ok {
		if obs, isArray := outboundsRaw.([]interface{}); isArray {
			outbounds = obs
		}
	}
	outbounds = append(outbounds, map[string]interface{}{
		"type": "direct",
		"tag":  "ci-direct-out",
	})
	data["outbounds"] = outbounds

	// 3. route.rule_set download_detour = 'ci-direct-out'
	if routeRaw, ok := data["route"]; ok {
		if routeMap, isMap := routeRaw.(map[string]interface{}); isMap {
			if ruleSetRaw, ok := routeMap["rule_set"]; ok {
				if ruleSets, isArray := ruleSetRaw.([]interface{}); isArray {
					for _, ruleSetRaw := range ruleSets {
						if ruleSet, isMap := ruleSetRaw.(map[string]interface{}); isMap {
							ruleSet["download_detour"] = "ci-direct-out"
						}
					}
				}
			}
		}
	}

	outBytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "", ui.NewAppError("JSON_MARSHAL_ERR", "Failed to marshal injected JSON", err.Error(), ui.SeverityError, err)
	}
	return string(outBytes), nil
}
