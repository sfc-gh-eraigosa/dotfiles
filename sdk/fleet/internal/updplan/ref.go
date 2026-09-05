package updplan

import (
	"fmt"
	"sort"
	"strings"
)

// WithRef applies one --ref spec to the plan and returns a modified copy —
// the receiver's Repos map and each Repo's Branches slice are never
// mutated in place.
//
// spec is either a bare ref ("main") or "repo=ref" ("dotfiles=main"). A
// bare ref targets the repo named "dotfiles" if the plan has one,
// otherwise the plan's sole repo, otherwise it is an error listing every
// repo name and suggesting the "repo=branch" form.
//
// The target repo's Branches[0] is replaced by ref; if ref also appears
// later in Branches, that later (now-duplicate) entry is dropped.
func (p Plan) WithRef(spec string) (Plan, error) {
	repoName, ref, err := splitRefSpec(p, spec)
	if err != nil {
		return Plan{}, err
	}
	if !ValidRef(ref) {
		return Plan{}, fmt.Errorf("updplan: --ref %q: invalid ref", ref)
	}

	target, ok := p.Repos[repoName]
	if !ok {
		return Plan{}, fmt.Errorf("updplan: --ref: unknown repo %q", repoName)
	}

	newBranches := make([]string, 0, len(target.Branches))
	newBranches = append(newBranches, ref)
	for i, b := range target.Branches {
		if i == 0 {
			continue // replaced
		}
		if b == ref {
			continue // drop the now-duplicate extra
		}
		newBranches = append(newBranches, b)
	}
	// Re-run the branch-list rules: --ref must not mint a plan Parse itself
	// would reject (duplicates, an option-lookalike, "default" out of place).
	if err := validateBranches(fmt.Sprintf("repo %q", repoName), newBranches); err != nil {
		return Plan{}, fmt.Errorf("updplan: --ref: %w", err)
	}
	target.Branches = newBranches

	newRepos := make(map[string]Repo, len(p.Repos))
	for k, v := range p.Repos {
		newRepos[k] = v
	}
	newRepos[repoName] = target

	out := p
	out.Repos = newRepos
	out.Steps = append([]Step(nil), p.Steps...)
	return out, nil
}

// WithRefs applies each spec in turn via WithRef.
func (p Plan) WithRefs(specs []string) (Plan, error) {
	out := p
	for _, spec := range specs {
		var err error
		out, err = out.WithRef(spec)
		if err != nil {
			return Plan{}, err
		}
	}
	return out, nil
}

func splitRefSpec(p Plan, spec string) (repoName, ref string, err error) {
	if i := strings.IndexByte(spec, '='); i >= 0 {
		return spec[:i], spec[i+1:], nil
	}

	if _, ok := p.Repos["dotfiles"]; ok {
		return "dotfiles", spec, nil
	}
	if len(p.Repos) == 1 {
		for name := range p.Repos {
			return name, spec, nil
		}
	}

	names := make([]string, 0, len(p.Repos))
	for name := range p.Repos {
		names = append(names, name)
	}
	sort.Strings(names)
	return "", "", fmt.Errorf(
		"updplan: --ref %q is ambiguous: plan has repos %s; use repo=branch",
		spec, strings.Join(names, ", "))
}
