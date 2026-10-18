package video

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeRunner is a scripted Runner. It never touches a real binary, so the
// tests cannot reach nvidia-smi or powershell.exe on the host.
type fakeRunner struct {
	calls []string
	fn    func(name string) ([]byte, error)
}

func (f *fakeRunner) Run(_ context.Context, name string, _ ...string) ([]byte, error) {
	f.calls = append(f.calls, name)
	return f.fn(name)
}

// only returns a runner that answers name with out/err and fails everything else.
func only(name string, out string, err error) *fakeRunner {
	return &fakeRunner{fn: func(n string) ([]byte, error) {
		if n == name {
			return []byte(out), err
		}
		return nil, errors.New("not found: " + n)
	}}
}

var errBoom = errors.New("boom")

func TestParseNvidia(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    Stats
		wantErr bool
	}{
		{
			name: "normal line",
			in:   "12, 2845, 16384\n",
			want: Stats{Util: 12, UsedBytes: 2845 * mib, TotalBytes: 16384 * mib, Source: "nvidia"},
		},
		{
			name: "first GPU only on multi-GPU output",
			in:   "12, 2845, 16384\n95, 8000, 8192\n",
			want: Stats{Util: 12, UsedBytes: 2845 * mib, TotalBytes: 16384 * mib, Source: "nvidia"},
		},
		{
			name: "util over 100 is clamped",
			in:   "140, 1, 2\n",
			want: Stats{Util: 100, UsedBytes: mib, TotalBytes: 2 * mib, Source: "nvidia"},
		},
		{name: "empty", in: "", wantErr: true},
		{name: "garbage", in: "hello world", wantErr: true},
		{name: "two fields", in: "12, 2845\n", wantErr: true},
		{name: "N/A field", in: "[N/A], 2845, 16384\n", wantErr: true},
		{name: "negative field", in: "12, -1, 16384\n", wantErr: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseNvidia([]byte(c.in))
			if c.wantErr {
				if err == nil {
					t.Fatalf("ParseNvidia(%q) = %+v, want error", c.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseNvidia(%q) error: %v", c.in, err)
			}
			if got != c.want {
				t.Fatalf("ParseNvidia(%q) = %+v, want %+v", c.in, got, c.want)
			}
		})
	}
}

func TestParseWSL(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    Stats
		wantErr bool
	}{
		{
			name: "normal",
			in:   "37|2147483648|8589934592\r\n",
			want: Stats{Util: 37, UsedBytes: 2147483648, TotalBytes: 8589934592, Source: "wsl"},
		},
		{
			name: "dedicated=0 path uses shared memory figures as given",
			in:   "5|1073741824|17179869184",
			want: Stats{Util: 5, UsedBytes: 1073741824, TotalBytes: 17179869184, Source: "wsl"},
		},
		{
			name: "noise lines before the result are ignored",
			in:   "WARNING: progress\n\n44|1|2\n",
			want: Stats{Util: 44, UsedBytes: 1, TotalBytes: 2, Source: "wsl"},
		},
		{
			name: "util over 100 is clamped",
			in:   "150|1|2\n",
			want: Stats{Util: 100, UsedBytes: 1, TotalBytes: 2, Source: "wsl"},
		},
		{name: "empty", in: "", wantErr: true},
		{name: "garbage", in: "not a reading", wantErr: true},
		{name: "two fields", in: "10|20\n", wantErr: true},
		{name: "non-numeric util", in: "abc|1|2\n", wantErr: true},
		{name: "negative bytes rejected by uint parse", in: "10|-1|2\n", wantErr: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseWSL([]byte(c.in))
			if c.wantErr {
				if err == nil {
					t.Fatalf("ParseWSL(%q) = %+v, want error", c.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseWSL(%q) error: %v", c.in, err)
			}
			if got != c.want {
				t.Fatalf("ParseWSL(%q) = %+v, want %+v", c.in, got, c.want)
			}
		})
	}
}

// writeCard builds <root>/card0/device/ with the given sysfs files.
func writeCard(t *testing.T, root string, files map[string]string) {
	t.Helper()
	dev := filepath.Join(root, "card0", "device")
	if err := os.MkdirAll(dev, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dev, name), []byte(body+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestReadDRM(t *testing.T) {
	t.Run("all three files present", func(t *testing.T) {
		root := t.TempDir()
		writeCard(t, root, map[string]string{
			"gpu_busy_percent":    "63",
			"mem_info_vram_used":  "2069835776",
			"mem_info_vram_total": "8319870976",
		})
		got, err := ReadDRM(root)
		if err != nil {
			t.Fatalf("ReadDRM: %v", err)
		}
		want := Stats{Util: 63, UsedBytes: 2069835776, TotalBytes: 8319870976, Source: "drm"}
		if got != want {
			t.Fatalf("ReadDRM = %+v, want %+v", got, want)
		}
	})
	t.Run("busy only: memory fields stay zero", func(t *testing.T) {
		root := t.TempDir()
		writeCard(t, root, map[string]string{"gpu_busy_percent": "7"})
		got, err := ReadDRM(root)
		if err != nil {
			t.Fatalf("ReadDRM: %v", err)
		}
		if got.Util != 7 || got.UsedBytes != 0 || got.TotalBytes != 0 || got.Source != "drm" {
			t.Fatalf("ReadDRM = %+v", got)
		}
	})
	t.Run("missing busy file is no source", func(t *testing.T) {
		root := t.TempDir()
		writeCard(t, root, map[string]string{"mem_info_vram_used": "1", "mem_info_vram_total": "2"})
		if _, err := ReadDRM(root); !errors.Is(err, ErrNoSource) {
			t.Fatalf("ReadDRM err = %v, want ErrNoSource", err)
		}
	})
	t.Run("corrupt busy file is no source", func(t *testing.T) {
		root := t.TempDir()
		writeCard(t, root, map[string]string{"gpu_busy_percent": "n/a"})
		if _, err := ReadDRM(root); !errors.Is(err, ErrNoSource) {
			t.Fatalf("ReadDRM err = %v, want ErrNoSource", err)
		}
	})
	t.Run("empty root is no source", func(t *testing.T) {
		if _, err := ReadDRM(t.TempDir()); !errors.Is(err, ErrNoSource) {
			t.Fatalf("ReadDRM err = %v, want ErrNoSource", err)
		}
	})
}

const (
	nvOut  = "12, 2845, 16384\n"
	wslOut = "37|2147483648|8589934592\n"
)

func TestCollectSourceOrder(t *testing.T) {
	cases := []struct {
		name       string
		runner     func() *fakeRunner
		isWSL      bool
		withDRM    bool
		wantSource string // "" means error
		wantErr    error
		wantCalls  []string
	}{
		{
			name:       "nvidia present wins even on WSL",
			runner:     func() *fakeRunner { return only("nvidia-smi", nvOut, nil) },
			isWSL:      true,
			withDRM:    true,
			wantSource: "nvidia",
			wantCalls:  []string{"nvidia-smi"},
		},
		{
			name: "nvidia garbage falls through to WSL",
			runner: func() *fakeRunner {
				return &fakeRunner{fn: func(n string) ([]byte, error) {
					if n == "nvidia-smi" {
						return []byte("[N/A], [N/A], [N/A]"), nil
					}
					return []byte(wslOut), nil
				}}
			},
			isWSL:      true,
			wantSource: "wsl",
			wantCalls:  []string{"nvidia-smi", "powershell.exe"},
		},
		{
			name:       "nvidia missing, WSL reading used",
			runner:     func() *fakeRunner { return only("powershell.exe", wslOut, nil) },
			isWSL:      true,
			withDRM:    true,
			wantSource: "wsl",
			wantCalls:  []string{"nvidia-smi", "powershell.exe"},
		},
		{
			name:       "WSL probe fails, DRM fallback",
			runner:     func() *fakeRunner { return only("none", "", errBoom) },
			isWSL:      true,
			withDRM:    true,
			wantSource: "drm",
			wantCalls:  []string{"nvidia-smi", "powershell.exe"},
		},
		{
			name:       "WSL probe returns garbage, DRM fallback",
			runner:     func() *fakeRunner { return only("powershell.exe", "junk", nil) },
			isWSL:      true,
			withDRM:    true,
			wantSource: "drm",
			wantCalls:  []string{"nvidia-smi", "powershell.exe"},
		},
		{
			name:       "not WSL: powershell never called, DRM used",
			runner:     func() *fakeRunner { return only("none", "", errBoom) },
			isWSL:      false,
			withDRM:    true,
			wantSource: "drm",
			wantCalls:  []string{"nvidia-smi"},
		},
		{
			name:      "nothing anywhere is ErrNoSource",
			runner:    func() *fakeRunner { return only("none", "", errBoom) },
			isWSL:     false,
			withDRM:   false,
			wantErr:   ErrNoSource,
			wantCalls: []string{"nvidia-smi"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			if c.withDRM {
				writeCard(t, root, map[string]string{"gpu_busy_percent": "21"})
			}
			r := c.runner()
			col := Collector{
				Runner:  r,
				DRMRoot: root,
				IsWSL:   func() bool { return c.isWSL },
			}
			got, err := col.Collect(context.Background())
			if c.wantErr != nil {
				if !errors.Is(err, c.wantErr) {
					t.Fatalf("Collect err = %v, want %v", err, c.wantErr)
				}
			} else {
				if err != nil {
					t.Fatalf("Collect err = %v", err)
				}
				if got.Source != c.wantSource {
					t.Fatalf("Collect source = %q, want %q (stats %+v)", got.Source, c.wantSource, got)
				}
			}
			if strings.Join(r.calls, ",") != strings.Join(c.wantCalls, ",") {
				t.Fatalf("runner calls = %v, want %v", r.calls, c.wantCalls)
			}
		})
	}
}

// A zero-value Collector has no Runner, so the subprocess probes (nvidia and
// WSL) are skipped entirely and only DRM is consulted. This pins that
// behaviour; the production path always sets a Runner.
func TestCollectNilRunnerSkipsSubprocesses(t *testing.T) {
	root := t.TempDir()
	writeCard(t, root, map[string]string{"gpu_busy_percent": "9"})
	got, err := Collector{DRMRoot: root, IsWSL: func() bool { return true }}.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if got.Source != "drm" || got.Util != 9 {
		t.Fatalf("Collect = %+v, want drm util 9", got)
	}
}

func TestHumanBytes(t *testing.T) {
	const gib = 1 << 30
	cases := []struct {
		in   uint64
		want string
	}{
		{0, "0M"},
		{512 * mib, "512M"},
		{1023 * mib, "1023M"},
		{2069835776, "1.9G"}, // 1.93 GiB
		{8319870976, "7.7G"}, // 7.75 GiB, rounds down
		{9 * gib, "9G"},      // "9.0" with the trailing ".0" trimmed
		{10 * gib, "10G"},
		{16 * gib, "16G"},
		{2800 * mib, "2.7G"},
	}
	for _, c := range cases {
		if got := HumanBytes(c.in); got != c.want {
			t.Errorf("HumanBytes(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}
