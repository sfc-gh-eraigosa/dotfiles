package shwords

import (
	"reflect"
	"strings"
	"testing"
)

func TestLines(t *testing.T) {
	cases := []struct {
		name, src string
		want      []string
	}{
		{"plain", "gss push", []string{"gss push"}},
		{"quotes", `"gss" 'feature' p"r" --ready`, []string{"gss feature pr --ready"}},
		{"escapes", `\gss pu\sh`, []string{"gss push"}},
		{"continuation inside a word", "g\\\nss push", []string{"gss push"}},
		{"even backslashes are a literal", "echo \\\\\nrm x", []string{`echo \`, "rm x"}},
		{"comment ends at newline", "# \\\ngff set k v", []string{"gff set k v"}},
		{"quoted # is not a comment", "echo \" # \" && g\\\nss push", []string{"echo  # ", "gss push"}},
		{"ansi-c", `$'\x67ff' set gss.approval-token.enabled false`, []string{"gff set gss.approval-token.enabled false"}},
		{"ansi-c octal", `$'\147ss' push`, []string{"gss push"}},
		{"dollar double quote", `$"gss" push`, []string{"gss push"}},
		{"redirect before command", "2>&1 gff set k v", []string{"gff set k v 2>&1"}},
		{"redirect glued", "gff set>/dev/null k", []string{"gff set k >/dev/null"}},
		{"redirect target", "echo x > ~/.config/gff/config.yaml", []string{"echo x >~/.config/gff/config.yaml"}},
		{"path cleaned", "rm ~/.config/a/b/../../gff/config.yaml", []string{"rm ~/.config/gff/config.yaml"}},
		{"flag path cleaned", "git diff --output=~/.config//gff/x", []string{"git diff --output=~/.config/gff/x"}},
		{"coproc", "coproc gff set k v", []string{"gff set k v"}},
		{"time -p", "time -p gss push", []string{"gss push"}},
		{"subshell and pipe", "(true) | gss sync", []string{"true", "gss sync"}},
		{"command substitution", "echo $(gss push)", []string{"echo $(...)", "gss push"}},
		{"bash -c", `bash -c 'gss push'`, []string{"bash -c gss push", "gss push"}},
		{"bash -lc glued flags", `bash -lc "gff  unset gss.approval-token.enabled"`, []string{"bash -lc gff  unset gss.approval-token.enabled", "gff unset gss.approval-token.enabled"}},
		{"eval", `eval "gss" push`, []string{"eval gss push", "gss push"}},
		{"ssh", `ssh -p 22 pi1 gff set k v`, []string{"ssh -p 22 pi1 gff set k v", "gff set k v"}},
		{"heredoc into a shell", "bash <<EOF\ngss push\nEOF\n", []string{"bash <<EOF", "gss push"}},
		{"heredoc into cat is data", "cat <<EOF\ngss push\nEOF\n", []string{"cat <<EOF"}},
		{"assignment", "GIT_CONFIG_GLOBAL=~/.config/gff/x git config a b", []string{"GIT_CONFIG_GLOBAL=~/.config/gff/x git config a b"}},
		{"group redirect", "{ echo x; } > ~/.config/gff/y", []string{">~/.config/gff/y", "echo x"}},
		{"assignment with a space", `X="a cat" rm y`, []string{"X=a_cat rm y"}},
		{"redirect target with a space", `> "/tmp/a cat" rm y`, []string{"rm y >/tmp/a_cat"}},
		{"parameter kept as a marker", "gff unset $k", []string{"gff unset $k"}},
		{"quoted > is text", `gh pr comment 1 --body "x > ~/.config/gff/y"`, []string{"gh pr comment 1 --body x   ~/.config/gff/y"}},
		{"message text is one word", `git commit -m "gss push; gff set x"`, []string{"git commit -m gss push; gff set x"}},
	}
	for _, c := range cases {
		got, err := Lines(c.src)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: Lines(%q)\n got  %q\n want %q", c.name, c.src, got, c.want)
		}
	}
}

func TestUnparsable(t *testing.T) {
	if _, err := Lines(`echo "unterminated`); err == nil {
		t.Fatal("want a parse error")
	}
}

func TestNestingIsBounded(t *testing.T) {
	src := "gss push"
	for i := 0; i < 10; i++ {
		src = "eval " + strings.ReplaceAll(src, " ", `\ `)
	}
	if _, err := Lines(src); err != nil {
		t.Fatal(err)
	}
}

func TestPrint(t *testing.T) {
	var b strings.Builder
	if err := Print(&b, "a; b"); err != nil || b.String() != "a\nb\n" {
		t.Fatalf("Print = %q, %v", b.String(), err)
	}
	if err := Print(&b, "("); err == nil {
		t.Fatal("want a parse error")
	}
}
