package wsdashboard

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

// failingAttachTracker is the fake tracker whose attachment calls fail
// with an error that is none of the attachment sentinels, as a host fails
// on a transport, a store lock, or a disk.
type failingAttachTracker struct {
	*fakeTracker
	err error
}

func (f failingAttachTracker) StageAttachment(context.Context, string, string, []byte) (TrackerAttachment, error) {
	return TrackerAttachment{}, f.err
}

func (f failingAttachTracker) ReadAttachment(context.Context, string, string) ([]byte, error) {
	return nil, f.err
}

func (f failingAttachTracker) ReadWikiImage(context.Context, string, string, string) ([]byte, error) {
	return nil, f.err
}

// TestTrackerAttachmentRoutesAnswer500OnHostFailure: a host failure that is
// not an attachment refusal is the server's problem: the upload and both
// image reads answer 500, never 404 (which would tell the browser the
// image is gone while the remote may still hold it) or 415/413.
func TestTrackerAttachmentRoutesAnswer500OnHostFailure(t *testing.T) {
	d := startTrackerDaemon(t, failingAttachTracker{fakeTracker: newFakeTracker(), err: errors.New("ledger: fetch attachment: connection reset")})
	oid := strings.Repeat("a", 64)
	for name, call := range map[string]func() int{
		"upload": func() int {
			return callRaw(d, http.MethodPost, "/tracker/stores/team/attachments?name=x.png", tinyPNG, nil).Code
		},
		"read": func() int { return callRaw(d, http.MethodGet, "/tracker/stores/team/attachments/"+oid, nil, nil).Code },
		"wiki image": func() int {
			return callRaw(d, http.MethodGet, "/tracker/stores/team/wiki/docs/attachments/"+oid, nil, nil).Code
		},
	} {
		if got := call(); got != http.StatusInternalServerError {
			t.Errorf("%s over a failing host = %d, want 500", name, got)
		}
	}
}
