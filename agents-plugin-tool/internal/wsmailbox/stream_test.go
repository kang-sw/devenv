package wsmailbox

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsinbox"
	"github.com/kang-sw/devenv/internal/wswake"
)

func TestStreamDeduplicatesPendingButRetriesWithoutPostponing(t *testing.T) {
	a, b := Envelope{Content: "a"}, Envelope{Content: "b"}
	clock := &fakeClock{now: time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)}
	stop := make(chan struct{})
	clock.onSleep = func(time.Duration) {
		if clock.sleeps == 9 {
			close(stop)
		}
	}
	// t=1: unchanged; t=2: identical append; t=3: same-count replacement;
	// t=4: no change; t=5: reminder includes everything; t=6: drain;
	// t=7: identical mail after an observed drain is new again; t=8: quiet.
	snapshots := []WaitResult{
		{Reply: []Envelope{a}}, {Reply: []Envelope{a}},
		{Reply: []Envelope{a, a}}, {Reply: []Envelope{a, b}},
		{Reply: []Envelope{a, b}}, {Reply: []Envelope{a, b}}, {},
		{Reply: []Envelope{a}}, {Reply: []Envelope{a}},
	}
	var times []int
	var arrivals [][]Envelope
	err := streamLoop(func() (WaitResult, error) {
		return snapshots[clock.sleeps], nil
	}, WaitOptions{Timeout: 5 * time.Second, Poll: time.Second, Now: clock.Now, Sleep: clock.Sleep, Done: stop}, func(r WaitResult) error {
		times = append(times, clock.sleeps)
		arrivals = append(arrivals, r.Reply)
		return nil
	})
	if !errors.Is(err, ErrWaitCancelled) {
		t.Fatalf("Stream = %v", err)
	}
	if !reflect.DeepEqual(times, []int{0, 2, 3, 5, 7}) || !reflect.DeepEqual(arrivals, [][]Envelope{{a}, {a}, {b}, {a, b}, {a}}) {
		t.Fatalf("emissions at %v: %#v", times, arrivals)
	}
}

func TestStreamBoardSameCountReplacementAndSteerFilter(t *testing.T) {
	for _, steerOnly := range []bool{false, true} {
		t.Run(map[bool]string{false: "held", true: "steer"}[steerOnly], func(t *testing.T) {
			wakeWaitEnv(t)
			inbox := filepath.Join(t.TempDir(), "inbox")
			sendBoardMessage(t, inbox, "held")
			clock := &fakeClock{now: time.Now()}
			stop := make(chan struct{})
			clock.onSleep = func(time.Duration) {
				switch clock.sleeps {
				case 1:
					// Merges under a fresh filename, still one pending entry.
					sendBoardMessage(t, inbox, "merged")
				case 2:
					names, err := wsinbox.MessageNames(inbox)
					if err != nil || len(names) != 1 {
						t.Fatalf("names = %v, %v", names, err)
					}
					if _, err := wsinbox.Steer(inbox, names[0]); err != nil {
						t.Fatal(err)
					}
				case 3:
					if _, _, err := wsinbox.Interrupt(inbox, clock.now); err != nil {
						t.Fatal(err)
					}
				case 5:
					close(stop)
				}
			}
			var times []int
			err := Stream(WaitTarget{SessionKey: "amber-tide-fox", InboxDir: inbox, InboxSteerOnly: steerOnly, InboxInterrupt: true},
				WaitOptions{Now: clock.Now, Sleep: clock.Sleep, Done: stop}, func(r WaitResult) error {
					times = append(times, clock.sleeps)
					if clock.sleeps == 3 && (r.Board != 0 || r.Interrupt != 1) {
						t.Fatalf("interrupt arrival = %#v", r)
					}
					return nil
				})
			want := []int{0, 1, 2, 3}
			if steerOnly {
				want = []int{2, 3}
			}
			if !errors.Is(err, ErrWaitCancelled) || !reflect.DeepEqual(times, want) {
				t.Fatalf("emissions at %v, err %v; want %v", times, err, want)
			}
		})
	}
}

func TestStreamWakeBackstopReminderAndCancellation(t *testing.T) {
	wakeWaitEnv(t)
	const key = "amber-tide-fox"
	clock := newFakeTimers()
	hub := &wswake.Hub{}
	stop := make(chan struct{})
	defer func() {
		select {
		case <-stop:
		default:
			close(stop)
		}
	}()
	done := make(chan error, 1)
	arrivals := make(chan WaitResult, 10)
	go func() {
		done <- Stream(WaitTarget{SessionKey: key}, WaitOptions{Wake: hub, Now: clock.Now, After: clock.After, Done: stop}, func(r WaitResult) error {
			arrivals <- r
			return nil
		})
	}()
	clock.nextArmed(t)
	seedWaitReplyMail(t, key, "one")
	hub.Wake()
	clock.nextArmed(t)
	hub.Wake() // An undrained queue must still block, not spin or emit again.
	clock.nextArmed(t)
	if len(arrivals) != 1 {
		t.Fatalf("arrivals = %d, want 1", len(arrivals))
	}
	clock.Advance(DefaultStreamReminder)
	clock.nextArmed(t)
	if len(arrivals) != 2 {
		t.Fatalf("default reminder arrivals = %d, want 2", len(arrivals))
	}
	close(stop)
	select {
	case err := <-done:
		if !errors.Is(err, ErrWaitCancelled) {
			t.Fatalf("cancel = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("stream did not cancel")
	}
}

func TestStreamErrorsAndEmptyTimeout(t *testing.T) {
	want := errors.New("output closed")
	if err := streamLoop(func() (WaitResult, error) { return WaitResult{Reply: []Envelope{{Content: "x"}}}, nil }, WaitOptions{}, func(WaitResult) error { return want }); !errors.Is(err, want) {
		t.Fatalf("emit error = %v", err)
	}
	if err := streamLoop(func() (WaitResult, error) { return WaitResult{}, want }, WaitOptions{}, func(WaitResult) error { t.Fatal("emitted on peek error"); return nil }); !errors.Is(err, want) {
		t.Fatalf("peek error = %v", err)
	}
	clock := &fakeClock{now: time.Now()}
	stop := make(chan struct{})
	clock.onSleep = func(time.Duration) {
		if clock.sleeps == 4 {
			close(stop)
		}
	}
	err := streamLoop(func() (WaitResult, error) { return WaitResult{}, nil }, WaitOptions{Timeout: time.Second, Poll: time.Second, Now: clock.Now, Sleep: clock.Sleep, Done: stop}, func(WaitResult) error { t.Fatal("empty timeout emitted"); return nil })
	if !errors.Is(err, ErrWaitCancelled) || clock.sleeps != 4 {
		t.Fatalf("empty stream ended early: %v, sleeps %d", err, clock.sleeps)
	}
}
