package git

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// RemoteWebURL returns the https web URL of dir's origin remote ("" dir = the
// process cwd), normalized by NormalizeRemote. One git exec; the caller threads
// the result through render.Deps so no segment has to shell out for it.
func RemoteWebURL(ctx context.Context, r Runner, dir string) (string, error) {
	args := buildArgs(dir, "remote", "get-url", "origin")
	out, err := r.Run(ctx, args[0], args[1:]...)
	if err != nil {
		return "", fmt.Errorf("git remote get-url origin: %w", err)
	}
	raw := strings.TrimSpace(string(out))
	web, ok := NormalizeRemote(raw)
	if !ok {
		return "", fmt.Errorf("git remote get-url origin: unrecognized remote %q", raw)
	}
	return web, nil
}

// NormalizeRemote maps the common remote spellings onto https://host/owner/repo:
// scp-like (git@host:o/r.git), ssh:// (with or without a port), https:// (with
// or without userinfo), and git://. Local paths and anything without a host
// yield ok=false.
func NormalizeRemote(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, ".") {
		return "", false
	}
	var host, path string
	switch {
	case strings.Contains(raw, "://"):
		u, err := url.Parse(raw)
		if err != nil || u.Hostname() == "" {
			return "", false
		}
		host, path = u.Hostname(), u.Path
	case strings.Contains(raw, "@") && strings.Contains(raw, ":"):
		hp := raw[strings.Index(raw, "@")+1:]
		i := strings.Index(hp, ":")
		if i < 0 {
			return "", false
		}
		host, path = hp[:i], hp[i+1:]
	default:
		return "", false
	}
	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	if host == "" || path == "" {
		return "", false
	}
	return "https://" + host + "/" + path, true
}
