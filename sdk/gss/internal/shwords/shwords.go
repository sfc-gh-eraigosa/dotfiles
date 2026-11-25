// Package shwords turns a shell command into its simple commands, one per
// line, the way bash would see them: quotes, escapes, backslash-newlines and
// comments resolved, $'...' decoded, and the scripts handed to bash -c, sh
// -c, eval, ssh, or a heredoc or here-string fed to a shell parsed again
// (#474).
//
// The safety_guard hook runs its gss and gff checks on these lines as well
// as on the raw text, so a spelling the text match cannot follow (g\<nl>ss,
// echo \\<nl>rm ..., $'\x67ff' set ...) is still seen. It only adds
// refusals: when the command does not parse, the hook keeps its text match
// (and the lines bash would run before the syntax error).
package shwords

import (
	"fmt"
	"io"
	"path"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"mvdan.cc/sh/v3/syntax"
)

// maxDepth bounds how far nested bash -c / eval / ssh scripts are followed.
const maxDepth = 4

// Lines parses src as bash and returns one line per simple command: its
// assignments and words, then each redirection as its operator glued to its
// target (">/x", "2>&1", "<<EOF"). Each assignment value, argument and
// target is one token in the line: its spaces become _, and the characters
// that would be shell syntax outside quotes (< > ; & | ( ) `) are blanked.
// A statement with redirections and no simple command ({ ...; } >x) gives a
// line of its redirections. A parameter keeps a $name marker; the commands
// inside a substitution are lines of their own.
//
// On a syntax error Lines returns the lines of the statements before it,
// which bash would have run, with the error.
func Lines(src string) ([]string, error) {
	var out []string
	err := lines(src, 0, &out)
	return out, err
}

func lines(src string, depth int, out *[]string) error {
	return syntax.NewParser(syntax.Variant(syntax.LangBash)).Stmts(strings.NewReader(src), func(top *syntax.Stmt) bool {
		syntax.Walk(top, func(n syntax.Node) bool {
			if st, ok := n.(*syntax.Stmt); ok {
				stmt(st, depth, out)
			}
			return true
		})
		return true
	})
}

func stmt(st *syntax.Stmt, depth int, out *[]string) {
	var parts, raw []string
	if call, ok := st.Cmd.(*syntax.CallExpr); ok {
		for _, a := range call.Assigns {
			v := ""
			if a.Value != nil {
				v = word(a.Value)
			}
			parts = append(parts, a.Name.Value+"="+token(v))
		}
		for _, w := range call.Args {
			r := word(w)
			raw = append(raw, r)
			parts = append(parts, token(r))
		}
	}
	for _, r := range st.Redirs {
		s := ""
		if r.N != nil {
			s = r.N.Value
		}
		s += r.Op.String()
		if r.Word != nil {
			s += token(word(r.Word))
		}
		parts = append(parts, s)
	}
	if len(parts) > 0 {
		*out = append(*out, strings.Join(parts, " "))
	}
	if depth < maxDepth {
		for _, inner := range nested(raw, st.Redirs) {
			// bash runs the statements before a syntax error, and so do we
			_ = lines(inner, depth+1, out)
		}
	}
}

// token makes one resolved word one token of a line.
func token(s string) string {
	s = cleanPaths(s)
	s = strings.Map(func(r rune) rune {
		if strings.ContainsRune("<>;&|()`", r) {
			return ' '
		}
		return r
	}, s)
	return strings.Join(strings.Fields(s), "_")
}

// A wrapper runs the command after its options; nested looks past them.
type wrapper struct {
	short string   // short options that take a value: -u x, -ux
	long  []string // long options that take a value: --user x, --user=x
}

var wrappers = map[string]wrapper{
	"env":     {"uSCP", []string{"--unset", "--chdir", "--split-string"}},
	"command": {}, "builtin": {}, "nohup": {}, "setsid": {},
	"exec":    {"a", nil},
	"sudo":    {"ugpChDrtUT", []string{"--user", "--group", "--host", "--prompt", "--chdir", "--close-from", "--role", "--type", "--command-timeout", "--other-user"}},
	"doas":    {"uC", nil},
	"time":    {"fo", []string{"--format", "--output"}},
	"nice":    {"n", []string{"--adjustment"}},
	"timeout": {"sk", []string{"--signal", "--kill-after"}},
	"stdbuf":  {"ioe", []string{"--input", "--output", "--error"}},
	"xargs":   {"aEdILnPs", []string{"--arg-file", "--delimiter", "--max-args", "--max-procs", "--max-lines", "--max-chars", "--process-slot-var"}},
}

// shortValue reads a cluster of short options (-tp, -uroot, -oX=y) as getopt
// does: the first letter that takes a value takes the rest of the word, or
// the next word when it is last. It returns that letter (0 for none), its
// value, and whether the next word was taken.
func shortValue(w, opts string, next []string) (byte, string, bool) {
	for k := 1; k < len(w); k++ {
		if strings.IndexByte(opts, w[k]) >= 0 {
			if k+1 < len(w) {
				return w[k], w[k+1:], false
			}
			if len(next) > 0 {
				return w[k], next[0], true
			}
			return w[k], "", false
		}
	}
	return 0, "", false
}

// skipWrappers returns the index of the command a run of wrappers runs.
// env -S STRING splits STRING into words and runs them followed by the rest
// of the arguments; that whole command line is returned as split, with the
// index past the end.
func skipWrappers(words []string) (int, []string) {
	i := 0
	for i < len(words) {
		name := path.Base(words[i])
		wr, ok := wrappers[name]
		if !ok {
			return i, nil
		}
		i++
		splitAt := func(v string, rest []string) (int, []string) {
			line := v
			for _, w := range rest {
				line += " " + shQuote(w)
			}
			return len(words), []string{line}
		}
	options:
		for i < len(words) {
			w := words[i]
			switch {
			case w == "--":
				i++
				break options
			case name == "env" && w == "-":
				i++
			case strings.HasPrefix(w, "--"):
				i++
				k, v, attached := strings.Cut(w, "=")
				if !slices.Contains(wr.long, k) {
					continue
				}
				if !attached && i < len(words) {
					v = words[i]
					i++
				}
				if name == "env" && k == "--split-string" {
					return splitAt(v, words[i:])
				}
			case strings.HasPrefix(w, "-") && len(w) > 1:
				i++
				letter, v, took := shortValue(w, wr.short, words[i:])
				if took {
					i++
				}
				if name == "env" && letter == 'S' {
					return splitAt(v, words[i:])
				}
			default:
				break options
			}
		}
		// NAME=VALUE before the command, after the options or a --
		for (name == "env" || name == "sudo") && i < len(words) &&
			strings.Contains(words[i], "=") && !strings.HasPrefix(words[i], "-") {
			i++
		}
		if name == "timeout" && i < len(words) {
			i++ // the duration
		}
	}
	return i, nil
}

// shQuote quotes s as one bash word.
func shQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// nested returns the shell scripts a command hands to another shell: the -c
// script of bash/sh/dash/zsh/ksh (else its heredoc or here-string), eval's
// arguments, the remote command of ssh, and env -S's string, after any
// wrappers.
func nested(words []string, redirs []*syntax.Redirect) []string {
	i, split := skipWrappers(words)
	if i >= len(words) {
		return split
	}
	args := words[i+1:]
	switch path.Base(words[i]) {
	case "bash", "sh", "dash", "zsh", "ksh":
		return append(split, shellScripts(args, redirs)...)
	case "eval":
		if len(args) > 0 && args[0] == "--" {
			args = args[1:]
		}
		return append(split, strings.Join(args, " "))
	case "ssh":
		j := 0
		for j < len(args) && strings.HasPrefix(args[j], "-") {
			a := args[j]
			j++
			if a == "--" {
				break
			}
			if _, _, took := shortValue(a, "BbcDEeFIiJLlmOoPpQRSWw", args[j:]); took {
				j++
			}
		}
		if j+1 < len(args) {
			return append(split, strings.Join(args[j+1:], " "))
		}
	}
	return split
}

// stdinPath names the script file operands that are the shell's own stdin.
var stdinPath = map[string]bool{"/dev/stdin": true, "/dev/fd/0": true, "/proc/self/fd/0": true}

// shellScripts is what a shell runs from its arguments and redirections:
// the first operand after its options when one of them has c; else, when
// there is no script file operand (or -s says read stdin), the body of its
// heredoc or here-string.
func shellScripts(args []string, redirs []*syntax.Redirect) []string {
	c, s, j := false, false, 0
options:
	for ; j < len(args); j++ {
		a := args[j]
		switch {
		case a == "--" || a == "-":
			j++
			break options
		case len(a) < 2 || (a[0] != '-' && a[0] != '+'):
			break options
		case a == "-o" || a == "+o" || a == "-O" || a == "+O":
			j++
		case a == "--rcfile" || a == "--init-file":
			j++
		case !strings.HasPrefix(a, "--"):
			if a[0] == '-' {
				c = c || strings.ContainsRune(a, 'c')
				s = s || strings.ContainsRune(a, 's')
			}
			if strings.ContainsAny(a[1:], "oO") {
				j++ // -eo pipefail: the option name
			}
		}
	}
	if c {
		if j < len(args) {
			return []string{args[j]}
		}
		return nil
	}
	if j < len(args) && !s && !stdinPath[args[j]] {
		return nil // stdin is the script's data, not a script
	}
	var docs []string
	for _, r := range redirs {
		switch {
		case r.Hdoc != nil:
			docs = append(docs, heredoc(r))
		case r.Op == syntax.WordHdoc && r.Word != nil:
			docs = append(docs, word(r.Word))
		}
	}
	return docs
}

// heredoc is a heredoc's body as the shell reading it gets it: as written
// when the delimiter is quoted (<<'EOF'), else with \$ \` \\ and
// \<newline> resolved.
func heredoc(r *syntax.Redirect) string {
	quoted := false
	if r.Word != nil {
		for _, p := range r.Word.Parts {
			if l, ok := p.(*syntax.Lit); !ok || strings.Contains(l.Value, `\`) {
				quoted = true
			}
		}
	}
	var b strings.Builder
	for _, p := range r.Hdoc.Parts {
		if l, ok := p.(*syntax.Lit); ok {
			if quoted {
				b.WriteString(l.Value)
			} else {
				b.WriteString(unescapeSet(l.Value, true, "$`\\\n"))
			}
			continue
		}
		part(&b, p)
	}
	return b.String()
}

// word is what bash passes as the argument, for the parts that are literals.
func word(w *syntax.Word) string {
	var b strings.Builder
	for _, p := range w.Parts {
		part(&b, p)
	}
	return b.String()
}

func part(b *strings.Builder, p syntax.WordPart) {
	switch p := p.(type) {
	case *syntax.Lit:
		b.WriteString(unescapeSet(p.Value, false, ""))
	case *syntax.SglQuoted:
		if p.Dollar {
			b.WriteString(ansiC(p.Value))
		} else {
			b.WriteString(p.Value)
		}
	case *syntax.DblQuoted:
		for _, q := range p.Parts {
			if l, ok := q.(*syntax.Lit); ok {
				b.WriteString(unescapeSet(l.Value, true, "$`\"\\\n"))
				continue
			}
			part(b, q)
		}
	case *syntax.ParamExp:
		if p.Param != nil {
			b.WriteString("$" + p.Param.Value)
		} else {
			b.WriteString("$")
		}
	case *syntax.CmdSubst:
		b.WriteString("$(...)")
	default:
		b.WriteString("$")
	}
}

// unescapeSet drops the backslashes bash removes: every \x when not quoted;
// when quoted (double quotes, an unquoted heredoc body) only before a
// character in set. \<newline> is removed whole.
func unescapeSet(s string, quoted bool, set string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != '\\' || i+1 == len(s) {
			b.WriteByte(c)
			continue
		}
		n := s[i+1]
		if quoted && !strings.ContainsRune(set, rune(n)) {
			b.WriteByte(c)
			continue
		}
		i++
		if n != '\n' {
			b.WriteByte(n)
		}
	}
	return b.String()
}

var simpleEscapes = map[byte]byte{
	'n': '\n', 't': '\t', 'r': '\r', 'a': '\a', 'b': '\b',
	'e': 0x1b, 'E': 0x1b, 'f': '\f', 'v': '\v',
}

// ansiC decodes the body of a $'...' string as bash does, ending it at a
// NUL as bash does.
func ansiC(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 == len(s) {
			b.WriteByte(s[i])
			continue
		}
		i++
		c := s[i]
		if e, ok := simpleEscapes[c]; ok {
			b.WriteByte(e)
			continue
		}
		var v uint64
		switch c {
		case 'x', 'u', 'U':
			width := map[byte]int{'x': 2, 'u': 4, 'U': 8}[c]
			j := i + 1
			for j < len(s) && j < i+1+width && isHex(s[j]) {
				j++
			}
			if j == i+1 {
				b.WriteByte('\\')
				b.WriteByte(c)
				continue
			}
			v, _ = strconv.ParseUint(s[i+1:j], 16, 32)
			i = j - 1
			if c != 'x' && v >= utf8.RuneSelf {
				b.WriteRune(rune(v))
				continue
			}
		case '0', '1', '2', '3', '4', '5', '6', '7':
			j := i
			for j < len(s) && j < i+3 && s[j] >= '0' && s[j] <= '7' {
				j++
			}
			v, _ = strconv.ParseUint(s[i:j], 8, 16)
			v &= 0xff
			i = j - 1
		case 'c':
			if i+1 == len(s) {
				b.WriteString(`\c`)
				continue
			}
			i++
			v = uint64(s[i] & 0x1f)
		case '\\', '\'', '"', '?':
			b.WriteByte(c)
			continue
		default:
			// an escape bash does not know keeps its backslash (\<newline> too)
			b.WriteByte('\\')
			b.WriteByte(c)
			continue
		}
		if v == 0 {
			return b.String()
		}
		b.WriteByte(byte(v))
	}
	return b.String()
}

func isHex(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

// cleanPaths resolves //, /./ and /../ in a word the way the kernel would
// read it as a path. A leading NAME= or --flag= (no / before the first =)
// stays as it is; an = later in the word is part of the path
// (~/.config/a=b/../gff). Spaces are part of the path too ("a b/../gff").
func cleanPaths(s string) string {
	head, p := "", s
	if i := strings.IndexByte(s, '='); i >= 0 && !strings.Contains(s[:i], "/") {
		head, p = s[:i+1], s[i+1:]
	}
	if !strings.Contains(p, "//") && !strings.Contains(p, "/./") && !strings.Contains(p, "/../") &&
		!strings.HasSuffix(p, "/.") && !strings.HasSuffix(p, "/..") {
		return s
	}
	c := path.Clean(p)
	if strings.HasSuffix(p, "/") && c != "/" {
		c += "/"
	}
	return head + c
}

// Print writes Lines(src) to w, one per line, and returns the parse error
// after the lines before it.
func Print(w io.Writer, src string) error {
	ls, perr := Lines(src)
	for _, l := range ls {
		if _, err := fmt.Fprintln(w, l); err != nil {
			return err
		}
	}
	return perr
}
