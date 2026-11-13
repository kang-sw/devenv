package wsdashboard

import (
	"regexp"
	"testing"
)

// noteImagesDOM is a minimal DOM for dashboard.js's composer images section
// with board note pens: elements with classes, attributes, children, and
// sibling insertion; text fields with a caret, setRangeText, and focus that
// a hidden field refuses; a board of one policy card, one off-board slot,
// and one topic note; and the page's board input listener, which a note's
// input event runs (syncChips, then syncMarkers and followImgs).
const noteImagesDOM = `
var bad = [];
function check(what, got, want) { if (JSON.stringify(got) !== JSON.stringify(want)) bad.push(what + ": got " + JSON.stringify(got) + ", want " + JSON.stringify(want)); }
function El(tag, cls) {
  this.tag = tag; this.className = cls || ""; this.children = []; this.attrs = {}; this.parentNode = null; this.hidden = false; this.value = ""; this.style = {}; this.title = ""; this._text = "";
  var self = this;
  this.classList = { contains: function (c) { return self.className.split(" ").indexOf(c) >= 0; } };
}
El.prototype.setAttribute = function (k, v) { this.attrs[k] = String(v); };
El.prototype.getAttribute = function (k) { return Object.prototype.hasOwnProperty.call(this.attrs, k) ? this.attrs[k] : null; };
El.prototype.appendChild = function (c) { c.parentNode = this; this.children.push(c); return c; };
El.prototype.insertBefore = function (c, ref) { var i = ref ? this.children.indexOf(ref) : -1; c.parentNode = this; if (i < 0) this.children.push(c); else this.children.splice(i, 0, c); return c; };
El.prototype.removeChild = function (c) { this.children = this.children.filter(function (x) { return x !== c; }); c.parentNode = null; };
Object.defineProperty(El.prototype, "nextSibling", { get: function () { var p = this.parentNode; return p ? p.children[p.children.indexOf(this) + 1] || null : null; } });
Object.defineProperty(El.prototype, "textContent", {
  get: function () { return this.children.length ? this.children.map(function (c) { return c.textContent; }).join("") : this._text; },
  set: function (v) { this.children = []; this._text = String(v); }
});
El.prototype.setSelectionRange = function (a, b) { this.selectionStart = a; this.selectionEnd = b; };
El.prototype.setRangeText = function (s, a, b) { this.value = this.value.slice(0, a) + s + this.value.slice(b); this.selectionStart = this.selectionEnd = a + s.length; };
El.prototype.focus = function () { if (!this.hidden) document.activeElement = this; };
El.prototype.dispatchEvent = function (e) { if (this === ctext) syncMarkers(); else boardInput(); };
function Event(type) { this.type = type; }
function walk(n, fn) { n.children.forEach(function (c) { fn(c); walk(c, fn); }); }
var document = { activeElement: null, createElement: function (tag) { return new El(tag); }, execCommand: function () { return false; } }, doc = document;
var URL = { createObjectURL: function (f) { return "blob:" + f.name; }, revokeObjectURL: function () {} };
function FileReader() {}
FileReader.prototype.readAsDataURL = function (f) { this.result = "data:" + f.type + ";base64,QUJD"; this.onload(); };
function file(name) { return { name: name, type: "image/png", size: 3 }; }
function wsT(k, a) { return a === undefined ? k : k + ":" + a; }
var warns = [];
function noteWarn(t) { warns.push(t); }
var rid = "s1";

var ctext = new El("textarea"), cimgs = new El("div", "cimgs"), board = new El("div");
ctext.selectionStart = ctext.selectionEnd = 0;
var card = new El("div", "card policy"), cbody = new El("div", "cbody"), note3 = new El("textarea", "note");
card.setAttribute("data-id", "3");
card.appendChild(cbody); cbody.appendChild(note3);
var topic = new El("section", "topic"), tnote = new El("textarea", "note tnote");
topic.setAttribute("data-topic", "plan");
topic.appendChild(new El("div", "tbar")); topic.appendChild(tnote);
board.appendChild(topic); board.appendChild(card);
[note3, tnote].forEach(function (n) { n.selectionStart = n.selectionEnd = 0; });
function $(id) { return id === "cimgs" ? cimgs : null; }
function boardQ(sel) {
  var out = [];
  walk(board, function (c) {
    if (sel === "textarea.note" ? c.tag === "textarea" && c.classList.contains("note") : sel === ".nimgs" && c.classList.contains("nimgs")) out.push(c);
  });
  return out;
}
function inBoard(n) { for (; n; n = n.parentNode) if (n === board) return true; return false; }
var chips = {}, chipOrder = [];
function chipList() { return chipOrder.map(function (id) { return chips[id]; }); }
function putChip(c) { if (!chips[c.id]) chipOrder.push(c.id); chips[c.id] = c; }
function dropChip(id) { delete chips[id]; chipOrder = chipOrder.filter(function (x) { return x !== id; }); }
function boardItem(id) { var f = null; walk(board, function (c) { if (c.getAttribute("data-id") === id) f = c; }); return f; }
// syncChips reads each on-board item's note into its chip, as the page's does.
function syncChips() {
  walk(board, function (c) {
    var id = c.getAttribute("data-id");
    if (!id) return;
    var n = null; walk(c, function (x) { if (x.tag === "textarea") n = x; });
    if (n && n.value.trim()) putChip({ id: id, answer: "", note: n.value.trim() }); else if (chips[id] && !chips[id].answer) dropChip(id);
  });
}
function renderChips() {}
function boardInput() { syncChips(); syncMarkers(); followImgs(); }
// setItem and restoreTopics put a restored note on the board through the
// input path, as the page's do.
function setItem(id, c) { var item = boardItem(id); if (!item) return; var n = null; walk(item, function (x) { if (x.tag === "textarea") n = x; }); n.value = c && c.note ? c.note : ""; n.dispatchEvent(new Event("change")); }
function restoreTopics(list) {
  var stray = [];
  list.forEach(function (tn) { if (tn.topic !== "plan") { stray.push("[" + tn.topic + "] " + tn.note); return; } tnote.value = tn.note; tnote.dispatchEvent(new Event("input")); });
  return stray;
}
var cmdSel = null, bmentions = [];
function grow() {} function shiftMentions() {} function clearMentions() {} function setCommand() {}
function validMentions() { return []; }
function attachSrc(n) { return "/a/" + n; }
// rowNums is the image numbers shown in a chip row; noteRow the row after a note.
function rowNums(row) { return row ? row.children.map(function (c) { return c.children[1].textContent; }) : null; }
function noteRow(n) { var r = n.nextSibling; return r && r.classList.contains("nimgs") ? r : null; }
`

// TestBoardNoteImages runs dashboard.js's composer images section and its
// restoreHeld under node with board note pens: an image pasted into an item
// note or a topic note puts its marker at that note's caret and its chip in
// a row after that note; one number sequence spans the composer and every
// note; a composer keystroke keeps the notes' images; deleting a note's
// marker drops its image and undoing it brings it back; the x on a note's
// chip removes that note's marker; a note whose item leaves the board keeps
// its images through the answer chip, shown in the composer's row; a board
// swap that rebuilds the notes gets their rows back; a cancel restore
// renumbers the held message's markers in its text, chip notes, and topic
// notes without repeating the note markers in the composer, and gives an
// image no restored text names a composer marker. The cases execute under
// node when it is installed.
func TestBoardNoteImages(t *testing.T) {
	data, err := dashboardFS.ReadFile("assets/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	section := regexp.MustCompile(`(?s)\n  // --- composer images -.*?\n  // --- composer file mentions`).FindString(js)
	if section == "" {
		t.Fatal("dashboard.js holds no composer images section before the file mentions one")
	}
	restore := composerFunc(t, js, "restoreHeld") + composerFunc(t, js, "restoreHeldNow")
	runNode(t, "board note images", noteImagesDOM+section+restore+`
// one sequence across the composer and the notes
ctext.value = "see "; ctext.selectionStart = ctext.selectionEnd = 4;
addFiles([file("a.png")]);
note3.value = "why"; note3.selectionStart = note3.selectionEnd = 3;
addFiles([file("b.png"), file("c.png")], note3);
addFiles([file("d.png")], tnote);
check("composer and note markers", [ctext.value, note3.value, tnote.value], ["see [Image #1] ", "why [Image #2] [Image #3] ", "[Image #4] "]);
check("one image list numbered across them", imgs.map(function (p) { return [p.n, p.name, p.data !== null]; }), [[1, "a.png", true], [2, "b.png", true], [3, "c.png", true], [4, "d.png", true]]);
check("chips sit with their fields", [rowNums(cimgs), rowNums(noteRow(note3)), rowNums(noteRow(tnote))], [["#1"], ["#2", "#3"], ["#4"]]);
check("the note chips index the one list", noteRow(note3).children.map(function (c) { return c.children[2].getAttribute("data-unimg"); }), ["1", "2"]);

// a composer keystroke keeps the notes' images
ctext.value = "see [Image #1] now"; ctext.dispatchEvent(new Event("input"));
check("a composer edit keeps every image", imgs.map(function (p) { return p.n; }), [1, 2, 3, 4]);

// a note's marker deleted drops its image; undo brings it back
note3.value = "why [Image #2] "; note3.dispatchEvent(new Event("input"));
check("a deleted note marker drops its image", [imgs.map(function (p) { return p.n; }), rowNums(noteRow(note3))], [[1, 2, 4], ["#2"]]);
note3.value = "why [Image #2] [Image #3] "; note3.dispatchEvent(new Event("input"));
check("undo brings it back", [imgs.map(function (p) { return p.n; }), rowNums(noteRow(note3))], [[1, 2, 3, 4], ["#2", "#3"]]);

// the next image numbers past every note's markers
addFiles([file("e.png")]);
check("the next number follows the notes", imgs.map(function (p) { return p.n; }), [1, 2, 3, 4, 5]);
unattach(imgs[4]);

// the x on a note chip removes that note's marker
unattach(imgs[2]);
check("x removes the note's marker", [note3.value, imgs.map(function (p) { return p.n; }), rowNums(noteRow(note3))], ["why [Image #2] ", [1, 2, 4], ["#2"]]);
// a closed note takes the edit without the focus
document.activeElement = ctext; tnote.hidden = true;
unattach(imgs[2]);
check("a closed note's marker goes, the focus stays", [tnote.value, document.activeElement === ctext, noteRow(tnote)], ["", true, null]);
tnote.hidden = false;

// a board swap that rebuilds the notes gets their rows back
var row = noteRow(note3);
cbody.removeChild(row);
check("the swap took the row", noteRow(note3), null);
syncMarkers(); renderImgs();
check("the rows come back after the swap", rowNums(noteRow(note3)), ["#2"]);

// an item that leaves the board keeps its images through its chip
board.removeChild(card);
syncMarkers(); renderImgs();
check("a stray chip's image stays and shows in the composer row", [imgs.map(function (p) { return p.n; }), rowNums(cimgs)], [[1, 2], ["#1", "#2"]]);
unattach(imgs[1]);
check("its x strips the stray chip's note", [chips["3"] ? chips["3"].note : null, imgs.map(function (p) { return p.n; })], ["why", [1]]);
board.appendChild(card);

// a cancel restore renumbers text, chip, and topic markers once
imgs = []; imgTrash = {}; imgSeq = 0; chips = {}; chipOrder = []; cimgs.textContent = "";
var card5 = new El("div", "arow"), note5 = new El("textarea", "note");
card5.setAttribute("data-id", "5"); card5.appendChild(note5); board.appendChild(card5);
imgs.push({ name: "x.png", url: "u1", data: "QQ", ref: "", n: 1 }, { name: "y.png", url: "u2", data: "QQ", ref: "", n: 2 });
ctext.value = "draft [Image #1]";
note3.value = "mine [Image #2]"; syncChips();
restoreHeld([{ text: "held [Image #1]", images: ["h1.png", "h2.png", "h3.png"], nums: [1, 2, 3],
  chips: [{ id: "5", answer: "", note: "see [Image #2]" }], topics: [{ topic: "plan", note: "look [Image #3]" }] }]);
check("restored text and notes, renumbered", [ctext.value, note5.value, tnote.value], ["held [Image #3]\n\ndraft [Image #1]", "see [Image #4]", "look [Image #5]"]);
check("restored images", imgs.map(function (p) { return [p.n, p.ref]; }), [[1, ""], [2, ""], [3, "h1.png"], [4, "h2.png"], [5, "h3.png"]]);
check("chips back with their notes", [rowNums(cimgs), rowNums(noteRow(note3)), rowNums(noteRow(note5)), rowNums(noteRow(tnote))], [["#1", "#3"], ["#2"], ["#4"], ["#5"]]);
check("the next number follows the restore", imgSeq, 5);

// an image no restored text names gets a composer marker
// (the user rewrote note 5 and the topic note, dropping images 4 and 5)
tnote.value = "keep"; note5.value = "newer"; tnote.dispatchEvent(new Event("input"));
check("rewritten notes drop their images", imgs.map(function (p) { return p.n; }), [1, 2, 3]);
restoreHeld([{ text: "", images: ["h9.png"], nums: [1], chips: [{ id: "5", answer: "", note: "old [Image #1]" }] }]);
check("a replaced note's image is named in the composer", [ctext.value, note5.value, imgs.map(function (p) { return p.n; })], ["[Image #6]\n\nheld [Image #3]\n\ndraft [Image #1]", "newer", [1, 2, 3, 6]]);
process.stdout.write(bad.join("\n"));
`)
}
