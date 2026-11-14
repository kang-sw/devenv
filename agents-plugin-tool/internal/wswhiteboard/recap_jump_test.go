package wswhiteboard

import (
	"os/exec"
	"strings"
	"testing"
)

// TestRecapJumpButtonNamesItsLogRow pins the recap's link to the log row it
// summarizes: on the full page an agent recap is a keyboard button whose
// data-entry is the stream's current agent row; the chat embed has no log
// rows, and a topic without an agent line has nothing to jump to.
func TestRecapJumpButtonNamesItsLogRow(t *testing.T) {
	st := &State{}
	mustApply(t, st, `<wb:topic name="open"><wb:log>old</wb:log><wb:log>latest</wb:log></wb:topic>`+
		`<wb:topic name="done"><wb:log>final</wb:log><wb:close result="Shipped."/></wb:topic>`+
		addXML(ClassTechnical, "quiet", "no log yet"))
	button := `class="trecap" data-entry="e2" data-kind="agent" role="button" tabindex="0"`
	full := render(t, BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"}))
	wantAll(t, full, button, `class="wb-entry agent current" id="e2" data-history-anchor="e2" data-entry="e2"`,
		// A closed topic's folded recap jumps into its (closed) stream too.
		`class="trecap" data-entry="e3" data-kind="agent" role="button" tabindex="0"`, `id="e3"`)
	if n := strings.Count(full, `role="button" tabindex="0"`); n != 2 {
		t.Fatalf("full page recap buttons = %d, want 2 (one per agent recap)", n)
	}
	embed := render(t, BuildView(st, ViewInput{Layout: LayoutEmbed, ReplyID: "r"}))
	wantAll(t, embed, `class="trecap" data-entry="e2" data-kind="agent">`)
	if strings.Contains(embed, `class="trecap" data-entry="e2" data-kind="agent" role=`) {
		t.Fatal("embed recap must not be a jump button: the embed has no log rows")
	}
	css, err := embeddedAsset("page.css")
	if err != nil {
		t.Fatal(err)
	}
	wantAll(t, string(css), `.trecap[role="button"] { cursor: pointer; }`,
		`.trecap[role="button"]:focus-visible { outline: 2px solid var(--wb-primary); outline-offset: -2px; }`)
}

// TestRecapJumpScriptUnderNode drives the recap-jump block of stream.js
// against a fake page: a plain click or Enter/Space on the recap loads its
// row through wbHistory.loadTarget and replays the latest-log blink; links,
// drags, selections, and multi-clicks keep their own behavior.
func TestRecapJumpScriptUnderNode(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	asset, err := embeddedAsset("stream.js")
	if err != nil {
		t.Fatal(err)
	}
	_, block, ok := strings.Cut(string(asset), "/* Recap jump:")
	if !ok {
		t.Fatal("missing recap-jump script")
	}
	block, _, ok = strings.Cut(block, "/* Latest agent arrival:")
	if !ok {
		t.Fatal("recap-jump script must precede the latest-log block")
	}
	block = "/* Recap jump:" + block
	script := `
var bad=[];function check(what,got,want){if(JSON.stringify(got)!==JSON.stringify(want))bad.push(what+': got '+JSON.stringify(got)+', want '+JSON.stringify(want));}
var BUTTON='.trecap[data-entry][role="button"]', INNER='a[href], button, input, textarea, select, summary, label, [contenteditable]';
function el(name,parent){var cls=new Set();return {name:name,parent:parent,offsetWidth:0,
  classList:{add:function(c){cls.add(c);},remove:function(c){cls.delete(c);},contains:function(c){return cls.has(c);}},
  closest:function(s){for(var n=this;n;n=n.parent){if(s===BUTTON&&n.name==='recap')return n;if(s==='.topic'&&n.name==='topic')return n;if(s===INNER&&n.name==='link')return n;}return null;},
  matches:function(s){return s===BUTTON&&this.name==='recap';},
  contains:function(o){for(var n=o;n;n=n.parent)if(n===this)return true;return false;},
  getAttribute:function(a){return a==='data-entry'&&this.name==='recap'?'e2':null;}};}
var topic=el('topic',null), recap=el('recap',topic), text=el('text',recap), link=el('link',text), other=el('other',topic), row=el('row',topic);
var listeners={}, loads=[], timers=[], reduced=false, found=row, unfold=function(){};
var selection={isCollapsed:true,rangeCount:0,getRangeAt:function(){return {intersectsNode:function(n){return selection.over===n;}};}};
var document={addEventListener:function(type,fn){(listeners[type]=listeners[type]||[]).push(fn);}};
var window={matchMedia:function(){return {matches:reduced};},getSelection:function(){return selection;},
  setTimeout:function(fn,ms){timers.push([fn,ms]);return timers.length;},clearTimeout:function(){},
  WsCards:{unfoldTopic:unfold},
  wbHistory:{loadTarget:function(root,id,before){loads.push([root.name,id,before===unfold]);return found;}}};
function fire(type,e){e.prevented=false;e.preventDefault=function(){e.prevented=true;};(listeners[type]||[]).forEach(function(fn){fn(e);});return e;}
function click(target,o){o=o||{};var x=o.x||0,y=o.y||0;
  fire('pointerdown',{target:o.down||target,clientX:o.dx||0,clientY:o.dy||0,button:0});
  return fire('click',{target:target,clientX:x,clientY:y,button:o.button||0,detail:o.detail===undefined?1:o.detail,defaultPrevented:!!o.prevented});}
function key(target,k,o){o=o||{};return fire('keydown',Object.assign({target:target,key:k},o));}
function took(label,want){check(label,loads.length,want);loads=[];}
` + block + `
click(text);check('click loads the recap row',loads,[['topic','e2',true]]);loads=[];
check('row replays the latest-log blink',row.classList.contains('wb-log-blink'),true);
check('blink lasts three seconds',timers.length&&timers[0][1],3000);timers.shift()[0]();
check('blink ends',row.classList.contains('wb-log-blink'),false);
click(link);took('a link inside the recap keeps its own click',0);
click(text,{x:30,y:0});took('a drag selects instead of jumping',0);
click(text,{detail:2});took('a double-click selects a word instead',0);
click(text,{prevented:true});took('a consumed click is left alone',0);
click(text,{button:1});took('a middle click is not a jump',0);
click(other);took('clicks outside the recap are ignored',0);
selection.isCollapsed=false;selection.rangeCount=1;selection.over=recap;
click(text);took('a selection over the recap blocks the jump',0);
selection.over=other;click(text);took('a selection elsewhere does not',1);
selection.isCollapsed=true;selection.rangeCount=0;selection.over=null;
var e=key(recap,'Enter');took('Enter on the recap jumps',1);check('Enter is consumed',e.prevented,true);
e=key(recap,' ');took('Space on the recap jumps',1);check('Space does not scroll the page',e.prevented,true);
e=key(link,'Enter');took('Enter on a recap link follows the link',0);check('link Enter untouched',e.prevented,false);
key(recap,'Enter',{isComposing:true});took('IME composition is not a jump',0);
key(recap,'Enter',{shiftKey:true});took('modified Enter is not a jump',0);
key(recap,'a');took('other keys are ignored',0);
reduced=true;timers=[];row.classList.remove('wb-log-blink');click(text);took('reduced motion still jumps',1);
check('reduced motion has no blink',[row.classList.contains('wb-log-blink'),timers.length],[false,0]);
reduced=false;found=null;click(text);took('a missing row is tolerated',1);
var count=listeners.click.length;
` + block + `
check('re-evaluating the shared asset adds no listeners',listeners.click.length,count);
if(bad.length){console.error(bad.join('\n'));process.exit(1);}
`
	out, err := nodeScript(node, script).CombinedOutput()
	if err != nil {
		t.Fatalf("recap jump: %v\n%s", err, out)
	}
}
