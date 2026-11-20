// Command bodytime-hover writes a finite, offline UI fixture. It does not open
// a store, launch a daemon, or assert timestamp provenance: the four view times
// model the already-validated TopicView contract.
package main

import (
	"flag"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsi18n"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

func main() {
	out := flag.String("out", "", "required output directory")
	flag.Parse()
	if *out == "" {
		panic("-out is required; no runtime or server is started by this fixture")
	}
	must(os.MkdirAll(*out, 0755))
	now := time.Date(2026, 11, 14, 13, 0, 0, 0, time.UTC)
	st := &wswhiteboard.State{Seq: 10}
	for _, name := range []string{"minutes", "unknown", "stale-years", "months"} {
		st.Topics = append(st.Topics, wswhiteboard.TopicDoc{
			Name: name, Changed: 1, ChangedAt: now.Add(-time.Minute).Format(time.RFC3339),
			Body: "New paragraph with enough words to wrap across multiple lines on a narrow screen, keeping the text column unchanged.\n\n" +
				"Modified paragraph with a [stable anchor](#probe-anchor-" + name + ") and more multiline content for geometry comparison.\n\n" +
				"<span id=\"probe-anchor-" + name + "\">Last paragraph</span> with both a deletion before it and the final deletion tick after it.",
			Marks: []wswhiteboard.BlockMark{{Block: 0, Kind: wswhiteboard.MarkNew}, {Block: 1, Kind: wswhiteboard.MarkMod},
				{Block: 2, Kind: wswhiteboard.MarkDel}, {Block: 3, Kind: wswhiteboard.MarkDel}},
			Entries: []wswhiteboard.Entry{{ID: "log-" + name, Kind: wswhiteboard.EntryAgent,
				At: now.Add(-3 * time.Hour).Format(time.RFC3339), Text: "Multiline recap and log text stays inline after its author and timestamp, taking the available width rather than a middle time column.\nSecond line remains readable."}},
		})
	}
	view := wswhiteboard.BuildView(st, wswhiteboard.ViewInput{Layout: wswhiteboard.LayoutFull, ReplyID: "bodytime-fixture"})
	view.Lang = "ko"
	ages := map[string]time.Duration{"minutes": 5 * time.Minute, "stale-years": 730 * 24 * time.Hour, "months": 60 * 24 * time.Hour}
	for i := range view.Topics {
		topic := &view.Topics[i]
		if age, ok := ages[topic.Name]; ok {
			topic.BodyUpdatedAt = now.Add(-age).Format(time.RFC3339)
		}
		topic.DocStale = topic.Name == "stale-years"
	}
	// A fixed clock makes host and opaque frames independent of execution date.
	// Insert before every shipped script; this is fixture-only, not product JS.
	clock := `<script>Date.now = function () { return ` + fmt.Sprint(now.UnixMilli()) + `; };</script>`
	regions, err := wswhiteboard.RenderRegions(view)
	must(err)
	assets, err := wswhiteboard.PageAssets()
	must(err)
	board := string(regions.Board)
	// Clock injection stays inside each srcdoc's head and preserves the exact
	// sandbox tokens emitted by the renderer.
	board = strings.ReplaceAll(board, "&lt;head&gt;", "&lt;head&gt;"+template.HTMLEscapeString(clock))
	framed := `<!DOCTYPE html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">` + clock +
		`<style>` + string(assets.CSS) + `</style></head><body class="full"><button id="probe-focus">Keep focus</button>` + string(assets.Sprite) +
		`<div class="wrap">` + board + `</div><script>` + string(wsi18n.Script("ko")) + `</script><script>` + string(assets.AnswersScript) +
		`</script><script>(function () {` + string(assets.FormScript) + `})();</script><script>` + mountFrames + `</script></body></html>`
	must(os.WriteFile(filepath.Join(*out, "framed.html"), []byte(framed), 0644))
	for _, name := range regexp.MustCompile(`wb-frame-[a-f0-9]{12}\.(?:css|js)`).FindAllString(board, -1) {
		data, _, ok := wswhiteboard.FrameAsset(name)
		if !ok {
			panic("missing shared frame asset " + name)
		}
		must(os.WriteFile(filepath.Join(*out, name), data, 0644))
	}
	fmt.Println("wrote framed.html and hashed frame assets to", *out)
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

// This minimal fixture mount consumes the emitted placeholder contract. It is
// deliberately not dashboard lifecycle/scroll code and changes no sandbox.
const mountFrames = `
window.addEventListener('message', function (e) {
  if (!e.data || e.data.wbDoc !== 'height') return;
  document.querySelectorAll('iframe.doc-frame').forEach(function (f) {
    if (e.source === f.contentWindow && e.data.docKey === f.dataset.docKey) f.style.height = e.data.height + 'px';
  });
});
document.querySelectorAll('div.doc-frame[data-srcdoc]').forEach(function (p) {
  var f = document.createElement('iframe');
  Array.from(p.attributes).forEach(function (a) { f.setAttribute(a.name, a.value); });
  f.setAttribute('sandbox', p.getAttribute('data-sandbox'));
  f.srcdoc = p.getAttribute('data-srcdoc');
  p.replaceWith(f);
});
`
