package ghapp

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
)

func TestParsePrivateKeyPKCS1AndPKCS8(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pkcs1 := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	der8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	pkcs8 := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der8})
	for name, b := range map[string][]byte{"pkcs1": pkcs1, "pkcs8": pkcs8} {
		got, err := ParsePrivateKey(b)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got.N.Cmp(key.N) != 0 {
			t.Fatalf("%s: parsed a different key", name)
		}
	}
	if _, err := ParsePrivateKey([]byte("not a pem")); err == nil {
		t.Fatal("want error for garbage PEM")
	}
}

func TestSignJWTClaimsAndSignature(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	tok, err := SignJWT(12345, key, now)
	if err != nil {
		t.Fatal(err)
	}
	claims := jwt.MapClaims{}
	parsed, err := jwt.ParseWithClaims(tok, claims, func(tk *jwt.Token) (any, error) {
		if tk.Method.Alg() != "RS256" {
			t.Fatalf("alg = %s, want RS256", tk.Method.Alg())
		}
		return &key.PublicKey, nil
	}, jwt.WithTimeFunc(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	if !parsed.Valid {
		t.Fatal("token not valid")
	}
	if claims["iss"] != "12345" {
		t.Errorf("iss = %v, want \"12345\"", claims["iss"])
	}
	if got := int64(claims["iat"].(float64)); got != now.Add(-60*time.Second).Unix() {
		t.Errorf("iat = %d, want now-60s (%d)", got, now.Add(-60*time.Second).Unix())
	}
	if got := int64(claims["exp"].(float64)); got != now.Add(9*time.Minute).Unix() {
		t.Errorf("exp = %d, want now+9m (%d)", got, now.Add(9*time.Minute).Unix())
	}
}

func TestSignJWTRejectsWrongKey(t *testing.T) {
	k1, _ := rsa.GenerateKey(rand.Reader, 2048)
	k2, _ := rsa.GenerateKey(rand.Reader, 2048)
	tok, err := SignJWT(1, k1, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	_, err = jwt.Parse(tok, func(*jwt.Token) (any, error) { return &k2.PublicKey, nil })
	if err == nil {
		t.Fatal("token verified with the wrong public key")
	}
}

func TestParsePrivateKeyRejectsNonRSA(t *testing.T) {
	ec, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(ec)
	if err != nil {
		t.Fatal(err)
	}
	b := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	if _, err := ParsePrivateKey(b); err == nil || !strings.Contains(err.Error(), "not RSA") {
		t.Fatalf("want 'not RSA' error, got %v", err)
	}
	if _, err := ParsePrivateKey(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte{1, 2, 3}})); err == nil {
		t.Fatal("want error for undecodable DER")
	}
}
