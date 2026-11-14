package keystore

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"testing"

	"golang.org/x/crypto/ssh"
)

// plainTestKey returns an unencrypted OpenSSH key generated at test time.
func plainTestKey(t *testing.T) []byte {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	blk, err := ssh.MarshalPrivateKey(priv, "plain")
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(blk)
}
