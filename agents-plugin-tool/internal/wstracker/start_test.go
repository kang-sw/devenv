package wstracker

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsledger"
)

// startRefusal dispatches one event by stream's member on the folded
// linear history steps (the reasons genesis) and returns its refusal
// reason, "" when it is accepted.
func startRefusal(t *testing.T, steps []step, stream, typ string, payload map[string]any) string {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	trk := New(newMemSource(loadReasonCases(t).Genesis, stream, history(steps)), stream)
	err = trk.validate([]wsledger.NewEvent{{Type: typ, Payload: raw}})
	var ref *Refusal
	if errors.As(err, &ref) {
		return ref.Reason
	}
	if err != nil {
		t.Fatal(err)
	}
	return ""
}

func strp(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
}

// TestItemStartDate covers the optional start beside due: set on create
// with or without a due, cleared by null on edit, and refused at write time
// only when it would fall after the due (the side an edit leaves out is the
// folded item's; a clear never refuses).
func TestItemStartDate(t *testing.T) {
	steps := []step{
		{"ada.pc", TypeItemCreate, map[string]any{"kind": "task", "title": "Ship", "start": "2026-11-05", "due": "2026-11-10"}},
		{"maya.pc", TypeItemCreate, map[string]any{"kind": "task", "title": "Open", "start": "2026-11-20"}},
	}
	v := labelView(t, steps)
	wantEq(t, "start", strp(v.Items["ada.pc#1"].Start), "2026-11-05")
	wantEq(t, "due", strp(v.Items["ada.pc#1"].Due), "2026-11-10")
	wantEq(t, "start without due", strp(v.Items["maya.pc#1"].Start), "2026-11-20")
	wantEq(t, "no due", strp(v.Items["maya.pc#1"].Due), "<nil>")

	create := func(p map[string]any) map[string]any {
		p["kind"], p["title"] = "task", "x"
		return p
	}
	edit := func(p map[string]any) map[string]any {
		p["item"] = "ada.pc#1"
		return p
	}
	for _, c := range []struct {
		what, typ string
		payload   map[string]any
		want      string
	}{
		{"create start after due", TypeItemCreate, create(map[string]any{"start": "2026-11-20", "due": "2026-11-10"}), "start 2026-11-20 is after due 2026-11-10"},
		{"create start on the due day", TypeItemCreate, create(map[string]any{"start": "2026-11-10", "due": "2026-11-10"}), ""},
		{"create start without due", TypeItemCreate, create(map[string]any{"start": "2026-11-20"}), ""},
		{"edit start after the item's due", TypeItemEdit, edit(map[string]any{"start": "2026-11-11"}), "start 2026-11-11 is after due 2026-11-10"},
		{"edit due before the item's start", TypeItemEdit, edit(map[string]any{"due": "2026-11-04"}), "start 2026-11-05 is after due 2026-11-04"},
		{"edit both inverted", TypeItemEdit, edit(map[string]any{"start": "2026-12-02", "due": "2026-12-01"}), "start 2026-12-02 is after due 2026-12-01"},
		{"edit both in order", TypeItemEdit, edit(map[string]any{"start": "2026-12-01", "due": "2026-12-02"}), ""},
		{"edit start later while clearing due", TypeItemEdit, edit(map[string]any{"start": "2026-12-01", "due": nil}), ""},
		{"edit due earlier while clearing start", TypeItemEdit, edit(map[string]any{"start": nil, "due": "2026-11-01"}), ""},
		{"clear start", TypeItemEdit, edit(map[string]any{"start": nil}), ""},
		{"clear due", TypeItemEdit, edit(map[string]any{"due": nil}), ""},
		{"edit another field", TypeItemEdit, edit(map[string]any{"title": "y"}), ""},
	} {
		wantEq(t, c.what, startRefusal(t, steps, "ada.pc", c.typ, c.payload), c.want)
	}

	// null clears start and leaves due; a cleared start drops out of the
	// view's JSON. Clearing due leaves start.
	v = labelView(t, append(steps,
		step{"ada.pc", TypeItemEdit, map[string]any{"item": "ada.pc#1", "start": nil}},
		step{"maya.pc", TypeItemEdit, map[string]any{"item": "maya.pc#1", "due": "2026-11-30"}},
		step{"maya.pc", TypeItemEdit, map[string]any{"item": "maya.pc#1", "due": nil}}))
	wantEq(t, "cleared start", strp(v.Items["ada.pc#1"].Start), "<nil>")
	wantEq(t, "due after clearing start", strp(v.Items["ada.pc#1"].Due), "2026-11-10")
	wantEq(t, "start after clearing due", strp(v.Items["maya.pc#1"].Start), "2026-11-20")
	b, err := json.Marshal(v.Items["ada.pc#1"])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), `"start"`) {
		t.Fatalf("a cleared start stays in the view JSON: %s", b)
	}
}

// TestConcurrentStartAndDueFoldInverted: one clone moves the due earlier
// while another, not having seen it, moves the start later. Each write is
// in order on what its writer saw, the replay applies both without a flag,
// and the view carries the inverted pair as written.
func TestConcurrentStartAndDueFoldInverted(t *testing.T) {
	steps := []step{
		{"ada.pc", TypeItemCreate, map[string]any{"kind": "task", "title": "Ship", "start": "2026-11-05", "due": "2026-11-10"}},
		{"ada.pc", TypeItemEdit, map[string]any{"item": "ada.pc#1", "due": "2026-11-06"}},
		{"ada.lap", TypeItemEdit, map[string]any{"item": "ada.pc#1", "start": "2026-11-08"}, "", map[string]int{"ada.pc": 1}},
	}
	v := labelView(t, steps)
	wantEq(t, "start", strp(v.Items["ada.pc#1"].Start), "2026-11-08")
	wantEq(t, "due", strp(v.Items["ada.pc#1"].Due), "2026-11-06")

	// A later write that leaves both dates alone is not refused over them.
	wantEq(t, "title edit on the inverted pair", startRefusal(t, steps, "ada.pc", TypeItemEdit, map[string]any{"item": "ada.pc#1", "title": "y"}), "")
	wantEq(t, "due fix on the inverted pair", startRefusal(t, steps, "ada.pc", TypeItemEdit, map[string]any{"item": "ada.pc#1", "due": "2026-11-09"}), "")
}
