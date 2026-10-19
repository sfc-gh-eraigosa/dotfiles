package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gdash/internal/config"
)

func labConfig(t *testing.T) config.Config {
	t.Helper()
	cred := filepath.Join(t.TempDir(), "lab.cred")
	if err := os.WriteFile(cred, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	return config.Config{
		Profiles:       map[string]config.Profile{"lab": {URL: "http://192.0.2.128:30300", Auth: config.Auth{File: cred}}},
		DefaultProfile: "lab",
		Views:          map[string]config.View{"ci": {Dashboard: "ci-lab"}, "conductor": {Dashboard: "conductor-metrics", Range: "6h"}},
	}
}

func TestPickView(t *testing.T) {
	c := labConfig(t)
	name, v, p, err := pickView(c, []string{"conductor"}, viewFlags{})
	if err != nil || name != "conductor" || v.Range != "6h" || p.URL == "" {
		t.Fatalf("named view: %s %+v %v", name, v, err)
	}
	if _, _, _, err := pickView(c, nil, viewFlags{}); err == nil || !strings.Contains(err.Error(), "ci, conductor") {
		t.Fatalf("no view named: %v", err)
	}
	if _, _, _, err := pickView(c, []string{"nope"}, viewFlags{}); err == nil || !strings.Contains(err.Error(), "ci, conductor") {
		t.Fatalf("unknown view: %v", err)
	}
	name, v, _, err = pickView(c, nil, viewFlags{dashboard: "Node Exporter / Nodes", rng: "15m"})
	if err != nil || v.Dashboard != "Node Exporter / Nodes" || v.Range != "15m" || v.Refresh != "30s" || name != "Node Exporter / Nodes" {
		t.Fatalf("ad hoc --dashboard: %s %+v %v", name, v, err)
	}
	if _, _, _, err := pickView(c, []string{"ci"}, viewFlags{profile: "nope"}); err == nil {
		t.Fatal("unknown --profile accepted")
	}
	if _, _, _, err := pickView(config.Config{}, nil, viewFlags{dashboard: "x"}); err == nil || !strings.Contains(err.Error(), "gdash config add profile") {
		t.Fatalf("no profiles at all should say how to add one: %v", err)
	}
}

func TestRootWithUnknownViewFails(t *testing.T) {
	withConfig(t)
	if _, err := run(t, "nope"); err == nil {
		t.Fatal("unknown view accepted")
	}
}
