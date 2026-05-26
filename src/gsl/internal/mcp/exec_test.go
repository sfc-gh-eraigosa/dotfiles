package mcp_test

import (
	"testing"

	"github.com/wenlock/dotfiles/gsl/internal/mcp"
)

// TestNewSystemRunnerImplementsRunner is a compile-time check that
// *SystemRunner satisfies the Runner interface.
func TestNewSystemRunnerImplementsRunner(t *testing.T) {
	var _ mcp.Runner = mcp.NewSystemRunner()
}
