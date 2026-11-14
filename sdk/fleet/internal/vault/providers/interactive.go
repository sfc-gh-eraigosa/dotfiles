package providers

import (
	"context"
	"io"
	"os/exec"
)

// Interactive runs a vendor sign-in (`lpass login`, `op signin`) on the
// operator's terminal. It is the one runner both adapters use (Open wires it).
//
// Callers pass the command's real *os.File stdio (os.Stdin/Stdout/Stderr),
// never a bufio.Reader wrapped around it: exec hands an *os.File's fd straight
// to the child, whereas any other io.Reader is copied through a goroutine and
// a pipe, which keeps fleet blocked on a read after the child exits (the next
// typed line is lost) and feeds the child bytes the wrapper had buffered.
func Interactive(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, name string, args ...string) error {
	c := exec.CommandContext(ctx, name, args...)
	c.Stdin, c.Stdout, c.Stderr = stdin, stdout, stderr
	return c.Run()
}
