package wsxmlcmd

import (
	"slices"
	"strings"
	"testing"
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
