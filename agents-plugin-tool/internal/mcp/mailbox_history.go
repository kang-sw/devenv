package mcp

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsmailbox"
	"github.com/kang-sw/devenv/internal/wsstate"
)

// mailbox_history.go serves each session's history window: the last
// mailboxHistoryCap messages it sent or drained, full bodies included, so
// mail a conversation lost (a tangled session, a mis-aimed rewind) can be
// recovered with mailbox.history. The window lives in the memory-first
// mailbox store (wsmailbox.Store.History); a send or a recv appends to it in
// the same transaction that moves the mail (Tx.AppendHistory).

const (
	// mailboxHistoryCap is the window's capacity, received and sent together.
	mailboxHistoryCap = wsmailbox.HistoryCap
	// mailboxHistoryTitleMax bounds a title in runes; a longer first line is
	// cut there with an ellipsis.
	mailboxHistoryTitleMax = 120

	mailboxHistoryRecv = wsmailbox.HistoryRecv
	mailboxHistorySent = wsmailbox.HistorySent
)

// mailboxHistoryEntry is one entry to append. Its time is not stored: the
// entry name's stamp is the single source of the exchange time.
type mailboxHistoryEntry struct {
	Direction    string
	Counterparty string
	Body         string
}

// mailboxHistoryRecord is a read entry: the stored fields plus the exchange
// time (RFC3339 UTC, from the name's stamp), the derived title, and the
// entry's name (the key a conversation ring mail line carries).
type mailboxHistoryRecord struct {
	mailboxHistoryEntry
	Time  string
	Title string
	Name  string
}

// mailboxHistoryStore is the mailbox store of the cache home cacheHome
// (empty: this process's).
func mailboxHistoryStore(cacheHome string) wsmailbox.Store {
	return wsmailbox.Paths{State: wsstate.Options{CacheHome: cacheHome}}.Store()
}

// appendMailboxHistoryTx appends entries, in order, to sessionKey's window
// inside tx, stamped from mailboxNow, and returns their names.
func appendMailboxHistoryTx(tx *wsmailbox.Tx, sessionKey string, entries []mailboxHistoryEntry) ([]string, error) {
	if len(entries) == 0 {
		return nil, nil
	}
	out := make([]wsmailbox.HistoryEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, wsmailbox.HistoryEntry{Direction: e.Direction, Counterparty: e.Counterparty, Body: e.Body})
	}
	return tx.AppendHistory(sessionKey, mailboxNow(), out...)
}

// appendMailboxHistory appends entries to sessionKey's window in a
// transaction of its own and returns their names.
func appendMailboxHistory(cacheHome, sessionKey string, entries []mailboxHistoryEntry) ([]string, error) {
	var names []string
	err := mailboxHistoryStore(cacheHome).Update("history", func(tx *wsmailbox.Tx) error {
		var err error
		names, err = appendMailboxHistoryTx(tx, sessionKey, entries)
		return err
	})
	if err != nil {
		return nil, err
	}
	return names, nil
}

// readMailboxHistory returns sessionKey's window, newest first. An entry
// whose name carries no valid stamp is skipped.
func readMailboxHistory(cacheHome, sessionKey string) ([]mailboxHistoryRecord, error) {
	entries, err := mailboxHistoryStore(cacheHome).History(sessionKey)
	if err != nil {
		return nil, err
	}
	records := make([]mailboxHistoryRecord, 0, len(entries))
	for _, e := range entries {
		if rec, ok := mailboxHistoryRecordOf(e); ok {
			records = append(records, rec)
		}
	}
	return records, nil
}

func mailboxHistoryRecordOf(e wsmailbox.HistoryEntry) (mailboxHistoryRecord, bool) {
	at, ok := wsmailbox.HistoryTime(e.Name)
	if !ok {
		return mailboxHistoryRecord{}, false
	}
	return mailboxHistoryRecord{
		mailboxHistoryEntry: mailboxHistoryEntry{Direction: e.Direction, Counterparty: e.Counterparty, Body: e.Body},
		Time:                at.Format(time.RFC3339),
		Title:               mailboxHistoryTitle(e.Body),
		Name:                e.Name,
	}, true
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
	if _, found := s.lookupSession(sessionKey); !found {
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

	records, err := readMailboxHistory(s.getenv(wsstate.EnvCacheHome), sessionKey)
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
