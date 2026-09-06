// Package gh is gcfg's only door to GitHub. Everything above it — families,
// engine, verbs — talks to the Client interface, so tests run against the
// recording fake and never touch the network (plan §3.2).
package gh

import (
	"context"
	"encoding/json"
)

// Client is the seam. Do performs one request and decodes the response into
// out (when non-nil); Paginate walks a list endpoint, handing each element
// to each. Both return GitHub's status so a caller can tell "forbidden"
// from "broken" — a family that cannot be read is a finding, not a crash.
type Client interface {
	Do(ctx context.Context, method, path string, body any, out any) (status int, err error)
	Paginate(ctx context.Context, path string, each func(json.RawMessage) error) error
}
