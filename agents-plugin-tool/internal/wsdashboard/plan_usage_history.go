package wsdashboard

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"
)

const (
	planUsageHistoryCache = "plan-usage-history.json"
	planUsageHistoryAge   = 14 * 24 * time.Hour
	planUsageHistoryCap   = 5000
)

// Only these fields cross the viewer boundary. Extensions in the private file
// survive a supported-version round trip, but are never projected to viewers.
type planUsageSample struct {
	Harness  string  `json:"harness"`
	Kind     string  `json:"kind"`
	Source   string  `json:"source"`
	ResetsAt int64   `json:"resetsAt"`
	AsOf     int64   `json:"asOf"`
	Percent  float64 `json:"percent"`
}
type planUsageHistoryProjection struct {
	SchemaVersion int               `json:"schema_version"`
	Status        string            `json:"status"`
	Samples       []planUsageSample `json:"samples"`
}
type planUsageHistoryRecord struct {
	Sample planUsageSample
	Fields map[string]json.RawMessage
}
type planUsageHistoryFile struct {
	Fields  map[string]json.RawMessage
	Records []planUsageHistoryRecord
}

func validPlanUsageSample(v planUsageSample) bool {
	sourceOK := v.Harness == "claude" && v.Source == "Claude push" || v.Harness == "codex" && (v.Source == "via Codex CLI" || v.Source == "via Pi")
	kindOK := v.Kind == "seven_day" || v.Harness == "claude" && v.Kind == "five_hour"
	return sourceOK && kindOK && v.AsOf > 0 && v.ResetsAt >= 0 && validUsage(v.Percent)
}

// planUsageHistoryMemo holds the parsed file and the viewer projection built
// from it. Guarded by planUsageStore.mu.
type planUsageHistoryMemo struct {
	ok   bool
	stat planUsageHistoryStat
	file planUsageHistoryFile
	err  error // content verdict (corrupt/newer); IO errors are never memoized
	rev  uint64

	viewRev   uint64
	viewKey   string
	viewUntil int64
	view      json.RawMessage
}

// planUsageHistoryStat identifies the file version the memo reflects.
type planUsageHistoryStat struct {
	missing bool
	size    int64
	modTime int64
}

func (s *planUsageStore) statHistory(path string) (planUsageHistoryStat, error) {
	stat := os.Stat
	if s.statFile != nil {
		stat = s.statFile
	}
	info, err := stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return planUsageHistoryStat{missing: true}, nil
	}
	if err != nil {
		return planUsageHistoryStat{}, err
	}
	return planUsageHistoryStat{size: info.Size(), modTime: info.ModTime().UnixNano()}, nil
}

// The parsed file is memoized by its stat (size, mtime): an unchanged file
// costs one stat and no read. Any other writer's replace changes the stat and
// forces a re-read before the next write, so a newer-version or explicitly
// repaired file is still never overwritten from a stale snapshot. Stat comes
// before read: a replace in between leaves an older stat on newer content,
// which costs only one extra read. This assumes one writer per directory (the
// daemon): a foreign replace landing between our own write and its stat, or
// one keeping both size and mtime, goes unnoticed. The result is shared with
// the memo and must not be mutated. Caller holds s.mu.
func (s *planUsageStore) readHistoryLocked() (planUsageHistoryFile, error) {
	empty := planUsageHistoryFile{Fields: map[string]json.RawMessage{}}
	path := filepath.Join(s.dir, planUsageHistoryCache)
	stat, err := s.statHistory(path)
	if err != nil {
		s.hist.ok = false
		return empty, err
	}
	if s.hist.ok && s.hist.stat == stat {
		return s.hist.file, s.hist.err
	}
	h, err := empty, error(nil)
	if !stat.missing {
		read := os.ReadFile
		if s.readFile != nil {
			read = s.readFile
		}
		raw, readErr := read(path)
		switch {
		case errors.Is(readErr, os.ErrNotExist):
		case readErr != nil:
			// A transient IO failure (a scanner holding the file) must not pin
			// "unavailable" until the file next changes.
			s.hist.ok = false
			return empty, readErr
		default:
			h, err = parsePlanUsageHistory(raw)
		}
	}
	s.hist.ok, s.hist.stat, s.hist.file, s.hist.err = true, stat, h, err
	s.hist.rev++
	return h, err
}

func parsePlanUsageHistory(raw []byte) (planUsageHistoryFile, error) {
	h := planUsageHistoryFile{Fields: map[string]json.RawMessage{}}
	if err := json.Unmarshal(raw, &h.Fields); err != nil {
		return h, err
	}
	var version int
	if json.Unmarshal(h.Fields["schema_version"], &version) != nil || version != 1 {
		return h, errors.New("unsupported usage history")
	}
	var records []json.RawMessage
	if json.Unmarshal(h.Fields["samples"], &records) != nil || records == nil {
		return h, errors.New("invalid usage history")
	}
	for _, raw := range records {
		var v planUsageSample
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &v) != nil || !validPlanUsageSample(v) || json.Unmarshal(raw, &fields) != nil {
			return h, errors.New("invalid usage history sample")
		}
		// All six members are required, including a truthful zero/unknown reset.
		for _, key := range []string{"harness", "kind", "source", "resetsAt", "asOf", "percent"} {
			if value, ok := fields[key]; !ok || string(value) == "null" {
				return h, errors.New("incomplete usage history sample")
			}
		}
		h.Records = append(h.Records, planUsageHistoryRecord{Sample: v, Fields: fields})
	}
	return h, nil
}

// withSampleFields returns a fresh copy of fields with the six known members
// set from v; unknown extension members carry over unchanged.
func withSampleFields(fields map[string]json.RawMessage, v planUsageSample) map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(fields)+6)
	for k, raw := range fields {
		out[k] = raw
	}
	raw, _ := json.Marshal(v)
	var known map[string]json.RawMessage
	_ = json.Unmarshal(raw, &known)
	for k, raw := range known {
		out[k] = raw
	}
	return out
}

func prunePlanUsageHistory(records []planUsageHistoryRecord, now int64) []planUsageHistoryRecord {
	kept := make([]planUsageHistoryRecord, 0, len(records))
	cutoff := now - planUsageHistoryAge.Milliseconds()
	for _, record := range records {
		if record.Sample.AsOf >= cutoff {
			kept = append(kept, record)
		}
	}
	sort.SliceStable(kept, func(i, j int) bool {
		a, b := kept[i].Sample, kept[j].Sample
		if a.AsOf != b.AsOf {
			return a.AsOf < b.AsOf
		}
		if a.Harness != b.Harness {
			return a.Harness < b.Harness
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.Source != b.Source {
			return a.Source < b.Source
		}
		return strconv.FormatInt(a.ResetsAt, 10) < strconv.FormatInt(b.ResetsAt, 10)
	})
	counts := map[[2]string]int{}
	newest := make([]planUsageHistoryRecord, 0, len(kept))
	for i := len(kept) - 1; i >= 0; i-- {
		key := [2]string{kept[i].Sample.Harness, kept[i].Sample.Kind}
		if counts[key] < planUsageHistoryCap {
			newest = append(newest, kept[i])
			counts[key]++
		}
	}
	for i, j := 0, len(newest)-1; i < j; i, j = i+1, j-1 {
		newest[i], newest[j] = newest[j], newest[i]
	}
	return newest
}

// A record's Fields always hold its complete JSON object: the parsed members
// of a read record, withSampleFields for a new or replaced one. The memo is
// never mutated in place; a successful write becomes the new memo.
func (s *planUsageStore) appendHistoryLocked(samples []planUsageSample, now int64) {
	if len(samples) == 0 {
		return
	}
	current, err := s.readHistoryLocked()
	if err != nil {
		return
	} // Preserve corrupt/newer files for explicit repair.
	type identity struct {
		harness, kind, source string
		reset, at             int64
	}
	key := func(v planUsageSample) identity { return identity{v.Harness, v.Kind, v.Source, v.ResetsAt, v.AsOf} }
	h := planUsageHistoryFile{Fields: make(map[string]json.RawMessage, len(current.Fields)+2), Records: append([]planUsageHistoryRecord(nil), current.Records...)}
	for k, raw := range current.Fields {
		h.Fields[k] = raw
	}
	indices := map[identity]int{}
	for i, r := range h.Records {
		indices[key(r.Sample)] = i
	}
	changed := false
	for _, v := range samples {
		if !validPlanUsageSample(v) {
			continue
		}
		if i, ok := indices[key(v)]; ok {
			// Re-pushing an observation already stored as is writes nothing.
			if h.Records[i].Sample == v {
				continue
			}
			h.Records[i] = planUsageHistoryRecord{Sample: v, Fields: withSampleFields(h.Records[i].Fields, v)}
		} else {
			indices[key(v)] = len(h.Records)
			h.Records = append(h.Records, planUsageHistoryRecord{Sample: v, Fields: withSampleFields(nil, v)})
		}
		changed = true
	}
	if !changed {
		return
	}
	h.Records = prunePlanUsageHistory(h.Records, now)
	// An accepted sample the prune drops at once (past the age or the cap)
	// leaves the content as read: compare in memory and skip the write.
	if sameHistoryRecords(h.Records, current.Records) {
		return
	}
	records := make([]map[string]json.RawMessage, 0, len(h.Records))
	for _, r := range h.Records {
		records = append(records, r.Fields)
	}
	h.Fields["schema_version"] = json.RawMessage(`1`)
	h.Fields["samples"], err = json.Marshal(records)
	if err != nil {
		return
	}
	raw, err := json.Marshal(h.Fields)
	if err != nil || s.write(planUsageHistoryCache, raw) != nil {
		return
	}
	// Adopt what was just written so the next read costs one stat; without a
	// stat, drop the memo and let the next read reload.
	stat, err := s.statHistory(filepath.Join(s.dir, planUsageHistoryCache))
	if err != nil || stat.missing {
		s.hist.ok = false
		return
	}
	s.hist.ok, s.hist.stat, s.hist.file, s.hist.err = true, stat, h, nil
	s.hist.rev++
}

// sameHistoryRecords reports whether two record lists hold the same samples in
// the same order. A record's Fields change only alongside its Sample (see
// appendHistoryLocked), so equal samples mean equal written content.
func sameHistoryRecords(a, b []planUsageHistoryRecord) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Sample != b[i].Sample {
			return false
		}
	}
	return true
}

// history is the full pruned history; viewers get historyView.
func (s *planUsageStore) history(now int64) planUsageHistoryProjection {
	p := planUsageHistoryProjection{SchemaVersion: 1, Status: "empty", Samples: []planUsageSample{}}
	if s == nil {
		return p
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	h, err := s.readHistoryLocked()
	if err != nil {
		p.Status = "unavailable"
		return p
	}
	for _, r := range prunePlanUsageHistory(h.Records, now) {
		p.Samples = append(p.Samples, r.Sample)
	}
	if len(p.Samples) > 0 {
		p.Status = "ok"
	}
	return p
}

// historyView is the ?history=1 member: the status of the full history, but
// only the samples trend() in assets/usage.js can use for the entry windows
// sent in the same response. The encoded JSON is memoized by history
// revision and entries until the age prune would next drop a sample.
func (s *planUsageStore) historyView(entries []planUsageEntry, now int64) json.RawMessage {
	p := planUsageHistoryProjection{SchemaVersion: 1, Status: "empty", Samples: []planUsageSample{}}
	if s == nil {
		raw, _ := json.Marshal(p)
		return raw
	}
	key, _ := json.Marshal(entries)
	s.mu.Lock()
	defer s.mu.Unlock()
	h, err := s.readHistoryLocked()
	if err != nil {
		p.Status = "unavailable"
		raw, _ := json.Marshal(p)
		return raw
	}
	m := &s.hist
	if m.view != nil && m.viewRev == m.rev && m.viewKey == string(key) && now <= m.viewUntil {
		return m.view
	}
	pruned := prunePlanUsageHistory(h.Records, now)
	until := int64(math.MaxInt64)
	for _, r := range pruned {
		until = min(until, r.Sample.AsOf+planUsageHistoryAge.Milliseconds())
	}
	if len(pruned) > 0 {
		p.Status = "ok"
	}
	p.Samples = projectPlanUsageHistory(pruned, entries)
	raw, err := json.Marshal(p)
	if err != nil {
		raw, _ = json.Marshal(planUsageHistoryProjection{SchemaVersion: 1, Status: "unavailable", Samples: []planUsageSample{}})
		return raw
	}
	m.viewRev, m.viewKey, m.viewUntil, m.view = m.rev, string(key), until, raw
	return raw
}

// projectPlanUsageHistory keeps, per harness/kind, only what trend() can
// draw for that group's entry window. trend() draws the samples whose reset
// equals the window's and links each to the sample sorted directly before it
// in the whole harness/kind series, so that neighbor is kept even when it
// belongs to another window: dropping it would join samples the full series
// keeps apart. The window's current observation, which trend() inserts
// itself, keeps its neighbor too. An entry may carry several windows of one
// kind, and trend() runs once per window. A group without an entry window is
// never drawn and sends nothing.
func projectPlanUsageHistory(records []planUsageHistoryRecord, entries []planUsageEntry) []planUsageSample {
	type group struct{ harness, kind string }
	current := map[group][]planUsageSample{}
	for _, e := range entries {
		for _, w := range e.Windows {
			at := w.AsOf
			if at == 0 {
				at = e.AsOf
			}
			g := group{e.Harness, w.Kind}
			current[g] = append(current[g], planUsageSample{Harness: e.Harness, Kind: w.Kind, Source: e.Source, ResetsAt: w.ResetsAt, AsOf: at, Percent: w.Percent})
		}
	}
	series := map[group][]int{}
	for i, r := range records {
		if g := (group{r.Sample.Harness, r.Sample.Kind}); len(current[g]) > 0 {
			series[g] = append(series[g], i)
		}
	}
	keep := make([]bool, len(records))
	for g, indices := range series {
		sort.Slice(indices, func(a, b int) bool { return usageTrendBefore(records[indices[a]].Sample, records[indices[b]].Sample) })
		for _, cur := range current[g] {
			for n, i := range indices {
				if records[i].Sample.ResetsAt == cur.ResetsAt {
					keep[i] = true
					if n > 0 {
						keep[indices[n-1]] = true
					}
				}
			}
			if n := sort.Search(len(indices), func(n int) bool { return !usageTrendBefore(records[indices[n]].Sample, cur) }); n > 0 {
				keep[indices[n-1]] = true
			}
		}
	}
	out := []planUsageSample{}
	for i, r := range records {
		if keep[i] {
			out = append(out, r.Sample)
		}
	}
	return out
}

// usageTrendBefore must match trend()'s sort in assets/usage.js: asOf, then
// the identity string source|resetsAt|asOf. Sources are fixed ASCII, so byte
// order equals JavaScript's string order.
func usageTrendBefore(a, b planUsageSample) bool {
	if a.AsOf != b.AsOf {
		return a.AsOf < b.AsOf
	}
	return usageTrendIdentity(a) < usageTrendIdentity(b)
}

func usageTrendIdentity(v planUsageSample) string {
	return v.Source + "|" + strconv.FormatInt(v.ResetsAt, 10) + "|" + strconv.FormatInt(v.AsOf, 10)
}

func usageSamples(entry planUsageEntry) []planUsageSample {
	samples := make([]planUsageSample, 0, len(entry.Windows))
	for _, w := range entry.Windows {
		samples = append(samples, planUsageSample{Harness: entry.Harness, Kind: w.Kind, Source: entry.Source, ResetsAt: w.ResetsAt, AsOf: w.AsOf, Percent: w.Percent})
	}
	return samples
}
