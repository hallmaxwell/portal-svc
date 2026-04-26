package template

import (
	"testing"
)

func TestIsRawJSONValue(t *testing.T) {
	tests := []struct {
		val  string
		want bool
	}{
		{"123", true},
		{"12.3", true},
		{"true", true},
		{"false", true},
		{"[1, 2, 3]", true},
		{"{\"key\": \"value\"}", true},
		{"string", false},
		{"'123'", false},
	}

	for _, tt := range tests {
		t.Run(tt.val, func(t *testing.T) {
			if got := IsRawJSONValue(tt.val); got != tt.want {
				t.Errorf("IsRawJSONValue() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSubstitute(t *testing.T) {
	tests := []struct {
		name    string
		content string
		envMap  map[string]string
		want    string
	}{
		{
			name:    "Substitute Raw JSON Number (replace with quotes)",
			content: `{"port": "{PORT}"}`,
			envMap:  map[string]string{"PORT": "8080"},
			want:    `{"port": 8080}`,
		},
		{
			name:    "Substitute Raw JSON Boolean (replace with quotes)",
			content: `{"enabled": "{ENABLED}"}`,
			envMap:  map[string]string{"ENABLED": "true"},
			want:    `{"enabled": true}`,
		},
		{
			name:    "Substitute Non-Raw JSON String",
			content: `{"host": "{HOST}"}`,
			envMap:  map[string]string{"HOST": "example.com"},
			want:    `{"host": "example.com"}`,
		},
		{
			name:    "Substitute Raw JSON array",
			content: `{"ips": "{IPS}"}`,
			envMap:  map[string]string{"IPS": `["1.1.1.1", "8.8.8.8"]`},
			want:    `{"ips": ["1.1.1.1", "8.8.8.8"]}`,
		},
		{
			name:    "Substitute Raw JSON object",
			content: `{"config": "{CONFIG}"}`,
			envMap:  map[string]string{"CONFIG": `{"key": "value"}`},
			want:    `{"config": {"key": "value"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Substitute(tt.content, tt.envMap); got != tt.want {
				t.Errorf("Substitute() = %v, want %v", got, tt.want)
			}
		})
	}
}
