package wsdashboard

import (
	"encoding/json"
	"errors"
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

// Read again before every write: a newer-version or explicitly repaired file
// must not be overwritten by a stale in-memory snapshot. Caller holds s.mu.
func (s *planUsageStore) readHistoryLocked() (planUsageHistoryFile, error) {
	h := planUsageHistoryFile{Fields: map[string]json.RawMessage{}}
	raw, err := os.ReadFile(filepath.Join(s.dir, planUsageHistoryCache))
	if errors.Is(err, os.ErrNotExist) {
		return h, nil
	}
	if err != nil {
		return h, err
	}
	if err = json.Unmarshal(raw, &h.Fields); err != nil {
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

func (s *planUsageStore) appendHistoryLocked(samples []planUsageSample, now int64) {
	if len(samples) == 0 {
		return
	}
	h, err := s.readHistoryLocked()
	if err != nil {
		return
	} // Preserve corrupt/newer files for explicit repair.
	type identity struct {
		harness, kind, source string
		reset, at             int64
	}
	key := func(v planUsageSample) identity { return identity{v.Harness, v.Kind, v.Source, v.ResetsAt, v.AsOf} }
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
			h.Records[i].Sample = v
		} else {
			indices[key(v)] = len(h.Records)
			h.Records = append(h.Records, planUsageHistoryRecord{Sample: v})
		}
		changed = true
	}
	if !changed {
		return
	}
	h.Records = prunePlanUsageHistory(h.Records, now)
	records := make([]map[string]json.RawMessage, 0, len(h.Records))
	for _, r := range h.Records {
		fields := r.Fields
		if fields == nil {
			fields = map[string]json.RawMessage{}
		}
		raw, _ := json.Marshal(r.Sample)
		var known map[string]json.RawMessage
		_ = json.Unmarshal(raw, &known)
		for k, v := range known {
			fields[k] = v
		}
		records = append(records, fields)
	}
	h.Fields["schema_version"] = json.RawMessage(`1`)
	h.Fields["samples"], err = json.Marshal(records)
	if err != nil {
		return
	}
	raw, err := json.Marshal(h.Fields)
	if err == nil {
		_ = writePrivateFile(s.dir, planUsageHistoryCache, raw)
	}
}

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

func usageSamples(entry planUsageEntry) []planUsageSample {
	samples := make([]planUsageSample, 0, len(entry.Windows))
	for _, w := range entry.Windows {
		samples = append(samples, planUsageSample{Harness: entry.Harness, Kind: w.Kind, Source: entry.Source, ResetsAt: w.ResetsAt, AsOf: w.AsOf, Percent: w.Percent})
	}
	return samples
}
