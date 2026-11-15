package wsdashboard

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsi18n"
	"github.com/kang-sw/devenv/internal/wsinbox"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// WS_COMPOSER_CHIP_FIXTURE=/absolute/page.html retains an offline visual
// handoff page. Generation launches no server, browser, daemon or provider.
// The page reuses production rendering and the exact chip/navigation functions;
// transport, draft persistence and unrelated dashboard controls are absent.
func TestComposerChipHandoffFixture(t *testing.T) {
	state := &wswhiteboard.State{}
	_, problems := wswhiteboard.Apply(state, `<wb:topic name="handoff" label="Chip navigation"><wb:log>Immutable entry target: the entry chip must reveal this exact line.</wb:log><wb:add class="policy"><wb:title>Item target: choose a review outcome</wb:title><wb:option>Approve</wb:option><wb:option>Revisit</wb:option></wb:add></wb:topic>`, wswhiteboard.Env{Now: testNow})
	if len(problems) != 0 {
		t.Fatal(problems)
	}
	entry, ok := state.LatestAgent("handoff")
	if !ok || len(state.Items) != 1 {
		t.Fatal("missing canonical fixture targets")
	}
	board, err := wswhiteboard.RenderRegions(wswhiteboard.BuildView(state, wswhiteboard.ViewInput{Layout: wswhiteboard.LayoutFull}))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`id="topic-68616e646f6666"`, `id="item-1"`, `id="` + entry.ID + `"`, `data-entry="` + entry.ID + `"`} {
		if !strings.Contains(string(board.Board), want) {
			t.Fatalf("production board lacks canonical target %s", want)
		}
	}
	note := strings.Repeat("Full original history note 한글 — keep every word. ", 30) + "\nFinal line: \"quoted\" <tag> & apostrophe ' end."
	answers, _, ok := wsinbox.ParseAnswerBlock(wsinbox.AnswerBlock([]wsinbox.Chip{{ID: "1", Answer: "ok", Note: note}}, []wsinbox.TopicNote{{Topic: "handoff", Note: note}}))
	if !ok {
		t.Fatal("invalid fixture answers")
	}
	history := renderNamed(t, "userentry", ConvEntry{Answers: answers})
	mentions, err := json.Marshal([]wsinbox.Mention{
		{Topic: "handoff", ID: "01", Text: "Item target: choose a review outcome"},
		{Topic: "handoff", ID: "01", Entry: entry.ID, Text: "Immutable entry target (entry takes precedence over item)"},
		{Topic: "handoff", Text: "Whole topic target"},
	})
	if err != nil {
		t.Fatal(err)
	}
	assets, err := loadDashboardAssets()
	if err != nil {
		t.Fatal(err)
	}
	d := assets.data
	js := string(mustAsset(t, "dashboard.js"))
	remove := regexp.MustCompile(`    u = t.closest\("\[data-unmchip\]"\);\n    if \(u\) \{[^\n]+\}`).FindString(js)
	if remove == "" {
		t.Fatal("missing production mention removal action")
	}
	functions := dashboardFuncs(t, "region", "boardBoxes", "boardQ", "inBoard", "effectTopic", "effectJump", "renderChips", "renderSelectionChips", "selectionNote", "editSelection", "dropSelection", "validMentions", "hideMention", "placeMention", "lockMention", "addMention")
	page := `<!doctype html><html lang="en" data-theme="light" data-rid="chip-fixture"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<meta http-equiv="Content-Security-Policy" content="default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src data:; font-src data:">
<title>Composer chips and bundle memo popover — source fixture</title><style>` + string(d.WhiteboardCSS) + string(d.CSS) + string(d.TrackerCSS) + `
body { margin: 0; padding: 24px; background: var(--wb-bg); color: var(--wb-fg); font: 14px/1.5 system-ui; }
.handoff { max-width: 1280px; margin: auto; }
.handoff-grid { display: grid; grid-template-columns: minmax(0,1fr) minmax(0,1fr); gap: 24px; }
#bcol { height: 65vh; overflow: auto; border: 1px solid var(--wb-line); border-radius: 12px; padding: 12px; }
#r-board { padding-top: 180px; }
#cchips { display: flex; flex-wrap: wrap; gap: 8px; padding: 12px 0; }
.handoff-composer { width: 100%; box-sizing: border-box; padding: 8px; color: var(--wb-fg); background: var(--wb-card); border: 1px solid var(--wb-line); border-radius: 8px; }
.handoff-history { margin-top: 24px; }
.handoff .hchips { display: flex; flex-wrap: wrap; gap: 8px; }
.handoff .boardbar, .handoff .t-close-request { display: none; }
</style></head><body>` + string(d.WhiteboardSprite) + string(d.Sprite) + string(d.LucideSprite) + `
<main class="handoff"><h1>Composer chips · source-built offline fixture</h1>
<p>The first three mention chips reveal and briefly highlight their item, immutable entry, or topic on the left. Selection bundles below open their memo instead. Only a small right-hand × removes a chip. Reload resets the fixture.</p>
<div class="handoff-grid"><section><h2>Selected session board</h2><div id="bcol" class="bcol"><div id="r-board">` + string(board.Board) + `</div></div></section>
<section><h2>Composer mentions and selection bundles</h2><p>The first three chips navigate. Each selection bundle is one compact chip: its body opens a memo popover; Escape or blur saves. Only × deletes. A pen marks a nonempty memo. Try clearing a memo or switching between chips.</p><div id="cchips" class="cchips"></div>
<textarea id="ctext" class="handoff-composer" rows="2" aria-label="Fixture composer">Ordinary composer text stays unchanged by bundle edits.</textarea>
<div class="handoff-history"><h2>Compact history answer chips</h2><p>Hover the bold ID/topic or the clipped note: each whole chip has the same full original multiline title. Inspect its title attribute to compare the complete text (native tooltip display is browser-owned).</p>` + history + `</div>
<p><small>Real renderer, CSS, chip controls, ID navigation, card disclosure and history loader. No session transport, draft persistence, network access or shared runtime. Unrelated controls are outside this fixture's acceptance scope.</small></p></section></div></main>
<script>` + string(wsi18n.Script(wsi18n.Default)) + `</script><script>` + string(d.MentionScript) + `</script><script>` + string(d.AnswersScript) + `</script><script>(function () {` + string(d.FormScript) + `})();</script><script>` + string(d.CardsScript) + `</script>
<script>(function () {
var doc=document,root=doc.documentElement,grid=null,tkApi=null,effectJumpEpoch=0;
function q(s,el){return Array.prototype.slice.call((el||doc).querySelectorAll(s));}
function $(id){return doc.getElementById(id);}
var cchips=$('cchips'),ctext=$('ctext'),closeRequests=[],wpicks=[],wconsoles=[],bmentions=` + string(mentions) + `;
var rid='chip-fixture',mbtn=null,mnote=null,mimgs=null,mpend=null,mcomposing=false;
var bselections=[
{mentions:[{topic:'handoff',id:'1089',entry:'e7',text:'Selection with no memo yet'}],comment:'',images:[]},
{mentions:[{topic:'handoff',entry:'e8',text:'A second captured quote with its own saved memo'}],comment:'Saved memo 한글\nSecond line stays intact.',images:[]},
{mentions:[{topic:'handoff',entry:'e9',text:'First source in a multi-entry push'},{topic:'handoff',entry:'e10',text:'Second source in the same push'}],comment:'One memo for both sources.',images:[]}
];
function queueDraft(){}
function capsuleTopics(q,done){done([]);}
` + selectionImagesSource(t) + `
function chipList(){return [];}
function topicNotes(){return [];}
` + functions + `
window.WsCards.mount(region('board'),{key:'chip-fixture',column:$('bcol'),load:function(){return null;},save:function(){}});
doc.addEventListener('click',function(e){var t=e.target,u;if(!t||!t.closest)return;
` + remove + `
u=t.closest('[data-unselection]');if(u){dropSelection(+u.getAttribute('data-unselection'));return;}
u=t.closest('[data-selection-edit]');if(u){editSelection(+u.getAttribute('data-selection-edit'));return;}
});
renderChips();root.setAttribute('data-fixture-ready','true');
})();</script></body></html>`
	if strings.Contains(page, `<script src=`) || strings.Contains(page, `<link `) {
		t.Fatal("fixture must be self-contained")
	}
	encoded, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	runNode(t, "offline fixture script syntax", `const vm=require('node:vm'); const page=`+string(encoded)+`; for(const m of page.matchAll(/<script>([\s\S]*?)<\/script>/g)) new vm.Script(m[1]);`)
	if path := os.Getenv("WS_COMPOSER_CHIP_FIXTURE"); path != "" {
		if !filepath.IsAbs(path) || filepath.Ext(path) != ".html" {
			t.Fatal("WS_COMPOSER_CHIP_FIXTURE must name an absolute .html file")
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(page), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Logf("Offline handoff page: %s (open directly; no server needed)", path)
	}
}
