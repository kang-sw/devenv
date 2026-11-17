package wsdashboard

import (
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
