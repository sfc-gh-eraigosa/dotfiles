package registry

import (
	"encoding/json"
	"fmt"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/errors"
)

// SupportedSchemaVersion is the highest registry schema_version this build
// understands. A registry written by a newer gss (higher version) is
// refused rather than silently mis-parsed.
const SupportedSchemaVersion = 1

// Marshal renders a Registry to canonical, 2-space-indented JSON (the
// on-disk form). Output is deterministic for a given value, so a
// load→save cycle is byte-stable.
func Marshal(r Registry) ([]byte, error) {
	out, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("registry: marshal: %w", err)
	}
	out = append(out, '\n')
	return out, nil
}

// Unmarshal parses registry.json, rejecting a schema_version newer than
// this build supports (wrapping errors.ErrSchemaMismatch). A version of 0
// (absent) is tolerated and normalised to 1 for legacy/empty files.
func Unmarshal(data []byte) (Registry, error) {
	var r Registry
	if err := json.Unmarshal(data, &r); err != nil {
		return Registry{}, fmt.Errorf("registry: parse: %w", err)
	}
	if r.SchemaVersion == 0 {
		r.SchemaVersion = SupportedSchemaVersion
	}
	if r.SchemaVersion > SupportedSchemaVersion {
		return Registry{}, fmt.Errorf("%w: registry schema_version %d exceeds supported %d",
			errors.ErrSchemaMismatch, r.SchemaVersion, SupportedSchemaVersion)
	}
	return r, nil
}
