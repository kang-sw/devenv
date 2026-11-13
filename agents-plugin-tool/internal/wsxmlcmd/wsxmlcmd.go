// Package wsxmlcmd reads the XML-like command language an MCP tool takes as
// one string argument (whiteboard.update, tracker.update). Each caller
// supplies a Grammar: its tag prefix, its command table, and its group, if
// any. The package tokenizes the string against that grammar and records
// every problem it finds, and offers the shared value checks a caller's batch
// validation uses; what a command means stays with the caller.
//
// The language is not XML: Go's encoding/xml is strict and would make every
// `<` and `&` in a body an escape, which is the cost the format exists to
// avoid. The rules this tokenizer implements:
//
//   - Every command and field tag carries the grammar's literal prefix
//     ("wb:" for the whiteboard); there is no namespace declaration and no
//     root element.
//   - A grammar may declare one level of grouping (Group): a group element at
//     the root passes its attribute to the commands inside it. A scoped
//     command sits only inside a group and every other command only outside
//     one; groups do not nest, and a group takes no command position. A
//     group holds at least one command, unless the grammar declares an
//     optional extra group attribute (Group.Extra) and the group carries it.
//   - A body is raw text up to the first exact closing tag of its element
//     (`</wb:why>`), so `<`, `&`, inline HTML, and mermaid are written as is.
//     A CDATA section (`<![CDATA[...]]>`) passes its content through
//     literally, for a body that must contain its own closing tag.
//   - A body loses one newline right after its opening tag and one right
//     before its closing tag, so a body written on its own lines keeps its
//     text exactly; nothing else is trimmed.
//   - An attribute value is quoted with " or ' and runs to the matching
//     quote.
//   - Escaping is never required, but an agent escapes anyway, so the
//     entities Decode knows are decoded in every attribute value and in each
//     body the grammar marks as plain text (Spec.DecodeBody,
//     Spec.DecodeFields), outside CDATA. Every other body is kept exactly as
//     written: it may carry HTML whose entities are meant, or be an operand
//     matched against stored text.
//   - An unknown tag or attribute is a problem; so is text between commands
//     (inside a group too) or between a command's fields.
//
// The workflow manual's shared command-syntax section states these rules to
// the lead; a change here changes that text's contract.
package wsxmlcmd

import (
	"fmt"
	"sort"
	"strconv"
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
	// DecodeBody marks a Leaf's body as plain text: its entities are
	// decoded (Decode), outside CDATA.
	DecodeBody bool
	// DecodeFields are the Container fields, without the prefix, whose
	// bodies are plain text and decoded like a DecodeBody body.
	DecodeFields []string
	// Scoped marks a command that sits only inside the grammar's group and
	// takes the group's attribute from it (Command.Scoped).
	Scoped bool
}

// Group is a grammar's one level of grouping: an element at the root whose
// attribute, required and non-blank, passes to the scoped commands inside
// it.
type Group struct {
	// Name is the group's tag name without the prefix.
	Name string
	// Attr is the group's required attribute.
	Attr string
	// Extra, when set, is an optional second attribute. A group that
	// carries it may hold no commands (or be written self-closing), and it
	// comes out of Parse as a Command of its own ahead of its commands
	// (Command.IsGroup), so the caller sees the attribute even then. A grammar
	// that leaves Extra empty reads groups exactly as before.
	Extra string
	// Hint, when set, follows the problem a group with no commands, or with
	// text ahead of its first command, gets: a group that once took a body
	// of its own points its callers at the form that replaced it.
	Hint string
}

// Grammar is a caller's command language: its tag prefix, its command
// table, keyed by command name without the prefix, and its group (nil for
// none). Parsed commands and fields carry the full tag as written (prefix
// included).
type Grammar struct {
	Prefix   string
	Commands map[string]Spec
	Group    *Group
	// RefuseFieldOpenings rejects literal sibling field openings in field bodies.
	RefuseFieldOpenings bool
}

func (g Grammar) isGroup(tag string) bool {
	return g.Group != nil && tag == g.Prefix+g.Group.Name
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
	if g.Group != nil {
		names = append(names, g.Prefix+g.Group.Name)
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

func (g Grammar) decodeFieldTags(spec Spec) map[string]bool {
	tags := map[string]bool{}
	for _, f := range spec.DecodeFields {
		tags[g.Prefix+f] = true
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

// Scope is the group a command sits in.
type Scope struct {
	Tag  string // as written, prefix included
	Line int
	// Attr is the group's attribute name; Value its value, "" when the
	// attribute is missing or blank (a problem Parse records).
	Attr, Value string
	// Extra is the value of the grammar's optional extra group attribute
	// (Group.Extra) as written, and HasExtra whether the group carries it.
	Extra    string
	HasExtra bool
	// Commands counts the commands the group holds.
	Commands int
}

// Label names the group in a problem: its tag and line.
func (s *Scope) Label() string {
	return fmt.Sprintf("<%s> (line %d)", s.Tag, s.Line)
}

// At names the group's attribute in a problem.
func (s *Scope) At() string {
	return s.Label() + " " + s.Attr
}

// Command is one parsed command, at the root or inside a group. A group
// that carries its grammar's extra attribute also comes out as a Command:
// Pos 0, Tag the group's tag, Attrs the group's, and Scope the group
// itself (IsGroup).
type Command struct {
	Pos   int // 1-based position in the batch; groups take none (0)
	Line  int
	Tag   string
	Attrs []Attr
	// Body is a leaf command's text.
	Body   string
	Fields []Field
	// Scope is the group the command sits in, nil at the root. Commands of
	// one group share it.
	Scope *Scope
}

// Scoped returns the attribute value of the command's group, and false at
// the root or when the group's attribute is missing or blank; Parse has
// recorded the problem either way.
func (c Command) Scoped() (string, bool) {
	if c.Scope == nil || c.Scope.Value == "" {
		return "", false
	}
	return c.Scope.Value, true
}

// IsGroup reports whether c stands for a group that carries its grammar's
// extra attribute rather than for a command.
func (c Command) IsGroup() bool {
	return c.Pos == 0 && c.Scope != nil
}

// Label names the command in a problem: its position, tag, and line; a
// group's Command is named as its group.
func (c Command) Label() string {
	if c.IsGroup() {
		return c.Scope.Label()
	}
	return fmt.Sprintf("command %d <%s> (line %d)", c.Pos, c.Tag, c.Line)
}

// At names one attribute or field of the command in a problem.
func (c Command) At(name string) string {
	return c.Label() + " " + name
}

// Attr returns the value of the attribute named name and whether the
// command carries it.
func (c Command) Attr(name string) (string, bool) {
	return attrOf(c.Attrs, name)
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
// Commands inside a group come out in order among the root's, each carrying
// the group as its Scope.
//
// A problem the tokenizer can step past (an unknown tag with a closing tag,
// stray text) is recorded and reading continues; one it cannot (an
// unterminated body or tag) ends reading, and the commands read before it are
// still returned, so the caller's validation lists as much as it can in one
// refusal.
func Parse(p *Problems, g Grammar, s string) []Command {
	r := &parser{tokenizer: tokenizer{s: s, p: p}, g: g}
	for !r.fatal {
		r.skipSpace()
		if r.i >= len(r.s) {
			break
		}
		if r.s[r.i] != '<' {
			r.skipText("outside a command")
			continue
		}
		if r.strayClosing() {
			continue
		}
		tag, ok := r.readOpen()
		if !ok {
			break
		}
		if g.isGroup(tag.name) {
			r.group(tag)
			continue
		}
		if !r.command(tag, nil) {
			break
		}
	}
	return r.out
}

// parser is one Parse call: the tokenizer, the grammar, and the commands
// read so far.
type parser struct {
	tokenizer
	g   Grammar
	out []Command
	pos int
}

// command reads the command whose opening tag was just read, inside scope
// (nil at the root); it reports false when reading cannot go on.
func (r *parser) command(tag openTag, scope *Scope) bool {
	r.pos++
	p := r.p
	cmd := Command{Pos: r.pos, Line: r.lineAt(tag.at), Tag: tag.name, Attrs: tag.attrs, Scope: scope}
	spec, known := r.g.lookup(tag.name)
	if !known {
		p.Add("%s: unknown command (commands: %s)", cmd.Label(), r.g.commandNames())
		r.skipUnknown(tag)
		return !r.fatal
	}
	if group := r.g.Group; group != nil {
		switch {
		case spec.Scoped && scope == nil:
			p.Add("%s: goes inside a <%s%s %s=\"...\"> group", cmd.Label(), r.g.Prefix, group.Name, group.Attr)
		case !spec.Scoped && scope != nil:
			p.Add("%s: goes at the root, outside %s", cmd.Label(), scope.Label())
		}
	}
	checkAttrs(p, cmd.Label(), tag.attrs, spec.Attrs)
	if !tag.selfClosing {
		switch spec.Kind {
		case Leaf:
			body, ok := r.readBody(tag.name, tag.at, spec.DecodeBody, nil, "")
			if !ok {
				return false
			}
			cmd.Body = body
		case Empty:
			r.skipSpace()
			closing := "</" + tag.name + ">"
			if strings.HasPrefix(r.s[r.i:], closing) {
				r.i += len(closing)
			} else {
				p.Add("%s: takes no body; write it as <%s .../>", cmd.Label(), tag.name)
				r.skipUnknown(tag)
			}
		case Container:
			if !r.readFields(&cmd, r.g.fieldTags(spec), r.g.decodeFieldTags(spec), r.g.RefuseFieldOpenings) {
				return false
			}
		}
	}
	r.out = append(r.out, cmd)
	return true
}

// group reads a group whose opening tag was just read: its attribute, then
// its commands up to its closing tag. Text or an unprefixed tag ahead of the
// first command is read as the body the group does not take: one problem,
// and reading steps past the whole group rather than reading that body as
// commands.
func (r *parser) group(tag openTag) {
	g, p := r.g.Group, r.p
	scope := &Scope{Tag: tag.name, Line: r.lineAt(tag.at), Attr: g.Attr}
	known := []string{g.Attr}
	if g.Extra != "" {
		known = append(known, g.Extra)
		scope.Extra, scope.HasExtra = attrOf(tag.attrs, g.Extra)
	}
	checkAttrs(p, scope.Label(), tag.attrs, known)
	if v, present := attrOf(tag.attrs, g.Attr); !present {
		p.Add("%s: required", scope.At())
	} else if NonEmpty(p, scope.At(), v) {
		scope.Value = v
	}
	if scope.HasExtra {
		r.out = append(r.out, Command{Line: scope.Line, Tag: tag.name, Attrs: tag.attrs, Scope: scope})
	}
	hint := ""
	if g.Hint != "" {
		hint = "; " + g.Hint
	}
	if tag.selfClosing {
		if !scope.HasExtra {
			p.Add("%s: holds no commands%s", scope.Label(), hint)
		}
		return
	}
	closing := "</" + tag.name + ">"
	commands := 0
	for {
		r.skipSpace()
		if r.i >= len(r.s) {
			r.fail(len(r.s), "<%s> from line %d has no closing %s", tag.name, scope.Line, closing)
			return
		}
		rest := r.s[r.i:]
		if strings.HasPrefix(rest, closing) {
			r.i += len(closing)
			break
		}
		if commands == 0 && (rest[0] != '<' || !strings.HasPrefix(rest[1:], r.g.Prefix) && !strings.HasPrefix(rest, "</")) {
			end := strings.Index(rest, closing)
			if end < 0 {
				r.fail(tag.at, "<%s> has no closing %s", tag.name, closing)
				return
			}
			p.Add("%s: text before its first command: %q%s", scope.Label(), excerpt(rest[:end]), hint)
			r.i += end + len(closing)
			return
		}
		if rest[0] != '<' {
			r.skipText("inside " + scope.Label() + " between its commands")
			continue
		}
		if r.strayClosing() {
			if r.fatal {
				return
			}
			continue
		}
		inner, ok := r.readOpen()
		if !ok {
			return
		}
		if r.g.isGroup(inner.name) {
			p.Add("line %d: <%s> inside %s; groups do not nest", r.lineAt(inner.at), inner.name, scope.Label())
			r.skipUnknown(inner)
			if r.fatal {
				return
			}
			continue
		}
		commands++
		scope.Commands = commands
		if !r.command(inner, scope) {
			return
		}
	}
	if commands == 0 && !scope.HasExtra {
		p.Add("%s: holds no commands%s", scope.Label(), hint)
	}
}

func attrOf(attrs []Attr, name string) (string, bool) {
	for _, a := range attrs {
		if a.Name == name {
			return a.Value, true
		}
	}
	return "", false
}

// excerpt is the start of s, trimmed, for a problem that quotes text.
func excerpt(s string) string {
	text := strings.TrimSpace(s)
	if len([]rune(text)) > 40 {
		text = string([]rune(text)[:40]) + "..."
	}
	return text
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
		tag.attrs = append(tag.attrs, Attr{Name: name, Value: Decode(t.s[t.i+1 : t.i+1+end])})
		t.i += end + 2
	}
}

// bodySegment is one run of a body: text as written, or a CDATA section's
// content.
type bodySegment struct {
	text  string
	cdata bool
}

// readBody reads raw text up to the exact closing tag of name, passing CDATA
// sections through, and strips the one newline at each end; with decode, it
// then decodes the text outside CDATA.
func (t *tokenizer) readBody(name string, from int, decode bool, fields []string, label string) (string, bool) {
	closing := "</" + name + ">"
	var segs []bodySegment
	var text strings.Builder
	flush := func() {
		if text.Len() > 0 {
			segs = append(segs, bodySegment{text: text.String()})
			text.Reset()
		}
	}
	for {
		next := strings.IndexByte(t.s[t.i:], '<')
		if next < 0 {
			t.fail(from, "<%s> has no closing %s", name, closing)
			return "", false
		}
		text.WriteString(t.s[t.i : t.i+next])
		t.i += next
		rest := t.s[t.i:]
		switch {
		case strings.HasPrefix(rest, closing):
			t.i += len(closing)
			flush()
			return joinBody(segs, decode), true
		case strings.HasPrefix(rest, "<![CDATA["):
			end := strings.Index(rest[9:], "]]>")
			if end < 0 {
				t.fail(t.i, "<%s>: a CDATA section has no closing ]]>", name)
				return "", false
			}
			flush()
			segs = append(segs, bodySegment{text: rest[9 : 9+end], cdata: true})
			t.i += 9 + end + 3
		default:
			for _, field := range fields {
				if strings.HasPrefix(rest, "<"+field+">") {
					t.p.Add("%s field <%s> (line %d): body holds an opening <%s> - missing </%s>?", label, name, t.lineAt(t.i), field, name)
					break
				}
			}
			text.WriteByte('<')
			t.i++
		}
	}
}

// joinBody strips the body's one newline at each end before decoding, so a
// decoded &#10; is never mistaken for the newline after the opening tag.
func joinBody(segs []bodySegment, decode bool) string {
	for i := range segs {
		if segs[i].text != "" {
			segs[i].text = stripLeadingNewline(segs[i].text)
			break
		}
	}
	for i := len(segs) - 1; i >= 0; i-- {
		if segs[i].text != "" {
			segs[i].text = stripTrailingNewline(segs[i].text)
			break
		}
	}
	var b strings.Builder
	for _, seg := range segs {
		if decode && !seg.cdata {
			b.WriteString(Decode(seg.text))
		} else {
			b.WriteString(seg.text)
		}
	}
	return b.String()
}

func stripLeadingNewline(s string) string {
	if strings.HasPrefix(s, "\r\n") {
		return s[2:]
	}
	return strings.TrimPrefix(s, "\n")
}

func stripTrailingNewline(s string) string {
	if strings.HasSuffix(s, "\r\n") {
		return s[:len(s)-2]
	}
	return strings.TrimSuffix(s, "\n")
}

// Decode replaces the five predefined XML entities (&lt; &gt; &amp; &quot;
// &apos;) and numeric character references (&#60; &#x3c;) with the
// characters they name, in one pass, so &amp;lt; becomes &lt;. Any other &
// sequence, and a numeric reference naming no valid character (zero, a
// surrogate, past U+10FFFF), stays as written.
func Decode(s string) string {
	if !strings.Contains(s, "&") {
		return s
	}
	var b strings.Builder
	for {
		amp := strings.IndexByte(s, '&')
		if amp < 0 {
			b.WriteString(s)
			return b.String()
		}
		b.WriteString(s[:amp])
		s = s[amp:]
		semi := strings.IndexByte(s, ';')
		if semi < 0 {
			b.WriteString(s)
			return b.String()
		}
		if r, ok := entityRune(s[1:semi]); ok {
			b.WriteRune(r)
			s = s[semi+1:]
			continue
		}
		b.WriteByte('&')
		s = s[1:]
	}
}

// entityRune is the character an entity or numeric reference name (the
// text between & and ;) stands for.
func entityRune(name string) (rune, bool) {
	switch name {
	case "lt":
		return '<', true
	case "gt":
		return '>', true
	case "amp":
		return '&', true
	case "quot":
		return '"', true
	case "apos":
		return '\'', true
	}
	digits, ok := strings.CutPrefix(name, "#")
	if !ok {
		return 0, false
	}
	base := 10
	if hex, isHex := strings.CutPrefix(digits, "x"); isHex {
		digits, base = hex, 16
	} else if hex, isHex := strings.CutPrefix(digits, "X"); isHex {
		digits, base = hex, 16
	}
	// ParseUint takes no sign, and its 32-bit bound keeps an absurdly long
	// reference from wrapping into range.
	if digits == "" || digits[0] == '+' || digits[0] == '-' {
		return 0, false
	}
	n, err := strconv.ParseUint(digits, base, 32)
	if err != nil {
		return 0, false
	}
	r := rune(n)
	if n == 0 || n > utf8.MaxRune || r >= 0xD800 && r <= 0xDFFF {
		return 0, false
	}
	return r, true
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
// nothing open; it reports false when t.i holds no closing tag. A closing
// tag with no '>' anywhere after it is fatal, and t.i moves to the end so no
// caller re-reads the same text.
func (t *tokenizer) strayClosing() bool {
	if !strings.HasPrefix(t.s[t.i:], "</") {
		return false
	}
	end := strings.IndexByte(t.s[t.i:], '>')
	if end < 0 {
		t.fail(t.i, "a closing tag is not closed with '>'")
		t.i = len(t.s)
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
	t.p.Add("line %d: text %s: %q", t.lineAt(start), where, excerpt(t.s[start:t.i]))
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
// to its closing tag, decoding the bodies of the tags in decode.
func (t *tokenizer) readFields(cmd *Command, fields []string, decode map[string]bool, refuseOpenings bool) bool {
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
			if t.fatal {
				return false
			}
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
			var openings []string
			if refuseOpenings {
				openings = fields
			}
			body, ok := t.readBody(tag.name, tag.at, decode[tag.name], openings, cmd.Label())
			if !ok {
				return false
			}
			f.Body = body
		}
		cmd.Fields = append(cmd.Fields, f)
	}
}
