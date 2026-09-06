package gh

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Source names where a credential came from. It is printed in `auth status`
// and in error messages; it never carries the credential itself.
type Source int

// The chain, in the order Resolve walks it (plan §3.2).
const (
	SourceNone Source = iota
	SourceEnv
	SourceGHToken
	SourceGHLogin
	SourceApp
)

// String is the human name of a source.
func (s Source) String() string {
	switch s {
	case SourceEnv:
		return "GH_TOKEN"
	case SourceGHToken:
		return "GITHUB_TOKEN"
	case SourceGHLogin:
		return "gh login"
	case SourceApp:
		return "GitHub App"
	default:
		return "none"
	}
}

// AuthOpts steers credential resolution.
type AuthOpts struct {
	// Prefer pins one source instead of walking the chain: env, gh, app, or
	// "" / auto for the chain.
	Prefer string
	// Owner and Repo scope an App installation token.
	Owner, Repo string
	// MintApp mints a GitHub App installation token (sdk/ghapp); nil when
	// no App is configured.
	MintApp func(ctx context.Context, owner, repo string) (string, error)
	// BaseURL and lookups are test seams.
	BaseURL string
}

// ErrNoCredential is returned when nothing in the chain has a credential.
var ErrNoCredential = errors.New("no GitHub credential")

// Resolve returns a Client authenticated by the first source in the chain
// that has a credential, and says which one it was.
func Resolve(ctx context.Context, o AuthOpts) (Client, Source, error) {
	_, src, tok, err := resolveToken(ctx, o)
	if err != nil {
		return nil, src, err
	}
	return NewREST(RESTOpts{Bearer: tok, BaseURL: o.BaseURL}), src, nil
}

// resolveToken is Resolve without building a client, so tests can assert on
// the source and the value without a server. The first return value is
// reserved for the resolved options a caller may want to log.
func resolveToken(ctx context.Context, o AuthOpts) (AuthOpts, Source, string, error) {
	env := func() (string, bool) {
		if v := strings.TrimSpace(os.Getenv("GH_TOKEN")); v != "" {
			return v, true
		}
		return "", false
	}
	githubEnv := func() (string, bool) {
		if v := strings.TrimSpace(os.Getenv("GITHUB_TOKEN")); v != "" {
			return v, true
		}
		return "", false
	}
	app := func() (string, bool) {
		if o.MintApp == nil {
			return "", false
		}
		v, err := o.MintApp(ctx, o.Owner, o.Repo)
		if err != nil || strings.TrimSpace(v) == "" {
			return "", false
		}
		return v, true
	}

	switch strings.ToLower(strings.TrimSpace(o.Prefer)) {
	case "", "auto":
		if v, ok := env(); ok {
			return o, SourceEnv, v, nil
		}
		if v, ok := githubEnv(); ok {
			return o, SourceGHToken, v, nil
		}
		if v, ok := ghLoginToken(); ok {
			return o, SourceGHLogin, v, nil
		}
		if v, ok := app(); ok {
			return o, SourceApp, v, nil
		}
		return o, SourceNone, "", fmt.Errorf("%w: set GH_TOKEN, run `gh auth login`, or create a GitHub App with `ghapp create` (see `gcfg auth doctor`)", ErrNoCredential)
	case "env":
		if v, ok := env(); ok {
			return o, SourceEnv, v, nil
		}
		if v, ok := githubEnv(); ok {
			return o, SourceGHToken, v, nil
		}
		return o, SourceNone, "", fmt.Errorf("%w: --auth env, but neither GH_TOKEN nor GITHUB_TOKEN is set", ErrNoCredential)
	case "gh":
		if v, ok := ghLoginToken(); ok {
			return o, SourceGHLogin, v, nil
		}
		return o, SourceNone, "", fmt.Errorf("%w: --auth gh, but no token in gh's hosts.yml — run `gh auth login`", ErrNoCredential)
	case "app":
		if v, ok := app(); ok {
			return o, SourceApp, v, nil
		}
		return o, SourceNone, "", fmt.Errorf("%w: --auth app, but no GitHub App could mint a token — run `ghapp create` then `ghapp install`", ErrNoCredential)
	default:
		return o, SourceNone, "", fmt.Errorf("unknown --auth %q: use env, gh, app, or auto", o.Prefer)
	}
}

// ghHosts is the part of gh's hosts.yml gcfg reads.
type ghHosts map[string]struct {
	OAuthToken string `yaml:"oauth_token"`
	User       string `yaml:"user"`
}

// askGH is the seam for `gh auth token`; tests replace it so no test needs
// gh installed.
var askGH = func() (string, error) {
	out, err := exec.Command("gh", "auth", "token").Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// ghLoginToken returns the credential `gh auth login` established. gh keeps
// it either in hosts.yml or in the system keyring (what `gh auth status`
// prints as "(keyring)"), so the file is read first and gh itself is asked
// only when the file has nothing — reading a file beats spawning a process,
// and a keyring login is still a perfectly good credential.
func ghLoginToken() (string, bool) {
	if v := hostsFileToken(); v != "" {
		return v, true
	}
	v, err := askGH()
	if err != nil {
		return "", false
	}
	v = strings.TrimSpace(v)
	if v == "" {
		return "", false
	}
	return v, true
}

// hostsFileToken reads gh's hosts.yml, returning "" when there is none.
func hostsFileToken() string {
	path := ghHostsPath()
	if path == "" {
		return ""
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var hosts ghHosts
	if err := yaml.Unmarshal(b, &hosts); err != nil {
		return ""
	}
	return strings.TrimSpace(hosts["github.com"].OAuthToken)
}

// ghHostsPath honours GH_CONFIG_DIR, then XDG_CONFIG_HOME, then ~/.config.
func ghHostsPath() string {
	if d := os.Getenv("GH_CONFIG_DIR"); d != "" {
		return filepath.Join(d, "hosts.yml")
	}
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "gh", "hosts.yml")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "gh", "hosts.yml")
}

// remoteRE matches the GitHub remotes git actually writes.
var remoteRE = regexp.MustCompile(`^(?:https://github\.com/|git@github\.com:|ssh://git@github\.com/)([^/]+)/(.+?)(?:\.git)?$`)

// ParseRemote pulls owner and repo out of a git remote URL.
func ParseRemote(remote string) (owner, repo string, err error) {
	m := remoteRE.FindStringSubmatch(strings.TrimSpace(remote))
	if m == nil {
		return "", "", fmt.Errorf("cannot read a github.com owner/repo out of remote %q — pass -R owner/repo", remote)
	}
	return m[1], m[2], nil
}
