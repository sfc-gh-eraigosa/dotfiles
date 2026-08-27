package resolvconf

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DefaultStockResolvTarget is where WSL's stock /etc/resolv.conf symlink
// points. That file is SHARED ACROSS DISTROS, which is why pinning replaces the
// link rather than writing through it.
const DefaultStockResolvTarget = "/mnt/wsl/resolv.conf"

// Paths locates the files wlink manages. They are parameters rather than
// constants so the whole read-modify-write path is testable against a temp
// directory — no privileged writes in the test suite.
type Paths struct {
	ResolvConf string
	WslConf    string
	BackupDir  string
	// StockResolvTarget is used only by the no-snapshot repair path; empty
	// means DefaultStockResolvTarget.
	StockResolvTarget string
}

func (p Paths) stockTarget() string {
	if p.StockResolvTarget == "" {
		return DefaultStockResolvTarget
	}
	return p.StockResolvTarget
}

// Snapshot layout. Which file exists encodes what was found, so restoring can
// reproduce the original shape rather than guessing at it.
const (
	snapResolvSymlink = "resolv.conf.symlink"
	snapResolvFile    = "resolv.conf.file"
	snapResolvAbsent  = "resolv.conf.absent"
	snapWslFile       = "wsl.conf.file"
	snapWslAbsent     = "wsl.conf.absent"
	// snapComplete is written LAST. Its absence means the snapshot was
	// interrupted partway — a directory alone is not an undo point, and
	// treating it as one lets a later unpin delete resolv.conf and restore
	// nothing.
	snapComplete = "complete"
)

// HasSnapshot reports whether a COMPLETE undo point exists.
//
// Completeness matters more than existence. TakeSnapshot creates the directory
// before writing its records, so an interrupted run (ENOSPC, EACCES on the
// second write, a signal) leaves a directory with nothing in it. Accepting that
// as an undo point is how `pin` ends up writing with no way back, and how
// `unpin` then removes resolv.conf, restores nothing, and reports success.
func HasSnapshot(p Paths) bool {
	_, err := os.Stat(filepath.Join(p.BackupDir, snapComplete))
	return err == nil
}

// TakeSnapshot records the current state, ONCE.
//
// Taking it again on a re-run would capture wlink's own managed files, and
// Restore would then faithfully "restore" the very pin it is meant to remove.
// So an existing snapshot is left strictly alone.
func TakeSnapshot(p Paths) error {
	if HasSnapshot(p) {
		return nil
	}
	if err := os.MkdirAll(p.BackupDir, 0o755); err != nil {
		return fmt.Errorf("creating snapshot dir: %w", err)
	}

	switch fi, err := os.Lstat(p.ResolvConf); {
	case err == nil && fi.Mode()&os.ModeSymlink != 0:
		target, rerr := os.Readlink(p.ResolvConf)
		if rerr != nil {
			return fmt.Errorf("reading resolv.conf symlink: %w", rerr)
		}
		if werr := os.WriteFile(filepath.Join(p.BackupDir, snapResolvSymlink), []byte(target+"\n"), 0o644); werr != nil {
			return werr
		}
	case err == nil:
		content, rerr := os.ReadFile(p.ResolvConf)
		if rerr != nil {
			return rerr
		}
		if werr := os.WriteFile(filepath.Join(p.BackupDir, snapResolvFile), content, 0o644); werr != nil {
			return werr
		}
	case errors.Is(err, os.ErrNotExist):
		if werr := os.WriteFile(filepath.Join(p.BackupDir, snapResolvAbsent), nil, 0o644); werr != nil {
			return werr
		}
	default:
		return err
	}

	switch content, err := os.ReadFile(p.WslConf); {
	case err == nil:
		if werr := os.WriteFile(filepath.Join(p.BackupDir, snapWslFile), content, 0o644); werr != nil {
			return werr
		}
	case errors.Is(err, os.ErrNotExist):
		if werr := os.WriteFile(filepath.Join(p.BackupDir, snapWslAbsent), nil, 0o644); werr != nil {
			return werr
		}
	default:
		return err
	}

	// Marker LAST: everything above must have succeeded for this snapshot to
	// count as an undo point.
	return os.WriteFile(filepath.Join(p.BackupDir, snapComplete), nil, 0o644)
}

// Apply snapshots, then writes both managed files.
//
// The ordering is the safety property: if the snapshot cannot be written,
// NOTHING is written. Proceeding without an undo path is the one outcome that
// could strand a machine with no way back.
func Apply(p Paths, resolvContent string) error {
	if err := TakeSnapshot(p); err != nil {
		return fmt.Errorf("refusing to change DNS without an undo path: %w", err)
	}

	// resolv.conf FIRST, atomically.
	//
	// Order matters: doing wsl.conf first and crashing before the resolver is
	// written would leave the machine with no /etc/resolv.conf AND WSL
	// forbidden from regenerating one — total DNS loss. This way the worst
	// interruption leaves a working resolver that WSL may simply overwrite.
	//
	// Atomic because os.WriteFile is not: a partial write or ENOSPC between
	// removing the symlink and finishing the file would leave no resolver at
	// all. Renaming over the path also replaces the distro-shared symlink in a
	// single step, so there is never a window with nothing there.
	if err := writeFileAtomic(p.ResolvConf, []byte(resolvContent)); err != nil {
		return fmt.Errorf("writing %s: %w", p.ResolvConf, err)
	}

	wsl, err := os.ReadFile(p.WslConf)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := writeFileAtomic(p.WslConf, []byte(SetGenerateResolvConf(string(wsl)))); err != nil {
		return fmt.Errorf("writing %s: %w", p.WslConf, err)
	}
	return nil
}

// writeFileAtomic writes via a temp file in the same directory and renames into
// place, so a reader never sees a partial file and the target is never briefly
// absent. Same directory because rename is only atomic within a filesystem.
func writeFileAtomic(path string, content []byte) (err error) {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".wlink-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()

	// Clean up the temp file on every failure path. Named return so this sees
	// the error the function is actually returning, and the removal error is
	// deliberately discarded: it would mask the real failure, and a stray
	// .wlink-* file is a far smaller problem than the write that just failed.
	defer func() {
		if err != nil {
			_ = tmp.Close() // may already be closed; harmless
			_ = os.Remove(tmpName)
		}
	}()

	if _, err = tmp.Write(content); err != nil {
		return err
	}
	// Sync before rename: without it a crash can leave the renamed file
	// present but empty, which for resolv.conf means a machine with a resolver
	// file and no resolvers in it.
	if err = tmp.Sync(); err != nil {
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	if err = os.Chmod(tmpName, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// RestoreReport says how the machine was returned to its prior state.
type RestoreReport struct {
	// Repaired is true when there was no snapshot and the stock layout was
	// reconstructed instead of restored.
	Repaired bool
	Detail   string
}

// Restore undoes a pin.
//
// With a snapshot it reproduces exactly what was found, symlink target
// included. Without one it still repairs the machine to WSL's stock layout
// (EC-16) rather than leaving it half-managed — a user who lost the backup
// directory still needs a way out.
func Restore(p Paths) (RestoreReport, error) {
	if !HasSnapshot(p) {
		// No snapshot: only repair a file wlink actually wrote.
		//
		// `unpin` is advertised as "undo any time" and doctor suggests it
		// verbatim, so it will be run on machines wlink never touched. Blindly
		// replacing a hand-maintained or systemd-resolved resolv.conf with
		// WSL's stock symlink would destroy configuration wlink has no claim
		// to, with no snapshot to recover from.
		if !IsManaged(readOrEmpty(p.ResolvConf)) {
			return RestoreReport{Detail: fmt.Sprintf(
				"%s was not written by wlink and there is no snapshot; leaving it alone",
				p.ResolvConf)}, nil
		}
		return repairStock(p)
	}

	if err := os.RemoveAll(p.ResolvConf); err != nil {
		return RestoreReport{}, err
	}
	switch {
	case fileExists(filepath.Join(p.BackupDir, snapResolvSymlink)):
		target, err := os.ReadFile(filepath.Join(p.BackupDir, snapResolvSymlink))
		if err != nil {
			return RestoreReport{}, err
		}
		if err := os.Symlink(strings.TrimSpace(string(target)), p.ResolvConf); err != nil {
			return RestoreReport{}, err
		}
	case fileExists(filepath.Join(p.BackupDir, snapResolvFile)):
		content, err := os.ReadFile(filepath.Join(p.BackupDir, snapResolvFile))
		if err != nil {
			return RestoreReport{}, err
		}
		if err := os.WriteFile(p.ResolvConf, content, 0o644); err != nil {
			return RestoreReport{}, err
		}
	}
	// snapResolvAbsent: there was no file before, so leaving it removed is correct.

	switch {
	case fileExists(filepath.Join(p.BackupDir, snapWslFile)):
		content, err := os.ReadFile(filepath.Join(p.BackupDir, snapWslFile))
		if err != nil {
			return RestoreReport{}, err
		}
		if err := os.WriteFile(p.WslConf, content, 0o644); err != nil {
			return RestoreReport{}, err
		}
	case fileExists(filepath.Join(p.BackupDir, snapWslAbsent)):
		if err := os.Remove(p.WslConf); err != nil && !errors.Is(err, os.ErrNotExist) {
			return RestoreReport{}, err
		}
	}

	// Clearing the snapshot matters: a stale one would make the NEXT pin
	// "restore" a state that is no longer the machine's actual prior state.
	if err := os.RemoveAll(p.BackupDir); err != nil {
		return RestoreReport{}, err
	}
	return RestoreReport{Detail: "restored the pre-pin files from the snapshot"}, nil
}

func repairStock(p Paths) (RestoreReport, error) {
	if err := os.RemoveAll(p.ResolvConf); err != nil {
		return RestoreReport{}, err
	}
	if err := os.Symlink(p.stockTarget(), p.ResolvConf); err != nil {
		return RestoreReport{}, fmt.Errorf("recreating the stock resolv.conf symlink: %w", err)
	}
	if content, err := os.ReadFile(p.WslConf); err == nil {
		if werr := os.WriteFile(p.WslConf, []byte(RemoveGenerateResolvConf(string(content))), 0o644); werr != nil {
			return RestoreReport{}, werr
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return RestoreReport{}, err
	}
	return RestoreReport{
		Repaired: true,
		Detail:   "no snapshot found; restored WSL's stock layout instead",
	}, nil
}

// Drift is a managed file changed since wlink wrote it.
type Drift struct {
	File   string
	Detail string
}

// DetectDrift reports a hand-edit to a file wlink is managing.
//
// An UNMANAGED machine is not drift — it is simply one wlink has not pinned.
// Only a file that lost the managed marker after we wrote it counts, which is
// why the marker is part of the rendered output.
func DetectDrift(p Paths) (*Drift, error) {
	if !HasSnapshot(p) {
		return nil, nil // nothing pinned: nothing to drift from
	}
	content, err := os.ReadFile(p.ResolvConf)
	if errors.Is(err, os.ErrNotExist) {
		return &Drift{File: p.ResolvConf, Detail: "the managed resolv.conf was deleted"}, nil
	}
	if err != nil {
		return nil, err
	}
	if !IsManaged(string(content)) {
		return &Drift{
			File:   p.ResolvConf,
			Detail: "edited outside wlink — the managed marker is gone, so the pin is no longer in force",
		}, nil
	}
	return nil, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func readOrEmpty(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

// OriginalNameservers returns the resolvers that were in force BEFORE wlink
// first pinned — from the snapshot when one exists, else from the live file.
//
// Seeding fallbacks from the live file is wrong once wlink has pinned: it would
// re-seed from its own output, so every re-pin demotes the previous winner and
// eventually evicts the WSL NAT proxy, which is the only fallback guaranteed to
// answer.
func OriginalNameservers(p Paths) []string {
	if HasSnapshot(p) {
		if content, err := os.ReadFile(filepath.Join(p.BackupDir, snapResolvFile)); err == nil {
			return Nameservers(string(content))
		}
		// A symlink snapshot: read through to whatever it pointed at.
		if target, err := os.ReadFile(filepath.Join(p.BackupDir, snapResolvSymlink)); err == nil {
			if content, rerr := os.ReadFile(strings.TrimSpace(string(target))); rerr == nil {
				return Nameservers(string(content))
			}
		}
		return nil
	}
	return Nameservers(readOrEmpty(p.ResolvConf))
}

// OriginalContent returns the pre-pin resolv.conf text — from the snapshot when
// one exists, else the live file. Used to carry across directives wlink does
// not own, without re-reading its own output on a re-pin.
func OriginalContent(p Paths) string {
	if HasSnapshot(p) {
		if content, err := os.ReadFile(filepath.Join(p.BackupDir, snapResolvFile)); err == nil {
			return string(content)
		}
		if target, err := os.ReadFile(filepath.Join(p.BackupDir, snapResolvSymlink)); err == nil {
			if content, rerr := os.ReadFile(strings.TrimSpace(string(target))); rerr == nil {
				return string(content)
			}
		}
		return ""
	}
	return readOrEmpty(p.ResolvConf)
}
