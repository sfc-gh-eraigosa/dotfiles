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
)

// HasSnapshot reports whether an undo point exists.
func HasSnapshot(p Paths) bool {
	st, err := os.Stat(p.BackupDir)
	return err == nil && st.IsDir()
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
	return nil
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

	wsl, err := os.ReadFile(p.WslConf)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.WriteFile(p.WslConf, []byte(SetGenerateResolvConf(string(wsl))), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", p.WslConf, err)
	}

	// Replace the symlink rather than writing through it: WSL's stock
	// resolv.conf points into /mnt/wsl, which is shared across distros, so a
	// write would leak this machine's pin to all of them — and be regenerated.
	if fi, lerr := os.Lstat(p.ResolvConf); lerr == nil && fi.Mode()&os.ModeSymlink != 0 {
		if rerr := os.Remove(p.ResolvConf); rerr != nil {
			return fmt.Errorf("removing the shared resolv.conf symlink: %w", rerr)
		}
	}
	if err := os.WriteFile(p.ResolvConf, []byte(resolvContent), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", p.ResolvConf, err)
	}
	return nil
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
