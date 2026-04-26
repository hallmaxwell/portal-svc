package main

import "testing"

func TestIsRawJSONValue(t *testing.T) {
	tests := []struct {
		name     string
		val      string
		expected bool
	}{
		{"integer", "123", true},
		{"negative integer", "-456", true},
		{"float", "3.14", true},
		{"negative float", "-0.99", true},
		{"boolean true", "true", true},
		{"boolean false", "false", true},
		{"array empty", "[]", true},
		{"array with elements", "[1, 2, 3]", true},
		{"object empty", "{}", true},
		{"object with elements", `{"key": "value"}`, true},
		{"string", "hello", false},
		{"quoted string", `"world"`, false},
		{"partial array start", "[1, 2", false},
		{"partial array end", "1, 2]", false},
		{"partial object start", "{key: value", false},
		{"partial object end", "key: value}", false},
		{"empty string", "", false},
		{"boolean capitalized", "True", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isRawJSONValue(tt.val); got != tt.expected {
				t.Errorf("isRawJSONValue(%q) = %v, want %v", tt.val, got, tt.expected)
			}
		})
	}
}
