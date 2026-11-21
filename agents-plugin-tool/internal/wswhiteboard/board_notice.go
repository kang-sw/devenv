package wswhiteboard

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsstate"
)

type noticeCoverage struct {
	TopicDir string `json:"topic_dir"`
	RunID    string `json:"run_id"`
	ETAEntry string `json:"eta_entry"`
}

type boardNotice struct {
	V           int              `json:"v"`
	ReplyID     string           `json:"reply_id"`
	SentAt      string           `json:"sent_at"`
	Reasons     []string         `json:"reasons"`
	HourlyEpoch string           `json:"hourly_epoch,omitempty"`
	Revision    string           `json:"revision,omitempty"`
	Covered     []noticeCoverage `json:"covered"`
}

type noticePlan struct {
	receipt        boardNotice
	topic, message string
	next           time.Time
}

func eligibleState(v TopicStateView) bool {
	return v.State != nil && (v.State.Value == StateImplementing || v.State.Value == StateWaiting)
}

// Epochs follow numeric global history, including closed topics. Neither topic
// order, equal timestamps nor late imported recaps can establish an epoch.
func (s *State) hourlyEpoch() (string, time.Time) {
	views := map[string]TopicStateView{}
	id := ""
	var at time.Time
	count := 0
	for _, event := range s.stateEvents() {
		was := eligibleState(views[event.topic])
		foldStateEvent(views, event)
		is := eligibleState(views[event.topic])
		if was && !is {
			count--
		}
		if !was && is {
			count++
			if count == 1 {
				id = event.entry.ID
				at, _ = time.Parse(time.RFC3339Nano, event.entry.At)
			}
		}
		if count == 0 {
			id, at = "", time.Time{}
		}
	}
	return id, at
}

func parseBoardNotice(e Entry, reply string) (boardNotice, bool) {
	var n boardNotice
	if json.Unmarshal(e.BoardNotice, &n) != nil || n.V != 1 || n.ReplyID != reply || len(n.Reasons) == 0 {
		return n, false
	}
	at, err := time.Parse(time.RFC3339Nano, n.SentAt)
	if canonical, ok := stateTimestamp(at); err != nil || !ok || canonical != n.SentAt || n.SentAt != e.At {
		return n, false
	}
	for i, reason := range n.Reasons {
		if reason != "hourly" && reason != "overdue" || slices.Contains(n.Reasons[:i], reason) {
			return n, false
		}
	}
	if slices.Contains(n.Reasons, "hourly") && folderNumber(n.HourlyEpoch, "e") == 0 {
		return n, false
	}
	for _, c := range n.Covered {
		if !folderDirID(c.TopicDir) || folderNumber(c.RunID, "e") == 0 || folderNumber(c.ETAEntry, "e") == 0 {
			return n, false
		}
	}
	return n, true
}

func sooner(a, b time.Time) time.Time {
	if a.IsZero() || !b.IsZero() && b.Before(a) {
		return b
	}
	return a
}

func (s *State) planNotice(reply string, dirs map[string]string, now time.Time) noticePlan {
	p := noticePlan{receipt: boardNotice{V: 1, ReplyID: reply, Revision: s.boardRevision(), Covered: []noticeCoverage{}}}
	if _, ok := stateTimestamp(now); !ok {
		return p
	}
	views := s.TopicStates()
	epoch, activation := s.hourlyEpoch()
	// A matching older receipt is not evidence that the current revision was
	// notified: authority can return to an earlier fingerprint after a revert.
	// Retain only the newest receipt for each covered run and hourly epoch.
	type coveredReceipt struct {
		id       int
		revision string
	}
	covered := map[noticeCoverage]coveredReceipt{}
	var hourly time.Time
	var hourlyID int
	hourlyRevision := ""
	future := false
	for _, event := range s.stateEvents() {
		n, ok := parseBoardNotice(event.entry, reply)
		if !ok {
			continue
		}
		sent, _ := time.Parse(time.RFC3339Nano, n.SentAt)
		if sent.After(now) {
			future = true
		}
		// Every full overdue section covers its entries, including hourly-only sends.
		id := folderNumber(event.entry.ID, "e")
		for _, c := range n.Covered {
			if id > covered[c].id {
				covered[c] = coveredReceipt{id, n.Revision}
			}
		}
		if n.HourlyEpoch == epoch && slices.Contains(n.Reasons, "hourly") {
			if sent.After(hourly) {
				hourly = sent
			}
			if id > hourlyID {
				hourlyID, hourlyRevision = id, n.Revision
			}
		}
	}
	topics := make([]string, 0, len(dirs))
	for topic := range dirs {
		if !s.topicClosed(topic) {
			topics = append(topics, topic)
		}
	}
	sort.Slice(topics, func(i, j int) bool { return dirs[topics[i]] < dirs[topics[j]] })
	var overdue []string
	trigger, eligible := false, false
	for _, topic := range topics {
		v := views[topic]
		if !eligibleState(v) {
			continue
		}
		eligible = true
		if p.topic == "" {
			p.topic = topic
		}
		if v.State.Value != StateImplementing {
			continue
		}
		run := v.State.Run
		deadline, _ := time.Parse(time.RFC3339Nano, run.DeadlineAt)
		start, _ := time.Parse(time.RFC3339Nano, run.StartedAt)
		if start.After(now) {
			future = true
		}
		if now.Before(deadline) {
			p.next = sooner(p.next, deadline)
			continue
		}
		overdue = append(overdue, topic)
		c := noticeCoverage{dirs[topic], run.ID, v.State.Entry}
		p.receipt.Covered = append(p.receipt.Covered, c)
		if covered[c].id == 0 || covered[c].revision != p.receipt.Revision {
			p.next = sooner(p.next, now)
			trigger = true
		}
	}
	if trigger {
		p.receipt.Reasons = append(p.receipt.Reasons, "overdue")
	}
	if eligible && epoch != "" && (hourlyID == 0 || hourlyRevision != p.receipt.Revision) {
		due := activation.Add(time.Hour)
		if !hourly.IsZero() {
			due = hourly.Add(time.Hour)
		}
		p.next = sooner(p.next, due)
		if !now.Before(due) {
			p.receipt.Reasons = append(p.receipt.Reasons, "hourly")
			p.receipt.HourlyEpoch = epoch
		}
	}
	if len(p.receipt.Reasons) == 0 {
		return p
	}
	p.receipt.SentAt, _ = stateTimestamp(now)
	var b strings.Builder
	fmt.Fprintf(&b, "Whiteboard lead nudge (%s).\n", strings.Join(p.receipt.Reasons, ", "))
	if slices.Contains(p.receipt.Reasons, "hourly") {
		b.WriteString("Open topics:\n")
		for _, topic := range topics {
			value := "absent"
			if v := views[topic]; v.State != nil {
				value = v.State.Value
			} else if v.Diagnostic != nil {
				value = "unknown (" + v.Diagnostic.Code + ")"
			}
			fmt.Fprintf(&b, "- %s: %s\n", s.TopicLabel(topic), value)
		}
	}
	b.WriteString("Overdue topics:\n")
	if len(overdue) == 0 {
		b.WriteString("- none\n")
	}
	for _, topic := range overdue {
		run := views[topic].State.Run
		fmt.Fprintf(&b, "- %s; owner %s; initial/current ETA %d/%d minutes; elapsed %d ms; extensions %d\n", s.TopicLabel(topic), run.Owner, run.InitialETAMinutes, run.ETAMinutes, runElapsed(run, now), run.ExtensionCount)
		for _, e := range s.topicDoc(topic).Entries {
			if e.Kind != EntrySystem || e.Imported || e.ItemID != 0 || len(e.TopicState) == 0 {
				continue
			}
			r, code := parseTopicState(e)
			if code == "" && r.Op == "extend" && r.Run.ID == run.ID {
				fmt.Fprintf(&b, "  - extension to %d minutes: %s\n", r.Run.ETAMinutes, r.Reason)
			}
		}
	}
	if future {
		b.WriteString("Clock diagnostic: future stored timestamp; elapsed is clamped at zero and notification clocks are not advanced early.\n")
	}
	b.WriteString("Reconcile declared work state and ETA with actual progress; keep unfinished, unworked topics actionable for the owner.")
	p.message = b.String()
	return p
}

// Notify replays authority and decides, enqueues, wakes, then records under the
// existing board lock. Callbacks MUST be low-level: no session mutex, key write,
// nested board open, render or delivery wait. eligible re-reads direct process/
// identity evidence immediately before enqueue. The caller resolves paths first.
// The optional ticket resolver also runs under this lock and must do bounded,
// read-only lookup without acquiring session or board locks.
// PID reuse is not authenticated; exit/stop can race the enqueue. Already queued
// mail retains the transport's ordinary wake behavior, not a delivery-time gate.
func (f FolderStore) Notify(reply string, now time.Time, eligible func() bool, enqueue func(string) error, wake func(), tickets ...TicketResolver) (next time.Time, err error) {
	present := false
	for _, name := range []string{"meta.json", "pending.json", "committed.json"} {
		if _, statErr := wsstate.Stat(filepath.Join(f.Dir, name)); statErr == nil {
			present = true
			break
		} else if !os.IsNotExist(statErr) {
			return next, statErr
		}
	}
	if !present {
		return next, nil
	}
	_, _, err = SharedFolderCache.openRevision(f, nil, func(before *State, meta *folderSession) error {
		if before == nil || meta == nil {
			return nil
		}
		dirs := map[string]string{}
		for _, ref := range meta.Topics {
			dirs[ref.Key] = ref.Dir
		}
		work := before.clone()
		var changed bool
		next, changed, err = mutateBoardNotice(work, dirs, reply, now, eligible, enqueue, wake, f.hit, tickets...)
		if err != nil || !changed {
			return err
		}
		intent, err := f.plan(meta, before, work)
		if err != nil {
			return err
		}
		if err := f.commit(intent); err != nil {
			return err
		}
		next = work.planNotice(reply, dirs, now).next
		return nil
	})
	return next, err
}

func (b *BoardDB) Notify(reply string, now time.Time, eligible func() bool, enqueue func(string) error, wake func(), tickets ...TicketResolver) (next time.Time, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return next, fmt.Errorf("whiteboard board DB is closed")
	}
	err = b.updateLocked(func(work *State) error {
		var changed bool
		next, changed, err = mutateBoardNotice(work, b.topicDirs, reply, now, eligible, enqueue, wake, b.hit, tickets...)
		if err == nil && changed {
			next = work.planNotice(reply, b.topicDirs, now).next
		}
		return err
	})
	return next, err
}

func mutateBoardNotice(work *State, dirs map[string]string, reply string, now time.Time, eligible func() bool, enqueue func(string) error, wake func(), hit func(string) error, tickets ...TicketResolver) (next time.Time, changed bool, err error) {
	p := work.planNotice(reply, dirs, now)
	next = p.next
	var resolver TicketResolver
	if len(tickets) > 0 {
		resolver = tickets[0]
	}
	unblocked := work.planUnblocked(reply, dirs, resolver)
	send := func(message string) (bool, error) {
		if message == "" || !eligible() {
			return false, nil
		}
		if err := enqueue(message); err != nil {
			return false, err
		}
		wake()
		return true, hit("notice-enqueued")
	}
	if sent, e := send(p.message); e != nil {
		return next, false, e
	} else if sent {
		data, e := json.Marshal(p.receipt)
		if e != nil {
			return next, false, e
		}
		work.appendEntry(p.topic, Entry{Kind: EntrySystem, Text: "Lead nudge sent: " + strings.Join(p.receipt.Reasons, ", "), BoardNotice: data}, now)
		changed = true
	}
	for _, ready := range unblocked {
		if sent, e := send(ready.message); e != nil {
			return next, false, e
		} else if sent {
			work.appendUnblocked(ready, now)
			changed = true
		}
	}
	return next, changed, nil
}
