//go:build live

package lpass_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/vault"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/vault/lpass"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/vault/providers"
)

// TestLiveRoundTrip drives the real lpass CLI. The item fleet/fleet:spike is
// deliberately left in the vault (operator ruling); it carries a comment saying
// so. Only versions are logged, never payloads.
func TestLiveRoundTrip(t *testing.T) {
	if os.Getenv("FLEET_VAULT_LIVE") != "lpass" {
		t.Skip("set FLEET_VAULT_LIVE=lpass to run against the real lpass")
	}
	ctx := context.Background()
	s := lpass.New(vault.Config{Provider: "lpass", Vault: "fleet"}, providers.SystemExec)
	if err := s.Ready(ctx); err != nil {
		t.Fatalf("Ready: %v", err)
	}

	now := time.Now()
	payload := func(n int) vault.Secret {
		b, err := json.Marshal(map[string]any{
			"schema": "fleet-vault/v1",
			"kind":   "spike",
			"comment": fmt.Sprintf("Created by fleet TestLiveRoundTrip (fleet-vault-keys) on %s; test data only, safe to delete after %s.",
				now.Format("2006-01-02"), now.AddDate(0, 0, 7).Format("2006-01-02")),
			"revision": n,
			"stamp":    now.UnixNano(),
		})
		if err != nil {
			t.Fatal(err)
		}
		return vault.Secret(b)
	}

	var v1 string
	b, err := s.Get(ctx, "spike")
	switch {
	case errors.Is(err, vault.ErrNotFound):
		v1, err = s.Put(ctx, "spike", payload(1), "")
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		t.Logf("created, version %s", v1)
		b, err = s.Get(ctx, "spike")
		if err != nil {
			t.Fatalf("get after create: %v", err)
		}
		if b.Version != v1 {
			t.Logf("NOTE: Put returned %s but Get hashes to %s (lpass notes round-trip differs)", v1, b.Version)
		}
	case err != nil:
		t.Fatalf("get: %v", err)
	default:
		t.Logf("reusing existing item")
	}
	v1 = b.Version
	t.Logf("version before update: %s", v1)

	v2, err := s.Put(ctx, "spike", payload(2), v1)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	t.Logf("Put returned version %s", v2)
	b2, err := s.Get(ctx, "spike")
	if err != nil {
		t.Fatalf("get after update: %v", err)
	}
	t.Logf("version after update: %s", b2.Version)
	if b2.Version == v1 {
		t.Fatal("version did not change after update")
	}
	if b2.Version != v2 {
		t.Logf("NOTE: Put returned %s but Get hashes to %s", v2, b2.Version)
	}

	if _, err := s.Put(ctx, "spike", payload(3), v1); !errors.Is(err, vault.ErrConflict) {
		t.Fatalf("stale put: want ErrConflict, got %v", err)
	}

	names, err := s.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !slices.Contains(names, "spike") {
		t.Fatalf("List = %v, want spike", names)
	}
}
