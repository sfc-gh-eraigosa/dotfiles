// Package grafana is gdash's read-only Grafana client: health, auth check,
// dashboard search and JSON, and /api/ds/query parsed into series (spec F1).
package grafana

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// MaxConcurrent is the most panel queries in flight at once (spec F1).
const MaxConcurrent = 4

// Credential sets a request's Authorization header (auth.Credential fits).
type Credential interface {
	Apply(r *http.Request)
}

// redactor is a Credential that can scrub itself out of text. The client
// runs every error message through it, so a server or proxy that echoes the
// Authorization header back cannot get the credential onto the screen.
type redactor interface {
	Redact(s string) string
}

// Client is everything gdash asks of Grafana.
type Client interface {
	Health(ctx context.Context) (Health, error)
	Whoami(ctx context.Context) error
	Search(ctx context.Context, query string) ([]Hit, error)
	Dashboard(ctx context.Context, uid string) (json.RawMessage, error)
	Query(ctx context.Context, r QueryRequest) (Result, error)
	Datasources(ctx context.Context) ([]Datasource, error)
}

// Datasource is one entry of /api/datasources (a Viewer may list them).
type Datasource struct {
	UID          string
	Name         string
	Type         string
	IsDefault    bool
	TimeInterval string // the scrape interval, e.g. "30s"; "" when unset
}

// Health is /api/health.
type Health struct {
	Version  string `json:"version"`
	Database string `json:"database"`
	Commit   string `json:"commit"`
}

// Hit is one /api/search result.
type Hit struct {
	UID         string `json:"uid"`
	Title       string `json:"title"`
	FolderTitle string `json:"folderTitle"`
	Type        string `json:"type"`
}

// DSRef names a datasource.
type DSRef struct {
	Type string `json:"type"`
	UID  string `json:"uid"`
}

// Query is one target of a panel.
type Query struct {
	RefID         string `json:"refId"`
	Datasource    DSRef  `json:"datasource"`
	Expr          string `json:"expr"`
	IntervalMs    int64  `json:"intervalMs,omitempty"`
	MaxDataPoints int    `json:"maxDataPoints,omitempty"`
	Instant       bool   `json:"instant"`
	Range         bool   `json:"range"`
}

// QueryRequest is one /api/ds/query call: a panel's targets over a range.
type QueryRequest struct {
	From, To time.Time
	Queries  []Query
}

// Series is one line: a number field of a frame, with its labels.
type Series struct {
	RefID  string
	Name   string // the display name Grafana sent, else the field name
	Labels map[string]string
	Times  []time.Time
	Values []float64 // NaN where the point is null
}

// Last is the last non-null value.
func (s Series) Last() (float64, bool) {
	for i := len(s.Values) - 1; i >= 0; i-- {
		if !math.IsNaN(s.Values[i]) {
			return s.Values[i], true
		}
	}
	return 0, false
}

// Result is a parsed /api/ds/query response.
type Result struct {
	Series []Series
	Errors map[string]string // refId → error Grafana reported for that target
}

// MaxDataPoints is how many points a chart this many cells wide can draw:
// two braille columns per cell.
func MaxDataPoints(width int) int {
	if width < 1 {
		width = 1
	}
	return 2 * width
}

// IntervalMs is the step for a range drawn in maxPoints points: rounded up
// to a whole second and never below the datasource's minimum (spec F1).
func IntervalMs(rng time.Duration, maxPoints int, minInterval time.Duration) int64 {
	if maxPoints < 1 {
		maxPoints = 1
	}
	ms := int64(math.Ceil(float64(rng.Milliseconds())/float64(maxPoints)/1000)) * 1000
	if ms < 1000 {
		ms = 1000
	}
	if m := minInterval.Milliseconds(); ms < m {
		ms = m
	}
	return ms
}

type httpClient struct {
	base string
	cred Credential
	hc   *http.Client
}

// NewHTTP is a Client for the Grafana at base. cred may be nil (health only).
func NewHTTP(base string, cred Credential, timeout time.Duration) Client {
	return &httpClient{base: strings.TrimRight(base, "/"), cred: cred, hc: &http.Client{Timeout: timeout}}
}

// HTTPError is a non-2xx answer.
type HTTPError struct {
	Status  int
	Path    string
	Message string // Grafana's "message" (or the start of its body), when it sent one
}

func (e *HTTPError) Error() string {
	hint := ""
	switch e.Status {
	case http.StatusUnauthorized:
		hint = " (the credential was refused)"
	case http.StatusForbidden:
		hint = " (the credential may not read this)"
	case http.StatusNotFound:
		hint = " (not found)"
	}
	msg := ""
	if e.Message != "" {
		msg = ": " + e.Message
	}
	return fmt.Sprintf("grafana %s: HTTP %d%s%s", e.Path, e.Status, hint, msg)
}

func (c *httpClient) do(ctx context.Context, method, path string, body any, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.cred != nil {
		c.cred.Apply(req)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		// net/http errors carry the URL, never the headers: no secret leaks here.
		return fmt.Errorf("grafana %s: %s", strings.SplitN(path, "?", 2)[0], c.redact(err.Error()))
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode/100 != 2 {
		return &HTTPError{Status: resp.StatusCode, Path: strings.SplitN(path, "?", 2)[0], Message: c.redact(errorMessage(resp.Body))}
	}
	switch o := out.(type) {
	case nil:
		return nil
	case *json.RawMessage:
		b, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
		*o = b
		return err
	default:
		return json.NewDecoder(resp.Body).Decode(out)
	}
}

func (c *httpClient) redact(s string) string {
	if r, ok := c.cred.(redactor); ok {
		return r.Redact(s)
	}
	return s
}

func (c *httpClient) Health(ctx context.Context) (Health, error) {
	var h Health
	err := c.do(ctx, http.MethodGet, "/api/health", nil, &h)
	return h, err
}

// Whoami checks the credential with the cheapest call a Viewer may make.
func (c *httpClient) Whoami(ctx context.Context) error {
	return c.do(ctx, http.MethodGet, "/api/search?limit=1", nil, nil)
}

func (c *httpClient) Search(ctx context.Context, query string) ([]Hit, error) {
	v := url.Values{"type": {"dash-db"}}
	if query != "" {
		v.Set("query", query)
	}
	var hits []Hit
	err := c.do(ctx, http.MethodGet, "/api/search?"+v.Encode(), nil, &hits)
	return hits, err
}

func (c *httpClient) Dashboard(ctx context.Context, uid string) (json.RawMessage, error) {
	var raw json.RawMessage
	err := c.do(ctx, http.MethodGet, "/api/dashboards/uid/"+url.PathEscape(uid), nil, &raw)
	return raw, err
}

func (c *httpClient) Datasources(ctx context.Context) ([]Datasource, error) {
	var wire []struct {
		UID       string `json:"uid"`
		Name      string `json:"name"`
		Type      string `json:"type"`
		IsDefault bool   `json:"isDefault"`
		JSONData  struct {
			TimeInterval string `json:"timeInterval"`
		} `json:"jsonData"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/datasources", nil, &wire); err != nil {
		return nil, err
	}
	out := make([]Datasource, 0, len(wire))
	for _, w := range wire {
		out = append(out, Datasource{UID: w.UID, Name: w.Name, Type: w.Type, IsDefault: w.IsDefault, TimeInterval: w.JSONData.TimeInterval})
	}
	return out, nil
}

type wireFrame struct {
	Schema struct {
		Fields []struct {
			Name   string            `json:"name"`
			Type   string            `json:"type"`
			Labels map[string]string `json:"labels"`
			Config struct {
				DisplayNameFromDS string `json:"displayNameFromDS"`
			} `json:"config"`
		} `json:"fields"`
	} `json:"schema"`
	Data struct {
		Values [][]json.Number `json:"values"`
	} `json:"data"`
}

func (c *httpClient) Query(ctx context.Context, r QueryRequest) (Result, error) {
	body := map[string]any{
		"from":    strconv.FormatInt(r.From.UnixMilli(), 10),
		"to":      strconv.FormatInt(r.To.UnixMilli(), 10),
		"queries": r.Queries,
	}
	var wire struct {
		Results map[string]struct {
			Error  string      `json:"error"`
			Frames []wireFrame `json:"frames"`
		} `json:"results"`
	}
	if err := c.do(ctx, http.MethodPost, "/api/ds/query", body, &wire); err != nil {
		return Result{}, err
	}
	res := Result{Errors: map[string]string{}}
	order := make([]string, 0, len(r.Queries))
	for _, q := range r.Queries {
		order = append(order, q.RefID)
	}
	for ref := range wire.Results {
		if !contains(order, ref) {
			order = append(order, ref)
		}
	}
	for _, ref := range order {
		w, ok := wire.Results[ref]
		if !ok {
			continue
		}
		if w.Error != "" {
			res.Errors[ref] = c.redact(w.Error)
		}
		for _, f := range w.Frames {
			res.Series = append(res.Series, framesToSeries(ref, f)...)
		}
	}
	return res, nil
}

// framesToSeries turns one frame (a time field plus number fields) into one
// series per number field. Frames without a time field are skipped.
func framesToSeries(ref string, f wireFrame) []Series {
	ti := -1
	for i, fl := range f.Schema.Fields {
		if fl.Type == "time" {
			ti = i
			break
		}
	}
	if ti < 0 || ti >= len(f.Data.Values) {
		return nil
	}
	times := make([]time.Time, len(f.Data.Values[ti]))
	for i, n := range f.Data.Values[ti] {
		ms, _ := n.Int64()
		times[i] = time.UnixMilli(ms)
	}
	var out []Series
	for i, fl := range f.Schema.Fields {
		if fl.Type != "number" || i >= len(f.Data.Values) {
			continue
		}
		vals := make([]float64, len(f.Data.Values[i]))
		for j, n := range f.Data.Values[i] {
			if n == "" {
				vals[j] = math.NaN()
				continue
			}
			v, err := n.Float64()
			if err != nil {
				v = math.NaN()
			}
			vals[j] = v
		}
		name := fl.Config.DisplayNameFromDS
		if name == "" {
			name = fl.Name
		}
		out = append(out, Series{RefID: ref, Name: name, Labels: fl.Labels, Times: times, Values: vals})
	}
	return out
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// Resolve turns a dashboard reference into a uid: an exact uid, else the one
// exact title, else the one title equal ignoring case. Anything else is an
// error naming the candidates (spec E2).
func Resolve(ctx context.Context, c Client, ref string) (string, error) {
	hits, err := c.Search(ctx, ref)
	if err != nil {
		return "", err
	}
	all, err := c.Search(ctx, "")
	if err == nil {
		for _, h := range all {
			if h.UID == ref {
				return h.UID, nil
			}
		}
	}
	var exact, folded []Hit
	for _, h := range hits {
		switch {
		case h.Title == ref:
			exact = append(exact, h)
		case strings.EqualFold(h.Title, ref):
			folded = append(folded, h)
		}
	}
	switch {
	case len(exact) == 1:
		return exact[0].UID, nil
	case len(exact) == 0 && len(folded) == 1:
		return folded[0].UID, nil
	}
	cands := append(exact, folded...)
	if len(cands) == 0 {
		cands = hits
	}
	if len(cands) == 0 {
		return "", fmt.Errorf("no dashboard matches %q (gdash list shows them all)", ref)
	}
	names := make([]string, 0, len(cands))
	for _, h := range cands {
		names = append(names, fmt.Sprintf("%s (%s)", h.Title, h.UID))
	}
	sort.Strings(names)
	return "", fmt.Errorf("%q matches more than one dashboard; use a uid: %s", ref, strings.Join(names, "; "))
}

// FetchAll runs one query per request, at most MaxConcurrent at a time, and
// returns results in request order. A failed request's Result carries its
// error under refId "*".
func FetchAll(ctx context.Context, c Client, reqs []QueryRequest) []Result {
	out := make([]Result, len(reqs))
	sem := make(chan struct{}, MaxConcurrent)
	var wg sync.WaitGroup
	for i, r := range reqs {
		wg.Add(1)
		go func(i int, r QueryRequest) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			res, err := c.Query(ctx, r)
			if err != nil {
				res = Result{Errors: map[string]string{"*": err.Error()}}
			}
			out[i] = res
		}(i, r)
	}
	wg.Wait()
	return out
}

// errorMessage reads Grafana's error body: its JSON "message", else the
// first line of the text, at most 200 characters. Grafana's error bodies
// never echo request headers, so no credential can surface here.
func errorMessage(r io.Reader) string {
	b, _ := io.ReadAll(io.LimitReader(r, 1<<16))
	var j struct {
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	msg := ""
	if json.Unmarshal(b, &j) == nil {
		msg = j.Message
		if msg == "" {
			msg = j.Error
		}
	}
	if msg == "" {
		msg = strings.TrimSpace(strings.SplitN(string(b), "\n", 2)[0])
	}
	if len(msg) > 200 {
		msg = msg[:200] + "…"
	}
	return msg
}
