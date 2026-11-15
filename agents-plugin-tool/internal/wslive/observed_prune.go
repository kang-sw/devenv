package wslive

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Retention of the model-evidence directory. Nothing reads a single record
// back: every reader folds the records into ObservedModel (summarizeObserved),
// whose Current is an agent's latest record and whose History lists its first
// record and each model change. Per agent, any record set holding the first,
// every change point and the latest yields the same model as the full set,
// since each other record repeats the model of the record before it. A prune
// deletes only such repeats, so every binary that reads the directory,
// including one that never prunes, shows the same Current and History.
//
// Limitation: a record that arrives late (a child's replay into its parent,
// or another writer racing the prune) with an At inside a pruned span and
// another model would have split that span; with the repeat gone, the change
// back is dated at the next surviving record instead. A record is therefore
// deleted only when its At and its file time are both more than
// observedPruneAge older than its agent's newest record: a producer's replay
// is ordered by At, so a late arrival that far back is not expected.
var (
	// observedPruneLimit is the record count above which an append prunes.
	observedPruneLimit = 2256
	// observedPruneAge is how much older than its agent's newest record a
	// repeat must be, by At and by file time, to be deleted.
	observedPruneAge = time.Hour
	// observedPruneEvery is the least time between two prunes of one
	// directory in one process. Records within observedPruneAge of their
	// agent's newest stay (a delegate's whole short life, say), so a pruned
	// directory can stay above the limit; this keeps every append after it
	// from listing and reading the directory again.
	observedPruneEvery = 10 * time.Minute
	// removeObserved deletes one record file (a test seam).
	removeObserved = os.Remove
	// pruneMu admits one prune per process; another append skips it. It
	// guards prunedAt, each directory's last prune in this process.
	pruneMu  sync.Mutex
	prunedAt = map[string]time.Time{}
)

// pruneObserved deletes the repeated records of the evidence directory
// evidenceDir when it holds more than observedPruneLimit record files. It is
// opportunistic: a file that vanished, or that cannot be deleted now (a
// locked file on Windows), is skipped for a later prune. It deletes nothing
// but regular .json files directly inside evidenceDir, and never a record that
// does not read, does not validate, or lost its identity to an earlier file.
func pruneObserved(evidenceDir string) {
	if !pruneMu.TryLock() {
		return
	}
	defer pruneMu.Unlock()
	entries, err := os.ReadDir(evidenceDir)
	if err != nil {
		return
	}
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			count++
		}
	}
	if count <= observedPruneLimit {
		return
	}
	now := time.Now()
	if last, ok := prunedAt[evidenceDir]; ok && now.Sub(last) >= 0 && now.Sub(last) < observedPruneEvery {
		return
	}
	if len(prunedAt) >= 256 {
		prunedAt = map[string]time.Time{}
	}
	prunedAt[evidenceDir] = now
	for _, path := range observedRepeats(evidenceDir, entries) {
		// A vanished file is already gone; any other failure leaves the file
		// for the next prune, and the rest still go.
		_ = removeObserved(path)
	}
}

// observedRepeats lists the record files of entries (evidenceDir's sorted
// listing) that a prune may delete: per agent, every record but the first,
// each model change and the latest, in summarizeObserved's order, whose At
// and file time are both more than observedPruneAge older than the agent's
// newest record. Records are read, cleaned and deduplicated as ReadObserved
// reads them (first file in name order wins an identity).
func observedRepeats(evidenceDir string, entries []os.DirEntry) []string {
	type candidate struct {
		path  string
		r     ObservedRecord
		mtime time.Time
	}
	st := NewState("")
	seen := map[string]bool{}
	grouped := map[string][]candidate{}
	for _, entry := range entries {
		if !entry.Type().IsRegular() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		path := filepath.Join(evidenceDir, entry.Name())
		r, valid, err := readObservedRecord(path, os.Open)
		if err != nil || !valid {
			continue
		}
		if r.AgentID != "" && !agentIDPattern.MatchString(r.AgentID) {
			continue
		}
		o := st.cleanObservation(&r.ModelObservation)
		if o == nil {
			continue
		}
		r.ModelObservation = *o
		key := observationKey(r)
		if seen[key] {
			continue
		}
		seen[key] = true
		grouped[r.AgentID] = append(grouped[r.AgentID], candidate{path: path, r: r, mtime: info.ModTime()})
	}
	age := observedPruneAge.Milliseconds()
	var out []string
	for _, list := range grouped {
		sort.Slice(list, func(i, j int) bool {
			if list[i].r.At != list[j].r.At {
				return list[i].r.At < list[j].r.At
			}
			return observationKey(list[i].r) < observationKey(list[j].r)
		})
		newest := list[len(list)-1].r.At
		for i := 1; i < len(list)-1; i++ {
			c := list[i]
			if c.r.Model != list[i-1].r.Model {
				continue
			}
			if newest-c.r.At > age && newest-c.mtime.UnixMilli() > age {
				out = append(out, c.path)
			}
		}
	}
	sort.Strings(out)
	return out
}
