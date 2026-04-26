package template

import (
	"strconv"
	"strings"
)

// IsRawJSONValue checks whether the value should be treated as a raw JSON value.
func IsRawJSONValue(val string) bool {
	if _, err := strconv.Atoi(val); err == nil {
		return true
	}

	if _, err := strconv.ParseFloat(val, 64); err == nil {
		return true
	}

	if val == "true" || val == "false" {
		return true
	}

	if strings.HasPrefix(val, "[") && strings.HasSuffix(val, "]") {
		return true
	}

	if strings.HasPrefix(val, "{") && strings.HasSuffix(val, "}") {
		return true
	}

	return false
}

// Substitute takes a template string and replaces placeholders with environment variables.
func Substitute(content string, envMap map[string]string) string {
	for key, val := range envMap {
		if IsRawJSONValue(val) {
			content = strings.ReplaceAll(content, `"{`+key+`}"`, val)
			content = strings.ReplaceAll(content, `{`+key+`}`, val)
		} else {
			content = strings.ReplaceAll(content, `{`+key+`}`, val)
		}
	}
	return content
}
