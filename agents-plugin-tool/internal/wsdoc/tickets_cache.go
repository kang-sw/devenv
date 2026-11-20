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

// lstatTicketFile and readTicketDir are the ticket map's targeted closed-ref
// lookup and status-directory listing, variables so a test can count file
// touches per build.
var (
	lstatTicketFile = os.Lstat
	readTicketDir   = os.ReadDir
)

type ticketCacheEntry struct {
	size    int64
	modTime time.Time
	info    TicketInfo
	// doc is the ticket map's feature document, filled only by a map build
	// (scanStatusDirWith wantDoc=true) and carried across plain scans. A plain
	// listing never reads or returns it.
	doc *ticketDoc
}

// ticketScan is one listed ticket of scanStatusDirWith: the parse, the map
// document when requested, and the listing facts the map fingerprint needs.
// stable is false for a symlinked entry or a file younger than
// ticketCacheRacyWindow, whose listing cannot prove it unchanged.
type ticketScan struct {
	info    TicketInfo
	doc     *ticketDoc
	size    int64
	modTime time.Time
	stable  bool
}

// listStatusDirEntries lists one status directory through readTicketDir,
// sorted like sortedEntries. Every read error is returned, unlike
// sortedEntries, so a map build on an unreadable board fails instead of
// reporting an empty one; a missing directory returns an error wrapping
// fs.ErrNotExist, which the caller treats as an empty listing.
func listStatusDirEntries(statusDir string) ([]os.DirEntry, error) {
	entries, err := readTicketDir(statusDir)
	if err != nil {
		return nil, err
	}
	slices.SortFunc(entries, func(a, b os.DirEntry) int {
		if a.IsDir() != b.IsDir() {
			if a.IsDir() {
				return -1
			}
			return 1
		}
		switch {
		case a.Name() < b.Name():
			return -1
		case a.Name() > b.Name():
			return 1
		}
		return 0
	})
	return entries, nil
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
	scans, err := scanStatusEntries(root, statusDir, status, sortedEntries(statusDir), false)
	if err != nil {
		return nil, err
	}
	out := make([]TicketInfo, len(scans))
	for i, scan := range scans {
		out[i] = scan.info
	}
	return out, nil
}

// scanStatusDirWith is scanStatusDir for the ticket map: the listing goes
// through the error-returning listStatusDirEntries, and wantDoc fills each
// entry's ticketDoc from the same single read that parses it. A revalidated
// hit that already carries a doc costs no open; a hit cached by a plain
// listing (doc nil) is read once and its doc carried from then on.
func scanStatusDirWith(root, statusDir, status string, wantDoc bool) ([]ticketScan, error) {
	entries, err := listStatusDirEntries(statusDir)
	if err != nil {
		return nil, err
	}
	return scanStatusEntries(root, statusDir, status, entries, wantDoc)
}

func scanStatusEntries(root, statusDir, status string, entries []os.DirEntry, wantDoc bool) ([]ticketScan, error) {
	ticketCache.Lock()
	prev := ticketCache.dirs[statusDir]
	ticketCache.Unlock()

	next := map[string]ticketCacheEntry{}
	out := []ticketScan{}
	for _, entry := range entries {
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
				if wantDoc && hit.doc == nil {
					raw, err := readTicketFile(path)
					if err != nil {
						return nil, err
					}
					hit.doc = buildTicketDoc(hit.info.Stem, string(raw), false)
				}
				next[entry.Name()] = hit
				out = append(out, ticketScan{info: cloneTicketInfo(hit.info), doc: hit.doc, size: hit.size, modTime: hit.modTime, stable: true})
				continue
			}
		}
		raw, err := readTicketFile(path)
		if err != nil {
			return nil, err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil, err
		}
		info := readTicketFromBytes(filepath.ToSlash(rel), status, string(raw))
		var doc *ticketDoc
		if wantDoc {
			doc = buildTicketDoc(info.Stem, string(raw), false)
		}
		scan := ticketScan{info: info, doc: doc, size: int64(len(raw))}
		if stat != nil {
			scan.size, scan.modTime = stat.Size(), stat.ModTime()
			if time.Since(stat.ModTime()) > ticketCacheRacyWindow {
				scan.stable = true
				next[entry.Name()] = ticketCacheEntry{size: stat.Size(), modTime: stat.ModTime(), info: cloneTicketInfo(info), doc: doc}
			}
		}
		out = append(out, scan)
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
