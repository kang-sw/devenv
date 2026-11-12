package wsinbox

import (
	"errors"
	"reflect"
	"strings"
	"testing"
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
		{Topic: "ship-gate", ID: "316", Text: "a < b &amp; c > d"},
		{Topic: `Plain "name" &`, Text: "multi\nline"},
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

func TestParseMentionsLeavesOtherTextWhole(t *testing.T) {
	for _, text := range []string{
		"plain text",
		"text first\n\n<wb:mention topic=\"t\">x</wb:mention>",
		"<wb:mention topic=\"t\">no close",
		"<wb:mention topic=\"t\">raw <b>tag</b></wb:mention>",
		"<wb:mention topic=\"t\" id=\"x\">bad id</wb:mention>",
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
