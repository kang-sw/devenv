package wsdashboard

import (
	"testing"
	"time"
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
