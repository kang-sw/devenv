package wsagent

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/kang-sw/devenv/internal/wsdashboard"
)

// CreateHireLead reuses CreateLead's owner-created authority. The correlation
// lives in a daemon-only sidecar, not ParentRef, a key parent, or agent.json:
// old typed record writers leave it intact. A record is the creation boundary;
// the marker is installed before it, so restart retries find even a lead whose
// first registration was interrupted.
func (r *Runner) CreateHireLead(requestID, root string, launch wsdashboard.HireLaunch) (string, error) {
	if requestID == "" {
		return "", fmt.Errorf("empty hiring request id")
	}
	r.hireMu.Lock()
	defer r.hireMu.Unlock()
	rec, err := r.findHireLead(requestID)
	if err != nil {
		return "", err
	}
	if rec != nil {
		if rec.Root != root {
			return "", fmt.Errorf("hiring creation identity mismatch")
		}
		return rec.ReplyID, nil
	}
	st, err := r.CreateLead(launch.Harness, root, LeadOptions{
		Model: launch.Model, Effort: launch.Effort, PermissionMode: launch.PermissionMode,
		PluginIsolation: launch.PluginIsolation, HireRequestID: requestID, HireLabel: launch.Label,
	})
	return st.ID, err
}

// CancelHireLead rolls back an uncommitted approval after rejection, expiry,
// or withdrawal. Only the daemon's proposal reconciler calls it; there is no
// requester-facing control. Already approved proposals never reach this path.
func (r *Runner) CancelHireLead(requestID string) error {
	if requestID == "" {
		return fmt.Errorf("empty hiring request id")
	}
	r.hireMu.Lock()
	defer r.hireMu.Unlock()
	rec, err := r.findHireLead(requestID)
	if err != nil || rec == nil {
		return err
	}
	if rec.ClosedAt != "" || rec.handedOff() {
		return nil
	}
	_, err = r.CloseConversation(rec.ReplyID)
	return err
}

func (r *Runner) findHireLead(requestID string) (*Record, error) {
	dirs, err := os.ReadDir(filepath.Join(r.opts.Dir, "sessions"))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	for _, dir := range dirs {
		if !dir.IsDir() || !wsdashboard.ValidReplyID(dir.Name()) {
			continue
		}
		meta, err := wsdashboard.ReadHiringMetadata(r.opts.Dir, dir.Name())
		if err != nil {
			return nil, err
		}
		if !meta.Hired || meta.RequestID != requestID {
			continue
		}
		rec, err := ReadRecord(r.opts.Dir, dir.Name())
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if !rec.Lead {
			return nil, fmt.Errorf("hiring creation identity mismatch")
		}
		return &rec, nil
	}
	return nil, nil
}
