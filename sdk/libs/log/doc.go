// Package log is the shared logging driver for every sdk tool.
//
// It exists because gsl had already proven the shape — logrus for structured
// records, lumberjack for rotation, and a construction path that can never
// fail — and every tool after it was about to reinvent some worse fraction of
// that. fleet was writing install output with bare fmt.Fprintf and hand-rolled
// pruning when this package was extracted.
//
// # Two writers, deliberately
//
// A tool produces two different kinds of output, and forcing them through one
// pipe makes both worse:
//
//   - DIAGNOSTICS — what the tool itself did and why. Structured (JSON),
//     rotated, machine-greppable. Use New / Default.
//   - CAPTURED OUTPUT — bytes produced by something else, such as a remote
//     install's stdout. Its whole value is being readable as-is in `less`;
//     wrapping each line in JSON destroys that. Use NewCapture.
//
// Both share this package's guarantees so a tool does not hand-roll file
// modes, directory creation, or retention.
//
// # Construction is total
//
// Nothing here returns an error for a logging problem. A logger whose file
// cannot be opened writes to io.Discard and stays usable. Observability must
// never introduce a failure mode into the thing it observes — a tool that dies
// because it could not log is strictly worse than one that runs unlogged.
//
// # Paths
//
// ResolvePath(tool) picks, in order:
//
//  1. $<TOOL>_LOG_FILE — verbatim override (tests, power users)
//  2. $XDG_STATE_HOME/<tool>/<tool>.log
//  3. $HOME/.local/state/<tool>/<tool>.log
//  4. $HOME/.cache/<tool>/<tool>.log
//  5. "" — caller falls back to io.Discard
package log
