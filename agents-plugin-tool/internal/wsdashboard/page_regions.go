// Page regions render session and index templates.

package wsdashboard

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html/template"
	"strings"

	"github.com/kang-sw/devenv/internal/wslive"
)

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
	// LAN is used only by standalone local Settings, never a shared region.
	LAN *LANAccess
	// SysOpen offers the link menu's "open with system" item: the
	// page was asked for by a browser on this machine. It is outside every
	// region, since regions are shared by every viewer's stream.
	SysOpen bool
	// Terminal offers terminals: it is the emulator script's name beside
	// the page (XtermAsset) when the daemon runs terminals and the page
	// was asked for by a browser on this machine, and empty otherwise.
	Terminal string
	// Worktree selects the session-less consumer of the files and card columns.
	Worktree                  string
	Workspace, WorktreeBranch string
	// Frame renders the session page as the iframe shell frames it
	// (?frame=1, shellPageData): the shell's rail and header stand over
	// it, so the page keeps both only as invisible placeholders that lay
	// it out as before. Without it the page stands alone (the tests'
	// renders).
	Frame   bool
	regions regionSet
}

func (p sessionPageData) regionByName(name string) renderedRegion { return p.regions[name] }

func (p sessionPageData) Mode() string {
	if p.Worktree != "" {
		return "worktree"
	}
	return p.SessionView.Mode()
}

// shellPageData is the iframe shell (the "shell" template): the session
// rail and the top header, which stay across navigations, over the frames
// the dashboard's pages load in (shell.js, dashboard.js). RID is the
// session it opens on, empty at the root; Src is the first frame's path,
// Title the tab's title until that frame names one. Its regions are the
// rail and, on a session, the opening session's head; later heads come
// from the frames. Settings and its local-only LAN block are outside those
// regions, so no shared stream ever discloses the token.
type shellPageData struct {
	Lang, LangPref, Theme string
	Assets                pageAssetsData
	SysOpen, OwnHead      bool
	RID, Src, Title       string
	Version               string
	LAN                   *LANAccess
	regions               regionSet
}

func (p shellPageData) regionByName(name string) renderedRegion { return p.regions[name] }

// LoginLocal keeps the Settings action outside shared regions and LAN pages.
func (p shellPageData) LoginLocal() bool   { return p.SysOpen }
func (p sessionPageData) LoginLocal() bool { return p.SysOpen }
func (p indexPageData) LoginLocal() bool   { return p.Local }

type indexPageData struct {
	IndexView
	Assets pageAssetsData
	// LAN is only used if rendering standalone local Settings. The root's
	// framed response gets none; the owning shell has the local-only block.
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
		if name == regionBoard && view.HasBoard && view.BoardUnits != nil {
			regions[name] = view.BoardUnits.rendered(view.Theme).region
			continue
		}
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
