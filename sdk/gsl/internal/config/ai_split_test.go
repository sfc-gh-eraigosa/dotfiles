package config

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func types(c Config) []string {
	var out []string
	for _, s := range c.Segments {
		out = append(out, s.Type)
	}
	return out
}

// #426: the ai segment is four — model, context, mcp, limits — in its place.
func TestDefault_AISplit(t *testing.T) {
	want := []string{"host", "dirgit", "repo", "model", "context", "mcp", "limits", "video", "time"}
	if got := types(Default()); !slices.Equal(got, want) {
		t.Fatalf("default segments = %v, want %v", got, want)
	}
	for _, t2 := range []string{"model", "context", "mcp", "limits"} {
		if !slices.Contains(KnownSegmentTypes, t2) {
			t.Errorf("%s not a known type", t2)
		}
	}
	if slices.Contains(KnownSegmentTypes, "ai") {
		t.Error("ai is no longer a segment type (it migrates)")
	}
	// The pieces drop in this order, all between host and dirgit as ai did.
	p := DefaultSegmentPriority
	order := []int{p("host"), p("limits"), p("mcp"), p("context"), p("model"), p("dirgit")}
	if !slices.IsSorted(order) || len(slices.Compact(slices.Clone(order))) != len(order) {
		t.Errorf("priorities: limits %d mcp %d context %d model %d", p("limits"), p("mcp"), p("context"), p("model"))
	}
}

// An old config's ai entry becomes the four, in its place, with its on/off
// state, an explicit priority, and its options where they belong.
func TestLoad_MigratesAI(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		dir := t.TempDir()
		path := filepath.Join(dir, "config.json")
		on := "false"
		if enabled {
			on = "true"
		}
		data := `{"enabled":true,"segments":[{"type":"dirgit","enabled":true},{"type":"ai","enabled":` + on +
			`,"priority":25,"options":{"usage_url":"https://u.invalid","model_url":"https://m.invalid/{family}"}},{"type":"video","enabled":true},{"type":"time","enabled":true}]}`
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
		c, err := Load(path)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := types(c), []string{"dirgit", "model", "context", "mcp", "limits", "video", "time"}; !slices.Equal(got, want) {
			t.Fatalf("migrated = %v, want %v", got, want)
		}
		for _, s := range c.Segments[1:5] {
			if s.Enabled != enabled || s.Priority != 25 {
				t.Errorf("%s: enabled %v priority %d, want %v 25", s.Type, s.Enabled, s.Priority, enabled)
			}
		}
		if c.Segments[1].Options["model_url"] != "https://m.invalid/{family}" || c.Segments[1].Options["usage_url"] != nil {
			t.Errorf("model options = %v", c.Segments[1].Options)
		}
		for _, i := range []int{2, 4} { // context, limits link to the usage page
			if c.Segments[i].Options["usage_url"] != "https://u.invalid" {
				t.Errorf("%s options = %v", c.Segments[i].Type, c.Segments[i].Options)
			}
		}
	}
}

// `ai` still works as a name on the verbs: it means all four.
func TestSegmentVerbs_AIAlias(t *testing.T) {
	c := Default()
	if err := c.DisableSegment("ai"); err != nil {
		t.Fatal(err)
	}
	for _, s := range c.Segments {
		if slices.Contains(AIParts, s.Type) && s.Enabled {
			t.Errorf("%s still on after disable ai", s.Type)
		}
	}
	if err := c.ToggleSegment("ai"); err != nil {
		t.Fatal(err)
	}
	for _, s := range c.Segments {
		if slices.Contains(AIParts, s.Type) && !s.Enabled {
			t.Errorf("%s still off after toggle ai", s.Type)
		}
	}
	if err := c.DisableSegment("mcp"); err != nil {
		t.Fatal(err)
	}
	if c.findSegment("mcp").Enabled || !c.findSegment("model").Enabled {
		t.Error("one part toggles alone")
	}
}
