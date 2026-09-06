package ghapp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// RepoAccess probes GET /repos/{ownerRepo} with an installation token and
// returns the permissions GitHub reports for it (admin/push/pull…).
func RepoAccess(ctx context.Context, hc *http.Client, apiURL string, tok Token, ownerRepo string) (map[string]bool, error) {
	if hc == nil {
		hc = http.DefaultClient
	}
	if apiURL == "" {
		apiURL = "https://api.github.com"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL+"/repos/"+ownerRepo, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok.Value)
	req.Header.Set("Accept", "application/vnd.github+json")
	res, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ghapp: GET /repos/%s: %w", ownerRepo, err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	var body struct {
		Permissions map[string]bool `json:"permissions"`
		Message     string          `json:"message"`
	}
	_ = json.Unmarshal(raw, &body)
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("ghapp: GET /repos/%s: HTTP %d %s", ownerRepo, res.StatusCode, body.Message)
	}
	return body.Permissions, nil
}
