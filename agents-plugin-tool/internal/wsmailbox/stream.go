package wsmailbox

import "time"

// DefaultStreamReminder bounds retries of arrivals an adapter failed to drain.
const DefaultStreamReminder = 10 * time.Minute

// Stream peeks for the lifetime of Done, emitting arrivals without draining.
// Timeout is a reminder interval, not a lifetime: unchanged pending items are
// reported again at that interval (zero uses DefaultStreamReminder). New items
// are reported immediately, without postponing the next pending reminder.
// Each emission contains only new items, or all pending items on a reminder.
func Stream(target WaitTarget, opts WaitOptions, emit func(WaitResult) error) error {
	resolved, err := resolveWaitTarget(target)
	if err != nil {
		return err
	}
	return streamLoop(resolved.peek, opts, emit)
}

func streamLoop(peek func() (WaitResult, error), opts WaitOptions, emit func(WaitResult) error) error {
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	reminder := opts.Timeout
	if reminder <= 0 {
		reminder = DefaultStreamReminder
	}
	opts.Timeout = 0
	if opts.Poll <= 0 {
		opts.Poll = DefaultWaitPoll
		if opts.Wake != nil {
			opts.Poll = WakeBackstopPoll
		}
	}
	if opts.Poll > reminder {
		opts.Poll = reminder
	}
	var previous WaitResult
	var remindAt time.Time
	_, err := waitLoop(func() (WaitResult, error) {
		pending, err := peek()
		if err != nil {
			return WaitResult{}, err
		}
		arrival := WaitResult{
			Named:     newPending(pending.Named, previous.Named),
			Reply:     newPending(pending.Reply, previous.Reply),
			Board:     len(newPending(pending.boardNames, previous.boardNames)),
			Interrupt: len(newPending(pending.interruptNames, previous.interruptNames)),
		}
		previous = pending
		t := now()
		if !pending.Woke() {
			remindAt = time.Time{}
		} else if remindAt.IsZero() || !t.Before(remindAt) {
			arrival = pending
			remindAt = t.Add(reminder)
		}
		if arrival.Woke() {
			if err := emit(arrival); err != nil {
				return WaitResult{}, err
			}
		}
		// Always block after a peek, even when mail remains undrained. Calling
		// level-triggered Wait repeatedly here would spin on that same mail.
		return WaitResult{}, nil
	}, opts)
	return err
}

// Envelope has no message ID. Compare values with multiplicity, so adding an
// identical envelope still wakes; an identical replacement between peeks is
// indistinguishable, but the bounded reminder retries it. Retain only the last
// pending snapshot, not an ever-growing history of delivered payloads.
func newPending[T comparable](current, previous []T) []T {
	counts := make(map[T]int, len(previous))
	for _, item := range previous {
		counts[item]++
	}
	var added []T
	for _, item := range current {
		if counts[item] > 0 {
			counts[item]--
		} else {
			added = append(added, item)
		}
	}
	return added
}
