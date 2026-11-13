package wsinbox

import (
	"errors"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestMentionBlockGrammar(t *testing.T) {
	got := MentionBlock([]Mention{
		{Topic: "ship-gate", ID: "316", Text: "a < b && c > d"},
		{Topic: `Plain "name" & <more>`, Text: "line one\nline two"},
	})
	want := `<wb:mention topic="ship-gate" id="316">a &lt; b &amp;&amp; c &gt; d</wb:mention>` + "\n" +
		`<wb:mention topic="Plain &quot;name&quot; &amp; &lt;more&gt;">line one` + "\nline two</wb:mention>"
	if got != want {
		t.Fatalf("MentionBlock =\n%s\nwant\n%s", got, want)
	}
	if MentionBlock(nil) != "" {
		t.Fatal("no mentions must render nothing")
	}
}

// TestEntryTextPlacesMentions: the mentions follow the answer block and come
// before the typed text and the image lines, in the order added; a message
// with no answers starts with them.
func TestEntryTextPlacesMentions(t *testing.T) {
	e := Entry{
		Parts:    []string{"my text"},
		Chips:    []Chip{{ID: "5", Answer: "a"}},
		Mentions: []Mention{{Topic: "t", ID: "5", Text: "first"}, {Topic: "u", Text: "second"}},
		Images:   []string{"/x/a.png"},
	}
	want := "<wb:answers>\n5=a\n</wb:answers>\n\n" +
		`<wb:mention topic="t" id="5">first</wb:mention>` + "\n" + `<wb:mention topic="u">second</wb:mention>` +
		"\n\nmy text\n\nImage #1: /x/a.png"
	if got := e.Text(); got != want {
		t.Fatalf("Text =\n%s\nwant\n%s", got, want)
	}
	e.Chips = nil
	if got := e.Text(); !strings.HasPrefix(got, `<wb:mention topic="t" id="5">first</wb:mention>`) {
		t.Fatalf("without answers the mentions open the message:\n%s", got)
	}
}

func TestNormalizeMentions(t *testing.T) {
	out, err := Normalize(Entry{Mentions: []Mention{
		{Topic: " t ", ID: " 12 ", Text: "  kept  "},
		{Topic: "t", Text: " \n "},
		{Topic: "u", Text: "no id"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	want := []Mention{{Topic: "t", ID: "12", Text: "kept"}, {Topic: "u", Text: "no id"}}
	if !reflect.DeepEqual(out.Mentions, want) {
		t.Fatalf("mentions = %+v, want %+v", out.Mentions, want)
	}
	if _, err := Normalize(Entry{Mentions: []Mention{{Topic: "t", Text: "  "}}}); !errors.Is(err, ErrEmpty) {
		t.Fatalf("a blank mention alone is no content: %v", err)
	}
	for name, m := range map[string]Mention{
		"empty topic":   {Topic: " ", Text: "x"},
		"newline topic": {Topic: "a\nb", Text: "x"},
		"long topic":    {Topic: strings.Repeat("t", maxTopicLen+1), Text: "x"},
		"bad id":        {Topic: "t", ID: "12a", Text: "x"},
		"too much text": {Topic: "t", Text: strings.Repeat("x", maxTextBytes+1)},
	} {
		if _, err := Normalize(Entry{Mentions: []Mention{m}}); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
	many := make([]Mention, maxMentions+1)
	for i := range many {
		many[i] = Mention{Topic: "t", Text: "x"}
	}
	if _, err := Normalize(Entry{Mentions: many}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("too many mentions: %v", err)
	}
}

func TestMergeAppendsMentions(t *testing.T) {
	base := Entry{Mentions: []Mention{{Topic: "a", Text: "1"}}}
	add := Entry{Mentions: []Mention{{Topic: "a", Text: "1"}, {Topic: "b", Text: "2"}}}
	got := Merge(base, add).Mentions
	want := []Mention{{Topic: "a", Text: "1"}, {Topic: "a", Text: "1"}, {Topic: "b", Text: "2"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("merged mentions = %+v, want %+v", got, want)
	}
	if got := Merge(Entry{Parts: []string{"x"}}, Entry{Parts: []string{"y"}}).Mentions; got != nil {
		t.Fatalf("no mentions merged to %+v", got)
	}
}

// TestParseMentionsRoundTrips: a delivered message's mentions come back,
// unescaped, after its answer block is taken out, and the typed text stays.
func TestParseMentionsRoundTrips(t *testing.T) {
	mentions := []Mention{
		{Topic: "ship-gate", ID: "316", Entry: "e17", Text: "a < b &amp; c > d"},
		{Topic: `Plain "name" &`, Entry: "e18", Text: "multi\nline"},
		{Topic: "legacy", ID: "7", Text: "old-shaped mention"},
	}
	text := Entry{Parts: []string{"user text"}, Chips: []Chip{{ID: "1", Answer: "ok"}}, Mentions: mentions}.Text()
	_, rest, ok := ParseAnswerBlock(text)
	if !ok {
		t.Fatalf("answer block not parsed:\n%s", text)
	}
	got, rest := ParseMentions(rest)
	if !reflect.DeepEqual(got, mentions) || rest != "user text" {
		t.Fatalf("ParseMentions = %+v, %q", got, rest)
	}
	got, rest = ParseMentions(MentionBlock(mentions[:1]))
	if len(got) != 1 || rest != "" {
		t.Fatalf("mentions alone: %+v %q", got, rest)
	}
}

func TestEntryMentionGrammarAndValidation(t *testing.T) {
	mentions := []Mention{
		{Topic: "topic", Entry: "e7", Text: "panel version"},
		{Topic: "topic", ID: "3", Entry: "e8", Text: "system provenance"},
	}
	want := `<wb:mention topic="topic" entry="e7">panel version</wb:mention>` + "\n" +
		`<wb:mention topic="topic" id="3" entry="e8">system provenance</wb:mention>`
	if got := MentionBlock(mentions); got != want {
		t.Fatalf("entry mention grammar = %q", got)
	}
	got, rest := ParseMentions(want)
	if !reflect.DeepEqual(got, mentions) || rest != "" {
		t.Fatalf("parse = %+v, %q", got, rest)
	}
	out, err := Normalize(Entry{Mentions: []Mention{{Topic: " t ", ID: " 3 ", Entry: " e8 ", Text: " selected "}}})
	if err != nil || !reflect.DeepEqual(out.Mentions, []Mention{{Topic: "t", ID: "3", Entry: "e8", Text: "selected"}}) {
		t.Fatalf("normalize = %+v, %v", out.Mentions, err)
	}
	for _, id := range []string{"p7", "e0", "e01", "e-1", "7", "e7x", "e1\nentry", `e7" id="9`} {
		if _, err := Normalize(Entry{Mentions: []Mention{{Topic: "t", Entry: id, Text: "x"}}}); !errors.Is(err, ErrInvalid) {
			t.Errorf("entry %q: %v", id, err)
		}
	}
	// T2 emission intentionally does not parse with the old strict grammar,
	// while old topic/item-only elements still parse with either reader.
	legacyHead := regexp.MustCompile(`^<wb:mention topic="([^"]*)"(?: id="([0-9]{1,6})")?>`)
	if legacyHead.MatchString(want) || !legacyHead.MatchString(MentionBlock([]Mention{{Topic: "t", ID: "7", Text: "legacy"}})) {
		t.Fatal("legacy strict-parser compatibility assumption changed")
	}
}

func TestEntryMentionsSurviveInboxLifecycle(t *testing.T) {
	dir := Dir(t.TempDir())
	first := Mention{Topic: "t", Entry: "e7", Text: "same selected text"}
	second := Mention{Topic: "t", ID: "3", Entry: "e8", Text: "same selected text"}
	now := time.Now()
	if _, _, err := Send(dir, Entry{Mentions: []Mention{first}}, now); err != nil {
		t.Fatal(err)
	}
	name, merged, err := Send(dir, Entry{Mentions: []Mention{second}}, now.Add(time.Millisecond))
	if err != nil || !merged {
		t.Fatalf("merge = %v, %v", merged, err)
	}
	want := []Mention{first, second}
	held, err := Held(dir)
	if err != nil || len(held) != 1 || !reflect.DeepEqual(held[0].Entry.Mentions, want) {
		t.Fatalf("held = %+v, %v", held, err)
	}
	cancelled, err := Cancel(dir, name)
	if err != nil || !reflect.DeepEqual(cancelled.Mentions, want) {
		t.Fatalf("cancel = %+v, %v", cancelled.Mentions, err)
	}
	name, _, err = Send(dir, cancelled, now.Add(2*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Steer(dir, name); err != nil {
		t.Fatal(err)
	}
	taken, _, err := Interrupt(dir, now.Add(3*time.Millisecond))
	if err != nil || len(taken) != 1 || !reflect.DeepEqual(taken[0].Entry.Mentions, want) {
		t.Fatalf("interrupt = %+v, %v", taken, err)
	}
	if _, _, err := Send(dir, taken[0].Entry, now.Add(4*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	drained, err := DrainSteer(dir)
	if err != nil || len(drained) != 1 || !reflect.DeepEqual(drained[0].Entry.Mentions, want) {
		t.Fatalf("steer drain = %+v, %v", drained, err)
	}
	got, rest := ParseMentions(drained[0].Entry.Text())
	if rest != "" || !reflect.DeepEqual(got, want) {
		t.Fatalf("delivered locators = %+v, %q", got, rest)
	}
}

func TestParseMentionsLeavesOtherTextWhole(t *testing.T) {
	for _, text := range []string{
		"plain text",
		"text first\n\n<wb:mention topic=\"t\">x</wb:mention>",
		"<wb:mention topic=\"t\">no close",
		"<wb:mention topic=\"t\">raw <b>tag</b></wb:mention>",
		"<wb:mention topic=\"t\" id=\"x\">bad id</wb:mention>",
		`<wb:mention topic="t" entry="p7">panel target is not a version</wb:mention>`,
		`<wb:mention topic="t" entry="e0">zero</wb:mention>`,
		`<wb:mention topic="t" entry="e01">noncanonical</wb:mention>`,
		`<wb:mention topic="t" entry="e-1">negative</wb:mention>`,
		`<wb:mention topic="t" entry="e7x">suffix</wb:mention>`,
		`<wb:mention topic="t" entry="e7" id="3">wrong attribute order</wb:mention>`,
		`<wb:mention topic="t" entry="e7" entry="e8">duplicate</wb:mention>`,
		`<wb:mention topic="t" entry="e7" other="x">unknown attribute</wb:mention>`,
		"<wb:mention topic=\"t\">x</wb:mention>glued",
	} {
		if got, rest := ParseMentions(text); got != nil || rest != text {
			t.Errorf("ParseMentions(%q) = %+v, %q; want the text whole", text, got, rest)
		}
	}
	// Parsing stops at the first malformed element; the ones before it stand.
	got, rest := ParseMentions("<wb:mention topic=\"t\">ok</wb:mention>\n<wb:mention topic=\"t\">open\n\ntext")
	if len(got) != 1 || got[0].Text != "ok" || rest != "<wb:mention topic=\"t\">open\n\ntext" {
		t.Fatalf("partial parse = %+v, %q", got, rest)
	}
}
