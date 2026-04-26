package util

import (
	"testing"
)

func TestIsRawJSONValue(t *testing.T) {
	tests := []struct {
		val      string
		expected bool
	}{
		{"123", true},
		{"12.3", true},
		{"true", true},
		{"false", true},
		{"[1, 2]", true},
		{"{\"a\": 1}", true},
		{"string", false},
		{"\"string\"", false},
	}

	for _, test := range tests {
		if result := IsRawJSONValue(test.val); result != test.expected {
			t.Errorf("IsRawJSONValue(%q) = %v, expected %v", test.val, result, test.expected)
		}
	}
}
