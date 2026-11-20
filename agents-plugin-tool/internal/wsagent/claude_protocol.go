// Claude stream output types and mapper state.

package wsagent

import (
	"encoding/json"
	"github.com/kang-sw/devenv/internal/wslive"
	"strings"
	"time"
)

// claudeMapper maps Claude Code's stream-json output to runner events: the
// init message to the session id (after the init check), assistant text to
// final replies, tool calls to the lead's batch (one per assistant
// message), rate-limit and result figures to usage, and the result to the
// turn's end, and the subagent tasks the Agent tool starts to subagents.
// A line a subagent wrote (parent_tool_use_id set) is not the lead's: its
// text and tool calls stay out of the lead's reply and batches.
type claudeMapper struct {
	spec    StartSpec
	now     func() time.Time
	batches batchTracker
	// tasks are the subagent task ids started and not yet ended.
	tasks map[string]bool
	// wakeTasks retains task identity past task_updated's terminal state:
	// task_notification follows it and carries the delivered summary.
	wakeTasks map[string]claudeWakeTask

	context *wslive.UsageContext
	limits  map[string]wslive.UsageLimit
	// limitsAsOf is the last rate_limit_event observation. Other status
	// events keep it, so context activity cannot make old limits look fresh.
	limitsAsOf int64
	cost       *wslive.UsageCost
	// window is the largest modelUsage.contextWindow a result reported: the
	// model's window, the fallback for compactWindow.
	window int64
	// model is the model the lead's last init named. compactWindow is the
	// window the harness compacts against (get_context_usage's
	// rawMaxTokens, which honours CLAUDE_CODE_AUTO_COMPACT_WINDOW), as
	// answered for compactModel; it counts only while that is model.
	// askModel is the model the last ask was sent under, and asks numbers
	// them.
	model         string
	compactWindow int64
	compactModel  string
	askModel      string
	asks          int
	// described are the descriptions the initialize answer gave the
	// commands, by name: a turn's init lists names only, and its list
	// replaces the answer's.
	described map[string]string

	// compacted is set from a compact_boundary to the next result: the
	// stream re-emits the last assistant message in between, with no replay
	// flag but under the line uuid it had before, and it is dropped. uuids
	// are the recent assistant lines' (claudeSeenUUIDs of them), so a real
	// message after an automatic compaction mid-turn is kept.
	compacted bool
	uuids     []string
	// evidence keeps the first original timestamp across split message blocks.
	evidence []wslive.ModelObservation
	// states is set once the stream carried a session_state_changed line
	// (a harness that emits them): from then on the idle state ends a
	// lead's turn, not the result, which an idle follows and whose error
	// turnErr holds until then. Ending the turn at the result too would
	// let that trailing idle end the next turn, which a held message
	// submitted at the result had already begun.
	//
	// The exception is a result while background tasks run (background,
	// the last background_tasks_changed count): the harness then stays
	// running and sends no idle until every task ended and the turn their
	// completion starts is done, also across later prompts (seen on
	// 2.1.323, claude-lead-background-agent-states.jsonl), so that result
	// ends the turn. ended marks such a result until the next turn start
	// is mapped: an idle in between belongs to the turn already ended and
	// ends nothing.
	states     bool
	turnErr    string
	background int
	ended      bool
	// levers are the lead's compaction lever calls (*__request_compaction)
	// waiting for their result, by tool call id. lever marks a lead turn
	// one of whose lever calls returned a result that is not an error,
	// from that result to the turn's result; leverEnd carries it,
	// unless that result was an interrupt, to the idle state that then
	// ends the turn (states). The ending TurnEnd reports it
	// (Event.CompactRequested).
	levers   map[string]bool
	lever    bool
	leverEnd bool
	// approvals are the can_use_tool requests not seen settled, request id
	// by tool call id.
	approvals map[string]string

	// retrying is set from an api_retry line until a response or the turn's
	// end settles it.
	retrying bool
	// rejected are the usage limits the last rate-limit lines reported as
	// refusing requests, each with its reset time (epoch ms; 0 unknown).
	rejected map[string]int64
	// spawns are the lead's Agent calls not yet started as a task (their
	// requested model and effort), and subs the running subagent tasks'
	// telemetry; both by the Agent call's tool call id.
	spawns map[string]claudeSpawn
	subs   map[string]*claudeSub

	// partial is the lead's text content block still streaming
	// (--include-partial-messages), and partialAt when it was last sent as
	// the reply so far; the block's whole assistant line replaces it.
	partial   strings.Builder
	partialAt int64
}

// claudePartialEvery is the least time between two sends of the streaming
// reply: every reply event makes the runner take a snapshot.
const claudePartialEvery = 100

// claudeRingSkip matches the partial-message lines the mapper drops: tool
// input, thinking, and signature deltas. A lead's holder keeps them out of
// its ring, where a large Write's input streams as about 15 bytes of
// stream_event per byte written and would push the turn's start out. A line
// whose shape this no longer matches is kept, as before the filter.
const claudeRingSkip = `^\{"type":"stream_event","event":\{"type":"content_block_delta","index":[0-9]+,"delta":\{"type":"(?:input_json|thinking|signature)_delta"`

// claudeSpawn is what a lead's Agent call asked of its subagent: the model
// (the call's model parameter) and the effort its subagent type carries.
type claudeSpawn struct {
	model, effort string
}

func newClaudeMapper(spec StartSpec) *claudeMapper {
	return &claudeMapper{spec: spec, now: time.Now, batches: newBatchTracker("c"), limits: map[string]wslive.UsageLimit{}, tasks: map[string]bool{}, approvals: map[string]string{},
		rejected: map[string]int64{}, spawns: map[string]claudeSpawn{}, subs: map[string]*claudeSub{}, wakeTasks: map[string]claudeWakeTask{}, levers: map[string]bool{}}
}

// lead reports a lead conversation's mapper.
func (m *claudeMapper) lead() bool { return m.spec.Lead != nil }

type claudeLine struct {
	LocalCommandSource json.RawMessage `json:"local_command_source"`
	IsSynthetic        bool            `json:"isSynthetic"`
	Type               string          `json:"type"`
	Subtype            string          `json:"subtype"`
	Message            json.RawMessage `json:"message"`
	// UUID is the line's own id; a re-emitted line keeps it.
	UUID      string `json:"uuid"`
	Timestamp string `json:"timestamp"`
	// RequestID and Request are a control_request's (can_use_tool) and a
	// control_cancel_request's.
	RequestID string          `json:"request_id"`
	Request   json.RawMessage `json:"request"`
	// State is a session_state_changed line's: running, requires_action,
	// or idle. PermissionMode is a status line's, after a mode change.
	State          string `json:"state"`
	PermissionMode string `json:"permissionMode"`
	// Tasks is a background_tasks_changed line's whole current list.
	Tasks *[]json.RawMessage `json:"tasks"`
	// ToolUseID and SubagentType are a task_started line's: the Agent call
	// that started it, and the subagent type it asked for.
	ToolUseID    string `json:"tool_use_id"`
	SubagentType string `json:"subagent_type"`
	// CompactResult and CompactError report a compaction that failed.
	CompactResult string `json:"compact_result"`
	CompactError  string `json:"compact_error"`
	// Error, APIError, and APIErrorParams are a failed request's typed
	// error: on an assistant line (authentication_failed, model_not_found,
	// rate_limit; usage_limit_reached) and on an api_retry line, with that
	// line's Attempt, MaxRetries, and ErrorStatus. Kept raw: a shape this
	// mapper does not expect must not cost the whole line.
	Error          json.RawMessage `json:"error"`
	APIError       json.RawMessage `json:"api_error"`
	APIErrorParams *struct {
		RateLimit *claudeRateLimit `json:"rate_limit_info"`
	} `json:"api_error_params"`
	Attempt     json.RawMessage `json:"attempt"`
	MaxRetries  json.RawMessage `json:"max_retries"`
	ErrorStatus json.RawMessage `json:"error_status"`
	// ParentToolUseID names the Agent call whose subagent wrote the line.
	ParentToolUseID string `json:"parent_tool_use_id"`
	// Event is a stream_event line's raw API stream event.
	Event json.RawMessage `json:"event"`
	// TaskID, TaskType, Description, Status, and Patch describe a
	// background task's system lines (task_started, task_updated,
	// task_notification).
	TaskID          string `json:"task_id"`
	TaskType        string `json:"task_type"`
	Description     string `json:"description"`
	Summary         string `json:"summary"`
	IsBackgrounded  *bool  `json:"is_backgrounded"`
	OwnedBySubagent bool   `json:"owned_by_subagent"`
	Status          string `json:"status"`
	Patch           struct {
		Status string `json:"status"`
	} `json:"patch"`
	Response struct {
		Subtype   string          `json:"subtype"`
		RequestID string          `json:"request_id"`
		Error     string          `json:"error"`
		Response  json.RawMessage `json:"response"`
	} `json:"response"`
	RateLimit     *claudeRateLimit `json:"rate_limit_info"`
	IsError       bool             `json:"is_error"`
	Result        string           `json:"result"`
	Errors        []string         `json:"errors"`
	TerminalState string           `json:"terminal_reason"`
	TotalCostUSD  *float64         `json:"total_cost_usd"`
	ModelUsage    map[string]struct {
		ContextWindow int64 `json:"contextWindow"`
	} `json:"modelUsage"`
	CompactMetadata *struct {
		Trigger string `json:"trigger"`
	} `json:"compact_metadata"`
}

// claudeRateLimit is a rate-limit report: a rate_limit_event's, or the one
// a usage-limit refusal carries.
type claudeRateLimit struct {
	Status         string `json:"status"`
	RateLimitType  string `json:"rateLimitType"`
	ResetsAt       int64  `json:"resetsAt"`
	OverageStatus  string `json:"overageStatus"`
	IsUsingOverage bool   `json:"isUsingOverage"`
	UnifiedWindows map[string]struct {
		Utilization *float64 `json:"utilization"`
		ResetsAt    int64    `json:"resetsAt"`
	} `json:"unifiedWindows"`
}

type claudeMessage struct {
	ID      string          `json:"id"`
	Model   string          `json:"model"`
	Content json.RawMessage `json:"content"`
	Usage   *struct {
		InputTokens              int64 `json:"input_tokens"`
		CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
		CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
		OutputTokens             int64 `json:"output_tokens"`
	} `json:"usage"`
}

type claudeBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	IsError   bool            `json:"is_error"`
	// Content is a tool_result's content: a string, or text blocks.
	Content json.RawMessage `json:"content"`
}

// claudeResultText is a tool_result's content as text: the string itself,
// or its text blocks joined.
func claudeResultText(raw json.RawMessage) string {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	var blocks []claudeBlock
	if json.Unmarshal(raw, &blocks) != nil {
		return ""
	}
	var parts []string
	for _, b := range blocks {
		if b.Type == "text" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n")
}
