// Package grafanatest serves the lab fixtures (internal/grafana/testdata/lab)
// as a fake Grafana, so every gdash test runs against real dashboard JSON and
// real /api/ds/query responses without a network.
package grafanatest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// ErrorExpr is an expression the fake answers with a per-query error.
const ErrorExpr = "gdash_fixture_error"

// Server is a fake Grafana over the lab fixtures.
type Server struct {
	*httptest.Server
	// Delay holds every response this long (timeouts, concurrency tests).
	Delay time.Duration
	// OnQuery, when set, is called as each /api/ds/query starts and ends.
	OnQuery func(start bool)
	// Status, when non-zero, answers every request with this HTTP status,
	// and Message as its body.
	Status  int
	Message string
	// EchoAuth makes those error bodies repeat the request's Authorization
	// header, like a careless proxy (the E12 leak test).
	EchoAuth bool

	dir     string
	byExpr  map[string]json.RawMessage
	queries atomic.Int32
}

// Dir is the fixture directory.
func Dir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "testdata", "lab")
}

// New starts a fake Grafana; it is closed when the test ends. Every request
// except /api/health needs an Authorization header, or it gets a 401.
func New(t testing.TB) *Server {
	t.Helper()
	s := &Server{dir: Dir(), byExpr: map[string]json.RawMessage{}}
	files, _ := filepath.Glob(filepath.Join(s.dir, "queries", "*", "*.json"))
	for _, f := range files {
		var q struct {
			Request struct {
				Queries []struct {
					RefID string `json:"refId"`
					Expr  string `json:"expr"`
				} `json:"queries"`
			} `json:"request"`
			Response struct {
				Results map[string]json.RawMessage `json:"results"`
			} `json:"response"`
		}
		b, err := os.ReadFile(f) //nolint:gosec // fixture path from Glob
		if err != nil || json.Unmarshal(b, &q) != nil {
			t.Fatalf("fixture %s unreadable", f)
		}
		for _, rq := range q.Request.Queries {
			if r, ok := q.Response.Results[rq.RefID]; ok {
				s.byExpr[rq.Expr] = r
			}
		}
	}
	s.Server = httptest.NewServer(http.HandlerFunc(s.handle))
	t.Cleanup(s.Close)
	return s
}

// Queries is how many /api/ds/query requests the server has answered.
func (s *Server) Queries() int { return int(s.queries.Load()) }

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	if s.Delay > 0 {
		select {
		case <-time.After(s.Delay):
		case <-r.Context().Done():
			return
		}
	}
	if s.Status != 0 {
		msg := s.Message
		if msg == "" {
			msg = http.StatusText(s.Status)
		}
		if s.EchoAuth {
			msg = `{"message":"refused: ` + r.Header.Get("Authorization") + `"}`
		}
		http.Error(w, msg, s.Status)
		return
	}
	if r.URL.Path != "/api/health" && r.Header.Get("Authorization") == "" {
		http.Error(w, `{"message":"Unauthorized"}`, http.StatusUnauthorized)
		return
	}
	switch {
	case r.URL.Path == "/api/health":
		s.file(w, "health.json")
	case r.URL.Path == "/api/datasources":
		s.file(w, "datasources.json")
	case r.URL.Path == "/api/search":
		s.search(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/dashboards/uid/"):
		uid := strings.TrimPrefix(r.URL.Path, "/api/dashboards/uid/")
		if strings.ContainsAny(uid, "/.") {
			http.NotFound(w, r)
			return
		}
		s.file(w, filepath.Join("dashboards", uid+".json"))
	case r.URL.Path == "/api/ds/query" && r.Method == http.MethodPost:
		s.query(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) file(w http.ResponseWriter, name string) {
	b, err := os.ReadFile(filepath.Join(s.dir, name)) //nolint:gosec // fixed fixture names
	if err != nil {
		http.Error(w, `{"message":"Dashboard not found"}`, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(b) // #nosec G705 -- test fake: serves fixed fixture files under testdata as JSON
}

// search filters like Grafana: a case-insensitive substring of the title.
func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	b, _ := os.ReadFile(filepath.Join(s.dir, "search.json"))
	var hits []map[string]any
	_ = json.Unmarshal(b, &hits)
	q := strings.ToLower(r.URL.Query().Get("query"))
	out := []map[string]any{}
	for _, h := range hits {
		if title, _ := h["title"].(string); q == "" || strings.Contains(strings.ToLower(title), q) {
			out = append(out, h)
		}
	}
	if lim := r.URL.Query().Get("limit"); lim == "1" && len(out) > 1 {
		out = out[:1]
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func (s *Server) query(w http.ResponseWriter, r *http.Request) {
	s.queries.Add(1)
	if s.OnQuery != nil {
		s.OnQuery(true)
		defer s.OnQuery(false)
	}
	var req struct {
		Queries []struct {
			RefID string `json:"refId"`
			Expr  string `json:"expr"`
		} `json:"queries"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	results := map[string]json.RawMessage{}
	for _, q := range req.Queries {
		switch res, ok := s.byExpr[q.Expr]; {
		case q.Expr == ErrorExpr:
			results[q.RefID] = json.RawMessage(`{"status":400,"error":"bad_data: parse error"}`)
		case ok:
			results[q.RefID] = res
		default:
			results[q.RefID] = json.RawMessage(`{"status":200,"frames":[]}`)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"results": results})
}
