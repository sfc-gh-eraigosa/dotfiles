package pins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Release is an upstream's newest stable version (no leading "v") and, when the
// source says so, when it shipped.
type Release struct {
	Version   string
	Published time.Time
}

// Resolver looks up the newest release for a manifest `upstream` value.
type Resolver interface {
	Latest(ctx context.Context, upstream string) (Release, error)
}

// ErrNoUpstream is returned for the `-` upstream: nothing to look up. Callers
// show it as "n/a", not as a failure.
var ErrNoUpstream = errors.New("no upstream")

// Default endpoints; HTTPResolver's fields override them so tests can point at
// httptest servers.
const (
	defaultGitHubAPI   = "https://api.github.com"
	defaultGoDownloads = "https://go.dev/dl/?mode=json"
	maxBody            = 1 << 20
)

// HTTPResolver implements Resolver for gh:, go: and text: upstreams.
type HTTPResolver struct {
	Client      *http.Client
	Bearer      string // GitHub bearer token; sent to GitHubAPI only
	GitHubAPI   string
	GoDownloads string
}

// NewHTTPResolver returns the production resolver. A nil client gets a default
// with a 15 s timeout (the caller's ctx still bounds each lookup). token may be
// empty; it is sent only to the GitHub API, never to go.dev or text: URLs.
func NewHTTPResolver(c *http.Client, token string) Resolver {
	if c == nil {
		c = &http.Client{Timeout: 15 * time.Second}
	}
	return &HTTPResolver{Client: c, Bearer: token, GitHubAPI: defaultGitHubAPI, GoDownloads: defaultGoDownloads}
}

// Latest resolves upstream: gh:<owner>/<repo>, go:, text:<url> or -.
func (r *HTTPResolver) Latest(ctx context.Context, upstream string) (Release, error) {
	switch {
	case upstream == "-" || upstream == "":
		return Release{}, ErrNoUpstream
	case strings.HasPrefix(upstream, "gh:"):
		return r.github(ctx, strings.TrimPrefix(upstream, "gh:"))
	case upstream == "go:":
		return r.golang(ctx)
	case strings.HasPrefix(upstream, "text:"):
		return r.text(ctx, strings.TrimPrefix(upstream, "text:"))
	}
	return Release{}, fmt.Errorf("pins: unsupported upstream %q", upstream)
}

func (r *HTTPResolver) get(ctx context.Context, url string, bearer bool) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "fleet-pins")
	if bearer && r.Bearer != "" {
		req.Header.Set("Authorization", "Bearer "+r.Bearer)
	}
	resp, err := r.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return body, nil
}

// cleanTag turns a release tag into a bare version: "v1.2.3" -> "1.2.3", and
// "release-2.6.0" -> "2.6.0" via ExtractVersion.
func cleanTag(tag string) (string, error) {
	t := strings.TrimPrefix(strings.TrimSpace(tag), "v")
	if versionRe.MatchString(t) {
		return t, nil
	}
	if v := ExtractVersion(t); v != "" {
		return v, nil
	}
	return "", fmt.Errorf("no version in %q", tag)
}

func (r *HTTPResolver) github(ctx context.Context, repo string) (Release, error) {
	if parts := strings.Split(repo, "/"); len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return Release{}, fmt.Errorf("pins: gh upstream wants owner/repo, got %q", repo)
	}
	body, err := r.get(ctx, r.GitHubAPI+"/repos/"+repo+"/releases/latest", true)
	if err != nil {
		return Release{}, err
	}
	var j struct {
		Tag       string `json:"tag_name"`
		Published string `json:"published_at"`
	}
	if err := json.Unmarshal(body, &j); err != nil {
		return Release{}, fmt.Errorf("gh:%s: %w", repo, err)
	}
	v, err := cleanTag(j.Tag)
	if err != nil {
		return Release{}, fmt.Errorf("gh:%s: %w", repo, err)
	}
	rel := Release{Version: v}
	if t, err := time.Parse(time.RFC3339, j.Published); err == nil {
		rel.Published = t
	}
	return rel, nil
}

func (r *HTTPResolver) golang(ctx context.Context) (Release, error) {
	body, err := r.get(ctx, r.GoDownloads, false)
	if err != nil {
		return Release{}, err
	}
	var rels []struct {
		Version string `json:"version"`
		Stable  bool   `json:"stable"`
	}
	if err := json.Unmarshal(body, &rels); err != nil {
		return Release{}, fmt.Errorf("go.dev: %w", err)
	}
	for _, e := range rels {
		if e.Stable {
			return Release{Version: strings.TrimPrefix(e.Version, "go")}, nil
		}
	}
	return Release{}, errors.New("go.dev: no stable release listed")
}

func (r *HTTPResolver) text(ctx context.Context, url string) (Release, error) {
	body, err := r.get(ctx, url, false)
	if err != nil {
		return Release{}, err
	}
	first, _, _ := strings.Cut(strings.TrimSpace(string(body)), "\n")
	v, err := cleanTag(first)
	if err != nil {
		return Release{}, fmt.Errorf("text:%s: %w", url, err)
	}
	return Release{Version: v}, nil
}
