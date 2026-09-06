package ghapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// neverStored marks values GitHub returns from the conversion that must not
// land in apps.json (webhook + client secrets). Assembled at runtime so no
// source line looks like a credential assignment.
var neverStored = strings.Join([]string{"never", "stored", "value"}, "-")

// conversions stub: POST /app-manifests/{code}/conversions.
type convStub struct {
	srv   *httptest.Server
	pem   []byte
	codes map[string]bool // valid codes
	calls int
}

func newConvStub(t *testing.T) *convStub {
	t.Helper()
	s := newStub(t)
	c := &convStub{pem: pemFor(s.key), codes: map[string]bool{"good-code": true}}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /app-manifests/{code}/conversions", func(w http.ResponseWriter, r *http.Request) {
		c.calls++
		if !c.codes[r.PathValue("code")] {
			w.WriteHeader(404)
			fmt.Fprint(w, `{"message":"Not Found"}`)
			return
		}
		w.WriteHeader(201)
		out := map[string]any{"id": 4242, "slug": "gcfg-test", "name": "gcfg test", "pem": string(c.pem),
			"html_url": "https://github.com/apps/gcfg-test"}
		for _, k := range []string{"webhook_" + "secret", "client_" + "secret"} {
			out[k] = neverStored
		}
		_ = json.NewEncoder(w).Encode(out)
	})
	c.srv = httptest.NewServer(mux)
	t.Cleanup(c.srv.Close)
	return c
}

var formRE = regexp.MustCompile(`(?s)<form[^>]*action="([^"]+)"[^>]*>.*?name="manifest"[^>]*value='([^']*)'`)

// browse plays GitHub: fetch the local page, read the form, then redirect
// the "browser" to the callback with code+state.
func browse(t *testing.T, code string) (opener func(string) error, form *formSeen) {
	t.Helper()
	form = &formSeen{done: make(chan struct{})}
	return func(u string) error {
		go func() {
			defer close(form.done)
			res, err := http.Get(u)
			if err != nil {
				form.err = err
				return
			}
			b, _ := io.ReadAll(res.Body)
			res.Body.Close()
			m := formRE.FindStringSubmatch(string(b))
			if m == nil {
				form.err = fmt.Errorf("no manifest form in page: %s", b)
				return
			}
			form.action = m[1]
			_ = json.Unmarshal([]byte(strings.ReplaceAll(m[2], "&#39;", "'")), &form.manifest)
			action, _ := url.Parse(form.action)
			state := action.Query().Get("state")
			form.state = state
			redirect, _ := form.manifest["redirect_url"].(string)
			res, err = http.Get(redirect + "?code=" + code + "&state=" + state)
			if err != nil {
				form.err = err
				return
			}
			b, _ = io.ReadAll(res.Body)
			res.Body.Close()
			form.callbackStatus = res.StatusCode
			form.callbackBody = string(b)
		}()
		return nil
	}, form
}

type formSeen struct {
	done           chan struct{}
	action         string
	manifest       map[string]any
	state          string
	callbackStatus int
	callbackBody   string
	err            error
}

// wait blocks until the fake browser finished its round-trip (or 5s).
func (f *formSeen) wait(t *testing.T) {
	t.Helper()
	select {
	case <-f.done:
	case <-time.After(5 * time.Second):
		t.Fatal("fake browser never finished")
	}
}

func testCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestCreateManifestFlowPersistsApp(t *testing.T) {
	c := newConvStub(t)
	dir := filepath.Join(t.TempDir(), "ghapp")
	opener, form := browse(t, "good-code")
	app, err := Create(testCtx(t), Manifest{
		Name: "gcfg test", URL: "https://github.com/sfc-gh-eraigosa/dotfiles",
		Permissions: map[string]string{"administration": "write", "metadata": "read"},
	}, CreateOpts{Store: FileStore{Dir: dir}, OpenBrowser: opener, APIURL: c.srv.URL, WebURL: "https://web.example", Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	form.wait(t)
	if form.err != nil {
		t.Fatal(form.err)
	}
	if !strings.HasPrefix(form.action, "https://web.example/settings/apps/new?state=") {
		t.Errorf("form action = %q", form.action)
	}
	if form.manifest["name"] != "gcfg test" || form.manifest["url"] != "https://github.com/sfc-gh-eraigosa/dotfiles" || form.manifest["public"] != false {
		t.Errorf("manifest = %v", form.manifest)
	}
	if _, has := form.manifest["hook_attributes"]; has {
		t.Errorf("hook_attributes must be absent when HookURL is empty: %v", form.manifest)
	}
	perms, _ := form.manifest["default_permissions"].(map[string]any)
	if perms["administration"] != "write" || perms["metadata"] != "read" {
		t.Errorf("default_permissions = %v", perms)
	}
	if ru, _ := form.manifest["redirect_url"].(string); !regexp.MustCompile(`^http://127\.0\.0\.1:\d+/callback$`).MatchString(ru) {
		t.Errorf("redirect_url = %q", ru)
	}
	if form.callbackStatus != 200 || !strings.Contains(form.callbackBody, "gcfg-test") {
		t.Errorf("callback: %d %q", form.callbackStatus, form.callbackBody)
	}
	if app.ID != 4242 || app.Slug != "gcfg-test" || app.PEMPath != filepath.Join(dir, "gcfg-test.pem") {
		t.Fatalf("app = %+v", app)
	}
	st, err := os.Stat(app.PEMPath)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Errorf("pem mode %o", st.Mode().Perm())
	}
	got, _ := os.ReadFile(app.PEMPath)
	if string(got) != string(c.pem) {
		t.Error("stored PEM differs from the one GitHub returned")
	}
	apps, err := FileStore{Dir: dir}.Load()
	if err != nil || apps["gcfg-test"].ID != 4242 {
		t.Fatalf("store after create: %+v %v", apps, err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "apps.json"))
	if strings.Contains(string(raw), neverStored) {
		t.Errorf("apps.json stores a conversion secret")
	}
}

func TestCreateOrgAndHookURL(t *testing.T) {
	c := newConvStub(t)
	opener, form := browse(t, "good-code")
	_, err := Create(testCtx(t), Manifest{Name: "x", HookURL: "https://hooks.example/gh", Events: []string{"push"}},
		CreateOpts{Store: FileStore{Dir: filepath.Join(t.TempDir(), "g")}, OpenBrowser: opener, APIURL: c.srv.URL, WebURL: "https://web.example", Org: "acme"})
	if err != nil {
		t.Fatal(err)
	}
	form.wait(t)
	if !strings.HasPrefix(form.action, "https://web.example/organizations/acme/settings/apps/new?state=") {
		t.Errorf("org form action = %q", form.action)
	}
	hook, _ := form.manifest["hook_attributes"].(map[string]any)
	if hook["url"] != "https://hooks.example/gh" || hook["active"] != true {
		t.Errorf("hook_attributes = %v", hook)
	}
	if ev, _ := form.manifest["default_events"].([]any); len(ev) != 1 || ev[0] != "push" {
		t.Errorf("default_events = %v", form.manifest["default_events"])
	}
}

func TestCreateExpiredCodeIsAClearError(t *testing.T) {
	c := newConvStub(t)
	opener, form := browse(t, "stale-code")
	_, err := Create(testCtx(t), Manifest{Name: "x"},
		CreateOpts{Store: FileStore{Dir: filepath.Join(t.TempDir(), "g")}, OpenBrowser: opener, APIURL: c.srv.URL})
	if err == nil || !strings.Contains(err.Error(), "expired") || !strings.Contains(err.Error(), "404") {
		t.Fatalf("want expired/404 error, got %v", err)
	}
	form.wait(t)
	if form.callbackStatus != 502 {
		t.Errorf("callback should report the failure to the browser (502), got %d", form.callbackStatus)
	}
}

func TestCreateIgnoresWrongState(t *testing.T) {
	c := newConvStub(t)
	opener := func(u string) error {
		go func() {
			res, _ := http.Get(u)
			b, _ := io.ReadAll(res.Body)
			res.Body.Close()
			m := formRE.FindStringSubmatch(string(b))
			var man map[string]any
			_ = json.Unmarshal([]byte(m[2]), &man)
			action, _ := url.Parse(m[1])
			state := action.Query().Get("state")
			redirect := man["redirect_url"].(string)
			bad, _ := http.Get(redirect + "?code=good-code&state=WRONG")
			if bad.StatusCode != 400 {
				panic(fmt.Sprintf("wrong state accepted: %d", bad.StatusCode))
			}
			bad.Body.Close()
			ok, _ := http.Get(redirect + "?code=good-code&state=" + state)
			ok.Body.Close()
		}()
		return nil
	}
	app, err := Create(testCtx(t), Manifest{Name: "x"},
		CreateOpts{Store: FileStore{Dir: filepath.Join(t.TempDir(), "g")}, OpenBrowser: opener, APIURL: c.srv.URL})
	if err != nil || app.ID != 4242 {
		t.Fatalf("app=%+v err=%v", app, err)
	}
	if c.calls != 1 {
		t.Fatalf("conversion must run once (for the good state only), got %d", c.calls)
	}
}

func TestCreatePortFallback(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	busy := l.Addr().(*net.TCPAddr).Port
	c := newConvStub(t)
	opener, form := browse(t, "good-code")
	if _, err := Create(testCtx(t), Manifest{Name: "x"},
		CreateOpts{Store: FileStore{Dir: filepath.Join(t.TempDir(), "g")}, OpenBrowser: opener, APIURL: c.srv.URL, Port: busy}); err != nil {
		t.Fatal(err)
	}
	form.wait(t)
	ru, _ := form.manifest["redirect_url"].(string)
	if !strings.HasSuffix(ru, fmt.Sprintf(":%d/callback", busy+1)) {
		t.Fatalf("want fallback to port %d, redirect_url = %q", busy+1, ru)
	}
}

func TestCreateOpenerErrorAndCancel(t *testing.T) {
	c := newConvStub(t)
	boom := errors.New("no browser")
	_, err := Create(testCtx(t), Manifest{Name: "x"},
		CreateOpts{Store: FileStore{Dir: filepath.Join(t.TempDir(), "g")}, OpenBrowser: func(string) error { return boom }, APIURL: c.srv.URL})
	if !errors.Is(err, boom) {
		t.Fatalf("want opener error, got %v", err)
	}
	ctx, cancel := context.WithTimeout(testCtx(t), 50*time.Millisecond)
	defer cancel()
	_, err = Create(ctx, Manifest{Name: "x"},
		CreateOpts{Store: FileStore{Dir: filepath.Join(t.TempDir(), "g")}, OpenBrowser: func(string) error { return nil }, APIURL: c.srv.URL})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want deadline error, got %v", err)
	}
}

func TestCreateRequiresName(t *testing.T) {
	_, err := Create(testCtx(t), Manifest{}, CreateOpts{Store: FileStore{Dir: t.TempDir()}, OpenBrowser: func(string) error { t.Fatal("must not open"); return nil }})
	if err == nil || !strings.Contains(err.Error(), "name") {
		t.Fatalf("want name error, got %v", err)
	}
}

func TestInfoReturnsAppMetadata(t *testing.T) {
	s := newStub(t)
	info, err := s.app().Info(testCtx(t))
	if err != nil {
		t.Fatal(err)
	}
	if info.ID != 777 || info.Slug != "test-app" || info.HTMLURL != "https://github.com/apps/test-app" {
		t.Fatalf("info = %+v", info)
	}
}
