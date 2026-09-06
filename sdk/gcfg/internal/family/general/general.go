// Package general is the repository's own metadata and merge behaviour:
// description, homepage, topics, visibility, default branch, the feature
// tabs, and the pull-request merge settings.
package general

import (
	"context"
	"fmt"
	"net/http"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gcfg/internal/family"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gcfg/internal/gh"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gcfg/internal/schema"
	"gopkg.in/yaml.v3"
)

// Live is the slice of GET /repos/{owner}/{repo} this family manages.
type Live struct {
	Description              string   `json:"description"`
	Homepage                 string   `json:"homepage"`
	Topics                   []string `json:"topics"`
	Visibility               string   `json:"visibility"`
	DefaultBranch            string   `json:"default_branch"`
	HasIssues                bool     `json:"has_issues"`
	HasProjects              bool     `json:"has_projects"`
	HasWiki                  bool     `json:"has_wiki"`
	HasDiscussions           bool     `json:"has_discussions"`
	AllowSquashMerge         bool     `json:"allow_squash_merge"`
	AllowMergeCommit         bool     `json:"allow_merge_commit"`
	AllowRebaseMerge         bool     `json:"allow_rebase_merge"`
	AllowAutoMerge           bool     `json:"allow_auto_merge"`
	DeleteBranchOnMerge      bool     `json:"delete_branch_on_merge"`
	AllowUpdateBranch        bool     `json:"allow_update_branch"`
	SquashTitle              string   `json:"squash_merge_commit_title"`
	SquashMessage            string   `json:"squash_merge_commit_message"`
	WebCommitSignoffRequired bool     `json:"web_commit_signoff_required"`
	AllowForking             bool     `json:"allow_forking"`
}

// Family manages the general settings.
type Family struct{}

// New returns the family.
func New() Family { return Family{} }

func init() { family.Register(New()) }

// Name is the key in gcfg.yaml.
func (Family) Name() string { return "general" }

// Scope is the repository.
func (Family) Scope() family.Scope { return family.ScopeRepo }

// Permission is what its writes need.
func (Family) Permission() string { return "repo:Administration:write" }

// Read fetches the repository.
func (Family) Read(ctx context.Context, c gh.Client, t family.Target) (family.Live, error) {
	var live Live
	if _, err := c.Do(ctx, http.MethodGet, "/repos/"+t.String(), nil, &live); err != nil {
		return nil, err
	}
	return &live, nil
}

// key/value plumbing shared by Export and Diff, so the two can never drift
// apart: one table naming each setting, how to read it from Live, and how
// GitHub spells it in a PATCH.
type field struct {
	key    string // path in gcfg.yaml, e.g. features.wiki
	api    string // GitHub's field name in the repo PATCH
	get    func(*Live) any
	target string // "" repo PATCH, "topics" its own endpoint
}

var fields = []field{
	{key: "description", api: "description", get: func(l *Live) any { return l.Description }},
	{key: "homepage", api: "homepage", get: func(l *Live) any { return l.Homepage }},
	{key: "visibility", api: "visibility", get: func(l *Live) any { return l.Visibility }},
	{key: "default_branch", api: "default_branch", get: func(l *Live) any { return l.DefaultBranch }},
	{key: "features.issues", api: "has_issues", get: func(l *Live) any { return l.HasIssues }},
	{key: "features.projects", api: "has_projects", get: func(l *Live) any { return l.HasProjects }},
	{key: "features.wiki", api: "has_wiki", get: func(l *Live) any { return l.HasWiki }},
	{key: "features.discussions", api: "has_discussions", get: func(l *Live) any { return l.HasDiscussions }},
	{key: "merge.squash", api: "allow_squash_merge", get: func(l *Live) any { return l.AllowSquashMerge }},
	{key: "merge.merge_commit", api: "allow_merge_commit", get: func(l *Live) any { return l.AllowMergeCommit }},
	{key: "merge.rebase", api: "allow_rebase_merge", get: func(l *Live) any { return l.AllowRebaseMerge }},
	{key: "merge.auto_merge", api: "allow_auto_merge", get: func(l *Live) any { return l.AllowAutoMerge }},
	{key: "merge.delete_branch_on_merge", api: "delete_branch_on_merge", get: func(l *Live) any { return l.DeleteBranchOnMerge }},
	{key: "merge.allow_update_branch", api: "allow_update_branch", get: func(l *Live) any { return l.AllowUpdateBranch }},
	{key: "merge.squash_title", api: "squash_merge_commit_title", get: func(l *Live) any { return l.SquashTitle }},
	{key: "merge.squash_message", api: "squash_merge_commit_message", get: func(l *Live) any { return l.SquashMessage }},
	{key: "web_commit_signoff_required", api: "web_commit_signoff_required", get: func(l *Live) any { return l.WebCommitSignoffRequired }},
	{key: "allow_forking", api: "allow_forking", get: func(l *Live) any { return l.AllowForking }},
}

// apiField maps a gcfg key to GitHub's PATCH field name.
func apiField(key string) (string, bool) {
	for _, f := range fields {
		if f.key == key {
			return f.api, true
		}
	}
	return "", false
}

// Export renders live state as the node that belongs under repo.general.
func (Family) Export(live family.Live) (*yaml.Node, error) {
	l, ok := live.(*Live)
	if !ok {
		return nil, fmt.Errorf("general: unexpected live type %T", live)
	}
	var topics *yaml.Node
	if len(l.Topics) > 0 {
		topics = family.Seq(l.Topics)
	}
	var homepage *yaml.Node
	if l.Homepage != "" {
		homepage = family.Scalar(l.Homepage)
	}
	var description *yaml.Node
	if l.Description != "" {
		description = family.Scalar(l.Description)
	}
	return family.Map(
		"description", description,
		"homepage", homepage,
		"topics", topics,
		"visibility", family.Scalar(l.Visibility),
		"default_branch", family.Scalar(l.DefaultBranch),
		"features", family.Map(
			"issues", family.Scalar(l.HasIssues),
			"projects", family.Scalar(l.HasProjects),
			"wiki", family.Scalar(l.HasWiki),
			"discussions", family.Scalar(l.HasDiscussions),
		),
		"merge", family.Map(
			"squash", family.Scalar(l.AllowSquashMerge),
			"merge_commit", family.Scalar(l.AllowMergeCommit),
			"rebase", family.Scalar(l.AllowRebaseMerge),
			"auto_merge", family.Scalar(l.AllowAutoMerge),
			"delete_branch_on_merge", family.Scalar(l.DeleteBranchOnMerge),
			"allow_update_branch", family.Scalar(l.AllowUpdateBranch),
			"squash_title", family.Scalar(l.SquashTitle),
			"squash_message", family.Scalar(l.SquashMessage),
		),
		"web_commit_signoff_required", family.Scalar(l.WebCommitSignoffRequired),
		"allow_forking", family.Scalar(l.AllowForking),
	), nil
}

// Diff compares the declared node with live state. Undeclared keys are left
// alone under either ownership: this family is a fixed set of scalars, so
// there is no such thing as an extra one (that is a list family's problem).
func (f Family) Diff(desired *yaml.Node, live family.Live, own schema.Ownership) ([]family.Finding, []family.Change) {
	d := family.NewDiffer(f.Name())
	l, ok := live.(*Live)
	if !ok {
		return d.Result()
	}
	for _, fld := range fields {
		want, declared := declaredValue(desired, fld.key, fld.get(l))
		d.Scalar(fld.key, declared, want, fld.get(l))
	}
	if topics, declared := family.Strings(desired, "topics"); declared {
		d.List("topics", true, topics, l.Topics)
	}
	return d.Result()
}

// declaredValue reads the declared value for a dotted key, typed like the
// live value so a comparison is meaningful.
func declaredValue(n *yaml.Node, key string, live any) (any, bool) {
	node := n
	name := key
	if i := indexByte(key, '.'); i >= 0 {
		sub, ok := family.Field(n, key[:i])
		if !ok {
			return nil, false
		}
		node, name = sub, key[i+1:]
	}
	switch live.(type) {
	case bool:
		return family.Bool(node, name)
	default:
		return family.Str(node, name)
	}
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

// Apply writes the changes: one PATCH for the repository fields, plus the
// topics endpoint when topics changed. Only changed keys are sent — a PATCH
// replaces the whole object, so echoing an unchanged value would clobber a
// concurrent edit.
func (f Family) Apply(ctx context.Context, c gh.Client, t family.Target, changes []family.Change) error {
	patch := map[string]any{}
	var topics []string
	haveTopics := false
	for _, ch := range changes {
		if ch.Key == "topics" {
			if ss, ok := ch.Want.([]string); ok {
				topics, haveTopics = ss, true
			}
			continue
		}
		api, ok := apiField(ch.Key)
		if !ok {
			return fmt.Errorf("general: no GitHub field for %q", ch.Key)
		}
		patch[api] = ch.Want
	}
	if len(patch) > 0 {
		if _, err := c.Do(ctx, http.MethodPatch, "/repos/"+t.String(), patch, nil); err != nil {
			return err
		}
	}
	if haveTopics {
		if _, err := c.Do(ctx, http.MethodPut, "/repos/"+t.String()+"/topics", map[string]any{"names": topics}, nil); err != nil {
			return err
		}
	}
	return nil
}
