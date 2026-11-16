package wslive

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"testing"
)

func retentionRecords(agent string, n int) []ObservedRecord {
	var records []ObservedRecord
	for i := 1; i <= n; i++ {
		records = append(records, ObservedRecord{AgentID: agent, ModelObservation: ModelObservation{
			ID: fmt.Sprintf("y-%02d", i), Model: "Y", Source: "pi.assistant", At: int64(i * 1000),
		}})
	}
	return records
}

func assertRetentionMembers(t *testing.T, dir string, want []ObservedRecord) {
	t.Helper()
	paths := make([]string, 0, len(want))
	for _, r := range want {
		paths = append(paths, evidencePath(dir, r))
	}
	sort.Strings(paths)
	got, err := filepath.Glob(filepath.Join(dir, ObservedDirName, "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(paths) || (len(paths) > 0 && !reflect.DeepEqual(got, paths)) {
		t.Fatalf("retained files:\n%v\nwant:\n%v", got, paths)
	}
	if evidenceCount(t, dir) != len(want) {
		t.Fatal("evidence count differs from exact membership")
	}
}

func TestObservedRetentionRecentEightMembership(t *testing.T) {
	withoutPruneInterval(t)
	for n := 0; n <= 12; n++ {
		t.Run(fmt.Sprintf("size-%02d", n), func(t *testing.T) {
			dir := t.TempDir()
			records := retentionRecords("", n)
			writeFixture(t, dir, records)
			want := records
			if n > 8 {
				want = append([]ObservedRecord{records[0]}, records[n-8:]...)
			}
			pruneObserved(filepath.Join(dir, ObservedDirName))
			assertRetentionMembers(t, dir, want)
		})
	}
	t.Run("ties-use-observation-key-not-file-or-input-order", func(t *testing.T) {
		dir := t.TempDir()
		records := retentionRecords("child-a", 12)
		for i := range records {
			records[i].At = 1000
			if i >= 6 {
				records[i].Source = "claude.assistant"
			}
		}
		// At ties sort claude before pi, then by ID within each source.
		ordered := append(append([]ObservedRecord(nil), records[6:]...), records[:6]...)
		for i, j := 0, len(records)-1; i < j; i, j = i+1, j-1 {
			records[i], records[j] = records[j], records[i]
		}
		writeFixture(t, dir, records)
		pruneObserved(filepath.Join(dir, ObservedDirName))
		want := append([]ObservedRecord{ordered[0]}, ordered[4:]...)
		assertRetentionMembers(t, dir, want)
	})
	t.Run("changes-and-agent-independence", func(t *testing.T) {
		dir := t.TempDir()
		lead := retentionRecords("", 20)
		for i := 1; i < 6; i += 2 {
			lead[i].Model = "X"
		}
		child := retentionRecords("child-a", 12)
		short := retentionRecords("child-b", 3)
		// Give the other agents much later clocks; their recent window must
		// not evict any of the lead's records or a short child's records.
		for i := range child {
			child[i].At += 1000000
		}
		all := append(append(append([]ObservedRecord(nil), lead...), child...), short...)
		writeFixture(t, dir, all)
		wantModels, wantSnap := visibleModels(t, dir)
		pruneObserved(filepath.Join(dir, ObservedDirName))
		want := append(append([]ObservedRecord(nil), lead[:7]...), lead[12:]...)
		want = append(want, child[0])
		want = append(want, child[4:]...)
		want = append(want, short...)
		assertRetentionMembers(t, dir, want)
		gotModels, gotSnap := visibleModels(t, dir)
		if !reflect.DeepEqual(gotModels, wantModels) || !reflect.DeepEqual(gotSnap, wantSnap) {
			t.Fatal("retaining early alternating changes changed the fold")
		}
	})
}

func TestObservedRetentionInOrderAppendOracle(t *testing.T) {
	withoutPruneInterval(t)
	dir := t.TempDir()
	all := append(retentionRecords("", 12), retentionRecords("child-a", 12)...)
	writeFixture(t, dir, all)
	var cache ObservedCache
	cache.Load(dir)
	check := func() {
		t.Helper()
		oracle := NewState("")
		oracle.SeedObserved(all)
		wantSnap := Snapshot{Agents: []Agent{{ID: "child-a"}, {ID: "child-b"}}}
		OverlayObserved(&wantSnap, all)
		gotModels, gotSnap := visibleModels(t, dir)
		if !reflect.DeepEqual(gotModels, oracle.observed) || !reflect.DeepEqual(gotSnap, wantSnap) {
			t.Fatalf("retained fold differs from unpruned oracle: got=%+v want=%+v", gotModels, oracle.observed)
		}
		cached := cache.Load(dir)
		if !reflect.DeepEqual(cached.models, oracle.observed) {
			t.Fatal("warm cache Current/History differs from unpruned oracle")
		}
		cachedSnap := Snapshot{Agents: []Agent{{ID: "child-a"}, {ID: "child-b"}}}
		cached.Overlay(&cachedSnap)
		if !reflect.DeepEqual(cachedSnap, wantSnap) {
			t.Fatal("warm cached overlay differs from unpruned overlay")
		}
	}
	pruneObserved(filepath.Join(dir, ObservedDirName))
	check()
	for i, model := range []string{"Y", "X", "X", "Y"} {
		var next []ObservedRecord
		for _, agent := range []string{"", "child-a"} {
			rate := float64(20 + i)
			next = append(next, ObservedRecord{AgentID: agent, ModelObservation: ModelObservation{
				ID: fmt.Sprintf("next-%d", i), Model: model, Source: "pi.assistant", At: int64((13 + i) * 1000),
				Effort: "high", EffortSource: "message", OutputTps: &rate,
			}})
		}
		all = append(all, next...)
		writeFixture(t, dir, next)
		check()
		pruneObserved(filepath.Join(dir, ObservedDirName))
		check()
		models, _ := visibleModels(t, dir)
		for _, r := range next {
			if !reflect.DeepEqual(models[r.AgentID].Current, r.ModelObservation) {
				t.Fatalf("Current lost appended metadata for %q", r.AgentID)
			}
		}
	}
}

func TestObservedRetentionProtectedFiles(t *testing.T) {
	withoutPruneInterval(t)
	dir := t.TempDir()
	records := retentionRecords("", 12)
	writeFixture(t, dir, records)
	evidenceDir := filepath.Join(dir, ObservedDirName)
	protected := map[string][]byte{}
	put := func(path string, data []byte) {
		t.Helper()
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		protected[path] = data
	}
	marshal := func(r ObservedRecord) []byte {
		t.Helper()
		data, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	put(filepath.Join(evidenceDir, "invalid.json"), []byte("{not-json"))
	for name, mutate := range map[string]func(*ObservedRecord){
		"invalid-agent":      func(r *ObservedRecord) { r.AgentID = "bad/agent" },
		"invalid-time":       func(r *ObservedRecord) { r.At = 0 },
		"invalid-model":      func(r *ObservedRecord) { r.Model = "" },
		"invalid-id":         func(r *ObservedRecord) { r.ID = "" },
		"unsupported-source": func(r *ObservedRecord) { r.Source = "future.assistant" },
		"unsupported-alias":  func(r *ObservedRecord) { r.Source, r.Model = "claude.assistant", "opus" },
	} {
		r := records[2]
		mutate(&r)
		put(filepath.Join(evidenceDir, name+".json"), marshal(r))
	}
	put(filepath.Join(evidenceDir, "keep.txt"), marshal(records[2]))
	nested := filepath.Join(evidenceDir, "nested.json")
	if err := os.Mkdir(nested, 0700); err != nil {
		t.Fatal(err)
	}
	put(filepath.Join(nested, "record.json"), marshal(records[2]))
	// t2 would normally be pruned. Its lexical winner must survive too,
	// otherwise the different-content loser would become visible as X.
	winner := filepath.Join(evidenceDir, "000-winner.json")
	if err := os.Rename(evidencePath(dir, records[1]), winner); err != nil {
		t.Fatal(err)
	}
	protected[winner] = marshal(records[1])
	loser := records[1]
	loser.Model, loser.At = "X", 1500
	put(filepath.Join(evidenceDir, "zzz-loser.json"), marshal(loser))
	target := filepath.Join(dir, "symlink-target.json")
	put(target, marshal(records[2]))
	link := filepath.Join(evidenceDir, "symlink.json")
	linked := false
	if err := os.Symlink(target, link); err != nil {
		if runtime.GOOS != "windows" {
			t.Fatal(err)
		}
		t.Logf("symlinks unavailable on this Windows fixture: %v", err)
	} else {
		linked = true
	}
	wantModels, wantSnap := visibleModels(t, dir)
	var cache ObservedCache
	cache.Load(dir)
	pruneObserved(evidenceDir)
	for path, want := range protected {
		got, err := os.ReadFile(path)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("protected file changed: %s: %v", path, err)
		}
	}
	if info, err := os.Stat(nested); err != nil || !info.IsDir() {
		t.Fatalf("nested directory changed: %v", err)
	}
	if linked {
		if got, err := os.Readlink(link); err != nil || got != target {
			t.Fatalf("symlink changed: %q %v", got, err)
		}
	}
	for i, r := range records {
		if i == 1 { // The duplicate winner was renamed and checked above.
			continue
		}
		_, err := os.Stat(evidencePath(dir, r))
		// The readable symlink aliases t3. Its regular identity winner is
		// protected with the loser; count bounds use unique-record fixtures.
		if i == 3 || (i == 2 && !linked) {
			if !os.IsNotExist(err) {
				t.Fatalf("ordinary repeat was not pruned: %s: %v", r.ID, err)
			}
		} else if err != nil {
			t.Fatalf("first/recent record disappeared: %s: %v", r.ID, err)
		}
	}
	gotModels, gotSnap := visibleModels(t, dir)
	if !reflect.DeepEqual(gotModels, wantModels) || !reflect.DeepEqual(gotSnap, wantSnap) || !reflect.DeepEqual(cache.Load(dir).models, wantModels) {
		t.Fatal("protected duplicate loser became visible or protected-file prune changed fold")
	}
	if len(gotModels[""].History) != 1 || gotModels[""].History[0].Model != "Y" {
		t.Fatal("different-content loser was exposed")
	}
}

func TestObservedRetentionProtectsUniqueSymlinkRepeat(t *testing.T) {
	withoutPruneInterval(t)
	dir := t.TempDir()
	records := retentionRecords("", 12)
	writeFixture(t, dir, records)
	// The unique t2 record repeats t1's model and precedes the recent eight
	// (t5..t12), so only the regular-file guard protects this link.
	link := evidencePath(dir, records[1])
	target := filepath.Join(dir, "outside-target.json")
	if err := os.Rename(link, target); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlinks unavailable on this Windows fixture: %v", err)
		}
		t.Fatal(err)
	}
	raw, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	wantModels, wantSnap := visibleModels(t, dir)
	var cache ObservedCache
	cache.Load(dir)
	pruneObserved(filepath.Join(dir, ObservedDirName))
	if got, err := os.Readlink(link); err != nil || got != target {
		t.Fatalf("unique same-model symlink repeat was pruned or changed: %q: %v", got, err)
	}
	if got, err := os.ReadFile(target); err != nil || !reflect.DeepEqual(got, raw) {
		t.Fatalf("symlink target changed: %v", err)
	}
	for _, r := range records[2:4] {
		if _, err := os.Stat(evidencePath(dir, r)); !os.IsNotExist(err) {
			t.Fatalf("ordinary repeat was not pruned: %s: %v", r.ID, err)
		}
	}
	if got := evidenceCount(t, dir); got != 10 { // first, protected link, recent eight
		t.Fatalf("kept %d records, want 10", got)
	}
	gotModels, gotSnap := visibleModels(t, dir)
	if !reflect.DeepEqual(gotModels, wantModels) || !reflect.DeepEqual(gotSnap, wantSnap) || !reflect.DeepEqual(cache.Load(dir).models, wantModels) {
		t.Fatal("protected symlink repeat prune changed visible models")
	}
}

func TestObservedRetentionProtectedSymlinkChanges(t *testing.T) {
	withoutPruneInterval(t)
	for _, duplicate := range []bool{false, true} {
		t.Run(fmt.Sprintf("duplicate-winner-%t", duplicate), func(t *testing.T) {
			dir := t.TempDir()
			records := retentionRecords("", 20)
			writeFixture(t, dir, records)
			x := records[1]
			x.Model, x.At = "X", 2500
			if !duplicate {
				x.ID = "link-only-x"
			}
			raw, err := json.Marshal(x)
			if err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(dir, "outside-target.json")
			if err := os.WriteFile(target, raw, 0600); err != nil {
				t.Fatal(err)
			}
			link := filepath.Join(dir, ObservedDirName, "000-link.json")
			if err := os.Symlink(target, link); err != nil {
				if runtime.GOOS != "windows" {
					t.Fatal(err)
				}
				t.Logf("symlinks unavailable on this Windows fixture: %v", err)
				return
			}
			wantModels, wantSnap := visibleModels(t, dir)
			var cache ObservedCache
			cache.Load(dir)
			pruneObserved(filepath.Join(dir, ObservedDirName))
			gotModels, gotSnap := visibleModels(t, dir)
			if !reflect.DeepEqual(gotModels, wantModels) || !reflect.DeepEqual(gotSnap, wantSnap) || !reflect.DeepEqual(cache.Load(dir).models, wantModels) {
				t.Fatal("protected symlink transition lost its regular return point")
			}
			if len(gotModels[""].History) != 3 || gotModels[""].History[2].At != 3000 {
				t.Fatalf("static Y/X/Y return re-dated: %+v", gotModels[""].History)
			}
			if got, err := os.ReadFile(target); err != nil || !reflect.DeepEqual(got, raw) {
				t.Fatal("symlink target changed", err)
			}
			if got, err := os.Readlink(link); err != nil || got != target {
				t.Fatal("symlink changed", err)
			}
			if _, err := os.Stat(evidencePath(dir, records[2])); err != nil {
				t.Fatal("regular return transition deleted", err)
			}
		})
	}
}
