package wsinbox

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func selectionBlock(t *testing.T, s Selection) string {
	t.Helper()
	block, err := SelectionBlock(s)
	if err != nil {
		t.Fatal(err)
	}
	return block
}

// oldWriterRemapFixture models only the pre-two-queue writer's flat marker
// rewrite. It is not a production path or installed-version acceptance.
func oldWriterRemapFixture(text string, renum map[int]int) string {
	var b strings.Builder
	last := 0
	ImageMarkers(text, func(start, end, n int) {
		if to, ok := renum[n]; ok && to != n {
			b.WriteString(text[last:start])
			b.WriteString(ImageMarker(to))
			last = end
		}
	})
	b.WriteString(text[last:])
	return b.String()
}

// Opaque text must remain byte-identical through the actual independent
// message lifecycle; the two-queue writer has no marker-remapping path.
func opaqueSelectionQueueFixture(t *testing.T, text string) string {
	t.Helper()
	dir := Dir(t.TempDir())
	now := time.Now()
	name, merged, err := Send(dir, Entry{Parts: []string{text}}, now)
	if err != nil || merged {
		t.Fatalf("opaque send = %v, %v", merged, err)
	}
	steered, err := Steer(dir, name)
	if err != nil || steered <= name {
		t.Fatalf("opaque promotion = %q, %v (was %q)", steered, err, name)
	}
	e, err := Cancel(dir, steered)
	if err != nil || e.SentAt != now.UnixMilli() {
		t.Fatalf("opaque cancel = %+v, %v", e, err)
	}
	if _, merged, err := Send(dir, e, now.Add(time.Second)); err != nil || merged {
		t.Fatalf("opaque resend = %v, %v", merged, err)
	}
	drained, err := Drain(dir)
	if err != nil || len(drained) != 1 || drained[0].Entry.SentAt != e.SentAt {
		t.Fatalf("opaque drain = %+v, %v", drained, err)
	}
	return drained[0].Entry.Text()
}

func TestSelectionBlockRoundTrip(t *testing.T) {
	s := Selection{
		Mentions: []Mention{
			{Topic: `a "topic" & <x>`, ID: "12", Entry: "e7", Text: " first [Image #1]\n<wb:comment>not a memo</wb:comment> &amp; "},
			{Topic: "a", Entry: "e8", Text: "second"},
		},
		Comment: " memo [Image #1]\n</wb:selection><wb:selection> &amp; ",
		Images:  []int{1, 3},
	}
	block := selectionBlock(t, s)
	want := "<wb:selection>\n" +
		`<wb:mention topic="a &quot;topic&quot; &amp; &lt;x&gt;" id="12" entry="e7"> first &#91;Image #1]` + "\n" +
		`&lt;wb:comment&gt;not a memo&lt;/wb:comment&gt; &amp;amp; </wb:mention>` + "\n" +
		`<wb:mention topic="a" entry="e8">second</wb:mention>` + "\n" +
		"<wb:comment> memo &#91;Image #1]\n&lt;/wb:selection&gt;&lt;wb:selection&gt; &amp;amp; </wb:comment>\n" +
		"[Image #1]\n[Image #3]\n</wb:selection>"
	if block != want {
		t.Fatalf("block =\n%s\nwant\n%s", block, want)
	}
	parts := ParseSelections("before\n\n" + block + "\n\nafter")
	if len(parts) != 3 || parts[0].Text != "before\n\n" || parts[2].Text != "\n\nafter" || parts[1].Text != block || !reflect.DeepEqual(parts[1].Selection, &s) {
		t.Fatalf("parsed parts = %+v", parts)
	}
	if got := ParseSelections(strings.ReplaceAll(block, "\n", "\r\n")); len(got) != 1 || got[0].Selection == nil {
		t.Fatalf("CRLF group = %+v", got)
	}
}

func TestSelectionsKeepPushesAndEmptyComment(t *testing.T) {
	for _, mentions := range [][]Mention{
		{{Topic: "t", Text: "single"}},
		{{Topic: "t", ID: "3", Entry: "e1", Text: "first"}, {Topic: "t", Entry: "e2", Text: "second"}},
	} {
		first := Selection{Mentions: mentions}
		second := Selection{Mentions: []Mention{{Topic: "u", Text: "other"}}, Comment: "different memo"}
		text := selectionBlock(t, first) + "\n\n" + selectionBlock(t, second)
		parts := ParseSelections(text)
		if len(parts) != 3 || !reflect.DeepEqual(parts[0].Selection, &first) || parts[1].Text != "\n\n" || !reflect.DeepEqual(parts[2].Selection, &second) {
			t.Fatalf("push association lost: %+v", parts)
		}
		if !strings.Contains(parts[0].Text, "<wb:comment></wb:comment>") {
			t.Fatal("empty comment must be emitted, not omitted")
		}
	}
}

func TestMalformedSelectionsStayWhole(t *testing.T) {
	m := `<wb:mention topic="t" entry="e1">source [Image #1]</wb:mention>`
	good := "<wb:selection>\n" + m + "\n<wb:comment>memo</wb:comment>\n</wb:selection>"
	for _, text := range []string{
		"<wb:selection>\n" + m + "\n</wb:selection>",
		"<wb:selection>\n<wb:comment>memo</wb:comment>\n</wb:selection>",
		strings.Replace(good, "</wb:comment>", "</wb:comment>\n<wb:comment>again</wb:comment>", 1),
		strings.Replace(good, "<wb:comment>", "<wb:comment extra=\"x\">", 1),
		strings.Replace(good, "</wb:comment>", "</wb:comment>\n"+m, 1),
		strings.Replace(good, "memo", "raw <b>memo</b>", 1),
		strings.Replace(good, "source", "raw <b>source</b>", 1),
		strings.Replace(good, "entry=\"e1\"", "entry=\"p1\"", 1),
		strings.Replace(good, "entry=\"e1\"", "entry=\"e1\" id=\"3\"", 1),
		strings.Replace(good, "<wb:selection>", "<wb:selection unknown=\"x\">", 1),
		"<wb:selection/>\n" + good + "\n</wb:selection>",
		"<wb:selection=bad>\n" + good + "\n</wb:selection>",
		strings.Replace(good, "</wb:selection>", "<unknown/>\n</wb:selection>", 1),
		strings.Replace(good, "</wb:selection>", "[Image #0]\n</wb:selection>", 1),
		strings.Replace(good, "</wb:selection>", "&#91;Image #1]\n</wb:selection>", 1),
		strings.Replace(good, "</wb:selection>", "[Image #1] [Image #2]\n</wb:selection>", 1),
		strings.Replace(good, "</wb:selection>", "[Image #1]\n[Image #1]\n</wb:selection>", 1),
		"<wb:selection>\n" + good + "\n</wb:selection>",
		"<wb:selection unknown=\"x\">\n" + good + "\n</wb:selection>",
		"<wb:selection>\n" + m + "\n" + good,
		"prefix" + good,
		good + "glued",
		strings.TrimSuffix(good, "</wb:selection>"),
		strings.TrimSuffix(good, ">") + " \n" + good,
		strings.Replace(good, "</wb:selection>", "</wb:selection unknown> ends", 1),
	} {
		parts := ParseSelections(text)
		if len(parts) != 1 || parts[0].Selection != nil || parts[0].Text != text {
			t.Errorf("malformed group partially extracted: %q -> %+v", text, parts)
		}
		// Failed groups cannot normalize into a different, apparently
		// valid source association, even when they contain image markers.
		if got, mentions := normalizeSelectionText(text); got != text || mentions != 0 {
			t.Errorf("malformed group rewritten: %q -> %q (%d mentions)", text, got, mentions)
		}
	}
	bad := strings.Replace(good, "memo", "<unknown/>", 1)
	parts := ParseSelections(bad + "\n\n" + good)
	if len(parts) != 2 || parts[0].Text != bad+"\n\n" || parts[0].Selection != nil || parts[1].Selection == nil {
		t.Fatalf("valid group following malformed group = %+v", parts)
	}
}

func TestSelectionEnvelopeContextOpacity(t *testing.T) {
	inner := "<wb:selection>\n" + `<wb:mention topic="t">literal [Image #1]</wb:mention>` + "\n<wb:comment>memo [Image #1]</wb:comment>\n[Image #1]\n</wb:selection>"
	for name, prefix := range map[string]string{
		"review comment":     "<wb:selection>\n<!-- </wb:selection>\n-->\n",
		"outer attribute":    "<wb:selection ignored=\"</wb:selection>\">\n",
		"double attribute":   "<wb:selection>\n<unknown ignored=\"</wb:selection>\">\n",
		"single attribute":   "<wb:selection>\n<unknown ignored='</wb:selection>'>\n",
		"CDATA":              "<wb:selection>\n<![CDATA[</wb:selection>]]>\n",
		"instruction":        "<wb:selection>\n<?unknown </wb:selection> ?>\n",
		"unterminated quote": "<wb:selection ignored=\"</wb:selection>\n",
	} {
		t.Run(name, func(t *testing.T) {
			text := prefix + inner + "\n</wb:selection>"
			parts := ParseSelections(text)
			if len(parts) != 1 || parts[0].Selection != nil || parts[0].Text != text {
				t.Errorf("forbidden nested group extracted: %+v", parts)
			}
			if got, mentions := normalizeSelectionText(text); got != text || mentions != 0 {
				t.Errorf("opaque normalization = %q, %d; want exact text, zero mentions", got, mentions)
			}
			if got := opaqueSelectionQueueFixture(t, text); got != text {
				t.Errorf("opaque queue rewrite = %q; want %q", got, text)
			}
			// With no attachment, ordinary failed-envelope text must be a
			// valid zero-image send, not an extracted dangling reference.
			dir := Dir(t.TempDir())
			if _, _, err := Send(dir, Entry{Parts: []string{text}}, time.Now()); err != nil {
				t.Errorf("zero-image send rejected ordinary failed envelope: %v", err)
			} else if held, err := Held(dir); err != nil || len(held) != 1 || held[0].Entry.Text() != text {
				t.Errorf("zero-image send changed text: %+v, %v", held, err)
			}
			if name != "unterminated quote" {
				// A complete failed envelope must not swallow an independent
				// valid sibling after its actual close.
				sibling := selectionBlock(t, Selection{Mentions: []Mention{{Topic: "u", Text: "independent"}}})
				parts = ParseSelections(text + "\n\n" + sibling)
				if len(parts) != 2 || parts[0].Text != text+"\n\n" || parts[0].Selection != nil || parts[1].Selection == nil {
					t.Errorf("failed envelope lost its boundary or sibling: %+v", parts)
				}
			}
		})
	}
}

func TestSelectionOrdinaryPrefixDiscovery(t *testing.T) {
	failed := "<wb:selection>\n<!-- </wb:selection>\n-->\n<wb:selection>\n" +
		`<wb:mention topic="t">literal [Image #1]</wb:mention>` + "\n<wb:comment>memo [Image #1]</wb:comment>\n[Image #1]\n</wb:selection>\n</wb:selection>"
	s := Selection{Mentions: []Mention{{Topic: "u", Text: "source [Image #1]"}}, Comment: "memo [Image #1]", Images: []int{1}}
	valid := selectionBlock(t, s)
	for _, prefix := range []string{"1 < 2\n\n", "a<b\n\n", "ordinary < unclosed '\n\n"} {
		t.Run(prefix, func(t *testing.T) {
			text := prefix + failed
			parts := ParseSelections(text)
			if len(parts) != 1 || parts[0].Selection != nil || parts[0].Text != text {
				t.Errorf("ordinary prefix exposed a nested group: %+v", parts)
			}
			if got, mentions := normalizeSelectionText(text); got != text || mentions != 0 {
				t.Errorf("prefixed failed-envelope normalization = %q, %d", got, mentions)
			}
			if got := opaqueSelectionQueueFixture(t, text); got != text {
				t.Errorf("ordinary prefix disabled failed-envelope queue opacity: %q", got)
			}
			// Ordinary text must not suppress a canonical group or a valid
			// sibling after the failed envelope's actual close.
			for _, before := range []string{prefix, text + "\n\n"} {
				combined := before + valid
				parts = ParseSelections(combined)
				if len(parts) != 2 || parts[0].Text != before || parts[0].Selection != nil || !reflect.DeepEqual(parts[1].Selection, &s) {
					t.Errorf("ordinary prefix hid a valid group/sibling: %+v", parts)
				}
				if got, mentions := normalizeSelectionText(combined); got != combined || mentions != 1 {
					t.Errorf("prefixed canonical normalization = %q, %d", got, mentions)
				}
				if _, err := Normalize(Entry{Parts: []string{combined}}); !errors.Is(err, ErrInvalid) {
					t.Errorf("ordinary prefix bypassed attached-image validation: %v", err)
				}
				out, err := Normalize(Entry{Parts: []string{combined}, Images: []string{"/one.png"}, ImageNums: []int{1}})
				if err != nil || len(out.Parts) != 1 || out.Parts[0] != combined || !reflect.DeepEqual(out.ImageNums, []int{1}) {
					t.Errorf("prefixed group/sibling changed content or numbering: %+v, %v", out, err)
				}
			}
		})
	}
	// The same non-tag '<' inside an envelope is still ambiguous content:
	// do not recover the valid-looking nested group from that failed source.
	inside := "<wb:selection>\n1 < 2\n\n" + valid + "\n</wb:selection>"
	if parts := ParseSelections(inside); len(parts) != 1 || parts[0].Text != inside || parts[0].Selection != nil {
		t.Errorf("inside ambiguity exposed a nested group: %+v", parts)
	}
	if got, mentions := normalizeSelectionText(inside); got != inside || mentions != 0 {
		t.Errorf("inside ambiguity was normalized: %q, %d", got, mentions)
	}
	if got := opaqueSelectionQueueFixture(t, inside); got != inside {
		t.Errorf("inside ambiguity was rewritten by a queue operation: %q", got)
	}
}

func TestSelectionBlockRejectsInvalidStructuredInput(t *testing.T) {
	for _, s := range []Selection{
		{},
		{Mentions: []Mention{{Text: "x"}}},
		{Mentions: []Mention{{Topic: "t\nx", Text: "x"}}},
		{Mentions: []Mention{{Topic: strings.Repeat("t", maxTopicLen+1), Text: "x"}}},
		{Mentions: []Mention{{Topic: "t", ID: "x", Text: "x"}}},
		{Mentions: []Mention{{Topic: "t", Entry: "p1", Text: "x"}}},
		{Mentions: []Mention{{Topic: "t", Text: " "}}},
		{Mentions: []Mention{{Topic: "t", Text: "x"}}, Images: []int{0}},
		{Mentions: []Mention{{Topic: "t", Text: "x"}}, Images: []int{1, 1}},
	} {
		if _, err := SelectionBlock(s); !errors.Is(err, ErrInvalid) {
			t.Errorf("invalid selection accepted: %+v: %v", s, err)
		}
	}
}

func TestNormalizeSelectionsUsesOnlyExistingPartsAndLimits(t *testing.T) {
	s := Selection{Mentions: []Mention{{Topic: "t", Entry: "e1", Text: " exact source "}}, Comment: " memo ", Images: []int{2}}
	block := selectionBlock(t, s)
	e, err := Normalize(Entry{Parts: []string{"before\n" + block + "\nafter"}, Images: []string{"/a.png"}, ImageNums: []int{2}})
	if err != nil || e.Parts[0] != "before\n"+block+"\nafter" || e.Mentions != nil || SchemaVersion != 1 {
		t.Fatalf("normalized selection = %+v, %v", e, err)
	}
	raw, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	for key := range fields {
		if key != "schemaVersion" && key != "parts" && key != "images" && key != "imageNums" {
			t.Fatalf("new persisted field %q in %s", key, raw)
		}
	}
	if _, err := Normalize(Entry{Parts: []string{block}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("dangling owned image reference: %v", err)
	}
	s.Images = nil
	s.Comment = strings.Repeat("x", maxTextBytes)
	if _, err := Normalize(Entry{Parts: []string{selectionBlock(t, s)}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("selection bypassed text limit: %v", err)
	}
	s.Comment = ""
	s.Mentions = make([]Mention, maxMentions)
	for i := range s.Mentions {
		s.Mentions[i] = Mention{Topic: "t", Text: "x"}
	}
	if _, err := Normalize(Entry{Parts: []string{selectionBlock(t, s)}, Mentions: []Mention{{Topic: "t", Text: "legacy"}}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("grouped plus legacy mentions bypassed count limit: %v", err)
	}
	malformed := "<wb:selection>\n<unknown/>\n</wb:selection>"
	if e, err := Normalize(Entry{Parts: []string{malformed}}); err != nil || e.Parts[0] != malformed {
		t.Fatalf("malformed selection must remain ordinary text: %+v, %v", e, err)
	}
}

func TestSelectionsSurviveIndependentMessageQueues(t *testing.T) {
	dir := Dir(t.TempDir())
	a := attach(t, dir, "0123456789abcdef-1.png")
	b := attach(t, dir, "fedcba9876543210-1.png")
	first := Selection{Mentions: []Mention{{Topic: "t", ID: "3", Entry: "e1", Text: "first [Image #1]"}, {Topic: "t", Entry: "e2", Text: "second"}}, Comment: "first memo", Images: []int{1}}
	second := Selection{Mentions: []Mention{{Topic: "u", Entry: "e9", Text: "other [Image #1]"}}, Images: []int{1}}
	one, two := "typed\n\n"+selectionBlock(t, first), selectionBlock(t, second)
	secondInSameMessage := second
	secondInSameMessage.Images = []int{2}
	multi := one + "\n\n" + selectionBlock(t, secondInSameMessage)
	now := time.Now()
	entries := []Entry{
		{Parts: []string{one}, Images: []string{a}, ImageNums: []int{1}},
		{Parts: []string{two}, Images: []string{b}, ImageNums: []int{1}},
		{Parts: []string{multi}, Images: []string{a, b}, ImageNums: []int{1, 2}},
	}
	var names []string
	for i, entry := range entries {
		name, merged, err := Send(dir, entry, now.Add(time.Duration(i)*time.Millisecond))
		if err != nil || merged {
			t.Fatalf("independent send %d = %v, %v", i, merged, err)
		}
		entry.SentAt = now.Add(time.Duration(i) * time.Millisecond).UnixMilli()
		entries[i] = entry
		names = append(names, name)
		stored, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || strings.Contains(string(stored), `"selections"`) || strings.Contains(string(stored), `"comment"`) {
			t.Fatalf("stored representation = %s, %v", stored, err)
		}
	}
	check := func(e Entry, want Entry) {
		t.Helper()
		if !reflect.DeepEqual(e.Parts, want.Parts) || e.Mentions != nil || !reflect.DeepEqual(e.Images, want.Images) || !reflect.DeepEqual(e.ImageNums, want.ImageNums) || e.SentAt != want.SentAt || e.Text() != want.Text() {
			t.Fatalf("message ownership changed: %+v, want %+v", e, want)
		}
	}
	held, err := Held(dir)
	if err != nil || len(held) != 3 {
		t.Fatalf("held = %+v, %v", held, err)
	}
	for i, r := range held {
		check(r.Entry, entries[i])
	}
	cancelled, err := Cancel(dir, names[1])
	if err != nil {
		t.Fatal(err)
	}
	check(cancelled, entries[1])
	follow, merged, err := Send(dir, cancelled, now.Add(time.Second))
	if err != nil || merged || follow == names[1] {
		t.Fatalf("cancel/resend = %q, %v, %v", follow, merged, err)
	}
	// Promote newer multi-group text first, then the older first message.
	// Fresh stamps put the latter at the steering tail, ahead of follow-ups.
	steerMulti, err := Steer(dir, names[2])
	if err != nil || steerMulti <= follow {
		t.Fatalf("multi promotion = %q, %v", steerMulti, err)
	}
	steerFirst, err := Steer(dir, names[0])
	if err != nil || steerFirst <= steerMulti {
		t.Fatalf("first promotion = %q, %v", steerFirst, err)
	}
	held, err = Held(dir)
	if err != nil || len(held) != 3 || held[0].Name != steerMulti || held[1].Name != steerFirst || held[2].Name != follow {
		t.Fatalf("steering-tail order = %+v, %v", held, err)
	}
	wantOrder := []Entry{entries[2], entries[0], entries[1]}
	for i, r := range held {
		check(r.Entry, wantOrder[i])
	}
	taken, _, err := Interrupt(dir, now.Add(2*time.Second))
	if err != nil || len(taken) != 3 {
		t.Fatalf("interrupt = %+v, %v", taken, err)
	}
	for i, r := range taken {
		check(r.Entry, wantOrder[i])
	}
	// Reinsert the follow-up first; a full drain still takes steering first.
	for _, i := range []int{2, 0, 1} {
		if _, merged, err := Send(dir, taken[i].Entry, now.Add(3*time.Second)); err != nil || merged {
			t.Fatalf("interrupt resend = %v, %v", merged, err)
		}
	}
	drained, err := Drain(dir)
	if err != nil || len(drained) != 3 {
		t.Fatalf("full drain = %+v, %v", drained, err)
	}
	for i, r := range drained {
		check(r.Entry, wantOrder[i])
	}
	if records, err := Drained(dir); err != nil || len(records) != 3 {
		t.Fatalf("drained history = %+v, %v", records, err)
	}
}

func TestSelectionNumericEscapeBytesAndOneDecode(t *testing.T) {
	literal := "&#91; | &amp;#91; | & | < | [Image #1]"
	encoded := "&amp;#91; | &amp;amp;#91; | &amp; | &lt; | &#91;Image #1]"
	s := Selection{Mentions: []Mention{{Topic: "t", ID: "3", Entry: "e1", Text: literal}}, Comment: literal, Images: []int{1}}
	want := "<wb:selection>\n" + `<wb:mention topic="t" id="3" entry="e1">` + encoded + "</wb:mention>\n" +
		"<wb:comment>" + encoded + "</wb:comment>\n[Image #1]\n</wb:selection>"
	block := selectionBlock(t, s)
	if block != want {
		t.Fatalf("serialized bytes = %q, want %q", block, want)
	}
	parts := ParseSelections(block)
	if len(parts) != 1 || !reflect.DeepEqual(parts[0].Selection, &s) || selectionBlock(t, *parts[0].Selection) != block {
		t.Fatalf("one decode or canonical round trip changed content: %+v", parts)
	}
	// The old writer fixture's flat remapping touches only the tail.
	oldText := oldWriterRemapFixture(block, map[int]int{1: 4})
	if oldText != strings.Replace(want, "\n[Image #1]\n", "\n[Image #4]\n", 1) {
		t.Fatalf("old remapper changed escaped content: %q", oldText)
	}
	s.Images = []int{4}
	if got := ParseSelections(oldText); len(got) != 1 || !reflect.DeepEqual(got[0].Selection, &s) {
		t.Fatalf("old remapper round trip = %+v", got)
	}
}

func TestSelectionEscapesSurviveTheOldFlatMarkerRemapper(t *testing.T) {
	s := Selection{
		Mentions: []Mention{{Topic: "[Image #1] & &#91;", Text: "[Image #1] &#91;Image #1] &amp;#91;Image #1]"}},
		Comment:  "[Image #1] &#91;Image #1]",
		Images:   []int{1},
	}
	block := selectionBlock(t, s)
	var markers []int
	ImageMarkers(block, func(_, _, n int) { markers = append(markers, n) })
	if !reflect.DeepEqual(markers, []int{1}) {
		t.Fatalf("old writer can see source/memo markers: %v in %q", markers, block)
	}
	// This fixture models the pre-two-queue writer, not the current runtime
	// or an installed mixed-version session. No unknown field must survive.
	oldWriterText := oldWriterRemapFixture(block, map[int]int{1: 4})
	parts := ParseSelections(oldWriterText)
	s.Images = []int{4}
	if len(parts) != 1 || !reflect.DeepEqual(parts[0].Selection, &s) {
		t.Fatalf("old flat rewrite changed literal source/memo: %+v", parts)
	}
	// Legacy mentions do not acquire the new group's numeric decoding.
	legacy := MentionBlock([]Mention{{Topic: "t", Text: "&#91;Image #1]"}})
	mentions, rest := ParseMentions(legacy)
	if len(mentions) != 1 || mentions[0].Text != "&#91;Image #1]" || rest != "" {
		t.Fatalf("legacy entity behavior changed: %+v %q", mentions, rest)
	}
}

func FuzzParseSelectionsLossless(f *testing.F) {
	for _, text := range []string{
		"plain [Image #1]",
		"<wb:selection>\n<wb:mention topic=\"t\">&#91;Image #1]</wb:mention>\n<wb:comment>&amp;#91;</wb:comment>\n[Image #1]\n</wb:selection>",
		"<wb:selection/>\n<wb:selection>\n</wb:selection>",
		"<wb:selection>\n</wb:selection \n<wb:selection>\n</wb:selection>",
	} {
		f.Add(text)
	}
	f.Fuzz(func(t *testing.T, text string) {
		var joined strings.Builder
		for _, part := range ParseSelections(text) {
			joined.WriteString(part.Text)
			if part.Selection != nil {
				block := selectionBlock(t, *part.Selection)
				parsed := ParseSelections(block)
				if len(parsed) != 1 || !reflect.DeepEqual(parsed[0].Selection, part.Selection) {
					t.Fatalf("group decode was not stable: %+v -> %+v", part.Selection, parsed)
				}
			}
		}
		if joined.String() != text {
			t.Fatalf("parser changed fallback bytes: %q -> %q", text, joined.String())
		}
	})
}

func TestSelectionLegacyTextFallback(t *testing.T) {
	legacy := `<wb:mention topic="t" id="3">old source</wb:mention>` + "\n\nold typed memo [Image #1]"
	parts := ParseSelections(legacy)
	if len(parts) != 1 || parts[0].Text != legacy || parts[0].Selection != nil {
		t.Fatalf("legacy transcript reinterpreted: %+v", parts)
	}
	block := selectionBlock(t, Selection{Mentions: []Mention{{Topic: "t", Text: "source"}}, Comment: "memo"})
	if mentions, rest := ParseMentions(block); mentions != nil || rest != block {
		t.Fatal("old leading-mention reader must leave the self-describing block as plain text")
	}
	// The pre-selection entry shape is unchanged: an old known-field
	// reader/writer retains the entire group in Parts, not unknown fields.
	type oldEntry struct {
		SchemaVersion int      `json:"schemaVersion"`
		Parts         []string `json:"parts,omitempty"`
		Images        []string `json:"images,omitempty"`
		ImageNums     []int    `json:"imageNums,omitempty"`
	}
	raw, err := json.Marshal(Entry{SchemaVersion: SchemaVersion, Parts: []string{block}})
	if err != nil {
		t.Fatal(err)
	}
	var old oldEntry
	if err := json.Unmarshal(raw, &old); err != nil {
		t.Fatal(err)
	}
	raw, err = json.Marshal(old)
	if err != nil {
		t.Fatal(err)
	}
	var restored Entry
	if err := json.Unmarshal(raw, &restored); err != nil || restored.Text() != block {
		t.Fatalf("old-shape text round trip = %q, %v", restored.Text(), err)
	}
}
