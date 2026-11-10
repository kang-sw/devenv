package wsdashboard

import (
	"bytes"
	"fmt"
	"net/http"

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

// railView reads what the rail shows: every listed session of the index,
// its snapshot (stale or not), its repository, and its last message, with
// the worktree lists read so far. A session's repository joins the lister's
// wanted set, so a render is what tells the lister which repositories to
// read.
func (d *Daemon) railView(r pageReader) RailView {
	if d.index == nil {
		return RailView{}
	}
	now := r.clock()
	var inputs []railInput
	wanted := map[string][]Registration{}
	for _, s := range d.index.Sessions() {
		if s.Unlisted {
			continue
		}
		reg := s.Registration
		if !reg.Live(now) && now.Sub(reg.LastSeen()) > endedSessionWindow {
			continue
		}
		in := railInput{Session: s, Snapshot: r.readSnapshot(reg.ReplyID), Repo: railRepoRoot(reg)}
		in.Message = d.railMsgs.last(reg.ReplyID, s.Rev, wslive.RingDir(SessionDir(d.dir, reg.ReplyID)))
		if in.Repo != "" {
			wanted[in.Repo] = append(wanted[in.Repo], reg)
		}
		inputs = append(inputs, in)
	}
	return buildRailView(inputs, d.rail.lists(wanted), now)
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
// every session page's stream, keyed without a reply-id. Like the index it
// depends on every wake (a session's change, and the worktree lister's),
// and its refresh stamp covers what follows the clock.
func (d *Daemon) railPageSource(lang, theme string) pageSource {
	pages := d.pagesIn(lang)
	return pageSource{
		key: pageKey{kind: regionRail, lang: lang, theme: theme},
		render: func() pageRender {
			d.stats.RailRenders.Add(1)
			region, err := renderRailRegion(d.railView(pages), lang)
			if err != nil {
				fmt.Fprintf(d.log, "dashboard: render rail: %v\n", err)
				return pageRender{ok: true}
			}
			return pageRender{regions: regionSet{regionRail: region}, ok: true}
		},
		// The index's stamp covers every listed session's revision (its
		// snapshot and ring among what raises it), liveness, adapter
		// presence, and window; the lister's changes come by wake.
		stamp: func(refreshInputs) (string, bool) { return pages.indexStamp() },
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
