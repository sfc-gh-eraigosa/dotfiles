package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/mcp"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/mcp/fake"
)

// withMCP points the mcp verbs at a temp cache, an empty HOME and r.
func withMCP(t *testing.T, r mcp.Runner) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Chdir(t.TempDir())
	oldR, oldO := mcpRunner, mcpOptions
	mcpRunner, mcpOptions = r, mcp.Options{CacheDir: t.TempDir()}
	t.Cleanup(func() { mcpRunner, mcpOptions = oldR, oldO })
}

func runMCP(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs(append([]string{"mcp"}, args...))
	t.Cleanup(func() { rootCmd.SetArgs(nil); rootCmd.SetOut(nil); rootCmd.SetErr(nil); mcpStatusJSON = false })
	err := rootCmd.ExecuteContext(context.Background())
	return out.String(), err
}

func TestMCPCmd_StatusCold(t *testing.T) {
	withMCP(t, &fake.Runner{})
	out, err := runMCP(t, "status")
	if err != nil || !strings.Contains(out, "not probed yet") {
		t.Fatalf("cold status: %v %q", err, out)
	}
}

// refresh probes (the real capture), then status shows the counts and names
// the servers that need a look.
func TestMCPCmd_RefreshThenStatus(t *testing.T) {
	capture, err := os.ReadFile("../internal/mcp/testdata/mcp-list-real.txt")
	if err != nil {
		t.Fatal(err)
	}
	r := &fake.Runner{Default: fake.Response{Stdout: capture}}
	withMCP(t, r)
	out, err := runMCP(t, "refresh")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "13 connected · 5 failed · 26 needs auth") {
		t.Fatalf("refresh summary: %q", out)
	}
	if !strings.Contains(out, "failed: ") || !strings.Contains(out, "needs authentication: ") {
		t.Errorf("refresh should list failed and needs-auth servers: %q", out)
	}
	out, err = runMCP(t, "status", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var s mcp.Status
	if err := json.Unmarshal([]byte(out), &s); err != nil || s.Connected != 13 {
		t.Fatalf("status --json: %v %+v", err, s)
	}
}

// By hand, refresh ignores the failure backoff; a failure is reported.
func TestMCPCmd_RefreshFailure(t *testing.T) {
	r := &fake.Runner{Default: fake.Response{Err: errors.New("claude: not found")}}
	withMCP(t, r)
	if _, err := runMCP(t, "refresh"); err == nil {
		t.Fatal("a failed probe must fail the verb")
	}
	if _, err := runMCP(t, "refresh"); err == nil || len(r.Calls) != 2 {
		t.Fatalf("second manual refresh must probe again despite the backoff (calls=%d)", len(r.Calls))
	}
}

func TestWriteMCPStatus_Stale(t *testing.T) {
	var b bytes.Buffer
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	writeMCPStatus(&b, mcp.Status{Connected: 1, Configured: 1, FetchedAt: now.Add(-3 * time.Minute), Stale: true}, now)
	if !strings.Contains(b.String(), "probed 3m0s ago (stale)") {
		t.Fatalf("%q", b.String())
	}
}
