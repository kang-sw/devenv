// Package wsinbox is a session's board-answer inbox: the messages a user
// sends from the dashboard page, held on disk until the session's host
// adapter drains them at the end of a turn and injects them as the user's own
// message.
//
// The inbox is a directory of entry files, one per held message, named
// <fixed-width UTC nanosecond stamp>-<random hex>.json. An entry file is
// never rewritten, and a send never joins an entry already held: every send
// is its own new entry, so each message the user sends stays visible and
// keeps its place. Every state change of an entry starts with an atomic
// claim, a rename of its file away from the inbox, so of a drain, a steer,
// and a cancel racing for one entry exactly one wins:
//
//   - Drain renames each entry into the drained/ subdirectory, where it stays
//     as the record the page's sent marks read (the newest DrainedCap).
//   - Cancel renames the entry to a claim file, reads it, and removes it.
//     When the claim fails, the entry was already drained.
//   - Steer renames the entry to a claim file, writes the same entry marked
//     for steering under a new name with a fresh stamp, and removes the
//     claim, so the promoted message joins the end of the steer queue (see
//     below). A steered entry is still held (cancel still takes it).
//   - Interrupt claims every held message entry, steered or not, removes
//     them, and writes an interrupt entry, returning the claimed messages so
//     the page can put them back in its composer.
//
// The held messages form two queues in the one directory: the steered
// entries, then the rest (follow-ups and command entries), each in stamp
// order. Queue order (queueRecords) is that order; Drain, Held, and Interrupt
// return records in it, so the page shows, and the adapter receives, the
// steer queue ahead of the follow-ups. A steered entry's stamp is the time
// it joined the steer queue, not its first send's (SentAt keeps that).
//
// An interrupt entry asks the adapter to stop the session's running turn. It
// carries only its creation time (SentAt) and is never a message: it counts
// toward no image cap, Held and MessageNames leave it out, and Cancel and
// Steer refuse it.
//
// A command entry holds one slash command the user picked in the composer
// (Command): its name and arguments, never text the inbox parses. It carries
// nothing else, so the adapter runs it natively instead of submitting it as
// text. It is a held message (Held, MessageNames, Cancel, Interrupt, and
// Drain treat it as one). Unsteered, it waits for the turn end, as one typed
// in the terminal does; Steer turns it into a steered text entry holding its
// typed line, which the agent reads and acts on itself, since an adapter
// cannot run a command natively mid-turn. A drain returns it in its place in
// the follow-up queue; the adapter splits its batch there, so follow-up text
// before a command reaches the session before it runs and text after it,
// after. Steered entries are not held back by a command: they precede the
// whole follow-up queue, as a steer-only drain mid-turn already takes them.
//
// Held means undrained: an entry in the inbox directory. A reader tells a
// held entry from a drained one by the directory it sits in. A full drain
// takes every held message and drops every interrupt entry, which it neither
// returns nor keeps as a drained record; a steer-only drain (DrainSteer)
// takes only the steered messages, which the adapter injects mid-turn, and
// leaves interrupt entries in place; an interrupt drain (DrainInterrupt)
// takes only the interrupt entries, dropping them too. An interrupt entry no
// interrupt drain took is dropped by the next full drain.
//
// An attached image carries a number, which the send's text names it by
// with an "[Image #N]" marker and the delivered message lists it by with an
// "Image #N: <path>" line. Numbers are unique within an entry.
//
// Images a send attaches live in the attachments/ subdirectory as
// <upload-id>-<n>.<ext> files the dashboard daemon writes; an entry records
// their absolute paths. A file is never renamed and its name is never tied to
// an entry name, since a steer gives the entry a new name. A file is deleted
// only once no held entry and no drained record lists it: when the drained
// record listing it is trimmed (Drain), or by the daemon's sweep of old
// unreferenced files (SweepAttachments).
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
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kang-sw/devenv/internal/wsstate"
)

const (
	// SchemaVersion is the entry format version.
	SchemaVersion = 1
	// DrainedCap is how many drained entries stay as sent records; a trimmed
	// record's images go with it.
	DrainedCap = 50
	// MaxImages is how many images one held entry may carry.
	MaxImages = 8

	dirName            = "inbox"
	drainedDirName     = "drained"
	attachmentsDirName = "attachments"
	stampLayout        = "20060102T150405.000000000Z"

	// Limits on one send.
	maxTextBytes = 64 << 10
	maxChips     = 100
	maxNoteBytes = 4 << 10
	maxTopics    = 100
	maxTopicLen  = 200
	maxMentions  = 50
	// Limits on the web card's attachments (webpicks.go). They bound the
	// elements themselves and do not count toward maxTextBytes.
	maxPicks            = 50
	maxConsoleBlocks    = 10
	maxConsoleLines     = 50 // per block
	maxConsoleLineBytes = 1024
	maxSelectorBytes    = 512
	maxPickTextBytes    = 1024
	maxWebURLBytes      = 2048
	// maxImageNum bounds an image number a send names; an entry merged
	// before sends stopped merging may number past it.
	maxImageNum = 9999
)

var (
	// ErrEmpty reports a send with no text, no chips, no topic notes, no
	// mentions, no picks, no console blocks, and no images.
	ErrEmpty = errors.New("empty message: no text and no answers")
	// ErrGone reports that an entry is no longer held: a drain delivered it,
	// or a cancel or steer already took it.
	ErrGone = errors.New("message is no longer held")
	// ErrInvalid reports a send that breaks a limit or carries a malformed
	// answer; its wrapped text says which.
	ErrInvalid = errors.New("invalid message")

	entryNamePattern = regexp.MustCompile(`^[0-9]{8}T[0-9]{6}\.[0-9]{9}Z-[0-9a-f]{8}\.json$`)
	chipIDPattern    = regexp.MustCompile(`^[0-9]{1,6}$`)
	// imageMarkerPattern is an ImageMarker in a send's text.
	imageMarkerPattern = regexp.MustCompile(`\[Image #([1-9][0-9]{0,5})\]`)
	// imageLinePattern is an ImageLine.
	imageLinePattern = regexp.MustCompile(`^Image #([1-9][0-9]{0,5}): (.+)$`)
	answerPattern    = regexp.MustCompile(`^[A-Za-z0-9]{1,16}$`)
	// commandNamePattern is a slash command's name without its slash: a
	// built-in (compact), a namespaced plugin or skill command (ws:lead-run,
	// skill:name), or an MCP prompt (mcp__server__prompt).
	commandNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,199}$`)
	// attachmentNamePattern is <upload-id>-<n>.<ext>: a 16-hex upload id
	// minted per message request, the image's 1-based index in it, and the
	// extension of its sniffed type.
	attachmentNamePattern = regexp.MustCompile(`^[0-9a-f]{16}-[1-9][0-9]?\.(png|jpg|gif|webp)$`)
)

// Chip is one board item's answer: the option letter, ok, or no, and an
// optional note. ID is the item's number as the board shows it.
type Chip struct {
	ID     string `json:"id"`
	Answer string `json:"answer,omitempty"`
	Note   string `json:"note,omitempty"`
	// SentAt is the time of the send that carried this chip, epoch
	// milliseconds; Send sets it. An entry merged before sends stopped
	// merging kept it per chip, since its own SentAt is its first send's.
	// Zero on a chip written before the field.
	SentAt int64 `json:"sentAt,omitempty"`
}

// Entry is one held message.
type Entry struct {
	SchemaVersion int `json:"schemaVersion"`
	// Parts holds the send's text, including server-serialized selections
	// (selections.go). An older combined entry may hold several parts,
	// each kept as its own paragraph; no new stored field is needed.
	Parts []string `json:"parts,omitempty"`
	// Chips holds one answer per board item.
	Chips []Chip `json:"chips,omitempty"`
	// Topics holds one note per board topic, kept as Chips are. An entry
	// written before the field carries its topic notes as "[topic] text"
	// lines in Parts, delivered as they are.
	Topics []TopicNote `json:"topics,omitempty"`
	// Mentions holds the text the user selected on the board and added to
	// the message (mentions.go), in the order added.
	Mentions []Mention `json:"mentions,omitempty"`
	// CloseRequests holds optional per-topic close requests, including empty reasons.
	CloseRequests []CloseRequest `json:"closeRequests,omitempty"`
	// Picks and Consoles hold the page elements the user picked in a web
	// card and the console errors it attached (webpicks.go), in the order
	// added.
	Picks    []Pick    `json:"picks,omitempty"`
	Consoles []Console `json:"consoles,omitempty"`
	// Images holds the absolute paths of the attached image files, in send
	// order; a path is listed once.
	Images []string `json:"images,omitempty"`
	// ImageNums holds each image's number, parallel to Images. An entry
	// written before images were numbered has none; Nums numbers its images
	// 1..n.
	ImageNums []int `json:"imageNums,omitempty"`
	// SentAt is the first send's time, epoch milliseconds; an interrupt
	// entry's creation time.
	SentAt int64 `json:"sentAt,omitempty"`
	// UTCOffsetMinutes is the sending browser's minutes east of UTC. Nil
	// keeps legacy entries in the daemon host's local zone; zero is UTC.
	UTCOffsetMinutes *int `json:"utcOffsetMinutes,omitempty"`
	// Steer marks an entry the user promoted to steering: the adapter
	// injects it at the session's next tool boundary instead of at the turn
	// end. Steered entries queue ahead of the rest (queueRecords).
	Steer bool `json:"steer,omitempty"`
	// Interrupt marks an interrupt entry (Interrupt): a request to stop the
	// session's running turn, never a message. It carries no other content.
	Interrupt bool `json:"interrupt,omitempty"`
	// Command marks a command entry: one slash command for the adapter to
	// run natively. It carries no other content but SentAt.
	Command *Command `json:"command,omitempty"`
}

// Command is a slash command as the composer picked it: the name without
// its slash, and everything after the name as the user typed it.
type Command struct {
	Name string `json:"name"`
	Args string `json:"args,omitempty"`
}

// Line is the command as typed in a terminal: "/name" or "/name args".
func (c Command) Line() string {
	if c.Args == "" {
		return "/" + c.Name
	}
	return "/" + c.Name + " " + c.Args
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

// AttachmentsDir is where the images attached to the inbox at inboxDir live.
// It is absolute, as the paths an entry lists are, so a caller that named
// the inbox by a relative path still matches them.
func AttachmentsDir(inboxDir string) string {
	dir := filepath.Join(inboxDir, attachmentsDirName)
	if abs, err := filepath.Abs(dir); err == nil {
		return abs
	}
	return dir
}

// ValidAttachmentName reports whether name is an attachment file name
// (<upload-id>-<n>.<ext>), so a caller-supplied name never names another
// path.
func ValidAttachmentName(name string) bool { return attachmentNamePattern.MatchString(name) }

// AttachmentName returns the file name of path when it names an attachment
// directly inside attachDir; false for any other path. It reads no file.
func AttachmentName(attachDir, path string) (string, bool) {
	if path == "" || !filepath.IsAbs(path) {
		return "", false
	}
	path = filepath.Clean(path)
	if filepath.Dir(path) != filepath.Clean(attachDir) {
		return "", false
	}
	name := filepath.Base(path)
	return name, ValidAttachmentName(name)
}

// ImageMarker is the marker a send's text names image n by.
func ImageMarker(n int) string { return "[Image #" + strconv.Itoa(n) + "]" }

// ImageMarkers calls fn with each ImageMarker in text: the byte range it
// spans and its number.
func ImageMarkers(text string, fn func(start, end, n int)) {
	for _, m := range imageMarkerPattern.FindAllStringSubmatchIndex(text, -1) {
		n, _ := strconv.Atoi(text[m[2]:m[3]])
		fn(m[0], m[1], n)
	}
}

// ImageLine is the line the delivered message carries for attached image n;
// the dashboard page reads it back to show the image.
func ImageLine(n int, path string) string { return "Image #" + strconv.Itoa(n) + ": " + path }

// ParseImageLine returns the number and path an image line carries: an
// ImageLine, or the "[image: <path>]" line messages carried before images
// were numbered, whose number is 0. False for any other line.
func ParseImageLine(line string) (n int, path string, ok bool) {
	if m := imageLinePattern.FindStringSubmatch(line); m != nil {
		n, _ = strconv.Atoi(m[1])
		return n, m[2], true
	}
	if strings.HasPrefix(line, "[image: ") && strings.HasSuffix(line, "]") {
		path = line[len("[image: ") : len(line)-1]
		return 0, path, path != ""
	}
	return 0, "", false
}

// Nums returns each image's number, parallel to Images: ImageNums, or 1..n
// for an entry that has none.
func (e Entry) Nums() []int {
	if len(e.ImageNums) == len(e.Images) {
		return e.ImageNums
	}
	nums := make([]int, len(e.Images))
	for i := range nums {
		nums[i] = i + 1
	}
	return nums
}

// NameForID maps an entry ID back to its file name; false when id is not a
// well-formed entry ID, so a caller-supplied ID never names another path.
func NameForID(id string) (string, bool) {
	name := id + ".json"
	return name, entryNamePattern.MatchString(name)
}

// bodyText assembles the envelope's content: the answer block (AnswerBlock)
// first, then mentions and close requests, then the picks (PickBlock) and the
// console blocks (ConsoleBlock) as one paragraph each, then each part as its
// own paragraph, then one ImageLine per image. A command entry's text is its
// typed line (Command.Line), which a host that runs a bare "/name args"
// message as the command takes as it is.
func (e Entry) bodyText() string {
	if e.Command != nil {
		return e.Command.Line()
	}
	var paras []string
	if block := AnswerBlock(e.Chips, e.Topics); block != "" {
		paras = append(paras, block)
	}
	if block := MentionBlock(e.Mentions); block != "" {
		paras = append(paras, block)
	}
	if block := CloseRequestBlock(e.CloseRequests); block != "" {
		paras = append(paras, block)
	}
	if block := PickBlock(e.Picks); block != "" {
		paras = append(paras, block)
	}
	if block := ConsoleBlock(e.Consoles); block != "" {
		paras = append(paras, block)
	}
	for _, p := range e.Parts {
		if p = strings.TrimSpace(p); p != "" {
			paras = append(paras, p)
		}
	}
	if len(e.Images) > 0 {
		nums := e.Nums()
		lines := make([]string, len(e.Images))
		for i, path := range e.Images {
			lines[i] = ImageLine(nums[i], path)
		}
		paras = append(paras, strings.Join(lines, "\n"))
	}
	return strings.Join(paras, "\n\n")
}

// Normalize trims and checks one send: empty parts are dropped, chips are
// validated, a later chip for an item (or note for a topic) replaces an
// earlier one, an empty topic note is dropped, a mention is trimmed and
// dropped when blank (its text counts toward the text limit), a pick and a
// console block are validated and capped (normalizeWeb; they do not count
// toward the text limit). Close requests dedupe by topic (last reason wins),
// count as content even without a reason, and their reasons count toward the
// text limit. A repeated image path is dropped with its
// number. An image, a pick, and a console block count as content. Image
// numbers are optional: ImageNums is empty or parallel to Images, a number is
// unique and at most maxImageNum, and an image numbered 0 (or every image,
// when ImageNums is empty) gets the next number above the highest. The
// result always numbers its images. It returns ErrEmpty when nothing is left
// to send and an ErrInvalid error for a malformed one. A command entry
// (normalizeCommand) carries its command alone.
func Normalize(e Entry) (Entry, error) {
	if e.UTCOffsetMinutes != nil && (*e.UTCOffsetMinutes < -720 || *e.UTCOffsetMinutes > 840) {
		return Entry{}, fmt.Errorf("%w: UTC offset must be -720..840 minutes", ErrInvalid)
	}
	if e.Command != nil {
		return normalizeCommand(e)
	}
	out := Entry{SchemaVersion: SchemaVersion, SentAt: e.SentAt, Steer: e.Steer, UTCOffsetMinutes: e.UTCOffsetMinutes}
	size, selectionMentions := 0, 0
	for _, p := range e.Parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		var count int
		p, count = normalizeSelectionText(p)
		selectionMentions += count
		size += len(p)
		out.Parts = append(out.Parts, p)
	}
	if len(e.Mentions)+selectionMentions > maxMentions {
		return Entry{}, fmt.Errorf("%w: more than %d mentions", ErrInvalid, maxMentions)
	}
	for _, m := range e.Mentions {
		m.Topic = strings.TrimSpace(m.Topic)
		m.ID = strings.TrimSpace(m.ID)
		m.Entry = strings.TrimSpace(m.Entry)
		m.Text = strings.TrimSpace(m.Text)
		if m.Topic == "" || len(m.Topic) > maxTopicLen || strings.ContainsAny(m.Topic, "\r\n") {
			return Entry{}, fmt.Errorf("%w: mention topic %q", ErrInvalid, m.Topic)
		}
		if m.ID != "" && !chipIDPattern.MatchString(m.ID) {
			return Entry{}, fmt.Errorf("%w: mention item id %q", ErrInvalid, m.ID)
		}
		if m.Entry != "" && !mentionEntryPattern.MatchString(m.Entry) {
			return Entry{}, fmt.Errorf("%w: mention entry %q (expected eN)", ErrInvalid, m.Entry)
		}
		if m.Text == "" {
			continue
		}
		size += len(m.Text)
		out.Mentions = append(out.Mentions, m)
	}
	if len(e.CloseRequests) > maxTopics {
		return Entry{}, fmt.Errorf("%w: more than %d close requests", ErrInvalid, maxTopics)
	}
	for _, r := range e.CloseRequests {
		r.Topic, r.Reason = strings.TrimSpace(r.Topic), strings.TrimSpace(r.Reason)
		if r.Topic == "" || len(r.Topic) > maxTopicLen || strings.ContainsAny(r.Topic, "\x00\r\n") {
			return Entry{}, fmt.Errorf("%w: close request topic %q", ErrInvalid, r.Topic)
		}
		if len(r.Reason) > maxNoteBytes {
			return Entry{}, fmt.Errorf("%w: close reason for %s is longer than %d bytes", ErrInvalid, r.Topic, maxNoteBytes)
		}
		found := false
		for i := range out.CloseRequests {
			if out.CloseRequests[i].Topic == r.Topic {
				out.CloseRequests[i] = r
				found = true
				break
			}
		}
		if !found {
			out.CloseRequests = append(out.CloseRequests, r)
		}
	}
	for _, r := range out.CloseRequests {
		size += len(r.Reason)
	}
	if size > maxTextBytes {
		return Entry{}, fmt.Errorf("%w: text is longer than %d bytes", ErrInvalid, maxTextBytes)
	}
	var err error
	if out.Picks, out.Consoles, err = normalizeWeb(e.Picks, e.Consoles); err != nil {
		return Entry{}, err
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
	if len(e.Topics) > maxTopics {
		return Entry{}, fmt.Errorf("%w: more than %d topic notes", ErrInvalid, maxTopics)
	}
	for _, tn := range e.Topics {
		tn.Topic = strings.TrimSpace(tn.Topic)
		tn.Note = strings.TrimSpace(tn.Note)
		// A topic key is one answer-block line's bracketed head, so it
		// holds no line break and no "]^", which ends the head.
		if tn.Topic == "" || len(tn.Topic) > maxTopicLen || strings.ContainsAny(tn.Topic, "\r\n") || strings.Contains(tn.Topic, "]^") {
			return Entry{}, fmt.Errorf("%w: topic %q", ErrInvalid, tn.Topic)
		}
		if len(tn.Note) > maxNoteBytes {
			return Entry{}, fmt.Errorf("%w: note for topic %s is longer than %d bytes", ErrInvalid, tn.Topic, maxNoteBytes)
		}
		if tn.Note == "" {
			continue
		}
		out.Topics = putTopic(out.Topics, tn)
	}
	if len(e.ImageNums) != 0 && len(e.ImageNums) != len(e.Images) {
		return Entry{}, fmt.Errorf("%w: %d image numbers for %d images", ErrInvalid, len(e.ImageNums), len(e.Images))
	}
	var nums []int
	for i, path := range e.Images {
		if path = strings.TrimSpace(path); path == "" {
			continue
		}
		if !filepath.IsAbs(path) || strings.ContainsAny(path, "\x00\r\n") {
			return Entry{}, fmt.Errorf("%w: image path %q", ErrInvalid, path)
		}
		if indexOf(out.Images, path) >= 0 {
			continue
		}
		n := 0
		if len(e.ImageNums) != 0 {
			n = e.ImageNums[i]
		}
		if n < 0 || n > maxImageNum {
			return Entry{}, fmt.Errorf("%w: image number %d", ErrInvalid, n)
		}
		if n != 0 && containsInt(nums, n) {
			return Entry{}, fmt.Errorf("%w: image number %d given twice", ErrInvalid, n)
		}
		out.Images = append(out.Images, path)
		nums = append(nums, n)
	}
	if len(out.Images) > MaxImages {
		return Entry{}, fmt.Errorf("%w: more than %d images", ErrInvalid, MaxImages)
	}
	if len(out.Images) > 0 {
		next := maxInt(nums)
		for i := range nums {
			if nums[i] == 0 {
				next++
				nums[i] = next
			}
		}
		out.ImageNums = nums
	}
	for _, p := range out.Parts {
		for _, part := range ParseSelections(p) {
			if part.Selection == nil {
				continue
			}
			for _, n := range part.Selection.Images {
				if !containsInt(out.ImageNums, n) {
					return Entry{}, fmt.Errorf("%w: selection image %d is not attached", ErrInvalid, n)
				}
			}
		}
	}
	if len(out.Parts) == 0 && len(out.Chips) == 0 && len(out.Topics) == 0 && len(out.Mentions) == 0 && len(out.CloseRequests) == 0 && len(out.Picks) == 0 && len(out.Consoles) == 0 && len(out.Images) == 0 {
		return Entry{}, ErrEmpty
	}
	return out, nil
}

// normalizeCommand checks a command send: a name (one leading slash
// dropped) that commandNamePattern accepts, arguments trimmed and within
// the text limit, and no other content, since a command is never joined
// with a message.
func normalizeCommand(e Entry) (Entry, error) {
	c := Command{Name: strings.TrimPrefix(strings.TrimSpace(e.Command.Name), "/"), Args: strings.TrimSpace(e.Command.Args)}
	if !commandNamePattern.MatchString(c.Name) {
		return Entry{}, fmt.Errorf("%w: command name %q", ErrInvalid, c.Name)
	}
	if len(c.Args) > maxTextBytes {
		return Entry{}, fmt.Errorf("%w: command arguments are longer than %d bytes", ErrInvalid, maxTextBytes)
	}
	for _, p := range e.Parts {
		if strings.TrimSpace(p) != "" {
			return Entry{}, fmt.Errorf("%w: a command carries no text", ErrInvalid)
		}
	}
	if len(e.Chips) > 0 || len(e.Topics) > 0 || len(e.Mentions) > 0 || len(e.CloseRequests) > 0 || len(e.Picks) > 0 || len(e.Consoles) > 0 || len(e.Images) > 0 || e.Steer || e.Interrupt {
		return Entry{}, fmt.Errorf("%w: a command carries no answers, mentions, close requests, picks, console blocks, or images", ErrInvalid)
	}
	return Entry{SchemaVersion: SchemaVersion, SentAt: e.SentAt, UTCOffsetMinutes: e.UTCOffsetMinutes, Command: &c}, nil
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

// putTopic replaces the note for t.Topic in place, or appends t.
func putTopic(topics []TopicNote, t TopicNote) []TopicNote {
	for i := range topics {
		if topics[i].Topic == t.Topic {
			topics[i] = t
			return topics
		}
	}
	return append(topics, t)
}

func indexOf(list []string, s string) int {
	for i, v := range list {
		if v == s {
			return i
		}
	}
	return -1
}

func containsInt(list []int, n int) bool {
	for _, v := range list {
		if v == n {
			return true
		}
	}
	return false
}

func maxInt(list []int) int {
	m := 0
	for _, v := range list {
		if v > m {
			m = v
		}
	}
	return m
}

// stampNow is the entry name clock (a test seam).
var stampNow = time.Now

var stampClock struct {
	sync.Mutex
	last time.Time
}

// nextStamp is strictly increasing within this process, so an entry written
// later (a send, or a steer's rewrite) always sorts after those before it.
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
	return writeEntryJournaled(dir, entry, false)
}

func writeEntryJournaled(dir string, entry Entry, journal bool) (string, error) {
	stamp := nextStamp().Format(stampLayout)
	raw, err := json.Marshal(entry)
	if err != nil {
		return "", err
	}
	suffix, err := randomHex(4)
	if err != nil {
		return "", err
	}
	name := stamp + "-" + suffix + ".json"
	tmp, err := wsstate.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return "", fmt.Errorf("create inbox temp file: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		remove(tmpPath)
		return "", fmt.Errorf("write inbox entry: %w", err)
	}
	if journal {
		if err := tmp.Sync(); err != nil {
			tmp.Close()
			remove(tmpPath)
			return "", fmt.Errorf("sync inbox entry: %w", err)
		}
	}
	if err := tmp.Close(); err != nil {
		remove(tmpPath)
		return "", fmt.Errorf("write inbox entry: %w", err)
	}
	var pending string
	if journal {
		journalDir := filepath.Join(dir, "answer-pending")
		if err := os.MkdirAll(journalDir, 0o700); err != nil {
			remove(tmpPath)
			return "", err
		}
		// Persist both the stage name and the journal directory before the
		// journal can be considered committed by the later rename.
		if err := syncInboxDir(dir); err != nil {
			remove(tmpPath)
			return "", fmt.Errorf("sync inbox directory: %w", err)
		}
		// Retain a copy even if the entry is drained or cancelled. The name
		// identifies the staging file: its disappearance is the atomic commit
		// point. A partial journal still has its stage and is discarded on replay.
		pending = filepath.Join(journalDir, name+"--"+filepath.Base(tmpPath))
		journalFile, err := wsstate.OpenFile(pending, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			remove(tmpPath)
			return "", fmt.Errorf("journal inbox answer: %w", err)
		}
		_, writeErr := journalFile.Write(raw)
		if writeErr == nil {
			writeErr = journalFile.Sync()
		}
		if closeErr := journalFile.Close(); writeErr == nil {
			writeErr = closeErr
		}
		if writeErr != nil {
			discardUncommitted(pending, tmpPath)
			return "", fmt.Errorf("journal inbox answer: %w", writeErr)
		}
		if err := syncInboxDir(journalDir); err != nil {
			discardUncommitted(pending, tmpPath)
			return "", fmt.Errorf("sync answer journal: %w", err)
		}
	}
	commit := rename
	if journal {
		commit = wsstate.AtomicReplaceFile
	}
	err = commit(tmpPath, filepath.Join(dir, name))
	bumpNames()
	if err != nil {
		// Keep the stage if removing the journal fails: recovery must still
		// recognize this as an uncommitted send.
		if pending == "" {
			remove(tmpPath)
		} else {
			discardUncommitted(pending, tmpPath)
		}
		return "", fmt.Errorf("commit inbox entry: %w", err)
	}
	if journal {
		// The write is already visible; a sync error cannot safely be reported
		// as a failed send without prompting a duplicate submission.
		if err := syncInboxDir(dir); err != nil {
			fmt.Fprintf(os.Stderr, "wsinbox: sync published inbox entry: %v\n", err)
		}
	}
	return name, nil
}

func discardUncommitted(pending, stage string) {
	if remove(pending) == nil && syncInboxDir(filepath.Dir(pending)) == nil {
		remove(stage)
	}
}

func syncInboxDir(path string) error {
	// Windows uses a write-through publication; directory Sync is unsupported.
	if runtime.GOOS == "windows" {
		return nil
	}
	dir, err := wsstate.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func readEntry(path string) (Entry, error) {
	raw, err := readFile(path)
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
// (a drain, a steer, a cancel) already took it.
func claim(dir, name, op string) (string, error) {
	suffix, err := randomHex(4)
	if err != nil {
		return "", err
	}
	claimed := filepath.Join(dir, "."+name+"."+op+"-"+suffix)
	if err := rename(filepath.Join(dir, name), claimed); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", ErrGone
		}
		return "", err
	}
	return claimed, nil
}

// Send holds add in the inbox at dir as a new entry and returns its name;
// merged is always false (a send never joins a held entry, so every message
// keeps its own place in the queue). add is normalized first (Normalize);
// its SentAt is set from now when zero.
func Send(dir string, add Entry, now time.Time) (name string, merged bool, err error) {
	return sendEntry(dir, add, now, false)
}

// SendJournaled retains an answer until its board revision has been recorded.
func SendJournaled(dir string, add Entry, now time.Time) (name string, merged bool, err error) {
	return sendEntry(dir, add, now, true)
}

func sendEntry(dir string, add Entry, now time.Time, journal bool) (name string, merged bool, err error) {
	if add.SentAt == 0 {
		add.SentAt = now.UnixMilli()
	}
	add, err = Normalize(add)
	if err != nil {
		return "", false, err
	}
	// Every chip carries this send's time, whatever the caller put there.
	for i := range add.Chips {
		add.Chips[i].SentAt = add.SentAt
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", false, fmt.Errorf("create inbox dir: %w", err)
	}
	if journal {
		if err := syncInboxDir(filepath.Dir(dir)); err != nil {
			return "", false, fmt.Errorf("sync inbox parent: %w", err)
		}
	}
	name, err = writeEntryJournaled(dir, add, journal)
	return name, false, err
}

// Steer marks the held entry name at dir for steering: it claims the entry
// and writes the same entry with Steer set under a fresh name, so entry files
// stay never rewritten. The fresh name's stamp is now, so the entry moves to
// the end of the steer queue. It returns the steered entry's name; an entry already
// steered is left as it is and its own name returned. A command entry is
// steered as text: its typed line (Command.Line) becomes the entry's one
// part and the command is dropped, so the adapter injects it mid-turn as a
// message the agent reads. It returns ErrGone when the entry is no longer
// held, or is an interrupt entry, which it leaves in place.
func Steer(dir, name string) (string, error) {
	return SteerWithAdmission(dir, name, nil)
}

// SteerWithAdmission checks the already-read, exclusively claimed entry before
// rewriting it. A refusal restores the original entry. Admission adds no read.
func SteerWithAdmission(dir, name string, admit func(Entry) error) (string, error) {
	if !entryNamePattern.MatchString(name) {
		return "", ErrGone
	}
	claimed, err := claim(dir, name, "steer")
	if err != nil {
		return "", err
	}
	entry, err := readEntry(claimed)
	if err == nil && entry.Interrupt {
		_ = rename(claimed, filepath.Join(dir, name))
		return "", ErrGone
	}
	if err == nil && admit != nil {
		if err := admit(entry); err != nil {
			_ = rename(claimed, filepath.Join(dir, name))
			return "", err
		}
	}
	if err != nil || entry.Steer {
		// Leave an unreadable or already steered entry held as it was.
		_ = rename(claimed, filepath.Join(dir, name))
		if err != nil {
			return "", err
		}
		return name, nil
	}
	entry.Steer = true
	if entry.Command != nil {
		entry.Parts = []string{entry.Command.Line()}
		entry.Command = nil
	}
	steered, err := writeEntry(dir, entry)
	if err != nil {
		// Put the claimed entry back so the message is not lost.
		_ = rename(claimed, filepath.Join(dir, name))
		return "", err
	}
	_ = remove(claimed)
	return steered, nil
}

// Cancel takes the held entry name out of the inbox at dir and returns it.
// It returns ErrGone when the entry is no longer held, or is an interrupt
// entry, which it leaves in place.
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
		_ = rename(claimed, filepath.Join(dir, name))
		return Entry{}, err
	}
	if entry.Interrupt {
		_ = rename(claimed, filepath.Join(dir, name))
		return Entry{}, ErrGone
	}
	_ = remove(claimed)
	return entry, nil
}

// writeInterruptEntry commits Interrupt's entry (a test seam).
var writeInterruptEntry = writeEntry

// Interrupt claims every held message entry at dir, steered or not, removes
// them, and writes an interrupt entry whose SentAt is now. It returns the
// claimed records, in queue order (queueRecords), and the interrupt entry's
// name.
// A message another claim took first is skipped, and an unreadable one is
// left held, as Cancel leaves it. When the interrupt entry cannot be written,
// every claimed message is put back and the error returned.
func Interrupt(dir string, now time.Time) (held []Record, name string, err error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, "", fmt.Errorf("create inbox dir: %w", err)
	}
	names, err := entryNames(dir)
	if err != nil {
		return nil, "", err
	}
	// An entry file is never rewritten, so reading it before the claim tells
	// whether the claimed file is a message.
	names = filterNames(dir, names, func(e Entry) bool { return !e.Interrupt })
	var claims []string
	putBack := func() {
		for i, c := range claims {
			_ = rename(c, filepath.Join(dir, held[i].Name))
		}
	}
	for _, n := range names {
		claimed, err := claim(dir, n, "interrupt")
		if errors.Is(err, ErrGone) {
			continue
		}
		if err != nil {
			putBack()
			return nil, "", err
		}
		entry, err := readEntry(claimed)
		if err != nil {
			_ = rename(claimed, filepath.Join(dir, n))
			continue
		}
		claims = append(claims, claimed)
		held = append(held, Record{Name: n, Entry: entry})
	}
	name, err = writeInterruptEntry(dir, Entry{SchemaVersion: SchemaVersion, Interrupt: true, SentAt: now.UnixMilli()})
	if err != nil {
		putBack()
		return nil, "", err
	}
	for _, c := range claims {
		_ = remove(c)
	}
	return queueRecords(held), name, nil
}

// Drain claims every held message at dir, moving each into the drained
// directory, and returns those this call claimed, in queue order
// (queueRecords): the steered ones first, then the rest. An entry another
// claim took first is skipped. Every interrupt entry it meets is claimed and
// deleted: never moved to the drained directory and never returned. The
// drained directory is then trimmed to the newest DrainedCap records, and
// the images only trimmed records listed are deleted.
func Drain(dir string) ([]Record, error) { return drain(dir, false) }

// DrainSteer is Drain restricted to steered entries: it claims only the held
// entries marked for steering and leaves every other one, interrupt entries
// included, held.
func DrainSteer(dir string) ([]Record, error) { return drain(dir, true) }

// DrainInterrupt claims only the interrupt entries at dir, oldest first,
// deletes them, keeping no drained record, and returns those this call
// claimed. Every message stays held.
func DrainInterrupt(dir string) ([]Record, error) {
	names, err := InterruptNames(dir)
	if err != nil || len(names) == 0 {
		return nil, err
	}
	return takeInterrupts(dir, names)
}

// takeInterrupts claims and deletes each named interrupt entry at dir and
// returns those it claimed. A claimed entry that does not read as an
// interrupt entry is put back.
func takeInterrupts(dir string, names []string) ([]Record, error) {
	var out []Record
	var firstErr error
	for _, name := range names {
		claimed, err := claim(dir, name, "drain")
		if err != nil {
			if !errors.Is(err, ErrGone) && firstErr == nil {
				firstErr = err
			}
			continue
		}
		entry, err := readEntry(claimed)
		if err != nil || !entry.Interrupt {
			_ = rename(claimed, filepath.Join(dir, name))
			if err != nil && firstErr == nil {
				firstErr = err
			}
			continue
		}
		_ = remove(claimed)
		out = append(out, Record{Name: name, Entry: entry})
	}
	return out, firstErr
}

func drain(dir string, steerOnly bool) ([]Record, error) {
	names, err := entryNames(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if steerOnly {
		// An entry file is never rewritten, so reading it before the claim
		// tells whether the claimed file is steered.
		names = filterNames(dir, names, func(e Entry) bool { return e.Steer })
	} else {
		// Drop the interrupt entries; every other entry, an unreadable one
		// included, takes the drained path below. A failed drop leaves the
		// entry for the next full drain.
		interrupts := filterNames(dir, names, func(e Entry) bool { return e.Interrupt })
		if len(interrupts) > 0 {
			_, _ = takeInterrupts(dir, interrupts)
			names = without(names, interrupts)
		}
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
		if err := rename(filepath.Join(dir, name), filepath.Join(drained, name)); err != nil {
			if !errors.Is(err, fs.ErrNotExist) && firstErr == nil {
				firstErr = err
			}
			continue
		}
		entry, err := readEntry(filepath.Join(drained, name))
		if err != nil {
			// Put an unreadable entry back rather than lose it unreported.
			_ = rename(filepath.Join(drained, name), filepath.Join(dir, name))
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		out = append(out, Record{Name: name, Entry: entry})
	}
	trimDrained(dir)
	return queueRecords(out), firstErr
}

// queueRecords orders records, given in stamp order, as the inbox's two
// queues: the steered entries first, then the rest (follow-ups and command
// entries), each part keeping stamp order. A steer stamps the entry anew, so
// a promoted message lands at the end of the steer queue.
func queueRecords(records []Record) []Record {
	out := make([]Record, 0, len(records))
	for _, r := range records {
		if r.Entry.Steer {
			out = append(out, r)
		}
	}
	for _, r := range records {
		if !r.Entry.Steer {
			out = append(out, r)
		}
	}
	return out
}

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

// without returns names less every name in drop; both are sorted.
func without(names, drop []string) []string {
	out := names[:0:0]
	for _, name := range names {
		if len(drop) > 0 && drop[0] == name {
			drop = drop[1:]
			continue
		}
		out = append(out, name)
	}
	return out
}

// trimDrained trims the drained records of the inbox at dir to the newest
// DrainedCap, then deletes the images the trimmed records listed that no
// held entry and no remaining record lists.
func trimDrained(dir string) {
	drained := DrainedDir(dir)
	names, err := entryNames(drained)
	if err != nil || len(names) <= DrainedCap {
		return
	}
	attachDir := AttachmentsDir(dir)
	var listed []string
	for _, name := range names[:len(names)-DrainedCap] {
		if e, err := readEntry(filepath.Join(drained, name)); err == nil {
			for _, path := range e.Images {
				if n, ok := AttachmentName(attachDir, path); ok {
					listed = append(listed, n)
				}
			}
		}
		_ = remove(filepath.Join(drained, name))
	}
	if len(listed) == 0 {
		return
	}
	referenced := referencedAttachments(dir)
	for _, n := range listed {
		if !referenced[n] {
			_ = os.Remove(filepath.Join(attachDir, n))
		}
	}
}

// referencedAttachments is the set of attachment names some entry of the
// inbox at dir lists. It reads the held side, then the drained records, then
// the held side again: a drain moves an entry from held to drained, so an
// entry that moves between the reads is seen in one of them. The held side
// is every file directly in dir that parses as an entry, so an entry a steer
// or cancel has claimed, or a steer is writing, counts too.
func referencedAttachments(dir string) map[string]bool {
	attachDir := AttachmentsDir(dir)
	out := map[string]bool{}
	add := func(e Entry) {
		for _, path := range e.Images {
			if n, ok := AttachmentName(attachDir, path); ok {
				out[n] = true
			}
		}
	}
	readHeldSide := func() {
		des, err := wsstate.ReadDir(dir)
		if err != nil {
			return
		}
		for _, de := range des {
			if de.IsDir() {
				continue
			}
			if e, err := readEntry(filepath.Join(dir, de.Name())); err == nil {
				add(e)
			}
		}
	}
	readHeldSide()
	if records, err := readAll(DrainedDir(dir)); err == nil {
		for _, r := range records {
			add(r.Entry)
		}
	}
	readHeldSide()
	return out
}

// SweepAttachments deletes each attachment of the inbox at dir that was last
// modified more than maxAge before now and that no held entry, no drained
// record, and no name in keep lists: an image whose message was cancelled and
// never sent again. The dashboard daemon runs it under its inbox lock on each
// message write, keeping the attachments that write references.
func SweepAttachments(dir string, now time.Time, maxAge time.Duration, keep ...string) {
	attachDir := AttachmentsDir(dir)
	des, err := wsstate.ReadDir(attachDir)
	if err != nil {
		return
	}
	var old []string
	for _, de := range des {
		if !de.Type().IsRegular() || !ValidAttachmentName(de.Name()) {
			continue
		}
		info, err := de.Info()
		if err != nil || now.Sub(info.ModTime()) <= maxAge {
			continue
		}
		old = append(old, de.Name())
	}
	if len(old) == 0 {
		return
	}
	referenced := referencedAttachments(dir)
	for _, n := range keep {
		referenced[n] = true
	}
	for _, n := range old {
		if !referenced[n] {
			_ = os.Remove(filepath.Join(attachDir, n))
		}
	}
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
