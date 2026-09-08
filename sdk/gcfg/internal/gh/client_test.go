package gh

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// leakCanary is the only token value these tests use; gcfg-ci greps the
// verbose test log for it. Assembled at runtime so no source line looks like
// a credential assignment.
var leakCanary = strings.Join([]string{"ghs", "FIXTURE_TOKEN_DO_NOT_PRINT"}, "_")

func TestRESTSendsAuthAndAPIHeaders(t *testing.T) {
	var gotAuth, gotAccept, gotVersion, gotAgent string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotAccept = r.Header.Get("Authorization"), r.Header.Get("Accept")
		gotVersion, gotAgent = r.Header.Get("X-GitHub-Api-Version"), r.Header.Get("User-Agent")
		fmt.Fprint(w, `{"full_name":"o/r"}`)
	}))
	defer srv.Close()
	c := NewREST(RESTOpts{Bearer: leakCanary, BaseURL: srv.URL, HTTP: srv.Client()})
	var out struct {
		FullName string `json:"full_name"`
	}
	status, err := c.Do(context.Background(), "GET", "/repos/o/r", nil, &out)
	if err != nil || status != 200 || out.FullName != "o/r" {
		t.Fatalf("status=%d out=%+v err=%v", status, out, err)
	}
	if gotAuth != "Bearer "+leakCanary {
		t.Errorf("Authorization header wrong")
	}
	if gotAccept != "application/vnd.github+json" || gotVersion != "2022-11-28" {
		t.Errorf("accept=%q version=%q", gotAccept, gotVersion)
	}
	if !strings.HasPrefix(gotAgent, "gcfg/") {
		t.Errorf("User-Agent = %q", gotAgent)
	}
}

func TestRESTSendsJSONBodyAndReturnsStatusForKnownFailures(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PATCH" {
			_ = json.NewDecoder(r.Body).Decode(&body)
			if r.Header.Get("Content-Type") != "application/json" {
				t.Errorf("Content-Type = %q", r.Header.Get("Content-Type"))
			}
			w.WriteHeader(200)
			fmt.Fprint(w, `{}`)
			return
		}
		w.WriteHeader(403)
		fmt.Fprint(w, `{"message":"Resource not accessible by personal access token"}`)
	}))
	defer srv.Close()
	c := NewREST(RESTOpts{Bearer: "t", BaseURL: srv.URL, HTTP: srv.Client()})
	if _, err := c.Do(context.Background(), "PATCH", "/repos/o/r", map[string]any{"has_wiki": false}, nil); err != nil {
		t.Fatal(err)
	}
	if body["has_wiki"] != false {
		t.Errorf("body = %v", body)
	}
	// A 4xx is returned as a status plus an error naming what GitHub said —
	// the caller decides whether it is fatal (a family read is not).
	status, err := c.Do(context.Background(), "GET", "/repos/o/r", nil, nil)
	if status != 403 || err == nil || !strings.Contains(err.Error(), "not accessible") {
		t.Fatalf("status=%d err=%v", status, err)
	}
}

func TestRESTRetriesServerErrorsAndHonoursRetryAfter(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch calls.Add(1) {
		case 1:
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(503)
		case 2:
			w.WriteHeader(500)
		default:
			fmt.Fprint(w, `{"ok":true}`)
		}
	}))
	defer srv.Close()
	var slept []time.Duration
	c := NewREST(RESTOpts{Bearer: "t", BaseURL: srv.URL, HTTP: srv.Client(), Sleep: func(d time.Duration) { slept = append(slept, d) }})
	status, err := c.Do(context.Background(), "GET", "/x", nil, nil)
	if err != nil || status != 200 {
		t.Fatalf("status=%d err=%v", status, err)
	}
	if calls.Load() != 3 {
		t.Fatalf("want 3 attempts, got %d", calls.Load())
	}
	if len(slept) != 2 || slept[0] != 2*time.Second {
		t.Fatalf("Retry-After must win the first wait: %v", slept)
	}
	if slept[1] < 100*time.Millisecond {
		t.Errorf("second wait should back off, got %v", slept[1])
	}
}

func TestRESTGivesUpAfterTheLastAttempt(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(502)
	}))
	defer srv.Close()
	c := NewREST(RESTOpts{Bearer: "t", BaseURL: srv.URL, HTTP: srv.Client(), Sleep: func(time.Duration) {}})
	status, err := c.Do(context.Background(), "GET", "/x", nil, nil)
	if status != 502 || err == nil {
		t.Fatalf("status=%d err=%v", status, err)
	}
	if calls.Load() != 4 {
		t.Fatalf("want 4 attempts (1 + 3 retries), got %d", calls.Load())
	}
}

func TestRESTSecondaryRateLimitIsRetried(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(429)
			return
		}
		fmt.Fprint(w, `{}`)
	}))
	defer srv.Close()
	c := NewREST(RESTOpts{Bearer: "t", BaseURL: srv.URL, HTTP: srv.Client(), Sleep: func(time.Duration) {}})
	if status, err := c.Do(context.Background(), "GET", "/x", nil, nil); err != nil || status != 200 {
		t.Fatalf("status=%d err=%v", status, err)
	}
}

func TestRESTPaginatesFollowingLinkHeaders(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("page") {
		case "", "1":
			w.Header().Set("Link", fmt.Sprintf(`<%s/labels?per_page=100&page=2>; rel="next", <%s/labels?page=2>; rel="last"`, srv.URL, srv.URL))
			fmt.Fprint(w, `[{"name":"bug"},{"name":"chore"}]`)
		default:
			fmt.Fprint(w, `[{"name":"docs"}]`)
		}
	}))
	defer srv.Close()
	c := NewREST(RESTOpts{Bearer: "t", BaseURL: srv.URL, HTTP: srv.Client()})
	var names []string
	err := c.Paginate(context.Background(), "/labels", func(raw json.RawMessage) error {
		var l struct{ Name string }
		if err := json.Unmarshal(raw, &l); err != nil {
			return err
		}
		names = append(names, l.Name)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(names, ",") != "bug,chore,docs" {
		t.Fatalf("names = %v", names)
	}
}

func TestPaginateStopsOnCallbackErrorAndOnFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/bad" {
			w.WriteHeader(404)
			fmt.Fprint(w, `{"message":"Not Found"}`)
			return
		}
		fmt.Fprint(w, `[{"name":"a"},{"name":"b"}]`)
	}))
	defer srv.Close()
	c := NewREST(RESTOpts{Bearer: "t", BaseURL: srv.URL, HTTP: srv.Client()})
	boom := fmt.Errorf("stop here")
	seen := 0
	err := c.Paginate(context.Background(), "/labels", func(json.RawMessage) error {
		seen++
		return boom
	})
	if err != boom || seen != 1 {
		t.Fatalf("err=%v seen=%d", err, seen)
	}
	if err := c.Paginate(context.Background(), "/bad", func(json.RawMessage) error { return nil }); err == nil {
		t.Fatal("want an error for a failing page")
	}
}

func TestRESTErrorsNeverCarryTheToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		fmt.Fprint(w, `{"message":"Bad credentials"}`)
	}))
	defer srv.Close()
	c := NewREST(RESTOpts{Bearer: leakCanary, BaseURL: srv.URL, HTTP: srv.Client()})
	_, err := c.Do(context.Background(), "GET", "/x", nil, nil)
	if err == nil || strings.Contains(err.Error(), leakCanary) {
		t.Fatalf("error leaks the token or is missing: %v", err)
	}
	if err := c.Paginate(context.Background(), "/x", func(json.RawMessage) error { return nil }); err == nil || strings.Contains(err.Error(), leakCanary) {
		t.Fatalf("paginate error leaks the token or is missing: %v", err)
	}
}

func TestRESTContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{}`) }))
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := NewREST(RESTOpts{Bearer: "t", BaseURL: srv.URL, HTTP: srv.Client()})
	if _, err := c.Do(ctx, "GET", "/x", nil, nil); err == nil {
		t.Fatal("want a context error")
	}
}
