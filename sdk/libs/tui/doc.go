// Package tui is the umbrella for the shared TUI behaviors every sdk tool
// composes: keymap (data-driven keys + dispatch), nav (cursor/viewport),
// prompt (line editor), search (incremental smartcase regex), cmdline
// (ex-style commands + completion), overlay (help + confirm).
//
// Read GUIDE.md in this directory before writing or changing a TUI. The
// packages are pure: they own state over ints and strings, take closures for
// the tool's edges, and never touch disk, network, colors, or tool types.
package tui
