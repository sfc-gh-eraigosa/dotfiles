package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreatePersistsAndPrintsNoSecrets(t *testing.T) {
	w := newWorld(t)
	out, errs, err := w.create(t)
	if err != nil {
		t.Fatalf("create: %v\n%s%s", err, out, errs)
	}
	assertNoLeak(t, "create stdout", out+errs)
	for _, want := range []string{"gcfg-test", "4242", filepath.Join(w.dir, "gcfg-test.pem"), "ghapp install"} {
		if !strings.Contains(out, want) {
			t.Errorf("create output missing %q:\n%s", want, out)
		}
	}
	st, err := os.Stat(filepath.Join(w.dir, "gcfg-test.pem"))
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("pem: %v %v", st, err)
	}
}

func TestCreateWithManifestFileAndOrg(t *testing.T) {
	w := newWorld(t)
	mf := filepath.Join(t.TempDir(), "m.json")
	os.WriteFile(mf, []byte(`{"name":"from-file","permissions":{"contents":"read"}}`), 0o600)
	out, errs, err := w.create(t, "--manifest", mf, "--org", "acme")
	if err != nil {
		t.Fatalf("%v\n%s%s", err, out, errs)
	}
	assertNoLeak(t, "create", out+errs)
}

func TestCreateRefusesDuplicateSlugWithoutForce(t *testing.T) {
	w := newWorld(t)
	if _, _, err := w.create(t); err != nil {
		t.Fatal(err)
	}
	_, _, err := w.create(t)
	if err == nil || !strings.Contains(err.Error(), "exists") {
		t.Fatalf("want 'exists' error, got %v", err)
	}
	if _, _, err := w.create(t, "--force"); err != nil {
		t.Fatalf("--force: %v", err)
	}
}

func TestStatusEmptyIsUsage(t *testing.T) {
	w := newWorld(t)
	out, _, err := w.run("status")
	if !errors.Is(err, ErrUsage) {
		t.Fatalf("want ErrUsage, got %v", err)
	}
	if !strings.Contains(out, "ghapp create") {
		t.Errorf("status should point at create:\n%s", out)
	}
}

func TestStatusListsAppsWithoutSecrets(t *testing.T) {
	w := newWorld(t)
	w.create(t)
	if _, _, err := w.run("install", "--no-browser"); err != nil {
		t.Fatal(err)
	}
	out, errs, err := w.run("status")
	if err != nil {
		t.Fatal(err)
	}
	assertNoLeak(t, "status", out+errs)
	for _, want := range []string{"gcfg-test", "4242", "0600", "sfc-gh-eraigosa", "11", "other-org", "22"} {
		if !strings.Contains(out, want) {
			t.Errorf("status missing %q:\n%s", want, out)
		}
	}
}

func TestInstallRecordsInstallations(t *testing.T) {
	w := newWorld(t)
	w.create(t)
	var opened string
	old := openBrowser
	openBrowser = func(u string) error { opened = u; return nil }
	t.Cleanup(func() { openBrowser = old })
	out, errs, err := w.run("install")
	if err != nil {
		t.Fatalf("%v\n%s%s", err, out, errs)
	}
	if opened != "https://web.example/apps/gcfg-test/installations/new" {
		t.Errorf("opened %q", opened)
	}
	assertNoLeak(t, "install", out+errs)
	if !strings.Contains(out, "sfc-gh-eraigosa") || !strings.Contains(out, "other-org") {
		t.Errorf("install should list recorded installations:\n%s", out)
	}
	raw, _ := os.ReadFile(filepath.Join(w.dir, "apps.json"))
	if !strings.Contains(string(raw), `"sfc-gh-eraigosa": 11`) || !strings.Contains(string(raw), `"other-org": 22`) {
		t.Errorf("apps.json missing installations:\n%s", raw)
	}
}

func TestTokenRepoPrintsOnlyTheToken(t *testing.T) {
	w := newWorld(t)
	w.create(t)
	out, errs, err := w.run("token", "--repo", "sfc-gh-eraigosa/dotfiles", "--permissions", "administration=write", "--permissions", "contents=read")
	if err != nil {
		t.Fatalf("%v %s", err, errs)
	}
	if out != leakCanary+"\n" {
		t.Fatalf("stdout must be exactly the token + newline, got %q", out)
	}
	assertNoLeak(t, "token stderr", errs)
	if w.lastBody["_installation"] != "11" {
		t.Errorf("want installation 11 for owner sfc-gh-eraigosa, got %v", w.lastBody)
	}
	repos, _ := w.lastBody["repositories"].([]any)
	if len(repos) != 1 || repos[0] != "dotfiles" {
		t.Errorf("repositories = %v", w.lastBody["repositories"])
	}
	perms, _ := w.lastBody["permissions"].(map[string]any)
	if perms["administration"] != "write" || perms["contents"] != "read" {
		t.Errorf("permissions = %v", perms)
	}
	// The installation discovered on the way is recorded for next time.
	raw, _ := os.ReadFile(filepath.Join(w.dir, "apps.json"))
	if !strings.Contains(string(raw), `"sfc-gh-eraigosa": 11`) {
		t.Errorf("token should record the installation it resolved:\n%s", raw)
	}
}

func TestTokenOrgUsesOrgInstallation(t *testing.T) {
	w := newWorld(t)
	w.create(t)
	out, _, err := w.run("token", "--org", "other-org")
	if err != nil || out != leakCanary+"\n" {
		t.Fatalf("out=%q err=%v", out, err)
	}
	if w.lastBody["_installation"] != "22" {
		t.Errorf("want installation 22, got %v", w.lastBody)
	}
	if _, has := w.lastBody["repositories"]; has {
		t.Errorf("org token must not scope repositories: %v", w.lastBody)
	}
}

func TestTokenUsageErrors(t *testing.T) {
	w := newWorld(t)
	if _, _, err := w.run("token", "--repo", "a/b"); !errors.Is(err, ErrUsage) {
		t.Fatalf("no app: want ErrUsage, got %v", err)
	}
	w.create(t)
	if _, _, err := w.run("token"); !errors.Is(err, ErrUsage) {
		t.Fatalf("no target: want ErrUsage, got %v", err)
	}
	if _, _, err := w.run("token", "--repo", "nobody/x"); err == nil || !strings.Contains(err.Error(), "nobody") {
		t.Fatalf("unknown owner: want error naming it, got %v", err)
	}
	if _, _, err := w.run("token", "--repo", "notaslug"); !errors.Is(err, ErrUsage) {
		t.Fatalf("bad --repo: want ErrUsage, got %v", err)
	}
	if _, _, err := w.run("token", "--repo", "a/b", "--permissions", "nope"); !errors.Is(err, ErrUsage) {
		t.Fatalf("bad --permissions: want ErrUsage, got %v", err)
	}
}

func TestDoctorHealthy(t *testing.T) {
	w := newWorld(t)
	w.create(t)
	out, errs, err := w.run("doctor", "--repo", "sfc-gh-eraigosa/dotfiles")
	if err != nil {
		t.Fatalf("%v\n%s%s", err, out, errs)
	}
	assertNoLeak(t, "doctor", out+errs)
	for _, want := range []string{"store", "0700", "pem", "0600", "jwt", "gcfg-test", "installations", "2", "token", "repo", "sfc-gh-eraigosa/dotfiles", "admin"} {
		if !strings.Contains(out, want) {
			t.Errorf("doctor output missing %q:\n%s", want, out)
		}
	}
	if w.repoHits != 1 {
		t.Errorf("doctor should probe the repo once with the minted token, got %d", w.repoHits)
	}
}

func TestDoctorFailsOnBadCredentials(t *testing.T) {
	w := newWorld(t)
	w.create(t)
	w.badJWT = true
	out, _, err := w.run("doctor")
	if err == nil {
		t.Fatalf("want doctor failure, got ok:\n%s", out)
	}
	if !strings.Contains(out, "401") {
		t.Errorf("doctor should show the 401:\n%s", out)
	}
	assertNoLeak(t, "doctor", out)
}

func TestDoctorNoAppIsUsage(t *testing.T) {
	w := newWorld(t)
	if _, _, err := w.run("doctor"); !errors.Is(err, ErrUsage) {
		t.Fatalf("want ErrUsage, got %v", err)
	}
}

func TestAppFlagSelectsAmongSeveral(t *testing.T) {
	w := newWorld(t)
	w.create(t)
	if _, _, err := w.run("status", "--app", "missing"); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("want unknown app error, got %v", err)
	}
	if _, _, err := w.run("status", "--app", "gcfg-test"); err != nil {
		t.Fatal(err)
	}
}
