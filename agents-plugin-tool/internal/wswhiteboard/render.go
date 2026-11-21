package wswhiteboard

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"html/template"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"unicode"
	"unicode/utf8"

	"github.com/kang-sw/devenv/internal/wsdevassets"
	"github.com/kang-sw/devenv/internal/wsi18n"
)

// The page assets: one template set, the shared stylesheet, the answer-form
// and dependency-popover scripts, the mermaid theme script, the script a
// served page's topic-body frames run, the image viewer script, and the icon
// sprite.
//
//go:embed assets/page.tmpl assets/page.css assets/page-*.css assets/stream.js assets/stream-*.js assets/chg.js assets/panel-diff.js assets/answers.js assets/form.js assets/code.js assets/pill.js assets/rail.js assets/ago.js assets/deps.js assets/card-find.js assets/mermaid-init.js assets/doc-frame.js assets/viewer.js assets/svg-view.js assets/select-all.js assets/sprite.svg
var assetFS embed.FS

// The dashboard and topic-body frames use this same byte-for-byte stylesheet
// assembly, preserving its cascade and frame hashes.
var pageStyleFiles = []string{
	"page.css",
	"page-items.css",
	"page-stream.css",
	"page-panel-diff.css",
	"page-panels.css",
	"page-sticky.css",
}

// The page and dashboard use this same ordered stream-script assembly.
var pageStreamFiles = []string{
	"stream.js",
	"stream-navigation.js",
	"stream-view.js",
	"stream-panels.js",
	"stream-events.js",
}

func readPageSource(read func(string) ([]byte, error), name string) ([]byte, error) {
	files := pageStyleFiles
	if name == "stream.js" {
		files = pageStreamFiles
	} else if name != "page.css" {
		return read(name)
	}
	var source bytes.Buffer
	for _, file := range files {
		raw, err := read(file)
		if err != nil {
			return nil, err
		}
		source.Write(raw)
	}
	return source.Bytes(), nil
}

type pageAssets struct {
	// tmpl is the English template set; tmpls holds one per language, each
	// parsed with the translation funcs bound to it (templateFor picks).
	tmpl  *template.Template
	tmpls map[string]*template.Template
	css   template.CSS
	// answers builds the answer block (window.wbAnswers), which form.js
	// and the dashboard composer read.
	answers string
	form    string
	// selectAll scopes page selections; sandboxed body frames never load it.
	selectAll string
	// code adds line numbers and a copy button to code blocks, on every
	// page that runs form (it follows form, which defines wbBindBoard) and
	// inside a topic-body frame.
	code   string
	stream string
	// pill is the open-question pill, shared by the full page and the dashboard.
	pill string
	// rail sticks the current topic's bar and shows the jump capsules, on the
	// same two.
	rail string
	// ago keeps the topic bars' updated-ago times current, on the same two.
	ago string
	// deps drives dependency popovers in the dashboard.
	deps string
	// cardFind provides in-card Ctrl/Cmd+F search on board cards.
	cardFind string
	// mermaidInit themes and runs mermaid after the shared script loads.
	mermaidInit template.JS
	// chg draws a topic body's change marks (wbChangeBars, wbWatchChanges).
	chg string
	// panelDiff binds the latest panel's comparison controls, in host or frame.
	panelDiff string
	// docFrame runs inside a served page's topic-body frame (framedDoc).
	docFrame string
	// frameCSSFile and frameJSFile are what every topic-body frame loads
	// beside the page instead of inlining (framedDoc, FrameAsset): the page
	// stylesheet plus frameCSS, and docFrame, each under a name that carries
	// a hash of its bytes.
	frameCSSFile, frameJSFile frameFile
	frameI18nFiles            map[string]frameFile
	// viewer is the image viewer, a self-contained script.
	viewer string
	sprite template.HTML
	icons  map[string]bool
}

var (
	assetsOnce   sync.Once
	loadedAssets *pageAssets
	assetsErr    error

	spriteSymbolPattern = regexp.MustCompile(`<symbol id="i-([a-z0-9-]+)"`)
	// itemRefPattern finds the {{nn}} item references inside lead-authored
	// text. Only digits between the braces make a reference; any other
	// {{...}} text renders verbatim.
	itemRefPattern     = regexp.MustCompile(`\{\{([0-9]+)\}\}`)
	authoredRefPattern = regexp.MustCompile(`\{\{(p[1-9][0-9]*|[0-9]+)\}\}`)
)

// fallbackIcon is the sprite symbol a panel gets when it names an unknown one.
const fallbackIcon = "list"

// loadAssets returns the page assets. The embedded copy is built once; with
// dev assets on (SetDevAssets) every call rebuilds from the working tree, so
// an edit shows on the next request and a failed build is never kept.
func loadAssets() (*pageAssets, error) {
	if root := devAssets.Load(); root != nil {
		return buildAssets(func(name string) ([]byte, error) { return root.ReadFile(devAssetDir + "/" + name) })
	}
	assetsOnce.Do(func() { loadedAssets, assetsErr = buildAssets(embeddedAsset) })
	return loadedAssets, assetsErr
}

// embeddedAsset reads an asset file from the embedded copy.
func embeddedAsset(name string) ([]byte, error) { return assetFS.ReadFile("assets/" + name) }

// devAssets, when set, is the source root loadAssets reads from instead of
// the embedded copy. The daemon sets it at startup.
var devAssets atomic.Pointer[wsdevassets.Root]

// SetDevAssets turns dev assets on for this process, or off with nil. The
// mermaid bundle stays embedded either way (MermaidAsset names it by hash).
func SetDevAssets(root *wsdevassets.Root) { devAssets.Store(root) }

// DevAssetsOn reports whether dev assets are on (SetDevAssets): the
// templates are then re-read on every render, so a render's output is not a
// function of its view alone.
func DevAssetsOn() bool { return devAssets.Load() != nil }

// devAssetDir is where the whiteboard assets live under a dev source root.
const devAssetDir = "internal/wswhiteboard/assets"

// frameKeyPrefix keeps the frame's copy and relative body-time strings local
// to the opaque-origin document, without the rest of the dashboard table.
const frameKeyPrefix = "js.wb."

// buildAssets reads every asset by file name through read and parses the
// template set.
func buildAssets(read func(name string) ([]byte, error)) (*pageAssets, error) {
	var err error
	get := func(name string) string {
		if err != nil {
			return ""
		}
		var data []byte
		data, err = readPageSource(read, name)
		return string(data)
	}
	svgView := get("svg-view.js")
	chg := get("chg.js")
	panelDiff := get("panel-diff.js")
	code := get("code.js")
	ago := get("ago.js")
	a := &pageAssets{
		css:         template.CSS(get("page.css")),
		answers:     get("answers.js"),
		form:        get("form.js"),
		selectAll:   get("select-all.js"),
		code:        code,
		stream:      get("stream.js"),
		pill:        get("pill.js"),
		rail:        get("rail.js"),
		ago:         ago,
		deps:        get("deps.js"),
		cardFind:    get("card-find.js"),
		mermaidInit: template.JS(get("mermaid-init.js")),
		docFrame:    svgView + "\n" + ago + "\n" + chg + "\n" + code + "\n" + panelDiff + "\n" + get("doc-frame.js"),
		chg:         chg,
		panelDiff:   panelDiff,
		viewer:      "(function () {\n" + svgView + "\n" + get("viewer.js") + "})();\n",
		sprite:      template.HTML(strings.TrimSpace(get("sprite.svg"))),
		icons:       map[string]bool{},
	}
	a.frameCSSFile = newFrameFile("css", string(a.css)+"\n"+frameCSS)
	a.frameJSFile = newFrameFile("js", a.docFrame)
	a.frameI18nFiles = map[string]frameFile{}
	for _, lang := range wsi18n.Languages() {
		a.frameI18nFiles[lang.Code] = newFrameFile("js", string(wsi18n.SubsetScript(lang.Code, frameKeyPrefix)))
	}
	source := get("page.tmpl")
	if err != nil {
		return nil, err
	}
	for _, match := range spriteSymbolPattern.FindAllStringSubmatch(string(a.sprite), -1) {
		a.icons[match[1]] = true
	}
	a.tmpls = map[string]*template.Template{}
	for _, l := range wsi18n.Languages() {
		lang := l.Code
		funcs := template.FuncMap{
			"clockOf": func(at string, ms int64, stamp, clock, class string) clockView {
				return clockView{At: at, Ms: ms, Stamp: stamp, Clock: clock, Class: class}
			},
			"icon": func(name string) (template.HTML, error) {
				if !a.icons[name] {
					return "", fmt.Errorf("whiteboard page: no sprite symbol %q", name)
				}
				return template.HTML(`<svg class="i" aria-hidden="true"><use href="#i-` + name + `"/></svg>`), nil
			},
			"waitingSummary": func(deps []DependencyView, ready bool, waiters int) string {
				if len(deps) == 0 {
					return wsi18n.T(lang, "wb.waiting.reverse", waiters)
				}
				if ready {
					return wsi18n.T(lang, "wb.waiting.ready") + " · " + wsi18n.T(lang, "wb.waiting.allResolved")
				}
				n := resolvedTargets(deps)
				return wsi18n.T(lang, "wb.waiting.progress", len(deps), n, len(deps)-n)
			},
			"waitingPercent": func(deps []DependencyView) int {
				if len(deps) == 0 {
					return 0
				}
				return resolvedTargets(deps) * 100 / len(deps)
			},
			"manualRelease": func(dep DependencyView) bool {
				return dep.Derived || strings.HasPrefix(dep.Token, "exec:")
			},
			"lines":       rationaleLines,
			"box":         rowBox,
			"href":        rowHref,
			"actionAttrs": panelActionAttrs,
			"t":           func(key string, args ...any) string { return wsi18n.T(lang, key, args...) },
		}
		// The file's template is "page.tmpl" under the root "page", as
		// ParseFS names it.
		t := template.New("page").Funcs(funcs)
		if _, err := t.New("page.tmpl").Parse(source); err != nil {
			return nil, err
		}
		a.tmpls[lang] = t
	}
	a.tmpl = a.tmpls[wsi18n.Default]
	return a, nil
}

func resolvedTargets(deps []DependencyView) int {
	n := 0
	for _, dep := range deps {
		if dep.Resolved {
			n++
		}
	}
	return n
}

// templateFor is the template set that renders in lang; an unsupported or
// empty lang renders English.
func (a *pageAssets) templateFor(lang string) *template.Template {
	if t, ok := a.tmpls[lang]; ok {
		return t
	}
	return a.tmpl
}

// panelActionAttrs are the attributes of a panel row's file action link: the
// ones actionAttrs gives a markdown file target, so the link menu reads both
// alike, then the row's path as the title.
func panelActionAttrs(r PanelRow) template.HTMLAttr {
	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	writeAttrs(w, append(actionAttrs(target{action: r.Action}), [2]string{"title", r.Path}))
	_ = w.Flush()
	return template.HTMLAttr(buf.String())
}

// Regions are a served page's board and panels rendered as fragments.
type Regions struct {
	// Board is the topic blocks and the reply bar.
	Board template.HTML
	// Panels is every panel, in order.
	Panels template.HTML
	// Mermaid reports that Board holds a mermaid block outside the topic-body
	// frames, so the page loads the shared mermaid script (MermaidAsset) and
	// MermaidInitScript; a frame loads them itself for a block in its body.
	Mermaid bool
	// CountPolicy and CountAcks are the header's counts: open policy and
	// check items (the amber "answer needed" chip), then open technical and
	// announced items.
	CountPolicy, CountAcks int
}

// TopicRegions is a board's ordered direct sections and non-topic bytes.
// Board preserves their original positions; Shell collects inter-section
// whitespace and the trailing controls/deferred history, as the stream parser does.
type TopicRegions struct {
	Board  template.HTML
	Topics []TopicUnit
	Shell  template.HTML
}

// TopicUnit is an immutable rendered topic, with the same hash as data-th.
type TopicUnit struct {
	ID, Hash string
	HTML     template.HTML
	Mermaid  bool
	// after is template whitespace outside the section: it is reassembled
	// in place in Board, but must never enter a single-element wire unit.
	after template.HTML
}

// TopicReuse reuses a topic by its complete render revision. The caller owns
// the cache and must also invalidate it when its external render inputs change.
type TopicReuse func(id, revision string, render func() (TopicUnit, error)) (TopicUnit, error)

// RenderRegions renders view's board and panels as fragments. Each topic body
// renders inside its own sandboxed frame (framedDoc), since the page that
// hosts the fragments is served from the dashboard daemon's origin and a
// body's raw HTML and scripts must not run there. Build view with LayoutFull.
func RenderRegions(view View) (Regions, error) {
	regions, _, err := RenderRegionsWithTopics(view, nil)
	return regions, err
}

// RenderRegionsWithTopics is RenderRegions with source-owned topic reuse.
// Cache hits skip both body/panel markdown and the topic template.
func RenderRegionsWithTopics(view View, reuse TopicReuse) (Regions, *TopicRegions, error) {
	a, err := loadAssets()
	if err != nil {
		return Regions{}, nil, fmt.Errorf("whiteboard page assets: %w", err)
	}
	view.Layout = LayoutFull
	data := pageDataBase(view, a)
	data.topicReuse = reuse
	data.lazyAssets = a
	var board, panels bytes.Buffer
	tmpl := a.templateFor(view.Lang)
	if err := tmpl.ExecuteTemplate(&board, "board", data); err != nil {
		return Regions{}, nil, fmt.Errorf("whiteboard board: %w", err)
	}
	for _, panel := range view.Panels {
		if err := tmpl.ExecuteTemplate(&panels, "panel", data.PanelOf(panel)); err != nil {
			return Regions{}, nil, fmt.Errorf("whiteboard panel %s: %w", panel.ID, err)
		}
	}
	boardHTML := template.HTML(board.String())
	return Regions{
		Board:       boardHTML,
		Panels:      template.HTML(panels.String()),
		Mermaid:     data.Mermaid,
		CountPolicy: data.CountPolicy,
		CountAcks:   data.CountAcks,
	}, collectTopicRegions(boardHTML, view.Topics, data.topicUnits), nil
}

// RenderClosedTopics renders the closed topics of a view built with a
// ViewInput.Closed window as one fragment, as RenderRegions renders them on
// the board (each topic body in its sandboxed frame), followed by the
// show-more control when more remain: the served page's show-more answer.
func RenderClosedTopics(view View) (template.HTML, error) {
	a, err := loadAssets()
	if err != nil {
		return "", fmt.Errorf("whiteboard page assets: %w", err)
	}
	view.Layout = LayoutFull
	var buf bytes.Buffer
	if err := a.templateFor(view.Lang).ExecuteTemplate(&buf, "closedpage", newPageData(view, a)); err != nil {
		return "", fmt.Errorf("whiteboard closed topics: %w", err)
	}
	return template.HTML(buf.String()), nil
}

// SharedAssets are the assets for a page that hosts
// RenderRegions fragments: the stylesheet the fragments are styled by, the
// icon sprite they reference, the answer-form script that drives the board's
// reply bar (a function body: run it inside a function scope), and the
// mermaid theme script that runs after MermaidAsset.
type SharedAssets struct {
	CSS             template.CSS
	Sprite          template.HTML
	FormScript      template.JS
	SelectAllScript template.JS
	// AnswersScript defines window.wbAnswers, the answer-block builder
	// FormScript calls; a self-contained script for the page to inline once,
	// ahead of the first FormScript run.
	AnswersScript template.JS
	MermaidInit   template.JS
	// ViewerScript is the image viewer, a self-contained script for the page
	// to inline once, outside any region (it listens on the document).
	ViewerScript template.JS
}

// PageAssets returns the served page's shared assets.
func PageAssets() (SharedAssets, error) {
	a, err := loadAssets()
	if err != nil {
		return SharedAssets{}, fmt.Errorf("whiteboard page assets: %w", err)
	}
	return SharedAssets{CSS: a.css, Sprite: a.sprite, FormScript: template.JS(a.form + "\n" + a.code + "\n" + a.pill + "\n" + a.rail + "\n" + a.ago + "\n" + a.deps + "\n" + a.stream + "\n" + a.panelDiff + "\n" + a.cardFind), AnswersScript: template.JS(a.answers), SelectAllScript: template.JS(a.selectAll), MermaidInit: a.mermaidInit, ViewerScript: template.JS(a.viewer)}, nil
}

// pageData is what the fragment templates execute on.
type pageData struct {
	View
	// docs holds each topic body's sandboxed frame, keyed by topic name.
	docs    map[string]template.HTML
	entries map[string]template.HTML
	// tmpl is the template set the page renders with (templateFor), which
	// TopicHTML executes the topic template from.
	tmpl         *template.Template
	lazyAssets   *pageAssets
	topicReuse   TopicReuse
	topicUnits   map[string]TopicUnit
	topicContext string
	// framing is set while a framed body renders, so its file images take
	// DocImageSrc.
	framing bool
	// inDoc is set while any topic body renders, framed or not, so its file
	// targets never take OutsideImage as a src or link; a framed body's
	// image takes it only as a relay (relayImage).
	inDoc bool
	// relays collects the relay URLs of the framed body being rendered
	// (relayImage), which framedDoc lists on its frame.
	relays []string

	CountPolicy, CountAcks int
	// CountBulk is how many items the bulk checkbox answers.
	CountBulk int

	// Mermaid records blocks outside body frames so the hosting page can
	// load Mermaid for those regions. Each body frame loads its own assets.
	Mermaid bool
	// noMermaid renders a mermaid fence as a plain code block (RenderText
	// without TextTargets.Mermaid).
	noMermaid bool
	// noInlineTags keeps the item pipelines' allowlisted inline tags
	// (inline_tags.go) escaped text, for RenderText: they are an item-field
	// rule, and other authors' text keeps every tag as text.
	noInlineTags bool
	// attachmentSrc is RenderText's TextTargets.AttachmentSrc.
	attachmentSrc func(oid string) string
	// wikiLink is RenderText's TextTargets.WikiLink.
	wikiLink func(name, path string) string

	icons map[string]bool
	// onPage holds every item ID the page renders, the targets an in-page
	// link may point at.
	onPage      map[int]bool
	panelOnPage map[string]bool
	md          *markdown
	// trusted, set only while a finished reply renders, records the opening
	// attributes of each link and image the markdown renderer resolved
	// (writeTrustRef) for the sanitizer.
	trusted *[]trustedTarget
	// svgIDs is the id prefix of the finished reply being sanitized.
	svgIDs string
	// footnotePrefix is the footnote id prefix of the text being rendered
	// (convert).
	footnotePrefix string
}

// MermaidFile is the shared mermaid script's name, beside the page.
func (p *pageData) MermaidFile() string {
	name, _ := MermaidAsset()
	return name
}

// newPageData builds the data a page renders, with every topic body inside a
// sandboxed frame (framedDoc).
func newPageData(view View, a *pageAssets) *pageData {
	p := pageDataBase(view, a)
	topics := view.Topics
	if !view.LazyHistory {
		topics = append(append([]TopicView(nil), view.Topics...), view.DeferredTopics...)
	}
	for _, t := range topics {
		p.prepareTopic(t, a)
	}
	return p
}

func pageDataBase(view View, a *pageAssets) *pageData {
	p := &pageData{
		View:        view,
		icons:       a.icons,
		onPage:      map[int]bool{},
		panelOnPage: map[string]bool{},
		docs:        map[string]template.HTML{},
		entries:     map[string]template.HTML{},
		tmpl:        a.templateFor(view.Lang),
		topicUnits:  map[string]TopicUnit{},
	}
	for _, t := range view.Topics {
		for _, panel := range t.PanelRecaps {
			p.panelOnPage[panel.PanelID] = true
		}
		p.CountPolicy += t.AskCount()
		p.CountAcks += len(t.Acks)
		for _, list := range [][]ItemView{t.Policy, t.Acks, t.Checks, t.Expanded, t.Collapsed} {
			for _, iv := range list {
				p.onPage[iv.ID] = true
			}
		}
	}
	for _, t := range view.DeferredTopics {
		for _, panel := range t.PanelRecaps {
			p.panelOnPage[panel.PanelID] = true
		}
	}
	p.CountBulk = p.CountAcks
	return p
}

// prepareTopic runs only after board-wide reference targets are complete.
func (p *pageData) prepareTopic(t TopicView, a *pageAssets) {
	// A lazy render emits only the current bodies of live rows; gap rows
	// and earlier versions come with the complete topic on demand.
	live := lazyLiveEntries(t, p.LazyHistory)
	pages := append([]EntryPage{t.Stream}, t.Older...)
	for _, page := range pages {
		for _, run := range page.Runs {
			e := run.Entry
			if e.Kind == EntryPanel && e.HasBody() && (live == nil || live[e.ID] && !e.Historical) {
				p.entries[e.ID] = p.panelHTML(t.Name, e, a)
			}
		}
	}
	if t.Doc == "" {
		return
	}
	page := p.Mermaid
	p.Mermaid = false
	p.framing, p.inDoc, p.relays = true, true, nil
	html := p.renderDoc(t.Doc, t.DocMarks)
	p.framing, p.inDoc = false, false
	html = p.framedDoc(t.Name, "", html, t.DocStale, p.Mermaid, p.relays, a, t.BodyUpdatedAt)
	p.Mermaid = false
	p.Mermaid = p.Mermaid || page
	p.docs[t.Name] = html
}

// DocHTML is topic's rendered body (see docs).
func (p *pageData) DocHTML(topic string) template.HTML { return p.docs[topic] }

// EntryHTML is a panel's content, isolated just like the topic body.
func (p *pageData) EntryHTML(id string) template.HTML { return p.entries[id] }

func (p *pageData) panelHTML(topic string, e EntryView, a *pageAssets) template.HTML {
	pageMermaid := p.Mermaid
	p.Mermaid = false
	p.framing, p.inDoc, p.relays = true, true, nil
	out := p.renderPanel(e)
	p.framing, p.inDoc = false, false
	out = p.framedDoc(topic, e.ID, out, false, p.Mermaid, p.relays, a, "")
	p.Mermaid = false
	p.Mermaid = p.Mermaid || pageMermaid
	return out
}

// frameCSS is what a topic-body frame adds to the page stylesheet: a
// transparent page, so the topic card shows through, that never scrolls, since the host
// sizes the frame to its content.
const frameCSS = "html, body { background: transparent; overflow: hidden; }"

// frameFile is one shared topic-body frame asset: its name, which carries a
// hash of its bytes, so a new build never changes the bytes under a name a
// cached frame still loads, and its bytes.
type frameFile struct {
	name string
	data []byte
}

func newFrameFile(ext, data string) frameFile {
	sum := sha256.Sum256([]byte(data))
	return frameFile{name: "wb-frame-" + hex.EncodeToString(sum[:])[:12] + "." + ext, data: []byte(data)}
}

// FrameAsset returns the bytes and content type of the shared topic-body
// frame asset called name (framedDoc), or ok false when no current asset has
// that name. A frame's document loads both by a relative name, so the
// daemon serves them beside the page that hosts the frames. Neither depends
// on any session: each is the page stylesheet or the frame script, the same
// for every viewer.
func FrameAsset(name string) (data []byte, contentType string, ok bool) {
	a, err := loadAssets()
	if err != nil {
		return nil, "", false
	}
	switch name {
	case a.frameCSSFile.name:
		return a.frameCSSFile.data, "text/css; charset=utf-8", true
	case a.frameJSFile.name:
		return a.frameJSFile.data, "text/javascript; charset=utf-8", true
	}
	for _, file := range a.frameI18nFiles {
		if name == file.name {
			return file.data, "text/javascript; charset=utf-8", true
		}
	}
	return nil, "", false
}

// framedDoc wraps a rendered topic body in the sandboxed frame a served page
// shows it in, one frame per body. The page lives in the dashboard daemon's
// origin, whose routes post messages and steers to sessions, so a body's raw
// HTML must not run there:
// the frame's srcdoc document gets allow-scripts, so an inline canvas script
// still draws, but no allow-same-origin, so it runs in an opaque origin that
// can neither read the host page nor reach the daemon's routes with the
// viewer's credentials. allow-popups and allow-popups-to-escape-sandbox let
// a plain link open its new tab (<base target="_blank">) as an ordinary
// page; there is no allow-forms, allow-top-navigation, or allow-modals.
//
// The document loads the page stylesheet, so a body keeps the page's theme
// variables and light/dark switch, and doc-frame.js, which posts the
// content height and hands the host its marked-link and item-reference
// clicks (the host's half lives in the dashboard script), both as shared
// files beside the page (FrameAsset) rather than inline, so a board's
// frames parse them once and every document stays small. mermaid tells the
// frame to load the shared mermaid script beside the page for its own
// blocks. An image reaches the frame as whatever src ImageSrc (or
// DocImageSrc) gave it, except that the frame's requests carry no cookie, so
// an image a served page points at its cookie-gated image route for, inside
// the session root or outside it (whose bytes must not be handed to every
// viewer), keeps its path text marked data-relay instead, and the frame
// lists those URLs as its own
// data-relay, space-separated. The host fetches a URL its frame asks for only
// when it is on that list, which the body's own markup cannot extend, and
// hands the frame the image (doc-frame.js and the dashboard script).
// The frame carries its topic name as data-doc, so the host can keep each
// frame's height across a board swap.
//
// The server never mounts a frame: it writes an inert placeholder, a div
// carrying the frame's class and attributes, its document as data-srcdoc,
// and its sandbox token set as data-sandbox (frameSandbox). The host page
// replaces the placeholder with the iframe (dashboard.js mountFrame), sandbox
// and srcdoc set before it is connected, only once every <details> around it
// is open (mountFrames), so a frame under a closed fold costs no document
// until it is opened. An <iframe> left without srcdoc would not be inert: it
// commits an about:blank document synchronously as the page parses.
func (p *pageData) framedDoc(topic, entry string, body template.HTML, stale, mermaid bool, relays []string, a *pageAssets, bodyAt string) template.HTML {
	title, key, entryAttr := topic, "topic:"+topic, ""
	if entry != "" {
		title, key = topic+"/"+entry, "entry:"+entry
		entryAttr = ` data-entry="` + template.HTMLEscapeString(entry) + `"`
	}
	var doc strings.Builder
	doc.WriteString(frameDocStart + `<head><meta charset="utf-8"><meta name="wb-doc-key" content="` + template.HTMLEscapeString(key) + `"><base target="_blank"><link rel="stylesheet" href="` + a.frameCSSFile.name + `">`)
	if entry == "" && bodyAt != "" {
		doc.WriteString(`<meta name="wb-body-at" data-body-at="` + template.HTMLEscapeString(bodyAt) + `">`)
	}
	doc.WriteString(`</head><body class="doc-frame"><div class="doc-body md` + staleClass(stale) + `">`)
	doc.WriteString(string(body))
	// This opaque-origin frame cannot reach the host's wsT or cookie-gated
	// assets. Its fixed public subset uses the same exemption as frameJSFile.
	lang := p.Lang
	if !wsi18n.Supported(lang) {
		lang = wsi18n.Default
	}
	doc.WriteString(`</div><script src="` + a.frameI18nFiles[lang].name + `"></script><script src="` + a.frameJSFile.name + `"></script>`)
	if mermaid {
		doc.WriteString(`<script src="` + template.HTMLEscapeString(p.MermaidFile()) + `"></script><script>wbDocMermaid(function () {` + string(a.mermaidInit) + `});</script>`)
	}
	doc.WriteString(`</body></html>`)
	relay := ""
	if len(relays) > 0 {
		relay = ` data-relay="` + template.HTMLEscapeString(strings.Join(relays, " ")) + `"`
	}
	return template.HTML(`<div class="doc-frame" title="` + template.HTMLEscapeString(title) + `" data-doc="` + template.HTMLEscapeString(key) + `" data-doc-key="` + template.HTMLEscapeString(key) + `" data-topic="` + template.HTMLEscapeString(topic) + `"` + entryAttr + relay + ` data-sandbox="` + frameSandbox + `" data-srcdoc="` +
		template.HTMLEscapeString(doc.String()) + `"></div>`)
}

// frameSandbox is the sandbox token set of every srcdoc frame the daemon
// builds (SandboxedPage, and framedDoc's placeholder, whose data-sandbox the
// page copies onto the frame it mounts): allow-scripts, so the document's own
// scripts run, but no allow-same-origin, so they run in an opaque origin
// that can neither read the host page nor reach the daemon's routes with
// the viewer's credentials; popups open as ordinary pages. Adding a token
// here (allow-same-origin above all, or allow-forms, allow-top-navigation,
// allow-modals, allow-downloads) breaks that isolation for every frame.
const frameSandbox = "allow-scripts allow-popups allow-popups-to-escape-sandbox"

// SandboxedPage returns the frame that shows doc, a whole author-written
// HTML document, under frameSandbox: an <iframe> titled title whose srcdoc
// is doc verbatim, escaped as an attribute. Nothing is injected - no
// stylesheet, script, <base>, or CSP meta - so the author's page renders
// as written; the host neither posts to nor listens for messages from it.
//
// A srcdoc document inherits the policy container of the page that frames
// it, sandbox or not: whatever Content-Security-Policy the hosting page
// sends governs the author's page too. The tracker pages send only
// frame-src and frame-ancestors (pinned in the dashboard's page tests), so
// an author page may load scripts and styles from any origin; a later
// script-src, connect-src, or default-src on a page that hosts these
// frames silently breaks them, and frame-src 'self' already keeps an author
// page from framing another origin.
func SandboxedPage(title, doc string) template.HTML {
	return template.HTML(`<iframe title="` + template.HTMLEscapeString(title) + `" sandbox="` + frameSandbox + `" srcdoc="` +
		template.HTMLEscapeString(doc) + `"></iframe>`)
}

// staleClass is the class a topic body carries once its change marks are
// stale (TopicView.DocStale); chg.js dims the bars from it.
func staleClass(stale bool) string {
	if stale {
		return " chg-stale"
	}
	return ""
}

// frameDocStart opens every topic-body frame's document (framedDoc), the
// root ThemeFrames pins a theme on.
const frameDocStart = `<!DOCTYPE html><html>`

// ThemeFrames pins theme ("light" or "dark") on the root of every topic-body
// frame in board, as the host page pins it on its own root: a frame is a
// separate document whose stylesheet otherwise follows the OS, under a
// transparent background that shows the host's pinned card. Any other theme
// returns board unchanged (the frames follow the OS, as the page does). The
// daemon applies it per request, from the request's theme cookie, so the
// board itself renders theme-free; the page script carries a later switch
// to the frames (doc-frame.js). The match is a plain substring, so it pins
// a placeholder's data-srcdoc document (framedDoc) just as a frame's srcdoc
// one: the document must open with frameDocStart, nothing before it.
func ThemeFrames(board template.HTML, theme string) template.HTML {
	if theme != "light" && theme != "dark" {
		return board
	}
	from := `srcdoc="` + template.HTMLEscapeString(frameDocStart)
	to := `srcdoc="` + template.HTMLEscapeString(`<!DOCTYPE html><html data-theme="`+theme+`">`)
	return template.HTML(strings.ReplaceAll(string(board), from, to))
}

// topicData is one topic block with the page it sits on.
type topicData struct {
	TopicView
	Page *pageData
}

// DocClass is the class list of the topic's framed body box, plus chg-stale
// once its change marks are stale (TopicView.DocStale).
func (d topicData) DocClass() string {
	return "framed" + staleClass(d.DocStale)
}

// TopicOf pairs a topic with the page for the topic template.
func (p *pageData) TopicOf(t TopicView) topicData { return topicData{TopicView: t, Page: p} }

// TopicHTML renders t's topic section. The section carries data-th, a hash of
// its own rendered bytes (data-th excluded): the board swap compares it in place of
// the section's whole HTML to keep an unchanged topic's node. Hashing the
// bytes covers everything the HTML compare covered (classes, frame
// documents, language); the theme is pinned after render (ThemeFrames), so
// the hash is theme-free. The section's opening tag holds no raw ">" (its
// attribute values are escaped), so data-th goes before the first one.
func (p *pageData) TopicHTML(t TopicView) (template.HTML, error) {
	if p.lazyAssets != nil {
		render := func() (TopicUnit, error) {
			previous := p.Mermaid
			p.Mermaid = false
			p.prepareTopic(t, p.lazyAssets)
			html, err := p.renderTopicHTML(t)
			mermaid := p.Mermaid
			p.Mermaid = previous || mermaid
			if err != nil {
				return TopicUnit{}, err
			}
			// The topic template ends with whitespace after </section>.
			// Keep it in the region, not in the single-element delta unit.
			section := strings.TrimRight(string(html), " \t\r\n\f")
			if !strings.HasSuffix(section, "</section>") {
				return TopicUnit{}, fmt.Errorf("topic %s: section is not the final element", t.Name)
			}
			return TopicUnit{ID: t.Name, Hash: topicHTMLHash(html), HTML: template.HTML(section), after: html[len(section):], Mermaid: mermaid}, nil
		}
		var unit TopicUnit
		var err error
		if p.topicReuse == nil {
			unit, err = render()
		} else {
			unit, err = p.topicReuse(t.Name, p.topicRevision(t), render)
		}
		p.Mermaid = p.Mermaid || unit.Mermaid
		p.topicUnits[t.Name] = unit
		return unit.HTML + unit.after, err
	}
	return p.renderTopicHTML(t)
}

func (p *pageData) renderTopicHTML(t TopicView) (template.HTML, error) {
	var buf bytes.Buffer
	if err := p.tmpl.ExecuteTemplate(&buf, "topic", p.TopicOf(t)); err != nil {
		return "", err
	}
	html := buf.String()
	sum := sha256.Sum256(buf.Bytes())
	at := strings.IndexByte(html, '>')
	if !strings.HasPrefix(html, "<section ") || at < 0 {
		return "", fmt.Errorf("topic %s: the section's opening tag is not first", t.Name)
	}
	return template.HTML(html[:at] + ` data-th="` + hex.EncodeToString(sum[:8]) + `"` + html[at:]), nil
}

// RecapHTML renders the topic's recap through itemField, as an item's block
// fields render, salted with the topic's key.
func (p *pageData) RecapHTML(t TopicView) template.HTML {
	if t.Head != nil {
		return p.Inline(t.Head.Summary())
	}
	return p.itemField(t.Recap, "topic/"+t.Name+"/recap")
}

// itemData is one item with the page it sits on.
type itemData struct {
	ItemView
	Page *pageData
	// Static renders the item's options as display, not answer controls
	// (the "opts" template), for a settled row's expanded body.
	Static bool
	// History is set only for the unified full-page timeline row.
	History *EntryView
	// AskClock/AskAt/AskReasked put a question's ask time and re-ask time in
	// its own history row, or in a stacked open card's jump to that row.
	AskClock, AskAt, AskReasked string
	// AskMs/AskStamp give the ask clock the same relative-time data every
	// other history clock carries.
	AskMs    int64
	AskStamp string
	// AskEntry is a bottom-stacked open ask's history event: its card links
	// there (the ask-time line) through the shared fragment navigation.
	AskEntry                                 string
	PreviousID, PreviousQuote, PreviousClock string
}

// ItemOf pairs an item with the page for the item templates.
func (p *pageData) ItemOf(iv ItemView) itemData { return itemData{ItemView: iv, Page: p} }

// AskOf is an open ask's bottom card: the item with its ask-time line.
func (p *pageData) AskOf(a openAsk) itemData {
	return itemData{ItemView: a.Item, Page: p, AskEntry: a.Entry, AskClock: a.Clock, AskAt: a.At, AskMs: a.Ms, AskStamp: a.Stamp, AskReasked: a.Reasked, PreviousID: a.PreviousID, PreviousQuote: a.PreviousQuote, PreviousClock: a.PreviousClock}
}

// Num is the item's number as its label shows it, without the parentheses;
// the reply grammar wraps it back as (nn).
func (d itemData) Num() string { return strings.TrimSuffix(strings.TrimPrefix(d.Label, "("), ")") }

// Void reports a withdrawn or superseded item: settled without an answer.
func (d itemData) Void() bool {
	return d.Status == StatusWithdrawn || d.Status == StatusSuperseded
}

// Assumed reports an item whose answer is the lead's reading, which its
// settled row marks.
func (d itemData) Assumed() bool { return d.Status == StatusAssumed }

// PickedOption is the option a settled or assumed answer chose, nil when none.
func (d itemData) PickedOption() *OptionView {
	for i := range d.Options {
		if d.Options[i].Picked {
			return &d.Options[i]
		}
	}
	return nil
}

// AsStatic is the item with its options rendered Static.
func (d itemData) AsStatic() itemData {
	d.Static = true
	return d
}

// TimelineCheck keeps an assumed check's real controls (its veto note) in
// its history row. An open check's controls stack at the topic's bottom.
func (d itemData) TimelineCheck() bool {
	return d.Page != nil && d.Class == ClassCheck && d.HistoryAnchor() && d.Assumed()
}

// Expandable reports whether a row has controls, options or rationale beyond
// its compact text. Links never count; the compact row prints them.
func (d itemData) Expandable() bool {
	return d.TimelineCheck() || len(d.Options) > 0 || d.Rationale != ""
}

// OptionHTML, RationaleHTML, and RecHTML render the item's block fields
// through itemField, each salted with its place on the page so identical
// texts never share a footnote or SVG id prefix.
func (d itemData) OptionHTML(o OptionView) template.HTML {
	return d.Page.itemField(o.Text, fmt.Sprintf("%d/option/%s", d.ID, o.Letter))
}

func (d itemData) RationaleHTML() template.HTML {
	return d.Page.itemField(d.Rationale, fmt.Sprintf("%d/why", d.ID))
}

func (d itemData) RecHTML() template.HTML {
	return d.Page.itemField(d.RecText, fmt.Sprintf("%d/recommend", d.ID))
}

// PickedChip is the picked option's chip text: its first line as inline
// markdown, with the HTML an option may hold dropped to its text (PlainHTML),
// since the chip sits in one line of the settled row's summary.
func (d itemData) PickedChip(o OptionView) template.HTML {
	return d.Page.FirstLine(PlainHTML(o.Text))
}

// Stacked reports whether the item's options render as stacked cards: any
// option holds block content (IsBlock).
func (d itemData) Stacked() bool {
	for _, o := range d.Options {
		if d.Page.IsBlock(o.Text) {
			return true
		}
	}
	return false
}

// checkWord splits a check's answer at its first word: the word, lowered,
// when it is "ok" or "no" standing alone or followed by punctuation ("no,
// use A", "ok - one typo"), and the text after it with the joining spaces and punctuation
// trimmed. Any other answer, "no problem" included, returns "" and the whole
// answer, trimmed.
func checkWord(answer string) (word, rest string) {
	a := strings.TrimSpace(answer)
	end := strings.IndexFunc(a, func(r rune) bool { return !unicode.IsLetter(r) })
	if end < 0 {
		end = len(a)
	}
	after := strings.TrimLeftFunc(a[end:], unicode.IsSpace)
	if next, _ := utf8.DecodeRuneInString(after); after != "" && !unicode.IsPunct(next) {
		return "", a
	}
	switch w := strings.ToLower(a[:end]); w {
	case "ok", "no":
		return w, strings.TrimLeftFunc(a[end:], func(r rune) bool { return unicode.IsSpace(r) || unicode.IsPunct(r) })
	}
	return "", a
}

// CheckVerdict is an answered or assumed check's stamp, "ok" or "no", when
// its answer starts with that word (case aside); "" for any other answer and
// for an open check.
func (d itemData) CheckVerdict() string {
	if d.Status == StatusOpen {
		return ""
	}
	word, _ := checkWord(d.Answer)
	return word
}

// CheckNo reports a check answered no, which its row marks with a red x and
// the user's words.
func (d itemData) CheckNo() bool { return d.CheckVerdict() == "no" }

// CheckReading is the part of a check's answer its stamp does not show: the
// text after ok or no ("no, use the other one"), or the whole answer when it
// starts with neither. Empty for an open check.
func (d itemData) CheckReading() string {
	if d.Status == StatusOpen {
		return ""
	}
	_, rest := checkWord(d.Answer)
	return rest
}

// Reading is the lead's reading of the answer beyond a picked option: the
// text after the option letter, or the whole answer when it names none.
func (d itemData) Reading() string {
	if d.Page != nil && d.Class == ClassCheck {
		return d.CheckReading()
	}
	if d.PickedOption() != nil {
		return d.AnswerText
	}
	return d.Answer
}

// textPart is a run of lead-authored text; Ref is the item it anchors to,
// zero for plain text.
type textPart struct {
	Text   string
	Ref    int
	Target string
}

// Parts splits lead-authored text at its {{nn}} item references. A reference
// to an item this page renders becomes an in-page anchor labelled (nn); one
// to an item the store holds but the page omits becomes plain (nn); one to an
// ID the store does not hold stays verbatim. References are display only:
// the store never refuses an update over one.
func (p *pageData) Parts(s string) []textPart {
	var parts []textPart
	last := 0
	for _, m := range authoredRefPattern.FindAllStringSubmatchIndex(s, -1) {
		ref := s[m[2]:m[3]]
		id, err := strconv.Atoi(ref)
		panel := strings.HasPrefix(ref, "p") && p.StoredPanels[ref]
		if !panel && (err != nil || !p.Stored[id]) {
			continue
		}
		if m[0] > last {
			parts = append(parts, textPart{Text: s[last:m[0]]})
		}
		part := textPart{Text: FormatID(id)}
		if panel {
			part.Text = ref
			if p.panelOnPage[ref] {
				part.Target = ref
			}
		} else if p.onPage[id] {
			part.Ref = id
		}
		parts = append(parts, part)
		last = m[1]
	}
	if last < len(s) {
		parts = append(parts, textPart{Text: s[last:]})
	}
	return parts
}

// Parts splits the item's lead-authored text for the "text" template.
func (d itemData) Parts(s string) []textPart { return d.Page.Parts(s) }

// panelData is one panel with the page it sits on.
type panelData struct {
	Panel
	Page *pageData
}

// PanelOf pairs a panel with the page for the panel template.
func (p *pageData) PanelOf(panel Panel) panelData { return panelData{Panel: panel, Page: p} }

// IconName is the panel's sprite symbol, or the fallback when the sprite has
// no symbol of that name.
func (d panelData) IconName() string {
	if d.Page.icons[d.Icon] {
		return d.Icon
	}
	return fallbackIcon
}

// rationaleLines splits a rationale into its non-blank lines, one ack-row
// reason each.
func rationaleLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, line)
		}
	}
	return out
}

// rowHref trusts a panel row link that is a relative path or a file: URL.
// html/template rewrites any file: URL to #ZgotmplZ; every other scheme keeps
// the template's normal URL filtering.
func rowHref(link string) any {
	u, err := url.Parse(link)
	if err == nil && (u.Scheme == "" || u.Scheme == "file") {
		return template.URL(link)
	}
	return link
}

// rowBox is the sprite symbol of the checklist box a panel row's state shows.
func rowBox(state string) string {
	switch state {
	case "done":
		return "box-done"
	case "wip":
		return "box-wip"
	case "defer":
		return "box-defer"
	default:
		return "box"
	}
}

// clockView is the "clock" template's input: one entry-at time element with
// its datetime, exact stamp, Unix-millisecond time and any extra class.
type clockView struct {
	At    string
	Ms    int64
	Stamp string
	Clock string
	Class string
}
