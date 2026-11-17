package wsagent

import (
	"fmt"
	"io"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wsstate"
)

// evidenceFileIO sums the file I/O counted at the evidence writer's sites.
func evidenceFileIO() int64 {
	var n int64
	for site, v := range wsstate.FileIOCounters() {
		if strings.Contains(site, "wslive/observed.go:") {
			n += v
		}
	}
	return n
}

func evidenceFiles(t *testing.T, dir string) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, wslive.ObservedDirName, "*"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	return files
}

// After a daemon swap the writer is seeded from a pruned directory, then the
// holder ring replays the lead's responses and seedSpawnRows re-applies a Pi
// child's whole window. Neither touches an evidence file; a model change on
// either path is still written.
func TestObservedReplayAndSpawnReseedTouchNoEvidence(t *testing.T) {
	dir := t.TempDir()
	obs := func(id, model string, at int64) wslive.ModelObservation {
		return wslive.ModelObservation{ID: id, Model: model, Source: "pi.assistant", At: at}
	}
	var own, child []wslive.ModelObservation
	for i := 0; i < 20; i++ {
		own = append(own, obs(fmt.Sprintf("own-%d", i), "lead-model", int64(1000+i)))
		child = append(child, obs(fmt.Sprintf("child-%d", i), "child-model", int64(1000+i)))
	}
	// What a prune leaves of that history: each agent's first and latest.
	var kept []wslive.ObservedRecord
	for _, o := range []wslive.ModelObservation{own[0], own[19]} {
		kept = append(kept, wslive.ObservedRecord{ModelObservation: o})
	}
	for _, o := range []wslive.ModelObservation{child[0], child[19]} {
		kept = append(kept, wslive.ObservedRecord{AgentID: "pi:kid", ModelObservation: o})
	}
	if err := wslive.AppendObserved(dir, kept); err != nil {
		t.Fatal(err)
	}
	files := evidenceFiles(t, dir)

	w := newLiveWriter(dir, "replay-key", "pi", time.Now, io.Discard)
	want := w.st.Snapshot(0)
	before := evidenceFileIO()
	for i := range own {
		w.applyQuiet(wslive.Event{Kind: wslive.KindStatus, Observed: &own[i]})
	}
	for _, ev := range SpawnAgentEvents(State{ID: "kid", Observations: child}, "kid-key") {
		w.applyQuiet(ev)
	}
	if delta := evidenceFileIO() - before; delta != 0 {
		t.Fatalf("replay made %d evidence file operations, want 0", delta)
	}
	if got := evidenceFiles(t, dir); !reflect.DeepEqual(got, files) {
		t.Fatalf("replay changed the evidence files: %v -> %v", files, got)
	}
	got := w.st.Snapshot(0)
	if !reflect.DeepEqual(got.Lead.Observed, want.Lead.Observed) || !reflect.DeepEqual(got.ObservedAgents["pi:kid"], want.ObservedAgents["pi:kid"]) {
		t.Fatalf("replay changed the fold: %+v / %+v", got, want)
	}

	ownChange, childChange := obs("own-new", "lead-next", 2000), obs("child-new", "child-next", 2000)
	w.apply(wslive.Event{Kind: wslive.KindStatus, Observed: &ownChange})
	for _, ev := range SpawnAgentEvents(State{ID: "kid", Observations: append(child, childChange)}, "kid-key") {
		w.applyQuiet(ev)
	}
	for _, r := range []wslive.ObservedRecord{{ModelObservation: ownChange}, {AgentID: "pi:kid", ModelObservation: childChange}} {
		found := false
		records, err := wslive.ReadObserved(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, got := range records {
			found = found || reflect.DeepEqual(got, r)
		}
		if !found {
			t.Fatalf("model change %+v not written", r)
		}
	}
	if n := len(evidenceFiles(t, dir)); n != len(files)+2 {
		t.Fatalf("model changes wrote %d files, want 2", n-len(files))
	}
}
