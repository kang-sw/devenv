package wslive

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/kang-sw/devenv/internal/wsmailbox"
	"github.com/kang-sw/devenv/internal/wsredact"
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
	// status, and telemetry (model, effort, context tokens, output rate).
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
	// KindCommands replaces the slash commands the session's adapter can
	// run for the page (Commands); an empty list clears them, and so does
	// reset, so an adapter re-sends its list after a reset.
	KindCommands = "commands"
	// KindCommand is a slash command the adapter ran from a page command
	// entry: Command its typed line, Text the output it showed as text
	// (empty when it showed none). It writes a command ring entry.
	KindCommand = "command"
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
var OptionalKinds = []string{KindStatus, KindCompact, KindCommands, KindCommand}

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
	// AgentKind marks what runs a subagent when it is not the host's own
	// (AgentKindPi); it is a subagent's alone and, once set, stays. The
	// event type is Kind, under "kind", so the marker has a key of its own.
	AgentKind string `json:"agentKind,omitempty"`
	Status    string `json:"status,omitempty"`
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
	// Model, Effort, ContextTokens, and OutputTps are a subagent's
	// telemetry on an agent event: each replaces the held value when
	// present and keeps it when absent. A blank text or a negative or
	// out-of-range number is ignored. Numbers are floats so a fractional
	// count never fails the whole event.
	Model         string   `json:"model,omitempty"`
	Effort        string   `json:"effort,omitempty"`
	ContextTokens *float64 `json:"contextTokens,omitempty"`
	OutputTps     *float64 `json:"outputTps,omitempty"`
	// Commands is a commands event's list. Command is a command event's
	// typed line ("/name args"), its output in Text.
	Commands []SlashCommand `json:"commands,omitempty"`
	Command  string         `json:"command,omitempty"`
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
	kind                                 string
	startedAt, endedAt, touched          int64
	model, effort                        string
	contextTokens                        int64
	outputTps                            float64
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
	// spawnAliases groups native/daemon producer ids for retention. A native
	// alias cannot revive an evicted daemon lifecycle; only the daemon can.
	spawnAliases map[string]string
	// spawnReplies preserves delegate identity across the same eviction window,
	// including a native producer first seen after canonical eviction.
	spawnReplies map[string]string
	usage        *Usage
	// commands is the adapter's slash command list (KindCommands).
	commands []SlashCommand
	// host is what a host that runs the harness itself reports (SetHost);
	// an adapter's reset keeps it.
	host *Host
	// batchesEnded counts the tool batches finished so far, the lead's and
	// every subagent's (BatchesEnded).
	batchesEnded uint64
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

// BatchesEnded counts the tool batches finished so far, the lead's and
// every subagent's: it moves whenever a tool call may have changed the
// working tree, and never moves back (a reset keeps it).
func (st *State) BatchesEnded() uint64 { return st.batchesEnded }

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
		*st = State{replyID: st.replyID, adapter: st.adapter, scrub: st.scrub, agents: map[string]*agentState{}, batchesEnded: st.batchesEnded, host: st.host}
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
		fresh := closeBatches(&st.lead, at)
		st.batchesEnded += uint64(len(fresh))
		st.gapAdd(fresh)
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
			act.permission = &Permission{ToolName: cleanToolName(ev.Tool), ArgsPreview: st.argsPreview(flattenArgs(ev.Args)), StartedAt: at}
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
			entry := RingEntry{Kind: RingCompact, Trigger: trigger, CompactFailed: ev.Phase == CompactFailed}
			if entry.CompactFailed {
				// A failed compaction's own error text, when the host has it.
				entry.Text = cleanLine(st.scrub.scrub(ev.Text), NoticeTextRunes)
			}
			res.Ring = append(res.Ring, entry)
			st.compactStartedAt, st.compactTrigger = 0, ""
		default:
			return Result{}, fmt.Errorf("compact phase must be %q, %q, or %q", CompactStart, CompactEnd, CompactFailed)
		}
		res.Changed = true
	case KindCommands:
		if ev.AgentID != "" {
			return Result{}, fmt.Errorf("commands are the session's only; omit agentId")
		}
		commands := normalizeCommands(ev.Commands)
		res.Changed = !reflect.DeepEqual(commands, st.commands)
		st.commands = commands
	case KindCommand:
		if ev.AgentID != "" {
			return Result{}, fmt.Errorf("command is the main conversation's only; omit agentId")
		}
		line := cleanLine(st.scrub.scrub(ev.Command), CommandLineRunes)
		if !strings.HasPrefix(line, "/") || len(line) < 2 {
			return Result{}, fmt.Errorf("command must be the typed line, /name or /name args")
		}
		text, truncated := capText(st.scrub.scrub(ev.Text))
		st.flushTools(&res)
		res.Ring = append(res.Ring, RingEntry{Kind: RingCommand, Command: line, Text: text, Truncated: truncated})
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

// CardTitleRunes caps a card line's title.
const CardTitleRunes = 200

// Card applies a tracker card reference the lead showed at now: the reply
// streamed so far and the open tool gap are written first, so the card
// follows the text and the calls that led to it. It is not an adapter event
// kind: only a host that reads tool results itself (the agent runner) shows
// cards.
func (st *State) Card(ref CardRef, now int64) (Result, error) {
	if !cardIDPattern.MatchString(ref.Store) || !cardIDPattern.MatchString(ref.Alias) {
		return Result{}, fmt.Errorf("card store and alias must be 1-128 letters, digits, or ._- starting with a letter or digit")
	}
	ref.Title = cleanLine(st.scrub.scrub(ref.Title), CardTitleRunes)
	return st.lineAfterReply(RingEntry{Kind: RingCard, Card: &ref}, now), nil
}

// lineAfterReply writes entry at now after the reply streamed so far and
// the open tool gap, so the line follows the text and the calls that led
// to it (a card line, a ui line).
func (st *State) lineAfterReply(entry RingEntry, now int64) Result {
	leadReply := st.reply
	var res Result
	if st.reply != nil {
		st.flushTools(&res)
		res.Ring = append(res.Ring, st.finishReply(""))
	}
	st.flushTools(&res)
	res.Ring = append(res.Ring, entry)
	res.Changed = true
	st.refreshPhases(now)
	if st.reply != leadReply {
		res.Urgent = true
	}
	return res
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
	if ev.AgentKind != AgentKindPi {
		target := st.spawnAliases[ev.AgentID]
		if target == "" && strings.TrimSpace(ev.SessionKey) != "" {
			target = st.spawnReplies[wsmailbox.ReplyID(strings.TrimSpace(ev.SessionKey))]
		}
		if target == "" && slices.Contains(st.evicted, "pi:"+ev.AgentID) {
			target = "pi:" + ev.AgentID
		}
		if target != "" && slices.Contains(st.evicted, target) {
			if st.spawnAliases == nil {
				st.spawnAliases = map[string]string{}
			}
			st.spawnAliases[ev.AgentID] = target
			return true
		}
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
		if ev.Status != "" || ev.ParentID != "" || ev.Label != "" || ev.SessionKey != "" || ev.AgentKind != "" || hasTelemetry(ev) {
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
	switch ev.AgentKind {
	case "", AgentKindPi:
	default:
		return fmt.Errorf("agentKind must be %q or absent", AgentKindPi)
	}
	a := st.agent(ev.AgentID, at)
	if ev.AgentKind != "" {
		a.kind = ev.AgentKind
	}
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
	st.setTelemetry(a, ev)
	switch ev.Status {
	case StatusRunning:
		a.status, a.endedAt = StatusRunning, 0
	case StatusDone, StatusFailed:
		if a.status == StatusRunning {
			a.endedAt = at
		}
		a.status = ev.Status
		a.permission = nil
		st.batchesEnded += uint64(len(closeBatches(&a.activityState, at)))
	}
	if a.status != StatusRunning {
		// An ended subagent produces no output, so it shows no rate.
		a.outputTps = 0
	}
	return nil
}

// Agent kinds an agent event's AgentKind takes.
const (
	// AgentKindPi is a Pi subagent the session spawned through ws (the
	// pi.* tools), which the daemon's agent runner hosts.
	AgentKindPi = "pi"
)

// knownAgentKind is kind when it is a known agent kind, else "".
func knownAgentKind(kind string) string {
	if kind == AgentKindPi {
		return kind
	}
	return ""
}

// hasTelemetry reports an event that carries any subagent telemetry field.
func hasTelemetry(ev Event) bool {
	return ev.Model != "" || ev.Effort != "" || ev.ContextTokens != nil || ev.OutputTps != nil
}

// setTelemetry takes the event's telemetry fields that are present and
// valid; the rest keep their held values.
func (st *State) setTelemetry(a *agentState, ev Event) {
	if m := cleanLine(st.scrub.scrub(ev.Model), ModelRunes); m != "" {
		a.model = m
	}
	if e := cleanLine(st.scrub.scrub(ev.Effort), EffortRunes); e != "" {
		a.effort = e
	}
	if n := ev.ContextTokens; n != nil && *n >= 0 && *n <= MaxContextTokens {
		a.contextTokens = int64(math.Round(*n))
	}
	if r := ev.OutputTps; r != nil && *r >= 0 && *r <= MaxOutputTps {
		a.outputTps = *r
	}
}

func (st *State) setActivity(act *activityState, ev Event, batches []BatchView) {
	if ev.Batches != nil {
		fresh := act.setBatches(batches)
		st.batchesEnded += uint64(len(fresh))
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
	var identities []Agent
	for _, a := range st.agents {
		identities = append(identities, Agent{ID: a.id, ReplyID: a.replyID, AgentKind: a.kind})
		if a.kind == AgentKindPi && a.replyID != "" {
			if st.spawnReplies == nil {
				st.spawnReplies = map[string]string{}
			}
			st.spawnReplies[a.replyID] = a.id
		}
	}
	for i, j := range spawnAgentTargets(identities, st.spawnAliases) {
		if st.spawnAliases == nil {
			st.spawnAliases = map[string]string{}
		}
		st.spawnAliases[identities[i].ID] = identities[j].ID
	}
	groups := map[string][]*agentState{}
	for _, a := range st.agents {
		id := a.id
		if target := st.spawnAliases[id]; st.agents[target] != nil {
			id = target
		}
		groups[id] = append(groups[id], a)
	}
	var running, finished []*agentState
	for id := range groups {
		a := st.agents[id]
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
			for _, producer := range groups[a.id] {
				delete(st.agents, producer.id)
			}
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
	// Alias tombstones have the same bounded lifetime as ordinary evictions.
	for _, identities := range []map[string]string{st.spawnAliases, st.spawnReplies} {
		for alias, target := range identities {
			if st.agents[target] == nil && !slices.Contains(st.evicted, target) {
				delete(identities, alias)
			}
		}
	}
}

// SeedFinished takes the finished subagents (done or failed) of prev, a
// snapshot an earlier writer left for this session, so a restarted writer
// does not lose them; retain still bounds them. A snapshot of another
// session, an agent the state already holds, and an agent prev shows
// running are skipped: no end will come for a running one.
func (st *State) SeedFinished(prev Snapshot) {
	if prev.ReplyID != st.replyID {
		return
	}
	for _, p := range prev.Agents {
		if p.Status != StatusDone && p.Status != StatusFailed {
			continue
		}
		if _, ok := st.agents[p.ID]; ok || !agentIDPattern.MatchString(p.ID) {
			continue
		}
		a := &agentState{
			id:            p.ID,
			label:         cleanLine(st.scrub.scrub(p.Label), LabelRunes),
			replyID:       p.ReplyID,
			kind:          knownAgentKind(p.AgentKind),
			status:        p.Status,
			startedAt:     p.StartedAt,
			endedAt:       p.EndedAt,
			touched:       p.EndedAt,
			model:         cleanLine(st.scrub.scrub(p.Model), ModelRunes),
			effort:        cleanLine(st.scrub.scrub(p.Effort), EffortRunes),
			contextTokens: p.ContextTokens,
		}
		if p.ParentID != p.ID && agentIDPattern.MatchString(p.ParentID) {
			a.parentID = p.ParentID
		}
		a.caption = cleanLine(st.scrub.scrub(p.Caption), CaptionRunes)
		a.phase, a.phaseStartedAt = PhaseIdle, p.PhaseStartedAt
		a.done, a.failed = p.Done, p.Failed
		a.counted = map[string]bool{}
		for _, b := range p.Batches {
			if b.Running != nil || b.Generating != nil {
				continue
			}
			a.batches = append(a.batches, b.clone())
			a.counted[batchKey(b)] = true
		}
		st.agents[a.id] = a
		st.evicted = slices.DeleteFunc(st.evicted, func(e string) bool { return e == a.id })
	}
	st.retain()
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
	snap.Host = st.host.clone()
	snap.Commands = append([]SlashCommand(nil), st.commands...)
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
			AgentKind: a.kind,

			Model:         a.model,
			Effort:        a.effort,
			ContextTokens: a.contextTokens,
			OutputTps:     a.outputTps,
		})
	}
	sort.Slice(snap.Agents, func(i, j int) bool {
		if snap.Agents[i].StartedAt != snap.Agents[j].StartedAt {
			return snap.Agents[i].StartedAt < snap.Agents[j].StartedAt
		}
		return snap.Agents[i].ID < snap.Agents[j].ID
	})
	snap.Agents = mergeSpawnAgents(snap.Agents, st.spawnAliases)
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
	Name        string `json:"name"`
	StartedAt   int64  `json:"startedAt"`
	EndedAt     int64  `json:"endedAt"`
	Failed      bool   `json:"failed"`
	ArgsPreview string `json:"argsPreview"`
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
		r.ArgsPreview = st.argsPreview(r.ArgsPreview)
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
// dropped, names are scrubbed and capped like toolNames, argument previews
// are scrubbed and capped like the running call's, unknown or inconsistent
// clocks read 0, and at most BatchCallCap (and never more than the batch's
// callCount) are kept. nil when none is left.
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
		c := BatchCall{Name: cleanToolName(st.scrub.scrub(in.Name)), StartedAt: max(in.StartedAt, 0), Failed: in.Failed, ArgsPreview: st.argsPreview(in.ArgsPreview)}
		if in.EndedAt >= c.StartedAt {
			c.EndedAt = in.EndedAt
		}
		out = append(out, c)
	}
	return out
}

// argsPreviewScanBytes bounds the text argsPreview scans: an adapter may
// send a whole raw argument object, and only its head can reach the
// ArgsPreviewCap-rune display, so the masking rules never run over more.
const argsPreviewScanBytes = 4096

// argsPreview is every argument preview the state keeps (a permission
// wait's, a running call's, a finished call's), whatever adapter sent it:
// session keys scrubbed (before any cut, so none is cut in half), bounded
// to its head, credentials masked (wsredact.Secrets, best-effort), then one
// line capped at ArgsPreviewCap. It is the one place an argument preview
// enters the state, so a preview is never stored, shown, or written to the
// ring unmasked.
func (st *State) argsPreview(s string) string {
	s = st.scrub.scrub(s)
	if len(s) > argsPreviewScanBytes {
		cut := argsPreviewScanBytes
		for cut > 0 && !utf8.RuneStart(s[cut]) {
			cut--
		}
		s = s[:cut]
	}
	return cleanLine(wsredact.Secrets(s), ArgsPreviewCap)
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

// commandNamePattern is a slash command's name without its slash, as the
// board-answer inbox takes it.
var commandNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,199}$`)

// normalizeCommands keeps the commands whose name (one leading slash
// dropped) is a valid name, the first of each name, at most CommandCap,
// with text capped and an unknown layer cleared; nil when none is left.
func normalizeCommands(in []SlashCommand) []SlashCommand {
	var out []SlashCommand
	seen := map[string]bool{}
	for _, c := range in {
		if len(out) == CommandCap {
			break
		}
		c.Name = strings.TrimPrefix(strings.TrimSpace(c.Name), "/")
		if !commandNamePattern.MatchString(c.Name) || seen[c.Name] {
			continue
		}
		seen[c.Name] = true
		c.Description = cleanLine(c.Description, CommandDescriptionRunes)
		c.Source = cleanLine(c.Source, 32)
		if c.Layer != LayerUser && c.Layer != LayerAgent {
			c.Layer = ""
		}
		out = append(out, c)
	}
	return out
}
