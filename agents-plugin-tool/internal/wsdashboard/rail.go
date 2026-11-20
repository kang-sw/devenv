package wsdashboard

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"sync"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

// The session rail's daemon side: its region, its read, its shared stream
// render, and its refresh route. The view model and its rules are in
// pageview_rail.go.

// regionRail is the session rail's region. It is in every session page,
// live or board-first, but not in sessionRegions: its render is shared by
// every session page (railPageSource), not part of one session's.
const regionRail = "rail"

// railRoute is the rail's refresh route (POST): an immediate re-read of the
// worktree lists.
const railRoute = "/rail/refresh"

// railEventsRoute is the iframe shell's stream: the rail alone
// (handleRailEvents).
const railEventsRoute = "/rail/events"

// railOrderedView reads the full rail inventory before presentation
// partitioning: railBaseView ordered by the rail layout, archived splits,
// worktree admissions, and memo counts.
func (d *Daemon) railOrderedView(r pageReader) RailView {
	view := d.railBaseView(r)
	layout, gen := d.layout.current()
	orderRailView(&view, layout, railFactsOf(view))
	splitRailArchived(&view)
	view.LayoutGen = gen
	classifyRailAdmissions(&view, layout)
	for i := range view.Workspaces {
		// Optional memo state must never prevent the rail from rendering.
		doc, err := readMemoDoc(memoFile(d.dir, view.Workspaces[i].Path))
		if err == nil {
			view.Workspaces[i].MemoCount = len(doc.memos())
		}
	}
	return view
}

// railView reads what the rail shows: railOrderedView partitioned into
// explicit Workspaces, PendingWorkspaces, and RejectedWorkspaces.
func (d *Daemon) railView(r pageReader) RailView {
	view := d.railOrderedView(r)
	// Without a workspace store there is no admission route or explicit-add
	// path. Keep the original rail inventory visible in that mode.
	if d.workspaces != nil {
		partitionRailWorkspaces(&view, d.wsAdmission.rejected())
	}
	return view
}

// railBaseView reads the rail before its layout orders it: every listed
// session of the index, its snapshot (stale or not), its repository, and
// its last message, with the stored workspaces and the worktree lists read
// so far. A session's repository and a stored one join the lister's wanted
// set, so a render is what tells the lister which repositories to read.
// The layout route validates its ops against it (railFactsOf).
func (d *Daemon) railBaseView(r pageReader) RailView {
	if d.index == nil {
		return RailView{}
	}
	now := r.clock()
	var inputs []railInput
	wanted := map[string][]Registration{}
	d.railInputs.mu.Lock()
	defer d.railInputs.mu.Unlock()
	if d.railInputs.sessions == nil {
		d.railInputs.sessions = map[string]cachedRailInput{}
	}
	seen := map[string]bool{}
	for _, s := range d.index.Sessions() {
		if s.Unlisted {
			continue
		}
		reg := s.Registration
		seen[reg.ReplyID] = true
		version := d.railInputVersion(s)
		cached, have := d.railInputs.sessions[reg.ReplyID]
		// Explicit current-state queries (the hiring roster) retain their
		// read-through contract; only presentation passes reuse these inputs.
		if r.freshSnapshots || !have || cached.retry || cached.version != version {
			if r.freshSnapshots {
				r.snapshots.Invalidate(r.dir, reg.ReplyID)
			}
			snap, err := r.snapshots.snapshotResult(r.dir, reg.ReplyID)
			retry := err != nil && !errors.Is(err, os.ErrNotExist)
			if retry && have && !r.freshSnapshots {
				snap = cached.input.Snapshot // retain last-good presentation
			}
			in := railInput{Snapshot: snap, Repo: railRepoRoot(reg)}
			if in.Repo != "" {
				in.Linked = worktreeMain(reg.Project) != ""
			}
			var messageOK bool
			in.Message, messageOK = d.railMsgs.lastResult(reg.ReplyID, railRev(s.Rev, version.ring), wslive.RingDir(SessionDir(d.dir, reg.ReplyID)))
			cached = cachedRailInput{version: version, input: in, retry: retry || !messageOK}
			d.railInputs.sessions[reg.ReplyID] = cached
			if d.railInputs.onRead != nil {
				d.railInputs.onRead(reg.ReplyID)
			}
		}
		in := cached.input
		// Heartbeat-only handoffs raise no revision or wake. Age the cached
		// snapshot using the index's latest in-memory clock, never a stat.
		if in.Snapshot != nil && !s.SnapshotUpdatedAt.IsZero() && s.SnapshotUpdatedAt.UnixMilli() > in.Snapshot.UpdatedAt {
			snap := *in.Snapshot
			snap.UpdatedAt = s.SnapshotUpdatedAt.UnixMilli()
			in.Snapshot = &snap
		}
		in.Session, in.CanRevive = s, d.agents != nil
		if in.Repo != "" {
			wanted[in.Repo] = append(wanted[in.Repo], reg)
		}
		inputs = append(inputs, in)
	}
	for id := range d.railInputs.sessions {
		if !seen[id] {
			delete(d.railInputs.sessions, id)
		}
	}
	// A stored repository's worktrees are listed whether or not a session
	// runs in it.
	stored := d.workspaces.workspaces()
	for _, w := range stored {
		if _, ok := wanted[w.Path]; w.Repo && !ok {
			wanted[w.Path] = nil
		}
	}
	view := buildRailView(inputs, d.rail.lists(wanted), stored, now)
	view.CanAdd = d.workspaces != nil
	view.CanSpawn = d.agents != nil
	return view
}

// railInputCache shares immutable store inputs across languages and streams.
// Only a session revision or a named session/snapshot/ring wake refreshes them;
// clock-derived rows are rebuilt from these inputs on every rail pass.
type railInputCache struct {
	mu       sync.Mutex
	sessions map[string]cachedRailInput
	onRead   func(string) // test-only counting hook, under mu
}

type cachedRailInput struct {
	version railInputVersion
	input   railInput
	retry   bool
}

func (c *railInputCache) retryPending() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, in := range c.sessions {
		if in.retry {
			return true
		}
	}
	return false
}

type railInputVersion struct {
	rev, ring, snapshot, session uint64
}

func (d *Daemon) railInputVersion(s Session) railInputVersion {
	v := railInputVersion{rev: s.Rev, ring: d.live.ringGen(s.ReplyID)}
	if d.broker != nil {
		d.broker.mu.Lock()
		v.snapshot = d.broker.changed[liveDep(s.ReplyID)]
		v.session = d.broker.changed[s.ReplyID]
		// A ring notification can arrive before the index read raises Rev.
		v.ring = max(v.ring, d.broker.changed[ringDep(s.ReplyID)])
		d.broker.mu.Unlock()
	}
	return v
}

// renderRailRegion renders the rail region in lang.
func renderRailRegion(view RailView, lang string) (renderedRegion, error) {
	a, err := loadDashboardAssets()
	if err != nil {
		return renderedRegion{}, err
	}
	var buf bytes.Buffer
	if err := a.templateFor(lang).ExecuteTemplate(&buf, "r-"+regionRail, view); err != nil {
		return renderedRegion{}, fmt.Errorf("dashboard region %s: %w", regionRail, err)
	}
	return newRegion(buf.String()), nil
}

// railPageSource is the rail's shared render in lang and theme: one for
// every session page's stream, keyed without a reply-id. Wakes refresh only
// the named sessions' inputs, at most once a second with a trailing pass.
// Clock liveness, worktree lists and layout are recomputed from memory;
// only a changed view runs the template and row decomposition.
func (d *Daemon) railPageSource(lang, theme string) pageSource {
	return d.railSource(d.pagesIn(lang), lang, theme)
}

func (d *Daemon) railSource(pages pageReader, lang, theme string) pageSource {
	return pageSource{
		key:          pageKey{kind: regionRail, lang: lang, theme: theme},
		compareReads: true,
		minInterval:  time.Second,
		render: func(previous *liveRender) pageRender {
			view := d.railView(pages)
			if previous != nil {
				if prior, ok := previous.prev.reads.(RailView); ok && reflect.DeepEqual(prior, view) {
					return previous.prev
				}
			}
			d.stats.RailRenders.Add(1)
			region, err := renderRailRegion(view, lang)
			if err != nil {
				fmt.Fprintf(d.log, "dashboard: render rail: %v\n", err)
				return pageRender{ok: true}
			}
			return pageRender{regions: regionSet{regionRail: region}, rail: splitRailRows(string(region.HTML)), reads: view, ok: true}
		},
		// The index's stamp covers every listed session's revision (its
		// snapshot and ring among what raises it), liveness, adapter
		// presence, and window; the lister's changes come by wake. The
		// layout's generation joins it, so a layout change is never skipped
		// as unchanged: this daemon's ops move it in memory, another
		// process's write shows at the next foreign look
		// (foreign_schedule.go).
		stamp: func(refreshInputs) (string, bool) {
			if d.railInputs.retryPending() {
				return "", false // a refresh must retry failed inputs, even at the same revision
			}
			stamp, ok := pages.indexStamp()
			gen := d.layout.generationSince(d.foreign.filesSince())
			admGen := d.wsAdmission.generation()
			return stamp + "/" + strconv.FormatUint(gen, 10) + "/" + strconv.FormatUint(admGen, 10), ok
		},
	}
}

// handleRailRefresh serves POST /rail/refresh: an immediate re-read of the
// rail's worktree lists. It needs the daemon's own Origin, as the other
// browser writes do, and answers 204 once the read is asked for (it lands
// on the pages through their streams), 409 when no page shows the rail.
func (d *Daemon) handleRailRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !sameOrigin(r) {
		http.Error(w, "cross-origin request refused", http.StatusForbidden)
		return
	}
	if !d.rail.refresh() {
		http.Error(w, "no open page shows the session rail", http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
