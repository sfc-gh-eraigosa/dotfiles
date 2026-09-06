package ghapp

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net"
	"net/http"
	"strings"
	"time"
)

// Manifest is what GitHub's App Manifest flow needs to create an App.
// Docs: https://docs.github.com/apps/sharing-github-apps/registering-a-github-app-from-a-manifest
type Manifest struct {
	Name        string            `json:"name"`
	URL         string            `json:"url,omitempty"`
	Description string            `json:"description,omitempty"`
	HookURL     string            `json:"hook_url,omitempty"`
	Public      bool              `json:"public"`
	Permissions map[string]string `json:"permissions,omitempty"`
	Events      []string          `json:"events,omitempty"`
}

// CreateOpts wires the manifest flow.
type CreateOpts struct {
	Store       FileStore          // where the PEM + apps.json land
	Org         string             // create under an organization instead of the user
	Port        int                // callback port; 0 = ephemeral; busy → next ports are tried
	OpenBrowser func(string) error // opens the local page; nil = OpenBrowser
	WebURL      string             // default https://github.com
	APIURL      string             // default https://api.github.com
	HTTP        *http.Client       // default http.DefaultClient
	Force       bool               // replace an existing slug in the store
}

// wireManifest is the JSON shape GitHub reads from the form field.
type wireManifest struct {
	Name           string            `json:"name"`
	URL            string            `json:"url"`
	Description    string            `json:"description,omitempty"`
	HookAttributes *hookAttributes   `json:"hook_attributes,omitempty"`
	RedirectURL    string            `json:"redirect_url"`
	Public         bool              `json:"public"`
	Permissions    map[string]string `json:"default_permissions,omitempty"`
	Events         []string          `json:"default_events,omitempty"`
}

type hookAttributes struct {
	URL    string `json:"url"`
	Active bool   `json:"active"`
}

// page renders the self-submitting form. Built by hand (not html/template)
// because the manifest JSON must survive inside a single-quoted attribute:
// only & ' < are entity-escaped, which every browser decodes back verbatim.
func page(name, action, manifestJSON string) string {
	attr := strings.NewReplacer("&", "&amp;", "'", "&#39;", "<", "&lt;").Replace(manifestJSON)
	return `<!doctype html><html><body>
<p>Redirecting to GitHub to create the App <b>` + html.EscapeString(name) + `</b>…</p>
<form id="f" method="post" action="` + html.EscapeString(action) + `">
<input type="hidden" name="manifest" value='` + attr + `'>
<noscript><button type="submit">Continue to GitHub</button></noscript>
</form>
<script>document.getElementById("f").submit()</script>
</body></html>`
}

// ErrManifestCode is wrapped when GitHub rejects the conversion code
// (expired after one hour, or already used).
var ErrManifestCode = errors.New("manifest code expired or invalid")

// Create runs the manifest flow: serve a self-submitting form on
// localhost, open it in the browser, receive the code on /callback,
// exchange it for the App, persist the PEM (0600) + app id, return the App.
// Webhook/client secrets returned by GitHub are dropped, never stored.
func Create(ctx context.Context, m Manifest, o CreateOpts) (App, error) {
	if strings.TrimSpace(m.Name) == "" {
		return App{}, errors.New("ghapp: manifest name is required")
	}
	if o.WebURL == "" {
		o.WebURL = "https://github.com"
	}
	if o.APIURL == "" {
		o.APIURL = "https://api.github.com"
	}
	if o.HTTP == nil {
		o.HTTP = http.DefaultClient
	}
	if o.OpenBrowser == nil {
		o.OpenBrowser = OpenBrowser
	}
	if o.Store.Dir == "" {
		o.Store.Dir = DefaultDir()
	}
	existing, err := o.Store.Load()
	if err != nil {
		return App{}, err
	}

	ln, err := listen(o.Port)
	if err != nil {
		return App{}, err
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	state := randomState()
	redirect := fmt.Sprintf("http://127.0.0.1:%d/callback", port)

	action := o.WebURL + "/settings/apps/new?state=" + state
	if o.Org != "" {
		action = o.WebURL + "/organizations/" + o.Org + "/settings/apps/new?state=" + state
	}
	wm := wireManifest{Name: m.Name, URL: m.URL, Description: m.Description, RedirectURL: redirect,
		Public: m.Public, Permissions: m.Permissions, Events: m.Events}
	if m.URL == "" {
		wm.URL = "https://github.com/sfc-gh-eraigosa/dotfiles/tree/main/sdk/ghapp"
	}
	if m.HookURL != "" {
		wm.HookAttributes = &hookAttributes{URL: m.HookURL, Active: true}
	}
	manifestJSON, err := json.Marshal(wm)
	if err != nil {
		return App{}, fmt.Errorf("ghapp: encoding manifest: %w", err)
	}

	type result struct {
		app App
		err error
	}
	done := make(chan result, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(page(m.Name, action, string(manifestJSON))))
	})
	mux.HandleFunc("GET /callback", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			http.Error(w, "state mismatch; ignoring", http.StatusBadRequest)
			return
		}
		code := r.URL.Query().Get("code")
		app, err := exchange(r.Context(), o, code, existing)
		if err != nil {
			http.Error(w, "ghapp: "+err.Error(), http.StatusBadGateway)
			select {
			case done <- result{err: err}:
			default:
			}
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, "<!doctype html><p>App <b>%s</b> (id %d) created and stored. You can close this tab and return to the terminal.</p>", html.EscapeString(app.Slug), app.ID)
		select {
		case done <- result{app: app}:
		default:
		}
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	defer srv.Close()

	if err := o.OpenBrowser(fmt.Sprintf("http://127.0.0.1:%d/", port)); err != nil {
		return App{}, fmt.Errorf("ghapp: opening browser: %w", err)
	}
	select {
	case r := <-done:
		return r.app, r.err
	case <-ctx.Done():
		return App{}, fmt.Errorf("ghapp: waiting for the GitHub callback: %w", ctx.Err())
	}
}

// exchange trades the code for the App and persists it.
func exchange(ctx context.Context, o CreateOpts, code string, existing Apps) (App, error) {
	if code == "" {
		return App{}, errors.New("callback carried no code")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.APIURL+"/app-manifests/"+code+"/conversions", bytes.NewReader(nil))
	if err != nil {
		return App{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	res, err := o.HTTP.Do(req)
	if err != nil {
		return App{}, fmt.Errorf("conversion request: %w", err)
	}
	defer res.Body.Close()
	var conv struct {
		ID      int64  `json:"id"`
		Slug    string `json:"slug"`
		Name    string `json:"name"`
		PEM     string `json:"pem"`
		HTMLURL string `json:"html_url"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(res.Body).Decode(&conv); err != nil && res.StatusCode == 201 {
		return App{}, fmt.Errorf("decoding conversion: %w", err)
	}
	if res.StatusCode != 201 {
		return App{}, fmt.Errorf("%w (HTTP %d %s)", ErrManifestCode, res.StatusCode, conv.Message)
	}
	if _, dup := existing[conv.Slug]; dup && !o.Force {
		return App{}, fmt.Errorf("app %q already exists in the store; re-run with --force to replace it", conv.Slug)
	}
	pemPath, err := o.Store.SavePEM(conv.Slug, []byte(conv.PEM))
	if err != nil {
		return App{}, err
	}
	app := App{ID: conv.ID, Slug: conv.Slug, PEMPath: pemPath}
	existing[conv.Slug] = app
	if err := o.Store.Save(existing); err != nil {
		return App{}, err
	}
	return app, nil
}

// listen binds 127.0.0.1:port, trying the next nine ports when busy.
func listen(port int) (net.Listener, error) {
	if port == 0 {
		return net.Listen("tcp", "127.0.0.1:0")
	}
	var last error
	for p := port; p < port+10; p++ {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
		if err == nil {
			return ln, nil
		}
		last = err
	}
	return nil, fmt.Errorf("ghapp: no free callback port in %d-%d: %w", port, port+9, last)
}

func randomState() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Info is the App's own metadata (GET /app).
type Info struct {
	ID      int64  `json:"id"`
	Slug    string `json:"slug"`
	Name    string `json:"name"`
	HTMLURL string `json:"html_url"`
}

// Info fetches the App's metadata with an App JWT — the cheapest proof
// that id + key are a matching pair.
func (a App) Info(ctx context.Context) (Info, error) {
	var out Info
	if _, err := a.appCall(ctx, http.MethodGet, "/app", nil, &out); err != nil {
		return Info{}, err
	}
	return out, nil
}
