package wsdashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Live page updates over SSE, or over WebSocket with the same events
// (stream_ws.go). A stream re-renders its page when a change its page can
// show is reported (a session's own change, or a change of a session its
// page looks up: a peer, a delegate; the sessions index shows every session)
// and sends only the regions whose HTML hash differs from what that browser
// holds, so an unchanged region is never resent. The streams of one page
// share one render per change (stream_cache.go).
//
// The browser names the hashes it holds in the stream URL (?h=name.hash,...),
// so a stream that opens after the page was rendered, or that reconnects,
// sends exactly the regions that changed in between. Every event is
// idempotent: the page skips a region whose hash it already shows.
//
// Protocol: "event: region" with data {"name", "hash", "html"}; for the
// session page, the pseudo-region "mode" carries the page mode as its html,
// and a change means the layout no longer fits; "event: gone" when the
// session left the index. A daemon with a tracker host also sends
// "event: tracker" with data {"rev": N} on both pages' streams whenever the
// tracker revision (TrackerChanged) differs from what that stream last
// sent, including once right after connect; the page then re-reads the
// tracker routes. A daemon without one never sends it.
//
// A session stream may name the regions it wants (?regions=entries,convlive):
// it then sends only those, no mode pseudo-region unless named, and watches
// no git root unless git is named. The tracker drawer's store chat follows
// a support conversation's entries this way.

const (
	// sseMinInterval coalesces bursts (a streamed reply nudges about ten times
	// a second) into at most one render per stream per interval.
	sseMinInterval = 250 * time.Millisecond
	// sseRefreshInterval is the refresh tick: liveness (a peer or session
	// reaching its live window's end) follows the clock, and a page source
	// reads files no wake reports, so each tick re-renders unless the page's
	// refresh stamp shows nothing changed (stream_cache.go).
	sseRefreshInterval = 15 * time.Second
	// sseKeepAlive is the idle-frame period: the SSE comment line that keeps
	// an idle stream open through proxies and lets the server notice a closed
	// one, and the WebSocket ping the page answers (stream_ws.go).
	sseKeepAlive = 25 * time.Second
)

// broker routes change wakes to the streams whose page can show the
// change, and numbers every wake so a shared render can tell whether it is
// still current (pageCache).
//
// A wake names one dependency token: a session's reply-id (its own change,
// which every page that looks it up also shows) or a git root's (gitDep).
// A stream hears the wakes its last render depends on (follow); before its
// first render, and for the sessions index (nil deps), it hears every wake.
// A wake naming no token ("") reaches every stream and leaves every render
// stale.
type broker struct {
	mu    sync.Mutex
	subs  map[*subscription]struct{}
	stats *streamStats
	// seq numbers the wakes. changed holds each token's last wake, anyWake
	// the last wake of any token, and allWake the last wake that named none.
	seq              uint64
	changed          map[string]uint64
	anyWake, allWake uint64
}

// subscription is one stream's wake channel and the tokens it hears.
type subscription struct {
	ch chan struct{}
	// deps are the tokens its page depends on; nil hears every wake.
	deps map[string]bool
}

func newBroker(stats *streamStats) *broker {
	return &broker{subs: map[*subscription]struct{}{}, stats: stats, changed: map[string]uint64{}}
}

// subscribe returns a subscription that hears every wake until follow
// narrows it, and its cancel function.
func (b *broker) subscribe() (*subscription, func()) {
	sub := &subscription{ch: make(chan struct{}, 1)}
	b.mu.Lock()
	b.subs[sub] = struct{}{}
	b.mu.Unlock()
	return sub, func() {
		b.mu.Lock()
		delete(b.subs, sub)
		b.mu.Unlock()
	}
}

// wake records a change of token and signals, without blocking, every
// stream that hears it; a stream already signalled keeps one pending wake.
func (b *broker) wake(token string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.seq++
	if token == "" {
		b.allWake = b.seq
	} else {
		b.changed[token] = b.seq
	}
	b.anyWake = b.seq
	for sub := range b.subs {
		if token == "" || sub.deps == nil || sub.deps[token] {
			b.signal(sub)
		}
	}
}

// signalAll signals every stream and records no change: what it reports
// (the tracker revision) is sent beside the regions, never in one, so no
// render goes stale.
func (b *broker) signalAll() {
	b.mu.Lock()
	defer b.mu.Unlock()
	for sub := range b.subs {
		b.signal(sub)
	}
}

func (b *broker) signal(sub *subscription) {
	select {
	case sub.ch <- struct{}{}:
		b.stats.Signals.Add(1)
	default:
	}
}

// current is the sequence of the latest wake.
func (b *broker) current() uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.seq
}

// unchangedSince reports that no wake after sequence at named one of deps
// (any token, for nil deps) or named none.
func (b *broker) unchangedSince(deps []string, at uint64) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.unchangedLocked(deps, at)
}

func (b *broker) unchangedLocked(deps []string, at uint64) bool {
	if b.allWake > at {
		return false
	}
	if deps == nil {
		return b.anyWake <= at
	}
	for _, token := range deps {
		if b.changed[token] > at {
			return false
		}
	}
	return true
}

// liveOnlySince reports that wakes after sequence at named deps' live-only
// tokens alone (liveToken): no other token of deps and no wake naming none.
// A page with nil deps (every wake) is never live-only.
func (b *broker) liveOnlySince(deps []string, at uint64) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if deps == nil || b.allWake > at {
		return false
	}
	for _, token := range deps {
		if !liveToken(token) && b.changed[token] > at {
			return false
		}
	}
	return true
}

// follow narrows sub to the tokens of the render it just sent, taken at
// sequence at (nil hears every wake). A wake that landed after at for one of
// them, which sub may not have heard under its old tokens, signals it now.
func (b *broker) follow(sub *subscription, deps []string, at uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if deps == nil {
		sub.deps = nil
	} else {
		sub.deps = make(map[string]bool, len(deps))
		for _, token := range deps {
			sub.deps[token] = true
		}
	}
	if !b.unchangedLocked(deps, at) {
		b.signal(sub)
	}
}

// gitDep is the wake token of a git root's handle reads (gitWatcher). A
// reply-id never holds a colon, so it never collides with one.
func gitDep(root string) string { return "git:" + gitKey(root) }

// regionEvent is an "event: region" payload.
type regionEvent struct {
	Name string `json:"name"`
	Hash string `json:"hash"`
	HTML string `json:"html"`
}

// parseHeld reads the ?h= list of region hashes a page holds.
func parseHeld(raw string) map[string]string {
	held := map[string]string{}
	for _, part := range strings.Split(raw, ",") {
		if name, hash, ok := strings.Cut(part, "."); ok && name != "" {
			held[name] = hash
		}
	}
	return held
}

func (d *Daemon) handleSessionEvents(w http.ResponseWriter, r *http.Request, replyID string) {
	if !streamMethod(w, r) {
		return
	}
	var s Session
	ok := d.index != nil && ValidReplyID(replyID)
	if ok {
		s, ok = d.index.Session(replyID)
	}
	if !ok {
		http.Error(w, "unknown session", http.StatusNotFound)
		return
	}
	lang, _ := requestLang(r)
	only := parseRegionFilter(r.URL.Query().Get("regions"))
	// The stream is the page's presence: the git handle's root is read while
	// some stream watches it.
	if d.git != nil && s.Project != "" && only.has(regionGit) {
		release := d.git.watch(s.Project)
		defer release()
	}
	// The session rail is a render of its own, shared by every session
	// page, sent over the same stream; its worktree lists are read while
	// some stream shows it.
	var side *pageSource
	if only.has(regionRail) {
		release := d.rail.watch()
		defer release()
		rail := d.railPageSource(lang, requestTheme(r))
		side = &rail
	}
	d.stream(w, r, d.sessionPageSource(replyID, lang, requestTheme(r)), only, side)
}

// sessionPageSource renders replyID's session page in lang and theme for
// the streams that share it.
func (d *Daemon) sessionPageSource(replyID, lang, theme string) pageSource {
	pages := d.pagesIn(lang)
	return pageSource{
		key: pageKey{replyID: replyID, lang: lang, theme: theme},
		render: func(live *liveRender) pageRender {
			var prev *sessionReads
			ringChanged := true
			if live != nil {
				prev, _ = live.prev.reads.(*sessionReads)
				ringChanged = live.changed(ringDep(replyID))
			}
			view, inputs, reads, ok := pages.readSessionWith(replyID, prev, ringChanged)
			if !ok {
				return pageRender{deps: []string{replyID}}
			}
			d.stats.SessionRenders.Add(1)
			if prev != nil {
				d.stats.LiveRenders.Add(1)
			}
			// Each looked-up session's live-only token beside its own: a
			// hosted session's snapshot hand-off wakes the pages that show it
			// without a store re-read (live_hosted.go).
			deps := make([]string, 0, 2*len(inputs.ids)+2)
			for _, id := range inputs.ids {
				deps = append(deps, id, liveDep(id))
			}
			deps = append(deps, ringDep(replyID))
			if d.git != nil && view.Project != "" {
				deps = append(deps, gitDep(view.Project))
			}
			pr := pageRender{ok: true, deps: deps, inputs: inputs, reads: reads}
			view.Lang, view.Theme = lang, theme
			view.Git = d.git.badge(view.Project)
			// The snapshot's repo figures come from the session's ws-mcp;
			// the handle reads again when they changed (follow).
			if view.Band.Repo != nil {
				d.git.follow(view.Project, replyID, *view.Band.Repo)
			}
			regions, err := renderSessionRegions(view, d.entries)
			if err != nil {
				fmt.Fprintf(d.log, "dashboard: render session %s: %v\n", replyID, err)
				return pr
			}
			pr.regions = regions
			return pr
		},
		stamp: func(in refreshInputs) (string, bool) { return pages.sessionStamp(replyID, in) },
	}
}

// regionFilter is the region set a stream asked for; nil is every region.
type regionFilter map[string]bool

// parseRegionFilter reads a ?regions= list; empty asks for every region.
func parseRegionFilter(raw string) regionFilter {
	var f regionFilter
	for _, name := range strings.Split(raw, ",") {
		if name = strings.TrimSpace(name); name != "" {
			if f == nil {
				f = regionFilter{}
			}
			f[name] = true
		}
	}
	return f
}

func (f regionFilter) has(name string) bool { return f == nil || f[name] }

// apply returns the regions f names, in a new set: the set it is given is
// a shared render that other streams read.
func (f regionFilter) apply(regions regionSet) regionSet {
	if f == nil {
		return regions
	}
	kept := regionSet{}
	for name, region := range regions {
		if f[name] {
			kept[name] = region
		}
	}
	return kept
}

// streamMethod admits GET only: a stream has no headers-only form, and a
// HEAD would hold the connection until the client left.
func streamMethod(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return false
	}
	return true
}

func (d *Daemon) handleIndexEvents(w http.ResponseWriter, r *http.Request) {
	if !streamMethod(w, r) {
		return
	}
	lang, _ := requestLang(r)
	pages := d.pagesIn(lang)
	d.stream(w, r, pageSource{
		key: pageKey{lang: lang},
		render: func(*liveRender) pageRender {
			d.stats.IndexRenders.Add(1)
			view := pages.indexView()
			view.Lang = lang
			regions, err := renderIndexRegions(view)
			if err != nil {
				fmt.Fprintf(d.log, "dashboard: render index: %v\n", err)
				return pageRender{ok: true}
			}
			return pageRender{regions: regions, ok: true}
		},
		stamp: func(refreshInputs) (string, bool) { return pages.indexStamp() },
	}, nil, nil)
}

// stream runs one stream connection: it sends the regions of src's shared
// page that differ from the held hashes, then, on every wake the page depends
// on and every refresh tick, at most once per sseMinInterval, takes the page's
// current render and sends what changed, until the client leaves or the
// daemon stops. A page gone before the first render answers 404 (before a
// WebSocket upgrade, too); one gone later sends "gone". A WebSocket handshake
// upgrades (stream_ws.go) and every other request is served SSE: the events
// and their data are the same, only the framing differs.
//
// side, when not nil, is a further shared page whose regions the stream
// sends too (the session rail on a session page): it has its own
// subscription and is re-rendered only on a wake it hears or a refresh
// tick, so a change that reaches one page never re-renders the other. Only
// src decides that the page is gone.
func (d *Daemon) stream(w http.ResponseWriter, r *http.Request, src pageSource, only regionFilter, side *pageSource) {
	upgrade := wsUpgrade(r)
	if upgrade && !wsOriginAllowed(r) {
		http.Error(w, "cross-origin stream refused", http.StatusForbidden)
		return
	}
	flusher, canFlush := w.(http.Flusher)
	_, canHijack := w.(http.Hijacker)
	if (upgrade && !canHijack) || (!upgrade && !canFlush) {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	page := d.cache.acquire(src.key)
	defer d.cache.release(src.key, page)
	// Subscribed before the first render, so a change it misses still wakes
	// the stream.
	sub, cancel := d.broker.subscribe()
	defer cancel()
	first, at := d.cache.get(page, src, false)
	if !first.ok {
		http.Error(w, "unknown session", http.StatusNotFound)
		return
	}
	held := parseHeld(r.URL.Query().Get("h"))
	var parts []sidePart
	if side != nil {
		part := sidePart{src: *side, page: d.cache.acquire(side.key)}
		defer d.cache.release(side.key, part.page)
		var cancelSide func()
		part.sub, cancelSide = d.broker.subscribe()
		defer cancelSide()
		parts = append(parts, part)
	}

	run := func(sink streamSink) {
		// sentRev is the tracker revision this stream last sent; the
		// sentinel -1 sends revision 0 on connect too.
		sentRev := int64(-1)
		send := func(pr pageRender, at uint64) bool {
			d.broker.follow(sub, pr.deps, at)
			if !pr.ok {
				_ = sink.event("gone", []byte("{}"))
				_ = sink.flush()
				return false
			}
			regions := only.apply(pr.regions)
			sent := false
			sent, ok := d.sendRegions(sink, regions, held)
			if !ok {
				return false
			}
			if d.tracker != nil {
				if rev := d.trackerRev.Load(); rev != sentRev {
					if sink.event("tracker", fmt.Appendf(nil, `{"rev":%d}`, rev)) != nil {
						return false
					}
					sentRev = rev
					sent = true
				}
			}
			return !sent || sink.flush() == nil
		}
		// sendSide sends a side page's render, which never ends the page.
		sendSide := func(part sidePart, pr pageRender, at uint64) bool {
			d.broker.follow(part.sub, pr.deps, at)
			if !pr.ok {
				return true
			}
			sent, ok := d.sendRegions(sink, only.apply(pr.regions), held)
			return ok && (!sent || sink.flush() == nil)
		}
		if !send(first, at) {
			return
		}
		for _, part := range parts {
			pr, at := d.cache.get(part.page, part.src, false)
			if !sendSide(part, pr, at) {
				return
			}
		}
		// sideWake is the side page's wake channel; nil (never ready)
		// without one.
		var sideWake <-chan struct{}
		if len(parts) > 0 {
			sideWake = parts[0].sub.ch
		}
		refresh := time.NewTicker(d.refreshInterval)
		defer refresh.Stop()
		keepAlive := time.NewTicker(d.keepAlive)
		defer keepAlive.Stop()
		var last time.Time
		for {
			tick, primary, sideWoken := false, false, false
			select {
			case <-sink.gone():
				return
			case <-d.stop:
				return
			case <-keepAlive.C:
				if sink.keepAlive() != nil {
					return
				}
				continue
			case <-refresh.C:
				tick = true
			case <-sub.ch:
				primary = true
			case <-sideWake:
				sideWoken = true
			}
			if wait := sseMinInterval - time.Since(last); wait > 0 {
				select {
				case <-sink.gone():
					return
				case <-d.stop:
					return
				case <-time.After(wait):
				}
			}
			// Wakes that arrived while waiting are folded into this render,
			// and so is the other part's signal of the same wake (the broker
			// signals every subscription under one lock): the page renders
			// first, so a page that is gone never sends a side render first.
			select {
			case <-sub.ch:
				primary = true
			default:
			}
			select {
			case <-sideWake:
				sideWoken = true
			default:
			}
			last = time.Now()
			if primary || tick {
				pr, at := d.cache.get(page, src, tick)
				if !send(pr, at) {
					return
				}
			}
			if sideWoken || tick {
				for _, part := range parts {
					pr, at := d.cache.get(part.page, part.src, tick)
					if !sendSide(part, pr, at) {
						return
					}
				}
			}
		}
	}
	if upgrade {
		d.serveWebSocket(w, r, run)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "retry: 3000\n\n")
	flusher.Flush()
	run(&sseSink{w: w, flusher: flusher, done: r.Context().Done()})
}

// sidePart is one side page of a stream (see stream): its source, its
// shared page, and its own subscription.
type sidePart struct {
	src  pageSource
	page *sharedPage
	sub  *subscription
}

// sendRegions writes the regions whose hash differs from what the browser
// holds, recording each sent hash in held; sent reports that one was
// written, ok false that the connection failed.
func (d *Daemon) sendRegions(sink streamSink, regions regionSet, held map[string]string) (sent, ok bool) {
	for _, name := range sortedRegionNames(regions) {
		region := regions[name]
		if held[name] == region.Hash {
			continue
		}
		payload, _ := json.Marshal(regionEvent{Name: name, Hash: region.Hash, HTML: string(region.HTML)})
		if sink.event("region", payload) != nil {
			return sent, false
		}
		d.stats.Sends.Add(1)
		held[name] = region.Hash
		sent = true
	}
	return sent, true
}

// streamSink is one stream connection's framing.
type streamSink interface {
	// event writes one event: its name and its JSON data.
	event(name string, data []byte) error
	// flush pushes the events written so far to the peer.
	flush() error
	// keepAlive writes the idle frame that keeps the connection open and
	// lets a dead peer be noticed.
	keepAlive() error
	// gone is closed once the peer left.
	gone() <-chan struct{}
}

// sseSink frames events as SSE ("event: name\ndata: json\n\n").
type sseSink struct {
	w       http.ResponseWriter
	flusher http.Flusher
	done    <-chan struct{}
}

func (s *sseSink) event(name string, data []byte) error {
	_, err := fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", name, data)
	return err
}

func (s *sseSink) flush() error {
	s.flusher.Flush()
	return nil
}

func (s *sseSink) keepAlive() error {
	if _, err := fmt.Fprint(s.w, ": keep-alive\n\n"); err != nil {
		return err
	}
	return s.flush()
}

func (s *sseSink) gone() <-chan struct{} { return s.done }

// sortedRegionNames orders the mode pseudo-region first, so a page learns
// that its layout changed before it applies regions of the new layout.
func sortedRegionNames(regions regionSet) []string {
	names := make([]string, 0, len(regions))
	if _, ok := regions[regionMode]; ok {
		names = append(names, regionMode)
	}
	for _, name := range append(sessionRegions(modeLive), regionSessions, regionRail) {
		if _, ok := regions[name]; ok {
			names = append(names, name)
		}
	}
	return names
}
