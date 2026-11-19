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
// Only sessions explicitly pinned while holding an active daemon HostedSlot
// (PinRing / UnpinRing) keep their ring in RAM and append to owner-only JSONL
// segment files seg-<16-digit-seq>.jsonl (mode 0600, in 0700 dir). Because the
// daemon holds dashboard.lock and unpins all rings before releasing it, at most
// one process ever writes, truncates, rotates, or prunes seg-*.jsonl.
//
// Unpinned/external writers (standalone ws-mcp serve --stdio, direct ws-mcp
// call, a lingering predecessor daemon after Close, or non-slot in-daemon
// callers) write atomic per-entry <stamp>-<hex>.json files and immediately
// close their fd, so they never race on segment files or hold handles open.
// Readers merge seg-*.jsonl and legacy .json files by Name on load, and
// SyncExternalRing merges external .json appends into a pinned RAM ring on
// external nudge/reread without disturbing the held segment writer.

var (
	// Segment rotation bounds and I/O seams (tests may override).
	ringSegmentEntries       = 500
	ringSegmentBytes   int64 = 1 << 20 // 1 MiB
	ringMaxSegments          = 16
	ringReadFile             = wsstate.ReadFileOnce
	ringTruncate             = os.Truncate
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

// ringSegmentNameOnly decodes only the entry name from a JSONL line so
// unpinned RingNames calls skip RingEntry body decoding.
type ringSegmentNameOnly struct {
	Name string `json:"name"`
}

type ringSegMeta struct {
	seq   uint64
	count int
}

type ringMem struct {
	mu          sync.Mutex
	dir         string
	refs        int
	loaded      bool
	closed      bool
	records     []RingRecord // newest <= RingHistoryCap, sorted ascending by Name
	segments    []ringSegMeta
	legacyNames []string            // legacy .json files seen on disk
	legacySet   map[string]struct{} // fast membership check for SyncExternalRing
	file        *os.File
	segSeq      uint64
	segEntries  int
	segBytes    int64
	tornTail    bool // newest segment on disk has a torn trailing line not yet truncated
	forceRotate bool // newest segment disappeared; rotate to segSeq+1 on next write
	gen         uint64
}

var (
	ringPinsMu sync.Mutex
	ringPins   = map[string]*ringMem{} // normalized ring dir -> *ringMem
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

// PinRing pins the conversation ring at dir into daemon RAM and enables
// held-open append-only JSONL segment writes for it while a HostedSlot owns
// the session. Calls are reference-counted per dir; the returned release
// function is idempotent.
func PinRing(dir string) func() error {
	clean := filepath.Clean(dir)
	key := normPathKey(clean)
	ringPinsMu.Lock()
	rm := ringPins[key]
	if rm == nil {
		rm = &ringMem{dir: clean, legacySet: map[string]struct{}{}}
		ringPins[key] = rm
	}
	rm.refs++
	ringPinsMu.Unlock()

	var once sync.Once
	var err error
	return func() error {
		once.Do(func() {
			err = unpinInstance(key, rm)
		})
		return err
	}
}

func unpinInstance(key string, target *ringMem) error {
	ringPinsMu.Lock()
	rm := ringPins[key]
	if rm == nil || (target != nil && rm != target) {
		ringPinsMu.Unlock()
		return nil
	}
	rm.refs--
	if rm.refs > 0 {
		ringPinsMu.Unlock()
		return nil
	}
	delete(ringPins, key)
	ringPinsMu.Unlock()
	return rm.close()
}

// UnpinRing decrements the pin count for dir; when the last pin releases, it
// drops the RAM cache entry and closes any held segment file descriptor.
func UnpinRing(dir string) error {
	return unpinInstance(normPathKey(dir), nil)
}

// UnpinRingsUnder unpins every ring under sessionsRoot and closes all held
// segment file descriptors. Called on daemon shutdown before releasing the
// daemon instance lock.
func UnpinRingsUnder(sessionsRoot string) error {
	cleanRoot := filepath.Clean(sessionsRoot)
	var list []*ringMem
	ringPinsMu.Lock()
	for key, rm := range ringPins {
		if pathUnderRoot(rm.dir, cleanRoot) {
			delete(ringPins, key)
			list = append(list, rm)
		}
	}
	ringPinsMu.Unlock()

	var errs []error
	for _, rm := range list {
		if err := rm.close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// DropRing forcibly evicts dir from the pinned RAM cache and closes any open
// segment file descriptor so directory removal succeeds on Windows.
func DropRing(dir string) {
	key := normPathKey(dir)
	ringPinsMu.Lock()
	rm := ringPins[key]
	delete(ringPins, key)
	ringPinsMu.Unlock()
	if rm != nil {
		_ = rm.close()
	}
}

// RingPinned reports whether dir currently has a pinned RAM ring cache entry.
func RingPinned(dir string) bool {
	return pinnedRing(dir) != nil
}

// pinnedRingCount returns the number of currently pinned ring directories (test seam).
func pinnedRingCount() int {
	ringPinsMu.Lock()
	defer ringPinsMu.Unlock()
	return len(ringPins)
}

func pinnedRing(dir string) *ringMem {
	key := normPathKey(dir)
	ringPinsMu.Lock()
	rm := ringPins[key]
	ringPinsMu.Unlock()
	return rm
}

func (rm *ringMem) close() error {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.closed = true
	if rm.file != nil {
		err := rm.file.Close()
		rm.file = nil
		return err
	}
	return nil
}

// SyncExternalRing merges any external legacy .json entries (or higher-seq
// segment files) written outside the pinned daemon writer into RAM without
// closing the held segment fd or rereading already-loaded segments.
func SyncExternalRing(dir string) {
	rm := pinnedRing(dir)
	if rm == nil {
		return
	}
	rm.mu.Lock()
	defer rm.mu.Unlock()
	if rm.closed || !rm.loaded {
		return
	}
	dirEntries, err := wsstate.ReadDir(rm.dir)
	if err != nil {
		return
	}
	var newLegacy []string
	var newSegs []uint64
	for _, de := range dirEntries {
		if de.IsDir() {
			continue
		}
		name := de.Name()
		if isRingEntryName(name) {
			if _, seen := rm.legacySet[name]; !seen {
				newLegacy = append(newLegacy, name)
			}
		} else if seq, ok := parseRingSegmentSeq(name); ok && seq > rm.segSeq {
			newSegs = append(newSegs, seq)
		}
	}
	if len(newLegacy) == 0 && len(newSegs) == 0 {
		return
	}
	sort.Strings(newLegacy)
	sort.Slice(newSegs, func(i, j int) bool { return newSegs[i] < newSegs[j] })

	var added []RingRecord
	for _, name := range newLegacy {
		if rec, ok := readLegacyRingEntry(rm.dir, name); ok {
			added = append(added, rec)
			rm.legacyNames = append(rm.legacyNames, name)
			rm.legacySet[name] = struct{}{}
		}
	}
	for _, seq := range newSegs {
		recs, _, _, rerr := readSegmentFile(filepath.Join(rm.dir, ringSegmentName(seq)), false)
		if rerr != nil {
			continue
		}
		added = append(added, recs...)
		if seq > rm.segSeq {
			rm.segSeq = seq
			rm.forceRotate = true
		}
	}
	if len(added) > 0 {
		rm.insertRecordsLocked(added)
	}
}

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
	rm.legacySet = make(map[string]struct{}, len(st.legacyNames))
	for _, name := range st.legacyNames {
		rm.legacySet[name] = struct{}{}
	}
	rm.segSeq = st.segSeq
	rm.segEntries = st.segEntries
	rm.segBytes = st.segBytes
	rm.tornTail = st.tornTail
	rm.forceRotate = st.forceRotate
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
	forceRotate bool
}

func parseRingNameStamp(name string) (time.Time, bool) {
	if !ValidRingName(name) {
		return time.Time{}, false
	}
	dash := strings.IndexByte(name, '-')
	if dash <= 0 {
		return time.Time{}, false
	}
	at, err := time.Parse(ringStampLayout, name[:dash])
	if err != nil {
		return time.Time{}, false
	}
	return at, true
}

func readSegmentFile(path string, cutTail bool) (records []RingRecord, validBytes int64, torn bool, err error) {
	raw, err := ringReadFile(path)
	if err != nil {
		return nil, 0, false, err
	}
	if len(raw) == 0 {
		return nil, 0, false, nil
	}
	if raw[len(raw)-1] != '\n' {
		lastNL := bytes.LastIndexByte(raw, '\n')
		if lastNL < 0 {
			raw = nil
			validBytes = 0
		} else {
			raw = raw[:lastNL+1]
			validBytes = int64(len(raw))
		}
		torn = true
	} else {
		validBytes = int64(len(raw))
	}
	offset := 0
	for offset < len(raw) {
		nl := bytes.IndexByte(raw[offset:], '\n')
		if nl < 0 {
			break
		}
		line := bytes.TrimSpace(raw[offset : offset+nl])
		offset += nl + 1
		if len(line) == 0 {
			continue
		}
		var item ringSegmentLine
		if err := json.Unmarshal(line, &item); err != nil {
			continue
		}
		at, ok := parseRingNameStamp(item.Name)
		if !ok {
			continue
		}
		records = append(records, RingRecord{RingEntry: item.Entry, Name: item.Name, At: at})
	}
	if torn && cutTail {
		if terr := ringTruncate(path, validBytes); terr == nil {
			torn = false
		}
	}
	return records, validBytes, torn, nil
}

func readSegmentNames(path string) ([]string, error) {
	raw, err := ringReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, nil
	}
	if raw[len(raw)-1] != '\n' {
		lastNL := bytes.LastIndexByte(raw, '\n')
		if lastNL < 0 {
			return nil, nil
		}
		raw = raw[:lastNL+1]
	}
	var names []string
	offset := 0
	for offset < len(raw) {
		nl := bytes.IndexByte(raw[offset:], '\n')
		if nl < 0 {
			break
		}
		line := bytes.TrimSpace(raw[offset : offset+nl])
		offset += nl + 1
		if len(line) == 0 {
			continue
		}
		var item ringSegmentNameOnly
		if err := json.Unmarshal(line, &item); err != nil {
			continue
		}
		if _, ok := parseRingNameStamp(item.Name); !ok {
			continue
		}
		names = append(names, item.Name)
	}
	return names, nil
}

// loadRingDir reads all segment files and legacy .json files in dir once,
// merges them by Name, sorts ascending, and caps to RingHistoryCap.
// Non-ErrNotExist segment read errors are returned rather than silently
// dropping the newest segment and resetting segSeq to 0.
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
	var forceRotate bool

	for i, seq := range segSeqs {
		path := filepath.Join(dir, ringSegmentName(seq))
		isNewest := i == len(segSeqs)-1
		recs, size, torn, rerr := readSegmentFile(path, cutTail && isNewest)
		if rerr != nil {
			if errors.Is(rerr, fs.ErrNotExist) {
				if isNewest {
					// Preserve monotonic sequence so we never reopen seg-1.
					lastSeq = seq
					forceRotate = true
				}
				continue
			}
			return loadedRing{}, fmt.Errorf("read conversation ring segment %s: %w", ringSegmentName(seq), rerr)
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
		forceRotate: forceRotate,
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
	for _, rec := range records {
		raw, err := json.Marshal(ringSegmentLine{Name: rec.Name, Entry: rec.RingEntry})
		if err != nil {
			return nil, err
		}
		buf.Write(raw)
		buf.WriteByte('\n')
	}
	return buf.Bytes(), nil
}

func (rm *ringMem) openSegmentLocked(seq uint64) error {
	if err := os.MkdirAll(rm.dir, 0o700); err != nil {
		return fmt.Errorf("create conversation ring: %w", err)
	}
	path := filepath.Join(rm.dir, ringSegmentName(seq))
	f, err := wsstate.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
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
	// If the newest segment disappeared or is already at/above rotation bounds,
	// open a fresh segment rather than appending past the bound.
	if rm.forceRotate || rm.segEntries >= ringSegmentEntries || rm.segBytes >= ringSegmentBytes {
		rm.forceRotate = false
		rm.tornTail = false
		return rm.rotateLocked(true)
	}
	if rm.tornTail {
		if err := ringTruncate(filepath.Join(rm.dir, ringSegmentName(rm.segSeq)), rm.segBytes); err != nil {
			// Cannot safely truncate the torn tail: rotate to a fresh segment
			// so the next append is never glued onto a partial line.
			rm.tornTail = false
			return rm.rotateLocked(true)
		}
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
				delete(rm.legacySet, name)
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
	if len(recs) > 0 {
		rm.gen++
	}
}

func (rm *ringMem) appendRecordsLocked(records []RingRecord) error {
	if len(records) == 0 {
		return nil
	}
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
				if terr := rm.file.Truncate(rm.segBytes); terr != nil {
					rm.tornTail = true
				}
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
			if err := rm.rotateLocked(pos < len(records)); err != nil {
				return err
			}
		}
	}
	return nil
}

// writeLegacyRingRaw writes one atomic owner-only per-entry .json file and
// immediately closes its handle (used by unpinned/external writers so they
// never contend on segment files or leave handles open).
func writeLegacyRingRaw(dir, name string, raw []byte) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create conversation ring: %w", err)
	}
	tmp, err := wsstate.CreateTemp(dir, ".ring-*.tmp")
	if err != nil {
		return fmt.Errorf("write conversation ring: %w", err)
	}
	tmpPath := tmp.Name()
	if err := tmp.Chmod(0o600); err != nil && runtime.GOOS != "windows" {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write conversation ring: %w", err)
	}
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write conversation ring: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write conversation ring: %w", err)
	}
	wsstate.CountFileIO("atomic_replace")
	if err := os.Rename(tmpPath, filepath.Join(dir, name)); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write conversation ring: %w", err)
	}
	return nil
}

// trimLegacyRing removes only surplus legacy .json files past RingHistoryCap,
// never touching seg-*.jsonl segment files owned by a daemon writer.
func trimLegacyRing(dir string) {
	entries, err := wsstate.ReadDir(dir)
	if err != nil {
		return
	}
	var names []string
	for _, de := range entries {
		if !de.IsDir() && isRingEntryName(de.Name()) {
			names = append(names, de.Name())
		}
	}
	if len(names) <= RingHistoryCap {
		return
	}
	sort.Strings(names)
	for _, name := range names[:len(names)-RingHistoryCap] {
		_ = os.Remove(filepath.Join(dir, name))
	}
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
	if rm := pinnedRing(dir); rm != nil {
		rm.mu.Lock()
		defer rm.mu.Unlock()
		if !rm.closed {
			if err := rm.ensureLoadedLocked(true); err != nil {
				return err
			}
			return rm.appendRecordsLocked(records)
		}
	}
	clean := filepath.Clean(dir)
	for _, rec := range records {
		raw, err := json.Marshal(rec.RingEntry)
		if err != nil {
			return err
		}
		if err := writeLegacyRingRaw(clean, rec.Name, raw); err != nil {
			return err
		}
	}
	trimLegacyRing(clean)
	return nil
}

// appendRingNamed writes one pre-named frozen entry (SummaryCommits), refusing
// conflicting bytes for an already-present name. Pinned hosted rings append to
// their active JSONL segment; unpinned callers write an atomic .json file so
// they never race with or hold open a segment file.
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
	if rm := pinnedRing(dir); rm != nil {
		rm.mu.Lock()
		defer rm.mu.Unlock()
		if !rm.closed {
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
			return rm.appendRecordsLocked([]RingRecord{rec})
		}
	}
	clean := filepath.Clean(dir)
	if existing, found := readRingByName(clean, name); found {
		prior, err := json.Marshal(existing.RingEntry)
		if err == nil && bytes.Equal(prior, raw) {
			return nil
		}
		return fmt.Errorf("immutable summary turn already exists with different bytes")
	}
	if err := writeLegacyRingRaw(clean, name, raw); err != nil {
		return err
	}
	trimLegacyRing(clean)
	return nil
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

// RingFingerprint returns compact change-detection tokens for the ring at dir
// without reading file bodies or pinning closed/non-hosted rings in RAM.
// Pinned hosted rings are served directly from RAM with zero disk operations.
func RingFingerprint(dir string) ([]string, error) {
	if rm := pinnedRing(dir); rm != nil {
		rm.mu.Lock()
		defer rm.mu.Unlock()
		if !rm.closed {
			if err := rm.ensureLoadedLocked(false); err != nil {
				return nil, err
			}
			if len(rm.records) == 0 {
				return nil, nil
			}
			last := rm.records[len(rm.records)-1].Name
			return []string{fmt.Sprintf("mem:%d:%d:%s", rm.gen, len(rm.records), last)}, nil
		}
	}
	dirEntries, err := wsstate.ReadDir(filepath.Clean(dir))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	tokens := make([]string, 0, len(dirEntries))
	for _, de := range dirEntries {
		if de.IsDir() {
			continue
		}
		name := de.Name()
		if _, ok := parseRingSegmentSeq(name); ok {
			if info, ierr := de.Info(); ierr == nil {
				tokens = append(tokens, fmt.Sprintf("%s:%d:%d", name, info.Size(), info.ModTime().UnixNano()))
			} else {
				tokens = append(tokens, name)
			}
		} else if isRingEntryName(name) {
			tokens = append(tokens, name)
		}
	}
	sort.Strings(tokens)
	return tokens, nil
}

func listRingNames(dir string) ([]string, error) {
	if rm := pinnedRing(dir); rm != nil {
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
	clean := filepath.Clean(dir)
	dirEntries, err := wsstate.ReadDir(clean)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var segSeqs []uint64
	seen := map[string]struct{}{}
	for _, de := range dirEntries {
		if de.IsDir() {
			continue
		}
		name := de.Name()
		if seq, ok := parseRingSegmentSeq(name); ok {
			segSeqs = append(segSeqs, seq)
		} else if isRingEntryName(name) {
			seen[name] = struct{}{}
		}
	}
	sort.Slice(segSeqs, func(i, j int) bool { return segSeqs[i] < segSeqs[j] })
	for _, seq := range segSeqs {
		segNames, rerr := readSegmentNames(filepath.Join(clean, ringSegmentName(seq)))
		if rerr != nil {
			if errors.Is(rerr, fs.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("read conversation ring segment %s: %w", ringSegmentName(seq), rerr)
		}
		for _, name := range segNames {
			seen[name] = struct{}{}
		}
	}
	if len(seen) == 0 {
		return nil, nil
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) > RingHistoryCap {
		names = names[len(names)-RingHistoryCap:]
	}
	return names, nil
}

func readRingPage(dir, before string, n int) (RingPage, error) {
	if rm := pinnedRing(dir); rm != nil {
		rm.mu.Lock()
		defer rm.mu.Unlock()
		if !rm.closed {
			if err := rm.ensureLoadedLocked(false); err != nil {
				return RingPage{}, err
			}
			return sliceRingPage(rm.records, before, n), nil
		}
	}
	return readRingPageWindow(filepath.Clean(dir), before, n)
}

// readRingPageWindow reads only the window needed to satisfy an unpinned
// ReadRingPage(dir, before, n) call: legacy .json files are filtered by
// `< before` and capped to the newest n before opening any file, and segments
// are scanned newest-first stopping as soon as the window (plus one older
// entry to settle End) is covered.
func readRingPageWindow(dir, before string, n int) (RingPage, error) {
	if n < 0 {
		n = 0
	}
	dirEntries, err := wsstate.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return RingPage{End: true}, nil
	}
	if err != nil {
		return RingPage{}, err
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

	if before != "" {
		legacy = legacy[:sort.SearchStrings(legacy, before)]
	}

	// Fast path: legacy-only directory (no segments) reads at most n files.
	if len(segSeqs) == 0 {
		start := len(legacy) - n
		if start < 0 {
			start = 0
		}
		page := RingPage{End: start == 0}
		window := legacy[start:]
		if len(window) > 0 {
			page.Before = window[0]
			page.Records = make([]RingRecord, 0, len(window))
			for _, name := range window {
				if rec, ok := readLegacyRingEntry(dir, name); ok {
					page.Records = append(page.Records, rec)
				}
			}
		}
		return page, nil
	}

	hasOlder := false
	if len(legacy) > n {
		hasOlder = true
		legacy = legacy[len(legacy)-n:]
	}

	byName := make(map[string]RingRecord, n+1)
	for i := len(segSeqs) - 1; i >= 0; i-- {
		seq := segSeqs[i]
		recs, _, _, rerr := readSegmentFile(filepath.Join(dir, ringSegmentName(seq)), false)
		if rerr != nil {
			if errors.Is(rerr, fs.ErrNotExist) {
				continue
			}
			return RingPage{}, fmt.Errorf("read conversation ring segment %s: %w", ringSegmentName(seq), rerr)
		}
		for _, rec := range recs {
			if before != "" && rec.Name >= before {
				continue
			}
			if _, exists := byName[rec.Name]; !exists {
				byName[rec.Name] = rec
			}
		}
		if len(byName) > n {
			hasOlder = true
			break
		}
	}

	// Skip reading legacy .json files strictly older than the n-th newest
	// segment record we already have; their presence only marks hasOlder.
	var cutoff string
	if n > 0 && len(byName) >= n {
		segNames := make([]string, 0, len(byName))
		for name := range byName {
			segNames = append(segNames, name)
		}
		sort.Strings(segNames)
		cutoff = segNames[len(segNames)-n]
	}
	for _, name := range legacy {
		if _, exists := byName[name]; exists {
			continue
		}
		if cutoff != "" && name < cutoff {
			hasOlder = true
			continue
		}
		if n == 0 {
			hasOlder = true
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
	if len(records) > n {
		hasOlder = true
		records = records[len(records)-n:]
	}
	page := RingPage{End: !hasOlder}
	if len(records) > 0 {
		page.Before = records[0].Name
		page.Records = records
	}
	return page, nil
}

func readRingByName(dir, name string) (RingRecord, bool) {
	if _, ok := parseRingNameStamp(name); !ok {
		return RingRecord{}, false
	}
	if rm := pinnedRing(dir); rm != nil {
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
	clean := filepath.Clean(dir)
	// Fast path: unpinned legacy entry file exists directly by name.
	if rec, ok := readLegacyRingEntry(clean, name); ok {
		return rec, true
	}
	// Otherwise scan only segment files newest-first, stopping on match.
	dirEntries, err := wsstate.ReadDir(clean)
	if err != nil {
		return RingRecord{}, false
	}
	var segSeqs []uint64
	for _, de := range dirEntries {
		if !de.IsDir() {
			if seq, ok := parseRingSegmentSeq(de.Name()); ok {
				segSeqs = append(segSeqs, seq)
			}
		}
	}
	sort.Slice(segSeqs, func(i, j int) bool { return segSeqs[i] < segSeqs[j] })
	for i := len(segSeqs) - 1; i >= 0; i-- {
		recs, _, _, rerr := readSegmentFile(filepath.Join(clean, ringSegmentName(segSeqs[i])), false)
		if rerr != nil {
			continue
		}
		for _, rec := range recs {
			if rec.Name == name {
				return rec, true
			}
		}
	}
	return RingRecord{}, false
}
