package ghapp

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

const (
	dirPerm  = 0o700
	filePerm = 0o600
	appsFile = "apps.json"
)

var slugRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// DefaultDir is ~/.config/ghapp (honouring $XDG_CONFIG_HOME).
func DefaultDir() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "ghapp")
	}
	return filepath.Join(os.Getenv("HOME"), ".config", "ghapp")
}

// FileStore keeps apps.json and one <slug>.pem per App under Dir
// (directory 0700, files 0600). Load refuses a PEM that is readable by
// anyone but the owner.
type FileStore struct {
	Dir string
}

var _ Store = FileStore{}

func (s FileStore) ensureDir() error {
	if err := os.MkdirAll(s.Dir, dirPerm); err != nil {
		return fmt.Errorf("ghapp store: %w", err)
	}
	// MkdirAll honours the umask; force the mode we promised.
	return os.Chmod(s.Dir, dirPerm)
}

// SavePEM writes the private key for slug and returns its path.
func (s FileStore) SavePEM(slug string, pemBytes []byte) (string, error) {
	if !slugRE.MatchString(slug) {
		return "", fmt.Errorf("ghapp store: invalid app slug %q", slug)
	}
	if err := s.ensureDir(); err != nil {
		return "", err
	}
	p := filepath.Join(s.Dir, slug+".pem")
	if err := writePrivate(p, pemBytes); err != nil {
		return "", err
	}
	return p, nil
}

// Save writes apps.json.
func (s FileStore) Save(apps Apps) error {
	if err := s.ensureDir(); err != nil {
		return err
	}
	b, err := json.MarshalIndent(apps, "", "  ")
	if err != nil {
		return fmt.Errorf("ghapp store: %w", err)
	}
	return writePrivate(filepath.Join(s.Dir, appsFile), append(b, '\n'))
}

// Load reads apps.json; a missing store is empty, not an error. Every
// referenced PEM must exist with mode 0600.
func (s FileStore) Load() (Apps, error) {
	b, err := os.ReadFile(filepath.Join(s.Dir, appsFile))
	if errors.Is(err, os.ErrNotExist) {
		return Apps{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("ghapp store: %w", err)
	}
	apps := Apps{}
	if err := json.Unmarshal(b, &apps); err != nil {
		return nil, fmt.Errorf("ghapp store: parsing %s: %w", appsFile, err)
	}
	for slug, a := range apps {
		if err := checkPrivate(a.PEMPath); err != nil {
			return nil, fmt.Errorf("ghapp store: app %q: %w", slug, err)
		}
	}
	return apps, nil
}

func writePrivate(path string, b []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, filePerm)
	if err != nil {
		return fmt.Errorf("ghapp store: %w", err)
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		return fmt.Errorf("ghapp store: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("ghapp store: %w", err)
	}
	return os.Chmod(path, filePerm)
}

func checkPrivate(path string) error {
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	if mode := st.Mode().Perm(); mode&0o077 != 0 {
		return fmt.Errorf("%s is mode %04o; must be 0600 (chmod 600 it)", filepath.Base(path), mode)
	}
	return nil
}
