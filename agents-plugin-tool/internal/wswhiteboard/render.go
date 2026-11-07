package wswhiteboard

import (
	"bufio"
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// The page assets: one template set, the shared stylesheet, the answer-form
// script both layouts run, the change-signal script only the full page runs,
// the mermaid theme script a page with a mermaid block runs, the script a
// served page's Now frame runs, and the icon sprite. Everything is inlined,
// so a page needs no other file beyond the full page's stamp script and,
// when it holds a mermaid block, the shared mermaid script (MermaidAsset).
//
//go:embed assets/page.tmpl assets/page.css assets/form.js assets/live.js assets/pill.js assets/mermaid-init.js assets/now-frame.js assets/sprite.svg
var assetFS embed.FS

type pageAssets struct {
	tmpl *template.Template
	css  template.CSS
	form string
	live string
	// pill is the open-question pill, shared by the full page and the dashboard.
	pill string
	// mermaidInit themes and runs mermaid after the shared script loads.
	mermaidInit template.JS
	// nowFrame runs inside a served page's Now frame (framedNow).
	nowFrame string
	sprite   template.HTML
	icons    map[string]bool
}

var (
	assetsOnce   sync.Once
	loadedAssets *pageAssets
	assetsErr    error

	spriteSymbolPattern = regexp.MustCompile(`<symbol id="i-([a-z0-9-]+)"`)
	// itemRefPattern finds the {{nn}} item references inside lead-authored
	// text. Only digits between the braces make a reference; any other
	// {{...}} text renders verbatim.
	itemRefPattern = regexp.MustCompile(`\{\{([0-9]+)\}\}`)
)

// fallbackIcon is the sprite symbol a panel gets when it names an unknown one.
const fallbackIcon = "list"

func loadAssets() (*pageAssets, error) {
	assetsOnce.Do(func() {
		read := func(name string) string {
			if assetsErr != nil {
				return ""
			}
			data, err := assetFS.ReadFile("assets/" + name)
			if err != nil {
				assetsErr = err
			}
			return string(data)
		}
		a := &pageAssets{
			css:         template.CSS(read("page.css")),
			form:        read("form.js"),
			live:        read("live.js"),
			pill:        read("pill.js"),
			mermaidInit: template.JS(read("mermaid-init.js")),
			nowFrame:    read("now-frame.js"),
			sprite:      template.HTML(strings.TrimSpace(read("sprite.svg"))),
			icons:       map[string]bool{},
		}
		if assetsErr != nil {
			return
		}
		for _, match := range spriteSymbolPattern.FindAllStringSubmatch(string(a.sprite), -1) {
			a.icons[match[1]] = true
		}
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
		}
		a.tmpl, assetsErr = template.New("page").Funcs(funcs).ParseFS(assetFS, "assets/page.tmpl")
		if assetsErr == nil {
			loadedAssets = a
		}
	})
	return loadedAssets, assetsErr
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
// markdown with raw HTML escaped (markdown.go); view.Now renders as markdown
// with its raw HTML written as is (pageData.renderNow).
func RenderPage(view View) (string, error) {
	a, err := loadAssets()
	if err != nil {
		return "", fmt.Errorf("whiteboard page assets: %w", err)
	}
	data := newPageData(view, a, false)
	var buf bytes.Buffer
	if err := a.tmpl.ExecuteTemplate(&buf, "page", data); err != nil {
		return "", fmt.Errorf("whiteboard page: %w", err)
	}
	return buf.String(), nil
}

// Regions are a full page's board and panels rendered alone, by the same
// templates RenderPage uses, so a page that lays them out itself (the
// dashboard daemon's session page) shows exactly what the static page shows.
type Regions struct {
	// Board is the Now slot, the topic blocks, and the reply bar.
	Board template.HTML
	// Panels is every panel, in order.
	Panels template.HTML
	// Mermaid reports that Board holds a mermaid block outside the Now
	// frame, so the page loads the shared mermaid script (MermaidAsset) and
	// MermaidInitScript; the frame loads them itself for a block in Now.
	Mermaid bool
	// CountPolicy, CountAssumed, and CountAcks are the header's counts.
	CountPolicy, CountAssumed, CountAcks int
}

// RenderRegions renders view's board and panels as fragments, as the full
// page shows them, with one difference: the Now slot renders inside a
// sandboxed frame (framedNow), since the page that hosts the fragments is
// served from the dashboard daemon's origin, and Now's raw HTML and scripts
// must not run there. Build view with LayoutFull.
func RenderRegions(view View) (Regions, error) {
	a, err := loadAssets()
	if err != nil {
		return Regions{}, fmt.Errorf("whiteboard page assets: %w", err)
	}
	view.Layout = LayoutFull
	view.StampFile = ""
	data := newPageData(view, a, true)
	var board, panels bytes.Buffer
	if err := a.tmpl.ExecuteTemplate(&board, "board", data); err != nil {
		return Regions{}, fmt.Errorf("whiteboard board: %w", err)
	}
	for _, panel := range view.Panels {
		if err := a.tmpl.ExecuteTemplate(&panels, "panel", data.PanelOf(panel)); err != nil {
			return Regions{}, fmt.Errorf("whiteboard panel %s: %w", panel.ID, err)
		}
	}
	return Regions{
		Board:        template.HTML(board.String()),
		Panels:       template.HTML(panels.String()),
		Mermaid:      data.Mermaid,
		CountPolicy:  data.CountPolicy,
		CountAssumed: data.CountAssumed,
		CountAcks:    data.CountAcks,
	}, nil
}

// SharedAssets are the static page's inlined assets, for a page that hosts
// RenderRegions fragments: the stylesheet the fragments are styled by, the
// icon sprite they reference, the answer-form script that drives the board's
// reply bar (a function body: run it inside a function scope, as RenderPage
// does), and the mermaid theme script that runs after MermaidAsset.
type SharedAssets struct {
	CSS         template.CSS
	Sprite      template.HTML
	FormScript  template.JS
	MermaidInit template.JS
}

// PageAssets returns the static page's shared assets.
func PageAssets() (SharedAssets, error) {
	a, err := loadAssets()
	if err != nil {
		return SharedAssets{}, fmt.Errorf("whiteboard page assets: %w", err)
	}
	return SharedAssets{CSS: a.css, Sprite: a.sprite, FormScript: template.JS(a.form + "\n" + a.pill), MermaidInit: a.mermaidInit}, nil
}

// pageData is what the page template executes on: the view plus the inlined
// assets and the board-wide counts the header shows.
type pageData struct {
	View
	Full, Embed bool
	CSS         template.CSS
	Script      template.JS
	Sprite      template.HTML
	// NowHTML is view.Now rendered by renderNow on every page render, or,
	// with NowFramed, the sandboxed frame that holds that rendering.
	NowHTML template.HTML
	// NowFramed renders Now inside a sandboxed frame (framedNow).
	NowFramed bool
	// framing is set while a framed Now renders, so its file images take
	// NowImageSrc.
	framing bool

	CountPolicy, CountAssumed, CountAcks, CountFresh int
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

	icons map[string]bool
	// onPage holds every item ID the page renders, the targets an in-page
	// link may point at.
	onPage map[int]bool
	md     *markdown
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

// newPageData builds the data a page renders from; framed renders the Now
// slot inside a sandboxed frame (framedNow).
func newPageData(view View, a *pageAssets, framed bool) *pageData {
	full := view.Layout != LayoutEmbed
	script := a.form
	if full {
		script += a.pill + a.live
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
		NowFramed:   framed,
	}
	if !full {
		// The embed has no change-signal loader, so it carries no stamp.
		p.StampFile = ""
	}
	for _, t := range view.Topics {
		p.CountPolicy += len(t.Policy)
		p.CountAssumed += len(t.Assumed)
		p.CountAcks += len(t.Acks)
		p.CountFresh += t.Fresh
		for _, list := range [][]ItemView{t.Policy, t.Assumed, t.Acks, t.Expanded, t.Collapsed} {
			for _, iv := range list {
				p.onPage[iv.ID] = true
			}
		}
	}
	p.CountBulk = p.CountAssumed + p.CountAcks
	// After onPage is complete, so a reference in Now links like one in an
	// item; a mermaid block in Now sets Mermaid before the template runs.
	if view.Now != "" {
		p.framing = framed
		p.NowHTML = p.renderNow(view.Now)
		p.framing = false
		if framed {
			// The frame loads mermaid for its own blocks; the page loads it
			// only for a block outside Now.
			p.NowHTML = p.framedNow(p.NowHTML, p.Mermaid, a)
			p.Mermaid = false
		}
	}
	return p
}

// frameCSS is what a Now frame adds to the page stylesheet: a transparent
// page, so the Now card shows through, that never scrolls, since the host
// sizes the frame to its content.
const frameCSS = "html, body { background: transparent; overflow: hidden; }"

// framedNow wraps a rendered Now in the sandboxed frame a served page shows
// it in. The page lives in the dashboard daemon's origin, whose routes post
// messages and steers to sessions, so Now's raw HTML must not run there:
// the frame's srcdoc document gets allow-scripts, so an inline canvas script
// still draws, but no allow-same-origin, so it runs in an opaque origin that
// can neither read the host page nor reach the daemon's routes with the
// viewer's credentials. allow-popups and allow-popups-to-escape-sandbox let
// a plain link open its new tab (<base target="_blank">) as an ordinary
// page; there is no allow-forms, allow-top-navigation, or allow-modals.
//
// The document carries the page stylesheet, so Now keeps the page's theme
// variables and light/dark switch, and now-frame.js, which posts the
// content height and hands the host its marked-link and item-reference
// clicks (the host's half lives in the dashboard script). mermaid tells the
// frame to load the shared mermaid script beside the page for its own
// blocks. An image reaches the frame as whatever src ImageSrc (or
// NowImageSrc) gave it: the frame's requests carry no cookie, so a served
// page inlines Now images rather than pointing at a route that needs one.
func (p *pageData) framedNow(body template.HTML, mermaid bool, a *pageAssets) template.HTML {
	var doc strings.Builder
	doc.WriteString(`<!DOCTYPE html><html><head><meta charset="utf-8"><base target="_blank"><style>`)
	doc.WriteString(string(a.css))
	doc.WriteString("\n" + frameCSS + `</style></head><body class="now-frame"><div class="now-body md">`)
	doc.WriteString(string(body))
	doc.WriteString(`</div><script>` + a.nowFrame + `</script>`)
	if mermaid {
		doc.WriteString(`<script src="` + template.HTMLEscapeString(p.MermaidFile()) + `"></script><script>wbNowMermaid(function () {` + string(a.mermaidInit) + `});</script>`)
	}
	doc.WriteString(`</body></html>`)
	return template.HTML(`<iframe class="now-frame" title="now" sandbox="allow-scripts allow-popups allow-popups-to-escape-sandbox" srcdoc="` +
		template.HTMLEscapeString(doc.String()) + `"></iframe>`)
}

// topicData is one topic block with the page it sits on.
type topicData struct {
	TopicView
	Page *pageData
}

// TopicOf pairs a topic with the page for the topic template.
func (p *pageData) TopicOf(t TopicView) topicData { return topicData{TopicView: t, Page: p} }

// itemData is one item with the page it sits on.
type itemData struct {
	ItemView
	Page *pageData
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

// PickedOption is the option a settled or assumed answer chose, nil when none.
func (d itemData) PickedOption() *OptionView {
	for i := range d.Options {
		if d.Options[i].Picked {
			return &d.Options[i]
		}
	}
	return nil
}

// Stacked reports whether the item's options render as stacked cards: any
// option holds block markdown.
func (d itemData) Stacked() bool {
	for _, o := range d.Options {
		if d.Page.IsBlock(o.Text) {
			return true
		}
	}
	return false
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
	Text string
	Ref  int
}

// Parts splits lead-authored text at its {{nn}} item references. A reference
// to an item this page renders becomes an in-page anchor labelled (nn); one
// to an item the store holds but the page omits becomes plain (nn); one to an
// ID the store does not hold stays verbatim. References are display only:
// the store never refuses an update over one.
func (p *pageData) Parts(s string) []textPart {
	var parts []textPart
	last := 0
	for _, m := range itemRefPattern.FindAllStringSubmatchIndex(s, -1) {
		id, err := strconv.Atoi(s[m[2]:m[3]])
		if err != nil || !p.Stored[id] {
			continue
		}
		if m[0] > last {
			parts = append(parts, textPart{Text: s[last:m[0]]})
		}
		part := textPart{Text: FormatID(id)}
		if p.onPage[id] {
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
