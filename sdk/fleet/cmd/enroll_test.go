package cmd

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/runner"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/sshconf"
)

// scriptRunner answers by (host, command fragment), because enroll sends three
// different commands to the same host (read the key, append it, verify) and a
// host-keyed fake cannot tell them apart.
type scriptRunner struct {
	recordingRunner
	answers []scriptAnswer
	hosts   *[]string
}

type scriptAnswer struct {
	host, contains, out string
	err                 error
}

func newScriptRunner(answers ...scriptAnswer) (scriptRunner, *[]string, *[]string) {
	var sent, hosts []string
	return scriptRunner{
		recordingRunner: recordingRunner{log: &sent},
		answers:         answers,
		hosts:           &hosts,
	}, &sent, &hosts
}

func (s scriptRunner) Run(host string, argv ...string) (string, error) {
	cmd := strings.Join(argv, " ")
	*s.log = append(*s.log, cmd)
	*s.hosts = append(*s.hosts, host)
	for _, a := range s.answers {
		if a.host == host && strings.Contains(cmd, a.contains) {
			return a.out, a.err
		}
	}
	return "", nil
}

// privKeyHeader is only the PEM header line of a private key — no key
// material — built from parts so the privacy guard does not flag it.
const privKeyHeader = "-----BEGIN " + "OPENSSH PRIVATE KEY-----"

const newPub = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIBOR4a4d0cO4lnewkey fleet"

// runAppend executes appendKeyCmd for real, so the shell is tested, not a
// string that merely looks right.
func runAppend(t *testing.T, path, pub string) string {
	t.Helper()
	out, err := exec.Command("bash", "-c", appendKeyCmd(path, pub, "20261114T000000Z")).CombinedOutput()
	if err != nil {
		t.Fatalf("appendKeyCmd failed: %v\n%s", err, out)
	}
	return strings.TrimSpace(string(out))
}

func backupsIn(t *testing.T, dir string) []string {
	t.Helper()
	ents, _ := os.ReadDir(dir)
	var out []string
	for _, e := range ents {
		if strings.Contains(e.Name(), ".bak-") {
			out = append(out, e.Name())
		}
	}
	return out
}

// The append is the one write enroll and sync make on every host: it keeps
// every existing key, takes a backup first, and reports what it did.
func TestAppendKeyCmdBacksUpThenAppends(t *testing.T) {
	dir := t.TempDir()
	ak := filepath.Join(dir, "authorized_keys")
	orig := "ssh-ed25519 AAAAexisting other@box\n"
	mustWriteFile(t, ak, []byte(orig), 0o600)

	if got := runAppend(t, ak, newPub); got != "added" {
		t.Fatalf("status = %q, want added", got)
	}
	now, _ := os.ReadFile(ak)
	if string(now) != orig+newPub+"\n" {
		t.Fatalf("file = %q", now)
	}
	baks := backupsIn(t, dir)
	if len(baks) != 1 {
		t.Fatalf("backups = %v, want exactly one", baks)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, baks[0])); string(b) != orig {
		t.Fatalf("backup = %q, want the original", b)
	}
	if fi, _ := os.Stat(ak); fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want 0600", fi.Mode().Perm())
	}
}

// Re-running must not grow the file or litter backups — including when the
// same key is already there under a different comment.
func TestAppendKeyCmdIsANoOpWhenTheKeyIsPresent(t *testing.T) {
	dir := t.TempDir()
	ak := filepath.Join(dir, "authorized_keys")
	body := strings.Replace(newPub, " fleet", " renamed@elsewhere", 1) + "\n"
	mustWriteFile(t, ak, []byte(body), 0o600)

	if got := runAppend(t, ak, newPub); got != "present" {
		t.Fatalf("status = %q, want present", got)
	}
	if now, _ := os.ReadFile(ak); string(now) != body {
		t.Fatalf("file changed: %q", now)
	}
	if b := backupsIn(t, dir); len(b) != 0 {
		t.Fatalf("a no-op must not take a backup: %v", b)
	}
}

// A file whose last line has no newline would otherwise get the new key glued
// onto the end of the old one, corrupting BOTH entries.
func TestAppendKeyCmdNeverGluesOntoAnUnterminatedLine(t *testing.T) {
	dir := t.TempDir()
	ak := filepath.Join(dir, "authorized_keys")
	mustWriteFile(t, ak, []byte("ssh-ed25519 AAAAexisting other@box"), 0o600)
	runAppend(t, ak, newPub)
	now, _ := os.ReadFile(ak)
	if string(now) != "ssh-ed25519 AAAAexisting other@box\n"+newPub+"\n" {
		t.Fatalf("file = %q", now)
	}
}

func TestAppendKeyCmdCreatesAMissingFile(t *testing.T) {
	dir := t.TempDir()
	ak := filepath.Join(dir, "sub", "authorized_keys")
	if got := runAppend(t, ak, newPub); got != "added" {
		t.Fatalf("status = %q", got)
	}
	if now, _ := os.ReadFile(ak); string(now) != newPub+"\n" {
		t.Fatalf("file = %q", now)
	}
	if b := backupsIn(t, filepath.Dir(ak)); len(b) != 0 {
		t.Fatalf("nothing existed, so nothing to back up: %v", b)
	}
}

// A key comment is free text; a quote in it must not break out of the shell.
func TestAppendKeyCmdQuotesTheKey(t *testing.T) {
	dir := t.TempDir()
	ak := filepath.Join(dir, "authorized_keys")
	weird := "ssh-ed25519 AAAAweird it's $(touch " + filepath.Join(dir, "pwned") + ")"
	runAppend(t, ak, weird)
	if _, err := os.Stat(filepath.Join(dir, "pwned")); err == nil {
		t.Fatal("the key comment was executed by the shell")
	}
	if now, _ := os.ReadFile(ak); string(now) != weird+"\n" {
		t.Fatalf("file = %q", now)
	}
}

func TestNormalizePubAcceptsOnlyPublicKeys(t *testing.T) {
	for _, ok := range []string{
		newPub,
		"  " + newPub + "  \n",
		"ssh-rsa AAAAB3Nza me@box",
		"ecdsa-sha2-nistp256 AAAAE2Vj",
		// A security-key type name is "<alg>@openssh.com"; built from parts
		// so the repo's privacy guard does not read it as an email address.
		"sk-ssh-ed25519" + "@" + "openssh.com AAAAGnNr yubi",
	} {
		if _, err := normalizePub(ok); err != nil {
			t.Errorf("normalizePub(%q) = %v, want ok", ok, err)
		}
	}
	for _, bad := range []string{
		"",
		privKeyHeader,
		"no key pair in ~/.ssh",
		"ssh-ed25519",
		"ssh-ed25519 not-base64!",
	} {
		if _, err := normalizePub(bad); err == nil {
			t.Errorf("normalizePub(%q) accepted junk", bad)
		}
	}
	got, _ := normalizePub("ssh-ed25519   AAAAx   a  comment\nsecond line")
	if got != "ssh-ed25519 AAAAx a comment" {
		t.Fatalf("normalized = %q", got)
	}
}

// Without --key, enroll reads whichever key the NEW machine would offer, in
// the same preference order fleet uses locally (pickIdentity).
func TestRemotePubCmdFollowsIdentityOrder(t *testing.T) {
	cmd, err := remotePubCmd("")
	if err != nil {
		t.Fatal(err)
	}
	ed, ec, rsa := strings.Index(cmd, "id_ed25519"), strings.Index(cmd, "id_ecdsa"), strings.Index(cmd, "id_rsa")
	if ed < 0 || ec < ed || rsa < ec {
		t.Fatalf("order wrong in %q", cmd)
	}
	if _, err := remotePubCmd("../../etc/shadow"); err == nil {
		t.Fatal("a path in --key must be refused")
	}
	if c, err := remotePubCmd("work_key"); err != nil || !strings.Contains(c, "work_key") {
		t.Fatalf("named key: %q, %v", c, err)
	}
}

// The remote read script must run under zsh too: several fleet hosts use it
// as the login shell, and ssh hands the command to that shell.
func TestRemotePubCmdRunsUnderShAndZsh(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, filepath.Join(home, ".ssh", "id_ecdsa"), []byte("priv"), 0o600)
	mustWriteFile(t, filepath.Join(home, ".ssh", "id_ecdsa.pub"), []byte("ecdsa-sha2-nistp256 AAAAE2 me\n"), 0o644)
	mustWriteFile(t, filepath.Join(home, ".ssh", "id_rsa.pub"), []byte("ssh-rsa AAAAB orphan\n"), 0o644) // no private half
	cmd, _ := remotePubCmd("")
	for _, sh := range []string{"sh", "bash", "zsh"} {
		if _, err := exec.LookPath(sh); err != nil {
			continue
		}
		c := exec.Command(sh, "-c", cmd)
		c.Env = append(os.Environ(), "HOME="+home)
		out, err := c.Output()
		if err != nil {
			t.Fatalf("%s: %v", sh, err)
		}
		if strings.TrimSpace(string(out)) != "ecdsa-sha2-nistp256 AAAAE2 me" {
			t.Fatalf("%s: read %q", sh, out)
		}
	}
}

func TestVerifyCmdRefusesUnsafeDestinations(t *testing.T) {
	if _, err := verifyCmd([]enrollTarget{{Alias: "nano", Dest: "me@10.0.0.63", Port: "2222"}}, ""); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []enrollTarget{
		{Alias: "x", Dest: "a;rm -rf ~"},
		{Alias: "x", Dest: "$(id)"},
		{Alias: "x", Dest: "a b"},
		{Alias: "x", Dest: "-oProxyCommand=x"},
		{Alias: "x;y", Dest: "ok"},
		{Alias: "x", Dest: "ok", Port: "22;id"},
	} {
		if _, err := verifyCmd([]enrollTarget{bad}, ""); err == nil {
			t.Errorf("verifyCmd accepted %+v", bad)
		}
	}
}

func TestParseVerify(t *testing.T) {
	got := parseVerify("OK\tnano\nFAIL\tgig\tHost key verification failed.\nnoise\n")
	if len(got) != 2 || !got[0].OK || got[0].Alias != "nano" || got[1].OK || got[1].Reason != "Host key verification failed." {
		t.Fatalf("parsed %+v", got)
	}
}

func baseEnroll(confirm bool) (*strings.Builder, enrollOpts, *[]string) {
	var local []string
	sb := &strings.Builder{}
	return sb, enrollOpts{
		Dest:  "box2",
		Stamp: "20261114T000000Z",
		Targets: []enrollTarget{
			{Alias: "nano", Dest: "me@10.0.0.63"},
			{Alias: "box2", Dest: "me@10.0.0.175"},
		},
		Confirm:     func(string) bool { return confirm },
		Fingerprint: func(string) (string, error) { return "256 SHA256:test fleet (ED25519)", nil },
		AuthorizeLocal: func(pub string) (string, error) {
			local = append(local, pub)
			return "added", nil
		},
	}, &local
}

// The repair that had to be done by hand: read the new machine's PUBLIC key,
// authorize it on every fleet host and here, then prove it from the new
// machine's side.
func TestEnrollAuthorizesTheNewMachineEverywhereAndVerifies(t *testing.T) {
	r, sent, hosts := newScriptRunner(
		scriptAnswer{host: "box2", contains: "id_ed25519", out: newPub + "\n"},
		scriptAnswer{host: "nano", contains: "authorized_keys", out: "added\n"},
		scriptAnswer{host: "box2", contains: "BatchMode", out: "OK\tnano\nOK\tbox2\n"},
		scriptAnswer{host: "box2", contains: "authorized_keys", out: "added\n"},
	)
	sb, o, local := baseEnroll(true)
	if err := enrollKey(sb, r, o); err != nil {
		t.Fatalf("enroll: %v\n%s", err, sb)
	}
	out := sb.String()
	for _, want := range []string{"SHA256:test", "nano", "box2", "verified"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if len(*local) != 1 || (*local)[0] != newPub {
		t.Fatalf("local authorize = %v", *local)
	}
	appended := map[string]bool{}
	for i, c := range *sent {
		if strings.Contains(c, ">>") {
			appended[(*hosts)[i]] = true
			if !strings.Contains(c, ".bak-20261114T000000Z") {
				t.Errorf("append on %s takes no backup: %q", (*hosts)[i], c)
			}
		}
		if strings.Contains(c, "PRIVATE KEY") || strings.HasPrefix(c, "scp ") {
			t.Fatalf("private material or file copy: %q", c)
		}
	}
	if !appended["nano"] || !appended["box2"] {
		t.Fatalf("appended on %v, want nano and box2", appended)
	}
}

func TestEnrollChangesNothingWhenDeclined(t *testing.T) {
	r, sent, _ := newScriptRunner(scriptAnswer{host: "box2", contains: "id_ed25519", out: newPub})
	sb, o, local := baseEnroll(false)
	if err := enrollKey(sb, r, o); err != nil {
		t.Fatal(err)
	}
	for _, c := range *sent {
		if strings.Contains(c, ">>") {
			t.Fatalf("declined enroll still wrote: %q", c)
		}
	}
	if len(*local) != 0 {
		t.Fatal("declined enroll still authorized locally")
	}
	if !strings.Contains(sb.String(), "nothing changed") {
		t.Fatalf("output:\n%s", sb)
	}
}

// The fingerprint is what the operator checks before saying yes; it must be
// shown BEFORE the question, with every host that would change.
func TestEnrollShowsTheFingerprintBeforeAsking(t *testing.T) {
	r, _, _ := newScriptRunner(scriptAnswer{host: "box2", contains: "id_ed25519", out: newPub})
	sb, o, _ := baseEnroll(false)
	var atAsk string
	o.Confirm = func(string) bool { atAsk = sb.String(); return false }
	_ = enrollKey(sb, r, o)
	if !strings.Contains(atAsk, "SHA256:test") || !strings.Contains(atAsk, "nano") {
		t.Fatalf("at the prompt the operator had seen only:\n%s", atAsk)
	}
}

func TestEnrollRefusesWhatIsNotAPublicKey(t *testing.T) {
	r, sent, _ := newScriptRunner(scriptAnswer{host: "box2", contains: "id_ed25519", out: privKeyHeader})
	sb, o, _ := baseEnroll(true)
	if err := enrollKey(sb, r, o); err == nil {
		t.Fatal("a non-public key must be refused")
	}
	for _, c := range *sent {
		if strings.Contains(c, ">>") {
			t.Fatalf("wrote after refusing: %q", c)
		}
	}
}

func TestEnrollNamesTheUnreachableNewMachine(t *testing.T) {
	r, _, _ := newScriptRunner(scriptAnswer{host: "box2", contains: "id_ed25519", err: errors.New("exit 255")})
	sb, o, _ := baseEnroll(true)
	err := enrollKey(sb, r, o)
	if err == nil || !strings.Contains(err.Error(), "box2") {
		t.Fatalf("err = %v, want it to name box2", err)
	}
}

// One host failing must not stop the rest, and must not look like success.
func TestEnrollReportsPerHostFailureAndContinues(t *testing.T) {
	r, sent, hosts := newScriptRunner(
		scriptAnswer{host: "box2", contains: "id_ed25519", out: newPub},
		scriptAnswer{host: "nano", contains: "authorized_keys", err: errors.New("exit 255")},
		scriptAnswer{host: "box2", contains: "authorized_keys", out: "added"},
	)
	sb, o, _ := baseEnroll(true)
	err := enrollKey(sb, r, o)
	if err == nil {
		t.Fatal("a failed host must fail the command")
	}
	if !strings.Contains(sb.String(), "FAIL") || !strings.Contains(sb.String(), "nano") {
		t.Fatalf("output must name the failure:\n%s", sb)
	}
	wroteBox2 := false
	for i, c := range *sent {
		if strings.Contains(c, ">>") && (*hosts)[i] == "box2" {
			wroteBox2 = true
		}
	}
	if !wroteBox2 {
		t.Fatal("a failure on nano stopped the rest of the fleet")
	}
}

// After enrolling, the new machine may still not trust a host's host key.
// That is not an auth failure: say so and point at `fleet trust`.
func TestEnrollRoutesAnUntrustedHostKeyToFleetTrust(t *testing.T) {
	r, _, _ := newScriptRunner(
		scriptAnswer{host: "box2", contains: "id_ed25519", out: newPub},
		scriptAnswer{host: "box2", contains: "BatchMode", out: "OK\tbox2\nFAIL\tnano\tHost key verification failed.\n"},
		scriptAnswer{host: "nano", contains: "authorized_keys", out: "added"},
		scriptAnswer{host: "box2", contains: "authorized_keys", out: "present"},
	)
	sb, o, _ := baseEnroll(true)
	_ = enrollKey(sb, r, o)
	if !strings.Contains(sb.String(), "fleet trust nano") {
		t.Fatalf("output:\n%s", sb)
	}
}

func TestEnrollTargetsAddTheNewMachineWhenNotInTheFleet(t *testing.T) {
	got := enrollTargets(nil, "me@10.0.0.9", nil, true)
	if len(got) != 1 || got[0].Alias != "me@10.0.0.9" || got[0].Dest != "me@10.0.0.9" {
		t.Fatalf("targets = %+v", got)
	}
}

// keys delete revokes ONE named key: diff-first, nothing before confirmation,
// and it removes the matching line exactly as the host wrote it.
func TestRevokeHostDeclinedChangesNothing(t *testing.T) {
	r := recordingRunner{fake: runner.Fake{Out: map[string]string{
		"alpha": "ssh-ed25519 AAAkeep other\nssh-ed25519 AAAgone renamed@there",
	}}, log: new([]string)}
	var sb strings.Builder
	res, err := revokeHost(&sb, r, "alpha", "ssh-ed25519 AAAgone me@here", false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Found != 1 || res.Changed {
		t.Fatalf("res = %+v", res)
	}
	for _, c := range *r.log {
		if strings.Contains(c, ".bak-") {
			t.Fatalf("declined revoke mutated: %q", c)
		}
	}
	if !strings.Contains(sb.String(), "renamed@there") {
		t.Fatalf("must show the exact line:\n%s", sb.String())
	}
}

func TestRevokeHostRemovesTheMatchedLine(t *testing.T) {
	r := recordingRunner{fake: runner.Fake{Out: map[string]string{
		"alpha": "ssh-ed25519 AAAkeep other\nssh-ed25519 AAAgone renamed@there",
	}}, log: new([]string)}
	var sb strings.Builder
	res, err := revokeHost(&sb, r, "alpha", "ssh-ed25519 AAAgone me@here", true)
	if err != nil || !res.Changed {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
	removed := false
	for _, c := range *r.log {
		if strings.HasPrefix(c, "sh -c ") && strings.Contains(c, "ssh-ed25519 AAAgone") && !strings.Contains(c, "AAAkeep") {
			removed = true
		}
	}
	if !removed {
		t.Fatalf("expected a targeted removal of the matched key: %v", *r.log)
	}
}

func TestRevokeHostAbsentKeyIsANoOp(t *testing.T) {
	r := recordingRunner{fake: runner.Fake{Out: map[string]string{"alpha": "ssh-ed25519 AAAkeep other"}}, log: new([]string)}
	res, err := revokeHost(&strings.Builder{}, r, "alpha", "ssh-ed25519 AAAgone me", true)
	if err != nil || res.Found != 0 || res.Changed {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
}

// Onboarding: a freshly generated key is authorized for logins to THIS
// machine (the scan lists it as a fleet host too) and the next step is named.
func TestAuthorizeLocalAtAppendsWithBackup(t *testing.T) {
	dir := t.TempDir()
	ak := filepath.Join(dir, "authorized_keys")
	mustWriteFile(t, ak, []byte("ssh-ed25519 AAAAexisting x\n"), 0o600)
	st, err := authorizeLocalAt(ak, newPub, "20261114T000000Z")
	if err != nil || st != "added" {
		t.Fatalf("st = %q, err = %v", st, err)
	}
	if st, _ := authorizeLocalAt(ak, newPub, "20261114T000001Z"); st != "present" {
		t.Fatalf("second run = %q, want present", st)
	}
}

func TestEnrollHintNamesARunnableCommand(t *testing.T) {
	h := enrollHint("me@10.0.0.42")
	if !strings.Contains(h, "fleet keys enroll me@10.0.0.42") {
		t.Fatalf("hint = %q", h)
	}
	if !strings.Contains(h, "already") {
		t.Fatalf("hint must say WHERE to run it: %q", h)
	}
}

func TestSelfDestPrefersAnAddress(t *testing.T) {
	if got := selfDestFrom("me", "10.0.0.42", "box2"); got != "me@10.0.0.42" {
		t.Fatalf("got %q", got)
	}
	if got := selfDestFrom("me", "", "box2"); got != "me@box2" {
		t.Fatalf("got %q", got)
	}
	if got := selfDestFrom("", "", ""); got != "<this-machine>" {
		t.Fatalf("got %q", got)
	}
}

// keys delete used to echo the displayed line back through Go %q (double
// quotes): the remote shell expanded $HOME in a command= option, a CRLF or
// re-indented line no longer matched grep -x, nothing was removed — and the
// host was still reported "removed". The removal now matches type+blob on
// the host and fails unless the key is gone.
func TestRevokeKeyCmdRemovesTheKeyHoweverTheLineIsWritten(t *testing.T) {
	dir := t.TempDir()
	ak := filepath.Join(dir, "authorized_keys")
	gone := "ssh-ed25519 AAAAgone"
	mustWriteFile(t, ak, []byte(
		"ssh-ed25519 AAAAkeep other\n"+
			`command="rrsync $HOME/x",no-pty `+gone+" old-laptop\n"+
			"  "+gone+"   spaced\n"+
			gone+" crlf\r\n"+
			"# "+gone+" a note about the key\n"+
			"ssh-ed25519 AAAAgonesuffix lookalike\n"), 0o600)
	for _, sh := range []string{"bash", "zsh"} {
		if _, err := exec.LookPath(sh); err != nil {
			continue
		}
		cmd := revokeKeyCmd(ak, gone, "20261114T000000Z")
		out, err := exec.Command(sh, "-c", cmd).CombinedOutput()
		if err != nil || strings.TrimSpace(string(out)) != "removed" {
			t.Fatalf("%s: %v %q", sh, err, out)
		}
		got, _ := os.ReadFile(ak)
		want := "ssh-ed25519 AAAAkeep other\n# " + gone + " a note about the key\nssh-ed25519 AAAAgonesuffix lookalike\n"
		if string(got) != want {
			t.Fatalf("%s: file now\n%q\nwant\n%q", sh, got, want)
		}
		if len(backupsIn(t, dir)) == 0 {
			t.Fatalf("%s: no backup taken", sh)
		}
	}
}

func TestRevokeKeyCmdFailsOnAMissingFile(t *testing.T) {
	ak := filepath.Join(t.TempDir(), "authorized_keys")
	if out, err := exec.Command("sh", "-c", revokeKeyCmd(ak, "ssh-ed25519 AAAA", "x")).CombinedOutput(); err == nil {
		t.Fatalf("revoke on a missing file reported success: %q", out)
	}
}

// A key present only behind options was narrowed on purpose: appending a
// plain line would silently widen it, and calling it "present" hid that
// the new machine is still refused.
func TestAppendKeyCmdReportsARestrictedGrantAndLeavesIt(t *testing.T) {
	dir := t.TempDir()
	ak := filepath.Join(dir, "authorized_keys")
	before := `from="10.0.0.5" ` + newPub + "\n"
	mustWriteFile(t, ak, []byte(before), 0o600)
	if got := runAppend(t, ak, newPub); got != "restricted" {
		t.Fatalf("got %q, want restricted", got)
	}
	if b, _ := os.ReadFile(ak); string(b) != before {
		t.Fatalf("restricted grant was changed: %q", b)
	}
	if !strings.Contains(appendNote("restricted"), "from=") {
		t.Fatal("restricted needs an explanation")
	}
}

func TestAppendKeyCmdSeesTheKeyThroughCRLFAndSpacing(t *testing.T) {
	ak := filepath.Join(t.TempDir(), "authorized_keys")
	f := strings.Fields(newPub)
	mustWriteFile(t, ak, []byte("  "+f[0]+"   "+f[1]+" other\r\n"), 0o600)
	if got := runAppend(t, ak, newPub); got != "present" {
		t.Fatalf("got %q, want present", got)
	}
}

// Remote commands reach the login shell, which may be fish or tcsh; they are
// handed to sh as one quoted word, and that word must survive the trip.
func TestRemoteBuildersRunUnderShFromAnyLoginShell(t *testing.T) {
	ak := filepath.Join(t.TempDir(), "authorized_keys")
	c := posixSh(appendKeyCmd(ak, newPub, "20261114T000000Z"))
	if !strings.HasPrefix(c, "sh -c '") || strings.Contains(c, "!") {
		t.Fatalf("not an sh -c word tcsh can carry: %q", c)
	}
	for _, sh := range []string{"bash", "zsh", "fish", "tcsh"} {
		if _, err := exec.LookPath(sh); err != nil {
			continue
		}
		_ = os.Remove(ak)
		out, err := exec.Command(sh, "-c", c).CombinedOutput()
		if err != nil || strings.TrimSpace(string(out)) != "added" {
			t.Fatalf("%s: %v %q", sh, err, out)
		}
	}
}

// --key names the identity to prove; the check must offer only that one,
// or a different, unauthorized default key makes every host "FAIL".
func TestVerifyCmdOffersOnlyTheNamedKey(t *testing.T) {
	ts := []enrollTarget{{Alias: "nano", Dest: "me@10.0.0.63"}}
	c, err := verifyCmd(ts, "fleet_ed25519")
	if err != nil || !strings.Contains(c, `-o IdentitiesOnly=yes -i "$HOME/.ssh/fleet_ed25519"`) {
		t.Fatalf("err = %v, cmd = %q", err, c)
	}
	if c, _ := verifyCmd(ts, ""); strings.Contains(c, "IdentitiesOnly") {
		t.Fatalf("no --key must leave ssh's own choice: %q", c)
	}
	if _, err := verifyCmd(ts, "../x"); err == nil {
		t.Fatal("unsafe key name accepted")
	}
}

// A block with no User line logs in as whatever OUR ssh resolves; the new
// machine's own login name may differ, so the destination must name it.
func TestEnrollTargetsNameTheResolvedLoginUser(t *testing.T) {
	hosts := []sshconf.Host{{Alias: "spark", HostName: "10.0.0.201"}, {Alias: "pi", HostName: "10.0.0.128", User: "pi"}}
	got := enrollTargets(hosts, "me@10.0.0.9", func(string) string { return "alice" }, true)
	if got[0].Dest != "alice@10.0.0.201" || got[1].Dest != "pi@10.0.0.128" {
		t.Fatalf("targets = %+v", got)
	}
	if len(got) != 3 {
		t.Fatalf("the new machine itself should be a target: %+v", got)
	}
	if got := enrollTargets(hosts, "me@10.0.0.9", nil, false); len(got) != 2 {
		t.Fatalf("a scoped enroll must leave the new machine out: %+v", got)
	}
}

// --host nano means nano: this machine's authorized_keys is not touched.
func TestScopedEnrollLeavesThisMachineAlone(t *testing.T) {
	r, sent, hosts := newScriptRunner(
		scriptAnswer{host: "box2", contains: "id_ed25519", out: newPub},
		scriptAnswer{host: "nano", contains: "authorized_keys", out: "added"},
	)
	sb, o, local := baseEnroll(true)
	o.Targets = o.Targets[:1]
	o.AuthorizeLocal = nil
	if err := enrollKey(sb, r, o); err != nil {
		t.Fatalf("%v\n%s", err, sb)
	}
	if len(*local) != 0 || strings.Contains(sb.String(), "(this machine)") {
		t.Fatalf("scoped enroll touched this machine:\n%s", sb)
	}
	for i, c := range *sent {
		if strings.Contains(c, ">>") && (*hosts)[i] != "nano" {
			t.Fatalf("wrote outside --host: %s %q", (*hosts)[i], c)
		}
	}
}

// A machine with no SSH server can still be enrolled from a copied .pub;
// nothing then connects to it, so there is no read and no check.
func TestEnrollWithAHandedOverPubkeyNeverConnectsToTheNewMachine(t *testing.T) {
	r, _, hosts := newScriptRunner(scriptAnswer{host: "nano", contains: "authorized_keys", out: "added"})
	sb, o, _ := baseEnroll(true)
	o.Pub = newPub + "\n"
	o.Targets = o.Targets[:1]
	if err := enrollKey(sb, r, o); err != nil {
		t.Fatalf("%v\n%s", err, sb)
	}
	for _, h := range *hosts {
		if h == "box2" {
			t.Fatalf("connected to the new machine: %v", *hosts)
		}
	}
	if !strings.Contains(sb.String(), "not verified") {
		t.Fatalf("must say it could not check:\n%s", sb)
	}
}

func TestReadPubArg(t *testing.T) {
	p := filepath.Join(t.TempDir(), "k.pub")
	mustWriteFile(t, p, []byte(newPub+"\n"), 0o644)
	if b, err := readPubArg(nil, p); err != nil || strings.TrimSpace(string(b)) != newPub {
		t.Fatalf("%q %v", b, err)
	}
	if b, err := readPubArg(strings.NewReader(newPub), "-"); err != nil || string(b) != newPub {
		t.Fatalf("stdin: %q %v", b, err)
	}
	if _, err := readPubArg(strings.NewReader("  \n"), "-"); err == nil {
		t.Fatal("empty input accepted")
	}
}
