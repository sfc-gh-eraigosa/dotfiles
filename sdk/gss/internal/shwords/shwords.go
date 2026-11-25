// Package shwords turns a shell command into its simple commands, one per
// line, the way bash would see them: quotes, escapes, backslash-newlines and
// comments resolved, $'...' decoded, and the strings handed to bash -c, sh
// -c, eval and ssh parsed again (#474).
//
// The safety_guard hook runs its gss and gff checks on these lines as well
// as on the raw text, so a spelling the text match cannot follow (g\<nl>ss,
// echo \\<nl>rm ..., $'\x67ff' set ...) is still seen. It only adds
// refusals: when the command does not parse, the hook keeps its text match.
package shwords

import (
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// maxDepth bounds how far nested bash -c / eval / ssh strings are followed.
const maxDepth = 4

// Lines parses src as bash and returns one line per simple command: its
// assignments and words joined by single spaces, then each redirection as
// its operator glued to its target (">/x", "2>&1", "<<EOF"). A statement
// with redirections and no simple command ({ ...; } >x) gives a line of its
// redirections. A word that is not a literal (a parameter or a command
// substitution) keeps a $ marker; the commands inside a substitution are
// lines of their own.
func Lines(src string) ([]string, error) {
	var out []string
	if err := lines(src, 0, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func lines(src string, depth int, out *[]string) error {
	f, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(src), "")
	if err != nil {
		return err
	}
	syntax.Walk(f, func(n syntax.Node) bool {
		st, ok := n.(*syntax.Stmt)
		if !ok {
			return true
		}
		var parts []string
		call, _ := st.Cmd.(*syntax.CallExpr)
		var words []string
		if call != nil {
			for _, a := range call.Assigns {
				v := ""
				if a.Value != nil {
					v = word(a.Value)
				}
				parts = append(parts, a.Name.Value+"="+oneWord(data(v)))
			}
			for _, w := range call.Args {
				words = append(words, word(w))
			}
			for _, w := range words {
				parts = append(parts, data(w))
			}
		}
		for _, r := range st.Redirs {
			parts = append(parts, redirect(r))
		}
		if len(parts) > 0 {
			*out = append(*out, strings.Join(parts, " "))
		}
		if depth < maxDepth {
			for _, inner := range nested(words, st.Redirs) {
				// a string that does not parse is not a command bash runs
				_ = lines(inner, depth+1, out)
			}
		}
		return true
	})
	return nil
}

// nested returns the shell source a command hands to another shell: the
// string after -c for bash/sh/dash/zsh/ksh, eval's arguments, the remote
// command of ssh, and a here-document fed to a shell.
func nested(words []string, redirs []*syntax.Redirect) []string {
	if len(words) == 0 {
		return nil
	}
	switch path.Base(words[0]) {
	case "bash", "sh", "dash", "zsh", "ksh":
		for i := 1; i < len(words); i++ {
			w := words[i]
			if !strings.HasPrefix(w, "-") || w == "--" {
				break
			}
			if !strings.HasPrefix(w, "--") && strings.Contains(w, "c") && i+1 < len(words) {
				return []string{words[i+1]}
			}
		}
		var docs []string
		for _, r := range redirs {
			if r.Hdoc != nil {
				docs = append(docs, word(r.Hdoc))
			}
		}
		return docs
	case "eval":
		return []string{strings.Join(words[1:], " ")}
	case "ssh":
		i := 1
		for i < len(words) && strings.HasPrefix(words[i], "-") {
			w := words[i]
			i++
			if w == "--" {
				break
			}
			// the options that take a value, given apart: -p 22, -o X=y
			if len(w) == 2 && strings.ContainsRune("BbcDEeFIiJLlmOoPpQRSWw", rune(w[1])) {
				i++
			}
		}
		if i+1 < len(words) {
			return []string{strings.Join(words[i+1:], " ")}
		}
	}
	return nil
}

func redirect(r *syntax.Redirect) string {
	s := ""
	if r.N != nil {
		s = r.N.Value
	}
	s += r.Op.String()
	if r.Word != nil {
		s += oneWord(word(r.Word))
	}
	return s
}

// word is what bash passes as the argument, for the parts that are
// literals, with each path-like piece cleaned (a//b, a/./b, a/x/../b).
func word(w *syntax.Word) string {
	var b strings.Builder
	for _, p := range w.Parts {
		part(&b, p)
	}
	return cleanPaths(oneLine(b.String()))
}

func part(b *strings.Builder, p syntax.WordPart) {
	switch p := p.(type) {
	case *syntax.Lit:
		b.WriteString(unescape(p.Value, false))
	case *syntax.SglQuoted:
		if p.Dollar {
			b.WriteString(ansiC(p.Value))
		} else {
			b.WriteString(p.Value)
		}
	case *syntax.DblQuoted:
		for _, q := range p.Parts {
			if l, ok := q.(*syntax.Lit); ok {
				b.WriteString(unescape(l.Value, true))
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

// unescape drops the backslashes bash removes: any \x outside quotes, and
// \$ \` \" \\ and \<newline> inside double quotes.
func unescape(s string, dq bool) string {
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
		if dq && !strings.ContainsRune("$`\"\\\n", rune(n)) {
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

// data blanks < and > in an argument: unquoted they would be redirections, so
// in a word they are text (a message, a pattern), and a > left in the line
// would read as a redirection to whatever follows it.
func data(s string) string {
	return strings.NewReplacer("<", " ", ">", " ").Replace(s)
}

// ansiC decodes the body of a $'...' string.
func ansiC(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 == len(s) {
			b.WriteByte(s[i])
			continue
		}
		i++
		switch c := s[i]; c {
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		case 'r':
			b.WriteByte('\r')
		case 'a':
			b.WriteByte('\a')
		case 'b':
			b.WriteByte('\b')
		case 'e', 'E':
			b.WriteByte(0x1b)
		case 'f':
			b.WriteByte('\f')
		case 'v':
			b.WriteByte('\v')
		case 'x':
			j := i + 1
			for j < len(s) && j < i+3 && isHex(s[j]) {
				j++
			}
			if j == i+1 {
				b.WriteString(`\x`)
				continue
			}
			v, _ := strconv.ParseUint(s[i+1:j], 16, 8)
			b.WriteByte(byte(v))
			i = j - 1
		case '0', '1', '2', '3', '4', '5', '6', '7':
			j := i
			for j < len(s) && j < i+3 && s[j] >= '0' && s[j] <= '7' {
				j++
			}
			v, _ := strconv.ParseUint(s[i:j], 8, 8)
			b.WriteByte(byte(v))
			i = j - 1
		default:
			// \\ \' \" \? and anything else: the character itself
			b.WriteByte(c)
		}
	}
	return b.String()
}

func isHex(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

// oneWord keeps an assignment value or a redirection target one word in the
// line: its spaces become _, so X="a cat" rm ... does not read as cat.
func oneWord(s string) string {
	return strings.Join(strings.Fields(s), "_")
}

func oneLine(s string) string {
	return strings.NewReplacer("\n", " ", "\r", " ").Replace(s)
}

// cleanPaths cleans the path after the last = (so --output=a/../b and
// X=a//b are cleaned too) when it has a //, /./ or /../ in it.
func cleanPaths(s string) string {
	head, p := "", s
	if i := strings.LastIndexByte(s, '='); i >= 0 {
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

// Print writes Lines(src) to w, one per line.
func Print(w io.Writer, src string) error {
	ls, err := Lines(src)
	if err != nil {
		return err
	}
	for _, l := range ls {
		if _, err := fmt.Fprintln(w, l); err != nil {
			return err
		}
	}
	return nil
}
