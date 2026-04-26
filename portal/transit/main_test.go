package main

import "testing"

func TestIsRawJSONValue(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"integer", "42", true},
		{"negative integer", "-42", true},
		{"float", "3.14", true},
		{"negative float", "-3.14", true},
		{"boolean true", "true", true},
		{"boolean false", "false", true},
		{"array", `["a", "b"]`, true},
		{"object", `{"a": 1}`, true},
		{"string", `"hello"`, false},
		{"plain string", "hello", false},
		{"empty string", "", false},
		{"array without end", `["a", "b"`, false},
		{"object without end", `{"a": 1`, false},
		{"leading bracket but not array", `[hello`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isRawJSONValue(tt.input); got != tt.expected {
				t.Errorf("isRawJSONValue(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}
