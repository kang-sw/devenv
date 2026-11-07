package wslive

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/kang-sw/devenv/internal/wsmailbox"
)

// Event kinds an adapter sends.
const (
	// KindReset clears the lead and the tree: an adapter sends it when it
	// starts, so state from an earlier host process never lingers.
	KindReset = "reset"
	// KindTurnStart and KindTurnEnd bound a lead turn. turn_end writes the
	// turn summary to the ring.
	KindTurnStart = "turn_start"
	KindTurnEnd   = "turn_end"
	// KindAgent updates the lead (no agentId) or one subagent: its batches,
	// caption, and, for a subagent, its parent, label, delegate session key,
	// and status.
	KindAgent = "agent"
	// KindPermission sets or (clear) ends a permission wait.
	KindPermission = "permission"
	// KindReply carries the lead's reply text so far; final moves it to the
	// ring.
	KindReply = "reply"
	// KindUser is a finished user message for the ring.
	KindUser = "user"
	// KindStatus replaces the session's usage figures (context, limits,
	// cost); a part it omits is shown as unknown. reset clears them too.
	KindStatus = "status"
	// KindCompact marks a main-conversation compaction: phase start shows
	// the lead compacting, and end or failed settles it into a ring entry.
	KindCompact = "compact"
)

// Compaction phases a compact event carries.
const (
	CompactStart  = "start"
	CompactEnd    = "end"
	CompactFailed = "failed"
)

// Compaction triggers: the host's own threshold or overflow compaction, or
// one a person (or a command acting for one) asked for.
const (
	TriggerAuto   = "auto"
	TriggerManual = "manual"
)

// OptionalKinds are the event kinds an older ws-mcp refuses: the intake
// result names them in an "accepts:" line, and an adapter sends one only
// while the latest result it received names it.
var OptionalKinds = []string{KindStatus, KindCompact}

// Event is one adapter event. Clocks are epoch milliseconds; an absent At
// is the intake time.
type Event struct {
	Kind string `json:"kind"`
	At   int64  `json:"at,omitempty"`
	// AgentID selects a subagent by its native agent id; empty is the lead.
	AgentID  string `json:"agentId,omitempty"`
	ParentID string `json:"parentId,omitempty"`
	Label    string `json:"label,omitempty"`
	// SessionKey is a delegate's ws session key. It is relabelled to the
	// delegate's reply-id and never stored.
	SessionKey string `json:"sessionKey,omitempty"`
	Status     string `json:"status,omitempty"`
	// Caption replaces the caption when present; "" clears it.
	Caption *string `json:"caption,omitempty"`
	// Batches replaces the agent's batch list when present.
	Batches []json.RawMessage `json:"batches,omitempty"`
	Tool    string            `json:"tool,omitempty"`
	Args    any               `json:"args,omitempty"`
	Clear   bool              `json:"clear,omitempty"`
	Text    string            `json:"text,omitempty"`
	Final   bool              `json:"final,omitempty"`
	Origin  string            `json:"origin,omitempty"`
	// Phase and Trigger are a compact event's: CompactStart, CompactEnd, or
	// CompactFailed, and TriggerAuto, TriggerManual, or empty when the host
	// names neither.
	Phase   string `json:"phase,omitempty"`
	Trigger string `json:"trigger,omitempty"`
	// Context, Limits, and Cost are a status event's figures. They are not
	// under a "status" key, which is an agent's status above.
	Context *UsageContext `json:"context,omitempty"`
	Limits  []UsageLimit  `json:"limits,omitempty"`
	Cost    *UsageCost    `json:"cost,omitempty"`
}

// Result is what one event changed.
type Result struct {
	// Changed reports a snapshot change; Urgent a change to the lead's phase
	// or streamed reply, which the writer follows on a shorter debounce.
	Changed bool
	Urgent  bool
	// Ring holds the conversation entries the event finished.
	Ring []RingEntry
}

type activityState struct {
	batches    []BatchView
	caption    string
	permission *Permission
	// counted holds the finished batches already added to done and failed,
	// keyed epoch/seq; it is pruned to the batches the adapter still lists
	// (more than the BatchCap kept in batches: see BatchWireCap).
	counted        map[string]bool
	done, failed   int
	phase          string
	phaseStartedAt int64
}

type agentState struct {
	activityState
	id, parentID, label, replyID, status string
	startedAt, endedAt, touched          int64
}

// State is one session's live state: the reducer from adapter events to a
// snapshot. It is not safe for concurrent use.
type State struct {
	replyID string
	adapter string
	scrub   scrubber

	lead           activityState
	turnStartedAt  int64
	turnOrigin     string
	previousTurnMs int64
	reply          *Reply
	// compactStartedAt is the running compaction's start, 0 when none runs;
	// compactTrigger is its trigger, which an end without one inherits.
	compactStartedAt int64
	compactTrigger   string

	// gap holds the lead's batches finished since its last conversation
	// entry (toolGap); flushTools moves it to the ring.
	gap toolGap

	agents map[string]*agentState
	usage  *Usage
	// evicted names the finished subagents retain dropped, oldest first (at
	// most evictedCap), so a late event for one does not bring it back as a
	// running agent (staleEvent).
	evicted []string
}

// evictedCap bounds State.evicted; an id forgotten past it is recreated by a
// late event, as before it was remembered.
const evictedCap = 256

// NewState is an empty state for the session holding sessionKey. The key
// names the session's reply-id and is scrubbed from every stored text.
func NewState(sessionKey string) *State {
	st := &State{replyID: wsmailbox.ReplyID(sessionKey), agents: map[string]*agentState{}}
	st.scrub.add(sessionKey)
	st.lead.phase = PhaseIdle
	return st
}

// ReplyID is the session's reply-id.
func (st *State) ReplyID() string { return st.replyID }

// SetAdapter records the adapter name and reports whether it changed.
func (st *State) SetAdapter(name string) bool {
	name = cleanLine(name, 32)
	if name == st.adapter {
		return false
	}
	st.adapter = name
	return true
}

var agentIDPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)

// Apply applies one event at now (epoch ms). An event that cannot apply
// returns an error and changes nothing.
func (st *State) Apply(ev Event, now int64) (Result, error) {
	at := ev.At
	if at <= 0 {
		at = now
	}
	if ev.AgentID != "" && !agentIDPattern.MatchString(ev.AgentID) {
		return Result{}, fmt.Errorf("agentId must be 1-128 letters, digits, or ._:-")
	}
	if ev.SessionKey != "" {
		st.scrub.add(ev.SessionKey)
	}
	leadPhase, leadReply := st.lead.phase, st.reply
	var res Result
	switch ev.Kind {
	case KindReset:
		*st = State{replyID: st.replyID, adapter: st.adapter, scrub: st.scrub, agents: map[string]*agentState{}}
		st.lead.phase = PhaseIdle
		res.Changed = true
	case KindTurnStart:
		origin, err := turnOrigin(ev.Origin)
		if err != nil {
			return Result{}, err
		}
		if st.reply != nil {
			st.flushTools(&res)
			res.Ring = append(res.Ring, st.finishReply(""))
		}
		st.turnStartedAt, st.turnOrigin = at, origin
		st.lead.done, st.lead.failed, st.lead.permission = 0, 0, nil
		// No adapter starts a turn while a compaction runs, so one still
		// marked running lost its end (the host failed under it): it stops
		// showing, with no ring entry, since how it ended is unknown.
		st.compactStartedAt, st.compactTrigger = 0, ""
		res.Changed = true
	case KindTurnEnd:
		if st.reply != nil {
			st.flushTools(&res)
			res.Ring = append(res.Ring, st.finishReply(""))
		}
		st.gapAdd(closeBatches(&st.lead, at))
		st.lead.permission = nil
		st.flushTools(&res)
		if st.turnStartedAt > 0 {
			duration := max(at-st.turnStartedAt, 0)
			done, failed := st.lead.totals()
			res.Ring = append(res.Ring, RingEntry{Kind: RingTurn, Origin: st.turnOrigin, Done: done, Failed: failed, DurationMs: duration})
			st.previousTurnMs = duration
		}
		st.turnStartedAt, st.turnOrigin = 0, ""
		res.Changed = true
	case KindAgent:
		if st.staleEvent(ev) {
			return res, nil
		}
		if err := st.applyAgent(ev, at); err != nil {
			return Result{}, err
		}
		res.Changed = true
	case KindPermission:
		if !ev.Clear && strings.TrimSpace(ev.Tool) == "" {
			return Result{}, fmt.Errorf("permission needs tool, or clear: true")
		}
		if st.staleEvent(ev) {
			return res, nil
		}
		act, err := st.activityFor(ev.AgentID, at)
		if err != nil {
			return Result{}, err
		}
		if ev.Clear {
			act.permission = nil
		} else {
			act.permission = &Permission{ToolName: cleanToolName(ev.Tool), ArgsPreview: cleanLine(st.scrub.scrub(flattenArgs(ev.Args)), ArgsPreviewCap), StartedAt: at}
		}
		res.Changed = true
	case KindReply:
		if ev.AgentID != "" {
			return Result{}, fmt.Errorf("reply is the lead's only; omit agentId")
		}
		if ev.Final {
			if ev.Text != "" || st.reply != nil {
				st.flushTools(&res)
				res.Ring = append(res.Ring, st.finishReply(ev.Text))
			}
		} else {
			text, truncated := capText(st.scrub.scrub(ev.Text))
			started := at
			if st.reply != nil {
				started = st.reply.StartedAt
			}
			st.reply = &Reply{Text: text, StartedAt: started, Truncated: truncated}
		}
		res.Changed = true
	case KindUser:
		origin := strings.TrimSpace(ev.Origin)
		switch origin {
		case "":
			origin = OriginTerminal
		case OriginTerminal, OriginWhiteboard:
		default:
			return Result{}, fmt.Errorf("user origin must be %q or %q", OriginTerminal, OriginWhiteboard)
		}
		text, truncated := capText(st.scrub.scrub(ev.Text))
		st.flushTools(&res)
		res.Ring = append(res.Ring, RingEntry{Kind: RingUser, Origin: origin, Text: text, Truncated: truncated})
	case KindStatus:
		if ev.AgentID != "" {
			return Result{}, fmt.Errorf("status is the session's only; omit agentId")
		}
		usage := normalizeUsage(ev)
		res.Changed = !reflect.DeepEqual(usage, st.usage)
		st.usage = usage
	case KindCompact:
		if ev.AgentID != "" {
			return Result{}, fmt.Errorf("compact is the main conversation's only; omit agentId")
		}
		trigger := strings.TrimSpace(ev.Trigger)
		switch trigger {
		case "", TriggerAuto, TriggerManual:
		default:
			return Result{}, fmt.Errorf("compact trigger must be %q or %q", TriggerAuto, TriggerManual)
		}
		switch ev.Phase {
		case CompactStart:
			st.compactStartedAt, st.compactTrigger = at, trigger
		case CompactEnd, CompactFailed:
			if trigger == "" {
				trigger = st.compactTrigger
			}
			st.flushTools(&res)
			res.Ring = append(res.Ring, RingEntry{Kind: RingCompact, Trigger: trigger, CompactFailed: ev.Phase == CompactFailed})
			st.compactStartedAt, st.compactTrigger = 0, ""
		default:
			return Result{}, fmt.Errorf("compact phase must be %q, %q, or %q", CompactStart, CompactEnd, CompactFailed)
		}
		res.Changed = true
	case "":
		return Result{}, fmt.Errorf("event kind is required")
	default:
		return Result{}, fmt.Errorf("unknown event kind %q", ev.Kind)
	}
	st.refreshPhases(at)
	st.retain()
	if st.lead.phase != leadPhase || st.reply != leadReply {
		res.Urgent = true
	}
	return res, nil
}

func turnOrigin(origin string) (string, error) {
	switch origin = strings.TrimSpace(origin); origin {
	case "", OriginTerminal, OriginWhiteboard, OriginMail:
		return origin, nil
	default:
		return "", fmt.Errorf("turn origin must be %q, %q, or %q", OriginTerminal, OriginWhiteboard, OriginMail)
	}
}

// finishReply ends the streamed reply: text (or, when empty, the text
// streamed so far) becomes a ring entry.
func (st *State) finishReply(text string) RingEntry {
	entry := RingEntry{Kind: RingReply}
	if text != "" {
		entry.Text, entry.Truncated = capText(st.scrub.scrub(text))
	} else if st.reply != nil {
		entry.Text, entry.Truncated = st.reply.Text, st.reply.Truncated
	}
	st.reply = nil
	return entry
}

// staleEvent reports whether ev, an agent or permission event, names a
// subagent the state does not hold and must not create: a permission clear
// (nothing to clear), or an agent event short of status running for a
// finished subagent retain already dropped. Either would otherwise recreate
// the subagent as running, a phantom until the next reset. An explicit
// running status brings the subagent back.
func (st *State) staleEvent(ev Event) bool {
	if ev.AgentID == "" {
		return false
	}
	if _, ok := st.agents[ev.AgentID]; ok {
		return false
	}
	if ev.Kind == KindPermission {
		return ev.Clear
	}
	return ev.Status != StatusRunning && slices.Contains(st.evicted, ev.AgentID)
}

// activityFor is the lead's activity for an empty id, else the subagent's,
// created running when new.
func (st *State) activityFor(id string, at int64) (*activityState, error) {
	if id == "" {
		return &st.lead, nil
	}
	return &st.agent(id, at).activityState, nil
}

func (st *State) agent(id string, at int64) *agentState {
	a, ok := st.agents[id]
	if !ok {
		a = &agentState{id: id, status: StatusRunning, startedAt: at}
		st.agents[id] = a
		st.evicted = slices.DeleteFunc(st.evicted, func(e string) bool { return e == id })
	}
	a.touched = at
	return a
}

func (st *State) applyAgent(ev Event, at int64) error {
	var batches []BatchView
	if ev.Batches != nil {
		batches = st.normalizeBatches(ev.Batches)
	}
	if ev.AgentID == "" {
		if ev.Status != "" || ev.ParentID != "" || ev.Label != "" || ev.SessionKey != "" {
			return fmt.Errorf("the lead takes only batches and caption; set agentId for a subagent")
		}
		st.setActivity(&st.lead, ev, batches)
		return nil
	}
	switch ev.Status {
	case "", StatusRunning, StatusDone, StatusFailed:
	default:
		return fmt.Errorf("agent status must be %q, %q, or %q", StatusRunning, StatusDone, StatusFailed)
	}
	a := st.agent(ev.AgentID, at)
	if ev.ParentID != "" && ev.ParentID != ev.AgentID && agentIDPattern.MatchString(ev.ParentID) {
		a.parentID = ev.ParentID
	}
	if ev.Label != "" {
		a.label = cleanLine(st.scrub.scrub(ev.Label), LabelRunes)
	}
	if ev.SessionKey != "" {
		a.replyID = wsmailbox.ReplyID(strings.TrimSpace(ev.SessionKey))
	}
	st.setActivity(&a.activityState, ev, batches)
	switch ev.Status {
	case StatusRunning:
		a.status, a.endedAt = StatusRunning, 0
	case StatusDone, StatusFailed:
		if a.status == StatusRunning {
			a.endedAt = at
		}
		a.status = ev.Status
		a.permission = nil
		closeBatches(&a.activityState, at)
	}
	return nil
}

func (st *State) setActivity(act *activityState, ev Event, batches []BatchView) {
	if ev.Batches != nil {
		fresh := act.setBatches(batches)
		if act == &st.lead {
			st.gapAdd(fresh)
		}
	}
	if ev.Caption != nil {
		act.caption = cleanLine(st.scrub.scrub(*ev.Caption), CaptionRunes)
	}
}

func batchKey(b BatchView) string {
	return b.Epoch + "/" + strconv.FormatInt(b.Seq, 10)
}

// setBatches replaces the batch list, adding every newly finished batch to
// the done and failed totals once, and returns those batches. The whole list
// is counted; only its newest BatchCap are kept.
func (act *activityState) setBatches(batches []BatchView) []BatchView {
	fresh := act.countFinished(batches, true)
	if len(batches) > BatchCap {
		batches = batches[len(batches)-BatchCap:]
	}
	act.batches = batches
	return fresh
}

// gapAdd adds the lead's newly finished batches to the open tool gap.
func (st *State) gapAdd(fresh []BatchView) {
	for _, b := range fresh {
		st.gap.add(b)
	}
}

// flushTools ends the open tool gap into res.Ring, ahead of the
// conversation entry the caller writes next. An empty gap adds nothing.
func (st *State) flushTools(res *Result) {
	if e := st.gap.flush(); e != nil {
		res.Ring = append(res.Ring, *e)
	}
}

// FlushTools ends the open tool gap into a ring entry, for a conversation
// entry the state does not write itself (a mail line): the caller appends it
// before that entry. It returns nil when the gap is empty.
func (st *State) FlushTools() []RingEntry {
	var res Result
	st.flushTools(&res)
	return res.Ring
}

// countFinished counts each newly finished batch of list once and returns
// them in list order. With prune, list is the adapter's whole list and the
// batches it no longer holds are forgotten: an adapter drops a finished
// batch only after a delivered call carried it, so a forgotten one never
// comes back to be counted again.
func (act *activityState) countFinished(list []BatchView, prune bool) []BatchView {
	var fresh []BatchView
	if act.counted == nil {
		act.counted = map[string]bool{}
	}
	listed := make(map[string]bool, len(list))
	for _, b := range list {
		key := batchKey(b)
		listed[key] = true
		if b.EndedAt > 0 && !act.counted[key] {
			act.counted[key] = true
			act.done += b.CallCount - b.Failed
			act.failed += b.Failed
			fresh = append(fresh, b)
		}
	}
	if prune {
		for key := range act.counted {
			if !listed[key] {
				delete(act.counted, key)
			}
		}
	}
	return fresh
}

// totals is done and failed including the running batch's finished calls.
func (act *activityState) totals() (int, int) {
	done, failed := act.done, act.failed
	for _, b := range act.batches {
		if b.Running != nil {
			done += max(b.Running.Done-b.Failed, 0)
			failed += b.Failed
		}
	}
	return done, failed
}

// closeBatches ends a running batch and drops a generating message, as the
// host batch model does at a turn's or an agent's end: calls still
// unfinished count as failed. It returns the batches it newly finished.
func closeBatches(act *activityState, at int64) []BatchView {
	kept := act.batches[:0]
	for _, b := range act.batches {
		if b.Generating != nil {
			continue
		}
		if b.Running != nil {
			b.Failed = min(b.Failed+max(b.CallCount-b.Running.Done, 0), b.CallCount)
			b.Running, b.EndedAt = nil, max(at, b.StartedAt)
		}
		kept = append(kept, b)
	}
	act.batches = kept
	return act.countFinished(kept, false)
}

// refreshPhases recomputes each phase, restarting its clock on a change.
func (st *State) refreshPhases(at int64) {
	lead := derivePhase(&st.lead)
	if st.compactStartedAt > 0 && lead != PhasePermission {
		lead = PhaseCompacting
	}
	if lead == "" {
		switch {
		case st.reply != nil:
			lead = PhaseReplying
		case st.turnStartedAt > 0:
			lead = PhaseThinking
		default:
			lead = PhaseIdle
		}
	}
	st.lead.setPhase(lead, at)
	if lead == PhaseCompacting {
		st.lead.phaseStartedAt = st.compactStartedAt
	}
	for _, a := range st.agents {
		phase := derivePhase(&a.activityState)
		if phase == "" {
			phase = PhaseIdle
			if a.status == StatusRunning {
				phase = PhaseThinking
			}
		}
		a.setPhase(phase, at)
	}
}

// derivePhase is the phase the activity shows by itself, or "" when it
// shows none.
func derivePhase(act *activityState) string {
	if act.permission != nil {
		return PhasePermission
	}
	if n := len(act.batches); n > 0 {
		last := act.batches[n-1]
		if last.Generating != nil {
			return last.Generating.Kind
		}
		if last.Running != nil {
			return PhaseTool
		}
	}
	return ""
}

func (act *activityState) setPhase(phase string, at int64) {
	if phase == act.phase {
		return
	}
	act.phase, act.phaseStartedAt = phase, at
	if n := len(act.batches); n > 0 && act.batches[n-1].Generating != nil && act.batches[n-1].Generating.Kind == phase {
		act.phaseStartedAt = act.batches[n-1].Generating.PhaseStartedAt
	}
	if phase == PhasePermission {
		act.phaseStartedAt = act.permission.StartedAt
	}
}

// retain keeps every running agent (up to RunningAgentCap, least recently
// updated dropped first) and the FinishedAgentCap most recently finished.
func (st *State) retain() {
	var running, finished []*agentState
	for _, a := range st.agents {
		if a.status == StatusRunning {
			running = append(running, a)
		} else {
			finished = append(finished, a)
		}
	}
	drop := func(list []*agentState, keep int, newer func(a, b *agentState) bool) {
		if len(list) <= keep {
			return
		}
		sort.Slice(list, func(i, j int) bool { return newer(list[i], list[j]) })
		for _, a := range list[keep:] {
			delete(st.agents, a.id)
			if a.status != StatusRunning {
				st.evicted = append(st.evicted, a.id)
			}
		}
	}
	drop(running, RunningAgentCap, func(a, b *agentState) bool {
		if a.touched != b.touched {
			return a.touched > b.touched
		}
		return a.id < b.id
	})
	drop(finished, FinishedAgentCap, func(a, b *agentState) bool {
		if a.endedAt != b.endedAt {
			return a.endedAt > b.endedAt
		}
		return a.id < b.id
	})
	if over := len(st.evicted) - evictedCap; over > 0 {
		st.evicted = slices.Delete(st.evicted, 0, over)
	}
}

// Snapshot is the state as a snapshot, stamped with now (epoch ms).
func (st *State) Snapshot(now int64) Snapshot {
	snap := Snapshot{
		SchemaVersion: SchemaVersion,
		ReplyID:       st.replyID,
		Adapter:       st.adapter,
		UpdatedAt:     now,
		HeartbeatMs:   HeartbeatInterval.Milliseconds(),
		Lead: Lead{
			Activity:       st.lead.view(),
			TurnStartedAt:  st.turnStartedAt,
			TurnOrigin:     st.turnOrigin,
			PreviousTurnMs: st.previousTurnMs,
		},
	}
	if st.reply != nil {
		reply := *st.reply
		snap.Lead.Reply = &reply
	}
	snap.Lead.ToolGap = st.gap.summary()
	snap.Usage = st.usage.clone()
	for _, a := range st.agents {
		snap.Agents = append(snap.Agents, Agent{
			Activity:  a.view(),
			ID:        a.id,
			ParentID:  a.parentID,
			Label:     a.label,
			ReplyID:   a.replyID,
			Status:    a.status,
			StartedAt: a.startedAt,
			EndedAt:   a.endedAt,
		})
	}
	sort.Slice(snap.Agents, func(i, j int) bool {
		if snap.Agents[i].StartedAt != snap.Agents[j].StartedAt {
			return snap.Agents[i].StartedAt < snap.Agents[j].StartedAt
		}
		return snap.Agents[i].ID < snap.Agents[j].ID
	})
	return snap
}

func (act *activityState) view() Activity {
	done, failed := act.totals()
	out := Activity{Phase: act.phase, PhaseStartedAt: act.phaseStartedAt, Done: done, Failed: failed, Caption: act.caption}
	if len(act.batches) > 0 {
		out.Batches = make([]BatchView, len(act.batches))
		for i, b := range act.batches {
			out.Batches[i] = b.clone()
		}
	}
	if act.permission != nil {
		p := *act.permission
		out.Permission = &p
	}
	return out
}

func (b BatchView) clone() BatchView {
	b.ToolNames = append([]string(nil), b.ToolNames...)
	b.Calls = append([]BatchCall(nil), b.Calls...)
	if b.Usage != nil {
		u := *b.Usage
		b.Usage = &u
	}
	if b.Running != nil {
		r := *b.Running
		b.Running = &r
	}
	if b.Generating != nil {
		g := *b.Generating
		b.Generating = &g
	}
	return b
}

// batchIn is a batch as an adapter sends it; pointers tell absent from zero.
type batchIn struct {
	Epoch      string          `json:"epoch"`
	Seq        int64           `json:"seq"`
	ToolNames  []any           `json:"toolNames"`
	CallCount  int             `json:"callCount"`
	Failed     int             `json:"failed"`
	StartedAt  int64           `json:"startedAt"`
	EndedAt    *int64          `json:"endedAt"`
	Running    *RunningView    `json:"running"`
	Generating *GeneratingView `json:"generating"`
	// Usage and Calls are optional, and parsed apart so a malformed one is
	// dropped alone and never takes the batch with it.
	Usage json.RawMessage   `json:"usage"`
	Calls []json.RawMessage `json:"calls"`
}

// callIn is one entry of a batch's calls as an adapter sends it.
type callIn struct {
	Name      string `json:"name"`
	StartedAt int64  `json:"startedAt"`
	EndedAt   int64  `json:"endedAt"`
	Failed    bool   `json:"failed"`
}

var epochPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`)

// normalizeBatches validates and bounds an adapter's batch list: a
// malformed batch is dropped, names and previews are cleaned and capped, and
// only the newest BatchWireCap are kept (setBatches keeps BatchCap of them).
func (st *State) normalizeBatches(raw []json.RawMessage) []BatchView {
	out := []BatchView{}
	for _, item := range raw {
		if b, ok := st.normalizeBatch(item); ok {
			out = append(out, b)
		}
	}
	if len(out) > BatchWireCap {
		out = out[len(out)-BatchWireCap:]
	}
	return out
}

func (st *State) normalizeBatch(raw json.RawMessage) (BatchView, bool) {
	var in batchIn
	if json.Unmarshal(raw, &in) != nil {
		return BatchView{}, false
	}
	if in.Seq <= 0 || in.CallCount < 0 || in.Failed < 0 || in.Failed > in.CallCount || in.StartedAt < 0 {
		return BatchView{}, false
	}
	present := 0
	for _, set := range []bool{in.EndedAt != nil, in.Running != nil, in.Generating != nil} {
		if set {
			present++
		}
	}
	if present != 1 {
		return BatchView{}, false
	}
	out := BatchView{Seq: in.Seq, ToolNames: []string{}, CallCount: in.CallCount, Failed: in.Failed, StartedAt: in.StartedAt}
	if epochPattern.MatchString(in.Epoch) {
		out.Epoch = in.Epoch
	}
	for _, name := range in.ToolNames {
		s, ok := name.(string)
		if !ok || strings.TrimSpace(s) == "" || len(out.ToolNames) >= ToolNameCap {
			continue
		}
		s = cleanToolName(st.scrub.scrub(s))
		dup := false
		for _, have := range out.ToolNames {
			dup = dup || have == s
		}
		if !dup {
			out.ToolNames = append(out.ToolNames, s)
		}
	}
	switch {
	case in.EndedAt != nil:
		if *in.EndedAt <= 0 {
			return BatchView{}, false
		}
		out.EndedAt = *in.EndedAt
		out.Calls = st.normalizeCalls(in.Calls, in.CallCount)
		out.Usage = normalizeTokens(in.Usage)
	case in.Running != nil:
		r := *in.Running
		if r.Done < 0 || r.OthersRunning < 0 || r.StartedAt < 0 {
			return BatchView{}, false
		}
		r.ToolName = cleanToolName(r.ToolName)
		r.ArgsPreview = cleanLine(st.scrub.scrub(r.ArgsPreview), ArgsPreviewCap)
		out.Running = &r
	default:
		g := *in.Generating
		switch g.Kind {
		case PhaseThinking, PhaseReplying:
			g.ToolName, g.Chars = "", 0
		case PhaseTool:
			if g.Chars < 0 {
				return BatchView{}, false
			}
			g.ToolName = cleanToolName(g.ToolName)
		default:
			return BatchView{}, false
		}
		if g.PhaseStartedAt < 0 {
			return BatchView{}, false
		}
		out.Generating = &g
	}
	return out, true
}

// normalizeCalls is a finished batch's per-call entries: a malformed entry is
// dropped, names are scrubbed and capped like toolNames, unknown or
// inconsistent clocks read 0, and at most BatchCallCap (and never more than
// the batch's callCount) are kept. nil when none is left.
func (st *State) normalizeCalls(raw []json.RawMessage, callCount int) []BatchCall {
	var out []BatchCall
	for _, item := range raw {
		if len(out) >= min(BatchCallCap, callCount) {
			break
		}
		var in callIn
		if json.Unmarshal(item, &in) != nil || strings.TrimSpace(in.Name) == "" {
			continue
		}
		c := BatchCall{Name: cleanToolName(st.scrub.scrub(in.Name)), StartedAt: max(in.StartedAt, 0), Failed: in.Failed}
		if in.EndedAt >= c.StartedAt {
			c.EndedAt = in.EndedAt
		}
		out = append(out, c)
	}
	return out
}

// normalizeTokens is a batch's usage: negative counts read 0, and nil when
// absent, malformed, or all zero.
func normalizeTokens(raw json.RawMessage) *TokenUsage {
	if len(raw) == 0 {
		return nil
	}
	var in TokenUsage
	if json.Unmarshal(raw, &in) != nil {
		return nil
	}
	u := TokenUsage{Input: max(in.Input, 0), Output: max(in.Output, 0), CacheRead: max(in.CacheRead, 0), CacheWrite: max(in.CacheWrite, 0)}
	if u == (TokenUsage{}) {
		return nil
	}
	return &u
}

// normalizeUsage is a status event's figures as the snapshot keeps them:
// negative or out-of-range numbers read as unknown, a limit of another kind
// is dropped (the last of a kind wins, the 5-hour limit first), and nil when
// nothing is left.
func normalizeUsage(ev Event) *Usage {
	u := &Usage{}
	if c := ev.Context; c != nil {
		ctx := &UsageContext{Window: max(c.Window, 0)}
		if c.Tokens != nil && *c.Tokens >= 0 {
			tokens := *c.Tokens
			ctx.Tokens = &tokens
		}
		switch {
		case ctx.Tokens == nil:
		case c.Percent != nil && *c.Percent >= 0:
			pct := *c.Percent
			ctx.Percent = &pct
		case ctx.Window > 0:
			pct := float64(*ctx.Tokens) / float64(ctx.Window) * 100
			ctx.Percent = &pct
		}
		if c.CacheHit != nil && *c.CacheHit >= 0 && *c.CacheHit <= 100 {
			hit := *c.CacheHit
			ctx.CacheHit = &hit
		}
		if ctx.Window > 0 || ctx.Tokens != nil {
			u.Context = ctx
		}
	}
	byKind := map[string]UsageLimit{}
	for _, l := range ev.Limits {
		if (l.Kind != LimitFiveHour && l.Kind != LimitSevenDay) || l.Percent < 0 {
			continue
		}
		byKind[l.Kind] = UsageLimit{Kind: l.Kind, Percent: l.Percent, ResetsAt: max(l.ResetsAt, 0)}
	}
	for _, kind := range []string{LimitFiveHour, LimitSevenDay} {
		if l, ok := byKind[kind]; ok {
			u.Limits = append(u.Limits, l)
		}
	}
	if c := ev.Cost; c != nil && c.USD >= 0 {
		cost := *c
		cost.Incomplete = cost.Incomplete && cost.Estimate
		u.Cost = &cost
	}
	if u.Context == nil && u.Limits == nil && u.Cost == nil {
		return nil
	}
	return u
}

// clone is a deep copy, so a snapshot never shares the state's pointers.
func (u *Usage) clone() *Usage {
	if u == nil {
		return nil
	}
	out := &Usage{Limits: append([]UsageLimit(nil), u.Limits...)}
	if u.Context != nil {
		c := *u.Context
		if c.Tokens != nil {
			v := *c.Tokens
			c.Tokens = &v
		}
		if c.Percent != nil {
			v := *c.Percent
			c.Percent = &v
		}
		if c.CacheHit != nil {
			v := *c.CacheHit
			c.CacheHit = &v
		}
		out.Context = &c
	}
	if u.Cost != nil {
		c := *u.Cost
		out.Cost = &c
	}
	return out
}
