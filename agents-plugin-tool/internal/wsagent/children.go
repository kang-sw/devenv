package wsagent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wslive"
)

// children.go is the spawn tree under a lead: a spawned Pi worker spawns
// Pi children of its own through the daemon, which owns the whole tree.
//
//   - Token. A conversation started through Runner.Spawn with a recorded
//     parent and tier, whose profile allows a pi.* tool, gets a capability
//     token in its attached ws-mcp's environment (EnvPiToken, beside the
//     daemon's loopback base URL in EnvPiEndpoint). The token is drawn at
//     every process start, never persisted, and void once the process ends
//     or the conversation closes. Its key stays a parentless delegate key:
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
	tokenWaitMax     = 1800 * time.Second
)

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
	// Cwd is absolute, or relative to the holder's root; empty is that
	// root.
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
	cwd := strings.TrimSpace(req.Cwd)
	switch {
	case cwd == "":
		cwd = rec.Root
	case !filepath.IsAbs(cwd):
		cwd = filepath.Join(rec.Root, cwd)
	}
	label := req.Label
	if strings.TrimSpace(label) == "" {
		label = req.Prompt
	}
	st, err := r.Spawn(SpawnSpec{
		Profile: PiWorkerProfile,
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

// steeredReport is a child's report steered into the running turn: seq is
// its steer command's place among the process's (steerSeq).
type steeredReport struct {
	text string
	seq  int
}

// takeReport takes a child's report into the conversation: steered into
// the running turn when its harness takes a message mid-turn and tells
// which it took (reportSteerer), else kept for its next turn
// (Record.Reports), which starts now when none runs. It runs in the loop.
func (c *conv) takeReport(text string) {
	if st, ok := c.proc.(reportSteerer); ok && c.turn && !c.rec.Lead && !c.replaying && !c.wrapHold {
		err := st.Steer(text)
		if err == nil {
			c.steerSeq++
			c.steered = append(c.steered, steeredReport{text: text, seq: c.steerSeq})
			c.apply(wslive.Event{Kind: wslive.KindUser, Origin: wslive.OriginWhiteboard, Text: text})
			c.changed(false)
			return
		}
		fmt.Fprintf(c.r.opts.Log, "agent %s: steer a child's report: %v\n", c.rec.ReplyID, err)
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
			if s.seq == c.steerAnswers {
				fmt.Fprintf(c.r.opts.Log, "agent %s: the harness refused a steered report (%s); it goes as a turn\n", c.rec.ReplyID, refusal)
				c.steered = slices.Delete(c.steered, i, i+1)
				c.update(func(rec *Record) { rec.Reports = append(rec.Reports, s.text) })
				break
			}
		}
	}
	if !c.turn {
		c.unsteer()
		c.kick()
	}
}

// steerTaken drops a steered report the running turn took. It runs in the
// loop.
func (c *conv) steerTaken(text string) {
	for i, s := range c.steered {
		if s.text == text {
			c.steered = slices.Delete(c.steered, i, i+1)
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
		if s.seq > c.steerAnswers {
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
		texts = append(texts, s.text)
	}
	c.steered = nil
	c.update(func(rec *Record) { rec.Reports = append(rec.Reports, texts...) })
	return true
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
// a message was sent it that no settled turn covered yet (one held for its
// own children included), a turn of it runs, or a report of it waits to be
// handed over. A closed child is not open. A settle clears TurnOpen and
// adds the pending report in one record update (publish), so no read
// falls between them.
func (c *conv) openChildOf(parent ParentRef) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.rec.Parent == nil || *c.rec.Parent != parent {
		return false
	}
	return c.rec.TurnOpen || len(c.rec.PendingReports) > 0 || len(c.rec.Queued) > 0 || c.sends > c.settled.mark
}

// releaseHeld reports the done turn held for the children (settleTurn)
// once nothing is left to wait for: a child closed with its report never
// sent. It runs in the loop.
func (c *conv) releaseHeld() {
	if c.heldRes == nil || c.turn || c.closed || c.holdsReport() {
		return
	}
	res := *c.heldRes
	c.heldRes = nil
	c.publish(res)
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

// wait is Wait. asParent marks the wait as the conversation parent's own
// (the token route's): a turn that settles while it waits is handed to no
// other path (publish), and a settled turn it answers with takes that
// turn's pending report, so the wait's answer is the report's delivery.
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
	// settled is the turn that answers the wait, if one settled: the
	// wait's registration leaves it under mu, so a settle that counted
	// the wait is found here however the wait ended.
	settled := func() (TurnResult, bool) {
		c.mu.Lock()
		last, report := c.settled, c.settledReport
		if asParent && last.n > 0 && last.mark >= target {
			c.unwait(target)
		}
		c.mu.Unlock()
		if last.n == 0 || last.mark < target {
			return TurnResult{}, false
		}
		if asParent && report != "" {
			c.update(func(rec *Record) {
				rec.PendingReports = slices.DeleteFunc(rec.PendingReports, func(p PendingReport) bool { return p.ID == report })
			})
		}
		return last, true
	}
	end := func(status string) (TurnResult, error) {
		if res, ok := settled(); ok {
			return res, nil
		}
		if asParent {
			c.mu.Lock()
			c.unwait(target)
			c.mu.Unlock()
		}
		return TurnResult{ID: id, Status: status}, nil
	}
	for {
		if res, ok := settled(); ok {
			return res, nil
		}
		c.mu.Lock()
		ch, ended := c.settleC, c.closed
		c.mu.Unlock()
		if ended {
			return end(TurnClosed)
		}
		select {
		case <-ch:
		case <-c.done:
			return end(TurnClosed)
		case <-timer.C:
			return end(TurnRunning)
		case <-ctx.Done():
			return end(TurnRunning)
		}
	}
}

// unwait ends a conversation parent's wait for target. Under mu, once per
// wait: a wait leaves through settled or end, never both.
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
