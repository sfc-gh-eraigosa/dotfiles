package pins

import (
	"context"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// probeTimeout bounds one row's probe. A var so a test can shrink it.
var probeTimeout = 5 * time.Second

// probeWorkers bounds concurrent probes.
const probeWorkers = 8

// Exec runs a shell command and returns its combined stdout+stderr (many tools
// print their version on stderr).
type Exec func(ctx context.Context, shellCmd string) (string, error)

// ShellExec is the real Exec: `sh -c cmd` with the caller's context as the
// deadline. On timeout the child is killed; WaitDelay stops a grandchild
// holding the pipe open from hanging Wait.
func ShellExec(ctx context.Context, shellCmd string) (string, error) {
	c := exec.CommandContext(ctx, "sh", "-c", shellCmd)
	c.WaitDelay = time.Second
	out, err := c.CombinedOutput()
	return string(out), err
}

// InstalledRow is one tool's installed-vs-wanted result. State is one of
// ok, drift, missing, unknown.
type InstalledRow struct {
	Tool      string `json:"tool"`
	Track     string `json:"track"`
	Want      string `json:"want"`
	Installed string `json:"installed"`
	State     string `json:"state"`
}

// Summary counts rows per state.
type Summary struct {
	OK      int `json:"ok"`
	Drift   int `json:"drift"`
	Missing int `json:"missing"`
	Unknown int `json:"unknown"`
}

// CheckInstalled probes every manifest row. mode is the gff `pins.track`
// value: "manifest" (default; honour each row's track), "latest" (everything
// is wanted at latest, so nothing can drift), or "pin" (rows tracked latest are
// held to their concrete version when they have one; floors stay floors). Rows come back in manifest order.
func CheckInstalled(ctx context.Context, m *Manifest, ex Exec, mode string) ([]InstalledRow, Summary) {
	rows := make([]InstalledRow, len(m.Entries))
	idx := make(chan int)
	var wg sync.WaitGroup
	workers := probeWorkers
	if workers > len(m.Entries) {
		workers = len(m.Entries)
	}
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range idx {
				rows[i] = checkOne(ctx, m.Entries[i], ex, mode)
			}
		}()
	}
	for i := range m.Entries {
		idx <- i
	}
	close(idx)
	wg.Wait()

	var s Summary
	for _, r := range rows {
		switch r.State {
		case "ok":
			s.OK++
		case "drift":
			s.Drift++
		case "missing":
			s.Missing++
		default:
			s.Unknown++
		}
	}
	return rows, s
}

func checkOne(ctx context.Context, e Entry, ex Exec, mode string) InstalledRow {
	r := InstalledRow{Tool: e.Tool, Track: string(e.Track), Want: e.Version}
	if e.Track == TrackLatest || mode == "latest" {
		r.Want = "latest"
	}
	if e.Probe == "-" || e.Probe == "" {
		r.State = "unknown"
		return r
	}
	pctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	out, err := ex(pctx, e.Probe)

	if pctx.Err() != nil { // timed out or cancelled: we do not know
		r.State = "unknown"
		return r
	}
	// "not found" with a failing exit is the shell saying the command is absent,
	// whatever digits its message happens to contain.
	if err != nil && strings.Contains(out, "not found") {
		r.State = "missing"
		return r
	}
	v := ExtractVersion(out)
	switch {
	case v == "" && (err != nil || strings.TrimSpace(out) == ""):
		r.State = "missing"
		return r
	case v == "":
		r.State = "unknown"
		return r
	}
	r.Installed = v

	switch {
	case r.Want == "latest":
		r.State = "ok"
	case e.Track == TrackFloor:
		r.State = stateIf(Compare(v, e.Version) >= 0)
	default:
		r.State = stateIf(Compare(v, e.Version) == 0)
	}
	return r
}

func stateIf(good bool) string {
	if good {
		return "ok"
	}
	return "drift"
}
