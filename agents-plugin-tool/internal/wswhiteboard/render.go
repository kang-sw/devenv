package wswhiteboard

import (
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
// and the icon sprite. Everything is inlined, so a page needs no other file
// beyond the full page's stamp script.
//
//go:embed assets/page.tmpl assets/page.css assets/form.js assets/live.js assets/sprite.svg
var assetFS embed.FS

type pageAssets struct {
	tmpl   *template.Template
	css    template.CSS
	form   string
	live   string
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
			css:    template.CSS(read("page.css")),
			form:   read("form.js"),
			live:   read("live.js"),
			sprite: template.HTML(strings.TrimSpace(read("sprite.svg"))),
			icons:  map[string]bool{},
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
			"lines":  rationaleLines,
			"marker": rowMarker,
			"href":   rowHref,
		}
		a.tmpl, assetsErr = template.New("page").Funcs(funcs).ParseFS(assetFS, "assets/page.tmpl")
		if assetsErr == nil {
			loadedAssets = a
		}
	})
	return loadedAssets, assetsErr
}

// RenderPage renders view as one self-contained HTML document: the full page
// for LayoutFull, the embed snapshot for LayoutEmbed. Item text is escaped;
// view.Now is written unescaped, as the HTML fragment it is.
func RenderPage(view View) (string, error) {
	a, err := loadAssets()
	if err != nil {
		return "", fmt.Errorf("whiteboard page assets: %w", err)
	}
	data := newPageData(view, a)
	var buf bytes.Buffer
	if err := a.tmpl.ExecuteTemplate(&buf, "page", data); err != nil {
		return "", fmt.Errorf("whiteboard page: %w", err)
	}
	return buf.String(), nil
}

// pageData is what the page template executes on: the view plus the inlined
// assets and the board-wide counts the header shows.
type pageData struct {
	View
	Full, Embed bool
	CSS         template.CSS
	Script      template.JS
	Sprite      template.HTML
	NowHTML     template.HTML

	CountPolicy, CountAssumed, CountAcks, CountFresh int
	// CountBulk is how many items the bulk checkbox answers.
	CountBulk int

	icons map[string]bool
	// onPage holds every item ID the page renders, the targets an in-page
	// link may point at.
	onPage map[int]bool
}

func newPageData(view View, a *pageAssets) *pageData {
	full := view.Layout != LayoutEmbed
	script := a.form
	if full {
		script += a.live
	}
	p := &pageData{
		View:    view,
		Full:    full,
		Embed:   !full,
		CSS:     a.css,
		Script:  template.JS("(function () {\n" + script + "})();\n"),
		Sprite:  a.sprite,
		NowHTML: template.HTML(view.Now),
		icons:   a.icons,
		onPage:  map[int]bool{},
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
	return p
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

// rationaleLines splits a rationale into its non-blank lines, one context
// bullet each.
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

// rowMarker is the checklist box a panel row's state shows.
func rowMarker(state string) string {
	switch state {
	case "done":
		return "[x]"
	case "wip":
		return "[~]"
	case "defer":
		return "[>]"
	default:
		return "[ ]"
	}
}
