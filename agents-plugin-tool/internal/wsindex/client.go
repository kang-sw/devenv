package wsindex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Defaults. The read bound keeps tickets.query under roughly two seconds on
// network trouble; the write bound is per remote command.
const (
	DefaultReadTTL      = 60 * time.Second
	DefaultAbsenceTTL   = 10 * time.Minute
	DefaultReadTimeout  = 1500 * time.Millisecond
	DefaultWriteTimeout = 10 * time.Second
	DefaultMaxAttempts  = 5
)

// ErrRetryExhausted is returned when every bounded CAS attempt lost the race.
var ErrRetryExhausted = errors.New("ticket index write lost the compare-and-swap race on every attempt; the remote index is under heavy concurrent writes, retry later")

// Options configures a Client. Zero values take the defaults.
type Options struct {
	Runner       Runner
	Now          func() time.Time
	Getenv       func(string) string
	ReadTTL      time.Duration
	AbsenceTTL   time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	MaxAttempts  int
}

// Client operates the index for one checkout (any worktree of a clone; all
// local state is clone-shared).
type Client struct {
	root      string
	commonDir string
	runner    Runner
	now       func() time.Time
	getenv    func(string) string
	opts      Options
}

// Open binds a Client to the git checkout at root.
func Open(ctx context.Context, root string, opts Options) (*Client, error) {
	if opts.Runner == nil {
		opts.Runner = ExecRunner{}
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Getenv == nil {
		opts.Getenv = os.Getenv
	}
	if opts.ReadTTL <= 0 {
		opts.ReadTTL = DefaultReadTTL
	}
	if opts.AbsenceTTL <= 0 {
		opts.AbsenceTTL = DefaultAbsenceTTL
	}
	if opts.ReadTimeout <= 0 {
		opts.ReadTimeout = DefaultReadTimeout
	}
	if opts.WriteTimeout <= 0 {
		opts.WriteTimeout = DefaultWriteTimeout
	}
	if opts.MaxAttempts <= 0 {
		opts.MaxAttempts = DefaultMaxAttempts
	}
	c := &Client{root: root, runner: opts.Runner, now: opts.Now, getenv: opts.Getenv, opts: opts}
	common, err := c.git(ctx, nil, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return nil, fmt.Errorf("open ticket index: %w", err)
	}
	c.commonDir = common
	return c, nil
}

// Root returns the checkout the client operates in.
func (c *Client) Root() string { return c.root }

// ---- clone-shared sync state ------------------------------------------------

// syncState is the clone-shared timing state behind the read TTL and the
// absence cache. It lives in the git common dir so every worktree shares it.
type syncState struct {
	LastSync *time.Time `json:"last_sync,omitempty"`
	AbsentAt *time.Time `json:"absent_at,omitempty"`
}

func (c *Client) statePath() string {
	return filepath.Join(c.commonDir, stateDirName, "state.json")
}

func (c *Client) loadState() syncState {
	var st syncState
	data, err := os.ReadFile(c.statePath())
	if err == nil {
		_ = json.Unmarshal(data, &st)
	}
	return st
}

func (c *Client) saveState(st syncState) {
	path := c.statePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	data, err := json.Marshal(st)
	if err != nil {
		return
	}
	tmp := fmt.Sprintf("%s.%s.tmp", path, randomHex(4))
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
	}
}

func (c *Client) markSynced() {
	now := c.now().UTC()
	c.saveState(syncState{LastSync: &now})
}

func (c *Client) markAbsent() {
	now := c.now().UTC()
	c.saveState(syncState{AbsentAt: &now})
}

func (c *Client) absenceCached() bool {
	st := c.loadState()
	return st.AbsentAt != nil && c.now().Sub(*st.AbsentAt) < c.opts.AbsenceTTL
}

// ---- continuity -------------------------------------------------------------

// syncOutcome is the result of one remote read reconciled into the cache.
type syncOutcome struct {
	state   RemoteState
	tip     string // cache tip after reconciliation; "" unless present
	reports []string
}

// sync fetches the remote tip (or runs discovery first when the clone has
// never seen an index) and reconciles it into the cache ref. timeout bounds
// the remote commands only; local reconciliation runs on ctx.
func (c *Client) sync(ctx context.Context, timeout time.Duration, seen bool) (syncOutcome, error) {
	rctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if !seen {
		_, state, err := c.lsRemote(rctx)
		if state == RemoteAbsent {
			c.markAbsent()
		}
		if state != RemotePresent {
			return syncOutcome{state: state}, err
		}
	}
	tip, state, err := c.fetchTip(ctx, rctx)
	switch state {
	case RemoteUnreachable:
		return syncOutcome{state: state}, err
	case RemoteAbsent:
		reports, derr := c.discontinue(ctx, true)
		c.markAbsent()
		return syncOutcome{state: RemoteAbsent, reports: reports}, derr
	}
	cacheTip, reports, err := c.reconcile(ctx, tip)
	if err != nil {
		return syncOutcome{state: RemoteUnreachable}, err
	}
	c.markSynced()
	return syncOutcome{state: RemotePresent, tip: cacheTip, reports: reports}, nil
}

// reconcile moves the cache ref toward an observed remote tip and returns the
// cache tip afterward. A remote tip that is an ancestor of the cache is a
// stale read (a sibling worktree's push landed in between) and changes
// nothing; unrelated histories mean the index was deleted and re-initialized,
// which discards the pending log and adopts the new index.
func (c *Client) reconcile(ctx context.Context, remoteTip string) (string, []string, error) {
	for attempt := 0; attempt < maxLocalCASAttempts; attempt++ {
		cacheTip, ok, err := c.refOID(ctx, CacheRef)
		if err != nil {
			return "", nil, err
		}
		if !ok {
			// A pending log without a cache ref cannot be judged for
			// continuity; it was recorded against a cache that is gone.
			done, err := c.casRef(ctx, CacheRef, remoteTip, "")
			if err != nil {
				return "", nil, err
			}
			if done {
				return remoteTip, nil, nil
			}
			continue
		}
		if cacheTip == remoteTip {
			return cacheTip, nil, nil
		}
		forward, err := c.isAncestor(ctx, cacheTip, remoteTip)
		if err != nil {
			return "", nil, err
		}
		if forward {
			done, err := c.casRef(ctx, CacheRef, remoteTip, cacheTip)
			if err != nil {
				return "", nil, err
			}
			if done {
				return remoteTip, nil, nil
			}
			continue
		}
		stale, err := c.isAncestor(ctx, remoteTip, cacheTip)
		if err != nil {
			return "", nil, err
		}
		if stale {
			return cacheTip, nil, nil
		}
		reports, err := c.discontinue(ctx, false)
		if err != nil {
			return "", nil, err
		}
		done, err := c.casRef(ctx, CacheRef, remoteTip, cacheTip)
		if err != nil {
			return "", nil, err
		}
		if done {
			return remoteTip, reports, nil
		}
	}
	return "", nil, fmt.Errorf("update ticket-index cache: local compare-and-swap kept losing")
}

// discontinue discards the pending log and, on remote absence, deletes the
// cache ref (on re-initialization the caller replaces it instead). The report
// is emitted only when entries were discarded; dropping only a cache ref is
// silent.
func (c *Client) discontinue(ctx context.Context, deleteCache bool) ([]string, error) {
	n, err := c.discardPending(ctx)
	if err != nil {
		return nil, err
	}
	if deleteCache {
		for attempt := 0; attempt < maxLocalCASAttempts; attempt++ {
			cacheTip, ok, err := c.refOID(ctx, CacheRef)
			if err != nil {
				return nil, err
			}
			if !ok {
				break
			}
			done, err := c.casRef(ctx, CacheRef, "", cacheTip)
			if err != nil {
				return nil, err
			}
			if done {
				break
			}
		}
	}
	if n > 0 {
		return []string{DiscardReport(n)}, nil
	}
	return nil, nil
}

// Seen reports whether this clone has observed an index (its cache ref
// exists).
func (c *Client) Seen(ctx context.Context) (bool, error) {
	_, ok, err := c.refOID(ctx, CacheRef)
	return ok, err
}

// ---- read path --------------------------------------------------------------

// ViewState classifies what a read returned.
type ViewState string

const (
	// ViewAbsent: no origin, no index ref (including cached absence), or an
	// unreachable remote on a clone that never saw an index. Callers behave
	// exactly as they did before the index existed.
	ViewAbsent ViewState = "absent"
	// ViewFresh: the cache matches a remote read within the TTL.
	ViewFresh ViewState = "fresh"
	// ViewStale: the remote could not be read; the cache is served with its
	// age.
	ViewStale ViewState = "stale"
	// ViewUnknown: the index exists on the remote but nothing could be read
	// and no cache exists. Ownership is unknown, never unowned.
	ViewUnknown ViewState = "unknown"
)

// View is one read of the index.
type View struct {
	State   ViewState
	Index   *Index
	Tip     string
	Age     time.Duration // time since the last successful remote read; -1 when unknown
	Pending []PendingEntry
	Reports []string
}

// Read serves the index for display. Within the read TTL it makes no remote
// call; otherwise it fetches under the short read timeout and falls back to
// the stale cache. It never writes to the remote and never flushes the
// pending log; the only local mutation is cache reconciliation and the
// discontinuity discard.
func (c *Client) Read(ctx context.Context) (View, error) {
	view := View{State: ViewAbsent, Age: -1}
	if !c.hasOrigin(ctx) {
		return view, nil
	}
	seen, err := c.Seen(ctx)
	if err != nil {
		return view, err
	}
	st := c.loadState()
	if !seen && c.absenceCached() {
		return view, nil
	}
	if seen && st.LastSync != nil && c.now().Sub(*st.LastSync) < c.opts.ReadTTL {
		return c.viewFromCache(ctx, ViewFresh, c.now().Sub(*st.LastSync), nil)
	}
	out, _ := c.sync(ctx, c.opts.ReadTimeout, seen)
	switch out.state {
	case RemotePresent:
		return c.viewFromCache(ctx, ViewFresh, 0, out.reports)
	case RemoteAbsent:
		if !seen {
			c.markAbsent()
		}
		view.Reports = out.reports
		return view, nil
	}
	if !seen {
		// Never seen and unreachable: index-absent, and the negative result is
		// cached so an offline project does not pay the timeout per call.
		c.markAbsent()
		return view, nil
	}
	age := time.Duration(-1)
	if st.LastSync != nil {
		age = c.now().Sub(*st.LastSync)
	}
	return c.viewFromCache(ctx, ViewStale, age, nil)
}

func (c *Client) viewFromCache(ctx context.Context, state ViewState, age time.Duration, reports []string) (View, error) {
	view := View{State: state, Age: age, Reports: reports}
	tip, ok, err := c.refOID(ctx, CacheRef)
	if err != nil {
		return view, err
	}
	if !ok {
		view.State = ViewUnknown
	} else {
		idx, err := c.readIndexAt(ctx, tip)
		if err != nil {
			return view, err
		}
		view.Index, view.Tip = idx, tip
	}
	pending, err := c.Pending(ctx)
	if err != nil {
		return view, err
	}
	view.Pending = pending
	return view, nil
}

// ---- write path -------------------------------------------------------------

// WriteStatus is how a write ended.
type WriteStatus string

const (
	WriteAbsent      WriteStatus = "absent"      // index-absent: nothing written
	WriteWritten     WriteStatus = "written"     // a new version landed on the remote
	WriteNoChange    WriteStatus = "no-change"   // the operation changed nothing
	WritePending     WriteStatus = "pending"     // offline: recorded in the pending log
	WriteUnreachable WriteStatus = "unreachable" // index seen, remote unreachable, nothing recorded
)

// Replayer applies one pending entry to an index during a flush and returns
// its audit lines for the commit plus a report for the flushing tool's output
// ("" when it applied cleanly). A conflicting entry is dropped: it changes
// nothing and reports why.
type Replayer func(idx *Index, entry PendingEntry) (audit []string, report string)

// WriteOp is one CAS write. Replay (optional) flushes the pending log first,
// in recorded order; Mutate is the caller's own operation. Both are
// re-applied to the fresh tip on every attempt.
type WriteOp struct {
	Subject string
	Replay  Replayer
	Mutate  func(idx *Index) (audit []string, err error)
}

// WriteResult reports a write.
type WriteResult struct {
	Status  WriteStatus
	Tip     string
	Flushed int
	Reports []string
}

// Write performs one bounded CAS write against the fresh remote tip.
func (c *Client) Write(ctx context.Context, op WriteOp) (WriteResult, error) {
	if !c.hasOrigin(ctx) {
		return WriteResult{Status: WriteAbsent}, nil
	}
	seen, err := c.Seen(ctx)
	if err != nil {
		return WriteResult{}, err
	}
	if !seen && c.absenceCached() {
		return WriteResult{Status: WriteAbsent}, nil
	}
	var reports []string
	for attempt := 0; attempt < c.opts.MaxAttempts; attempt++ {
		out, syncErr := c.sync(ctx, c.opts.WriteTimeout, seen)
		reports = append(reports, out.reports...)
		switch out.state {
		case RemoteAbsent:
			return WriteResult{Status: WriteAbsent, Reports: reports}, nil
		case RemoteUnreachable:
			if !seen {
				c.markAbsent()
				return WriteResult{Status: WriteAbsent, Reports: reports}, nil
			}
			return WriteResult{Status: WriteUnreachable, Reports: reports}, syncErr
		}
		seen = true
		// Push against the tip the remote just returned, not a stale cache
		// tip a sibling may still be catching up on.
		base, err := c.readIndexAt(ctx, out.tip)
		if err != nil {
			return WriteResult{Reports: reports}, err
		}
		work := base.Clone()
		var audit, replayReports []string
		ids := map[string]bool{}
		if op.Replay != nil {
			entries, _, err := c.readPending(ctx)
			if err != nil {
				return WriteResult{Reports: reports}, err
			}
			for _, e := range entries {
				ids[e.ID] = true
				a, r := op.Replay(work, e)
				audit = append(audit, a...)
				if r != "" {
					replayReports = append(replayReports, r)
				}
			}
		}
		if op.Mutate != nil {
			a, err := op.Mutate(work)
			if err != nil {
				return WriteResult{Reports: reports}, err
			}
			audit = append(audit, a...)
		}
		if equalIndex(work, base) {
			n, err := c.removePending(ctx, ids)
			return WriteResult{Status: WriteNoChange, Tip: out.tip, Flushed: n, Reports: append(reports, replayReports...)}, err
		}
		commit, err := c.writeIndexCommit(ctx, work, out.tip, commitMessage(op.Subject, audit))
		if err != nil {
			return WriteResult{Reports: reports}, err
		}
		pctx, cancel := context.WithTimeout(ctx, c.opts.WriteTimeout)
		outcome, detail, pushErr := c.pushCAS(pctx, commit, out.tip)
		cancel()
		switch outcome {
		case pushAccepted:
			if _, rreports, err := c.reconcile(ctx, commit); err == nil {
				reports = append(reports, rreports...)
			}
			c.markSynced()
			n, err := c.removePending(ctx, ids)
			return WriteResult{Status: WriteWritten, Tip: commit, Flushed: n, Reports: append(reports, replayReports...)}, err
		case pushLost:
			continue
		case pushRefused:
			return WriteResult{Reports: reports}, fmt.Errorf("the remote refused the ticket index push: %s", detail)
		default:
			return WriteResult{Status: WriteUnreachable, Reports: reports}, pushErr
		}
	}
	return WriteResult{Reports: reports}, ErrRetryExhausted
}

func commitMessage(subject string, audit []string) string {
	if strings.TrimSpace(subject) == "" {
		subject = "ticket-index: update"
	}
	var b strings.Builder
	b.WriteString(subject)
	b.WriteString("\n")
	if len(audit) > 0 {
		b.WriteString("\n")
		for _, line := range audit {
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	return b.String()
}

// Submit is the index-write path every mutating tool uses: it flushes the
// pending log through replay, applies entry, and CAS-writes. When the remote
// is unreachable on a clone that has seen an index, entry is recorded in the
// pending log instead; a clone that never saw one stays index-absent.
func (c *Client) Submit(ctx context.Context, entry PendingEntry, replay Replayer) (WriteResult, error) {
	res, err := c.Write(ctx, WriteOp{
		Subject: fmt.Sprintf("ticket-index: %s %s", entry.Op, entry.Stem),
		Replay:  replay,
		Mutate: func(idx *Index) ([]string, error) {
			audit, report := replay(idx, entry)
			if report != "" {
				return nil, errors.New(report)
			}
			return audit, nil
		},
	})
	if res.Status != WriteUnreachable {
		return res, err
	}
	if _, perr := c.appendPending(ctx, entry); perr != nil {
		return res, perr
	}
	res.Status = WritePending
	return res, nil
}

// RecordPending appends entry to the pending log without contacting the
// remote. Offline decision paths (acquire's cached-view evaluation) use it
// after they have decided locally.
func (c *Client) RecordPending(ctx context.Context, entry PendingEntry) (PendingEntry, error) {
	return c.appendPending(ctx, entry)
}

// ---- creation and state check -----------------------------------------------

// CreateResult reports an index creation attempt.
type CreateResult struct {
	Created bool // this call created the ref; false means an existing index was adopted
	Tip     string
}

// Create creates the remote index with initial as its first version, using a
// must-not-exist lease. When another clone created it first, the existing
// index is adopted without error.
func (c *Client) Create(ctx context.Context, initial *Index, subject string) (CreateResult, error) {
	if !c.hasOrigin(ctx) {
		return CreateResult{}, fmt.Errorf("no %s remote: the ticket index lives on %s", RemoteName, RemoteName)
	}
	for attempt := 0; attempt < c.opts.MaxAttempts; attempt++ {
		lctx, cancel := context.WithTimeout(ctx, c.opts.WriteTimeout)
		_, state, err := c.lsRemote(lctx)
		cancel()
		switch state {
		case RemoteUnreachable:
			return CreateResult{}, fmt.Errorf("could not reach %s: %w", RemoteName, err)
		case RemotePresent:
			return c.adopt(ctx)
		}
		commit, err := c.writeIndexCommit(ctx, initial, "", commitMessage(subject, nil))
		if err != nil {
			return CreateResult{}, err
		}
		pctx, cancel := context.WithTimeout(ctx, c.opts.WriteTimeout)
		outcome, detail, pushErr := c.pushCAS(pctx, commit, "")
		cancel()
		switch outcome {
		case pushAccepted:
			if _, _, err := c.reconcile(ctx, commit); err != nil {
				return CreateResult{}, err
			}
			c.markSynced()
			return CreateResult{Created: true, Tip: commit}, nil
		case pushLost:
			continue
		case pushRefused:
			return CreateResult{}, fmt.Errorf("the remote refused the ticket index push: %s", detail)
		default:
			return CreateResult{}, fmt.Errorf("could not push the ticket index to %s: %w", RemoteName, pushErr)
		}
	}
	return CreateResult{}, ErrRetryExhausted
}

func (c *Client) adopt(ctx context.Context) (CreateResult, error) {
	out, err := c.sync(ctx, c.opts.WriteTimeout, true)
	if out.state != RemotePresent {
		if err == nil {
			err = fmt.Errorf("remote index is %s", out.state)
		}
		return CreateResult{}, err
	}
	return CreateResult{Created: false, Tip: out.tip}, nil
}

// CheckState is the live index state the init check reports.
type CheckState string

const (
	CheckInitialized   CheckState = "initialized"
	CheckUninitialized CheckState = "uninitialized"
	CheckNoOrigin      CheckState = "no-origin"
	CheckUnreachable   CheckState = "unreachable"
)

// Check reports the live index state with one ls-remote, bypassing the
// absence cache. It never pushes. Only an initialized result may touch local
// state (refreshing the cache ref); the other states leave no local trace.
func (c *Client) Check(ctx context.Context) (CheckState, error) {
	if !c.hasOrigin(ctx) {
		return CheckNoOrigin, nil
	}
	lctx, cancel := context.WithTimeout(ctx, c.opts.WriteTimeout)
	_, state, err := c.lsRemote(lctx)
	cancel()
	switch state {
	case RemoteAbsent:
		return CheckUninitialized, nil
	case RemoteUnreachable:
		return CheckUnreachable, err
	}
	_, _ = c.sync(ctx, c.opts.WriteTimeout, true)
	return CheckInitialized, nil
}
