package wswhiteboard

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

// The whiteboard.update input is one string of flat commands in an XML-like
// shape. It is not XML: Go's encoding/xml is strict and would make every `<`
// and `&` in a body an escape, which is the cost the format exists to avoid.
// The rules this tokenizer implements:
//
//   - Every command and field tag carries the literal prefix "wb:"; there is
//     no namespace declaration and no root element.
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

// tagKind is how a tag's content is read.
type tagKind int

const (
	// kindLeaf takes a raw text body (or none, when self-closing).
	kindLeaf tagKind = iota
	// kindContainer takes field tags, each a leaf without attributes.
	kindContainer
	// kindEmpty takes nothing between its tags.
	kindEmpty
)

type tagSpec struct {
	kind     tagKind
	attrs    []string
	children []string
}

var itemFieldTags = []string{"wb:title", "wb:option", "wb:recommend", "wb:why", "wb:link"}

// commandSpecs is the command grammar; the order of the playbook's table.
var commandSpecs = map[string]tagSpec{
	"wb:add":       {kind: kindContainer, attrs: []string{"id", "class", "topic"}, children: itemFieldTags},
	"wb:answer":    {kind: kindLeaf, attrs: []string{"id", "ids", "value", "assumed"}},
	"wb:patch":     {kind: kindContainer, attrs: []string{"id", "status", "clear"}, children: itemFieldTags},
	"wb:topic":     {kind: kindLeaf, attrs: []string{"name"}},
	"wb:edit":      {kind: kindContainer, attrs: []string{"topic", "replace_all"}, children: []string{"wb:old", "wb:new"}},
	"wb:close":     {kind: kindEmpty, attrs: []string{"topic"}},
	"wb:floor":     {kind: kindEmpty, attrs: []string{"id"}},
	"wb:embed-dir": {kind: kindEmpty, attrs: []string{"path"}},
}

func commandNames() string {
	names := make([]string, 0, len(commandSpecs))
	for name := range commandSpecs {
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

type attr struct {
	name, value string
}

// field is one field tag inside a container command.
type field struct {
	tag  string
	line int
	body string
}

// command is one parsed top-level command.
type command struct {
	pos   int // 1-based position in the batch
	line  int
	tag   string
	attrs []attr
	// body is a leaf command's text.
	body   string
	fields []field
}

// label names the command in a problem: its position, tag, and line.
func (c command) label() string {
	return fmt.Sprintf("command %d <%s> (line %d)", c.pos, c.tag, c.line)
}

// at names one attribute or field of the command in a problem.
func (c command) at(name string) string {
	return c.label() + " " + name
}

func (c command) attr(name string) (string, bool) {
	for _, a := range c.attrs {
		if a.name == name {
			return a.value, true
		}
	}
	return "", false
}

// fieldsOf returns the bodies of every field tag named tag, in order.
func (c command) fieldsOf(tag string) []string {
	var out []string
	for _, f := range c.fields {
		if f.tag == tag {
			out = append(out, f.body)
		}
	}
	return out
}

// single returns the one field tag named tag; more than one is a problem.
func (c command) single(p *problems, tag string) (string, bool) {
	bodies := c.fieldsOf(tag)
	if len(bodies) > 1 {
		p.add("%s: more than one <%s>", c.label(), tag)
	}
	if len(bodies) == 0 {
		return "", false
	}
	return bodies[0], true
}

// tokenizer reads a command string. A problem it can step past (an unknown
// tag with a closing tag, stray text) is recorded and reading continues; one
// it cannot (an unterminated body or tag) ends reading, and the commands read
// before it are still validated, so one refusal lists as much as it can.
type tokenizer struct {
	s     string
	i     int
	p     *problems
	fatal bool
}

func (t *tokenizer) lineAt(i int) int {
	return strings.Count(t.s[:i], "\n") + 1
}

func (t *tokenizer) fail(i int, format string, args ...any) {
	t.p.add("line %d: %s", t.lineAt(i), fmt.Sprintf(format, args...))
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
	attrs       []attr
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
		tag.attrs = append(tag.attrs, attr{name: name, value: t.s[t.i+1 : t.i+1+end]})
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
	t.p.add("line %d: %s closes nothing open", t.lineAt(t.i), t.s[t.i:t.i+end+1])
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
	t.p.add("line %d: text %s: %q", t.lineAt(start), where, text)
}

func checkAttrs(p *problems, label string, attrs []attr, known []string) {
	seen := map[string]bool{}
	for _, a := range attrs {
		if seen[a.name] {
			p.add("%s: attribute %s given twice", label, a.name)
			continue
		}
		seen[a.name] = true
		found := false
		for _, k := range known {
			if k == a.name {
				found = true
				break
			}
		}
		if !found {
			if len(known) == 0 {
				p.add("%s: unknown attribute %s (it takes none)", label, a.name)
			} else {
				p.add("%s: unknown attribute %s (known: %s)", label, a.name, strings.Join(known, ", "))
			}
		}
	}
}

// parseCommands reads every command in s, recording each problem in p.
func parseCommands(p *problems, s string) []command {
	t := &tokenizer{s: s, p: p}
	var out []command
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
		cmd := command{pos: pos, line: t.lineAt(tag.at), tag: tag.name, attrs: tag.attrs}
		spec, known := commandSpecs[tag.name]
		if !known {
			p.add("%s: unknown command (commands: %s)", cmd.label(), commandNames())
			t.skipUnknown(tag)
			continue
		}
		checkAttrs(p, cmd.label(), tag.attrs, spec.attrs)
		if !tag.selfClosing {
			switch spec.kind {
			case kindLeaf:
				body, ok := t.readBody(tag.name, tag.at)
				if !ok {
					return out
				}
				cmd.body = body
			case kindEmpty:
				t.skipSpace()
				closing := "</" + tag.name + ">"
				if strings.HasPrefix(t.s[t.i:], closing) {
					t.i += len(closing)
				} else {
					p.add("%s: takes no body; write it as <%s .../>", cmd.label(), tag.name)
					t.skipUnknown(tag)
				}
			case kindContainer:
				if !t.readFields(&cmd, spec) {
					return out
				}
			}
		}
		out = append(out, cmd)
	}
	return out
}

// readFields reads a container command's field tags up to its closing tag.
func (t *tokenizer) readFields(cmd *command, spec tagSpec) bool {
	closing := "</" + cmd.tag + ">"
	for {
		t.skipSpace()
		if t.i >= len(t.s) {
			t.fail(len(t.s), "<%s> from line %d has no closing %s", cmd.tag, cmd.line, closing)
			return false
		}
		if strings.HasPrefix(t.s[t.i:], closing) {
			t.i += len(closing)
			return true
		}
		if t.s[t.i] != '<' {
			t.skipText("inside " + cmd.label() + " outside its field tags")
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
		for _, child := range spec.children {
			if child == tag.name {
				known = true
				break
			}
		}
		line := t.lineAt(tag.at)
		if !known {
			t.p.add("%s: unknown field <%s> (fields: %s)", cmd.label(), tag.name, strings.Join(spec.children, ", "))
			t.skipUnknown(tag)
			if t.fatal {
				return false
			}
			continue
		}
		checkAttrs(t.p, fmt.Sprintf("%s field <%s>", cmd.label(), tag.name), tag.attrs, nil)
		f := field{tag: tag.name, line: line}
		if !tag.selfClosing {
			body, ok := t.readBody(tag.name, tag.at)
			if !ok {
				return false
			}
			f.body = body
		}
		cmd.fields = append(cmd.fields, f)
	}
}
