package mcp

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsstate"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

type whiteboardMeasurementFile struct {
	Size int64
	Sum  [sha256.Size]byte
}

type whiteboardMeasurementDelta struct {
	Created      int   `json:"created"`
	Modified     int   `json:"modified"`
	Removed      int   `json:"removed"`
	BytesWritten int64 `json:"observed_sizes_of_changed_files"`
}

type whiteboardMeasurementRow struct {
	Index      int                                   `json:"index"`
	Phase      string                                `json:"phase"`
	WallNS     int64                                 `json:"wall_ns"`
	FileIO     map[string]int64                      `json:"file_io"`
	Filesystem map[string]whiteboardMeasurementDelta `json:"filesystem"`
	WAL        whiteboardMeasurementWALDelta         `json:"wal"`
}

type whiteboardStoreMeasurement struct {
	WallNS int64                         `json:"wall_ns"`
	FileIO map[string]int64              `json:"file_io"`
	WAL    whiteboardMeasurementWALDelta `json:"wal"`
}

type whiteboardMeasurementWALDelta struct {
	Status     string `json:"status"`
	BytesDelta int64  `json:"bytes_delta"`
	Frames     int    `json:"new_frames"`
	Commits    int    `json:"commit_markers"`
}

type whiteboardMeasurementCounterStats struct {
	Median int64 `json:"median"`
	P95    int64 `json:"p95"`
}

type whiteboardMeasurementLatency struct {
	MedianNS int64 `json:"median_ns"`
	P95NS    int64 `json:"p95_ns"`
}

type whiteboardMeasurementReport struct {
	Label         string                                       `json:"label"`
	Updates       int                                          `json:"updates"`
	FirstWrite    whiteboardMeasurementLatency                 `json:"first_write"`
	SteadyState   whiteboardMeasurementLatency                 `json:"steady_state"`
	All           whiteboardMeasurementLatency                 `json:"all"`
	Rows          []whiteboardMeasurementRow                   `json:"rows"`
	EmptyAction   whiteboardMeasurementRow                     `json:"empty_action"`
	WrapperSteady map[string]whiteboardMeasurementCounterStats `json:"wrapper_steady_state"`
	Limitations   string                                       `json:"limitations"`
}

// TestWhiteboardUpdateMeasurement is an opt-in before/after harness. It uses
// the same top-level tool dispatch and fresh-session fixture as the daemon's
// acceptance tests. The filesystem snapshots cross-check FileIOCounters for
// direct os.* calls, but intentionally remain a lower bound: an atomic temp
// file created and removed inside one update is absent from both snapshots.
func TestWhiteboardUpdateMeasurement(t *testing.T) {
	raw := os.Getenv("WS_WHITEBOARD_MEASURE_UPDATES")
	if raw == "" {
		t.Skip("set WS_WHITEBOARD_MEASURE_UPDATES to run the whiteboard update measurement")
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 3 {
		t.Fatalf("WS_WHITEBOARD_MEASURE_UPDATES=%q, want an integer >= 3", raw)
	}
	label := os.Getenv("WS_WHITEBOARD_MEASURE_LABEL")
	if label == "" {
		label = "unlabeled"
	}

	f := newWhiteboardFixture(t)
	pageDir := f.pageDir()
	rootDir := f.dir()
	before := whiteboardMeasurementScopes(t, rootDir, pageDir)
	walBefore := whiteboardMeasurementReadWAL(t, pageDir)
	rows := make([]whiteboardMeasurementRow, 0, n)
	const commands = `<wb:topic name="measurement"><wb:panel name="status" recap="Realistic update">Panel body</wb:panel><wb:log>Steady update</wb:log></wb:topic>`
	for i := 0; i < n; i++ {
		countersBefore := wsstate.FileIOCounters()
		started := time.Now()
		f.update(t, commands)
		wall := time.Since(started)
		countersAfter := wsstate.FileIOCounters()
		after := whiteboardMeasurementScopes(t, rootDir, pageDir)
		walAfter := whiteboardMeasurementReadWAL(t, pageDir)
		phase := "steady_state"
		if i == 0 {
			phase = "first_write"
		}
		rows = append(rows, whiteboardMeasurementRow{
			Index:      i + 1,
			Phase:      phase,
			WallNS:     wall.Nanoseconds(),
			FileIO:     whiteboardMeasurementCounterDelta(countersBefore, countersAfter),
			Filesystem: whiteboardMeasurementScopeDelta(before, after),
			WAL:        whiteboardMeasurementCompareWAL(walBefore, walAfter),
		})
		before = after
		walBefore = walAfter
	}

	// An empty command is accepted, but may still change board sequence metadata.
	// Report it separately; do not assume it represents a truly unchanged state.
	countersBefore := wsstate.FileIOCounters()
	started := time.Now()
	f.update(t, "")
	wall := time.Since(started)
	countersAfter := wsstate.FileIOCounters()
	after := whiteboardMeasurementScopes(t, rootDir, pageDir)
	walAfter := whiteboardMeasurementReadWAL(t, pageDir)
	empty := whiteboardMeasurementRow{Phase: "empty_command", WallNS: wall.Nanoseconds(), FileIO: whiteboardMeasurementCounterDelta(countersBefore, countersAfter), Filesystem: whiteboardMeasurementScopeDelta(before, after), WAL: whiteboardMeasurementCompareWAL(walBefore, walAfter)}

	all := make([]int64, 0, len(rows))
	steady := make([]int64, 0, len(rows)-1)
	for _, row := range rows {
		all = append(all, row.WallNS)
		if row.Phase == "steady_state" {
			steady = append(steady, row.WallNS)
		}
	}
	report := whiteboardMeasurementReport{
		Label:         label,
		Updates:       n,
		FirstWrite:    whiteboardMeasurementLatencyOf([]int64{rows[0].WallNS}),
		SteadyState:   whiteboardMeasurementLatencyOf(steady),
		All:           whiteboardMeasurementLatencyOf(all),
		Rows:          rows,
		EmptyAction:   empty,
		WrapperSteady: whiteboardMeasurementWrapperStats(rows[1:]),
		Limitations:   "Supporting evidence only: wsstate wrappers exclude direct OS calls, flock and SQLite VFS; snapshots report observed final file sizes, not bytes written; WAL frame deltas do not count write syscalls or fsync. Empty commands may change sequence metadata.",
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("WHITEBOARD_MEASUREMENT %s", data)
}

// WAL snapshots are taken outside the timed and wrapper-counted action.
func whiteboardMeasurementReadWAL(t *testing.T, dir string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, ".board.sqlite-wal"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func whiteboardMeasurementCompareWAL(before, after []byte) whiteboardMeasurementWALDelta {
	result := whiteboardMeasurementWALDelta{BytesDelta: int64(len(after) - len(before))}
	if len(after) == 0 && len(before) == 0 {
		result.Status = "absent"
		return result
	}
	if len(after) < 32 {
		result.Status = "invalid_or_removed"
		return result
	}
	magic := binary.BigEndian.Uint32(after[:4])
	pageSize := int(binary.BigEndian.Uint32(after[8:12]))
	if pageSize == 1 {
		pageSize = 65536
	}
	if (magic != 0x377f0682 && magic != 0x377f0683) || pageSize < 512 || pageSize > 65536 || pageSize&(pageSize-1) != 0 || (len(after)-32)%(24+pageSize) != 0 {
		result.Status = "invalid_or_partial"
		return result
	}
	start := 32
	if len(before) > 0 {
		if len(before) < 32 || len(before) > len(after) || !bytes.Equal(before, after[:len(before)]) || (len(before)-32)%(24+pageSize) != 0 {
			result.Status = "reset_or_rewritten"
			return result
		}
		start = len(before)
	}
	for offset := start; offset < len(after); offset += 24 + pageSize {
		if !bytes.Equal(after[16:24], after[offset+8:offset+16]) {
			result.Status = "invalid_frame_salt"
			result.Frames = 0
			result.Commits = 0
			return result
		}
		result.Frames++
		if binary.BigEndian.Uint32(after[offset+4:offset+8]) != 0 {
			result.Commits++
		}
	}
	result.Status = "append_observed"
	return result
}

func whiteboardMeasurementWrapperStats(rows []whiteboardMeasurementRow) map[string]whiteboardMeasurementCounterStats {
	keys := map[string]bool{}
	for _, row := range rows {
		for key := range row.FileIO {
			keys[key] = true
		}
	}
	out := map[string]whiteboardMeasurementCounterStats{}
	for key := range keys {
		values := make([]int64, len(rows))
		for i, row := range rows {
			values[i] = row.FileIO[key]
		}
		stats := whiteboardMeasurementLatencyOf(values)
		out[key] = whiteboardMeasurementCounterStats{Median: stats.MedianNS, P95: stats.P95NS}
	}
	return out
}

func whiteboardMeasurementScopes(t *testing.T, rootDir, pageDir string) map[string]map[string]whiteboardMeasurementFile {
	t.Helper()
	stored, wal, derived := map[string]whiteboardMeasurementFile{}, map[string]whiteboardMeasurementFile{}, map[string]whiteboardMeasurementFile{}
	for path, file := range whiteboardMeasurementSnapshot(t, pageDir, "") {
		name := filepath.Base(path)
		if name == ".board.sqlite-wal" {
			wal[path] = file
		} else if name == "index.html" || name == "index.stamp.js" || strings.HasPrefix(name, "wb-mermaid-") {
			derived[path] = file
		} else {
			stored[path] = file
		}
	}
	return map[string]map[string]whiteboardMeasurementFile{
		"authoritative_board_except_wal": stored,
		"wal":                            wal,
		"derived_pages":                  derived,
		"embed_root":                     whiteboardMeasurementSnapshot(t, rootDir, pageDir),
	}
}

func whiteboardMeasurementSnapshot(t *testing.T, root, exclude string) map[string]whiteboardMeasurementFile {
	t.Helper()
	out := map[string]whiteboardMeasurementFile{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if exclude != "" && path == exclude && entry.IsDir() {
			return filepath.SkipDir
		}
		if entry.IsDir() || !entry.Type().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = whiteboardMeasurementFile{Size: int64(len(data)), Sum: sha256.Sum256(data)}
		return nil
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	return out
}

func whiteboardMeasurementCounterDelta(before, after map[string]int64) map[string]int64 {
	out := map[string]int64{}
	for key, value := range after {
		if !strings.HasPrefix(key, "file_io_") || strings.HasPrefix(key, "file_io_site:") {
			continue
		}
		if delta := value - before[key]; delta != 0 {
			out[key] = delta
		}
	}
	return out
}

func whiteboardMeasurementScopeDelta(before, after map[string]map[string]whiteboardMeasurementFile) map[string]whiteboardMeasurementDelta {
	out := map[string]whiteboardMeasurementDelta{}
	all := whiteboardMeasurementDelta{}
	for scope, next := range after {
		delta := whiteboardMeasurementFilesDelta(before[scope], next)
		out[scope] = delta
		all.Created += delta.Created
		all.Modified += delta.Modified
		all.Removed += delta.Removed
		all.BytesWritten += delta.BytesWritten
	}
	out["all_unique"] = all
	return out
}

func whiteboardMeasurementFilesDelta(before, after map[string]whiteboardMeasurementFile) whiteboardMeasurementDelta {
	var out whiteboardMeasurementDelta
	for path, next := range after {
		prior, ok := before[path]
		if !ok {
			out.Created++
			out.BytesWritten += next.Size
			continue
		}
		if prior != next {
			out.Modified++
			out.BytesWritten += next.Size
		}
	}
	for path := range before {
		if _, ok := after[path]; !ok {
			out.Removed++
		}
	}
	return out
}

func whiteboardMeasurementLatencyOf(samples []int64) whiteboardMeasurementLatency {
	ordered := append([]int64(nil), samples...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	middle := len(ordered) / 2
	median := ordered[middle]
	if len(ordered)%2 == 0 {
		median = (ordered[middle-1] + ordered[middle]) / 2
	}
	p95 := ordered[(95*len(ordered)+99)/100-1]
	return whiteboardMeasurementLatency{MedianNS: median, P95NS: p95}
}

// Run alongside the update harness to isolate a panel-only action's cost.
func TestWhiteboardPanelActionMeasurement(t *testing.T) {
	if os.Getenv("WS_WHITEBOARD_MEASURE_UPDATES") == "" {
		t.Skip("opt-in measurement")
	}
	f := newWhiteboardFixture(t)
	f.update(t, `<wb:topic name="measurement"><wb:log>board</wb:log></wb:topic>`)
	before := whiteboardMeasurementScopes(t, f.dir(), f.pageDir())
	counters := wsstate.FileIOCounters()
	start := time.Now()
	resp := f.call(t, "agenda.set", map[string]any{"key": "measurement", "value": map[string]any{"step": 1}})
	elapsed := time.Since(start)
	if isToolError(resp) {
		t.Fatal(resp)
	}
	afterCounters := wsstate.FileIOCounters()
	after := whiteboardMeasurementScopes(t, f.dir(), f.pageDir())
	row := whiteboardMeasurementRow{Index: 1, Phase: "agenda.set", WallNS: elapsed.Nanoseconds(), FileIO: whiteboardMeasurementCounterDelta(counters, afterCounters), Filesystem: whiteboardMeasurementScopeDelta(before, after)}
	data, err := json.Marshal(row)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("WHITEBOARD_PANEL_MEASUREMENT %s", data)
}

func TestWhiteboardMeasurementWALDelta(t *testing.T) {
	wal := make([]byte, 32+2*(24+512))
	binary.BigEndian.PutUint32(wal[0:4], 0x377f0682)
	binary.BigEndian.PutUint32(wal[8:12], 512)
	copy(wal[16:24], []byte("salt1234"))
	for offset := 32; offset < len(wal); offset += 24 + 512 {
		copy(wal[offset+8:offset+16], wal[16:24])
	}
	binary.BigEndian.PutUint32(wal[32+24+512+4:], 2)
	first := append([]byte(nil), wal[:32+24+512]...)
	got := whiteboardMeasurementCompareWAL(first, wal)
	if got.Status != "append_observed" || got.Frames != 1 || got.Commits != 1 || got.BytesDelta != 536 {
		t.Fatalf("append: %+v", got)
	}
	if got := whiteboardMeasurementCompareWAL(wal, wal); got.Status != "append_observed" || got.Frames != 0 || got.Commits != 0 {
		t.Fatalf("unchanged: %+v", got)
	}
	if got := whiteboardMeasurementCompareWAL(wal, first); got.Status != "reset_or_rewritten" {
		t.Fatalf("reset: %+v", got)
	}
	if got := whiteboardMeasurementCompareWAL(nil, wal[:len(wal)-1]); got.Status != "invalid_or_partial" {
		t.Fatalf("partial: %+v", got)
	}
}

// This measures the store API separately because an empty MCP command advances
// sequence metadata and therefore does not represent an unchanged board.
func TestWhiteboardStoreMeasurement(t *testing.T) {
	if os.Getenv("WS_WHITEBOARD_MEASURE_UPDATES") == "" {
		t.Skip("opt-in measurement")
	}
	dir := t.TempDir()
	if _, err := (wswhiteboard.FolderStore{Dir: dir}).Open(&wswhiteboard.State{}); err != nil {
		t.Fatal(err)
	}
	board, err := wswhiteboard.OpenBoardDB(dir, nil, wswhiteboard.BoardDBOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = board.Close() })
	measure := func(update func(*wswhiteboard.State) error) whiteboardStoreMeasurement {
		walBefore := whiteboardMeasurementReadWAL(t, dir)
		countersBefore := wsstate.FileIOCounters()
		started := time.Now()
		if err := board.Update(update); err != nil {
			t.Fatal(err)
		}
		wall := time.Since(started)
		countersAfter := wsstate.FileIOCounters()
		walAfter := whiteboardMeasurementReadWAL(t, dir)
		return whiteboardStoreMeasurement{
			WallNS: wall.Nanoseconds(),
			FileIO: whiteboardMeasurementCounterDelta(countersBefore, countersAfter),
			WAL:    whiteboardMeasurementCompareWAL(walBefore, walAfter),
		}
	}
	changed := measure(func(s *wswhiteboard.State) error { s.Seq++; return nil })
	unchanged := measure(func(*wswhiteboard.State) error { return nil })
	if a := changed.WAL; a.Status != "append_observed" || a.Frames == 0 || a.Commits != 1 {
		t.Fatalf("changed WAL: %+v", a)
	}
	if b := unchanged.WAL; b.Status != "append_observed" || b.Frames != 0 || b.Commits != 0 || b.BytesDelta != 0 {
		t.Fatalf("unchanged WAL: %+v", b)
	}
	data, err := json.Marshal(map[string]any{"scope": "standalone BoardDB.Update; not MCP update", "changed": changed, "unchanged": unchanged, "limitations": "WAL snapshots count frames and commit markers, not write syscalls or fsync"})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("WHITEBOARD_STORE_MEASUREMENT %s", data)
}

// This opt-in sample forces the settled PASSIVE checkpoint between pairs of
// panel-and-log updates. A changed WAL header identifies the reuse cycle; the
// elapsed time includes the whole tool action, rather than isolating xSync.
func TestWhiteboardCheckpointResetMeasurement(t *testing.T) {
	if os.Getenv("WS_WHITEBOARD_MEASURE_UPDATES") == "" {
		t.Skip("opt-in measurement")
	}
	f := newWhiteboardFixture(t)
	const commands = `<wb:topic name="measurement"><wb:panel name="status" recap="Realistic update">Panel body</wb:panel><wb:log>Steady update</wb:log></wb:topic>`
	f.update(t, commands)
	const samples = 10
	steady, reset := make([]int64, 0, samples), make([]int64, 0, samples)
	for i := 0; i < samples; i++ {
		started := time.Now()
		f.update(t, commands)
		steady = append(steady, time.Since(started).Nanoseconds())
		if err := f.s.sessions.boards.use(f.root, f.key, nil, false, func(slot *whiteboardSlot) error {
			return slot.board.Checkpoint()
		}); err != nil {
			t.Fatal(err)
		}

		before := whiteboardMeasurementReadWAL(t, f.pageDir())
		started = time.Now()
		f.update(t, commands)
		reset = append(reset, time.Since(started).Nanoseconds())
		after := whiteboardMeasurementReadWAL(t, f.pageDir())
		if len(before) < 32 || len(after) < 32 || bytes.Equal(before[:32], after[:32]) {
			t.Fatalf("sample %d did not reset the WAL header", i+1)
		}
	}
	data, err := json.Marshal(map[string]any{
		"samples_per_action": samples,
		"steady":             whiteboardMeasurementLatencyOf(steady),
		"wal_reset":          whiteboardMeasurementLatencyOf(reset),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("WHITEBOARD_CHECKPOINT_RESET_MEASUREMENT %s", data)
}
