package gh

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gcfg/internal/version"
)

// DefaultBaseURL is GitHub's REST API.
const DefaultBaseURL = "https://api.github.com"

// maxAttempts is one try plus three retries; retries only ever apply to 5xx
// and rate-limit responses, never to a write GitHub already accepted.
const maxAttempts = 4

// RESTOpts configures the real client.
type RESTOpts struct {
	Bearer  string
	BaseURL string
	HTTP    *http.Client
	// Sleep is the backoff hook; tests replace it so retries cost nothing.
	Sleep func(time.Duration)
}

type rest struct {
	bearer  string
	baseURL string
	http    *http.Client
	sleep   func(time.Duration)
}

// NewREST builds the production client.
func NewREST(o RESTOpts) Client {
	c := &rest{baseURL: o.BaseURL, http: o.HTTP, sleep: o.Sleep}
	c.bearer = o.Bearer
	if c.baseURL == "" {
		c.baseURL = DefaultBaseURL
	}
	if c.http == nil {
		c.http = &http.Client{Timeout: 30 * time.Second}
	}
	if c.sleep == nil {
		c.sleep = time.Sleep
	}
	return c
}

// Do performs one request, retrying 5xx and rate-limit answers.
func (c *rest) Do(ctx context.Context, method, path string, body, out any) (int, error) {
	var payload []byte
	if body != nil {
		var err error
		if payload, err = json.Marshal(body); err != nil {
			return 0, fmt.Errorf("encoding request body for %s %s: %w", method, path, err)
		}
	}
	status, raw, _, err := c.attempt(ctx, method, path, payload)
	if err != nil {
		return status, err
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return status, fmt.Errorf("decoding %s %s: %w", method, path, err)
		}
	}
	return status, nil
}

// attempt runs the request with retries and returns status, body, and the
// Link header.
func (c *rest) attempt(ctx context.Context, method, path string, payload []byte) (int, []byte, string, error) {
	url := path
	if !strings.HasPrefix(path, "http") {
		url = c.baseURL + path
	}
	var lastStatus int
	var lastErr error
	for i := 0; i < maxAttempts; i++ {
		var rdr io.Reader
		if payload != nil {
			rdr = bytes.NewReader(payload)
		}
		req, err := http.NewRequestWithContext(ctx, method, url, rdr)
		if err != nil {
			return 0, nil, "", fmt.Errorf("%s %s: %w", method, path, err)
		}
		if c.bearer != "" {
			req.Header.Set("Authorization", "Bearer "+c.bearer)
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		req.Header.Set("User-Agent", "gcfg/"+version.Version)
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		res, err := c.http.Do(req)
		if err != nil {
			// A cancelled context is final; a transport error is worth
			// another try.
			if ctx.Err() != nil {
				return 0, nil, "", fmt.Errorf("%s %s: %w", method, path, ctx.Err())
			}
			lastErr = fmt.Errorf("%s %s: %w", method, path, err)
			if i == maxAttempts-1 {
				return 0, nil, "", lastErr
			}
			c.sleep(backoff(i, ""))
			continue
		}
		raw, readErr := io.ReadAll(io.LimitReader(res.Body, 8<<20))
		link := res.Header.Get("Link")
		retryAfter := res.Header.Get("Retry-After")
		res.Body.Close()
		if readErr != nil {
			return res.StatusCode, nil, link, fmt.Errorf("%s %s: reading response: %w", method, path, readErr)
		}
		lastStatus = res.StatusCode
		if retryable(res.StatusCode) && i < maxAttempts-1 {
			c.sleep(backoff(i, retryAfter))
			continue
		}
		if res.StatusCode < 200 || res.StatusCode > 299 {
			return res.StatusCode, raw, link, fmt.Errorf("%s %s: HTTP %d %s", method, path, res.StatusCode, message(raw))
		}
		return res.StatusCode, raw, link, nil
	}
	if lastErr != nil {
		return lastStatus, nil, "", lastErr
	}
	return lastStatus, nil, "", fmt.Errorf("%s %s: gave up after %d attempts (HTTP %d)", method, path, maxAttempts, lastStatus)
}

// retryable is true for the answers that mean "not now" rather than "no".
func retryable(status int) bool {
	return status >= 500 || status == http.StatusTooManyRequests
}

// backoff prefers GitHub's own Retry-After, else exponential 0.5s, 1s, 2s.
func backoff(attempt int, retryAfter string) time.Duration {
	if retryAfter != "" {
		if secs, err := strconv.Atoi(strings.TrimSpace(retryAfter)); err == nil && secs >= 0 {
			return time.Duration(secs) * time.Second
		}
	}
	return time.Duration(math.Pow(2, float64(attempt))*500) * time.Millisecond
}

// message pulls GitHub's own error text out of a response body. The body is
// never returned wholesale: it can echo what was sent.
func message(raw []byte) string {
	var body struct {
		Message string `json:"message"`
		Errors  []struct {
			Field   string `json:"field"`
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(raw, &body); err != nil || body.Message == "" {
		return "(no message)"
	}
	var parts []string
	for _, e := range body.Errors {
		switch {
		case e.Message != "":
			parts = append(parts, e.Message)
		case e.Field != "":
			parts = append(parts, e.Field+" "+e.Code)
		}
	}
	if len(parts) == 0 {
		return body.Message
	}
	return body.Message + " (" + strings.Join(parts, "; ") + ")"
}

var nextLinkRE = regexp.MustCompile(`<([^>]+)>;\s*rel="next"`)

// Paginate walks every page of a list endpoint, calling each once per
// element. It stops at the first error, from GitHub or from each.
func (c *rest) Paginate(ctx context.Context, path string, each func(json.RawMessage) error) error {
	url := withPerPage(path)
	for url != "" {
		_, raw, link, err := c.attempt(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		var page []json.RawMessage
		if err := json.Unmarshal(raw, &page); err != nil {
			return fmt.Errorf("GET %s: decoding page: %w", path, err)
		}
		for _, item := range page {
			if err := each(item); err != nil {
				return err
			}
		}
		url = ""
		if m := nextLinkRE.FindStringSubmatch(link); m != nil {
			url = m[1]
		}
	}
	return nil
}

// withPerPage asks for the largest page GitHub allows.
func withPerPage(path string) string {
	if strings.Contains(path, "per_page=") {
		return path
	}
	if strings.Contains(path, "?") {
		return path + "&per_page=100"
	}
	return path + "?per_page=100"
}
