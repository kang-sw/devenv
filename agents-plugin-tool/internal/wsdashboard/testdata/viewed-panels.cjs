// Deliberately small geometry/immutable-input model, not browser evidence.
var bad = [];
function check(what, ok) { if (!ok) bad.push(what); }
function panel(id, bytes, exposed) {
  return {id:id, outerHTML:bytes, exposed:!!exposed, classList:{contains:function(){return false;}}, getAttribute:function(){return null;}};
}
function topic(key, hash, panels, height) {
  return {
    nodeType:1, tagName:"SECTION", id:"", panels:panels || [], templates:[], height:height || 100,
    classList:{contains:function(c){return c === "topic";}},
    hasAttribute:function(k){return k === "data-topic" || k === "data-th";},
    getAttribute:function(k){return k === "data-topic" ? key : k === "data-th" ? hash : null;},
    cloneNode:function(){return topic(key,hash,this.panels.slice(),this.height);}
  };
}
function q(sel,n) {
  if (sel === "template") return n.templates || [];
  if (sel === ".entry-panel, .wb-entry:not(.panel), .doc-frame") return n.panels || [];
  throw new Error("unmodeled selector: " + sel);
}
var main = {name:"main"}, pinScroller = {name:"pins"};
var window = {wbHistory:{keep:function(n,opt){return {
  scroller:n.scroller || main,
  visible:function(p){return !!p.exposed;},
  viewport:function(){return {top:0,bottom:200};}
};}}};
var board = {children:[],childNodes:[]}, pins = {children:[],childNodes:[]};
function boardBoxes(){return [board,pins];}
function setLive(a,b){board.children=board.childNodes=a;pins.children=pins.childNodes=b || [];}
function input(list){var topics=Object.create(null);list.forEach(function(n){topics[n.getAttribute("data-topic")]=n;});return {topics:topics,order:list.map(function(n){return n.getAttribute("data-topic");}),before:[],after:[]};}
var boardPending=Object.create(null),boardFrozen=[],boardOwn=Object.create(null),boardImmediate=Object.create(null),boardRelease=false;
var boardRendered=null,boardLatest=null,boardOffered=null,boardChanges=[],boardHeightHeld=new Map(),boardHeightChanges=new Map(),boardBadge=null,boardReleaseFrame=0;
function keys(list){return list.map(function(n){return n.getAttribute("data-topic");}).join();}
function geometry(list,scroll){var y=0,out={};list.forEach(function(n){out[n.getAttribute("data-topic")]=y-scroll;y+=n.height;});return out;}
function project(current,want,exposed){return boardStageOrder(current,want,exposed);}
// A single owner delta is modeled only after the production staging projection.
function freezeCheck(current,want,exposed,label){
  var before=geometry(current,0),staged=project(current,want,exposed),after=geometry(staged,0);
  var delta=after[exposed[0].getAttribute("data-topic")]-before[exposed[0].getAttribute("data-topic")];
  after=geometry(staged,delta);
  exposed.forEach(function(n){var k=n.getAttribute("data-topic");check(label+":"+k,after[k]===before[k]);});
  return staged;
}
var doc={body:{appendChild:function(){}},createElement:function(){return {addEventListener:function(kind,fn){this.click=fn;}};}};
