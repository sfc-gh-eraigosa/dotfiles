package ghapp

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
)

func pemFor(k *rsa.PrivateKey) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k)})
}
