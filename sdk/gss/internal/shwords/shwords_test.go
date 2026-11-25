package shwords

import (
	"reflect"
	"slices"
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
		{"quoted # is not a comment", "echo \" # \" && g\\\nss push", []string{"echo #", "gss push"}},
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
		{"command substitution", "echo $(gss push)", []string{"echo $_...", "gss push"}},
		{"bash -c", `bash -c 'gss push'`, []string{"bash -c gss_push", "gss push"}},
		{"bash -lc glued flags", `bash -lc "gff  unset gss.approval-token.enabled"`, []string{"bash -lc gff_unset_gss.approval-token.enabled", "gff unset gss.approval-token.enabled"}},
		{"eval", `eval "gss" push`, []string{"eval gss push", "gss push"}},
		{"ssh", `ssh -p 22 pi1 gff set k v`, []string{"ssh -p 22 pi1 gff set k v", "gff set k v"}},
		{"heredoc into a shell", "bash <<EOF\ngss push\nEOF\n", []string{"bash <<EOF", "gss push"}},
		{"heredoc into cat is data", "cat <<EOF\ngss push\nEOF\n", []string{"cat <<EOF"}},
		{"assignment", "GIT_CONFIG_GLOBAL=~/.config/gff/x git config a b", []string{"GIT_CONFIG_GLOBAL=~/.config/gff/x git config a b"}},
		{"group redirect", "{ echo x; } > ~/.config/gff/y", []string{">~/.config/gff/y", "echo x"}},
		{"assignment with a space", `X="a cat" rm y`, []string{"X=a_cat rm y"}},
		{"redirect target with a space", `> "/tmp/a cat" rm y`, []string{"rm y >/tmp/a_cat"}},
		{"parameter kept as a marker", "gff unset $k", []string{"gff unset $k"}},
		{"quoted > is text", `gh pr comment 1 --body "x > ~/.config/gff/y"`, []string{"gh pr comment 1 --body x_~/.config/gff/y"}},
		// reviews of #475
		{"heredoc keeps its lines", "bash <<'EOF'\nset -e\ngff set k v\nEOF\n", []string{"bash <<EOF", "set -e", "gff set k v"}},
		{"quoted heredoc keeps backslashes", "bash <<'EOF'\necho \\\\; gss push\nEOF\n", []string{"bash <<EOF", `echo \`, "gss push"}},
		{"bash -c script keeps its lines", "bash -c $'echo hi\\ngss push'", []string{"bash -c echo_hi_gss_push", "echo hi", "gss push"}},
		{"a path in a script does not erase the script", "bash -c 'gss push; : a/../b'", []string{"bash -c b", "gss push", ": b"}},
		{"here-string into a shell", "bash <<< $'\\x67ss push'", []string{"bash <<<gss_push", "gss push"}},
		{"bash -c --", "bash -c -- 'gss push'", []string{"bash -c -- gss_push", "gss push"}},
		{"bash -o pipefail -c", "bash -o pipefail -c 'gss push'", []string{"bash -o pipefail -c gss_push", "gss push"}},
		{"bash +e -c", "bash +e -c 'gss push'", []string{"bash +e -c gss_push", "gss push"}},
		{"eval --", "eval -- 'gss' push", []string{"eval -- gss push", "gss push"}},
		{"env bash -c", "env X=1 bash -c 'gss push'", []string{"env X=1 bash -c gss_push", "gss push"}},
		{"sudo -u x bash heredoc", "sudo -u x bash <<EOF\ngss push\nEOF\n", []string{"sudo -u x bash <<EOF", "gss push"}},
		{"timeout 5 bash -c", "timeout -s KILL 5 bash -c 'gss push'", []string{"timeout -s KILL 5 bash -c gss_push", "gss push"}},
		{"ssh -tp", "ssh -tp 22 host gss push", []string{"ssh -tp 22 host gss push", "gss push"}},
		{"unicode escape", "$'\\u0067ss' push", []string{"gss push"}},
		{"NUL ends the string", "$'gss\\0x' push", []string{"gss push"}},
		{"control escape", "x$'\\cA'", []string{"x\x01"}},
		{"a quoted space in an env arg", "env \"X=a cat\" rm y", []string{"env X=a_cat rm y"}},
		// reviews of #475 (ffd41576)
		{"an = inside a path", "rm ~/.config/a=b/../gff/config.yaml", []string{"rm ~/.config/gff/config.yaml"}},
		{"an = after the path", "rm ~/.config/a/../gff/x=1", []string{"rm ~/.config/gff/x=1"}},
		{"a flag= keeps its name", "--out=a/../b", []string{"--out=b"}},
		{"a space inside a path", "rm ~/.config/\"a b\"/../gff/c", []string{"rm ~/.config/gff/c"}},
		{"ssh -oK=v takes no next word", "ssh -oBatchMode=no host gss push", []string{"ssh -oBatchMode=no host gss push", "gss push"}},
		{"sudo -uroot", "sudo -uroot bash -c 'gss push'", []string{"sudo -uroot bash -c gss_push", "gss push"}},
		{"timeout -k5s", "timeout -k5s 10 bash -c 'gss push'", []string{"timeout -k5s 10 bash -c gss_push", "gss push"}},
		{"env -", "env - bash -c 'gss push'", []string{"env - bash -c gss_push", "gss push"}},
		{"env --unset X", "env --unset FOO bash -c 'gss push'", []string{"env --unset FOO bash -c gss_push", "gss push"}},
		{"sudo --user=x", "sudo --user=root bash -c 'gss push'", []string{"sudo --user=root bash -c gss_push", "gss push"}},
		{"env -S", "env -S 'gss push'", []string{"env -S gss_push", "gss push"}},
		{"env --split-string=", "env --split-string='gss push'", []string{"env --split-string=gss_push", "gss push"}},
		{"a script file's stdin is data", "bash ./setup.sh <<'IN'\ngss push\nIN\n", []string{"bash ./setup.sh <<IN"}},
		{"bash /dev/stdin reads stdin", "bash /dev/stdin <<'E'\ngss push\nE\n", []string{"bash /dev/stdin <<E", "gss push"}},
		{"sh /dev/fd/0 reads stdin", "sh /dev/fd/0 <<'E'\ngss push\nE\n", []string{"sh /dev/fd/0 <<E", "gss push"}},
		{"bash --rcfile x -c", "bash --rcfile /dev/null -c 'gss push'", []string{"bash --rcfile /dev/null -c gss_push", "gss push"}},
		// agy review of #475 (ae70eae4)
		{"bash -eo pipefail -c", "bash -eo pipefail -c 'gss push'", []string{"bash -eo pipefail -c gss_push", "gss push"}},
		{"bash -co pipefail script", "bash -co pipefail 'gss push'", []string{"bash -co pipefail gss_push", "gss push"}},
		{"bash -eo pipefail heredoc", "bash -eo pipefail <<'E'\ngss push\nE\n", []string{"bash -eo pipefail <<E", "gss push"}},
		{"env -S prepends to the rest", "env -S 'bash -c' 'gss push'", []string{"env -S bash_-c gss_push", "bash -c gss_push", "gss push"}},
		{"env -- NAME=v", "env -- FOO=bar bash -c 'gss push'", []string{"env -- FOO=bar bash -c gss_push", "gss push"}},
		{"sudo NAME=v", "sudo FOO=bar bash -c 'gss push'", []string{"sudo FOO=bar bash -c gss_push", "gss push"}},
		{"xargs -i takes no next word", "xargs -i bash -c 'gss push'", []string{"xargs -i bash -c gss_push", "gss push"}},
		{"$'\\<nl>' keeps its backslash", "bash -c $'g\\\nss push'", []string{`bash -c g\_ss_push`, "gss push"}},
		{"unknown $'\\q' keeps its backslash", "echo $'\\q'", []string{`echo \q`}},
		{"bash -s reads stdin", "bash -s x <<'IN'\ngss push\nIN\n", []string{"bash -s x <<IN", "gss push"}},
		{"message text is one word", `git commit -m "gss push; gff set x"`, []string{"git commit -m gss_push_gff_set_x"}},
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
	// bash runs the lines before a syntax error
	ls, err := Lines("echo \\\\\ngff set k v\nfi\n")
	if err == nil || !reflect.DeepEqual(ls, []string{`echo \`, "gff set k v"}) {
		t.Fatalf("Lines = %q, %v; want the two lines before the error", ls, err)
	}
}

func TestNestingIsBounded(t *testing.T) {
	src := "gss push"
	for i := 0; i < 10; i++ {
		src = "eval " + strings.ReplaceAll(src, " ", `\ `)
	}
	ls, err := Lines(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(ls) != maxDepth+1 || slices.Contains(ls, "gss push") {
		t.Fatalf("Lines = %d lines (want %d), reached the innermost: %v", len(ls), maxDepth+1, slices.Contains(ls, "gss push"))
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
