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
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kang-sw/devenv/internal/wsdevassets"
	"github.com/kang-sw/devenv/internal/wsi18n"
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

//go:embed assets/dashboard.tmpl assets/dashboard.css assets/dashboard.js assets/sprite.svg assets/tracker.css assets/tracker.js assets/mention.js assets/toast.js assets/rail.js assets/memo.js assets/memo.css assets/usage.js assets/usage.css assets/cards.js assets/preview.js assets/files.js assets/terminal.js assets/shell.js
var dashboardFS embed.FS

// Region names. The session page's regions in order; the index page has one.
//
// The conversation is two regions inside one scroll wrapper: entries (the
// finished messages of the live view, which the page merges by entry name
// and never re-renders) and convlive (the streaming reply, the approval or
// activity line, the held messages), which a streaming tick changes alone.
// The status band is two regions inside the composer: ctx (the context bar
// above the message box) and sline (the line below it). The composer also
// holds cmds, the slash commands the session's adapter runs, as hidden data
// the page's command picker reads; empty when the adapter reports none.
const (
	regionHead     = "head"
	regionTree     = "tree"
	regionEntries  = "entries"
	regionConvLive = "convlive"
	regionCtx      = "ctx"
	regionCmds     = "cmds"
	regionGit      = "git"
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
		return []string{regionHead, regionGit, regionTree, regionEntries, regionConvLive, regionCtx, regionCmds, regionSLine, regionBoard, regionPanels}
	}
	return []string{regionHead, regionGit, regionBoard, regionPanels}
}

type dashboardAssets struct {
	// tmpl is the default language's template set; tmpls holds one per
	// language, each parsed with the translation funcs bound to it, since a
	// parsed set's FuncMap is fixed (templateFor picks).
	tmpl  *template.Template
	tmpls map[string]*template.Template
	data  pageAssetsData
}

// templateFor is the template set that renders in lang; an unsupported or
// empty lang renders the default language.
func (a *dashboardAssets) templateFor(lang string) *template.Template {
	if t, ok := a.tmpls[lang]; ok {
		return t
	}
	return a.tmpl
}

// pageAssetsData is what a page template inlines.
type pageAssetsData struct {
	CSS              template.CSS
	Script           template.JS
	Sprite           template.HTML
	WhiteboardCSS    template.CSS
	WhiteboardSprite template.HTML
	FormScript       template.JS
	AnswersScript    template.JS
	MermaidInit      template.JS
	MermaidFile      string
	ViewerScript     template.JS
	// TrackerCSS and TrackerScript are the tracker renderer (tracker.js),
	// which dashboard.js mounts when the daemon serves the tracker routes.
	TrackerCSS    template.CSS
	TrackerScript template.JS
	// MentionScript is the @ file-mention popup module (mention.js), which
	// the session page's composer attaches.
	MentionScript template.JS
	// ToastScript is the shared toast stack (toast.js, window.WsToast),
	// inlined ahead of the tracker and page scripts that both use it.
	ToastScript template.JS
	// RailScript is the session rail's browser-side state (rail.js,
	// window.WsRail): per-session drafts, stars, and the rail's own
	// toggles, which the session page's script uses.
	RailScript  template.JS
	MemoScript  template.JS
	UsageScript template.JS
	// ShellScript is the iframe shell's pure helpers (shell.js,
	// window.WsShell): the frame LRU, the paths a shell frames, and the
	// shell-frame message shapes, which the shell and its frames both load.
	ShellScript template.JS
	// CardsScript is the board card framework (cards.js, window.WsCards),
	// PreviewScript its file preview kind (preview.js, window.WsPreview),
	// and FilesScript the conversation column's file explorer (files.js,
	// window.WsFiles); LucideSprite holds the vendored icons they draw
	// (lucide.go). A session page loads them before its own script, which
	// mounts them.
	CardsScript   template.JS
	PreviewScript template.JS
	FilesScript   template.JS
	// TerminalScript is the terminal card kind (terminal.js,
	// window.WsTerm), which only a page that offers terminals carries
	// (sessionPageData.Terminal).
	TerminalScript template.JS
	// XtermCSS is the vendored terminal emulator's stylesheet, which a
	// page that offers terminals inlines (sessionPageData.Terminal).
	XtermCSS     template.CSS
	LucideSprite template.HTML
	// ID is the assets' identity (assetID), which the page roots carry
	// (data-assets) and every stream sends first (sse.go), so an open page
	// can tell that the daemon now serves different assets than it runs.
	ID string `json:"-"`
}

var (
	dashboardAssetsOnce sync.Once
	loadedDashboard     *dashboardAssets
	dashboardAssetsErr  error

	symbolPattern = regexp.MustCompile(`<symbol id="i-([a-z0-9-]+)"`)
)

// loadDashboardAssets returns the dashboard assets. The embedded copy is
// built once; with dev assets on (SetDevAssets) every call rebuilds from the
// working tree, so an edit shows on the next request and a failed build is
// never kept.
func loadDashboardAssets() (*dashboardAssets, error) {
	if root := devAssets.Load(); root != nil {
		return buildDashboardAssets(func(name string) ([]byte, error) { return root.ReadFile(devAssetDir + "/" + name) })
	}
	dashboardAssetsOnce.Do(func() { loadedDashboard, dashboardAssetsErr = buildDashboardAssets(embeddedDashboardAsset) })
	return loadedDashboard, dashboardAssetsErr
}

func embeddedDashboardAsset(name string) ([]byte, error) {
	return dashboardFS.ReadFile("assets/" + name)
}

// devAssets, when set, is the source root loadDashboardAssets reads from
// instead of the embedded copy.
var devAssets atomic.Pointer[wsdevassets.Root]

// devAssetDir is where the dashboard assets live under a dev source root.
const devAssetDir = "internal/wsdashboard/assets"

// SetDevAssets turns dev assets on for this process, for the dashboard pages
// and the whiteboard page assets they inline, or off with nil. The daemon
// calls it at startup; the mermaid bundle stays embedded.
func SetDevAssets(root *wsdevassets.Root) {
	devAssets.Store(root)
	wswhiteboard.SetDevAssets(root)
}

// buildDashboardAssets reads every dashboard asset by file name through read
// and parses the template for each language.
func buildDashboardAssets(read func(name string) ([]byte, error)) (*dashboardAssets, error) {
	var err error
	get := func(name string) string {
		if err != nil {
			return ""
		}
		var raw []byte
		raw, err = read(name)
		return string(raw)
	}
	wb, werr := wswhiteboard.PageAssets()
	if werr != nil {
		return nil, werr
	}
	mermaidName, _ := wswhiteboard.MermaidAsset()
	lucide, lerr := lucideSprite()
	if lerr != nil {
		return nil, lerr
	}
	a := &dashboardAssets{data: pageAssetsData{
		CSS:              template.CSS(get("dashboard.css") + "\n" + get("memo.css") + "\n" + get("usage.css")),
		Script:           template.JS(get("dashboard.js")),
		Sprite:           template.HTML(strings.TrimSpace(get("sprite.svg"))),
		TrackerCSS:       template.CSS(get("tracker.css")),
		TrackerScript:    template.JS(get("tracker.js")),
		MentionScript:    template.JS(get("mention.js")),
		ToastScript:      template.JS(get("toast.js")),
		RailScript:       template.JS(get("rail.js")),
		MemoScript:       template.JS(get("memo.js")),
		UsageScript:      template.JS(get("usage.js")),
		ShellScript:      template.JS(get("shell.js")),
		CardsScript:      template.JS(get("cards.js")),
		PreviewScript:    template.JS(get("preview.js")),
		FilesScript:      template.JS(get("files.js")),
		TerminalScript:   template.JS(get("terminal.js")),
		XtermCSS:         xtermStyle(),
		LucideSprite:     lucide,
		WhiteboardCSS:    wb.CSS,
		WhiteboardSprite: wb.Sprite,
		FormScript:       wb.FormScript,
		AnswersScript:    wb.AnswersScript,
		MermaidInit:      wb.MermaidInit,
		MermaidFile:      mermaidName,
		ViewerScript:     wb.ViewerScript,
	}}
	source := get("dashboard.tmpl")
	if err != nil {
		return nil, err
	}
	icons := map[string]bool{}
	for _, sprite := range []string{string(a.data.Sprite), string(wb.Sprite)} {
		for _, m := range symbolPattern.FindAllStringSubmatch(sprite, -1) {
			icons[m[1]] = true
		}
	}
	a.data.ID = assetID(a.data, source)
	a.tmpls = map[string]*template.Template{}
	for _, l := range wsi18n.Languages() {
		// The file's template is "dashboard.tmpl" under the root
		// "dashboard", as ParseFS names it.
		t := template.New("dashboard").Funcs(pageFuncs(l.Code, icons))
		if _, err := t.New("dashboard.tmpl").Parse(source); err != nil {
			return nil, err
		}
		a.tmpls[l.Code] = t
	}
	a.tmpl = a.tmpls[wsi18n.Default]
	return a, nil
}

// assetID is the content hash of everything a dashboard page inlines: the
// asset data (every inlined script, stylesheet, and sprite), the template
// source, and every language's client string table. It is a content hash, not
// the build time, so a daemon replacement that leaves the assets unchanged
// keeps it; and it covers every language alike, so a page and its stream
// agree whatever language each was rendered in. With dev assets on it follows
// the files read for the build, so a page and the stream it opens disagree
// only while a file changed in between, and the reload converges.
func assetID(data pageAssetsData, source string) string {
	h := sha256.New()
	raw, _ := json.Marshal(data)
	h.Write(raw)
	h.Write([]byte(source))
	for _, l := range wsi18n.Languages() {
		h.Write([]byte(wsi18n.Script(l.Code)))
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// pageFuncs is the template FuncMap for one language: the page helpers and
// the translation funcs t, tn, and tcode, with the Go-built display strings
// (token usage, batch lines, phase and band words) bound to lang.
func pageFuncs(lang string, icons map[string]bool) template.FuncMap {
	return template.FuncMap{
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
		"marked":   func(text string) template.HTML { return markedText(lang, text) },
		// The tool summary's token and batch lines.
		"tokenText":  func(requests int, u *wslive.TokenUsage) string { return tokenText(lang, requests, u) },
		"tokenTitle": func(requests int, u *wslive.TokenUsage) string { return tokenTitle(lang, requests, u) },
		"requests":   summaryRequests,
		"batchNums":  func(b wslive.ToolBatch) []batchNum { return batchNums(lang, b) },
		"batchArgs":  batchArgs,
		"argsList":   batchArgsList,
		"phaseWord":  func(phase string) string { return phaseWord(lang, phase) },
		"flagText":   func(flag string) string { return flagText(lang, flag) },
		"limitName":  func(name string) string { return limitName(lang, name) },
		// Translation: t is a table lookup, tn the count form of a key, tcode
		// a lookup whose one placeholder renders as <code>.
		"t":     func(key string, args ...any) string { return wsi18n.T(lang, key, args...) },
		"tn":    func(key string, n int, args ...any) string { return wsi18n.Plural(lang, key, n, args...) },
		"tcode": func(key string, arg string) template.HTML { return codeText(lang, key, arg) },
		// lang is the language this set renders in; langs the available ones;
		// i18n the inline script giving the page scripts their table.
		"lang":  func() string { return effectiveLang(lang) },
		"langs": wsi18n.Languages,
		"i18n":  func() template.JS { return wsi18n.Script(lang) },
		// pwa is a page head's manifest link and theme-color metas (pwa.go).
		"pwa": pwaHead,
	}
}

// effectiveLang is lang when a table exists for it, else the default.
func effectiveLang(lang string) string {
	if wsi18n.Supported(lang) {
		return lang
	}
	return wsi18n.Default
}

// codeText is the table text for key with its {0} placeholder drawn as a
// <code> element holding arg; the rest of the text and arg are escaped.
func codeText(lang, key, arg string) template.HTML {
	text, _ := wsi18n.Lookup(lang, key)
	before, after, found := strings.Cut(text, "{0}")
	if !found {
		return template.HTML(template.HTMLEscapeString(text))
	}
	return template.HTML(template.HTMLEscapeString(before) + "<code>" + template.HTMLEscapeString(arg) + "</code>" + template.HTMLEscapeString(after))
}

// phaseWord is a lead or agent phase's display word.
func phaseWord(lang, phase string) string {
	switch phase {
	case "permission":
		return wsi18n.T(lang, "phase.approval")
	case "thinking":
		return wsi18n.T(lang, "phase.thinking")
	case "replying":
		return wsi18n.T(lang, "phase.replying")
	case "tool":
		return wsi18n.T(lang, "phase.tool")
	case "compacting":
		return wsi18n.T(lang, "phase.compacting")
	case "idle":
		return wsi18n.T(lang, "phase.idle")
	}
	return phase
}

// flagText is the context bar's warning word (ContextBar.Flag) in lang.
func flagText(lang, flag string) string {
	if flag == "near limit" {
		return wsi18n.T(lang, "band.flag.near")
	}
	return wsi18n.T(lang, "band.flag.high")
}

// limitName is a usage limit chip's name (LimitChip.Name) in lang.
func limitName(lang, name string) string {
	if name == "weekly" {
		return wsi18n.T(lang, "band.limit.weekly")
	}
	return wsi18n.T(lang, "band.limit.5h")
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

// tokenText is a token usage in one compact line: the model requests it
// covers when more than one, the whole prompt the model read (input plus
// cache reads and writes) with the cache-write part as a trailing +N when
// there was any, then the output.
func tokenText(lang string, requests int, u *wslive.TokenUsage) string {
	if u == nil {
		return ""
	}
	var parts []string
	if requests > 1 {
		parts = append(parts, wsi18n.T(lang, "tok.calls", requests))
	}
	prompt := wsi18n.T(lang, "tok.prompt", compactTokens(u.Input+u.CacheRead+u.CacheWrite))
	if u.CacheWrite > 0 {
		prompt += " +" + compactTokens(u.CacheWrite)
	}
	parts = append(parts, prompt, wsi18n.T(lang, "tok.out", compactTokens(u.Output)))
	return strings.Join(parts, " · ")
}

// tokenTitle is a token usage's exact counts, for a title attribute: every
// part tokenText folds together, named.
func tokenTitle(lang string, requests int, u *wslive.TokenUsage) string {
	if u == nil {
		return ""
	}
	title := wsi18n.T(lang, "tok.title", commafy(u.Input+u.CacheRead+u.CacheWrite),
		commafy(u.Input), commafy(u.CacheRead), commafy(u.CacheWrite), commafy(u.Output))
	if requests > 1 {
		title = wsi18n.T(lang, "tok.title.calls", requests) + " · " + title
	}
	return title
}

// summaryRequests is the model requests a tool summary covers: one per
// batch, the omitted ones included.
func summaryRequests(s *wslive.ToolSummary) int {
	return len(s.Batches) + s.BatchesOmitted
}

// batchNum is one number on a tool summary batch line: its text, a title
// naming it with the exact value, the separator before it, and an optional
// class.
type batchNum struct {
	Sep, Text, Title, Class string
}

// batchNums is a tool summary batch line's trailing numbers, unlabelled:
// its duration, failed calls, and the request's prompt (input plus cache
// reads and writes, the cache-write part as a trailing +N) and output, each
// with its own title. A batch without per-call tools (an older adapter's)
// leads with its call count, which its chips cannot show; so does one whose
// calls outnumber those it named.
func batchNums(lang string, b wslive.ToolBatch) []batchNum {
	var nums []batchNum
	add := func(sep, text, title, class string) {
		if len(nums) == 0 {
			sep = ""
		}
		nums = append(nums, batchNum{Sep: sep, Text: text, Title: title, Class: class})
	}
	named := 0
	for _, t := range b.Tools {
		named += t.Calls
	}
	if named < b.Calls {
		add(" · ", "×"+strconv.Itoa(b.Calls), wsi18n.Plural(lang, "batch.calls", b.Calls), "")
	}
	if b.DurationMs > 0 {
		add(" · ", batchSpanText(b.DurationMs), wsi18n.T(lang, "batch.duration", commafy(b.DurationMs)), "")
	}
	if b.Failed > 0 {
		add(" · ", "✗"+strconv.Itoa(b.Failed), wsi18n.T(lang, "count.failed", b.Failed), "bf")
	}
	if u := b.Usage; u != nil {
		prompt := u.Input + u.CacheRead + u.CacheWrite
		add(" · ", compactTokens(prompt), wsi18n.T(lang, "tok.title.prompt", commafy(prompt), commafy(u.Input), commafy(u.CacheRead), commafy(u.CacheWrite)), "")
		if u.CacheWrite > 0 {
			add(" ", "+"+compactTokens(u.CacheWrite), wsi18n.T(lang, "tok.title.cachewrite", commafy(u.CacheWrite)), "")
		}
		add(" · ", compactTokens(u.Output), wsi18n.T(lang, "tok.out", commafy(u.Output)), "")
	}
	return nums
}

// batchArgs is a one-call batch's argument preview, shown after its tool
// name; empty for a batch of several calls or one that carried none.
func batchArgs(b wslive.ToolBatch) string {
	if b.Calls != 1 || len(b.CallArgs) != 1 {
		return ""
	}
	return b.CallArgs[0].ArgsPreview
}

// batchArgsList is a batch of several calls' "name args" lines, one per
// carried call, for the row's title; empty for a one-call batch (its args
// show inline) or one that carried no argument preview.
func batchArgsList(b wslive.ToolBatch) string {
	if b.Calls < 2 || len(b.CallArgs) == 0 {
		return ""
	}
	lines := make([]string, len(b.CallArgs))
	for i, c := range b.CallArgs {
		lines[i] = strings.TrimSpace(toolName(c.Name) + " " + c.ArgsPreview)
	}
	return strings.Join(lines, "\n")
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
	// Terminal offers terminals: it is the emulator script's name beside
	// the page (XtermAsset) when the daemon runs terminals and the page
	// was asked for by a browser on this machine, and empty otherwise.
	Terminal string
	// Frame renders the session page as the iframe shell frames it
	// (?frame=1, shellPageData): the shell's rail and header stand over
	// it, so the page keeps both only as invisible placeholders that lay
	// it out as before. Without it the page stands alone (the tests'
	// renders).
	Frame   bool
	regions regionSet
}

func (p sessionPageData) regionByName(name string) renderedRegion { return p.regions[name] }

// shellPageData is the iframe shell (the "shell" template): the session
// rail and the top header, which stay across navigations, over the frames
// the dashboard's pages load in (shell.js, dashboard.js). RID is the
// session it opens on, empty on the index; Src is the first frame's path,
// Title the tab's title until that frame names one. Its regions are the
// rail and, on a session, the opening session's head; later heads come
// from the frames. OwnHead marks a first page that keeps its own header
// (the index), beside which the shell shows only its rail button.
type shellPageData struct {
	Lang, LangPref, Theme string
	Assets                pageAssetsData
	SysOpen, OwnHead      bool
	RID, Src, Title       string
	regions               regionSet
}

func (p shellPageData) regionByName(name string) renderedRegion { return p.regions[name] }

type indexPageData struct {
	IndexView
	Assets pageAssetsData
	// LAN is the LAN access block, set only for a local viewer in LAN mode.
	// It is outside every region, so no SSE update ever carries the token.
	LAN *LANAccess
	// Local marks a page asked for by a browser on this machine, like
	// sessionPageData.SysOpen.
	Local bool
	// Frame marks the index as the iframe shell frames it (?frame=1): laid
	// out beside a placeholder for the shell's rail, under its own header.
	Frame   bool
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
	view.Conversation.Lang = view.Lang
	if err := renderEntries(a, cache, view.ReplyID, &view.Conversation); err != nil {
		return nil, err
	}
	tmpl := a.templateFor(view.Lang)
	regions := regionSet{regionMode: newRegion(view.Mode())}
	for _, name := range sessionRegions(view.Mode()) {
		var buf bytes.Buffer
		if err := tmpl.ExecuteTemplate(&buf, "r-"+name, view); err != nil {
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
		e.Lang = cv.Lang
		html, err := cache.render(replyID, e, func() (template.HTML, error) {
			if e.Kind == wslive.RingReply && cv.markdown != nil {
				e.HTML = cv.markdown(e.Text)
			}
			var buf bytes.Buffer
			if err := a.templateFor(cv.Lang).ExecuteTemplate(&buf, "entry", e); err != nil {
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
// sysOpen offers the system-open link action (sessionPageData.SysOpen), and
// terminal, when not empty, terminals (sessionPageData.Terminal).
func renderSessionPage(view SessionView, regions regionSet, sysOpen bool, terminal string) (string, error) {
	return renderSessionPageData(sessionPageData{SessionView: view, SysOpen: sysOpen, Terminal: terminal, regions: regions})
}

// renderSessionPageData renders the session template from p, filling in the
// assets.
func renderSessionPageData(p sessionPageData) (string, error) {
	a, err := loadDashboardAssets()
	if err != nil {
		return "", err
	}
	p.Assets = a.data
	var buf bytes.Buffer
	err = a.templateFor(p.Lang).ExecuteTemplate(&buf, "session", p)
	return buf.String(), err
}

// renderShellPage renders the shell page p, filling in the assets. view,
// when not nil, is the session the shell opens on, whose head the shell
// shows until its frame hands over its own.
func renderShellPage(p shellPageData, view *SessionView) (string, error) {
	a, err := loadDashboardAssets()
	if err != nil {
		return "", err
	}
	p.Assets = a.data
	t := a.templateFor(p.Lang)
	var buf bytes.Buffer
	head := renderedRegion{}
	if view != nil {
		if err := t.ExecuteTemplate(&buf, "r-"+regionHead, *view); err != nil {
			return "", fmt.Errorf("dashboard region %s: %w", regionHead, err)
		}
		head = newRegion(buf.String())
		buf.Reset()
	}
	regions := regionSet{regionHead: head}
	for name, r := range p.regions {
		regions[name] = r
	}
	p.regions = regions
	err = t.ExecuteTemplate(&buf, "shell", p)
	return buf.String(), err
}

func renderIndexRegions(view IndexView) (regionSet, error) {
	a, err := loadDashboardAssets()
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := a.templateFor(view.Lang).ExecuteTemplate(&buf, "r-"+regionSessions, view); err != nil {
		return nil, fmt.Errorf("dashboard region %s: %w", regionSessions, err)
	}
	return regionSet{regionSessions: newRegion(buf.String())}, nil
}

// renderIndexPage renders the index; frame renders it as the iframe shell
// frames it.
func renderIndexPage(view IndexView, regions regionSet, lan *LANAccess, local, frame bool) (string, error) {
	a, err := loadDashboardAssets()
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	err = a.templateFor(view.Lang).ExecuteTemplate(&buf, "index", indexPageData{IndexView: view, Assets: a.data, LAN: lan, Local: local, Frame: frame, regions: regions})
	return buf.String(), err
}

// pageReader reads what the pages render from the index and disk. It holds
// no state of its own: every read re-reads, so a replacement daemon renders
// the same pages from the same files. (The streams share their renders in
// memory, and the daemon's caches serve unchanged files, which only skips
// reads; stream_cache.go, entry_files.go, file_cache.go.)
type pageReader struct {
	// lang is the viewer's language, passed to a LangPageSource.
	lang    string
	dir     string
	index   *Index
	source  SessionSource
	version string
	now     func() time.Time
	// entries holds the ring and drained entries already read (nil reads
	// every entry each time).
	entries *entryFiles
	// snapshots holds the snapshots already parsed (nil reads each time).
	snapshots *SnapshotCache
}

func (r pageReader) clock() time.Time {
	if r.now != nil {
		return r.now()
	}
	return time.Now()
}

// readSnapshot reads a session's live snapshot; nil when it has none. The
// snapshot may be shared with other readers (SnapshotCache): the caller
// only reads it.
func (r pageReader) readSnapshot(replyID string) *wslive.Snapshot {
	return r.snapshots.Snapshot(r.dir, replyID)
}

func (r pageReader) readPage(s Session) (PageContent, bool) {
	if ls, ok := r.source.(LangPageSource); ok {
		return ls.ReadPageLang(s.Registration, r.lang)
	}
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

// lookup resolves served sessions, reading a session's snapshot only when
// a caller asks for it, and once per lookup.
func (r pageReader) lookup() sessionLookup {
	snaps := map[string]*wslive.Snapshot{}
	return sessionLookup{
		session: func(id string) (Session, bool) {
			if r.index == nil {
				return Session{}, false
			}
			return r.index.Session(id)
		},
		snapshot: func(id string) *wslive.Snapshot {
			snap, read := snaps[id]
			if !read {
				snap = r.readSnapshot(id)
				snaps[id] = snap
			}
			return snap
		},
	}
}

// session builds one session page's view; false when the daemon does not
// serve that reply-id.
func (r pageReader) session(replyID string) (SessionView, bool) {
	view, _, ok := r.readSession(replyID)
	return view, ok
}

// readSession is session with the page's refresh inputs: every session the
// view looked up (replyID first among them, sorted), the files its page
// source named and the attachments directory its conversation reads, and
// whether it shows contacts (only a live page does).
func (r pageReader) readSession(replyID string) (SessionView, refreshInputs, bool) {
	view, inputs, _, ok := r.readSessionWith(replyID, nil, true)
	return view, inputs, ok
}

// sessionReads are the store reads one session page read made, which a
// live render reuses (liveRender): the page source's content (the board
// panels, the contacts) and, once an adapter showed, the conversation's
// ring and the board-answer inbox's held and drained entries.
type sessionReads struct {
	page          PageContent
	conversation  bool
	ring          []wslive.RingRecord
	held, drained []wsinbox.Record
}

// readSessionWith is readSession for a render that may reuse prev, the
// reads of the page's previous render (nil reads everything): the index
// entry and the snapshots are read again, every other store is taken from
// prev, the ring included unless ringChanged. A conversation prev did not
// read (no adapter showed then) is read now. It returns the reads it used.
func (r pageReader) readSessionWith(replyID string, prev *sessionReads, ringChanged bool) (SessionView, refreshInputs, *sessionReads, bool) {
	var s Session
	var page PageContent
	ok := false
	if prev != nil && r.index != nil && ValidReplyID(replyID) {
		s, ok = r.index.Session(replyID)
		page = prev.page
	} else {
		s, page, ok = r.served(replyID)
	}
	if !ok {
		return SessionView{}, refreshInputs{}, nil, false
	}
	reads := &sessionReads{page: page}
	in := pageInputs{Session: s, Page: page, Snapshot: r.readSnapshot(replyID), Now: r.clock(), Version: r.version}
	if !s.Registration.Hosted && !s.Live {
		record, _ := readAgentRecordEnd(SessionDir(r.dir, replyID))
		in.HandedOff = record.Lead && record.HandedOffAt != ""
	}
	looked := map[string]bool{replyID: true}
	inputs := refreshInputs{watch: slices.Clone(page.Watch), pids: slices.Clone(page.WatchPIDs)}
	if s.Registration.Hosted && s.Registration.HostPID > 0 {
		inputs.pids = append(inputs.pids, s.Registration.HostPID)
	}
	if adapterPresent(in.Snapshot, in.Now) {
		inbox := sessionInbox(r.dir, replyID)
		reads.conversation = true
		if prev != nil && prev.conversation {
			reads.held, reads.drained, reads.ring = prev.held, prev.drained, prev.ring
			if ringChanged {
				reads.ring, _ = r.entries.readRing(replyID, wslive.RingDir(SessionDir(r.dir, replyID)))
			}
		} else {
			reads.ring, _ = r.entries.readRing(replyID, wslive.RingDir(SessionDir(r.dir, replyID)))
			reads.held, _ = r.entries.readHeld(inbox)
			reads.drained, _ = r.entries.readDrained(replyID, inbox)
		}
		in.Ring, in.Held, in.Drained = reads.ring, reads.held, reads.drained
		in.AttachDir = wsinbox.AttachmentsDir(inbox)
		lookup := r.lookup()
		in.Lookup = sessionLookup{
			session: func(id string) (Session, bool) {
				looked[id] = true
				return lookup.get(id)
			},
			snapshot: lookup.snap,
		}
		inputs.watch = append(inputs.watch, in.AttachDir)
		inputs.contacts = len(page.Contacts) > 0
	}
	view := buildSessionView(in)
	for id := range looked {
		if id != replyID {
			inputs.ids = append(inputs.ids, id)
		}
	}
	slices.Sort(inputs.ids)
	inputs.ids = append([]string{replyID}, inputs.ids...)
	slices.Sort(inputs.watch)
	inputs.watch = slices.Compact(inputs.watch)
	slices.Sort(inputs.pids)
	inputs.pids = slices.Compact(inputs.pids)
	return view, inputs, reads, true
}

// sessionStamp is the refresh stamp of replyID's session page over the
// inputs of its last read (stream_cache.go): for each session it looked up,
// whether the index holds it, its revision, its liveness, and whether its
// snapshot names a present adapter, all at call time; the state of the
// watched paths (StampWatched); the liveness of each watched process; the
// session's mail print (MailPrintSource); and the contacts as re-read now.
// ok is false when the page shows contacts the source cannot re-read.
func (r pageReader) sessionStamp(replyID string, in refreshInputs) (string, bool) {
	if r.index == nil {
		return "", false
	}
	now := r.clock()
	h := newStampHash()
	for _, id := range in.ids {
		s, ok := r.index.Session(id)
		present := ok && adapterPresent(r.readSnapshot(id), now)
		fmt.Fprintf(h, "%s\x00%t\x00%d\x00%t\x00%t\x01", id, ok, s.Rev, ok && s.Registration.Live(now), present)
	}
	StampWatched(h, in.watch)
	for _, pid := range in.pids {
		fmt.Fprintf(h, "pid\x00%d\x00%t\x01", pid, processAlive(pid))
	}
	if src, ok := r.source.(MailPrintSource); ok {
		if s, ok := r.index.Session(replyID); ok {
			if print, ok := src.MailPrint(s.Registration); ok {
				fmt.Fprintf(h, "mail\x00%s\x01", print)
			}
		}
	}
	if in.contacts {
		src, ok := r.source.(ContactSource)
		if !ok {
			return "", false
		}
		s, ok := r.index.Session(replyID)
		if !ok {
			return "", false
		}
		contacts, ok := src.ReadContacts(s.Registration)
		if !ok {
			return "", false
		}
		for _, c := range contacts {
			fmt.Fprintf(h, "%s\x00%s\x00%t\x00%s\x00%d\x01", c.Address, c.ReplyID, c.PresenceLive, c.Cwd, c.LastExchange.UnixNano())
		}
	}
	return sumHex(h), true
}

// indexStamp is the refresh stamp of the sessions index: for each listed
// session, its revision, its liveness, whether its snapshot names a present
// adapter, and whether it is inside the ended-session window, all at call
// time. Everything else the index shows comes from the session records,
// which the revision covers.
func (r pageReader) indexStamp() (string, bool) {
	if r.index == nil {
		return "", true
	}
	now := r.clock()
	h := newStampHash()
	for _, s := range r.index.Sessions() {
		if s.Unlisted {
			continue
		}
		reg := s.Registration
		fmt.Fprintf(h, "%s\x00%d\x00%t\x00%t\x00%t\x01", reg.ReplyID, s.Rev, reg.Live(now),
			adapterPresent(r.readSnapshot(reg.ReplyID), now), now.Sub(reg.LastSeen()) <= endedSessionWindow)
	}
	return sumHex(h), true
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
	splitUserEntries(cv.Entries, wsinbox.AttachmentsDir(sessionInbox(r.dir, replyID)), reg.ReplyID)
	return cv, ring, true, nil
}

// indexView builds the / view. It reads no session page: what it shows of
// a board (whether there is one, its open policy and check items) comes with the
// session from the index (SessionData.CountPolicy).
func (r pageReader) indexView() IndexView {
	var inputs []indexInput
	if r.index != nil {
		for _, s := range r.index.Sessions() {
			if s.Unlisted {
				continue
			}
			inputs = append(inputs, indexInput{Session: s, Snapshot: r.readSnapshot(s.ReplyID)})
		}
	}
	return buildIndexView(inputs, r.clock(), r.version)
}

func (d *Daemon) pages() pageReader {
	return pageReader{dir: d.dir, index: d.index, source: d.source, version: d.endpoint.Version, entries: d.entryFiles, snapshots: d.snapshots}
}

// pagesIn is pages reading in the viewer's language.
func (d *Daemon) pagesIn(lang string) pageReader {
	r := d.pages()
	r.lang = lang
	return r
}

// handleIndex serves /: the iframe shell, which loads the index in a frame
// of its own, or with ?frame=1 the index as the shell frames it.
func (d *Daemon) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if !readMethod(w, r) {
		return
	}
	if !framedRequest(r) {
		d.serveShell(w, r, nil, "/?frame=1")
		return
	}
	lang, pref := requestLang(r)
	view := d.pagesIn(lang).indexView()
	view.Lang, view.LangPref, view.Theme = lang, pref, requestTheme(r)
	regions, err := renderIndexRegions(view)
	if err == nil {
		var html string
		if html, err = renderIndexPage(view, regions, d.lanAccessFor(r), localViewer(r), true); err == nil {
			admitShellFrame(w, frameAncestorsSelf)
			writeHTML(w, html)
			return
		}
	}
	fmt.Fprintf(d.log, "dashboard: render index: %v\n", err)
	http.Error(w, "render failed", http.StatusInternalServerError)
}

// handleSessionRoutes serves everything under /s/: a session page, its SSE
// stream, its image, file view, files, history, closed-topic, worktree pool,
// git status, log, and commit, attachment, and file browser routes, its
// message, cancel, steer, interrupt, system-open, file browser host,
// outside-link mint, git
// refresh, git push, pull, and fetch, worktree cleanup, and display label
// routes, and the shared assets a page loads by relative path.
func (d *Daemon) handleSessionRoutes(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/s/")
	if replyID, sub, _ := strings.Cut(rest, "/"); sub == "message" || sub == "cancel" || sub == "steer" || sub == "interrupt" {
		d.handleInboxWrite(w, r, replyID, sub)
		return
	} else if sub == "open" {
		d.handleSystemOpen(w, r, replyID)
		return
	} else if sub == "terminals" || strings.HasPrefix(sub, "terminals/") {
		d.handleTerminals(w, r, replyID, strings.TrimPrefix(sub, "terminals"))
		return
	} else if sub == "fs/act" {
		d.handleFSAct(w, r, replyID)
		return
	} else if sub == "fs/outside" {
		d.handleFSOutside(w, r, replyID)
		return
	} else if sub == "git" {
		d.handleGitRefresh(w, r, replyID)
		return
	} else if action, ok := strings.CutPrefix(sub, "git/"); ok && gitActions[action] {
		d.handleGitAction(w, r, replyID, action)
		return
	} else if sub == "worktrees/cleanup" {
		d.handleWorktreeCleanup(w, r, replyID)
		return
	} else if sub == "label" {
		d.handleSessionLabel(w, r, replyID)
		return
	}
	if !readMethod(w, r) {
		return
	}
	if name, _ := wswhiteboard.MermaidAsset(); rest == name {
		serveMermaid(w)
		return
	}
	if name, _ := XtermAsset(); rest == name {
		serveXterm(w)
		return
	}
	if name, _ := TermSymbolsAsset(); rest == name {
		serveTermSymbols(w)
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
	case sub == "files":
		d.handleFiles(w, r, replyID)
	case sub == "history":
		d.handleHistory(w, r, replyID)
	case sub == "closed":
		d.handleClosedTopics(w, r, replyID)
	case sub == "worktrees":
		d.handleWorktrees(w, r, replyID)
	case sub == "git/status":
		d.handleGitStatus(w, r, replyID)
	case sub == "git/log":
		d.handleGitLog(w, r, replyID)
	case strings.HasPrefix(sub, "git/commit/") && strings.HasSuffix(sub, "/diff"):
		d.handleGitCommitDiff(w, r, replyID, strings.TrimSuffix(strings.TrimPrefix(sub, "git/commit/"), "/diff"))
	case strings.HasPrefix(sub, "git/commit/"):
		d.handleGitCommit(w, r, replyID, strings.TrimPrefix(sub, "git/commit/"))
	case sub == "git/worktree/diff":
		d.handleGitWorktreeDiff(w, r, replyID)
	case strings.HasPrefix(sub, "attachment/"):
		d.handleAttachment(w, r, replyID, strings.TrimPrefix(sub, "attachment/"))
	case strings.HasPrefix(sub, "fs/"):
		d.handleFSRoutes(w, r, replyID, strings.TrimPrefix(sub, "fs/"))
	default:
		http.NotFound(w, r)
	}
}

// handleSessionPage serves /s/<reply-id>: the iframe shell, which keeps the
// rail and the top header across session switches and loads each session
// page in a frame of its own, or with ?frame=1 that session page as the
// shell frames it (framedRequest).
func (d *Daemon) handleSessionPage(w http.ResponseWriter, r *http.Request, replyID string) {
	frame := framedRequest(r)
	lang, pref := requestLang(r)
	view, ok := d.pagesIn(lang).session(replyID)
	if !ok {
		if ValidReplyID(replyID) {
			d.serveFindingSession(w, r, replyID, lang)
			return
		}
		http.Error(w, "unknown session", http.StatusNotFound)
		return
	}
	view.Lang, view.LangPref, view.Theme = lang, pref, requestTheme(r)
	if !frame {
		d.serveShell(w, r, &view, "/s/"+replyID+"?frame=1")
		return
	}
	view.Git = d.git.badge(view.Project)
	view.Worktrees = d.worktrees != nil
	view.Holders = d.holders != nil
	regions, err := renderSessionRegions(view, d.entries)
	if err == nil {
		var html string
		page := sessionPageData{SessionView: view, SysOpen: localViewer(r), Frame: true, regions: regions}
		if d.offersTerminals(r) {
			page.Terminal, _ = XtermAsset()
		}
		if html, err = renderSessionPageData(page); err == nil {
			admitShellFrame(w, sessionPageCSP(true))
			writeHTML(w, html)
			return
		}
	}
	fmt.Fprintf(d.log, "dashboard: render session %s: %v\n", replyID, err)
	http.Error(w, "render failed", http.StatusInternalServerError)
}

// serveShell serves the iframe shell whose first frame loads src: the
// session rail, rendered as its stream renders it (railPageSource), and the
// top header, holding view's head when the shell opens on a session; with
// no view (the index) the first page keeps its own header.
func (d *Daemon) serveShell(w http.ResponseWriter, r *http.Request, view *SessionView, src string) {
	lang, pref := requestLang(r)
	page := shellPageData{Lang: lang, LangPref: pref, Theme: requestTheme(r), SysOpen: localViewer(r), OwnHead: view == nil, Src: src, Title: "ws dashboard"}
	if view != nil {
		page.RID = view.ReplyID
		page.Title = view.Label
		if page.Title == "" {
			page.Title = view.ReplyID
		}
		page.Title += " - ws dashboard"
	}
	rail, err := renderRailRegion(d.railView(d.pagesIn(lang)), lang)
	if err == nil {
		page.regions = regionSet{regionRail: rail}
		var html string
		if html, err = renderShellPage(page, view); err == nil {
			w.Header().Set("Content-Security-Policy", sessionPageCSP(false))
			writeHTML(w, html)
			return
		}
	}
	fmt.Fprintf(d.log, "dashboard: render shell: %v\n", err)
	http.Error(w, "render failed", http.StatusInternalServerError)
}

// framedRequest reports a request for the page a shell frames (?frame=1).
func framedRequest(r *http.Request) bool { return r.URL.Query().Get("frame") == "1" }

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
	lang, _ := requestLang(r)
	cv, page, ok, err := d.pagesIn(lang).history(replyID, before, n)
	if !ok {
		http.Error(w, "unknown session", http.StatusNotFound)
		return
	}
	if err == nil {
		var a *dashboardAssets
		if a, err = loadDashboardAssets(); err == nil {
			cv.Lang = lang
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

// closedPageMax bounds one closed-topic page's n: a page that re-fetches
// every closed topic it had shown after a board swap asks for them at once.
const closedPageMax = 10 * wswhiteboard.ClosedCap

// handleClosedTopics serves /s/<reply-id>/closed?from=<index>&n=<count>: n
// (default wswhiteboard.ClosedCap) of the closed topics the board leaves
// past its cap, from index from in close order, the most recently closed
// first, rendered as the board renders them (ClosedTopicsSource), with the
// next show-more control when more remain. It is read-only and serves what
// the session page shows, so it takes the page's own access rule.
func (d *Daemon) handleClosedTopics(w http.ResponseWriter, r *http.Request, replyID string) {
	query := r.URL.Query()
	from, err := strconv.Atoi(query.Get("from"))
	if err != nil || from < 1 {
		http.Error(w, "from must be a positive index", http.StatusBadRequest)
		return
	}
	n := wswhiteboard.ClosedCap
	if raw := query.Get("n"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 1 {
			http.Error(w, "n must be a positive count", http.StatusBadRequest)
			return
		}
		n = min(v, closedPageMax)
	}
	// The daemon's source is agentSessions, which forwards to the configured
	// one; a source without closed topics answers not found.
	src, hasClosed := d.source.(ClosedTopicsSource)
	var s Session
	ok := hasClosed && d.index != nil && ValidReplyID(replyID)
	if ok {
		s, ok = d.index.Session(replyID)
	}
	var html template.HTML
	if ok {
		lang, _ := requestLang(r)
		html, ok = src.ReadClosedTopics(s.Registration, lang, from, n)
	}
	if !ok {
		http.Error(w, "unknown session", http.StatusNotFound)
		return
	}
	raw, _ := json.Marshal(map[string]string{"html": string(wswhiteboard.ThemeFrames(html, requestTheme(r)))})
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
// steer, cancel, interrupt, or system open; only refusing the frame stops it.
// The one exception is a page the iframe shell frames (?frame=1,
// admitShellFrame), which only a page of the daemon's own origin may frame.
const frameAncestorsNone = "frame-ancestors 'none'"

// sessionPageCSP is the session page's policy: frame-src 'self', so no
// frame on the page (a preview's sandboxed document, a topic body, a
// shell's session frame) can navigate itself to another origin, where a
// preview's document would carry its capability (fs_capability.go), and
// the page's own framing: the shell refuses every framing
// (frameAncestorsNone), and the page a shell frames (frame) admits its
// own origin only. A topic body's srcdoc frame loads under it.
func sessionPageCSP(frame bool) string {
	if frame {
		return "frame-src 'self'; " + frameAncestorsSelf
	}
	return "frame-src 'self'; " + frameAncestorsNone
}

// frameAncestorsSelf admits framing by a page of the daemon's own origin
// only: the iframe shell framing a page (?frame=1, admitShellFrame).
const frameAncestorsSelf = "frame-ancestors 'self'"

// admitShellFrame sets the framing headers of a page the iframe shell
// frames: csp, which admits the daemon's own origin as the page's framer
// (frameAncestorsSelf), and X-Frame-Options SAMEORIGIN in place of
// writeHTML's DENY.
func admitShellFrame(w http.ResponseWriter, csp string) {
	w.Header().Set("Content-Security-Policy", csp)
	w.Header().Set("X-Frame-Options", "SAMEORIGIN")
}

// writeHTML writes a daemon HTML page. A caller that sets its own
// Content-Security-Policy includes frameAncestorsNone in it (or, for a page
// the shell frames, frameAncestorsSelf: admitShellFrame); without one the
// page gets frameAncestorsNone alone, and X-Frame-Options DENY unless the
// caller set SAMEORIGIN.
func writeHTML(w http.ResponseWriter, html string) { writeHTMLStatus(w, http.StatusOK, html) }

// writeHTMLStatus is writeHTML with the status the page answers.
func writeHTMLStatus(w http.ResponseWriter, status int, html string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if w.Header().Get("Content-Security-Policy") == "" {
		w.Header().Set("Content-Security-Policy", frameAncestorsNone)
	}
	if w.Header().Get("X-Frame-Options") == "" {
		w.Header().Set("X-Frame-Options", "DENY")
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(html))
}
