package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsgit"
	"github.com/kang-sw/devenv/internal/wsi18n"
	"github.com/kang-sw/devenv/internal/wsmailbox"
	"github.com/kang-sw/devenv/internal/wsstate"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// The session whiteboard: whiteboard.update applies a command string to the
// authoritative topic-folder store (wswhiteboard.Apply). The daemon renders
// views from committed state; updates produce no derived page files.

const (
	whiteboardPlaybookName = "whiteboard"
	// whiteboardDirRel is the whiteboard folder under a session root. It is
	// registered in the repository's local exclude file when a board is created.
	whiteboardDirRel  = "ai-docs/whiteboards"
	whiteboardOnValue = "on"
)

var (
	errWhiteboardRefused = errors.New("whiteboard batch refused")
)

func isWhiteboardTool(name string) bool {
	return name == "whiteboard.update" || name == "whiteboard.query"
}

// whiteboardStatusPlaybookName is the print playbook teaching a sub-lead the
// status-only subset of the board grammar; refusals to a sub-lead key name it.
const whiteboardStatusPlaybookName = "whiteboard-status"

func whiteboardLeadOnlyDenial(name string) string {
	return fmt.Sprintf("%s is not available to a spawned session: the whiteboard is the top-level lead's channel to its user. Put what the user should see in your final report; your lead decides what reaches the board.", name)
}

// whiteboardSectionProvider supplies one auxiliary panel and
// declares which tool calls dirty it. Only real sections are registered; a
// provider with nothing to show yields no panel. A provider gathers its data
// once per call, so a page never observes mixed source data.
type whiteboardSectionProvider struct {
	id        string
	dirtiedBy []string
	panel     func(in whiteboardPanelInput) (wswhiteboard.Panel, bool)
}

// whiteboardPanelInput is everything a provider reads, resolved once by the
// renderer.
type whiteboardPanelInput struct {
	rec sessionRecord
	// root is the session root, where the ticket directories live.
	root       string
	sessionKey string
	replyID    string
	// lang is the viewer's language; empty selects English.
	lang string
	// watch, when not nil, is told each file and directory a panel read
	// that a writer may change without a nudge (the ticket directories and
	// files), which a served page names for its refresh check.
	watch func(path string)
	// history, when not nil, reads the session's mailbox history in place
	// of readMailboxHistory.
	history func() ([]mailboxHistoryRecord, error)
	// replies, when not nil, reads the reply-id registry in place of the
	// store's LoadReply (the served page's shared read); false when it
	// cannot be read.
	replies func() (wsmailbox.ReplyStore, bool)
}

var whiteboardProviders = []whiteboardSectionProvider{
	{
		id:        "todo",
		dirtiedBy: []string{"todo.add", "todo.check", "todo.erase", "todo.clear", "todo.reorder", "route.resolve_implement"},
		panel:     todoPanel,
	},
	{
		id:        "agenda",
		dirtiedBy: []string{"agenda.set", "agenda.clear", "route.resolve_implement"},
		panel:     agendaPanel,
	},
	{
		id:        "tickets",
		dirtiedBy: []string{"tickets.move", "tickets.close", "tickets.create_empty"},
		panel:     ticketPanel,
	},
	{
		id:        "mailbox",
		dirtiedBy: []string{"mailbox.send", "mailbox.recv"},
		panel:     mailboxPanel,
	},
}

// todoPanel lists the unfinished todos in store order and folds the done ones
// below them.
func todoPanel(in whiteboardPanelInput) (wswhiteboard.Panel, bool) {
	todos := in.rec.Todos
	if len(todos) == 0 {
		return wswhiteboard.Panel{}, false
	}
	var rows, folded []wswhiteboard.PanelRow
	for _, todo := range todos {
		row := wswhiteboard.PanelRow{State: string(todo.Status), Label: todo.Key, Text: todo.Title}
		switch {
		case todo.Status != todoDone:
			rows = append(rows, row)
		default:
			folded = append(folded, row)
		}
	}
	count := len(todos) - len(rows)
	return wswhiteboard.Panel{ID: "todo", Title: wsi18n.T(in.lang, "panel.todo"), Icon: "todo", Count: fmt.Sprintf("%d/%d", count, len(todos)), Rows: rows, Folded: folded}, true
}

// agendaPanel shows the agenda blobs in key order, each laid out whole as a
// foldable tree.
func agendaPanel(in whiteboardPanelInput) (wswhiteboard.Panel, bool) {
	agenda := in.rec.Agenda
	if len(agenda) == 0 {
		return wswhiteboard.Panel{}, false
	}
	keys := make([]string, 0, len(agenda))
	for k := range agenda {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	panel := wswhiteboard.Panel{ID: "agenda", Title: wsi18n.T(in.lang, "panel.agenda"), Icon: "agenda", Count: fmt.Sprint(len(keys))}
	for _, k := range keys {
		panel.Tree = append(panel.Tree, wswhiteboard.JSONTree("agenda/", k, agenda[k]))
	}
	return panel, true
}

func whiteboardPanels(in whiteboardPanelInput) []wswhiteboard.Panel {
	var panels []wswhiteboard.Panel
	for _, provider := range whiteboardProviders {
		if panel, ok := provider.panel(in); ok {
			panels = append(panels, panel)
		}
	}
	return panels
}

func whiteboardDirtyingTool(name string) bool {
	for _, provider := range whiteboardProviders {
		for _, dirty := range provider.dirtiedBy {
			if dirty == name {
				return true
			}
		}
	}
	return false
}

// handleWhiteboardUpdate applies a whiteboard.update call to the board of
// args' session_key. sublead is non-nil for a marked sub-lead's call, which
// callTool already rewrote onto its top-level lead's key: Apply then admits
// status commands only and attributes them (wswhiteboard.Env.StatusAuthor),
// refusals name the whiteboard-status playbook, and success is one
// acknowledgement line. A sub-lead write never enables the lead's board knob
// and never returns the lead directives (the page URL, the applied info). It
// stamps no user inputs, which the lead's own next entry answers.
func (s *Server) handleWhiteboardUpdate(id json.RawMessage, args map[string]any, sublead *subleadBoard) response {
	const tool = "whiteboard.update"
	key, err := sessionStateKey(tool, args)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	refusal := func(problems []string) string {
		if sublead != nil {
			return s.commandRefusal(tool, problems, whiteboardStatusPlaybookName)
		}
		return s.whiteboardRefusal(tool, problems)
	}
	// The JSON payload object was replaced by the command string; the
	// precheck defers a call carrying it here (handlerOwnedArguments), so the
	// refusal names the replacement instead of a generic unknown argument.
	if _, present := args["payload"]; present {
		return toolErrorTextResponse(id, refusal([]string{
			"payload: no longer accepted; whiteboard.update takes commands, one string of wb: commands",
		}))
	}
	commands := ""
	if raw, present := args["commands"]; present {
		str, ok := raw.(string)
		if !ok {
			return toolErrorTextResponse(id, refusal([]string{
				"commands: must be a string of wb: commands, not a JSON value",
			}))
		}
		commands = str
	}
	var (
		problems []string
		outcome  wswhiteboard.Outcome
		observed boardMutationObservation
		root     string
	)
	err = s.sessions.mutateRecordPartsObserved(key, partWhiteboard, partWhiteboard, func(rec *sessionRecord) error {
		if rec.Whiteboard == nil {
			rec.Whiteboard = &wswhiteboard.State{}
		}
		env := wswhiteboard.Env{
			Now: time.Now(),
			// wb:panel src reads inside the session's project root, the root
			// board images resolve against.
			SourceRoot: rec.Root,
		}
		if sublead != nil {
			env.StatusAuthor = sublead.author
		}
		root = rec.Root
		beforeEntry := rec.Whiteboard.EntrySeq
		outcome, problems = wswhiteboard.Apply(rec.Whiteboard, commands, env)
		if len(problems) > 0 && (outcome.Accepted == 0 || outcome.Unpartitioned) {
			return errWhiteboardRefused
		}
		if sublead == nil {
			s.stampBoardInputs(key, rec.Whiteboard, beforeEntry)
		}
		return nil
	}, &observed)
	// A successful Apply is not a commit. Conversely a failed replay does
	// not undo a synced intent. This independent
	// best-effort append accepts the commit-to-ring crash gap: never replay
	// board commands or manufacture projection entries during recovery.
	if observed.Committed {
		s.recordWhiteboardEffects(key, outcome.Effects)
	}
	s.debugEvent("whiteboard.commit_outcome", map[string]any{"committed": observed.Committed})
	if errors.Is(err, errWhiteboardRefused) {
		text := refusal(problems)
		if outcome.Unpartitioned {
			text += "\nWhole input refused without writes; command boundaries or rendering prerequisites are not safe for partial application.\n"
		} else {
			text += whiteboardFailedRetry(outcome)
		}
		return toolErrorTextResponse(id, text)
	}
	if err != nil {
		if observed.Committed {
			err = fmt.Errorf("board committed; %w; do not resend accepted commands to recover missing conversation history", err)
		} else {
			err = fmt.Errorf("board commitment not confirmed; %w; inspect the board before retrying commands", err)
		}
		text := fmt.Sprintf("%s: %v\n", tool, err)
		if observed.Committed {
			text += whiteboardAcceptedResult(outcome)
		}
		for _, problem := range problems {
			text += "- " + problem + "\n"
		}
		text += whiteboardFailedRetry(outcome)
		return toolErrorTextResponse(id, text)
	}
	if s.whiteboardChanged != nil {
		s.whiteboardChanged(s.getenv(wsstate.EnvCacheHome), root, key)
	}
	if sublead != nil {
		return s.subleadWhiteboardResult(id, outcome, problems)
	}
	var sb strings.Builder
	if len(problems) > 0 {
		fmt.Fprintf(&sb, "applied: %d accepted commands committed; failed commands were not applied.\n", outcome.Accepted)
	}
	if len(outcome.IDs) > 0 {
		labels := make([]string, len(outcome.IDs))
		for i, itemID := range outcome.IDs {
			labels[i] = wswhiteboard.FormatID(itemID)
		}
		sb.WriteString("ids: " + strings.Join(labels, ", ") + "\n")
	}
	full := "page unavailable"
	if s.boardOwner {
		full = "page not yet served"
	}
	if url, ok := s.dashboardPageURL(key); ok {
		full = url
	}
	sb.WriteString("full: " + full + "\n")
	if outcome.Info != "" {
		sb.WriteString("info: " + outcome.Info + "\n")
	}
	if err := s.enableWhiteboardKnobForSession(key); err != nil {
		sb.WriteString("warning: " + err.Error() + "\n")
	}
	for _, summary := range outcome.Summaries {
		sb.WriteString(summary + "\n")
	}
	sb.WriteString(whiteboardAppliedInfo + "\n")
	if len(problems) > 0 {
		sb.WriteString("Problems:\n")
		for _, problem := range problems {
			sb.WriteString("- " + problem + "\n")
		}
		sb.WriteString(whiteboardFailedRetry(outcome))
		return toolErrorTextResponse(id, sb.String())
	}
	return toolTextResponse(id, sb.String(), nil)
}

// subleadWhiteboardResult is a committed sub-lead write's result: one
// acknowledgement line, or, when some commands failed, the accepted count,
// the problems, the whiteboard-status pointer, and the failed commands to
// resend.
func (s *Server) subleadWhiteboardResult(id json.RawMessage, out wswhiteboard.Outcome, problems []string) response {
	if len(problems) == 0 {
		return toolTextResponse(id, fmt.Sprintf("posted: %d status commands to your lead's whiteboard.\n", out.Accepted), nil)
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "posted: %d status commands to your lead's whiteboard; the failed commands were not applied. Problems:\n", out.Accepted)
	for _, problem := range problems {
		sb.WriteString("- " + problem + "\n")
	}
	fmt.Fprintf(&sb, "\nSyntax and commands: the `%s` playbook (%s/playbook.read).\n", whiteboardStatusPlaybookName, s.runtimeNamespace())
	sb.WriteString(whiteboardFailedRetry(out))
	return toolErrorTextResponse(id, sb.String())
}

func whiteboardFailedRetry(out wswhiteboard.Outcome) string {
	if out.Failed == "" {
		return ""
	}
	return "\n" + out.Failed + "\nCorrect and resend only the commands inside <wb:failed>, not the wrapper or accepted commands. After an uncertain commitment, inspect the board first; never replay the whole batch.\n"
}

func whiteboardAcceptedResult(out wswhiteboard.Outcome) string {
	text := fmt.Sprintf("applied: %d accepted commands committed.\n", out.Accepted)
	if len(out.IDs) > 0 {
		var ids []string
		for _, id := range out.IDs {
			ids = append(ids, wswhiteboard.FormatID(id))
		}
		text += "ids: " + strings.Join(ids, ", ") + "\n"
	}
	for _, summary := range out.Summaries {
		text += summary + "\n"
	}
	return text
}

// whiteboardAppliedInfo accompanies committed, successfully published updates.
const whiteboardAppliedInfo = "info: applied to the whiteboard; the user sees this content there. In whiteboard mode, do not repeat or summarize it in the conversation; write there only what the board does not carry."

func (s *Server) handleWhiteboardQuery(id json.RawMessage, args map[string]any) response {
	const tool = "whiteboard.query"
	key, err := sessionStateKey(tool, args)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	rec, err := s.sessions.readWhiteboard(key)
	if err != nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: %w", tool, err))
	}
	result, problems := wswhiteboard.Query(rec.Whiteboard, args["payload"])
	if len(problems) > 0 {
		return toolErrorTextResponse(id, s.whiteboardRefusal(tool, problems))
	}
	return toolTextResponse(id, wswhiteboard.FormatQuery(result), nil)
}

// whiteboardLoadTool replaces the caller's board with a board zip (the
// dashboard's board download). It is top-level-lead-only with no sub-lead
// pass-through, so it is not an isWhiteboardTool name: that gate would
// rewrite a sub-lead key onto its lead's board.
const whiteboardLoadTool = "whiteboard.load"

// handleWhiteboardLoad loads the zip at args' path into the caller's board
// folder through the store (wswhiteboard.FolderStore.LoadArchive), keeping
// the previous board as a backup folder beside it. Afterwards it behaves like
// a successful whiteboard.update: the cached board is dropped, the board knob
// turns on, and the dispatcher nudges the dashboard.
func (s *Server) handleWhiteboardLoad(id json.RawMessage, args map[string]any) response {
	key, err := sessionStateKey(whiteboardLoadTool, args)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	raw, _ := args["path"].(string)
	if strings.TrimSpace(raw) == "" {
		return toolTextResponse(id, "", fmt.Errorf("%s: path is required", whiteboardLoadTool))
	}
	rec, err := s.sessions.readStateWithError(key, partKey)
	if err != nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: %w", whiteboardLoadTool, err))
	}
	// The path is read on the machine running this server; a relative one
	// resolves against the session root.
	path := raw
	if !filepath.IsAbs(path) {
		path = filepath.Join(rec.Root, path)
	}
	if info, err := os.Stat(path); err != nil || info.IsDir() {
		return toolTextResponse(id, "", fmt.Errorf("%s: no board zip at %s on the host running ws-mcp", whiteboardLoadTool, path))
	}
	folder := whiteboardFolder(rec.Root, key)
	if err := registerWhiteboardExclude(s.gitRunner(), rec.Root, filepath.Dir(folder.Dir)); err != nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: register %s in the git exclude file: %w", whiteboardLoadTool, whiteboardDirRel, err))
	}
	backup, topics, err := folder.LoadArchive(path, time.Now())
	wswhiteboard.SharedFolderCache.Forget(folder.Dir)
	if err != nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: %s: %w", whiteboardLoadTool, path, err))
	}
	if s.whiteboardChanged != nil {
		s.whiteboardChanged(s.getenv(wsstate.EnvCacheHome), rec.Root, key)
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "loaded: %d topics from %s\n", topics, path)
	if backup != "" {
		sb.WriteString("backup: the previous board is in " + backup + "\n")
	} else {
		sb.WriteString("backup: none; this session had no board\n")
	}
	if err := s.enableWhiteboardKnobForSession(key); err != nil {
		sb.WriteString("warning: " + err.Error() + "\n")
	}
	return toolTextResponse(id, sb.String(), nil)
}

// whiteboardRefusal lists problems with a compact playbook pointer for update,
// or the playbook's query section for query. Command retries add their complete
// rejected payloads separately; the central argument precheck has no such context.
func (s *Server) whiteboardRefusal(tool string, problems []string) string {
	if tool != "whiteboard.query" {
		return s.commandRefusal(tool, problems, whiteboardPlaybookName)
	}
	var sb strings.Builder
	writeRefusalProblems(&sb, tool, problems)
	if grammar := s.whiteboardPlaybookSection("## Query"); grammar != "" {
		sb.WriteString("\n" + grammar + "\n")
	} else {
		fmt.Fprintf(&sb, "\nRead the %s playbook with %s/playbook.read for the input format.\n", whiteboardPlaybookName, s.runtimeNamespace())
	}
	return sb.String()
}

// whiteboardPlaybookSection returns the playbook's section under heading
// (heading through the line before the next heading of its level or higher).
func (s *Server) whiteboardPlaybookSection(heading string) string {
	rsrcRoot, err := s.resolveRsrcRoot("")
	if err != nil {
		return ""
	}
	body, _, err := renderPlaybookBody(s, rsrcRoot, whiteboardPlaybookName, nil, s.configOptions(), "", "", "", nil, "")
	if err != nil {
		return ""
	}
	return markdownSection(body, heading)
}

// markdownSection returns the text from the line equal to heading up to the
// next heading of the same or a higher level, trimmed; "" when absent.
func markdownSection(body, heading string) string {
	level := strings.Index(heading, " ")
	lines := strings.Split(body, "\n")
	start := -1
	inFence := false
	for i, line := range lines {
		if strings.HasPrefix(line, "```") {
			inFence = !inFence
		}
		if start < 0 {
			if strings.TrimSpace(line) == heading {
				start = i
			}
			continue
		}
		if !inFence && strings.HasPrefix(line, "#") {
			hashes := len(line) - len(strings.TrimLeft(line, "#"))
			if hashes <= level && strings.HasPrefix(line[hashes:], " ") {
				return strings.TrimSpace(strings.Join(lines[start:i], "\n"))
			}
		}
	}
	if start < 0 {
		return ""
	}
	return strings.TrimSpace(strings.Join(lines[start:], "\n"))
}

func responseIsError(resp response) bool {
	if resp.Error != nil {
		return true
	}
	result, ok := resp.Result.(map[string]any)
	if !ok {
		return false
	}
	isError, _ := result["isError"].(bool)
	return isError
}

// registerWhiteboardExclude adds /ai-docs/whiteboards/ to the repository's
// local exclude file in the common git dir, so every worktree is covered and
// nothing is committed. A root outside git has nothing to exclude.
func registerWhiteboardExclude(runner wsgit.Runner, root, dir string) error {
	if common, ok := whiteboardCommonDirs.Load(root); ok {
		// A removed or moved repository must not get a stray exclude file.
		if _, err := wsstate.Stat(common.(string)); err == nil {
			return registerPoolExclude(common.(string), root, dir)
		}
		whiteboardCommonDirs.Delete(root)
	}
	out, err := runner.RunGit(context.Background(), root, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return nil
	}
	common := strings.TrimSpace(string(out))
	whiteboardCommonDirs.Store(root, common)
	return registerPoolExclude(common, root, dir)
}

// whiteboardCommonDirs memoizes each root's git common dir, which a
// repository keeps for its lifetime, so registration stats instead of spawning a git
// subprocess. A root outside git is not remembered and is asked again.
var whiteboardCommonDirs sync.Map // root -> common dir

// liveReplyIDs returns the reply id of every session key on disk, in either
// layout. ok is false when the key store cannot be read.
func (s *sessionStore) liveReplyIDs() (map[string]bool, bool) {
	d, err := s.dirs()
	if err != nil {
		return nil, false
	}
	// listKeys reads directory names only, including in-progress claims.
	// The background sweep must not hold up a session response on s.mu.
	keys, err := s.listKeys(d)
	if err != nil {
		return nil, false
	}
	live := map[string]bool{}
	for _, key := range keys {
		live[wsmailbox.ReplyID(key)] = true
	}
	return live, true
}

// whiteboardEnabled reads the knob that adds the whiteboard section to the
// workflow manual and the knob-on variants of lead-discuss and lead-ticket,
// for this server's session harness. resolver carries the reader's scopes:
// the lead's session resolver, or a root-anchored one for a bootstrap render
// that has no session yet; it must be built with builtinConfigDefaults so the
// bare key's builtin applies.
func (s *Server) whiteboardEnabled(resolver *wsconfig.Resolver, sessionKey string) (bool, error) {
	return s.bucketedKnobOn(resolver, sessionKey, wsconfig.ItemWorkflowWhiteboardEnabled)
}

// enableWhiteboardKnobForSession turns workflow.whiteboard_enabled on at
// session scope after a successful whiteboard.update, so a session that
// started using the board keeps the manual's whiteboard mandate (revive
// included) when the knob resolves off. See enableBucketedKnobForSession for
// the write rule. Callers reach it only past the top-level-lead gate on the
// whiteboard tools.
func (s *Server) enableWhiteboardKnobForSession(sessionKey string) error {
	if err := s.enableBucketedKnobForSession(sessionKey, wsconfig.ItemWorkflowWhiteboardEnabled); err != nil {
		return fmt.Errorf("whiteboard knob not enabled: %w", err)
	}
	return nil
}

// bucketedKnobOn reads a harness-bucketed on/off knob (item is the bare key)
// for this server's session harness. resolver must be built with
// builtinConfigDefaults so the bare key's builtin applies.
func (s *Server) bucketedKnobOn(resolver *wsconfig.Resolver, sessionKey, item string) (bool, error) {
	rv, _, err := resolveHarnessBucketed(resolver, sessionKey, item, s.playbookHarness())
	if err != nil {
		return false, fmt.Errorf("resolve %s: %w", item, err)
	}
	return strings.ToLower(strings.TrimSpace(rv.Value)) == whiteboardOnValue, nil
}

// enableBucketedKnobForSession turns a harness-bucketed on/off knob on at
// session scope when it resolves off. It writes what config.tune with scope
// session writes: the session's harness bucket. A session-scope value for the
// harness's read buckets or the bare key is the session's own choice and is
// left alone, so an explicit session off stands.
func (s *Server) enableBucketedKnobForSession(sessionKey, item string) error {
	resolver := s.sessionResolver(sessionKey, builtinConfigDefaults())
	on, err := s.bucketedKnobOn(&resolver, sessionKey, item)
	if err != nil {
		return err
	}
	if on {
		return nil
	}
	harness := s.playbookHarness()
	buckets := harnessBucketReadOrder(harness)
	storeKeys := []string{item}
	for _, bucket := range buckets {
		storeKeys = append(storeKeys, harnessBucketKey(item, bucket))
	}
	for _, storeKey := range storeKeys {
		if value, ok := s.sessions.getOverride(sessionKey, storeKey); ok && strings.TrimSpace(value) != "" {
			return nil
		}
	}
	return resolver.Set(harnessBucketKey(item, buckets[0]), whiteboardOnValue,
		wsconfig.SetOptions{ExplicitScope: wsconfig.ScopeSession, SessionKey: sessionKey})
}
