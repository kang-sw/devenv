package wswhiteboard

import (
	"os/exec"
	"strings"
	"testing"
)

// TestRecapJumpButtonNamesItsLogRow pins the recap's link to the log row it
// summarizes: on the full page an agent recap's clock chip is a keyboard
// button and the recap's data-entry is the stream's current agent row. The
// recap itself is not a button, since its text may hold links. The chat
// embed has no log rows, and a topic without an agent line has nothing to
// jump to.
func TestRecapJumpButtonNamesItsLogRow(t *testing.T) {
	st := &State{}
	mustApply(t, st, `<wb:topic name="open"><wb:log>old</wb:log><wb:log>latest [see](https://example.com)</wb:log></wb:topic>`+
		`<wb:topic name="done"><wb:log>final</wb:log><wb:close result="Shipped."/></wb:topic>`+
		addXML(ClassTechnical, "quiet", "no log yet"))
	// Each created topic gets a default state event right after its creating
	// log: open is e1 old, e2 default, e3 latest; done is e4 final, e5
	// default, e6 close.
	chip := `<span class="rat" role="button" tabindex="0" title=`
	full := render(t, BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"}))
	wantAll(t, full, `class="trecap" data-entry="e3" data-kind="agent"><span class="rat" role="button" tabindex="0" title=`,
		`class="wb-entry agent current" id="e3" data-history-anchor="e3" data-entry="e3"`,
		// A closed topic's folded recap jumps into its (closed) stream too.
		`class="trecap" data-entry="e4" data-kind="agent"><span class="rat" role="button" tabindex="0" title=`, `id="e4"`)
	if n := strings.Count(full, chip); n != 2 {
		t.Fatalf("full page recap chips = %d, want 2 (one per agent recap)", n)
	}
	wantNone(t, full, `class="trecap" data-entry="e3" data-kind="agent" role=`, `class="trecap" data-entry="e4" data-kind="agent" role=`)
	embed := render(t, BuildView(st, ViewInput{Layout: LayoutEmbed, ReplyID: "r"}))
	wantAll(t, embed, `class="trecap" data-entry="e3" data-kind="agent"><span class="rat" title=`)
	if strings.Contains(embed, chip) {
		t.Fatal("embed recap chip must not be a jump button: the embed has no log rows")
	}
	css, err := embeddedAsset("page.css")
	if err != nil {
		t.Fatal(err)
	}
	wantAll(t, string(css), `.trecap:has(> .rat[role="button"]) { cursor: pointer; }`,
		`.trecap > .rat[role="button"]:focus-visible { outline: 2px solid var(--wb-primary); outline-offset: -2px;`)
}

// TestRecapJumpScriptUnderNode drives the recap-jump block of stream.js
// against a fake page: the chip's click or Enter/Space loads its row through
// wbHistory.loadTarget at once; a plain text click does so once the
// double-click interval passes, and a real double-click (down, click 1,
// down, click 2) never does. Links, drags, selections, and modified clicks
// keep their own behavior, and the loader's flash is the only highlight.
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
var CHIP='.trecap[data-entry] > .rat[role="button"]', INNER='a[href], button, input, textarea, select, summary, label, [contenteditable]', chipOn=true;
function el(name,parent){var cls=new Set();return {name:name,parent:parent,parentElement:parent,isConnected:true,
  classList:{add:function(c){cls.add(c);},remove:function(c){cls.delete(c);},contains:function(c){return cls.has(c);}},
  closest:function(s){for(var n=this;n;n=n.parent){if(s==='.trecap[data-entry]'&&n.name==='recap')return n;if(s==='.rat[role="button"]'&&n.name==='chip'&&chipOn)return n;
    if(s==='.topic'&&n.name==='topic')return n;if(s===INNER&&n.name==='link')return n;}return null;},
  matches:function(s){return s===CHIP&&this.name==='chip'&&chipOn;},
  querySelector:function(s){return s===':scope > .rat[role="button"]'&&this.name==='recap'&&chipOn?chip:null;},
  contains:function(o){for(var n=o;n;n=n.parent)if(n===this)return true;return false;},
  getAttribute:function(a){return a==='data-entry'&&this.name==='recap'?'e2':null;}};}
var topic=el('topic',null), recap=el('recap',topic), chip=el('chip',recap), text=el('text',recap), link=el('link',text), other=el('other',topic), row=el('row',topic);
var listeners={}, loads=[], timers=[], found=row, unfold=function(){};
var selection={isCollapsed:true,rangeCount:0,getRangeAt:function(){return {intersectsNode:function(n){return selection.over===n;}};}};
var document={addEventListener:function(type,fn){(listeners[type]=listeners[type]||[]).push(fn);}};
var window={getSelection:function(){return selection;},
  setTimeout:function(fn,ms){timers.push({fn:fn,ms:ms,live:true});return timers.length;},clearTimeout:function(id){if(id&&timers[id-1])timers[id-1].live=false;},
  WsCards:{unfoldTopic:unfold},
  wbHistory:{loadTarget:function(root,id,before){loads.push([root.name,id,before===unfold]);if(found)found.classList.add('flash');return found;}}};
// run fires the live timers of one delay, as if that much time passed.
function run(ms){timers.forEach(function(t){if(t.live&&t.ms===ms){t.live=false;t.fn();}});}
function fire(type,e){e.prevented=false;e.preventDefault=function(){e.prevented=true;};(listeners[type]||[]).forEach(function(fn){fn(e);});return e;}
function click(target,o){o=o||{};var x=o.x||0,y=o.y||0;
  fire('pointerdown',{target:o.down||target,clientX:o.dx||0,clientY:o.dy||0,button:0});
  return fire('click',Object.assign({target:target,clientX:x,clientY:y,button:o.button||0,detail:o.detail===undefined?1:o.detail,defaultPrevented:!!o.prevented},o.mods||{}));}
function key(target,k,o){o=o||{};return fire('keydown',Object.assign({target:target,key:k},o));}
function took(label,want){run(500);check(label,loads.length,want);loads=[];}
` + block + `
click(chip);check('a chip click loads the recap row at once',loads,[['topic','e2',true]]);loads=[];
click(text);check('a text click waits out the double-click interval',loads.length,0);
run(500);check('then loads the recap row',loads,[['topic','e2',true]]);loads=[];
click(text);click(text,{detail:2});took('a double-click (down, click 1, down, click 2) never jumps',0);
click(text);click(text,{detail:2});click(text,{detail:3});took('a triple-click never jumps',0);
click(text);click(other);took('a later press elsewhere drops the pending jump',0);
click(chip);click(chip,{detail:2});took('a chip double-click jumps once',1);
row.classList.add('wb-log-blink');click(chip);loads=[];
check('the jump removes the arrival blink that would mask and replay the flash',row.classList.contains('wb-log-blink'),false);
check('the loader flash is the highlight',row.classList.contains('flash'),true);
run(2000);check('the flash class does not outlive one run',row.classList.contains('flash'),false);
click(chip);click(chip);check('a second jump restarts one flash timer',timers.filter(function(t){return t.live&&t.ms===2000;}).length,1);loads=[];run(2000);
['metaKey','ctrlKey','altKey','shiftKey'].forEach(function(m){var mods={};mods[m]=true;
  click(text,{mods:mods});took(m+'-click on the text is not a jump',0);
  click(chip,{mods:mods});took(m+'-click on the chip is not a jump',0);});
click(link);took('a link inside the recap keeps its own click',0);
click(text,{x:30,y:0});took('a drag selects instead of jumping',0);
click(text,{prevented:true});took('a consumed click is left alone',0);
click(text,{button:1});took('a middle click is not a jump',0);
click(other);took('clicks outside the recap are ignored',0);
selection.isCollapsed=false;selection.rangeCount=1;selection.over=recap;
click(text);took('a selection over the recap blocks the jump',0);
selection.over=other;click(text);took('a selection elsewhere does not',1);
selection.isCollapsed=true;selection.rangeCount=0;selection.over=null;
click(text);selection.isCollapsed=false;selection.rangeCount=1;selection.over=recap;took('a selection made before the deferred jump blocks it',0);
selection.isCollapsed=true;selection.rangeCount=0;selection.over=null;
var e=key(chip,'Enter');took('Enter on the chip jumps',1);check('Enter is consumed',e.prevented,true);
e=key(chip,' ');took('Space on the chip jumps',1);check('Space does not scroll the page',e.prevented,true);
e=key(recap,'Enter');took('the recap itself is not the keyboard button',0);
e=key(link,'Enter');took('Enter on a recap link follows the link',0);check('link Enter untouched',e.prevented,false);
key(chip,'Enter',{defaultPrevented:true});took('a consumed Enter is left alone',0);
key(chip,'Enter',{isComposing:true});took('IME composition is not a jump',0);
key(chip,'Enter',{shiftKey:true});took('modified Enter is not a jump',0);
key(chip,'a');took('other keys are ignored',0);
chipOn=false;click(text);took('a recap without a chip button is plain text',0);
key(chip,'Enter');took('nor a keyboard target',0);chipOn=true;
found=null;click(chip);took('a missing row is tolerated',1);
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
