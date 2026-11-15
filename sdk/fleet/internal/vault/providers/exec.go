package providers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/vault"
)

// SystemExec is a vault.Exec over os/exec. It pipes stdin to the child. A
// failure reports the exit status and trimmed stderr, never stdin. A missing
// binary maps to vault.ErrNotInstalled.
func SystemExec(ctx context.Context, name string, stdin []byte, args ...string) ([]byte, error) {
	c := exec.CommandContext(ctx, name, args...)
	if stdin != nil {
		c.Stdin = bytes.NewReader(stdin)
	}
	var out, errb bytes.Buffer
	c.Stdout, c.Stderr = &out, &errb
	if err := c.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, vault.ErrNotInstalled
		}
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return nil, fmt.Errorf("%s: exit status %d: %s", name, ee.ExitCode(), strings.TrimSpace(errb.String()))
		}
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return out.Bytes(), nil
}
