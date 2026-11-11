package wsmailbox

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wswake"
)

// fakeTimers drives a Wake wait's Now/After: no timer fires until Advance
// moves the clock past it, and each After call is reported on armed, which
// tells a test the wait is blocked.
type fakeTimers struct {
	mu     sync.Mutex
	now    time.Time
	timers []fakeTimer
	armed  chan time.Duration
}

type fakeTimer struct {
	at time.Time
	c  chan time.Time
}

func newFakeTimers() *fakeTimers {
	return &fakeTimers{now: time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC), armed: make(chan time.Duration, 64)}
}

func (f *fakeTimers) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

func (f *fakeTimers) After(d time.Duration) <-chan time.Time {
	f.mu.Lock()
	c := make(chan time.Time, 1)
	f.timers = append(f.timers, fakeTimer{at: f.now.Add(d), c: c})
	f.mu.Unlock()
	f.armed <- d
	return c
}

func (f *fakeTimers) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
	kept := f.timers[:0]
	for _, t := range f.timers {
		if t.at.After(f.now) {
			kept = append(kept, t)
			continue
		}
		t.c <- f.now
	}
	f.timers = kept
}

func (f *fakeTimers) nextArmed(t *testing.T) time.Duration {
	t.Helper()
	select {
	case d := <-f.armed:
		return d
	case <-time.After(5 * time.Second):
		t.Fatal("the wait never blocked")
		return 0
	}
}

type waitOutcome struct {
	result WaitResult
	err    error
}

func wakeWaitEnv(t *testing.T) {
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
}

func awaitOutcome(t *testing.T, done <-chan waitOutcome) WaitResult {
	t.Helper()
	select {
	case out := <-done:
		if out.err != nil {
			t.Fatalf("Wait: %v", out.err)
		}
		return out.result
	case <-time.After(5 * time.Second):
		t.Fatal("the wait did not return after the wake")
		return WaitResult{}
	}
}

// TestWakeWaitRechecksOnASignalWithoutAPollTick pins both signal kinds: no
// timer ever fires (the fake clock never advances), yet mail written while
// the wait blocks is found as soon as a hard wake or a first nudge arrives.
// The backstop it arms is WakeBackstopPoll.
func TestWakeWaitRechecksOnASignalWithoutAPollTick(t *testing.T) {
	for _, kind := range []string{"hard", "nudge"} {
		t.Run(kind, func(t *testing.T) {
			wakeWaitEnv(t)
			const sessionKey = "amber-tide-fox"
			hub := &wswake.Hub{}
			clock := newFakeTimers()
			done := make(chan waitOutcome, 1)
			go func() {
				r, err := Wait(WaitTarget{SessionKey: sessionKey}, WaitOptions{Wake: hub, Now: clock.Now, After: clock.After})
				done <- waitOutcome{r, err}
			}()
			if d := clock.nextArmed(t); d != WakeBackstopPoll {
				t.Fatalf("backstop = %v, want %v", d, WakeBackstopPoll)
			}
			if WakeBackstopPoll != 10*time.Second {
				t.Fatalf("WakeBackstopPoll = %v, want 10s", WakeBackstopPoll)
			}
			seedWaitReplyMail(t, sessionKey, "arrived")
			if kind == "hard" {
				hub.Wake()
			} else {
				hub.Nudge()
			}
			result := awaitOutcome(t, done)
			if len(result.Reply) != 1 || result.Reply[0].Content != "arrived" {
				t.Fatalf("Wait = %#v, want the mail written during the block", result)
			}
		})
	}
}

// TestWakeWaitBackstopStillRechecks pins the backstop: with no signal, the
// poll timer's firing finds the mail.
func TestWakeWaitBackstopStillRechecks(t *testing.T) {
	wakeWaitEnv(t)
	const sessionKey = "amber-tide-fox"
	clock := newFakeTimers()
	done := make(chan waitOutcome, 1)
	go func() {
		r, err := Wait(WaitTarget{SessionKey: sessionKey}, WaitOptions{Wake: &wswake.Hub{}, Now: clock.Now, After: clock.After})
		done <- waitOutcome{r, err}
	}()
	clock.nextArmed(t)
	seedWaitReplyMail(t, sessionKey, "late")
	clock.Advance(WakeBackstopPoll)
	if result := awaitOutcome(t, done); len(result.Reply) != 1 {
		t.Fatalf("Wait = %#v, want the mail the backstop found", result)
	}
}

// TestWakeWaitCoalescesNudges pins the nudge pacing on one waiter: a nudge
// a second or more after the last nudge-driven recheck rechecks at once; a
// burst inside the second costs one trailing recheck at its end, and the
// nudges folded into it cost nothing more. A hard wake is never held back.
func TestWakeWaitCoalescesNudges(t *testing.T) {
	hub := &wswake.Hub{}
	clock := newFakeTimers()
	peeks := make(chan struct{}, 64)
	stop := make(chan struct{})
	done := make(chan waitOutcome, 1)
	go func() {
		r, err := waitLoop(func() (WaitResult, error) {
			peeks <- struct{}{}
			return WaitResult{}, nil
		}, WaitOptions{Wake: hub, Now: clock.Now, After: clock.After, Done: stop})
		done <- waitOutcome{r, err}
	}()
	expectPeek := func(what string) {
		t.Helper()
		select {
		case <-peeks:
		case <-time.After(5 * time.Second):
			t.Fatalf("no recheck after %s", what)
		}
	}
	expectNoPeek := func(what string) {
		t.Helper()
		select {
		case <-peeks:
			t.Fatalf("a recheck after %s", what)
		case <-time.After(50 * time.Millisecond):
		}
	}

	expectPeek("the start")
	clock.nextArmed(t) // backstop
	hub.Nudge()
	expectPeek("a first nudge")
	clock.nextArmed(t) // backstop

	hub.Nudge()
	if d := clock.nextArmed(t); d != WakeNudgeInterval {
		t.Fatalf("trailing recheck armed for %v, want %v", d, WakeNudgeInterval)
	}
	for i := 0; i < 5; i++ {
		hub.Nudge()
	}
	expectNoPeek("a nudge burst inside the interval")

	clock.Advance(WakeNudgeInterval)
	expectPeek("the interval's end")
	clock.nextArmed(t) // backstop
	expectNoPeek("the trailing recheck that covered the burst")

	// The next nudge is again inside the interval of the trailing recheck.
	hub.Nudge()
	if d := clock.nextArmed(t); d != WakeNudgeInterval {
		t.Fatalf("second trailing recheck armed for %v, want %v", d, WakeNudgeInterval)
	}
	expectNoPeek("a nudge right after the trailing recheck")
	hub.Wake()
	expectPeek("a hard wake while a trailing recheck is armed")
	clock.nextArmed(t) // backstop

	close(stop)
	select {
	case out := <-done:
		if out.err != ErrWaitCancelled {
			t.Fatalf("wait after Done = %v, want ErrWaitCancelled", out.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Done did not end the wait")
	}
}
