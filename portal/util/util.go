package util

import (
	"strconv"
	"strings"
)

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
