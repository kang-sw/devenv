package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsgit"
	"github.com/kang-sw/devenv/internal/wsi18n"
	"github.com/kang-sw/devenv/internal/wsmailbox"
	"github.com/kang-sw/devenv/internal/wsstate"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// The session whiteboard: whiteboard.update applies a command string to the
// authoritative topic-folder store (wswhiteboard.Apply) and renders, in one
// call, the full page, its change stamp, and a new embed snapshot. The model,
// folder transactions, view, and templates live in internal/wswhiteboard;
// this file owns derived page publication and artifact lifetime.

const (
	whiteboardPlaybookName = "whiteboard"
	// whiteboardDirRel is the whiteboard folder under a session root. It is
	// created, and registered in the repository's local exclude file, at a
	// session's first render, never earlier.
	whiteboardDirRel = "ai-docs/whiteboards"
	// whiteboardGCMarker bookkeeps the once-a-day orphan scan (the
	// maybePrune marker pattern). Its leading dot keeps it out of the file
	// patterns the scan deletes.
	whiteboardGCMarker  = ".gc-marker"
	whiteboardGCCadence = 24 * time.Hour
	whiteboardOnValue   = "on"
)

var (
	errWhiteboardRefused = errors.New("whiteboard batch refused")
	errNoWhiteboard      = errors.New("session has no whiteboard")
	// whiteboardFilePattern matches the files a session's board writes:
	// <reply-id>.html, <reply-id>.stamp.js, <reply-id>-<hash>.html.
	whiteboardFilePattern = regexp.MustCompile(`^([a-z0-9]+)(?:\.html|\.stamp\.js|-[0-9a-f]+\.html)$`)
)

// Test seams for the rename retry (technical decision 119): a browser polling
// the stamp holds a brief read handle, and on Windows a rename over a file
// that is open fails with a sharing violation.
var (
	whiteboardRename          = os.Rename
	whiteboardRenameRetryable = renameSharingViolation
	whiteboardRenameAttempts  = 5
	whiteboardRenameBackoff   = 40 * time.Millisecond
)

func isWhiteboardTool(name string) bool {
	return name == "whiteboard.update" || name == "whiteboard.query"
}

func whiteboardLeadOnlyDenial(name string) string {
	return fmt.Sprintf("%s is not available to a spawned session: the whiteboard is the top-level lead's channel to its user. Put what the user should see in your final report; your lead decides what reaches the board.", name)
}

// whiteboardSectionProvider supplies one auxiliary panel of both pages and
// declares which tool calls dirty it. Only real sections are registered; a
// provider with nothing to show yields no panel. A provider gathers its data
// once per call and shapes the rows for in.layout, so the two pages never
// drift apart.
type whiteboardSectionProvider struct {
	id        string
	dirtiedBy []string
	panel     func(in whiteboardPanelInput) (wswhiteboard.Panel, bool)
}

// whiteboardPanelInput is everything a provider reads, resolved once by the
// renderer.
type whiteboardPanelInput struct {
	rec    sessionRecord
	layout wswhiteboard.Layout
	// root is the session root, where the ticket directories live; env is
	// the environment the ticket read's git runs under (nil means this
	// process's, the daemon's served page).
	root       string
	env        []string
	sessionKey string
	replyID    string
	// cacheHome is the WS_CACHE_HOME the session's mailbox history ring
	// lives under; empty means this process's. paths resolves the mailbox
	// stores, reply registry, and listening marker; the zero value is this
	// process's environment (the daemon's served page).
	cacheHome string
	paths     wsmailbox.Paths
	// mailbox is this process's mailbox identity and whether the session
	// holds its named inbox (mailboxOwnerCheck), which the own address and
	// the named-inbox unread count depend on.
	mailbox whiteboardMailbox
	// served builds the panels for a page served over http (the dashboard
	// daemon's session page) rather than one written to disk: a ticket row
	// carries its repo-relative path as text instead of a file link.
	served bool
	// lang is the language of the text the panels draw (titles, counts, row
	// phrases; a wsi18n code). Empty is English, which is what a page
	// written to disk uses; only a served page follows its viewer.
	lang string
	// watch, when not nil, is told each file and directory a panel read
	// that a writer may change without a nudge (the ticket directories and
	// files, the mailbox stores, history directory, and listening marker),
	// which a served page names for its refresh check.
	watch func(path string)
	// history, when not nil, reads the session's mailbox history in place
	// of readMailboxHistory (the served page's cached read).
	history func() ([]mailboxHistoryRecord, error)
	// replies, when not nil, reads the reply-id registry in place of
	// wsmailbox.LoadReplyStore (the served page's cached read); false when
	// it cannot be read.
	replies func() (wsmailbox.ReplyStore, bool)
}

type whiteboardMailbox struct {
	owner    bool
	identity mailboxIdentity
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

// todoPanel lists the unfinished todos in store order. The full page folds the
// done ones below them; the embed keeps the count and omits them.
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
		case in.layout != wswhiteboard.LayoutEmbed:
			folded = append(folded, row)
		}
	}
	count := len(todos) - len(rows)
	return wswhiteboard.Panel{ID: "todo", Title: wsi18n.T(in.lang, "panel.todo"), Icon: "todo", Count: fmt.Sprintf("%d/%d", count, len(todos)), Rows: rows, Folded: folded}, true
}

// agendaPanel shows the agenda blobs in key order. The full page (on disk
// and served) lays each blob out whole as a foldable tree: the user asked to
// read the agendas there, which reverses the earlier rule that the panel
// never shows a blob body. The embed, a frozen snapshot kept in chat
// history, stays compact: one row per key with the one-line summary
// agenda.list produces.
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
		if in.layout == wswhiteboard.LayoutEmbed {
			panel.Rows = append(panel.Rows, wswhiteboard.PanelRow{Label: k, Text: agendaSummary(agenda[k])})
		} else {
			panel.Tree = append(panel.Tree, wswhiteboard.JSONTree("agenda/", k, agenda[k]))
		}
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

func (s *Server) handleWhiteboardUpdate(id json.RawMessage, args map[string]any) response {
	const tool = "whiteboard.update"
	key, err := sessionStateKey(tool, args)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	// The JSON payload object was replaced by the command string; the
	// precheck defers a call carrying it here (handlerOwnedArguments), so the
	// refusal names the replacement instead of a generic unknown argument.
	if _, present := args["payload"]; present {
		return toolErrorTextResponse(id, s.whiteboardRefusal(tool, []string{
			"payload: no longer accepted; whiteboard.update takes commands, one string of wb: commands",
		}))
	}
	commands := ""
	if raw, present := args["commands"]; present {
		str, ok := raw.(string)
		if !ok {
			return toolErrorTextResponse(id, s.whiteboardRefusal(tool, []string{
				"commands: must be a string of wb: commands, not a JSON value",
			}))
		}
		commands = str
	}
	var (
		problems []string
		outcome  wswhiteboard.Outcome
		render   whiteboardRender
	)
	err = s.sessions.mutateRecordParts(key, partAll, partWhiteboard, func(rec *sessionRecord) error {
		if rec.Whiteboard == nil {
			rec.Whiteboard = &wswhiteboard.State{}
		}
		env := wswhiteboard.Env{
			Now: time.Now(),
			IsDir: func(path string) bool {
				info, err := os.Stat(path)
				return err == nil && info.IsDir()
			},
			IsAbs: filepath.IsAbs,
		}
		outcome, problems = wswhiteboard.Apply(rec.Whiteboard, commands, env)
		if len(problems) > 0 {
			return errWhiteboardRefused
		}
		var renderErr error
		render, renderErr = s.renderWhiteboard(rec, key, true)
		return renderErr
	})
	if errors.Is(err, errWhiteboardRefused) {
		return toolErrorTextResponse(id, s.whiteboardRefusal(tool, problems))
	}
	if err != nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: %w", tool, err))
	}
	var sb strings.Builder
	if len(outcome.IDs) > 0 {
		labels := make([]string, len(outcome.IDs))
		for i, itemID := range outcome.IDs {
			labels[i] = wswhiteboard.FormatID(itemID)
		}
		sb.WriteString("ids: " + strings.Join(labels, ", ") + "\n")
	}
	full := render.full
	if url, ok := s.dashboardPageURL(key); ok {
		full = url
	}
	sb.WriteString("full: " + full + "\n")
	sb.WriteString("embed: " + render.embed + "\n")
	if outcome.Info != "" {
		sb.WriteString("info: " + outcome.Info + "\n")
	}
	if err := s.enableWhiteboardKnobForSession(key); err != nil {
		sb.WriteString("warning: " + err.Error() + "\n")
	}
	return toolTextResponse(id, sb.String(), nil)
}

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

// whiteboardRefusal lists every problem and then the tool's own input
// grammar, read from the rsrc playbooks so the grammar has one source, so
// that one retry can fix the call: for whiteboard.update the shared command
// syntax and the whiteboard's "## Commands" section (commandRefusal), for
// whiteboard.query the "## Query" section. The central precheck cannot do
// this: it sees only the advertised argument names.
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

// refreshWhiteboardAfter re-renders the full page (stamp bumped, no embed)
// after a successful write by a tool that dirties a panel (see
// whiteboardProviders), once the session's board exists.
// The gate is board existence, not the knob: a session that never rendered
// writes no file, and an ad hoc board stays current.
func (s *Server) refreshWhiteboardAfter(args map[string]any, resp response) {
	if responseIsError(resp) {
		return
	}
	key, _ := args["session_key"].(string)
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	if rec, ok := s.sessions.readState(key, partWhiteboard); !ok || rec.Whiteboard == nil {
		return
	}
	err := s.sessions.mutateRecordParts(key, partAll, partWhiteboard, func(rec *sessionRecord) error {
		rec.Whiteboard.BeginRender()
		_, err := s.renderWhiteboard(rec, key, false)
		return err
	})
	if err != nil && !errors.Is(err, errNoWhiteboard) {
		s.debugEvent("whiteboard.refresh_failed", map[string]any{"error": err.Error()})
	}
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

// renderWhiteboard prepares derived pages and embed bookkeeping. The folder
// transaction publishes them only after its durable commit, with the stamp
// last. The returned paths are relative to the root (a host embed is bare).
func (s *Server) renderWhiteboard(rec *sessionRecord, sessionKey string, withEmbed bool) (whiteboardRender, error) {
	var out whiteboardRender
	st := rec.Whiteboard
	replyID := wsmailbox.ReplyID(sessionKey)
	dir := filepath.Join(rec.Root, filepath.FromSlash(whiteboardDirRel))
	if err := registerWhiteboardExclude(s.gitRunner(), rec.Root, dir); err != nil {
		return out, fmt.Errorf("register %s in the git exclude file: %w", whiteboardDirRel, err)
	}
	pageDir := filepath.Join(dir, replyID)
	panelInput := whiteboardPanelInput{
		rec:        *rec,
		root:       rec.Root,
		env:        s.environ(),
		sessionKey: sessionKey,
		replyID:    replyID,
		cacheHome:  s.getenv(wsstate.EnvCacheHome),
		paths:      s.mailboxPaths(),
		mailbox:    s.whiteboardMailbox(sessionKey, rec.Root),
	}
	viewInput := func(layout wswhiteboard.Layout) wswhiteboard.ViewInput {
		in := panelInput
		in.layout = layout
		return wswhiteboard.ViewInput{
			Layout:        layout,
			ReplyID:       replyID,
			Panels:        whiteboardPanels(in),
			FileLink:      whiteboardItemFileLink(rec.Root, whiteboardPageDir(in)),
			FullBoardLink: whiteboardFileLink(rec.Root, whiteboardPageDir(in), filepath.Join(pageDir, "index.html")),
		}
	}

	if withEmbed {
		html, err := wswhiteboard.RenderPage(wswhiteboard.BuildView(st, viewInput(wswhiteboard.LayoutEmbed)))
		if err != nil {
			return out, fmt.Errorf("render embed: %w", err)
		}
		name := replyID + "-" + whiteboardEmbedHash(st.Seq, html) + ".html"
		target := dir
		if st.EmbedDir != "" {
			target = st.EmbedDir
		}
		out.files = append(out.files, whiteboardOutput{filepath.Join(target, name), html, "embed"})
		if st.EmbedDir != "" {
			// The host owns its folder's lifetime: the server never evicts
			// there, since the host re-reads a visual's file when it redraws
			// its chat history.
			out.embed = name
		} else {
			st.Embeds = append(st.Embeds, wswhiteboard.Embed{Seq: st.Seq, Name: name})
			out.evicted = evictWhiteboardEmbeds(st, dir)
			out.embed = whiteboardDirRel + "/" + name
		}
		st.LastEmbedSeq = st.Seq
	}

	view := wswhiteboard.BuildView(st, viewInput(wswhiteboard.LayoutFull))
	view.StampFile = "index.stamp.js"
	html, err := wswhiteboard.RenderPage(view)
	if err != nil {
		return out, fmt.Errorf("render full page: %w", err)
	}
	out.files = append(out.files, whiteboardOutput{filepath.Join(pageDir, "index.html"), html, "full page"})
	stamp := fmt.Sprintf("wbStamp(%q);\n", fmt.Sprint(st.Seq))
	out.files = append(out.files, whiteboardOutput{filepath.Join(pageDir, "index.stamp.js"), stamp, "stamp"})
	out.full = whiteboardDirRel + "/" + replyID + "/index.html"
	rec.boardOutput = &out
	return out, nil
}

// ensureWhiteboardMermaid writes the shared mermaid script into dir, before
// the page that loads it, when that page holds a mermaid block and the file
// is missing: about 3 MB is written once per folder, never per update. Its
// name carries the asset's hash, so an upgrade adds a file instead of
// replacing one an older page still loads, and it matches no whiteboard GC
// pattern.
func ensureWhiteboardMermaid(dir, html string) error {
	if !wswhiteboard.UsesMermaid(html) {
		return nil
	}
	name, data := wswhiteboard.MermaidAsset()
	path := filepath.Join(dir, name)
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if err := writeWhiteboardFile(path, data); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	return nil
}

// whiteboardRender is derived output prepared during a store mutation.
// Publication and retention run under the folder lock after commit.
type whiteboardRender struct {
	full, embed string
	files       []whiteboardOutput
	evicted     []string
}

type whiteboardOutput struct {
	path, text, label string
}

// Publication follows the durable store commit; a failed page write never
// rolls back accepted history or publishes a stamp ahead of its page.
func (r whiteboardRender) publish() error {
	for _, file := range r.files {
		dir := filepath.Dir(file.path)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		if strings.HasSuffix(file.path, ".html") {
			if err := ensureWhiteboardMermaid(dir, file.text); err != nil {
				return err
			}
		}
		if err := writeWhiteboardFile(file.path, []byte(file.text)); err != nil {
			return fmt.Errorf("board committed; write %s: %w", file.label, err)
		}
	}
	for _, path := range r.evicted {
		_ = os.Remove(path)
	}
	return nil
}

// whiteboardEmbedHash derives an embed's short hash from the render sequence
// together with the content, so two identical renders still get distinct
// names: hosts expect a new file per visual, and retention deletes recorded
// names.
func whiteboardEmbedHash(seq int, html string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d\n%s", seq, html)))
	return hex.EncodeToString(sum[:])[:10]
}

// evictWhiteboardEmbeds keeps the newest EmbedRetention embeds in the
// repository folder, ordered by render sequence, never by file mtime, and
// returns the paths of the dropped ones for the caller to delete.
func evictWhiteboardEmbeds(st *wswhiteboard.State, dir string) []string {
	if len(st.Embeds) <= wswhiteboard.EmbedRetention {
		return nil
	}
	sort.SliceStable(st.Embeds, func(i, j int) bool { return st.Embeds[i].Seq < st.Embeds[j].Seq })
	drop := len(st.Embeds) - wswhiteboard.EmbedRetention
	var paths []string
	for _, old := range st.Embeds[:drop] {
		paths = append(paths, filepath.Join(dir, old.Name))
	}
	st.Embeds = append([]wswhiteboard.Embed(nil), st.Embeds[drop:]...)
	return paths
}

// writeWhiteboardFile replaces path through a temp file and a rename, so a
// reader never sees a torn file. The temp name starts with a dot, outside
// the patterns the orphan scan deletes.
func writeWhiteboardFile(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	// CreateTemp makes the file 0600; a browser or host reading the page may
	// run as another user.
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := renameWithRetry(tmpName, path); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}

// renameWithRetry retries a rename the platform refused because another
// process briefly holds the target open, before giving up.
func renameWithRetry(from, to string) error {
	var err error
	for attempt := 0; attempt < whiteboardRenameAttempts; attempt++ {
		if attempt > 0 {
			time.Sleep(whiteboardRenameBackoff)
		}
		err = whiteboardRename(from, to)
		if err == nil || !whiteboardRenameRetryable(err) {
			return err
		}
	}
	return err
}

// renameSharingViolation reports a Windows rename refused because the target
// is open elsewhere: ERROR_SHARING_VIOLATION (32), or ERROR_ACCESS_DENIED (5),
// which MoveFileEx returns for a target opened without delete sharing.
func renameSharingViolation(err error) bool {
	if runtime.GOOS != "windows" {
		return false
	}
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return errno == 32 || errno == 5
	}
	return false
}

// registerWhiteboardExclude adds /ai-docs/whiteboards/ to the repository's
// local exclude file in the common git dir, so every worktree is covered and
// nothing is committed. A root outside git has nothing to exclude.
func registerWhiteboardExclude(runner wsgit.Runner, root, dir string) error {
	out, err := runner.RunGit(context.Background(), root, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return nil
	}
	return registerPoolExclude(strings.TrimSpace(string(out)), root, dir)
}

// maybeCollectWhiteboards deletes, at most once a day per whiteboard folder
// and only when the folder exists, every whiteboard file whose reply id
// matches no live session key record. A host embed folder is never scanned.
func (s *Server) maybeCollectWhiteboards(root string) {
	if strings.TrimSpace(root) == "" {
		return
	}
	dir := filepath.Join(root, filepath.FromSlash(whiteboardDirRel))
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return
	}
	marker := filepath.Join(dir, whiteboardGCMarker)
	if info, err := os.Stat(marker); err == nil && time.Since(info.ModTime()) < whiteboardGCCadence {
		return
	}
	live, ok := s.sessions.liveReplyIDs()
	if !ok {
		return // an unreadable key store must never read as "no session is live"
	}
	f, err := os.OpenFile(marker, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return
	}
	f.Close()
	now := time.Now()
	_ = os.Chtimes(marker, now, now)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		match := whiteboardFilePattern.FindStringSubmatch(entry.Name())
		if match == nil || live[match[1]] {
			continue
		}
		_ = os.Remove(filepath.Join(dir, entry.Name()))
	}
}

// liveReplyIDs returns the reply id of every session key on disk, in either
// layout. ok is false when the key store cannot be read.
func (s *sessionStore) liveReplyIDs() (map[string]bool, bool) {
	d, err := s.dirs()
	if err != nil {
		return nil, false
	}
	s.mu.Lock()
	keys, err := s.listKeys(d)
	s.mu.Unlock()
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
	rv, _, err := resolveHarnessBucketed(resolver, sessionKey, item, s.currentHarness())
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
	harness := s.currentHarness()
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
