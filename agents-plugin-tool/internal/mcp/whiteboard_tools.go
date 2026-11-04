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
	"github.com/kang-sw/devenv/internal/wsmailbox"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// The session whiteboard: whiteboard.update applies a batch to the item
// store in the session state record (wswhiteboard.Apply) and renders, in one
// call, the full page, its change stamp, and a new embed snapshot. The pure
// store, view, and templates live in internal/wswhiteboard; this file owns
// the files on disk and their lifecycle.

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
// provider with nothing to show yields no panel.
type whiteboardSectionProvider struct {
	id        string
	dirtiedBy []string
	panel     func(rec sessionRecord) (wswhiteboard.Panel, bool)
}

var whiteboardProviders = []whiteboardSectionProvider{
	{
		id:        "todo",
		dirtiedBy: []string{"todo.add", "todo.check", "todo.erase", "todo.clear", "todo.reorder", "route.resolve_implement", "route.resolve_proceed"},
		panel:     todoPanel,
	},
	{
		id:        "agenda",
		dirtiedBy: []string{"agenda.set", "agenda.clear", "route.resolve_implement", "route.resolve_proceed"},
		panel:     agendaPanel,
	},
}

func todoPanel(rec sessionRecord) (wswhiteboard.Panel, bool) {
	if len(rec.Todos) == 0 {
		return wswhiteboard.Panel{}, false
	}
	done := 0
	rows := make([]wswhiteboard.PanelRow, 0, len(rec.Todos))
	for _, todo := range rec.Todos {
		if todo.Status == todoDone {
			done++
		}
		rows = append(rows, wswhiteboard.PanelRow{State: string(todo.Status), Label: todo.Key, Text: todo.Title})
	}
	return wswhiteboard.Panel{ID: "todo", Title: "Todos", Icon: "todo", Count: fmt.Sprintf("%d/%d", done, len(rec.Todos)), Rows: rows}, true
}

// agendaPanel shows each agenda blob as its key and the one-line summary
// agenda.list produces, never the blob body: agendas are the lead's working
// notes and may hold unconfirmed reasoning.
func agendaPanel(rec sessionRecord) (wswhiteboard.Panel, bool) {
	if len(rec.Agenda) == 0 {
		return wswhiteboard.Panel{}, false
	}
	keys := make([]string, 0, len(rec.Agenda))
	for k := range rec.Agenda {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	rows := make([]wswhiteboard.PanelRow, 0, len(keys))
	for _, k := range keys {
		rows = append(rows, wswhiteboard.PanelRow{Label: k, Text: agendaSummary(rec.Agenda[k])})
	}
	return wswhiteboard.Panel{ID: "agenda", Title: "Agendas", Icon: "agenda", Count: fmt.Sprint(len(keys)), Rows: rows}, true
}

func whiteboardPanels(rec sessionRecord) []wswhiteboard.Panel {
	var panels []wswhiteboard.Panel
	for _, provider := range whiteboardProviders {
		if panel, ok := provider.panel(rec); ok {
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
	payload := args["payload"]
	var (
		problems []string
		outcome  wswhiteboard.Outcome
		full     string
		embed    string
	)
	err = s.sessions.mutateRecord(key, func(rec *sessionRecord) error {
		if rec.Whiteboard == nil {
			rec.Whiteboard = &wswhiteboard.State{}
		}
		root := rec.Root
		env := wswhiteboard.Env{
			Now: time.Now(),
			ReadFile: func(path string) ([]byte, error) {
				if !filepath.IsAbs(path) {
					path = filepath.Join(root, path)
				}
				return os.ReadFile(path)
			},
			IsDir: func(path string) bool {
				info, err := os.Stat(path)
				return err == nil && info.IsDir()
			},
			IsAbs: filepath.IsAbs,
		}
		outcome, problems = wswhiteboard.Apply(rec.Whiteboard, payload, env)
		if len(problems) > 0 {
			return errWhiteboardRefused
		}
		var renderErr error
		full, embed, renderErr = renderWhiteboard(rec, key, true)
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
	sb.WriteString("full: " + full + "\n")
	sb.WriteString("embed: " + embed + "\n")
	return toolTextResponse(id, sb.String(), nil)
}

func (s *Server) handleWhiteboardQuery(id json.RawMessage, args map[string]any) response {
	const tool = "whiteboard.query"
	key, err := sessionStateKey(tool, args)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	rec, ok := s.sessions.readState(key)
	if !ok {
		return toolTextResponse(id, "", fmt.Errorf("%s: session key not found: %s", tool, key))
	}
	items, problems := wswhiteboard.Query(rec.Whiteboard, args["payload"])
	if len(problems) > 0 {
		return toolErrorTextResponse(id, s.whiteboardRefusal(tool, problems))
	}
	return toolTextResponse(id, wswhiteboard.FormatItems(items), nil)
}

// whiteboardRefusal lists every problem and then the inner payload schema,
// read from the whiteboard playbook so the schema has one source, so that
// one retry can fix the call. The central precheck cannot do this: it sees
// only the advertised {session_key, payload}.
func (s *Server) whiteboardRefusal(tool string, problems []string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s refused; nothing was applied. Problems:\n", tool)
	for _, problem := range problems {
		sb.WriteString("- " + problem + "\n")
	}
	if schema := s.whiteboardPayloadSection(); schema != "" {
		sb.WriteString("\n" + schema + "\n")
	} else {
		fmt.Fprintf(&sb, "\nRead the %s playbook with %s/playbook.read for the payload schema.\n", whiteboardPlaybookName, RuntimeNamespace())
	}
	return sb.String()
}

// whiteboardPayloadSection returns the playbook's "## Payload" section
// (heading through the line before the next level-2 heading).
func (s *Server) whiteboardPayloadSection() string {
	rsrcRoot, err := resolveRsrcRoot("")
	if err != nil {
		return ""
	}
	body, _, err := renderPlaybookBody(s, rsrcRoot, whiteboardPlaybookName, nil, wsconfig.Options{}, "", "", "", nil, "")
	if err != nil {
		return ""
	}
	return markdownSection(body, "## Payload")
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
// after a successful todo or agenda write, once the session's board exists.
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
	err := s.sessions.mutateRecord(key, func(rec *sessionRecord) error {
		if rec.Whiteboard == nil {
			return errNoWhiteboard
		}
		rec.Whiteboard.BeginRender()
		_, _, err := renderWhiteboard(rec, key, false)
		return err
	})
	if err != nil && !errors.Is(err, errNoWhiteboard) {
		appendDebugEvent("whiteboard.refresh_failed", map[string]any{"error": err.Error()})
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

// renderWhiteboard writes the session's pages for the current state: the
// embed snapshot when withEmbed, then the full page, then the stamp, so a
// reload triggered by the stamp never reads an older page. It returns the
// full page path relative to the session root and the embed path (relative,
// or the bare file name inside a host embed folder).
func renderWhiteboard(rec *sessionRecord, sessionKey string, withEmbed bool) (string, string, error) {
	st := rec.Whiteboard
	replyID := wsmailbox.ReplyID(sessionKey)
	dir := filepath.Join(rec.Root, filepath.FromSlash(whiteboardDirRel))
	if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", "", fmt.Errorf("create %s: %w", whiteboardDirRel, err)
		}
		if err := registerWhiteboardExclude(rec.Root, dir); err != nil {
			return "", "", fmt.Errorf("register %s in the git exclude file: %w", whiteboardDirRel, err)
		}
	}
	panels := whiteboardPanels(*rec)

	embedResult := ""
	if withEmbed {
		html, err := wswhiteboard.RenderPage(wswhiteboard.BuildView(st, wswhiteboard.ViewInput{Layout: wswhiteboard.LayoutEmbed, ReplyID: replyID, Panels: panels}))
		if err != nil {
			return "", "", fmt.Errorf("render embed: %w", err)
		}
		name := replyID + "-" + whiteboardEmbedHash(st.Seq, html) + ".html"
		target := dir
		if st.EmbedDir != "" {
			target = st.EmbedDir
		}
		if err := writeWhiteboardFile(filepath.Join(target, name), []byte(html)); err != nil {
			return "", "", fmt.Errorf("write embed: %w", err)
		}
		if st.EmbedDir != "" {
			// The host owns its folder's lifetime: the server never evicts
			// there, since the host re-reads a visual's file when it redraws
			// its chat history.
			embedResult = name
		} else {
			st.Embeds = append(st.Embeds, wswhiteboard.Embed{Seq: st.Seq, Name: name})
			evictWhiteboardEmbeds(st, dir)
			embedResult = whiteboardDirRel + "/" + name
		}
		st.LastEmbedSeq = st.Seq
	}

	html, err := wswhiteboard.RenderPage(wswhiteboard.BuildView(st, wswhiteboard.ViewInput{Layout: wswhiteboard.LayoutFull, ReplyID: replyID, Panels: panels}))
	if err != nil {
		return "", "", fmt.Errorf("render full page: %w", err)
	}
	if err := writeWhiteboardFile(filepath.Join(dir, replyID+".html"), []byte(html)); err != nil {
		return "", "", fmt.Errorf("write full page: %w", err)
	}
	stamp := fmt.Sprintf("wbStamp(%q);\n", fmt.Sprint(st.Seq))
	if err := writeWhiteboardFile(filepath.Join(dir, replyID+".stamp.js"), []byte(stamp)); err != nil {
		return "", "", fmt.Errorf("write stamp: %w", err)
	}
	return whiteboardDirRel + "/" + replyID + ".html", embedResult, nil
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
// repository folder, ordered by render sequence, never by file mtime.
func evictWhiteboardEmbeds(st *wswhiteboard.State, dir string) {
	if len(st.Embeds) <= wswhiteboard.EmbedRetention {
		return
	}
	sort.SliceStable(st.Embeds, func(i, j int) bool { return st.Embeds[i].Seq < st.Embeds[j].Seq })
	drop := len(st.Embeds) - wswhiteboard.EmbedRetention
	for _, old := range st.Embeds[:drop] {
		_ = os.Remove(filepath.Join(dir, old.Name))
	}
	st.Embeds = append([]wswhiteboard.Embed(nil), st.Embeds[drop:]...)
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
func registerWhiteboardExclude(root, dir string) error {
	out, err := wsgit.ExecRunner{}.RunGit(context.Background(), root, "rev-parse", "--path-format=absolute", "--git-common-dir")
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

// liveReplyIDs returns the reply id of every session key record on disk.
// ok is false when the key store cannot be read.
func (s *sessionStore) liveReplyIDs() (map[string]bool, bool) {
	dir, err := s.keysDir()
	if err != nil {
		return nil, false
	}
	s.mu.Lock()
	entries, err := os.ReadDir(dir)
	s.mu.Unlock()
	if err != nil {
		return nil, false
	}
	live := map[string]bool{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		key := strings.TrimSuffix(name, ".json")
		if sessionKeyFilenamePattern.MatchString(key) {
			live[wsmailbox.ReplyID(key)] = true
		}
	}
	return live, true
}

// workflowWhiteboardEnabled reads the global-only knob that adds the
// whiteboard section to the workflow manual and the knob-on variants of
// lead-discuss and lead-ticket.
func workflowWhiteboardEnabled(configOpts wsconfig.Options) (bool, error) {
	resolver := wsconfig.NewResolver(configOpts, builtinConfigDefaults(), nil, nil)
	rv, err := resolver.Get("", wsconfig.ItemWorkflowWhiteboardEnabled)
	if err != nil {
		return false, err
	}
	return strings.ToLower(strings.TrimSpace(rv.Value)) == whiteboardOnValue, nil
}
