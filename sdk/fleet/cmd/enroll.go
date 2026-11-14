package cmd

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/runner"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/sshconf"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/sshfail"
	"github.com/spf13/cobra"
)

// remoteAuthorizedKeys is authorized_keys as a double-quotable shell word.
// $HOME rather than ~ because the path is quoted (a quoted ~ never expands),
// and quoted because the same command builder is run against temp paths.
const remoteAuthorizedKeys = "$HOME/.ssh/authorized_keys"

// backupStamp names the authorized_keys / config backups, matching the
// config.bak-<ts> and known_hosts.fleet-bak-<ts> files fleet already writes.
func backupStamp(t time.Time) string { return t.UTC().Format("20060102T150405Z") }

// shq single-quotes s for any POSIX shell (and zsh, the login shell on several
// fleet hosts). A key comment is free text, so it must never be interpolated
// raw into a command line.
func shq(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

var (
	pubTypeRE = regexp.MustCompile(`^(ssh-(ed25519|rsa|dss)|ecdsa-sha2-nistp(256|384|521)|sk-(ssh-ed25519|ecdsa-sha2-nistp256)@openssh\.com)$`)
	pubBlobRE = regexp.MustCompile(`^[A-Za-z0-9+/]+={0,2}$`)
	keyNameRE = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._-]*$`)
	// destRE is what may appear as an ssh destination or alias inside a
	// command fleet builds: no shell metacharacters, and no leading '-' so a
	// value can never be read by ssh as an option (-oProxyCommand=...).
	destRE = regexp.MustCompile(`^[A-Za-z0-9._][A-Za-z0-9._@:%-]*$`)
	portRE = regexp.MustCompile(`^[0-9]{1,5}$`)
)

// normalizePub returns the first line of raw as a single-spaced public key,
// or an error if it is not one. Enroll reads this from ANOTHER machine, so it
// is checked before anything is written anywhere; the error never echoes the
// input, in case a private key was handed over by mistake.
func normalizePub(raw string) (string, error) {
	line, _, _ := strings.Cut(strings.TrimSpace(raw), "\n")
	f := strings.Fields(line)
	if len(f) < 2 || !pubTypeRE.MatchString(f[0]) || !pubBlobRE.MatchString(f[1]) {
		return "", fmt.Errorf("not an SSH public key")
	}
	return strings.Join(f, " "), nil
}

// keyNeedle is the part of a public key that identifies it: type and blob.
// Matching on it rather than the whole line means a key already authorized
// under a different comment counts as present.
func keyNeedle(pub string) string {
	f := strings.Fields(pub)
	if len(f) < 2 {
		return pub
	}
	return f[0] + " " + f[1]
}

// appendKeyCmd builds the ONE write fleet makes to an authorized_keys file
// when adding a key (keys sync, keys enroll, onboarding). It:
//   - keeps every existing line — it only ever appends;
//   - is a no-op (prints "present") when the key is already authorized under
//     any comment, so re-runs neither grow the file nor litter backups;
//   - copies a non-empty file to <file>.bak-<stamp> before touching it;
//   - terminates an unterminated last line first, so the new key is never
//     glued onto the previous one (which corrupts both);
//   - prints "added" on success and exits non-zero on any failure.
//
// path is used inside double quotes, so it may reference $HOME.
func appendKeyCmd(path, pub, stamp string) string {
	return fmt.Sprintf(`f="%s"; k=%s; n=%s; d=$(dirname "$f"); `+
		`{ mkdir -p "$d" && chmod 700 "$d" && touch "$f" && chmod 600 "$f"; } || exit 1; `+
		`if grep -v '^[[:space:]]*#' "$f" | grep -qF -- "$n"; then echo present; exit 0; fi; `+
		`if [ -s "$f" ]; then cp -p "$f" "$f.bak-%s" || exit 1; `+
		`if [ -n "$(tail -c 1 "$f")" ]; then echo >> "$f" || exit 1; fi; fi; `+
		`printf '%%s\n' "$k" >> "$f" && echo added`,
		path, shq(pub), shq(keyNeedle(pub)), stamp)
}

// appendStatus reads the last word appendKeyCmd printed.
func appendStatus(out string) string {
	f := strings.Fields(out)
	if len(f) == 0 {
		return "ok"
	}
	return f[len(f)-1]
}

// remotePubCmd prints the public half of the key a machine would offer: the
// named one, or the first of identityOrder whose private AND public halves
// both exist (privateKeysIn's rule). Only a .pub is ever read.
func remotePubCmd(name string) (string, error) {
	names := identityOrder
	if name != "" {
		name = strings.TrimSuffix(name, ".pub")
		if !keyNameRE.MatchString(name) {
			return "", fmt.Errorf("--key %q: want a file name in ~/.ssh, not a path", name)
		}
		names = []string{name}
	}
	return fmt.Sprintf(`for k in %s; do if [ -f "$HOME/.ssh/$k" ] && [ -f "$HOME/.ssh/$k.pub" ]; then `+
		`head -n 1 "$HOME/.ssh/$k.pub"; exit 0; fi; done; `+
		`echo "no SSH key pair (%s) in ~/.ssh" >&2; exit 3`,
		strings.Join(names, " "), strings.Join(names, ", ")), nil
}

// enrollTarget is one host to authorize the new key on: Alias is how THIS
// machine reaches it, Dest/Port how the NEW machine should (it may not share
// our ssh config, so the alias would not resolve there).
type enrollTarget struct{ Alias, Dest, Port string }

// verifyCmd is run ON the new machine: one BatchMode login per target,
// printing "OK\t<alias>" or "FAIL\t<alias>\t<last ssh stderr line>". Every
// value is validated, then quoted, before it reaches the command line.
func verifyCmd(ts []enrollTarget) (string, error) {
	parts := make([]string, 0, len(ts))
	for _, t := range ts {
		if !destRE.MatchString(t.Alias) || !destRE.MatchString(t.Dest) {
			return "", fmt.Errorf("refusing unsafe host name %q / %q", t.Alias, t.Dest)
		}
		port := ""
		if t.Port != "" {
			if !portRE.MatchString(t.Port) {
				return "", fmt.Errorf("refusing unsafe port %q for %s", t.Port, t.Alias)
			}
			port = " -p " + t.Port
		}
		parts = append(parts, fmt.Sprintf(
			`if e=$(ssh -n -o BatchMode=yes -o ConnectTimeout=5%s %s true 2>&1 >/dev/null); `+
				`then printf 'OK\t%%s\n' %s; `+
				`else printf 'FAIL\t%%s\t%%s\n' %s "$(printf '%%s' "$e" | tail -n 1)"; fi`,
			port, shq(t.Dest), shq(t.Alias), shq(t.Alias)))
	}
	return strings.Join(parts, "; "), nil
}

type verifyResult struct {
	Alias, Reason string
	OK            bool
}

func parseVerify(out string) []verifyResult {
	var res []verifyResult
	for _, line := range strings.Split(out, "\n") {
		f := strings.SplitN(strings.TrimRight(line, "\r"), "\t", 3)
		switch {
		case len(f) >= 2 && f[0] == "OK":
			res = append(res, verifyResult{Alias: f[1], OK: true})
		case len(f) >= 2 && f[0] == "FAIL":
			r := verifyResult{Alias: f[1]}
			if len(f) == 3 {
				r.Reason = strings.TrimSpace(f[2])
			}
			res = append(res, r)
		}
	}
	return res
}

// enrollTargets turns this machine's fleet into targets, adding the new
// machine itself when the fleet does not already list it, so the new key can
// also log in to the machine that owns it (`ssh <self-alias>`).
func enrollTargets(hosts []sshconf.Host, dest string) []enrollTarget {
	_, destHost := splitDest(dest)
	out := make([]enrollTarget, 0, len(hosts)+1)
	self := false
	for _, h := range hosts {
		addr := h.HostName
		if addr == "" {
			addr = h.Alias
		}
		d := addr
		if h.User != "" {
			d = h.User + "@" + addr
		}
		out = append(out, enrollTarget{Alias: h.Alias, Dest: d, Port: h.Port})
		if h.Alias == dest || h.Alias == destHost || addr == destHost {
			self = true
		}
	}
	if !self {
		out = append(out, enrollTarget{Alias: dest, Dest: dest})
	}
	return out
}

func splitDest(dest string) (userPart, host string) {
	if u, h, ok := strings.Cut(dest, "@"); ok {
		return u, h
	}
	return "", dest
}

type enrollOpts struct {
	Dest    string // how THIS machine reaches the new one (alias or user@host)
	Key     string // key name on the new machine; "" = its preferred identity
	Stamp   string // backup suffix
	Targets []enrollTarget
	Confirm func(question string) bool
	// Fingerprint renders the key for the operator to check out of band.
	Fingerprint func(pub string) (string, error)
	// AuthorizeLocal adds the key to THIS machine's authorized_keys.
	AuthorizeLocal func(pub string) (string, error)
}

// enrollKey authorizes another machine's PUBLIC key on every fleet host and on
// this machine, then proves it by logging in from the new machine's side.
//
// This is the step a new machine cannot do for itself: when every host takes
// keys only, appending to their authorized_keys needs the very access the new
// key does not have yet, and ssh-copy-id has no password to fall back on. A
// machine that already has access does it instead. Nothing is written before
// the operator has seen the fingerprint and every host that would change and
// said yes; each write keeps existing keys and takes a backup first.
func enrollKey(out io.Writer, r runner.Runner, o enrollOpts) error {
	readCmd, err := remotePubCmd(o.Key)
	if err != nil {
		return err
	}
	raw, err := r.Run(o.Dest, readCmd)
	if err != nil {
		why := ""
		if sshfail.Classify(err) == sshfail.Auth && !sshfail.IsHostKey(sshfail.Note(err)) {
			why = " — this machine cannot log in to it either; run enroll from one that can"
		}
		return fmt.Errorf("%s: reading its public key: %w%s", o.Dest, err, why)
	}
	pub, err := normalizePub(raw)
	if err != nil {
		return fmt.Errorf("%s: %w", o.Dest, err)
	}
	fp, err := o.Fingerprint(pub)
	if err != nil {
		return fmt.Errorf("fingerprinting %s's key: %w", o.Dest, err)
	}

	fmt.Fprintf(out, "%s offers this key — check it on that machine (ssh-keygen -lf ~/.ssh/<key>.pub):\n    %s\n", o.Dest, strings.TrimSpace(fp))
	fmt.Fprintln(out, "authorize it for logins to:")
	fmt.Fprintf(out, "    %-20s ~/.ssh/authorized_keys\n", "(this machine)")
	for _, t := range o.Targets {
		fmt.Fprintf(out, "    %-20s %s\n", t.Alias, t.Dest)
	}
	fmt.Fprintln(out, "existing keys are kept; each file is backed up before the key is appended.")
	if !o.Confirm("authorize this key on all of the above?") {
		fmt.Fprintln(out, "nothing changed")
		return nil
	}

	failures := 0
	if st, err := o.AuthorizeLocal(pub); err != nil {
		fmt.Fprintf(out, "FAIL %-20s %v\n", "(this machine)", err)
		failures++
	} else {
		fmt.Fprintf(out, "ok   %-20s %s\n", "(this machine)", st)
	}
	var done []enrollTarget
	for _, t := range o.Targets {
		res, err := r.Run(t.Alias, appendKeyCmd(remoteAuthorizedKeys, pub, o.Stamp))
		if err != nil {
			fmt.Fprintf(out, "FAIL %-20s %v\n", t.Alias, err)
			failures++
			continue
		}
		fmt.Fprintf(out, "ok   %-20s %s\n", t.Alias, appendStatus(res))
		done = append(done, t)
	}

	failures += verifyFromNewMachine(out, r, o.Dest, done)
	if failures > 0 {
		return fmt.Errorf("%d host(s) failed", failures)
	}
	return nil
}

// verifyFromNewMachine logs in from the new machine to each authorized host and
// returns how many still refuse its key. A host-key fault is not counted: the
// key is authorized, the new machine just has not trusted that host yet, and
// the fix (`fleet trust`) is on the new machine, not here.
func verifyFromNewMachine(out io.Writer, r runner.Runner, dest string, ts []enrollTarget) int {
	if len(ts) == 0 {
		return 0
	}
	vc, err := verifyCmd(ts)
	if err != nil {
		fmt.Fprintf(out, "not verified: %v\n", err)
		return 0
	}
	vout, err := r.Run(dest, vc)
	if err != nil {
		fmt.Fprintf(out, "not verified: could not run the check on %s: %v\n", dest, err)
		return 0
	}
	res := parseVerify(vout)
	if len(res) == 0 {
		fmt.Fprintf(out, "not verified: %s returned no result\n", dest)
		return 0
	}
	fmt.Fprintf(out, "verified from %s:\n", dest)
	failed := 0
	var untrusted []string
	for _, v := range res {
		switch {
		case v.OK:
			fmt.Fprintf(out, "  ok   %s\n", v.Alias)
		case strings.Contains(v.Reason, "Host key verification failed") || strings.Contains(v.Reason, "HOST IDENTIFICATION HAS CHANGED"):
			fmt.Fprintf(out, "  key  %s — authorized, but %s does not trust its host key yet\n", v.Alias, dest)
			untrusted = append(untrusted, v.Alias)
		default:
			fmt.Fprintf(out, "  FAIL %s — %s\n", v.Alias, v.Reason)
			failed++
		}
	}
	for _, a := range untrusted {
		fmt.Fprintf(out, "  on %s, compare fingerprints and accept: fleet trust %s\n", dest, a)
	}
	return failed
}

// authorizeLocalAt runs appendKeyCmd against a local authorized_keys file.
// Same command as the remote path, so both behave identically.
func authorizeLocalAt(path, pub, stamp string) (string, error) {
	out, err := exec.Command("sh", "-c", appendKeyCmd(path, pub, stamp)).Output()
	if err != nil {
		return "", fmt.Errorf("authorizing in %s: %w", path, err)
	}
	return appendStatus(string(out)), nil
}

func authorizeLocal(pub string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return authorizeLocalAt(filepath.Join(home, ".ssh", "authorized_keys"), pub, backupStamp(time.Now()))
}

// fingerprintPub renders a public key the way `ssh-keygen -lf` does, which is
// what the operator compares against the new machine.
func fingerprintPub(pub string) (string, error) {
	c := exec.Command("ssh-keygen", "-lf", "-")
	c.Stdin = strings.NewReader(pub + "\n")
	out, err := c.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// selfDest is how ANOTHER machine could reach this one, for the enroll hint.
// A variable so tests can pin it. Computed once: the TUI tip asks on every
// redraw, and the answer does not change while fleet runs.
var selfDest = sync.OnceValue(defaultSelfDest)

func defaultSelfDest() string {
	name := os.Getenv("USER")
	if u, err := user.Current(); err == nil && u.Username != "" {
		name = u.Username
	}
	host, _ := os.Hostname()
	return selfDestFrom(name, outboundIP(), host)
}

// selfDestFrom prefers the LAN address: another machine's ssh config rarely
// knows this one's hostname, but it can always reach the address.
func selfDestFrom(userName, addr, host string) string {
	where := addr
	if where == "" {
		where = host
	}
	if where == "" {
		return "<this-machine>"
	}
	if userName == "" {
		return where
	}
	return userName + "@" + where
}

// outboundIP is the source address this machine would use to reach the LAN.
// Connecting a UDP socket sends nothing; it only asks the kernel for a route.
// 192.0.2.1 is TEST-NET-1 (RFC 5737), never a real peer.
func outboundIP() string {
	c, err := net.Dial("udp", "192.0.2.1:9")
	if err != nil {
		return ""
	}
	defer func() { _ = c.Close() }()
	a, ok := c.LocalAddr().(*net.UDPAddr)
	if !ok || a.IP.IsLoopback() || a.IP.IsUnspecified() {
		return ""
	}
	return a.IP.String()
}

// enrollHint is the next step for a machine whose key nothing accepts yet.
func enrollHint(dest string) string {
	return fmt.Sprintf("  on a machine that already has SSH access to the fleet, run:\n      fleet keys enroll %s\n", dest)
}

// revokeHost removes ONE key from a host's authorized_keys: every line holding
// that key (under any comment) is shown first and removed, exactly as written,
// only when confirmed. The keys-delete twin of pruneHost.
func revokeHost(out io.Writer, r runner.Runner, host, pub string, confirmed bool) (PruneResult, error) {
	rk, err := remoteKeys(r, host)
	if err != nil {
		return PruneResult{}, err
	}
	needle := keyNeedle(pub)
	var hits []string
	for _, line := range rk {
		line = strings.TrimRight(line, "\r")
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		if strings.Contains(strings.Join(strings.Fields(t), " "), needle) {
			hits = append(hits, line)
		}
	}
	res := PruneResult{Found: len(hits)}
	if res.Found == 0 {
		return res, nil
	}
	fmt.Fprintf(out, "%s: would remove:\n", host)
	for _, h := range hits {
		fmt.Fprintf(out, "  - %s\n", h)
	}
	if !confirmed {
		fmt.Fprintf(out, "%s: not confirmed — nothing changed\n", host)
		return res, nil
	}
	for _, h := range hits {
		if _, err := r.Run(host, removeKeyCmd(authorizedKeys, h)); err != nil {
			return res, fmt.Errorf("%s: removing key: %w", host, err)
		}
	}
	res.Changed = true
	fmt.Fprintf(out, "%s: removed\n", host)
	return res, nil
}

var (
	enrollKeyName string
	enrollHosts   []string
	enrollYes     bool
	deleteHosts   []string
	deleteYes     bool
)

var keysEnrollCmd = &cobra.Command{
	Use:   "enroll <ssh-destination>",
	Short: "Authorize ANOTHER machine's public key on every fleet host (run where you already have access)",
	Long: `Run this on a machine that can already log in to the fleet, naming a new
machine whose key nothing accepts yet (an alias, or user@address):

    fleet keys enroll me@192.168.0.42

It reads the new machine's PUBLIC key over SSH, shows its fingerprint and
every host that would change, and on confirmation appends it (backup first,
existing keys kept) to authorized_keys here and on every fleet host, then
logs in from the new machine to prove it worked. No private key moves.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dest := args[0]
		if !destRE.MatchString(dest) {
			return fmt.Errorf("%q is not an ssh destination fleet will use", dest)
		}
		hosts, err := fleetHosts()
		if err != nil {
			return err
		}
		if hosts, err = checkHosts(hosts, enrollHosts); err != nil {
			return err
		}
		return enrollKey(cmd.OutOrStdout(), runner.Exec{}, enrollOpts{
			Dest:    dest,
			Key:     enrollKeyName,
			Stamp:   backupStamp(time.Now()),
			Targets: enrollTargets(hosts, dest),
			Confirm: func(q string) bool {
				return enrollYes || askYesNo(cmd, q)
			},
			Fingerprint:    fingerprintPub,
			AuthorizeLocal: authorizeLocal,
		})
	},
}

var keysDeleteCmd = &cobra.Command{
	Use:   "delete <key-name>",
	Short: "Revoke one of your keys (~/.ssh/<key-name>.pub) on every fleet host, on confirmation",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := strings.TrimSuffix(args[0], ".pub")
		mine, err := managedKeys()
		if err != nil {
			return err
		}
		pub, ok := mine[name]
		if !ok {
			return fmt.Errorf("no managed public key named %q (looked for ~/.ssh/%s.pub)", name, name)
		}
		hosts, err := fleetHosts()
		if err != nil {
			return err
		}
		if hosts, err = checkHosts(hosts, deleteHosts); err != nil {
			return err
		}
		out := cmd.OutOrStdout()
		if home, err := os.UserHomeDir(); err == nil && pickIdentity(privateKeysIn(filepath.Join(home, ".ssh"))) == name {
			fmt.Fprintf(out, "warning: ~/.ssh/%s is the key this machine uses to reach the fleet —\n"+
				"  revoking it ends this machine's access to every host you confirm.\n", name)
		}
		r := runner.Exec{}
		failures := 0
		for _, h := range hosts {
			res, err := revokeHost(out, r, h.Alias, pub, deleteYes)
			if err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "FAIL %s: %v\n", h.Alias, err)
				failures++
				continue
			}
			if res.Found == 0 || res.Changed {
				continue
			}
			if !askYesNo(cmd, fmt.Sprintf("remove it from %s?", h.Alias)) {
				fmt.Fprintf(out, "%s: skipped\n", h.Alias)
				continue
			}
			if _, err := revokeHost(io.Discard, r, h.Alias, pub, true); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "FAIL %s: %v\n", h.Alias, err)
				failures++
			} else {
				fmt.Fprintf(out, "%s: removed\n", h.Alias)
			}
		}
		fmt.Fprintf(out, "~/.ssh/%s and %s.pub are untouched; delete them yourself once nothing uses them.\n", name, name)
		if failures > 0 {
			return fmt.Errorf("%d host(s) failed", failures)
		}
		return nil
	},
}

func init() {
	keysEnrollCmd.Flags().StringVar(&enrollKeyName, "key", "", "key name in the new machine's ~/.ssh (default: the one it would offer)")
	keysEnrollCmd.Flags().StringSliceVar(&enrollHosts, "host", nil, "authorize on only these fleet hosts (repeatable); default is every fleet host")
	keysEnrollCmd.Flags().BoolVar(&enrollYes, "yes", false, "skip the confirmation prompt (non-interactive)")
	keysDeleteCmd.Flags().StringSliceVar(&deleteHosts, "host", nil, "revoke on only these fleet hosts (repeatable); default is every fleet host")
	keysDeleteCmd.Flags().BoolVar(&deleteYes, "yes", false, "skip the confirmation prompts (non-interactive)")
	keysCmd.AddCommand(keysEnrollCmd, keysDeleteCmd)
}
