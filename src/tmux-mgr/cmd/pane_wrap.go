package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
)

var paneWrapWorkerRef string

var paneWrapCmd = &cobra.Command{
	Use:   "pane-wrap --worker-ref <ref> -- <agent-cli-cmd> [args...]",
	Short: "Run an agent CLI, then auto-checkpoint its gss worker on exit",
	Long: `pane-wrap execs the agent CLI given after "--", forwarding stdio and signals,
and on the agent's exit runs "gss feature checkpoint --auto --worker <ref>"
synchronously. The agent's exit code is forwarded; a checkpoint failure is
logged but does not change it. This is the localised pane-close
auto-checkpoint (design.md → tmux-mgr refactor "What's new"; resolution #5) —
no global tmux config change. The agent command MUST follow "--".`,
	// Everything after "--" is the agent argv; cobra puts it in args.
	RunE: func(cmd *cobra.Command, args []string) error {
		if paneWrapWorkerRef == "" {
			return fmt.Errorf("pane-wrap: --worker-ref is required")
		}
		if len(args) == 0 {
			return fmt.Errorf("pane-wrap: no agent command given after --")
		}
		os.Exit(runPaneWrap(paneWrapWorkerRef, args, paneWrapDeps{
			runChild:   execChild,
			checkpoint: runGssCheckpoint,
			stderr:     os.Stderr,
		}))
		return nil
	},
}

// paneWrapDeps are the injectable seams so the orchestration is unit-testable
// without spawning real processes.
type paneWrapDeps struct {
	runChild   func(argv []string) int      // run the agent; return its exit code
	checkpoint func(workerRef string) error // run the gss auto-checkpoint
	stderr     io.Writer
}

// runPaneWrap runs the agent, then auto-checkpoints, returning the AGENT's exit
// code (the checkpoint is best-effort: its failure is surfaced on stderr but
// never overrides the forwarded code).
func runPaneWrap(workerRef string, argv []string, deps paneWrapDeps) int {
	code := deps.runChild(argv)
	if err := deps.checkpoint(workerRef); err != nil {
		fmt.Fprintf(deps.stderr, "pane-wrap: auto-checkpoint failed: %v\n", err)
	}
	return code
}

// execChild runs argv with the parent's stdio, in its own process group so
// terminal signals can be relayed to the whole child tree, and returns the
// child's exit code (128+signal when it was killed by a signal).
func execChild(argv []string) int {
	c := exec.Command(argv[0], argv[1:]...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := c.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "pane-wrap: failed to start %q: %v\n", argv[0], err)
		return 127
	}

	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case s := <-sigc:
				if sig, ok := s.(syscall.Signal); ok && c.Process != nil {
					_ = syscall.Kill(-c.Process.Pid, sig) // negative pid → process group
				}
			case <-done:
				return
			}
		}
	}()

	err := c.Wait()
	signal.Stop(sigc)
	close(done)
	return exitCodeOf(err)
}

// exitCodeOf maps an exec.Cmd.Wait() error to a conventional exit code.
func exitCodeOf(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if ws, ok := ee.Sys().(syscall.WaitStatus); ok {
			if ws.Signaled() {
				return 128 + int(ws.Signal())
			}
			return ws.ExitStatus()
		}
		return ee.ExitCode()
	}
	return 1
}

// runGssCheckpoint runs the synchronous auto-checkpoint for the worker.
func runGssCheckpoint(workerRef string) error {
	c := exec.Command("gss", "feature", "checkpoint", "--auto", "--worker", workerRef)
	c.Stdout, c.Stderr = os.Stdout, os.Stderr
	return c.Run()
}

func init() {
	paneWrapCmd.Flags().StringVar(&paneWrapWorkerRef, "worker-ref", "", "gss worker ref to auto-checkpoint on the agent's exit (required)")
	internalCmd.AddCommand(paneWrapCmd)
}
