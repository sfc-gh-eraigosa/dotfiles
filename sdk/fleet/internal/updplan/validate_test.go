package updplan

import (
	"strings"
	"testing"
)

const validBase = `version: 1
update:
  root: ~/git
  repos:
    dotfiles:
      path: dotfiles
      branches: [main]
  steps:
    - id: dotfiles.sync
      kind: sync
      repo: dotfiles
`

func TestParseRejects(t *testing.T) {
	cases := []struct {
		name    string
		yaml    string
		wantErr string // substring the aggregated error must contain
	}{
		{
			name: "unknown key",
			yaml: `version: 1
update:
  bogus: true
  repos: {}
  steps: []
`,
			wantErr: "bogus",
		},
		{
			name: "version not 1",
			yaml: `version: 2
update:
  repos: {}
  steps: []
`,
			wantErr: "version",
		},
		{
			name: "duplicate id",
			yaml: `version: 1
update:
  repos:
    dotfiles: {path: dotfiles, branches: [main]}
  steps:
    - id: a
      kind: sync
      repo: dotfiles
    - id: a
      kind: sync
      repo: dotfiles
`,
			wantErr: "duplicate step id",
		},
		{
			name: "unknown kind",
			yaml: `version: 1
update:
  repos:
    dotfiles: {path: dotfiles, branches: [main]}
  steps:
    - id: a
      kind: bogus
`,
			wantErr: "kind",
		},
		{
			name: "sync without repo",
			yaml: `version: 1
update:
  repos:
    dotfiles: {path: dotfiles, branches: [main]}
  steps:
    - id: a
      kind: sync
`,
			wantErr: "repo",
		},
		{
			name: "unknown repo",
			yaml: `version: 1
update:
  repos:
    dotfiles: {path: dotfiles, branches: [main]}
  steps:
    - id: a
      kind: sync
      repo: nope
`,
			wantErr: "unknown repo",
		},
		{
			name: "gh-auth with repo",
			yaml: `version: 1
update:
  repos:
    dotfiles: {path: dotfiles, branches: [main]}
  steps:
    - id: a
      kind: gh-auth
      repo: dotfiles
`,
			wantErr: "repo",
		},
		{
			name: "run without run:",
			yaml: `version: 1
update:
  repos: {}
  steps:
    - id: a
      kind: run
`,
			wantErr: "run",
		},
		{
			name:    "NUL in run",
			yaml:    "version: 1\nupdate:\n  repos: {}\n  steps:\n    - id: a\n      kind: run\n      run: \"echo\\x00hi\"\n",
			wantErr: "NUL",
		},
		{
			name:    "newline in run",
			yaml:    "version: 1\nupdate:\n  repos: {}\n  steps:\n    - id: a\n      kind: run\n      run: |\n        echo hi\n        echo bye\n",
			wantErr: "newline",
		},
		{
			name: "interactive on sync",
			yaml: `version: 1
update:
  repos:
    dotfiles: {path: dotfiles, branches: [main]}
  steps:
    - id: a
      kind: sync
      repo: dotfiles
      interactive: true
`,
			wantErr: "interactive",
		},
		{
			name: "retry on interactive",
			yaml: `version: 1
update:
  repos: {}
  steps:
    - id: a
      kind: run
      run: echo hi
      interactive: true
      retry: { attempts: 2 }
`,
			wantErr: "retry",
		},
		{
			name: "unknown need",
			yaml: `version: 1
update:
  repos: {}
  steps:
    - id: a
      kind: run
      run: echo hi
      needs: [nope]
`,
			wantErr: "unknown step",
		},
		{
			name: "self need",
			yaml: `version: 1
update:
  repos: {}
  steps:
    - id: a
      kind: run
      run: echo hi
      needs: [a]
`,
			wantErr: "itself",
		},
		{
			name: "cycle",
			yaml: `version: 1
update:
  repos: {}
  steps:
    - id: a
      kind: run
      run: echo hi
      needs: [b]
    - id: b
      kind: run
      run: echo hi
      needs: [a]
`,
			wantErr: "cycle",
		},
		{
			name: "default not first",
			yaml: `version: 1
update:
  repos:
    dotfiles: {path: dotfiles, branches: [main, "default"]}
  steps: []
`,
			wantErr: "default",
		},
		{
			name: "duplicate branches",
			yaml: `version: 1
update:
  repos:
    dotfiles: {path: dotfiles, branches: [main, main]}
  steps: []
`,
			wantErr: "duplicate branch",
		},
		{
			name: "expect.exit out of range",
			yaml: `version: 1
update:
  repos: {}
  steps:
    - id: a
      kind: run
      run: echo hi
      expect: { exit: [256] }
`,
			wantErr: "expect.exit",
		},
		{
			name: "bad on_failure",
			yaml: `version: 1
update:
  repos: {}
  steps:
    - id: a
      kind: run
      run: echo hi
      on_failure: maybe
`,
			wantErr: "on_failure",
		},
		{
			name: "bad local",
			yaml: `version: 1
update:
  repos:
    dotfiles: {path: dotfiles, branches: [main], local: whenever}
  steps: []
`,
			wantErr: "local",
		},
		{
			name: "attempts 0",
			yaml: `version: 1
update:
  repos: {}
  steps:
    - id: a
      kind: run
      run: echo hi
      retry: { attempts: 0 }
`,
			wantErr: "attempts",
		},
		{
			name: "unknown retry.on token",
			yaml: `version: 1
update:
  repos: {}
  steps:
    - id: a
      kind: run
      run: echo hi
      retry: { on: [bogus] }
`,
			wantErr: "retry.on",
		},
		{
			name: "factor < 1",
			yaml: `version: 1
update:
  repos: {}
  steps:
    - id: a
      kind: run
      run: echo hi
      retry: { backoff: { factor: 0.5 } }
`,
			wantErr: "factor",
		},
		{
			name: "unparsable duration",
			yaml: `version: 1
update:
  repos: {}
  steps:
    - id: a
      kind: run
      run: echo hi
      timeout: soon
`,
			wantErr: "duration",
		},
		{
			name: "negative timeout",
			yaml: `version: 1
update:
  repos: {}
  steps:
    - id: a
      kind: run
      run: echo hi
      timeout: -5s
`,
			wantErr: "negative",
		},
		{
			name: "injection in run repo path shell metachar",
			yaml: `version: 1
update:
  repos:
    dotfiles: {path: "dotfiles; rm -rf ~", branches: [main]}
  steps: []
`,
			wantErr: "path",
		},
		{
			name: "injection command substitution in path",
			yaml: `version: 1
update:
  repos:
    dotfiles: {path: "$(id)", branches: [main]}
  steps: []
`,
			wantErr: "path",
		},
		{
			name: "injection dotdot in path",
			yaml: `version: 1
update:
  repos:
    dotfiles: {path: "../x", branches: [main]}
  steps: []
`,
			wantErr: "path",
		},
		{
			name: "injection semicolon after tilde path",
			yaml: `version: 1
update:
  repos:
    dotfiles: {path: "~/git; id", branches: [main]}
  steps: []
`,
			wantErr: "path",
		},
		{
			name: "injection leading dash path",
			yaml: `version: 1
update:
  repos:
    dotfiles: {path: "-rf", branches: [main]}
  steps: []
`,
			wantErr: "path",
		},
		{
			name: "injection quote in url",
			yaml: `version: 1
update:
  repos:
    dotfiles: {path: dotfiles, url: "https://example.com/x'.git", branches: [main]}
  steps: []
`,
			wantErr: "url",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(tc.yaml))
			if err == nil {
				t.Fatalf("Parse(%s) = nil error, want one containing %q", tc.name, tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("Parse(%s) error = %q, want it to contain %q", tc.name, err.Error(), tc.wantErr)
			}
		})
	}
}

// TestParseAggregatesEveryError checks that two independent faults in one
// file are BOTH named in the aggregated error, not just the first.
func TestParseAggregatesEveryError(t *testing.T) {
	yaml := `version: 1
update:
  repos:
    dotfiles: {path: dotfiles, branches: [main]}
  steps:
    - id: a
      kind: sync
    - id: b
      kind: run
      run: echo hi
      on_failure: maybe
`
	_, err := Parse([]byte(yaml))
	if err == nil {
		t.Fatal("Parse() = nil error, want aggregated errors")
	}
	msg := err.Error()
	if !strings.Contains(msg, "repo") {
		t.Errorf("aggregated error missing the sync-without-repo fault: %q", msg)
	}
	if !strings.Contains(msg, "on_failure") {
		t.Errorf("aggregated error missing the bad on_failure fault: %q", msg)
	}
}

func TestLocalDefaultsToSkipAndRestoreToTrue(t *testing.T) {
	p, err := Parse([]byte(validBase))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	r, ok := p.Repos["dotfiles"]
	if !ok {
		t.Fatal("missing repo dotfiles")
	}
	if r.Local != LocalSkip {
		t.Errorf("Local = %q, want skip", r.Local)
	}
	if !r.Restore {
		t.Error("Restore = false, want true")
	}
}
