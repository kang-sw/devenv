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
	mustApply(t, st, addXML(ClassTechnical, "open", "x")+topicXML("open", "the body")+topicXML("shut", "closed body"))
	legacyRecap(st, "open", "**READY** · see ![shot](shot.png) <u>kept</u><script>alert(1)</script>")
	legacyRecap(st, "shut", "was moving")
	legacyRecap(st, "lone", "only a recap")
	mustApply(t, st, `<wb:topic name="shut"><wb:close result="Shipped."/></wb:topic><wb:topic name="lone"><wb:close result="Dropped."/></wb:topic>`)
	for name, layout := range layouts() {
		out := render(t, BuildView(st, ViewInput{Layout: layout, ReplyID: "r"}))
		block := recapBlock(t, out, "open")
		recap, doc, items := strings.Index(block, `<div class="trecap">`), strings.Index(block, `<details class="tdoc"`), strings.Index(block, `class="tbody"`)
		if recap < 0 || (layout == LayoutFull && (recap > doc || doc > items)) || (layout == LayoutEmbed && recap > items) {
			t.Fatalf("%s: recap %d, doc %d, items %d", name, recap, doc, items)
		}
		wantAll(t, block, `<div class="rtext md"><strong>READY</strong> · see <img`, `shot.png`, `<u>kept</u>`, `datetime="2026-11-04T09:00:00Z"`)
		wantNone(t, block, "<script>alert")
		shut := recapBlock(t, out, "shut")
		fold := docTag.FindStringIndex(shut)
		if fold == nil || strings.HasSuffix(shut[fold[0]:fold[1]], " open>") || strings.Index(shut, `<div class="trecap">`) < fold[1] {
			t.Fatalf("%s: closed legacy recap is not folded: %s", name, shut)
		}
		wantAll(t, shut, `<span class="tresult" title="how the topic ended">Shipped.</span>`)
		wantAll(t, recapBlock(t, out, "lone"), `<div class="rtext md">only a recap</div>`)
	}
}

func TestLegacyRecapReferencesAndSandbox(t *testing.T) {
	st := &State{}
	mustApply(t, st, addXML(ClassTechnical, "t", "target")+topicXML("t", "framed body"))
	legacyRecap(st, "t", "waiting on {{1}}, see {{40}} and {{99}}; `{{1}}` <img src=x onerror=alert(1)>")
	for _, layout := range []Layout{LayoutFull, LayoutEmbed} {
		view := BuildView(st, ViewInput{Layout: layout, ReplyID: "r"})
		view.Stored[40] = true
		wantAll(t, recapBlock(t, render(t, view), "t"), `waiting on <a href="#item-1">(01)</a>, see (40) and {{99}}; <code>{{1}}</code>`)
	}
	regions, err := RenderRegions(BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"}))
	if err != nil {
		t.Fatal(err)
	}
	board := string(regions.Board)
	recap, frame := strings.Index(board, `<div class="trecap">`), strings.Index(board, `<iframe class="doc-frame"`)
	if recap < 0 || frame < 0 || recap > frame {
		t.Fatalf("legacy recap %d, frame %d", recap, frame)
	}
	wantNone(t, board[:frame], "onerror")
	if got := st.TextRefs("on {{1}} and {{01}}, not {{99}} or {{x}}"); got != "on (01) and (01), not {{99}} or {{x}}" {
		t.Errorf("TextRefs = %q", got)
	}
	if got := (*State)(nil).TextRefs("{{1}}"); got != "{{1}}" {
		t.Errorf("nil TextRefs = %q", got)
	}
}

func TestLegacyQueryPrintsTheRecap(t *testing.T) {
	st := &State{}
	mustApply(t, st, addXML(ClassTechnical, "t", "x")+topicXML("t", "body"))
	legacyRecap(st, "t", "READY · <code>x</code>\nline two")
	legacyRecap(st, "c", "closed recap")
	mustApply(t, st, `<wb:topic name="c"><wb:close result="done"/></wb:topic>`)
	at := testNow.Format(time.RFC3339)
	wantText(t, "depth topics", queryText(t, st, `{"depth":"topics"}`), []string{`topic "t" [open] items: open 1; body: 4 chars; changed: ` + at + "\nrecap (changed: " + at + "): READY · <code>x</code>\n  line two\n"}, []string{"topic body:", "closed recap"})
	wantText(t, "depth items", queryText(t, st, `{}`), []string{"\nrecap (changed: " + at + "): READY", "  (01) [open]"}, nil)
	wantText(t, "body", queryText(t, st, `{"topic":"t"}`), []string{"recap (changed: " + at + "): READY · <code>x</code>\n  line two\ntopic body:\n<<<\nbody\n>>>\n"}, nil)
	wantText(t, "closed", queryText(t, st, `{"closed":true,"depth":"topics"}`), []string{`topic "c" [closed] items: none; changed: ` + at + "; result: done\nrecap (changed: " + at + "): closed recap\n"}, nil)
}
