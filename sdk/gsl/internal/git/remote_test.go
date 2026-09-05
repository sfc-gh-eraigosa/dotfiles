package git

import (
	"context"
	"errors"
	"testing"

	gitfake "github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/git/fake"
)

func TestNormalizeRemote(t *testing.T) {
	cases := map[string]string{
		"git@github.com:o/r.git":            "https://github.com/o/r",
		"ssh://git@github.com/o/r.git":      "https://github.com/o/r",
		"ssh://git@gitlab.com:2222/o/r.git": "https://gitlab.com/o/r",
		"https://github.com/o/r.git":        "https://github.com/o/r",
		"https://user@github.com/o/r":       "https://github.com/o/r",
		"git://github.com/o/r.git":          "https://github.com/o/r",
		"/srv/git/r.git":                    "",
		"":                                  "",
	}
	for in, want := range cases {
		got, ok := NormalizeRemote(in)
		if got != want || ok != (want != "") {
			t.Errorf("NormalizeRemote(%q) = %q,%v want %q", in, got, ok, want)
		}
	}
}

func TestRemoteWebURL_UsesOriginAndDir(t *testing.T) {
	r := &gitfake.Runner{Script: []gitfake.Response{{Stdout: []byte("git@github.com:o/r.git\n")}}}
	got, err := RemoteWebURL(context.Background(), r, "/repo")
	if err != nil || got != "https://github.com/o/r" {
		t.Fatalf("got %q, %v", got, err)
	}
	c := r.Calls[0]
	if c.Name != "-C" || c.Args[0] != "/repo" || c.Args[1] != "remote" || c.Args[2] != "get-url" || c.Args[3] != "origin" {
		t.Errorf("unexpected call %+v", c)
	}
}

func TestRemoteWebURL_NoOriginIsError(t *testing.T) {
	r := &gitfake.Runner{Script: []gitfake.Response{{Stdout: []byte("error: No such remote 'origin'\n"), ExitCode: 2, Err: errors.New("exit status 2")}}}
	if _, err := RemoteWebURL(context.Background(), r, "/repo"); err == nil {
		t.Fatal("want error")
	}
}
