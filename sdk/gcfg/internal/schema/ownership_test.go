package schema

import (
	"strings"
	"testing"
)

// Every family answers Own() the same way: its own override, else the
// file's, else declared. The engine relies on that for the whole file.
func TestEveryFamilyResolvesOwnership(t *testing.T) {
	f, _, err := Load(write(t, full))
	if err != nil {
		t.Fatal(err)
	}
	owners := map[string]func(Ownership) Ownership{
		"general":          f.Repo.General.Own,
		"security":         f.Repo.Security.Own,
		"actions":          f.Repo.Actions.Own,
		"rulesets":         f.Repo.Rulesets.Own,
		"autolinks":        f.Repo.Autolinks.Own,
		"environments":     f.Repo.Environments.Own,
		"secrets":          f.Repo.Secrets.Own,
		"webhooks":         f.Repo.Webhooks.Own,
		"collaborators":    f.Repo.Collaborators.Own,
		"org profile":      f.Org.Profile.Own,
		"org members":      f.Org.Members.Own,
		"org sec defaults": f.Org.SecurityDefaults.Own,
		"org actions":      f.Org.Actions.Own,
		"org rulesets":     f.Org.Rulesets.Own,
		"org apps":         f.Org.Apps.Own,
	}
	for name, own := range owners {
		if got := own(Full); got != Full {
			t.Errorf("%s: with file=full want full, got %q", name, got)
		}
		if got := own(""); got != Declared {
			t.Errorf("%s: with no file default want declared, got %q", name, got)
		}
	}
	// labels declared `ownership: full` and keeps it against a declared file.
	if got := f.Repo.Labels.Own(Declared); got != Full {
		t.Errorf("labels own = %q, want full", got)
	}
	// A nil list is still answerable — verify walks families that are absent.
	var missing *List[Label]
	if got := missing.Own(Full); got != Full {
		t.Errorf("nil list own = %q, want full", got)
	}
	if got := missing.Len(); got != 0 {
		t.Errorf("nil list len = %d", got)
	}
	if got := f.Repo.Labels.Len(); got != 1 {
		t.Errorf("labels len = %d", got)
	}
}

// Round-tripping keeps both list forms: bare stays bare, an ownership
// override keeps the {ownership, items} mapping.
func TestListMarshalKeepsTheSimplerForm(t *testing.T) {
	f, _, err := Load(write(t, "version: 1\nrepo:\n  autolinks: [{key_prefix: \"J-\", url_template: \"https://example.com/<num>\"}]\n  labels:\n    ownership: full\n    items: [{name: bug, color: aaaaaa}]\n"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := f.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	out := string(b)
	if next := lineAfter(out, "autolinks:"); !strings.HasPrefix(next, "- ") {
		t.Errorf("autolinks should round-trip as a bare list, next line was %q:\n%s", next, out)
	}
	if next := lineAfter(out, "labels:"); !strings.HasPrefix(next, "ownership:") {
		t.Errorf("labels should keep the mapping form, next line was %q:\n%s", next, out)
	}
	if !strings.Contains(out, "ownership: full") || !strings.Contains(out, "items:") {
		t.Errorf("labels should keep the mapping form:\n%s", out)
	}
	again, _, err := Parse(b, "round-trip")
	if err != nil {
		t.Fatalf("re-parse: %v\n%s", err, out)
	}
	if again.Repo.Labels.Ownership != Full || again.Repo.Autolinks.Ownership != "" || again.Repo.Autolinks.Items[0].KeyPrefix != "J-" {
		t.Errorf("round-trip lost information: %+v", again.Repo)
	}
}

func TestParseNamesTheSourceInErrors(t *testing.T) {
	_, _, err := Parse([]byte("version: 1\nnope: true\n"), "some/path.yaml")
	if err == nil || !strings.Contains(err.Error(), "some/path.yaml") {
		t.Fatalf("want the path in the error, got %v", err)
	}
}

// lineAfter returns the trimmed line following the first line whose trimmed
// text equals key.
func lineAfter(doc, key string) string {
	lines := strings.Split(doc, "\n")
	for i, l := range lines {
		if strings.TrimSpace(l) == key && i+1 < len(lines) {
			return strings.TrimSpace(lines[i+1])
		}
	}
	return ""
}
