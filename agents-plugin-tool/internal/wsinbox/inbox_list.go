// Inbox name listings, listing cache, and entry reads.
package wsinbox

import (
	"errors"
	"github.com/kang-sw/devenv/internal/wsstate"
	"io/fs"
	"path/filepath"
	"slices"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// filterNames keeps the names whose entry reads and satisfies keep; an entry
// that cannot be read (a claim took it meanwhile, or it does not parse) is
// dropped.
func filterNames(dir string, names []string, keep func(Entry) bool) []string {
	out := names[:0:0]
	for _, name := range names {
		if e, err := readEntry(filepath.Join(dir, name)); err == nil && keep(e) {
			out = append(out, name)
		}
	}
	return out
}

// entryNames lists dir's entry file names, oldest first.
func entryNames(dir string) ([]string, error) {
	des, err := wsstate.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(des))
	for _, de := range des {
		if !de.IsDir() && entryNamePattern.MatchString(de.Name()) {
			names = append(names, de.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

// Names returns the held entry names at dir, interrupt entries included,
// oldest first; a missing directory holds none. An entry file is never rewritten, so the names alone
// tell whether the inbox changed.
//
// Readers poll unchanged inboxes many times a second, so the last listing of
// a directory is reused while nothing can have changed it: no inbox write
// in this process since (namesGen), and a stat showing the directory's
// modification time unchanged, which catches a writer in another process.
// A listing is kept only when that time was already namesTrustAge old at the
// stat: a change in the same timestamp tick could keep the time and hide a
// message, and later changes stamp a later time.
func Names(dir string) ([]string, error) {
	gen := namesGen.Load()
	now := time.Now()
	info, err := wsstate.Stat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err == nil {
		if names, ok := cachedNames(dir, gen, info.ModTime()); ok {
			return names, nil
		}
	}
	names, listErr := entryNames(dir)
	if errors.Is(listErr, fs.ErrNotExist) {
		return nil, nil
	}
	if listErr == nil && err == nil && info.ModTime().Before(now.Add(-namesTrustAge)) {
		storeNames(dir, namesListing{gen: gen, mod: info.ModTime(), names: slices.Clone(names)})
	}
	return names, listErr
}

// namesTrustAge exceeds every local filesystem's directory timestamp
// granularity, FAT's two seconds included.
const namesTrustAge = 3 * time.Second

// namesCacheMax bounds the reused listings; past it they are all dropped.
const namesCacheMax = 4096

// namesGen counts this process's inbox writes. Every entry name change here
// goes through rename, remove or a commit, each bumping it once done, so a
// listing taken before a write never passes for one taken after it.
var namesGen atomic.Uint64

func bumpNames() { namesGen.Add(1) }

type namesListing struct {
	gen   uint64
	mod   time.Time
	names []string
}

var namesCache struct {
	sync.Mutex
	m map[string]namesListing
}

func cachedNames(dir string, gen uint64, mod time.Time) ([]string, bool) {
	namesCache.Lock()
	defer namesCache.Unlock()
	l, ok := namesCache.m[dir]
	if !ok || l.gen != gen || !l.mod.Equal(mod) {
		return nil, false
	}
	return slices.Clone(l.names), true
}

func storeNames(dir string, l namesListing) {
	namesCache.Lock()
	defer namesCache.Unlock()
	if namesCache.m == nil || len(namesCache.m) >= namesCacheMax {
		namesCache.m = map[string]namesListing{}
	}
	namesCache.m[dir] = l
}

// SteerNames returns the names of the held entries at dir marked for
// steering, oldest first; a missing directory holds none.
func SteerNames(dir string) ([]string, error) {
	names, err := Names(dir)
	if err != nil || len(names) == 0 {
		return nil, err
	}
	return filterNames(dir, names, func(e Entry) bool { return e.Steer }), nil
}

// MessageNames returns the names of the held entries at dir that are
// messages, not interrupt entries, oldest first; a missing directory holds
// none.
func MessageNames(dir string) ([]string, error) {
	names, err := Names(dir)
	if err != nil || len(names) == 0 {
		return nil, err
	}
	return filterNames(dir, names, func(e Entry) bool { return !e.Interrupt }), nil
}

// InterruptNames returns the names of the interrupt entries at dir, oldest
// first; a missing directory holds none.
func InterruptNames(dir string) ([]string, error) {
	names, err := Names(dir)
	if err != nil || len(names) == 0 {
		return nil, err
	}
	return filterNames(dir, names, func(e Entry) bool { return e.Interrupt }), nil
}

// DrainedNames returns the drained record names of the inbox at dir, oldest
// first.
func DrainedNames(dir string) ([]string, error) { return Names(DrainedDir(dir)) }

// Held reads the held messages at dir in queue order (queueRecords);
// interrupt entries are left out. An entry that cannot be read (a claim took
// it meanwhile, or it does not parse) is skipped.
func Held(dir string) ([]Record, error) {
	records, err := readAll(dir)
	if err != nil {
		return nil, err
	}
	out := records[:0]
	for _, r := range records {
		if !r.Entry.Interrupt {
			out = append(out, r)
		}
	}
	return queueRecords(out), nil
}

// Drained reads the drained records of the inbox at dir, oldest first.
func Drained(dir string) ([]Record, error) { return readAll(DrainedDir(dir)) }

// ReadDrained reads the one drained record name of the inbox at dir. A
// drained entry file is never rewritten, so a reader may keep a read record
// by its name.
func ReadDrained(dir, name string) (Record, error) {
	entry, err := readEntry(filepath.Join(DrainedDir(dir), name))
	if err != nil {
		return Record{}, err
	}
	return Record{Name: name, Entry: entry}, nil
}

// ReadHeld reads the one held entry name of the inbox at dir. A held entry
// file is never rewritten (Steer writes a fresh name), so a reader may keep
// what it read by its name.
func ReadHeld(dir, name string) (Entry, error) {
	return readEntry(filepath.Join(dir, name))
}

func readAll(dir string) ([]Record, error) {
	names, err := Names(dir)
	if err != nil {
		return nil, err
	}
	out := make([]Record, 0, len(names))
	for _, name := range names {
		entry, err := readEntry(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		out = append(out, Record{Name: name, Entry: entry})
	}
	return out, nil
}
