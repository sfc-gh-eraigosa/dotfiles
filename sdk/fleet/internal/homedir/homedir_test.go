package homedir

import "testing"

func TestExpand(t *testing.T) {
	cases := map[string]string{
		"~/git/dotfiles": "/home/u/git/dotfiles",
		"~":              "/home/u",
		"/srv/repo":      "/srv/repo",
		"~other/x":       "~other/x",
		"rel/path":       "rel/path",
	}
	for in, want := range cases {
		if got := Expand(in, "/home/u"); got != want {
			t.Errorf("Expand(%q) = %q, want %q", in, got, want)
		}
	}
}
