// Package identity owns the worker-identity machinery for gss: the
// embedded suffix wordlist (this file), the random suffix draw (PR-07),
// and user/purpose resolution + validation (PR-08). A worker reference has
// the form feature/user/purpose[-suffix]; the suffix is drawn from the
// pool defined here.
package identity

import (
	_ "embed"
	"strings"
)

// wordlistRaw is the built-in suffix pool, embedded at build time. The
// file holds exactly 256 short (3-5 letter) lowercase words drawn from
// nature, weather, terrain, plants, minerals, and small everyday objects
// (design.md → "Suffix wordlist"). Whitespace layout in the file is
// irrelevant — Words() splits on any run of whitespace.
//
//go:embed wordlist.txt
var wordlistRaw string

// words is the parsed pool, computed once at package init.
var words = strings.Fields(wordlistRaw)

// Words returns the built-in suffix pool. The result is a fresh copy on
// every call, so callers (e.g. the config "append"/"replace" merge in
// PR-07) cannot mutate the shared pool.
func Words() []string {
	return append([]string(nil), words...)
}
