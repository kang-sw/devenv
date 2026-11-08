package wsxmlcmd

import (
	"slices"
	"strings"
	"testing"
	"time"
)

// The whiteboard's commands_test.go pins the language rules through the
// whiteboard grammar; these cases pin what a caller's grammar controls.
var testGrammar = Grammar{
	Prefix: "tr:",
	Commands: map[string]Spec{
		"note": {Kind: Leaf, Attrs: []string{"id"}},
		"set":  {Kind: Container, Attrs: []string{"id", "flag"}, Fields: []string{"name", "tag"}},
		"drop": {Kind: Empty, Attrs: []string{"id"}},
	},
}

func parse(g Grammar, s string) ([]Command, []string) {
	p := &Problems{}
	return Parse(p, g, s), p.List
}

func TestCallerPrefixAndTableDecideTheTags(t *testing.T) {
	cmds, problems := parse(testGrammar, "<tr:note id=\"1\">a <b>raw</b> & body</tr:note>\n<tr:set id=\"2\"><tr:name>x</tr:name><tr:tag>y</tr:tag><tr:tag/></tr:set><tr:drop id=\"3\"/>")
	if len(problems) > 0 {
		t.Fatalf("problems: %q", problems)
	}
	if len(cmds) != 3 || cmds[0].Tag != "tr:note" || cmds[0].Body != "a <b>raw</b> & body" || cmds[2].Tag != "tr:drop" {
		t.Fatalf("commands = %+v", cmds)
	}
	if got := cmds[1].FieldsOf("tr:tag"); !slices.Equal(got, []string{"y", ""}) || cmds[1].Line != 2 || cmds[1].Pos != 2 {
		t.Fatalf("set = %+v", cmds[1])
	}

	// Another caller's prefix, and a bare name, are unknown commands; the
	// list names this grammar's commands with its prefix, sorted.
	_, problems = parse(testGrammar, `<wb:note id="1">x</wb:note><note id="1">x</note>`)
	want := []string{
		"command 1 <wb:note> (line 1): unknown command (commands: tr:drop, tr:note, tr:set)",
		"command 2 <note> (line 1): unknown command (commands: tr:drop, tr:note, tr:set)",
	}
	if !slices.Equal(problems, want) {
		t.Fatalf("problems = %q, want %q", problems, want)
	}

	// Fields are prefixed too, and listed in the table's order.
	_, problems = parse(testGrammar, `<tr:set id="1" mode="x"><name>x</name></tr:set>`)
	want = []string{
		"command 1 <tr:set> (line 1): unknown attribute mode (known: id, flag)",
		"command 1 <tr:set> (line 1): unknown field <name> (fields: tr:name, tr:tag)",
	}
	if !slices.Equal(problems, want) {
		t.Fatalf("problems = %q, want %q", problems, want)
	}
}

func TestEmptyPrefixTakesBareNames(t *testing.T) {
	g := Grammar{Commands: map[string]Spec{"go": {Kind: Empty}}}
	cmds, problems := parse(g, `<go/><go></go>`)
	if len(problems) > 0 || len(cmds) != 2 || cmds[1].Tag != "go" {
		t.Fatalf("commands = %+v, problems = %q", cmds, problems)
	}
}

func TestValueChecks(t *testing.T) {
	cmds, problems := parse(testGrammar, `<tr:set id="7" flag="maybe"></tr:set><tr:drop id="0"/><tr:drop/><tr:note id=" ">x</tr:note>`)
	if len(problems) > 0 {
		t.Fatalf("problems: %q", problems)
	}
	p := &Problems{}
	if n, ok := IntAttr(p, cmds[0], "id", true); !ok || n != 7 {
		t.Fatalf("id = %d, %v", n, ok)
	}
	if _, ok := BoolAttr(p, cmds[0], "flag"); ok {
		t.Fatal("flag=maybe accepted")
	}
	if v, ok := BoolAttr(p, cmds[1], "flag"); !ok || v {
		t.Fatal("absent flag is not false")
	}
	IntAttr(p, cmds[1], "id", true)
	IntAttr(p, cmds[2], "id", true)
	if _, ok := IntAttr(p, cmds[2], "missing", false); ok {
		t.Fatal("absent optional int reported present")
	}
	RequiredAttr(p, cmds[3], "id")
	if _, ok := EnumAttr(p, cmds[0], "flag", []string{"on", "off"}); ok {
		t.Fatal("enum accepted maybe")
	}
	if _, ok := cmds[0].Single(p, "tr:name"); ok {
		t.Fatal("absent field reported present")
	}
	want := []string{
		`command 1 <tr:set> (line 1) flag: must be "true" or "false"; got "maybe"`,
		`command 2 <tr:drop> (line 1) id: must be a positive whole number; got "0"`,
		`command 3 <tr:drop> (line 1) id: required`,
		`command 4 <tr:note> (line 1) id: must be non-empty`,
		`command 1 <tr:set> (line 1) flag: must be one of on, off; got "maybe"`,
	}
	if !slices.Equal(p.List, want) {
		t.Fatalf("problems =\n%s\nwant\n%s", strings.Join(p.List, "\n"), strings.Join(want, "\n"))
	}
}

// parseWithin fails the test rather than hanging it when Parse does not
// return: the loop this guards against never ends and grows the problem list.
func parseWithin(t *testing.T, s string) []string {
	t.Helper()
	type result struct{ problems []string }
	done := make(chan result, 1)
	go func() {
		p := &Problems{}
		Parse(p, testGrammar, s)
		done <- result{p.List}
	}()
	select {
	case r := <-done:
		return r.problems
	case <-time.After(2 * time.Second):
		t.Fatalf("Parse(%q) did not return", s)
		return nil
	}
}

// A closing tag cut short (no '>') is reported once, with its line, and
// reading ends, whether the cut tag closes a container or sits at top level.
func TestTruncatedClosingTagEndsReading(t *testing.T) {
	for name, in := range map[string]string{
		"container closing without >":     "<tr:set id=\"1\">\n<tr:name>x</tr:name>\n</tr:set",
		"container closing cut at prefix": "<tr:set id=\"1\">\n<tr:name>x</tr:name>\n</tr:",
		"top-level closing without >":     "<tr:drop id=\"1\"/></tr:set",
	} {
		problems := parseWithin(t, in)
		var cut []string
		for _, p := range problems {
			if strings.Contains(p, "a closing tag is not closed with '>'") {
				cut = append(cut, p)
			}
		}
		if len(cut) != 1 || !strings.HasPrefix(cut[0], "line ") {
			t.Errorf("%s: problems = %q, want the unclosed closing tag reported once with its line", name, problems)
		}
	}
}

func TestDecode(t *testing.T) {
	for name, tc := range map[string]struct{ in, want string }{
		"lt":                     {"&lt;", "<"},
		"gt":                     {"&gt;", ">"},
		"amp":                    {"&amp;", "&"},
		"quot":                   {"&quot;", `"`},
		"apos":                   {"&apos;", "'"},
		"decimal":                {"&#60;&#233;", "<é"},
		"hex":                    {"&#x3c;&#X3E;&#x1F600;", "<>😀"},
		"one pass":               {"&amp;lt;", "&lt;"},
		"in text":                {"a &lt;key&gt; b", "a <key> b"},
		"unknown entity kept":    {"&nbsp;&foo;&LT;", "&nbsp;&foo;&LT;"},
		"bare & kept":            {"a && b & c", "a && b & c"},
		"no semicolon kept":      {"&lt &amp", "&lt &amp"},
		"zero kept":              {"&#0;&#x0;", "&#0;&#x0;"},
		"surrogate kept":         {"&#xD800;&#57343;", "&#xD800;&#57343;"},
		"out of range kept":      {"&#x110000;&#99999999999999999999;", "&#x110000;&#99999999999999999999;"},
		"malformed numeric kept": {"&#;&#x;&#-1;&#+60;&#xg;&# 60;", "&#;&#x;&#-1;&#+60;&#xg;&# 60;"},
		"kept then decoded":      {"&foo &lt;", "&foo <"},
		"empty":                  {"", ""},
	} {
		if got := Decode(tc.in); got != tc.want {
			t.Errorf("%s: Decode(%q) = %q, want %q", name, tc.in, got, tc.want)
		}
	}
}

// Attributes are always decoded; a body only where the grammar marks it,
// and never inside CDATA.
func TestGrammarDecidesWhichBodiesDecode(t *testing.T) {
	g := Grammar{
		Prefix: "tr:",
		Commands: map[string]Spec{
			"note": {Kind: Leaf, Attrs: []string{"id"}, DecodeBody: true},
			"raw":  {Kind: Leaf, Attrs: []string{"id"}},
			"set":  {Kind: Container, Fields: []string{"name", "tag"}, DecodeFields: []string{"name"}},
		},
	}
	cmds, problems := parse(g, `<tr:note id="&lt;1&gt;">a &lt;b&gt; <![CDATA[&lt;c&gt;]]> &amp;d</tr:note>`+
		`<tr:raw id="x &amp; y">a &lt;b&gt;</tr:raw>`+
		`<tr:set><tr:name>&lt;n&gt;</tr:name><tr:tag>&lt;t&gt;</tr:tag></tr:set>`+
		"<tr:note>\n&#10;x&#10;\n</tr:note>")
	if len(problems) > 0 {
		t.Fatalf("problems: %q", problems)
	}
	if id, _ := cmds[0].Attr("id"); id != "<1>" || cmds[0].Body != "a <b> &lt;c&gt; &d" {
		t.Errorf("decoded leaf = %+v", cmds[0])
	}
	if id, _ := cmds[1].Attr("id"); id != "x & y" || cmds[1].Body != "a &lt;b&gt;" {
		t.Errorf("verbatim leaf = %+v", cmds[1])
	}
	if name, tag := cmds[2].FieldsOf("tr:name"), cmds[2].FieldsOf("tr:tag"); name[0] != "<n>" || tag[0] != "&lt;t&gt;" {
		t.Errorf("fields = %+v", cmds[2].Fields)
	}
	// The body's own newlines are stripped before decoding, so a decoded
	// newline at either end stays.
	if cmds[3].Body != "\nx\n" {
		t.Errorf("decoded newlines = %q", cmds[3].Body)
	}
}

// groupGrammar declares a group: tr:in passes name to the scoped tr:note and
// tr:set; tr:drop stays at the root.
var groupGrammar = Grammar{
	Prefix: "tr:",
	Group:  &Group{Name: "in", Attr: "name", Hint: `write <tr:in name="..."><tr:note>...</tr:note></tr:in>`},
	Commands: map[string]Spec{
		"note": {Kind: Leaf, Attrs: []string{"id"}, Scoped: true},
		"set":  {Kind: Container, Attrs: []string{"flag"}, Fields: []string{"name"}, Scoped: true},
		"drop": {Kind: Empty, Attrs: []string{"id"}},
	},
}

// A group passes its attribute to the commands inside it and takes no
// position; a batch may hold several groups, root commands between them.
func TestGroupPassesItsAttributeToItsCommands(t *testing.T) {
	cmds, problems := parse(groupGrammar, "<tr:in name=\"a\"><tr:note id=\"1\">x</tr:note><tr:set><tr:name>n</tr:name></tr:set></tr:in>\n"+
		"<tr:drop id=\"2\"/><tr:in name='b &amp; c'>\n<tr:note>y</tr:note>\n</tr:in>")
	if len(problems) > 0 {
		t.Fatalf("problems: %q", problems)
	}
	if len(cmds) != 4 {
		t.Fatalf("commands = %+v", cmds)
	}
	for i, want := range []struct {
		tag, scope string
		scoped     bool
	}{{"tr:note", "a", true}, {"tr:set", "a", true}, {"tr:drop", "", false}, {"tr:note", "b & c", true}} {
		got, ok := cmds[i].Scoped()
		if cmds[i].Pos != i+1 || cmds[i].Tag != want.tag || got != want.scope || ok != want.scoped {
			t.Errorf("command %d = %+v, scope %q %v", i, cmds[i], got, ok)
		}
	}
	if cmds[0].Scope != cmds[1].Scope || cmds[3].Scope.Line != 2 || cmds[3].Line != 3 || cmds[0].Body != "x" {
		t.Fatalf("scopes = %+v / %+v", cmds[0].Scope, cmds[3].Scope)
	}
	if got := cmds[3].Scope.At(); got != "<tr:in> (line 2) name" {
		t.Fatalf("scope label = %q", got)
	}
}

func TestGroupRules(t *testing.T) {
	for name, tc := range map[string]struct {
		in   string
		want []string
	}{
		"scoped command outside its group": {`<tr:note id="1">x</tr:note>`, []string{
			`command 1 <tr:note> (line 1): goes inside a <tr:in name="..."> group`,
		}},
		"root-only command inside a group": {`<tr:in name="a"><tr:note>x</tr:note><tr:drop id="1"/></tr:in>`, []string{
			"command 2 <tr:drop> (line 1): goes at the root, outside <tr:in> (line 1)",
		}},
		"nested group": {"<tr:in name=\"a\">\n<tr:in name=\"b\"><tr:note>x</tr:note></tr:in><tr:note>y</tr:note></tr:in>", []string{
			"line 2: <tr:in> inside <tr:in> (line 1); groups do not nest",
		}},
		"group attribute on an inner command": {`<tr:in name="a"><tr:note name="b">x</tr:note></tr:in>`, []string{
			"command 1 <tr:note> (line 1): unknown attribute name (known: id)",
		}},
		"text between commands": {"<tr:in name=\"a\"><tr:note>x</tr:note>\nloose\n<tr:note>y</tr:note></tr:in>", []string{
			`line 2: text inside <tr:in> (line 1) between its commands: "loose"`,
		}},
		"group attribute missing, blank, unknown": {`<tr:in><tr:note>x</tr:note></tr:in><tr:in name=" " mode="x"><tr:note>y</tr:note></tr:in>`, []string{
			"<tr:in> (line 1) name: required",
			"<tr:in> (line 1): unknown attribute mode (known: name)",
			"<tr:in> (line 1) name: must be non-empty",
		}},
		"empty groups": {`<tr:in name="a"/><tr:in name="b"> </tr:in>`, []string{
			`<tr:in> (line 1): holds no commands; write <tr:in name="..."><tr:note>...</tr:note></tr:in>`,
			`<tr:in> (line 1): holds no commands; write <tr:in name="..."><tr:note>...</tr:note></tr:in>`,
		}},
		// A body where the group takes commands is one problem, with the
		// hint, and reading resumes after the group.
		"body text in a group": {"<tr:in name=\"a\">\n## Body <b>bold</b> & <br>\n</tr:in><tr:drop id=\"x\" mode=\"y\"/>", []string{
			`<tr:in> (line 1): text before its first command: "## Body <b>bold</b> & <br>"; write <tr:in name="..."><tr:note>...</tr:note></tr:in>`,
			"command 1 <tr:drop> (line 3): unknown attribute mode (known: id)",
		}},
		"unprefixed tag first in a group": {`<tr:in name="a"><div>x</div></tr:in>`, []string{
			`<tr:in> (line 1): text before its first command: "<div>x</div>"; write <tr:in name="..."><tr:note>...</tr:note></tr:in>`,
		}},
		"unterminated group": {"<tr:in name=\"a\">\n<tr:note>x</tr:note>\n", []string{
			"line 3: <tr:in> from line 1 has no closing </tr:in>",
		}},
	} {
		_, problems := parse(groupGrammar, tc.in)
		if !slices.Equal(problems, tc.want) {
			t.Errorf("%s: problems =\n%s\nwant\n%s", name, strings.Join(problems, "\n"), strings.Join(tc.want, "\n"))
		}
	}
	// The unknown-command list names the group with the commands.
	_, problems := parse(groupGrammar, `<tr:nope/>`)
	if want := []string{"command 1 <tr:nope> (line 1): unknown command (commands: tr:drop, tr:in, tr:note, tr:set)"}; !slices.Equal(problems, want) {
		t.Fatalf("problems = %q", problems)
	}
}
