package wsindex

import (
	"errors"
	"strings"
	"testing"
	"time"
)

var (
	t0      = time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	ownerA  = Owner{Email: "a@example.com", CloneID: "ca", Track: "develop"}
	ownerA2 = Owner{Email: "A@Example.com", CloneID: "ca2", Track: "develop"} // same email, other clone
	ownerAT = Owner{Email: "a@example.com", CloneID: "ca", Track: "track/other"}
	ownerB  = Owner{Email: "b@example.com", CloneID: "cb", Track: "develop"} // same track name, other person
)

const stemX = "260924-feat-x"

func leased(owner Owner, phase string) *Index {
	idx := NewIndex()
	idx.Register(stemX, t0)
	idx.Registrations[stemX].Lease = newLease(owner, phase, t0)
	return idx
}

func acquireEntry(owner Owner) PendingEntry {
	return PendingEntry{Op: OpAcquire, Stem: stemX, Owner: owner}
}

func withOverride(e PendingEntry, holder Owner, reason string) PendingEntry {
	e.Override = &Override{Holder: holder, Reason: reason}
	return e
}

func holderOf(t *testing.T, idx *Index) Owner {
	t.Helper()
	reg := idx.Registrations[stemX]
	if reg == nil || reg.Lease == nil {
		t.Fatalf("%s has no lease", stemX)
	}
	return reg.Lease.Owner()
}

// C1-C5, C8: the acquire rows of the owner-conflict matrix.
func TestAcquireMatrix(t *testing.T) {
	later := t0.Add(time.Hour)
	cases := []struct {
		name       string
		holder     Owner
		entry      PendingEntry
		refused    bool
		wantHolder Owner
		effect     Effect
		warn       bool
	}{
		{"C1 different email refused", ownerA, acquireEntry(ownerB), true, ownerA, EffectNone, false},
		{"C1 different email with override", ownerA, withOverride(acquireEntry(ownerB), Owner{}, "user asked"), false, ownerB, EffectTakeover, false},
		{"C2 same email different clone", ownerA, acquireEntry(ownerA2), false, ownerA2, EffectTakeover, true},
		{"C3 same clone different track refused", ownerA, acquireEntry(ownerAT), true, ownerA, EffectNone, false},
		{"C3 same clone different track with override", ownerA, withOverride(acquireEntry(ownerAT), Owner{}, "user asked"), false, ownerAT, EffectTakeover, false},
		{"C4 same owner refreshes", ownerA, acquireEntry(ownerA), false, ownerA, EffectRefreshed, false},
		{"C5 same track name, other person", ownerB, acquireEntry(ownerA), true, ownerB, EffectNone, false},
		{"C8 changed email is a different email", ownerA, acquireEntry(Owner{Email: "new@example.com", CloneID: "ca", Track: "develop"}), true, ownerA, EffectNone, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			idx := leased(tc.holder, PhaseActive)
			before := idx.Clone()
			a := &Applier{Now: later}
			out := a.Live(idx, tc.entry)
			if (out.Refusal != nil) != tc.refused {
				t.Fatalf("refusal = %v, want refused=%v", out.Refusal, tc.refused)
			}
			if tc.refused {
				if !equalIndex(idx, before) {
					t.Fatal("a refusal changed the index")
				}
				if !strings.Contains(out.Refusal.Message, tc.holder.Email) || !strings.Contains(out.Refusal.Message, "dangerously_override_lease_status") {
					t.Fatalf("refusal must name the holder and the override flag: %s", out.Refusal.Message)
				}
				return
			}
			if got := holderOf(t, idx); got != tc.wantHolder {
				t.Fatalf("holder = %v, want %v", got, tc.wantHolder)
			}
			if out.Effect != tc.effect {
				t.Fatalf("effect = %q, want %q", out.Effect, tc.effect)
			}
			if (out.Warning != "") != tc.warn {
				t.Fatalf("warning = %q, want warn=%v", out.Warning, tc.warn)
			}
			if !idx.Registrations[stemX].Lease.TouchedAt.Equal(later) {
				t.Fatalf("touched_at = %v, want %v", idx.Registrations[stemX].Lease.TouchedAt, later)
			}
			if tc.effect == EffectTakeover {
				audit := strings.Join(out.Audit, "\n")
				if !strings.Contains(audit, tc.holder.Email) {
					t.Fatalf("takeover audit must name the previous holder: %s", audit)
				}
				if tc.entry.Override != nil && !strings.Contains(audit, "user asked") {
					t.Fatalf("override audit must carry the reason: %s", audit)
				}
			}
		})
	}
}

// D2 at library level: the holder's own impl branch records only impl.
func TestAcquireImplRecord(t *testing.T) {
	idx := leased(ownerA, PhaseActive)
	e := acquireEntry(ownerA)
	e.ImplBranch = "impl/develop/260924-feat-x"
	out := (&Applier{Now: t0}).Live(idx, e)
	if out.Refusal != nil || out.Effect != EffectImplRecorded {
		t.Fatalf("outcome = %+v", out)
	}
	lease := idx.Registrations[stemX].Lease
	if lease.Owner() != ownerA || lease.Impl == nil || lease.Impl.Branch != e.ImplBranch {
		t.Fatalf("lease = %+v", lease)
	}
	// The same branch under another identity goes through the matrix.
	other := acquireEntry(ownerB)
	other.ImplBranch = e.ImplBranch
	if out := (&Applier{Now: t0}).Live(idx, other); out.Refusal == nil {
		t.Fatal("impl record by a different email must be refused")
	}
}

// C7 and I13: release removes only the caller's own lease.
func TestReleaseOwnership(t *testing.T) {
	idx := leased(ownerA, PhaseActive)
	rel := PendingEntry{Op: OpRelease, Stem: stemX, Owner: ownerB}
	if out := (&Applier{Now: t0}).Live(idx, rel); out.Refusal == nil {
		t.Fatal("releasing another owner's lease must be refused")
	}
	if audit, report := (&Applier{Now: t0}).Replay(idx, rel); report == "" || audit != nil {
		t.Fatalf("a taken-over release replay must drop with a note, got audit %v report %q", audit, report)
	}
	if holderOf(t, idx) != ownerA {
		t.Fatal("a refused release touched the lease")
	}
	rel.Owner = ownerA
	if out := (&Applier{Now: t0}).Live(idx, rel); out.Refusal != nil || out.Effect != EffectReleased {
		t.Fatalf("own release: %+v", out)
	}
	if idx.Registrations[stemX] == nil || idx.Registrations[stemX].Lease != nil {
		t.Fatal("release must keep the registration and drop the lease")
	}
}

// E2 and the close effect on an existing lease.
func TestCloseLease(t *testing.T) {
	idx := NewIndex()
	out := (&Applier{Now: t0}).Live(idx, PendingEntry{Op: OpClose, Stem: stemX, Owner: ownerA})
	if out.Effect != EffectClosed || holderOf(t, idx) != ownerA || idx.Registrations[stemX].Lease.Phase != PhaseClosed {
		t.Fatalf("close on unleased: %+v %+v", out, idx.Registrations[stemX])
	}
	idx = leased(ownerA, PhaseActive)
	(&Applier{Now: t0}).Live(idx, PendingEntry{Op: OpClose, Stem: stemX, Owner: ownerAT})
	if holderOf(t, idx) != ownerA || idx.Registrations[stemX].Lease.Phase != PhaseClosed {
		t.Fatal("close must set the holder's lease closed without transferring it")
	}
}

// I2 and I12: a replayed override applies only against the holder it recorded.
func TestReplayOverride(t *testing.T) {
	idx := leased(ownerB, PhaseActive)
	e := withOverride(acquireEntry(ownerA), ownerB, "user asked")
	if _, report := (&Applier{Now: t0}).Replay(idx, e); report != "" {
		t.Fatalf("override against the recorded holder must apply: %s", report)
	}
	if holderOf(t, idx) != ownerA {
		t.Fatal("override did not take over")
	}
	third := Owner{Email: "z@example.com", CloneID: "cz", Track: "develop"}
	idx = leased(third, PhaseActive)
	_, report := (&Applier{Now: t0}).Replay(idx, e)
	if report == "" || !strings.Contains(report, "z@example.com") {
		t.Fatalf("override against another holder must drop and name the holder: %q", report)
	}
	if holderOf(t, idx) != third {
		t.Fatal("dropped override changed the lease")
	}
}

// I4 and I16: a landed stem refuses a replayed acquire, and resolves close
// and registration silently through pruning.
func TestReplayLanded(t *testing.T) {
	a := &Applier{Now: t0, Closed: map[string]bool{stemX: true}}
	idx := NewIndex()
	idx.Register(stemX, t0)
	if _, report := a.Replay(idx, acquireEntry(ownerA)); !strings.Contains(report, "closed on origin") {
		t.Fatalf("landed acquire replay report = %q", report)
	}
	for _, op := range []string{OpClose, OpRegister, OpRelease} {
		if _, report := a.Replay(idx, PendingEntry{Op: op, Stem: stemX, Owner: ownerA}); report != "" {
			t.Fatalf("%s on a landed stem must be silent, got %q", op, report)
		}
	}
	a.Maintain(idx)
	if _, ok := idx.Registrations[stemX]; ok {
		t.Fatal("landed registration was not pruned")
	}
}

// E6 and E3: the GC predicates.
func TestMaintainGC(t *testing.T) {
	now := t0.Add(60 * 24 * time.Hour)
	old := t0
	build := func() *Index {
		idx := NewIndex()
		for _, s := range []string{"260101-feat-stale", "260101-feat-open-elsewhere", "260101-feat-leased", "260101-feat-closed-lease"} {
			idx.Register(s, old)
		}
		idx.Register("260101-feat-young", now.Add(-24*time.Hour))
		idx.Registrations["260101-feat-leased"].Lease = newLease(ownerA, PhaseActive, old)
		idx.Registrations["260101-feat-closed-lease"].Lease = newLease(ownerA, PhaseClosed, old)
		return idx
	}
	open := func(stem string) (bool, error) { return stem == "260101-feat-open-elsewhere", nil }

	idx := build()
	a := &Applier{Now: now, OpenAnywhere: open}
	a.Maintain(idx)
	want := []string{"260101-feat-closed-lease", "260101-feat-leased", "260101-feat-open-elsewhere", "260101-feat-young"}
	if got := idx.Stems(); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("after GC stems = %v, want %v", got, want)
	}
	if idx.Meta.LastGC == nil || !idx.Meta.LastGC.Equal(now) {
		t.Fatalf("last_gc = %v", idx.Meta.LastGC)
	}

	// last_gc younger than 30 days: no GC run.
	idx = build()
	recent := now.Add(-24 * time.Hour)
	idx.Meta.LastGC = &recent
	a.Maintain(idx)
	if len(idx.Stems()) != 5 {
		t.Fatalf("GC ran inside its period: %v", idx.Stems())
	}
	// A failed branch inventory prunes nothing and leaves last_gc unset.
	idx = build()
	(&Applier{Now: now, OpenAnywhere: func(string) (bool, error) { return false, errors.New("fetch failed") }}).Maintain(idx)
	if len(idx.Stems()) != 5 || idx.Meta.LastGC != nil {
		t.Fatal("GC advanced or pruned after a failed branch inventory")
	}
	// No predicate (offline or unknown branches): no GC.
	idx = build()
	(&Applier{Now: now}).Maintain(idx)
	if len(idx.Stems()) != 5 || idx.Meta.LastGC != nil {
		t.Fatal("GC ran without an open-anywhere predicate")
	}
}

func TestNeedsOverride(t *testing.T) {
	cases := []struct {
		op     string
		holder Owner
		caller Owner
		want   bool
	}{
		{OpAcquire, ownerA, ownerB, true},
		{OpAcquire, ownerA, ownerAT, true},
		{OpAcquire, ownerA, ownerA2, false},
		{OpAcquire, ownerA, ownerA, false},
		{OpClose, ownerA, ownerB, true},
		{OpClose, ownerA, ownerAT, false},
		{OpClose, ownerA, ownerA2, false},
		// A move registers; like close, only a different email needs one.
		{OpRegister, ownerA, ownerB, true},
		{OpRegister, ownerA, ownerAT, false},
		{OpRegister, ownerA, ownerA2, false},
		{OpRegister, ownerA, ownerA, false},
	}
	for _, tc := range cases {
		if got := NeedsOverride(tc.op, tc.holder, tc.caller); got != tc.want {
			t.Errorf("NeedsOverride(%s, %v, %v) = %v, want %v", tc.op, tc.holder, tc.caller, got, tc.want)
		}
	}
}

// A replayed move override applies only against the holder it recorded.
func TestReplayMoveOverride(t *testing.T) {
	e := PendingEntry{Op: OpRegister, Stem: stemX, Owner: ownerA, Override: &Override{Holder: ownerB, Reason: "user asked"}}
	idx := leased(ownerB, PhaseActive)
	if audit, report := (&Applier{Now: t0}).Replay(idx, e); report != "" || len(audit) != 1 || !strings.Contains(audit[0], "user asked") {
		t.Fatalf("override against the recorded holder: audit %v report %q", audit, report)
	}
	third := Owner{Email: "z@example.com", CloneID: "cz", Track: "develop"}
	idx = leased(third, PhaseActive)
	if _, report := (&Applier{Now: t0}).Replay(idx, e); !strings.Contains(report, "z@example.com") {
		t.Fatalf("override against another holder must drop and name it: %q", report)
	}
	idx = NewIndex()
	idx.Register(stemX, t0)
	if audit, report := (&Applier{Now: t0}).Replay(idx, e); report != "" || len(audit) != 0 {
		t.Fatalf("override of a released lease must be a plain registration: audit %v report %q", audit, report)
	}
}

// Live close never changes a different person's lease without an override
// naming that holder: the lease stays, and the outcome warns instead of
// refusing.
func TestLiveCloseLeavesDifferentEmailLease(t *testing.T) {
	third := Owner{Email: "z@example.com", CloneID: "cz", Track: "develop"}
	for name, e := range map[string]PendingEntry{
		"no override":                 {Op: OpClose, Stem: stemX, Owner: ownerA},
		"override naming another one": withOverride(PendingEntry{Op: OpClose, Stem: stemX, Owner: ownerA}, third, "user closed it"),
	} {
		idx := leased(ownerB, PhaseActive)
		out := (&Applier{Now: t0}).Live(idx, e)
		if out.Refusal != nil || len(out.Audit) != 0 || !strings.Contains(out.Warning, "b@example.com") {
			t.Fatalf("%s: outcome = %+v, want a warning naming the holder and no audit", name, out)
		}
		if named := strings.Contains(out.Warning, "the override named z@example.com"); named != (e.Override != nil) {
			t.Fatalf("%s: warning = %q, want it to name the stale override holder exactly when one was given", name, out.Warning)
		}
		if l := idx.Registrations[stemX].Lease; l.Owner() != ownerB || l.Phase != PhaseActive {
			t.Fatalf("%s: the holder's lease changed: %+v", name, l)
		}
	}
	idx := leased(ownerB, PhaseActive)
	out := (&Applier{Now: t0}).Live(idx, withOverride(PendingEntry{Op: OpClose, Stem: stemX, Owner: ownerA}, ownerB, "user closed it"))
	if out.Warning != "" || idx.Registrations[stemX].Lease.Phase != PhaseClosed || len(out.Audit) != 1 {
		t.Fatalf("an override naming the holder must close its lease with an audit line: %+v", out)
	}
	// Closing a lease another person already closed is a silent no-op.
	idx = leased(ownerB, PhaseClosed)
	if out := (&Applier{Now: t0}).Live(idx, PendingEntry{Op: OpClose, Stem: stemX, Owner: ownerA}); out.Warning != "" || len(out.Audit) != 0 {
		t.Fatalf("close of an already-closed lease = %+v, want a silent no-op", out)
	}
}

// C3: a close of an already-closed lease writes no audit line, with or
// without an override, live or replayed.
func TestCloseOfClosedLeaseIsNoOp(t *testing.T) {
	for _, e := range []PendingEntry{
		withOverride(PendingEntry{Op: OpClose, Stem: stemX, Owner: ownerA}, ownerB, "user closed it"),
		{Op: OpClose, Stem: stemX, Owner: ownerA}, // a different email, no override: still no report
	} {
		assertCloseOfClosedIsNoOp(t, e)
	}
}

func assertCloseOfClosedIsNoOp(t *testing.T, e PendingEntry) {
	t.Helper()
	for _, replay := range []bool{false, true} {
		idx := leased(ownerB, PhaseClosed)
		a := &Applier{Now: t0}
		var audit []string
		if replay {
			var report string
			audit, report = a.Replay(idx, e)
			if report != "" {
				t.Fatalf("replay report = %q", report)
			}
		} else {
			audit = a.Live(idx, e).Audit
		}
		if len(audit) != 0 {
			t.Fatalf("replay=%v: audit = %v, want none", replay, audit)
		}
	}
}

// C4: a replayed takeover from the same email on another clone reports the
// takeover warning, under the ticket-index: prefix the conflict reports in the
// same slot carry.
func TestReplayKeepsTakeoverWarning(t *testing.T) {
	idx := leased(ownerA, PhaseActive)
	e := acquireEntry(ownerA2)
	audit, report := (&Applier{Now: t0}).Replay(idx, e)
	if len(audit) != 1 || !strings.HasPrefix(report, "ticket-index: replayed the offline acquire of "+stemX) || !strings.Contains(report, "another clone") {
		t.Fatalf("replayed takeover = %v, %q", audit, report)
	}
	if holderOf(t, idx) != ownerA2 {
		t.Fatal("the replayed takeover did not move the lease")
	}
}

// An override move's audit line carries its pending entry id when replayed.
func TestOverrideMoveAuditCarriesEntryID(t *testing.T) {
	idx := leased(ownerB, PhaseActive)
	e := withOverride(PendingEntry{ID: "0123abcd", Op: OpRegister, Stem: stemX, Owner: ownerA}, ownerB, "user moved it")
	audit, _ := (&Applier{Now: t0}).Replay(idx, e)
	if len(audit) != 1 || !strings.Contains(audit[0], "(pending entry 0123abcd)") {
		t.Fatalf("audit = %v", audit)
	}
}
