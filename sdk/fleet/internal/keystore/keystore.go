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
// <file>.bak-<now> (0600, fsync'd, never overwriting: a taken name gets a -1,
// -2, ... suffix); the private key's backup path is returned ("" when nothing
// existed).
//
// Partial state: the private key is replaced before the .pub. If a step after
// the private key's backup fails, the backup path is still returned together
// with the error, so the old key is never lost; when the error says the .pub
// failed, the new private key is already in place and the .pub is stale.
func Write(path string, private, public []byte, now time.Time) (backup string, err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	suffix := ".bak-" + now.Format(tsLayout)
	pubPath := path + ".pub"
	if b, rerr := os.ReadFile(path); rerr == nil {
		if backup, err = writeBackup(path+suffix, b); err != nil {
			return "", err
		}
	} else if !os.IsNotExist(rerr) {
		return "", rerr
	}
	if b, rerr := os.ReadFile(pubPath); rerr == nil {
		if _, err := writeBackup(pubPath+suffix, b); err != nil {
			return backup, fmt.Errorf("keystore: backing up .pub (private key not yet replaced): %w", err)
		}
	} else if !os.IsNotExist(rerr) {
		return backup, fmt.Errorf("keystore: reading existing .pub (private key not yet replaced): %w", rerr)
	}
	if err := atomicWrite(path, private, 0o600); err != nil {
		return backup, err
	}
	if err := atomicWrite(pubPath, public, 0o644); err != nil {
		return backup, fmt.Errorf("keystore: private key written but .pub failed: %w", err)
	}
	return backup, nil
}

// writeBackup creates name exclusively (0600), trying name-1, name-2, ... when
// taken, writes data and fsyncs. O_EXCL never follows a planted symlink.
func writeBackup(name string, data []byte) (string, error) {
	for i := 0; i < 1000; i++ {
		p := name
		if i > 0 {
			p = fmt.Sprintf("%s-%d", name, i)
		}
		f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		_, werr := f.Write(data)
		if werr == nil {
			werr = f.Sync()
		}
		if cerr := f.Close(); werr == nil {
			werr = cerr
		}
		if werr != nil {
			return "", werr
		}
		return p, nil
	}
	return "", fmt.Errorf("keystore: no free backup name for %s", name)
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

// Encrypt re-wraps an UNENCRYPTED private key (any format ssh.ParseRawPrivateKey
// reads) as an OpenSSH PEM protected by passphrase. An empty passphrase, an
// unparseable key, or an already-encrypted key is refused. Pure: nothing is
// written; the caller decides where the bytes go.
func Encrypt(rawPrivate, passphrase []byte, comment string) ([]byte, error) {
	if len(passphrase) == 0 {
		return nil, errors.New("keystore: empty passphrase refused")
	}
	k, err := ssh.ParseRawPrivateKey(rawPrivate)
	if err != nil {
		return nil, fmt.Errorf("keystore: parse private key: %w", err)
	}
	blk, err := ssh.MarshalPrivateKeyWithPassphrase(k, comment, passphrase)
	if err != nil {
		return nil, fmt.Errorf("keystore: marshal private key: %w", err)
	}
	return pem.EncodeToMemory(blk), nil
}

// Backup copies data to <path>.bak-<YYYYmmddTHHMMSS> exclusively (0600, fsync),
// trying -1, -2, ... when the name is taken, and returns the name used.
func Backup(path string, data []byte, now time.Time) (string, error) {
	return writeBackup(path+".bak-"+now.Format(tsLayout), data)
}

// AtomicWrite replaces path with data via a temp file in the same directory
// (fsync, then rename) at the given mode.
func AtomicWrite(path string, data []byte, mode os.FileMode) error {
	return atomicWrite(path, data, mode)
}
