package wsdashboard

import (
	"encoding/json"
	"testing"
)

func TestBoardStateFilter(t *testing.T) {
	cards, _ := json.Marshal(mustAsset(t, "cards.js"))
	runNode(t, "board state filter", "var CARDS = "+string(cards)+";\n"+cardsDOM+`
var store = {}, A, B, C, D, legacy, closed, tail;
function stateTopic(name, state, isClosed) {
  var t = topic(name, 100, isClosed);
  if (state) t.setAttribute("data-state", state);
  return t;
}
function served() {
  A = stateTopic("a", "awaiting-confirmation"); B = stateTopic("b", "discussing");
  C = stateTopic("c", "implementing"); D = stateTopic("d", "waiting");
  legacy = stateTopic("legacy"); closed = stateTopic("old", null, true); tail = more(1, 7);
  return [A, B, C, D, legacy, closed, tail];
}
boot(store, served, { wide: true });
function radio(value, pop) { return page.W.toolbar().querySelector((pop ? ".board-state-pop" : ".board-state-inline") + ' [data-filter="' + value + '"]'); }
function counts() { return page.W.toolbar().querySelectorAll('.board-state-inline [role="radio"]').map(function (n) { return n.querySelector(".board-state-count").textContent; }); }
function choose(value) { radio(value).fire("click", {}); }
function shown() { return [A, B, C, D, legacy, closed].filter(function (n) { return !n.hasAttribute("data-state-filtered"); }).map(function (n) { return n.getAttribute("data-topic"); }); }
check("all includes legacy and loaded closed; count includes unloaded closed", [shown(), counts()], [["a", "b", "c", "d", "legacy", "old"], ["13", "1", "1", "1", "1", "8"]]);
["awaiting-confirmation", "discussing", "implementing", "waiting"].forEach(function (s, i) {
  choose(s);
  check("only authoritative " + s, shown(), [["a"], ["b"], ["c"], ["d"]][i]);
  check("operational filter hides closed paging", tail.hasAttribute("data-state-filtered"), true);
  check("filter counts do not depend on visibility", counts(), ["13", "1", "1", "1", "1", "8"]);
  check("radio groups share selection", [radio(s).getAttribute("aria-checked"), radio(s, true).getAttribute("aria-checked")], ["true", "true"]);
});
choose("closed"); check("closed lifecycle includes stateless history", [shown(), tail.hasAttribute("data-state-filtered")], [["old"], false]);
choose("all"); clickPin(C);
choose("implementing"); check("pinned topic keeps its node and placement", [shown(), C.parentNode === page.W.pinList(), counts()], [["c"], true, ["13", "1", "1", "1", "1", "8"]]);
var before = page.board.childNodes.slice(), savedCards = store.cards;
choose("closed"); choose("all");
check("filtering never moves or persists topic placement", [page.board.childNodes.every(function(n,i){return n === before[i];}), store.cards === savedCards], [true, true]);
choose("waiting");
var replacement = stateTopic("d", "discussing"); D = replacement;
swap([A, B, C, D, legacy, closed, tail]);
check("refresh re-evaluates selected state and zero counts", [shown(), counts(), radio("waiting").getAttribute("aria-checked")], [[], ["13", "1", "2", "1", "0", "8"], "true"]);
choose("closed"); C = stateTopic("c", null, true);
swap([A, B, D, legacy, C, closed, tail]);
check("close removes operational state and increments closed", [counts(), C.hasAttribute("data-state-filtered")], [["13", "1", "2", "0", "0", "9"], false]);
var closedPage = mk("div.closed-page", {}, [stateTopic("older", null, true), more(2, 6)]);
page.board.insertBefore(closedPage, tail); page.board.removeChild(tail); page.W.decorate();
check("history load preserves total and selected lifecycle", [counts(), closedPage.firstChild.hasAttribute("data-state-filtered")], [["13", "1", "2", "0", "0", "9"], false]);
boot(store, served);
check("reload restores selection independent of cards", [radio("closed").getAttribute("aria-checked"), shown()], ["true", ["old"]]);
var prevented = false;
radio("closed").fire("keydown", { key: "ArrowRight", preventDefault: function(){prevented=true;}, stopPropagation: function(){} });
check("radio arrow wraps, selects and persists", [radio("all").getAttribute("aria-checked"), store["cards.state-filter"], prevented], ["true", "all", true]);
radio("all").fire("keydown", { key: "End", preventDefault: function(){}, stopPropagation: function(){} });
check("End selects closed", radio("closed").getAttribute("aria-checked"), "true");
store["cards.state-filter"] = "invented"; boot(store, served);
check("invalid persisted filter is all, never inferred", [shown().length, radio("all").getAttribute("aria-checked")], [6, "true"]);
boot({}, function(){return [];});
check("empty board has selectable zero counts", counts(), ["0","0","0","0","0","0"]);
var deferred;
boot({}, served, { toolbar: { filter: function(run){ deferred = run; } } });
choose("discussing");
check("filter waits for overview restoration callback", [shown(), radio("all").getAttribute("aria-checked"), page.store["cards.state-filter"]], [["a","b","c","d","legacy","old"], "true", undefined]);
deferred();
check("overview callback applies and persists selection", [shown(), radio("discussing").getAttribute("aria-checked"), page.store["cards.state-filter"]], [["b"], "true", "discussing"]);
process.stdout.write(bad.join("\n"));
`)
}
