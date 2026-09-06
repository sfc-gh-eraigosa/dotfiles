package cmd

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	jwt "github.com/golang-jwt/jwt/v5"
)

// leakCanary is the only token value the stub ever mints; every cmd test
// asserts it never appears in output that is not the bare `token` verb.
var leakCanary = strings.Join([]string{"ghs", "FIXTURE_TOKEN_DO_NOT_PRINT"}, "_")

// world is a fake GitHub (API stub) + a fake config dir for one test.
type world struct {
	t        *testing.T
	srv      *httptest.Server
	key      *rsa.PrivateKey
	dir      string
	lastBody map[string]any
	tokens   int
	repoHits int
	badJWT   bool
}

func newWorld(t *testing.T) *world {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	w := &world{t: t, key: key, dir: filepath.Join(t.TempDir(), "ghapp")}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	mux := http.NewServeMux()
	requireJWT := func(rw http.ResponseWriter, r *http.Request) bool {
		raw := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		claims := jwt.MapClaims{}
		_, err := jwt.ParseWithClaims(raw, claims, func(*jwt.Token) (any, error) { return &key.PublicKey, nil })
		if err != nil || w.badJWT {
			rw.WriteHeader(401)
			fmt.Fprint(rw, `{"message":"Bad credentials"}`)
			return false
		}
		return true
	}
	mux.HandleFunc("POST /app-manifests/{code}/conversions", func(rw http.ResponseWriter, r *http.Request) {
		if r.PathValue("code") != "good-code" {
			rw.WriteHeader(404)
			fmt.Fprint(rw, `{"message":"Not Found"}`)
			return
		}
		rw.WriteHeader(201)
		_ = json.NewEncoder(rw).Encode(map[string]any{"id": 4242, "slug": "gcfg-test", "name": "gcfg test", "pem": string(pemBytes), "html_url": "https://github.com/apps/gcfg-test"})
	})
	mux.HandleFunc("GET /app", func(rw http.ResponseWriter, r *http.Request) {
		if !requireJWT(rw, r) {
			return
		}
		fmt.Fprint(rw, `{"id":4242,"slug":"gcfg-test","name":"gcfg test","html_url":"https://github.com/apps/gcfg-test"}`)
	})
	mux.HandleFunc("GET /app/installations", func(rw http.ResponseWriter, r *http.Request) {
		if !requireJWT(rw, r) {
			return
		}
		fmt.Fprint(rw, `[{"id":11,"account":{"login":"sfc-gh-eraigosa"},"target_type":"User","repository_selection":"all"},{"id":22,"account":{"login":"other-org"},"target_type":"Organization","repository_selection":"selected"}]`)
	})
	mux.HandleFunc("POST /app/installations/{id}/access_tokens", func(rw http.ResponseWriter, r *http.Request) {
		if !requireJWT(rw, r) {
			return
		}
		w.tokens++
		b, _ := io.ReadAll(r.Body)
		w.lastBody = nil
		if len(b) > 0 {
			_ = json.Unmarshal(b, &w.lastBody)
		}
		w.lastBody = mergeInst(w.lastBody, r.PathValue("id"))
		rw.WriteHeader(201)
		fmt.Fprintf(rw, `{"token":%q,"expires_at":"2026-09-05T13:00:00Z","permissions":{"administration":"write"},"repository_selection":"selected"}`, leakCanary)
	})
	mux.HandleFunc("GET /repos/{owner}/{repo}", func(rw http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+leakCanary {
			rw.WriteHeader(401)
			fmt.Fprint(rw, `{"message":"Bad credentials"}`)
			return
		}
		w.repoHits++
		fmt.Fprintf(rw, `{"full_name":"%s/%s","permissions":{"admin":true,"push":true,"pull":true}}`, r.PathValue("owner"), r.PathValue("repo"))
	})
	w.srv = httptest.NewServer(mux)
	t.Cleanup(w.srv.Close)
	return w
}

func mergeInst(body map[string]any, inst string) map[string]any {
	if body == nil {
		body = map[string]any{}
	}
	body["_installation"] = inst
	return body
}

// run executes the CLI against this world and returns stdout, stderr, err.
func (w *world) run(args ...string) (string, string, error) {
	var out, errb bytes.Buffer
	root := NewRootCmd()
	root.SetOut(&out)
	root.SetErr(&errb)
	root.SetArgs(append([]string{"--config-dir", w.dir, "--api-url", w.srv.URL, "--web-url", "https://web.example"}, args...))
	err := root.Execute()
	return out.String(), errb.String(), err
}

// create drives the manifest flow to completion with a fake browser.
func (w *world) create(t *testing.T, extra ...string) (string, string, error) {
	t.Helper()
	old := openBrowser
	openBrowser = fakeBrowser(t, "good-code")
	t.Cleanup(func() { openBrowser = old })
	return w.run(append([]string{"create", "--name", "gcfg test"}, extra...)...)
}

func fakeBrowser(t *testing.T, code string) func(string) error {
	return func(u string) error {
		go func() {
			res, err := http.Get(u)
			if err != nil {
				return
			}
			b, _ := io.ReadAll(res.Body)
			res.Body.Close()
			page := string(b)
			i := strings.Index(page, "redirect_url")
			if i < 0 {
				return
			}
			// value='{"...","redirect_url":"http://127.0.0.1:PORT/callback",...}'
			rest := page[i:]
			rest = rest[strings.Index(rest, "http://"):]
			redirect := rest[:strings.Index(rest, `"`)]
			si := strings.Index(page, "state=")
			state := page[si+len("state="):]
			state = state[:strings.IndexAny(state, `"&`)]
			r2, err := http.Get(redirect + "?code=" + code + "&state=" + state)
			if err == nil {
				r2.Body.Close()
			}
		}()
		return nil
	}
}

func assertNoLeak(t *testing.T, name, s string) {
	t.Helper()
	if strings.Contains(s, leakCanary) {
		t.Fatalf("%s leaks the token value", name)
	}
	if strings.Contains(s, "PRIVATE KEY") {
		t.Fatalf("%s leaks PEM material", name)
	}
}
