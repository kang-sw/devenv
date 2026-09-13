package wsmailbox

import (
	"path/filepath"
	"testing"
	"time"
)

// fakeClock drives Wait's Now/Sleep without any real wall-clock delay:
// Sleep advances the clock by d and invokes onSleep (if set) so a test can
// deposit mail "during" a simulated block, exercising the arrival path
// without a real timer.
type fakeClock struct {
	now     time.Time
	sleeps  int
	onSleep func(d time.Duration)
}

func (c *fakeClock) Now() time.Time { return c.now }

func (c *fakeClock) Sleep(d time.Duration) {
	c.sleeps++
	c.now = c.now.Add(d)
	if c.onSleep != nil {
		c.onSleep(d)
	}
}

func TestWaitReturnsImmediatelyOnReplyIDUnread(t *testing.T) {
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	const sessionKey = "amber-tide-fox"
	secret, err := EnsureMachineSecret()
	if err != nil {
		t.Fatalf("EnsureMachineSecret: %v", err)
	}
	replyID := ReplyID(secret, sessionKey)
	replyPath, err := ReplyRegistryPath()
	if err != nil {
		t.Fatalf("ReplyRegistryPath: %v", err)
	}
	if err := WithReplyLock(replyPath, func(store *ReplyStore) error {
		store.Queues = AppendQueue(store.Queues, replyID, Envelope{Content: "hi", SentAt: "2026-09-13T00:00:00Z"})
		return nil
	}); err != nil {
		t.Fatalf("seed reply queue: %v", err)
	}

	clock := &fakeClock{now: time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC)}
	result, err := Wait(WaitTarget{SessionKey: sessionKey}, WaitOptions{Now: clock.Now, Sleep: clock.Sleep})
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if result.TimedOut {
		t.Fatalf("Wait reported TimedOut for already-unread mail")
	}
	if len(result.Reply) != 1 || result.Reply[0].Content != "hi" {
		t.Fatalf("Wait.Reply = %#v, want one envelope with content hi", result.Reply)
	}
	if clock.sleeps != 0 {
		t.Fatalf("Wait slept %d times for a level-triggered hit, want 0 (no block)", clock.sleeps)
	}
}

func TestWaitReturnsImmediatelyOnNamedInboxUnreadWhenOwner(t *testing.T) {
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	const sessionKey = "amber-tide-fox"
	path, err := MachinePath()
	if err != nil {
		t.Fatalf("MachinePath: %v", err)
	}
	if err := WithLock(path, func(store *StoreFile) error {
		store.Presence["alice"] = Presence{Name: "alice", Scope: ScopeMachine, Owner: sessionKey, LastSeen: "2026-09-13T00:00:00Z"}
		store.Queues = AppendQueue(store.Queues, "alice", Envelope{Content: "run ticket X", SentAt: "2026-09-13T00:00:00Z"})
		return nil
	}); err != nil {
		t.Fatalf("seed named inbox: %v", err)
	}

	clock := &fakeClock{now: time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC)}
	result, err := Wait(WaitTarget{SessionKey: sessionKey, Slug: "alice@machine"}, WaitOptions{Now: clock.Now, Sleep: clock.Sleep})
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if result.TimedOut || len(result.Named) != 1 || result.Named[0].Content != "run ticket X" {
		t.Fatalf("Wait = %#v, want one named envelope", result)
	}
}

func TestWaitIgnoresNamedInboxWhenNotOwner(t *testing.T) {
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	path, err := MachinePath()
	if err != nil {
		t.Fatalf("MachinePath: %v", err)
	}
	if err := WithLock(path, func(store *StoreFile) error {
		store.Presence["alice"] = Presence{Name: "alice", Scope: ScopeMachine, Owner: "someone-else-key", LastSeen: "2026-09-13T00:00:00Z"}
		store.Queues = AppendQueue(store.Queues, "alice", Envelope{Content: "not yours", SentAt: "2026-09-13T00:00:00Z"})
		return nil
	}); err != nil {
		t.Fatalf("seed named inbox: %v", err)
	}

	clock := &fakeClock{now: time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC)}
	result, err := Wait(WaitTarget{SessionKey: "amber-tide-fox", Slug: "alice@machine"}, WaitOptions{
		Timeout: 2 * DefaultWaitPoll, Now: clock.Now, Sleep: clock.Sleep,
	})
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if !result.TimedOut {
		t.Fatalf("Wait = %#v, want TimedOut: a non-owner must not see the named inbox's mail", result)
	}
}

func TestWaitBlocksThenReturnsOnArrivalDuringSleep(t *testing.T) {
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	const sessionKey = "amber-tide-fox"
	secret, err := EnsureMachineSecret()
	if err != nil {
		t.Fatalf("EnsureMachineSecret: %v", err)
	}
	replyID := ReplyID(secret, sessionKey)
	replyPath, err := ReplyRegistryPath()
	if err != nil {
		t.Fatalf("ReplyRegistryPath: %v", err)
	}

	clock := &fakeClock{now: time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC)}
	clock.onSleep = func(time.Duration) {
		if clock.sleeps == 2 {
			// Deposit mail "during" the second simulated block, so the third
			// peek (right after this Sleep call returns) finds it — exercising
			// arrival-while-waiting rather than the immediate level-trigger.
			_ = WithReplyLock(replyPath, func(store *ReplyStore) error {
				store.Queues = AppendQueue(store.Queues, replyID, Envelope{Content: "arrived", SentAt: "2026-09-13T00:00:03Z"})
				return nil
			})
		}
	}

	result, err := Wait(WaitTarget{SessionKey: sessionKey}, WaitOptions{Now: clock.Now, Sleep: clock.Sleep})
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if result.TimedOut {
		t.Fatalf("Wait reported TimedOut for mail deposited mid-block")
	}
	if len(result.Reply) != 1 || result.Reply[0].Content != "arrived" {
		t.Fatalf("Wait.Reply = %#v, want the mail deposited during the block", result.Reply)
	}
	if clock.sleeps != 2 {
		t.Fatalf("Wait slept %d times, want 2 (two empty peeks, mail deposited during the second, found on the third peek)", clock.sleeps)
	}
}

func TestWaitTimesOutCleanlyWithNoMail(t *testing.T) {
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	clock := &fakeClock{now: time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC)}
	result, err := Wait(WaitTarget{SessionKey: "amber-tide-fox"}, WaitOptions{
		Timeout: 3 * DefaultWaitPoll, Now: clock.Now, Sleep: clock.Sleep,
	})
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if !result.TimedOut {
		t.Fatalf("Wait = %#v, want TimedOut on an empty queue past the deadline", result)
	}
	if result.Total() != 0 {
		t.Fatalf("Wait.Total() = %d on timeout, want 0", result.Total())
	}
}

// TestWaitClampsFinalSleepToRemaining exercises Wait's partial-final-sleep clamp
// branch (`remaining < poll` -> `sleep(remaining)`): with a Timeout that is not
// an exact multiple of the poll interval, the last block before the deadline must
// be the clamped remainder, never a full poll that overshoots the deadline.
func TestWaitClampsFinalSleepToRemaining(t *testing.T) {
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	const poll = 500 * time.Millisecond
	const timeout = 1200 * time.Millisecond // not a multiple of poll: 500 + 500 + 200

	var slept []time.Duration
	clock := &fakeClock{now: time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC)}
	clock.onSleep = func(d time.Duration) { slept = append(slept, d) }

	result, err := Wait(WaitTarget{SessionKey: "amber-tide-fox"}, WaitOptions{
		Timeout: timeout, Poll: poll, Now: clock.Now, Sleep: clock.Sleep,
	})
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if !result.TimedOut {
		t.Fatalf("Wait = %#v, want TimedOut with no mail", result)
	}
	if len(slept) < 2 {
		t.Fatalf("Wait slept %v, want at least two blocks (full polls then a clamped remainder)", slept)
	}
	// No single block may exceed the poll interval, and the total slept must not
	// overshoot the timeout — the clamp is what guarantees both.
	var totalSlept time.Duration
	for i, d := range slept {
		if d > poll {
			t.Fatalf("sleep[%d] = %s exceeds the poll interval %s (clamp branch missing)", i, d, poll)
		}
		totalSlept += d
	}
	if totalSlept > timeout {
		t.Fatalf("total slept %s overshoots the timeout %s", totalSlept, timeout)
	}
	last := slept[len(slept)-1]
	if last != timeout-poll*time.Duration(len(slept)-1) {
		t.Fatalf("final sleep = %s, want the clamped remainder %s", last, timeout-poll*time.Duration(len(slept)-1))
	}
	if last >= poll {
		t.Fatalf("final sleep = %s was a full poll, not the clamped remainder", last)
	}
}

func TestNamedInboxStatusReportsPresenceAndOwnership(t *testing.T) {
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	if present, owned, err := NamedInboxStatus(WaitTarget{SessionKey: "amber-tide-fox"}); err != nil || present || owned {
		t.Fatalf("NamedInboxStatus(no slug) = present=%v owned=%v err=%v, want false/false/nil", present, owned, err)
	}
	if present, owned, err := NamedInboxStatus(WaitTarget{SessionKey: "amber-tide-fox", Slug: "alice@machine"}); err != nil || present || owned {
		t.Fatalf("NamedInboxStatus(no presence yet) = present=%v owned=%v err=%v, want false/false/nil", present, owned, err)
	}

	path, err := MachinePath()
	if err != nil {
		t.Fatalf("MachinePath: %v", err)
	}
	if err := WithLock(path, func(store *StoreFile) error {
		store.Presence["alice"] = Presence{Name: "alice", Scope: ScopeMachine, Owner: "someone-else-key", LastSeen: "2026-09-13T00:00:00Z"}
		return nil
	}); err != nil {
		t.Fatalf("seed presence: %v", err)
	}
	if present, owned, err := NamedInboxStatus(WaitTarget{SessionKey: "amber-tide-fox", Slug: "alice@machine"}); err != nil || !present || owned {
		t.Fatalf("NamedInboxStatus(owned by someone else) = present=%v owned=%v err=%v, want true/false/nil", present, owned, err)
	}
	if present, owned, err := NamedInboxStatus(WaitTarget{SessionKey: "someone-else-key", Slug: "alice@machine"}); err != nil || !present || !owned {
		t.Fatalf("NamedInboxStatus(owned by this session) = present=%v owned=%v err=%v, want true/true/nil", present, owned, err)
	}
}

func TestWaitEnvLessTargetsOwnReplyIDOnly(t *testing.T) {
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	// A slug that exists but is owned by someone else, plus this caller's
	// own reply-id mail: an env-less caller (Slug == "") must not attempt to
	// resolve a named inbox at all, and must still see its own reply mail.
	path, err := MachinePath()
	if err != nil {
		t.Fatalf("MachinePath: %v", err)
	}
	if err := WithLock(path, func(store *StoreFile) error {
		store.Presence["alice"] = Presence{Name: "alice", Scope: ScopeMachine, Owner: "someone-else-key", LastSeen: "2026-09-13T00:00:00Z"}
		return nil
	}); err != nil {
		t.Fatalf("seed presence: %v", err)
	}

	const sessionKey = "amber-tide-fox"
	secret, err := EnsureMachineSecret()
	if err != nil {
		t.Fatalf("EnsureMachineSecret: %v", err)
	}
	replyID := ReplyID(secret, sessionKey)
	replyPath, err := ReplyRegistryPath()
	if err != nil {
		t.Fatalf("ReplyRegistryPath: %v", err)
	}
	if err := WithReplyLock(replyPath, func(store *ReplyStore) error {
		store.Queues = AppendQueue(store.Queues, replyID, Envelope{Content: "reply", SentAt: "2026-09-13T00:00:00Z"})
		return nil
	}); err != nil {
		t.Fatalf("seed reply queue: %v", err)
	}

	clock := &fakeClock{now: time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC)}
	result, err := Wait(WaitTarget{SessionKey: sessionKey}, WaitOptions{Now: clock.Now, Sleep: clock.Sleep})
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if len(result.Named) != 0 {
		t.Fatalf("Wait.Named = %#v, want empty for an env-less (no-slug) target", result.Named)
	}
	if len(result.Reply) != 1 || result.Reply[0].Content != "reply" {
		t.Fatalf("Wait.Reply = %#v, want the caller's own reply mail", result.Reply)
	}
}
