package wsdoc

import (
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"
)

// ticketCacheRacyWindow is how old a ticket file's mtime must be, at the
// moment its directory entry was listed, before its parse is cached. A
// rewrite inside the same mtime tick with the same size would otherwise
// revalidate as unchanged; a write that happens after the listing always
// carries a later mtime than one already older than this window.
const ticketCacheRacyWindow = 2 * time.Second

// readTicketFile is the board's file read, a variable so a test can count
// reads.
var readTicketFile = os.ReadFile

type ticketCacheEntry struct {
	size    int64
	modTime time.Time
	info    TicketInfo
}

// ticketCache holds parsed tickets per status directory, keyed by file name.
// It is a pure read cache: every scan still lists the directory and
// revalidates each entry by the size and mtime the listing returned, so a
// ticket created, moved, closed or edited by any process or by plain git is
// seen on the next scan. A directory's map is replaced on every scan, so a
// vanished file leaves the cache with it.
var ticketCache = struct {
	sync.Mutex
	dirs map[string]map[string]ticketCacheEntry
}{dirs: map[string]map[string]ticketCacheEntry{}}

// scanStatusDir parses every ticket file in one status directory, rereading
// only files whose listed size or mtime changed since the last scan.
//
// The size and mtime come from the directory listing itself: free on Windows
// (the find data carries them) and one lstat per entry elsewhere, instead of
// an open and read per ticket. A symlinked ticket is always reread, because
// the listing describes the link, not its target.
func scanStatusDir(root, statusDir, status string) ([]TicketInfo, error) {
	ticketCache.Lock()
	prev := ticketCache.dirs[statusDir]
	ticketCache.Unlock()

	next := map[string]ticketCacheEntry{}
	out := []TicketInfo{}
	for _, entry := range sortedEntries(statusDir) {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ticketFileSuffix {
			continue
		}
		path := filepath.Join(statusDir, entry.Name())
		var stat fs.FileInfo
		if entry.Type()&fs.ModeSymlink == 0 {
			stat, _ = entry.Info()
		}
		if stat != nil {
			if hit, ok := prev[entry.Name()]; ok && hit.size == stat.Size() && hit.modTime.Equal(stat.ModTime()) {
				next[entry.Name()] = hit
				out = append(out, cloneTicketInfo(hit.info))
				continue
			}
		}
		info, err := readTicket(root, path, status)
		if err != nil {
			return nil, err
		}
		if stat != nil && time.Since(stat.ModTime()) > ticketCacheRacyWindow {
			next[entry.Name()] = ticketCacheEntry{size: stat.Size(), modTime: stat.ModTime(), info: cloneTicketInfo(info)}
		}
		out = append(out, info)
	}

	ticketCache.Lock()
	ticketCache.dirs[statusDir] = next
	ticketCache.Unlock()
	return out, nil
}

// cloneTicketInfo deep-copies the slices and maps of a parsed ticket, so a
// caller that appends to or edits a returned ticket never reaches the cache.
func cloneTicketInfo(info TicketInfo) TicketInfo {
	info.Related = maps.Clone(info.Related)
	info.Plans = slices.Clone(info.Plans)
	info.Skeletons = slices.Clone(info.Skeletons)
	info.UnresolvedPhases = slices.Clone(info.UnresolvedPhases)
	info.Phases = slices.Clone(info.Phases)
	info.MatchingSnippets = slices.Clone(info.MatchingSnippets)
	info.RouteFacts = maps.Clone(info.RouteFacts)
	info.BlockedBy = slices.Clone(info.BlockedBy)
	info.BlockedHeadings = slices.Clone(info.BlockedHeadings)
	info.Assignee = slices.Clone(info.Assignee)
	if info.DispatchBlocked != nil {
		block := *info.DispatchBlocked
		info.DispatchBlocked = &block
	}
	if info.AssigneeGate != nil {
		gate := *info.AssigneeGate
		info.AssigneeGate = &gate
	}
	if info.Ownership != nil {
		own := *info.Ownership
		info.Ownership = &own
	}
	return info
}
