package wswhiteboard

import (
	"encoding/json"
	"strings"
	"testing"
)

// A labelled group creates a stemmed topic; later groups address it by its
// stem alone, and a self-closing labelled group renames its label.
func TestStemmedTopicCreateAddressRename(t *testing.T) {
	st := &State{}
	mustApply(t, st, `<wb:topic name="ship-gate" label="0.48.16 출시 게이트"><wb:add class="technical"><wb:title>one</wb:title></wb:add></wb:topic>`)
	if got := item(t, st, 1).Topic; got != "ship-gate" || st.Labels["ship-gate"] != "0.48.16 출시 게이트" {
		t.Fatalf("topic %q, labels %v", got, st.Labels)
	}
	mustApply(t, st, `<wb:topic name="ship-gate"><wb:add class="technical"><wb:title>two</wb:title></wb:add><wb:body>notes</wb:body></wb:topic>`)
	if item(t, st, 2).Topic != "ship-gate" || st.TopicBody("ship-gate") != "notes" {
		t.Fatalf("addressed by stem: %+v", st)
	}
	mustApply(t, st, `<wb:topic name="ship-gate" label="Release gate"/>`)
	if st.Labels["ship-gate"] != "Release gate" || st.TopicLabel("ship-gate") != "Release gate" {
		t.Fatalf("rename: %v", st.Labels)
	}
	// The label addresses the topic too (a stem first, then a stemless key,
	// then a label).
	mustApply(t, st, `<wb:topic name="Release gate"><wb:close result="shipped"/></wb:topic>`)
	if !st.topicClosed("ship-gate") || st.CloseResults["ship-gate"] != "shipped" || item(t, st, 1).Status != StatusWithdrawn {
		t.Fatalf("addressed by label: %+v", st)
	}
	if len(st.Items) != 2 || st.topicKnown("Release gate") {
		t.Fatalf("a label must never become a key of its own: %+v", st.Items)
	}
}

// A labelled group whose stem is unknown and whose label is a stemless
// topic's key adopts that topic: items, body, close state, and close result
// move to the stem, and the old title still addresses it.
func TestStemAdoptsAStemlessTopicByLabel(t *testing.T) {
	st := &State{}
	mustApply(t, st, `<wb:topic name="Long old title"><wb:add class="technical"><wb:title>one</wb:title></wb:add><wb:body>b</wb:body><wb:close result="done"/></wb:topic>`)
	mustApply(t, st, `<wb:topic name="old" label="Long old title"/>`)
	if item(t, st, 1).Topic != "old" || st.TopicBody("old") != "b" || !st.topicClosed("old") || st.CloseResults["old"] != "done" {
		t.Fatalf("adoption did not move the topic: %+v", st)
	}
	if st.topicKnown("Long old title") || st.topicClosed("Long old title") || st.CloseResults["Long old title"] != "" {
		t.Fatalf("the old key is still in use: %+v", st)
	}
	mustApply(t, st, `<wb:topic name="Long old title"><wb:add class="technical"><wb:title>two</wb:title></wb:add></wb:topic>`)
	if item(t, st, 2).Topic != "old" || st.topicClosed("old") {
		t.Fatalf("the old title must resolve to the stem: %+v", st)
	}
}

// A stemless topic whose own key fits the stem pattern takes it as its stem
// when addressed with a label, without a key change.
func TestStemAdoptsAStemlessTopicInPlace(t *testing.T) {
	st := &State{}
	mustApply(t, st, addXML(ClassTechnical, "rollout", "one"))
	mustApply(t, st, `<wb:topic name="rollout" label="Rollout plan"/>`)
	if item(t, st, 1).Topic != "rollout" || st.Labels["rollout"] != "Rollout plan" {
		t.Fatalf("in-place adoption: %+v", st)
	}
}

func TestStemRefusals(t *testing.T) {
	st := &State{}
	mustApply(t, st, `<wb:topic name="alpha" label="Alpha"><wb:add class="technical"><wb:title>a</wb:title></wb:add></wb:topic>
<wb:topic name="beta" label="Beta"><wb:add class="technical"><wb:title>b</wb:title></wb:add></wb:topic>`)
	mustApply(t, st, addXML(ClassTechnical, "Plain title", "c"))
	for name, tc := range map[string]struct {
		in   string
		want string
	}{
		"empty group creating a topic":   {`<wb:topic name="fresh" label="Fresh"/>`, `<wb:topic> (line 1): holds no commands, and "fresh" names no topic to rename or adopt`},
		"malformed stem":                 {`<wb:topic name="Not A Stem" label="x"><wb:body>b</wb:body></wb:topic>`, `<wb:topic> (line 1) name: with label, name is a stem`},
		"long stem":                      {`<wb:topic name="` + strings.Repeat("a", 41) + `" label="x"><wb:body>b</wb:body></wb:topic>`, "with label, name is a stem"},
		"blank label":                    {`<wb:topic name="gamma" label=" "><wb:body>b</wb:body></wb:topic>`, `<wb:topic> (line 1) label: must be non-empty`},
		"label equal to another label":   {`<wb:topic name="gamma" label="Alpha"><wb:body>b</wb:body></wb:topic>`, `<wb:topic> (line 1) label: "Alpha" is already another topic's name`},
		"label equal to another stem":    {`<wb:topic name="alpha" label="beta"/>`, `"beta" is already another topic's name`},
		"rename onto a stemless key":     {`<wb:topic name="alpha" label="Plain title"/>`, `"Plain title" is already another topic's name`},
		"stem equal to another label":    {`<wb:topic name="gamma" label="delta-x"><wb:body>b</wb:body></wb:topic><wb:topic name="delta-x" label="D"><wb:body>b</wb:body></wb:topic>`, `<wb:topic> (line 1) name: "delta-x" is topic gamma's label`},
		"label adopting a stemmed topic": {`<wb:topic name="gamma" label="Beta"/>`, `"Beta" is already another topic's name`},
	} {
		problems := refused(t, st, tc.in)
		if !hasProblem(problems, tc.want) {
			t.Errorf("%s: problems %q, want %q", name, problems, tc.want)
		}
	}
	// A stem given as an unlabelled stemless key is no collision: it
	// addresses the stemmed topic.
	mustApply(t, st, addXML(ClassTechnical, "Alpha", "d"))
	if item(t, st, 4).Topic != "alpha" {
		t.Fatalf("label as name: %q", item(t, st, 4).Topic)
	}
}

// A label goes when its topic disappears, so the stem is free for a new
// topic; archive eviction of the topic's last items drops it too.
func TestLabelDroppedWhenItsTopicDisappears(t *testing.T) {
	st := &State{}
	mustApply(t, st, `<wb:topic name="tmp" label="Temporary"><wb:body>only a body</wb:body></wb:topic>`)
	mustApply(t, st, `<wb:topic name="tmp"><wb:body/></wb:topic>`)
	if st.Labels != nil {
		t.Fatalf("label kept after its topic went: %v", st.Labels)
	}
	// "Temporary" is free again: a new stemless topic takes it.
	mustApply(t, st, topicXML("Temporary", "x"))
	if st.TopicBody("Temporary") != "x" {
		t.Fatalf("freed label: %+v", st.Topics)
	}

	st = &State{}
	mustApply(t, st, `<wb:topic name="old" label="Old"><wb:add class="technical"><wb:title>first</wb:title></wb:add></wb:topic>`)
	mustApply(t, st, `<wb:answer id="1" value="ok">ok</wb:answer>`)
	settleItems(t, st, "other", ArchiveCap)
	if st.topicKnown("old") || st.stemmed("old") {
		t.Fatalf("eviction left topic old: items %d, labels %v", len(st.Items), st.Labels)
	}
}

// The query topic filter resolves like a group, and the topic line names a
// stemmed topic by stem and label, a stemless one by its label.
func TestQueryStemmedTopics(t *testing.T) {
	st := &State{}
	mustApply(t, st, `<wb:topic name="ship-gate" label="Ship <code>gate</code>"><wb:add class="technical"><wb:title>one</wb:title></wb:add></wb:topic>`)
	mustApply(t, st, addXML(ClassTechnical, "Plain title", "two"))
	header := `topic ship-gate "Ship ` + "`gate`" + `" [open] items: open 1; ` + changedAt + "\n"
	for _, name := range []string{"ship-gate", "Ship <code>gate</code>"} {
		if text := queryText(t, st, `{"topic":"`+strings.ReplaceAll(name, `"`, `\"`)+`","depth":"items"}`); text != header+"  (01) [open] [technical] one\n" {
			t.Fatalf("topic %q:\n%s", name, text)
		}
	}
	wantText(t, "stemless header", queryText(t, st, ``), []string{header, `topic "Plain title" [open] items: open 1; `}, nil)
}

// A store written before stems loads with every topic stemless, addressed
// by its title, and writes no labels field until a label is set.
func TestPreStemStoreLoads(t *testing.T) {
	stored := `{"counter":2,"seq":3,"items":[{"id":1,"class":"technical","topic":"Old topic","title":"kept","status":"open","changed":3},{"id":2,"class":"technical","topic":"Done topic","title":"x","status":"withdrawn","changed":3,"settled":1}],"closed_topics":["Done topic"],"close_results":{"Done topic":"over"},"topics":[{"name":"Old topic","body":"b","changed":3}]}`
	var st State
	if err := json.Unmarshal([]byte(stored), &st); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	mustApply(t, &st, `<wb:topic name="Old topic"><wb:add class="technical"><wb:title>next</wb:title></wb:add></wb:topic><wb:topic name="Done topic"><wb:close result="still over"/></wb:topic>`)
	if item(t, &st, 3).Topic != "Old topic" || st.CloseResults["Done topic"] != "still over" || st.TopicLabel("Old topic") != "Old topic" {
		t.Fatalf("pre-stem store: %+v", st)
	}
	if strings.Contains(string(mustMarshal(t, &st)), `"labels"`) {
		t.Fatalf("a store with no stems must not write labels")
	}
	wantText(t, "pre-stem query", queryText(t, &st, `{"topic":"Done topic"}`), []string{`topic "Done topic" [closed]`, "(02) [withdrawn]"}, nil)
}
