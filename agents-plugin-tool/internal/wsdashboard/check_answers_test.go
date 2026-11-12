package wsdashboard

import (
	"regexp"
	"testing"
)

// TestBoardSwapCarriesACheckRowsAnswer pins dashboard.js's answer carry-over
// for a check row: readAnswers records a ticked box and a staged "no" (the
// row's data-no) by item number, and writeAnswers puts both back on the new
// board's row, pressing its x. The cases execute under node when it is
// installed.
func TestBoardSwapCarriesACheckRowsAnswer(t *testing.T) {
	data, err := dashboardFS.ReadFile("assets/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	block := regexp.MustCompile(`(?s)\n  function pickKey.*?\n  // A board swap waits while the user is typing`).FindString(string(data))
	if block == "" {
		t.Fatal("dashboard.js holds no pickKey ... writeAnswers block")
	}
	runNode(t, "check carry-over", `
var bad = [];
function check(what, got, want) { if (JSON.stringify(got) !== JSON.stringify(want)) bad.push(what + ": got " + JSON.stringify(got) + ", want " + JSON.stringify(want)); }
function N(attrs, sel) { this.attrs = attrs || {}; this.sel = sel || {}; this.checked = false; this.hidden = true; this.value = ""; this.style = {}; }
N.prototype.querySelectorAll = function (s) { return this.sel[s] || []; };
N.prototype.querySelector = function (s) { return (this.sel[s] || [])[0] || null; };
N.prototype.getAttribute = function (k) { return k in this.attrs ? this.attrs[k] : null; };
N.prototype.setAttribute = function (k, v) { this.attrs[k] = String(v); };
N.prototype.removeAttribute = function (k) { delete this.attrs[k]; };
N.prototype.hasAttribute = function (k) { return k in this.attrs; };
function q(s, el) { return el.querySelectorAll(s); }
var ITEMS = ".card[data-id], .arow[data-id], .ckrow[data-id], .srow[data-id]", BOX = "label.ack > input, label.ck > input";
function checkRow(id) {
  var box = new N(), x = new N({ "aria-pressed": "false" }), note = new N(), s = {};
  s[BOX] = [box]; s["button.no-toggle"] = [x]; s["textarea.note"] = [note];
  var row = new N({ "data-id": id }, s); row.box = box; row.x = x; return row;
}
function board(rows) { var s = {}; s[ITEMS] = rows; return new N({}, s); }
`+block+`
var no = checkRow("03"), ok = checkRow("04"), idle = checkRow("05");
no.setAttribute("data-no", ""); no.x.setAttribute("aria-pressed", "true"); ok.box.checked = true;
var saved = readAnswers(board([no, ok, idle]));
check("read", saved, { "03": { no: true }, "04": { checked: true } });
var no2 = checkRow("03"), ok2 = checkRow("04"), idle2 = checkRow("05");
writeAnswers(board([no2, ok2, idle2]), saved);
check("the no carries over", [no2.hasAttribute("data-no"), no2.x.getAttribute("aria-pressed"), no2.box.checked], [true, "true", false]);
check("the tick carries over", [ok2.box.checked, ok2.hasAttribute("data-no")], [true, false]);
check("an unanswered row stays", [idle2.box.checked, idle2.hasAttribute("data-no")], [false, false]);
process.stdout.write(bad.join("\n"));`)
}
