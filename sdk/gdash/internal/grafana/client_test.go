package grafana_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gdash/internal/grafana"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gdash/internal/grafana/grafanatest"
)

type bearer string

func (b bearer) Apply(r *http.Request) { r.Header.Set("Authorization", "Bearer "+string(b)) }

func client(t *testing.T) (grafana.Client, *grafanatest.Server) {
	t.Helper()
	s := grafanatest.New(t)
	return grafana.NewHTTP(s.URL, bearer("fixture"), 5*time.Second), s
}

func TestHealthAndWhoami(t *testing.T) {
	c, _ := client(t)
	h, err := c.Health(context.Background())
	if err != nil || h.Version != "13.1.3" || h.Database != "ok" {
		t.Fatalf("Health = %+v %v", h, err)
	}
	if err := c.Whoami(context.Background()); err != nil {
		t.Fatal(err)
	}
	bad := grafana.NewHTTP(grafanatest.New(t).URL, nil, time.Second)
	if err := bad.Whoami(context.Background()); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("no credential should be a 401: %v", err)
	}
}

func TestSearch(t *testing.T) {
	c, _ := client(t)
	hits, err := c.Search(context.Background(), "")
	if err != nil || len(hits) != 27 {
		t.Fatalf("Search = %d hits, %v", len(hits), err)
	}
	var found bool
	for _, h := range hits {
		if h.UID == "ci-lab" && h.Title == "CI — lab runners" {
			found = true
		}
	}
	if !found {
		t.Fatal("ci-lab missing from search")
	}
}

func TestDashboardRawJSON(t *testing.T) {
	c, _ := client(t)
	raw, err := c.Dashboard(context.Background(), "conductor-metrics")
	if err != nil {
		t.Fatal(err)
	}
	var d struct {
		Dashboard struct{ Title string } `json:"dashboard"`
	}
	if json.Unmarshal(raw, &d) != nil || d.Dashboard.Title != "Conductor — metrics against goals" {
		t.Fatalf("dashboard title = %q", d.Dashboard.Title)
	}
	if _, err := c.Dashboard(context.Background(), "nope"); err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("missing dashboard: %v", err)
	}
}

// E2: a title resolves only on one exact match; otherwise the error names
// the candidates.
func TestResolve(t *testing.T) {
	c, _ := client(t)
	ctx := context.Background()
	for ref, want := range map[string]string{
		"ci-lab":                "ci-lab",
		"CI — lab runners":      "ci-lab",
		"node exporter / nodes": "7d57716318ee0dddbac5a7f451fb7753", // case differs, single match
	} {
		if got, err := grafana.Resolve(ctx, c, ref); err != nil || got != want {
			t.Errorf("Resolve(%q) = %q %v, want %q", ref, got, err, want)
		}
	}
	_, err := grafana.Resolve(ctx, c, "Kubernetes / Networking")
	if err == nil || !strings.Contains(err.Error(), "Kubernetes / Networking / Pod") {
		t.Fatalf("ambiguous title should list candidates: %v", err)
	}
	if _, err := grafana.Resolve(ctx, c, "no such dashboard"); err == nil {
		t.Fatal("unknown title resolved")
	}
}

func TestQueryParsesFrames(t *testing.T) {
	c, _ := client(t)
	now := time.UnixMilli(1792385568077)
	res, err := c.Query(context.Background(), grafana.QueryRequest{
		From: now.Add(-time.Hour), To: now,
		Queries: []grafana.Query{
			{RefID: "A", Datasource: grafana.DSRef{Type: "prometheus", UID: "prometheus"}, Expr: `conductor_measure{name="dispatch_pass_rate"}`, IntervalMs: 30000, MaxDataPoints: 240},
			{RefID: "B", Datasource: grafana.DSRef{Type: "prometheus", UID: "prometheus"}, Expr: `conductor_goal{name="dispatch_pass_rate"}`, IntervalMs: 30000, MaxDataPoints: 240},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Series) != 2 || len(res.Errors) != 0 {
		t.Fatalf("series = %d, errors = %v", len(res.Series), res.Errors)
	}
	s := res.Series[0]
	if s.RefID != "A" || len(s.Times) != 121 || len(s.Values) != 121 || s.Labels["host"] != "spark1" {
		t.Fatalf("series A = refId %s, %d points, labels %v", s.RefID, len(s.Times), s.Labels)
	}
	if last, ok := s.Last(); !ok || last <= 0 {
		t.Fatalf("Last = %v %v", last, ok)
	}
}

func TestQueryReportsPerQueryErrors(t *testing.T) {
	c, _ := client(t)
	res, err := c.Query(context.Background(), grafana.QueryRequest{From: time.Now().Add(-time.Hour), To: time.Now(),
		Queries: []grafana.Query{{RefID: "A", Datasource: grafana.DSRef{Type: "prometheus", UID: "prometheus"}, Expr: grafanatest.ErrorExpr}}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Errors["A"] == "" {
		t.Fatalf("query error not reported: %+v", res)
	}
}

// E1: one ds/query per panel, never more than 4 in flight.
func TestFetchAllCapsConcurrency(t *testing.T) {
	s := grafanatest.New(t)
	var inflight, peak atomic.Int32
	s.Delay = 50 * time.Millisecond
	s.OnQuery = func(start bool) {
		if start {
			n := inflight.Add(1)
			for {
				p := peak.Load()
				if n <= p || peak.CompareAndSwap(p, n) {
					break
				}
			}
		} else {
			inflight.Add(-1)
		}
	}
	c := grafana.NewHTTP(s.URL, bearer("fixture"), 5*time.Second)
	reqs := make([]grafana.QueryRequest, 10)
	for i := range reqs {
		reqs[i] = grafana.QueryRequest{From: time.Now().Add(-time.Hour), To: time.Now(),
			Queries: []grafana.Query{{RefID: "A", Datasource: grafana.DSRef{Type: "prometheus", UID: "prometheus"}, Expr: "up"}}}
	}
	out := grafana.FetchAll(context.Background(), c, reqs)
	if len(out) != 10 || s.Queries() != 10 {
		t.Fatalf("results %d, server saw %d queries", len(out), s.Queries())
	}
	if p := peak.Load(); p > grafana.MaxConcurrent || p < 2 {
		t.Fatalf("peak in-flight = %d, want 2..%d", p, grafana.MaxConcurrent)
	}
}

func TestPointsAndInterval(t *testing.T) {
	if got := grafana.MaxDataPoints(60); got != 120 {
		t.Fatalf("MaxDataPoints(60) = %d", got)
	}
	if got := grafana.MaxDataPoints(0); got != 2 {
		t.Fatalf("width 0 → %d, want 2", got)
	}
	// 1h over 120 points = 30s; rounded up to a whole second; never under the ds minimum.
	if got := grafana.IntervalMs(time.Hour, 120, 0); got != 30000 {
		t.Fatalf("IntervalMs = %d", got)
	}
	if got := grafana.IntervalMs(time.Hour, 7000, 0); got != 1000 {
		t.Fatalf("rounded up to a second: %d", got)
	}
	if got := grafana.IntervalMs(time.Hour, 7000, 15*time.Second); got != 15000 {
		t.Fatalf("ds minimum ignored: %d", got)
	}
}

func TestTimeout(t *testing.T) {
	s := grafanatest.New(t)
	s.Delay = 500 * time.Millisecond
	c := grafana.NewHTTP(s.URL, bearer("fixture"), 100*time.Millisecond)
	start := time.Now()
	if _, err := c.Health(context.Background()); err == nil {
		t.Fatal("slow server did not time out")
	}
	if time.Since(start) > 400*time.Millisecond {
		t.Fatal("timeout not honoured")
	}
}

func TestDatasources(t *testing.T) {
	c, _ := client(t)
	ds, err := c.Datasources(context.Background())
	if err != nil || len(ds) != 2 {
		t.Fatalf("Datasources = %+v %v", ds, err)
	}
	var def grafana.Datasource
	for _, d := range ds {
		if d.IsDefault {
			def = d
		}
	}
	if def.UID != "prometheus" || def.TimeInterval != "30s" {
		t.Fatalf("default ds = %+v", def)
	}
}

func TestHTTPErrorCarriesGrafanasMessage(t *testing.T) {
	s := grafanatest.New(t)
	s.Status = 400
	s.Message = `{"message":"duplicate refId"}`
	c := grafana.NewHTTP(s.URL, bearer("fixture"), time.Second)
	_, err := c.Health(context.Background())
	if err == nil || !strings.Contains(err.Error(), "HTTP 400") || !strings.Contains(err.Error(), "duplicate refId") {
		t.Fatalf("error = %v", err)
	}
}

type redactingCred string

func (r redactingCred) Apply(req *http.Request) { req.Header.Set("Authorization", "Bearer "+string(r)) }
func (r redactingCred) Redact(s string) string {
	return strings.ReplaceAll(s, string(r), "‹redacted›")
}

// A server that echoes the Authorization header in its error body must not
// get the credential into gdash's error message.
func TestErrorsAreRedacted(t *testing.T) {
	s := grafanatest.New(t)
	s.Status, s.EchoAuth = 401, true
	c := grafana.NewHTTP(s.URL, redactingCred("sekrit-value"), time.Second)
	_, err := c.Health(context.Background())
	if err == nil || strings.Contains(err.Error(), "sekrit-value") || !strings.Contains(err.Error(), "‹redacted›") {
		t.Fatalf("error = %v", err)
	}
}

// The body is redacted before it is cut to length, so a token echoed past
// byte 200 cannot survive as a prefix.
func TestRedactBeforeTruncate(t *testing.T) {
	s := grafanatest.New(t)
	s.Status = 401
	tok := strings.Repeat("t", 64)
	s.Message = `{"message":"` + strings.Repeat("x", 180) + " Bearer " + tok + `"}`
	c := grafana.NewHTTP(s.URL, redactingCred(tok), time.Second)
	_, err := c.Health(context.Background())
	if err == nil || strings.Contains(err.Error(), strings.Repeat("t", 8)) {
		t.Fatalf("a token prefix survived: %v", err)
	}
}
