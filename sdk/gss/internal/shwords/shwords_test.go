package shwords

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"mvdan.cc/sh/v3/syntax"
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
		{"a path in a script does not erase the script", "bash -c 'gss push; : a/../b'", []string{"bash -c b_gss_push_:_b", "gss push", ": b"}},
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
		{"a space inside a path", "rm ~/.config/\"a b\"/../gff/c", []string{"rm ~/.config/gff/c_~/.config/a_gff/c"}},
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
		// reviews of #475 (dd5057e7)
		{"env -S bash heredoc", "env -S bash <<'E'\ngss push\nE\n", []string{"env -S bash <<E", "bash", "gss push"}},
		{"env -S with env options", "env -S '-i bash -c' 'gss push'", []string{"env -S -i_bash_-c gss_push", "bash -c gss_push", "gss push"}},
		{"env -S \\_ is a space", `env -S 'gss\_push'`, []string{`env -S gss\_push`, "gss push"}},
		{"bash -oo a b", "bash -oo pipefail errexit -c 'gss push'", []string{"bash -oo pipefail errexit -c gss_push", "gss push"}},
		{"bash /dev/./stdin", "bash /dev/./stdin <<'E'\ngss push\nE\n", []string{"bash /dev/stdin <<E", "gss push"}},
		{"source /dev/stdin", "source /dev/stdin <<'E'\ngss push\nE\n", []string{"source /dev/stdin <<E", "gss push"}},
		{". /dev/fd/0 here-string", ". /dev/fd/0 <<< 'gss push'", []string{". /dev/fd/0 <<<gss_push", "gss push"}},
		{"( bash ) heredoc", "(bash) <<'E'\ngss push\nE\n", []string{"<<E", "bash", "gss push"}},
		{"{ bash; } here-string", "{ bash; } <<< 'gss push'", []string{"<<<gss_push", "bash", "gss push"}},
		// agy review of #475 (4421ae47)
		{"env -S ; is a character", "env -S 'X=; gss push'", []string{"env -S X=_gss_push", "X= gss push"}},
		{"env -S ( is a character", "env -S 'X=( gss push'", []string{"env -S X=_gss_push", "X= gss push"}},
		{"$'\\c\\\\' takes both backslashes", "echo $'a\\c\\\\0b'", []string{"echo a\x1c0b"}},
		{"source -- /dev/stdin", "source -- /dev/stdin <<'E'\ngss push\nE\n", []string{"source -- /dev/stdin <<E", "gss push"}},
		{"eval bash heredoc", "eval bash <<'E'\ngss push\nE\n", []string{"eval bash <<E", "bash", "gss push"}},
		{"bash -c bash heredoc", "bash -c bash <<'E'\ngss push\nE\n", []string{"bash -c bash <<E", "bash", "gss push"}},
		{"a > redirect is not stdin", "{ bash -c 'echo hi'; } >/dev/null", []string{">/dev/null", "bash -c echo_hi", "echo hi"}},
		// review of #475 (d8ae013e): text piped into a shell
		{"echo | bash", "echo 'gss push' | bash", []string{"gss push", "echo gss_push", "bash"}},
		{"printf | sh -s", "printf '%s\\n' 'gss push' | sh -s", []string{"gss push", "printf %s\\n gss_push", "sh -s"}},
		{"cat heredoc | bash", "cat <<'E' | bash\ngss push\nE\n", []string{"gss push", "cat <<E", "bash"}},
		{"echo | bash -c is not stdin", "echo 'gss push' | bash -c 'wc -l'", []string{"echo gss_push", "bash -c wc_-l", "wc -l"}},
		{"echo | bash script is not stdin", "echo 'gss push' | bash ./run.sh", []string{"echo gss_push", "bash ./run.sh"}},
		// agy review of #475 (d8ae013e)
		{"$(...) in a group's here-string", "{ :; } <<< \"$(gss push)\"", []string{"<<<$_...", ":", "gss push"}},
		{"$(...) in a group's heredoc", "( : ) <<E\n$(gss push)\nE\n", []string{"<<E", ":", "gss push"}},
		{"<<- strips tabs", "bash <<-'E'\n\tg\\\n\tss push\nE\n", []string{"bash <<-E", "gss push"}},
		{"<<- unquoted keeps a continued line's tab", "cat <<-E\n\tg\\\n\tss push\nE\n", []string{"cat <<-E"}},
		{"env -a x", "env -a x bash -c 'gss push'", []string{"env -a x bash -c gss_push", "gss push"}},
		{"env -S keeps NAME=", "env -S 'G=~/.config/gff/c git x a/../../b'", []string{"env -S G=~/.config/b_~/.config/gff/c_git_x_../b", "G=~/.config/gff/c git x ../b"}},
		{"heredoc read by bash in it, once", "bash <<'E'\n" + strings.Repeat("bash;", 20) + "\nE\n", append([]string{"bash <<E"}, slices.Repeat([]string{"bash"}, 20)...)},
		// review of #475 (97da8f3c): a script seen without stdin, then with it
		{"sh -c sh, then with a heredoc", "sh -c sh </dev/null && sh -c sh <<'E'\ngss push\nE\n", []string{"sh -c sh </dev/null", "sh", "sh -c sh <<E", "sh", "gss push"}},
		{"eval bash, then with a here-string", "eval bash </dev/null; eval bash <<< 'gss push'", []string{"eval bash </dev/null", "bash", "eval bash <<<gss_push", "bash", "gss push"}},
		// agy review of #475 (97da8f3c)
		{"3-stage pipe", "true | echo 'gss push' | bash", []string{"gss push", "true", "echo gss_push", "bash"}},
		{"command echo | bash", "command echo 'gss push' | bash", []string{"gss push", "command echo gss_push", "bash"}},
		{"printf format operands", "printf '%s %s\\n' gss push | bash", []string{"gss push", "printf %s_%s\\n gss push", "bash"}},
		{"printf %b", "printf '%b\\n' '\\0147ss push' | bash", []string{"gss push", "printf %b\\n \\0147ss_push", "bash"}},
		{"echo -e \\0NNN", "echo -e '\\0147ss push' | bash", []string{"0147ss push", "gss push", "echo -e \\0147ss_push", "bash"}},
		{"cat - heredoc", "cat - <<E | bash\ngss push\nE\n", []string{"gss push", "cat - <<E", "bash"}},
		{"env -S -a 1=2", "env -S '-a 1=2 gss push'", []string{"env -S -a_1=2_gss_push", "gss push"}},
		{"env -S splits on CR", "env -S 'gss\rpush'", []string{"env -S gss_push", "gss push"}},
		// review of #475 (932acf3d): pass-through stages
		{"echo | cat | bash", "echo 'gss push' | cat | bash", []string{"gss push", "echo gss_push", "cat", "bash"}},
		{"echo | tee log | bash", "echo 'gss push' | tee log | bash", []string{"gss push", "echo gss_push", "tee log", "bash"}},
		{"echo | cat file | bash is the file", "echo 'gss push' | cat f | bash", []string{"echo gss_push", "cat f", "bash"}},
		// agy review of #475 (932acf3d)
		{"echo | env -S bash", "echo 'gss push' | env -S bash", []string{"gss push", "echo gss_push", "env -S bash", "bash"}},
		{"env -S echo | bash", "env -S 'echo gss' push | bash", []string{"gss push", "env -S echo_gss push", "echo gss push", "bash"}},
		{"cat -u heredoc | bash", "cat -u <<E | bash\ngss push\nE\n", []string{"gss push", "cat -u <<E", "bash"}},
		{"cat -- heredoc | bash", "cat -- <<E | bash\ngss push\nE\n", []string{"gss push", "cat -- <<E", "bash"}},
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

// Crafted input cannot hold the hook past its timeout (review of #475,
// 4421ae47): env -S chains, deeply nested redirected groups and long
// command lists each finish fast, within the line cap.
func TestAdversarialInputIsBounded(t *testing.T) {
	cases := map[string]string{
		"env -S chain":    strings.Repeat("env -S '' ", 8000) + "; gff set k v",
		"nested groups":   strings.Repeat("{ ", 1200) + strings.Repeat("bash; ", 1200) + strings.Repeat("} <<<x ", 1200),
		"many commands":   strings.Repeat("gss push; ", 6000),
		"nested bash -c":  strings.Repeat("bash -c ", 3000) + "'gss push'",
		"eval of evals":   strings.Repeat("eval eval ", 3000),
		"long heredoc in": "bash <<'E'\n" + strings.Repeat("gff set k v\n", 6000) + "E\n",
	}
	for name, src := range cases {
		start := time.Now()
		ls, err := Lines(src)
		if d := time.Since(start); d > time.Second {
			t.Errorf("%s (%d bytes): %v, want under 1s", name, len(src), d)
		}
		if len(ls) > maxLines {
			t.Errorf("%s: %d lines, over maxLines", name, len(ls))
		}
		if err != nil && !errors.Is(err, ErrTooBig) {
			t.Logf("%s: %v", name, err)
		}
	}
	if _, err := Lines(strings.Repeat("gss push; ", 20000)); !errors.Is(err, ErrTooBig) {
		t.Errorf("20000 commands: err = %v, want ErrTooBig", err)
	}
}

// envSplit matches GNU env -S (checked against coreutils env).
func TestEnvSplit(t *testing.T) {
	cases := map[string][]string{
		`X=; a b`:       {"X=;", "a", "b"},
		`a\_b "c d" #x`: {"a", "b", "c d"},
		`a 'b c' d\tq`:  {"a", "b c", "d\tq"},
		`a\cb c`:        {"a"},
		`"x\_y" z`:      {"x y", "z"},
		`X=( b|c`:       {"X=(", "b|c"},
		`X='\'' b`:      {"X='", "b"},
		`'a\\b'`:        {`a\b`},
		`printf [%s] a`: {"printf", "[%s]", "a"},
	}
	for in, want := range cases {
		if got := envSplit(in); !reflect.DeepEqual(got, want) {
			t.Errorf("envSplit(%q) = %q, want %q", in, got, want)
		}
	}
}

// One heredoc read by many shells is parsed once, so a short command cannot
// spend the work budget before a later line (review of #475, d8ae013e).
func TestSharedHeredocParsedOnce(t *testing.T) {
	src := "{ " + strings.Repeat("bash; ", 60) + "} <<< '" + strings.Repeat(":;", 600) + "'\ngss push\n"
	ls, err := Lines(src)
	if err != nil || !slices.Contains(ls, "gss push") {
		t.Fatalf("err %v; the line after the shared here-string was not reached (%d lines)", err, len(ls))
	}
}

// heredoc bodies match what bash gives cat (checked with od -c): <<- strips
// leading tabs, and an unquoted body joins \<newline> first.
func TestHeredocBodies(t *testing.T) {
	cases := map[string]string{
		"cat <<-'Q'\n\tg\\\n\tss push\nQ\n": "g\\\nss push\n",
		"cat <<-E\n\tg\\\n\tss push\nE\n":   "g\tss push\n",
		"cat <<'Q'\n\tx\nQ\n":               "\tx\n",
	}
	for src, want := range cases {
		f, err := syntax.NewParser().Parse(strings.NewReader(src), "")
		if err != nil {
			t.Fatal(err)
		}
		if got := heredoc(f.Stmts[0].Redirs[0]); got != want {
			t.Errorf("heredoc(%q) = %q, want %q", src, got, want)
		}
	}
}

// printf as bash prints it (review of #475, 932acf3d): escapes in the
// format, precision, and a bound on what a repeating format produces.
func TestPrintfText(t *testing.T) {
	cases := []struct {
		format string
		args   []string
		want   string
	}{
		{`\x67ss push`, nil, "gss push"},
		{`gss\x20push`, nil, "gss push"},
		{`gss push`, nil, "gss push"},
		{`\147ss push`, nil, "gss push"},
		{`%.3s push`, []string{"gssxyz"}, "gss push"},
		{`%s %s\n`, []string{"gss", "push", "a", "b"}, "gss push\na b\n"},
		{`%b`, []string{`\0147ss push`}, "gss push"},
		{`100%% %s`, []string{"x"}, "100% x"},
	}
	for _, c := range cases {
		if got := printfText(c.format, c.args); got != c.want {
			t.Errorf("printfText(%q, %q) = %q, want %q", c.format, c.args, got, c.want)
		}
	}
	big := strings.Repeat("a", 500<<10) + "%s"
	args := slices.Repeat([]string{"x"}, 1000)
	start := time.Now()
	if got := printfText(big, args); len(got) > maxPrinted {
		t.Errorf("printfText grew to %d bytes, want at most %d", len(got), maxPrinted)
	}
	if d := time.Since(start); d > time.Second {
		t.Errorf("printfText took %v on a repeating 500 KB format", d)
	}
}

// echo -e / printf %b and printf formats, as bash prints them (agy review
// of #475, 932acf3d; each want checked with od -c against bash 5).
func TestPrintEscapes(t *testing.T) {
	echo := map[string]string{
		`echo \\c` + "\n" + `\0147ss push`: "echo \\c\ngss push",
		`\x00` + "\n" + `\x67ss push`:      "\x00\ngss push",
		`a\cb`:                             "a",
		`\u0067ss`:                         "gss",
	}
	for in, want := range echo {
		if got := echoEscapes(in); got != want {
			t.Errorf("echoEscapes(%q) = %q, want %q", in, got, want)
		}
	}
	printf := []struct {
		format string
		args   []string
		want   string
	}{
		{`%*s%s\n`, []string{"0", "g", "ss push"}, "gss push\n"},
		{`%c%s\n`, []string{"gx", "ss push"}, "gss push\n"},
		{`%.*s push`, []string{"3", "gssx"}, "gss push"},
		{`a\x00` + "\n" + `b`, nil, "a\x00\nb"},
		{`a\cb`, nil, `a\cb`},
	}
	for _, c := range printf {
		if got := printfText(c.format, c.args); got != c.want {
			t.Errorf("printfText(%q, %q) = %q, want %q", c.format, c.args, got, c.want)
		}
	}
	if got := ansiC(`\c?`); got != "\x7f" {
		t.Errorf(`ansiC(\c?) = %q, want DEL`, got)
	}
}
