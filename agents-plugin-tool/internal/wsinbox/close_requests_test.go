package wsinbox

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestCloseRequestsNormalizeAndEnvelope(t *testing.T) {
	want := []CloseRequest{{Topic: `a"<&>`, Reason: `why </wb:close-request> & </ws:user>`}, {Topic: "other", Reason: ""}}
	e, err := Normalize(Entry{CloseRequests: []CloseRequest{{Topic: ` a"<&> `, Reason: "old"}, want[1], want[0]}, Mentions: []Mention{{Topic: "a", Text: "context"}}, Parts: []string{"words"}})
	if err != nil || !reflect.DeepEqual(e.CloseRequests, want) {
		t.Fatalf("normalize = %+v, %v", e, err)
	}
	blocks, ok := ParseUserMessages(e.Text())
	if !ok || len(blocks) != 1 || blocks[0].At == "" {
		t.Fatalf("envelope = %q", e.Text())
	}
	mentions, rest := ParseMentions(blocks[0].Text)
	got, rest := ParseCloseRequests(rest)
	if len(mentions) != 1 || !reflect.DeepEqual(got, want) || rest != "words" {
		t.Fatalf("parsed = %+v, %+v, %q", mentions, got, rest)
	}
	if !strings.Contains(blocks[0].Text, `topic="a&quot;&lt;&amp;&gt;">why &lt;/wb:close-request&gt; &amp; &lt;/ws:user&gt;`) {
		t.Fatalf("unsafe escaping: %s", blocks[0].Text)
	}
	if _, err := Normalize(Entry{CloseRequests: []CloseRequest{{Topic: "only"}}}); err != nil {
		t.Fatalf("empty-reason request is content: %v", err)
	}
	for _, e := range []Entry{
		{CloseRequests: []CloseRequest{{Topic: " "}}},
		{CloseRequests: []CloseRequest{{Topic: "a\nb"}}},
		{CloseRequests: []CloseRequest{{Topic: strings.Repeat("a", maxTopicLen+1)}}},
		{CloseRequests: []CloseRequest{{Topic: "a", Reason: strings.Repeat("x", maxNoteBytes+1)}}},
		{CloseRequests: make([]CloseRequest, maxTopics+1)},
		{Parts: []string{strings.Repeat("x", maxTextBytes)}, CloseRequests: []CloseRequest{{Topic: "a", Reason: "x"}}},
		{CloseRequests: []CloseRequest{{Topic: "a"}}, Command: &Command{Name: "compact"}},
	} {
		if _, err := Normalize(e); !errors.Is(err, ErrInvalid) {
			t.Errorf("invalid entry accepted: %+v (%v)", e, err)
		}
	}
}

func TestCloseRequestsParserPreservesMalformedAndNonLeading(t *testing.T) {
	for _, text := range []string{
		`<wb:close-request topic="">x</wb:close-request>`,
		`<wb:close-request topic="a">missing end`,
		`<wb:close-request topic="a"><script></wb:close-request>`,
		`<wb:close-request topic="a">x</wb:close-request>suffix`,
		`words <wb:close-request topic="a"></wb:close-request>`,
	} {
		got, rest := ParseCloseRequests(text)
		if len(got) != 0 || rest != text {
			t.Errorf("parser changed malformed/plain text: %+v, %q", got, rest)
		}
	}
}

func TestCloseRequestsSurviveInboxRewrites(t *testing.T) {
	for _, steerDrain := range []bool{false, true} {
		dir := Dir(t.TempDir())
		want := []CloseRequest{{Topic: "closed-or-gone"}, {Topic: "open", Reason: "done"}}
		name, _, err := Send(dir, Entry{CloseRequests: want}, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		name, err = Steer(dir, name)
		if err != nil {
			t.Fatal(err)
		}
		back, err := Cancel(dir, name)
		if err != nil || !reflect.DeepEqual(back.CloseRequests, want) {
			t.Fatalf("cancel: %+v, %v", back, err)
		}
		name, _, err = Send(dir, back, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if _, err = Steer(dir, name); err != nil {
			t.Fatal(err)
		}
		var drained []Record
		if steerDrain {
			drained, err = DrainSteer(dir)
		} else {
			drained, err = Drain(dir)
		}
		if err != nil || len(drained) != 1 {
			t.Fatalf("drain: %+v, %v", drained, err)
		}
		saved, err := Drained(dir)
		if err != nil || len(saved) != 1 {
			t.Fatalf("saved: %+v, %v", saved, err)
		}
		for _, r := range append(drained, saved...) {
			if !reflect.DeepEqual(r.Entry.CloseRequests, want) {
				t.Fatalf("lost requests: %+v", r)
			}
		}
	}
}
