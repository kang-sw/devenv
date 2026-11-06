package mcp

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kang-sw/devenv/internal/wsstate"
)

// mailbox_history.go keeps each session's history ring: the last
// mailboxHistoryCap messages it sent or drained, full bodies included, so
// mail a conversation lost (a tangled session, a mis-aimed rewind) can be
// recovered with mailbox.history.
//
// Storage is one file per entry under
// <cache root>/mailbox-history/<session-key>/, named
// <fixed-width UTC nanosecond stamp>-<random hex>.json. Every entry gets a
// fresh name through temp+rename, so concurrent writers to one session key
// (several MCP processes can hold the same key) never replace each other's
// file and need no lock, and a reader never sees a partial entry. The ring
// stays off the session record, which more than one process writes without a
// cross-process lock, and off the machine reply-id registry, which every
// peer's send locks.

const (
	// mailboxHistoryCap is the ring's capacity, received and sent together.
	mailboxHistoryCap = 10
	// mailboxHistoryTitleMax bounds a title in runes; a longer first line is
	// cut there with an ellipsis.
	mailboxHistoryTitleMax = 120
	// mailboxHistoryDirName is the cache-root sibling of keys/ that holds one
	// directory per session key, so no enumerator of keys/ sees it.
	mailboxHistoryDirName = "mailbox-history"
	// mailboxHistoryStampLayout is the entry file-name stamp: fixed width with
	// nanoseconds, so names sort lexically in time order. The RFC3339 seconds
	// stamp the mailbox uses elsewhere is too coarse to order one drain.
	mailboxHistoryStampLayout = "20060102T150405.000000000Z"

	mailboxHistoryRecv = "recv"
	mailboxHistorySent = "sent"
)

// mailboxHistoryEntry is the on-disk entry file. Its time is not stored: the
// file-name stamp is the single source of the exchange time.
type mailboxHistoryEntry struct {
	Direction    string `json:"direction"`
	Counterparty string `json:"counterparty"`
	Body         string `json:"body"`
}

// mailboxHistoryRecord is a read entry: the stored fields plus the exchange
// time (RFC3339 UTC, from the file-name stamp) and the derived title.
type mailboxHistoryRecord struct {
	mailboxHistoryEntry
	Time  string
	Title string
}

// mailboxHistoryClock makes entry stamps strictly increasing within this
// process, so the entries of one drain keep their order even when the clock
// does not advance between them. Ties across processes order arbitrarily.
var mailboxHistoryClock struct {
	sync.Mutex
	last time.Time
}

func nextMailboxHistoryStamp() time.Time {
	mailboxHistoryClock.Lock()
	defer mailboxHistoryClock.Unlock()
	now := mailboxNow().UTC()
	if !now.After(mailboxHistoryClock.last) {
		now = mailboxHistoryClock.last.Add(time.Nanosecond)
	}
	mailboxHistoryClock.last = now
	return now
}

// mailboxHistoryRoot is <cache root>/mailbox-history; it is not created.
func mailboxHistoryRoot() (string, error) {
	cacheRoot, err := wsstate.CacheRoot(wsstate.Options{})
	if err != nil {
		return "", err
	}
	return filepath.Join(cacheRoot, mailboxHistoryDirName), nil
}

func mailboxHistoryDir(sessionKey string) (string, error) {
	root, err := mailboxHistoryRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, sessionKey), nil
}

// isMailboxHistoryEntryName reports whether name is a committed entry file,
// excluding in-flight temp files (dot-prefixed, no .json suffix).
func isMailboxHistoryEntryName(name string) bool {
	return !strings.HasPrefix(name, ".") && strings.HasSuffix(name, ".json")
}

// appendMailboxHistory writes entries, in order, to sessionKey's ring and then
// trims it to the newest mailboxHistoryCap. It returns the first write error
// and stops there; callers log it and never surface it, because the send has
// already been enqueued or the mail already drained. Trimming is best effort.
func appendMailboxHistory(sessionKey string, entries []mailboxHistoryEntry) error {
	if len(entries) == 0 {
		return nil
	}
	dir, err := mailboxHistoryDir(sessionKey)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create mailbox history dir: %w", err)
	}
	var werr error
	for _, entry := range entries {
		if werr = writeMailboxHistoryEntry(dir, entry); werr != nil {
			break
		}
	}
	trimMailboxHistory(dir)
	return werr
}

func writeMailboxHistoryEntry(dir string, entry mailboxHistoryEntry) error {
	raw, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	var suffix [4]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return err
	}
	name := nextMailboxHistoryStamp().Format(mailboxHistoryStampLayout) + "-" + hex.EncodeToString(suffix[:]) + ".json"

	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("create mailbox history temp file: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write mailbox history entry: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("write mailbox history entry: %w", err)
	}
	if err := os.Rename(tmpPath, filepath.Join(dir, name)); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("commit mailbox history entry: %w", err)
	}
	return nil
}

// trimMailboxHistory removes all but the newest mailboxHistoryCap entries,
// ignoring every error: a concurrent trim or a failed remove only leaves
// extra entries until the next write.
func trimMailboxHistory(dir string) {
	names, err := mailboxHistoryEntryNames(dir)
	if err != nil || len(names) <= mailboxHistoryCap {
		return
	}
	for _, name := range names[mailboxHistoryCap:] {
		_ = os.Remove(filepath.Join(dir, name))
	}
}

// mailboxHistoryEntryNames lists dir's committed entry names, newest first.
func mailboxHistoryEntryNames(dir string) ([]string, error) {
	dirEntries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(dirEntries))
	for _, de := range dirEntries {
		if isMailboxHistoryEntryName(de.Name()) {
			names = append(names, de.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	return names, nil
}

// readMailboxHistory returns sessionKey's ring, newest first. A missing
// directory is an empty ring; an entry that cannot be read or parsed, or
// whose name carries no valid stamp, is skipped.
func readMailboxHistory(sessionKey string) ([]mailboxHistoryRecord, error) {
	dir, err := mailboxHistoryDir(sessionKey)
	if err != nil {
		return nil, err
	}
	names, err := mailboxHistoryEntryNames(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	records := make([]mailboxHistoryRecord, 0, len(names))
	for _, name := range names {
		stamp, _, ok := strings.Cut(name, "-")
		if !ok {
			continue
		}
		at, perr := time.Parse(mailboxHistoryStampLayout, stamp)
		if perr != nil {
			continue
		}
		raw, rerr := os.ReadFile(filepath.Join(dir, name))
		if rerr != nil {
			continue
		}
		var entry mailboxHistoryEntry
		if json.Unmarshal(raw, &entry) != nil {
			continue
		}
		records = append(records, mailboxHistoryRecord{
			mailboxHistoryEntry: entry,
			Time:                at.UTC().Format(time.RFC3339),
			Title:               mailboxHistoryTitle(entry.Body),
		})
	}
	return records, nil
}

// mailboxHistoryTitle is the one title rule for a ring entry: the first line
// the sender wrote. A leading peer-lead note line is skipped, an empty body
// gives an empty title, and a first line longer than mailboxHistoryTitleMax
// runes is cut there with an ellipsis.
func mailboxHistoryTitle(body string) string {
	if hasPeerLeadNote(body) {
		_, rest, ok := strings.Cut(body, "\n")
		if !ok {
			return ""
		}
		body = rest
	}
	line, _, _ := strings.Cut(body, "\n")
	line = strings.TrimSuffix(line, "\r")
	if runes := []rune(line); len(runes) > mailboxHistoryTitleMax {
		return string(runes[:mailboxHistoryTitleMax]) + "…"
	}
	return line
}

// recordMailboxHistory appends entries to sessionKey's ring, logging a failed
// write rather than surfacing it.
func recordMailboxHistory(sessionKey string, entries []mailboxHistoryEntry) {
	if err := appendMailboxHistory(sessionKey, entries); err != nil {
		appendDebugEvent("mailbox.history_write_error", map[string]any{"error": err.Error()})
	}
}

// mailboxHistoryOut is mailbox.history's per-entry JSON shape. Body is a
// pointer so body:false omits it while body:true keeps an empty body.
type mailboxHistoryOut struct {
	Direction    string  `json:"direction"`
	Counterparty string  `json:"counterparty"`
	Time         string  `json:"time"`
	Title        string  `json:"title"`
	Body         *string `json:"body,omitempty"`
}

func (s *Server) handleMailboxHistory(id json.RawMessage, args map[string]any) response {
	const tool = "mailbox.history"
	sessionKey, err := sessionStateKey(tool, args)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	if _, found := s.sessions.lookup(sessionKey); !found {
		return toolTextResponse(id, "", mailboxUnknownSessionError(tool))
	}
	limit := intFromArgument(args["limit"], mailboxHistoryCap)
	withBody := boolArgument(args["body"])
	direction, _ := args["direction"].(string)
	direction = strings.TrimSpace(direction)
	switch direction {
	case "", "all":
		direction = ""
	case mailboxHistoryRecv, mailboxHistorySent:
	default:
		return toolTextResponse(id, "", fmt.Errorf(`%s: direction must be "recv", "sent", or "all"`, tool))
	}

	records, err := readMailboxHistory(sessionKey)
	if err != nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: %w", tool, err))
	}
	out := []mailboxHistoryOut{}
	for _, r := range records {
		if len(out) == limit {
			break
		}
		if direction != "" && r.Direction != direction {
			continue
		}
		item := mailboxHistoryOut{Direction: r.Direction, Counterparty: r.Counterparty, Time: r.Time, Title: r.Title}
		if withBody {
			body := r.Body
			item.Body = &body
		}
		out = append(out, item)
	}

	if wantsJSON(args) {
		return toolJSONResponse(id, out, nil)
	}
	if len(out) == 0 {
		return toolTextResponse(id, "no mailbox history\n", nil)
	}
	var b strings.Builder
	for _, item := range out {
		preposition := "from"
		if item.Direction == mailboxHistorySent {
			preposition = "to"
		}
		fmt.Fprintf(&b, "[%s] %s %s %s: %s\n", item.Time, item.Direction, preposition, item.Counterparty, item.Title)
		if item.Body != nil {
			b.WriteString(*item.Body)
			if !strings.HasSuffix(*item.Body, "\n") {
				b.WriteString("\n")
			}
			b.WriteString("---\n")
		}
	}
	return toolTextResponse(id, b.String(), nil)
}
