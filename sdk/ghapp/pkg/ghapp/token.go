package ghapp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

// TokenScope narrows an installation token (GitHub's access_tokens body).
// Empty means "everything the installation grants".
type TokenScope struct {
	Permissions  map[string]string `json:"permissions,omitempty"`
	Repositories []string          `json:"repositories,omitempty"`
}

// Token is a minted installation token. Its value is deliberately hidden
// from String/%v/JSON: read .Value on purpose, never print it.
type Token struct {
	Value               string
	ExpiresAt           time.Time
	Permissions         map[string]string
	RepositorySelection string
}

// String redacts the value.
func (t Token) String() string {
	return fmt.Sprintf("ghs_*** (expires %s)", t.ExpiresAt.UTC().Format(time.RFC3339))
}

// Format routes every fmt verb through String so %v/%+v/%s cannot leak.
func (t Token) Format(f fmt.State, verb rune) { io.WriteString(f, t.String()) }

// MarshalJSON emits metadata only.
func (t Token) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Token               string            `json:"token"`
		ExpiresAt           time.Time         `json:"expires_at"`
		Permissions         map[string]string `json:"permissions,omitempty"`
		RepositorySelection string            `json:"repository_selection,omitempty"`
	}{"ghs_***", t.ExpiresAt, t.Permissions, t.RepositorySelection})
}

// cacheGuard is how long before expiry a cached token stops being reused.
const cacheGuard = 2 * time.Minute

// Token mints (or returns the cached) installation token for inst, scoped
// by scope. Cached until expiry−2m.
func (a App) Token(ctx context.Context, inst int64, scope TokenScope) (Token, error) {
	d, err := a.ready()
	if err != nil {
		return Token{}, err
	}
	key := cacheKey(inst, scope)
	d.mu.Lock()
	if t, ok := d.cache[key]; ok && d.now().Before(t.ExpiresAt.Add(-cacheGuard)) {
		d.mu.Unlock()
		return t, nil
	}
	d.mu.Unlock()

	var body io.Reader
	if len(scope.Permissions) > 0 || len(scope.Repositories) > 0 {
		b, err := json.Marshal(scope)
		if err != nil {
			return Token{}, fmt.Errorf("ghapp: encoding token scope: %w", err)
		}
		body = bytes.NewReader(b)
	}
	var resp struct {
		Token               string            `json:"token"`
		ExpiresAt           time.Time         `json:"expires_at"`
		Permissions         map[string]string `json:"permissions"`
		RepositorySelection string            `json:"repository_selection"`
	}
	if _, err := a.appCall(ctx, http.MethodPost, fmt.Sprintf("/app/installations/%d/access_tokens", inst), body, &resp); err != nil {
		return Token{}, err
	}
	t := Token{Value: resp.Token, ExpiresAt: resp.ExpiresAt, Permissions: resp.Permissions, RepositorySelection: resp.RepositorySelection}
	d.mu.Lock()
	d.cache[key] = t
	d.mu.Unlock()
	return t, nil
}

func cacheKey(inst int64, s TokenScope) string {
	perms := make([]string, 0, len(s.Permissions))
	for k, v := range s.Permissions {
		perms = append(perms, k+"="+v)
	}
	sort.Strings(perms)
	repos := append([]string(nil), s.Repositories...)
	sort.Strings(repos)
	return fmt.Sprintf("%d|%s|%s", inst, strings.Join(perms, ","), strings.Join(repos, ","))
}

// ready returns the deps, loading the private key from PEMPath on first use.
func (a App) ready() (*deps, error) {
	if a.deps == nil {
		a = a.With(Options{})
	}
	d := a.deps
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.key == nil {
		if a.PEMPath == "" {
			return nil, errors.New("ghapp: app has no private key (PEMPath empty)")
		}
		b, err := os.ReadFile(a.PEMPath)
		if err != nil {
			return nil, fmt.Errorf("ghapp: reading private key: %w", err)
		}
		k, err := ParsePrivateKey(b)
		if err != nil {
			return nil, err
		}
		d.key = k
	}
	return d, nil
}

// appCall performs one App-authenticated request (JWT bearer). The response
// JSON is decoded into out when non-nil; the Link header is returned for
// pagination. Errors carry the status and GitHub's message, never the body.
func (a App) appCall(ctx context.Context, method, path string, body io.Reader, out any) (link string, err error) {
	d, err := a.ready()
	if err != nil {
		return "", err
	}
	jwt, err := SignJWT(a.ID, d.key, d.now())
	if err != nil {
		return "", err
	}
	url := path
	if !strings.HasPrefix(path, "http") {
		url = d.baseURL + path
	}
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return "", fmt.Errorf("ghapp: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := d.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("ghapp: %s %s: %w", method, path, err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return "", fmt.Errorf("ghapp: reading response: %w", err)
	}
	if res.StatusCode < 200 || res.StatusCode > 299 {
		var gh struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(raw, &gh)
		return "", fmt.Errorf("ghapp: %s %s: HTTP %d %s", method, path, res.StatusCode, gh.Message)
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return "", fmt.Errorf("ghapp: decoding response: %w", err)
		}
	}
	return res.Header.Get("Link"), nil
}
