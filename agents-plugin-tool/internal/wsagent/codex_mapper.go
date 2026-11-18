package wsagent

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

// Codex app-server omits the JSON-RPC version member. Request ids may be
// strings or integers; server request ids are echoed without conversion.
type codexWire struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	Result json.RawMessage `json:"result"`
	Error  *codexError     `json:"error"`
}

type codexError struct {
	Message string          `json:"message"`
	Info    json.RawMessage `json:"codexErrorInfo"`
}

type codexTurn struct {
	ID     string      `json:"id"`
	Status string      `json:"status"`
	Items  []codexItem `json:"items"`
	Error  *codexError `json:"error"`
}

type codexItem struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Status    string          `json:"status"`
	Command   string          `json:"command"`
	Server    string          `json:"server"`
	Tool      string          `json:"tool"`
	Arguments json.RawMessage `json:"arguments"`
	Result    json.RawMessage `json:"result"`
	Error     json.RawMessage `json:"error"`
	ExitCode  *int            `json:"exitCode"`
}

type codexStatus struct {
	Type        string   `json:"type"`
	ActiveFlags []string `json:"activeFlags"`
}

type codexThread struct {
	ID      string      `json:"id"`
	Version string      `json:"cliVersion"`
	Model   *string     `json:"model"`
	Status  codexStatus `json:"status"`
	Turns   []codexTurn `json:"turns"`
}

type codexPendingApproval struct {
	raw         json.RawMessage
	method      string
	turn        string
	permissions json.RawMessage
	answered    bool
}

// Notification state is scoped by thread AND turn. An interrupted command
// can complete during a later turn, still naming the old turn. Such a line
// must not finish the new turn's tools, reply, or approvals.
type codexMapper struct {
	mu sync.Mutex

	sessionID string
	turnID    string
	endedTurn string
	batches   batchTracker
	texts     map[string]string
	started   map[string]bool
	finished  map[string]bool
	approvals map[string]*codexPendingApproval
	compact   bool
	retry     int
}

func newCodexMapper(session string) *codexMapper {
	return &codexMapper{
		sessionID: session,
		batches:   newBatchTracker("codex"),
		texts:     map[string]string{},
		started:   map[string]bool{},
		finished:  map[string]bool{},
		approvals: map[string]*codexPendingApproval{},
	}
}

func (m *codexMapper) session() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessionID
}

func (m *codexMapper) active() (string, string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessionID, m.turnID
}

func codexRequestKind(id json.RawMessage) string {
	var text string
	if json.Unmarshal(id, &text) != nil {
		return ""
	}
	parts := strings.Split(text, "/")
	if len(parts) != 4 || parts[0] != "wsc" {
		return ""
	}
	return parts[1]
}

func (m *codexMapper) mapLine(line []byte, replies lineReplies) []Event {
	var msg codexWire
	if json.Unmarshal(line, &msg) != nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().UnixMilli()
	hasID := len(msg.ID) > 0 && string(msg.ID) != "null"
	switch {
	case msg.Method != "" && hasID:
		return m.request(msg, replies, now)
	case msg.Method == "" && hasID:
		return m.response(msg, replies, now)
	case msg.Method == "account/rateLimits/updated":
		return codexLimits(msg.Params)
	}

	var p struct {
		ThreadID  string          `json:"threadId"`
		TurnID    string          `json:"turnId"`
		Turn      codexTurn       `json:"turn"`
		Item      codexItem       `json:"item"`
		ItemID    string          `json:"itemId"`
		Delta     string          `json:"delta"`
		Status    codexStatus     `json:"status"`
		RequestID json.RawMessage `json:"requestId"`
		Error     codexError      `json:"error"`
		WillRetry bool            `json:"willRetry"`
	}
	if json.Unmarshal(msg.Params, &p) != nil || m.sessionID == "" || p.ThreadID != m.sessionID {
		return nil
	}
	switch msg.Method {
	case "turn/started":
		if p.Turn.ID == "" || p.Turn.ID == m.turnID || p.Turn.ID == m.endedTurn {
			return nil
		}
		m.turnID = p.Turn.ID
		clear(m.texts)
		clear(m.started)
		clear(m.finished)
		return []Event{{TurnStart: true}}
	case "turn/completed":
		if p.Turn.ID == "" || p.Turn.ID == m.endedTurn || (m.turnID != "" && p.Turn.ID != m.turnID) {
			return nil
		}
		return m.endTurn(p.Turn, now)
	case "thread/status/changed":
		return []Event{{HarnessState: codexHarnessState(p.Status)}}
	case "serverRequest/resolved":
		id := string(compactRaw(p.RequestID))
		if _, ok := m.approvals[id]; !ok {
			return nil
		}
		delete(m.approvals, id)
		return []Event{{ApprovalDone: id}}
	case "thread/compacted":
		m.compact = false
		return []Event{{Live: &wslive.Event{Kind: wslive.KindCompact, Phase: wslive.CompactEnd}}}
	case "error":
		if p.TurnID != "" && p.TurnID != m.turnID {
			return nil
		}
		if p.WillRetry {
			m.retry++
			return []Event{{Retry: &wslive.Retry{Attempt: m.retry, Error: p.Error.Message}}}
		}
		return append(codexAlert(&p.Error), Event{Notice: "codex: " + p.Error.Message})
	}

	// Items and token usage are turn-local, unlike the thread-wide status
	// and approval-resolution notifications handled above.
	if p.TurnID == "" || p.TurnID != m.turnID {
		return nil
	}
	switch msg.Method {
	case "item/agentMessage/delta":
		if p.ItemID == "" || m.finished[p.ItemID] {
			return nil
		}
		m.texts[p.ItemID] += p.Delta
		return []Event{
			{RetryEnd: true},
			{Live: &wslive.Event{Kind: wslive.KindReply, Text: m.texts[p.ItemID]}},
		}
	case "item/started":
		return m.item(p.Item, false, now)
	case "item/completed":
		return m.item(p.Item, true, now)
	case "thread/tokenUsage/updated":
		return codexUsage(msg.Params)
	}
	return nil
}

func codexHarnessState(status codexStatus) string {
	switch status.Type {
	case "idle":
		return HarnessIdle
	case "active":
		if slices.Contains(status.ActiveFlags, "waitingOnApproval") {
			return HarnessWaiting
		}
		return HarnessRunning
	}
	return ""
}

func (m *codexMapper) response(msg codexWire, replies lineReplies, now int64) []Event {
	kind := codexRequestKind(msg.ID)
	if kind == "" {
		return nil
	}
	answer := controlAnswer{ok: msg.Error == nil, body: msg.Result}
	if msg.Error != nil {
		answer.err = msg.Error.Message
	}
	var out []Event
	if answer.ok {
		switch kind {
		case "open":
			var r struct {
				Thread         codexThread     `json:"thread"`
				ApprovalPolicy json.RawMessage `json:"approvalPolicy"`
				Sandbox        json.RawMessage `json:"sandbox"`
			}
			if json.Unmarshal(msg.Result, &r) != nil || r.Thread.ID == "" {
				answer.ok, answer.err = false, "Codex thread response has no thread id"
				break
			}
			m.sessionID = r.Thread.ID
			var demoted []CapabilityItem
			if r.Thread.Model == nil {
				demoted = append(demoted, ItemModelEffort)
			}
			if !codexPresent(r.ApprovalPolicy) || !codexPresent(r.Sandbox) {
				demoted = append(demoted, ItemApproval)
			}
			// No app-server report gives an aggregate running-background
			// count. Tool removal is not evidence of known zero work.
			unknown := -1
			out = []Event{
				{SessionID: m.sessionID},
				{Harness: &HarnessReport{Version: r.Thread.Version, Demoted: demoted}},
				{BackgroundTasks: &unknown},
				{Live: &wslive.Event{Kind: wslive.KindCommands, Commands: []wslive.SlashCommand{
					{Name: "compact", Description: "Compact the conversation"},
				}}},
			}
		case "models":
			codexModels(msg.Result)
		case "recover":
			out = m.recover(msg.Result, now)
		}
	}
	// Notifications, not control acknowledgements, own turn boundaries.
	// In particular, a late turn/start response cannot reopen an ended turn.
	replies.answer(rawString(msg.ID), answer)
	return out
}

func codexPresent(raw json.RawMessage) bool {
	return len(raw) > 0 && string(raw) != "null"
}

func (m *codexMapper) item(item codexItem, done bool, now int64) []Event {
	if item.ID == "" || m.finished[item.ID] || (!done && m.started[item.ID]) {
		return nil
	}
	if done {
		m.finished[item.ID] = true
	} else {
		m.started[item.ID] = true
	}
	switch item.Type {
	case "agentMessage":
		if !done {
			return nil
		}
		delete(m.texts, item.ID)
		if strings.TrimSpace(item.Text) == "" {
			return nil
		}
		return []Event{
			{Live: &wslive.Event{Kind: wslive.KindReply, Text: item.Text, Final: true}},
			{RetryEnd: true},
		}
	case "commandExecution", "mcpToolCall", "fileChange", "webSearch", "imageView":
		if !done {
			name, args := item.Type, item.Command
			if item.Type == "mcpToolCall" {
				name, args = item.Server+"/"+item.Tool, argsPreview(item.Arguments)
			}
			return m.batches.addCall(m.turnID, item.ID, name, args, now)
		}
		failed := item.Status == "failed" || item.Status == "declined" ||
			(item.ExitCode != nil && *item.ExitCode != 0) || codexPresent(item.Error)
		out := m.batches.endCall(item.ID, failed, now)
		if !failed && (item.Server == "ws" || item.Server == "wsflow") && slices.Contains(resultTools, item.Tool) {
			var result struct {
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
			}
			_ = json.Unmarshal(item.Result, &result)
			var texts []string
			for _, content := range result.Content {
				texts = append(texts, content.Text)
			}
			out = append(out, resultEvents(item.Tool, strings.Join(texts, "\n"))...)
		}
		return out
	case "contextCompaction":
		m.compact = !done
		phase := wslive.CompactStart
		if done {
			phase = wslive.CompactEnd
		}
		return []Event{{Live: &wslive.Event{Kind: wslive.KindCompact, Phase: phase}}}
	}
	return nil
}

func (m *codexMapper) endTurn(turn codexTurn, now int64) []Event {
	var out []Event
	// The summary contains the final answer even when item/completed left
	// the holder ring. Already completed messages are not emitted twice.
	for _, item := range turn.Items {
		if item.Type == "agentMessage" {
			out = append(out, m.item(item, true, now)...)
		}
	}
	if event := m.batches.finish(now, true); event != nil {
		out = append(out, *event)
	}
	for id, approval := range m.approvals {
		if approval.turn == turn.ID || approval.turn == "" {
			delete(m.approvals, id)
			out = append(out, Event{ApprovalDone: id})
		}
	}
	if m.compact {
		phase := wslive.CompactEnd
		if turn.Status != "completed" {
			phase = wslive.CompactFailed
		}
		out = append(out, Event{Live: &wslive.Event{Kind: wslive.KindCompact, Phase: phase}})
		m.compact = false
	}
	out = append(out, codexAlert(turn.Error)...)
	end := Event{TurnEnd: true}
	switch turn.Status {
	case "interrupted":
		end.TurnReason = wslive.TurnAborted
	case "failed":
		end.TurnError = "Codex turn failed"
		if turn.Error != nil && turn.Error.Message != "" {
			end.TurnError = turn.Error.Message
		}
	}
	m.turnID, m.endedTurn, m.retry = "", turn.ID, 0
	return append(out, Event{RetryEnd: true}, end)
}

// recover runs on the reader, in order with notifications. Its final reply
// precedes TurnEnd on the same stream, so queued input cannot overtake it.
func (m *codexMapper) recover(raw json.RawMessage, now int64) []Event {
	var r struct {
		Thread codexThread `json:"thread"`
	}
	if json.Unmarshal(raw, &r) != nil || r.Thread.ID != m.sessionID {
		return []Event{{Notice: "Codex recovery returned a different or malformed thread; input remains held"}}
	}
	out := []Event{{HarnessState: codexHarnessState(r.Thread.Status)}}
	switch r.Thread.Status.Type {
	case "active":
		for _, turn := range r.Thread.Turns {
			if turn.Status == "inProgress" {
				m.turnID = turn.ID
			}
		}
	case "idle":
		if len(r.Thread.Turns) == 0 {
			// An empty history proves only an idle, never-used thread.
			return append(out, Event{TurnEnd: true})
		}
		turn := r.Thread.Turns[len(r.Thread.Turns)-1]
		if turn.Status == "completed" || turn.Status == "interrupted" || turn.Status == "failed" {
			return append(out, m.endTurn(turn, now)...)
		}
	}
	return out
}

func (m *codexMapper) request(msg codexWire, replies lineReplies, now int64) []Event {
	var p struct {
		ThreadID    string          `json:"threadId"`
		TurnID      string          `json:"turnId"`
		ItemID      string          `json:"itemId"`
		Reason      string          `json:"reason"`
		Message     string          `json:"message"`
		Command     string          `json:"command"`
		Permissions json.RawMessage `json:"permissions"`
		Meta        struct {
			ApprovalKind string `json:"codex_approval_kind"`
		} `json:"_meta"`
	}
	refuse := func(text string) []Event {
		replies.send(map[string]any{"id": msg.ID, "error": map[string]any{"code": -32601, "message": text}})
		return []Event{{Notice: text}}
	}
	if json.Unmarshal(msg.Params, &p) != nil || m.sessionID == "" || p.ThreadID != m.sessionID {
		return refuse("ws does not serve requests from another or malformed Codex thread")
	}
	if p.TurnID != "" && (p.TurnID == m.endedTurn || (m.turnID != "" && p.TurnID != m.turnID)) {
		return refuse("ws does not serve requests from a stale Codex turn")
	}
	switch msg.Method {
	case "item/commandExecution/requestApproval", "item/fileChange/requestApproval", "item/permissions/requestApproval":
	case "mcpServer/elicitation/request":
		if p.Meta.ApprovalKind != "mcp_tool_call" {
			return refuse(claudeChatOnlyDenial)
		}
	case "item/tool/requestUserInput":
		return refuse(claudeChatOnlyDenial)
	default:
		return refuse("ws does not support the Codex server request " + msg.Method)
	}

	raw := compactRaw(msg.ID)
	id := string(raw)
	if _, exists := m.approvals[id]; exists {
		return nil
	}
	m.approvals[id] = &codexPendingApproval{
		raw: raw, method: msg.Method, turn: p.TurnID, permissions: p.Permissions,
	}
	description := p.Reason
	if description == "" {
		description = p.Message
	}
	if description == "" {
		description = p.Command
	}
	description += " (ws supports allow/deny only; session grants, policy amendments, cancel and elicitation persist are unsupported.)"
	return []Event{{Approval: &ApprovalRequest{
		ID: id, Tool: msg.Method, Input: slices.Clone(msg.Params),
		Description: description, ToolUseID: p.ItemID, StartedAt: now,
	}}}
}

func (m *codexMapper) approvalAnswer(id string, allow bool) (json.RawMessage, any, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	approval := m.approvals[id]
	if approval == nil || approval.answered {
		return nil, nil, fmt.Errorf("Codex has no unanswered approval %q", id)
	}
	decision := "decline"
	if allow {
		decision = "accept"
	}
	var result any = map[string]any{"decision": decision}
	switch approval.method {
	case "item/permissions/requestApproval":
		permissions := json.RawMessage(`{}`)
		if allow {
			if !codexPresent(approval.permissions) {
				return nil, nil, fmt.Errorf("Codex permission request has no permissions to grant")
			}
			permissions = approval.permissions
		}
		result = map[string]any{"permissions": permissions, "scope": "turn"}
	case "mcpServer/elicitation/request":
		result = map[string]any{"action": decision}
	}
	approval.answered = true
	return approval.raw, result, nil
}

func codexAlert(err *codexError) []Event {
	if err == nil {
		return nil
	}
	kind := ""
	switch rawString(err.Info) {
	case "unauthorized":
		kind = wslive.NoticeLogin
	case "usageLimitExceeded", "rateLimitExceeded":
		kind = wslive.NoticeUsageLimit
	}
	if kind == "" {
		return nil
	}
	return []Event{{Alert: &wslive.Notice{Kind: kind, Text: err.Message}}}
}

// codexModels keeps only the harness's advertised choices, including the
// union of visible models' effort values. Opening a dashboard never probes
// an account or creates a thread just to populate these lists.
func codexModels(raw json.RawMessage) {
	var r struct {
		Data []struct {
			Model   string `json:"model"`
			Hidden  bool   `json:"hidden"`
			Efforts []struct {
				Effort string `json:"reasoningEffort"`
			} `json:"supportedReasoningEfforts"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &r) != nil || r.Data == nil {
		return
	}
	var models []ModelChoice
	var efforts []string
	for _, model := range r.Data {
		if model.Hidden || model.Model == "" {
			continue
		}
		thinking := len(model.Efforts) > 0
		models = append(models, ModelChoice{ID: model.Model, Thinking: &thinking})
		for _, effort := range model.Efforts {
			if effort.Effort != "" && !slices.Contains(efforts, effort.Effort) {
				efforts = append(efforts, effort.Effort)
			}
		}
	}
	codexChoices.Lock()
	defer codexChoices.Unlock()
	codexChoices.models, codexChoices.efforts = models, efforts
}

func codexUsage(raw json.RawMessage) []Event {
	var p struct {
		TokenUsage struct {
			Last *struct {
				Total  int64 `json:"totalTokens"`
				Input  int64 `json:"inputTokens"`
				Cached int64 `json:"cachedInputTokens"`
			} `json:"last"`
			Window int64 `json:"modelContextWindow"`
		} `json:"tokenUsage"`
	}
	if json.Unmarshal(raw, &p) != nil || p.TokenUsage.Last == nil {
		return nil
	}
	last := p.TokenUsage.Last
	context := &wslive.UsageContext{Tokens: &last.Total, Window: p.TokenUsage.Window}
	if context.Window > 0 {
		percent := float64(last.Total) * 100 / float64(context.Window)
		context.Percent = &percent
	}
	if last.Input > 0 {
		hit := float64(last.Cached) * 100 / float64(last.Input)
		context.CacheHit = &hit
	}
	return []Event{{Live: &wslive.Event{Kind: wslive.KindStatus, Context: context}}}
}

type codexLimit struct {
	Window  int     `json:"windowDurationMins"`
	Percent float64 `json:"usedPercent"`
	Reset   int64   `json:"resetsAt"`
}

func codexLimits(raw json.RawMessage) []Event {
	var p struct {
		RateLimits struct {
			Primary   *codexLimit `json:"primary"`
			Secondary *codexLimit `json:"secondary"`
		} `json:"rateLimits"`
	}
	if json.Unmarshal(raw, &p) != nil {
		return nil
	}
	var limits []wslive.UsageLimit
	for _, limit := range []*codexLimit{p.RateLimits.Primary, p.RateLimits.Secondary} {
		if limit == nil {
			continue
		}
		kind := ""
		switch limit.Window {
		case 300:
			kind = wslive.LimitFiveHour
		case 10080:
			kind = wslive.LimitSevenDay
		}
		if kind != "" {
			limits = append(limits, wslive.UsageLimit{Kind: kind, Percent: limit.Percent, ResetsAt: limit.Reset * 1000})
		}
	}
	if len(limits) == 0 {
		return nil
	}
	return []Event{{Live: &wslive.Event{Kind: wslive.KindStatus, Limits: limits}}}
}
