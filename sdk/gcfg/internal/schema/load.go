package schema

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Version is the only file format this build understands.
const Version = 1

// decodeStrict decodes n into out, rejecting keys the struct does not
// declare. yaml.v3 only offers KnownFields on a Decoder, so the node is
// re-encoded and decoded through one.
func decodeStrict(n *yaml.Node, out any) error {
	b, err := yaml.Marshal(n)
	if err != nil {
		return err
	}
	dec := yaml.NewDecoder(strings.NewReader(string(b)))
	dec.KnownFields(true)
	if err := dec.Decode(out); err != nil {
		return err
	}
	return nil
}

// Load reads and strictly decodes a gcfg.yaml. Unknown keys are an error
// naming the key and its line. Warnings are advisory (an empty file).
func Load(path string) (*File, []string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return Parse(b, path)
}

// Parse is Load over bytes already in hand (the TUI, tests, `--from`).
func Parse(b []byte, path string) (*File, []string, error) {
	dec := yaml.NewDecoder(strings.NewReader(string(b)))
	dec.KnownFields(true)
	var f File
	if err := dec.Decode(&f); err != nil {
		return nil, nil, fmt.Errorf("%s: %w", path, err)
	}
	if f.Version == 0 {
		return nil, nil, fmt.Errorf("%s: `version` is required (expected %d)", path, Version)
	}
	if f.Version != Version {
		return nil, nil, fmt.Errorf("%s: unsupported version %d (this build understands %d)", path, f.Version, Version)
	}
	if f.Ownership != "" && f.Ownership != Declared && f.Ownership != Full {
		return nil, nil, fmt.Errorf("%s: ownership %q must be one of %s", path, f.Ownership, strings.Join(Ownerships, ", "))
	}
	var warns []string
	if f.Repo == nil && f.Org == nil {
		warns = append(warns, fmt.Sprintf("%s declares no settings — verify and apply have nothing to do", path))
	}
	return &f, warns, nil
}

// Bytes renders the file back to YAML.
func (f *File) Bytes() ([]byte, error) {
	var sb strings.Builder
	enc := yaml.NewEncoder(&sb)
	enc.SetIndent(2)
	if err := enc.Encode(f); err != nil {
		return nil, fmt.Errorf("encoding: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("encoding: %w", err)
	}
	return []byte(sb.String()), nil
}

func ptr[T any](v T) *T { return &v }

// Default is the file `gcfg init` writes: the settings worth declaring on
// any repository, with values that match GitHub's own defaults where there
// is no obviously better choice.
func Default() *File {
	return &File{
		Version:   Version,
		Ownership: Declared,
		Repo: &Repo{
			General: &General{
				Features: &Features{Issues: ptr(true), Wiki: ptr(false), Projects: ptr(false), Discussions: ptr(false)},
				Merge: &Merge{
					Squash: ptr(true), MergeCommit: ptr(false), Rebase: ptr(false),
					AutoMerge: ptr(true), DeleteBranchOnMerge: ptr(true), AllowUpdateBranch: ptr(true),
					SquashTitle: ptr("COMMIT_OR_PR_TITLE"), SquashMessage: ptr("COMMIT_MESSAGES"),
				},
			},
			Security: &Security{
				SecretScanning: ptr(true), PushProtection: ptr(true),
				DependabotAlerts: ptr(true), DependabotSecurityUpdates: ptr(true),
			},
			Actions: &Actions{
				DefaultWorkflowPermissions:   ptr("read"),
				CanApprovePullRequestReviews: ptr(false),
			},
		},
	}
}
