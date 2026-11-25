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
	"errors"
	"fmt"
	"io"
	"path"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"mvdan.cc/sh/v3/syntax"
)

// maxDepth bounds how far nested bash -c / eval / ssh / env -S scripts are
// followed.
const maxDepth = 4

// maxSteps and maxLines bound the work for one command, so crafted input
// (thousands of env -S, deeply nested redirected groups) cannot hold the
// hook past its timeout: past either, Lines stops and returns ErrTooBig
// with the lines so far.
const (
	maxSteps = 50000
	maxLines = 10000
)

// ErrTooBig: the command needs more work than maxSteps or maxLines allow.
var ErrTooBig = errors.New("command too large to parse")

// walk is one Lines call: its lines and the work spent.
type walk struct {
	out   []string
	steps int
	seen  map[string]bool // scripts already parsed: one heredoc read by 60 shells is parsed once
}

// script parses a nested script once; bash runs the statements before a
// syntax error, and so do we.
func (w *walk) script(src string, depth int, stdin []*syntax.Redirect) {
	if w.seen == nil {
		w.seen = map[string]bool{}
	}
	if w.seen[src] {
		return
	}
	w.seen[src] = true
	_ = w.lines(src, depth, stdin)
}

// spend counts one unit of work and reports whether the budget holds.
func (w *walk) spend() bool {
	w.steps++
	return w.steps <= maxSteps && len(w.out) < maxLines
}

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
// which bash would have run, with the error; past the work budget, the
// lines so far with ErrTooBig.
func Lines(src string) ([]string, error) {
	w := &walk{}
	err := w.lines(src, 0, nil)
	if !w.spend() {
		return w.out, ErrTooBig
	}
	return w.out, err
}

// lines parses src and records its commands. stdin is the heredocs and
// here-strings the script inherits (bash -c, eval and a script fed to a
// shell read the stdin of the command that ran them).
func (w *walk) lines(src string, depth int, stdin []*syntax.Redirect) error {
	return syntax.NewParser(syntax.Variant(syntax.LangBash)).Stmts(strings.NewReader(src), func(top *syntax.Stmt) bool {
		w.visit(top, depth, stdin)
		return w.spend()
	})
}

// visit records each statement under n. A statement's stdin is its own
// heredoc or here-string, else the one around it: ( bash ) <<E and
// { bash; } <<<x give it to the commands inside.
func (w *walk) visit(n syntax.Node, depth int, stdin []*syntax.Redirect) {
	syntax.Walk(n, func(m syntax.Node) bool {
		if !w.spend() {
			return false
		}
		if b, ok := m.(*syntax.BinaryCmd); ok && depth < maxDepth {
			for _, inner := range pipedScript(b) {
				w.script(inner, depth+1, nil)
			}
		}
		st, ok := m.(*syntax.Stmt)
		if !ok {
			return true
		}
		eff := stdin
		if own := stdinRedirs(st.Redirs); len(own) > 0 {
			eff = own
		}
		w.stmt(st, depth, eff)
		if _, simple := st.Cmd.(*syntax.CallExpr); !simple && st.Cmd != nil && len(eff) > 0 {
			w.visit(st.Cmd, depth, eff)
			// the redirections themselves: a $(...) in a here-string or an
			// unquoted heredoc runs too
			for _, r := range st.Redirs {
				w.visit(r, depth, stdin)
			}
			return false
		}
		return true
	})
}

// stdinRedirs is the heredocs and here-strings in redirs.
func stdinRedirs(redirs []*syntax.Redirect) []*syntax.Redirect {
	var in []*syntax.Redirect
	for _, r := range redirs {
		if r.Hdoc != nil || (r.Op == syntax.WordHdoc && r.Word != nil) {
			in = append(in, r)
		}
	}
	return in
}

func (w *walk) stmt(st *syntax.Stmt, depth int, stdin []*syntax.Redirect) {
	var parts, raw []string
	if call, ok := st.Cmd.(*syntax.CallExpr); ok {
		for _, a := range call.Assigns {
			v := ""
			if a.Value != nil {
				v = word(a.Value)
			}
			parts = append(parts, a.Name.Value+"="+token(v))
		}
		for _, a := range call.Args {
			r := word(a)
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
	w.emit(parts)
	if depth >= maxDepth || len(raw) == 0 {
		return
	}
	for _, inner := range w.nested(raw, stdin, depth) {
		// bash runs the statements before a syntax error, and so do we
		w.script(inner, depth+1, stdin)
	}
}

// emit records one line.
func (w *walk) emit(parts []string) {
	if len(parts) > 0 {
		w.out = append(w.out, strings.Join(parts, " "))
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
	"env":     {"uSCPa", []string{"--unset", "--chdir", "--split-string", "--argv0"}},
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
			// env -S STRING ARGS... runs env with STRING's words then ARGS
			ws := append([]string{"env"}, envSplit(v)...)
			return len(words), append(ws, rest...)
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

// nested returns the shell scripts a command hands to another shell: the -c
// script of bash/sh/dash/zsh/ksh (else its heredoc or here-string), what
// source or . reads from stdin, eval's arguments, the remote command of
// ssh, and the command env -S builds, after any wrappers. redirs are the
// command's own redirections and those of a ( ) or { } around it.
//
// depth counts env -S levels with the script levels, so a chain of env -S
// stops at maxDepth like nested scripts do.
func (w *walk) nested(words []string, redirs []*syntax.Redirect, depth int) []string {
	if !w.spend() {
		return nil
	}
	i, split := skipWrappers(words)
	if split != nil {
		if depth >= maxDepth {
			return nil
		}
		// env -S: the command env runs after its split options gets a line of
		// its own, and is followed like any other
		if k, again := skipWrappers(split); again == nil && k < len(split) {
			// from the first NAME=VALUE on, so an assignment stays on the line
			for k > 1 && strings.Contains(split[k-1], "=") && !strings.HasPrefix(split[k-1], "-") {
				k--
			}
			parts := make([]string, 0, len(split)-k)
			for _, a := range split[k:] {
				parts = append(parts, token(a))
			}
			w.emit(parts)
		}
		return w.nested(split, redirs, depth+1)
	}
	if i >= len(words) {
		return nil
	}
	args := words[i+1:]
	switch path.Base(words[i]) {
	case "bash", "sh", "dash", "zsh", "ksh":
		return shellScripts(args, redirs)
	case "source", ".":
		if len(args) > 0 && args[0] == "--" {
			args = args[1:]
		}
		if len(args) > 0 && isStdin(args[0]) {
			return stdinScripts(redirs)
		}
	case "eval":
		if len(args) > 0 && args[0] == "--" {
			args = args[1:]
		}
		return []string{strings.Join(args, " ")}
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
			return []string{strings.Join(args[j+1:], " ")}
		}
	}
	return nil
}

// envSplit splits an env -S string the way GNU env does: on spaces and
// tabs, with '...' literal, "..." and unquoted text taking \" \\ \' \_
// (a space) \t \n \r \f \v \# \$, \c ending the string, and # after a
// space starting a comment; an unquoted \_ separates words, a quoted one is a space. ; | & ( ) < > and ` are ordinary characters.
func envSplit(s string) []string {
	var words []string
	var cur strings.Builder
	in, inWord := byte(0), false
	flush := func() {
		if inWord {
			words = append(words, cur.String())
			cur.Reset()
			inWord = false
		}
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case in == '\'':
			if c == '\\' && i+1 < len(s) && (s[i+1] == '\'' || s[i+1] == '\\') {
				i++ // GNU env: \' and \\ inside '...'
				cur.WriteByte(s[i])
			} else if c == '\'' {
				in = 0
			} else {
				cur.WriteByte(c)
			}
			continue
		case c == '\\' && i+1 < len(s):
			i++
			e := s[i]
			if e == 'c' {
				flush()
				return words
			}
			if in == 0 && e == '_' {
				flush() // unquoted \_ separates words
				continue
			}
			inWord = true
			if r, ok := map[byte]byte{'t': '\t', 'n': '\n', 'r': '\r', 'f': '\f', 'v': '\v', '_': ' '}[e]; ok {
				cur.WriteByte(r)
			} else {
				cur.WriteByte(e)
			}
			continue
		case in == '"':
			if c == '"' {
				in = 0
			} else {
				cur.WriteByte(c)
			}
			continue
		case c == '\'' || c == '"':
			in, inWord = c, true
		case c == ' ' || c == '\t' || c == '\n':
			flush()
		case c == '#' && !inWord:
			return words
		default:
			cur.WriteByte(c)
			inWord = true
		}
	}
	flush()
	return words
}

// isStdin reports whether a script file operand is the shell's own stdin.
func isStdin(p string) bool {
	switch path.Clean(p) {
	case "/dev/stdin", "/dev/fd/0", "/proc/self/fd/0":
		return true
	}
	return false
}

// shellScripts is what a shell runs from its arguments and redirections:
// the first operand after its options when one of them has c; else, when
// there is no script file operand (or -s, or the operand is stdin), the
// body of its heredoc or here-string.
func shellScripts(args []string, redirs []*syntax.Redirect) []string {
	c, j, s := shellOptions(args)
	if c {
		if j < len(args) {
			return []string{args[j]}
		}
		return nil
	}
	if j < len(args) && !s && !isStdin(args[j]) {
		return nil // stdin is the script's data, not a script
	}
	return stdinScripts(redirs)
}

// shellOptions reads a shell's options: whether one has c, the index of
// the first operand, and whether one has s.
func shellOptions(args []string) (c bool, j int, s bool) {
options:
	for ; j < len(args); j++ {
		a := args[j]
		switch {
		case a == "--" || a == "-":
			j++
			break options
		case len(a) < 2 || (a[0] != '-' && a[0] != '+'):
			break options
		case a == "--rcfile" || a == "--init-file":
			j++
		case !strings.HasPrefix(a, "--"):
			if a[0] == '-' {
				c = c || strings.ContainsRune(a, 'c')
				s = s || strings.ContainsRune(a, 's')
			}
			// each o / O takes an option name: -eo pipefail, -oo a b
			j += strings.Count(a[1:], "o") + strings.Count(a[1:], "O")
		}
	}
	return c, j, s
}

// pipedScript is the text a pipeline pipes into a shell that reads its
// script from stdin (echo '...' | bash, printf ... | sh, cat <<E | bash):
// the arguments of echo or printf, or cat's heredoc or here-string.
func pipedScript(b *syntax.BinaryCmd) []string {
	if b.Op != syntax.Pipe && b.Op != syntax.PipeAll {
		return nil
	}
	right, ok := b.Y.Cmd.(*syntax.CallExpr)
	if !ok || !readsScriptFromStdin(callWords(right)) {
		return nil
	}
	left, ok := b.X.Cmd.(*syntax.CallExpr)
	if !ok {
		return nil
	}
	ws := callWords(left)
	if len(ws) == 0 {
		return nil
	}
	switch path.Base(ws[0]) {
	case "echo":
		args := ws[1:]
		for len(args) > 0 && strings.HasPrefix(args[0], "-") {
			args = args[1:]
		}
		// as printed, and with echo -e's escapes decoded
		text := strings.Join(args, " ")
		return []string{text, ansiC(text)}
	case "printf":
		args := ws[1:]
		if len(args) > 0 && args[0] == "--" {
			args = args[1:]
		}
		if len(args) == 0 {
			return nil
		}
		// the format with its escapes decoded, and the arguments one per line
		// (printf '%s\n' 'gss push')
		return []string{ansiC(args[0]), strings.Join(args[1:], "\n")}
	case "cat":
		if len(ws) == 1 {
			return stdinScripts(b.X.Redirs)
		}
	}
	return nil
}

// readsScriptFromStdin reports whether a command (past its wrappers) is a
// shell that reads its script from stdin, or source / . of stdin.
func readsScriptFromStdin(words []string) bool {
	i, split := skipWrappers(words)
	if split != nil || i >= len(words) {
		return false
	}
	args := words[i+1:]
	switch path.Base(words[i]) {
	case "bash", "sh", "dash", "zsh", "ksh":
		c, j, s := shellOptions(args)
		return !c && (j >= len(args) || s || isStdin(args[j]))
	case "source", ".":
		if len(args) > 0 && args[0] == "--" {
			args = args[1:]
		}
		return len(args) > 0 && isStdin(args[0])
	}
	return false
}

// callWords is the resolved words of a simple command.
func callWords(call *syntax.CallExpr) []string {
	ws := make([]string, 0, len(call.Args))
	for _, a := range call.Args {
		ws = append(ws, word(a))
	}
	return ws
}

// stdinScripts is the body of each heredoc and here-string in redirs.
func stdinScripts(redirs []*syntax.Redirect) []string {
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
	lineStart := true
	for _, p := range r.Hdoc.Parts {
		if l, ok := p.(*syntax.Lit); ok {
			v := l.Value
			if !quoted {
				// \<newline> joins lines before <<- strips tabs, so a
				// continued line keeps its tab (checked against bash)
				v = unescapeSet(v, true, "$`\\\n")
			}
			if r.Op == syntax.DashHdoc {
				v = stripTabs(v, lineStart) // <<- drops each line's leading tabs
			}
			if v != "" {
				lineStart = strings.HasSuffix(v, "\n")
			}
			b.WriteString(v)
			continue
		}
		lineStart = false
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
			if s[i] == '\\' && i+1 < len(s) && s[i+1] == '\\' {
				i++ // \c\\ is control-backslash, both backslashes taken
			}
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
	if strings.ContainsAny(p, " \t\n") {
		// a path with a space (a b/../gff), or several words: clean it whole
		// and each word, and keep both, so .. cannot erase a word
		f := strings.Fields(p)
		for i := range f {
			f[i] = cleanPaths(f[i])
		}
		return head + c + " " + strings.Join(f, " ")
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

// stripTabs drops the tabs that start each line of a <<- heredoc, as bash
// does before it reads the body; atStart says s begins a line.
func stripTabs(s string, atStart bool) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if atStart && s[i] == '\t' {
			continue
		}
		atStart = s[i] == '\n'
		b.WriteByte(s[i])
	}
	return b.String()
}
