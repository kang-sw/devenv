package wsdashboard

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsinbox"
)

type answerRevisionRecorder struct {
	SessionSource
	ids chan string
}

func (r *answerRevisionRecorder) RecordDashboardAnswer(reg Registration, id string, _ time.Time) error {
	if reg.ReplyID == "livesession" {
		r.ids <- id
	}
	return nil
}

func TestDashboardAnswerJournalRecoveredOnIndexChange(t *testing.T) {
	ids := make(chan string, 4)
	f := startPageFixture(t, func(o *DaemonOptions) {
		o.Sessions = &answerRevisionRecorder{SessionSource: o.Sessions, ids: ids}
	})
	dir := sessionInbox(f.dir, "livesession")
	name, _, err := wsinbox.SendJournaled(dir, wsinbox.Entry{Chips: []wsinbox.Chip{{ID: "03", Answer: "a"}}}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wsinbox.Cancel(dir, name); err != nil {
		t.Fatal(err)
	}
	// Index discovery uses this hook on daemon startup, independently of the
	// original send and even after the held message has gone away.
	f.d.inboxMu.Lock()
	delete(f.d.answerPending, "livesession") // simulate the first startup discovery
	f.d.inboxMu.Unlock()
	f.d.sessionChanged("livesession")
	select {
	case got := <-ids:
		if want := (wsinbox.Record{Name: name}).ID(); got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	case <-time.After(time.Second):
		t.Fatal("pending answer not recovered")
	}
}

type blockedAnswerRecorder struct {
	SessionSource
	record func() error
}

func (r blockedAnswerRecorder) RecordDashboardAnswer(Registration, string, time.Time) error {
	return r.record()
}

func TestAnswerReplayDoesNotBlockSend(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	d := &Daemon{dir: t.TempDir(), log: io.Discard, stop: make(chan struct{})}
	d.source = blockedAnswerRecorder{record: func() error {
		close(entered)
		<-release // stands for the board lock
		return nil
	}}
	defer close(release)
	reg := Registration{ReplyID: "livesession"}
	dir := sessionInbox(d.dir, reg.ReplyID)
	if _, _, err := wsinbox.SendJournaled(dir, wsinbox.Entry{Parts: []string{"answer"}}, time.Now()); err != nil {
		t.Fatal(err)
	}
	d.scheduleAnswerReplay(reg)
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("replay did not enter board write")
	}
	done := make(chan int, 1)
	go func() {
		status, _ := d.messageWrite(dir, reg.ReplyID, []byte(`{"text":"concurrent send"}`), true, time.Now())
		done <- status
	}()
	select {
	case status := <-done:
		if status != http.StatusOK {
			t.Fatalf("send: %d", status)
		}
	case <-time.After(time.Second):
		t.Fatal("board write blocked inbox send")
	}
}

func TestAnswerReplayRetryBudget(t *testing.T) {
	if retryDashboardAnswer(errors.New("session identity unavailable"), 1) {
		t.Fatal("permanent failure retried")
	}
	for attempts := 1; attempts <= 5; attempts++ {
		if got := retryDashboardAnswer(context.DeadlineExceeded, attempts); got != (attempts < 5) {
			t.Fatalf("timeout attempt %d retry=%v", attempts, got)
		}
	}
}

func TestAnswerReplayDropsPermanentIdentityError(t *testing.T) {
	var calls atomic.Int32
	d := &Daemon{dir: t.TempDir(), log: io.Discard, stop: make(chan struct{})}
	d.source = blockedAnswerRecorder{record: func() error {
		calls.Add(1)
		return ErrDashboardAnswerIdentity
	}}
	reg := Registration{ReplyID: "livesession"}
	if _, _, err := wsinbox.SendJournaled(sessionInbox(d.dir, reg.ReplyID), wsinbox.Entry{Parts: []string{"answer"}}, time.Now()); err != nil {
		t.Fatal(err)
	}
	d.scheduleAnswerReplay(reg)
	deadline := time.Now().Add(time.Second)
	for {
		d.inboxMu.Lock()
		pending, known := d.answerPending[reg.ReplyID]
		d.inboxMu.Unlock()
		if known && !pending {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("permanent error left journal pending")
		}
		time.Sleep(time.Millisecond)
	}
	d.scheduleAnswerReplay(reg)
	if got := calls.Load(); got != 1 {
		t.Fatalf("permanent failure called %d times", got)
	}
}

func TestDashboardAnswerSendRecordsRevision(t *testing.T) {
	ids := make(chan string, 4)
	f := startPageFixture(t, func(o *DaemonOptions) {
		o.Sessions = &answerRevisionRecorder{SessionSource: o.Sessions, ids: ids}
	})
	f.send(t, `{"text":"ordinary message"}`)
	select {
	case id := <-ids:
		t.Fatalf("ordinary message recorded as answer: %s", id)
	default:
	}
	id, _ := f.send(t, `{"chips":[{"id":"03","answer":"a"}]}`)
	select {
	case got := <-ids:
		if got != id {
			t.Fatalf("recorded %q, want %q", got, id)
		}
	case <-time.After(time.Second):
		t.Fatal("answer revision was not recorded")
	}
}
