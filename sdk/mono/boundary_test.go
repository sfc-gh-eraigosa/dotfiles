package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// E24: mono only runs tools. It never imports gdash (or depends on it in
// go.mod); gdash stays a standalone program mono starts by name.
func TestMonoNeverImportsGdash(t *testing.T) {
	gomod, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(gomod), "sdk/gdash") {
		t.Fatal("go.mod depends on sdk/gdash")
	}
	out, err := exec.Command("go", "list", "-deps", "./...").Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	for _, dep := range strings.Fields(string(out)) {
		if strings.Contains(dep, "/sdk/gdash") {
			t.Fatalf("mono imports %s", dep)
		}
	}
}
