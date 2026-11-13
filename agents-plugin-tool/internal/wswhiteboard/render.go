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
// script both layouts run, the change-signal script only the full page runs,
// the mermaid theme script a page with a mermaid block runs, the script a
// served page's topic-body frames run, the image viewer script the full page
// runs (with the SVG helpers it shares with those frames), and the icon
// sprite. Everything is inlined, so a page needs no other file beyond the
// full page's stamp script and, when it holds a mermaid block, the shared
// mermaid script (MermaidAsset).
//
//go:embed assets/page.tmpl assets/page.css assets/stream.js assets/chg.js assets/answers.js assets/form.js assets/code.js assets/live.js assets/pill.js assets/rail.js assets/ago.js assets/mermaid-init.js assets/doc-frame.js assets/viewer.js assets/svg-view.js assets/sprite.svg
var assetFS embed.FS

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
	// code adds line numbers and a copy button to code blocks, on every
	// page that runs form (it follows form, which defines wbBindBoard) and
	// inside a topic-body frame.
	code   string
	live   string
	stream string
	// pill is the open-question pill, shared by the full page and the dashboard.
	pill string
	// rail sticks the current topic's bar and shows the jump capsules, on the
	// same two.
	rail string
	// ago keeps the topic bars' updated-ago times current, on the same two.
	ago string
	// mermaidInit themes and runs mermaid after the shared script loads.
	mermaidInit template.JS
	// chg draws a topic body's change marks (wbChangeBars, wbWatchChanges).
	chg string
	// docFrame runs inside a served page's topic-body frame (framedDoc).
	docFrame string
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
// the embedded copy. The daemon sets it at startup; the MCP server never
// does, so the pages it writes keep the embedded assets.
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

// trackerKeyPrefix marks the client strings of the dashboard's tracker
// renderer (tracker.js). A whiteboard page never loads that script, and
// the strings would take a large share of the embed's size budget, so its
// client table leaves them out.
const trackerKeyPrefix = "js.tk."

// codeKeyPrefix marks the copy button's client strings (code.js), the only
// ones a topic-body frame's script reads.
const codeKeyPrefix = "js.wb.copy"

// buildAssets reads every asset by file name through read and parses the
// template set.
func buildAssets(read func(name string) ([]byte, error)) (*pageAssets, error) {
	var err error
	get := func(name string) string {
		if err != nil {
			return ""
		}
		var data []byte
		data, err = read(name)
		return string(data)
	}
	svgView := get("svg-view.js")
	chg := get("chg.js")
	code := get("code.js")
	a := &pageAssets{
		css:         template.CSS(get("page.css")),
		answers:     get("answers.js"),
		form:        get("form.js"),
		code:        code,
		live:        get("live.js"),
		stream:      get("stream.js"),
		pill:        get("pill.js"),
		rail:        get("rail.js"),
		ago:         get("ago.js"),
		mermaidInit: template.JS(get("mermaid-init.js")),
		docFrame:    svgView + "\n" + chg + "\n" + code + "\n" + get("doc-frame.js"),
		chg:         chg,
		viewer:      "(function () {\n" + svgView + "\n" + get("viewer.js") + "})();\n",
		sprite:      template.HTML(strings.TrimSpace(get("sprite.svg"))),
		icons:       map[string]bool{},
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
			"icon": func(name string) (template.HTML, error) {
				if !a.icons[name] {
					return "", fmt.Errorf("whiteboard page: no sprite symbol %q", name)
				}
				return template.HTML(`<svg class="i" aria-hidden="true"><use href="#i-` + name + `"/></svg>`), nil
			},
			"lines":       rationaleLines,
			"box":         rowBox,
			"href":        rowHref,
			"actionAttrs": panelActionAttrs,
			"t":           func(key string, args ...any) string { return wsi18n.T(lang, key, args...) },
			"i18n":        func() template.JS { return wsi18n.ScriptExcept(lang, trackerKeyPrefix) },
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

// RenderPage renders view as one self-contained HTML document: the full page
// for LayoutFull, the embed snapshot for LayoutEmbed. Item text renders as
// markdown with raw HTML escaped (markdown.go); a topic body renders as
// markdown with its raw HTML written as is (pageData.renderDoc).
func RenderPage(view View) (string, error) {
	a, err := loadAssets()
	if err != nil {
		return "", fmt.Errorf("whiteboard page assets: %w", err)
	}
	data := newPageData(view, a, false)
	var buf bytes.Buffer
	if err := a.templateFor(view.Lang).ExecuteTemplate(&buf, "page", data); err != nil {
		return "", fmt.Errorf("whiteboard page: %w", err)
	}
	return buf.String(), nil
}

// Regions are a full page's board and panels rendered alone, by the same
// templates RenderPage uses, so a page that lays them out itself (the
// dashboard daemon's session page) shows exactly what the static page shows.
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

// RenderRegions renders view's board and panels as fragments, as the full
// page shows them, with one difference: each topic body renders inside its
// own sandboxed frame (framedDoc), since the page that hosts the fragments is
// served from the dashboard daemon's origin, and a body's raw HTML and
// scripts must not run there. Build view with LayoutFull.
func RenderRegions(view View) (Regions, error) {
	a, err := loadAssets()
	if err != nil {
		return Regions{}, fmt.Errorf("whiteboard page assets: %w", err)
	}
	view.Layout = LayoutFull
	view.StampFile = ""
	data := newPageData(view, a, true)
	var board, panels bytes.Buffer
	tmpl := a.templateFor(view.Lang)
	if err := tmpl.ExecuteTemplate(&board, "board", data); err != nil {
		return Regions{}, fmt.Errorf("whiteboard board: %w", err)
	}
	for _, panel := range view.Panels {
		if err := tmpl.ExecuteTemplate(&panels, "panel", data.PanelOf(panel)); err != nil {
			return Regions{}, fmt.Errorf("whiteboard panel %s: %w", panel.ID, err)
		}
	}
	return Regions{
		Board:       template.HTML(board.String()),
		Panels:      template.HTML(panels.String()),
		Mermaid:     data.Mermaid,
		CountPolicy: data.CountPolicy,
		CountAcks:   data.CountAcks,
	}, nil
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
	view.StampFile = ""
	var buf bytes.Buffer
	if err := a.templateFor(view.Lang).ExecuteTemplate(&buf, "closedpage", newPageData(view, a, true)); err != nil {
		return "", fmt.Errorf("whiteboard closed topics: %w", err)
	}
	return template.HTML(buf.String()), nil
}

// SharedAssets are the static page's inlined assets, for a page that hosts
// RenderRegions fragments: the stylesheet the fragments are styled by, the
// icon sprite they reference, the answer-form script that drives the board's
// reply bar (a function body: run it inside a function scope, as RenderPage
// does), and the mermaid theme script that runs after MermaidAsset.
type SharedAssets struct {
	CSS        template.CSS
	Sprite     template.HTML
	FormScript template.JS
	// AnswersScript defines window.wbAnswers, the answer-block builder
	// FormScript calls; a self-contained script for the page to inline once,
	// ahead of the first FormScript run.
	AnswersScript template.JS
	MermaidInit   template.JS
	// ViewerScript is the image viewer, a self-contained script for the page
	// to inline once, outside any region (it listens on the document).
	ViewerScript template.JS
}

// PageAssets returns the static page's shared assets.
func PageAssets() (SharedAssets, error) {
	a, err := loadAssets()
	if err != nil {
		return SharedAssets{}, fmt.Errorf("whiteboard page assets: %w", err)
	}
	return SharedAssets{CSS: a.css, Sprite: a.sprite, FormScript: template.JS(a.form + "\n" + a.code + "\n" + a.pill + "\n" + a.rail + "\n" + a.ago + "\n" + a.stream), AnswersScript: template.JS(a.answers), MermaidInit: a.mermaidInit, ViewerScript: template.JS(a.viewer)}, nil
}

// pageData is what the page template executes on: the view plus the inlined
// assets and the board-wide counts the header shows.
type pageData struct {
	View
	Full, Embed bool
	CSS         template.CSS
	Script      template.JS
	Sprite      template.HTML
	// docs holds each topic body rendered by renderDoc on every page render,
	// or, with DocFramed, the sandboxed frame that holds that rendering,
	// keyed by topic name.
	docs    map[string]template.HTML
	entries map[string]template.HTML
	// DocFramed renders each topic body inside a sandboxed frame (framedDoc).
	DocFramed bool
	// tmpl is the template set the page renders with (templateFor), which
	// TopicHTML executes the topic template from.
	tmpl *template.Template
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

	CountPolicy, CountAcks, CountFresh int
	// CountBulk is how many items the bulk checkbox answers.
	CountBulk int

	// Mermaid is set while the template renders a mermaid block, so the
	// page's closing script tags, rendered after every item, load mermaid
	// only on a page that holds one.
	Mermaid     bool
	MermaidInit template.JS
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

// chgWire starts the change marks of every unframed topic body on the page;
// a framed body's own frame script starts its own (doc-frame.js).
const chgWire = "\nArray.prototype.forEach.call(document.querySelectorAll('.doc-body:not(.framed)'), wbWatchChanges);\n"

// BulkChips reports that a topic bar's count chips render as buttons that
// answer every item they count (form.js, button[data-bulk]): only on the
// dashboard's board (RenderRegions, the framed render), where a staged answer
// is sent; the static page and the embed keep them plain counts.
func (p *pageData) BulkChips() bool { return p.DocFramed }

// MermaidFile is the shared mermaid script's name, beside the page.
func (p *pageData) MermaidFile() string {
	name, _ := MermaidAsset()
	return name
}

// newPageData builds the data a page renders from; framed renders each topic
// body inside a sandboxed frame (framedDoc).
func newPageData(view View, a *pageAssets, framed bool) *pageData {
	full := view.Layout != LayoutEmbed
	script := a.answers + a.form + a.code
	if full {
		// The viewer is for the standalone page; the embed is a chat
		// snapshot with a size budget and runs only the answer form.
		script = a.viewer + script + a.pill + a.rail + a.ago + a.stream + a.live + a.chg + chgWire
	}
	p := &pageData{
		View:        view,
		Full:        full,
		Embed:       !full,
		CSS:         a.css,
		Script:      template.JS("(function () {\n" + script + "})();\n"),
		Sprite:      a.sprite,
		MermaidInit: a.mermaidInit,
		icons:       a.icons,
		onPage:      map[int]bool{},
		panelOnPage: map[string]bool{},
		docs:        map[string]template.HTML{},
		entries:     map[string]template.HTML{},
		DocFramed:   framed,
		tmpl:        a.templateFor(view.Lang),
	}
	if !full {
		// The embed has no change-signal loader, so it carries no stamp.
		p.StampFile = ""
	}
	for _, t := range view.Topics {
		for _, panel := range t.PanelRecaps {
			p.panelOnPage[panel.PanelID] = true
		}
		p.CountPolicy += t.AskCount()
		p.CountAcks += len(t.Acks)
		p.CountFresh += t.Fresh
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
	// After onPage is complete, so a reference in a body links like one in
	// an item; a mermaid block in an unframed body sets Mermaid before the
	// template runs.
	topics := append(append([]TopicView(nil), view.Topics...), view.DeferredTopics...)
	for _, t := range topics {
		if full {
			pages := append([]EntryPage{t.Stream}, t.Older...)
			for _, page := range pages {
				for _, run := range page.Runs {
					e := run.Entry
					if e.Kind == EntryPanel && e.Text != "" {
						p.entries[e.ID] = p.panelHTML(t.Name, e.ID, e.Text, framed, a)
					}
				}
			}
		}
		if t.Doc == "" || !full {
			continue
		}
		page := p.Mermaid
		p.Mermaid = false
		p.framing, p.inDoc, p.relays = framed, true, nil
		marks := t.DocMarks
		if !full {
			marks = nil // the embed has no script to draw them
		}
		html := p.renderDoc(t.Doc, marks)
		p.framing, p.inDoc = false, false
		if framed {
			// The frame loads mermaid for its own blocks; the page loads it
			// only for a block outside every frame.
			html = p.framedDoc(t.Name, "", html, t.DocStale, p.Mermaid, p.relays, a)
			p.Mermaid = false
		}
		p.Mermaid = p.Mermaid || page
		p.docs[t.Name] = html
	}
	return p
}

// DocHTML is topic's rendered body (see docs).
func (p *pageData) DocHTML(topic string) template.HTML { return p.docs[topic] }

// EntryHTML is a panel's content, isolated just like the topic body.
func (p *pageData) EntryHTML(id string) template.HTML { return p.entries[id] }

func (p *pageData) panelHTML(topic, entry, content string, framed bool, a *pageAssets) template.HTML {
	pageMermaid := p.Mermaid
	p.Mermaid = false
	p.framing, p.inDoc, p.relays = framed, true, nil
	out := p.convertWith(p.rawMarkdown(), content, "entry/"+entry, nil)
	p.framing, p.inDoc = false, false
	if framed {
		out = p.framedDoc(topic, entry, out, false, p.Mermaid, p.relays, a)
		p.Mermaid = false
	}
	p.Mermaid = p.Mermaid || pageMermaid
	return out
}

// frameCSS is what a topic-body frame adds to the page stylesheet: a
// transparent page, so the topic card shows through, that never scrolls, since the host
// sizes the frame to its content.
const frameCSS = "html, body { background: transparent; overflow: hidden; }"

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
// The document carries the page stylesheet, so a body keeps the page's theme
// variables and light/dark switch, and doc-frame.js, which posts the
// content height and hands the host its marked-link and item-reference
// clicks (the host's half lives in the dashboard script). mermaid tells the
// frame to load the shared mermaid script beside the page for its own
// blocks. An image reaches the frame as whatever src ImageSrc (or
// DocImageSrc) gave it: the frame's requests carry no cookie, so a served
// page inlines body images rather than pointing at a route that needs one.
// An image outside the session root is the one exception: inlining it would
// hand its bytes to every viewer, so the body keeps its path text marked
// data-relay (relayImage), and the frame lists those URLs as its own
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
func (p *pageData) framedDoc(topic, entry string, body template.HTML, stale, mermaid bool, relays []string, a *pageAssets) template.HTML {
	title, key, entryAttr := topic, "topic:"+topic, ""
	if entry != "" {
		title, key = topic+"/"+entry, "entry:"+entry
		entryAttr = ` data-entry="` + template.HTMLEscapeString(entry) + `"`
	}
	var doc strings.Builder
	doc.WriteString(frameDocStart + `<head><meta charset="utf-8"><meta name="wb-doc-key" content="` + template.HTMLEscapeString(key) + `"><base target="_blank"><style>`)
	doc.WriteString(string(a.css))
	doc.WriteString("\n" + frameCSS + `</style></head><body class="doc-frame"><div class="doc-body md` + staleClass(stale) + `">`)
	doc.WriteString(string(body))
	// The frame cannot reach the host page's wsT, so it carries its own over
	// the copy button's two strings (code.js), in the page's language.
	doc.WriteString(`</div><script>` + string(wsi18n.SubsetScript(p.Lang, codeKeyPrefix)) + "\n" + a.docFrame + `</script>`)
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

// DocClass is the class list of the topic's body box: framed or md, and
// chg-stale once its change marks are stale (TopicView.DocStale), which
// chg.js dims the bars from.
func (d topicData) DocClass() string {
	class := "md"
	if d.Page.DocFramed {
		class = "framed"
	}
	return class + staleClass(d.DocStale)
}

// TopicOf pairs a topic with the page for the topic template.
func (p *pageData) TopicOf(t TopicView) topicData { return topicData{TopicView: t, Page: p} }

// TopicHTML renders t's topic section. On the dashboard's board (DocFramed)
// the section carries data-th, a hash of its own rendered bytes (data-th
// excluded): the board swap (dashboard.js morphBoard) compares it in place of
// the section's whole HTML to keep an unchanged topic's node. Hashing the
// bytes covers everything the HTML compare covered (classes, frame
// documents, language); the theme is pinned after render (ThemeFrames), so
// the hash is theme-free. The section's opening tag holds no raw ">" (its
// attribute values are escaped), so data-th goes before the first one.
func (p *pageData) TopicHTML(t TopicView) (template.HTML, error) {
	var buf bytes.Buffer
	if err := p.tmpl.ExecuteTemplate(&buf, "topic", p.TopicOf(t)); err != nil {
		return "", err
	}
	html := buf.String()
	if !p.DocFramed {
		return template.HTML(html), nil
	}
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
	if t.LatestAgent != nil {
		return p.Inline(t.LatestAgent.Text)
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
}

// ItemOf pairs an item with the page for the item templates.
func (p *pageData) ItemOf(iv ItemView) itemData { return itemData{ItemView: iv, Page: p} }

// Num is the item's number as its label shows it, without the parentheses;
// the reply grammar wraps it back as (nn).
func (d itemData) Num() string { return strings.TrimSuffix(strings.TrimPrefix(d.Label, "("), ")") }

// IsNew reports whether the item carries the embed's new badge.
func (d itemData) IsNew() bool { return d.Page.Embed && d.New }

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

// Expandable reports whether a settled row has more to show than its compact
// text: options or a rationale. Links never count; the compact row prints them.
func (d itemData) Expandable() bool {
	return len(d.Options) > 0 || d.Rationale != ""
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
// html/template rewrites any file: URL to #ZgotmplZ, and an embed written
// outside the repository can only reach a ticket file through one; every
// other scheme keeps the template's normal URL filtering.
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
