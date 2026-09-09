package resolvconf

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// All of these run against a temp root, so the risky part — replacing a symlink
// and rewriting system files — is covered without a privileged write anywhere.
func tempPaths(t *testing.T) Paths {
	t.Helper()
	root := t.TempDir()
	return Paths{
		ResolvConf: filepath.Join(root, "resolv.conf"),
		WslConf:    filepath.Join(root, "wsl.conf"),
		BackupDir:  filepath.Join(root, "backup"),
	}
}

// WSL ships /etc/resolv.conf as a symlink into /mnt/wsl, which is shared across
// distros. Seed that exact shape.
func seedStockLayout(t *testing.T, p Paths, sharedContent, wslConf string) string {
	t.Helper()
	shared := filepath.Join(filepath.Dir(p.ResolvConf), "shared-resolv.conf")
	if err := os.WriteFile(shared, []byte(sharedContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(shared, p.ResolvConf); err != nil {
		t.Fatal(err)
	}
	if wslConf != "" {
		if err := os.WriteFile(p.WslConf, []byte(wslConf), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return shared
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

// EC-17, the subtlest rule here. /mnt/wsl/resolv.conf is DISTRO-SHARED, so
// writing through the symlink would leak this machine's pin into every other
// WSL distro — and WSL would regenerate it anyway. Pin must replace the link
// with a real file and leave the shared target untouched.
func TestApply_ReplacesTheSharedSymlinkInsteadOfWritingThroughIt(t *testing.T) {
	p := tempPaths(t)
	shared := seedStockLayout(t, p, "nameserver 10.255.255.254\n", "[boot]\nsystemd=true\n")

	if err := Apply(p, RenderResolvConf(Render{Winner: "10.10.0.1"})); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	fi, err := os.Lstat(p.ResolvConf)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		t.Error("resolv.conf is still a symlink — the pin would leak into other distros and be regenerated")
	}
	if got := read(t, shared); got != "nameserver 10.255.255.254\n" {
		t.Errorf("the DISTRO-SHARED file was modified: %q", got)
	}
	if !IsManaged(read(t, p.ResolvConf)) {
		t.Error("resolv.conf is not the managed file")
	}
	if got := read(t, p.WslConf); !strings.Contains(got, "generateResolvConf = false") {
		t.Errorf("wsl.conf missing the key: %q", got)
	}
}

// EC-4: the snapshot must exist before the first byte is written, and must
// record that resolv.conf WAS a symlink and where it pointed — restoring it as
// a plain file would leave the machine subtly different from how it was found.
func TestApply_SnapshotsBeforeWritingAndRecordsTheSymlinkTarget(t *testing.T) {
	p := tempPaths(t)
	shared := seedStockLayout(t, p, "nameserver 10.255.255.254\n", "[boot]\nsystemd=true\n")

	if err := Apply(p, RenderResolvConf(Render{Winner: "10.10.0.1"})); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !HasSnapshot(p) {
		t.Fatal("no snapshot after Apply — there would be no undo path")
	}
	if got := read(t, filepath.Join(p.BackupDir, "resolv.conf.symlink")); got != shared+"\n" {
		t.Errorf("snapshot symlink target = %q, want %q", got, shared+"\n")
	}
	if got := read(t, filepath.Join(p.BackupDir, "wsl.conf.file")); got != "[boot]\nsystemd=true\n" {
		t.Errorf("snapshot wsl.conf = %q", got)
	}
}

// EC-4, the rule that makes the tool safe: if the snapshot cannot be written,
// NOTHING is written. Proceeding without an undo path is the one outcome that
// could strand a machine.
func TestApply_RefusesToWriteWhenTheSnapshotCannotBeTaken(t *testing.T) {
	p := tempPaths(t)
	seedStockLayout(t, p, "nameserver 10.255.255.254\n", "[boot]\nsystemd=true\n")

	// Make the backup's parent unwritable so the snapshot cannot be created.
	root := filepath.Dir(p.BackupDir)
	if err := os.Chmod(root, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o700) })

	err := Apply(p, RenderResolvConf(Render{Winner: "10.10.0.1"}))
	if err == nil {
		t.Fatal("Apply succeeded without a snapshot — it must refuse")
	}
	fi, lerr := os.Lstat(p.ResolvConf)
	if lerr != nil {
		t.Fatal(lerr)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Error("resolv.conf was replaced despite the snapshot failing")
	}
}

// Re-running pin must NOT re-snapshot: the second snapshot would capture our
// OWN managed files, and unpin would then "restore" the very state it is meant
// to undo.
func TestApply_DoesNotOverwriteAGoodSnapshotOnRerun(t *testing.T) {
	p := tempPaths(t)
	shared := seedStockLayout(t, p, "nameserver 10.255.255.254\n", "[boot]\nsystemd=true\n")

	for range 3 {
		if err := Apply(p, RenderResolvConf(Render{Winner: "10.10.0.1"})); err != nil {
			t.Fatalf("Apply: %v", err)
		}
	}
	if got := read(t, filepath.Join(p.BackupDir, "resolv.conf.symlink")); got != shared+"\n" {
		t.Errorf("snapshot was clobbered by a re-run: %q", got)
	}
	if _, err := os.Stat(filepath.Join(p.BackupDir, "resolv.conf.file")); err == nil {
		t.Error("re-run snapshotted our own managed file — unpin would restore the pin it is meant to remove")
	}
}

// EC-4 + EC-18: the whole undo path, end to end.
func TestRestore_RoundTripsByteForByteAndClearsTheSnapshot(t *testing.T) {
	p := tempPaths(t)
	originalWsl := "[boot]\nsystemd=true\n\n[user]\ndefault=someone\n"
	shared := seedStockLayout(t, p, "nameserver 10.255.255.254\n", originalWsl)

	if err := Apply(p, RenderResolvConf(Render{Winner: "10.10.0.1"})); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if _, err := Restore(p); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	target, err := os.Readlink(p.ResolvConf)
	if err != nil {
		t.Fatalf("resolv.conf is not a symlink again: %v", err)
	}
	if target != shared {
		t.Errorf("symlink target = %q, want %q", target, shared)
	}
	if got := read(t, p.WslConf); got != originalWsl {
		t.Errorf("wsl.conf not restored byte-for-byte\n got: %q\nwant: %q", got, originalWsl)
	}
	// EC-18: a stale snapshot would make the NEXT pin restore an old state.
	if HasSnapshot(p) {
		t.Error("snapshot survived Restore — the next pin would capture stale state")
	}
}

// EC-16: with no snapshot, unpin must still repair the machine rather than
// leaving it half-managed.
func TestRestore_WithoutASnapshotRepairsTheStockLayout(t *testing.T) {
	p := tempPaths(t)
	// A machine mid-way: managed resolv.conf as a real file, key set, no backup.
	if err := os.WriteFile(p.ResolvConf, []byte(RenderResolvConf(Render{Winner: "10.10.0.1"})), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.WslConf, []byte("[boot]\nsystemd=true\n\n[network]\ngenerateResolvConf = false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p.StockResolvTarget = filepath.Join(filepath.Dir(p.ResolvConf), "stock-target")

	rep, err := Restore(p)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if !rep.Repaired {
		t.Error("report must say this was a repair, not a snapshot restore")
	}
	target, err := os.Readlink(p.ResolvConf)
	if err != nil {
		t.Fatalf("resolv.conf was not restored to a symlink: %v", err)
	}
	if target != p.StockResolvTarget {
		t.Errorf("symlink target = %q, want the stock target %q", target, p.StockResolvTarget)
	}
	if got := read(t, p.WslConf); got != "[boot]\nsystemd=true\n" {
		t.Errorf("wsl.conf not repaired to stock: %q", got)
	}
}

// EC-11: a hand-edit after pinning must be visible. Silent drift means wlink
// reports a pin it no longer controls.
func TestDetectDrift(t *testing.T) {
	p := tempPaths(t)
	seedStockLayout(t, p, "nameserver 10.255.255.254\n", "[boot]\nsystemd=true\n")
	if err := Apply(p, RenderResolvConf(Render{Winner: "10.10.0.1"})); err != nil {
		t.Fatal(err)
	}

	if d, err := DetectDrift(p); err != nil || d != nil {
		t.Errorf("DetectDrift right after Apply = %+v, %v; want no drift", d, err)
	}

	if err := os.WriteFile(p.ResolvConf, []byte("nameserver 203.0.113.1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := DetectDrift(p)
	if err != nil {
		t.Fatalf("DetectDrift: %v", err)
	}
	if d == nil {
		t.Fatal("hand-edited resolv.conf reported no drift")
	}
	if d.Detail == "" {
		t.Error("drift must say what changed; a bare flag is not actionable")
	}
}

// Nothing pinned is not drift — it is simply an unmanaged machine.
func TestDetectDrift_UnmanagedIsNotDrift(t *testing.T) {
	p := tempPaths(t)
	seedStockLayout(t, p, "nameserver 10.255.255.254\n", "[boot]\nsystemd=true\n")
	d, err := DetectDrift(p)
	if err != nil {
		t.Fatalf("DetectDrift: %v", err)
	}
	if d != nil {
		t.Errorf("unmanaged machine reported drift: %+v", d)
	}
}

// A stock WSL install often has NO /etc/wsl.conf at all — WSL does not create
// one. So pin creates it, and unpin must REMOVE it rather than leaving an empty
// file behind that was never the user's.
func TestRestore_RemovesFilesThatDidNotExistBefore(t *testing.T) {
	p := tempPaths(t)
	shared := seedStockLayout(t, p, "nameserver 10.255.255.254\n", "") // no wsl.conf

	if err := Apply(p, RenderResolvConf(Render{Winner: "10.10.0.1"})); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if _, err := os.Stat(p.WslConf); err != nil {
		t.Fatalf("Apply should have created wsl.conf: %v", err)
	}
	if _, err := os.Stat(filepath.Join(p.BackupDir, "wsl.conf.absent")); err != nil {
		t.Error("snapshot did not record that wsl.conf was absent")
	}

	if _, err := Restore(p); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if _, err := os.Stat(p.WslConf); !os.IsNotExist(err) {
		t.Error("wsl.conf survived unpin, but the machine never had one — that is a file wlink left behind")
	}
	if target, err := os.Readlink(p.ResolvConf); err != nil || target != shared {
		t.Errorf("resolv.conf symlink not restored: target=%q err=%v", target, err)
	}
}

// resolv.conf missing entirely is a real state on a broken machine, and the
// snapshot must be able to reproduce "there was nothing here".
func TestSnapshot_RecordsAnAbsentResolvConf(t *testing.T) {
	p := tempPaths(t)
	if err := TakeSnapshot(p); err != nil {
		t.Fatalf("TakeSnapshot: %v", err)
	}
	if _, err := os.Stat(filepath.Join(p.BackupDir, "resolv.conf.absent")); err != nil {
		t.Error("snapshot did not record the absent resolv.conf")
	}
}

// A deleted managed file is drift too: wlink would otherwise report a pin that
// is no longer in force.
func TestDetectDrift_DeletedManagedFile(t *testing.T) {
	p := tempPaths(t)
	seedStockLayout(t, p, "nameserver 10.255.255.254\n", "[boot]\nsystemd=true\n")
	if err := Apply(p, RenderResolvConf(Render{Winner: "10.10.0.1"})); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(p.ResolvConf); err != nil {
		t.Fatal(err)
	}
	d, err := DetectDrift(p)
	if err != nil {
		t.Fatalf("DetectDrift: %v", err)
	}
	if d == nil {
		t.Fatal("a deleted managed resolv.conf reported no drift")
	}
}

// REVIEW FINDING 1 (HIGH). A snapshot interrupted after MkdirAll but before its
// records are written leaves a directory that LOOKS like an undo point. pin
// then writes believing it can be undone, and unpin deletes resolv.conf,
// restores nothing, and reports success — total DNS loss with no way back.
func TestHasSnapshot_RejectsAnIncompleteSnapshot(t *testing.T) {
	p := tempPaths(t)
	seedStockLayout(t, p, "nameserver 10.255.255.254\n", "[boot]\nsystemd=true\n")

	// Exactly what an interrupted TakeSnapshot leaves behind.
	if err := os.MkdirAll(p.BackupDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if HasSnapshot(p) {
		t.Fatal("an empty backup dir was accepted as an undo point")
	}

	// And Apply must refuse rather than trust it.
	if err := Apply(p, RenderResolvConf(Render{Winner: "10.10.0.1"})); err != nil {
		t.Fatalf("Apply should have completed the snapshot, not failed: %v", err)
	}
	if !HasSnapshot(p) {
		t.Error("Apply did not leave a complete snapshot")
	}
	if _, err := os.Readlink(p.ResolvConf); err == nil {
		t.Error("expected resolv.conf to be a real file after Apply")
	}
	// The undo path must actually work afterwards.
	if _, err := Restore(p); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if _, err := os.Readlink(p.ResolvConf); err != nil {
		t.Errorf("resolv.conf not restored to its symlink: %v", err)
	}
}

// REVIEW FINDING 3 (HIGH). `unpin` is advertised as "undo any time" and is
// suggested verbatim by doctor. On a machine wlink never touched — a
// hand-maintained or systemd-resolved resolv.conf — it must NOT delete the
// user's file and replace it with WSL's stock symlink.
func TestRestore_RefusesToRepairAFileWlinkNeverWrote(t *testing.T) {
	p := tempPaths(t)
	handMaintained := "# hand written\nnameserver 10.10.0.99\n"
	if err := os.WriteFile(p.ResolvConf, []byte(handMaintained), 0o644); err != nil {
		t.Fatal(err)
	}
	p.StockResolvTarget = filepath.Join(filepath.Dir(p.ResolvConf), "stock")

	rep, err := Restore(p)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if rep.Repaired {
		t.Error("reported a repair on a file wlink never wrote")
	}
	if got := read(t, p.ResolvConf); got != handMaintained {
		t.Errorf("destroyed a resolv.conf wlink did not write:\n got: %q\nwant: %q", got, handMaintained)
	}
	if rep.Detail == "" {
		t.Error("must explain why it declined")
	}
}

// The repair path still works for a file wlink DID write (EC-16).
func TestRestore_StillRepairsItsOwnFileWithoutASnapshot(t *testing.T) {
	p := tempPaths(t)
	if err := os.WriteFile(p.ResolvConf, []byte(RenderResolvConf(Render{Winner: "10.10.0.1"})), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.WslConf, []byte("[boot]\nsystemd=true\n\n[network]\ngenerateResolvConf = false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p.StockResolvTarget = filepath.Join(filepath.Dir(p.ResolvConf), "stock")

	rep, err := Restore(p)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if !rep.Repaired {
		t.Error("should have repaired its own managed file")
	}
	if _, err := os.Readlink(p.ResolvConf); err != nil {
		t.Errorf("not restored to a symlink: %v", err)
	}
}

// REVIEW FINDING 7 (MEDIUM). resolv.conf must be replaced atomically, and must
// exist before generateResolvConf is disabled — otherwise a crash between the
// two leaves no resolver AND WSL forbidden from regenerating one.
func TestApply_WritesResolvConfBeforeDisablingRegeneration(t *testing.T) {
	p := tempPaths(t)
	seedStockLayout(t, p, "nameserver 10.255.255.254\n", "[boot]\nsystemd=true\n")

	// Make wsl.conf unwritable: the resolv.conf write must already have
	// succeeded, so the machine still has a working resolver.
	if err := os.Chmod(p.WslConf, 0o400); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(p.WslConf, 0o600) })

	_ = Apply(p, RenderResolvConf(Render{Winner: "10.10.0.1"}))

	content, err := os.ReadFile(p.ResolvConf)
	if err != nil {
		t.Fatalf("resolv.conf is missing after a partial Apply — the machine has no resolver: %v", err)
	}
	if !IsManaged(string(content)) {
		t.Error("resolv.conf was not written before the wsl.conf step")
	}
}
