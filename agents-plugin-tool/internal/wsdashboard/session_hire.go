package wsdashboard

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kang-sw/devenv/internal/wsmailbox"
	"github.com/kang-sw/devenv/internal/wsstate"
)

const hireProposalsFile = "hire-proposals.json"
const hireLifetime = 30 * time.Minute
const hireSafetyPoll = 5 * time.Minute
const hireRetry = time.Second

// HireRequest contains preferences only. The owner chooses the final launch.
type HireRequest struct {
	Reason          string `json:"reason"`
	RecoveryContext string `json:"recovery_context"`
	Label           string `json:"label,omitempty"`
	Harness         string `json:"harness,omitempty"`
	Model           string `json:"model,omitempty"`
	Effort          string `json:"effort,omitempty"`
}

// HireLaunch is the owner's final selection; Root is never accepted from a
// request or approval body. It always comes from the requester's registration.
type HireLaunch struct {
	Harness         string `json:"harness"`
	Model           string `json:"model,omitempty"`
	Effort          string `json:"effort,omitempty"`
	Label           string `json:"label,omitempty"`
	PermissionMode  string `json:"permission_mode,omitempty"`
	PluginIsolation string `json:"plugin_isolation,omitempty"`
}

// HireCreator uses the ordinary hosted-lead creation path. RequestID makes a
// durable approval retry reuse its session, without setting authority lineage.
type HireCreator interface {
	CreateHireLead(requestID, root string, launch HireLaunch) (string, error)
	// CancelHireLead rolls back only a creation whose proposal did not approve.
	// It is daemon recovery, not control delegated to the requester.
	CancelHireLead(requestID string) error
}

// HireProposal is a daemon-private journal. Launch is an approval intent,
// persisted before creation; a restart completes it without creating twice.
type HireProposal struct {
	ID        string `json:"id"`
	Requester string `json:"requester"`
	Root      string `json:"root"`
	HireRequest
	CreatedAt         time.Time   `json:"created_at"`
	ExpiresAt         time.Time   `json:"expires_at"`
	Status            string      `json:"status"`
	Launch            *HireLaunch `json:"launch,omitempty"`
	Session           string      `json:"session,omitempty"`
	ReasonClosed      string      `json:"reason_closed,omitempty"`
	Notified          bool        `json:"notified,omitempty"`
	CreationError     string      `json:"creation_error,omitempty"`
	CreationAttempted bool        `json:"creation_attempted,omitempty"`
}

func (d *Daemon) hireClock() time.Time {
	if d.hireNow != nil {
		return d.hireNow()
	}
	return time.Now()
}

func (d *Daemon) readHires() ([]HireProposal, error) {
	if d.hireRead != nil {
		return d.hireRead()
	}
	raw, err := os.ReadFile(filepath.Join(d.dir, hireProposalsFile))
	if errors.Is(err, os.ErrNotExist) {
		return []HireProposal{}, nil
	}
	if err != nil {
		return nil, err
	}
	var proposals []HireProposal
	err = json.Unmarshal(raw, &proposals)
	return proposals, err
}

func (d *Daemon) saveHires(p []HireProposal) error {
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	if err := writePrivateFile(d.dir, hireProposalsFile, append(raw, '\n')); err != nil {
		return err
	}
	if !d.hireInLoop {
		d.pokeHires()
	}
	return nil
}

// hiringEligible reads the row on disk, not the eventually refreshed index:
// approval must refuse an end that raced the last index refresh.
func (d *Daemon) hiringEligible(replyID string) (Registration, error) {
	if !ValidReplyID(replyID) {
		return Registration{}, errors.New("unknown requester")
	}
	reg, err := ReadRegistration(d.dir, replyID)
	if err != nil {
		return reg, err
	}
	if !reg.Live(d.hireClock()) {
		return reg, errors.New("requester session has ended")
	}
	if reg.Agent || reg.Unlisted {
		return reg, errors.New("only lead sessions may request hiring")
	}
	meta, err := ReadHiringMetadata(d.dir, replyID)
	if err != nil {
		return reg, err
	}
	if meta.Hired {
		return reg, errors.New("hired sessions cannot hire")
	}
	if !meta.Enabled {
		return reg, errors.New("hiring is not enabled for this session")
	}
	if d.hireParent(replyID) != "" {
		return reg, errors.New("nested sessions cannot hire")
	}
	return reg, nil
}

// hireParent deliberately reads display hierarchy only, never key lineage.
func (d *Daemon) hireParent(replyID string) string {
	doc, _ := d.layout.current()
	facts := railFactsOf(d.railBaseView(d.pages()))
	row, ok := facts.rows[replyID]
	if !ok {
		return ""
	}
	scope := doc.scope(row.WS, facts, false)
	// Dormant links also preclude entitlement: reviving the parent would
	// otherwise turn an enabled root into a nested session without a write.
	return scope.parent(replyID)
}

// Read the stored links before layout projection drops unreadable parents.
// Readiness follows only this requester's chain, never the whole registry.
func (d *Daemon) hireHierarchyReady(replyID string) bool {
	if !d.index.hierarchyReady(replyID) {
		return false
	}
	doc, _ := d.layout.current()
	facts := railFactsOf(d.railBaseView(d.pages()))
	row, ok := facts.rows[replyID]
	if !ok {
		return false
	}
	ws := doc.Workspaces[row.WS]
	if ws == nil {
		return true
	}
	chain := []string{replyID}
	seen := map[string]bool{replyID: true}
	for i := 0; i < len(chain); i++ {
		for parent, children := range ws.Children {
			for _, child := range children {
				if child == chain[i] && !seen[parent] {
					seen[parent] = true
					chain = append(chain, parent)
				}
			}
		}
	}
	return d.index.hierarchyReady(replyID, chain[1:]...)
}

// RequestHire is called only after the MCP server authenticates a top-level
// lead key. There is no model-facing flag setter or approval operation.
func (d *Daemon) RequestHire(replyID string, request HireRequest) (HireProposal, error) {
	if strings.TrimSpace(request.Reason) == "" || strings.TrimSpace(request.RecoveryContext) == "" {
		return HireProposal{}, errors.New("reason and recovery_context are required")
	}
	if len(request.RecoveryContext) > 16<<10 || !utf8.ValidString(request.RecoveryContext) {
		return HireProposal{}, errors.New("recovery_context must be valid UTF-8 and at most 16 KiB")
	}
	label, err := cleanLabel(request.Label)
	if err != nil {
		return HireProposal{}, err
	}
	request.Label = label
	if request.Harness != "" {
		admitted, ok := d.agents.(interface{ AdmitsLead(string) bool })
		if !ok {
			return HireProposal{}, errors.New("this daemon's agent host cannot admit a lead harness")
		}
		if !admitted.AdmitsLead(request.Harness) {
			return HireProposal{}, errors.New("harness must be claude or pi")
		}
	}
	d.hireMu.Lock()
	defer d.hireMu.Unlock()
	if d.index == nil || !d.index.Loaded() {
		return HireProposal{}, errors.New("session index is not ready")
	}
	reg, err := d.hiringEligible(replyID)
	if err != nil {
		return HireProposal{}, err
	}
	proposals, err := d.readHires()
	if err != nil {
		return HireProposal{}, err
	}
	id, err := newToken()
	if err != nil {
		return HireProposal{}, err
	}
	now := d.hireClock()
	p := HireProposal{ID: id, Requester: replyID, Root: reg.Project, HireRequest: request, CreatedAt: now, ExpiresAt: now.Add(hireLifetime), Status: "pending"}
	proposals = append(proposals, p)
	if err = d.saveHires(proposals); err != nil {
		return HireProposal{}, err
	}
	d.broker.wake(railListRepoKey)
	return p, nil
}

// pokeHires folds registry changes into the daemon's proposal reconciler.
func (d *Daemon) pokeHires() {
	if d.hireWake == nil {
		return
	}
	select {
	case d.hireWake <- struct{}{}:
	default:
	}
}

func (d *Daemon) hireLoop() {
	defer close(d.hireDone)
	safety := d.hireClock().Add(hireSafetyPoll)
	for {
		select {
		case <-d.stop:
			return
		default:
		}
		next := d.settleHiresNext()
		now := d.hireClock()
		if !now.Before(safety) {
			safety = now.Add(hireSafetyPoll)
		}
		if next.IsZero() || safety.Before(next) {
			next = safety
		}
		delay := next.Sub(now)
		if delay < 0 {
			delay = 0
		}
		after := d.hireAfter
		if after == nil {
			after = func(delay time.Duration) (<-chan time.Time, func()) {
				timer := time.NewTimer(delay)
				return timer.C, func() { timer.Stop() }
			}
		}
		tick, stop := after(delay)
		select {
		case <-d.stop:
			stop()
			return
		case <-d.hireWake:
		case <-tick:
		}
		stop()
	}
}

func (d *Daemon) settleHires() { d.settleHiresNext() }

func (d *Daemon) settleHiresNext() time.Time {
	d.hireMu.Lock()
	defer d.hireMu.Unlock()
	d.hireInLoop = true
	defer func() { d.hireInLoop = false }()
	proposals, err := d.readHires()
	var next time.Time
	if err == nil {
		next, err = d.settleHiresDeadlineLocked(proposals)
	}
	if err != nil {
		fmt.Fprintf(d.log, "dashboard: hiring recovery: %v\n", err)
		retry := d.hireClock().Add(hireRetry)
		if next.IsZero() || retry.Before(next) {
			next = retry
		}
	}
	return next
}

// A stopped hosted process still has a live row and keeps its requests.
func (d *Daemon) settleHiresLocked(proposals []HireProposal) error {
	_, err := d.settleHiresDeadlineLocked(proposals)
	return err
}

func (d *Daemon) settleHiresDeadlineLocked(proposals []HireProposal) (time.Time, error) {
	var next time.Time
	arm := func(at time.Time) {
		if next.IsZero() || at.Before(next) {
			next = at
		}
	}
	var pendingErrors []error
	for i := range proposals {
		p := &proposals[i]
		if p.Status == "pending" {
			reg, err := ReadRegistration(d.dir, p.Requester)
			switch {
			case errors.Is(err, os.ErrNotExist) || (err == nil && !reg.Live(d.hireClock())):
				p.Status, p.ReasonClosed = "withdrawn", "requester session has ended"
			case !d.hireClock().Before(p.ExpiresAt):
				p.Status = "expired"
			case err != nil:
				pendingErrors = append(pendingErrors, err)
				continue
			case p.Launch != nil:
				// Startup recovery now runs immediately. Do not replay an
				// approval before the index supplies the hierarchy authority.
				if !d.hireHierarchyReady(p.Requester) {
					arm(d.hireClock().Add(hireRetry))
					break
				}
				if err := d.approveHireLocked(proposals, i); err != nil {
					// A failed creation leaves the request in the owner's queue;
					// it is not a new outcome or an automatic provider retry loop.
					p.Launch, p.CreationError = nil, err.Error()
					if err := d.saveHires(proposals); err != nil {
						return next, err
					}
				}
			}
			if p.Status == "pending" {
				arm(p.ExpiresAt)
				if err == nil {
					beat, parseErr := time.Parse(time.RFC3339, reg.HeartbeatAt)
					if parseErr == nil {
						// Live includes the boundary. Reread the row after it;
						// an unnotified heartbeat may have advanced it.
						arm(beat.Add(LiveWindow).Add(time.Nanosecond))
					}
				}
			}
		}
		if p.Status != "pending" && !p.Notified {
			// Commit the disposition before cleanup or mail, so a crash cannot
			// revive a cancelled request. Failed side effects retry from disk.
			if err := d.saveHires(proposals); err != nil {
				return next, err
			}
			// An interrupted approval may already have created its session before
			// grouping/outcome committed. Cancel that provisional creation only;
			// approved peers are never closed when their requester ends.
			if p.Status != "approved" && p.CreationAttempted {
				creator, ok := d.agents.(HireCreator)
				if !ok {
					pendingErrors = append(pendingErrors, errors.New("hiring creation recovery is unavailable"))
					continue
				}
				if err := creator.CancelHireLead(p.ID); err != nil {
					pendingErrors = append(pendingErrors, err)
					continue
				}
			}
			// Mail retries even when the requester is no longer indexed.
			if err := d.notifyHire(p); err != nil {
				pendingErrors = append(pendingErrors, err)
				continue
			}
			p.Notified = true
			if err := d.saveHires(proposals); err != nil {
				return next, err
			}
			d.broker.wake(railListRepoKey)
		}
	}
	return next, errors.Join(pendingErrors...)
}

func (d *Daemon) notifyHire(p *HireProposal) error {
	path, err := (wsmailbox.Paths{State: wsstate.Options{CacheHome: filepath.Dir(d.dir)}}).ReplyRegistry()
	if err != nil {
		return err
	}
	text := fmt.Sprintf("Hiring request %s: %s\n", p.ID, p.Status)
	if p.Session != "" {
		text += "Session: id:" + p.Session + "\n"
	}
	if p.ReasonClosed != "" {
		text += "Reason: " + p.ReasonClosed + "\n"
	}
	text += "\nRecovery context:\n" + p.RecoveryContext
	// Mail append and the proposal's Notified checkpoint are separate writes:
	// a crash between them can resend this correlated request-id outcome.
	if err := wsmailbox.PostSystem(path, p.Requester, "", text, d.hireClock()); err != nil {
		return err
	}
	if d.mailWake != nil {
		d.mailWake.Wake()
	}
	return nil
}

func (d *Daemon) approveHireLocked(proposals []HireProposal, index int) error {
	p := &proposals[index]
	unlock, err := LockRegistration(d.dir, p.Requester)
	if err != nil {
		return err
	}
	defer unlock()
	if _, err := d.hiringEligible(p.Requester); err != nil {
		p.Status, p.ReasonClosed = "withdrawn", err.Error()
		return nil
	}
	creator, ok := d.agents.(HireCreator)
	if !ok {
		return errors.New("hosted session creation is unavailable")
	}
	p.CreationAttempted = true
	if err := d.saveHires(proposals); err != nil {
		return err
	}
	id, err := creator.CreateHireLead(p.ID, p.Root, *p.Launch)
	if err != nil {
		return err
	}
	// Creation publishes the row before returning, but the asynchronous index
	// may not have seen it. Refresh before applying the exact rail attach op.
	d.index.Refresh()
	answer, err := d.layout.apply(railLayoutRequest{Op: "attach", RID: id, Parent: p.Requester}, railFactsOf(d.railBaseView(d.pages())), d.index.Loaded())
	if err != nil {
		return err
	}
	if answer.Reason != "" {
		return fmt.Errorf("attach hired session: %s", answer.Reason)
	}
	p.Session, p.Status = id, "approved"
	return nil
}

// pendingHires returns the owner's queue in arrival order, without stealing
// focus. The UI consumes this route in the following implementation phase.
func (d *Daemon) pendingHires() ([]HireProposal, error) {
	d.hireMu.Lock()
	defer d.hireMu.Unlock()
	proposals, err := d.readHires()
	if err != nil {
		return nil, err
	}
	if err = d.settleHiresLocked(proposals); err != nil {
		return nil, err
	}
	out := []HireProposal{}
	for _, p := range proposals {
		if p.Status == "pending" {
			out = append(out, p)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}
