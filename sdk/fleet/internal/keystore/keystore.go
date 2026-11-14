// Package keystore generates, writes and inspects passphrase-protected ed25519
// SSH keys entirely in-process, so the passphrase never reaches argv.
package keystore

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

const tsLayout = "20060102T150405"

// Generate makes a new ed25519 key whose private half is encrypted with
// passphrase (OpenSSH format). The public key reads "ssh-ed25519 AAAA… comment\n".
// An empty passphrase is refused: fleet never writes an unencrypted key.
func Generate(passphrase []byte, comment string) (private, public []byte, err error) {
	if len(passphrase) == 0 {
		return nil, nil, errors.New("keystore: empty passphrase refused")
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("keystore: generate: %w", err)
	}
	blk, err := ssh.MarshalPrivateKeyWithPassphrase(priv, comment, passphrase)
	if err != nil {
		return nil, nil, fmt.Errorf("keystore: marshal private key: %w", err)
	}
	sp, err := ssh.NewPublicKey(pub)
	if err != nil {
		return nil, nil, fmt.Errorf("keystore: public key: %w", err)
	}
	line := strings.TrimRight(string(ssh.MarshalAuthorizedKey(sp)), "\n")
	if comment != "" {
		line += " " + comment
	}
	return pem.EncodeToMemory(blk), []byte(line + "\n"), nil
}

// Write stores the key pair at path (0600) and path+".pub" (0644), creating
// the parent directory 0700 if missing. Existing files are first copied to
// <file>.bak-<now> (0600); the private key's backup path is returned ("" when
// nothing existed).
func Write(path string, private, public []byte, now time.Time) (backup string, err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	suffix := ".bak-" + now.Format(tsLayout)
	pubPath := path + ".pub"
	if b, rerr := os.ReadFile(path); rerr == nil {
		backup = path + suffix
		if err := os.WriteFile(backup, b, 0o600); err != nil {
			return "", err
		}
		if err := os.Chmod(backup, 0o600); err != nil {
			return "", err
		}
	} else if !os.IsNotExist(rerr) {
		return "", rerr
	}
	if b, rerr := os.ReadFile(pubPath); rerr == nil {
		if err := os.WriteFile(pubPath+suffix, b, 0o600); err != nil {
			return "", err
		}
		if err := os.Chmod(pubPath+suffix, 0o600); err != nil {
			return "", err
		}
	} else if !os.IsNotExist(rerr) {
		return "", rerr
	}
	if err := atomicWrite(path, private, 0o600); err != nil {
		return "", err
	}
	if err := atomicWrite(pubPath, public, 0o644); err != nil {
		return "", err
	}
	return backup, nil
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(tmp)
		}
	}()
	if err := f.Chmod(mode); err != nil {
		_ = f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	ok = true
	return nil
}

// Encrypted reports whether the private key at path needs a passphrase.
func Encrypted(path string) (bool, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	_, err = ssh.ParseRawPrivateKey(b)
	var missing *ssh.PassphraseMissingError
	switch {
	case err == nil:
		return false, nil
	case errors.As(err, &missing):
		return true, nil
	default:
		return false, err
	}
}

// Fingerprint returns the "SHA256:…" fingerprint of an authorized_keys line.
func Fingerprint(public []byte) (string, error) {
	k, _, _, _, err := ssh.ParseAuthorizedKey(public)
	if err != nil {
		return "", err
	}
	return ssh.FingerprintSHA256(k), nil
}
