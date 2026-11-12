package wsinbox

import (
	"reflect"
	"strings"
	"testing"
)

func TestAnswerBlockGrammar(t *testing.T) {
	block := func(lines ...string) string {
		return "<wb:answers>\n" + strings.Join(lines, "\n") + "\n</wb:answers>"
	}
	cases := []struct {
		name   string
		chips  []Chip
		topics []TopicNote
		want   string
	}{
		{"empty", nil, nil, ""},
		{"blank chips and topic notes", []Chip{{ID: "1"}, {ID: "2", Note: "  "}}, []TopicNote{{Topic: "t", Note: " "}}, ""},
		{"one answer", []Chip{{ID: "341", Answer: "a"}}, nil, block("341=a")},
		{"same value groups ascending", []Chip{{ID: "691", Answer: "b"}, {ID: "123", Answer: "b"}, {ID: "567", Answer: "b"}}, nil, block("123,567,691=b")},
		{"lines keep first-seen order", []Chip{{ID: "5", Answer: "ok"}, {ID: "2", Answer: "a"}, {ID: "1", Answer: "ok"}}, nil, block("1,5=ok", "2=a")},
		{"a run of three or more is a range", []Chip{{ID: "126", Answer: "ok"}, {ID: "124", Answer: "ok"}, {ID: "125", Answer: "ok"}, {ID: "127", Answer: "ok"}, {ID: "181", Answer: "ok"}, {ID: "128", Answer: "ok"}, {ID: "129", Answer: "ok"}}, nil, block("124..129,181=ok")},
		{"a run of two stays a list", []Chip{{ID: "7", Answer: "no"}, {ID: "8", Answer: "no"}, {ID: "10", Answer: "no"}}, nil, block("7,8,10=no")},
		{"padded ids range by number", []Chip{{ID: "01", Answer: "ok"}, {ID: "02", Answer: "ok"}, {ID: "03", Answer: "ok"}}, nil, block("01..03=ok")},
		{"a noted item has its own line", []Chip{{ID: "130", Answer: "ok", Note: " user note "}, {ID: "131", Answer: "ok"}}, nil, block("130=ok^user note", "131=ok")},
		{"note only", []Chip{{ID: "131", Note: "why?"}}, nil, block("131=^why?")},
		{"caret in a note", []Chip{{ID: "4", Answer: "a", Note: "x^y"}}, nil, block("4=a^x^y")},
		{"newline and backslash escape", []Chip{{ID: "5", Note: "line one\nC:\\path\\n here\r\nend"}}, nil, block(`5=^line one\nC:\\path\\n here\nend`)},
		{"topic notes last", []Chip{{ID: "9", Answer: "c"}}, []TopicNote{{Topic: "ship-gate", Note: "first\nsecond"}, {Topic: "Plain name", Note: "ok?"}}, block("9=c", `[ship-gate]^first\nsecond`, "[Plain name]^ok?")},
		{"topic notes alone", nil, []TopicNote{{Topic: "t", Note: "n"}}, block("[t]^n")},
	}
	for _, c := range cases {
		if got := AnswerBlock(c.chips, c.topics); got != c.want {
			t.Errorf("%s: AnswerBlock = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestParseAnswerBlockRoundTrips(t *testing.T) {
	chips := []Chip{
		{ID: "341", Answer: "a"}, {ID: "123", Answer: "b"}, {ID: "124", Answer: "ok"}, {ID: "125", Answer: "ok"}, {ID: "126", Answer: "ok"},
		{ID: "130", Answer: "ok", Note: "a^b"}, {ID: "131", Note: "multi\nline \\n literal \\ end"},
	}
	topics := []TopicNote{{Topic: "ship-gate", Note: "topic\nnote"}}
	text := Entry{Parts: []string{"user message here"}, Chips: chips, Topics: topics}.Text()
	lines, rest, ok := ParseAnswerBlock(text)
	if !ok {
		t.Fatalf("ParseAnswerBlock(%q) not ok", text)
	}
	want := []AnswerLine{
		{IDs: "341", Value: "a"}, {IDs: "123", Value: "b"}, {IDs: "124..126", Value: "ok"},
		{IDs: "130", Value: "ok", Note: "a^b"}, {IDs: "131", Note: "multi\nline \\n literal \\ end"},
		{Topic: "ship-gate", Note: "topic\nnote"},
	}
	if !reflect.DeepEqual(lines, want) {
		t.Errorf("lines = %+v\nwant  %+v", lines, want)
	}
	if rest != "user message here" {
		t.Errorf("rest = %q", rest)
	}
	if lines, rest, ok := ParseAnswerBlock(AnswerBlock(chips[:1], nil)); !ok || len(lines) != 1 || rest != "" {
		t.Errorf("block alone: %+v %q %v", lines, rest, ok)
	}
}

func TestParseAnswerBlockFallsBackOnMalformed(t *testing.T) {
	for _, text := range []string{
		"plain text",
		"text first\n\n<wb:answers>\n1=a\n</wb:answers>",
		"<wb:answers>\n1=a\n",                  // no close
		"<wb:answers>\n</wb:answers>\n\nx",     // no line
		"<wb:answers>\n1=a b\n</wb:answers>",   // bad value
		"<wb:answers>\nx=a\n</wb:answers>",     // bad ids
		"<wb:answers>\n1..=a\n</wb:answers>",   // bad range
		"<wb:answers>\n1=\n</wb:answers>",      // neither value nor note
		"<wb:answers>\n[t]note\n</wb:answers>", // topic without ^
		"<wb:answers>\n[]^n\n</wb:answers>",    // empty topic
		"<wb:answers>\n1=a\n</wb:answers>tail", // text glued to the close
		"<wb:answers>\n1=a\nfree words\n</wb:answers>",
	} {
		if lines, rest, ok := ParseAnswerBlock(text); ok || rest != text || lines != nil {
			t.Errorf("ParseAnswerBlock(%q) = %+v, %q, %v; want the text whole", text, lines, rest, ok)
		}
	}
}

func TestMergeLaysTopicNotesOver(t *testing.T) {
	base := Entry{Topics: []TopicNote{{Topic: "a", Note: "1"}, {Topic: "b", Note: "2"}}}
	add := Entry{Topics: []TopicNote{{Topic: "b", Note: "3"}, {Topic: "c", Note: "4"}}}
	got := Merge(base, add).Topics
	want := []TopicNote{{Topic: "a", Note: "1"}, {Topic: "b", Note: "3"}, {Topic: "c", Note: "4"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("merged topics = %+v, want %+v", got, want)
	}
	if got := Merge(Entry{Parts: []string{"x"}}, Entry{Parts: []string{"y"}}).Topics; got != nil {
		t.Fatalf("no topic notes merged to %+v", got)
	}
}
