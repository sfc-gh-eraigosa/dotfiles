package schema_test

import (
	"testing"

	gffv1 "github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/gen/gff/v1"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/schema"
)

// boolFeature is a helper to build a simple bool feature at the given path.
func boolFeature(path string) *gffv1.Feature {
	return &gffv1.Feature{
		Path:        path,
		Description: "test feature",
		Default:     &gffv1.Feature_BoolDefault{BoolDefault: true},
	}
}

// choiceFeature builds a choice feature with the given mode and options.
func choiceFeature(path string, mode gffv1.ChoiceMode, opts []*gffv1.ChoiceOption) *gffv1.Feature {
	return &gffv1.Feature{
		Path:        path,
		Description: "test choice",
		Default: &gffv1.Feature_ChoiceDefault{
			ChoiceDefault: &gffv1.ChoiceDefault{
				Mode:    mode,
				Options: opts,
			},
		},
	}
}

// singleOpt builds a ChoiceOption with a string value.
func singleOpt(id string, selected bool) *gffv1.ChoiceOption {
	return &gffv1.ChoiceOption{
		Id:          id,
		Description: "option " + id,
		Selected:    selected,
		Value:       &gffv1.ChoiceOption_StringValue{StringValue: id},
	}
}

func TestLint(t *testing.T) {
	tests := []struct {
		name        string
		ff          *gffv1.FeatureFile
		wantFindings bool // true = expect at least one finding; false = expect zero findings
		wantRule    string // if non-empty, at least one finding must have this Rule
	}{
		// --- Clean file: zero findings ---
		{
			name: "clean file",
			ff: &gffv1.FeatureFile{
				Namespace: "com.example.demo",
				Sets: []*gffv1.FeatureSet{
					{
						Area:     "install",
						Features: []*gffv1.Feature{boolFeature("install.ai.claude")},
					},
				},
			},
			wantFindings: false,
		},
		// --- Duplicate path ---
		{
			name: "duplicate path",
			ff: &gffv1.FeatureFile{
				Namespace: "com.example.demo",
				Sets: []*gffv1.FeatureSet{
					{
						Area: "install",
						Features: []*gffv1.Feature{
							boolFeature("install.ai.claude"),
							boolFeature("install.ai.claude"), // duplicate
						},
					},
				},
			},
			wantFindings: true,
			wantRule:     "duplicate-path",
		},
		// --- Negative prefix: no- ---
		{
			name: "negative prefix no-",
			ff: &gffv1.FeatureFile{
				Namespace: "com.example.demo",
				Sets: []*gffv1.FeatureSet{
					{
						Area:     "install",
						Features: []*gffv1.Feature{boolFeature("install.ai.no-claude")},
					},
				},
			},
			wantFindings: true,
			wantRule:     "negative-name",
		},
		// --- Negative prefix: not- ---
		{
			name: "negative prefix not-",
			ff: &gffv1.FeatureFile{
				Namespace: "com.example.demo",
				Sets: []*gffv1.FeatureSet{
					{
						Area:     "install",
						Features: []*gffv1.Feature{boolFeature("install.ai.not-claude")},
					},
				},
			},
			wantFindings: true,
			wantRule:     "negative-name",
		},
		// --- Negative prefix: disable- ---
		{
			name: "negative prefix disable-",
			ff: &gffv1.FeatureFile{
				Namespace: "com.example.demo",
				Sets: []*gffv1.FeatureSet{
					{
						Area:     "install",
						Features: []*gffv1.Feature{boolFeature("install.ai.disable-claude")},
					},
				},
			},
			wantFindings: true,
			wantRule:     "negative-name",
		},
		// --- Negative prefix: disabled- ---
		{
			name: "negative prefix disabled-",
			ff: &gffv1.FeatureFile{
				Namespace: "com.example.demo",
				Sets: []*gffv1.FeatureSet{
					{
						Area:     "install",
						Features: []*gffv1.Feature{boolFeature("install.ai.disabled-claude")},
					},
				},
			},
			wantFindings: true,
			wantRule:     "negative-name",
		},
		// --- Negative prefix: skip- ---
		{
			name: "negative prefix skip-",
			ff: &gffv1.FeatureFile{
				Namespace: "com.example.demo",
				Sets: []*gffv1.FeatureSet{
					{
						Area:     "install",
						Features: []*gffv1.Feature{boolFeature("install.ai.skip-claude")},
					},
				},
			},
			wantFindings: true,
			wantRule:     "negative-name",
		},
		// --- Negative prefix: off- ---
		{
			name: "negative prefix off-",
			ff: &gffv1.FeatureFile{
				Namespace: "com.example.demo",
				Sets: []*gffv1.FeatureSet{
					{
						Area:     "install",
						Features: []*gffv1.Feature{boolFeature("install.ai.off-claude")},
					},
				},
			},
			wantFindings: true,
			wantRule:     "negative-name",
		},
		// --- Depth 2: only 2 dotted segments ---
		{
			name: "depth 2",
			ff: &gffv1.FeatureFile{
				Namespace: "com.example.demo",
				Sets: []*gffv1.FeatureSet{
					{
						Area:     "install",
						Features: []*gffv1.Feature{boolFeature("install.claude")},
					},
				},
			},
			wantFindings: true,
			wantRule:     "path-depth",
		},
		// --- Depth 4: 4 dotted segments ---
		{
			name: "depth 4",
			ff: &gffv1.FeatureFile{
				Namespace: "com.example.demo",
				Sets: []*gffv1.FeatureSet{
					{
						Area:     "install",
						Features: []*gffv1.Feature{boolFeature("install.ai.claude.extra")},
					},
				},
			},
			wantFindings: true,
			wantRule:     "path-depth",
		},
		// --- Uppercase segment ---
		{
			name: "uppercase segment",
			ff: &gffv1.FeatureFile{
				Namespace: "com.example.demo",
				Sets: []*gffv1.FeatureSet{
					{
						Area:     "install",
						Features: []*gffv1.Feature{boolFeature("install.AI.claude")},
					},
				},
			},
			wantFindings: true,
			wantRule:     "segment-charset",
		},
		// --- Underscore segment ---
		{
			name: "underscore segment",
			ff: &gffv1.FeatureFile{
				Namespace: "com.example.demo",
				Sets: []*gffv1.FeatureSet{
					{
						Area:     "install",
						Features: []*gffv1.Feature{boolFeature("install.ai_ml.claude")},
					},
				},
			},
			wantFindings: true,
			wantRule:     "segment-charset",
		},
		// --- Choice: empty options ---
		{
			name: "choice empty options",
			ff: &gffv1.FeatureFile{
				Namespace: "com.example.demo",
				Sets: []*gffv1.FeatureSet{
					{
						Area: "install",
						Features: []*gffv1.Feature{
							choiceFeature("install.pkg.manager", gffv1.ChoiceMode_CHOICE_MODE_SINGLE, nil),
						},
					},
				},
			},
			wantFindings: true,
			wantRule:     "choice-empty-options",
		},
		// --- Choice: duplicate option ids ---
		{
			name: "choice duplicate option ids",
			ff: &gffv1.FeatureFile{
				Namespace: "com.example.demo",
				Sets: []*gffv1.FeatureSet{
					{
						Area: "install",
						Features: []*gffv1.Feature{
							choiceFeature("install.pkg.manager", gffv1.ChoiceMode_CHOICE_MODE_MULTI,
								[]*gffv1.ChoiceOption{
									singleOpt("apt", false),
									singleOpt("apt", false), // duplicate
								}),
						},
					},
				},
			},
			wantFindings: true,
			wantRule:     "choice-duplicate-option-id",
		},
		// --- Choice: non-kebab option id ---
		{
			name: "choice non-kebab option id",
			ff: &gffv1.FeatureFile{
				Namespace: "com.example.demo",
				Sets: []*gffv1.FeatureSet{
					{
						Area: "install",
						Features: []*gffv1.Feature{
							choiceFeature("install.pkg.manager", gffv1.ChoiceMode_CHOICE_MODE_MULTI,
								[]*gffv1.ChoiceOption{
									{
										Id:          "my_opt", // underscore: not kebab
										Description: "bad id",
										Value:       &gffv1.ChoiceOption_StringValue{StringValue: "v"},
									},
								}),
						},
					},
				},
			},
			wantFindings: true,
			wantRule:     "choice-option-id-charset",
		},
		// --- Choice: mixed value types ---
		{
			name: "choice mixed value types",
			ff: &gffv1.FeatureFile{
				Namespace: "com.example.demo",
				Sets: []*gffv1.FeatureSet{
					{
						Area: "install",
						Features: []*gffv1.Feature{
							choiceFeature("install.pkg.manager", gffv1.ChoiceMode_CHOICE_MODE_MULTI,
								[]*gffv1.ChoiceOption{
									{
										Id:    "str-opt",
										Value: &gffv1.ChoiceOption_StringValue{StringValue: "v"},
									},
									{
										Id:    "int-opt",
										Value: &gffv1.ChoiceOption_IntValue{IntValue: 42}, // mixed
									},
								}),
						},
					},
				},
			},
			wantFindings: true,
			wantRule:     "choice-mixed-value-type",
		},
		// --- Single mode: zero default-selected options ---
		{
			name: "single mode zero selected",
			ff: &gffv1.FeatureFile{
				Namespace: "com.example.demo",
				Sets: []*gffv1.FeatureSet{
					{
						Area: "install",
						Features: []*gffv1.Feature{
							choiceFeature("install.pkg.manager", gffv1.ChoiceMode_CHOICE_MODE_SINGLE,
								[]*gffv1.ChoiceOption{
									singleOpt("apt", false),  // not selected
									singleOpt("brew", false), // not selected
								}),
						},
					},
				},
			},
			wantFindings: true,
			wantRule:     "single-mode-arity",
		},
		// --- Single mode: two default-selected options ---
		{
			name: "single mode two selected",
			ff: &gffv1.FeatureFile{
				Namespace: "com.example.demo",
				Sets: []*gffv1.FeatureSet{
					{
						Area: "install",
						Features: []*gffv1.Feature{
							choiceFeature("install.pkg.manager", gffv1.ChoiceMode_CHOICE_MODE_SINGLE,
								[]*gffv1.ChoiceOption{
									singleOpt("apt", true),  // selected
									singleOpt("brew", true), // also selected: invalid
								}),
						},
					},
				},
			},
			wantFindings: true,
			wantRule:     "single-mode-arity",
		},
		// --- Mode unspecified ---
		{
			name: "mode unspecified",
			ff: &gffv1.FeatureFile{
				Namespace: "com.example.demo",
				Sets: []*gffv1.FeatureSet{
					{
						Area: "install",
						Features: []*gffv1.Feature{
							choiceFeature("install.pkg.manager", gffv1.ChoiceMode_CHOICE_MODE_UNSPECIFIED,
								[]*gffv1.ChoiceOption{singleOpt("apt", true)}),
						},
					},
				},
			},
			wantFindings: true,
			wantRule:     "choice-mode-unspecified",
		},
		// --- Path not starting with set's area ---
		{
			name: "path not starting with area",
			ff: &gffv1.FeatureFile{
				Namespace: "com.example.demo",
				Sets: []*gffv1.FeatureSet{
					{
						Area: "install",
						Features: []*gffv1.Feature{
							boolFeature("shell.ai.claude"), // area is "install" but path starts with "shell"
						},
					},
				},
			},
			wantFindings: true,
			wantRule:     "path-area-prefix",
		},
		// --- Missing namespace ---
		{
			name: "missing namespace",
			ff: &gffv1.FeatureFile{
				Namespace: "", // missing
				Sets: []*gffv1.FeatureSet{
					{
						Area:     "install",
						Features: []*gffv1.Feature{boolFeature("install.ai.claude")},
					},
				},
			},
			wantFindings: true,
			wantRule:     "namespace-missing",
		},
		// --- Bad charset namespace ---
		{
			name: "bad charset namespace",
			ff: &gffv1.FeatureFile{
				Namespace: "com.example.BAD_NAMESPACE", // uppercase + underscore
				Sets: []*gffv1.FeatureSet{
					{
						Area:     "install",
						Features: []*gffv1.Feature{boolFeature("install.ai.claude")},
					},
				},
			},
			wantFindings: true,
			wantRule:     "namespace-charset",
		},
		// --- Namespace with only one segment ---
		{
			name: "namespace single segment",
			ff: &gffv1.FeatureFile{
				Namespace: "example", // only 1 segment; requires >= 2
				Sets: []*gffv1.FeatureSet{
					{
						Area:     "install",
						Features: []*gffv1.Feature{boolFeature("install.ai.claude")},
					},
				},
			},
			wantFindings: true,
			wantRule:     "namespace-segments",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			findings := schema.Lint(tc.ff)
			if tc.wantFindings && len(findings) == 0 {
				t.Errorf("expected findings but got none")
				return
			}
			if !tc.wantFindings && len(findings) > 0 {
				t.Errorf("expected no findings but got: %+v", findings)
				return
			}
			if tc.wantRule != "" {
				found := false
				for _, f := range findings {
					if f.Rule == tc.wantRule {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected finding with Rule=%q, got: %+v", tc.wantRule, findings)
				}
			}
		})
	}
}

func TestLintFindingFields(t *testing.T) {
	// Verify Finding has non-empty Path, Rule, Msg for a real finding.
	ff := &gffv1.FeatureFile{
		Namespace: "com.example.demo",
		Sets: []*gffv1.FeatureSet{
			{
				Area: "install",
				Features: []*gffv1.Feature{
					boolFeature("install.ai.claude"),
					boolFeature("install.ai.claude"), // duplicate
				},
			},
		},
	}
	findings := schema.Lint(ff)
	if len(findings) == 0 {
		t.Fatal("expected findings")
	}
	f := findings[0]
	if f.Path == "" {
		t.Error("finding Path is empty")
	}
	if f.Rule == "" {
		t.Error("finding Rule is empty")
	}
	if f.Msg == "" {
		t.Error("finding Msg is empty")
	}
}
