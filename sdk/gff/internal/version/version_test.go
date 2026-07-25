package version

import (
	"strings"
	"testing"
)

func TestString(t *testing.T) {
	result := String()

	// Verify it contains all four fields and the description
	tests := []string{
		"v" + Version,
		"Commit:",
		Commit,
		"Dirty:",
		Dirty,
		"Build Date:",
		BuildDate,
		"gff — git fast features",
	}

	for _, want := range tests {
		if !strings.Contains(result, want) {
			t.Errorf("String() missing %q, got:\n%s", want, result)
		}
	}

	// Check it has the expected structure (ends with description)
	if !strings.Contains(result, "Description: gff —") {
		t.Errorf("String() missing description block, got:\n%s", result)
	}
}
