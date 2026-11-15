package wsdashboard

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

func TestOverviewPureContracts(t *testing.T) {
	js := mustAsset(t, "overview.js")
	var src strings.Builder
	for _, name := range []string{"anchor", "classify", "layer", "readState", "ease"} {
		f := regexp.MustCompile(`(?s)\n  function ` + name + `\([^\n]*\) \{.*?\n  \}\n`).FindString(js)
		if f == "" {
			t.Fatalf("missing %s", name)
		}
		src.WriteString(f)
	}
	runNode(t, "overview contracts", `
var bad=[];
function check(name, got, want) { if (JSON.stringify(got)!==JSON.stringify(want)) bad.push(name+": "+JSON.stringify(got)); }
var o={board:{contains:function(t){return t.inBoard;}}}, ITEM="items", showing="";
var doc={querySelector:function(sel){return showing && sel.indexOf(showing)>=0 ? {} : null;}};
`+src.String()+`
function node(top,bottom) { return {getBoundingClientRect:function(){return {top:top,bottom:bottom};}}; }
check("curve endpoints",[ease(0),ease(1)],[0,1]);
var prev=0;for(var i=0;i<=100;i++){var e=ease(i/100);check("curve monotone "+i,e>=prev,true);prev=e;}
check("approved curve at midpoint",Math.abs(ease(0.5)-0.946079)<0.0001,true);
var a=node(-40,20),b=node(20,80),v={top:10,bottom:100};
check("first visible",anchor([a,b],{top:-80,bottom:0},v)===a,true);
check("toolbar visible means no anchor",anchor([a,b],{top:0,bottom:40},v),null);
check("below viewport is not visible",anchor([node(101,150)],{top:-80,bottom:0},v),null);
check("floating face does not suppress reading anchor",anchor([a,b],{top:10,bottom:38},v,true)===b,true);
check("item entirely behind face is not an anchor",anchor([node(10,38)],{top:10,bottom:38},v,true),null);
check("partially exposed item remains anchor",anchor([node(20,39),b],{top:10,bottom:38},v,true).getBoundingClientRect().bottom,39);
check("face below viewport has no reading anchor",anchor([a,b],{top:10,bottom:110},v,true),null);
function target(kind) { return {inBoard:kind==='items',closest:function(sel){return sel.indexOf(kind)>=0?{}:null;}}; }
check("toolbar click",classify(target('.board-toolbar')),"toolbar");
check("popover click",classify(target('.tpop')),"popover");
check("palette click",classify(target('#tkqpop')),"palette");
check("item click",classify(target('items')),"item");
check("rail placeholder",classify(target('.rail-ph')),"outside");
check("no layer",layer(),"overview"); showing='.tpop';check("popover first",layer(),"popover");
showing='dialog';check("palette first",layer(),"palette");
var state={on:true,key:{topic:'sample'},offset:-127.25,pinTop:100};
check("storage round trip",readState(JSON.stringify(state)),state);
['null','{}','{"on":false,"offset":0}','{"on":true,"offset":"bad"}','broken'].forEach(function(s){check("invalid storage "+s,readState(s),null);});
process.stdout.write(bad.join('\n'));
`)
}

func TestOverviewFilterContracts(t *testing.T) {
	js := mustAsset(t, "overview.js")
	var src strings.Builder
	for _, name := range []string{"all", "items", "rove", "snapshot", "key"} {
		pattern := `(?s)\n  function ` + name + `\([^\n]*\) \{.*?\n  \}\n`
		if name == "all" || name == "items" {
			pattern = `(?m)^  function ` + name + `\([^\n]*\) \{[^\n]*\}$`
		}
		f := regexp.MustCompile(pattern).FindString(js)
		if f == "" {
			t.Fatalf("missing %s", name)
		}
		src.WriteString(f + "\n")
	}
	runNode(t, "overview filter contracts", `
var bad=[], ITEM="items", tabs=new Map(), on=true, busy=false, swallowed=0;
function check(name,got,want){if(JSON.stringify(got)!==JSON.stringify(want))bad.push(name+": "+JSON.stringify(got));}
var doc={activeElement:null};
function node(name,filtered){return {name:name,attrs:filtered?{"data-state-filtered":""}:{},
  hasAttribute:function(k){return k in this.attrs;},getAttribute:function(k){return this.hasAttribute(k)?this.attrs[k]:null;},
  setAttribute:function(k,v){this.attrs[k]=v;},focus:function(){doc.activeElement=this;},
  getBoundingClientRect:function(){return {top:20,bottom:60,height:40};},querySelector:function(){return null;}};}
var a=node("a",false),hidden=node("hidden",true),b=node("b",false),pin=node("pin",false),hiddenPin=node("hiddenPin",true);
var o={board:{querySelectorAll:function(){return [a,hidden,b];}},pins:{querySelectorAll:function(){return [pin,hiddenPin,a];}}};
function getComputedStyle(){return {marginBottom:"6px"};}
function layer(){return "overview";} function classify(){return "item";}
function swallow(){swallowed++;} function scroller(){return {};}
function view(){return {top:0,bottom:100,height:100};} function clearance(){return 0;}
`+src.String()+`
check("keyboard item list excludes filtered topics",items().map(function(n){return n.name;}),["a","b"]);
rove(a); key({key:"ArrowDown",target:a});
check("arrow skips filtered topic",[doc.activeElement.name,hidden.getAttribute("tabindex"),swallowed],["b",null,1]);
key({key:"ArrowUp",target:b});
check("reverse arrow also skips filtered topic",doc.activeElement.name,"a");
check("motion snapshot excludes filtered board and pinned topics and deduplicates",snapshot().map(function(s){return [s.n.name,s.height,s.margin];}),[["a",40,6],["b",40,6],["pin",40,6]]);
process.stdout.write(bad.join("\n"));
`)
}

// Execute the complete module: motion completion is the filter barrier, not
// after()'s exit path. The shared DOM keeps this independent of a browser.
func TestOverviewStateFilterLifecycle(t *testing.T) {
	js, _ := json.Marshal(mustAsset(t, "overview.js"))
	runNode(t, "overview state filter lifecycle", "var OVERVIEW = "+string(js)+";\n"+cardsDOM+`
var clock=0, frames=[], reduced=true, kept=[], storage={};
var doc=new Doc(), root=doc.documentElement, win={matchMedia:function(){return {matches:reduced};}};
page={doc:doc}; doc.scrollingElement=root;
doc.querySelector=function(s){return root.querySelector(s);};
Object.defineProperty(El.prototype,"parentElement",{get:function(){return this.parentNode;}});
El.prototype.focus=function(){doc.activeElement=this;};
El.prototype.remove=function(){if(this.parentNode)this.parentNode.removeChild(this);};
El.prototype.contains=function(n){for(;n;n=n.parentNode)if(n===this)return true;return false;};
var query=El.prototype.querySelector;
El.prototype.querySelector=function(s){return s.indexOf(":scope")>=0?null:query.call(this,s);};
El.prototype.getBoundingClientRect=function(){var top=this.top||0;return {top:top,bottom:top+32,height:32,left:0,right:500,width:500};};
function style(){return {overflowY:"visible",marginBottom:"0",scrollPaddingTop:"0",getPropertyValue:function(){return "";}};}
var board=doc.body.appendChild(mk("div")), toolbar=board.appendChild(mk("div.board-toolbar",{},[mk("button.board-overview")]));
toolbar.top=-80;toolbar.firstChild.top=-80;
var pins=board.appendChild(mk("div"));
var rows=["a","b","c","d"].map(function(name,i){var n=board.appendChild(mk("section.topic",{"data-topic":name}));n.top=20+i*40;return n;});
var a=rows[0],b=rows[1],c=rows[2],d=rows[3],button=toolbar.firstChild;
a.setAttribute("data-state-filtered","");
var session={getItem:function(k){return storage[k]||null;},setItem:function(k,v){storage[k]=v;},removeItem:function(k){delete storage[k];}};
new Function("document","window","sessionStorage","getComputedStyle","requestAnimationFrame","performance","innerHeight","innerWidth","setTimeout",OVERVIEW)(doc,win,session,style,function(fn){frames.push(fn);},{now:function(){return clock;}},500,500,function(){});
var api=win.WsOverview;
function mount(){api.mount({board:board,pins:pins,toolbar:toolbar,rid:"fixture",place:function(){},keep:function(n,opt){kept.push([n.getAttribute("data-topic")||"toolbar",opt.offset]);}});}
function drain(){while(frames.length){clock+=120;frames.shift()(clock);}}
function selected(){return rows.filter(function(n){return n.getAttribute("tabindex")==="0";}).map(function(n){return n.getAttribute("data-topic");});}
function state(){return [api.active(),root.hasAttribute("data-overview"),button.getAttribute("aria-pressed")];}
mount();var applied=0;api.filter(function(){applied++;});check("off applies immediately",applied,1);
api.toggle();check("entry topic is roved",selected(),["b"]);
button.focus();kept=[];
api.filter(function(){b.setAttribute("data-state-filtered","");a.removeAttribute("data-state-filtered");button.focus();});
check("filter keeps Overview and toolbar pressed",state(),[true,true,"true"]);
check("filtered roved item hands off at or below its old position",selected(),["c"]);
check("new eligibility joins roving set",a.getAttribute("tabindex"),"-1");
check("filter does not steal control focus",doc.activeElement===button,true);
check("topmost surviving visible row keeps its old offset",kept,[["c",100]]);
check("session view stays saved",JSON.parse(storage["ws.dashboard.overview.fixture"]).on,true);
api.exit(null);check("filtered entry lands on first eligible row",[doc.activeElement===a,kept[kept.length-1][0]],[true,"a"]);
check("exit restores original tabindex",rows.map(function(n){return n.getAttribute("tabindex");}),["-1",null,null,null]);
// Restore the saved view even when the entry key is no longer eligible.
storage["ws.dashboard.overview.fixture"]=JSON.stringify({on:true,key:{topic:"b"},offset:60});
mount();drain();check("session restore never roves hidden entry",selected(),["a"]);
api.filter(function(){a.setAttribute("data-state-filtered","");c.setAttribute("data-state-filtered","");d.setAttribute("data-state-filtered","");});
check("zero eligibility has no roving target",selected(),[]);
api.exit(null);check("zero eligibility exits to toolbar",[doc.activeElement===button,kept[kept.length-1][0]],[true,"toolbar"]);
rows.forEach(function(n){n.removeAttribute("data-state-filtered");});
reduced=false;applied=0;api.toggle();
api.filter(function(){applied++;a.setAttribute("data-state-filtered","");button.focus();});
check("entering defers apply",[applied,frames.length>0],[0,true]);drain();
check("entering barrier does not exit",[applied,state(),doc.activeElement===button],[1,[true,true,"true"],true]);
api.exit(null);api.filter(function(){applied++;});
check("exiting defers apply",applied,1);drain();
check("exiting barrier applies off",[applied,state()],[2,[false,false,"false"]]);
reduced=true;api.toggle();
// The last eligible row falls back upward when no row below it survives.
rows.forEach(function(n){n.setAttribute("tabindex",n===d?"0":"-1");});
api.filter(function(){d.setAttribute("data-state-filtered","");});
check("last roved row falls back to last eligible",selected(),["c"]);
api.pick(c);check("topic pick still exits and focuses selection",[api.active(),doc.activeElement===c],[false,true]);
process.stdout.write(bad.join("\n"));
`)
}

func TestOverviewPageIntegration(t *testing.T) {
	js := mustAsset(t, "dashboard.js")
	for _, want := range []string{"window.WsOverview.holding()", "boardOverviewHeld = { hash: hash, html: html }", "window.WsOverview.mount({", "keep: window.wbHistory.keep", "window.WsOverview.pick(topic, o)", "doc.addEventListener(\"ws-overview\", queue)", "window.WsOverview.active() ? 0.72 : 0", "if (cardBoard) mountBoardOverview();", "filter: function (run) { if (window.WsOverview) window.WsOverview.filter(run); else run(); }", "function afterOverview(fn) { if (window.WsOverview) window.WsOverview.after(fn); else fn(); }"} {
		if !strings.Contains(js, want) {
			t.Errorf("missing overview integration %q", want)
		}
	}
	overview := mustAsset(t, "overview.js")
	if strings.Contains(overview, "scrollTop =") || strings.Contains(overview, "scrollIntoView(") || strings.Contains(overview, "localStorage") || strings.Contains(overview, ".fold(") {
		t.Error("overview bypasses view-only/shared scroll ownership")
	}
	for _, want := range []string{"o.keep(n,", "ws.dashboard.overview.", "spacer.style.height", "o.place(true)", "o.place(false)", "scroller(landed) === o.pins", "e.stopImmediatePropagation()", "n.focus({ preventScroll: true })"} {
		if !strings.Contains(overview, want) {
			t.Errorf("missing overview contract %q", want)
		}
	}
	cards := mustAsset(t, "cards.js")
	for _, want := range []string{"if (overviewOn || dg", "window.WsOverview.after(function () { maximize(c); })", "if (opt.toolbar && opt.toolbar.filter) opt.toolbar.filter(run); else run();"} {
		if !strings.Contains(cards, want) {
			t.Errorf("missing card guard %q", want)
		}
	}
}
