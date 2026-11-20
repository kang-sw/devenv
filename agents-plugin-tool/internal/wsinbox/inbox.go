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
	"errors"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
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
