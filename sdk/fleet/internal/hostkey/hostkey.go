// Package hostkey re-establishes trust in a fleet host's SSH host key.
//
// fleet probes with BatchMode=yes, so a host whose key changed (a reinstall, a
// DHCP lease handed to a different machine) is refused instantly and stays
// refused: sshfail names it `host key CHANGED`, but nothing in fleet could
// repair it, and the one key the TUI offered (`A`, ssh-copy-id) connects twice
// and prints ssh's MITM banner twice without fixing anything.
//
// Accepting a changed key is exactly the decision ssh refuses to make for us,
// so this package never makes it either. It only gathers the evidence — the
// fingerprint known_hosts holds and the one the host presents now — and, once a
// human has compared them, swaps one for the other with a backup first. The
// decision lives in the caller (`fleet trust`), behind a typed confirmation.
//
// Every command goes through an injected Exec, so the whole flow is unit-tested
// without a socket or the operator's real known_hosts.
package hostkey

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Exec runs a local command with stdin and returns its stdout.
type Exec func(name, stdin string, args ...string) (string, error)

// Run is the real Exec.
func Run(name, stdin string, args ...string) (string, error) {
	c := exec.Command(name, args...)
	c.Stdin = strings.NewReader(stdin)
	out, err := c.Output()
	return string(out), err
}

// Target is what ssh itself resolves an alias to — the name known_hosts is
// keyed on is the HostName, not the alias.
type Target struct {
	Alias      string
	HostName   string
	Port       string
	KnownHosts string // the first UserKnownHostsFile: where ssh writes, so where we write
	Hash       bool   // HashKnownHosts: new entries match the file's existing form
}

// Key is one host key, identified the way ssh prints it.
type Key struct {
	Type        string // ED25519, ECDSA, RSA
	Fingerprint string // SHA256:...
}

func (k Key) String() string { return k.Type + " " + k.Fingerprint }

// Resolve asks ssh (`ssh -G`) rather than parsing ~/.ssh/config, so Include,
// Match and wildcard blocks resolve exactly as they do for a real connection.
func Resolve(x Exec, alias string) (Target, error) {
	out, err := x("ssh", "", "-G", alias)
	if err != nil {
		return Target{}, fmt.Errorf("ssh -G %s: %w", alias, err)
	}
	t := Target{Alias: alias, Port: "22"}
	for line := range strings.SplitSeq(out, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), " ")
		if !ok {
			continue
		}
		switch k {
		case "hostname":
			t.HostName = v
		case "port":
			t.Port = v
		case "hashknownhosts":
			t.Hash = v == "yes"
		case "userknownhostsfile":
			t.KnownHosts = expandHome(strings.Fields(v)[0])
		}
	}
	if t.HostName == "" || t.KnownHosts == "" {
		return Target{}, fmt.Errorf("ssh -G %s: no hostname or known_hosts file", alias)
	}
	return t, nil
}

func expandHome(p string) string {
	if rest, ok := strings.CutPrefix(p, "~/"); ok {
		if h, err := os.UserHomeDir(); err == nil {
			return filepath.Join(h, rest)
		}
	}
	return p
}

// Names are the known_hosts lookup keys ssh uses for this target: the bare
// host on port 22, the bracketed [host]:port form otherwise.
func (t Target) Names() []string {
	if t.Port == "" || t.Port == "22" {
		return []string{t.HostName}
	}
	return []string{fmt.Sprintf("[%s]:%s", t.HostName, t.Port)}
}

// Known fingerprints what known_hosts currently holds for the target. A host
// with no entry is an empty list, not an error (ssh-keygen -F exits 1).
func Known(x Exec, t Target) ([]Key, error) {
	var lines []string
	for _, n := range t.Names() {
		out, err := x("ssh-keygen", "", "-F", n, "-f", t.KnownHosts)
		if err != nil && strings.TrimSpace(out) == "" {
			continue
		}
		for l := range strings.SplitSeq(out, "\n") {
			if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "#") {
				lines = append(lines, l)
			}
		}
	}
	if len(lines) == 0 {
		return nil, nil
	}
	return fingerprint(x, strings.Join(lines, "\n")+"\n")
}

// Scan asks the host for the keys it presents now. It returns the raw
// known_hosts lines (hashed when the client hashes) alongside their
// fingerprints, so what gets written is exactly what was shown.
func Scan(x Exec, t Target) (string, []Key, error) {
	args := []string{"-p", t.Port, "-T", "5", t.HostName}
	if t.Hash {
		args = append([]string{"-H"}, args...)
	}
	raw, _ := x("ssh-keyscan", "", args...)
	if strings.TrimSpace(raw) == "" {
		return "", nil, fmt.Errorf("%s (%s) presented no host key — is sshd listening on port %s?", t.Alias, t.HostName, t.Port)
	}
	keys, err := fingerprint(x, raw)
	if err != nil {
		return "", nil, err
	}
	return raw, keys, nil
}

func fingerprint(x Exec, lines string) ([]Key, error) {
	out, err := x("ssh-keygen", lines, "-lf", "-")
	if err != nil {
		return nil, fmt.Errorf("ssh-keygen -lf: %w", err)
	}
	return parseFingerprints(out), nil
}

// parseFingerprints reads `ssh-keygen -l` output: "<bits> <fp> <comment> (<TYPE>)".
func parseFingerprints(out string) []Key {
	var keys []Key
	for l := range strings.SplitSeq(out, "\n") {
		f := strings.Fields(l)
		if len(f) < 3 {
			continue
		}
		typ := strings.Trim(f[len(f)-1], "()")
		keys = append(keys, Key{Type: typ, Fingerprint: f[1]})
	}
	return keys
}

// Verdict is how the presented keys relate to the stored ones.
type Verdict string

const (
	New     Verdict = "new"     // nothing stored: first contact
	Same    Verdict = "same"    // a presented key is already trusted: nothing to fix
	Changed Verdict = "changed" // stored keys exist and none matches: the MITM case
)

// Compare decides the verdict. One overlapping fingerprint is enough for Same:
// ssh accepts a host when any of its keys matches.
func Compare(stored, presented []Key) Verdict {
	if len(stored) == 0 {
		return New
	}
	for _, p := range presented {
		if HasFingerprint(stored, p.Fingerprint) {
			return Same
		}
	}
	return Changed
}

// HasFingerprint is an exact match; an empty fingerprint never matches.
func HasFingerprint(keys []Key, fp string) bool {
	if fp == "" {
		return false
	}
	for _, k := range keys {
		if k.Fingerprint == fp {
			return true
		}
	}
	return false
}

// Replace backs known_hosts up, removes every entry for the target, then
// appends the scanned lines. It returns the backup path. If a removal fails
// nothing is appended: a new key beside an old one ssh still matches first
// would leave the host just as broken, with a file that looks fixed.
func Replace(x Exec, t Target, raw, stamp string) (string, error) {
	backup := t.KnownHosts + ".fleet-bak-" + stamp
	if _, err := os.Stat(t.KnownHosts); errors.Is(err, os.ErrNotExist) {
		backup = "" // first contact on a fresh machine: nothing to back up
	} else if err := copyFile(t.KnownHosts, backup); err != nil {
		return "", fmt.Errorf("back up %s: %w", t.KnownHosts, err)
	}
	for _, n := range t.Names() {
		if _, err := x("ssh-keygen", "", "-R", n, "-f", t.KnownHosts); err != nil {
			return backup, fmt.Errorf("remove %s from %s: %w (backup at %s)", n, t.KnownHosts, err, backup)
		}
	}
	f, err := os.OpenFile(t.KnownHosts, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return backup, err
	}
	if !strings.HasSuffix(raw, "\n") {
		raw += "\n"
	}
	_, werr := f.WriteString(raw)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	return backup, werr
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
