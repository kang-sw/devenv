package wslive

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// The conversation ring keeps a session's last RingHistoryCap finished
// conversation messages, one file per entry named <fixed-width UTC
// nanosecond stamp>-<random hex>.json. Every entry gets a fresh name through
// temp+rename, so concurrent writers (several MCP processes can hold one
// session key) never replace each other's file and need no lock, and a
// reader never sees a partial entry. The name's stamp is the entry's time,
// and since names sort in time order and never change, a name is also a
// stable paging cursor: ReadRing is the newest RingCap entries (the live
// view) and ReadRingPage walks older ones a page at a time.

// Ring entry kinds.
const (
	RingUser  = "user"
	RingReply = "reply"
	RingTurn  = "turn"
	RingMail  = "mail"
)

// Mail directions.
const (
	MailSent = "sent"
	MailRecv = "recv"
)

// ringStampLayout sorts lexically in time order.
const ringStampLayout = "20060102T150405.000000000Z"

// RingEntry is one ring file.
type RingEntry struct {
	// Kind is user, reply, turn, or mail.
	Kind string `json:"kind"`
	// Origin tags a user message (terminal, whiteboard) or a turn summary
	// (terminal, whiteboard, mail; empty when unknown).
	Origin string `json:"origin,omitempty"`
	// Text is a user message or a lead reply.
	Text      string `json:"text,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
	// Done, Failed, and DurationMs summarize a turn: its tool calls and its
	// length.
	Done       int   `json:"done,omitempty"`
	Failed     int   `json:"failed,omitempty"`
	DurationMs int64 `json:"durationMs,omitempty"`
	// Direction, Peer, and Title describe a mail line; HistoryKey is the
	// file name of the message's entry in the session's mailbox history,
	// where its body stays while that shorter ring still holds it.
	Direction  string `json:"direction,omitempty"`
	Peer       string `json:"peer,omitempty"`
	Title      string `json:"title,omitempty"`
	HistoryKey string `json:"historyKey,omitempty"`
}

// RingRecord is a read entry with its file name and time.
type RingRecord struct {
	RingEntry
	Name string
	At   time.Time
}

// ringNow is the stamp clock (a test seam).
var ringNow = time.Now

var ringClock struct {
	sync.Mutex
	last time.Time
}

// nextRingStamp is strictly increasing within this process, so the entries of
// one call keep their order even when the clock does not advance.
func nextRingStamp() time.Time {
	ringClock.Lock()
	defer ringClock.Unlock()
	now := ringNow().UTC()
	if !now.After(ringClock.last) {
		now = ringClock.last.Add(time.Nanosecond)
	}
	ringClock.last = now
	return now
}

func isRingEntryName(name string) bool {
	return !strings.HasPrefix(name, ".") && strings.HasSuffix(name, ".json")
}

var ringNamePattern = regexp.MustCompile(`^[0-9]{8}T[0-9]{6}\.[0-9]{9}Z-[0-9a-f]+\.json$`)

// ValidRingName reports whether name has the grammar of an entry file name
// this package writes, so a caller can accept one as a paging cursor without
// it ever naming a path.
func ValidRingName(name string) bool { return ringNamePattern.MatchString(name) }

// AppendRing writes entries, in order, to the ring at dir (created when
// missing) and trims it to the newest RingHistoryCap. It returns the first write
// error and stops there. Trimming is best effort.
func AppendRing(dir string, entries []RingEntry) error {
	if len(entries) == 0 {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create conversation ring dir: %w", err)
	}
	var werr error
	for _, entry := range entries {
		if werr = writeRingEntry(dir, entry); werr != nil {
			break
		}
	}
	trimRing(dir)
	return werr
}

func writeRingEntry(dir string, entry RingEntry) error {
	raw, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	var suffix [4]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return err
	}
	name := nextRingStamp().Format(ringStampLayout) + "-" + hex.EncodeToString(suffix[:]) + ".json"
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("create conversation ring temp file: %w", err)
	}
	tmpPath := tmp.Name()
	// CreateTemp makes the file 0600; the reader may run as another user.
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write conversation ring entry: %w", err)
	}
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write conversation ring entry: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("write conversation ring entry: %w", err)
	}
	if err := os.Rename(tmpPath, filepath.Join(dir, name)); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("commit conversation ring entry: %w", err)
	}
	return nil
}

// trimRing removes all but the newest RingHistoryCap entries, ignoring
// errors: a concurrent trim or a failed remove only leaves extra entries
// until the next write, and a reader skips an entry removed under it.
func trimRing(dir string) {
	names, err := ringEntryNames(dir)
	if err != nil || len(names) <= RingHistoryCap {
		return
	}
	for _, name := range names[:len(names)-RingHistoryCap] {
		_ = os.Remove(filepath.Join(dir, name))
	}
}

// ringEntryNames lists dir's committed entry names, oldest first.
func ringEntryNames(dir string) ([]string, error) {
	dirEntries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(dirEntries))
	for _, de := range dirEntries {
		if isRingEntryName(de.Name()) {
			names = append(names, de.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

// RingNames returns the ring's entry file names at dir, oldest first; a
// missing directory is an empty ring. An entry file is never rewritten, so
// the names alone tell a reader whether the ring changed.
func RingNames(dir string) ([]string, error) {
	names, err := ringEntryNames(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return names, err
}

// ReadRing returns the ring's live view at dir: its newest RingCap entries,
// oldest first (ReadRingPage with no cursor).
func ReadRing(dir string) ([]RingRecord, error) {
	page, err := ReadRingPage(dir, "", RingCap)
	return page.Records, err
}

// RingPage is one page of the ring, read by ReadRingPage.
type RingPage struct {
	// Records are the page's entries, oldest first.
	Records []RingRecord
	// Before is the cursor for the next older page: the oldest entry name
	// the page covered, empty when it covered none.
	Before string
	// End reports that no entry older than this page remains.
	End bool
}

// ReadRingPage returns the newest n entries at dir whose names sort before
// before (exclusive; empty means the newest entries), oldest first. A
// missing directory is an empty ring. An entry that cannot be read or
// parsed, or whose name carries no valid stamp, is skipped but still counts
// toward n and the cursor, so a junk file never stalls paging. Unknown entry
// fields are ignored.
func ReadRingPage(dir, before string, n int) (RingPage, error) {
	names, err := ringEntryNames(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return RingPage{End: true}, nil
	}
	if err != nil {
		return RingPage{}, err
	}
	if before != "" {
		names = names[:sort.SearchStrings(names, before)]
	}
	if n < 0 {
		n = 0
	}
	start := len(names) - n
	if start < 0 {
		start = 0
	}
	page := RingPage{End: start == 0}
	names = names[start:]
	if len(names) > 0 {
		page.Before = names[0]
	}
	page.Records = make([]RingRecord, 0, len(names))
	for _, name := range names {
		if rec, ok := readRingEntry(dir, name); ok {
			page.Records = append(page.Records, rec)
		}
	}
	return page, nil
}

func readRingEntry(dir, name string) (RingRecord, bool) {
	stamp, _, ok := strings.Cut(name, "-")
	if !ok {
		return RingRecord{}, false
	}
	at, err := time.Parse(ringStampLayout, stamp)
	if err != nil {
		return RingRecord{}, false
	}
	raw, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return RingRecord{}, false
	}
	var entry RingEntry
	if json.Unmarshal(raw, &entry) != nil {
		return RingRecord{}, false
	}
	return RingRecord{RingEntry: entry, Name: name, At: at.UTC()}, true
}
