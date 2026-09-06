// Package security manages the repository's security-and-analysis settings:
// secret scanning and push protection, non-provider patterns, Dependabot
// alerts and security updates, and private vulnerability reporting.
//
// These live behind three different endpoint shapes, and one of them lies:
// GitHub accepts a write to non_provider_patterns and leaves it disabled
// when the plan has no Secret Protection. That is why apply re-reads and
// this family can report not_honoured (design §7).
package security

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gcfg/internal/family"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gcfg/internal/gh"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gcfg/internal/schema"
	"gopkg.in/yaml.v3"
)

// Live is the security state gcfg manages.
type Live struct {
	SecretScanning                bool
	PushProtection                bool
	NonProviderPatterns           bool
	DependabotAlerts              bool
	DependabotSecurityUpdates     bool
	PrivateVulnerabilityReporting bool
}

// repoView is the shape of the repository JSON this family reads.
type repoView struct {
	SecurityAndAnalysis struct {
		SecretScanning                    status `json:"secret_scanning"`
		SecretScanningPushProtection      status `json:"secret_scanning_push_protection"`
		SecretScanningNonProviderPatterns status `json:"secret_scanning_non_provider_patterns"`
		DependabotSecurityUpdates         status `json:"dependabot_security_updates"`
	} `json:"security_and_analysis"`
}

type status struct {
	Status string `json:"status"`
}

func (s status) on() bool { return s.Status == "enabled" }

// Family manages the security settings.
type Family struct{}

// New returns the family.
func New() Family { return Family{} }

func init() { family.Register(New()) }

// Name is the key in gcfg.yaml.
func (Family) Name() string { return "security" }

// Scope is the repository.
func (Family) Scope() family.Scope { return family.ScopeRepo }

// Permission is what its writes need.
func (Family) Permission() string { return "repo:Administration:write" }

// Read combines the repository's security_and_analysis block with the two
// endpoints that answer only with a status code.
func (Family) Read(ctx context.Context, c gh.Client, t family.Target) (family.Live, error) {
	var view repoView
	if _, err := c.Do(ctx, http.MethodGet, "/repos/"+t.String(), nil, &view); err != nil {
		return nil, err
	}
	sa := view.SecurityAndAnalysis
	live := &Live{
		SecretScanning:            sa.SecretScanning.on(),
		PushProtection:            sa.SecretScanningPushProtection.on(),
		NonProviderPatterns:       sa.SecretScanningNonProviderPatterns.on(),
		DependabotSecurityUpdates: sa.DependabotSecurityUpdates.on(),
	}
	// These two answer 204 when enabled and 404 when not; a 404 is an
	// answer, not a failure.
	live.DependabotAlerts = enabled(ctx, c, "/repos/"+t.String()+"/vulnerability-alerts")
	live.PrivateVulnerabilityReporting = enabled(ctx, c, "/repos/"+t.String()+"/private-vulnerability-reporting")
	return live, nil
}

// enabled treats 2xx as on and anything else as off.
func enabled(ctx context.Context, c gh.Client, path string) bool {
	status, err := c.Do(ctx, http.MethodGet, path, nil, nil)
	return err == nil && status >= 200 && status < 300
}

// field ties a gcfg key to how it is read and how it is written.
type field struct {
	key string
	api string // key inside security_and_analysis, "" for its own endpoint
	get func(*Live) bool
	set func(*Live, bool)
}

var fields = []field{
	{key: "secret_scanning", api: "secret_scanning", get: func(l *Live) bool { return l.SecretScanning }, set: func(l *Live, v bool) { l.SecretScanning = v }},
	{key: "push_protection", api: "secret_scanning_push_protection", get: func(l *Live) bool { return l.PushProtection }, set: func(l *Live, v bool) { l.PushProtection = v }},
	{key: "non_provider_patterns", api: "secret_scanning_non_provider_patterns", get: func(l *Live) bool { return l.NonProviderPatterns }, set: func(l *Live, v bool) { l.NonProviderPatterns = v }},
	{key: "dependabot_security_updates", api: "dependabot_security_updates", get: func(l *Live) bool { return l.DependabotSecurityUpdates }, set: func(l *Live, v bool) { l.DependabotSecurityUpdates = v }},
	{key: "dependabot_alerts", get: func(l *Live) bool { return l.DependabotAlerts }, set: func(l *Live, v bool) { l.DependabotAlerts = v }},
	{key: "private_vulnerability_reporting", get: func(l *Live) bool { return l.PrivateVulnerabilityReporting }, set: func(l *Live, v bool) { l.PrivateVulnerabilityReporting = v }},
}

// endpoints for the two settings that are not part of the PATCH.
var ownEndpoint = map[string]string{
	"dependabot_alerts":               "/vulnerability-alerts",
	"private_vulnerability_reporting": "/private-vulnerability-reporting",
}

// notHonouredReason explains a write GitHub accepted but did not apply.
var notHonouredReason = map[string]string{
	"non_provider_patterns": "GitHub accepted the write but the setting stayed off — non-provider patterns need GitHub Secret Protection on this plan",
}

// Export renders live state as the node under repo.security.
func (Family) Export(live family.Live) (*yaml.Node, error) {
	l, ok := live.(*Live)
	if !ok {
		return nil, fmt.Errorf("security: unexpected live type %T", live)
	}
	pairs := make([]any, 0, len(fields)*2)
	for _, f := range fields {
		pairs = append(pairs, f.key, family.Scalar(f.get(l)))
	}
	return family.Map(pairs...), nil
}

// Diff compares declared values with live state.
func (f Family) Diff(desired *yaml.Node, live family.Live, own schema.Ownership) ([]family.Finding, []family.Change) {
	return f.diff(desired, live, nil)
}

// DiffAfterApply is Diff with the changes apply just made in hand: a
// setting that still disagrees was accepted and ignored, which is a
// not_honoured finding rather than drift that would never clear.
func (f Family) DiffAfterApply(desired *yaml.Node, live family.Live, own schema.Ownership, applied []family.Change) ([]family.Finding, []family.Change) {
	return f.diff(desired, live, applied)
}

func (f Family) diff(desired *yaml.Node, live family.Live, applied []family.Change) ([]family.Finding, []family.Change) {
	d := family.NewDiffer(f.Name())
	l, ok := live.(*Live)
	if !ok {
		return d.Result()
	}
	wasApplied := map[string]bool{}
	for _, c := range applied {
		wasApplied[c.Key] = true
	}
	for _, fld := range fields {
		want, declared := family.Bool(desired, fld.key)
		if !declared {
			continue
		}
		got := fld.get(l)
		if want == got {
			continue
		}
		if wasApplied[fld.key] {
			reason := notHonouredReason[fld.key]
			if reason == "" {
				reason = "GitHub accepted the write but the setting did not change"
			}
			d.Report(family.Finding{Key: fld.key, Kind: family.NotHonoured, Want: want, Live: got, Reason: reason})
			continue
		}
		d.Scalar(fld.key, true, want, got)
	}
	return d.Result()
}

// Apply writes one security_and_analysis PATCH plus the PUT/DELETE pairs.
func (f Family) Apply(ctx context.Context, c gh.Client, t family.Target, changes []family.Change) error {
	sa := map[string]any{}
	for _, ch := range changes {
		want, _ := ch.Want.(bool)
		if path, own := ownEndpoint[ch.Key]; own {
			method := http.MethodDelete
			if want {
				method = http.MethodPut
			}
			if _, err := c.Do(ctx, method, "/repos/"+t.String()+path, nil, nil); err != nil {
				return err
			}
			continue
		}
		api, ok := apiField(ch.Key)
		if !ok {
			return fmt.Errorf("security: no GitHub field for %q", ch.Key)
		}
		sa[api] = map[string]any{"status": statusWord(want)}
	}
	if len(sa) > 0 {
		body := map[string]any{"security_and_analysis": sa}
		if _, err := c.Do(ctx, http.MethodPatch, "/repos/"+t.String(), body, nil); err != nil {
			return err
		}
	}
	return nil
}

func apiField(key string) (string, bool) {
	for _, f := range fields {
		if f.key == key && f.api != "" {
			return f.api, true
		}
	}
	return "", false
}

func statusWord(on bool) string {
	if on {
		return "enabled"
	}
	return "disabled"
}

// String makes a Live readable in a test failure.
func (l *Live) String() string {
	var on []string
	for _, f := range fields {
		if f.get(l) {
			on = append(on, f.key)
		}
	}
	return "security{" + strings.Join(on, ",") + "}"
}
