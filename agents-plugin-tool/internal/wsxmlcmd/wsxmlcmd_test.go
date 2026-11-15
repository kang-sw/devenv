package wsxmlcmd

import (
	"fmt"
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

func TestLiteralCodeAndWrappedFieldsAreGrammarOptIns(t *testing.T) {
	input := "<tr:note>`</tr:note>` tail</tr:note>"
	cmds, problems := parse(testGrammar, input)
	if len(problems) == 0 || len(cmds) != 1 || cmds[0].Body != "`" {
		t.Fatalf("default grammar changed delimiter behavior: %+v, %q", cmds, problems)
	}
	g := Grammar{Prefix: "x:", LiteralCode: true, RefuseFieldOpenings: true, Commands: map[string]Spec{
		"payload": {Kind: Leaf, Wrapper: "change", Fields: []string{"old", "new"}},
		"text":    {Kind: Leaf},
	}}
	for _, body := range []string{
		"`</x:text>`", "``a ` </x:text>``", "unmatched ` text",
		"```xml\n</x:text> `\n```\n", "~~~xml\n</x:text> `\n~~~\n",
		"   ````xml\n</x:text>\n```\n   `````\n",
		"```xml\ninterior ``` </x:text> is still code\n```",
		"~~~xml\n</x:text> `\n~~~",
	} {
		cmds, problems := parse(g, "<x:text>"+body+"</x:text>")
		if len(problems) != 0 || len(cmds) != 1 || cmds[0].Body != stripTrailingNewline(body) {
			t.Fatalf("literal %q: %+v, %q", body, cmds, problems)
		}
	}
	cmds, problems = parse(g, "<x:payload>\n<x:change><x:old>`<x:new></x:old>`</x:old><x:new/></x:change>\n<x:change><x:old><![CDATA[<x:new></x:old>]]></x:old><x:new> &amp; </x:new></x:change>\n</x:payload>")
	if len(problems) != 0 || len(cmds) != 1 || len(cmds[0].Children) != 2 || strings.TrimSpace(cmds[0].Body) != "" {
		t.Fatalf("wrapped fields: %+v, %q", cmds, problems)
	}
	children := cmds[0].Children
	if children[0].FieldsOf("x:old")[0] != "`<x:new></x:old>`" || children[1].FieldsOf("x:old")[0] != "<x:new></x:old>" || children[1].FieldsOf("x:new")[0] != " &amp; " {
		t.Fatalf("operand bytes changed: %+v", children)
	}
	// Undeclared wrapper spellings and code/CDATA examples remain raw payload.
	body := "`<x:change/>` <![CDATA[<x:change/>]]> <x:changex/>"
	cmds, problems = parse(g, "<x:payload>"+body+"</x:payload>")
	if len(problems) != 0 || len(cmds[0].Children) != 0 || cmds[0].Body != "`<x:change/>` <x:change/> <x:changex/>" {
		t.Fatalf("literal wrapper: %+v, %q", cmds, problems)
	}
	cmds, problems = parse(g, "<x:payload>stray<x:change/></x:payload>")
	if len(problems) != 0 || cmds[0].Body != "stray" || len(cmds[0].Children) != 1 {
		t.Fatalf("mixed content must remain visible to validation: %+v, %q", cmds, problems)
	}
	// Long unmatched runs must remain linear rather than rescanning every
	// suffix of the same delimiter run.
	long := strings.Repeat("`", 128*1024)
	cmds, problems = parse(g, "<x:text>"+long+"</x:text>")
	if len(problems) != 0 || cmds[0].Body != long {
		t.Fatal("unmatched run changed payload")
	}
	_, problems = parse(g, "<x:payload><x:change><x:old>missed<x:new>closing</x:old></x:change></x:payload>")
	if len(problems) == 0 || !strings.Contains(problems[0], "body holds an opening <x:new>") {
		t.Fatalf("literal opt-in relaxed real sibling validation: %q", problems)
	}
}

func TestLiteralCodeDoesNotPairAcrossStructuralBodies(t *testing.T) {
	g := Grammar{Prefix: "x:", LiteralCode: true, RefuseFieldOpenings: true, Commands: map[string]Spec{
		"text": {Kind: Leaf}, "other": {Kind: Leaf},
		"set": {Kind: Container, Fields: []string{"old", "new"}},
	}}
	for _, tc := range []struct{ input, first, second string }{
		{"<x:text>a `</x:text><x:text>b `</x:text>", "a `", "b `"},
		{"<x:text>a ``</x:text><x:other>b ``</x:other>", "a ``", "b ``"},
		{"<x:text>~~~\na</x:text><x:other>\n~~~\nb</x:other>", "~~~\na", "~~~\nb"},
	} {
		cmds, problems := parse(g, tc.input)
		if len(problems) != 0 || len(cmds) != 2 || cmds[0].Body != tc.first || cmds[1].Body != tc.second {
			t.Fatalf("cross-command code pairing: %+v, %q", cmds, problems)
		}
	}
	cmds, problems := parse(g, "<x:set><x:old>a `</x:old><x:new>b `</x:new></x:set>")
	if len(problems) != 0 || len(cmds) != 1 || len(cmds[0].Fields) != 2 || cmds[0].FieldsOf("x:old")[0] != "a `" || cmds[0].FieldsOf("x:new")[0] != "b `" {
		t.Fatalf("cross-operand code pairing: %+v, %q", cmds, problems)
	}
	// Complete examples can contain repeated enclosing elements in one span.
	body := "`<x:text>one</x:text><x:text>two</x:text>`"
	cmds, problems = parse(g, "<x:text>"+body+"</x:text>")
	if len(problems) != 0 || len(cmds) != 1 || cmds[0].Body != body {
		t.Fatalf("balanced literal example: %+v, %q", cmds, problems)
	}
}

func literalLookaheadFixture(kind string, size int) string {
	switch kind {
	case "tildes":
		return strings.Repeat("~a", size/2)
	case "unmatched-fences":
		return "```\n" + strings.Repeat("```x\n", (size-4)/5)
	default:
		return "Status `ready` with **next** action."
	}
}

func TestLiteralCodeLookaheadLargeLegalInputs(t *testing.T) {
	g := Grammar{Prefix: "x:", LiteralCode: true, Commands: map[string]Spec{"text": {Kind: Leaf}}}
	for _, kind := range []string{"tildes", "unmatched-fences"} {
		body := literalLookaheadFixture(kind, 200000)
		start := time.Now()
		cmds, problems := parse(g, "<x:text>"+body+"</x:text>")
		elapsed := time.Since(start)
		if len(problems) != 0 || len(cmds) != 1 || cmds[0].Body != stripTrailingNewline(body) {
			t.Fatalf("large legal %s changed bytes: %q", kind, problems)
		}
		t.Logf("%s: %d bytes in %s", kind, len(body), elapsed)
	}
}

func BenchmarkLiteralCodeLookahead(b *testing.B) {
	g := Grammar{Prefix: "x:", LiteralCode: true, Commands: map[string]Spec{"text": {Kind: Leaf}}}
	for _, kind := range []string{"typical", "tildes", "unmatched-fences"} {
		for _, size := range []int{100000, 200000} {
			if kind == "typical" && size != 100000 {
				continue
			}
			body := literalLookaheadFixture(kind, size)
			input := "<x:text>" + body + "</x:text>"
			b.Run(fmt.Sprintf("%s/%d", kind, len(body)), func(b *testing.B) {
				b.SetBytes(int64(len(body)))
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					cmds, problems := parse(g, input)
					if len(problems) != 0 || len(cmds) != 1 {
						b.Fatal(problems)
					}
				}
			})
		}
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

func TestParseOrderedRejectedGroupContainerFields(t *testing.T) {
	prefix := `<tr:drop id="1"/>`
	rejected := `<tr:in name="outer">junk<tr:set><tr:name></tr:set></tr:in><tr:in name="literal"><![CDATA[</tr:name></tr:set><tr:in>]]></tr:name></tr:set></tr:in>`
	suffix := `<tr:drop id="2"/>`
	p := &Problems{}
	cmds, complete := ParseOrdered(p, groupGrammar, prefix+rejected+suffix)
	if !complete || len(p.List) != 1 || len(cmds) != 3 {
		t.Fatalf("field payload escaped rejected group: complete=%v commands=%+v problems=%v", complete, cmds, p.List)
	}
	if cmds[0].Source != prefix || cmds[1].Source != rejected || cmds[2].Source != suffix || len(cmds[1].Scope.Problems) != 1 {
		t.Fatalf("lost source boundaries/diagnostics: %+v", cmds)
	}
}

func TestParseOrderedRejectedGroupTruncation(t *testing.T) {
	for _, suffix := range []string{
		`<!--`,
		`<![CDATA[`,
		`<div title='`,
		`<tr:set><!--`,
		`<tr:set><![CDATA[`,
		`<tr:set><tr:name attr="`,
		`<tr:set><tr:name><![CDATA[`,
		`<tr:set><tr:name></tr:set><tr:in name="literal">`,
	} {
		t.Run(suffix, func(t *testing.T) {
			done := make(chan bool, 1)
			go func() {
				p := &Problems{}
				_, complete := ParseOrdered(p, groupGrammar, `<tr:drop id="1"/><tr:in name="outer">junk`+suffix)
				done <- !complete && len(p.List) > 0
			}()
			select {
			case refused := <-done:
				if !refused {
					t.Fatal("truncated group was not refused")
				}
			case <-time.After(2 * time.Second):
				t.Fatal("ParseOrdered did not return")
			}
		})
	}
}

// extraGrammar is groupGrammar with an optional extra group attribute.
var extraGrammar = Grammar{
	Prefix:   "tr:",
	Group:    &Group{Name: "in", Attr: "name", Extras: []string{"label"}, Hint: groupGrammar.Group.Hint},
	Commands: groupGrammar.Commands,
}

// extrasGrammar is groupGrammar with two optional extra group attributes.
var extrasGrammar = Grammar{
	Prefix:   "tr:",
	Group:    &Group{Name: "in", Attr: "name", Extras: []string{"label", "tags"}, Hint: groupGrammar.Group.Hint},
	Commands: groupGrammar.Commands,
}

// A grammar may declare more than one extra group attribute: a group that
// carries any of them comes out as a Command and may hold none, each
// attribute readable by name; one that carries none still needs a command,
// and an attribute outside the list is still unknown.
func TestGroupExtraAttributes(t *testing.T) {
	cmds, problems := parse(extrasGrammar, `<tr:in name="a" tags="x y"/><tr:in name="b" label="B" tags=""><tr:note>z</tr:note></tr:in>`)
	if len(problems) > 0 {
		t.Fatalf("problems: %q", problems)
	}
	if len(cmds) != 3 || !cmds[0].IsGroup() || !cmds[1].IsGroup() || cmds[2].IsGroup() {
		t.Fatalf("commands = %+v", cmds)
	}
	if v, ok := cmds[0].Scope.Extra("tags"); !ok || v != "x y" {
		t.Fatalf("tags = %q, %v", v, ok)
	}
	if _, ok := cmds[0].Scope.Extra("label"); ok {
		t.Fatal("a group without label reads one")
	}
	label, lok := cmds[1].Scope.Extra("label")
	tags, tok := cmds[1].Scope.Extra("tags")
	if label != "B" || !lok || tags != "" || !tok || cmds[2].Scope != cmds[1].Scope {
		t.Fatalf("second group: %+v", cmds[1].Scope)
	}
	_, problems = parse(extrasGrammar, `<tr:in name="a"/>`)
	if want := []string{`<tr:in> (line 1): holds no commands; write <tr:in name="..."><tr:note>...</tr:note></tr:in>`}; !slices.Equal(problems, want) {
		t.Fatalf("empty group without extras: %q", problems)
	}
	_, problems = parse(extrasGrammar, `<tr:in name="a" kind="k" tags="t"/>`)
	if want := []string{"<tr:in> (line 1): unknown attribute kind (known: name, label, tags)"}; !slices.Equal(problems, want) {
		t.Fatalf("unknown group attribute: %q", problems)
	}
}

// A group carrying the grammar's extra attribute comes out as a Command of
// its own ahead of its commands and may hold none; one without it still
// needs a command. The extra attribute is opt-in: groupGrammar refuses it.
func TestGroupExtraAttribute(t *testing.T) {
	extraOf := func(s *Scope) string { v, _ := s.Extra("label"); return v }
	cmds, problems := parse(extraGrammar, `<tr:in name="a" label="A &amp; B"><tr:note>x</tr:note></tr:in><tr:in name="b" label="B"/><tr:in name="c" label="C"> </tr:in><tr:in name="d"><tr:note>y</tr:note></tr:in>`)
	if len(problems) > 0 {
		t.Fatalf("problems: %q", problems)
	}
	if len(cmds) != 5 {
		t.Fatalf("commands = %+v", cmds)
	}
	for i, want := range []struct {
		tag, scope, extra string
		group, hasExtra   bool
		commands          int
	}{
		{"tr:in", "a", "A & B", true, true, 1},
		{"tr:note", "a", "A & B", false, true, 1},
		{"tr:in", "b", "B", true, true, 0},
		{"tr:in", "c", "C", true, true, 0},
		{"tr:note", "d", "", false, false, 1},
	} {
		c := cmds[i]
		if c.Tag != want.tag || c.IsGroup() != want.group || c.Scope.Value != want.scope || extraOf(c.Scope) != want.extra || c.Scope.HasExtra() != want.hasExtra || c.Scope.Commands != want.commands {
			t.Errorf("command %d = %+v scope %+v", i, c, c.Scope)
		}
	}
	if cmds[0].Scope != cmds[1].Scope || cmds[0].Pos != 0 || cmds[1].Pos != 1 || cmds[4].Pos != 2 {
		t.Fatalf("positions/scopes = %+v / %+v", cmds[0], cmds[1])
	}
	if got := cmds[2].Label(); got != "<tr:in> (line 1)" {
		t.Fatalf("group label = %q", got)
	}
	_, problems = parse(extraGrammar, `<tr:in name="a"/>`)
	if want := []string{`<tr:in> (line 1): holds no commands; write <tr:in name="..."><tr:note>...</tr:note></tr:in>`}; !slices.Equal(problems, want) {
		t.Fatalf("empty unlabelled group: %q", problems)
	}
	cmds, problems = parse(groupGrammar, `<tr:in name="a" label="A"><tr:note>x</tr:note></tr:in>`)
	if want := []string{"<tr:in> (line 1): unknown attribute label (known: name)"}; !slices.Equal(problems, want) || len(cmds) != 1 || cmds[0].IsGroup() {
		t.Fatalf("grammar without Extra: problems %q, commands %+v", problems, cmds)
	}
}
