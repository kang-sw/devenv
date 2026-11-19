package wslive

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kang-sw/devenv/internal/wsstate"
)

// Append-only JSONL segment backing and in-memory daemon ring cache for
// conversation rings (<sessionDir>/ring/).
//
// Each segment is named seg-<16-digit-seq>.jsonl (mode 0600, in 0700 dir).
// Legacy per-entry <stamp>-<hex>.json files are still read and merged by
// Name on load, so upgrades and downgrade-reopens lose nothing; old binaries
// ignore .jsonl files (isRingEntryName matches only .json) and never delete
// or overwrite them.
//
// In daemon mode (EnableRingCache for <stateDir>/sessions), each session's
// ring is loaded once into RAM and its active append segment file descriptor
// stays open until rotation, slot close, drop, or daemon shutdown. Steady-
// state appends perform one write() with zero open/close/rename/ReadDir, and
// steady-state reads perform zero filesystem operations.

var (
	// Segment rotation bounds (test seams).
	ringSegmentEntries       = 500
	ringSegmentBytes   int64 = 1 << 20 // 1 MiB
	ringMaxSegments          = 16
)

var ringSegmentPattern = regexp.MustCompile(`^seg-([0-9]{16})\.jsonl$`)

func ringSegmentName(seq uint64) string {
	return fmt.Sprintf("seg-%016d.jsonl", seq)
}

func parseRingSegmentSeq(name string) (uint64, bool) {
	m := ringSegmentPattern.FindStringSubmatch(name)
	if m == nil {
		return 0, false
	}
	seq, err := strconv.ParseUint(m[1], 10, 64)
	if err != nil || seq == 0 {
		return 0, false
	}
	return seq, true
}

// ringSegmentLine is one JSONL record in a segment file.
type ringSegmentLine struct {
	Name  string    `json:"name"`
	Entry RingEntry `json:"entry"`
}

type ringSegMeta struct {
	seq   uint64
	count int
}

type ringMem struct {
	mu          sync.Mutex
	dir         string
	loaded      bool
	closed      bool
	records     []RingRecord // newest <= RingHistoryCap, sorted ascending by Name
	segments    []ringSegMeta
	legacyNames []string // legacy .json files seen on disk at load
	file        *os.File
	segSeq      uint64
	segEntries  int
	segBytes    int64
	tornTail    bool // newest segment on disk had a torn trailing line not yet truncated
}

type ringRootCache struct {
	root    string
	mu      sync.Mutex
	entries map[string]*ringMem // clean ring dir -> *ringMem
	closed  bool
}

var (
	ringRootsMu sync.Mutex
	ringRoots   = map[string]*ringRootCache{} // normalized root key -> cache

	// directDirMu serializes concurrent non-cached AppendRing calls per dir.
	directDirMu sync.Mutex
	directLocks = map[string]*sync.Mutex{}
)

func normPathKey(path string) string {
	clean := filepath.Clean(path)
	if runtime.GOOS == "windows" {
		return strings.ToLower(clean)
	}
	return clean
}

func pathUnderRoot(dir, root string) bool {
	prefix := root
	if !strings.HasSuffix(prefix, string(filepath.Separator)) {
		prefix += string(filepath.Separator)
	}
	if runtime.GOOS == "windows" {
		return len(dir) > len(prefix) && strings.EqualFold(dir[:len(prefix)], prefix)
	}
	return strings.HasPrefix(dir, prefix)
}

// EnableRingCache enables the in-memory ring cache and held-open segment
// writers for every conversation ring under sessionsRoot (typically
// <stateDir>/sessions). The returned close function closes every held segment
// file descriptor under that root and deregisters the cache; it is idempotent.
func EnableRingCache(sessionsRoot string) func() error {
	clean := filepath.Clean(sessionsRoot)
	key := normPathKey(clean)
	rc := &ringRootCache{
		root:    clean,
		entries: map[string]*ringMem{},
	}
	ringRootsMu.Lock()
	ringRoots[key] = rc
	ringRootsMu.Unlock()

	var once sync.Once
	var closeErr error
	return func() error {
		once.Do(func() {
			ringRootsMu.Lock()
			if ringRoots[key] == rc {
				delete(ringRoots, key)
			}
			ringRootsMu.Unlock()

			rc.mu.Lock()
			rc.closed = true
			list := make([]*ringMem, 0, len(rc.entries))
			for _, rm := range rc.entries {
				list = append(list, rm)
			}
			rc.entries = nil
			rc.mu.Unlock()

			var errs []error
			for _, rm := range list {
				rm.mu.Lock()
				rm.closed = true
				if rm.file != nil {
					if err := rm.file.Close(); err != nil {
						errs = append(errs, err)
					}
					rm.file = nil
				}
				rm.mu.Unlock()
			}
			closeErr = errors.Join(errs...)
		})
		return closeErr
	}
}

func findRingRoot(cleanDir string) *ringRootCache {
	ringRootsMu.Lock()
	defer ringRootsMu.Unlock()
	for _, rc := range ringRoots {
		if pathUnderRoot(cleanDir, rc.root) {
			return rc
		}
	}
	return nil
}

// cachedRing returns the cached *ringMem for dir if dir falls under an active
// EnableRingCache root; nil otherwise.
func cachedRing(dir string) *ringMem {
	clean := filepath.Clean(dir)
	rc := findRingRoot(clean)
	if rc == nil {
		return nil
	}
	key := normPathKey(clean)
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if rc.closed {
		return nil
	}
	rm := rc.entries[key]
	if rm == nil {
		rm = &ringMem{dir: clean}
		rc.entries[key] = rm
	}
	return rm
}

// CloseRingWriter closes any held-open segment file descriptor for dir while
// keeping its cached entries in RAM. Safe to call when dir is uncached or
// already closed.
func CloseRingWriter(dir string) {
	clean := filepath.Clean(dir)
	rc := findRingRoot(clean)
	if rc == nil {
		return
	}
	key := normPathKey(clean)
	rc.mu.Lock()
	rm := rc.entries[key]
	rc.mu.Unlock()
	if rm == nil {
		return
	}
	rm.mu.Lock()
	if rm.file != nil {
		_ = rm.file.Close()
		rm.file = nil
	}
	rm.mu.Unlock()
}

// DropRing closes any held-open segment file descriptor for dir and removes
// its in-memory cache entry. Call before removing a session directory so open
// handles never block deletion on Windows, and when a session leaves the index.
func DropRing(dir string) {
	clean := filepath.Clean(dir)
	rc := findRingRoot(clean)
	if rc == nil {
		return
	}
	key := normPathKey(clean)
	rc.mu.Lock()
	rm := rc.entries[key]
	delete(rc.entries, key)
	rc.mu.Unlock()
	if rm == nil {
		return
	}
	rm.mu.Lock()
	rm.closed = true
	if rm.file != nil {
		_ = rm.file.Close()
		rm.file = nil
	}
	rm.mu.Unlock()
}

// RefreshRing reloads dir's cached ring from disk if it is currently cached.
// Used for non-hosted sessions on authoritative refreshes (/nudge or periodic
// safety scan) so external legacy writers remain visible.
func RefreshRing(dir string) {
	clean := filepath.Clean(dir)
	rc := findRingRoot(clean)
	if rc == nil {
		return
	}
	key := normPathKey(clean)
	rc.mu.Lock()
	rm := rc.entries[key]
	rc.mu.Unlock()
	if rm == nil {
		return
	}
	rm.mu.Lock()
	defer rm.mu.Unlock()
	if rm.closed || rm.file != nil {
		// An open writer means this process owns the ring.
		return
	}
	rm.loaded = false
	_ = rm.ensureLoadedLocked(false)
}

func directLock(cleanDir string) *sync.Mutex {
	key := normPathKey(cleanDir)
	directDirMu.Lock()
	defer directDirMu.Unlock()
	mu := directLocks[key]
	if mu == nil {
		mu = &sync.Mutex{}
		directLocks[key] = mu
	}
	return mu
}

// ensureLoadedLocked populates rm from disk if not yet loaded. rm.mu is held.
func (rm *ringMem) ensureLoadedLocked(cutTail bool) error {
	if rm.loaded {
		return nil
	}
	st, err := loadRingDir(rm.dir, cutTail)
	if err != nil {
		return err
	}
	rm.records = st.records
	rm.segments = st.segments
	rm.legacyNames = st.legacyNames
	rm.segSeq = st.segSeq
	rm.segEntries = st.segEntries
	rm.segBytes = st.segBytes
	rm.tornTail = st.tornTail
	rm.loaded = true
	return nil
}

type loadedRing struct {
	records     []RingRecord
	segments    []ringSegMeta
	legacyNames []string
	segSeq      uint64
	segEntries  int
	segBytes    int64
	tornTail    bool
}

func parseRingNameStamp(name string) (time.Time, bool) {
	stamp, _, ok := strings.Cut(name, "-")
	if !ok {
		return time.Time{}, false
	}
	at, err := time.Parse(ringStampLayout, stamp)
	if err != nil {
		return time.Time{}, false
	}
	return at.UTC(), true
}

// readSegmentFile parses all complete JSONL lines from a segment file. A torn
// trailing line (no newline) is ignored, and truncated on disk when cut is true.
func readSegmentFile(path string, cut bool) (records []RingRecord, size int64, torn bool, err error) {
	raw, err := wsstate.ReadFileOnce(path)
	if err != nil {
		return nil, 0, false, err
	}
	if len(raw) == 0 {
		return nil, 0, false, nil
	}
	if end := bytes.LastIndexByte(raw, '\n'); end+1 != len(raw) {
		raw = raw[:end+1]
		if cut {
			_ = os.Truncate(path, int64(len(raw)))
		} else {
			torn = true
		}
	}
	size = int64(len(raw))
	rest := raw
	for len(rest) > 0 {
		var line []byte
		line, rest, _ = bytes.Cut(rest, []byte{'\n'})
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var sl ringSegmentLine
		if json.Unmarshal(line, &sl) != nil || sl.Name == "" {
			continue
		}
		at, ok := parseRingNameStamp(sl.Name)
		if !ok {
			continue
		}
		records = append(records, RingRecord{RingEntry: sl.Entry, Name: sl.Name, At: at})
	}
	return records, size, torn, nil
}

// loadRingDir reads all segment files and legacy .json files in dir once,
// merges them by Name, sorts ascending, and caps to RingHistoryCap.
func loadRingDir(dir string, cutTail bool) (loadedRing, error) {
	dirEntries, err := wsstate.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return loadedRing{}, nil
	}
	if err != nil {
		return loadedRing{}, err
	}
	var segSeqs []uint64
	var legacy []string
	for _, de := range dirEntries {
		if de.IsDir() {
			continue
		}
		name := de.Name()
		if seq, ok := parseRingSegmentSeq(name); ok {
			segSeqs = append(segSeqs, seq)
		} else if isRingEntryName(name) {
			legacy = append(legacy, name)
		}
	}
	sort.Slice(segSeqs, func(i, j int) bool { return segSeqs[i] < segSeqs[j] })
	sort.Strings(legacy)

	byName := make(map[string]RingRecord, len(legacy))
	segments := make([]ringSegMeta, 0, len(segSeqs))
	var lastSeq uint64
	var lastEntries int
	var lastBytes int64
	var lastTorn bool

	for i, seq := range segSeqs {
		path := filepath.Join(dir, ringSegmentName(seq))
		isNewest := i == len(segSeqs)-1
		recs, size, torn, rerr := readSegmentFile(path, cutTail && isNewest)
		if rerr != nil {
			continue
		}
		segments = append(segments, ringSegMeta{seq: seq, count: len(recs)})
		for _, rec := range recs {
			if _, exists := byName[rec.Name]; !exists {
				byName[rec.Name] = rec
			}
		}
		if isNewest {
			lastSeq = seq
			lastEntries = len(recs)
			lastBytes = size
			lastTorn = torn
		}
	}

	// Only read at most the newest RingHistoryCap legacy files that are not
	// already covered by segment entries.
	legacyToRead := legacy
	if len(legacyToRead) > RingHistoryCap {
		legacyToRead = legacyToRead[len(legacyToRead)-RingHistoryCap:]
	}
	for _, name := range legacyToRead {
		if _, exists := byName[name]; exists {
			continue
		}
		if rec, ok := readLegacyRingEntry(dir, name); ok {
			byName[name] = rec
		}
	}

	records := make([]RingRecord, 0, len(byName))
	for _, rec := range byName {
		records = append(records, rec)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Name < records[j].Name })
	if len(records) > RingHistoryCap {
		records = records[len(records)-RingHistoryCap:]
	}

	return loadedRing{
		records:     records,
		segments:    segments,
		legacyNames: legacy,
		segSeq:      lastSeq,
		segEntries:  lastEntries,
		segBytes:    lastBytes,
		tornTail:    lastTorn,
	}, nil
}

func makeRingName(entry RingEntry) (string, time.Time, error) {
	var suffix [4]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return "", time.Time{}, err
	}
	stamp := entry.ringStamp
	if stamp.IsZero() {
		stamp = nextRingStamp()
	}
	stamp = stamp.UTC()
	name := stamp.Format(ringStampLayout) + "-" + hex.EncodeToString(suffix[:]) + ".json"
	return name, stamp, nil
}

func encodeSegmentLines(records []RingRecord) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	for _, rec := range records {
		if err := enc.Encode(ringSegmentLine{Name: rec.Name, Entry: rec.RingEntry}); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

func (rm *ringMem) openSegmentLocked(seq uint64) error {
	if err := os.MkdirAll(rm.dir, 0o700); err != nil {
		return fmt.Errorf("create conversation ring dir: %w", err)
	}
	path := filepath.Join(rm.dir, ringSegmentName(seq))
	f, err := wsstate.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open conversation ring segment: %w", err)
	}
	rm.file = f
	rm.segSeq = seq
	return nil
}

func (rm *ringMem) ensureWriterLocked() error {
	if rm.file != nil {
		return nil
	}
	if rm.segSeq == 0 {
		if err := rm.openSegmentLocked(1); err != nil {
			return err
		}
		rm.segments = append(rm.segments, ringSegMeta{seq: 1, count: 0})
		rm.segEntries = 0
		rm.segBytes = 0
		return nil
	}
	// If the newest segment is already at or above rotation thresholds, open a
	// fresh segment rather than appending past the bound.
	if rm.segEntries >= ringSegmentEntries || rm.segBytes >= ringSegmentBytes {
		rm.tornTail = false
		return rm.rotateLocked(true)
	}
	if rm.tornTail {
		_ = os.Truncate(filepath.Join(rm.dir, ringSegmentName(rm.segSeq)), rm.segBytes)
		rm.tornTail = false
	}
	return rm.openSegmentLocked(rm.segSeq)
}

func (rm *ringMem) rotateLocked(openNew bool) error {
	if rm.file != nil {
		_ = rm.file.Close()
		rm.file = nil
	}
	nextSeq := rm.segSeq + 1
	if openNew {
		if err := rm.openSegmentLocked(nextSeq); err != nil {
			return err
		}
		rm.segments = append(rm.segments, ringSegMeta{seq: nextSeq, count: 0})
		rm.segEntries = 0
		rm.segBytes = 0
	}
	rm.pruneOldSegmentsLocked()
	return nil
}

func (rm *ringMem) pruneOldSegmentsLocked() {
	total := 0
	for _, s := range rm.segments {
		total += s.count
	}
	for len(rm.segments) > 1 && (total-rm.segments[0].count >= RingHistoryCap || len(rm.segments) > ringMaxSegments) {
		oldest := rm.segments[0]
		_ = os.Remove(filepath.Join(rm.dir, ringSegmentName(oldest.seq)))
		total -= oldest.count
		rm.segments = rm.segments[1:]
	}
	if len(rm.legacyNames) > 0 && len(rm.records) >= RingHistoryCap {
		cutoff := rm.records[0].Name
		kept := rm.legacyNames[:0]
		for _, name := range rm.legacyNames {
			if name < cutoff {
				_ = os.Remove(filepath.Join(rm.dir, name))
			} else {
				kept = append(kept, name)
			}
		}
		rm.legacyNames = kept
	}
}

func (rm *ringMem) insertRecordsLocked(recs []RingRecord) {
	needSort := false
	for _, rec := range recs {
		if len(rm.records) > 0 && rec.Name <= rm.records[len(rm.records)-1].Name {
			needSort = true
		}
		rm.records = append(rm.records, rec)
	}
	if needSort {
		sort.Slice(rm.records, func(i, j int) bool { return rm.records[i].Name < rm.records[j].Name })
		// Dedupe by Name, preserving the first occurrence.
		dedup := rm.records[:0]
		for i, rec := range rm.records {
			if i > 0 && rec.Name == dedup[len(dedup)-1].Name {
				continue
			}
			dedup = append(dedup, rec)
		}
		rm.records = dedup
	}
	if len(rm.records) > RingHistoryCap {
		rm.records = append([]RingRecord(nil), rm.records[len(rm.records)-RingHistoryCap:]...)
	}
}

func (rm *ringMem) appendRecordsLocked(records []RingRecord, keepOpen bool) error {
	if len(records) == 0 {
		return nil
	}
	// Split very large batches across segments so each segment stays bounded.
	pos := 0
	for pos < len(records) {
		if err := rm.ensureWriterLocked(); err != nil {
			return err
		}
		room := ringSegmentEntries - rm.segEntries
		if room <= 0 {
			if err := rm.rotateLocked(true); err != nil {
				return err
			}
			room = ringSegmentEntries
		}
		chunk := records[pos:]
		if len(chunk) > room {
			chunk = chunk[:room]
		}
		raw, err := encodeSegmentLines(chunk)
		if err != nil {
			return err
		}
		n, werr := rm.file.Write(raw)
		if werr != nil {
			if n > 0 {
				_ = rm.file.Truncate(rm.segBytes)
			}
			_ = rm.file.Close()
			rm.file = nil
			return fmt.Errorf("write conversation ring segment: %w", werr)
		}
		rm.segEntries += len(chunk)
		rm.segBytes += int64(n)
		if len(rm.segments) > 0 {
			rm.segments[len(rm.segments)-1].count += len(chunk)
		}
		rm.insertRecordsLocked(chunk)
		pos += len(chunk)
		if rm.segEntries >= ringSegmentEntries || rm.segBytes >= ringSegmentBytes {
			// Rotate and prune old whole segments; only open the next segment
			// immediately if more records remain in this batch.
			if err := rm.rotateLocked(pos < len(records)); err != nil {
				return err
			}
		}
	}
	if !keepOpen && rm.file != nil {
		_ = rm.file.Close()
		rm.file = nil
	}
	return nil
}

func appendRingEntries(dir string, entries []RingEntry) error {
	if len(entries) == 0 {
		return nil
	}
	records := make([]RingRecord, 0, len(entries))
	for _, entry := range entries {
		name, at, err := makeRingName(entry)
		if err != nil {
			return err
		}
		records = append(records, RingRecord{RingEntry: entry, Name: name, At: at})
	}
	if rm := cachedRing(dir); rm != nil {
		rm.mu.Lock()
		defer rm.mu.Unlock()
		if !rm.closed {
			if err := rm.ensureLoadedLocked(true); err != nil {
				return err
			}
			return rm.appendRecordsLocked(records, true)
		}
	}
	clean := filepath.Clean(dir)
	mu := directLock(clean)
	mu.Lock()
	defer mu.Unlock()
	rm := &ringMem{dir: clean}
	if err := rm.ensureLoadedLocked(true); err != nil {
		return err
	}
	return rm.appendRecordsLocked(records, false)
}

// appendRingNamed writes one pre-named frozen entry (SummaryCommits) into the
// append-only segment, refusing conflicting bytes for an already-present name.
func appendRingNamed(dir, name string, raw []byte) error {
	at, ok := parseRingNameStamp(name)
	if !ok {
		return fmt.Errorf("invalid ring entry name %q", name)
	}
	var entry RingEntry
	if err := json.Unmarshal(raw, &entry); err != nil {
		return err
	}
	rec := RingRecord{RingEntry: entry, Name: name, At: at}
	checkAndAppend := func(rm *ringMem, keepOpen bool) error {
		if err := rm.ensureLoadedLocked(true); err != nil {
			return err
		}
		idx := sort.Search(len(rm.records), func(i int) bool { return rm.records[i].Name >= name })
		if idx < len(rm.records) && rm.records[idx].Name == name {
			prior, err := json.Marshal(rm.records[idx].RingEntry)
			if err == nil && bytes.Equal(prior, raw) {
				return nil
			}
			return fmt.Errorf("immutable summary turn already exists with different bytes")
		}
		return rm.appendRecordsLocked([]RingRecord{rec}, keepOpen)
	}
	if rm := cachedRing(dir); rm != nil {
		rm.mu.Lock()
		defer rm.mu.Unlock()
		if !rm.closed {
			return checkAndAppend(rm, true)
		}
	}
	clean := filepath.Clean(dir)
	mu := directLock(clean)
	mu.Lock()
	defer mu.Unlock()
	rm := &ringMem{dir: clean}
	return checkAndAppend(rm, false)
}

func sliceRingPage(recs []RingRecord, before string, n int) RingPage {
	if len(recs) == 0 {
		return RingPage{End: true}
	}
	if before != "" {
		cut := sort.Search(len(recs), func(i int) bool { return recs[i].Name >= before })
		recs = recs[:cut]
	}
	if n < 0 {
		n = 0
	}
	start := len(recs) - n
	if start < 0 {
		start = 0
	}
	page := RingPage{End: start == 0}
	recs = recs[start:]
	if len(recs) > 0 {
		page.Before = recs[0].Name
		page.Records = make([]RingRecord, len(recs))
		copy(page.Records, recs)
	}
	return page
}

func listRingNames(dir string) ([]string, error) {
	if rm := cachedRing(dir); rm != nil {
		rm.mu.Lock()
		defer rm.mu.Unlock()
		if !rm.closed {
			if err := rm.ensureLoadedLocked(false); err != nil {
				return nil, err
			}
			if len(rm.records) == 0 {
				return nil, nil
			}
			names := make([]string, len(rm.records))
			for i, r := range rm.records {
				names[i] = r.Name
			}
			return names, nil
		}
	}
	st, err := loadRingDir(filepath.Clean(dir), false)
	if err != nil {
		return nil, err
	}
	if len(st.records) == 0 {
		return nil, nil
	}
	names := make([]string, len(st.records))
	for i, r := range st.records {
		names[i] = r.Name
	}
	return names, nil
}

func readRingPage(dir, before string, n int) (RingPage, error) {
	if rm := cachedRing(dir); rm != nil {
		rm.mu.Lock()
		defer rm.mu.Unlock()
		if !rm.closed {
			if err := rm.ensureLoadedLocked(false); err != nil {
				return RingPage{}, err
			}
			return sliceRingPage(rm.records, before, n), nil
		}
	}
	st, err := loadRingDir(filepath.Clean(dir), false)
	if err != nil {
		return RingPage{}, err
	}
	return sliceRingPage(st.records, before, n), nil
}

func readRingByName(dir, name string) (RingRecord, bool) {
	if _, ok := parseRingNameStamp(name); !ok {
		return RingRecord{}, false
	}
	if rm := cachedRing(dir); rm != nil {
		rm.mu.Lock()
		defer rm.mu.Unlock()
		if !rm.closed {
			if err := rm.ensureLoadedLocked(false); err != nil {
				return RingRecord{}, false
			}
			idx := sort.Search(len(rm.records), func(i int) bool { return rm.records[i].Name >= name })
			if idx < len(rm.records) && rm.records[idx].Name == name {
				return rm.records[idx], true
			}
			return RingRecord{}, false
		}
	}
	st, err := loadRingDir(filepath.Clean(dir), false)
	if err != nil {
		return RingRecord{}, false
	}
	idx := sort.Search(len(st.records), func(i int) bool { return st.records[i].Name >= name })
	if idx < len(st.records) && st.records[idx].Name == name {
		return st.records[idx], true
	}
	return RingRecord{}, false
}
