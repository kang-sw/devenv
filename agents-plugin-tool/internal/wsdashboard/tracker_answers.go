package wsdashboard

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// trackerAnswerTTL bounds how long a cached tracker answer serves an
// unchanged data revision. A ledger or registry change bumps it
// (TrackerChanged: the daemon's own writes, the store watch for other
// processes' writes, a poll round that moved something, a late reload),
// but some of what a view shows moves without a bump: the sync state
// another process records alone (host-sync.json, which the watch does not
// stamp), the poll's syncedAt, and the time-relative fields (now, the stale
// flags). The bound is the tracker poll's period
// (wstrackerhost.DefaultPollInterval), the reconcile those already wait on.
const trackerAnswerTTL = 3 * time.Minute

// trackerAnswers caches the two reads every page reload makes, the store
// list and each store's view, by the data revision they were built at. An
// answer at the current data revision is served from memory, with no call
// to the tracker host and so no file read, and as 304 to a request whose
// If-None-Match names its ETag. A view's ETag is its content apart from the
// time of day (TrackerView.Tag), so a rebuild that folds the same view
// still answers 304.
//
// The seen marks are part of a view's answer but not of its data revision:
// only this daemon writes them (wstrackerhost.MarkSeen), so the seen route
// adds its marks to the cached view (addSeen) instead of dropping it.
type trackerAnswers struct {
	mu sync.Mutex
	// views holds each store's last view answer by store id.
	views map[string]trackerViewAnswer
	// projections retain each store's immutable summary and 64-generation
	// change ring independently of the legacy full-view answer cache.
	projections map[string]*trackerProjectionState
	seen        map[string]TrackerSeen // daemon-local overlay; no host reread on seen
	// stores is the last store list, valid while storesOK.
	stores   []TrackerStore
	storesAt trackerStamp
	storesOK bool
	// seenGen counts the seen writes, so a view built across one is not
	// cached with the marks it may have missed.
	seenGen uint64
}

// trackerStamp is when an answer was built: the data revision read before
// the build began, and the time.
type trackerStamp struct {
	data int64
	at   time.Time
}

func (s trackerStamp) valid(data int64, now time.Time) bool {
	return s.data == data && now.Sub(s.at) < trackerAnswerTTL && now.Sub(s.at) >= 0
}

// trackerViewAnswer is one store's cached view answer.
type trackerViewAnswer struct {
	stamp trackerStamp
	view  TrackerView
	body  []byte
	etag  string
}

// storeList returns the cached store list when it was built at data.
func (c *trackerAnswers) storeList(data int64, now time.Time) ([]TrackerStore, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.storesOK || !c.storesAt.valid(data, now) {
		return nil, false
	}
	return c.stores, true
}

// putStoreList caches the store list read at at, unless a list read at a
// later data revision is cached.
func (c *trackerAnswers) putStoreList(stores []TrackerStore, at trackerStamp) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.storesOK && c.storesAt.data > at.data {
		return
	}
	c.stores, c.storesAt, c.storesOK = stores, at, true
	registered := make(map[string]bool, len(stores))
	for _, store := range stores {
		registered[store.ID] = true
	}
	for id := range c.projections {
		if !registered[id] {
			delete(c.projections, id)
			delete(c.seen, id)
		}
	}
}

// view returns store id's cached view answer when it was built at data.
func (c *trackerAnswers) view(id string, data int64, now time.Time) (trackerViewAnswer, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	a, ok := c.views[id]
	if !ok || !a.stamp.valid(data, now) {
		return trackerViewAnswer{}, false
	}
	return a, true
}

// seenGeneration returns the count of seen writes, read before a view build.
func (c *trackerAnswers) seenGeneration() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.seenGen
}

// putView caches a, built since the seen generation gen, unless a seen
// write landed meanwhile or the store's cached view is of a later data
// revision; it drops the answers of earlier data revisions.
func (c *trackerAnswers) putView(id string, a trackerViewAnswer, gen uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if gen != c.seenGen {
		return
	}
	if old, ok := c.views[id]; ok && old.stamp.data > a.stamp.data {
		return
	}
	if c.views == nil {
		c.views = map[string]trackerViewAnswer{}
	}
	for k, v := range c.views {
		if v.stamp.data < a.stamp.data {
			delete(c.views, k)
		}
	}
	c.views[id] = a
}

// addSeen adds the marks the seen route wrote to store id's cached view,
// as the host's MarkSeen adds them to the file (a sorted union, empty marks
// dropped).
func (c *trackerAnswers) addSeen(id string, add TrackerSeen) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.seenGen++
	if c.seen == nil {
		c.seen = map[string]TrackerSeen{}
	}
	marks := c.seen[id]
	c.seen[id] = TrackerSeen{Badge: sortedMarks(marks.Badge, add.Badge), Read: sortedMarks(marks.Read, add.Read)}
	a, ok := c.views[id]
	if !ok {
		return
	}
	a.view.Seen = TrackerSeen{Badge: sortedMarks(a.view.Seen.Badge, add.Badge), Read: sortedMarks(a.view.Seen.Read, add.Read)}
	if err := a.encode(); err != nil {
		delete(c.views, id)
		return
	}
	c.views[id] = a
}

// encode sets a's body and ETag from its view.
func (a *trackerViewAnswer) encode() error {
	body, err := json.Marshal(a.view)
	if err != nil {
		return err
	}
	a.body = append(body, '\n')
	if a.view.Tag == "" {
		a.etag = trackerETag(a.body)
		return nil
	}
	seen, err := json.Marshal(a.view.Seen)
	if err != nil {
		return err
	}
	a.etag = trackerETag([]byte(a.view.Tag), seen)
	return nil
}

func sortedMarks(lists ...[]string) []string {
	set := map[string]bool{}
	for _, l := range lists {
		for _, s := range l {
			if s != "" {
				set[s] = true
			}
		}
	}
	out := make([]string, 0, len(set))
	for s := range set {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// trackerETag is a weak ETag over parts.
func trackerETag(parts ...[]byte) string {
	h := fnv.New64a()
	for _, p := range parts {
		fmt.Fprintf(h, "%d:", len(p))
		h.Write(p)
	}
	return fmt.Sprintf(`W/"%016x"`, h.Sum64())
}

// etagMatches reports whether the If-None-Match header names etag, by the
// weak comparison a conditional GET uses.
func etagMatches(header, etag string) bool {
	if header == "" {
		return false
	}
	want := strings.TrimPrefix(etag, "W/")
	for _, t := range strings.Split(header, ",") {
		t = strings.TrimSpace(t)
		if t == "*" || strings.TrimPrefix(t, "W/") == want {
			return true
		}
	}
	return false
}

// writeTrackerAnswer writes a JSON answer the page may revalidate: 304
// with no body when the request's If-None-Match names etag, else body as
// writeJSON writes it.
func writeTrackerAnswer(w http.ResponseWriter, r *http.Request, body []byte, etag string) {
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "no-cache")
	if etagMatches(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
