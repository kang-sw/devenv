package wsdashboard

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"html/template"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

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
const (
	regionHead         = "head"
	regionLead         = "lead"
	regionTree         = "tree"
	regionConversation = "conversation"
	regionBoard        = "board"
	regionPanels       = "panels"
	regionSessions     = "sessions"
	// regionMode is a pseudo-region with no element: its value is the page
	// mode, and a change tells the page that its layout no longer fits.
	regionMode = "mode"
)

// sessionRegions lists the regions a session page in mode holds.
func sessionRegions(mode string) []string {
	if mode == modeLive {
		return []string{regionHead, regionLead, regionTree, regionConversation, regionBoard, regionPanels}
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
	Assets  pageAssetsData
	regions regionSet
}

func (p sessionPageData) regionByName(name string) renderedRegion { return p.regions[name] }

type indexPageData struct {
	IndexView
	Assets  pageAssetsData
	regions regionSet
}

func (p indexPageData) regionByName(name string) renderedRegion { return p.regions[name] }

// renderSessionRegions renders every region of a session page.
func renderSessionRegions(view SessionView) (regionSet, error) {
	a, err := loadDashboardAssets()
	if err != nil {
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

// renderSessionPage renders the whole session page around its regions.
func renderSessionPage(view SessionView, regions regionSet) (string, error) {
	a, err := loadDashboardAssets()
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	err = a.tmpl.ExecuteTemplate(&buf, "session", sessionPageData{SessionView: view, Assets: a.data, regions: regions})
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

func renderIndexPage(view IndexView, regions regionSet) (string, error) {
	a, err := loadDashboardAssets()
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	err = a.tmpl.ExecuteTemplate(&buf, "index", indexPageData{IndexView: view, Assets: a.data, regions: regions})
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

// session builds one session page's view; false when the daemon does not
// serve that reply-id.
func (r pageReader) session(replyID string) (SessionView, bool) {
	if r.index == nil || !ValidReplyID(replyID) {
		return SessionView{}, false
	}
	s, ok := r.index.Session(replyID)
	if !ok {
		return SessionView{}, false
	}
	page, ok := r.readPage(s)
	if !ok {
		return SessionView{}, false
	}
	in := pageInputs{Session: s, Page: page, Snapshot: r.readSnapshot(replyID), Now: r.clock(), Version: r.version}
	if adapterPresent(in.Snapshot, in.Now) {
		in.Ring, _ = wslive.ReadRing(wslive.RingDir(SessionDir(r.dir, replyID)))
		snaps := map[string]*wslive.Snapshot{}
		in.Lookup = func(id string) (Session, *wslive.Snapshot, bool) {
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
	return buildSessionView(in), true
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
		if html, err = renderIndexPage(view, regions); err == nil {
			writeHTML(w, html)
			return
		}
	}
	fmt.Fprintf(d.log, "dashboard: render index: %v\n", err)
	http.Error(w, "render failed", http.StatusInternalServerError)
}

// handleSessionRoutes serves everything under /s/: a session page, its SSE
// stream, and the shared assets a page loads by relative path.
func (d *Daemon) handleSessionRoutes(w http.ResponseWriter, r *http.Request) {
	if !readMethod(w, r) {
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/s/")
	if name, _ := wswhiteboard.MermaidAsset(); rest == name {
		serveMermaid(w)
		return
	}
	replyID, sub, _ := strings.Cut(rest, "/")
	switch sub {
	case "":
		d.handleSessionPage(w, r, replyID)
	case "events":
		d.handleSessionEvents(w, r, replyID)
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
	regions, err := renderSessionRegions(view)
	if err == nil {
		var html string
		if html, err = renderSessionPage(view, regions); err == nil {
			writeHTML(w, html)
			return
		}
	}
	fmt.Fprintf(d.log, "dashboard: render session %s: %v\n", replyID, err)
	http.Error(w, "render failed", http.StatusInternalServerError)
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

func writeHTML(w http.ResponseWriter, html string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	_, _ = w.Write([]byte(html))
}
