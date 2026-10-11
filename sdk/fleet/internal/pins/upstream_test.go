package pins

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestResolver(t *testing.T, token string, h http.HandlerFunc) *HTTPResolver {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	r := NewHTTPResolver(srv.Client(), token).(*HTTPResolver)
	r.GitHubAPI = srv.URL
	r.GoDownloads = srv.URL + "/dl/?mode=json"
	return r
}

func TestResolverGitHub(t *testing.T) {
	var gotAuth, gotPath string
	r := newTestResolver(t, "tok123", func(w http.ResponseWriter, req *http.Request) {
		gotAuth, gotPath = req.Header.Get("Authorization"), req.URL.Path
		_, _ = w.Write([]byte(`{"tag_name":"v0.13.1","published_at":"2026-09-30T12:00:00Z"}`))
	})
	rel, err := r.Latest(context.Background(), "gh:astral-sh/uv")
	if err != nil {
		t.Fatal(err)
	}
	if rel.Version != "0.13.1" || !rel.Published.Equal(time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)) {
		t.Errorf("release = %+v", rel)
	}
	if gotPath != "/repos/astral-sh/uv/releases/latest" || gotAuth != "Bearer tok123" {
		t.Errorf("path=%q auth=%q", gotPath, gotAuth)
	}
}

func TestResolverGitHubNoTokenNoAuthHeader(t *testing.T) {
	var gotAuth string
	r := newTestResolver(t, "", func(w http.ResponseWriter, req *http.Request) {
		gotAuth = req.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"tag_name":"3.13.1"}`))
	})
	rel, err := r.Latest(context.Background(), "gh:getsops/sops")
	if err != nil || rel.Version != "3.13.1" || !rel.Published.IsZero() {
		t.Fatalf("rel=%+v err=%v", rel, err)
	}
	if gotAuth != "" {
		t.Errorf("unexpected Authorization %q", gotAuth)
	}
}

func TestResolverGitHubPrefixedTag(t *testing.T) {
	r := newTestResolver(t, "", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"release-2.6.0"}`))
	})
	rel, err := r.Latest(context.Background(), "gh:rvaiya/keyd")
	if err != nil || rel.Version != "2.6.0" {
		t.Fatalf("rel=%+v err=%v", rel, err)
	}
}

func TestResolverGo(t *testing.T) {
	r := newTestResolver(t, "tok", func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("Authorization") != "" {
			t.Error("token must not be sent to go.dev")
		}
		_, _ = w.Write([]byte(`[{"version":"go1.28rc1","stable":false},{"version":"go1.27.1","stable":true},{"version":"go1.26.9","stable":true}]`))
	})
	rel, err := r.Latest(context.Background(), "go:")
	if err != nil || rel.Version != "1.27.1" {
		t.Fatalf("rel=%+v err=%v", rel, err)
	}
}

func TestResolverGoNoStable(t *testing.T) {
	r := newTestResolver(t, "", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"version":"go1.28rc1","stable":false}]`))
	})
	if _, err := r.Latest(context.Background(), "go:"); err == nil {
		t.Fatal("want error with no stable release")
	}
}

func TestResolverText(t *testing.T) {
	var gotAuth string
	r := newTestResolver(t, "tok", func(w http.ResponseWriter, req *http.Request) {
		gotAuth = req.Header.Get("Authorization")
		_, _ = w.Write([]byte("v1.37.2\n"))
	})
	rel, err := r.Latest(context.Background(), "text:"+r.GitHubAPI+"/release/stable.txt")
	if err != nil || rel.Version != "1.37.2" || !rel.Published.IsZero() {
		t.Fatalf("rel=%+v err=%v", rel, err)
	}
	if gotAuth != "" {
		t.Errorf("token must not be sent to arbitrary text urls, got %q", gotAuth)
	}
}

func TestResolverNoUpstream(t *testing.T) {
	r := NewHTTPResolver(nil, "")
	for _, u := range []string{"-", ""} {
		if _, err := r.Latest(context.Background(), u); !errors.Is(err, ErrNoUpstream) {
			t.Errorf("Latest(%q) err = %v, want ErrNoUpstream", u, err)
		}
	}
}

func TestResolverErrors(t *testing.T) {
	cases := []struct {
		name, upstream, body string
		status               int
	}{
		{"http 404", "gh:o/r", "nope", 404},
		{"http 500 text", "text:%URL%/x", "boom", 500},
		{"bad json gh", "gh:o/r", "{", 200},
		{"no version in tag", "gh:o/r", `{"tag_name":"nightly"}`, 200},
		{"empty tag", "gh:o/r", `{}`, 200},
		{"bad json go", "go:", "[", 200},
		{"text garbage", "text:%URL%/x", "<html>", 200},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newTestResolver(t, "", func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(c.status)
				_, _ = w.Write([]byte(c.body))
			})
			if _, err := r.Latest(context.Background(), strings.ReplaceAll(c.upstream, "%URL%", r.GitHubAPI)); err == nil {
				t.Fatal("want error")
			}
		})
	}
	r := NewHTTPResolver(nil, "")
	if _, err := r.Latest(context.Background(), "ftp://x"); err == nil || errors.Is(err, ErrNoUpstream) {
		t.Errorf("unknown scheme err = %v", err)
	}
	if _, err := r.Latest(context.Background(), "gh:badshape"); err == nil {
		t.Error("gh: without owner/repo must fail")
	}
}

func TestResolverHonoursContext(t *testing.T) {
	block := make(chan struct{})
	r := newTestResolver(t, "", func(http.ResponseWriter, *http.Request) { <-block })
	defer close(block)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := r.Latest(ctx, "gh:o/r"); err == nil {
		t.Fatal("want timeout error")
	}
	if time.Since(start) > 3*time.Second {
		t.Error("context deadline not honoured")
	}
}
