package wsxmlcmd

import (
	"fmt"
	"reflect"
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

// A field keeps its as-written spelling beside the decoded body, so an
// exact-match operand can detect that an entity may have meant itself.
func TestFieldLiteralKeepsEntitiesAsWritten(t *testing.T) {
	cmds, problems := parse(testGrammar, "<tr:set id=\"1\"><tr:name>\n&lt;b&gt; <tr:raw>&amp;</tr:raw> <![CDATA[&gt;]]>\n</tr:name><tr:tag>plain</tr:tag></tr:set>")
	if len(problems) > 0 || len(cmds) != 1 {
		t.Fatalf("commands = %+v, problems %q", cmds, problems)
	}
	name, tag := cmds[0].Fields[0], cmds[0].Fields[1]
	if name.Body != "<b> &amp; &gt;" || name.Literal != "&lt;b&gt; &amp; &gt;" || tag.Body != "plain" || tag.Literal != "plain" {
		t.Fatalf("fields = %+v", cmds[0].Fields)
	}
	if !cmds[0].LiteralAmbiguous("tr:name", "x <b> &lt;b&gt; &amp; &gt;") || cmds[0].LiteralAmbiguous("tr:name", "<b> &amp; &gt;") || cmds[0].LiteralAmbiguous("tr:tag", "plain") {
		t.Fatal("LiteralAmbiguous misjudged")
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
		input := "<x:text>" + body + "</x:text>"
		cmds, problems := parse(g, input)
		if strings.Contains(body, "</x:text>") {
			// Owner-b: a raw own-close candidate is not a literal.
			g0 := g
			g0.LiteralCode = false
			baseline, structuralProblems := parse(g0, input)
			if len(problems) == 0 || len(problems) != len(structuralProblems) || !reflect.DeepEqual(cmds, baseline) {
				t.Fatalf("structural %q: %+v / %+v, %q / %q", body, cmds, baseline, problems, structuralProblems)
			}
		} else if len(problems) != 0 || len(cmds) != 1 || cmds[0].Body != stripTrailingNewline(body) {
			t.Fatalf("literal %q: %+v, %q", body, cmds, problems)
		}
	}
	cmds, problems = parse(g, "<x:payload>\n<x:change><x:old>`<![CDATA[<x:new></x:old>]]>`</x:old><x:new/></x:change>\n<x:change><x:old><![CDATA[<x:new></x:old>]]></x:old><x:new> &amp; </x:new></x:change>\n</x:payload>")
	if len(problems) != 0 || len(cmds) != 1 || len(cmds[0].Children) != 2 || strings.TrimSpace(cmds[0].Body) != "" {
		t.Fatalf("wrapped fields: %+v, %q", cmds, problems)
	}
	children := cmds[0].Children
	if children[0].FieldsOf("x:old")[0] != "`<x:new></x:old>`" || children[1].FieldsOf("x:old")[0] != "<x:new></x:old>" || children[1].FieldsOf("x:new")[0] != " & " {
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

func TestLiteralCodeCannotEndInsideWrappedOperand(t *testing.T) {
	g := Grammar{Prefix: "x:", LiteralCode: true, Commands: map[string]Spec{
		"payload": {Kind: Leaf, Wrapper: "change", Fields: []string{"old", "new"}},
		"text":    {Kind: Leaf},
	}}
	for _, body := range []string{
		"` <x:change><x:old>alpha ` x</x:old><x:new>z</x:new></x:change>",
		"` <x:change><x:old>alpha</x:old><x:new>z ` x</x:new></x:change>",
		"` <x:change><x:old>alpha</x:old> ` <x:new>z</x:new></x:change>",
		"` <x:change> ` <x:old>alpha</x:old><x:new>z</x:new></x:change>",
		"` <x:change><x:old>alpha</x:old><x:new>z</x:new> ` </x:change>",
		"` <x:change> ` </x:change>",
		"` <x:change> ` `<x:old>quoted</x:old>` <x:new>z</x:new></x:change>",
		"```\n<x:change><x:old>alpha</x:old>\n```\n<x:new>z</x:new></x:change>",
		"```\n<x:change><x:old>alpha\n```\nx</x:old><x:new>z</x:new></x:change>",
	} {
		input := "<x:payload>" + body + "</x:payload><x:text>keep</x:text>"
		for _, ordered := range []bool{false, true} {
			p := &Problems{}
			var cmds []Command
			complete := true
			if ordered {
				cmds, complete = ParseOrdered(p, g, input)
			} else {
				cmds = Parse(p, g, input)
			}
			if !complete || len(p.List) == 0 || len(cmds) != 2 || len(cmds[0].Children) != 1 || cmds[1].Body != "keep" || !strings.Contains(strings.Join(p.List, " "), "ends inside") {
				t.Fatalf("ordered=%v input=%q commands=%+v complete=%v problems=%v", ordered, input, cmds, complete, p.List)
			}
			if ordered && (len(cmds[0].Problems) == 0 || len(cmds[1].Problems) != 0) {
				t.Fatalf("refusal was not command-local: %+v", cmds)
			}
		}
	}
	// Whole quoted wrappers, quoted openers, and CDATA examples stay inert.
	for _, body := range []string{
		"`<x:change><x:old>a</x:old><x:new>b</x:new></x:change>`",
		"`<x:change>`",
		"`<x:change>` <![CDATA[<x:old>a</x:old></x:change>]]>",
		"`<x:change>` `<x:old>a</x:old>`",
		"`<x:change>`\n~~~\n<x:old>a</x:old></x:change>\n~~~",
		"`<x:change><![CDATA[` <x:old>a</x:old>]]>",
		"`<x:change>` `<![CDATA[` <x:old>a</x:old>]]>",
		"`<![CDATA[<x:change><x:old>a]]>`",
		"`<x:change><x:old><![CDATA[</x:old>]]>a</x:old></x:change>`",
	} {
		cmds, p := parse(g, "<x:payload>"+body+"</x:payload>")
		if len(p) != 0 || len(cmds) != 1 || len(cmds[0].Children) != 0 {
			t.Fatalf("legal literal %q: %+v %v", body, cmds, p)
		}
	}
	cmds, p := parse(g, "<x:payload>`<x:change>`</x:payload><x:text><x:old>a</x:old></x:change></x:text>")
	if len(p) != 0 || len(cmds) != 2 || len(cmds[0].Children) != 0 {
		t.Fatalf("continuation must not cross a command close: %+v %v", cmds, p)
	}
}

func TestLiteralCodeQuotedWrapperAllowsIndependentTail(t *testing.T) {
	g := Grammar{Prefix: "x:", LiteralCode: true, Commands: map[string]Spec{
		"payload": {Kind: Leaf, Wrapper: "change", Fields: []string{"old", "new"}},
		"text":    {Kind: Leaf},
	}}
	for _, tc := range []struct {
		body     string
		children int
	}{
		{"`<x:change>` then <x:change><x:old>a</x:old><x:new>b</x:new></x:change>", 1},
		{"`<x:change>` then `x` <x:old>raw</x:old>", 0},
		{"`<x:change>` then <x:new>raw", 0},
	} {
		for _, ordered := range []bool{false, true} {
			input := "<x:payload>" + tc.body + "</x:payload><x:text>keep</x:text>"
			p := &Problems{}
			var cmds []Command
			complete := true
			if ordered {
				cmds, complete = ParseOrdered(p, g, input)
			} else {
				cmds = Parse(p, g, input)
			}
			if !complete || len(p.List) != 0 || len(cmds) != 2 || len(cmds[0].Children) != tc.children || cmds[1].Body != "keep" {
				t.Fatalf("ordered=%v body=%q: commands=%+v complete=%v problems=%v", ordered, tc.body, cmds, complete, p.List)
			}
			want := tc.body
			if tc.children != 0 {
				want = "`<x:change>` then "
			}
			if cmds[0].Body != want {
				t.Fatalf("ordered=%v body=%q: got body %q, want %q", ordered, tc.body, cmds[0].Body, want)
			}
		}
	}
}

func TestLiteralCodeQuotedWrapperDoesNotBorrowSiblingFields(t *testing.T) {
	g := Grammar{Prefix: "x:", LiteralCode: true, Commands: map[string]Spec{
		"payload": {Kind: Leaf, Wrapper: "change", Fields: []string{"old", "new"}},
		"text":    {Kind: Leaf},
	}}
	body := "`<x:change>`<x:old>missing"
	sibling := "keep</x:old><x:new>b</x:new></x:change>"
	input := "<x:payload>" + body + "</x:payload><x:text>" + sibling + "</x:text>"
	for _, ordered := range []bool{false, true} {
		p := &Problems{}
		var cmds []Command
		complete := true
		if ordered {
			cmds, complete = ParseOrdered(p, g, input)
		} else {
			cmds = Parse(p, g, input)
		}
		if !complete || len(p.List) != 0 || len(cmds) != 2 || len(cmds[0].Children) != 0 || cmds[0].Body != body || cmds[1].Body != sibling {
			t.Fatalf("ordered=%v: commands=%+v complete=%v problems=%v", ordered, cmds, complete, p.List)
		}
	}
}

func TestLiteralCodeQuotedWrapperTailRawFields(t *testing.T) {
	g := Grammar{Prefix: "x:", LiteralCode: true, Commands: map[string]Spec{
		"payload": {Kind: Leaf, Wrapper: "change", Fields: []string{"old", "new"}},
		"text":    {Kind: Leaf},
	}}
	for _, tc := range []struct {
		name, body, wantBody, old, new string
		children                       int
	}{
		{
			name:     "opener inside raw field",
			body:     "`<x:change>`<x:old>use <x:change> tag</x:old><x:new>b</x:new></x:change>",
			wantBody: "`", old: "use <x:change> tag", new: "b", children: 1,
		},
		{
			name:     "orphan fields before fresh wrapper",
			body:     "`<x:change>`<x:old>a</x:old><x:new>b</x:new> <x:change><x:old>c</x:old><x:new>d</x:new></x:change>",
			wantBody: "`<x:change>`<x:old>a</x:old><x:new>b</x:new> ", old: "c", new: "d", children: 1,
		},
		{
			name:     "unclosed unknown field inside quote",
			body:     "`<x:change><x:unknown>` ending in </x:change>.",
			wantBody: "`<x:change><x:unknown>` ending in </x:change>.",
		},
		{
			name:     "bare prose close with unclosed fields",
			body:     "Use `<x:change>` with <x:old> and <x:new> fields, ending in </x:change>.",
			wantBody: "Use `<x:change>` with <x:old> and <x:new> fields, ending in </x:change>.",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, ordered := range []bool{false, true} {
				p := &Problems{}
				input := "<x:payload>" + tc.body + "</x:payload><x:text>keep</x:text>"
				var cmds []Command
				complete := true
				if ordered {
					cmds, complete = ParseOrdered(p, g, input)
				} else {
					cmds = Parse(p, g, input)
				}
				if !complete || len(cmds) != 2 || cmds[1].Body != "keep" || !strings.Contains(strings.Join(p.List, " "), "wrap a literal wrapper example in <x:raw>...</x:raw>") {
					t.Fatalf("ordered=%v: commands=%+v complete=%v problems=%v", ordered, cmds, complete, p.List)
				}
				if ordered && (len(cmds[0].Problems) == 0 || len(cmds[1].Problems) != 0) {
					t.Fatalf("refusal was not command-local: %+v", cmds)
				}
				if cmds[0].Body != tc.wantBody || len(cmds[0].Children) != tc.children {
					t.Fatalf("ordered=%v: payload=%+v problems=%v", ordered, cmds[0], p.List)
				}
				if tc.children != 0 {
					child := cmds[0].Children[0]
					if !reflect.DeepEqual(child.FieldsOf("x:old"), []string{tc.old}) || !reflect.DeepEqual(child.FieldsOf("x:new"), []string{tc.new}) {
						t.Fatalf("raw fields changed: %+v", child)
					}
				}
			}
		})
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
	// Owner-b requires a raw element (or CDATA) for examples containing their
	// own close.
	body := "`<x:text>one</x:text><x:text>two</x:text>`"
	cmds, problems = parse(g, "<x:text>"+body+"</x:text>")
	if len(problems) == 0 || !strings.Contains(strings.Join(problems, " "), "<x:raw>...</x:raw>") {
		t.Fatalf("own-close example was not structural: %+v, %q", cmds, problems)
	}
	for _, wrapped := range []string{"<x:raw>" + body + "</x:raw>", "<![CDATA[" + body + "]]>"} {
		cmds, problems = parse(g, "<x:text>"+wrapped+"</x:text>")
		if len(problems) != 0 || len(cmds) != 1 || cmds[0].Body != body {
			t.Fatalf("wrapped example %q: %+v, %q", wrapped, cmds, problems)
		}
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

func literalMalformedMarkupFixture(n int) (Grammar, string, string) {
	g := Grammar{Prefix: "wb:", LiteralCode: true, Group: &Group{Name: "topic", Attr: "name"}, Commands: map[string]Spec{"panel": {Kind: Leaf, Attrs: []string{"recap"}, Scoped: true}}}
	body := "`</wb:panel>" + strings.Repeat("<a?", n) + "`"
	input := `<wb:topic name="t"><wb:panel recap="r">` + "`<![CDATA[</wb:panel>]]>" + strings.Repeat("<a?", n) + "`" + `</wb:panel></wb:topic>`
	return g, input, body
}

func BenchmarkLiteralCodeMalformedMarkup(b *testing.B) {
	for _, n := range []int{33300, 66600} {
		g, input, body := literalMalformedMarkupFixture(n)
		b.Run(fmt.Sprint(len(body)), func(b *testing.B) {
			b.SetBytes(int64(len(body)))
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				cmds, problems := parse(g, input)
				if len(problems) != 0 || len(cmds) != 1 || cmds[0].Body != body {
					b.Fatalf("legal literal changed: commands=%d problems=%v", len(cmds), problems)
				}
			}
		})
	}
}

func TestLiteralCodeMalformedMarkupIsVerbatim(t *testing.T) {
	g, input, body := literalMalformedMarkupFixture(66600)
	start := time.Now()
	cmds, problems := parse(g, input)
	t.Logf("%d-byte malformed-name literal parsed in %s", len(body), time.Since(start))
	if len(problems) != 0 || len(cmds) != 1 || cmds[0].Body != body {
		t.Fatalf("legal literal changed: commands=%d problems=%v", len(cmds), problems)
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

// Attributes and every body decode the same way; CDATA and raw segments
// do not.
func TestEveryBodyDecodes(t *testing.T) {
	cmds, problems := parse(testGrammar, `<tr:note id="&lt;1&gt;">a &lt;b&gt; <![CDATA[&lt;c&gt;]]> &amp;d</tr:note>`+
		`<tr:set><tr:name>&lt;n&gt;</tr:name><tr:tag>&lt;t&gt;</tr:tag></tr:set>`+
		"<tr:note>\n&#10;x&#10;\n</tr:note>")
	if len(problems) > 0 {
		t.Fatalf("problems: %q", problems)
	}
	if id, _ := cmds[0].Attr("id"); id != "<1>" || cmds[0].Body != "a <b> &lt;c&gt; &d" {
		t.Errorf("decoded leaf = %+v", cmds[0])
	}
	if name, tag := cmds[1].FieldsOf("tr:name"), cmds[1].FieldsOf("tr:tag"); name[0] != "<n>" || tag[0] != "<t>" {
		t.Errorf("fields = %+v", cmds[1].Fields)
	}
	// The body's own newlines are stripped before decoding, so a decoded
	// newline at either end stays.
	if cmds[2].Body != "\nx\n" {
		t.Errorf("decoded newlines = %q", cmds[2].Body)
	}
	// Older input stays accepted: a bare < or & that starts nothing is text.
	cmds, problems = parse(testGrammar, `<tr:note>a <b>bold</b> & c &nope; <![CDATA[<i>]]></tr:note>`)
	if len(problems) > 0 || cmds[0].Body != "a <b>bold</b> & c &nope; <i>" {
		t.Fatalf("old-style body = %+v, %q", cmds, problems)
	}
}

// A raw element is a text segment taken exactly as written up to its first
// close, anywhere text may go, mixed with decoded text.
func TestRawElementIsVerbatimTextSegment(t *testing.T) {
	for name, tc := range map[string]struct{ in, want string }{
		"html":      {`<tr:raw><div><b>a < b</b></div></tr:raw>`, `<div><b>a < b</b></div>`},
		"markers":   {`<tr:raw><![CDATA[x]]> ]]> </tr:note> &lt; <tr:note></tr:raw>`, `<![CDATA[x]]> ]]> </tr:note> &lt; <tr:note>`},
		"mixed":     {`a &lt; b <tr:raw><i>c</i></tr:raw> d <tr:raw>&amp;</tr:raw>`, `a < b <i>c</i> d &amp;`},
		"in-cdata":  {`<![CDATA[<tr:raw>]]>&lt;<tr:raw>&lt;</tr:raw>`, `<tr:raw><&lt;`},
		"edges":     {"\n<tr:raw>\nx\n</tr:raw>\n", "\nx\n"},
		"edge-text": {"\nx<tr:raw>y</tr:raw>\n", "xy"},
		"not-raw":   {`<tr:raw a="1">x</tr:raw> <tr:raw/> < tr:raw>`, `<tr:raw a="1">x</tr:raw> <tr:raw/> < tr:raw>`},
		"escaped":   {`&lt;/tr:raw>`, `</tr:raw>`},
	} {
		cmds, problems := parse(testGrammar, "<tr:note>"+tc.in+"</tr:note>")
		if len(problems) > 0 || len(cmds) != 1 || cmds[0].Body != tc.want {
			t.Errorf("%s: %+v, %q; want body %q", name, cmds, problems, tc.want)
		}
	}
	cmds, problems := parse(testGrammar, `<tr:set><tr:name><tr:raw></tr:name></tr:raw></tr:name><tr:tag>t</tr:tag></tr:set>`)
	if len(problems) > 0 || cmds[0].FieldsOf("tr:name")[0] != "</tr:name>" || cmds[0].FieldsOf("tr:tag")[0] != "t" {
		t.Fatalf("raw field: %+v, %q", cmds, problems)
	}

	// Unclosed: refused like an unclosed CDATA section, at its position.
	_, problems = parse(testGrammar, "<tr:note>\nx <tr:raw>never closed</tr:note>")
	if !slices.Equal(problems, []string{"line 2: <tr:note>: a <tr:raw> element has no closing </tr:raw>"}) {
		t.Fatalf("unclosed raw = %q", problems)
	}

	// Where a command or field is expected, a raw element is unknown; a
	// refused one ends at its first close in both readers.
	for _, ordered := range []bool{false, true} {
		p := &Problems{}
		input := `<tr:raw><tr:raw></tr:raw><tr:set><tr:raw>x</tr:raw></tr:set><tr:drop id="1"/>`
		var cmds []Command
		if ordered {
			cmds, _ = ParseOrdered(p, testGrammar, input)
		} else {
			cmds = Parse(p, testGrammar, input)
		}
		want := []string{
			"command 1 <tr:raw> (line 1): unknown command (commands: tr:drop, tr:note, tr:set)",
			"command 2 <tr:set> (line 1): unknown field <tr:raw> (fields: tr:name, tr:tag)",
		}
		if !slices.Equal(p.List, want) || cmds[len(cmds)-1].Tag != "tr:drop" {
			t.Errorf("ordered=%v refused raw: %+v, %q", ordered, cmds, p.List)
		}
	}
}

// Inside code, a raw element opens exactly where CDATA does, and hides a
// body close from the literal candidate like CDATA.
func TestRawElementInsideLiteralCode(t *testing.T) {
	g := Grammar{Prefix: "x:", LiteralCode: true, Commands: map[string]Spec{
		"text": {Kind: Leaf},
	}}
	for in, want := range map[string]string{
		"`<x:raw></x:text></x:raw>` &lt;":         "`</x:text>` <",
		"```\n<x:raw>&amp;</x:raw> &amp;\n```":    "```\n&amp; &\n```",
		"`<![CDATA[<x:raw>]]>` <x:raw>`</x:raw>`": "`<x:raw>` ``",
	} {
		cmds, problems := parse(g, "<x:text>"+in+"</x:text>")
		if len(problems) > 0 || len(cmds) != 1 || cmds[0].Body != want {
			t.Errorf("%q: %+v, %q; want %q", in, cmds, problems, want)
		}
	}
	// A code example crossing its own close is guided to the raw element.
	_, problems := parse(g, "<x:text>`</x:text>`</x:text>")
	if len(problems) == 0 || !strings.Contains(strings.Join(problems, " "), "<x:raw>...</x:raw>") || strings.Contains(strings.Join(problems, " "), "CDATA") {
		t.Fatalf("guidance = %q", problems)
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

func TestParseOrderedStrayFragmentLabels(t *testing.T) {
	for name, tc := range map[string]struct {
		input, label string
	}{
		"root-text":   {"loose\n<tr:drop id=\"1\"/>", "stray fragment (line 1)"},
		"root-close":  {"\n</tr:oops>", "stray fragment (line 2)"},
		"group-text":  {"<tr:in name=\"a\"><tr:note>x</tr:note>\nloose</tr:in>", `stray fragment inside <tr:in name="a"> (line 2)`},
		"group-close": {`<tr:in name="a"></tr:oops></tr:in>`, `stray fragment inside <tr:in name="a"> (line 1)`},
	} {
		t.Run(name, func(t *testing.T) {
			p := &Problems{}
			cmds, complete := ParseOrdered(p, groupGrammar, tc.input)
			if !complete {
				t.Fatal(p.List)
			}
			fragments := 0
			for _, cmd := range cmds {
				if cmd.Pos >= 0 {
					continue
				}
				fragments++
				if cmd.Label() != tc.label || len(cmd.Problems) != 1 || !strings.HasPrefix(cmd.Problems[0], tc.label+": ") || !slices.Contains(p.List, cmd.Problems[0]) {
					t.Fatalf("fragment label/diagnostics: %+v; %v", cmd, p.List)
				}
			}
			if fragments != 1 {
				t.Fatalf("fragment count %d: %+v", fragments, cmds)
			}
		})
	}
}

func TestParseOrderedRejectedGroupContainerFields(t *testing.T) {
	prefix := `<tr:drop id="1"/>`
	suffix := `<tr:drop id="2"/>`
	for _, verbatim := range []string{"<![CDATA[%s]]>", "<tr:raw>%s</tr:raw>"} {
		quoted := fmt.Sprintf(verbatim, `</tr:name></tr:set><tr:in>`)
		rejected := `<tr:in name="outer">junk<tr:set><tr:name></tr:set></tr:in><tr:in name="literal">` + quoted + `</tr:name></tr:set></tr:in>`
		p := &Problems{}
		cmds, complete := ParseOrdered(p, groupGrammar, prefix+rejected+suffix)
		if !complete || len(p.List) != 1 || len(cmds) != 3 {
			t.Fatalf("field payload escaped rejected group: complete=%v commands=%+v problems=%v", complete, cmds, p.List)
		}
		if cmds[0].Source != prefix || cmds[1].Source != rejected || cmds[2].Source != suffix || len(cmds[1].Scope.Problems) != 1 {
			t.Fatalf("lost source boundaries/diagnostics: %+v", cmds)
		}
	}
}

func TestParseOrderedRejectedGroupFieldDiagnostics(t *testing.T) {
	for name, tc := range map[string]struct {
		payload, diagnostic string
	}{
		"attribute":     {`<tr:name typo="x">T</tr:name>`, `field <tr:name>: unknown attribute typo (it takes none)`},
		"unknown-field": {`<tr:typo>T</tr:typo>`, `: unknown field <tr:typo> (fields: tr:name)`},
		"text":          {`loose<tr:name>T</tr:name>`, `outside its field tags: "loose"`},
	} {
		t.Run(name, func(t *testing.T) {
			prefix, suffix := `<tr:drop id="1"/>`, `<tr:drop id="2"/>`
			rejected := `<tr:in name="outer">junk<tr:set>` + tc.payload + `</tr:set></tr:in>`
			p := &Problems{}
			cmds, complete := ParseOrdered(p, groupGrammar, prefix+rejected+suffix)
			if !complete || len(cmds) != 3 || len(p.List) != 2 {
				t.Fatalf("skipped field boundary: complete=%v commands=%+v problems=%v", complete, cmds, p.List)
			}
			if cmds[0].Pos != 1 || cmds[0].Source != prefix || cmds[2].Pos != 2 || cmds[2].Source != suffix || cmds[1].Source != rejected || !slices.Equal(cmds[1].Scope.Problems, p.List) {
				t.Fatalf("lost positions/source/attribution: %+v %v", cmds, p.List)
			}
			diagnostics := strings.Join(p.List, "\n")
			if !strings.Contains(diagnostics, `<tr:in> (line 1) skipped <tr:set>`) || !strings.Contains(diagnostics, tc.diagnostic) || strings.Contains(diagnostics, "command 0") {
				t.Fatalf("skipped field diagnostics: %s", diagnostics)
			}
		})
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
		`<tr:raw>`,
		`<tr:set><tr:name><tr:raw></tr:name>`,
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
