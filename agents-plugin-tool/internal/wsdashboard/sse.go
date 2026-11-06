package wsdashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Live page updates over SSE. A stream re-renders its page whenever the
// session index reports any change (a session's own change, or a peer's,
// which its tree shows) and sends only the regions whose HTML hash differs
// from what that browser holds, so an unchanged region is never resent.
//
// The browser names the hashes it holds in the stream URL (?h=name.hash,...),
// so a stream that opens after the page was rendered, or that reconnects,
// sends exactly the regions that changed in between. Every event is
// idempotent: the page skips a region whose hash it already shows.
//
// Protocol: "event: region" with data {"name", "hash", "html"}; for the
// session page, the pseudo-region "mode" carries the page mode as its html,
// and a change means the layout no longer fits; "event: gone" when the
// session left the index.

const (
	// sseMinInterval coalesces bursts (a streamed reply nudges about ten times
	// a second) into at most one render per stream per interval.
	sseMinInterval = 250 * time.Millisecond
	// sseRefreshInterval re-renders even without a change, since liveness
	// (a peer or session reaching its live window's end) follows the clock.
	sseRefreshInterval = 15 * time.Second
	// sseKeepAlive is the comment-line period that keeps an idle stream open
	// through proxies and lets the server notice a closed one.
	sseKeepAlive = 25 * time.Second
)

// broker fans index changes out to every open stream.
type broker struct {
	mu   sync.Mutex
	subs map[chan struct{}]struct{}
}

func newBroker() *broker { return &broker{subs: map[chan struct{}]struct{}{}} }

// subscribe returns a wake channel and its cancel function.
func (b *broker) subscribe() (chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		delete(b.subs, ch)
		b.mu.Unlock()
	}
}

// wake signals every stream without blocking; a stream already signalled
// keeps one pending wake.
func (b *broker) wake(string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

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
	pages := d.pages()
	if _, ok := pages.session(replyID); !ok {
		http.Error(w, "unknown session", http.StatusNotFound)
		return
	}
	d.stream(w, r, func() (regionSet, bool) {
		view, ok := pages.session(replyID)
		if !ok {
			return nil, false
		}
		regions, err := renderSessionRegions(view)
		if err != nil {
			fmt.Fprintf(d.log, "dashboard: render session %s: %v\n", replyID, err)
			return nil, true
		}
		return regions, true
	})
}

func (d *Daemon) handleIndexEvents(w http.ResponseWriter, r *http.Request) {
	if !readMethod(w, r) {
		return
	}
	pages := d.pages()
	d.stream(w, r, func() (regionSet, bool) {
		regions, err := renderIndexRegions(pages.indexView())
		if err != nil {
			fmt.Fprintf(d.log, "dashboard: render index: %v\n", err)
			return nil, true
		}
		return regions, true
	})
}

// stream runs one SSE connection: it sends the regions that differ from the
// held hashes, then re-renders on every wake, at most once per
// sseMinInterval, until the client leaves or the daemon stops. render
// returns false once the page is gone; nil regions with true skip a round.
func (d *Daemon) stream(w http.ResponseWriter, r *http.Request, render func() (regionSet, bool)) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "retry: 3000\n\n")
	flusher.Flush()

	held := parseHeld(r.URL.Query().Get("h"))
	wake, cancel := d.broker.subscribe()
	defer cancel()

	send := func() bool {
		regions, ok := render()
		if !ok {
			fmt.Fprint(w, "event: gone\ndata: {}\n\n")
			flusher.Flush()
			return false
		}
		sent := false
		for _, name := range sortedRegionNames(regions) {
			region := regions[name]
			if held[name] == region.Hash {
				continue
			}
			payload, _ := json.Marshal(regionEvent{Name: name, Hash: region.Hash, HTML: string(region.HTML)})
			fmt.Fprintf(w, "event: region\ndata: %s\n\n", payload)
			held[name] = region.Hash
			sent = true
		}
		if sent {
			flusher.Flush()
		}
		return true
	}
	if !send() {
		return
	}
	refresh := time.NewTicker(sseRefreshInterval)
	defer refresh.Stop()
	keepAlive := time.NewTicker(sseKeepAlive)
	defer keepAlive.Stop()
	var last time.Time
	for {
		select {
		case <-r.Context().Done():
			return
		case <-d.stop:
			return
		case <-keepAlive.C:
			fmt.Fprint(w, ": keep-alive\n\n")
			flusher.Flush()
			continue
		case <-refresh.C:
		case <-wake:
		}
		if wait := sseMinInterval - time.Since(last); wait > 0 {
			select {
			case <-r.Context().Done():
				return
			case <-d.stop:
				return
			case <-time.After(wait):
			}
			// Wakes that arrived while waiting are folded into this render.
			select {
			case <-wake:
			default:
			}
		}
		last = time.Now()
		if !send() {
			return
		}
	}
}

// sortedRegionNames orders the mode pseudo-region first, so a page learns
// that its layout changed before it applies regions of the new layout.
func sortedRegionNames(regions regionSet) []string {
	names := make([]string, 0, len(regions))
	if _, ok := regions[regionMode]; ok {
		names = append(names, regionMode)
	}
	for _, name := range append(sessionRegions(modeLive), regionSessions) {
		if _, ok := regions[name]; ok {
			names = append(names, name)
		}
	}
	return names
}
