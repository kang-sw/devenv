package wsdashboard

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const trackerChangeRingSize = 64
const trackerDetailBatchLimit = 100

type trackerProjectionState struct {
	mu             sync.Mutex // serializes host builds and generation publication per store
	epoch          string     // daemon-minted and store-scoped; never persisted/reused
	stamp          trackerStamp
	snapshot       TrackerProjection
	ready          bool
	generation     uint64
	seenGeneration uint64
	items          map[string]TrackerSummaryRow
	globals        map[string]json.RawMessage
	feed           json.RawMessage
	ring           []trackerProjectionChange
}

type trackerProjectionChange struct {
	from                      uint64
	changed, removed, globals []string
	feed                      bool
}

type trackerSummaryAnswer struct {
	Cursor  string                       `json:"cursor"`
	Reset   bool                         `json:"reset"`
	Items   map[string]TrackerSummaryRow `json:"items"`
	Removed []string                     `json:"removed"`
	Globals map[string]json.RawMessage   `json:"globals"`
	Feed    json.RawMessage              `json:"feed,omitempty"`
}

func (c *trackerAnswers) projection(id string) *trackerProjectionState {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.projections == nil {
		c.projections = map[string]*trackerProjectionState{}
	}
	p := c.projections[id]
	if p == nil {
		p = &trackerProjectionState{epoch: uuid.NewString()}
		c.projections[id] = p
	}
	return p
}

func (c *trackerAnswers) projectionSeen(id string) (TrackerSeen, uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.seen[id], c.seenGen // stored slices are immutable; addSeen replaces them
}

func (c *trackerAnswers) forgetProjection(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.projections, id)
	delete(c.seen, id)
}

func (p *trackerProjectionState) cursor(gen uint64) string {
	return p.epoch + "." + strconv.FormatUint(gen, 10)
}

func intersectMarks(keys []string, seen map[string]bool) []string {
	out := []string{}
	for _, key := range keys {
		if seen[key] {
			out = append(out, key)
		}
	}
	return sortedMarks(out)
}

func markSet(lists ...[]string) map[string]bool {
	out := map[string]bool{}
	for _, list := range lists {
		for _, key := range list {
			if key != "" {
				out[key] = true
			}
		}
	}
	return out
}

func sameSummary(a, b TrackerSummaryRow) bool {
	return a.DetailVersion == b.DetailVersion && bytes.Equal(a.Data, b.Data) &&
		slices.Equal(a.Seen.Badge, b.Seen.Badge) && slices.Equal(a.Seen.Read, b.Seen.Read)
}

// publish diffs one coherent immutable host snapshot plus daemon-local seen
// marks. Only output changes mint a generation, not an SSE/data revision bump.
func (p *trackerProjectionState) publish(seen TrackerSeen) {
	snap := p.snapshot
	badge := markSet(snap.Seen.Badge, seen.Badge)
	read := markSet(snap.Seen.Read, seen.Read)
	items := make(map[string]TrackerSummaryRow, len(snap.Items))
	change := trackerProjectionChange{from: p.generation}
	for id, row := range snap.Items {
		keys := snap.Keys[id]
		row.Seen = TrackerSeen{Badge: intersectMarks(keys.Badge, badge), Read: intersectMarks(keys.Read, read)}
		items[id] = row
		if old, ok := p.items[id]; !ok || !sameSummary(old, row) {
			change.changed = append(change.changed, id)
		}
	}
	for id := range p.items {
		if _, ok := items[id]; !ok {
			change.removed = append(change.removed, id)
		}
	}
	for key, value := range snap.Globals {
		if old, ok := p.globals[key]; !ok || !bytes.Equal(old, value) {
			change.globals = append(change.globals, key)
		}
	}
	for key := range p.globals {
		if _, ok := snap.Globals[key]; !ok {
			change.globals = append(change.globals, key)
		}
	}
	change.feed = !bytes.Equal(p.feed, snap.Feed)
	if p.generation > 0 && len(change.changed)+len(change.removed)+len(change.globals) == 0 && !change.feed {
		return
	}
	p.generation++
	p.items, p.globals, p.feed = items, snap.Globals, snap.Feed
	// Generation one has no minted predecessor; every later entry covers
	// exactly one transition, including detail-only and seen-only changes.
	if change.from > 0 {
		p.ring = append(p.ring, change)
		if len(p.ring) > trackerChangeRingSize {
			copy(p.ring, p.ring[len(p.ring)-trackerChangeRingSize:])
			p.ring = p.ring[:trackerChangeRingSize]
		}
	}
}

// answer accepts only a minted cursor from this store/process and within the
// retained ring. Expired, malformed, future or foreign cursors get an inline
// full reset, never a second round trip. A removal followed by a re-add (or the
// reverse) is reduced against the CURRENT snapshot, not emitted twice.
func (p *trackerProjectionState) answer(cursor string) (trackerSummaryAnswer, bool) {
	out := trackerSummaryAnswer{Cursor: p.cursor(p.generation), Reset: true, Items: p.items,
		Removed: []string{}, Globals: p.globals, Feed: p.feed}
	epoch, seq, ok := strings.Cut(cursor, ".")
	gen, err := strconv.ParseUint(seq, 10, 64)
	if !ok || err != nil || epoch != p.epoch || gen == 0 || p.cursor(gen) != cursor || gen > p.generation {
		return out, false
	}
	if gen == p.generation {
		return out, true
	}
	if len(p.ring) == 0 || gen < p.ring[0].from {
		return out, false
	}
	out.Reset, out.Items, out.Globals, out.Feed = false, map[string]TrackerSummaryRow{}, map[string]json.RawMessage{}, nil
	ids, globals := map[string]bool{}, map[string]bool{}
	feed := false
	for _, change := range p.ring {
		if change.from < gen {
			continue
		}
		for _, id := range change.changed {
			ids[id] = true
		}
		for _, id := range change.removed {
			ids[id] = true
		}
		for _, key := range change.globals {
			globals[key] = true
		}
		feed = feed || change.feed
	}
	for id := range ids {
		if row, ok := p.items[id]; ok {
			out.Items[id] = row
		} else {
			out.Removed = append(out.Removed, id)
		}
	}
	sort.Strings(out.Removed)
	for key := range globals {
		if value, ok := p.globals[key]; ok {
			out.Globals[key] = value
		} else {
			out.Globals[key] = json.RawMessage("null")
		}
	}
	if feed {
		out.Feed = p.feed
	}
	return out, false
}

func detailIDs(r *http.Request) ([]string, bool) {
	ids := r.URL.Query()["id"]
	if len(ids) == 0 || len(ids) > trackerDetailBatchLimit {
		return nil, false
	}
	set := map[string]bool{}
	for _, id := range ids {
		if id == "" || len(id) > 256 {
			return nil, false
		}
		set[id] = true
	}
	out := make([]string, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	sort.Strings(out)
	return out, true
}

// trackerProjectionRead serves GET summary, delta?since=<cursor> and
// detail?id=<item>&id=<item> beside /view. Builds and detail calls are serialized
// for one store; no global cache lock spans a host operation.
func (d *Daemon) trackerProjectionRead(w http.ResponseWriter, r *http.Request, id, op string, data int64, now time.Time) {
	host, ok := d.tracker.(TrackerProjectionHost)
	if !ok {
		trackerError(w, http.StatusNotFound, "not found")
		return
	}
	var ids []string
	if op == "detail" {
		if ids, ok = detailIDs(r); !ok {
			trackerError(w, http.StatusBadRequest, "detail requires 1 to 100 id parameters")
			return
		}
	}
	p := d.trackerAnswers.projection(id)
	// Serialize against one generation, then release the store before any
	// socket write (including headers). A slow client must not stall readers.
	header := make(http.Header)
	status := http.StatusOK
	var body []byte
	reply := func(code int, value any) {
		status = code
		var err error
		body, err = json.Marshal(value)
		if err != nil {
			status = http.StatusInternalServerError
			body, _ = json.Marshal(map[string]string{"error": err.Error()})
		}
		body = append(body, '\n')
		header.Set("Content-Type", "application/json")
		header.Set("X-Content-Type-Options", "nosniff")
	}
	func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		built := false
		if !p.ready || !p.stamp.valid(data, now) {
			snapshot, err := host.Projection(r.Context(), id)
			if err != nil {
				code := http.StatusInternalServerError
				if errors.Is(err, ErrTrackerBusy) {
					code = http.StatusServiceUnavailable
					header.Set("Retry-After", trackerBusyRetry)
				}
				reply(code, map[string]string{"error": err.Error()})
				return
			}
			p.snapshot, p.stamp, p.ready = snapshot, trackerStamp{data: data, at: now}, true
			built = true
		}
		seen, seenGen := d.trackerAnswers.projectionSeen(id)
		if built || seenGen != p.seenGeneration {
			p.publish(seen)
			p.seenGeneration = seenGen
		}
		cursor := p.cursor(p.generation)
		header.Set("Cache-Control", "no-cache")
		header.Set("ETag", `W/"`+cursor+`"`)
		if op == "detail" {
			if p.snapshot.Detail == nil {
				reply(http.StatusNotFound, map[string]string{"error": "not found"})
				return
			}
			wanted, missing := []string{}, []string{}
			for _, iid := range ids {
				if _, ok := p.items[iid]; ok {
					wanted = append(wanted, iid)
				} else {
					missing = append(missing, iid)
				}
			}
			raw, err := p.snapshot.Detail(r.Context(), wanted)
			if err != nil {
				reply(http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			bundles := map[string]TrackerDetailBundle{}
			for _, iid := range wanted {
				if data, ok := raw[iid]; ok {
					bundles[iid] = TrackerDetailBundle{DetailVersion: p.items[iid].DetailVersion, Data: data}
				} else {
					missing = append(missing, iid)
				}
			}
			reply(http.StatusOK, map[string]any{"cursor": cursor, "items": bundles, "missing": missing})
			return
		}
		if op == "summary" {
			if etagMatches(r.Header.Get("If-None-Match"), `W/"`+cursor+`"`) {
				status = http.StatusNotModified
				return
			}
			answer, _ := p.answer("")
			reply(http.StatusOK, answer)
			return
		}
		answer, unchanged := p.answer(r.URL.Query().Get("since"))
		if unchanged {
			status = http.StatusNotModified
			return
		}
		reply(http.StatusOK, answer)
	}()
	for key, values := range header {
		w.Header()[key] = values
	}
	w.WriteHeader(status)
	if len(body) != 0 {
		_, _ = w.Write(body)
	}
}
