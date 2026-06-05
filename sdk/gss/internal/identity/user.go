package identity

import (
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/errors"
)

// UserSources supplies the inputs ResolveUser draws from. The external
// lookups are injected so callers wire real `gh`/`git`/env access and
// tests pin deterministic values. A nil func is treated as "unavailable"
// and skipped.
type UserSources struct {
	// Override is the --user flag value; when non-empty it wins outright
	// (after validation).
	Override string
	// GHLogin returns `gh api user --jq .login`.
	GHLogin func() (string, error)
	// GitEmail returns `git config user.email`.
	GitEmail func() (string, error)
	// Getenv resolves environment variables (for $USER); nil skips it.
	Getenv func(string) string
}

// ResolveUser resolves the worker <user> segment by precedence
// (design.md → "Worker identity"):
//
//	--user override  →  gh login  →  slug(git email)  →  $USER
//
// Each candidate is slugified and must satisfy ValidateUser; the first
// valid one wins. If the override is supplied it must validate (an invalid
// explicit --user is an error, not a silent fall-through). If nothing
// resolves, a *errors.ValidationError (wrapping ErrInvalidIdent) is
// returned telling the caller to pass --user.
func ResolveUser(src UserSources) (string, error) {
	if src.Override != "" {
		if err := ValidateUser(src.Override); err != nil {
			return "", err
		}
		return src.Override, nil
	}

	if src.GHLogin != nil {
		if login, err := src.GHLogin(); err == nil {
			if u := slugify(login); ValidateUser(u) == nil {
				return u, nil
			}
		}
	}

	if src.GitEmail != nil {
		if email, err := src.GitEmail(); err == nil {
			local := email
			if at := strings.IndexByte(email, '@'); at >= 0 {
				local = email[:at]
			}
			if u := slugify(local); ValidateUser(u) == nil {
				return u, nil
			}
		}
	}

	if src.Getenv != nil {
		if u := slugify(src.Getenv("USER")); ValidateUser(u) == nil {
			return u, nil
		}
	}

	return "", errors.NewValidationError("user", "could not resolve a valid user from gh login, git email, or $USER; pass --user")
}

// slugify lower-cases s, replaces every run of non-[a-z0-9] with a single
// hyphen, and trims leading/trailing hyphens. The result may still fail
// ValidateUser (e.g. starts with a digit, too short/long); callers check.
func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	b.Grow(len(s))
	prevHyphen := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevHyphen = false
		default:
			if !prevHyphen {
				b.WriteByte('-')
				prevHyphen = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}
