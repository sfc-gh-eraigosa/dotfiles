package gh_test

import (
	"testing"

	"github.com/wenlock/dotfiles/gsl/internal/gh"
)

// TestNewSystemRunnerImplementsRunner is a compile-time check that
// *SystemRunner satisfies the Runner interface.
func TestNewSystemRunnerImplementsRunner(t *testing.T) {
	var _ gh.Runner = gh.NewSystemRunner()
}
