package mcp

import (
	"crypto/sha256"
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
)

type whiteboardMeasurementFile struct {
	Size int64
	Sum  [sha256.Size]byte
}

type whiteboardMeasurementDelta struct {
	Created      int   `json:"created"`
	Modified     int   `json:"modified"`
	Removed      int   `json:"removed"`
	BytesWritten int64 `json:"bytes_written"`
}

type whiteboardMeasurementRow struct {
	Index      int                                   `json:"index"`
	Phase      string                                `json:"phase"`
	WallNS     int64                                 `json:"wall_ns"`
	FileIO     map[string]int64                      `json:"file_io"`
	Filesystem map[string]whiteboardMeasurementDelta `json:"filesystem"`
}

type whiteboardMeasurementLatency struct {
	MedianNS int64 `json:"median_ns"`
	P95NS    int64 `json:"p95_ns"`
}

type whiteboardMeasurementReport struct {
	Label       string                       `json:"label"`
	Updates     int                          `json:"updates"`
	FirstWrite  whiteboardMeasurementLatency `json:"first_write"`
	SteadyState whiteboardMeasurementLatency `json:"steady_state"`
	All         whiteboardMeasurementLatency `json:"all"`
	Rows        []whiteboardMeasurementRow   `json:"rows"`
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
	rows := make([]whiteboardMeasurementRow, 0, n)
	const commands = `<wb:topic name="measurement"><wb:panel name="status" recap="Realistic update">Panel body</wb:panel><wb:log>Steady update</wb:log></wb:topic>`
	for i := 0; i < n; i++ {
		countersBefore := wsstate.FileIOCounters()
		started := time.Now()
		f.update(t, commands)
		wall := time.Since(started)
		countersAfter := wsstate.FileIOCounters()
		after := whiteboardMeasurementScopes(t, rootDir, pageDir)
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
		})
		before = after
	}

	all := make([]int64, 0, len(rows))
	steady := make([]int64, 0, len(rows)-1)
	for _, row := range rows {
		all = append(all, row.WallNS)
		if row.Phase == "steady_state" {
			steady = append(steady, row.WallNS)
		}
	}
	report := whiteboardMeasurementReport{
		Label:       label,
		Updates:     n,
		FirstWrite:  whiteboardMeasurementLatencyOf([]int64{rows[0].WallNS}),
		SteadyState: whiteboardMeasurementLatencyOf(steady),
		All:         whiteboardMeasurementLatencyOf(all),
		Rows:        rows,
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("WHITEBOARD_MEASUREMENT %s", data)
}

func whiteboardMeasurementScopes(t *testing.T, rootDir, pageDir string) map[string]map[string]whiteboardMeasurementFile {
	t.Helper()
	stored, derived := map[string]whiteboardMeasurementFile{}, map[string]whiteboardMeasurementFile{}
	for path, file := range whiteboardMeasurementSnapshot(t, pageDir, "") {
		name := filepath.Base(path)
		if name == "index.html" || name == "index.stamp.js" || strings.HasPrefix(name, "wb-mermaid-") {
			derived[path] = file
		} else {
			stored[path] = file
		}
	}
	return map[string]map[string]whiteboardMeasurementFile{
		"authoritative_board": stored,
		"derived_pages":       derived,
		"embed_root":          whiteboardMeasurementSnapshot(t, rootDir, pageDir),
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
