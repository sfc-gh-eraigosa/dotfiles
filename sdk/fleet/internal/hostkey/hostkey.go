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
// fingerprints known_hosts holds and the ones the host presents now — and, once
// a human has compared them, swaps one for the other with backups first. The
// decision lives in the caller (`fleet trust`), behind a typed confirmation.
//
// Everything follows what ssh itself resolves (`ssh -G`): the name it looks a
// host up under (HostKeyAlias, else HostName, bracketed off port 22, plus the
// resolved addresses under CheckHostIP), and every UserKnownHostsFile it reads
// — a stale key in known_hosts2 fails the connection just as surely.
//
// Every command goes through an injected Exec, so the whole flow is unit-tested
// without a socket or the operator's real known_hosts.
package hostkey

import (
	"fmt"
	"io"
	"net"
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

// lookupHost resolves a HostName for CheckHostIP; a var so tests stay offline.
var lookupHost = net.LookupHost

// appendFile is the one write into known_hosts; a var so a test can make the
// write fail after the old entries are already gone.
var appendFile = func(path, data string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, werr := f.WriteString(data)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	return werr
}

// Target is what ssh itself resolves an alias to.
type Target struct {
	Alias    string
	HostName string
	Port     string
	KeyAlias string // HostKeyAlias: when set, the ONLY name ssh looks up and stores
	// KnownHosts is every UserKnownHostsFile, in order; ssh reads them all and
	// writes new keys to the first.
	KnownHosts []string
	Hash       bool     // HashKnownHosts: new entries match the file's existing form
	IPs        []string // CheckHostIP addresses, when it is on for a DNS HostName
}

// Key is one host key, identified the way ssh prints it.
type Key struct {
	Type        string // ED25519, ECDSA, RSA
	Fingerprint string // SHA256:...
}

func (k Key) String() string { return k.Type + " " + k.Fingerprint }

// Presented is one key the host offered: its fingerprint and the
// "<type> <base64>" blob that would be written for it.
type Presented struct {
	Key  Key
	Blob string
}

// Resolve asks ssh (`ssh -G`) rather than parsing ~/.ssh/config, so Include,
// Match and wildcard blocks resolve exactly as they do for a real connection.
func Resolve(x Exec, alias string) (Target, error) {
	out, err := x("ssh", "", "-G", alias)
	if err != nil {
		return Target{}, fmt.Errorf("ssh -G %s: %w", alias, err)
	}
	t := Target{Alias: alias, Port: "22"}
	checkIP := false
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
		case "hostkeyalias":
			t.KeyAlias = v
		case "hashknownhosts":
			t.Hash = v == "yes"
		case "checkhostip":
			checkIP = v == "yes"
		case "userknownhostsfile":
			for _, f := range strings.Fields(v) {
				t.KnownHosts = append(t.KnownHosts, expandHome(f))
			}
		}
	}
	if t.HostName == "" || len(t.KnownHosts) == 0 {
		return Target{}, fmt.Errorf("ssh -G %s: no hostname or known_hosts file", alias)
	}
	// ssh only checks the address for a NAME it had to resolve, and an alias
	// replaces the lookup key entirely.
	if checkIP && t.KeyAlias == "" && net.ParseIP(t.HostName) == nil {
		if ips, err := lookupHost(t.HostName); err == nil {
			t.IPs = ips
		}
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

// Names are the known_hosts keys ssh uses for this target: the HostKeyAlias
// verbatim when set; otherwise the host (bracketed [host]:port off port 22)
// and, under CheckHostIP, each resolved address in the same form.
func (t Target) Names() []string {
	if t.KeyAlias != "" {
		return []string{t.KeyAlias}
	}
	names := []string{t.hostPort(t.HostName)}
	for _, ip := range t.IPs {
		names = append(names, t.hostPort(ip))
	}
	return names
}

func (t Target) hostPort(h string) string {
	if t.Port == "" || t.Port == "22" {
		return h
	}
	return fmt.Sprintf("[%s]:%s", h, t.Port)
}

// existing is the subset of KnownHosts present on disk; ssh-keygen -F/-R fail
// on a missing file, and a missing file holds no keys anyway.
func (t Target) existing() []string {
	var out []string
	for _, f := range t.KnownHosts {
		if _, err := os.Stat(f); err == nil {
			out = append(out, f)
		}
	}
	return out
}

// Known fingerprints what every user known_hosts file holds for the target. A
// host with no entry is an empty list, not an error (ssh-keygen -F exits 1).
func Known(x Exec, t Target) ([]Key, error) {
	var keys []Key
	for _, file := range t.existing() {
		for _, n := range t.Names() {
			out, _ := x("ssh-keygen", "", "-F", n, "-f", file)
			for l := range strings.SplitSeq(out, "\n") {
				if l = strings.TrimSpace(l); l == "" || strings.HasPrefix(l, "#") {
					continue
				}
				k, err := fingerprint(x, l)
				if err != nil {
					return nil, err
				}
				keys = append(keys, k)
			}
		}
	}
	return keys, nil
}

// Scan asks the host for the keys it presents now, fingerprinting each one on
// its own so a pin can select exactly the key it names.
func Scan(x Exec, t Target) ([]Presented, error) {
	raw, _ := x("ssh-keyscan", "", "-p", t.Port, "-T", "5", t.HostName)
	var ps []Presented
	for l := range strings.SplitSeq(raw, "\n") {
		f := strings.Fields(l)
		if len(f) < 3 || strings.HasPrefix(l, "#") {
			continue
		}
		k, err := fingerprint(x, l)
		if err != nil {
			return nil, err
		}
		ps = append(ps, Presented{Key: k, Blob: f[1] + " " + f[2]})
	}
	if len(ps) == 0 {
		return nil, fmt.Errorf("%s (%s) presented no host key — is sshd listening on port %s?", t.Alias, t.HostName, t.Port)
	}
	return ps, nil
}

func fingerprint(x Exec, line string) (Key, error) {
	out, err := x("ssh-keygen", line+"\n", "-lf", "-")
	if err != nil {
		return Key{}, fmt.Errorf("ssh-keygen -lf: %w", err)
	}
	ks := parseFingerprints(out)
	if len(ks) == 0 {
		return Key{}, fmt.Errorf("ssh-keygen -lf: no fingerprint in %q", out)
	}
	return ks[0], nil
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

// Keys lists the presented fingerprints.
func Keys(ps []Presented) []Key {
	ks := make([]Key, len(ps))
	for i, p := range ps {
		ks[i] = p.Key
	}
	return ks
}

// OnlyFingerprint keeps the presented key a --fingerprint pin names: a pin
// vouches for ONE key, so it must not carry the host's others in with it.
func OnlyFingerprint(ps []Presented, fp string) []Presented {
	var out []Presented
	for _, p := range ps {
		if fp != "" && p.Key.Fingerprint == fp {
			out = append(out, p)
		}
	}
	return out
}

// Entries renders the known_hosts lines for the chosen keys, one per lookup
// name, hashed by ssh-keygen itself when the client hashes — a hand-rolled
// hash would be one more place to get the format wrong.
func Entries(x Exec, t Target, ps []Presented) (string, error) {
	var b strings.Builder
	for _, n := range t.Names() {
		for _, p := range ps {
			fmt.Fprintf(&b, "%s %s\n", n, p.Blob)
		}
	}
	if !t.Hash {
		return b.String(), nil
	}
	tmp, err := os.CreateTemp("", "fleet-known-hosts-*")
	if err != nil {
		return "", err
	}
	// Cleanup is best-effort: a leftover temp file holds only public keys.
	defer func() { _ = os.Remove(tmp.Name() + ".old") }()
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := io.WriteString(tmp, b.String()); err != nil {
		_ = tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if _, err := x("ssh-keygen", "", "-H", "-f", tmp.Name()); err != nil {
		return "", fmt.Errorf("ssh-keygen -H: %w", err)
	}
	out, err := os.ReadFile(tmp.Name())
	return string(out), err
}

// Verdict is how the presented keys relate to the stored ones.
type Verdict string

const (
	New     Verdict = "new"     // nothing stored: first contact
	Same    Verdict = "same"    // every key type both sides know matches: nothing to fix
	Changed Verdict = "changed" // stored keys exist and ssh would refuse: the MITM case
)

// Compare decides the verdict the way ssh does. ssh prefers the key types it
// already knows, so ONE mismatched type fails the connection no matter what
// else matches — treating "any match" as Same would wave a swapped key
// through. Stored keys of none of the presented types fail too (BatchMode
// cannot accept the new type).
func Compare(stored, presented []Key) Verdict {
	if len(stored) == 0 {
		return New
	}
	matched := false
	for _, p := range presented {
		haveType := false
		for _, s := range stored {
			if s.Type != p.Type {
				continue
			}
			haveType = true
			if s.Fingerprint == p.Fingerprint {
				matched = true
			}
		}
		if haveType && !HasFingerprint(stored, p.Fingerprint) {
			return Changed
		}
	}
	if !matched {
		return Changed
	}
	return Same
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

// Replace backs up every existing known_hosts file, removes every entry for
// the target from each, then appends the new entries to the first file (ssh's
// write target), creating it — and ~/.ssh at 0700 — on a fresh machine. It
// returns the backup paths.
//
// If a removal fails nothing is appended: a new key beside an old one ssh still
// matches would leave the host just as broken, in a file that looks fixed. If
// the append fails the backups are restored, since by then the old entries are
// already gone.
func Replace(x Exec, t Target, entries, stamp string) ([]string, error) {
	files := t.existing()
	var backups []string
	for _, f := range files {
		b := f + ".fleet-bak-" + stamp
		if err := copyFile(f, b, os.O_EXCL); err != nil {
			return backups, fmt.Errorf("back up %s: %w", f, err)
		}
		backups = append(backups, b)
	}
	for _, f := range files {
		for _, n := range t.Names() {
			if _, err := x("ssh-keygen", "", "-R", n, "-f", f); err != nil {
				return backups, fmt.Errorf("remove %s from %s: %w (backups: %s)", n, f, err, strings.Join(backups, ", "))
			}
		}
	}
	target := t.KnownHosts[0]
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return backups, err
	}
	if !strings.HasSuffix(entries, "\n") {
		entries += "\n"
	}
	if err := appendFile(target, entries); err != nil {
		for i, f := range files {
			if rerr := copyFile(backups[i], f, os.O_TRUNC); rerr != nil {
				return backups, fmt.Errorf("write %s: %w; RESTORE FAILED (%v) — copy %s back by hand", target, err, rerr, backups[i])
			}
		}
		return backups, fmt.Errorf("write %s: %w (previous file restored)", target, err)
	}
	return backups, nil
}

// copyFile writes dst from src at 0600. mode is os.O_EXCL for a backup (never
// overwrite an earlier one) or os.O_TRUNC for a restore.
func copyFile(src, dst string, mode int) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }() // read-only: a close error loses nothing
	out, err := os.OpenFile(dst, os.O_CREATE|mode|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close() // the copy error is the one worth reporting
		return err
	}
	return out.Close()
}
