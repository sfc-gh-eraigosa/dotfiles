package approval_test

import (
	"os"
	"testing"
)

// The handshake tests exercise the token itself, so they run with it on
// whatever this host's gff flag says.
func TestMain(m *testing.M) {
	_ = os.Setenv("GSS_REQUIRE_APPROVAL", "on")
	os.Exit(m.Run())
}
