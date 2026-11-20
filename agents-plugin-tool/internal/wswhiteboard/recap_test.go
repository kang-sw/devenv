package wswhiteboard

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// Legacy recap fixtures are snapshots, not commands. The old set/replace/
// clear/reopen command tests retired with wb:recap; entries_test.go covers
// the replacement agent log contract. Keep old storage/read coverage here.
func legacyRecap(st *State, name, recap string) {
	d := st.topicDoc(name)
	d.Recap, d.RecapChanged, d.RecapAt = recap, st.Seq, testNow.Format(time.RFC3339)
}

func TestLegacyRecapStorageAndBodyIndependence(t *testing.T) {
	var st State
	const raw = `{"seq":4,"topics":[{"name":"t","body":"alpha","changed":2,"changed_at":"2026-11-04T08:00:00Z","recap":"legacy &amp; {{1}}","recap_changed":4,"recap_at":"2026-11-04T09:00:00Z"},{"name":"solo","body":"","recap":"waiting","recap_changed":3}]}`
	if err := json.Unmarshal([]byte(raw), &st); err != nil {
		t.Fatal(err)
	}
	before := topicEntry(t, &st, "t")
	mustApply(t, &st, topicXML("t", "beta")+topicXML("t", "alpha"))
	got := topicEntry(t, &st, "t")
	if got.Recap != before.Recap || got.RecapChanged != before.RecapChanged || got.RecapAt != before.RecapAt || got.Changed != before.Changed || got.ChangedAt != before.ChangedAt {
		t.Fatalf("body revert changed legacy recap or body marks: %+v", got)
	}
	mustApply(t, &st, `<wb:topic name="t"><wb:body/></wb:topic><wb:topic name="solo"><wb:close result="done"/></wb:topic>`)
	if st.TopicBody("t") != "" || st.TopicRecap("t") != before.Recap || !st.topicKnown("solo") || !st.topicClosed("solo") {
		t.Fatalf("legacy recap lost: %+v", st)
	}
	var loaded State
	if err := json.Unmarshal(mustMarshal(t, &st), &loaded); err != nil {
		t.Fatal(err)
	}
	if loaded.TopicRecap("t") != before.Recap || topicEntry(t, &loaded, "t").RecapAt != before.RecapAt || loaded.TopicRecap("solo") != "waiting" {
		t.Fatalf("legacy recap round-trip: %+v", loaded.Topics)
	}
	wantProblems(t, refused(t, &st, `<wb:topic name="t"><wb:recap>new</wb:recap></wb:topic>`), "unknown command")
}

func recapBlock(t *testing.T, out, name string) string {
	t.Helper()
	start := strings.Index(out, `data-topic="`+name+`"`)
	if start < 0 {
		t.Fatalf("topic %q missing", name)
	}
	block := out[start:]
	return block[:strings.Index(block, "</section>")]
}

func TestLegacyRecapRemainsReadableOnBothLayouts(t *testing.T) {
	st := &State{}
	mustApply(t, st, addXML(ClassTechnical, "open", "x"))
	legacyRecap(st, "open", "**READY** · see ![shot](shot.png) <u>kept</u><script>alert(1)</script>")
	legacyRecap(st, "shut", "was moving")
	legacyRecap(st, "lone", "only a recap")
	mustApply(t, st, `<wb:topic name="shut"><wb:close/></wb:topic><wb:topic name="lone"><wb:close/></wb:topic>`)
	// Unconverted snapshots have no panel entry to supersede the old recap.
	st.topicDoc("open").Body = "the body"
	st.topicDoc("shut").Body = "closed body"
	for name, layout := range layouts() {
		out := render(t, BuildView(st, ViewInput{Layout: layout, ReplyID: "r"}))
		block := recapBlock(t, out, "open")
		itemContainer := `class="tbody"`
		if layout == LayoutFull {
			itemContainer = `class="topic-stream"`
		}
		recap, doc, items := strings.Index(block, `<div class="trecap">`), strings.Index(block, `<div class="tdoc"`), strings.Index(block, itemContainer)
		if recap < 0 || (doc >= 0 && recap > doc) || recap > items {
			t.Fatalf("%s: recap %d, doc %d, items %d", name, recap, doc, items)
		}
		wantAll(t, block, `<div class="rtext md" title="**READY**`, `"><strong>READY</strong> · see <img`, `shot.png`, `<u>kept</u>`, `datetime="2026-11-04T09:00:00Z"`)
		wantNone(t, block, "<script>alert")
		shut := recapBlock(t, out, "shut")
		wantAll(t, shut, `<div class="tdoc"`, `<div class="trecap">`)
		wantNone(t, shut, `class="tresult"`)
		wantAll(t, recapBlock(t, out, "lone"), `<div class="rtext md" title="only a recap">only a recap</div>`)
	}
}

func TestLegacyRecapReferencesAndSandbox(t *testing.T) {
	st := &State{}
	mustApply(t, st, addXML(ClassTechnical, "t", "target"))
	st.topicDoc("t").Body = "framed body"
	legacyRecap(st, "t", "waiting on {{1}}, see {{40}} and {{99}}; `{{1}}` <img src=x onerror=alert(1)>")
	for _, layout := range []Layout{LayoutFull} {
		view := BuildView(st, ViewInput{Layout: layout, ReplyID: "r"})
		view.Stored[40] = true
		wantAll(t, recapBlock(t, render(t, view), "t"), `waiting on <a href="#item-1">(01)</a>, see (40) and {{99}}; <code>{{1}}</code>`)
	}
	regions, err := RenderRegions(BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"}))
	if err != nil {
		t.Fatal(err)
	}
	board := string(regions.Board)
	recap, frame := strings.Index(board, `<div class="trecap">`), strings.Index(board, `<div class="doc-frame"`)
	if recap < 0 || frame < 0 || recap > frame {
		t.Fatalf("legacy recap %d, frame %d", recap, frame)
	}
	// The full source is inert, attribute-escaped hover text; it must never
	// return as an executable image/event attribute in the rendered recap.
	wantAll(t, board[:frame], `&lt;img src=x onerror=alert(1)&gt;`)
	wantNone(t, board[:frame], `<img src=x onerror=alert(1)>`, ` onerror="`, ` onerror='`)
	if got := st.TextRefs("on {{1}} and {{01}}, not {{99}} or {{x}}"); got != "on (01) and (01), not {{99}} or {{x}}" {
		t.Errorf("TextRefs = %q", got)
	}
	if got := (*State)(nil).TextRefs("{{1}}"); got != "{{1}}" {
		t.Errorf("nil TextRefs = %q", got)
	}
}

// The page uses a display-only head projection independent of latest-log
// highlighting and without changing durable state or panel payloads.
func TestRecapHeadChoosesNewestLogOrPanel(t *testing.T) {
	const early = "2026-11-04T08:00:00Z"
	const late = "2026-11-04T09:00:00Z"
	agent := Entry{ID: "e9", Kind: EntryAgent, Text: "Agent **ready**", At: early}
	panel := Entry{ID: "e10", Kind: EntryPanel, PanelID: "p1", Recap: "Panel **ready**", Text: "PRIVATE PANEL PAYLOAD", At: late}
	withAt := func(e Entry, at string) Entry { e.At = at; return e }
	withID := func(e Entry, id string) Entry { e.ID = id; return e }
	for _, tc := range []struct {
		name    string
		entries []Entry
		want    Entry
	}{
		{"panel-only", []Entry{panel}, panel},
		{"agent-only", []Entry{agent}, agent},
		{"later-panel", []Entry{agent, panel}, panel},
		{"new-log-after-panel", []Entry{panel, withAt(agent, "2026-11-04T10:00:00Z")}, withAt(agent, "2026-11-04T10:00:00Z")},
		{"equal-panel-larger-eN", []Entry{withAt(panel, early), agent}, withAt(panel, early)},
		{"equal-agent-larger-eN", []Entry{withID(agent, "e10"), withID(withAt(panel, early), "e9")}, withID(agent, "e10")},
		{"equivalent-time-offsets", []Entry{withAt(panel, "2026-11-04T10:00:00+01:00"), withAt(agent, late)}, withAt(panel, "2026-11-04T10:00:00+01:00")},
		{"timestamps-before-ID", []Entry{panel, withID(agent, "e99")}, panel},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entries := append([]Entry(nil), tc.entries...)
			entries = append(entries, Entry{ID: "e100", Kind: EntrySystem, Text: "never the summary", At: "2026-11-04T11:00:00Z"})
			st := &State{EntrySeq: 100, PanelCounter: 1, Topics: []TopicDoc{{Name: "t", Entries: entries}}}
			before := string(mustMarshal(t, st))
			for _, layout := range layouts() {
				view := BuildView(st, ViewInput{Layout: layout, ReplyID: "r", Location: time.UTC})
				tv := view.Topics[0]
				if tv.Head == nil || tv.Head.ID != tc.want.ID || tv.Summary() != tc.want.Summary() || tv.Head.At != tc.want.At || !tv.HeadSummary() {
					t.Fatalf("head = %+v, summary %q; want %+v", tv.Head, tv.Summary(), tc.want)
				}
				at, _ := time.Parse(time.RFC3339Nano, tc.want.At)
				if tv.RecapAt != at.UTC().Format(time.RFC3339) || tv.RecapMs != at.UnixMilli() || tv.RecapClock != at.UTC().Format("15:04") {
					t.Fatalf("head timestamp = %s/%d/%s, want %s", tv.RecapAt, tv.RecapMs, tv.RecapClock, tc.want.At)
				}
				out := render(t, view)
				wantAll(t, out, `class="tsub"><div class="trecap" data-entry="`+tc.want.ID+`" data-kind="`+tc.want.Kind+`"`,
					`class="rtext md" title="`+tc.want.Summary()+`">`, `<strong>ready</strong>`)
				if tc.want.Kind == EntryPanel {
					wantAll(t, out, `class="evi wb-evi-panel" title="Panels">P</span>`)
				} else {
					wantAll(t, out, `class="evi lead" title="Agent log line">L</span>`)
				}
			}
			if string(mustMarshal(t, st)) != before {
				t.Fatal("summary projection changed durable state")
			}
		})
	}
	if (TopicDoc{}).LatestSummary() != nil {
		t.Fatal("empty topic has a summary")
	}
}

func TestQueryReadsAgentLinesInsteadOfLegacyRecaps(t *testing.T) {
	st := &State{}
	mustApply(t, st, addXML(ClassTechnical, "t", "x")+topicXML("t", "body"))
	legacyRecap(st, "t", "legacy READY · <code>x</code>\nline two")
	legacyRecap(st, "c", "closed recap")
	mustApply(t, st, `<wb:topic name="c"><wb:close result="done"/></wb:topic>`)
	// Import belongs to the locked loader, not to a pure query fallback.
	// The body command already published a content panel; its recap, not
	// the unconverted legacy field, supplies the current summary.
	head := st.Topics[0].LatestSummary()
	wantText(t, "unconverted snapshot", queryText(t, st, `{"depth":"topics"}`), []string{`topic "t" [open]`, "current " + head.ID + " " + head.At + ": Current content"}, []string{"legacy READY", "closed recap"})
	mustApply(t, st, `<wb:topic name="t"><wb:log>READY · current agent line</wb:log></wb:topic>`)
	agent, _ := st.LatestAgent("t")
	current := "current " + agent.ID + " " + agent.At + ": READY · current agent line"
	wantText(t, "depth topics", queryText(t, st, `{"depth":"topics"}`), []string{current}, []string{"legacy READY", "topic body:", "closed recap"})
	wantText(t, "discovery", queryText(t, st, `{}`), []string{current}, []string{"(01) [open]", "topic body:"})
	wantText(t, "explicit items", queryText(t, st, `{"depth":"items"}`), []string{"(01) [open]"}, []string{"legacy READY"})
	wantText(t, "explicit body", queryText(t, st, `{"topic":"t","body":true}`), []string{current, "topic body:\n<<<\nbody\n>>>\n"}, []string{"legacy READY"})
	wantText(t, "closed", queryText(t, st, `{"closed":true,"depth":"topics"}`), []string{`topic "c" [closed]`}, []string{"closed recap", "legacy READY"})
}
