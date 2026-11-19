package wslive

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/kang-sw/devenv/internal/wsstate"
)

// The conversation ring keeps a session's last RingHistoryCap finished
// conversation messages, persisted in append-only JSONL segments
// (ring_store.go) and served from RAM in the daemon. Each entry keeps a
// stable name <fixed-width UTC nanosecond stamp>-<random hex>.json that sorts
// lexically in time order and acts as a paging cursor: ReadRing is the newest
// RingCap entries (the live view) and ReadRingPage walks older ones a page at
// a time. Legacy per-entry .json files are still read and merged on load.

// Ring entry kinds.
const (
	RingUser  = "user"
	RingReply = "reply"
	RingTurn  = "turn"
	RingMail  = "mail"
	RingWake  = "wake"
	// RingTools is the lead's finished tool calls between two conversation
	// entries, folded into one row (ToolSummary).
	RingTools = "tools"
	// RingCompact is a main-conversation compaction that landed or failed.
	RingCompact = "compact"
	// RingCard is a tracker card reference the lead showed (CardRef): a
	// store conversation's chat draws the item as a mini card.
	RingCard = "card"
	// RingUI is a dashboard UI command a store's support conversation sent
	// (UICommand): its chat moves the tracker view once, live (see UICommand).
	RingUI = "ui"
	// RingCommand is a slash command an adapter ran for the session: Command
	// is its typed line and Text the output it showed as text, empty when it
	// showed none (a terminal panel).
	RingCommand = "command"
	// RingWhiteboardEffects carries a versioned plain-text effects envelope
	// in Text, using only existing outer fields (Caption and Truncated).
	RingWhiteboardEffects = "whiteboard-effects"
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
	// Reserved only by the optional receiver path, so freezing a natural
	// turn before writing its preceding reply/tools cannot reorder the ring.
	ringStamp time.Time
	// Kind is user, reply, turn, mail, compact, tools, card, ui, command,
	// notice, or whiteboard-effects.
	Kind string `json:"kind"`
	// Origin tags a user message (terminal, whiteboard) or a turn summary
	// (terminal, whiteboard, mail; empty when unknown).
	Origin string `json:"origin,omitempty"`
	// Wake fields are optional; WakeTurn marks the turn a preceding wake started.
	Cause    string `json:"cause,omitempty"`
	Source   string `json:"source,omitempty"`
	Caption  string `json:"caption,omitempty"`
	WakeTurn bool   `json:"wakeTurn,omitempty"`
	// Text is a user message, a lead reply, or a failed compaction's error.
	Text string `json:"text,omitempty"`
	// Truncated qualifies retained text; on a summary-bearing natural turn
	// it carries aggregate clipping, separately from provider terminal facts.
	Truncated bool `json:"truncated,omitempty"`
	// Done, Failed, and DurationMs summarize a turn: its tool calls and its
	// length.
	Done       int   `json:"done,omitempty"`
	Failed     int   `json:"failed,omitempty"`
	DurationMs int64 `json:"durationMs,omitempty"`
	// Reason is a turn's end reason (Event.Reason), absent when its
	// turn_end named none.
	Reason         string          `json:"reason,omitempty"`
	SummaryHistory *SummaryHistory `json:"summaryHistory,omitempty"`
	// Tools is a tools row's summary.
	Tools *ToolSummary `json:"tools,omitempty"`
	// Direction, Peer, and Title describe a mail line; HistoryKey is the
	// file name of the message's entry in the session's mailbox history,
	// where its body stays while that shorter ring still holds it.
	Direction  string `json:"direction,omitempty"`
	Peer       string `json:"peer,omitempty"`
	Title      string `json:"title,omitempty"`
	HistoryKey string `json:"historyKey,omitempty"`
	// Trigger (TriggerAuto, TriggerManual, or empty when unknown) and
	// CompactFailed describe a compaction line: a failure or a veto, else a
	// compaction that landed.
	Trigger       string `json:"trigger,omitempty"`
	CompactFailed bool   `json:"compactFailed,omitempty"`
	// Card is a card line's item reference.
	Card *CardRef `json:"card,omitempty"`
	// UI is a ui line's dashboard command.
	UI *UICommand `json:"ui,omitempty"`
	// Notice is a notice line's typed failure.
	Notice *Notice `json:"notice,omitempty"`
	// Command is a command line's typed command ("/name args").
	Command string `json:"command,omitempty"`
}

// CardRef is a tracker item a conversation showed as a card: the store it
// belongs to, its alias, and its title when known. A page holding the
// store's view draws the card from that view; Title is what a page without
// it shows.
type CardRef struct {
	Store string `json:"store"`
	Alias string `json:"alias"`
	Title string `json:"title,omitempty"`
}

// cardIDPattern bounds a card's store id and alias: a word of letters,
// digits, and ._- (the shapes a tracker store id and item alias take).
var cardIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

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
// missing) in append-only JSONL segments and retains whole segments covering
// the newest RingHistoryCap entries. When EnableRingCache is active for dir,
// the active segment handle and entries stay in RAM.
func AppendRing(dir string, entries []RingEntry) error {
	return appendRingEntries(dir, entries)
}

// PrepareSummaryRing reserves existing ring IDs in reducer order before a
// receiver splits the real turn candidate from its ordinary reply/tool writes.
// Legacy paths never call it and retain their existing scheduling.
func PrepareSummaryRing(entries []RingEntry) {
	for i := range entries {
		if entries[i].ringStamp.IsZero() {
			entries[i].ringStamp = nextRingStamp()
		}
	}
}

// RingNames returns the ring's entry names at dir, oldest first; a missing
// directory is an empty ring. Served from RAM when EnableRingCache is active.
func RingNames(dir string) ([]string, error) {
	return listRingNames(dir)
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
// missing directory is an empty ring. Unknown entry fields are ignored.
func ReadRingPage(dir, before string, n int) (RingPage, error) {
	return readRingPage(dir, before, n)
}

// ReadRingEntry reads the one committed entry name of the ring at dir;
// false when it cannot be read or parsed or its name carries no valid
// stamp. Served from RAM when EnableRingCache is active.
func ReadRingEntry(dir, name string) (RingRecord, bool) {
	return readRingByName(dir, name)
}

// readLegacyRingEntry reads one legacy per-entry .json file from disk.
func readLegacyRingEntry(dir, name string) (RingRecord, bool) {
	at, ok := parseRingNameStamp(name)
	if !ok {
		return RingRecord{}, false
	}
	raw, err := wsstate.ReadFileOnce(filepath.Join(dir, name))
	if err != nil {
		return RingRecord{}, false
	}
	var entry RingEntry
	if json.Unmarshal(raw, &entry) != nil {
		return RingRecord{}, false
	}
	return RingRecord{RingEntry: entry, Name: name, At: at}, true
}
