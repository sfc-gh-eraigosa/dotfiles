package schema

import (
	"fmt"
	"regexp"
	"strings"

	gffv1 "github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/gen/gff/v1"
)

// Finding is a single lint result.
type Finding struct {
	Path string // feature path (or "" for file-level findings)
	Rule string // stable machine-readable rule id
	Msg  string // human-readable message
}

// segmentRe matches a valid kebab-case segment: ^[a-z0-9]+(-[a-z0-9]+)*$
var segmentRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// negativePrefixes are the banned last-segment prefixes per the spec §global-constraints.
var negativePrefixes = []string{"no-", "not-", "disable-", "disabled-", "skip-", "off-"}

// Lint checks a FeatureFile for well-formedness and returns all findings.
// An empty slice means the file is clean.
func Lint(f *gffv1.FeatureFile) []Finding {
	var findings []Finding

	// --- Namespace checks ---
	findings = append(findings, lintNamespace(f.Namespace)...)

	// --- Feature path checks ---
	seen := make(map[string]bool)
	for _, set := range f.GetSets() {
		for _, feat := range set.GetFeatures() {
			path := feat.GetPath()

			// Duplicate path.
			if seen[path] {
				findings = append(findings, Finding{
					Path: path,
					Rule: "duplicate-path",
					Msg:  fmt.Sprintf("path %q appears more than once", path),
				})
				continue // no further checks on this duplicate
			}
			seen[path] = true

			// Path segment checks.
			findings = append(findings, lintPath(path, set.GetArea())...)

			// Every feature must declare a default (bool or choice).
			if feat.GetDefault() == nil {
				findings = append(findings, Finding{
					Path: path,
					Rule: "missing-default",
					Msg:  "feature declares neither boolDefault nor choiceDefault",
				})
			}

			// Choice-specific checks.
			if cd := feat.GetChoiceDefault(); cd != nil {
				findings = append(findings, lintChoice(path, cd)...)
			}
		}
	}

	return findings
}

// lintNamespace checks namespace presence, charset, and minimum segment count.
func lintNamespace(ns string) []Finding {
	var findings []Finding
	if ns == "" {
		return []Finding{{
			Path: "",
			Rule: "namespace-missing",
			Msg:  "namespace is required",
		}}
	}
	segments := strings.Split(ns, ".")
	if len(segments) < 2 {
		findings = append(findings, Finding{
			Path: "",
			Rule: "namespace-segments",
			Msg:  fmt.Sprintf("namespace %q must have at least 2 dotted segments", ns),
		})
	}
	for _, seg := range segments {
		if !segmentRe.MatchString(seg) {
			findings = append(findings, Finding{
				Path: "",
				Rule: "namespace-charset",
				Msg:  fmt.Sprintf("namespace segment %q does not match ^[a-z0-9]+(-[a-z0-9]+)*$", seg),
			})
		}
	}
	return findings
}

// lintPath checks segment count, segment charset, negative prefix, and area prefix.
func lintPath(path, area string) []Finding {
	var findings []Finding

	segments := strings.Split(path, ".")
	if len(segments) != 3 {
		findings = append(findings, Finding{
			Path: path,
			Rule: "path-depth",
			Msg:  fmt.Sprintf("path %q must have exactly 3 dotted segments, got %d", path, len(segments)),
		})
		// Still check charset for the segments we have.
	}

	for _, seg := range segments {
		if !segmentRe.MatchString(seg) {
			findings = append(findings, Finding{
				Path: path,
				Rule: "segment-charset",
				Msg:  fmt.Sprintf("path segment %q in %q does not match ^[a-z0-9]+(-[a-z0-9]+)*$", seg, path),
			})
		}
	}

	// Negative-prefix check on the last segment only.
	if len(segments) > 0 {
		last := segments[len(segments)-1]
		for _, prefix := range negativePrefixes {
			if strings.HasPrefix(last, prefix) {
				findings = append(findings, Finding{
					Path: path,
					Rule: "negative-name",
					Msg:  fmt.Sprintf("path %q last segment has negative prefix %q", path, prefix),
				})
				break
			}
		}
	}

	// Area-prefix check: path must start with "<area>.".
	if area != "" && !strings.HasPrefix(path, area+".") {
		findings = append(findings, Finding{
			Path: path,
			Rule: "path-area-prefix",
			Msg:  fmt.Sprintf("path %q does not start with set area %q", path, area),
		})
	}

	return findings
}

// lintChoice checks choice-specific constraints.
func lintChoice(path string, cd *gffv1.ChoiceDefault) []Finding {
	var findings []Finding

	// Mode must be specified.
	if cd.GetMode() == gffv1.ChoiceMode_CHOICE_MODE_UNSPECIFIED {
		findings = append(findings, Finding{
			Path: path,
			Rule: "choice-mode-unspecified",
			Msg:  fmt.Sprintf("choice %q: mode must be SINGLE or MULTI", path),
		})
	}

	opts := cd.GetOptions()

	// Options must be non-empty.
	if len(opts) == 0 {
		findings = append(findings, Finding{
			Path: path,
			Rule: "choice-empty-options",
			Msg:  fmt.Sprintf("choice %q: options list is empty", path),
		})
		return findings // nothing more to check
	}

	// Unique option ids + kebab charset.
	seenIDs := make(map[string]bool, len(opts))
	for _, opt := range opts {
		id := opt.GetId()
		if seenIDs[id] {
			findings = append(findings, Finding{
				Path: path,
				Rule: "choice-duplicate-option-id",
				Msg:  fmt.Sprintf("choice %q: option id %q is duplicated", path, id),
			})
		}
		seenIDs[id] = true

		if !segmentRe.MatchString(id) {
			findings = append(findings, Finding{
				Path: path,
				Rule: "choice-option-id-charset",
				Msg:  fmt.Sprintf("choice %q: option id %q does not match ^[a-z0-9]+(-[a-z0-9]+)*$", path, id),
			})
		}
	}

	// Homogeneous value type within the feature.
	findings = append(findings, lintChoiceValueTypes(path, opts)...)

	// Single-mode arity: exactly 1 default-selected option.
	if cd.GetMode() == gffv1.ChoiceMode_CHOICE_MODE_SINGLE {
		selectedCount := 0
		for _, opt := range opts {
			if opt.GetSelected() {
				selectedCount++
			}
		}
		if selectedCount != 1 {
			findings = append(findings, Finding{
				Path: path,
				Rule: "single-mode-arity",
				Msg:  fmt.Sprintf("choice %q: SINGLE mode requires exactly 1 default-selected option, got %d", path, selectedCount),
			})
		}
	}

	return findings
}

// valueTypeName returns a stable string name for the oneof variant in ChoiceOption.
func valueTypeName(opt *gffv1.ChoiceOption) string {
	switch opt.GetValue().(type) {
	case *gffv1.ChoiceOption_IntValue:
		return "int"
	case *gffv1.ChoiceOption_FloatValue:
		return "float"
	case *gffv1.ChoiceOption_StringValue:
		return "string"
	case *gffv1.ChoiceOption_BoolValue:
		return "bool"
	default:
		return "none"
	}
}

// lintChoiceValueTypes checks that all options share the same value type.
func lintChoiceValueTypes(path string, opts []*gffv1.ChoiceOption) []Finding {
	if len(opts) == 0 {
		return nil
	}
	first := valueTypeName(opts[0])
	for _, opt := range opts[1:] {
		if got := valueTypeName(opt); got != first {
			return []Finding{{
				Path: path,
				Rule: "choice-mixed-value-type",
				Msg:  fmt.Sprintf("choice %q: mixed value types (%q and %q)", path, first, got),
			}}
		}
	}
	return nil
}
