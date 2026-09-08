package ghapp

import (
	"context"
	"net/http"
	"regexp"
)

// Installation is one place the App is installed.
type Installation struct {
	ID                  int64
	Account             string // login of the user/org
	TargetType          string // "User" | "Organization"
	RepositorySelection string // "all" | "selected"
}

var nextLinkRE = regexp.MustCompile(`<([^>]+)>;\s*rel="next"`)

// Installations lists every installation of the App (paginated).
func (a App) Installations(ctx context.Context) ([]Installation, error) {
	var out []Installation
	url := "/app/installations?per_page=100"
	for url != "" {
		var page []struct {
			ID      int64 `json:"id"`
			Account struct {
				Login string `json:"login"`
			} `json:"account"`
			TargetType          string `json:"target_type"`
			RepositorySelection string `json:"repository_selection"`
		}
		link, err := a.appCall(ctx, http.MethodGet, url, nil, &page)
		if err != nil {
			return nil, err
		}
		for _, p := range page {
			out = append(out, Installation{ID: p.ID, Account: p.Account.Login, TargetType: p.TargetType, RepositorySelection: p.RepositorySelection})
		}
		url = ""
		if m := nextLinkRE.FindStringSubmatch(link); m != nil {
			url = m[1]
		}
	}
	return out, nil
}
