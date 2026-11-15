package wsdashboard

import (
	"encoding/json"
	"testing"
)

func TestBoardStateFilter(t *testing.T) {
	cards, _ := json.Marshal(mustAsset(t, "cards.js"))
	runNode(t, "board state filter", "var CARDS = "+string(cards)+";\n"+cardsDOM+`
var store = {}, A, B, C, D, R, legacy, closed, tail;
var filterOrder = ["all", "researching", "discussing", "waiting", "implementing", "awaiting-confirmation", "closed", "cards"];
var filterLabels = ["all", "researching", "discuss", "wait", "implement", "confirm", "closed", "cards"].map(function (key) { return "js.board.filter." + key; });
function stateTopic(name, state, isClosed) {
  var t = topic(name, 100, isClosed);
  if (state) t.setAttribute("data-state", state);
  return t;
}
function served() {
  A = stateTopic("a", "awaiting-confirmation"); B = stateTopic("b", "discussing");
  C = stateTopic("c", "implementing"); D = stateTopic("d", "waiting"); R = stateTopic("r", "researching");
  legacy = stateTopic("legacy"); closed = stateTopic("old", null, true); tail = more(1, 7);
  // Topic-internal panels are not framework cards.
  R.appendChild(mk("article.wb-entry", { "data-panel": "panel-1" }));
  return [A, B, C, D, R, legacy, closed, tail];
}
boot(store, served, { wide: true });
function radio(value, pop) { return page.W.toolbar().querySelector((pop ? ".board-state-pop" : ".board-state-inline") + ' [data-filter="' + value + '"]'); }
[".board-state-inline", ".board-state-pop"].forEach(function (group) {
  var radios = page.W.toolbar().querySelectorAll(group + ' [role="radio"]');
  check(group + " exact lifecycle order and label keys", radios.map(function (n) { return [n.getAttribute("data-filter"), n.querySelector(".board-state-label").textContent]; }), filterOrder.map(function (value, i) { return [value, filterLabels[i]]; }));
  check(group + " uses existing hammer sprite", radio("cards", group === ".board-state-pop").querySelector(".board-state-icon").html, '<svg class="li" aria-hidden="true"><use href="#i-hammer"/></svg>');
});
function counts() { return page.W.toolbar().querySelectorAll('.board-state-inline [role="radio"]').map(function (n) { return n.querySelector(".board-state-count").textContent; }); }
function choose(value) { radio(value).fire("click", {}); }
function shown() { return [A, B, C, D, R, legacy, closed].filter(function (n) { return !n.hasAttribute("data-state-filtered"); }).map(function (n) { return n.getAttribute("data-topic"); }); }
check("all includes legacy and loaded closed; count includes unloaded closed", [shown(), counts()], [["a", "b", "c", "d", "r", "legacy", "old"], ["14", "1", "1", "1", "1", "1", "8", "0"]]);
var roots = ["preview", "terminal", "web", "explorer"].map(function(kind, i) {
  page.W.register(kind, { open: function(c) { c.body.appendChild(mk("iframe", { src: "about:blank" })); } });
  var c = page.W.open(kind, { path: "same-path" });
  if (i % 2) c.pin(true);
  return c;
});
var frames = roots.map(function(c) { return c.body.firstChild; });
function cardVisibility() { return roots.map(function(c) { return !c.el.hasAttribute("data-state-filtered"); }); }
check("framework roots counted across board and pins, not by path", counts(), ["14", "1", "1", "1", "1", "1", "8", "4"]);
["awaiting-confirmation", "discussing", "implementing", "waiting", "researching"].forEach(function (s, i) {
  choose(s);
  check("only authoritative " + s, shown(), [["a"], ["b"], ["c"], ["d"], ["r"]][i]);
  check("state filters hide framework roots", cardVisibility(), [false, false, false, false]);
  check("operational filter hides closed paging", tail.hasAttribute("data-state-filtered"), true);
  check("filter counts do not depend on visibility", counts(), ["14", "1", "1", "1", "1", "1", "8", "4"]);
  check("radio groups share selection", [radio(s).getAttribute("aria-checked"), radio(s, true).getAttribute("aria-checked")], ["true", "true"]);
});
choose("closed"); check("closed lifecycle hides cards", [shown(), tail.hasAttribute("data-state-filtered"), cardVisibility()], [["old"], false, [false, false, false, false]]);
choose("cards");
check("Cards shows roots only, no topics or paging", [shown(), cardVisibility(), tail.hasAttribute("data-state-filtered")], [[], [true, true, true, true], true]);
check("compact trigger uses hammer too", page.W.toolbar().querySelector(".board-state-trigger .board-state-icon").html, '<svg class="li" aria-hidden="true"><use href="#i-hammer"/></svg>');
roots[0].pin(true); roots[1].pin(false);
check("pin/unpin retains roots, frames and count", [roots.every(function(c,i){return c.body.firstChild === frames[i];}), counts()[7]], [true, "4"]);
choose("all");
check("All restores cards and topic-only total", [shown().length, cardVisibility(), counts()[0]], [7, [true, true, true, true], "14"]);
clickPin(C); choose("implementing");
check("pinned topic keeps its node and placement", [shown(), C.parentNode === page.W.pinList(), counts()], [["c"], true, ["14", "1", "1", "1", "1", "1", "8", "4"]]);
var before = page.board.childNodes.slice(), savedCards = store.cards;
choose("cards"); choose("closed"); choose("all");
check("filtering never moves or persists placement", [page.board.childNodes.every(function(n,i){return n === before[i];}), store.cards === savedCards], [true, true]);
choose("waiting");
D = stateTopic("d", "discussing"); R = stateTopic("r", "implementing");
swap([A, B, C, D, R, legacy, closed, tail]);
check("refresh re-evaluates selected state and research exit", [shown(), counts(), radio("waiting").getAttribute("aria-checked")], [[], ["14", "0", "2", "0", "2", "1", "8", "4"], "true"]);
check("refresh retains filtered framework identity and contents", roots.every(function(c,i){return c.body.firstChild === frames[i] && c.el.parentNode && c.el.hasAttribute("data-state-filtered");}), true);
// Card lifecycle still runs while hidden; opens and both close placements recount.
var extra = page.W.open("preview", {path:"same-path"});
check("open honors active state filter and recounts", [extra.el.hasAttribute("data-state-filtered"), counts()[7]], [true, "5"]);
extra.close(); check("unpinned close recounts", counts()[7], "4");
roots[0].close(); check("pinned close recounts", counts()[7], "3");
choose("closed"); C = stateTopic("c", null, true); R = stateTopic("r", null, true);
swap([A, B, D, R, legacy, C, closed, tail]);
check("close removes operational state and increments closed", [counts(), R.hasAttribute("data-state-filtered")], [["14", "0", "2", "0", "0", "1", "10", "3"], false]);
var closedPage = mk("div.closed-page", {}, [stateTopic("older", null, true), more(2, 6)]);
page.board.insertBefore(closedPage, tail); page.board.removeChild(tail); page.W.decorate();
check("history load preserves total and selected lifecycle", [counts(), closedPage.firstChild.hasAttribute("data-state-filtered")], [["14", "0", "2", "0", "0", "1", "10", "3"], false]);
choose("cards"); check("Cards hides loaded closed page and paging", [closedPage.firstChild.hasAttribute("data-state-filtered"), closedPage.children[1].hasAttribute("data-state-filtered")], [true, true]);
boot(store, served, { kinds: ["preview", "terminal", "web", "explorer"] });
check("reload restores Cards selection and retained framework records", [radio("cards").getAttribute("aria-checked"), shown(), counts()[7]], ["true", [], "3"]);
[false, true].forEach(function (pop) {
  choose("researching");
  [["ArrowRight", "discussing"], ["ArrowDown", "waiting"], ["ArrowLeft", "discussing"], ["ArrowUp", "researching"], ["Home", "all"], ["ArrowLeft", "cards"], ["ArrowRight", "all"], ["End", "cards"], ["ArrowDown", "all"], ["ArrowUp", "cards"]].forEach(function (step) {
    var selected = filterOrder.filter(function (value) { return radio(value, pop).getAttribute("aria-checked") === "true"; })[0], prevented = false;
    radio(selected, pop).fire("keydown", { key: step[0], preventDefault: function(){prevented=true;}, stopPropagation: function(){} });
    check("keyboard lifecycle " + pop + " " + step[0] + " to " + step[1], [radio(step[1]).getAttribute("aria-checked"), radio(step[1], true).getAttribute("aria-checked"), store["cards.state-filter"], prevented], ["true", "true", step[1], true]);
  });
});
store["cards.state-filter"] = "invented"; boot(store, served);
check("invalid persisted filter is all, never inferred", [shown().length, radio("all").getAttribute("aria-checked")], [7, "true"]);
boot({}, function(){return [];});
check("empty board has selectable zero counts", counts(), ["0","0","0","0","0","0","0","0"]);
var deferred;
boot({}, served, { toolbar: { filter: function(run){ deferred = run; } } });
var held = page.W.open("k", {name:"held"});
choose("cards");
check("filter waits for overview restoration callback", [shown(), held.el.hasAttribute("data-state-filtered"), radio("all").getAttribute("aria-checked"), page.store["cards.state-filter"]], [["a","b","c","d","r","legacy","old"], false, "true", undefined]);
deferred();
check("overview callback applies and persists selection", [shown(), held.el.hasAttribute("data-state-filtered"), radio("cards").getAttribute("aria-checked"), page.store["cards.state-filter"]], [[], false, "true", "cards"]);
process.stdout.write(bad.join("\n"));
`)
}
