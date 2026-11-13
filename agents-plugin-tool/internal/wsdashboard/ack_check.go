package wsdashboard

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kang-sw/devenv/internal/wsinbox"
	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// The answer check: once a session's turn settles, the daemon compares the
// board answers the session was sent with its board items' status, and when
// an answered item is still open it holds one nudge message in the session's
// board-answer inbox naming those items, asking the agent to record them.
//
// "Settled" is the conversation ring's newest turn record (newestTurn,
// turnStart), the same rule the page's note-only sent marks clear by
// (sentMarks): a chip sent before that turn's start was in the inbox when
// the turn began, and both adapters drain the inbox as a turn starts. The
// batch of one check is every value chip (an answer, with or without a
// note; a note-only chip never counts) of the drained records sent in
// [through, start), less the chips the re-ask rule drops; through is the
// previous checked turn's start, kept in ackCheckFileName in the session's
// registry directory. A check that finds a non-empty batch moves through to
// the turn's start whether or not it nudges, so each chip is checked at one
// settle, and the nudge itself carries no chip, so it is never a batch and
// a nudge never loops.
//
// The check runs in the daemon, on a coalescing per-session timer
// (ackChecker) poked by every session change, agent runner write, hosted
// snapshot change, and hosted ring append, so it needs no open page and two
// pages never nudge twice. Its gates, cheapest first: the source reads board
// items (BoardItemsSource); the session is indexed, has a board, and is not
// an agent conversation; its host adapter is live; the lead is idle (no
// turn, phase idle, no permission wait); the ring holds a turn with a
// duration whose end reason is not aborted (a terminal stop); that turn
// started after the latest dashboard interrupt (interruptedAt); and the
// inbox holds no message, since held input means another turn is coming and
// the batch rolls into its settle. The first check of a session (no
// through) seeds through with the settled turn's start and sends nothing, so
// answers from before the feature are never nudged.
//
// Reads run outside d.inboxMu; under it the check re-reads the held
// messages and the record, and writes the record and sends only when the
// record is unchanged since its unlocked read, so two checks never both
// send. The record is written before the send: a crash between them loses a
// nudge, never doubles one.

const (
	// ackCheckFileName is the answer check's record in a session's registry
	// directory, beside labelFileName. An older daemon leaves it alone, and
	// the registry prunes it with the session's directory.
	ackCheckFileName = "ack-check.json"
	// ackCheckDelay is how long a poke waits before its check runs; the pokes
	// in between join it.
	ackCheckDelay = time.Second
	// ackNudgeCap bounds the item IDs one nudge names.
	ackNudgeCap = 20
)

// ackNudgeFormat is the nudge message, sent to the agent only; %s is the
// item IDs (ackNudgeText).
const ackNudgeFormat = "Dashboard answer check: the owner's answers to %s reached you, but those board items are still open. Record each answer you acted on with wb:answer (the owner's words as its body); if an item should stay open, say why. This check runs once per answer batch."

// ackCheckRecord is ackCheckFileName's content. Readers ignore unknown
// fields.
type ackCheckRecord struct {
	// Through is the start (epoch ms) of the latest settled turn checked: a
	// chip sent before it was checked already. Zero (absent) before the
	// session's first check.
	Through int64 `json:"through,omitempty"`
	// InterruptedAt is the latest dashboard interrupt's time (epoch ms): a
	// settled turn that started at or before it was stopped by the owner,
	// and its check waits for the next turn.
	InterruptedAt int64 `json:"interruptedAt,omitempty"`
}

// readAckCheck reads the answer check record in sessionDir; a missing,
// unreadable, or malformed file reads as none.
func readAckCheck(sessionDir string) ackCheckRecord {
	var rec ackCheckRecord
	raw, err := os.ReadFile(filepath.Join(sessionDir, ackCheckFileName))
	if err != nil || json.Unmarshal(raw, &rec) != nil {
		return ackCheckRecord{}
	}
	return rec
}

// writeAckCheck replaces the answer check record in sessionDir atomically
// (temp file and rename).
func writeAckCheck(sessionDir string, rec ackCheckRecord) error {
	raw, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	return writePrivateFile(sessionDir, ackCheckFileName, append(raw, '\n'))
}

// recordInterrupt notes a dashboard interrupt of the session whose inbox is
// inbox at now (ackCheckRecord.InterruptedAt); d.inboxMu is held. A failure
// is logged: the turn's check then runs as if it ended normally.
func (d *Daemon) recordInterrupt(inbox string, now time.Time) {
	sessionDir := filepath.Dir(inbox)
	rec := readAckCheck(sessionDir)
	rec.InterruptedAt = now.UnixMilli()
	if err := writeAckCheck(sessionDir, rec); err != nil {
		fmt.Fprintf(d.log, "dashboard: record interrupt %s: %v\n", filepath.Base(sessionDir), err)
	}
}

// ackBatch is the item IDs, ascending, of the value chips of drained sent
// in [through, settled) that the re-ask rule keeps (a chip sent before its
// item's latest re-ask in reasked answered an earlier ask). A note-only
// chip, and a chip without a send time, never count.
func ackBatch(drained []wsinbox.Record, reasked map[int]int64, through, settled int64) []int {
	set := map[int]bool{}
	for _, r := range drained {
		for _, c := range r.Entry.Chips {
			if c.Answer == "" || c.SentAt <= 0 || c.SentAt < through || c.SentAt >= settled {
				continue
			}
			id, err := strconv.Atoi(c.ID)
			if err != nil {
				continue
			}
			if at, ok := reasked[id]; ok && c.SentAt < at {
				continue
			}
			set[id] = true
		}
	}
	ids := make([]int, 0, len(set))
	for id := range set {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

// ackNudgeIDs is the batch's items (ackBatch) the board still holds open:
// the ones a nudge names, ascending. An item the board no longer holds, or
// holds answered, assumed, withdrawn, or superseded, is handled.
func ackNudgeIDs(drained []wsinbox.Record, items BoardItems, through, settled int64) []int {
	var out []int
	for _, id := range ackBatch(drained, items.Reasked, through, settled) {
		if items.Open[id] {
			out = append(out, id)
		}
	}
	return out
}

// ackNudgeText is the nudge naming ids, ascending, at most ackNudgeCap of
// them.
func ackNudgeText(ids []int) string {
	var b strings.Builder
	for i, id := range ids {
		if i == ackNudgeCap {
			b.WriteString(", ...")
			break
		}
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(wswhiteboard.FormatID(id))
	}
	return fmt.Sprintf(ackNudgeFormat, b.String())
}

// leadIdle reports whether a snapshot's lead is between turns: no turn
// running, its phase idle, and no permission wait.
func leadIdle(lead wslive.Lead) bool {
	return lead.TurnStartedAt == 0 && (lead.Phase == "" || lead.Phase == wslive.PhaseIdle) && lead.Permission == nil
}

// holdsMessages reports whether the inbox holds a message (not an interrupt
// entry); an unreadable inbox reads as holding one, so the check waits.
func holdsMessages(inbox string) bool {
	names, err := wsinbox.MessageNames(inbox)
	return err != nil || len(names) > 0
}

// checkAnswers runs the answer check of session replyID once.
func (d *Daemon) checkAnswers(replyID string) {
	if d.index == nil {
		return
	}
	src, ok := d.source.(agentSessions)
	if !ok || !src.readsBoardItems() {
		return
	}
	s, ok := d.index.Session(replyID)
	if !ok || s.Agent || !s.HasBoard {
		return
	}
	snap := d.pages().readSnapshot(replyID)
	if !adapterPresent(snap, time.Now()) || !leadIdle(snap.Lead) {
		return
	}
	sessionDir := SessionDir(d.dir, replyID)
	ring, _ := d.entryFiles.readRing(replyID, wslive.RingDir(sessionDir))
	turn, ok := newestTurn(ring)
	if !ok || turn.Reason == wslive.TurnAborted {
		return
	}
	start, ok := turnStart(turn)
	if !ok {
		return
	}
	inbox := sessionInbox(d.dir, replyID)
	if holdsMessages(inbox) {
		return
	}
	rec := readAckCheck(sessionDir)
	if start <= rec.InterruptedAt {
		return
	}
	if rec.Through == 0 {
		d.commitAnswerCheck(s, sessionDir, inbox, rec, start, nil)
		return
	}
	if start <= rec.Through {
		return
	}
	drained, _ := d.entryFiles.readDrained(replyID, inbox)
	// The re-ask rule only drops chips, so a batch empty before the board
	// read stays empty: the board is read only for a batch.
	if len(ackBatch(drained, nil, rec.Through, start)) == 0 {
		return
	}
	items, ok := src.BoardItems(s.Registration)
	if !ok {
		return
	}
	d.commitAnswerCheck(s, sessionDir, inbox, rec, start, ackNudgeIDs(drained, items, rec.Through, start))
}

// commitAnswerCheck moves the session's record from read to through start
// and, when ids is non-empty, holds the nudge naming them, under
// d.inboxMu; it does neither when a message is held now or the record
// changed since read (another check, or an interrupt, ran in between).
func (d *Daemon) commitAnswerCheck(s Session, sessionDir, inbox string, read ackCheckRecord, start int64, ids []int) {
	d.inboxMu.Lock()
	if holdsMessages(inbox) || readAckCheck(sessionDir) != read {
		d.inboxMu.Unlock()
		return
	}
	if err := writeAckCheck(sessionDir, ackCheckRecord{Through: start, InterruptedAt: read.InterruptedAt}); err != nil {
		d.inboxMu.Unlock()
		fmt.Fprintf(d.log, "dashboard: answer check %s: %v\n", s.ReplyID, err)
		return
	}
	sent := false
	if len(ids) > 0 {
		if _, _, err := wsinbox.Send(inbox, wsinbox.Entry{Parts: []string{ackNudgeText(ids)}}, time.Now()); err != nil {
			fmt.Fprintf(d.log, "dashboard: answer check %s: hold the nudge: %v\n", s.ReplyID, err)
		} else {
			sent = true
		}
	}
	d.inboxMu.Unlock()
	if sent {
		d.inboxChanged(s)
	}
}

// ackChecker runs each session's answer check once its pokes go quiet for
// its delay: a poke arms the session's timer unless one is armed, so a
// burst of changes costs one check.
type ackChecker struct {
	delay time.Duration
	run   func(replyID string)

	mu     sync.Mutex
	timers map[string]*time.Timer
	closed bool
	// wg counts the armed timers and running checks, which close waits for.
	wg sync.WaitGroup
}

// poke schedules a check of replyID. A nil checker, or a closed one, does
// nothing.
func (c *ackChecker) poke(replyID string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.timers[replyID] != nil {
		return
	}
	if c.timers == nil {
		c.timers = map[string]*time.Timer{}
	}
	c.wg.Add(1)
	c.timers[replyID] = time.AfterFunc(c.delay, func() {
		defer c.wg.Done()
		c.mu.Lock()
		delete(c.timers, replyID)
		closed := c.closed
		c.mu.Unlock()
		if !closed {
			c.run(replyID)
		}
	})
}

// close stops the armed timers and waits for a running check to finish.
func (c *ackChecker) close() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.closed = true
	for id, t := range c.timers {
		if t.Stop() {
			c.wg.Done()
		}
		delete(c.timers, id)
	}
	c.mu.Unlock()
	c.wg.Wait()
}
