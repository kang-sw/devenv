package wsinbox

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

// TestWebBlockGrammar: one pick per line and one console element per block,
// with the mention escaping; a block renders nothing when empty.
func TestWebBlockGrammar(t *testing.T) {
	got := PickBlock([]Pick{
		{URL: "http://localhost:5173/settings", Selector: "#save", Text: "Save"},
		{URL: `http://localhost:5173/?a="b"&c=<d>`, Selector: `[data-testid="x"]`, Text: "a < b && c > d"},
		{URL: "http://localhost:5173/", Selector: "html > body", Text: ""},
	})
	want := `<wb:pick url="http://localhost:5173/settings" selector="#save">Save</wb:pick>` + "\n" +
		`<wb:pick url="http://localhost:5173/?a=&quot;b&quot;&amp;c=&lt;d&gt;" selector="[data-testid=&quot;x&quot;]">a &lt; b &amp;&amp; c &gt; d</wb:pick>` + "\n" +
		`<wb:pick url="http://localhost:5173/" selector="html &gt; body"></wb:pick>`
	if got != want {
		t.Fatalf("PickBlock =\n%s\nwant\n%s", got, want)
	}
	got = ConsoleBlock([]Console{
		{URL: "http://localhost:5173/settings", Lines: []string{"first error", "second <error>"}},
		{URL: "http://localhost:5173/x", Lines: []string{"only"}},
	})
	want = `<wb:console url="http://localhost:5173/settings">first error` + "\nsecond &lt;error&gt;</wb:console>\n" +
		`<wb:console url="http://localhost:5173/x">only</wb:console>`
	if got != want {
		t.Fatalf("ConsoleBlock =\n%s\nwant\n%s", got, want)
	}
	if PickBlock(nil) != "" || ConsoleBlock(nil) != "" {
		t.Fatal("no picks or console blocks must render nothing")
	}
}

// TestEntryTextPlacesWebElements: the picks and the console blocks follow
// the mention block and come before the typed text, as one paragraph each.
func TestEntryTextPlacesWebElements(t *testing.T) {
	e := Entry{
		Parts:    []string{"my text"},
		Chips:    []Chip{{ID: "5", Answer: "a"}},
		Mentions: []Mention{{Topic: "t", Text: "m"}},
		Picks:    []Pick{{URL: "http://localhost:1/", Selector: "#a", Text: "A"}, {URL: "http://localhost:1/", Selector: "#b", Text: "B"}},
		Consoles: []Console{{URL: "http://localhost:1/", Lines: []string{"e1", "e2"}}, {URL: "http://localhost:2/", Lines: []string{"e3"}}},
		Images:   []string{"/x/a.png"},
	}
	want := "<wb:answers>\n5=a\n</wb:answers>\n\n" +
		`<wb:mention topic="t">m</wb:mention>` + "\n\n" +
		`<wb:pick url="http://localhost:1/" selector="#a">A</wb:pick>` + "\n" + `<wb:pick url="http://localhost:1/" selector="#b">B</wb:pick>` + "\n\n" +
		`<wb:console url="http://localhost:1/">e1` + "\ne2</wb:console>\n" + `<wb:console url="http://localhost:2/">e3</wb:console>` +
		"\n\nmy text\n\nImage #1: /x/a.png"
	if got := e.Text(); got != want {
		t.Fatalf("Text =\n%s\nwant\n%s", got, want)
	}
}

// TestParseWebElementsRoundTrips: a delivered message's picks and console
// blocks come back unescaped after the mentions are taken out, and the typed
// text stays.
func TestParseWebElementsRoundTrips(t *testing.T) {
	picks := []Pick{
		{URL: `http://localhost:5173/?q="a"&b=1`, Selector: `[data-testid="save"]`, Text: "a < b &amp; c > d"},
		{URL: "http://localhost:5173/", Selector: "html > body", Text: "multi\nline text"},
		{URL: "http://localhost:5173/", Selector: "#empty"},
	}
	consoles := []Console{
		{URL: "http://localhost:5173/settings", Lines: []string{"TypeError: x is <undefined> & more", "  at f (app.js:1:2)"}},
		{URL: "http://localhost:5173/other", Lines: []string{"one"}},
	}
	mentions := []Mention{{Topic: "t", ID: "3", Text: "m"}}
	text := Entry{Parts: []string{"user text"}, Mentions: mentions, Picks: picks, Consoles: consoles}.Text()
	gotMentions, rest := ParseMentions(text)
	if len(gotMentions) != 1 {
		t.Fatalf("mentions not parsed:\n%s", text)
	}
	gotPicks, gotConsoles, rest := ParseWebElements(rest)
	if !reflect.DeepEqual(gotPicks, picks) || !reflect.DeepEqual(gotConsoles, consoles) || rest != "user text" {
		t.Fatalf("ParseWebElements = %+v, %+v, %q", gotPicks, gotConsoles, rest)
	}
	gotPicks, gotConsoles, rest = ParseWebElements(PickBlock(picks[:1]))
	if len(gotPicks) != 1 || gotConsoles != nil || rest != "" {
		t.Fatalf("picks alone: %+v %+v %q", gotPicks, gotConsoles, rest)
	}
	gotPicks, gotConsoles, rest = ParseWebElements(ConsoleBlock(consoles[:1]))
	if gotPicks != nil || len(gotConsoles) != 1 || rest != "" {
		t.Fatalf("console alone: %+v %+v %q", gotPicks, gotConsoles, rest)
	}
}

// TestParseWebElementsLeavesOtherTextWhole: malformed input stops the parse,
// and what was parsed before it stays.
func TestParseWebElementsLeavesOtherTextWhole(t *testing.T) {
	for _, text := range []string{
		"plain text",
		"text first\n\n" + `<wb:pick url="u" selector="#a">x</wb:pick>`,
		`<wb:pick url="u" selector="#a">no close`,
		`<wb:pick url="u" selector="#a">raw <b>tag</b></wb:pick>`,
		`<wb:pick url="u">no selector</wb:pick>`,
		`<wb:pick selector="#a" url="u">wrong order</wb:pick>`,
		`<wb:pick url="u" selector="#a" other="x">unknown attribute</wb:pick>`,
		`<wb:pick url="u" selector="#a">x</wb:pick>glued`,
		`<wb:console url="u">no close`,
		`<wb:console url="u">raw <b>tag</b></wb:console>`,
		`<wb:console>no url</wb:console>`,
		`<wb:console url="u">x</wb:console>glued`,
	} {
		if p, c, rest := ParseWebElements(text); p != nil || c != nil || rest != text {
			t.Errorf("ParseWebElements(%q) = %+v, %+v, %q; want the text whole", text, p, c, rest)
		}
	}
	// A console element before a pick is not "picks then consoles": the
	// console block is taken, the later pick is typed text.
	text := `<wb:console url="u">e</wb:console>` + "\n" + `<wb:pick url="u" selector="#a">x</wb:pick>`
	p, c, rest := ParseWebElements(text)
	if p != nil || len(c) != 1 || rest != `<wb:pick url="u" selector="#a">x</wb:pick>` {
		t.Errorf("console then pick = %+v, %+v, %q", p, c, rest)
	}
	// A malformed element after a good one stops there.
	text = `<wb:pick url="u" selector="#a">ok</wb:pick>` + "\n" + `<wb:pick url="u" selector="#b">bad <i></wb:pick>` + "\nrest"
	p, c, rest = ParseWebElements(text)
	if len(p) != 1 || c != nil || !strings.HasPrefix(rest, `<wb:pick url="u" selector="#b">bad`) {
		t.Errorf("good then bad = %+v, %+v, %q", p, c, rest)
	}
}

// TestNormalizeWebElements: fields are trimmed, a pick may have no text, and
// blank console lines and a block left with none are dropped.
func TestNormalizeWebElements(t *testing.T) {
	out, err := Normalize(Entry{
		Picks: []Pick{{URL: " http://localhost:1/ ", Selector: " #a ", Text: " \n kept \n "}, {URL: "http://localhost:1/", Selector: "#b", Text: "  "}},
		Consoles: []Console{
			{URL: " http://localhost:1/ ", Lines: []string{"e1", "", "   ", "  indented"}},
			{URL: "http://localhost:2/", Lines: []string{"", " "}},
			{URL: "http://localhost:3/"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	wantPicks := []Pick{{URL: "http://localhost:1/", Selector: "#a", Text: "kept"}, {URL: "http://localhost:1/", Selector: "#b"}}
	wantConsoles := []Console{{URL: "http://localhost:1/", Lines: []string{"e1", "  indented"}}}
	if !reflect.DeepEqual(out.Picks, wantPicks) || !reflect.DeepEqual(out.Consoles, wantConsoles) {
		t.Fatalf("normalized = %+v, %+v", out.Picks, out.Consoles)
	}
	if _, err := Normalize(Entry{Consoles: []Console{{URL: "u", Lines: []string{" "}}}}); !errors.Is(err, ErrEmpty) {
		t.Fatalf("a console block with no lines is no content: %v", err)
	}
	for name, e := range map[string]Entry{
		"pick no url":          {Picks: []Pick{{Selector: "#a"}}},
		"pick no selector":     {Picks: []Pick{{URL: "u", Selector: " "}}},
		"pick newline url":     {Picks: []Pick{{URL: "u\nv", Selector: "#a"}}},
		"pick newline select":  {Picks: []Pick{{URL: "u", Selector: "#a\r#b"}}},
		"console no url":       {Consoles: []Console{{Lines: []string{"e"}}}},
		"console newline url":  {Consoles: []Console{{URL: "u\nv", Lines: []string{"e"}}}},
		"console line LF":      {Consoles: []Console{{URL: "u", Lines: []string{"a\nb"}}}},
		"console line CR":      {Consoles: []Console{{URL: "u", Lines: []string{"a\rb"}}}},
		"console empty url ws": {Consoles: []Console{{URL: "  ", Lines: []string{"e"}}}},
	} {
		if _, err := Normalize(e); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
}

// TestNormalizeWebCaps: a message exactly at each cap is accepted, one over
// is ErrInvalid, and the elements do not count toward the text limit.
func TestNormalizeWebCaps(t *testing.T) {
	const url = "http://localhost:1/"
	pick := func(n int) []Pick {
		out := make([]Pick, n)
		for i := range out {
			out[i] = Pick{URL: url, Selector: "#a", Text: "x"}
		}
		return out
	}
	console := func(blocks, lines int) []Console {
		out := make([]Console, blocks)
		for i := range out {
			out[i] = Console{URL: url, Lines: make([]string, lines)}
			for j := range out[i].Lines {
				out[i].Lines[j] = "e"
			}
		}
		return out
	}
	long := func(n int) string { return strings.Repeat("x", n) }
	for _, c := range []struct {
		name     string
		at, over Entry
	}{
		{"picks", Entry{Picks: pick(maxPicks)}, Entry{Picks: pick(maxPicks + 1)}},
		{"console blocks", Entry{Consoles: console(maxConsoleBlocks, 1)}, Entry{Consoles: console(maxConsoleBlocks+1, 1)}},
		{"console lines", Entry{Consoles: console(1, maxConsoleLines)}, Entry{Consoles: console(1, maxConsoleLines+1)}},
		{"console line bytes", Entry{Consoles: []Console{{URL: url, Lines: []string{long(maxConsoleLineBytes)}}}}, Entry{Consoles: []Console{{URL: url, Lines: []string{long(maxConsoleLineBytes + 1)}}}}},
		{"selector bytes", Entry{Picks: []Pick{{URL: url, Selector: long(maxSelectorBytes)}}}, Entry{Picks: []Pick{{URL: url, Selector: long(maxSelectorBytes + 1)}}}},
		{"pick text bytes", Entry{Picks: []Pick{{URL: url, Selector: "#a", Text: long(maxPickTextBytes)}}}, Entry{Picks: []Pick{{URL: url, Selector: "#a", Text: long(maxPickTextBytes + 1)}}}},
		{"pick url bytes", Entry{Picks: []Pick{{URL: long(maxWebURLBytes), Selector: "#a"}}}, Entry{Picks: []Pick{{URL: long(maxWebURLBytes + 1), Selector: "#a"}}}},
		{"console url bytes", Entry{Consoles: []Console{{URL: long(maxWebURLBytes), Lines: []string{"e"}}}}, Entry{Consoles: []Console{{URL: long(maxWebURLBytes + 1), Lines: []string{"e"}}}}},
	} {
		if _, err := Normalize(c.at); err != nil {
			t.Errorf("%s at the cap: %v", c.name, err)
		}
		if _, err := Normalize(c.over); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s over the cap: err = %v, want ErrInvalid", c.name, err)
		}
	}
	// Every cap at once, beside text at its own limit: the elements' sizes
	// are bounded by their caps, not by maxTextBytes.
	full := Entry{Parts: []string{long(maxTextBytes)}, Consoles: console(maxConsoleBlocks, maxConsoleLines)}
	for i := 0; i < maxPicks; i++ {
		full.Picks = append(full.Picks, Pick{URL: long(maxWebURLBytes), Selector: long(maxSelectorBytes), Text: long(maxPickTextBytes)})
	}
	if _, err := Normalize(full); err != nil {
		t.Fatalf("a message at every cap: %v", err)
	}
}

// TestWebElementsOnlyMessagesAndCommands: a pick-only and a console-only
// message are accepted; a command carrying either is refused.
func TestWebElementsOnlyMessagesAndCommands(t *testing.T) {
	if _, err := Normalize(Entry{Picks: []Pick{{URL: "u", Selector: "#a"}}}); err != nil {
		t.Fatalf("pick-only: %v", err)
	}
	if _, err := Normalize(Entry{Consoles: []Console{{URL: "u", Lines: []string{"e"}}}}); err != nil {
		t.Fatalf("console-only: %v", err)
	}
	if _, err := Normalize(Entry{}); !errors.Is(err, ErrEmpty) {
		t.Fatalf("empty: %v", err)
	}
	cmd := &Command{Name: "compact"}
	if _, err := Normalize(Entry{Command: cmd, Picks: []Pick{{URL: "u", Selector: "#a"}}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("command with a pick: %v", err)
	}
	if _, err := Normalize(Entry{Command: cmd, Consoles: []Console{{URL: "u", Lines: []string{"e"}}}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("command with a console block: %v", err)
	}
}

// TestMergeAppendsWebElements: a later send's picks and console blocks follow
// the held ones, in order.
func TestMergeAppendsWebElements(t *testing.T) {
	a, b := Pick{URL: "u", Selector: "#a"}, Pick{URL: "u", Selector: "#b"}
	x, y := Console{URL: "u", Lines: []string{"x"}}, Console{URL: "v", Lines: []string{"y"}}
	got := Merge(Entry{Picks: []Pick{a}, Consoles: []Console{x}}, Entry{Picks: []Pick{a, b}, Consoles: []Console{y}})
	if !reflect.DeepEqual(got.Picks, []Pick{a, a, b}) || !reflect.DeepEqual(got.Consoles, []Console{x, y}) {
		t.Fatalf("merged = %+v, %+v", got.Picks, got.Consoles)
	}
	got = Merge(Entry{Parts: []string{"x"}}, Entry{Parts: []string{"y"}})
	if got.Picks != nil || got.Consoles != nil {
		t.Fatalf("none merged to %+v, %+v", got.Picks, got.Consoles)
	}
}

// TestWebElementsSurviveInboxLifecycle: picks and console blocks persist on
// disk through a send, a merge, a read, a cancel, and a drain, and the
// delivered text parses back to them.
func TestWebElementsSurviveInboxLifecycle(t *testing.T) {
	dir := Dir(t.TempDir())
	p1 := Pick{URL: "http://localhost:1/", Selector: "#a", Text: "A"}
	p2 := Pick{URL: "http://localhost:1/", Selector: "#b"}
	c1 := Console{URL: "http://localhost:1/", Lines: []string{"e1", "e2"}}
	c2 := Console{URL: "http://localhost:2/", Lines: []string{"e3"}}
	now := time.Now()
	if _, _, err := Send(dir, Entry{Picks: []Pick{p1}, Consoles: []Console{c1}}, now); err != nil {
		t.Fatal(err)
	}
	name, merged, err := Send(dir, Entry{Parts: []string{"look"}, Picks: []Pick{p2}, Consoles: []Console{c2}}, now.Add(time.Millisecond))
	if err != nil || !merged {
		t.Fatalf("merge = %v, %v", merged, err)
	}
	wantP, wantC := []Pick{p1, p2}, []Console{c1, c2}
	held, err := Held(dir)
	if err != nil || len(held) != 1 || !reflect.DeepEqual(held[0].Entry.Picks, wantP) || !reflect.DeepEqual(held[0].Entry.Consoles, wantC) {
		t.Fatalf("held = %+v, %v", held, err)
	}
	cancelled, err := Cancel(dir, name)
	if err != nil || !reflect.DeepEqual(cancelled.Picks, wantP) || !reflect.DeepEqual(cancelled.Consoles, wantC) {
		t.Fatalf("cancel = %+v, %v", cancelled, err)
	}
	if _, _, err := Send(dir, cancelled, now.Add(2*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	drained, err := Drain(dir)
	if err != nil || len(drained) != 1 || !reflect.DeepEqual(drained[0].Entry.Picks, wantP) || !reflect.DeepEqual(drained[0].Entry.Consoles, wantC) {
		t.Fatalf("drain = %+v, %v", drained, err)
	}
	gotP, gotC, rest := ParseWebElements(drained[0].Entry.Text())
	if !reflect.DeepEqual(gotP, wantP) || !reflect.DeepEqual(gotC, wantC) || rest != "look" {
		t.Fatalf("delivered = %+v, %+v, %q", gotP, gotC, rest)
	}
}
