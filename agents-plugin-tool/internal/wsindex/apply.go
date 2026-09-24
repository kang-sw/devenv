package wsindex

import (
	"fmt"
	"strings"
	"time"
)

// GCPeriod is both the GC cadence and the registration age GC requires.
const GCPeriod = 30 * 24 * time.Hour

// Relation is how a lease holder relates to a caller.
type Relation int

const (
	RelSelf Relation = iota
	RelDifferentEmail
	RelSameEmailDifferentClone
	RelSameCloneDifferentTrack
)

// SameOwner reports whether two triples are the same owner.
func SameOwner(a, b Owner) bool {
	return sameEmail(a.Email, b.Email) && a.CloneID == b.CloneID && a.Track == b.Track && a.Track != ""
}

func sameEmail(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

// Relate classifies holder against caller by the owner-conflict matrix rows.
// A caller with no resolvable track never matches a lease.
func Relate(holder, caller Owner) Relation {
	switch {
	case !sameEmail(holder.Email, caller.Email):
		return RelDifferentEmail
	case holder.CloneID != caller.CloneID:
		return RelSameEmailDifferentClone
	case holder.Track != caller.Track || caller.Track == "":
		return RelSameCloneDifferentTrack
	default:
		return RelSelf
	}
}

// NeedsOverride reports whether op by caller against holder is refused
// without the override flag: acquire in the different-email and
// same-clone-different-track rows, move and close only in the different-email
// row.
func NeedsOverride(op string, holder, caller Owner) bool {
	if SameOwner(holder, caller) {
		return false
	}
	switch Relate(holder, caller) {
	case RelDifferentEmail:
		return true
	case RelSameCloneDifferentTrack:
		return op == OpAcquire
	}
	return false
}

// RefusalError is an operation the owner-conflict matrix (or the origin-closed
// check, or release ownership) refused.
type RefusalError struct {
	Stem    string
	Message string
}

func (e *RefusalError) Error() string { return e.Message }

// Outcome is the effect of one live operation.
type Outcome struct {
	Refusal *RefusalError
	Effect  Effect
	Warning string
	Audit   []string
}

// Effect names what an applied operation did to the stem's record.
type Effect string

const (
	EffectNone         Effect = ""              // nothing (for example a stem already landed on origin)
	EffectRegistered   Effect = "registered"    // registration ensured
	EffectAcquired     Effect = "acquired"      // a new lease for the caller
	EffectTakeover     Effect = "takeover"      // the lease moved to the caller
	EffectRefreshed    Effect = "refreshed"     // the caller already held it; touched_at refreshed
	EffectImplRecorded Effect = "impl_recorded" // the caller already held it; impl branch recorded
	EffectReleased     Effect = "released"      // the caller's lease removed
	EffectNotLeased    Effect = "not_leased"    // release of a stem with no lease
	EffectClosed       Effect = "closed"        // the lease is phase closed
)

// Applier applies index operations with the context one write carries.
type Applier struct {
	Now time.Time
	// Closed holds stems whose ticket sits under .done/ or .dropped/ on the
	// origin review-track tree the write evaluated. nil means none known.
	Closed map[string]bool
	// OpenAnywhere reports whether stem is an open ticket on any origin
	// branch. nil disables GC for this write.
	OpenAnywhere func(stem string) bool
}

func holderString(l *Lease) string {
	return fmt.Sprintf("%s (track %s, clone %s)", l.Email, l.Track, l.CloneID)
}

func stamp(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func (a *Applier) entryTime(e PendingEntry) time.Time {
	if !e.RecordedAt.IsZero() {
		return e.RecordedAt.UTC().Truncate(time.Second)
	}
	return a.Now.UTC().Truncate(time.Second)
}

func newLease(owner Owner, phase string, at time.Time) *Lease {
	return &Lease{Email: owner.Email, CloneID: owner.CloneID, Track: owner.Track, Phase: phase, TouchedAt: at}
}

// ClosedMessage is the refusal for a stem already closed on the review-track.
func ClosedMessage(stem string) string {
	return fmt.Sprintf("%s is already closed on origin (under .done/ or .dropped/ on the review-track); this checkout is stale, pull before acting on it", stem)
}

// Live applies one operation as the calling tool performs it. A refusal
// leaves idx unchanged.
func (a *Applier) Live(idx *Index, e PendingEntry) Outcome {
	return a.apply(idx, e, false)
}

// Replay applies one pending entry during a flush. The remote wins: a
// conflicting entry is dropped (idx unchanged) and reported.
func (a *Applier) Replay(idx *Index, e PendingEntry) ([]string, string) {
	out := a.apply(idx, e, true)
	if out.Refusal != nil {
		return nil, out.Refusal.Message
	}
	return out.Audit, ""
}

func (a *Applier) apply(idx *Index, e PendingEntry, replay bool) Outcome {
	at := a.entryTime(e)
	if a.Closed[e.Stem] && e.Op != OpAcquire {
		// Registration, release, and close of a stem that already landed
		// resolve through pruning: the normal lifecycle, not a conflict.
		return Outcome{}
	}
	switch e.Op {
	case OpRegister:
		idx.Register(e.Stem, at)
		return Outcome{Effect: EffectRegistered}
	case OpAcquire:
		return a.acquire(idx, e, at, replay)
	case OpRelease:
		return a.release(idx, e, replay)
	case OpClose:
		return a.close(idx, e, at, replay)
	}
	return Outcome{Refusal: &RefusalError{Stem: e.Stem, Message: fmt.Sprintf("%s: unsupported ticket-index operation %q", e.Stem, e.Op)}}
}

func (a *Applier) conflict(e PendingEntry, holder *Lease) *RefusalError {
	where := "track " + e.Owner.Track
	if e.Worktree != "" {
		where += " (" + e.Worktree + ")"
	}
	if holder == nil {
		return &RefusalError{Stem: e.Stem, Message: fmt.Sprintf(
			"ticket-index: dropped the offline %s of %s recorded on %s: the lease it overrode was released meanwhile",
			e.Op, e.Stem, where)}
	}
	return &RefusalError{Stem: e.Stem, Message: fmt.Sprintf(
		"ticket-index: dropped the offline %s of %s recorded on %s: it is now held by %s since %s",
		e.Op, e.Stem, where, holderString(holder), stamp(holder.TouchedAt))}
}

func (a *Applier) acquire(idx *Index, e PendingEntry, at time.Time, replay bool) Outcome {
	if a.Closed[e.Stem] {
		msg := ClosedMessage(e.Stem)
		if replay {
			msg = fmt.Sprintf("ticket-index: dropped the offline acquire of %s recorded on track %s: %s", e.Stem, e.Owner.Track, msg)
		}
		return Outcome{Refusal: &RefusalError{Stem: e.Stem, Message: msg}}
	}
	idx.Register(e.Stem, at)
	reg := idx.Registrations[e.Stem]
	lease := reg.Lease
	caller := e.Owner
	if lease == nil {
		if replay && e.Override != nil {
			return Outcome{Refusal: a.conflict(e, nil)}
		}
		reg.Lease = newLease(caller, PhaseActive, at)
		if e.ImplBranch != "" {
			reg.Lease.Impl = &Impl{Branch: e.ImplBranch}
		}
		return Outcome{Effect: EffectAcquired, Audit: []string{fmt.Sprintf("acquire %s by %s", e.Stem, caller)}}
	}
	holder := lease.Owner()
	if SameOwner(holder, caller) {
		lease.TouchedAt = at
		if e.ImplBranch != "" {
			lease.Impl = &Impl{Branch: e.ImplBranch}
			return Outcome{Effect: EffectImplRecorded}
		}
		return Outcome{Effect: EffectRefreshed}
	}
	takeover := func(kind string) Outcome {
		prev := *lease
		reg.Lease = newLease(caller, PhaseActive, at)
		if e.ImplBranch != "" {
			reg.Lease.Impl = &Impl{Branch: e.ImplBranch}
		}
		audit := fmt.Sprintf("takeover %s (%s): previous holder %s, new holder %s", e.Stem, kind, holderString(&prev), caller)
		if e.Override != nil {
			audit += fmt.Sprintf(", reason: %s", e.Override.Reason)
		}
		out := Outcome{Effect: EffectTakeover, Audit: []string{audit}}
		if kind == "same email, different clone" {
			out.Warning = fmt.Sprintf("%s was held by your identity on another clone (%s); the lease moved to this track", e.Stem, holderString(&prev))
		}
		return out
	}
	if replay && e.Override != nil {
		if SameOwner(holder, e.Override.Holder) {
			return takeover("override")
		}
		return Outcome{Refusal: a.conflict(e, lease)}
	}
	switch Relate(holder, caller) {
	case RelSameEmailDifferentClone:
		return takeover("same email, different clone")
	default:
		if !replay && e.Override != nil {
			return takeover("override")
		}
		if replay {
			return Outcome{Refusal: a.conflict(e, lease)}
		}
		return Outcome{Refusal: &RefusalError{Stem: e.Stem, Message: fmt.Sprintf(
			"%s is held by %s since %s; acquiring it needs dangerously_override_lease_status: true with a non-empty reason, set only on the user's explicit instruction",
			e.Stem, holderString(lease), stamp(lease.TouchedAt))}}
	}
}

func (a *Applier) release(idx *Index, e PendingEntry, replay bool) Outcome {
	reg := idx.Registrations[e.Stem]
	if reg == nil || reg.Lease == nil {
		return Outcome{Effect: EffectNotLeased}
	}
	if !SameOwner(reg.Lease.Owner(), e.Owner) {
		if replay {
			return Outcome{Refusal: &RefusalError{Stem: e.Stem, Message: fmt.Sprintf(
				"ticket-index: dropped the offline release of %s: the lease was taken over meanwhile by %s", e.Stem, holderString(reg.Lease))}}
		}
		return Outcome{Refusal: &RefusalError{Stem: e.Stem, Message: fmt.Sprintf(
			"%s is held by %s, not by this track; releasing another owner's lease is refused (taking it over is an acquire)", e.Stem, holderString(reg.Lease))}}
	}
	reg.Lease = nil
	return Outcome{Effect: EffectReleased, Audit: []string{fmt.Sprintf("release %s by %s", e.Stem, e.Owner)}}
}

func (a *Applier) close(idx *Index, e PendingEntry, at time.Time, replay bool) Outcome {
	idx.Register(e.Stem, at)
	reg := idx.Registrations[e.Stem]
	if reg.Lease == nil {
		if replay && e.Override != nil {
			return Outcome{Refusal: a.conflict(e, nil)}
		}
		if e.Owner.Email == "" || e.Owner.Track == "" {
			return Outcome{Effect: EffectRegistered}
		}
		reg.Lease = newLease(e.Owner, PhaseClosed, at)
		return Outcome{Effect: EffectClosed, Audit: []string{fmt.Sprintf("close %s: closed lease created for %s", e.Stem, e.Owner)}}
	}
	holder := reg.Lease.Owner()
	if replay && !SameOwner(holder, e.Owner) {
		if e.Override != nil && !SameOwner(holder, e.Override.Holder) {
			return Outcome{Refusal: a.conflict(e, reg.Lease)}
		}
		if e.Override == nil && Relate(holder, e.Owner) == RelDifferentEmail {
			return Outcome{Refusal: a.conflict(e, reg.Lease)}
		}
	}
	if reg.Lease.Phase == PhaseClosed && e.Override == nil {
		return Outcome{Effect: EffectClosed}
	}
	reg.Lease.Phase = PhaseClosed
	audit := fmt.Sprintf("close %s: lease of %s set closed", e.Stem, holderString(reg.Lease))
	if e.Override != nil {
		audit += fmt.Sprintf(" by %s under override, reason: %s", e.Owner, e.Override.Reason)
	}
	return Outcome{Effect: EffectClosed, Audit: []string{audit}}
}

// Maintain runs landed-closure pruning, and GC when due, on idx. It is part
// of every index write's CAS commit.
func (a *Applier) Maintain(idx *Index) []string {
	var audit []string
	for _, stem := range idx.Stems() {
		if a.Closed[stem] {
			delete(idx.Registrations, stem)
			audit = append(audit, "prune landed "+stem)
		}
	}
	if a.OpenAnywhere == nil {
		return audit
	}
	now := a.Now.UTC().Truncate(time.Second)
	if idx.Meta.LastGC != nil && now.Sub(*idx.Meta.LastGC) < GCPeriod {
		return audit
	}
	for _, stem := range idx.Stems() {
		reg := idx.Registrations[stem]
		if reg.Lease != nil || now.Sub(reg.RegisteredAt) <= GCPeriod || a.OpenAnywhere(stem) {
			continue
		}
		delete(idx.Registrations, stem)
		audit = append(audit, "gc "+stem)
	}
	idx.Meta.LastGC = &now
	return audit
}

// GCDue reports whether a write at now would run GC on idx.
func GCDue(idx *Index, now time.Time) bool {
	return idx.Meta.LastGC == nil || now.Sub(*idx.Meta.LastGC) >= GCPeriod
}

// Apply returns a Replayer for plain registration replay, used where no
// review-track context is needed.
func Apply(now time.Time) Replayer {
	a := &Applier{Now: now}
	return a.Replay
}
