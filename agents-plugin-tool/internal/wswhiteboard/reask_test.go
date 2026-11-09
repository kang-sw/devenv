package wswhiteboard

import (
	"strings"
	"testing"
	"time"
)

// patchAt is applyAt with the time first, so a run of timed batches reads
// in time order.
func patchAt(t *testing.T, st *State, now time.Time, commands string) {
	t.Helper()
	applyAt(t, st, commands, now)
}

func TestPatchClassOnlyOnAnItemOpenAfterThePatch(t *testing.T) {
	st := &State{}
	mustApply(t, st, addXML(ClassTechnical, "t", "open one")+addXML(ClassTechnical, "t", "settled one"))
	mustApply(t, st, `<wb:answer id="2" value="ok">ok</wb:answer>`)

	// An open item reclasses.
	mustApply(t, st, `<wb:patch id="1" class="announced"/>`)
	if got := item(t, st, 1); got.Class != ClassAnnounced || got.Status != StatusOpen {
		t.Fatalf("reclassed open item = %+v", got)
	}
	wantProblems(t, refused(t, st, `<wb:patch id="1" class="question"/>`), "class: must be one of")

	// A settled item refuses a class or options change without status="open",
	// and with a status that leaves it settled.
	wantProblems(t, refused(t, st, `<wb:patch id="2" class="policy"/>`), "(02) is answered; class changes only on an open item")
	wantProblems(t, refused(t, st, `<wb:patch id="2" status="withdrawn" class="policy"/>`), "(02) is withdrawn; class changes only on an open item")
	wantProblems(t, refused(t, st, `<wb:patch id="1" status="withdrawn"><wb:option>x</wb:option></wb:patch>`), "(01) is withdrawn; options change only on an open item")

	// One patch reopens, reclasses, and gives options with a recommendation.
	mustApply(t, st, `<wb:patch id="2" status="open" class="policy"><wb:option>keep</wb:option><wb:option>drop</wb:option><wb:recommend>a</wb:recommend></wb:patch>`)
	got := item(t, st, 2)
	if got.Status != StatusOpen || got.Class != ClassPolicy || len(got.Options) != 2 || got.Answer != "" || got.Settled != 0 {
		t.Fatalf("reopened policy item = %+v", got)
	}
}

func TestOptionsOnANonPolicyItemWarn(t *testing.T) {
	const note = "Options on a non-policy item"
	st := &State{}
	out := mustApply(t, st, addXML(ClassTechnical, "t", "tech", "x", "y"))
	if !strings.Contains(out.Info, note+": (01) [technical]") {
		t.Fatalf("wb:add of a technical item with options: info = %q", out.Info)
	}
	out = mustApply(t, st, addXML(ClassAnnounced, "t", "ann")+addXML(ClassPolicy, "t", "pol", "x", "y"))
	if strings.Contains(out.Info, note) {
		t.Fatalf("no item gained options off policy: info = %q", out.Info)
	}
	out = mustApply(t, st, `<wb:patch id="2"><wb:option>x</wb:option></wb:patch>`)
	if !strings.Contains(out.Info, note+": (02) [announced]") {
		t.Fatalf("patch adding options to an announced item: info = %q", out.Info)
	}
	out = mustApply(t, st, `<wb:patch id="3" class="technical"/>`)
	if !strings.Contains(out.Info, note+": (03) [technical]") {
		t.Fatalf("reclass away from policy that keeps options: info = %q", out.Info)
	}
	if got := item(t, st, 3); len(got.Options) != 2 {
		t.Fatalf("a reclass keeps the options: %+v", got)
	}
	// The note judges each add or patch by the item after it: a reclass to
	// policy carries none, and any patch of an item still holding options off
	// policy carries it again.
	out = mustApply(t, st, `<wb:patch id="1" class="policy"/>`)
	if strings.Contains(out.Info, note) {
		t.Fatalf("a reclass to policy: info = %q", out.Info)
	}
	out = mustApply(t, st, `<wb:patch id="2"><wb:why>w</wb:why></wb:patch>`)
	if !strings.Contains(out.Info, note+": (02) [announced]") {
		t.Fatalf("a why edit of an announced item holding options: info = %q", out.Info)
	}
}

func TestReaskStampsOnlyARealClassOrOptionsChangeOrAReopen(t *testing.T) {
	t0 := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	at := func(d time.Duration) time.Time { return t0.Add(d) }
	ms := func(d time.Duration) int64 { return at(d).UnixMilli() }
	st := &State{}
	patchAt(t, st, at(0), addXML(ClassTechnical, "t", "tech")+addXML(ClassPolicy, "t", "pol", "x", "y")+addXML(ClassTechnical, "t", "done"))
	if got := st.ReaskTimes(); got != nil {
		t.Fatalf("a board never re-asked has no re-ask times: %v", got)
	}

	// Not a re-ask: title, why, recommend, links; the current class; the
	// identical options.
	patchAt(t, st, at(time.Second), `<wb:patch id="1"><wb:title>t2</wb:title><wb:why>w</wb:why><wb:link>l</wb:link></wb:patch>`+
		`<wb:patch id="1" class="technical"/><wb:patch id="2"><wb:option>x</wb:option><wb:option>y</wb:option><wb:recommend>a</wb:recommend></wb:patch>`)
	if got := st.ReaskTimes(); got != nil {
		t.Fatalf("no patch re-asked: %v", got)
	}

	// A class change on an open item, an options change on an open item, and
	// settled to open each stamp the patch's time, to the millisecond.
	patchAt(t, st, at(2*time.Second+5*time.Millisecond), `<wb:patch id="1" class="policy"><wb:option>p</wb:option></wb:patch><wb:patch id="2"><wb:option>y</wb:option><wb:option>x</wb:option></wb:patch>`)
	patchAt(t, st, at(3*time.Second), `<wb:answer id="3" value="ok">ok</wb:answer>`)
	patchAt(t, st, at(4*time.Second), `<wb:patch id="3" status="open"/>`)
	want := map[int]int64{1: ms(2*time.Second + 5*time.Millisecond), 2: ms(2*time.Second + 5*time.Millisecond), 3: ms(4 * time.Second)}
	if got := st.ReaskTimes(); len(got) != 3 || got[1] != want[1] || got[2] != want[2] || got[3] != want[3] {
		t.Fatalf("re-ask times = %v, want %v", got, want)
	}

	// A later re-ask replaces the time; an open item patched to open again
	// with nothing else is no re-ask.
	patchAt(t, st, at(5*time.Second), `<wb:patch id="2" clear="options"><wb:recommend></wb:recommend></wb:patch><wb:patch id="3" status="open"/>`)
	if got := st.ReaskTimes(); got[2] != ms(5*time.Second) || got[3] != ms(4*time.Second) {
		t.Fatalf("re-ask times after a second re-ask = %v", got)
	}
	// An assumed item set back to open is a re-ask too.
	patchAt(t, st, at(6*time.Second), `<wb:answer id="3" value="ok" assumed="true">fine I guess</wb:answer>`)
	patchAt(t, st, at(7*time.Second), `<wb:patch id="3" status="open"/>`)
	if got := st.ReaskTimes(); got[3] != ms(7*time.Second) {
		t.Fatalf("assumed to open: re-ask times = %v", got)
	}
}
