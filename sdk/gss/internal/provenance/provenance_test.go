package provenance

import (
	"strings"
	"testing"
)

func envOf(m map[string]string) Env { return func(k string) string { return m[k] } }

func TestDetect(t *testing.T) {
	cases := []struct {
		name          string
		env           map[string]string
		harness, sess string
	}{
		{"nothing", nil, "", ""},
		{"claude", map[string]string{"CLAUDECODE": "1", "CLAUDE_CODE_SESSION_ID": "abc-123"}, "claude", "abc-123"},
		{"override wins", map[string]string{"CLAUDECODE": "1", "GSS_BUILT_BY": "local:qwen3", "GSS_BUILT_SESSION": "draft-9"}, "local:qwen3", "draft-9"},
		{"ai agent", map[string]string{"AI_AGENT": "agy_1-2_agent"}, "agy", ""},
		{"ai agent code suffix", map[string]string{"AI_AGENT": "foo-code_1_agent"}, "foo", ""},
		{"injection stripped", map[string]string{"GSS_BUILT_BY": "x\nSigned-off-by: y"}, "xSigned-off-by:y", ""},
	}
	for _, c := range cases {
		got := Detect(envOf(c.env), "box.lan")
		if got.Harness != c.harness || got.Session != c.sess {
			t.Errorf("%s: Detect = %+v; want harness %q session %q", c.name, got, c.harness, c.sess)
		}
		if got.HostID != HostID("box") {
			t.Errorf("%s: HostID = %q", c.name, got.HostID)
		}
	}
}

func TestHostID(t *testing.T) {
	a := HostID("Box.lan")
	if a != HostID("box") || !strings.HasPrefix(a, "h-") || len(a) != 12 {
		t.Errorf("HostID = %q; want a stable h-<10 hex> for the short, lower-cased name", a)
	}
	if strings.Contains(a, "box") {
		t.Errorf("HostID %q carries the name", a)
	}
	if HostID("other") == a {
		t.Error("two hosts share an id")
	}
	if HostID("  ") != "" {
		t.Error("an empty name must give an empty id")
	}
}

func TestTrailers(t *testing.T) {
	if Trailers(Info{HostID: "h-1"}) != nil {
		t.Error("no assistant, no trailers")
	}
	got := strings.Join(Trailers(Info{Harness: "claude", Session: "s1", HostID: "h-1"}), "|")
	if got != "Built-By: claude|Built-On: h-1|Built-Session: s1|Hooks: global" {
		t.Errorf("Trailers = %s", got)
	}
}

func TestRenderBody(t *testing.T) {
	info := Info{Harness: "claude", Session: "s1", HostID: "h-1"}
	body := "prose\n\n<!-- gss:stack-begin -->\nstack\n<!-- gss:stack-end -->\n"
	once := RenderBody(body, info, "global")
	if !strings.HasPrefix(once, "prose") || !strings.Contains(once, "- Built by: claude") || !strings.Contains(once, "`h-1`") || !strings.Contains(once, "- Privacy hooks: global") {
		t.Errorf("first render:\n%s", once)
	}
	if twice := RenderBody(once, info, "global"); twice != once {
		t.Errorf("not idempotent:\n%s\n---\n%s", once, twice)
	}
	// Replaced in place: text after the section stays after it.
	moved := strings.Replace(once, "prose", "prose\n\n"+renderSection(Info{}, "x")+"\n\nafter", 1)
	out := RenderBody(moved, info, "local override: .husky")
	if strings.Count(out, markerBegin) != 1 || !strings.Contains(out, "local override: .husky") {
		t.Errorf("want one updated section:\n%s", out)
	}
	if strings.Index(out, markerBegin) > strings.Index(out, "after") {
		t.Errorf("section moved instead of being replaced in place:\n%s", out)
	}
	if got := RenderBody("", Info{}, ""); !strings.Contains(got, "no assistant detected") {
		t.Errorf("empty body render:\n%s", got)
	}
	if got := RenderBody("a <!-- gss:provenance-end --> b", info, ""); strings.Count(got, markerEnd) != 1 {
		t.Errorf("stray marker kept:\n%s", got)
	}
}
