// Package schema is gcfg's typed view of .github/gcfg.yaml: the structs the
// file maps onto (plan §3.1), a strict loader that rejects unknown keys, a
// linter for what types cannot express, and a JSON Schema generated from the
// same structs.
//
// Every settings field is a pointer (or a nil-able list): absent means
// "unmanaged", which is what makes `ownership: declared` possible. Nothing
// here talks to GitHub.
package schema

// Ownership decides what an undeclared live setting means.
type Ownership string

const (
	// Declared (default): only declared keys are managed; extras are
	// reported as unmanaged, never removed.
	Declared Ownership = "declared"
	// Full: the file is the whole truth; extras are drift and apply removes
	// them.
	Full Ownership = "full"
)

// Ownerships is every legal value (used by lint and the JSON Schema).
var Ownerships = []string{string(Declared), string(Full)}

// resolve returns own when set, else the file-level fallback (or Declared).
func resolve(own, fallback Ownership) Ownership {
	if own != "" {
		return own
	}
	if fallback != "" {
		return fallback
	}
	return Declared
}

// File is a whole gcfg.yaml.
type File struct {
	Version   int       `yaml:"version" json:"version" jsonschema:"required,the file format version; 1"`
	Ownership Ownership `yaml:"ownership,omitempty" json:"ownership,omitempty" jsonschema:"enum=ownership,default ownership for every family"`
	Repo      *Repo     `yaml:"repo,omitempty" json:"repo,omitempty" jsonschema:"settings of the repository this file lives in"`
	Org       *Org      `yaml:"org,omitempty" json:"org,omitempty" jsonschema:"settings of the owning organization; only in the org's .github repo"`
}

// Repo is the repository-scoped block.
type Repo struct {
	General       *General            `yaml:"general,omitempty" json:"general,omitempty"`
	Security      *Security           `yaml:"security,omitempty" json:"security,omitempty"`
	Actions       *Actions            `yaml:"actions,omitempty" json:"actions,omitempty"`
	Rulesets      *List[Ruleset]      `yaml:"rulesets,omitempty" json:"rulesets,omitempty"`
	Labels        *List[Label]        `yaml:"labels,omitempty" json:"labels,omitempty"`
	Autolinks     *List[Autolink]     `yaml:"autolinks,omitempty" json:"autolinks,omitempty"`
	Environments  *List[Environment]  `yaml:"environments,omitempty" json:"environments,omitempty"`
	Secrets       *Secrets            `yaml:"secrets,omitempty" json:"secrets,omitempty"`
	Webhooks      *List[Webhook]      `yaml:"webhooks,omitempty" json:"webhooks,omitempty"`
	Collaborators *List[Collaborator] `yaml:"collaborators,omitempty" json:"collaborators,omitempty"`
	Pages         *Pages              `yaml:"pages,omitempty" json:"pages,omitempty"`
}

// Org is the organization-scoped block.
type Org struct {
	Profile          *OrgProfile          `yaml:"profile,omitempty" json:"profile,omitempty"`
	Members          *OrgMembers          `yaml:"members,omitempty" json:"members,omitempty"`
	SecurityDefaults *OrgSecurityDefaults `yaml:"security_defaults,omitempty" json:"security_defaults,omitempty"`
	Actions          *OrgActions          `yaml:"actions,omitempty" json:"actions,omitempty"`
	Rulesets         *List[Ruleset]       `yaml:"rulesets,omitempty" json:"rulesets,omitempty"`
	Apps             *List[OrgApp]        `yaml:"apps,omitempty" json:"apps,omitempty"`
}

// General is the repository's own metadata and merge behaviour.
type General struct {
	Ownership                Ownership `yaml:"ownership,omitempty" json:"ownership,omitempty" jsonschema:"enum=ownership"`
	Description              *string   `yaml:"description,omitempty" json:"description,omitempty"`
	Homepage                 *string   `yaml:"homepage,omitempty" json:"homepage,omitempty"`
	Topics                   *[]string `yaml:"topics,omitempty" json:"topics,omitempty"`
	Visibility               *string   `yaml:"visibility,omitempty" json:"visibility,omitempty" jsonschema:"enum=visibility"`
	DefaultBranch            *string   `yaml:"default_branch,omitempty" json:"default_branch,omitempty"`
	Features                 *Features `yaml:"features,omitempty" json:"features,omitempty"`
	Merge                    *Merge    `yaml:"merge,omitempty" json:"merge,omitempty"`
	WebCommitSignoffRequired *bool     `yaml:"web_commit_signoff_required,omitempty" json:"web_commit_signoff_required,omitempty"`
	AllowForking             *bool     `yaml:"allow_forking,omitempty" json:"allow_forking,omitempty"`
}

// Own is General's effective ownership.
func (g *General) Own(fallback Ownership) Ownership { return resolve(g.Ownership, fallback) }

// Features are the repository's tabs.
type Features struct {
	Issues      *bool `yaml:"issues,omitempty" json:"issues,omitempty"`
	Projects    *bool `yaml:"projects,omitempty" json:"projects,omitempty"`
	Wiki        *bool `yaml:"wiki,omitempty" json:"wiki,omitempty"`
	Discussions *bool `yaml:"discussions,omitempty" json:"discussions,omitempty"`
}

// Merge is the pull-request merge configuration.
type Merge struct {
	Squash              *bool   `yaml:"squash,omitempty" json:"squash,omitempty"`
	MergeCommit         *bool   `yaml:"merge_commit,omitempty" json:"merge_commit,omitempty"`
	Rebase              *bool   `yaml:"rebase,omitempty" json:"rebase,omitempty"`
	AutoMerge           *bool   `yaml:"auto_merge,omitempty" json:"auto_merge,omitempty"`
	DeleteBranchOnMerge *bool   `yaml:"delete_branch_on_merge,omitempty" json:"delete_branch_on_merge,omitempty"`
	AllowUpdateBranch   *bool   `yaml:"allow_update_branch,omitempty" json:"allow_update_branch,omitempty"`
	SquashTitle         *string `yaml:"squash_title,omitempty" json:"squash_title,omitempty" jsonschema:"enum=squash_title"`
	SquashMessage       *string `yaml:"squash_message,omitempty" json:"squash_message,omitempty" jsonschema:"enum=squash_message"`
}

// Security is the repository's security-and-analysis block.
type Security struct {
	Ownership                     Ownership `yaml:"ownership,omitempty" json:"ownership,omitempty" jsonschema:"enum=ownership"`
	SecretScanning                *bool     `yaml:"secret_scanning,omitempty" json:"secret_scanning,omitempty"`
	PushProtection                *bool     `yaml:"push_protection,omitempty" json:"push_protection,omitempty"`
	NonProviderPatterns           *bool     `yaml:"non_provider_patterns,omitempty" json:"non_provider_patterns,omitempty"`
	DependabotAlerts              *bool     `yaml:"dependabot_alerts,omitempty" json:"dependabot_alerts,omitempty"`
	DependabotSecurityUpdates     *bool     `yaml:"dependabot_security_updates,omitempty" json:"dependabot_security_updates,omitempty"`
	PrivateVulnerabilityReporting *bool     `yaml:"private_vulnerability_reporting,omitempty" json:"private_vulnerability_reporting,omitempty"`
	CodeScanningDefaultSetup      *string   `yaml:"code_scanning_default_setup,omitempty" json:"code_scanning_default_setup,omitempty" jsonschema:"enum=code_scanning"`
}

// Own is Security's effective ownership.
func (s *Security) Own(fallback Ownership) Ownership { return resolve(s.Ownership, fallback) }

// Actions is the repository's Actions permissions.
type Actions struct {
	Ownership                    Ownership `yaml:"ownership,omitempty" json:"ownership,omitempty" jsonschema:"enum=ownership"`
	Enabled                      *bool     `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	AllowedActions               *string   `yaml:"allowed_actions,omitempty" json:"allowed_actions,omitempty" jsonschema:"enum=allowed_actions"`
	SHAPinningRequired           *bool     `yaml:"sha_pinning_required,omitempty" json:"sha_pinning_required,omitempty"`
	DefaultWorkflowPermissions   *string   `yaml:"default_workflow_permissions,omitempty" json:"default_workflow_permissions,omitempty" jsonschema:"enum=workflow_permissions"`
	CanApprovePullRequestReviews *bool     `yaml:"can_approve_pull_request_reviews,omitempty" json:"can_approve_pull_request_reviews,omitempty"`
}

// Own is Actions' effective ownership.
func (a *Actions) Own(fallback Ownership) Ownership { return resolve(a.Ownership, fallback) }

// Ruleset is one repository or organization ruleset.
type Ruleset struct {
	Name         string         `yaml:"name" json:"name"`
	Target       string         `yaml:"target,omitempty" json:"target,omitempty" jsonschema:"enum=ruleset_target"`
	Enforcement  string         `yaml:"enforcement,omitempty" json:"enforcement,omitempty" jsonschema:"enum=enforcement"`
	Conditions   *Conditions    `yaml:"conditions,omitempty" json:"conditions,omitempty"`
	BypassActors []BypassActor  `yaml:"bypass_actors,omitempty" json:"bypass_actors,omitempty"`
	Rules        []Rule         `yaml:"rules,omitempty" json:"rules,omitempty"`
	Extra        map[string]any `yaml:"-" json:"-"`
	_            struct{}       `yaml:"-" json:"-"`
}

// Conditions narrows which refs a ruleset applies to.
type Conditions struct {
	RefName *RefName `yaml:"ref_name,omitempty" json:"ref_name,omitempty"`
}

// RefName is the include/exclude ref filter.
type RefName struct {
	Include []string `yaml:"include,omitempty" json:"include,omitempty"`
	Exclude []string `yaml:"exclude,omitempty" json:"exclude,omitempty"`
}

// BypassActor may skip a ruleset.
type BypassActor struct {
	ActorID    *int64 `yaml:"actor_id,omitempty" json:"actor_id,omitempty"`
	ActorType  string `yaml:"actor_type,omitempty" json:"actor_type,omitempty"`
	BypassMode string `yaml:"bypass_mode,omitempty" json:"bypass_mode,omitempty"`
}

// Rule is one rule inside a ruleset; parameters stay open because GitHub
// adds rule types faster than a schema can chase them.
type Rule struct {
	Type       string         `yaml:"type" json:"type"`
	Parameters map[string]any `yaml:"parameters,omitempty" json:"parameters,omitempty"`
}

// Label is one issue label.
type Label struct {
	Name        string  `yaml:"name" json:"name"`
	Color       string  `yaml:"color,omitempty" json:"color,omitempty" jsonschema:"six hex digits, no leading #"`
	Description *string `yaml:"description,omitempty" json:"description,omitempty"`
}

// Autolink turns a reference prefix into a link.
type Autolink struct {
	KeyPrefix      string `yaml:"key_prefix" json:"key_prefix"`
	URLTemplate    string `yaml:"url_template" json:"url_template"`
	IsAlphanumeric *bool  `yaml:"is_alphanumeric,omitempty" json:"is_alphanumeric,omitempty"`
}

// Environment is a deployment environment.
type Environment struct {
	Name                   string   `yaml:"name" json:"name"`
	WaitTimer              *int     `yaml:"wait_timer,omitempty" json:"wait_timer,omitempty"`
	Reviewers              []string `yaml:"reviewers,omitempty" json:"reviewers,omitempty"`
	DeploymentBranchPolicy *string  `yaml:"deployment_branch_policy,omitempty" json:"deployment_branch_policy,omitempty"`
}

// Secrets declares Actions secrets by NAME only — never a value (G8).
type Secrets struct {
	Ownership Ownership `yaml:"ownership,omitempty" json:"ownership,omitempty" jsonschema:"enum=ownership"`
	Names     []string  `yaml:"names,omitempty" json:"names,omitempty"`
}

// Own is Secrets' effective ownership.
func (s *Secrets) Own(fallback Ownership) Ownership { return resolve(s.Ownership, fallback) }

// Webhook is a repository webhook; its secret is never declared here.
type Webhook struct {
	URL         string   `yaml:"url" json:"url"`
	Events      []string `yaml:"events,omitempty" json:"events,omitempty"`
	Active      *bool    `yaml:"active,omitempty" json:"active,omitempty"`
	ContentType *string  `yaml:"content_type,omitempty" json:"content_type,omitempty"`
}

// Collaborator is a user with direct repository access.
type Collaborator struct {
	Login      string `yaml:"login" json:"login"`
	Permission string `yaml:"permission,omitempty" json:"permission,omitempty" jsonschema:"enum=permission"`
}

// Pages is the GitHub Pages configuration (report-only in v1).
type Pages struct {
	Enabled *bool `yaml:"enabled,omitempty" json:"enabled,omitempty"`
}

// OrgProfile is the organization's public profile.
type OrgProfile struct {
	Ownership   Ownership `yaml:"ownership,omitempty" json:"ownership,omitempty" jsonschema:"enum=ownership"`
	Description *string   `yaml:"description,omitempty" json:"description,omitempty"`
	Blog        *string   `yaml:"blog,omitempty" json:"blog,omitempty"`
	Location    *string   `yaml:"location,omitempty" json:"location,omitempty"`
}

// Own is OrgProfile's effective ownership.
func (p *OrgProfile) Own(fallback Ownership) Ownership { return resolve(p.Ownership, fallback) }

// OrgMembers is the organization's member policy.
type OrgMembers struct {
	Ownership                    Ownership `yaml:"ownership,omitempty" json:"ownership,omitempty" jsonschema:"enum=ownership"`
	DefaultRepositoryPermission  *string   `yaml:"default_repository_permission,omitempty" json:"default_repository_permission,omitempty"`
	MembersCanCreateRepositories *bool     `yaml:"members_can_create_repositories,omitempty" json:"members_can_create_repositories,omitempty"`
	TwoFactorRequired            *bool     `yaml:"two_factor_required,omitempty" json:"two_factor_required,omitempty"`
}

// Own is OrgMembers' effective ownership.
func (m *OrgMembers) Own(fallback Ownership) Ownership { return resolve(m.Ownership, fallback) }

// OrgSecurityDefaults are the org's defaults for new repositories.
type OrgSecurityDefaults struct {
	Ownership                Ownership `yaml:"ownership,omitempty" json:"ownership,omitempty" jsonschema:"enum=ownership"`
	SecretScanningNewRepos   *bool     `yaml:"secret_scanning_new_repos,omitempty" json:"secret_scanning_new_repos,omitempty"`
	PushProtectionNewRepos   *bool     `yaml:"push_protection_new_repos,omitempty" json:"push_protection_new_repos,omitempty"`
	DependabotAlertsNewRepos *bool     `yaml:"dependabot_alerts_new_repos,omitempty" json:"dependabot_alerts_new_repos,omitempty"`
}

// Own is OrgSecurityDefaults' effective ownership.
func (d *OrgSecurityDefaults) Own(fallback Ownership) Ownership {
	return resolve(d.Ownership, fallback)
}

// OrgActions is the organization's Actions policy.
type OrgActions struct {
	Ownership                  Ownership `yaml:"ownership,omitempty" json:"ownership,omitempty" jsonschema:"enum=ownership"`
	AllowedActions             *string   `yaml:"allowed_actions,omitempty" json:"allowed_actions,omitempty" jsonschema:"enum=allowed_actions"`
	DefaultWorkflowPermissions *string   `yaml:"default_workflow_permissions,omitempty" json:"default_workflow_permissions,omitempty" jsonschema:"enum=workflow_permissions"`
}

// Own is OrgActions' effective ownership.
func (a *OrgActions) Own(fallback Ownership) Ownership { return resolve(a.Ownership, fallback) }

// OrgApp is an installed GitHub App (report-only in v1).
type OrgApp struct {
	Slug                string `yaml:"slug" json:"slug"`
	RepositorySelection string `yaml:"repository_selection,omitempty" json:"repository_selection,omitempty"`
}

// Enums are the closed value sets lint and the JSON Schema share, keyed by
// the `jsonschema:"enum=<key>"` struct tag.
var Enums = map[string][]string{
	"ownership":            Ownerships,
	"visibility":           {"public", "private", "internal"},
	"squash_title":         {"PR_TITLE", "COMMIT_OR_PR_TITLE"},
	"squash_message":       {"PR_BODY", "COMMIT_MESSAGES", "BLANK"},
	"code_scanning":        {"not-configured", "configured"},
	"allowed_actions":      {"all", "local_only", "selected"},
	"workflow_permissions": {"read", "write"},
	"ruleset_target":       {"branch", "tag", "push"},
	"enforcement":          {"active", "evaluate", "disabled"},
	"permission":           {"pull", "triage", "push", "maintain", "admin"},
}
