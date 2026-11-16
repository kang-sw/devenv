package wsdashboard

import "path/filepath"

// ChangeHost optionally accepts hosted store invalidations without HTTP.
// handled is false when the host cannot serve this state directory.
type ChangeHost interface {
	NotifySessionChange(stateDir string, body NudgeBody) (port int, handled bool)
}

// NotifySessionChange schedules the same targeted lane used by HTTP nudges.
// Index.Nudge validates the registration and project as the HTTP route does;
// notification is not authorization to mutate a store. Unknown sessions
// still wake mail waits.
func (d *Daemon) NotifySessionChange(stateDir string, body NudgeBody) (int, bool) {
	if stateDir == "" || d.dir == "" || filepath.Clean(stateDir) != filepath.Clean(d.dir) {
		return 0, false
	}
	select {
	case <-d.stop:
		return 0, false
	default:
	}
	if d.mailWake != nil {
		d.mailWake.Nudge()
	}
	if d.index == nil || !d.index.Nudge(body.ReplyID, body.Project) {
		return 0, true
	}
	return d.endpoint.Port, true
}
