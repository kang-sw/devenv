package wswhiteboard

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// recapXML sets a topic's recap.
func recapXML(name, recap string) string {
	return `<wb:topic name="` + name + `"><wb:recap>` + recap + `</wb:recap></wb:topic>`
}

// topicUpdated is the topic's latest change as the page shows it.
func topicUpdated(t *testing.T, st *State, name string) string {
	t.Helper()
	for _, tv := range BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"}).Topics {
		if tv.Name == name {
			return tv.UpdatedAt
		}
	}
	t.Fatalf("topic %q not on the page", name)
	return ""
}

func stamp(at time.Time) string { return at.UTC().Format(time.RFC3339) }

// TestRecapSetReplaceClear pins wb:recap: it sets the recap (stored trimmed)
// with its own change mark and time, replaces it whole, and an empty or blank
// body clears it; a recap alone starts a topic and keeps it known, and
// clearing it drops a topic left with nothing else.
func TestRecapSetReplaceClear(t *testing.T) {
	st := &State{}
	applyAt(t, st, recapXML("rc", "\n  READY · ticket X  \n"), testNow)
	got := topicEntry(t, st, "rc")
	if got.Recap != "READY · ticket X" || got.RecapChanged != st.Seq || got.RecapAt != stamp(testNow) || got.Body != "" || got.ChangedAt != "" {
		t.Fatalf("entry = %+v (Seq %d)", got, st.Seq)
	}
	if !st.topicKnown("rc") {
		t.Fatal("a recap alone must start a topic")
	}
	later := testNow.Add(10 * time.Minute)
	applyAt(t, st, recapXML("rc", "in implementation: branch Z"), later)
	if got := topicEntry(t, st, "rc"); got.Recap != "in implementation: branch Z" || got.RecapChanged != st.Seq || got.RecapAt != stamp(later) {
		t.Fatalf("replaced entry = %+v", got)
	}
	for name, clear := range map[string]string{
		"empty":        `<wb:topic name="rc"><wb:recap></wb:recap></wb:topic>`,
		"self-closing": `<wb:topic name="rc"><wb:recap/></wb:topic>`,
		"blank":        "<wb:topic name=\"rc\"><wb:recap>\n \n</wb:recap></wb:topic>",
	} {
		c := st.clone()
		mustApply(t, c, clear)
		if c.topicKnown("rc") || len(c.Topics) != 0 {
			t.Fatalf("%s: a topic left with nothing must be gone: %+v", name, c.Topics)
		}
		// Clearing again, and clearing a topic that never had a recap, is no problem.
		mustApply(t, c, clear)
		mustApply(t, c, `<wb:topic name="never"><wb:recap/></wb:topic>`)
		if len(c.Topics) != 0 {
			t.Fatalf("%s: topics = %+v", name, c.Topics)
		}
	}
	// wb:recap goes inside a group, like wb:body.
	wantProblems(t, refused(t, st, `<wb:recap>x</wb:recap>`), `goes inside a <wb:topic name="..."> group`)
}

// TestRecapUnchangedResendIsNoChange: the same recap again, however padded,
// moves neither the recap's time nor the topic's change time, and does not
// reopen a closed topic.
func TestRecapUnchangedResendIsNoChange(t *testing.T) {
	st := &State{}
	applyAt(t, st, addXML(ClassTechnical, "t", "x")+recapXML("t", "status"), testNow)
	before := topicEntry(t, st, "t")
	updated := topicUpdated(t, st, "t")
	applyAt(t, st, recapXML("t", "\n status \n"), testNow.Add(time.Hour))
	if got := topicEntry(t, st, "t"); got.RecapChanged != before.RecapChanged || got.RecapAt != before.RecapAt || got.ChangedAt != before.ChangedAt {
		t.Fatalf("an unchanged resend moved the recap: %+v, was %+v", got, before)
	}
	if got := topicUpdated(t, st, "t"); got != updated {
		t.Fatalf("an unchanged resend moved the topic's change time to %s (was %s)", got, updated)
	}
	applyAt(t, st, `<wb:topic name="t"><wb:close result="done"/></wb:topic>`, testNow.Add(2*time.Hour))
	applyAt(t, st, recapXML("t", "status"), testNow.Add(3*time.Hour))
	if !st.topicClosed("t") {
		t.Fatal("an unchanged recap reopened the topic")
	}
}

// TestRecapChangeIsATopicChange: a recap change moves the topic's change time
// and recency (the topic rises on the page and the stale-topic nudge stops
// counting it), and on a closed topic it reopens it. Clearing a recap is no
// new content, as clearing a body is: it reopens nothing.
func TestRecapChangeIsATopicChange(t *testing.T) {
	st := &State{}
	applyAt(t, st, addXML(ClassTechnical, "a", "x")+addXML(ClassTechnical, "b", "y"), testNow)
	mustAnswer := `<wb:answer ids="1 2" value="ok">ok</wb:answer>`
	applyAt(t, st, mustAnswer, testNow)
	applyAt(t, st, `<wb:patch id="2"><wb:why>touch b</wb:why></wb:patch>`, testNow.Add(time.Minute))
	if order := viewTopicNames(BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"})); order[0] != "b" {
		t.Fatalf("order = %v, want b first", order)
	}
	later := testNow.Add(2 * time.Hour)
	applyAt(t, st, recapXML("a", "moving again"), later)
	if order := viewTopicNames(BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"})); order[0] != "a" {
		t.Fatalf("order = %v, want a first after its recap changed", order)
	}
	if got := topicUpdated(t, st, "a"); got != stamp(later) {
		t.Fatalf("topic a updated %s, want %s", got, stamp(later))
	}
	r, problems := Query(st, queryPayload(t, `{"topic":"a","depth":"topics"}`))
	if len(problems) > 0 || r.Topics[0].LastChange != stamp(later) {
		t.Fatalf("query last change = %+v %v", r.Topics, problems)
	}
	// The nudge ages a topic by its recap too: a, changed just now, is not stale.
	for _, name := range []string{"c", "d", "e", "f"} {
		applyAt(t, st, topicXML(name, "body"), testNow)
	}
	st.LastNudgeAt = ""
	nudge := st.staleNudge(later.Add(30 * time.Minute))
	names := ""
	if i := strings.Index(nudge, "min ("); i >= 0 {
		names = nudge[i+len("min ("):]
		names = names[:strings.Index(names, ")")]
	}
	if names != "c, d, e, f, b" {
		t.Fatalf("nudge = %q, want b stale and a fresh", nudge)
	}

	// A recap written to a closed topic reopens it.
	applyAt(t, st, `<wb:topic name="a"><wb:close result="done"/></wb:topic>`, later)
	applyAt(t, st, recapXML("a", "reopened work"), later)
	if st.topicClosed("a") {
		t.Fatal("a changed recap must reopen a closed topic")
	}
	// Closing after the recap in the same batch keeps it closed.
	applyAt(t, st, recapXML("a", "last words")+`<wb:topic name="a"><wb:close result="done"/></wb:topic>`, later)
	if !st.topicClosed("a") || st.TopicRecap("a") != "last words" {
		t.Fatalf("close after a recap must stick: closed %v recap %q", st.ClosedTopics, st.TopicRecap("a"))
	}
	// Clearing the recap of a closed topic leaves it closed.
	applyAt(t, st, `<wb:topic name="a"><wb:recap/></wb:topic>`, later)
	if !st.topicClosed("a") || st.TopicRecap("a") != "" {
		t.Fatalf("clearing a recap reopened the topic: closed %v", st.ClosedTopics)
	}
}

// TestRecapAndBodyAreIndependent: clearing the body keeps the recap and the
// topic, and the reverse; a recap-only topic closes, keeps its stem, and
// refuses wb:edit as a topic without a body does.
func TestRecapAndBodyAreIndependent(t *testing.T) {
	st := &State{}
	applyAt(t, st, topicXML("t", "body")+recapXML("t", "recap"), testNow)
	mustApply(t, st, `<wb:topic name="t"><wb:close result="done"/></wb:topic>`)
	mustApply(t, st, `<wb:topic name="t"><wb:body/></wb:topic>`)
	if st.TopicRecap("t") != "recap" || st.TopicBody("t") != "" || !st.topicKnown("t") || !st.topicClosed("t") {
		t.Fatalf("clearing the body must keep the recap and the topic: %+v closed %v", st.Topics, st.ClosedTopics)
	}
	if got := topicEntry(t, st, "t"); got.Marks != nil || got.Changed != 0 || got.ChangedAt != "" {
		t.Fatalf("a cleared body keeps its change time: %+v", got)
	}
	wantProblems(t, refused(t, st, `<wb:topic name="t"><wb:edit><wb:old>a</wb:old><wb:new>b</wb:new></wb:edit></wb:topic>`), `topic "t" has no body; set one with <wb:body>`)

	other := &State{}
	mustApply(t, other, topicXML("t", "body")+recapXML("t", "recap"))
	mustApply(t, other, `<wb:topic name="t"><wb:recap/></wb:topic>`)
	if other.TopicBody("t") != "body" || other.TopicRecap("t") != "" {
		t.Fatalf("clearing the recap must keep the body: %+v", other.Topics)
	}
	if got := topicEntry(t, other, "t"); got.RecapChanged != 0 || got.RecapAt != "" {
		t.Fatalf("a cleared recap keeps its time: %+v", got)
	}

	// A recap-only stemmed topic closes and keeps its label; clearing the
	// recap of a topic with nothing else drops it, label and closed mark too.
	stem := &State{}
	mustApply(t, stem, `<wb:topic name="gate" label="Release gate"><wb:recap>waiting</wb:recap></wb:topic>`)
	mustApply(t, stem, `<wb:topic name="gate"><wb:close result="done"/></wb:topic>`)
	if !stem.topicClosed("gate") || stem.TopicLabel("gate") != "Release gate" {
		t.Fatalf("recap-only topic: closed %v labels %v", stem.ClosedTopics, stem.Labels)
	}
	mustApply(t, stem, `<wb:topic name="gate"><wb:recap/></wb:topic>`)
	if stem.topicKnown("gate") || stem.topicClosed("gate") || stem.Labels != nil {
		t.Fatalf("a topic left with nothing must be gone: %+v", stem)
	}
	wantProblems(t, refused(t, stem, `<wb:topic name="gate"><wb:close result="done"/></wb:topic>`), `no item, topic body, or recap has topic "gate"`)
}

// TestRecapSurvivesABodyRevertInItsBatch: a body the batch brings back to the
// one it began with restores only the body's marks and time, so a wb:recap
// earlier in the batch stands; a later body-only change leaves the recap's
// time alone.
func TestRecapSurvivesABodyRevertInItsBatch(t *testing.T) {
	st := &State{}
	applyAt(t, st, topicXML("t", "alpha"), testNow)
	base := topicEntry(t, st, "t")
	later := testNow.Add(time.Hour)
	applyAt(t, st, recapXML("t", "new recap")+topicXML("t", "beta")+topicXML("t", "alpha"), later)
	got := topicEntry(t, st, "t")
	if got.Recap != "new recap" || got.RecapChanged != st.Seq || got.RecapAt != stamp(later) {
		t.Fatalf("the body revert dropped the recap: %+v", got)
	}
	if got.Body != "alpha" || got.Changed != base.Changed || got.ChangedAt != base.ChangedAt {
		t.Fatalf("the body revert must restore the body's change: %+v, base %+v", got, base)
	}
	if updated := topicUpdated(t, st, "t"); updated != stamp(later) {
		t.Fatalf("the recap change must still move the topic: %s", updated)
	}

	// A body change later moves the body's time and leaves the recap's alone.
	latest := later.Add(time.Hour)
	recapSeq := got.RecapChanged
	applyAt(t, st, topicXML("t", "gamma"), latest)
	got = topicEntry(t, st, "t")
	if got.RecapChanged != recapSeq || got.RecapAt != stamp(later) || got.ChangedAt != stamp(latest) {
		t.Fatalf("a body-only change moved the recap's time: %+v", got)
	}
	// A recap the batch brings back keeps its time as well.
	applyAt(t, st, recapXML("t", "other")+recapXML("t", "new recap"), latest.Add(time.Hour))
	if got := topicEntry(t, st, "t"); got.RecapChanged != recapSeq || got.RecapAt != stamp(later) {
		t.Fatalf("a recap brought back in its batch moved its time: %+v", got)
	}
}

// recapBlock returns the rendered section of topic name.
func recapBlock(t *testing.T, out, name string) string {
	t.Helper()
	start := strings.Index(out, `data-topic="`+name+`"`)
	if start < 0 {
		t.Fatalf("topic %q missing", name)
	}
	block := out[start:]
	return block[:strings.Index(block, "</section>")]
}

// TestRecapRendersAtTheTopOfTheTopic pins the recap line: above the body and
// the items while the topic is open, on both layouts, its text by the item
// block-field rule (markdown, an image, and sanitized HTML), its time in the
// topic bar's notation with the local date and time as the hover title
// (relative on the full page through data-at, the HH:MM alone on the embed).
// A closed topic folds the recap with its body, and the close result stands
// in its place; a closed recap-only topic gets the fold to hold it.
func TestRecapRendersAtTheTopOfTheTopic(t *testing.T) {
	st := &State{}
	applyAt(t, st, addXML(ClassTechnical, "open", "x")+topicXML("open", "the body")+
		recapXML("open", "**READY** · see ![shot](shot.png) <u>kept</u><script>alert(1)</script>"), testNow)
	applyAt(t, st, addXML(ClassTechnical, "shut", "y")+topicXML("shut", "closed body")+recapXML("shut", "was moving")+
		recapXML("lone", "only a recap")+
		`<wb:topic name="shut"><wb:close result="Shipped."/></wb:topic><wb:topic name="lone"><wb:close result="Dropped."/></wb:topic>`, testNow)
	tz := time.FixedZone("test", 9*3600)
	for name, layout := range layouts() {
		out := render(t, BuildView(st, ViewInput{Layout: layout, ReplyID: "r", Location: tz}))
		block := recapBlock(t, out, "open")
		recap, doc, items := strings.Index(block, `<div class="trecap">`), strings.Index(block, `<details class="tdoc"`), strings.Index(block, `class="tbody"`)
		if recap < 0 || recap > doc || doc > items || recap < strings.Index(block, `class="tbar"`) {
			t.Fatalf("%s: recap %d, body %d, items %d: the recap is not at the top of the topic:\n%s", name, recap, doc, items, block)
		}
		wantAll(t, block, `<div class="rtext md"><strong>READY</strong> · see <img`, `shot.png`, `<u>kept</u>`)
		wantNone(t, block, "<script>alert")
		at := `<time class="tago" datetime="2026-11-04T09:00:00Z" title="2026-11-04 18:00"`
		if layout == LayoutFull {
			wantAll(t, block, fmt.Sprintf(`%s data-at="%d">18:00</time></div>`, at, testNow.UnixMilli()))
		} else {
			wantAll(t, block, at+`>18:00</time></div>`)
		}
		// The topic bar's own time comes first, so the board reads the bar's.
		if bar := strings.Index(block, `title="last change"`); bar < 0 || bar > recap {
			t.Errorf("%s: the bar's time must precede the recap's", name)
		}

		shut := recapBlock(t, out, "shut")
		fold := docTag.FindStringIndex(shut)
		if fold == nil || strings.HasSuffix(shut[fold[0]:fold[1]], " open>") {
			t.Fatalf("%s: a closed topic's fold missing or open: %s", name, shut)
		}
		if r := strings.Index(shut, `<div class="trecap">`); r < fold[1] || r > strings.Index(shut, "closed body") {
			t.Errorf("%s: a closed topic's recap must sit inside the fold above the body:\n%s", name, shut)
		}
		wantAll(t, shut, `<span class="tresult" title="how the topic ended">Shipped.</span>`)

		lone := recapBlock(t, out, "lone")
		wantAll(t, out, `<section class="topic quiet closed has-doc" data-topic="lone">`)
		wantAll(t, lone, `<details class="tdoc" data-key="doc:lone"><summary`, `<div class="rtext md">only a recap</div>`)
		wantNone(t, lone, "doc-body")
	}

	// An open recap-only topic shows its recap and is not quiet.
	solo := &State{}
	mustApply(t, solo, recapXML("solo", "waiting on CI"))
	out := render(t, BuildView(solo, ViewInput{Layout: LayoutFull, ReplyID: "r"}))
	wantAll(t, out, `<section class="topic" data-topic="solo">`)
	block := recapBlock(t, out, "solo")
	wantAll(t, block, `<div class="rtext md">waiting on CI</div>`)
	wantNone(t, block, "tdoc")
}

// TestRecapRendersOutsideTheBodyFrame: the served page renders the recap in
// its own markup, sanitized, never inside a topic body's sandboxed frame.
func TestRecapRendersOutsideTheBodyFrame(t *testing.T) {
	st := &State{}
	mustApply(t, st, topicXML("t", "framed body")+recapXML("t", "plain <b>recap</b><img src=x onerror=alert(1)>"))
	regions, err := RenderRegions(BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"}))
	if err != nil {
		t.Fatal(err)
	}
	board := string(regions.Board)
	recap, frame := strings.Index(board, `<div class="trecap"><div class="rtext md">plain <b>recap</b>`), strings.Index(board, `<iframe class="doc-frame"`)
	if recap < 0 || frame < 0 || recap > frame {
		t.Fatalf("recap %d, frame %d:\n%s", recap, frame, board)
	}
	wantNone(t, board[:frame], "onerror")
}

// TestQueryPrintsTheRecap: the recap and its time print as stored after the
// topic line at every depth, whatever the body filter, and for a closed topic
// listed with closed: true.
func TestQueryPrintsTheRecap(t *testing.T) {
	st := &State{}
	applyAt(t, st, addXML(ClassTechnical, "t", "x")+topicXML("t", "body")+recapXML("t", "READY · <code>x</code>\nline two"), testNow)
	applyAt(t, st, recapXML("c", "closed recap")+`<wb:topic name="c"><wb:close result="done"/></wb:topic>`, testNow)
	at := stamp(testNow)
	text := queryText(t, st, `{"depth":"topics"}`)
	wantText(t, "depth topics", text, []string{`topic "t" [open] items: open 1; body: 4 chars; changed: ` + at + "\nrecap (changed: " + at + "): READY · <code>x</code>\n  line two\n"}, []string{"topic body:", "closed recap"})
	wantText(t, "depth items", queryText(t, st, `{}`), []string{"\nrecap (changed: " + at + "): READY", "  (01) [open]"}, nil)
	wantText(t, "topic filter with body", queryText(t, st, `{"topic":"t"}`), []string{"recap (changed: " + at + "): READY · <code>x</code>\n  line two\ntopic body:\n<<<\nbody\n>>>\n"}, nil)
	wantText(t, "closed listed", queryText(t, st, `{"closed":true,"depth":"topics"}`), []string{`topic "c" [closed] items: none; changed: ` + at + "; result: done\nrecap (changed: " + at + "): closed recap\n"}, nil)
}
