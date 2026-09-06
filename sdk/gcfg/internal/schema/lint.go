package schema

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// Problem is one thing wrong with a file that the type system cannot say:
// a value outside its enum, a duplicate name, an org block in the wrong
// repository, a secret where no secret belongs.
type Problem struct {
	Path    string // dotted path into the file, e.g. repo.labels[1].name
	Message string
}

func (p Problem) String() string { return fmt.Sprintf("%s: %s", p.Path, p.Message) }

// LintOpts tells the linter what it cannot see in the file.
type LintOpts struct {
	Owner string // repository owner, for the org-block placement rule
	Repo  string // repository name; the org block belongs only in ".github"
}

// linter accumulates problems in schema order — the order plan §3.1 lists
// the families — so the same settings always report the same way.
type linter struct {
	ps []Problem
}

func (l *linter) add(path, format string, args ...any) {
	l.ps = append(l.ps, Problem{Path: path, Message: fmt.Sprintf(format, args...)})
}

// enum checks a value against a named set, skipping unset optionals.
func (l *linter) enum(path string, v *string, key string) {
	if v == nil || *v == "" {
		return
	}
	l.enumStr(path, *v, key)
}

func (l *linter) enumStr(path, v, key string) {
	if v == "" {
		return
	}
	for _, ok := range Enums[key] {
		if v == ok {
			return
		}
	}
	l.add(path, "%q must be one of %s", v, strings.Join(Enums[key], ", "))
}

// required flags an empty field that GitHub needs to identify the object.
func (l *linter) required(path, field, v string) {
	if strings.TrimSpace(v) == "" {
		l.add(path+"."+field, "%s is required", field)
	}
}

// dupes reports the second and later uses of a name.
func (l *linter) dupes(base, field string, names []string) {
	seen := map[string]bool{}
	for i, n := range names {
		if n == "" {
			continue
		}
		k := strings.ToLower(n)
		if seen[k] {
			l.add(fmt.Sprintf("%s[%d].%s", base, i, field), "duplicate %s %q", field, n)
		}
		seen[k] = true
	}
}

// secretish matches values shaped like a credential. It deliberately errs
// toward false positives: this file is committed, so a suspicious string is
// worth a human look.
var secretish = []*regexp.Regexp{
	regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{16,}`),         // GitHub PATs / tokens
	regexp.MustCompile(`github_pat_[A-Za-z0-9_]{20,}`),       // fine-grained PATs
	regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`), // PEM
	regexp.MustCompile(`(?i)\bAKIA[0-9A-Z]{16}\b`),           // AWS access key id
	regexp.MustCompile(`xox[baprs]-[A-Za-z0-9-]{10,}`),       // Slack
}

// secret flags a value that looks like a credential, never echoing it.
func (l *linter) secret(path string, v *string) {
	if v == nil {
		return
	}
	for _, re := range secretish {
		if re.MatchString(*v) {
			l.add(path, "value looks like a secret; gcfg.yaml is committed and manages secrets by name only")
			return
		}
	}
}

func (l *linter) secretStr(path, v string) { l.secret(path, &v) }

// hexColor is a GitHub label colour: six hex digits, no leading #.
var hexColor = regexp.MustCompile(`^[0-9a-fA-F]{6}$`)

// Lint returns everything questionable about f, in schema order. An empty
// slice means the file is coherent; it says nothing about the live repo.
func Lint(f *File, o LintOpts) []Problem {
	l := &linter{}
	if f == nil {
		return nil
	}
	if f.Ownership != "" {
		l.enumStr("ownership", string(f.Ownership), "ownership")
	}
	if f.Repo != nil {
		lintRepo(l, f.Repo)
	}
	if f.Org != nil {
		// The org block only has meaning in the organization's own .github
		// repository; anywhere else it would silently do nothing.
		if o.Repo != "" && o.Repo != ".github" {
			l.add("org", "the org block only works in the organization's `.github` repository, not %s/%s", o.Owner, o.Repo)
		}
		lintOrg(l, f.Org)
	}
	return l.ps
}

func lintRepo(l *linter, r *Repo) {
	if g := r.General; g != nil {
		l.enumStr("repo.general.ownership", string(g.Ownership), "ownership")
		l.secret("repo.general.description", g.Description)
		l.secret("repo.general.homepage", g.Homepage)
		l.enum("repo.general.visibility", g.Visibility, "visibility")
		if m := g.Merge; m != nil {
			l.enum("repo.general.merge.squash_title", m.SquashTitle, "squash_title")
			l.enum("repo.general.merge.squash_message", m.SquashMessage, "squash_message")
		}
	}
	if s := r.Security; s != nil {
		l.enumStr("repo.security.ownership", string(s.Ownership), "ownership")
		l.enum("repo.security.code_scanning_default_setup", s.CodeScanningDefaultSetup, "code_scanning")
	}
	if a := r.Actions; a != nil {
		l.enumStr("repo.actions.ownership", string(a.Ownership), "ownership")
		l.enum("repo.actions.allowed_actions", a.AllowedActions, "allowed_actions")
		l.enum("repo.actions.default_workflow_permissions", a.DefaultWorkflowPermissions, "workflow_permissions")
	}
	lintRulesets(l, "repo.rulesets", r.Rulesets)
	if r.Labels != nil {
		names := make([]string, len(r.Labels.Items))
		for i, lb := range r.Labels.Items {
			base := fmt.Sprintf("repo.labels[%d]", i)
			l.required(base, "name", lb.Name)
			names[i] = lb.Name
			if lb.Color != "" && !hexColor.MatchString(lb.Color) {
				l.add(base+".color", "%q must be six hex digits without a leading #", lb.Color)
			}
			l.secret(base+".description", lb.Description)
		}
		l.dupes("repo.labels", "name", names)
	}
	if r.Autolinks != nil {
		prefixes := make([]string, len(r.Autolinks.Items))
		for i, a := range r.Autolinks.Items {
			base := fmt.Sprintf("repo.autolinks[%d]", i)
			l.required(base, "key_prefix", a.KeyPrefix)
			l.required(base, "url_template", a.URLTemplate)
			l.secretStr(base+".url_template", a.URLTemplate)
			prefixes[i] = a.KeyPrefix
		}
		l.dupes("repo.autolinks", "key_prefix", prefixes)
	}
	if r.Environments != nil {
		names := make([]string, len(r.Environments.Items))
		for i, e := range r.Environments.Items {
			l.required(fmt.Sprintf("repo.environments[%d]", i), "name", e.Name)
			names[i] = e.Name
		}
		l.dupes("repo.environments", "name", names)
	}
	if r.Secrets != nil {
		l.enumStr("repo.secrets.ownership", string(r.Secrets.Ownership), "ownership")
		l.dupes("repo.secrets.names", "name", r.Secrets.Names)
	}
	if r.Webhooks != nil {
		urls := make([]string, len(r.Webhooks.Items))
		for i, w := range r.Webhooks.Items {
			base := fmt.Sprintf("repo.webhooks[%d]", i)
			l.required(base, "url", w.URL)
			if u, err := url.Parse(w.URL); err == nil && u.User != nil {
				l.add(base+".url", "URL carries a credential in its userinfo; put the secret in the webhook's own secret field instead")
			}
			l.secretStr(base+".url", w.URL)
			urls[i] = w.URL
		}
		l.dupes("repo.webhooks", "url", urls)
	}
	if r.Collaborators != nil {
		logins := make([]string, len(r.Collaborators.Items))
		for i, c := range r.Collaborators.Items {
			base := fmt.Sprintf("repo.collaborators[%d]", i)
			l.required(base, "login", c.Login)
			l.enumStr(base+".permission", c.Permission, "permission")
			logins[i] = c.Login
		}
		l.dupes("repo.collaborators", "login", logins)
	}
}

func lintRulesets(l *linter, base string, rs *List[Ruleset]) {
	if rs == nil {
		return
	}
	names := make([]string, len(rs.Items))
	for i, r := range rs.Items {
		p := fmt.Sprintf("%s[%d]", base, i)
		l.required(p, "name", r.Name)
		names[i] = r.Name
		l.enumStr(p+".target", r.Target, "ruleset_target")
		l.enumStr(p+".enforcement", r.Enforcement, "enforcement")
		for j, rule := range r.Rules {
			l.required(fmt.Sprintf("%s.rules[%d]", p, j), "type", rule.Type)
		}
	}
	l.dupes(base, "name", names)
}

func lintOrg(l *linter, o *Org) {
	if p := o.Profile; p != nil {
		l.enumStr("org.profile.ownership", string(p.Ownership), "ownership")
		l.secret("org.profile.description", p.Description)
		l.secret("org.profile.blog", p.Blog)
	}
	if m := o.Members; m != nil {
		l.enumStr("org.members.ownership", string(m.Ownership), "ownership")
	}
	if d := o.SecurityDefaults; d != nil {
		l.enumStr("org.security_defaults.ownership", string(d.Ownership), "ownership")
	}
	if a := o.Actions; a != nil {
		l.enumStr("org.actions.ownership", string(a.Ownership), "ownership")
		l.enum("org.actions.allowed_actions", a.AllowedActions, "allowed_actions")
		l.enum("org.actions.default_workflow_permissions", a.DefaultWorkflowPermissions, "workflow_permissions")
	}
	lintRulesets(l, "org.rulesets", o.Rulesets)
	if o.Apps != nil {
		slugs := make([]string, len(o.Apps.Items))
		for i, a := range o.Apps.Items {
			l.required(fmt.Sprintf("org.apps[%d]", i), "slug", a.Slug)
			slugs[i] = a.Slug
		}
		l.dupes("org.apps", "slug", slugs)
	}
}
