package wsdashboard

import (
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// selectionCodecSource reuses the production text codec, including its opaque
// malformed-envelope scanner; these harnesses define no parallel wire grammar.
func selectionCodecSource(t *testing.T) string {
	t.Helper()
	b, err := dashboardFS.ReadFile("assets/dashboard-bundles.js")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func selectionImagesSource(t *testing.T) string {
	t.Helper()
	src := mustAsset(t, "dashboard.js")
	block := regexp.MustCompile(`(?s)\n  // --- composer images -.*?\n  // --- composer file mentions`).FindString(src)
	if block == "" {
		t.Fatal("missing image section")
	}
	return block
}

// selectionActualSource extracts actual bindings and complete helpers in their
// original source order. Only unrelated page startup/event wiring is omitted;
// no replacement composer state or helper implementation is injected.
func selectionActualSource(t *testing.T, source string, variables, functions []string, composer bool) string {
	t.Helper()
	type piece struct{ start, end int }
	var pieces []piece
	for _, prefix := range variables {
		match := regexp.MustCompile(`(?m)^  var ` + regexp.QuoteMeta(prefix) + `[^\n]*\n`).FindStringIndex(source)
		if match == nil {
			t.Fatalf("missing actual declaration %s", prefix)
		}
		pieces = append(pieces, piece{match[0], match[1]})
	}
	if composer {
		for _, p := range regexp.MustCompile(`(?m)^  var [^\n]*\n`).FindAllStringIndex(source, -1) {
			pieces = append(pieces, piece{p[0], p[1]})
		}
		for _, m := range regexp.MustCompile(`(?m)^  function ([A-Za-z0-9_$]+)\(`).FindAllStringSubmatch(source, -1) {
			functions = append(functions, m[1])
		}
		for _, statement := range []string{"  if (ctext && window.WsCapsules) window.WsCapsules.attach(ctext);", "  if (ctext && window.WsCapsules) window.WsCapsules.topicPicker(ctext, capsuleTopics);"} {
			at := strings.Index(source, statement)
			if at < 0 {
				t.Fatal("missing production capsule initialization")
			}
			pieces = append(pieces, piece{at, at + len(statement)})
		}
	}
	for _, name := range functions {
		start := strings.Index(source, "  function "+name+"(")
		if start < 0 {
			t.Fatalf("missing actual helper %s", name)
		}
		end := 0
		if m := regexp.MustCompile(`^  function ` + regexp.QuoteMeta(name) + `\([^\n]*\) \{[^\n]*\}\n`).FindStringIndex(source[start:]); m != nil {
			end = start + m[1]
		} else {
			close := strings.Index(source[start:], "\n  }\n")
			if close < 0 {
				t.Fatalf("missing helper close %s", name)
			}
			end = start + close + 5
		}
		pieces = append(pieces, piece{start, end})
	}
	sort.Slice(pieces, func(i, j int) bool { return pieces[i].start < pieces[j].start })
	var out strings.Builder
	for _, p := range pieces {
		out.WriteString(source[p.start:p.end])
		out.WriteByte('\n')
	}
	return out.String()
}

func selectionProductionRuntime(t *testing.T) string {
	t.Helper()
	read := func(name string) string {
		b, err := dashboardFS.ReadFile("assets/" + name)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	base := read("dashboard.js")
	// Follow pages.go's fragment order, including the real initialization
	// bindings that the former function-only harnesses accidentally omitted.
	source := selectionActualSource(t, base, []string{"doc =", "mode =", "rid =", "shell =", "store =", "openState =", "PEER_KEY =", "SHUT_KEY =", "convo =", "grid ="}, []string{"q", "$", "load", "save", "region", "boardBoxes", "boardQ", "inBoard", "boardRead", "visible", "atBottom", "pinConvo", "followConvo", "isPanel", "shutList", "shutHas", "isPeer", "openKey", "restoreOpenOne", "restoreOpen", "postDoc", "pickKey", "readAnswers", "openNote", "setCheckNo", "writeAnswers"}, false)
	// Keep the actual guarded browser storage initialization too.
	storageInit := regexp.MustCompile(`(?m)^  try \{ store = window\.localStorage;[^\n]*\n`).FindString(base)
	if storageInit == "" {
		t.Fatal("missing production storage initialization")
	}
	source += storageInit
	source += selectionActualSource(t, read("dashboard-board.js"), []string{"boardSeen =", "boardOwn =", "boardChanges ="}, []string{"boardFlushViewed"}, false)
	source += read("dashboard-bundles.js")
	source += selectionActualSource(t, read("dashboard-composer.js"), nil, nil, true)
	source += selectionActualSource(t, read("dashboard-tracker.js"), []string{"capsuleTopicLoad ="}, []string{"paletteText", "capsuleTopics"}, false)
	source += selectionActualSource(t, read("dashboard-rail.js"), []string{"railEl =", "railStore ="}, nil, false)
	source += selectionDraftSource(t)
	return source
}

// The DOM/Range adapter is shared with the existing actual-capsule fixture.
// Modules are loaded as shipped: toast, mention/capsules, rail, then the
// selected dashboard fragments. Browser APIs/storage/fetch alone are synthetic.
func runSelectionRuntime(t *testing.T, name, body string) {
	t.Helper()
	quoted := func(s string) string {
		b, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	runNode(t, name, `
const fs=require('node:fs'), path=require('node:path'), vm=require('node:vm');
const localRequire=require('node:module').createRequire(path.resolve('testdata/capsules.test.cjs'));
const fixture=fs.readFileSync('testdata/capsules.test.cjs','utf8');
const boundary=fixture.indexOf("vm.runInContext(fs.readFileSync(path.join(root, 'mention.js')");
if(boundary<0)throw new Error('missing shared DOM/module boundary');
const prefix=fixture.slice(0,boundary);
const host=new Function('require','__dirname',prefix+'\nreturn {context,doc,El,Evt,advance,selection};')(localRequire,path.resolve('testdata'));
Object.assign(host.context,{require,process,TextEncoder,El:host.El,Event:host.Evt,setImmediate,advance:host.advance});
vm.runInContext(`+quoted(selectionRuntimeEnvironment)+`,host.context);
['toast.js','mention.js','capsules.js','rail.js'].forEach(n=>vm.runInContext(fs.readFileSync(path.join('assets',n),'utf8'),host.context));
vm.runInContext(`+quoted(selectionProductionRuntime(t))+`,host.context);
Promise.resolve(vm.runInContext(`+quoted("(async function(){\n"+body+"\n})()")+`,host.context)).catch(e=>{console.error(e);process.exitCode=1;});
`)
}

const selectionRuntimeEnvironment = `
var bad=[];
// Fixture elements are built in the host realm while expectations are built in
// the vm realm, so their Array prototypes differ and isDeepStrictEqual rejects
// identical values. Normalize both sides into this realm; undefined stays distinct.
function norm(v){return JSON.parse(JSON.stringify(v,function(k,x){return x===undefined?'\u0000undefined':x;}));}
function check(what,got,want){if(!require('util').isDeepStrictEqual(norm(got),norm(want)))bad.push(what+': got '+JSON.stringify(got)+', want '+JSON.stringify(want));}
function selectionValue(s){return {mentions:s.mentions,comment:s.comment,images:s.images};}
Object.defineProperty(El.prototype,'classList',{get:function(){var el=this;return {contains:function(c){return el.className.split(/\s+/).indexOf(c)>=0;},add:function(c){if(!this.contains(c))el.className+=' '+c;},remove:function(c){el.className=el.className.split(/\s+/).filter(function(x){return x!==c;}).join(' ');},toggle:function(c,on){if(on===undefined)on=!this.contains(c);if(on)this.add(c);else this.remove(c);return on;}};}});
El.prototype.matches=function(selector){var el=this;return selector.split(',').some(function(s){s=s.trim();var tag=/^[a-zA-Z][a-zA-Z0-9-]*/.exec(s);if(tag&&el.tagName!==tag[0].toUpperCase())return false;var id=/#([\w-]+)/.exec(s);if(id&&(el.getAttribute('id')||el.id)!==id[1])return false;var classes=s.match(/\.([\w-]+)/g)||[];if(classes.some(function(c){return !el.classList.contains(c.slice(1));}))return false;var re=/\[([\w-]+)(?:=["']?([^\]"']*)["']?)?\]/g,m;while((m=re.exec(s))){if(!el.hasAttribute(m[1])||m[2]!==undefined&&el.getAttribute(m[1])!==m[2])return false;}return true;});};
El.prototype.insertBefore=function(n,before){if(n.parentNode)n.parentNode.removeChild(n);var i=this.childNodes.indexOf(before);if(i<0)i=this.childNodes.length;this.childNodes.splice(i,0,n);n.parentNode=this;return n;};
El.prototype.getClientRects=function(){return this.hidden?[]:[this.getBoundingClientRect()];};
El.prototype.scrollIntoView=function(){};
El.prototype.getBoundingClientRect=function(){return {left:0,top:0,right:4,bottom:4,height:20};};
Event.prototype.stopPropagation=function(){this.propagationStopped=true;};
document.documentElement=new El('html');document.documentElement.setAttribute('data-mode','live');document.documentElement.setAttribute('data-rid','sess1');document.documentElement.appendChild(document.body);
document.querySelectorAll=function(s){return this.documentElement.querySelectorAll(s);};document.querySelector=function(s){return this.querySelectorAll(s)[0]||null;};document.getElementById=function(id){return this.querySelectorAll('#'+id)[0]||null;};document.contains=function(n){return this.documentElement.contains(n);};document.addEventListener=function(t,f){(this.listeners||(this.listeners={}))[t]=f;};document.hidden=false;
function element(tag,id,parent){var el=new El(tag);if(id)el.setAttribute('id',id);(parent||document.body).appendChild(el);return el;}
var composerElement=element('div','composer');element('div','cchips',composerElement);element('textarea','ctext',composerElement);element('div','cimgs',composerElement);element('div','r-board');element('div','grid').setAttribute('data-tab','convo');element('div','tabbar');
var storage=new Map();window.localStorage={getItem:function(k){return storage.has(k)?storage.get(k):null;},setItem:function(k,v){storage.set(k,String(v));},removeItem:function(k){storage.delete(k);}};
var URL={createObjectURL:function(f){return 'blob:'+f.name;},revokeObjectURL:function(){}};
function FileReader(){}FileReader.prototype.readAsDataURL=function(f){this.result='data:'+f.type+';base64,QUJD';this.onload();};
function file(name){return {name:name,type:'image/png',size:3};}
function rowNums(row){return row?row.children.map(function(c){return c.children[1].textContent;}):null;}
function toastTexts(){return document.querySelectorAll('.wst-body').map(function(el){return el.textContent;});}
function wsT(k,a){return a===undefined?k:k+':'+a;}
var pendingFetch=[],bodies=[],sent=0;
function fetch(url,options){sent++;bodies.push(JSON.parse(options.body));return new Promise(function(resolve,reject){pendingFetch.push({resolve:resolve,reject:reject,url:url});});}
async function respond(ok,value){var request=pendingFetch.shift();if(!request)throw new Error('no pending request');request.resolve({ok:ok,status:ok?200:400,text:function(){return Promise.resolve(typeof value==='string'?value:JSON.stringify(value));}});await new Promise(setImmediate);}
`

func TestSelectionComposerPopupImagesAndOwnership(t *testing.T) {
	runSelectionRuntime(t, "selection popup ownership", `
var source = {topic:"plan",entry:"e7",id:"3",text:"captured <source> [Image #1]"};
var sourceFrame = doc.body.appendChild(new El("iframe"));
placeMention({ms:[source],frame:sourceFrame,x:4,y:5});
check("actual topic-only capsule editor is reused", [!!mnote.wsc,window.WsCapsules.command(mnote)], [true,null]);
lockMention(); source.text = "changed live source";
doc.body.removeChild(sourceFrame); placeMention(mpend);
check("captured frame can disappear before attach", [mpend.ms[0].entry,mpend.screenX,mpend.screenY], ["e7",4,5]);
placeMention({ms:[{topic:"other",text:"replacement"}],x:90,y:90});
check("locked copied source survives changed selection", mpend.ms[0].text, "captured <source> [Image #1]");
mnote.value = "memo [Image #1]";
var paste = new Event("paste",{clipboardData:{files:[file("paste.png")],types:[]}});
var drop = new Event("drop",{dataTransfer:{files:[file("drop.png")],types:["Files"]}});
mnote.dispatchEvent(paste); advance(0); mbtn.dispatchEvent(drop);
check("paste/drop prevent default and drop propagation", [paste.defaultPrevented,drop.defaultPrevented,drop.propagationStopped], [true,true,true]);
check("popup images are actual refs, not memo markers", [mpend.images,mnote.value,ctext.value,rowNums(mimgs),rowNums(cimgs)], [[1,2],"memo [Image #1]","",["#1","#2"],[]]);
check("popup has not attached or sent anything", bselections.length, 0);
unattach(imgs[0]);
check("thumbnail removal touches only actual ref", [mpend.images,mnote.value,mpend.ms[0].text], [[2],"memo [Image #1]","captured <source> [Image #1]"]);
addMention();
check("one push one group with one memo", [bselections.length,bselections[0].comment,bselections[0].images,ctext.value,doc.contains(sourceFrame)], [1,"memo [Image #1]",[2],"",false]);
check("attached source bytes remain copied",bselections[0].mentions[0].text,"captured <source> [Image #1]");
check("group shows source identity and editable memo", [cchips.querySelector(".selection-source").getAttribute("data-wb-entry"),cchips.querySelector(".selection-note").title,rowNums(bselections[0].row)], ["e7","memo [Image #1]",["#2"]]);
var original = bselections[0];
editSelection(0); mnote.value = "edited"; addMention();
check("editing replaces the group object", [bselections[0] !== original,bselections[0].comment,bselections[0].images], [true,"edited",[2]]);
editSelection(0); addFiles([file("abandoned.png")],mnote); hideMention();
check("cancel edit retains committed image only", [imgs.map(function(p){return p.n;}),bselections[0].images], [[2],[2]]);
placeMention({ms:[{topic:"plan",entry:"e8",text:"second"}],x:1,y:1});
mnote.value = "different"; addFiles([file("second.png")],mnote); addMention();
check("multiple pushes have separate images and memos", bselections.map(function(s){return [s.comment,s.images];}), [["edited",[2]],["different",[4]]]);
dropSelection(0);
check("removal does not steal next group's image", [bselections.map(function(s){return s.comment;}),imgs.map(function(p){return p.n;})], [["different"],[4]]);
placeMention({ms:[{topic:"plan",text:"cancel [Image #4]"}],x:1,y:1});
mnote.value = "literal [Image #4]"; addFiles([file("cancel.png")],mnote); hideMention();
check("cancel popup preserves other group's actual ref", [imgs.map(function(p){return p.n;}),bselections[0].images], [[4],[4]]);
placeMention({ms:[{topic:"plan",text:"limits"}],x:1,y:1});
addFiles([{name:"bad",type:"image/svg+xml",size:1},{name:"huge",type:"image/png",size:IMG_BYTES+1}],mnote);
check("unsupported type and oversized bytes rejected", [mpend.images.length,toastTexts().length], [0,1]);
for(var i=0;i<8;i++) addFiles([file("cap"+i)],mnote);
check("shared eight-image cap includes attached group", [imgs.length,mpend.images.length,toastTexts()[0]], [8,7,"js.img.refused:js.img.max:8"]);
hideMention();
check("cancel cap popup releases only its images", imgs.map(function(p){return p.n;}), [4]);
process.stdout.write(bad.join("\n"));
`)
}

func TestSelectionComposerCompactMemoEditor(t *testing.T) {
	runSelectionRuntime(t, "compact selection memo editor", `
ctext.value = "ordinary text stays exact";
var first = {mentions:[{topic:"plan",entry:"e7",text:"first"},{topic:"plan",entry:"e8",text:"second"}],comment:"",images:[]};
bselections = [first]; renderChips();
var group = cchips.children[0], open = group.querySelector("[data-selection-edit]"), remove = group.querySelector("[data-unselection]");
check("one compact body holds all sources with separate remove", [cchips.children.length,group.classList.contains("selection-chip"),open.querySelectorAll(".selection-source").length,remove.textContent], [1,true,2,"×"]);
check("empty memo has no indicator or inline input", [open.querySelector(".selection-note").hidden,group.querySelectorAll("textarea").length], [true,0]);
editSelection(0);
check("body opens existing anchored editor, not attachment", [mpend.edit===first,mbtn.hidden,mbtn.querySelector("[data-wbmention]").hidden,mnote.value,bselections.length], [true,false,true,"",1]);
mnote.value = "saved on Escape\n한글";
mbtn.dispatchEvent(new Event("keydown",{key:"Escape"}));
check("Escape saves without changing composer or source grouping", [mpend,mbtn.hidden,bselections.length,bselections[0].comment,bselections[0].mentions,ctext.value], [null,true,1,"saved on Escape\n한글",first.mentions,"ordinary text stays exact"]);
check("memo is a subtle titled indicator", [cchips.querySelector(".selection-note").hidden,cchips.querySelector(".selection-note").title], [false,"saved on Escape\n한글"]);
editSelection(0); mnote.value = "saved on blur";
mbtn.dispatchEvent(new Event("focusout",{relatedTarget:mnote.wsc.editor}));
check("internal focus does not save or dismiss", [!!mpend,mbtn.hidden], [true,false]);
var before = bselections[0], retained = cchips.children[0], retainedRemove = retained.querySelector("[data-unselection]");
mbtn.dispatchEvent(new Event("focusout",{relatedTarget:retainedRemove}));
check("blur replaces snapshot but keeps pressed chip controls", [mpend,bselections[0]!==before,bselections[0].comment,cchips.children[0]===retained,cchips.querySelector("[data-unselection]")===retainedRemove,cchips.querySelector(".selection-note").title], [null,true,"saved on blur",true,true,"saved on blur"]);
editSelection(0); mnote.value = "   ";
mbtn.dispatchEvent(new Event("focusout",{relatedTarget:ctext.wsc.editor}));
check("whitespace memo hides indicator without trimming saved bytes", [bselections[0].comment,cchips.querySelector(".selection-note").hidden], ["   ",true]);
check("presentation edits do not send or write composer text", [sent,ctext.value], [0,"ordinary text stays exact"]);
// The only deletion action remains the existing explicit remove control.
dropSelection(0); check("explicit remove deletes bundle", bselections.length,0);
process.stdout.write(bad.join("\n"));
`)
}

func TestSelectionComposerSendSnapshotsAndCancel(t *testing.T) {
	runSelectionRuntime(t, "selection send snapshots", `
var first = {mentions:[{topic:"plan",entry:"e7",text:"literal [Image #1]"}],comment:"memo [Image #1]",images:[1]};
bselections = [first]; imgs = [{n:1,name:"one",data:"QUJD",ref:"",url:"blob:one"}];
placeMention({ms:[{topic:"plan",entry:"e8",text:"pending"}],x:1,y:1});
addFiles([file("popup-only")],mnote);
check("attachment alone has no network action",sent,0);
sendMessage();
check("explicit send excludes popup-only image", [bodies[0].images.map(function(p){return p.n;}),splitSelectionText(bodies[0].text).groups], [[1],[first]]);
await respond(false,"refused");
check("failure retains group and every local image", [bselections[0]===first,imgs.length], [true,2]);
sendMessage();
editSelection(0); mnote.value = "edited while sending"; addMention();
var edited = bselections[0];
placeMention({ms:[{topic:"plan",entry:"e9",text:"new push"}],x:1,y:1});
mnote.value = "later"; addFiles([file("later")],mnote); addMention();
await respond(true,{});
check("success retains replacement edit and concurrent push", [bselections[0]===edited,bselections.map(function(s){return s.comment;}),imgs.map(function(p){return p.n;})], [true,["edited while sending","later"],[1,3]]);
sendMessage(); await respond(true,{});
check("next explicit send clears only its snapshot", [bselections.length,imgs.length], [0,0]);

// Two independent held messages may each name image 1. Only their real
// image tails remap into this local combined restoration, never source/memo.
var a = {mentions:[{topic:"plan",entry:"e10",text:"a [Image #1]"}],comment:"memo [Image #1]",images:[1]};
var b = {mentions:[{topic:"plan",entry:"e11",text:"b [Image #1]"}],comment:"other [Image #1]",images:[1]};
ctext.value = ""; imgSeq = 0;
restoreHeld([{text:selectionPrefix([a]),images:["written-a.png"],nums:[1]},{text:selectionPrefix([b]),images:["written-b.png"],nums:[1]}]);
check("independent image-1 collisions remap actual refs only", bselections.map(function(s){return [s.mentions[0].text,s.comment,s.images];}), [["a [Image #1]","memo [Image #1]",[1]],["b [Image #1]","other [Image #1]",[2]]]);
check("written references survive cancel", imgs.map(function(p){return [p.n,p.ref];}), [[1,"written-a.png"],[2,"written-b.png"]]);
sendMessage();
check("resend uses written refs with local numbers", [bodies[bodies.length-1].refs,bodies[bodies.length-1].refNums], [["written-a.png","written-b.png"],[1,2]]);
await respond(true,{});
check("successful resend releases restored groups", bselections.length,0);
process.stdout.write(bad.join("\n"));
`)
}

func TestSelectionComposerDraftTextOnlyAndLostBytes(t *testing.T) {
	runSelectionRuntime(t, "selection drafts", `
var group = {mentions:[{topic:"plan",entry:"e7",text:"source [Image #2] <literal>"}],comment:"memo [Image #2] &amp;",images:[1,2]};
bselections = [group]; ctext.value = "@bob ordinary";
mentions = [{start:0,end:4,text:"@bob"}]; ctext.setSelectionRange(2,4);
// A real capsule must match its canonical backing bytes. This non-leading
// skill keeps the original "@bob ordinary" bytes and range 5..13 exactly.
var rich = [{start:5,end:13,kind:"skill",id:"ordinary",label:"ordinary"}];
ctext.wsc.restore(rich);
check("actual rich identity is valid before saving",ctext.wsc.snapshot(),rich);
imgs = [{n:1,name:"written",ref:"written.png"},{n:2,name:"unsent",data:"QUJD"}];
var prefix = selectionPrefix(bselections), saved = readDraft();
check("groups use existing text only", [saved.text,Object.prototype.hasOwnProperty.call(saved,"groups"),Object.prototype.hasOwnProperty.call(saved,"bselections")], [prefix+ctext.value,false,false]);
check("rich capsule and legacy mention offsets shift with prefix", [saved.capsules[0].start,saved.mentions[0].start,saved.sel], [prefix.length+5,prefix.length,[prefix.length+2,prefix.length+4]]);
saveDraft();
// Another session sees neither this group's source nor its references.
rid = "sess2"; bselections=[]; ctext.value=""; imgs=[]; mentions=[];
restoreDraft(); check("session isolation", [bselections.length,ctext.value,imgs.length], [0,"",0]);
rid = "sess1"; restoreDraft();
require('assert').deepStrictEqual(bselections.map(selectionValue), [{mentions:group.mentions,comment:group.comment,images:[1]}], "reload loses only unsent image tails");
check("source and memo marker-like text survive loss", [bselections[0].mentions[0].text,bselections[0].comment], ["source [Image #2] <literal>","memo [Image #2] &amp;"]);
check("written refs and ordinary text restore", [imgs.map(function(p){return [p.n,p.ref];}),ctext.value,toastTexts()], [[[1,"written.png"]],"@bob ordinary",["js.draft.imgslost:1"]]);
check("rich ranges and caret restore without prefix offsets", [ctext.wsc.snapshot(),mentions,[ctext.selectionStart,ctext.selectionEnd]], [rich,[{start:0,end:4,text:"@bob"}],[2,4]]);
// Legacy persisted drafts still have precisely the old text/range shape.
bselections=[]; ctext.value=""; imgs=[]; mentions=[];
draftStore.save(rid,{text:"legacy text",mentions:[],sel:[3,3]}); restoreDraft();
check("old text shape remains ordinary", [ctext.value,bselections,[ctext.selectionStart,ctext.selectionEnd]], ["legacy text",[],[3,3]]);
process.stdout.write(bad.join("\n"));
`)
}

func selectionDraftSource(t *testing.T) string {
	t.Helper()
	block := regexp.MustCompile(`(?s)\n  // --- per-session drafts -.*?\n\n  // --- iframe shell`).FindString(mustAsset(t, "dashboard.js"))
	if block == "" {
		t.Fatal("missing draft section")
	}
	return strings.TrimSuffix(block, "// --- iframe shell")
}
