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
// Limitation: static pruning, in-order appends and replayed same-model repeats
// matching their neighbours preserve that fold. A different-model record arriving
// late inside a discarded span can re-date the return transition to the next
// surviving record. Original evidence is then incomplete and history unreliable;
// neither reader version nor reopening can recover the deleted evidence.
// Keeping the most recent eight records per agent limits this span, not the risk.
const observedPruneRecent = 8

var (
	// observedPruneEvery is the least time between two prunes of one
	// directory in one process. The keep-set bound holds after an eligible
	// completed pass, not between passes: appends can grow it meanwhile.
	observedPruneEvery = 10 * time.Minute
	// removeObserved deletes one record file (a test seam).
	removeObserved = os.Remove
	// pruneMu admits one prune per process; another append skips it. It
	// guards prunedAt, each directory's last prune in this process.
	pruneMu  sync.Mutex
	prunedAt = map[string]time.Time{}
)

// pruneObserved deletes the repeated records of the evidence directory
// evidenceDir, retaining each agent's first, change points and most recent eight.
// It is opportunistic: a file that vanished, or that cannot be deleted now (a
// locked file on Windows), is skipped for a later prune. It deletes nothing
// but regular .json files directly inside evidenceDir, and never a record that
// does not read, does not validate, or lost its identity to an earlier file.
func pruneObserved(evidenceDir string) {
	if !pruneMu.TryLock() {
		return
	}
	defer pruneMu.Unlock()
	now := time.Now()
	if last, ok := prunedAt[evidenceDir]; ok && now.Sub(last) >= 0 && now.Sub(last) < observedPruneEvery {
		return
	}
	entries, err := os.ReadDir(evidenceDir)
	if err != nil {
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
// each model change and the most recent eight, in summarizeObserved's order.
// Records are read, cleaned and deduplicated as ReadObserved
// reads them (first file in name order wins an identity).
func observedRepeats(evidenceDir string, entries []os.DirEntry) []string {
	type candidate struct {
		path    string
		r       ObservedRecord
		regular bool
	}
	st := NewState("")
	seen := map[string]bool{}
	duplicates := map[string]bool{}
	grouped := map[string][]candidate{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(evidenceDir, entry.Name())
		regular := entry.Type().IsRegular()
		if !regular {
			// ReadObserved follows readable symlinks. Include their records
			// in its fold/identity order, but never delete a link or its target.
			if entry.Type()&os.ModeSymlink == 0 {
				continue
			}
			info, err := os.Stat(path)
			if err != nil || !info.Mode().IsRegular() {
				continue
			}
		}
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
			duplicates[key] = true
			continue
		}
		seen[key] = true
		grouped[r.AgentID] = append(grouped[r.AgentID], candidate{path: path, r: r, regular: regular})
	}
	var out []string
	for _, list := range grouped {
		sort.Slice(list, func(i, j int) bool {
			if list[i].r.At != list[j].r.At {
				return list[i].r.At < list[j].r.At
			}
			return observationKey(list[i].r) < observationKey(list[j].r)
		})
		for i := 1; i < len(list)-observedPruneRecent; i++ {
			c := list[i]
			// Keep duplicate winners too: deleting one could expose a losing
			// file with different contents and change the next reader's fold.
			if c.regular && c.r.Model == list[i-1].r.Model && !duplicates[observationKey(c.r)] {
				out = append(out, c.path)
			}
		}
	}
	sort.Strings(out)
	return out
}
