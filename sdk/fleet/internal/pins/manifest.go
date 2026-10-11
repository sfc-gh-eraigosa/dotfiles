// Package pins reads the version manifest (opt/profiles/versions.tsv), compares
// versions, resolves the upstream "latest" for each tool, and checks what is
// actually installed on this machine. It has no cobra or terminal dependency;
// cmd/pins.go is the thin CLI over it.
package pins

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Track says how a tool's version is governed.
type Track string

const (
	// TrackPin means the installed version must equal the manifest version.
	TrackPin Track = "pin"
	// TrackFloor means the installed version must be at least the manifest version.
	TrackFloor Track = "floor"
	// TrackLatest means whatever upstream ships; the manifest holds no version.
	TrackLatest Track = "latest"
)

// ManifestRel is the manifest's path relative to the dotfiles repo root.
const ManifestRel = "opt/profiles/versions.tsv"

var (
	toolRe    = regexp.MustCompile(`^[a-z0-9-]+$`)
	versionRe = regexp.MustCompile(`^\d+(\.\d+)*([-+][0-9A-Za-z.+-]*)?$`)
)

// Entry is one manifest row.
type Entry struct {
	Tool  string
	Track Track
	// Version is the wanted version without a leading "v". For an @file row it
	// is the file's trimmed content; for a latest row it is "-".
	Version string
	// VersionFile is the repo-relative file a "@path" version lives in, else "".
	VersionFile string
	Upstream    string // gh:owner/repo | go: | text:<url> | -
	Installer   string // repo-relative script, or "-" (report-only)
	Probe       string // shell command printing the version, or "-"
	Line        int    // 1-based manifest line
}

// Manifest is a parsed versions.tsv. Raw lines are kept so Save rewrites only
// the lines Set touched and leaves comments, blanks and spacing byte-for-byte.
type Manifest struct {
	Path, Root string
	Entries    []Entry

	lines []string          // every manifest line, no newlines
	dirty bool              // lines changed since Parse
	files map[string]string // repo-relative @file path -> new content, written on Save
}

// Load parses <repoRoot>/opt/profiles/versions.tsv.
func Load(repoRoot string) (*Manifest, error) {
	path := filepath.Join(repoRoot, filepath.FromSlash(ManifestRel))
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("pins: open manifest: %w", err)
	}
	defer func() { _ = f.Close() }()
	return Parse(f, repoRoot, path)
}

// Parse reads a manifest. root resolves "@file" versions; path is where Save
// writes. Any malformed row is an error naming its line.
func Parse(r io.Reader, root, path string) (*Manifest, error) {
	m := &Manifest{Path: path, Root: root}
	seen := map[string]int{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		m.lines = append(m.lines, line)
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		e, err := parseRow(line, n, root)
		if err != nil {
			return nil, fmt.Errorf("pins: %s line %d: %w", path, n, err)
		}
		if first, dup := seen[e.Tool]; dup {
			return nil, fmt.Errorf("pins: %s line %d: duplicate tool %q (first on line %d)", path, n, e.Tool, first)
		}
		seen[e.Tool] = n
		m.Entries = append(m.Entries, e)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("pins: read manifest: %w", err)
	}
	return m, nil
}

// fieldSpans returns the [start,end) byte offsets of the first n
// whitespace-separated fields of s; the last returned span (if there are more
// than n fields) is not extended — callers take the rest of the line from it.
func fieldSpans(s string, n int) [][2]int {
	var out [][2]int
	i := 0
	for i < len(s) && len(out) < n {
		for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
			i++
		}
		if i >= len(s) {
			break
		}
		j := i
		for j < len(s) && s[j] != ' ' && s[j] != '\t' {
			j++
		}
		out = append(out, [2]int{i, j})
		i = j
	}
	return out
}

func parseRow(line string, n int, root string) (Entry, error) {
	sp := fieldSpans(line, 6)
	if len(sp) < 6 {
		return Entry{}, fmt.Errorf("want 6 columns (tool track version upstream installer probe), got %d", len(sp))
	}
	f := make([]string, 5)
	for i := range f {
		f[i] = line[sp[i][0]:sp[i][1]]
	}
	e := Entry{Tool: f[0], Track: Track(f[1]), Upstream: f[3], Installer: f[4], Line: n}
	e.Probe = strings.TrimSpace(line[sp[5][0]:])
	if !toolRe.MatchString(e.Tool) {
		return Entry{}, fmt.Errorf("bad tool name %q (want [a-z0-9-]+)", e.Tool)
	}
	switch e.Track {
	case TrackPin, TrackFloor, TrackLatest:
	default:
		return Entry{}, fmt.Errorf("unknown track %q (want pin, floor or latest)", f[1])
	}
	v := f[2]
	switch {
	case strings.HasPrefix(v, "@"):
		e.VersionFile = v[1:]
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(e.VersionFile)))
		if err != nil {
			return Entry{}, fmt.Errorf("version file: %w", err)
		}
		e.Version = strings.TrimPrefix(strings.TrimSpace(string(b)), "v")
	case v == "-":
		if e.Track != TrackLatest {
			return Entry{}, fmt.Errorf("track %s needs a version, got -", e.Track)
		}
		e.Version = "-"
	default:
		e.Version = strings.TrimPrefix(v, "v")
		if !versionRe.MatchString(e.Version) {
			return Entry{}, fmt.Errorf("bad version %q", v)
		}
	}
	return e, nil
}

// Get returns the row for tool.
func (m *Manifest) Get(tool string) (Entry, bool) {
	for _, e := range m.Entries {
		if e.Tool == tool {
			return e, true
		}
	}
	return Entry{}, false
}

// Set changes tool's version in memory; Save persists it. Only pin and floor
// rows can move — a latest row has no version to set. For an "@file" row the
// file is what changes and the manifest line stays as it is.
func (m *Manifest) Set(tool, version string) error {
	version = strings.TrimPrefix(strings.TrimSpace(version), "v")
	idx := -1
	for i, e := range m.Entries {
		if e.Tool == tool {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("pins: unknown tool %q", tool)
	}
	e := m.Entries[idx]
	if e.Track == TrackLatest {
		return fmt.Errorf("pins: %s tracks latest; it has no version to set", tool)
	}
	if !versionRe.MatchString(version) {
		return fmt.Errorf("pins: bad version %q for %s", version, tool)
	}
	if e.VersionFile != "" {
		if m.files == nil {
			m.files = map[string]string{}
		}
		m.files[e.VersionFile] = version + "\n"
	} else {
		li := e.Line - 1
		line := m.lines[li]
		sp := fieldSpans(line, 4) // version is the 3rd field; the 4th gives the padding end
		s, next := sp[2][0], sp[3][0]
		// Keep the column alignment: pad to the old field+gap width when the
		// new value fits, else leave a single separating space.
		width := next - s
		repl := version
		if len(version) < width {
			repl += strings.Repeat(" ", width-len(version))
		} else {
			repl += " "
		}
		m.lines[li] = line[:s] + repl + line[next:]
		m.dirty = true
	}
	m.Entries[idx].Version = version
	return nil
}

// Save writes every pending change atomically (temp file + rename). Nothing is
// written when Set was never called.
func (m *Manifest) Save() error {
	if m.dirty {
		if err := writeAtomic(m.Path, []byte(strings.Join(m.lines, "\n")+"\n")); err != nil {
			return err
		}
		m.dirty = false
	}
	for rel, content := range m.files {
		if err := writeAtomic(filepath.Join(m.Root, filepath.FromSlash(rel)), []byte(content)); err != nil {
			return err
		}
	}
	m.files = nil
	return nil
}

func writeAtomic(path string, data []byte) error {
	mode := os.FileMode(0o644)
	if st, err := os.Stat(path); err == nil {
		mode = st.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp*")
	if err != nil {
		return fmt.Errorf("pins: write %s: %w", path, err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }() // no-op once renamed
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("pins: write %s: %w", path, err)
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("pins: write %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("pins: write %s: %w", path, err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("pins: write %s: %w", path, err)
	}
	return nil
}
