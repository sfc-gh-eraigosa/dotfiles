package cmd

import (
	"strings"
	"testing"
)

func TestMACCleaning(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"b4:2e:99:aa:79:8b", "b42e99aa798b"},
		{"B4-2E-99-AA-79-8B", "B42E99AA798B"},
		{"b42e.99aa.798b", "b42e99aa798b"},
		{"b42e99aa798b", "b42e99aa798b"},
	}

	for _, tt := range tests {
		clean := strings.ReplaceAll(tt.input, ":", "")
		clean = strings.ReplaceAll(clean, "-", "")
		clean = strings.ReplaceAll(clean, ".", "")
		if clean != tt.expected {
			t.Errorf("For input %s, expected %s, got %s", tt.input, tt.expected, clean)
		}
	}
}
