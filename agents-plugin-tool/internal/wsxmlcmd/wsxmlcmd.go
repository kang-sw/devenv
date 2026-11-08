// Package wsxmlcmd reads the flat XML-like command language an MCP tool takes
// as one string argument (whiteboard.update today). Each caller supplies a
// Grammar: its tag prefix and its command table. The package tokenizes the
// string against that grammar and records every problem it finds, and offers
// the shared value checks a caller's batch validation uses; what a command
// means stays with the caller.
//
// The language is not XML: Go's encoding/xml is strict and would make every
// `<` and `&` in a body an escape, which is the cost the format exists to
// avoid. The rules this tokenizer implements:
//
//   - Every command and field tag carries the grammar's literal prefix
//     ("wb:" for the whiteboard); there is no namespace declaration and no
//     root element.
//   - A body is raw text up to the first exact closing tag of its element
//     (`</wb:why>`). Nothing in it is decoded, so `<`, `&`, inline HTML, and
//     mermaid are written as is. A CDATA section (`<![CDATA[...]]>`) passes
//     its content through literally, for a body that must contain its own
//     closing tag.
//   - A body loses one newline right after its opening tag and one right
//     before its closing tag, so a body written on its own lines keeps its
//     text exactly; nothing else is trimmed.
//   - An attribute value is quoted with " or ' and runs to the matching
//     quote; nothing in it is decoded either.
//   - An unknown tag or attribute is a problem; so is text between commands
//     or between a command's fields.
//
// The workflow manual's shared command-syntax section states these rules to
// the lead; a change here changes that text's contract.
package wsxmlcmd

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

// Kind is how a command's content is read.
type Kind int

const (
	// Leaf takes a raw text body (or none, when self-closing).
	Leaf Kind = iota
	// Container takes field tags, each a leaf without attributes.
	Container
	// Empty takes nothing between its tags.
	Empty
)

// Spec is one command of a grammar.
type Spec struct {
	Kind Kind
	// Attrs are the attribute names the command takes, in the order a
	// problem lists them.
	Attrs []string
	// Fields are a Container's field tag names without the prefix, in the
	// order a problem lists them.
	Fields []string
}

// Grammar is a caller's command language: its tag prefix and its command
// table, keyed by command name without the prefix. Parsed commands and
// fields carry the full tag as written (prefix included).
type Grammar struct {
	Prefix   string
	Commands map[string]Spec
}

func (g Grammar) lookup(tag string) (Spec, bool) {
	if !strings.HasPrefix(tag, g.Prefix) {
		return Spec{}, false
	}
	spec, ok := g.Commands[tag[len(g.Prefix):]]
	return spec, ok
}

func (g Grammar) commandNames() string {
	names := make([]string, 0, len(g.Commands))
	for name := range g.Commands {
		names = append(names, g.Prefix+name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

func (g Grammar) fieldTags(spec Spec) []string {
	tags := make([]string, len(spec.Fields))
	for i, f := range spec.Fields {
		tags[i] = g.Prefix + f
	}
	return tags
}

// Attr is one attribute of a command tag.
type Attr struct {
	Name, Value string
}

// Field is one field tag inside a container command.
type Field struct {
	Tag  string
	Line int
	Body string
}

// Command is one parsed top-level command.
type Command struct {
	Pos   int // 1-based position in the batch
	Line  int
	Tag   string
	Attrs []Attr
	// Body is a leaf command's text.
	Body   string
	Fields []Field
}

// Label names the command in a problem: its position, tag, and line.
func (c Command) Label() string {
	return fmt.Sprintf("command %d <%s> (line %d)", c.Pos, c.Tag, c.Line)
}

// At names one attribute or field of the command in a problem.
func (c Command) At(name string) string {
	return c.Label() + " " + name
}

// Attr returns the value of the attribute named name and whether the
// command carries it.
func (c Command) Attr(name string) (string, bool) {
	for _, a := range c.Attrs {
		if a.Name == name {
			return a.Value, true
		}
	}
	return "", false
}

// FieldsOf returns the bodies of every field tag named tag, in order.
func (c Command) FieldsOf(tag string) []string {
	var out []string
	for _, f := range c.Fields {
		if f.Tag == tag {
			out = append(out, f.Body)
		}
	}
	return out
}

// Single returns the one field tag named tag; more than one is a problem.
func (c Command) Single(p *Problems, tag string) (string, bool) {
	bodies := c.FieldsOf(tag)
	if len(bodies) > 1 {
		p.Add("%s: more than one <%s>", c.Label(), tag)
	}
	if len(bodies) == 0 {
		return "", false
	}
	return bodies[0], true
}

// Parse reads every command in s under g, recording each problem in p.
//
// A problem the tokenizer can step past (an unknown tag with a closing tag,
// stray text) is recorded and reading continues; one it cannot (an
// unterminated body or tag) ends reading, and the commands read before it are
// still returned, so the caller's validation lists as much as it can in one
// refusal.
func Parse(p *Problems, g Grammar, s string) []Command {
	t := &tokenizer{s: s, p: p}
	var out []Command
	pos := 0
	for !t.fatal {
		t.skipSpace()
		if t.i >= len(t.s) {
			break
		}
		if t.s[t.i] != '<' {
			t.skipText("outside a command")
			continue
		}
		if t.strayClosing() {
			continue
		}
		tag, ok := t.readOpen()
		if !ok {
			break
		}
		pos++
		cmd := Command{Pos: pos, Line: t.lineAt(tag.at), Tag: tag.name, Attrs: tag.attrs}
		spec, known := g.lookup(tag.name)
		if !known {
			p.Add("%s: unknown command (commands: %s)", cmd.Label(), g.commandNames())
			t.skipUnknown(tag)
			continue
		}
		checkAttrs(p, cmd.Label(), tag.attrs, spec.Attrs)
		if !tag.selfClosing {
			switch spec.Kind {
			case Leaf:
				body, ok := t.readBody(tag.name, tag.at)
				if !ok {
					return out
				}
				cmd.Body = body
			case Empty:
				t.skipSpace()
				closing := "</" + tag.name + ">"
				if strings.HasPrefix(t.s[t.i:], closing) {
					t.i += len(closing)
				} else {
					p.Add("%s: takes no body; write it as <%s .../>", cmd.Label(), tag.name)
					t.skipUnknown(tag)
				}
			case Container:
				if !t.readFields(&cmd, g.fieldTags(spec)) {
					return out
				}
			}
		}
		out = append(out, cmd)
	}
	return out
}

// tokenizer reads a command string; fatal is set once reading cannot go on.
type tokenizer struct {
	s     string
	i     int
	p     *Problems
	fatal bool
}

func (t *tokenizer) lineAt(i int) int {
	return strings.Count(t.s[:i], "\n") + 1
}

func (t *tokenizer) fail(i int, format string, args ...any) {
	t.p.Add("line %d: %s", t.lineAt(i), fmt.Sprintf(format, args...))
	t.fatal = true
}

func (t *tokenizer) skipSpace() {
	for t.i < len(t.s) && isSpace(t.s[t.i]) {
		t.i++
	}
}

func isSpace(b byte) bool { return b == ' ' || b == '\t' || b == '\n' || b == '\r' }

func isNameByte(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == ':' || b == '_' || b == '-' || b == '.'
}

// openTag is a parsed opening tag.
type openTag struct {
	at          int
	name        string
	attrs       []Attr
	selfClosing bool
}

// readOpen reads an opening tag at t.i, which holds '<'.
func (t *tokenizer) readOpen() (openTag, bool) {
	tag := openTag{at: t.i}
	t.i++
	start := t.i
	for t.i < len(t.s) && isNameByte(t.s[t.i]) {
		t.i++
	}
	tag.name = t.s[start:t.i]
	if tag.name == "" {
		t.fail(tag.at, "expected a tag name after '<'")
		return tag, false
	}
	for {
		t.skipSpace()
		if t.i >= len(t.s) {
			t.fail(tag.at, "<%s> is not closed with '>'", tag.name)
			return tag, false
		}
		switch t.s[t.i] {
		case '>':
			t.i++
			return tag, true
		case '/':
			if strings.HasPrefix(t.s[t.i:], "/>") {
				t.i += 2
				tag.selfClosing = true
				return tag, true
			}
			t.fail(t.i, "<%s>: expected '/>'", tag.name)
			return tag, false
		}
		nameStart := t.i
		for t.i < len(t.s) && isNameByte(t.s[t.i]) {
			t.i++
		}
		name := t.s[nameStart:t.i]
		if name == "" {
			r, _ := utf8.DecodeRuneInString(t.s[t.i:])
			t.fail(t.i, "<%s>: unexpected %q in the tag", tag.name, r)
			return tag, false
		}
		t.skipSpace()
		if t.i >= len(t.s) || t.s[t.i] != '=' {
			t.fail(t.i, "<%s>: attribute %s needs =\"value\"", tag.name, name)
			return tag, false
		}
		t.i++
		t.skipSpace()
		if t.i >= len(t.s) || (t.s[t.i] != '"' && t.s[t.i] != '\'') {
			t.fail(t.i, "<%s>: attribute %s takes a value quoted with \" or '", tag.name, name)
			return tag, false
		}
		quote := t.s[t.i]
		end := strings.IndexByte(t.s[t.i+1:], quote)
		if end < 0 {
			t.fail(t.i, "<%s>: attribute %s has no closing %c", tag.name, name, quote)
			return tag, false
		}
		tag.attrs = append(tag.attrs, Attr{Name: name, Value: t.s[t.i+1 : t.i+1+end]})
		t.i += end + 2
	}
}

// readBody reads raw text up to the exact closing tag of name, passing CDATA
// sections through, and strips the one newline at each end.
func (t *tokenizer) readBody(name string, from int) (string, bool) {
	closing := "</" + name + ">"
	var b strings.Builder
	for {
		next := strings.IndexByte(t.s[t.i:], '<')
		if next < 0 {
			t.fail(from, "<%s> has no closing %s", name, closing)
			return "", false
		}
		b.WriteString(t.s[t.i : t.i+next])
		t.i += next
		rest := t.s[t.i:]
		switch {
		case strings.HasPrefix(rest, closing):
			t.i += len(closing)
			return stripBodyNewlines(b.String()), true
		case strings.HasPrefix(rest, "<![CDATA["):
			end := strings.Index(rest[9:], "]]>")
			if end < 0 {
				t.fail(t.i, "<%s>: a CDATA section has no closing ]]>", name)
				return "", false
			}
			b.WriteString(rest[9 : 9+end])
			t.i += 9 + end + 3
		default:
			b.WriteByte('<')
			t.i++
		}
	}
}

func stripBodyNewlines(s string) string {
	if strings.HasPrefix(s, "\r\n") {
		s = s[2:]
	} else if strings.HasPrefix(s, "\n") {
		s = s[1:]
	}
	if strings.HasSuffix(s, "\r\n") {
		s = s[:len(s)-2]
	} else if strings.HasSuffix(s, "\n") {
		s = s[:len(s)-1]
	}
	return s
}

// skipUnknown steps past an unknown tag's content up to its closing tag, so
// reading can continue; without one, reading ends.
func (t *tokenizer) skipUnknown(tag openTag) {
	if tag.selfClosing {
		return
	}
	closing := "</" + tag.name + ">"
	end := strings.Index(t.s[t.i:], closing)
	if end < 0 {
		t.fail(tag.at, "<%s> has no closing %s", tag.name, closing)
		return
	}
	t.i += end + len(closing)
}

// strayClosing records and steps past a closing tag at t.i that closes
// nothing open; it reports false when t.i holds no closing tag.
func (t *tokenizer) strayClosing() bool {
	if !strings.HasPrefix(t.s[t.i:], "</") {
		return false
	}
	end := strings.IndexByte(t.s[t.i:], '>')
	if end < 0 {
		t.fail(t.i, "a closing tag is not closed with '>'")
		return true
	}
	t.p.Add("line %d: %s closes nothing open", t.lineAt(t.i), t.s[t.i:t.i+end+1])
	t.i += end + 1
	return true
}

// skipText records stray text and steps to the next '<'.
func (t *tokenizer) skipText(where string) {
	start := t.i
	next := strings.IndexByte(t.s[t.i:], '<')
	if next < 0 {
		t.i = len(t.s)
	} else {
		t.i += next
	}
	text := strings.TrimSpace(t.s[start:t.i])
	if len([]rune(text)) > 40 {
		text = string([]rune(text)[:40]) + "..."
	}
	t.p.Add("line %d: text %s: %q", t.lineAt(start), where, text)
}

func checkAttrs(p *Problems, label string, attrs []Attr, known []string) {
	seen := map[string]bool{}
	for _, a := range attrs {
		if seen[a.Name] {
			p.Add("%s: attribute %s given twice", label, a.Name)
			continue
		}
		seen[a.Name] = true
		found := false
		for _, k := range known {
			if k == a.Name {
				found = true
				break
			}
		}
		if !found {
			if len(known) == 0 {
				p.Add("%s: unknown attribute %s (it takes none)", label, a.Name)
			} else {
				p.Add("%s: unknown attribute %s (known: %s)", label, a.Name, strings.Join(known, ", "))
			}
		}
	}
}

// readFields reads a container command's field tags (fields, full tags) up
// to its closing tag.
func (t *tokenizer) readFields(cmd *Command, fields []string) bool {
	closing := "</" + cmd.Tag + ">"
	for {
		t.skipSpace()
		if t.i >= len(t.s) {
			t.fail(len(t.s), "<%s> from line %d has no closing %s", cmd.Tag, cmd.Line, closing)
			return false
		}
		if strings.HasPrefix(t.s[t.i:], closing) {
			t.i += len(closing)
			return true
		}
		if t.s[t.i] != '<' {
			t.skipText("inside " + cmd.Label() + " outside its field tags")
			continue
		}
		if t.strayClosing() {
			continue
		}
		tag, ok := t.readOpen()
		if !ok {
			return false
		}
		known := false
		for _, f := range fields {
			if f == tag.name {
				known = true
				break
			}
		}
		line := t.lineAt(tag.at)
		if !known {
			t.p.Add("%s: unknown field <%s> (fields: %s)", cmd.Label(), tag.name, strings.Join(fields, ", "))
			t.skipUnknown(tag)
			if t.fatal {
				return false
			}
			continue
		}
		checkAttrs(t.p, fmt.Sprintf("%s field <%s>", cmd.Label(), tag.name), tag.attrs, nil)
		f := Field{Tag: tag.name, Line: line}
		if !tag.selfClosing {
			body, ok := t.readBody(tag.name, tag.at)
			if !ok {
				return false
			}
			f.Body = body
		}
		cmd.Fields = append(cmd.Fields, f)
	}
}
