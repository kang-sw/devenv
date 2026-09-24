package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdoc"
	"github.com/kang-sw/devenv/internal/wsindex"
)

// Phase 3 scenarios: the ownership view on tickets.query and git.status, the
// ownership filter, and the move/close guards.

// query runs a discovery listing of the open statuses.
func (c *ixCheckout) query(extra ...any) string {
	c.env.t.Helper()
	args := map[string]any{"statuses": []any{"idea", "todo", "ready"}}
	for i := 0; i+1 < len(extra); i += 2 {
		args[extra[i].(string)] = extra[i+1]
	}
	return c.mustCall("tickets.query", args)
}

// queryJSON returns the discovery listing as ticket infos keyed by stem.
func (c *ixCheckout) queryJSON(extra ...any) map[string]wsdoc.TicketInfo {
	c.env.t.Helper()
	text := c.query(append([]any{"format", "json"}, extra...)...)
	var tickets []wsdoc.TicketInfo
	if err := json.Unmarshal([]byte(text), &tickets); err != nil {
		c.env.t.Fatalf("query json: %v\n%s", err, text)
	}
	out := map[string]wsdoc.TicketInfo{}
	for _, t := range tickets {
		out[t.Stem] = t
	}
	return out
}

// resolve point-resolves one stem as JSON.
func (c *ixCheckout) resolve(stem string) wsdoc.TicketInfo {
	c.env.t.Helper()
	text := c.mustCall("tickets.query", map[string]any{"ticket_stem": stem, "format": "json"})
	var info wsdoc.TicketInfo
	if err := json.Unmarshal([]byte(text), &info); err != nil {
		c.env.t.Fatalf("resolve json: %v\n%s", err, text)
	}
	return info
}

func level(t wsdoc.TicketInfo) string {
	if t.Ownership == nil {
		return ""
	}
	return t.Ownership.Level
}

func keys(m map[string]wsdoc.TicketInfo) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

// ---- A, F: the no-ref path and the read-path cache ------------------------------

// A1 (with the move/close override-param clause), A2, A3: without an index
// ref every ticket tool prints exactly what a no-origin checkout prints, the
// override params change nothing, discovery is asked at most once per
// absence TTL, and a checkout with no origin makes no remote call.
func TestNoRefPathIsByteIdentical(t *testing.T) {
	e := newIxEnv(t)
	withOrigin := e.clone("x", "x@example.com")
	noOrigin := e.clone("n", "x@example.com")
	noOrigin.git("remote", "remove", "origin")
	for _, c := range []*ixCheckout{withOrigin, noOrigin} {
		c.git("checkout", "--quiet", "-b", "scratch")
		c.runner.reset()
	}
	override := []any{"dangerously_override_lease_status", true, "reason", "probe"}
	steps := []struct {
		tool string
		args map[string]any
	}{
		{"tickets.query", map[string]any{"statuses": []any{"idea", "todo", "ready"}}},
		{"tickets.query", map[string]any{"statuses": []any{"idea", "todo", "ready"}, "unleased_or_mine": true}},
		{"tickets.query", map[string]any{"statuses": []any{"ready"}, "format": "json", "unleased_or_mine": true}},
		{"tickets.query", map[string]any{"ticket_stem": stemAlpha}},
		{"tickets.move", map[string]any{"stem": stemGamma, "to": "idea"}},
		{"tickets.move", ixArgs(stemGamma, append([]any{"to", "todo"}, override...)...)},
		{"tickets.close", map[string]any{"stem": stemAlpha, "status": "done"}},
		{"tickets.close", ixArgs(stemBeta, append([]any{"status", "dropped"}, override...)...)},
	}
	for _, step := range steps {
		// ixArgs keys the stem as ticket_stem; move and close take stem.
		if v, ok := step.args["ticket_stem"]; ok && step.tool != "tickets.query" {
			step.args["stem"] = v
			delete(step.args, "ticket_stem")
		}
		want := noOrigin.mustCall(step.tool, step.args)
		got := withOrigin.mustCall(step.tool, step.args)
		if got != want {
			t.Fatalf("%s(%v) differs with an index-absent origin:\n%s\nwant:\n%s", step.tool, step.args, got, want)
		}
		if strings.Contains(got, "ticket-index") || strings.Contains(got, "ownership") {
			t.Fatalf("%s leaked index text: %s", step.tool, got)
		}
	}
	if n := withOrigin.runner.count("ls-remote"); n > 1 || withOrigin.runner.total() != n {
		t.Fatalf("A2: remote calls within the absence TTL = %v", withOrigin.runner.counts)
	}
	if n := noOrigin.runner.total(); n != 0 {
		t.Fatalf("A3: a checkout without origin made %d remote calls", n)
	}
}

// F1, F2, I8: within the TTL query makes no remote call; past it an
// unreachable origin serves the stale cache with its age inside the bound;
// pending entries show a marker offline and online, and an online query
// never pushes them.
func TestQueryReadPathCache(t *testing.T) {
	e := newIxEnv(t)
	x := e.clone("x", "a@example.com")
	x.init()
	x.acquire(stemAlpha)

	x.runner.reset()
	out := x.query()
	if x.runner.total() != 0 {
		t.Fatalf("F1: query inside the TTL made remote calls: %v", x.runner.counts)
	}
	if !strings.Contains(out, "ownership: yours (track develop)") || strings.Contains(out, "ticket-index:") {
		t.Fatalf("fresh query = %s", out)
	}

	e.clock.Advance(2 * time.Minute)
	x.offline()
	start := time.Now()
	out = x.query()
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("F2: stale query took %s", elapsed)
	}
	if !strings.Contains(out, "ownership is from the cached index (age 2m0s)") || !strings.Contains(out, "ownership: yours") {
		t.Fatalf("F2: stale query = %s", out)
	}
	alpha := x.queryJSON()[stemAlpha]
	if alpha.Ownership == nil || alpha.Ownership.IndexState != "stale" || alpha.Ownership.CacheAgeSeconds != 120 {
		t.Fatalf("F2: stale json ownership = %+v", alpha.Ownership)
	}

	// I8.
	x.acquire(stemBeta)
	if out := x.query(); !strings.Contains(out, "ticket-index: 1 pending offline entries") {
		t.Fatalf("I8: offline query lacks the pending marker: %s", out)
	}
	x.online()
	tip := e.remoteTip()
	x.runner.reset()
	if out := x.query(); !strings.Contains(out, "ticket-index: 1 pending offline entries") {
		t.Fatalf("I8: online query lacks the pending marker: %s", out)
	}
	if n := x.runner.count("push"); n != 0 || e.remoteTip() != tip || x.pendingCount() != 1 {
		t.Fatalf("I8: an online query pushed (%d pushes, pending %d)", n, x.pendingCount())
	}
}

// F5: no cache, discovery sees the ref, and the fetch fails: ownership is
// unknown (never unowned), and the ownership filter keeps those tickets.
func TestQueryUnknownWithoutCache(t *testing.T) {
	e := newIxEnv(t)
	x := e.clone("x", "a@example.com")
	y := e.clone("y", "b@example.com")
	x.init()
	x.acquire(stemAlpha)
	y.runner.failFetch.Store(true)
	out := y.query()
	if !strings.Contains(out, "ownership: unknown") || strings.Contains(out, "held elsewhere") {
		t.Fatalf("F5 query = %s", out)
	}
	got := y.queryJSON("unleased_or_mine", true)
	for _, stem := range []string{stemAlpha, stemBeta, stemGamma} {
		if level(got[stem]) != wsdoc.OwnershipUnknown {
			t.Fatalf("F5: %s = %+v, want unknown and included", stem, got[stem].Ownership)
		}
	}
	if y.hasRef(ixCacheRef) {
		t.Fatal("F5: a failed fetch created a cache ref")
	}
}

// I23: a deleted remote ref with a cache present: a transport failure serves
// the stale cache; a successful read resolves absence and discards.
func TestQueryDeletedRefDiscards(t *testing.T) {
	e := newIxEnv(t)
	x := e.clone("x", "a@example.com")
	x.init()
	x.acquire(stemAlpha)
	x.offline()
	x.acquire(stemBeta) // one pending entry
	runGit(t, e.origin, "update-ref", "-d", wsindex.RemoteRef)
	e.clock.Advance(2 * time.Minute)
	if out := x.query(); !strings.Contains(out, "ownership is from the cached index") || !strings.Contains(out, "ownership: yours") {
		t.Fatalf("I23: transport failure did not serve the stale cache: %s", out)
	}
	x.online()
	out := x.query()
	if !strings.Contains(out, "discarded") || strings.Contains(out, "ownership:") {
		t.Fatalf("I23: deleted ref query = %s", out)
	}
	if x.hasRef(ixCacheRef) || x.pendingCount() != 0 {
		t.Fatal("I23: the discard left the cache ref or the pending log")
	}
	if again := x.query(); strings.Contains(again, "ticket-index") || strings.Contains(again, "ownership") {
		t.Fatalf("I23: after the discard query is not index-absent: %s", again)
	}
}

// ---- G, C5: levels, collapse, filter ---------------------------------------------

// G1, G2, C5: the three levels, the local worktree path (never in the
// index), compact collapse, JSON filter, point-resolve never collapsed, and
// two people on one track name never treated as one owner.
func TestOwnershipLevelsAndCollapse(t *testing.T) {
	e := newIxEnv(t)
	x := e.clone("x", "a@example.com")
	y := e.clone("y", "b@example.com") // same track name (develop), other person
	x.init()
	x.acquire(stemAlpha)
	xb := x.worktree("x-b", "track/b", "develop")
	xb.acquire(stemBeta)
	y.acquire(stemGamma)
	x.acquire(stemAlpha) // refresh x's cache after y's write

	out := x.query()
	full, collapsed, _ := strings.Cut(out, "held elsewhere or closed on origin (2):\n")
	if !strings.Contains(full, stemAlpha) || !strings.Contains(full, "ownership: yours (track develop)") {
		t.Fatalf("G1 self entry:\n%s", out)
	}
	if strings.Contains(full, stemBeta) || strings.Contains(full, stemGamma) {
		t.Fatalf("G2: tickets held elsewhere were not collapsed:\n%s", out)
	}
	if !strings.Contains(collapsed, "x-b (track track/b)") || !strings.Contains(collapsed, "held by another worktree of this clone") {
		t.Fatalf("G1 local level lacks the worktree path:\n%s", out)
	}
	if !strings.Contains(collapsed, "held by b@example.com (track develop)") {
		t.Fatalf("G1 remote level:\n%s", out)
	}
	blob := string(runGitOutput(t, e.origin, "cat-file", "blob", e.remoteTip()+":index.json")) + e.history()
	if strings.Contains(blob, "x-b") || strings.Contains(blob, e.dir) {
		t.Fatalf("G1: a local path reached the remote index:\n%s", blob)
	}

	// G2: the JSON filter keeps only own and unowned; point-resolve of a
	// ticket held elsewhere is a full entry.
	got := x.queryJSON("unleased_or_mine", true)
	if _, ok := got[stemAlpha]; !ok || len(got) != 1 {
		t.Fatalf("G2 filtered json = %v", keys(got))
	}
	all := x.queryJSON()
	if level(all[stemBeta]) != wsdoc.OwnershipLocal || level(all[stemGamma]) != wsdoc.OwnershipRemote {
		t.Fatalf("G1 json levels: beta %+v gamma %+v", all[stemBeta].Ownership, all[stemGamma].Ownership)
	}
	point := x.mustCall("tickets.query", map[string]any{"ticket_stem": stemGamma})
	if strings.Contains(point, "held elsewhere") || !strings.Contains(point, "ownership: held by b@example.com") {
		t.Fatalf("G2 point-resolve = %s", point)
	}

	// C5: y on develop is not x on develop, in view, filter, guard, release,
	// and impl record.
	if r := y.resolve(stemAlpha); level(r) != wsdoc.OwnershipRemote {
		t.Fatalf("C5 view = %+v", r.Ownership)
	}
	if _, ok := y.queryJSON("unleased_or_mine", true)[stemAlpha]; ok {
		t.Fatal("C5: the filter kept another person's lease on the same track name")
	}
	y.mustRefuse("tickets.move", map[string]any{"stem": stemAlpha, "to": "todo"}, "a@example.com")
	y.mustRefuse("tickets.release", ixArgs(stemAlpha), "a@example.com")
	y.git("checkout", "--quiet", "-b", "impl/develop/"+stemAlpha)
	y.mustRefuse("tickets.acquire", ixArgs(stemAlpha), "a@example.com")
	if l := e.lease(stemAlpha); l.Email != "a@example.com" || l.Impl != nil {
		t.Fatalf("C5 lease changed: %+v", l)
	}
}

// G3, G4, G5, E4, E5, E9: the queue filter drops others' leases (a closed
// lease counts as owned) and origin-closed tickets, shows the origin-closed
// hint only once the tracking ref knows, lists only local files, and
// composes with assigned_to_me.
func TestOwnershipFilterAndOriginHint(t *testing.T) {
	e := newIxEnv(t)
	x := e.clone("x", "a@example.com")
	y := e.clone("y", "b@example.com")
	x.init()
	y.mustCall("tickets.close", map[string]any{"stem": stemGamma, "status": "done"}) // closed lease, unmerged
	y.mustCall("tickets.create_empty", map[string]any{"stem": "feat-y-only", "initial_state": "idea"})
	matches, _ := filepath.Glob(filepath.Join(y.root, "ai-docs/tickets/idea/*-feat-y-only.md"))
	if len(matches) != 1 {
		t.Fatalf("created ticket not found: %v", matches)
	}
	yOnly := strings.TrimSuffix(filepath.Base(matches[0]), ".md")
	if _, ok := e.index().Registrations[yOnly]; !ok {
		t.Fatalf("%s was not registered", yOnly)
	}
	x.acquire(stemBeta)
	x.mustCall("tickets.close", map[string]any{"stem": stemBeta, "status": "done"}) // own closed lease
	x.git("reset", "--quiet", "--hard")                                             // keep beta open locally

	got := x.queryJSON("unleased_or_mine", true)
	if _, ok := got[stemGamma]; ok {
		t.Fatal("G3: another person's closed lease was not treated as owned")
	}
	if _, ok := got[stemBeta]; !ok || level(got[stemBeta]) != wsdoc.OwnershipSelf {
		t.Fatalf("G3: the caller's own closed lease was dropped: %v", keys(got))
	}
	if _, ok := x.queryJSON()[yOnly]; ok {
		t.Fatalf("G4: an indexed stem with no local file appeared (%q)", yOnly)
	}

	// E9: a locally deleted file disappears; its registration stays.
	if err := os.Remove(filepath.Join(x.root, "ai-docs/tickets/ready/"+stemAlpha+".md")); err != nil {
		t.Fatal(err)
	}
	if _, ok := x.queryJSON()[stemAlpha]; ok {
		t.Fatal("E9: a deleted local file still listed")
	}
	if _, ok := e.index().Registrations[stemAlpha]; !ok {
		t.Fatal("E9: the registration of a deleted file vanished")
	}
	x.git("checkout", "--", ".")

	// E4, E5: the hint follows the local tracking ref, with no network.
	e.landOnDevelop(stemAlpha, "ready", ".done")
	e.landOnDevelop(stemBeta, "ready", ".dropped")
	if r := x.resolve(stemAlpha); level(r) != wsdoc.OwnershipUnowned || r.Ownership.OriginClosed {
		t.Fatalf("E4: a stale tracking ref must show unowned: %+v", r.Ownership)
	}
	x.git("fetch", "--quiet", "origin")
	out := x.query()
	if !strings.Contains(out, "held elsewhere or closed on origin (3):") || strings.Count(out, "closed on origin (this checkout is stale; pull)") != 2 {
		t.Fatalf("E4/E5 hint:\n%s", out)
	}
	got = x.queryJSON("unleased_or_mine", true)
	if _, ok := got[stemAlpha]; ok {
		t.Fatal("E4: the filter kept a ticket closed on origin")
	}
	if _, ok := got[stemBeta]; ok {
		t.Fatal("E5: the filter kept a ticket dropped on origin")
	}

	// G5: composes with assigned_to_me.
	mustWrite(t, x.root, ".ws-workflow/config.json", `{"schema_version":1,"overrides":{"ticket-assignee-aware":"on"}}`+"\n")
	mustWrite(t, x.root, "ai-docs/tickets/ready/260105-feat-mine.md", "---\ntitle: Mine\nassignee:\n  - a@example.com\n---\n# Mine\n")
	mustWrite(t, x.root, "ai-docs/tickets/ready/260105-feat-bobs.md", "---\ntitle: Bobs\nassignee:\n  - bob@example.com\n---\n# Bobs\n")
	got = x.queryJSON("unleased_or_mine", true, "assigned_to_me", true)
	if _, ok := got["260105-feat-mine"]; !ok {
		t.Fatalf("G5: lost the caller's assigned unowned ticket: %v", keys(got))
	}
	if _, ok := got["260105-feat-bobs"]; ok {
		t.Fatal("G5: assigned_to_me did not apply alongside the ownership filter")
	}
	if _, ok := got[stemGamma]; ok {
		t.Fatal("G5: the ownership filter did not apply alongside assigned_to_me")
	}
}

// ---- I: the offline view --------------------------------------------------------

// I1, I3, I11 (view clauses), I14, I15: a pending acquire is a provisional
// lease resolved by its recording triple; the move/close guards evaluate the
// overlaid cache offline; after the flush the winner shows.
func TestOfflineView(t *testing.T) {
	e := newIxEnv(t)
	x := e.clone("x", "a@example.com")
	y := e.clone("y", "b@example.com")
	x.init()
	xb := x.worktree("x-b", "track/b", "develop")
	y.acquire(stemGamma)
	x.acquire(stemBeta) // x's cache now shows gamma held by y
	x.offline()
	x.acquire(stemAlpha)

	// I1 view clause.
	r := x.resolve(stemAlpha)
	if level(r) != wsdoc.OwnershipSelf || !r.Ownership.Provisional {
		t.Fatalf("I1: provisional view = %+v", r.Ownership)
	}
	if out := x.mustCall("tickets.query", map[string]any{"ticket_stem": stemAlpha}); !strings.Contains(out, "provisional: pending offline acquire") {
		t.Fatalf("I1 text = %s", out)
	}
	// I15, I11 view clause.
	if _, ok := x.queryJSON("unleased_or_mine", true)[stemAlpha]; !ok {
		t.Fatal("I15: the recording worktree's filter dropped its provisional lease")
	}
	if _, ok := xb.queryJSON("unleased_or_mine", true)[stemAlpha]; ok {
		t.Fatal("I15: a sibling worktree's filter kept another track's provisional lease")
	}
	if r := xb.resolve(stemAlpha); level(r) != wsdoc.OwnershipLocal || r.Ownership.Worktree == "" {
		t.Fatalf("I11: sibling view = %+v", r.Ownership)
	}

	// I14: the guards refuse a cached different-email lease offline.
	x.mustRefuse("tickets.move", map[string]any{"stem": stemGamma, "to": "idea"}, "b@example.com", "dangerously_override_lease_status")
	x.mustRefuse("tickets.close", map[string]any{"stem": stemGamma, "status": "done"}, "b@example.com")
	x.mustRefuse("tickets.close", map[string]any{"stem": stemGamma, "status": "done", "dangerously_override_lease_status": true}, "reason")
	before := x.pendingCount()
	x.mustCall("tickets.move", map[string]any{"stem": stemGamma, "to": "idea", "dangerously_override_lease_status": true, "reason": "user asked"})
	x.mustCall("tickets.close", map[string]any{"stem": stemGamma, "status": "done", "dangerously_override_lease_status": true, "reason": "user asked"})
	cl, err := wsindex.Open(bgCtx(), x.root, wsindex.Options{})
	if err != nil {
		t.Fatal(err)
	}
	pending, err := cl.Pending(bgCtx())
	if err != nil || len(pending) != before+2 {
		t.Fatalf("I14: pending = %d (%v), want %d", len(pending), err, before+2)
	}
	last := pending[len(pending)-1]
	if last.Op != wsindex.OpClose || last.Override == nil || last.Override.Holder.Email != "b@example.com" || last.Override.Reason != "user asked" {
		t.Fatalf("I14: close pending entry = %+v", last)
	}

	// I3 view clause: y wins alpha online; after x's flush x sees y.
	y.acquire(stemAlpha)
	x.online()
	x.acquire(stemBeta)
	if r := x.resolve(stemAlpha); level(r) != wsdoc.OwnershipRemote || r.Ownership.Email != "b@example.com" || r.Ownership.Provisional {
		t.Fatalf("I3: view after the flush = %+v", r.Ownership)
	}
	if l := e.lease(stemGamma); l.Email != "b@example.com" || l.Phase != wsindex.PhaseClosed {
		t.Fatalf("I14: the replayed close override moved the lease: %+v", l)
	}
}

// ---- C6: move/close guard --------------------------------------------------------

// C6: a different email needs flag + reason; another clone or track warns;
// the holder's lease never moves, and an override is audited.
func TestMoveCloseGuard(t *testing.T) {
	e := newIxEnv(t)
	x := e.clone("x", "a@example.com")
	x2 := e.clone("x2", "a@example.com")
	y := e.clone("y", "b@example.com")
	x.init()
	x.acquire(stemAlpha)
	x.acquire(stemBeta)
	x.acquire(stemGamma)
	xb := x.worktree("x-b", "track/b", "develop")

	y.mustRefuse("tickets.move", map[string]any{"stem": stemAlpha, "to": "todo"}, "a@example.com", "dangerously_override_lease_status")
	y.mustRefuse("tickets.move", map[string]any{"stem": stemAlpha, "to": "todo", "dangerously_override_lease_status": true}, "reason")
	y.mustRefuse("tickets.close", map[string]any{"stem": stemAlpha, "status": "done"}, "a@example.com")
	y.mustCall("tickets.move", map[string]any{"stem": stemAlpha, "to": "todo", "dangerously_override_lease_status": true, "reason": "user moved it"})
	if l := e.lease(stemAlpha); l.Email != "a@example.com" || l.Track != "develop" {
		t.Fatalf("C6: an override move transferred the lease: %+v", l)
	}
	history := e.history()
	if !strings.Contains(history, "b@example.com") || !strings.Contains(history, "user moved it") {
		t.Fatalf("C6: the override move is not audited:\n%s", history)
	}

	out := x2.mustCall("tickets.move", map[string]any{"stem": stemBeta, "to": "todo"})
	if !strings.Contains(out, "ticket-index: "+stemBeta+" is held by a@example.com") {
		t.Fatalf("C6 same-email warning = %s", out)
	}
	out = xb.mustCall("tickets.close", map[string]any{"stem": stemGamma, "status": "done"})
	if !strings.Contains(out, "ticket-index: "+stemGamma+" is held by a@example.com") {
		t.Fatalf("C6 same-clone warning = %s", out)
	}
	if l := e.lease(stemGamma); l.Track != "develop" || l.Phase != wsindex.PhaseClosed {
		t.Fatalf("C6: close must close the holder's lease in place: %+v", l)
	}
	if l := e.lease(stemBeta); l.Email != "a@example.com" || l.Track != "develop" {
		t.Fatalf("C6: a warned move transferred the lease: %+v", l)
	}
	y.mustCall("tickets.close", map[string]any{"stem": stemBeta, "status": "dropped", "dangerously_override_lease_status": true, "reason": "user dropped it"})
	if l := e.lease(stemBeta); l.Email != "a@example.com" || l.Phase != wsindex.PhaseClosed {
		t.Fatalf("C6: an override close moved the lease: %+v", l)
	}
	if !strings.Contains(e.history(), "user dropped it") {
		t.Fatal("C6: the override close is not audited")
	}
}

// git.status shows the active ticket's owner and the caller's leases from
// the cache only.
func TestGitStatusShowsOwner(t *testing.T) {
	e := newIxEnv(t)
	x := e.clone("x", "a@example.com")
	x.init()
	x.acquire(stemAlpha)
	implBranch := implTicketBranch("develop", stemAlpha)
	worker := x.worktree("x-impl", implBranch, "develop")
	worker.acquire(stemAlpha)
	worker.runner.reset()
	e.clock.Advance(time.Hour) // past the TTL: still no network
	out := worker.mustCall("git.status", nil)
	if worker.runner.total() != 0 {
		t.Fatalf("git.status made remote calls: %v", worker.runner.counts)
	}
	if !strings.Contains(out, "ticket owner: yours (track develop); impl "+implBranch) || !strings.Contains(out, "leased to this track: "+stemAlpha) {
		t.Fatalf("git.status = %s", out)
	}
	var status gitStatusResult
	if err := json.Unmarshal([]byte(worker.mustCall("git.status", map[string]any{"format": "json"})), &status); err != nil {
		t.Fatal(err)
	}
	if status.ImplTicket == nil || status.ImplTicket.Owner == nil || status.ImplTicket.Owner.Level != wsdoc.OwnershipSelf || len(status.Leases) != 1 {
		t.Fatalf("git.status json = %+v", status)
	}
}

// A8: a clone whose local AGENTS.md declares another review-track still
// resolves origin's for init, the acquire refusal, pruning, and the query hint.
func TestLocalReviewTrackIsIgnored(t *testing.T) {
	e := newIxEnv(t)
	x := e.clone("x", "a@example.com")
	mustWrite(t, x.root, "AGENTS.md", "# Project\n\n### Review Policy\n\n```text\nreview-track: main\n```\n")
	x.git("commit", "--quiet", "-am", "local track main")
	// develop gains a ticket main does not have.
	e.landOnDevelop(stemGamma, "todo", "idea")
	x.git("fetch", "--quiet", "origin")
	if out := x.init(); !strings.Contains(out, "review_track: develop") {
		t.Fatalf("A8 init = %s", out)
	}
	if _, ok := e.index().Registrations[stemGamma]; !ok {
		t.Fatal("A8: init did not register from origin's develop")
	}
	x.acquire(stemBeta)
	e.landOnDevelop(stemAlpha, "ready", ".done") // closed on develop only, never on main
	e.landOnDevelop(stemBeta, "ready", ".done")
	x.mustRefuse("tickets.acquire", ixArgs(stemAlpha), "already closed on origin")
	if r := x.resolve(stemAlpha); r.Ownership == nil || !r.Ownership.OriginClosed {
		t.Fatalf("A8 query hint = %+v", r.Ownership)
	}
	x.acquire(stemGamma) // the next write prunes beta, landed on develop
	if _, ok := e.index().Registrations[stemBeta]; ok {
		t.Fatal("A8: pruning did not follow origin's review-track")
	}
}

// I18 at tool level: a discard found by the move guard's read or by an
// acquire's write still reports once in that tool's output.
func TestDiscardReportsReachToolOutput(t *testing.T) {
	e := newIxEnv(t)
	x := e.clone("x", "a@example.com")
	y := e.clone("y", "b@example.com")
	x.init()
	x.acquire(stemAlpha)
	y.acquire(stemBeta)
	for _, c := range []*ixCheckout{x, y} {
		c.offline()
		c.acquire(stemGamma) // one pending entry each
		c.online()
	}
	runGit(t, e.origin, "update-ref", "-d", wsindex.RemoteRef)
	e.clock.Advance(2 * time.Minute)

	out := x.mustCall("tickets.move", map[string]any{"stem": stemGamma, "to": "idea"})
	if strings.Count(out, "discarded") != 1 {
		t.Fatalf("move after a remote deletion = %s", out)
	}
	out = y.acquire(stemAlpha)
	if !strings.HasPrefix(out, "status: ok\n") || strings.Count(out, "discarded") != 1 {
		t.Fatalf("acquire after a remote deletion = %s", out)
	}
	if again := y.acquire(stemAlpha); again != "ok" {
		t.Fatalf("the next acquire is not the plain index-absent ok: %q", again)
	}
}

// A never-seen clone whose discovery sees the ref but whose fetch fails
// refuses loudly instead of passing as index-absent, and caches no absence.
func TestDiscoveredButUnfetchedWriteFailsLoudly(t *testing.T) {
	e := newIxEnv(t)
	x := e.clone("x", "a@example.com")
	y := e.clone("y", "b@example.com")
	x.init()
	y.runner.failFetch.Store(true)
	y.mustRefuse("tickets.acquire", ixArgs(stemAlpha), "could not be fetched")
	y.runner.failFetch.Store(false)
	if out := y.acquire(stemAlpha); !strings.Contains(out, "status: acquired") {
		t.Fatalf("acquire after the fetch recovers = %s", out)
	}
}

// A seen clone whose read times out reports the view as not refreshed, with
// its age; a real transport failure still reports origin unreachable. The
// JSON index_state stays "stale" either way.
func TestSeenCloneReadTimeoutIsNotRefreshed(t *testing.T) {
	e := newIxEnv(t)
	x := e.clone("x", "a@example.com")
	x.init()
	x.acquire(stemAlpha)
	x.server.indexOpts.ReadTimeout = 200 * time.Millisecond
	e.clock.Advance(2 * time.Minute)

	script := filepath.Join(e.dir, "hang-ssh.sh")
	mustWrite(t, e.dir, "hang-ssh.sh", "#!/bin/sh\nexec sleep 30\n")
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatal(err)
	}
	x.git("config", "core.sshCommand", script)
	x.git("remote", "set-url", "origin", "ssh://git@ticket-index.invalid/repo.git")
	out := x.query()
	if !strings.Contains(out, "ticket-index: origin did not answer within 200ms; ownership is from the cached index (age 2m0s), not refreshed\n") ||
		strings.Contains(out, "unreachable") || !strings.Contains(out, "ownership: yours") {
		t.Fatalf("timed-out query = %s", out)
	}
	if alpha := x.queryJSON()[stemAlpha]; alpha.Ownership == nil || alpha.Ownership.IndexState != "stale" {
		t.Fatalf("timed-out json ownership = %+v", alpha.Ownership)
	}

	x.offline()
	if out := x.query(); !strings.Contains(out, "ticket-index: origin unreachable; ownership is from the cached index (age 2m0s)\n") {
		t.Fatalf("unreachable query = %s", out)
	}
}

// C2: a close whose guard read a cached view with no lease, against a fresh
// tip where another person acquired meanwhile, leaves that lease unchanged
// and says so in one line; the ticket file still moves.
func TestLiveCloseLeavesLeaseAcquiredAfterGuardRead(t *testing.T) {
	e := newIxEnv(t)
	x := e.clone("x", "a@example.com")
	y := e.clone("y", "b@example.com")
	x.init() // x's cache is fresh: its guard reads it with no remote call
	y.acquire(stemAlpha)

	out := x.mustCall("tickets.close", map[string]any{"stem": stemAlpha, "status": "done"})
	if n := strings.Count(out, "ticket-index:"); n != 1 || !strings.Contains(out, "ticket-index: "+stemAlpha+" is held by b@example.com") {
		t.Fatalf("close output = %s, want one ticket-index line naming the holder", out)
	}
	if l := e.lease(stemAlpha); l.Email != "b@example.com" || l.Phase != wsindex.PhaseActive {
		t.Fatalf("the close changed another person's lease: %+v", l)
	}
	if _, err := os.Stat(filepath.Join(x.root, "ai-docs", "tickets", ".done", stemAlpha+".md")); err != nil {
		t.Fatalf("the ticket file did not move: %v", err)
	}
}
