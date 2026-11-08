package wstrackerhost

import (
	"path/filepath"
	"sort"
)

// seenFile, in a store directory (beside host-sync.json, outside git),
// holds what the viewer has seen on this clone.
const seenFile = "ui-seen.json"

// Seen is what the viewer has seen on this clone: the handle-badge seen
// marks and the notification read marks. Both lists are sorted and unique,
// and never nil.
type Seen struct {
	Badge []string `json:"badge"`
	Read  []string `json:"read"`
}

// seenDoc is Seen as ui-seen.json stores it.
type seenDoc struct {
	Version int      `json:"version"`
	Badge   []string `json:"badge"`
	Read    []string `json:"read"`
}

// sortedUnion returns the sorted, duplicate-free, non-nil union of the
// lists without "".
func sortedUnion(lists ...[]string) []string {
	set := map[string]bool{}
	for _, l := range lists {
		for _, s := range l {
			if s != "" {
				set[s] = true
			}
		}
	}
	out := make([]string, 0, len(set))
	for s := range set {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func readSeen(dir string) (Seen, error) {
	var doc seenDoc
	if err := readJSON(filepath.Join(dir, seenFile), &doc); err != nil {
		return Seen{}, err
	}
	return Seen{Badge: sortedUnion(doc.Badge), Read: sortedUnion(doc.Read)}, nil
}

// Seen returns store id's seen marks; a store with no ui-seen.json has
// none. An unregistered id is an error.
func (h *Host) Seen(id string) (Seen, error) {
	e, err := h.Entry(id)
	if err != nil {
		return Seen{}, err
	}
	h.seenMu.Lock()
	defer h.seenMu.Unlock()
	return readSeen(e.Dir)
}

// MarkSeen adds the marks of add to store id's seen marks; nothing is ever
// removed, and empty strings are dropped. An unregistered id is an error.
//
// It takes only seenMu, never the store's mu or its ledger: a store's mu
// is held across network calls of up to a minute, and a mark must not wait
// for one. Only the dashboard daemon, a single instance per machine,
// writes this file, so one in-process mutex is the whole exclusion.
func (h *Host) MarkSeen(id string, add Seen) error {
	e, err := h.Entry(id)
	if err != nil {
		return err
	}
	h.seenMu.Lock()
	defer h.seenMu.Unlock()
	have, err := readSeen(e.Dir)
	if err != nil {
		return err
	}
	return writeJSON(filepath.Join(e.Dir, seenFile), seenDoc{
		Version: 1,
		Badge:   sortedUnion(have.Badge, add.Badge),
		Read:    sortedUnion(have.Read, add.Read),
	})
}
