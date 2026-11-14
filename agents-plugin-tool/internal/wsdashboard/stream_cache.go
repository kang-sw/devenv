package wsdashboard

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/kang-sw/devenv/internal/wsstate"
)

// Shared page renders. Every stream of one page (one session, or the
// sessions index, in one language and theme) reads through one sharedPage,
// so N tabs on a session cost one read and render per change instead of N.
// A render is held in memory until a wake names one of its dependencies or
// a refresh tick finds its refresh stamp changed; the cache starts empty
// and is derived only from disk and the daemon's own reads, so a
// replacement daemon renders the same pages. An entry lives while a stream
// holds it.
//
// A render records its dependencies: the wake tokens (brokerDeps) whose
// change can alter it, and the refresh inputs (refreshInputs) its refresh
// stamp re-reads. The stamp covers what changes with no wake: liveness,
// which follows the clock, contact liveness, and the files a page source
// read that a writer may change without a nudge (PageContent.Watch).

// pageKey names one shared page. replyID is empty for the sessions index
// and the session rail; kind is empty but for the rail (regionRail), which
// is a render of its own every session page's stream adds (railPageSource).
// No region depends on the viewer class (a loopback or LAN viewer): what
// differs per viewer stays outside the regions (sessionPageData.SysOpen,
// indexPageData.LAN), so the key carries none.
type pageKey struct {
	kind, replyID, lang, theme string
}

// refreshInputs are what a page's refresh stamp re-reads: the sessions its
// render looked up (its own first), the files its page source named, and
// whether it showed contacts. Lists are sorted, so equal inputs compare
// equal.
type refreshInputs struct {
	ids      []string
	watch    []string
	contacts bool
	// pids are PageContent.WatchPIDs, sorted: the stamp folds each one's
	// liveness.
	pids []int
}

func (a refreshInputs) equal(b refreshInputs) bool {
	return a.contacts == b.contacts && slices.Equal(a.ids, b.ids) && slices.Equal(a.watch, b.watch) && slices.Equal(a.pids, b.pids)
}

// pageRender is one render of a page.
type pageRender struct {
	// regions are the rendered regions; nil with ok skips a round (a
	// render error). They are shared by every stream of the page: a stream
	// never modifies them (regionFilter.apply copies).
	regions regionSet
	// board is the topic decomposition of regions[board], computed once in
	// the shared session render, never per connection.
	board *boardTopics
	// ok is false once the page is gone.
	ok bool
	// deps are the wake tokens the render depends on; nil depends on every
	// wake (the sessions index).
	deps []string
	// inputs are what the refresh stamp re-reads.
	inputs refreshInputs
	// reads are the store reads the render made, which a live render of
	// the same page may reuse (liveRender); nil offers none.
	reads any
}

// liveRender is what a render may reuse when every wake since the
// previous render was live-only (broker.liveOnlySince): a hosted session's
// snapshot hand-off or ring append, after which no store but the snapshot
// (and the ring, after an append) needs reading again.
type liveRender struct {
	// prev is the previous render, whose reads the render may reuse.
	prev pageRender
	// changed reports whether a wake named token since prev.
	changed func(token string) bool
}

// pageSource renders one page and computes its refresh stamp.
type pageSource struct {
	key pageKey
	// render renders the page; live is nil for a full render (every read
	// afresh).
	render func(live *liveRender) pageRender
	// stamp hashes, at call time, what in may have changed with no wake;
	// ok false means the page has inputs the stamp cannot cover, so every
	// refresh tick re-renders it.
	stamp func(in refreshInputs) (stamp string, ok bool)
}

// pageCache holds the shared pages open streams read.
type pageCache struct {
	broker *broker
	stats  *streamStats

	mu    sync.Mutex
	pages map[pageKey]*sharedPage
}

func newPageCache(b *broker, stats *streamStats) *pageCache {
	return &pageCache{broker: b, stats: stats, pages: map[pageKey]*sharedPage{}}
}

// sharedPage is one page's latest render.
type sharedPage struct {
	// refs counts the streams holding the page; pageCache.mu guards it.
	refs int

	// mu serializes the page's renders: a stream that finds the render
	// stale renders while the page's other streams wait, then share it.
	mu       sync.Mutex
	rendered bool
	// at is the broker sequence taken before the render read anything, so a
	// wake that lands during the render leaves it stale.
	at uint64
	r  pageRender
	// stamp is the refresh stamp taken before the render read anything,
	// valid only when stampOK: the render's inputs equal the ones it was
	// taken from. Every render after the first takes it, a wake-driven one
	// included; the first has no inputs to take it from, so the next
	// refresh tick re-renders once.
	stamp   string
	stampOK bool
}

// acquire returns the shared page for key, holding it for the caller.
func (c *pageCache) acquire(key pageKey) *sharedPage {
	c.mu.Lock()
	defer c.mu.Unlock()
	p := c.pages[key]
	if p == nil {
		p = &sharedPage{}
		c.pages[key] = p
	}
	p.refs++
	return p
}

// release drops the caller's hold; the last one drops the page.
func (c *pageCache) release(key pageKey, p *sharedPage) {
	c.mu.Lock()
	defer c.mu.Unlock()
	p.refs--
	if p.refs == 0 && c.pages[key] == p {
		delete(c.pages, key)
	}
}

// get returns the page's current render and the broker sequence it was
// taken at. It renders only when a wake since the last render named one of
// its dependencies, or, on a refresh tick (refresh), when the refresh stamp
// is unknown or changed. A stamp taken before a render and unchanged since
// means a change during that render is still seen: it moves the stamp.
//
// When every wake since the last render was live-only, the render is a
// live one (liveRender), which reuses the previous render's store reads;
// it takes no new stamp and keeps the previous one, which still describes
// the reused reads, so the next refresh tick re-reads in full whatever
// changed under them. A refresh tick that finds the stamp changed renders
// in full.
func (c *pageCache) get(p *sharedPage, src pageSource, refresh bool) (pageRender, uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	current := p.rendered && c.broker.unchangedSince(p.r.deps, p.at)
	if current && !refresh {
		c.stats.Shared.Add(1)
		return p.r, p.at
	}
	liveOnly := !current && p.rendered && p.r.ok && p.r.reads != nil && c.broker.liveOnlySince(p.r.deps, p.at)
	if liveOnly && !refresh {
		return c.renderLive(p, src)
	}
	// The stamp is taken before any render, a wake-driven one included,
	// so the next refresh tick skips a page this render left current.
	var stamp string
	var stampOK bool
	if p.rendered {
		stamp, stampOK = src.stamp(p.r.inputs)
		unchanged := stampOK && p.stampOK && stamp == p.stamp
		if current && unchanged {
			c.stats.RefreshSkips.Add(1)
			return p.r, p.at
		}
		if liveOnly && unchanged {
			return c.renderLive(p, src)
		}
	}
	at := c.broker.current()
	prev := p.r.inputs
	r := src.render(nil)
	p.r, p.at, p.rendered = r, at, true
	// The stamp was taken before this read, from the previous render's
	// inputs: it is this render's only when the inputs did not change. A
	// failed render (no regions) keeps none, so the next tick retries it.
	p.stamp, p.stampOK = stamp, stampOK && r.inputs.equal(prev) && (r.regions != nil || !r.ok)
	return r, at
}

// renderLive runs a live render of p (get); p.mu is held. The previous
// stamp stays, valid while the inputs it was taken from did not change.
func (c *pageCache) renderLive(p *sharedPage, src pageSource) (pageRender, uint64) {
	since := p.at
	at := c.broker.current()
	prev := p.r
	r := src.render(&liveRender{prev: prev, changed: func(token string) bool {
		return !c.broker.unchangedSince([]string{token}, since)
	}})
	p.r, p.at = r, at
	p.stampOK = p.stampOK && r.inputs.equal(prev.inputs) && (r.regions != nil || !r.ok)
	return r, at
}

// statStamp folds one path's state into h: whether it exists, its
// modification time, and its size. An in-place edit moves the file's own
// time; a file added or removed moves its directory's.
func statStamp(h io.Writer, path string) {
	info, err := os.Stat(path)
	if err != nil {
		fmt.Fprintf(h, "%s\x00-\x01", path)
		return
	}
	fmt.Fprintf(h, "%s\x00%d\x00%d\x01", path, info.ModTime().UnixNano(), info.Size())
}

// StampWatched folds the state of each watched path into h: a watched
// directory that also has watched files directly inside it (the ticket
// panel's status directories and their ticket files) is stamped by its
// listing, each entry's name, size, and modification time, and those files
// are covered by it, so a directory of tickets costs one listing instead of
// a stat per file; every other path folds its own stat (statStamp). An
// in-place edit of a covered file moves its listed size or time, as its
// own stat would. A path is matched to its directory as named, so a watch
// list naming the same directory two ways stats each path itself.
func StampWatched(h io.Writer, watch []string) {
	watched := make(map[string]bool, len(watch))
	for _, path := range watch {
		watched[path] = true
	}
	listed := map[string]bool{}
	for _, path := range watch {
		if dir := filepath.Dir(path); dir != path && watched[dir] {
			listed[dir] = true
		}
	}
	for _, path := range watch {
		switch {
		case listed[path]:
			// Listed even when its own directory is listed too: that listing
			// shows this directory's time, which an edit inside it keeps.
			listStamp(h, path)
		case listed[filepath.Dir(path)] && filepath.Dir(path) != path:
			// Covered by its directory's listing.
		default:
			statStamp(h, path)
		}
	}
}

// listStamp folds a directory's listing into h: each entry's name, size,
// and modification time, from the listing alone (one lstat per entry on
// Linux, none on Windows), or that it does not list. A symbolic link is
// followed (a stat), as a covered file's own stat would follow it.
func listStamp(h io.Writer, dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		fmt.Fprintf(h, "%s\x00list\x00-\x01", dir)
		return
	}
	fmt.Fprintf(h, "%s\x00list\x01", dir)
	for _, e := range entries {
		info, err := e.Info()
		if err == nil && e.Type()&fs.ModeSymlink != 0 {
			info, err = os.Stat(filepath.Join(dir, e.Name()))
		}
		if err == nil {
			fmt.Fprintf(h, "%s\x00%d\x00%d\x00%t\x02", e.Name(), info.ModTime().UnixNano(), info.Size(), e.IsDir())
		} else {
			fmt.Fprintf(h, "%s\x00-\x02", e.Name())
		}
	}
}

// processAlive reports whether a watched process runs (a test seam).
var processAlive = wsstate.ProcessAlive

func sumHex(h hash.Hash) string { return hex.EncodeToString(h.Sum(nil)[:16]) }

// newStampHash is the hash a refresh stamp is built in.
func newStampHash() hash.Hash { return sha256.New() }
