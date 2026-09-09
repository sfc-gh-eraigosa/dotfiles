// Package histindex reads the update captures libs/log wrote — the
// <UTC>__<subject>.log files under fleet's state directory — into values the
// history command renders.
//
// It is pure text-in/struct-out apart from opening the files themselves: no
// clock, no network, no ssh. That is what lets the whole history surface be
// unit-tested from a temp directory.
package histindex

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// nameLayout is the timestamp format libs/log's CaptureName writes. Decoding
// depends on it being FIXED WIDTH — see decodeName.
const nameLayout = "20060102T150405Z"

// nameSep separates the timestamp from the subject in a capture filename.
const nameSep = "__"

// Run is one captured update of one host. Everything here is recovered from
// the filename, so listing hundreds of runs costs one ReadDir and no file
// opens at all.
type Run struct {
	Host string
	At   time.Time
	Path string
	Size int64
}

// Scan lists every capture in dir, newest first. Ties break on host so the
// order is total: a fleet-wide update writes several files in the same
// second, and a listing that reshuffled them between runs would be unusable.
//
// A missing directory is an EMPTY history rather than an error — "no runs
// yet" is the normal state on a machine that has never updated anything, and
// the same rule fleet applies to a missing ~/.ssh/config.
func Scan(dir string) ([]Run, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	var runs []Run
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		host, at, ok := decodeName(e.Name())
		if !ok {
			continue // not a capture; never guess at a bogus row
		}
		var size int64
		if info, err := e.Info(); err == nil {
			size = info.Size()
		}
		runs = append(runs, Run{
			Host: host,
			At:   at,
			Path: filepath.Join(dir, e.Name()),
			Size: size,
		})
	}

	sort.Slice(runs, func(i, j int) bool {
		if runs[i].At.Equal(runs[j].At) {
			return runs[i].Host < runs[j].Host
		}
		return runs[i].At.After(runs[j].At)
	})
	return runs, nil
}

// decodeName is the inverse of libs/log's CaptureName. It reads the
// timestamp POSITIONALLY rather than splitting on nameSep: SafeName permits
// underscores, so a host named "a__b" yields "<ts>__a__b.log" and a split on
// the first separator would call that host "a", on the last "b". The
// timestamp's width is fixed, so the boundary never has to be guessed.
//
// Anything that does not decode is reported as not-a-capture rather than
// repaired — a stray file in the log directory must not become a row
// claiming to be a run that never happened.
func decodeName(name string) (string, time.Time, bool) {
	body := strings.TrimSuffix(name, ".log")
	if body == name {
		return "", time.Time{}, false
	}
	if len(body) < len(nameLayout)+len(nameSep)+1 {
		return "", time.Time{}, false
	}
	if body[len(nameLayout):len(nameLayout)+len(nameSep)] != nameSep {
		return "", time.Time{}, false
	}
	at, err := time.Parse(nameLayout, body[:len(nameLayout)])
	if err != nil {
		return "", time.Time{}, false
	}
	return body[len(nameLayout)+len(nameSep):], at, true
}
