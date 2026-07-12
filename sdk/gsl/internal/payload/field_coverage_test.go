package payload_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/payload"
)

// TestParse_EveryFieldIsDecoded guards the maintenance trap introduced by the
// per-field tolerant decode: Parse no longer json.Unmarshal's into the Payload
// struct, it calls decodeField once PER FIELD by name. A field added to the
// struct but not to Parse therefore decodes to nil FOREVER — and every existing
// test stays green while the segment that needs it silently goes blank. That is
// the same "green test, dead code path" failure mode this whole objective exists
// to kill.
//
// This test reflects over Payload's json tags, feeds Parse a payload that
// populates every one of them, and fails if any field comes back zero. Adding a
// field to Payload requires adding it here AND to Parse.
func TestParse_EveryFieldIsDecoded(t *testing.T) {
	// A valid JSON value for each json tag on Payload.
	samples := map[string]string{
		"cwd":            `"/home/user/repo"`,
		"model":          `{"display_name":"X"}`,
		"context_window": `{"used_percentage":1.0}`,
		"rate_limits":    `{"five_hour":{"used_percentage":5.0}}`,
		"terminal_width": `120`,
		"quota":          `{"gemini-5h":{"remaining_fraction":0.5}}`,
		"product":        `"antigravity"`,
	}

	typ := reflect.TypeOf(payload.Payload{})

	// 1. Every json-tagged field must have a sample (forces this test to be
	//    updated when the struct grows).
	for i := range typ.NumField() {
		tag := typ.Field(i).Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}
		if _, ok := samples[tag]; !ok {
			t.Fatalf("Payload.%s has json tag %q with no sample in this test: add one, "+
				"and make sure Parse calls decodeField for it", typ.Field(i).Name, tag)
		}
	}

	// 2. Build the all-fields payload and parse it.
	obj := make(map[string]json.RawMessage, len(samples))
	for k, v := range samples {
		obj[k] = json.RawMessage(v)
	}
	data, err := json.Marshal(obj)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	p, err := payload.Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	// 3. No field may be zero — a zero field means Parse never decoded it.
	v := reflect.ValueOf(p)
	for i := range typ.NumField() {
		f := typ.Field(i)
		tag := f.Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}
		if v.Field(i).IsZero() {
			t.Errorf("Payload.%s (json %q) was NOT decoded by Parse — the input carried %s. "+
				"Parse is missing a decodeField call for this field.", f.Name, tag, samples[tag])
		}
	}
}

// TestParse_ProductFromLiveCapture — the discriminator cmd.deriveToolCtx relies
// on must survive the real wire payload.
func TestParse_ProductFromLiveCapture(t *testing.T) {
	for _, name := range []string{"agy_live.json", "agy_live_authenticating.json"} {
		p, err := payload.Parse(readFixture(t, name))
		if err != nil {
			t.Fatalf("Parse(%s): %v", name, err)
		}
		if p.Product == nil || *p.Product != "antigravity" {
			t.Errorf("%s: product: got %v, want \"antigravity\" "+
				"(this key is how gsl tells an agy render from a Claude render)", name, p.Product)
		}
		if !p.IsAntigravity() {
			t.Errorf("%s: IsAntigravity() = false", name)
		}
	}

	// A Claude payload has no product key and must not be mistaken for agy.
	p, err := payload.Parse(readFixture(t, "full.json"))
	if err != nil {
		t.Fatalf("Parse(full.json): %v", err)
	}
	if p.IsAntigravity() {
		t.Error("full.json (a Claude payload) was classified as Antigravity")
	}
}
