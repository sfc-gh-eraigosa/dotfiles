package report

import (
	"encoding/json"
	"io"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gcfg/internal/engine"
)

// jsonFinding is the documented shape of one finding. Kind is a name, never
// the internal number, so the output survives a reordering of the constants.
type jsonFinding struct {
	Family string `json:"family"`
	Key    string `json:"key"`
	Kind   string `json:"kind"`
	Want   any    `json:"want,omitempty"`
	Live   any    `json:"live,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// jsonReport is the documented top-level shape (plan §3.3).
type jsonReport struct {
	Target   string         `json:"target"`
	Clean    bool           `json:"clean"`
	Families int            `json:"families"`
	Counts   map[string]int `json:"counts"`
	Findings []jsonFinding  `json:"findings"`
}

// JSON writes the machine-readable report.
func JSON(w io.Writer, r engine.Report) error {
	out := jsonReport{
		Target:   r.Target,
		Clean:    r.Clean(),
		Families: r.Families,
		Counts:   map[string]int{},
		Findings: []jsonFinding{},
	}
	for kind, n := range r.Counts {
		if n > 0 {
			out.Counts[kindName(kind)] = n
		}
	}
	for _, f := range r.Findings {
		jf := jsonFinding{Family: f.Family, Key: f.Key, Kind: kindName(f.Kind), Reason: redacted(f.Reason)}
		if f.Want != nil {
			jf.Want = redacted(f.Want)
		}
		if f.Live != nil {
			jf.Live = redacted(f.Live)
		}
		out.Findings = append(out.Findings, jf)
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
