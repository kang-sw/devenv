// Package wsinbox is a session's board-answer inbox: the messages a user
// sends from the dashboard page, held on disk until the session's host
// adapter drains them at the end of a turn and injects them as the user's own
// message.
//
// The inbox is a directory of entry files, one per held message, named
// <fixed-width UTC nanosecond stamp>-<random hex>.json. An entry file is
// never rewritten: a merge replaces the entry with a new one. Every state
// change of an entry starts with an atomic claim, a rename of its file away
// from the inbox, so of a drain, a merge, and a cancel racing for one entry
// exactly one wins:
//
//   - Drain renames each entry into the drained/ subdirectory, where it stays
//     as the record the page's sent marks read (the newest DrainedCap).
//   - Merge renames the newest entry to a claim file, writes the merged entry
//     under a new name, and removes the claim. When the claim fails because a
//     drain took the entry, the send becomes a new entry instead, so a send is
//     never lost and a drained entry never comes back.
//   - Cancel renames the entry to a claim file, reads it, and removes it.
//     When the claim fails, the entry was already drained.
//
// Held means undrained: an entry in the inbox directory. A reader tells a
// held entry from a drained one by the directory it sits in.
//
// Readers ignore unknown entry fields and treat absent ones as empty. The
// package depends on nothing but the standard library, so the dashboard
// daemon, ws-mcp, and the mailbox wait all share it.
package wsinbox

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

const (
	// SchemaVersion is the entry format version.
	SchemaVersion = 1
	// DrainedCap is how many drained entries stay as sent records.
	DrainedCap = 20

	dirName        = "inbox"
	drainedDirName = "drained"
	stampLayout    = "20060102T150405.000000000Z"

	// Limits on one send.
	maxTextBytes = 64 << 10
	maxChips     = 100
	maxNoteBytes = 4 << 10
)

var (
	// ErrEmpty reports a send with no text and no chips.
	ErrEmpty = errors.New("empty message: no text and no answers")
	// ErrGone reports that an entry is no longer held: a drain delivered it,
	// or a cancel or merge already took it.
	ErrGone = errors.New("message is no longer held")
	// ErrInvalid reports a send that breaks a limit or carries a malformed
	// answer; its wrapped text says which.
	ErrInvalid = errors.New("invalid message")

	entryNamePattern = regexp.MustCompile(`^[0-9]{8}T[0-9]{6}\.[0-9]{9}Z-[0-9a-f]{8}\.json$`)
	chipIDPattern    = regexp.MustCompile(`^[0-9]{1,6}$`)
	answerPattern    = regexp.MustCompile(`^[A-Za-z0-9]{1,16}$`)
)

// Chip is one board item's answer: the option letter, ok, or no, and an
// optional note. ID is the item's number as the board shows it.
type Chip struct {
	ID     string `json:"id"`
	Answer string `json:"answer,omitempty"`
	Note   string `json:"note,omitempty"`
}

// Entry is one held message.
type Entry struct {
	SchemaVersion int `json:"schemaVersion"`
	// Parts holds the typed text of each send merged into this entry, in
	// order; the message keeps each as its own paragraph.
	Parts []string `json:"parts,omitempty"`
	// Chips holds one answer per board item; a later send's chip for the
	// same item replaced the earlier one in place.
	Chips []Chip `json:"chips,omitempty"`
	// SentAt is the first send's time, epoch milliseconds.
	SentAt int64 `json:"sentAt,omitempty"`
}

// Record is an entry read from disk with its file name.
type Record struct {
	Name  string
	Entry Entry
}

// ID is the record's entry ID: its file name without the extension.
func (r Record) ID() string { return strings.TrimSuffix(r.Name, ".json") }

// Dir is the inbox directory inside a session's dashboard directory.
func Dir(sessionDir string) string { return filepath.Join(sessionDir, dirName) }

// DrainedDir is where drained entries stay as sent records.
func DrainedDir(inboxDir string) string { return filepath.Join(inboxDir, drainedDirName) }

// NameForID maps an entry ID back to its file name; false when id is not a
// well-formed entry ID, so a caller-supplied ID never names another path.
func NameForID(id string) (string, bool) {
	name := id + ".json"
	return name, entryNamePattern.MatchString(name)
}

// Text is the message the session receives: each part as its own paragraph,
// then the answer block.
func (e Entry) Text() string {
	var paras []string
	for _, p := range e.Parts {
		if p = strings.TrimSpace(p); p != "" {
			paras = append(paras, p)
		}
	}
	if block := AnswerBlock(e.Chips); block != "" {
		paras = append(paras, block)
	}
	return strings.Join(paras, "\n\n")
}

// AnswerBlock renders chips in the board's reply grammar, the one the static
// page's copy-answers builds: "(nn) <answer>[, note: <text>]" per item, a
// note-only item as "(nn) note: <text>", the items answered ok without a note
// grouped last as "(nn), (mm) ok", and the parts joined by "; ".
func AnswerBlock(chips []Chip) string {
	var parts, acked []string
	for _, c := range chips {
		note := strings.TrimSpace(c.Note)
		if note == "" && c.Answer == "ok" {
			acked = append(acked, "("+c.ID+")")
			continue
		}
		s := ""
		if c.Answer != "" {
			s = "(" + c.ID + ") " + c.Answer
		}
		if note != "" {
			if s == "" {
				s = "(" + c.ID + ") note: " + note
			} else {
				s += ", note: " + note
			}
		}
		if s != "" {
			parts = append(parts, s)
		}
	}
	if len(acked) > 0 {
		parts = append(parts, strings.Join(acked, ", ")+" ok")
	}
	return strings.Join(parts, "; ")
}

// Normalize trims and checks one send: empty parts are dropped, chips are
// validated, and a later chip for an item replaces an earlier one. It
// returns ErrEmpty when nothing is left to send and an ErrInvalid error for a
// malformed one.
func Normalize(e Entry) (Entry, error) {
	out := Entry{SchemaVersion: SchemaVersion, SentAt: e.SentAt}
	size := 0
	for _, p := range e.Parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		size += len(p)
		out.Parts = append(out.Parts, p)
	}
	if size > maxTextBytes {
		return Entry{}, fmt.Errorf("%w: text is longer than %d bytes", ErrInvalid, maxTextBytes)
	}
	if len(e.Chips) > maxChips {
		return Entry{}, fmt.Errorf("%w: more than %d answers", ErrInvalid, maxChips)
	}
	for _, c := range e.Chips {
		c.ID = strings.TrimSpace(c.ID)
		c.Answer = strings.TrimSpace(c.Answer)
		c.Note = strings.TrimSpace(c.Note)
		if !chipIDPattern.MatchString(c.ID) {
			return Entry{}, fmt.Errorf("%w: item id %q", ErrInvalid, c.ID)
		}
		if c.Answer != "" && !answerPattern.MatchString(c.Answer) {
			return Entry{}, fmt.Errorf("%w: answer %q for item %s", ErrInvalid, c.Answer, c.ID)
		}
		if len(c.Note) > maxNoteBytes {
			return Entry{}, fmt.Errorf("%w: note for item %s is longer than %d bytes", ErrInvalid, c.ID, maxNoteBytes)
		}
		if c.Answer == "" && c.Note == "" {
			continue
		}
		out.Chips = putChip(out.Chips, c)
	}
	if len(out.Parts) == 0 && len(out.Chips) == 0 {
		return Entry{}, ErrEmpty
	}
	return out, nil
}

// putChip replaces the chip for c.ID in place, or appends c.
func putChip(chips []Chip, c Chip) []Chip {
	for i := range chips {
		if chips[i].ID == c.ID {
			chips[i] = c
			return chips
		}
	}
	return append(chips, c)
}

// Merge appends add's parts to base's and lays add's chips over base's: the
// later chip for an item wins. base's first-send time is kept.
func Merge(base, add Entry) Entry {
	out := Entry{SchemaVersion: SchemaVersion, SentAt: base.SentAt}
	if out.SentAt == 0 {
		out.SentAt = add.SentAt
	}
	out.Parts = append(append([]string{}, base.Parts...), add.Parts...)
	out.Chips = append([]Chip{}, base.Chips...)
	for _, c := range add.Chips {
		out.Chips = putChip(out.Chips, c)
	}
	return out
}

// stampNow is the entry name clock (a test seam).
var stampNow = time.Now

var stampClock struct {
	sync.Mutex
	last time.Time
}

// nextStamp is strictly increasing within this process, so a merged entry
// always sorts after the one it replaced.
func nextStamp() time.Time {
	stampClock.Lock()
	defer stampClock.Unlock()
	now := stampNow().UTC()
	if !now.After(stampClock.last) {
		now = stampClock.last.Add(time.Nanosecond)
	}
	stampClock.last = now
	return now
}

func randomHex(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// writeEntry commits entry under a fresh name in dir through temp+rename, so
// no reader sees a partial entry.
func writeEntry(dir string, entry Entry) (string, error) {
	raw, err := json.Marshal(entry)
	if err != nil {
		return "", err
	}
	suffix, err := randomHex(4)
	if err != nil {
		return "", err
	}
	name := nextStamp().Format(stampLayout) + "-" + suffix + ".json"
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return "", fmt.Errorf("create inbox temp file: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return "", fmt.Errorf("write inbox entry: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("write inbox entry: %w", err)
	}
	if err := os.Rename(tmpPath, filepath.Join(dir, name)); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("commit inbox entry: %w", err)
	}
	return name, nil
}

func readEntry(path string) (Entry, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Entry{}, err
	}
	var e Entry
	if err := json.Unmarshal(raw, &e); err != nil {
		return Entry{}, fmt.Errorf("parse inbox entry: %w", err)
	}
	return e, nil
}

// claim atomically takes the entry name in dir away from the inbox by
// renaming it to a hidden claim file. It returns ErrGone when another claim
// (a drain, a merge, a cancel) already took it.
func claim(dir, name, op string) (string, error) {
	suffix, err := randomHex(4)
	if err != nil {
		return "", err
	}
	claimed := filepath.Join(dir, "."+name+"."+op+"-"+suffix)
	if err := os.Rename(filepath.Join(dir, name), claimed); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", ErrGone
		}
		return "", err
	}
	return claimed, nil
}

// Send holds add in the inbox at dir: merged into the newest held entry when
// there is one, or as a new entry. It returns the resulting entry's name and
// whether it merged. add is normalized first (Normalize); its SentAt is set
// from now when zero.
func Send(dir string, add Entry, now time.Time) (name string, merged bool, err error) {
	if add.SentAt == 0 {
		add.SentAt = now.UnixMilli()
	}
	add, err = Normalize(add)
	if err != nil {
		return "", false, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", false, fmt.Errorf("create inbox dir: %w", err)
	}
	names, err := entryNames(dir)
	if err != nil {
		return "", false, err
	}
	if len(names) == 0 {
		name, err = writeEntry(dir, add)
		return name, false, err
	}
	newest := names[len(names)-1]
	claimed, err := claim(dir, newest, "merge")
	if errors.Is(err, ErrGone) {
		// A drain took the held entry: this send starts the next message.
		name, err = writeEntry(dir, add)
		return name, false, err
	}
	if err != nil {
		return "", false, err
	}
	base, err := readEntry(claimed)
	if err != nil {
		// Leave an unreadable entry where it was; this send stands alone.
		_ = os.Rename(claimed, filepath.Join(dir, newest))
		name, err = writeEntry(dir, add)
		return name, false, err
	}
	name, err = writeEntry(dir, Merge(base, add))
	if err != nil {
		// Put the claimed entry back so the earlier send is not lost.
		_ = os.Rename(claimed, filepath.Join(dir, newest))
		return "", false, err
	}
	_ = os.Remove(claimed)
	return name, true, nil
}

// Cancel takes the held entry name out of the inbox at dir and returns it.
// It returns ErrGone when the entry is no longer held.
func Cancel(dir, name string) (Entry, error) {
	if !entryNamePattern.MatchString(name) {
		return Entry{}, ErrGone
	}
	claimed, err := claim(dir, name, "cancel")
	if err != nil {
		return Entry{}, err
	}
	entry, err := readEntry(claimed)
	if err != nil {
		// Leave an unreadable entry held rather than lose it.
		_ = os.Rename(claimed, filepath.Join(dir, name))
		return Entry{}, err
	}
	_ = os.Remove(claimed)
	return entry, nil
}

// Drain claims every held entry at dir, oldest first, moving each into the
// drained directory, and returns those this call claimed. An entry another
// claim took first is skipped. The drained directory is then trimmed to the
// newest DrainedCap records.
func Drain(dir string) ([]Record, error) {
	names, err := entryNames(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(names) == 0 {
		return nil, nil
	}
	drained := DrainedDir(dir)
	if err := os.MkdirAll(drained, 0o755); err != nil {
		return nil, fmt.Errorf("create drained dir: %w", err)
	}
	var out []Record
	var firstErr error
	for _, name := range names {
		if err := os.Rename(filepath.Join(dir, name), filepath.Join(drained, name)); err != nil {
			if !errors.Is(err, fs.ErrNotExist) && firstErr == nil {
				firstErr = err
			}
			continue
		}
		entry, err := readEntry(filepath.Join(drained, name))
		if err != nil {
			// Put an unreadable entry back rather than lose it unreported.
			_ = os.Rename(filepath.Join(drained, name), filepath.Join(dir, name))
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		out = append(out, Record{Name: name, Entry: entry})
	}
	trimDrained(drained)
	return out, firstErr
}

func trimDrained(dir string) {
	names, err := entryNames(dir)
	if err != nil || len(names) <= DrainedCap {
		return
	}
	for _, name := range names[:len(names)-DrainedCap] {
		_ = os.Remove(filepath.Join(dir, name))
	}
}

// entryNames lists dir's entry file names, oldest first.
func entryNames(dir string) ([]string, error) {
	des, err := os.ReadDir(dir)
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

// Names returns the held entry names at dir, oldest first; a missing
// directory holds none. An entry file is never rewritten, so the names alone
// tell whether the inbox changed.
func Names(dir string) ([]string, error) {
	names, err := entryNames(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return names, err
}

// DrainedNames returns the drained record names of the inbox at dir, oldest
// first.
func DrainedNames(dir string) ([]string, error) { return Names(DrainedDir(dir)) }

// Held reads the held entries at dir, oldest first. An entry that cannot be
// read (a claim took it meanwhile, or it does not parse) is skipped.
func Held(dir string) ([]Record, error) { return readAll(dir) }

// Drained reads the drained records of the inbox at dir, oldest first.
func Drained(dir string) ([]Record, error) { return readAll(DrainedDir(dir)) }

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
