package wsagent

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsinbox"
	"github.com/kang-sw/devenv/internal/wslive"
)

// children.go is the spawn tree under a lead: a spawned Pi worker spawns
// Pi children of its own through the daemon, which owns the whole tree.
//
//   - Token. A conversation started through Runner.Spawn with a recorded
//     parent and tier, whose profile allows a pi.* tool, gets a capability
//     token in its attached ws-mcp's environment (EnvPiToken, beside the
//     daemon's loopback base URL in EnvPiEndpoint). The token is drawn at
//     every process start, persisted privately for held workers, and void
//     once the process ends or the conversation closes. Its key stays a parentless delegate key:
//     the token, not the key, authorizes the token route below, and only
//     for the holder's own children.
//   - Route. POST /agents/by-token/<op> (spawn, list, send, stop,
//     transcript, wait) with the token as a bearer credential, from this
//     machine only. A spawn records the holder as the child's parent
//     (ParentConversation); its depth (the lead is 0) may not pass
//     MaxSpawnDepth, and its tier may not pass the holder's own.
//   - Reports. A child's reported turn waits in its record
//     (Record.PendingReports) until the runner hands it to the parent:
//     steered into the parent's running turn, else kept for its next turn
//     (Record.Reports), which starts at once when none runs. A parent
//     blocked in a wait on that child takes it as the wait's answer
//     instead.
//   - Holds. A conversation's done turn is not reported upstream (no
//     report, no wait answer) while a child of its own is open: from a
//     message it was sent until its report reaches the parent, or it
//     closes. A failed or interrupted turn is never held.
//   - Close. Closing a conversation closes its open descendants first,
//     deepest first. A process stop (idle, settings, the stop route) does
//     not: a child's report resumes the stopped parent.

// Env names the runner sets on the ws-mcp it attaches to a spawned
// conversation that may spawn (children.go).
const (
	// EnvPiToken is the conversation's capability token for the token
	// route.
	EnvPiToken = "WS_MCP_PI_TOKEN"
	// EnvPiEndpoint is the daemon's loopback base URL ("http://127.0.0.1:
	// <port>"), where the token route is.
	EnvPiEndpoint = "WS_MCP_PI_ENDPOINT"
)

// MaxSpawnDepth caps the spawn tree: a lead is depth 0, a Pi worker it
// spawns depth 1, and that worker's child depth 2, which spawns none.
const MaxSpawnDepth = 2

// PiTierHarness is the agents.tier harness bucket a Pi spawn's tier
// resolves in, alone.
const PiTierHarness = "pi"

// tokenRoute is the id segment of the token route. It holds a dash, which
// no reply-id does, so it never names a conversation.
const tokenRoute = "by-token"

// The token route's wait: its default and its cap.
const (
	tokenWaitDefault = 300 * time.Second
	tokenWaitMax     = PiWaitMax
)

var errDaemonDetached = &requestError{status: http.StatusServiceUnavailable, msg: "the dashboard daemon detached; reissue the wait"}

var errBadToken = &requestError{status: http.StatusUnauthorized, msg: "the capability token is not valid: only a running Pi subagent the dashboard daemon spawned holds one, and it ends with that subagent's process"}

// SetEndpoint gives the runner the daemon's loopback base URL, which a
// token holder's ws-mcp calls back on. The daemon sets it before Start;
// with none, no token is issued.
func (r *Runner) SetEndpoint(baseURL string) {
	r.mu.Lock()
	r.endpoint = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	r.mu.Unlock()
}

// mayHoldToken reports whether a conversation's process gets a capability
// token: a spawned one (a recorded parent and tier) whose profile allows a
// pi.* tool. A conversation opened through POST /agents has neither.
func mayHoldToken(rec Record, def Profile) bool {
	if rec.Lead || rec.Parent == nil || rec.Tier == "" {
		return false
	}
	for _, name := range def.WSTools {
		if strings.HasPrefix(name, "pi.") {
			return true
		}
	}
	return false
}

// issueToken draws conversation id's token for a process start, voiding
// the one its last process had, and returns it with the endpoint; both are
// empty when the runner has no endpoint.
func (r *Runner) issueToken(id string) (token, endpoint string) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", ""
	}
	token = hex.EncodeToString(raw)
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.endpoint == "" {
		return "", ""
	}
	if old, ok := r.tokenOf[id]; ok {
		delete(r.tokens, old)
	}
	if r.tokens == nil {
		r.tokens, r.tokenOf = map[string]string{}, map[string]string{}
	}
	r.tokens[token], r.tokenOf[id] = id, token
	return token, r.endpoint
}

// revokeToken voids conversation id's token: its process ended, or it
// closed.
func capabilityHash(token string) string {
	if token == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// restoreToken is called only after the holder's recorded association matches.
func (r *Runner) restoreToken(rec Record) {
	if rec.Capability == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.tokens == nil {
		r.tokens, r.tokenOf = map[string]string{}, map[string]string{}
	}
	r.tokens[rec.Capability], r.tokenOf[rec.ReplyID] = rec.ReplyID, rec.Capability
}

func (r *Runner) revokeToken(id string) {
	r.mu.Lock()
	if old, ok := r.tokenOf[id]; ok {
		delete(r.tokens, old)
		delete(r.tokenOf, id)
	}
	r.mu.Unlock()
}

// tokenHolder is the open conversation token was issued to.
func (r *Runner) tokenHolder(token string) (*conv, error) {
	if token == "" {
		return nil, errBadToken
	}
	r.mu.Lock()
	id, ok := r.tokens[token]
	c := r.convs[id]
	r.mu.Unlock()
	if !ok || c == nil {
		return nil, errBadToken
	}
	c.mu.Lock()
	closed := c.closed
	c.mu.Unlock()
	if closed {
		return nil, errBadToken
	}
	return c, nil
}

// recordOf is conversation id's record, open or closed.
func (r *Runner) recordOf(id string) (Record, bool) {
	r.mu.Lock()
	c, open := r.convs[id]
	rec, closed := r.closed[id]
	r.mu.Unlock()
	if open {
		return c.record(), true
	}
	return rec, closed
}

// depth is rec's depth in its spawn tree (a lead is 0), walking the
// parent chain through the runner's records; ok is false when the chain
// cannot be traced to a lead.
func (r *Runner) depth(rec Record) (int, bool) {
	d := 0
	for range MaxSpawnDepth + 8 {
		if rec.Parent == nil {
			return d, true
		}
		d++
		if rec.Parent.Kind != ParentConversation {
			return d, true
		}
		next, ok := r.recordOf(rec.Parent.ReplyID)
		if !ok {
			return 0, false
		}
		rec = next
	}
	return 0, false
}

// tierRank orders the capability tiers; 0 is no tier.
func tierRank(tier string) int {
	switch tier {
	case "small":
		return 1
	case "medium":
		return 2
	case "large":
		return 3
	case "xlarge":
		return 4
	}
	return 0
}

// PiTierRefusal names the config.tune key a spawn's unset tier needs and
// the tiers that are set.
func PiTierRefusal(tool, tier string, tiers map[string]wsconfig.AgentTier) error {
	set := make([]string, 0, len(tiers))
	for _, t := range []string{"small", "medium", "large", "xlarge"} {
		if _, ok := tiers[t]; ok {
			set = append(set, t)
		}
	}
	tune := fmt.Sprintf(`config.tune key "agents.tier" with harness %q, tier %q, and a model Pi runs`, PiTierHarness, tier)
	if len(set) == 0 {
		return fmt.Errorf("%s: no Pi tier is set, so tier %q has no model; set it with %s", tool, tier, tune)
	}
	return fmt.Errorf("%s: tier %q is not set for Pi; pick a set tier (%s) or set this one with %s", tool, tier, strings.Join(set, ", "), tune)
}

// ChildSpawn is a token holder's spawn request: the pi.spawn arguments.
type ChildSpawn struct {
	Prompt string `json:"prompt"`
	Tier   string `json:"tier"`
	// Profile defaults to pi-worker; any resolved non-lead Pi profile is allowed
	// when its tools are within the holder's launched definition.
	Profile string `json:"profile,omitempty"`
	// Cwd is absolute, or relative to the holder's own working directory
	// (Record.workDir: a worker in a worktree spawns into that worktree);
	// empty is that directory.
	Cwd   string `json:"cwd,omitempty"`
	Label string `json:"label,omitempty"`
}

// spawnFor spawns a Pi child of caller, the token holder, after the depth
// and tier caps; model and effort come from the tier's entry in the pi
// bucket of agents.tier at the holder's root.
func (r *Runner) spawnFor(caller *conv, req ChildSpawn) (State, error) {
	const tool = "pi.spawn"
	rec := caller.record()
	if strings.TrimSpace(req.Prompt) == "" {
		return State{}, badRequest("%s: prompt is required", tool)
	}
	d, ok := r.depth(rec)
	if !ok {
		return State{}, &requestError{status: http.StatusForbidden, msg: fmt.Sprintf("%s: this conversation's spawn chain cannot be traced to its lead, so its depth is unknown and it spawns no subagent", tool)}
	}
	if d+1 > MaxSpawnDepth {
		return State{}, &requestError{status: http.StatusForbidden, msg: fmt.Sprintf("%s: the spawn depth cap is %d (lead -> Pi worker -> Pi subagent); this conversation is at depth %d, so it spawns no subagent: do the work itself", tool, MaxSpawnDepth, d)}
	}
	tier := wsconfig.ModelAlias(req.Tier)
	if tier == "" {
		return State{}, badRequest("%s: tier must be small, medium, large, or xlarge; got %q", tool, req.Tier)
	}
	if rec.Tier == "" {
		return State{}, &requestError{status: http.StatusForbidden, msg: fmt.Sprintf("%s: this conversation has no recorded tier, so no tier cap for its subagents can be set; it spawns none", tool)}
	}
	if tierRank(tier) > tierRank(rec.Tier) {
		return State{}, &requestError{status: http.StatusForbidden, msg: fmt.Sprintf("%s: tier %q is above this conversation's own tier %q, which caps its subagents' tier; pick %q or lower", tool, tier, rec.Tier, rec.Tier)}
	}
	cfg := r.opts.Config
	cfg.RepoRoot = rec.Root
	tiers, err := wsconfig.HarnessTiers(cfg, PiTierHarness)
	if err != nil {
		return State{}, fmt.Errorf("%s: read agents.tier: %w", tool, err)
	}
	mapping, ok := tiers[tier]
	if !ok {
		return State{}, badRequest("%v", PiTierRefusal(tool, tier, tiers))
	}
	cwd, base := strings.TrimSpace(req.Cwd), rec.workDir()
	switch {
	case cwd == "":
		cwd = base
	case !filepath.IsAbs(cwd):
		cwd = filepath.Join(base, cwd)
	}
	label := req.Label
	if strings.TrimSpace(label) == "" {
		label = req.Prompt
	}
	st, err := r.Spawn(SpawnSpec{
		Profile: req.Profile,
		Root:    rec.Root,
		Cwd:     cwd,
		Model:   mapping.Model,
		Effort:  mapping.Effort,
		Tier:    tier,
		Prompt:  req.Prompt,
		Label:   label,
		Parent:  &ParentRef{Kind: ParentConversation, ReplyID: rec.ReplyID},
	})
	if err != nil {
		return State{}, err
	}
	// A close of the caller that ran meanwhile listed its descendants
	// before this child existed: the child closes with it here.
	if caller.isClosed() {
		_, _ = r.CloseConversation(st.ID)
		return State{}, errClosed
	}
	return st, nil
}

// ownChild refuses an id that is not one of parent's children.
func (r *Runner) ownChild(tool, id string, parent ParentRef) error {
	st, err := r.Get(id)
	if err == nil && st.Parent != nil && *st.Parent == parent {
		return nil
	}
	return &requestError{status: http.StatusNotFound, msg: fmt.Sprintf("%s: %s is not one of this conversation's Pi subagents (pi.list lists them)", tool, id)}
}

// resumePiChild returns an open conversation for Send. Only a closed,
// spawned Pi child is revivable: its identity and launch settings stay in
// the same record, and the ordinary lazy-start path resumes its Pi file.
// Open children whose process died already use that same lazy-start path.
func (r *Runner) resumePiChild(id string) (*conv, error) {
	if c, err := r.open(id); err == nil {
		return c, nil
	}
	r.resumeMu.Lock()
	defer r.resumeMu.Unlock()
	// A concurrent send may have reopened it while this one waited.
	c, err := r.open(id)
	if err == nil {
		return c, nil
	}
	r.mu.Lock()
	rec, closed := r.closed[id]
	r.mu.Unlock()
	if !closed || rec.ClosedAt == "" || rec.Lead || rec.Parent == nil || rec.Backend != BackendPi {
		return nil, err
	}
	// A send authorized before its caller closed may have waited behind
	// that close's cascade. It must not resurrect a descendant afterward.
	if rec.Parent.Kind == ParentConversation {
		parent := r.conv(rec.Parent.ReplyID)
		if parent == nil || parent.isClosed() {
			return nil, conflict("cannot resume Pi subagent %s: its parent conversation is closed", id)
		}
	}
	dir := wsdashboard.SessionDir(r.opts.Dir, id)
	file := piSessionFile(dir)
	if info, statErr := os.Stat(file); statErr != nil || !info.Mode().IsRegular() {
		return nil, conflict("cannot resume Pi subagent %s: its session file is missing, inaccessible, or not a regular file: %s", id, file)
	}
	rec.ClosedAt = ""
	// A done turn held for descendants was abandoned by the close, not
	// left waiting for this new turn to publish it.
	rec.HeldTurn = nil
	if err := writeRecord(dir, rec); err != nil {
		return nil, err
	}
	c = r.startConv(rec, nil)
	c.watched()
	return c, nil
}

// tokenRequest is the token route's body: the fields each op reads.
type tokenRequest struct {
	ChildSpawn
	ID       string  `json:"id"`
	Message  string  `json:"message"`
	TimeoutS float64 `json:"timeout_s"`
}

// serveToken serves POST /agents/by-token/<op>: a token holder's pi.*
// call, scoped to its own children.
func (r *Runner) serveToken(w http.ResponseWriter, req *http.Request, op string) {
	if local, _ := r.opts.Trusted(req); !local {
		http.Error(w, "unknown agent route", http.StatusNotFound)
		return
	}
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	select {
	case <-r.rejoined:
	case <-req.Context().Done():
		return
	case <-r.quit:
		writeAnswer(w, 0, nil, errDaemonDetached)
		return
	}
	token, _ := strings.CutPrefix(req.Header.Get("Authorization"), "Bearer ")
	caller, err := r.tokenHolder(strings.TrimSpace(token))
	if err != nil {
		writeAnswer(w, 0, nil, err)
		return
	}
	if check := r.opts.ChildCheck; check != nil {
		if err := check(); err != nil {
			writeAnswer(w, 0, nil, &requestError{status: http.StatusServiceUnavailable, msg: "pi." + op + ": " + err.Error()})
			return
		}
	}
	var body tokenRequest
	if !decode(w, req, &body) {
		return
	}
	parent := ParentRef{Kind: ParentConversation, ReplyID: caller.record().ReplyID}
	tool := "pi." + op
	switch op {
	case "spawn":
		st, err := r.spawnFor(caller, body.ChildSpawn)
		writeAnswer(w, http.StatusCreated, st, err)
		return
	case "list":
		children := r.Children(parent)
		if children == nil {
			children = []State{}
		}
		writeAnswer(w, http.StatusOK, map[string]any{"conversations": children}, nil)
		return
	case "send", "stop", "transcript", "wait":
	default:
		http.Error(w, "unknown agent route", http.StatusNotFound)
		return
	}
	if err := r.ownChild(tool, body.ID, parent); err != nil {
		writeAnswer(w, 0, nil, err)
		return
	}
	switch op {
	case "send":
		if strings.TrimSpace(body.Message) == "" {
			writeAnswer(w, 0, nil, badRequest("%s: message is required", tool))
			return
		}
		st, err := r.Send(body.ID, body.Message)
		writeAnswer(w, http.StatusOK, st, err)
	case "stop":
		st, err := r.CloseConversation(body.ID)
		writeAnswer(w, http.StatusOK, st, err)
	case "transcript":
		records, err := r.Transcript(body.ID)
		if records == nil {
			records = []wslive.RingRecord{}
		}
		writeAnswer(w, http.StatusOK, map[string]any{"records": records}, err)
	case "wait":
		timeout := tokenWaitDefault
		if body.TimeoutS > 0 {
			timeout = time.Duration(body.TimeoutS * float64(time.Second))
		}
		timeout = min(timeout, tokenWaitMax)
		res, err := r.wait(req.Context(), body.ID, timeout, true)
		writeAnswer(w, http.StatusOK, res, err)
	}
}

// isClosed reports whether the conversation is closed.
func (c *conv) isClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

// newReportID tells one pending report from another.
func newReportID() string {
	raw := make([]byte, 8)
	_, _ = rand.Read(raw)
	return hex.EncodeToString(raw)
}

// deliverLater hands conversation id's pending reports to its parent off
// the caller's goroutine: the handover runs in the parent's loop, and no
// loop waits on another's.
func (r *Runner) deliverLater(id string) {
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		r.deliver(id)
	}()
}

// deliver hands open conversation id's pending reports
// (Record.PendingReports) to its open conversation parent, taking them
// from the child's record in the same step of the parent's loop, so the
// parent never reads the child with a report in neither place
// (holdsReport). A report whose parent is closed or unknown, or whose own
// conversation closed, stays where it is.
func (r *Runner) deliver(id string) {
	child := r.conv(id)
	if child == nil {
		return
	}
	rec := child.record()
	if len(rec.PendingReports) == 0 || rec.Parent == nil || rec.Parent.Kind != ParentConversation {
		return
	}
	parent := r.conv(rec.Parent.ReplyID)
	if parent == nil {
		return
	}
	parent.do(func() {
		if parent.closed {
			return
		}
		var taken []PendingReport
		child.update(func(rec *Record) {
			if !child.closed {
				taken, rec.PendingReports = rec.PendingReports, nil
			}
		})
		if len(taken) == 0 {
			return
		}
		st := child.state()
		for _, rep := range taken {
			parent.takeReport(ReportText(TurnReport{Parent: *rec.Parent, Child: st, Result: rep.Result}))
		}
	})
}

// deliverPending hands every open conversation's pending reports to its
// parent: after a daemon start loaded them all, since a child loaded
// before its parent found none to hand them to.
func (r *Runner) deliverPending() {
	r.mu.Lock()
	var ids []string
	for id, c := range r.convs {
		c.mu.Lock()
		pending := len(c.rec.PendingReports) > 0
		c.mu.Unlock()
		if pending {
			ids = append(ids, id)
		}
	}
	r.mu.Unlock()
	for _, id := range ids {
		r.deliverLater(id)
	}
}

// steeredReport is a report or user message steered into the running turn:
// seq is its steer command's place among the process's (steerSeq). Mark is
// the Send coverage of a user message, applied only once Pi takes it.
type steeredReport struct {
	Text string `json:"text"`
	Seq  int    `json:"seq"`
	Mark uint64 `json:"-"`
}

// steerWorker submits a spawned Pi worker's marked inbox messages through
// the same acknowledged, recoverable native steer path as child reports.
// The caller checked that its turn runs and replay has finished.
func (c *conv) steerWorker() {
	if c.rec.Parent == nil || c.rec.Backend != BackendPi {
		return
	}
	if _, ok := c.proc.(reportSteerer); !ok {
		return
	}
	// Every Runner.Send to this worker is marked for steering. Serialize
	// the drain and its coverage against Send, as kick does for prompts.
	c.mu.Lock()
	recs, _ := wsinbox.DrainSteer(c.inbox)
	covered := c.sends
	c.mu.Unlock()
	var parts []string
	for _, rec := range recs {
		if text := strings.TrimSpace(rec.Entry.Text()); text != "" {
			parts = append(parts, text)
		}
	}
	if len(parts) == 0 {
		return
	}
	c.drained = covered
	c.update(func(rec *Record) { rec.Drained = covered })
	c.takeSteer(strings.Join(parts, "\n\n"), covered)
}

// takeReport takes a child's report into the conversation: steered into
// the running turn when its harness takes a message mid-turn and tells
// which it took (reportSteerer), else kept for its next turn
// (Record.Reports), which starts now when none runs. It runs in the loop.
func (c *conv) takeReport(text string) {
	c.takeSteer(text, 0)
}

func (c *conv) takeSteer(text string, mark uint64) {
	if st, ok := c.proc.(reportSteerer); ok && c.turn && !c.rec.Lead && !c.replaying && !c.wrapHold {
		err := st.Steer(text)
		if err == nil {
			c.steerSeq++
			c.steered = append(c.steered, steeredReport{Text: text, Seq: c.steerSeq, Mark: mark})
			c.saveSteered()
			c.apply(wslive.Event{Kind: wslive.KindUser, Origin: wslive.OriginWhiteboard, Text: text})
			c.changed(false)
			return
		}
		fmt.Fprintf(c.r.opts.Log, "agent %s: steer a message: %v\n", c.rec.ReplyID, err)
	}
	c.update(func(rec *Record) { rec.Reports = append(rec.Reports, text) })
	c.kick()
	c.changed(false)
}

// steerAnswered takes the harness's answer to its next unanswered steer
// command: a refused report goes as a turn instead. With no turn running,
// the steered reports no run took go as the next turn once every one is
// answered (unsteer). It runs in the loop.
func (c *conv) steerAnswered(refusal string) {
	c.steerAnswers++
	if refusal != "" {
		for i, s := range c.steered {
			if s.Seq == c.steerAnswers {
				fmt.Fprintf(c.r.opts.Log, "agent %s: the harness refused a steered report (%s); it goes as a turn\n", c.rec.ReplyID, refusal)
				c.steered = slices.Delete(c.steered, i, i+1)
				c.saveSteered()
				c.update(func(rec *Record) { rec.Reports = append(rec.Reports, s.Text) })
				break
			}
		}
	}
	if !c.turn && !c.replaying {
		c.unsteer()
		c.kick()
	}
}

// steerTaken drops a steered message the running turn took and advances
// that turn's Send coverage. It runs in the loop.
func (c *conv) steerTaken(text string) {
	for i, s := range c.steered {
		if s.Text == text {
			c.turnMark = max(c.turnMark, s.Mark)
			c.steered = slices.Delete(c.steered, i, i+1)
			c.saveSteered()
			return
		}
	}
}

// unsteer moves the steered reports no run took to Record.Reports, for the
// next turn, once the turn ended and the harness answered each steer: it
// drops them from the harness's own queue first (ClearSteered), so each
// is sent once. A harness that cannot drop them is stopped, which drops
// them with it. Unanswered ones wait for their answer (steerAnswered). It
// runs in the loop.
func (c *conv) unsteer() {
	if len(c.steered) == 0 || c.turn {
		return
	}
	for _, s := range c.steered {
		if s.Seq > c.steerAnswers {
			return
		}
	}
	if st, ok := c.proc.(reportSteerer); ok {
		if err := st.ClearSteered(); err != nil {
			fmt.Fprintf(c.r.opts.Log, "agent %s: drop the steered reports the run did not take: %v\n", c.rec.ReplyID, err)
			c.stopProcess("")
		}
	}
	c.requeueSteered()
}

// requeueSteered moves every steered report to Record.Reports, for the
// next turn: the process ended, or the run ended without them. It reports
// whether there were any. It runs in the loop.
func (c *conv) requeueSteered() bool {
	if len(c.steered) == 0 {
		return false
	}
	texts := make([]string, 0, len(c.steered))
	for _, s := range c.steered {
		texts = append(texts, s.Text)
	}
	c.steered = nil
	c.saveSteered()
	c.update(func(rec *Record) { rec.Reports = append(rec.Reports, texts...) })
	return true
}

func (c *conv) saveSteered() {
	if c.rec.HolderID == "" && len(c.rec.Steered) == 0 && c.rec.TurnMark == c.turnMark {
		return
	}
	c.update(func(rec *Record) {
		rec.Steered = slices.Clone(c.steered)
		rec.SteerSeq, rec.TurnMark = c.steerSeq, c.turnMark
		rec.SteerMarks = nil
		for _, s := range c.steered {
			if s.Mark != 0 {
				if rec.SteerMarks == nil {
					rec.SteerMarks = map[int]uint64{}
				}
				rec.SteerMarks[s.Seq] = s.Mark
			}
		}
	})
}

// holdsReport reports whether a done turn of the conversation waits for
// its children: a child's report it holds and no turn took yet (steered
// and not taken, or kept for the next turn), or a child of its own that is
// open (openChildOf). It runs in the loop.
func (c *conv) holdsReport() bool {
	if len(c.steered) > 0 || len(c.rec.Reports) > 0 {
		return true
	}
	ref := ParentRef{Kind: ParentConversation, ReplyID: c.rec.ReplyID}
	c.r.mu.Lock()
	kids := make([]*conv, 0, len(c.r.convs))
	for _, k := range c.r.convs {
		if k != c {
			kids = append(kids, k)
		}
	}
	c.r.mu.Unlock()
	for _, k := range kids {
		if k.openChildOf(ref) {
			return true
		}
	}
	return false
}

// openChildOf reports whether the conversation is an open child of parent:
// a message was sent it that no settled turn covered yet and the page did
// not cancel (voidCancelled), a turn of it runs or is held for its own
// children, or a report of it waits to be handed over. A closed child is not open. A settle clears TurnOpen and
// adds the pending report in one record update (publish), so no read
// falls between them.
func (c *conv) openChildOf(parent ParentRef) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.rec.Parent == nil || *c.rec.Parent != parent {
		return false
	}
	return c.rec.TurnOpen || len(c.rec.PendingReports) > 0 || len(c.rec.Queued) > 0 || c.holding || c.sends > max(c.settled.mark, c.voided)
}

// releaseHeld reports the done turn held for the children (settleTurn)
// once nothing is left to wait for: a child closed with its report never
// sent. It runs in the loop.
func (c *conv) releaseHeld() {
	if c.heldRes == nil || c.turn || c.closed || c.holdsReport() {
		return
	}
	res := *c.heldRes
	c.hold(nil)
	c.publish(res)
}

// releaseLater releases conversation id's done turn held for its
// children (releaseHeld) off the caller's goroutine, which may be another
// conversation's loop: no loop waits on another's.
func (r *Runner) releaseLater(id string) {
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		if c := r.conv(id); c != nil {
			c.do(c.releaseHeld)
		}
	}()
}

// descendants are conversation id's open descendants through
// conversation parents, deepest first.
func (r *Runner) descendants(id string) []string {
	r.mu.Lock()
	byParent := map[string][]string{}
	for cid, c := range r.convs {
		c.mu.Lock()
		p := c.rec.Parent
		c.mu.Unlock()
		if p != nil && p.Kind == ParentConversation {
			byParent[p.ReplyID] = append(byParent[p.ReplyID], cid)
		}
	}
	r.mu.Unlock()
	var out []string
	seen := map[string]bool{id: true}
	var walk func(string)
	walk = func(parent string) {
		for _, cid := range byParent[parent] {
			if seen[cid] {
				continue
			}
			seen[cid] = true
			walk(cid)
			out = append(out, cid)
		}
	}
	walk(id)
	return out
}

// waitChecked, set by tests alone, runs in a wait after its check found no
// turn to answer it and before it blocks.
var waitChecked func()

// wait is Wait. asParent marks the wait as the conversation parent's own
// (the token route's): a turn that settles while it waits is handed to no
// other path (publish), and a settled turn it answers with takes that
// turn's pending report, so the wait's answer is the report's delivery.
// A turn whose report the runner already handed to the parent answers
// such a wait with its status alone, marked Delivered.
func (r *Runner) wait(ctx context.Context, id string, timeout time.Duration, asParent bool) (TurnResult, error) {
	r.mu.Lock()
	c, open := r.convs[id]
	_, closed := r.closed[id]
	r.mu.Unlock()
	switch {
	case closed:
		return TurnResult{ID: id, Status: TurnClosed}, nil
	case !open:
		return TurnResult{}, errUnknown
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	c.mu.Lock()
	target := c.sends
	if asParent {
		if c.parentWaits == nil {
			c.parentWaits = map[uint64]int{}
		}
		c.parentWaits[target]++
	}
	c.mu.Unlock()
	// settled is the turn that answers the wait, if one settled, else the
	// channel the next settle closes and whether the conversation ended.
	// One mu hold reads them all, so no settle falls between the check
	// and the channel. The wait's registration leaves in the same hold,
	// when a turn answers it or the wait is leaving, so a settle that
	// counted the wait is always the answer.
	settled := func(leaving bool) (res TurnResult, ok bool, next chan struct{}, ended bool) {
		c.mu.Lock()
		last, report := c.settled, c.settledReport
		next, ended = c.settleC, c.closed
		ok = last.n > 0 && last.mark >= target
		if asParent && (ok || leaving) {
			c.unwait(target)
		}
		c.mu.Unlock()
		if !ok {
			return TurnResult{}, false, next, ended
		}
		if asParent && report != "" {
			// The turn's report is this wait's to deliver only while it
			// still waits in the record: one the runner took meanwhile
			// already reached the parent, so the answer says so instead
			// of repeating it.
			handed := false
			c.update(func(rec *Record) {
				n := len(rec.PendingReports)
				rec.PendingReports = slices.DeleteFunc(rec.PendingReports, func(p PendingReport) bool { return p.ID == report })
				handed = len(rec.PendingReports) == n
			})
			if handed {
				last.Message, last.Error, last.Delivered = "", "", true
			}
		}
		return last, true, nil, false
	}
	end := func(status string) (TurnResult, error) {
		if res, ok, _, _ := settled(true); ok {
			return res, nil
		}
		return TurnResult{ID: id, Status: status}, nil
	}
	for {
		res, ok, ch, ended := settled(false)
		if ok {
			return res, nil
		}
		if ended {
			return end(TurnClosed)
		}
		if waitChecked != nil {
			waitChecked()
		}
		select {
		case <-ch:
		case <-c.done:
			if !c.isClosed() {
				return TurnResult{}, errDaemonDetached
			}
			return end(TurnClosed)
		case <-timer.C:
			return end(TurnRunning)
		case <-ctx.Done():
			return end(TurnRunning)
		}
	}
}

// unwait ends a conversation parent's wait for target. Under mu, once per
// wait: in the settled check that answers it, or in the one it leaves
// through.
func (c *conv) unwait(target uint64) {
	if c.parentWaits[target]--; c.parentWaits[target] <= 0 {
		delete(c.parentWaits, target)
	}
}

// parentWaitTakes reports whether a conversation parent's wait answers with
// a turn covering mark sends. Under mu.
func (c *conv) parentWaitTakes(mark uint64) bool {
	for target, n := range c.parentWaits {
		if n > 0 && target <= mark {
			return true
		}
	}
	return false
}

// ReportText is a settled turn's report as its parent reads it (a lead's
// completion mail, a Pi worker's steered message or turn): a head naming
// the child and how its turn ended, then its final assistant message.
func ReportText(rep TurnReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[Pi subagent %s", rep.Result.ID)
	if label := rep.Child.Label; label != "" {
		fmt.Fprintf(&b, " %q", label)
	}
	fmt.Fprintf(&b, "] turn %s", rep.Result.Status)
	if rep.Result.Error != "" {
		fmt.Fprintf(&b, ": %s", strings.Join(strings.Fields(rep.Result.Error), " "))
	}
	b.WriteString("\n")
	if rep.Result.Message != "" {
		b.WriteString("\n" + rep.Result.Message + "\n")
	} else if rep.Result.Status == TurnDone {
		b.WriteString("\n(no final message)\n")
	}
	return b.String()
}
