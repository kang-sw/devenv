package wsdashboard

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
)

// TestFilesTabOnTheSessionPage pins where the file explorer lives: a live
// session page's conversation panel header, between the splitter and the
// conversation, carries the "Conversation | Files" switch, the Files pane
// takes the conversation's place under its tab while the agents tree and
// the composer stay, and the page loads the explorer script after the
// preview kind it opens files in and before the page script that mounts
// it. A board-first page (no conversation column) and the index page carry
// none of it.
func TestFilesTabOnTheSessionPage(t *testing.T) {
	f := startPageFixture(t)
	_, live := get(t, f.base+"/s/livesession?frame=1")
	for _, want := range []string{
		`<div id="chead" class="thead chead"><nav id="ctabs" class="ctabs"`, `data-ctab="convo" aria-pressed="true"`, `data-ctab="files" aria-pressed="false"`,
		`<span class="csum"></span><span class="fsum"></span></div>`,
		`<section id="files" class="fpane"`, "window.WsFiles = {",
	} {
		if !strings.Contains(live, want) {
			t.Errorf("the live session page lacks %q", want)
		}
	}
	if i, j, k := strings.Index(live, "window.WsPreview = {"), strings.Index(live, "window.WsFiles = {"), strings.Index(live, "The dashboard page script"); i < 0 || j < i || k < j {
		t.Error("the explorer script is not between the preview kind and the page script")
	}
	if i, j, k := strings.Index(live, `id="splitter"`), strings.Index(live, `id="ctabs"`), strings.Index(live, `<div id="convo" class="convo">`); i < 0 || j < i || k < j {
		t.Error("the conversation panel's tabs do not head the conversation, below the splitter")
	}
	_, board := get(t, f.base+"/s/boardsession?frame=1")
	if strings.Contains(board, `id="ctabs"`) || strings.Contains(board, `id="files"`) {
		t.Error("the board-first page, which has no conversation column, carries the Files tab")
	}
	if _, index := get(t, f.base+"/?frame=1"); strings.Contains(index, "window.WsFiles = {") {
		t.Error("the index page carries the file explorer")
	}
	css := mustAsset(t, "dashboard.css")
	for _, want := range []string{
		// the Files pane in the conversation's place; the conversation hidden, never removed
		`.dgrid.live > .fpane { grid-area: convo; display: none; }`,
		`.dgrid.live[data-ctab="files"] > .convo { display: none; }`,
		`.dgrid.live[data-layout="split"][data-ctab="files"] > .fpane { display: flex; }`,
		`.dgrid.live:not([data-layout="tabs"])[data-ctab="files"] > .fpane { display: flex; }`,
		// the header shows the conversation's counts or the Files pane's root
		`.dgrid[data-ctab="files"] > .chead .csum, .dgrid:not([data-ctab="files"]) > .chead .fsum { display: none; }`,
		// The independent tab stays mounted but must not paint a second
		// shadow or root label behind the toolbar sheet.
		`body:has(.fpane[data-max]) .dgrid.live > .fpane,`,
		`body:has(.fpane[data-max]) .chead .fsum .froot { visibility: hidden; }`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("the stylesheet lacks %q", want)
		}
	}
}

// TestFilesExplorerInstances executes the complete production explorer with
// controlled promises; unlike helper extraction this covers closure ownership.
func TestFilesExplorerInstances(t *testing.T) {
	cards, _ := json.Marshal(mustAsset(t, "cards.js"))
	files, _ := json.Marshal(mustAsset(t, "files.js"))
	var picker strings.Builder
	for _, name := range []string{"rpWsBrowse", "rpWsMake"} {
		fn := regexp.MustCompile(`(?s)\n  function ` + name + `\(.*?\n  \}\n`).FindString(mustAsset(t, "dashboard.js"))
		if fn == "" {
			t.Fatalf("dashboard.js holds no %s", name)
		}
		picker.WriteString(fn)
	}
	pickerJSON, _ := json.Marshal(picker.String())
	pickerFixture, err := os.ReadFile("testdata/files-workspace-picker.test.cjs")
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := os.ReadFile("testdata/files-explorer.test.cjs")
	if err != nil {
		t.Fatal(err)
	}
	runNode(t, "file explorer instances", "var CARDS = "+string(cards)+", FILES = "+string(files)+", PICKER = "+string(pickerJSON)+";\n"+cardsDOM+string(pickerFixture)+string(fixture))
}

func TestFilesWorkspacePickerShellAssets(t *testing.T) {
	for _, local := range []bool{false, true} {
		page, err := renderShellPage(shellPageData{Lang: "en", SysOpen: local, Src: "/?frame=1"}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Contains(page, "window.WsFiles = {"); got != local {
			t.Errorf("local=%v explorer loaded=%v", local, got)
		}
		if local {
			cards := strings.Index(page, "window.WsCards = {")
			files := strings.Index(page, "window.WsFiles = {")
			script := strings.Index(page, "The dashboard page script")
			if cards < 0 || files < cards || script < files {
				t.Error("shell must load card helpers, explorer, then the popover consumer")
			}
		}
	}
	css := mustAsset(t, "dashboard.css")
	if !strings.Contains(css, ".fpicker .fdot { opacity: .5; }") || !strings.Contains(css, ".fworkspace-picker > .fpane { display: flex; height: 100%; }") {
		t.Error("picker requires translucent hidden directories and a bounded shared pane")
	}
}

func TestFilesExplorerPathBarLayout(t *testing.T) {
	css := mustAsset(t, "dashboard.css")
	for _, want := range []string{
		`.fcrumb-trail[hidden], .fpath[hidden] { display: none; }`,
		`.fpath { flex: 1 1 auto; min-width: 0; width: 0; height: 24px; box-sizing: border-box;`,
		`.fback .li { transform: rotate(180deg); }`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("path bar stylesheet lacks %q", want)
		}
	}
}

func TestFilesExplorerColumnLayout(t *testing.T) {
	css := mustAsset(t, "dashboard.css")
	for _, want := range []string{
		`.fworkspace[data-view="column"] { grid-template-columns: minmax(0, 1fr); }`,
		`.fworkspace[data-view="column"] > .ftree { display: none; }`,
		`.fdirectory[data-view="column"] { display: flex; padding: 0; overflow-x: auto; overflow-y: hidden; }`,
		`.fcolumn { flex: 0 0 224px; box-sizing: border-box; min-width: 0; overflow-y: auto;`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("column view stylesheet lacks %q", want)
		}
	}
}

// TestFilesExplorerRows runs the explorer's pure parts (files.js) under
// node, when it is installed: which entries the View toggles show, the
// rows the tree draws and the keyboard walks (open folders flattened in
// place, a cut folder ending in a "more" row), an entry's absolute path
// from its folder's listing, and the bounded per-root View record.
func TestFilesExplorerRows(t *testing.T) {
	js := mustAsset(t, "files.js")
	var parts strings.Builder
	for _, name := range []string{"showsEntry", "opens", "joinRel", "absPath", "flatRows", "rememberView"} {
		fn := regexp.MustCompile(`(?s)\n  function ` + name + `\(.*?\n  \}\n`).FindString(js)
		if fn == "" {
			t.Fatalf("files.js holds no function %s", name)
		}
		parts.WriteString(fn)
	}
	runNode(t, "file explorer rows", `
var bad = [];
function check(what, got, want) { if (JSON.stringify(got) !== JSON.stringify(want)) bad.push(what + ": got " + JSON.stringify(got) + ", want " + JSON.stringify(want)); }
`+parts.String()+`
function names(list, view) { return list.filter(function (e) { return showsEntry(e, view); }).map(function (e) { return e.name; }); }
var entries = [{ name: ".git", dir: true, ign: true }, { name: "build", dir: true, ign: true }, { name: ".github", dir: true },
  { name: "src", dir: true }, { name: ".env" }, { name: "noise.log", ign: true }, { name: "main.go" }];
check("ignored entries are hidden by default, dot entries shown", names(entries, {}), [".github", "src", ".env", "main.go"]);
check("show ignored", names(entries, { ign: true }), [".git", "build", ".github", "src", ".env", "noise.log", "main.go"]);
check("hide dot entries", names(entries, { dot: true }), ["src", "main.go"]);
check("both", names(entries, { ign: true, dot: true }), ["build", "src", "noise.log", "main.go"]);

var lists = {
  "": { entries: [{ name: "docs", dir: true }, { name: "src", dir: true, st: "*" }, { name: "link", link: true }, { name: "old", dir: true, gone: true, st: "D" }, { name: "a.md" }] },
  "src": { entries: [{ name: "pkg", dir: true }, { name: "main.go", st: "M" }], more: true },
  "src/pkg": { entries: [{ name: "x.go" }] },
  "docs": { entries: [{ name: "guide.md" }] }
};
function walk(open, view) { return flatRows(lists, open, view || {}).map(function (r) { return (r.more ? "more:" + r.dir : r.path) + "@" + r.depth + (r.open ? "+" : ""); }); }
check("a closed tree is the root's entries", walk({}), ["docs@0", "src@0", "link@0", "old@0", "a.md@0"]);
check("open folders flatten in place, a cut folder ends in a more row",
  walk({ src: true, "src/pkg": true }), ["docs@0", "src@0+", "src/pkg@1+", "src/pkg/x.go@2", "src/main.go@1", "more:src@1", "link@0", "old@0", "a.md@0"]);
check("an open folder under a closed one stays out", walk({ "src/pkg": true }), ["docs@0", "src@0", "link@0", "old@0", "a.md@0"]);
check("an open folder not listed yet shows open with no rows",
  flatRows({ "": lists[""] }, { docs: true }, {}).slice(0, 2).map(function (r) { return r.path + (r.open ? "+" : ""); }), ["docs+", "src"]);
check("a symlink and a deleted folder never open", walk({ link: true, old: true }), ["docs@0", "src@0", "link@0", "old@0", "a.md@0"]);
check("the View toggles apply at every depth", walk({ docs: true }, { dot: false, ign: false }).length, 6);
var r = flatRows(lists, { src: true }, {})[2];
check("a row names its folder and entry", [r.path, r.dir, r.e.name, r.e.st], ["src/pkg", "src", "pkg", undefined]);

check("absolute path on a POSIX host", absPath("/home/u/proj/src", "/", "main.go"), "/home/u/proj/src/main.go");
check("absolute path on a Windows host", absPath("C:\\work\\proj", "\\", "a.txt"), "C:\\work\\proj\\a.txt");
check("absolute path under a filesystem root", [absPath("/", "/", "etc"), absPath("C:\\", "\\", "x")], ["/etc", "C:\\x"]);

var m = {};
for (var i = 0; i < 5; i++) m = rememberView(m, "/r" + i, { ign: i % 2 === 0 }, 100 + i, 3);
check("the View record keeps the newest roots", Object.keys(m).sort(), ["/r2", "/r3", "/r4"]);
m = rememberView(m, "/r2", { dot: true }, 200, 3);
m = rememberView(m, "/r5", {}, 201, 3);
check("setting a root again keeps it", Object.keys(m).sort(), ["/r2", "/r4", "/r5"]);
check("with its new toggles", [m["/r2"].ign, m["/r2"].dot], [false, true]);
process.stdout.write(bad.join("\n"));
`)
}
