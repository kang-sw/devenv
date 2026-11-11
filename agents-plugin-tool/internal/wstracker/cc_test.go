package wstracker

import (
	"slices"
	"testing"
	"time"
)

// ccView derives a cc-* case (testdata/cases) for one viewer clone through
// the Go domain. The golden test pins the whole view; these tests name the
// CC behaviors the case was built to show.
func ccView(t *testing.T, name, viewer string) *View {
	t.Helper()
	doc, src := loadGolden(t, "testdata/golden/case-"+name+"__"+viewer+".json")
	now, err := time.Parse(time.RFC3339Nano, doc.Now)
	if err != nil {
		t.Fatal(err)
	}
	return Derive(src, viewer, now, 0)
}

func wantIDs(t *testing.T, what string, got, want []string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Fatalf("%s = %v, want %v", what, got, want)
	}
}

// Every applied event in a cc case applies: anyone CCs anyone, on an item
// they cannot edit, and a repeated add or a removed member's uncc is a no-op
// rather than a flag.
func TestCCIsOpenToEveryMember(t *testing.T) {
	v := ccView(t, "cc-related", "lena.pc")
	for _, e := range v.Events {
		if e.Status != "applied" {
			t.Fatalf("%s %s is %s (%v), want applied", e.ID, e.Type, e.Status, e.Reason)
		}
	}
	// raj (no edit right on ada's Ship) CCd lena; lena re-added herself;
	// sven was CC'd and removed, so the list drops him.
	wantIDs(t, "Ship cc", v.Items["ada.pc#1"].CC, []string{"lena", "maya", "omar"})
}

// The related lists: CC'd items by the latest CC add, then mentioned items
// newest mention first; an item both CC'd and mentioning the viewer is CC
// only; the viewer's own, hidden, and done items are left out of both, done
// ones moving to closed.
func TestRelatedLists(t *testing.T) {
	v := ccView(t, "cc-related", "lena.pc")
	r := v.My.Related
	// Infra (raj.pc#4) was CC'd after Ship, and mentions lena too: CC only.
	// Own (lena.pc#3) is hers; Old (bo.pc#4) is hidden; Review (ada.pc#5) is done.
	wantIDs(t, "related.cc", r.CC, []string{"raj.pc#4", "ada.pc#1"})
	// Docs' comment mention (omar.pc#1) is newer than Design's body mention
	// (maya.pc#3, whose CC was added and removed again).
	wantIDs(t, "related.mentioned", r.Mentioned, []string{"omar.pc#1", "maya.pc#3"})
	wantIDs(t, "related.closed", r.Closed, []string{"ada.pc#5", "maya.pc#5"})

	// maya holds Ship and is CC'd on it: no related row and no CC notice.
	m := ccView(t, "cc-related", "maya.pc").My
	if len(m.Related.CC)+len(m.Related.Mentioned)+len(m.Related.Closed)+len(m.CCNotices) != 0 {
		t.Fatalf("maya related = %+v, notices %+v, want none", m.Related, m.CCNotices)
	}
}

// A CC'd member hears of status writes, closes, confirms, rejects, and
// comments written after the CC; not of body edits, assignment changes,
// their own events, comments that mention them, or items they created.
func TestCCNotices(t *testing.T) {
	v := ccView(t, "cc-related", "lena.pc")
	var got []string
	for _, n := range v.My.CCNotices {
		got = append(got, n.Event+" "+n.Type)
		if n.Key != "cc:"+n.Event {
			t.Fatalf("notice key %q, want cc:%s", n.Key, n.Event)
		}
		if slices.Contains(v.My.Badge, n.Key) {
			t.Fatalf("CC notice %s is in the badge", n.Key)
		}
	}
	wantIDs(t, "lena CC notices", got, []string{
		"maya.pc#1 item.status",
		"bo.pc#1 comment.add",
		"maya.pc#2 item.close",
		"raj.pc#2 item.close",
		"ada.pc#7 item.reject",
		"raj.pc#3 item.close",
		"ada.pc#8 item.confirm",
		"raj.pc#5 comment.add",
	})
	first := v.My.CCNotices[0]
	if first.Status == nil || *first.Status != "doing" || first.To == nil || *first.To != "maya" {
		t.Fatalf("status notice = %+v, want doing for maya", first)
	}
	if rj := v.My.CCNotices[4]; rj.To == nil || *rj.To != "raj" || rj.By != "ada" {
		t.Fatalf("reject notice = %+v, want by ada to raj", rj)
	}
}

// A concurrent add and remove of one member fold by the register winner,
// and the CC dates from the winning add.
func TestCCConcurrentAddRemove(t *testing.T) {
	v := ccView(t, "cc-concurrent", "lena.pc")
	wantIDs(t, "Ship cc", v.Items["ada.pc#1"].CC, []string{"lena"})
	wantIDs(t, "related.cc", v.My.Related.CC, []string{"ada.pc#1"})
	marked := 0
	for _, e := range v.Events {
		marked += len(e.Markers)
	}
	if marked != 4 {
		t.Fatalf("concurrent-change markers = %d, want 4 (two pairs)", marked)
	}
	if len(v.My.CCNotices) != 1 || v.My.CCNotices[0].Event != "ada.pc#2" {
		t.Fatalf("lena notices = %+v, want ada.pc#2 only", v.My.CCNotices)
	}
}

// An org superior's status write on a CC'd item names the assignee it sets,
// and a dropped item, wrapped like a done one, moves from CC to closed.
func TestCCSuperiorStatusAndDropped(t *testing.T) {
	v := ccView(t, "cc-wrapped", "lena.pc")
	wantIDs(t, "related.cc", v.My.Related.CC, nil)
	wantIDs(t, "related.closed", v.My.Related.Closed, []string{"maya.pc#1"})
	if len(v.My.CCNotices) != 2 {
		t.Fatalf("lena notices = %+v, want two", v.My.CCNotices)
	}
	for _, n := range v.My.CCNotices {
		if n.By != "ada" || n.To == nil || *n.To != "maya" {
			t.Fatalf("notice %+v, want by ada to maya", n)
		}
	}
}
