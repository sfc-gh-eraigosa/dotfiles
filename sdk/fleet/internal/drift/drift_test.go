package drift

import (
	"testing"
	"time"
)

func TestClassifyAllFiveClasses(t *testing.T) {
	cases := []struct {
		name string
		in   Input
		want Class
	}{
		{"unreachable", Input{Reachable: false}, Unreachable},
		{"no stamp", Input{Reachable: true, HaveStamp: false}, Unknown},
		{"equal", Input{Reachable: true, HaveStamp: true, Commit: "aaa", Baseline: "aaa", IsAncestor: true}, UpToDate},
		{"behind", Input{Reachable: true, HaveStamp: true, Commit: "bbb", Baseline: "aaa", IsAncestor: true, BehindCount: 24}, Behind},
		{"divergent", Input{Reachable: true, HaveStamp: true, Commit: "ccc", Baseline: "aaa", IsAncestor: false}, Divergent},
	}
	for _, c := range cases {
		if got := Classify(c.in); got.Class != c.want {
			t.Errorf("%s: Classify = %q, want %q", c.name, got.Class, c.want)
		}
	}
}

func TestClassifyNeverReportsBehindWhenCommitsMatch(t *testing.T) {
	got := Classify(Input{Reachable: true, HaveStamp: true, Commit: "aaa", Baseline: "aaa", IsAncestor: true, BehindCount: 7})
	if got.Class != UpToDate || got.Behind != 0 {
		t.Fatalf("identical commits must be up-to-date with 0 behind, got %+v", got)
	}
}

// An unreachable host must never be reported as up-to-date just because no
// stamp came back — silence is not success.
func TestClassifyUnreachableBeatsEverything(t *testing.T) {
	got := Classify(Input{Reachable: false, HaveStamp: true, Commit: "aaa", Baseline: "aaa", IsAncestor: true})
	if got.Class != Unreachable {
		t.Fatalf("unreachable must dominate, got %q", got.Class)
	}
}

func TestClassifyCarriesBehindCount(t *testing.T) {
	got := Classify(Input{Reachable: true, HaveStamp: true, Commit: "b", Baseline: "a", IsAncestor: true, BehindCount: 24})
	if got.Behind != 24 {
		t.Fatalf("Behind = %d, want 24", got.Behind)
	}
}

func TestFormatAge(t *testing.T) {
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		then time.Time
		want string
	}{
		{now.Add(-30 * time.Second), "just now"},
		{now.Add(-5 * time.Minute), "5m ago"},
		{now.Add(-3 * time.Hour), "3h ago"},
		{now.Add(-72 * time.Hour), "3d ago"},
		{now.Add(-21 * 24 * time.Hour), "3w ago"},
	}
	for _, c := range cases {
		if got := FormatAge(now, c.then); got != c.want {
			t.Errorf("FormatAge(%v) = %q, want %q", c.then, got, c.want)
		}
	}
}

func TestFormatAgeZeroTimeIsUnknown(t *testing.T) {
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	if got := FormatAge(now, time.Time{}); got != "-" {
		t.Fatalf("zero time should render %q, got %q", "-", got)
	}
}
