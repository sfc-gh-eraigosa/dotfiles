// Package paths defines the well-known filesystem locations used by gff.
package paths

import (
	"fmt"
	"os"
	"path/filepath"
)

// Paths holds all well-known filesystem locations used by gff.
// SystemSnapshotDir and SystemOverride are read-only from gff's perspective;
// gff only writes to UserSnapshotDir and UserOverride.
type Paths struct {
	// SystemSnapshotDir is the admin-provisioned snapshot directory.
	// Default: /opt/conf/gff
	SystemSnapshotDir string

	// UserSnapshotDir is the per-user snapshot directory.
	// Default: ${XDG_DATA_HOME:-$HOME/.local/share}/gff/snapshots
	UserSnapshotDir string

	// SystemOverride is the system-wide override config file.
	// Default: /var/opt/conf/gff/config.yaml
	SystemOverride string

	// UserOverride is the per-user override config file.
	// Default: $HOME/.config/gff/config.yaml
	UserOverride string

	// RegistryFile is the per-user source registry file.
	// Default: $HOME/.config/gff/sources.yaml
	RegistryFile string

	// WorkDir is the current working directory used for repo discovery.
	// Default: os.Getwd()
	WorkDir string
}

// Default returns a Paths populated with the well-known default locations.
// It honors XDG_DATA_HOME when set; otherwise falls back to $HOME/.local/share.
func Default() (Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, fmt.Errorf("paths: resolving home directory: %w", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return Paths{}, fmt.Errorf("paths: resolving working directory: %w", err)
	}

	xdgData := os.Getenv("XDG_DATA_HOME")
	if xdgData == "" {
		xdgData = filepath.Join(home, ".local", "share")
	}

	return Paths{
		SystemSnapshotDir: "/opt/conf/gff",
		UserSnapshotDir:   filepath.Join(xdgData, "gff", "snapshots"),
		SystemOverride:    "/var/opt/conf/gff/config.yaml",
		UserOverride:      filepath.Join(home, ".config", "gff", "config.yaml"),
		RegistryFile:      filepath.Join(home, ".config", "gff", "sources.yaml"),
		WorkDir:           cwd,
	}, nil
}
