package resolvconf

import (
	"strings"
	"testing"
)

// EC-3: the winner must be FIRST, because nameserver #1 answers every query.
// The fallbacks exist only for when it is unreachable — they are reached on
// timeout, never on an NXDOMAIN.
func TestRenderResolvConf_WinnerFirstThenFallbacks(t *testing.T) {
	got := RenderResolvConf(Render{
		Winner:    "10.10.0.1",
		Fallbacks: []string{"10.255.255.254", "198.51.100.53"},
	})
	var servers []string
	for _, line := range strings.Split(got, "\n") {
		if f := strings.Fields(line); len(f) == 2 && f[0] == "nameserver" {
			servers = append(servers, f[1])
		}
	}
	want := []string{"10.10.0.1", "10.255.255.254", "198.51.100.53"}
	if strings.Join(servers, ",") != strings.Join(want, ",") {
		t.Errorf("nameservers = %v, want %v (winner first)", servers, want)
	}
	if !strings.Contains(got, "options timeout:1 attempts:1") {
		t.Error("missing `options timeout:1 attempts:1` — the off-network failover cap")
	}
	if !IsManaged(got) {
		t.Error("rendered file must carry the managed marker, or drift detection cannot tell it from a hand-edit")
	}
}

// The winner must never also appear as its own fallback: a duplicate wastes a
// full timeout on an unreachable server twice over.
func TestRenderResolvConf_DeduplicatesTheWinnerFromFallbacks(t *testing.T) {
	got := RenderResolvConf(Render{
		Winner:    "10.10.0.1",
		Fallbacks: []string{"10.10.0.1", "10.255.255.254", "10.255.255.254"},
	})
	if n := strings.Count(got, "nameserver 10.10.0.1"); n != 1 {
		t.Errorf("winner appears %d times, want 1", n)
	}
	if n := strings.Count(got, "nameserver 10.255.255.254"); n != 1 {
		t.Errorf("duplicate fallback appears %d times, want 1", n)
	}
}

// glibc reads at most 3 nameservers (MAXNS); anything beyond is silently
// ignored, so emitting more gives a false sense of redundancy.
func TestRenderResolvConf_CapsAtGlibcMaxns(t *testing.T) {
	got := RenderResolvConf(Render{
		Winner:    "10.10.0.1",
		Fallbacks: []string{"10.255.255.254", "198.51.100.53", "198.51.100.54", "203.0.113.1"},
	})
	if n := strings.Count(got, "nameserver "); n > 3 {
		t.Errorf("emitted %d nameservers, want at most 3 (glibc MAXNS)", n)
	}
}

func TestNameservers(t *testing.T) {
	content := "# comment\noptions timeout:1 attempts:1\nnameserver 10.10.0.1\nnameserver  10.255.255.254 \n\n"
	got := Nameservers(content)
	if len(got) != 2 || got[0] != "10.10.0.1" || got[1] != "10.255.255.254" {
		t.Errorf("Nameservers() = %v, want the two servers in order", got)
	}
}

// EC-7's budget must be DERIVED from the resolver config in force, not guessed.
// A hardcoded limit is what made the prototype's --verify report a regression
// on a run that was in fact a 5x improvement.
func TestFailBudgetSeconds(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content string
		want    int
	}{
		{
			// 3 nameservers x timeout 1 x 2 families + 1 slack
			name:    "managed config",
			content: "options timeout:1 attempts:1\nnameserver 10.10.0.1\nnameserver 10.255.255.254\nnameserver 198.51.100.53\n",
			want:    7,
		},
		{
			// No options line: glibc's default timeout is 5s.
			name:    "unmanaged single-resolver config",
			content: "nameserver 10.255.255.254\n",
			want:    11,
		},
		{
			name:    "no nameservers at all still yields a usable floor",
			content: "# empty\n",
			want:    11,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := FailBudgetSeconds(tc.content); got != tc.want {
				t.Errorf("FailBudgetSeconds() = %d, want %d", got, tc.want)
			}
		})
	}
}

// EC-3: all five INI shapes. Getting this wrong clobbers unrelated sections of
// a user's wsl.conf, which is not something they would notice until something
// else broke.
func TestSetGenerateResolvConf_AllFiveShapes(t *testing.T) {
	for _, tc := range []struct {
		name, in, want string
	}{
		{
			name: "key present with the wrong value",
			in:   "[boot]\nsystemd=true\n\n[network]\ngenerateResolvConf = true\nhostname = box\n",
			want: "[boot]\nsystemd=true\n\n[network]\ngenerateResolvConf = false\nhostname = box\n",
		},
		{
			name: "[network] present, key absent",
			in:   "[boot]\nsystemd=true\n\n[network]\nhostname = box\n",
			want: "[boot]\nsystemd=true\n\n[network]\nhostname = box\ngenerateResolvConf = false\n",
		},
		{
			name: "[network] present and last, empty",
			in:   "[boot]\nsystemd=true\n\n[network]\n",
			want: "[boot]\nsystemd=true\n\n[network]\ngenerateResolvConf = false\n",
		},
		{
			name: "no [network] section at all",
			in:   "[boot]\nsystemd=true\n",
			want: "[boot]\nsystemd=true\n\n[network]\ngenerateResolvConf = false\n",
		},
		{
			name: "empty file",
			in:   "",
			want: "[network]\ngenerateResolvConf = false\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := SetGenerateResolvConf(tc.in); got != tc.want {
				t.Errorf("SetGenerateResolvConf()\n got: %q\nwant: %q", got, tc.want)
			}
		})
	}
}

// Applying twice must not append a second key — install.sh re-runs.
func TestSetGenerateResolvConf_IsIdempotent(t *testing.T) {
	once := SetGenerateResolvConf("[boot]\nsystemd=true\n")
	if twice := SetGenerateResolvConf(once); twice != once {
		t.Errorf("not idempotent:\n once: %q\ntwice: %q", once, twice)
	}
}

// EC-16: removal is the repair path when no snapshot exists. A [network]
// section left empty by the removal must go too, so the file returns to its
// stock shape rather than carrying an empty section forever.
func TestRemoveGenerateResolvConf(t *testing.T) {
	for _, tc := range []struct {
		name, in, want string
	}{
		{
			name: "section had only our key — section removed with it",
			in:   "[boot]\nsystemd=true\n\n[network]\ngenerateResolvConf = false\n",
			want: "[boot]\nsystemd=true\n",
		},
		{
			name: "section has other keys — section kept, other keys untouched",
			in:   "[boot]\nsystemd=true\n\n[network]\ngenerateResolvConf = false\nhostname = box\n",
			want: "[boot]\nsystemd=true\n\n[network]\nhostname = box\n",
		},
		{
			name: "key absent — file unchanged",
			in:   "[boot]\nsystemd=true\n",
			want: "[boot]\nsystemd=true\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := RemoveGenerateResolvConf(tc.in); got != tc.want {
				t.Errorf("RemoveGenerateResolvConf()\n got: %q\nwant: %q", got, tc.want)
			}
		})
	}
}

// Set then Remove must return the original byte-for-byte. This is the property
// the whole undo path rests on.
func TestSetThenRemove_RoundTripsToTheOriginal(t *testing.T) {
	for _, original := range []string{
		"[boot]\nsystemd=true\n",
		"[boot]\nsystemd=true\n\n[user]\ndefault=someone\n",
		"[boot]\nsystemd=true\n\n[network]\nhostname = box\n",
	} {
		if got := RemoveGenerateResolvConf(SetGenerateResolvConf(original)); got != original {
			t.Errorf("round trip changed the file\n got: %q\nwant: %q", got, original)
		}
	}
}

// Comments and unrelated sections are someone else's configuration. wlink owns
// exactly one key and must leave everything else alone.
func TestSetGenerateResolvConf_PreservesCommentsAndOtherSections(t *testing.T) {
	in := "# top comment\n[boot]\nsystemd=true\n\n[user]\ndefault=someone\n\n[interop]\nenabled=true\n"
	got := SetGenerateResolvConf(in)
	for _, keep := range []string{"# top comment", "[user]", "default=someone", "[interop]", "enabled=true"} {
		if !strings.Contains(got, keep) {
			t.Errorf("lost %q from the file", keep)
		}
	}
}

// Renders the real artifacts into the test log so the evidence capture shows
// what wlink actually produces, rather than a transcription of it.
func TestRenderedArtifacts_ForEvidence(t *testing.T) {
	rc := RenderResolvConf(Render{
		Winner:    "10.10.0.1",
		Fallbacks: []string{"10.255.255.254", "198.51.100.53"},
	})
	t.Logf("--- /etc/resolv.conf ---\n%s", rc)
	t.Logf("FailBudgetSeconds(managed)   = %d", FailBudgetSeconds(rc))
	t.Logf("FailBudgetSeconds(unmanaged) = %d  // 1 nameserver, glibc default timeout 5",
		FailBudgetSeconds("nameserver 10.255.255.254\n"))
	t.Logf("--- /etc/wsl.conf ---\n%s",
		SetGenerateResolvConf("[boot]\nsystemd=true\n\n[user]\ndefault=someone\n"))
}
