package wsdashboard

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kang-sw/devenv/internal/wsinbox"
	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// The daemon's pages: / lists the sessions, /s/<reply-id> shows one. Both are
// built of regions, each rendered by its own template so the page can swap
// one region at a time when an SSE update names it (sse.go). The board and
// panels regions are the static whiteboard page's own fragments
// (wswhiteboard.RenderRegions); the rest render from the view model
// (pageview.go).

//go:embed assets/dashboard.tmpl assets/dashboard.css assets/dashboard.js assets/sprite.svg
var dashboardFS embed.FS

// Region names. The session page's regions in order; the index page has one.
//
// The conversation is two regions inside one scroll wrapper: entries (the
// finished messages of the live view, which the page merges by entry name
// and never re-renders) and convlive (the streaming reply, the approval or
// activity line, the held messages), which a streaming tick changes alone.
// The status band is two regions inside the composer: ctx (the context bar
// above the message box) and sline (the line below it).
const (
	regionHead     = "head"
	regionTree     = "tree"
	regionEntries  = "entries"
	regionConvLive = "convlive"
	regionCtx      = "ctx"
	regionSLine    = "sline"
	regionBoard    = "board"
	regionPanels   = "panels"
	regionSessions = "sessions"
	// regionMode is a pseudo-region with no element: its value is the page
	// mode, and a change tells the page that its layout no longer fits.
	regionMode = "mode"
)

// sessionRegions lists the regions a session page in mode holds.
func sessionRegions(mode string) []string {
	if mode == modeLive {
		return []string{regionHead, regionTree, regionEntries, regionConvLive, regionCtx, regionSLine, regionBoard, regionPanels}
	}
	return []string{regionHead, regionBoard, regionPanels}
}

type dashboardAssets struct {
	tmpl *template.Template
	data pageAssetsData
}

// pageAssetsData is what a page template inlines.
type pageAssetsData struct {
	CSS              template.CSS
	Script           template.JS
	Sprite           template.HTML
	WhiteboardCSS    template.CSS
	WhiteboardSprite template.HTML
	FormScript       template.JS
	MermaidInit      template.JS
	MermaidFile      string
}

var (
	dashboardAssetsOnce sync.Once
	loadedDashboard     *dashboardAssets
	dashboardAssetsErr  error

	symbolPattern = regexp.MustCompile(`<symbol id="i-([a-z0-9-]+)"`)
)

func loadDashboardAssets() (*dashboardAssets, error) {
	dashboardAssetsOnce.Do(func() {
		read := func(name string) string {
			raw, err := dashboardFS.ReadFile("assets/" + name)
			if err != nil && dashboardAssetsErr == nil {
				dashboardAssetsErr = err
			}
			return string(raw)
		}
		wb, err := wswhiteboard.PageAssets()
		if err != nil {
			dashboardAssetsErr = err
			return
		}
		mermaidName, _ := wswhiteboard.MermaidAsset()
		a := &dashboardAssets{data: pageAssetsData{
			CSS:              template.CSS(read("dashboard.css")),
			Script:           template.JS(read("dashboard.js")),
			Sprite:           template.HTML(strings.TrimSpace(read("sprite.svg"))),
			WhiteboardCSS:    wb.CSS,
			WhiteboardSprite: wb.Sprite,
			FormScript:       wb.FormScript,
			MermaidInit:      wb.MermaidInit,
			MermaidFile:      mermaidName,
		}}
		if dashboardAssetsErr != nil {
			return
		}
		icons := map[string]bool{}
		for _, sprite := range []string{string(a.data.Sprite), string(wb.Sprite)} {
			for _, m := range symbolPattern.FindAllStringSubmatch(sprite, -1) {
				icons[m[1]] = true
			}
		}
		funcs := template.FuncMap{
			"icon": func(name string) (template.HTML, error) {
				if !icons[name] {
					return "", fmt.Errorf("dashboard page: no sprite symbol %q", name)
				}
				return template.HTML(`<svg class="i" aria-hidden="true"><use href="#i-` + name + `"/></svg>`), nil
			},
			"region":   renderRegionElement,
			"clock":    clockText,
			"spanText": spanText,
			"toolName": toolName,
			"shortID":  shortID,
			"marked":   markedText,
			// The tool summary's token and batch lines.
			"tokenText":  tokenText,
			"tokenTitle": tokenTitle,
			"batchMeta":  batchMeta,
		}
		a.tmpl, dashboardAssetsErr = template.New("dashboard").Funcs(funcs).ParseFS(dashboardFS, "assets/dashboard.tmpl")
		if dashboardAssetsErr == nil {
			loadedDashboard = a
		}
	})
	return loadedDashboard, dashboardAssetsErr
}

// clockText is a time's fallback text, UTC and fixed, so a region's HTML
// does not change with the time it was rendered at; the page script replaces
// it with local or relative time.
func clockText(ms int64) string {
	if ms <= 0 {
		return ""
	}
	return time.UnixMilli(ms).UTC().Format("15:04:05Z")
}

// toolName is a tool's display name: an MCP tool, mcp__<server>__<tool>, shows
// as <tool> (the server name may itself hold single underscores). The full
// name stays in a title attribute where the page shows it.
func toolName(name string) string {
	if rest, ok := strings.CutPrefix(name, "mcp__"); ok {
		if _, tool, found := strings.Cut(rest, "__"); found && tool != "" {
			return tool
		}
	}
	return name
}

// shortID is the leading part of an agent or reply id, enough to tell rows
// apart without a 17-character hex string taking the row.
func shortID(id string) string {
	if r := []rune(id); len(r) > 6 {
		return string(r[:6])
	}
	return id
}

// spanText is a duration's text.
func spanText(ms int64) string {
	d := time.Duration(ms) * time.Millisecond
	switch {
	case d < time.Minute:
		return strconv.Itoa(int(d/time.Second)) + "s"
	case d < time.Hour:
		return fmt.Sprintf("%dm %ds", int(d/time.Minute), int(d%time.Minute/time.Second))
	}
	return fmt.Sprintf("%dh %dm", int(d/time.Hour), int(d%time.Hour/time.Minute))
}

// batchSpanText is one batch's duration: tenths of a second under ten
// seconds, since most batches take one or two, else spanText.
func batchSpanText(ms int64) string {
	switch {
	case ms < 100:
		return "<0.1s"
	case ms < 10_000:
		return strconv.FormatFloat(float64(ms)/1000, 'f', 1, 64) + "s"
	}
	return spanText(ms)
}

// compactTokens is a token count in compact form: 950, 1.2k, 12.3k, 210k, 1.4M.
func compactTokens(n int64) string {
	switch {
	case n < 1000:
		return strconv.FormatInt(n, 10)
	case n < 99_950:
		return strconv.FormatFloat(float64(n)/1000, 'f', 1, 64) + "k"
	case n < 999_500:
		return strconv.FormatInt((n+500)/1000, 10) + "k"
	}
	return strconv.FormatFloat(float64(n)/1e6, 'f', 1, 64) + "M"
}

// tokenText is a token usage in one compact line: input and output, then
// prompt-cache reads and writes when there were any.
func tokenText(u *wslive.TokenUsage) string {
	if u == nil {
		return ""
	}
	parts := []string{"in " + compactTokens(u.Input), "out " + compactTokens(u.Output)}
	if u.CacheRead > 0 {
		parts = append(parts, "cache "+compactTokens(u.CacheRead))
	}
	if u.CacheWrite > 0 {
		parts = append(parts, "write "+compactTokens(u.CacheWrite))
	}
	return strings.Join(parts, " · ")
}

// tokenTitle is a token usage's exact counts, for a title attribute.
func tokenTitle(u *wslive.TokenUsage) string {
	if u == nil {
		return ""
	}
	return fmt.Sprintf("tokens: %d input · %d output · %d cache read · %d cache write", u.Input, u.Output, u.CacheRead, u.CacheWrite)
}

// batchMeta is a tool summary batch line's trailing text: its duration,
// failed calls, and the request's tokens. A batch without per-call tools
// (an older adapter's) leads with its call count, which its chips cannot
// show; so does one whose calls outnumber those it named.
func batchMeta(b wslive.ToolBatch) string {
	var parts []string
	named := 0
	for _, t := range b.Tools {
		named += t.Calls
	}
	if named < b.Calls {
		if b.Calls == 1 {
			parts = append(parts, "1 call")
		} else {
			parts = append(parts, strconv.Itoa(b.Calls)+" calls")
		}
	}
	if b.DurationMs > 0 {
		parts = append(parts, batchSpanText(b.DurationMs))
	}
	if b.Failed > 0 {
		parts = append(parts, strconv.Itoa(b.Failed)+" failed")
	}
	if b.Usage != nil {
		parts = append(parts, tokenText(b.Usage))
	}
	return strings.Join(parts, " · ")
}

// renderedRegion is one region's HTML and its content hash.
type renderedRegion struct {
	HTML template.HTML
	Hash string
}

// regionSet is a page's rendered regions by name.
type regionSet map[string]renderedRegion

func newRegion(html string) renderedRegion {
	sum := sha256.Sum256([]byte(html))
	return renderedRegion{HTML: template.HTML(html), Hash: hex.EncodeToString(sum[:8])}
}

// regionHost is page data that carries rendered regions.
type regionHost interface {
	regionByName(name string) renderedRegion
}

// renderRegionElement wraps a rendered region in the element the page swaps.
func renderRegionElement(host regionHost, name string) template.HTML {
	r := host.regionByName(name)
	return template.HTML(`<div id="r-`+name+`" class="region r-`+name+`" data-region="`+name+`" data-h="`+r.Hash+`">`) + r.HTML + `</div>`
}

type sessionPageData struct {
	SessionView
	Assets pageAssetsData
	// SysOpen offers the link menu's "open in system browser" item: the
	// page was asked for by a browser on this machine. It is outside every
	// region, since regions are shared by every viewer's stream.
	SysOpen bool
	regions regionSet
}

func (p sessionPageData) regionByName(name string) renderedRegion { return p.regions[name] }

type indexPageData struct {
	IndexView
	Assets pageAssetsData
	// LAN is the LAN access block, set only for a local viewer in LAN mode.
	// It is outside every region, so no SSE update ever carries the token.
	LAN     *LANAccess
	regions regionSet
}

func (p indexPageData) regionByName(name string) renderedRegion { return p.regions[name] }

// renderSessionRegions renders every region of a session page, taking each
// conversation entry from cache (nil renders every entry).
func renderSessionRegions(view SessionView, cache *entryCache) (regionSet, error) {
	a, err := loadDashboardAssets()
	if err != nil {
		return nil, err
	}
	if err := renderEntries(a, cache, view.ReplyID, &view.Conversation); err != nil {
		return nil, err
	}
	regions := regionSet{regionMode: newRegion(view.Mode())}
	for _, name := range sessionRegions(view.Mode()) {
		var buf bytes.Buffer
		if err := a.tmpl.ExecuteTemplate(&buf, "r-"+name, view); err != nil {
			return nil, fmt.Errorf("dashboard region %s: %w", name, err)
		}
		regions[name] = newRegion(buf.String())
	}
	return regions, nil
}

// renderEntries sets each entry's Rendered element from cache, rendering it
// (a lead reply's markdown, then the entry template) only on a miss.
func renderEntries(a *dashboardAssets, cache *entryCache, replyID string, cv *ConversationView) error {
	for i := range cv.Entries {
		e := cv.Entries[i]
		html, err := cache.render(replyID, e, func() (template.HTML, error) {
			if e.Kind == wslive.RingReply && cv.markdown != nil {
				e.HTML = cv.markdown(e.Text)
			}
			var buf bytes.Buffer
			if err := a.tmpl.ExecuteTemplate(&buf, "entry", e); err != nil {
				return "", fmt.Errorf("dashboard conversation entry %s: %w", e.Key, err)
			}
			return template.HTML(buf.String()), nil
		})
		if err != nil {
			return err
		}
		cv.Entries[i].Rendered = html
	}
	return nil
}

// renderedEntries joins rendered entries into one fragment.
func renderedEntries(entries []ConvEntry) string {
	var b strings.Builder
	for _, e := range entries {
		b.WriteString(string(e.Rendered))
	}
	return b.String()
}

// renderSessionPage renders the whole session page around its regions;
// sysOpen offers the system-open link action (sessionPageData.SysOpen).
func renderSessionPage(view SessionView, regions regionSet, sysOpen bool) (string, error) {
	a, err := loadDashboardAssets()
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	err = a.tmpl.ExecuteTemplate(&buf, "session", sessionPageData{SessionView: view, Assets: a.data, SysOpen: sysOpen, regions: regions})
	return buf.String(), err
}

func renderIndexRegions(view IndexView) (regionSet, error) {
	a, err := loadDashboardAssets()
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := a.tmpl.ExecuteTemplate(&buf, "r-"+regionSessions, view); err != nil {
		return nil, fmt.Errorf("dashboard region %s: %w", regionSessions, err)
	}
	return regionSet{regionSessions: newRegion(buf.String())}, nil
}

func renderIndexPage(view IndexView, regions regionSet, lan *LANAccess) (string, error) {
	a, err := loadDashboardAssets()
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	err = a.tmpl.ExecuteTemplate(&buf, "index", indexPageData{IndexView: view, Assets: a.data, LAN: lan, regions: regions})
	return buf.String(), err
}

// pageReader reads what the pages render from the index and disk. It holds
// no state of its own: every render re-reads, so a replacement daemon
// renders the same pages from the same files.
type pageReader struct {
	dir     string
	index   *Index
	source  SessionSource
	version string
	now     func() time.Time
}

func (r pageReader) clock() time.Time {
	if r.now != nil {
		return r.now()
	}
	return time.Now()
}

// readSnapshot reads a session's live snapshot; nil when it has none.
func (r pageReader) readSnapshot(replyID string) *wslive.Snapshot {
	snap, err := wslive.ReadSnapshot(wslive.SnapshotPath(SessionDir(r.dir, replyID)))
	if err != nil {
		return nil
	}
	return &snap
}

func (r pageReader) readPage(s Session) (PageContent, bool) {
	if ps, ok := r.source.(PageSource); ok {
		return ps.ReadPage(s.Registration)
	}
	return PageContent{HasBoard: s.HasBoard}, true
}

// served reads a session this daemon serves and its page content; false
// when it does not serve that reply-id.
func (r pageReader) served(replyID string) (Session, PageContent, bool) {
	if r.index == nil || !ValidReplyID(replyID) {
		return Session{}, PageContent{}, false
	}
	s, ok := r.index.Session(replyID)
	if !ok {
		return Session{}, PageContent{}, false
	}
	page, ok := r.readPage(s)
	if !ok {
		return Session{}, PageContent{}, false
	}
	return s, page, true
}

// lookup returns a served session and its snapshot, reading each snapshot
// once per lookup function.
func (r pageReader) lookup() func(id string) (Session, *wslive.Snapshot, bool) {
	snaps := map[string]*wslive.Snapshot{}
	return func(id string) (Session, *wslive.Snapshot, bool) {
		peer, ok := r.index.Session(id)
		if !ok {
			return Session{}, nil, false
		}
		snap, read := snaps[id]
		if !read {
			snap = r.readSnapshot(id)
			snaps[id] = snap
		}
		return peer, snap, true
	}
}

// session builds one session page's view; false when the daemon does not
// serve that reply-id.
func (r pageReader) session(replyID string) (SessionView, bool) {
	s, page, ok := r.served(replyID)
	if !ok {
		return SessionView{}, false
	}
	in := pageInputs{Session: s, Page: page, Snapshot: r.readSnapshot(replyID), Now: r.clock(), Version: r.version}
	if adapterPresent(in.Snapshot, in.Now) {
		in.Ring, _ = wslive.ReadRing(wslive.RingDir(SessionDir(r.dir, replyID)))
		inbox := sessionInbox(r.dir, replyID)
		in.Held, _ = wsinbox.Held(inbox)
		in.Drained, _ = wsinbox.Drained(inbox)
		in.AttachDir = wsinbox.AttachmentsDir(inbox)
		in.Lookup = r.lookup()
	}
	return buildSessionView(in), true
}

// history reads one page of a served session's older conversation (see
// wslive.ReadRingPage) and builds its entries as the live view builds them.
// It reads whenever the session page itself is served, adapter present or
// not; false when the daemon does not serve that reply-id.
func (r pageReader) history(replyID, before string, n int) (ConversationView, wslive.RingPage, bool, error) {
	s, page, ok := r.served(replyID)
	if !ok {
		return ConversationView{}, wslive.RingPage{}, false, nil
	}
	ring, err := wslive.ReadRingPage(wslive.RingDir(SessionDir(r.dir, replyID)), before, n)
	if err != nil {
		return ConversationView{}, wslive.RingPage{}, true, err
	}
	reg := s.Registration
	contacts := resolvedContacts(page.Contacts, reg.ReplyID)
	cv := conversation(wslive.Lead{}, ring.Records, page.MailBodies, contacts, r.lookup(), replyMarkdown(reg.Project, reg.ReplyID))
	userEntryImages(cv.Entries, wsinbox.AttachmentsDir(sessionInbox(r.dir, replyID)), reg.ReplyID)
	return cv, ring, true, nil
}

// indexView builds the / view.
func (r pageReader) indexView() IndexView {
	var inputs []indexInput
	if r.index != nil {
		for _, s := range r.index.Sessions() {
			page, ok := r.readPage(s)
			if !ok {
				continue
			}
			inputs = append(inputs, indexInput{Session: s, Snapshot: r.readSnapshot(s.ReplyID), Page: page})
		}
	}
	return buildIndexView(inputs, r.clock(), r.version)
}

func (d *Daemon) pages() pageReader {
	return pageReader{dir: d.dir, index: d.index, source: d.source, version: d.endpoint.Version}
}

// handleIndex serves /.
func (d *Daemon) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if !readMethod(w, r) {
		return
	}
	view := d.pages().indexView()
	regions, err := renderIndexRegions(view)
	if err == nil {
		var html string
		if html, err = renderIndexPage(view, regions, d.lanAccessFor(r)); err == nil {
			writeHTML(w, html)
			return
		}
	}
	fmt.Fprintf(d.log, "dashboard: render index: %v\n", err)
	http.Error(w, "render failed", http.StatusInternalServerError)
}

// handleSessionRoutes serves everything under /s/: a session page, its SSE
// stream, its image, file view, history, and attachment routes, its message,
// cancel, steer, and system-open routes, and the shared assets a page loads
// by relative path.
func (d *Daemon) handleSessionRoutes(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/s/")
	if replyID, sub, _ := strings.Cut(rest, "/"); sub == "message" || sub == "cancel" || sub == "steer" {
		d.handleInboxWrite(w, r, replyID, sub)
		return
	} else if sub == "open" {
		d.handleSystemOpen(w, r, replyID)
		return
	}
	if !readMethod(w, r) {
		return
	}
	if name, _ := wswhiteboard.MermaidAsset(); rest == name {
		serveMermaid(w)
		return
	}
	replyID, sub, slash := strings.Cut(rest, "/")
	switch {
	case !slash:
		d.handleSessionPage(w, r, replyID)
	case sub == "" && ValidReplyID(replyID):
		// The page loads its assets by relative path, so it is served at
		// one URL only.
		http.Redirect(w, r, "/s/"+replyID, http.StatusMovedPermanently)
	case sub == "events":
		d.handleSessionEvents(w, r, replyID)
	case sub == "image":
		d.handleImage(w, r, replyID)
	case sub == "file":
		d.handleFile(w, r, replyID)
	case sub == "history":
		d.handleHistory(w, r, replyID)
	case strings.HasPrefix(sub, "attachment/"):
		d.handleAttachment(w, r, replyID, strings.TrimPrefix(sub, "attachment/"))
	default:
		http.NotFound(w, r)
	}
}

func (d *Daemon) handleSessionPage(w http.ResponseWriter, r *http.Request, replyID string) {
	view, ok := d.pages().session(replyID)
	if !ok {
		http.Error(w, "unknown session", http.StatusNotFound)
		return
	}
	regions, err := renderSessionRegions(view, d.entries)
	if err == nil {
		var html string
		if html, err = renderSessionPage(view, regions, localViewer(r)); err == nil {
			writeHTML(w, html)
			return
		}
	}
	fmt.Fprintf(d.log, "dashboard: render session %s: %v\n", replyID, err)
	http.Error(w, "render failed", http.StatusInternalServerError)
}

// historyPageMax bounds one history page's n.
const historyPageMax = 4 * wslive.RingCap

// historyAnswer is the history route's answer: the rendered entries, oldest
// first, the cursor for the next older page (empty when the page held no
// entry), and whether no older entry remains, so the page stops asking.
type historyAnswer struct {
	HTML   string `json:"html"`
	Before string `json:"before"`
	End    bool   `json:"end"`
}

// handleHistory serves /s/<reply-id>/history?before=<entry name>&n=<count>:
// the n entries (default wslive.RingCap) older than before, exclusive, or
// the newest n when before is omitted. before is only ever a sort cursor,
// never a path, and must have an entry name's grammar.
func (d *Daemon) handleHistory(w http.ResponseWriter, r *http.Request, replyID string) {
	query := r.URL.Query()
	before := query.Get("before")
	if before != "" && !wslive.ValidRingName(before) {
		http.Error(w, "before is not a conversation entry name", http.StatusBadRequest)
		return
	}
	n := wslive.RingCap
	if raw := query.Get("n"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 1 {
			http.Error(w, "n must be a positive count", http.StatusBadRequest)
			return
		}
		n = min(v, historyPageMax)
	}
	cv, page, ok, err := d.pages().history(replyID, before, n)
	if !ok {
		http.Error(w, "unknown session", http.StatusNotFound)
		return
	}
	if err == nil {
		var a *dashboardAssets
		if a, err = loadDashboardAssets(); err == nil {
			err = renderEntries(a, d.entries, replyID, &cv)
		}
	}
	if err != nil {
		fmt.Fprintf(d.log, "dashboard: history %s: %v\n", replyID, err)
		http.Error(w, "render failed", http.StatusInternalServerError)
		return
	}
	raw, _ := json.Marshal(historyAnswer{HTML: renderedEntries(cv.Entries), Before: page.Before, End: page.End})
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(raw)
}

// serveMermaid serves the shared mermaid script. Its name carries its hash,
// so it never changes under a name and the browser may cache it for good.
func serveMermaid(w http.ResponseWriter) {
	_, data := wswhiteboard.MermaidAsset()
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	_, _ = w.Write(data)
}

func readMethod(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return false
	}
	return true
}

// frameAncestorsNone refuses every framing of a daemon page. A framed page's
// own script posts carry the daemon's Origin, so the same-origin and loopback
// guards on the write routes would pass a clickjacked board answer, message,
// steer, cancel, or system open; only refusing the frame stops it.
const frameAncestorsNone = "frame-ancestors 'none'"

// writeHTML writes a daemon HTML page. A caller that sets its own
// Content-Security-Policy includes frameAncestorsNone in it; without one the
// page gets frameAncestorsNone alone.
func writeHTML(w http.ResponseWriter, html string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if w.Header().Get("Content-Security-Policy") == "" {
		w.Header().Set("Content-Security-Policy", frameAncestorsNone)
	}
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	_, _ = w.Write([]byte(html))
}
