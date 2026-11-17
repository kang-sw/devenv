package wslive

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsstate"
)

// evidenceIO sums the file I/O counted at the evidence writer's own sites.
func evidenceIO() int64 {
	var n int64
	for site, v := range wsstate.FileIOCounters() {
		if strings.Contains(site, "wslive/observed.go:") {
			n += v
		}
	}
	return n
}

func evidenceNames(t *testing.T, dir string) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, ObservedDirName, "*"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	return files
}

func applyObserved(t *testing.T, st *State, r ObservedRecord) []ObservedRecord {
	t.Helper()
	ev := Event{Kind: KindStatus, Observed: &r.ModelObservation}
	if r.AgentID != "" {
		ev.Kind, ev.AgentID = KindAgent, r.AgentID
	}
	res, err := st.Apply(ev, r.At)
	if err != nil {
		t.Fatal(err)
	}
	return res.Observed
}

// Out-of-order arrival included, the records Apply hands to storage fold to
// exactly the state's own models after every step, and repeats are not among
// them.
func TestObservedStorageGetsOnlyFoldChanges(t *testing.T) {
	rec := func(id, model string, at int64) ObservedRecord {
		return ObservedRecord{ModelObservation: ModelObservation{ID: id, Model: model, Source: "pi.assistant", At: at}}
	}
	st := NewState("fold-write")
	for _, step := range []struct {
		r    ObservedRecord
		want []string
	}{
		{rec("a", "m1", 100), []string{"a"}},      // first
		{rec("b", "m1", 200), []string{"b"}},      // newer than latest
		{rec("c", "m1", 300), []string{"c"}},      // newer than latest
		{rec("d", "m1", 150), nil},                // repeat inside the fold
		{rec("e", "m2", 250), []string{"e", "c"}}, // late change: c changes back
		{rec("f", "m1", 50), []string{"f"}},       // new first
		{rec("b", "m9", 999), nil},                // known identity
	} {
		var got []string
		for _, r := range applyObserved(t, st, step.r) {
			got = append(got, r.ID)
		}
		if !reflect.DeepEqual(got, step.want) {
			t.Fatalf("%s@%d stored %v, want %v", step.r.ID, step.r.At, got, step.want)
		}
	}

	rng := rand.New(rand.NewSource(1))
	for round := 0; round < 200; round++ {
		var all []ObservedRecord
		for _, agent := range []string{"", "child-a"} {
			for i := 0; i < 12; i++ {
				r := rec(fmt.Sprintf("%s-%d", agent, i), fmt.Sprintf("m%d", rng.Intn(3)), int64(1+rng.Intn(8)))
				r.AgentID = agent
				all = append(all, r)
			}
		}
		rng.Shuffle(len(all), func(i, j int) { all[i], all[j] = all[j], all[i] })
		st := NewState("fold-write")
		stored := NewState("")
		for _, r := range all {
			stored.SeedObserved(applyObserved(t, st, r))
			if !reflect.DeepEqual(stored.observed, st.observed) {
				t.Fatalf("round %d: stored fold %+v differs from state %+v", round, stored.observed, st.observed)
			}
		}
	}
}

// A daemon restart seeds the state from the pruned directory, then the holder
// ring replays every response: the replay touches no evidence file and
// leaves the fold intact, while a later model change is still written.
func TestObservedReplayAfterPruneWritesNothing(t *testing.T) {
	withoutPruneInterval(t)
	dir := t.TempDir()
	now := time.Now()
	ring := append(pruneFixture("", 40, now), pruneFixture("child-a", 40, now)...)
	first := NewState("restart")
	for _, r := range ring {
		if err := AppendObserved(dir, applyObserved(t, first, r)); err != nil {
			t.Fatal(err)
		}
	}
	pruneObserved(filepath.Join(dir, ObservedDirName))
	names := evidenceNames(t, dir)
	if len(names) >= len(ring) {
		t.Fatalf("fixture did not prune: %d files", len(names))
	}

	records, err := ReadObserved(dir)
	if err != nil {
		t.Fatal(err)
	}
	restarted := NewState("restart")
	restarted.SeedObserved(records)
	before := evidenceIO()
	for _, r := range ring {
		persist := applyObserved(t, restarted, r)
		if len(persist) != 0 {
			t.Fatalf("replayed %s stored %+v", r.ID, persist)
		}
		if err := AppendObserved(dir, persist); err != nil {
			t.Fatal(err)
		}
	}
	if delta := evidenceIO() - before; delta != 0 {
		t.Fatalf("replay made %d evidence file operations, want 0", delta)
	}
	if got := evidenceNames(t, dir); !reflect.DeepEqual(got, names) {
		t.Fatalf("replay changed the evidence files: %d -> %d", len(names), len(got))
	}
	if !reflect.DeepEqual(restarted.observed, first.observed) {
		t.Fatalf("replay changed the fold: %+v / %+v", restarted.observed, first.observed)
	}
	if got, _ := visibleModels(t, dir); !reflect.DeepEqual(got, first.observed) {
		t.Fatalf("directory fold %+v differs from the writer's %+v", got, first.observed)
	}

	change := ObservedRecord{ModelObservation: ModelObservation{ID: "after", Model: "m4", Source: "claude.assistant", At: now.Add(time.Minute).UnixMilli()}}
	persist := applyObserved(t, restarted, change)
	if len(persist) != 1 || persist[0].ID != "after" {
		t.Fatalf("model change stored %+v", persist)
	}
	if err := AppendObserved(dir, persist); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(evidencePath(dir, change)); err != nil {
		t.Fatalf("model change not written: %v", err)
	}
}
