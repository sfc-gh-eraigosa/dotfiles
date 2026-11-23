package approval

import (
	"os"
	"os/exec"
	"strings"
)

// FlagKey is the gff flag that turns the approval token on. It is off by
// default (operator, 2026-11-23: the handshake added a round trip per PR
// and did not hold up in practice); `gff set gss.approval-token.enabled true`
// turns it back on. The safety_guard hook reads the same flag.
const FlagKey = "gss.approval-token.enabled"

// EnvOverride, when set to on/true/1 or off/false/0, decides without asking
// gff (tests, and a one-off `GSS_REQUIRE_APPROVAL=on gss ...`).
const EnvOverride = "GSS_REQUIRE_APPROVAL"

// lookPath and runGFF are swapped in tests.
var (
	lookPath = exec.LookPath
	runGFF   = func(args ...string) error { return exec.Command("gff", args...).Run() }
)

// TokenRequired reports whether gated verbs need an approval token: the
// env override if set, else `gff enabled gss.approval-token.enabled` (exit
// 0 = on). No gff, an unknown flag, or any other answer means off, the
// default.
func TokenRequired() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(EnvOverride))) {
	case "on", "true", "1":
		return true
	case "off", "false", "0":
		return false
	}
	if _, err := lookPath("gff"); err != nil {
		return false
	}
	return runGFF("enabled", FlagKey) == nil
}
