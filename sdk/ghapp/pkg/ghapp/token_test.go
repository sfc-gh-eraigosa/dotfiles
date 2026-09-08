package ghapp

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
)

// leakCanary is the only token value tests ever see; ghapp-ci greps the
// verbose test log for it and fails on a hit. Assembled at runtime so no
// source line looks like a credential assignment.
var leakCanary = strings.Join([]string{"ghs", "FIXTURE_TOKEN_DO_NOT_PRINT"}, "_")

type stub struct {
	srv          *httptest.Server
	key          *rsa.PrivateKey
	now          time.Time
	tokenCalls   atomic.Int32
	installCalls atomic.Int32
	lastBody     map[string]any
	lastAuth     string
	failTokens   int // HTTP status to return from access_tokens when != 0
}

func newStub(t *testing.T) *stub {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	s := &stub{key: key, now: time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)}
	mux := http.NewServeMux()
	// Every App endpoint must carry a JWT signed by our key with iss=777.
	requireJWT := func(w http.ResponseWriter, r *http.Request) bool {
		s.lastAuth = r.Header.Get("Authorization")
		raw := strings.TrimPrefix(s.lastAuth, "Bearer ")
		claims := jwt.MapClaims{}
		_, err := jwt.ParseWithClaims(raw, claims, func(*jwt.Token) (any, error) { return &key.PublicKey, nil },
			jwt.WithTimeFunc(func() time.Time { return s.now }))
		if err != nil || claims["iss"] != "777" {
			w.WriteHeader(401)
			fmt.Fprint(w, `{"message":"Bad credentials"}`)
			return false
		}
		return true
	}
	mux.HandleFunc("GET /app", func(w http.ResponseWriter, r *http.Request) {
		if !requireJWT(w, r) {
			return
		}
		fmt.Fprint(w, `{"id":777,"slug":"test-app","name":"Test App","html_url":"https://github.com/apps/test-app"}`)
	})
	mux.HandleFunc("GET /app/installations", func(w http.ResponseWriter, r *http.Request) {
		s.installCalls.Add(1)
		if !requireJWT(w, r) {
			return
		}
		// Two pages: page 1 links to page 2.
		if r.URL.Query().Get("page") == "2" {
			fmt.Fprint(w, `[{"id":22,"account":{"login":"other-org"},"target_type":"Organization","repository_selection":"selected"}]`)
			return
		}
		w.Header().Set("Link", fmt.Sprintf(`<%s/app/installations?per_page=100&page=2>; rel="next"`, s.srv.URL))
		fmt.Fprint(w, `[{"id":11,"account":{"login":"sfc-gh-eraigosa"},"target_type":"User","repository_selection":"all"}]`)
	})
	mux.HandleFunc("POST /app/installations/{id}/access_tokens", func(w http.ResponseWriter, r *http.Request) {
		s.tokenCalls.Add(1)
		if !requireJWT(w, r) {
			return
		}
		if s.failTokens != 0 {
			w.WriteHeader(s.failTokens)
			fmt.Fprint(w, `{"message":"boom"}`)
			return
		}
		b, _ := io.ReadAll(r.Body)
		s.lastBody = nil
		if len(b) > 0 {
			_ = json.Unmarshal(b, &s.lastBody)
		}
		w.WriteHeader(201)
		fmt.Fprintf(w, `{"token":%q,"expires_at":%q,"permissions":{"administration":"write"},"repository_selection":"selected"}`,
			leakCanary, s.now.Add(time.Hour).Format(time.RFC3339))
	})
	s.srv = httptest.NewServer(mux)
	t.Cleanup(s.srv.Close)
	return s
}

func (s *stub) app() App {
	return App{ID: 777, Slug: "test-app"}.With(Options{
		Key: s.key, HTTP: s.srv.Client(), BaseURL: s.srv.URL, Now: func() time.Time { return s.now },
	})
}

func TestInstallationsPaginatesAndUsesAppJWT(t *testing.T) {
	s := newStub(t)
	got, err := s.app().Installations(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != 11 || got[0].Account != "sfc-gh-eraigosa" || got[0].TargetType != "User" ||
		got[1].ID != 22 || got[1].Account != "other-org" || got[1].RepositorySelection != "selected" {
		t.Fatalf("installations = %+v", got)
	}
	if s.installCalls.Load() != 2 {
		t.Fatalf("want 2 page fetches, got %d", s.installCalls.Load())
	}
	if !strings.HasPrefix(s.lastAuth, "Bearer ") {
		t.Fatalf("want Bearer JWT, got %q", s.lastAuth)
	}
}

func TestTokenScopesBodyAndReturnsExpiry(t *testing.T) {
	s := newStub(t)
	tok, err := s.app().Token(context.Background(), 11, TokenScope{
		Permissions:  map[string]string{"administration": "write", "contents": "read"},
		Repositories: []string{"dotfiles"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if tok.Value != leakCanary {
		t.Fatal("token value mismatch")
	}
	if !tok.ExpiresAt.Equal(s.now.Add(time.Hour)) {
		t.Fatalf("expires_at = %v", tok.ExpiresAt)
	}
	perms, _ := s.lastBody["permissions"].(map[string]any)
	if perms["administration"] != "write" || perms["contents"] != "read" {
		t.Fatalf("permissions body = %v", s.lastBody)
	}
	repos, _ := s.lastBody["repositories"].([]any)
	if len(repos) != 1 || repos[0] != "dotfiles" {
		t.Fatalf("repositories body = %v", s.lastBody)
	}
}

func TestTokenEmptyScopeSendsNoBody(t *testing.T) {
	s := newStub(t)
	if _, err := s.app().Token(context.Background(), 11, TokenScope{}); err != nil {
		t.Fatal(err)
	}
	if s.lastBody != nil {
		t.Fatalf("want empty body for an unscoped token, got %v", s.lastBody)
	}
}

func TestTokenCacheHitUntilTwoMinutesBeforeExpiry(t *testing.T) {
	s := newStub(t)
	app := s.app()
	scope := TokenScope{Permissions: map[string]string{"administration": "read"}}
	if _, err := app.Token(context.Background(), 11, scope); err != nil {
		t.Fatal(err)
	}
	// 57m later: still ≥2m before the 60m expiry → cache hit.
	s.now = s.now.Add(57 * time.Minute)
	if _, err := app.Token(context.Background(), 11, scope); err != nil {
		t.Fatal(err)
	}
	if s.tokenCalls.Load() != 1 {
		t.Fatalf("want 1 mint (cache hit), got %d", s.tokenCalls.Load())
	}
	// 58m30s: inside the 2m guard → miss, re-mint.
	s.now = s.now.Add(90 * time.Second)
	if _, err := app.Token(context.Background(), 11, scope); err != nil {
		t.Fatal(err)
	}
	if s.tokenCalls.Load() != 2 {
		t.Fatalf("want 2 mints after the guard, got %d", s.tokenCalls.Load())
	}
	// Different scope or installation → its own cache entry.
	if _, err := app.Token(context.Background(), 11, TokenScope{Repositories: []string{"x"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Token(context.Background(), 22, scope); err != nil {
		t.Fatal(err)
	}
	if s.tokenCalls.Load() != 4 {
		t.Fatalf("want 4 mints across scopes/installs, got %d", s.tokenCalls.Load())
	}
}

func TestTokenNeverPrintsItself(t *testing.T) {
	tok := Token{Value: leakCanary, ExpiresAt: time.Now()}
	for name, s := range map[string]string{
		"String":  tok.String(),
		"%v":      fmt.Sprintf("%v", tok),
		"%+v":     fmt.Sprintf("%+v", tok),
		"%s":      fmt.Sprintf("%s", tok),
		"pointer": fmt.Sprintf("%v", &tok),
	} {
		if strings.Contains(s, leakCanary) {
			t.Errorf("%s leaks the token value", name)
		}
		if !strings.Contains(s, "ghs_***") {
			t.Errorf("%s should show the redacted marker, got %q", name, s)
		}
	}
	b, err := json.Marshal(tok)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), leakCanary) {
		t.Error("json.Marshal leaks the token value")
	}
}

func TestTokenErrorsCarryStatusNotBody(t *testing.T) {
	s := newStub(t)
	s.failTokens = 403
	_, err := s.app().Token(context.Background(), 11, TokenScope{})
	if err == nil || !strings.Contains(err.Error(), "403") || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("want status + GitHub message in error, got %v", err)
	}
}

func TestBadJWTIsAnAuthError(t *testing.T) {
	s := newStub(t)
	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	app := App{ID: 777}.With(Options{Key: other, HTTP: s.srv.Client(), BaseURL: s.srv.URL, Now: func() time.Time { return s.now }})
	if _, err := app.Installations(context.Background()); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("want 401 error, got %v", err)
	}
}

func TestAppLoadsKeyFromPEMPathLazily(t *testing.T) {
	s := newStub(t)
	store := FileStore{Dir: t.TempDir() + "/ghapp"}
	pemPath, err := store.SavePEM("test-app", pemFor(s.key))
	if err != nil {
		t.Fatal(err)
	}
	app := App{ID: 777, Slug: "test-app", PEMPath: pemPath}.With(Options{HTTP: s.srv.Client(), BaseURL: s.srv.URL, Now: func() time.Time { return s.now }})
	if _, err := app.Installations(context.Background()); err != nil {
		t.Fatal(err)
	}
	bad := App{ID: 777, PEMPath: pemPath + ".missing"}.With(Options{HTTP: s.srv.Client(), BaseURL: s.srv.URL})
	if _, err := bad.Installations(context.Background()); err == nil {
		t.Fatal("want error for a missing PEM")
	}
}

func TestDefaultsHitAPIGitHub(t *testing.T) {
	app := App{ID: 1}.With(Options{})
	if app.deps.baseURL != "https://api.github.com" || app.deps.http == nil || app.deps.now == nil {
		t.Fatalf("defaults not applied: %+v", app.deps)
	}
}
