package wsdashboard

import "testing"

func TestConversationPresentationValidatedBoardNavigation(t *testing.T) {
	src := whiteboardBridgeFunctions(t, string(mustAsset(t, "dashboard.js")), "effectTopic", "effectJump", "effectJumpTo", "effectJumpClick", "effectJumpKey")
	runNode(t, "conversation effect navigation", `
var bad=[],effectJumpEpoch=0,topics=[],more=null,loads=[],scrolls=[],focused=[],reveals=[],results=[],tabs=[],pendingReveal=null,closedLoads=0,flashes=[],timers=[];
function setTimeout(fn){timers.push(fn);}
function check(label,got,want){if(JSON.stringify(got)!==JSON.stringify(want))bad.push(label+': '+JSON.stringify(got)+' != '+JSON.stringify(want));}
var doc={body:{classList:{contains:function(){return false;}}}},tkApi=null,grid={},root={getAttribute:function(){return '0';}};
function tabShown(){return false;}function setTab(t){tabs.push(t);}function wsT(k){return k;}
function inBoard(n){return !!n&&n.owned;}
function boardQ(s){return s.indexOf('button.')===0?(more?[more]:[]):topics;}
function node(id,topic,history){return {id:id,owned:true,tagName:'DIV',parentNode:topic||null,topic:topic,history:!!history,
 classList:{remove:function(c){flashes.push(['remove',id,c]);},add:function(c){flashes.push(['add',id,c]);}},
 closest:function(s){return s==='.wb-history'?(this.history?{}:null):s==='section.topic[data-topic]'?(this.topic||this):null;},
 matches:function(s){return s.indexOf('.card.policy')>=0?this.id.indexOf('item-')===0:false;},
 querySelector:function(s){var m=/\[id="([^"]+)"\]/.exec(s);return m&&this.targets?this.targets[m[1]]||null:null;},
 setAttribute:function(){},focus:function(o){focused.push([this.id,o.preventScroll]);},scrollIntoView:function(){scrolls.push(this.id);}};}
function topic(id){var n=node(id);n.tagName='SECTION';n.targets={};topics.push(n);return n;}
var a=topic('topic-6161'),b=topic('topic-6262');
var e=node('e7',a,true),p=node('p4',a,true),item=node('item-42',a);a.targets={'e7':e,'p4':p,'item-42':item};
var window={WsCards:{unfoldTopic:function(n){reveals.push(n.id);},revealTopic:function(n,done){if(pendingReveal===true)pendingReveal=done;else done();}},
 wbHistory:{loadTarget:function(topic,id,before){loads.push([topic.id,id]);var n=topic.targets[id];if(n&&before)before(n);return n||null;},keep:function(){}},getSelection:function(){return {isCollapsed:true};}};
function loadClosed(btn){closedLoads++;if(btn.load)btn.load();return Promise.resolve(true);}
function done(topic,exact){results.push([topic,exact]);}
var status={textContent:''},card={getAttribute:function(){return 'topic-6161';},querySelector:function(){return status;}},link=null,row=null,interactive=false;
var containedCard=card,box={contains:function(n){return n===containedCard;}};function entriesBox(){return box;}
function target(){return {closest:function(s){if(s==='.wbupdate-topic[data-wb-topic]')return card;if(s==='a[data-wb-jump]')return link;if(s==='.wbupdate-row[data-wb-element]')return row;if(s.indexOf('a, summary, button, input, textarea, select')>=0)return interactive?{}:null;return null;}};}
function click(){var event={target:target(),button:0,preventDefault:function(){this.prevented=true;}};return event;}
function keydown(k){return {target:target(),key:k,preventDefault:function(){this.prevented=true;}};}
`+src+`
(async function(){
 effectJump(a.id,a.id,done);
 check('topic opens board and reaches topic',[tabs,scrolls,results],[['board'],[a.id],[[true,true]]]);
 effectJump(a.id,'e7',done);effectJump(a.id,'p4',done);effectJump(a.id,'item-42',done);
 check('item and retained log/panel share topic-scoped ID-chip loader',loads,[[a.id,'e7'],[a.id,'p4'],[a.id,'item-42']]);
 check('topic-only navigation restarts the existing brief pulse',flashes.slice(0,2),[['remove',a.id,'palette-flash'],['add',a.id,'palette-flash']]);
 timers.shift()();check('topic pulse is removed after timeout',flashes[flashes.length-1],['remove',a.id,'palette-flash']);
 check('history geometry stays owned by loader',scrolls,[a.id,'item-42']);
 check('keyboard focus does not create another scroll',focused,[[a.id,true],['e7',true],['p4',true],['item-42',true]]);
 check('exact successes',results,[[true,true],[true,true],[true,true],[true,true]]);
 var count=loads.length,tabCount=tabs.length;
 ['e0','e01','p0','p01','item-0','item-1000000','e7[x]','e7,body','../p4','#e7','https://other/s/foreign#e7'].forEach(function(id){effectJump(a.id,id,done);});
 effectJump('topic-aa[x]','e7',done);
 check('invalid IDs do not load or select a tab',[loads.length,tabs.length],[count,tabCount]);
 a.targets.e8=node('e8',b,true);effectJump(a.id,'e8',done);effectJump(a.id,'p999',done);
 check('foreign or missing exact targets honestly fall back',results.slice(-2),[[true,false],[true,false]]);
 var history=window.wbHistory;window.wbHistory=null;effectJump(a.id,'e7',done);window.wbHistory=history;
 check('entry-shaped arbitrary DOM ID is not enough without shared validator',results[results.length-1],[true,false]);
 var before=focused.length;effectJump('topic-6363','topic-6363',done);
 check('missing topic does not fabricate focus or success',[focused.length,results[results.length-1]],[before,[false,false]]);
 // A deferred closed topic is fetched through existing closed pagination.
 var c=node('topic-6464');c.tagName='SECTION';c.targets={};
 more={disabled:false,getAttribute:function(){return '30';},load:function(){topics.push(c);more=null;}};
 effectJump(c.id,c.id,done);await Promise.resolve();
 check('closed topic is reached after existing route materializes it',[closedLoads,results[results.length-1],focused[focused.length-1]],[1,[true,true],[c.id,true]]);
 more={disabled:false,getAttribute:function(){return '60';}};
 effectJump('topic-6565','topic-6565',done);await Promise.resolve();
 check('nonprogressing closed cursor is bounded',[closedLoads,results[results.length-1]],[2,[false,false]]);more=null;
 // Overview/filter restoration finishes before history geometry; an old
 // asynchronous jump cannot steal focus from a newer navigation.
 pendingReveal=true;before=loads.length;effectJump(a.id,'e7',done);var old=pendingReveal;
 check('lazy history waits for filter owner',loads.length,before);
 pendingReveal=null;effectJump(b.id,b.id,done);before=focused.length;old();
 check('superseded restore cannot steal focus',focused.length,before);
 // Native link clicks (including Enter-generated clicks) share this path.
 link={getAttribute:function(){return 'e7';}};var ev=click();effectJumpClick(ev);
 check('native keyboard link reaches exact element',[ev.prevented,status.textContent,focused[focused.length-1]],[true,'',['e7',true]]);
 link={getAttribute:function(){return 'p999';}};effectJumpClick(click());
 check('exact failure is disclosed, not called reached',status.textContent,'js.whiteboard.jump.elementmissing');
 link=null;row={getAttribute:function(){return 'e7';},closest:function(s){return s==='.wbupdate-topic[data-wb-topic]'?card:null;}};effectJumpClick(click());
 check('individual log-row click targets log',focused[focused.length-1],['e7',true]);
 var k=keydown('Enter');effectJumpKey(k);check('log-row Enter targets log',[k.prevented,focused[focused.length-1]],[true,['e7',true]]);
 k=keydown(' ');effectJumpKey(k);check('log-row Space targets log',[k.prevented,focused[focused.length-1]],[true,['e7',true]]);
 before=focused.length;
 k=keydown('Escape');effectJumpKey(k);
 k=keydown('Enter');k.ctrlKey=true;effectJumpKey(k);
 interactive=true;effectJumpKey(keydown('Enter'));interactive=false;
 check('other keys, modified keys, and nested interactive targets do not jump on keydown',focused.length,before);
 row=null;
 before=focused.length;interactive=true;effectJumpClick(click());interactive=false;ev=click();ev.ctrlKey=true;effectJumpClick(ev);
 var ownCard=card;card={getAttribute:function(){return a.id;},querySelector:function(){return status;}};effectJumpClick(click());card=ownCard;
 check('disclosure, modified and foreign-container clicks do not navigate',focused.length,before);
 effectJumpClick(click());check('card background independently reaches required topic',focused[focused.length-1],[a.id,true]);
 process.stdout.write(bad.join('\n'));
})().catch(function(e){console.error(e);process.exitCode=1;});
`)
}

func TestConversationPresentationMergePreservesIdentityAndDisclosures(t *testing.T) {
	src := whiteboardBridgeFunctions(t, string(mustAsset(t, "dashboard.js")), "entryKeys", "mergeEntries", "gapped", "evictEntries")
	runNode(t, "conversation keyed insertion", `
var bad=[],ENTRY_CAP=150,restored=[];
function check(label,got,want){if(JSON.stringify(got)!==JSON.stringify(want))bad.push(label+': '+JSON.stringify(got));}
function restoreOpen(n){restored.push(n.key);}function fmtTimes(){}
function entry(key,state){return {key:key,state:state||null,open:true,raw:'<script>diagnostic stays text</script>',getAttribute:function(a){return a==='data-entry'?this.key:a==='data-mail-wake'?this.state:null;}};}
function box(nodes){var b={children:nodes||[],insertBefore:function(n,next){var i=next?this.children.indexOf(next):this.children.length;this.children.splice(i,0,n);this.sync();},replaceChild:function(n,old){this.children[this.children.indexOf(old)]=n;this.sync();},removeChild:function(n){this.children.splice(this.children.indexOf(n),1);this.sync();},sync:function(){var ns=this.children;this.firstChild=this.firstElementChild=ns[0];this.lastElementChild=ns[ns.length-1];ns.forEach(function(n,i){n.previousElementSibling=ns[i-1]||null;n.nextSibling=ns[i+1]||null;});}};b.sync();return b;}
`+src+`
var mail=entry('02'),wake=entry('03'),shown=box([mail,wake]);
mergeEntries(shown,box([entry('03'),entry('04')]));
check('live/reconnect preserves existing disclosures and raw diagnostics',[shown.children[0]===mail,shown.children[1]===wake,wake.open,wake.raw],[true,true,true,'<script>diagnostic stays text</script>']);
mergeEntries(shown,box([entry('01'),entry('02')]));
check('history/live remain ordered with no duplicate key',shown.children.map(function(n){return n.key;}),['01','02','03','04']);
check('only inserted nodes restore disclosure state',restored,['04','01']);
check('reconnect gap still uses stable ring cursors',[gapped(shown,box([entry('09')])),gapped(shown,box([entry('03')]))],[true,false]);
check('no eviction or transport grouping for a short page',evictEntries(shown),false);
var marker=entry('02','suppressed'),matched=entry('03','matched');
mergeEntries(shown,box([marker,matched]));
check('association replaces existing mail with a marker and wake with mail',[shown.children[1]===marker,shown.children[2]===matched,shown.children.length],[true,true,4]);
check('replacements restore disclosure state',restored,['04','01','02','03']);
mergeEntries(shown,box([entry('02','suppressed'),entry('03','matched')]));
check('unchanged association retains the open delivery details',shown.children[2]===matched,true);
var separateMail=entry('02'),unmatched=entry('03');
mergeEntries(shown,box([separateMail,unmatched]));
check('expiry restores both rows at the same keys',[shown.children[1]===separateMail,shown.children[2]===unmatched,shown.children.map(function(n){return n.key;})],[true,true,['01','02','03','04']]);
var nudgeFirst=entry('10'),nudgeSecond=entry('11'),nudges=box([nudgeFirst,nudgeSecond]);
var batchTwo=entry('10','nudge-two'),absorbed=entry('11','suppressed');
mergeEntries(nudges,box([batchTwo,absorbed]));
check('nudge collapse replaces first row and absorbs only the keyed neighbor',[nudges.children[0]===batchTwo,nudges.children[1]===absorbed,nudges.children.length],[true,true,2]);
mergeEntries(nudges,box([entry('10','nudge-two'),entry('11','suppressed')]));
check('unchanged nudge batch retains open details',nudges.children[0]===batchTwo,true);
var batchThree=entry('10','nudge-three');
mergeEntries(nudges,box([batchThree,entry('11','suppressed'),entry('12','suppressed')]));
check('growing batch replaces merged diagnostics but preserves absorbed markers',[nudges.children[0]===batchThree,nudges.children[1]===absorbed,nudges.children.length],[true,true,3]);
var restoredFirst=entry('10'),restoredSecond=entry('11');
mergeEntries(nudges,box([restoredFirst,restoredSecond,entry('12')]));
check('nudge window expiry restores ordinary keyed rows',[nudges.children[0]===restoredFirst,nudges.children[1]===restoredSecond,nudges.children.map(function(n){return n.state;})],[true,true,[null,null,null]]);
var report=entry('20'),reports=box([report]);
mergeEntries(reports,box([entry('20')]));
check('open child report survives live merge at the same entry key',[reports.children[0]===report,report.open],[true,true]);
report.open=false;
mergeEntries(reports,box([entry('20')]));
check('closed child report stays closed on reconnect',[reports.children[0]===report,report.open],[true,false]);
var windowed=box([entry('02','suppressed'),entry('03','matched')]);
check('hidden oldest marker remains the initial paging cursor',windowed.firstElementChild.getAttribute('data-entry'),'02');
mergeEntries(windowed,box([entry('01')]));
check('older history joins without reintroducing the associated mail',windowed.children.map(function(n){return [n.key,n.state];}),[['01',null],['02','suppressed'],['03','matched']]);
ENTRY_CAP=2;
check('eviction counts keyed ring entries including markers',evictEntries(windowed),true);
check('cursor and keyed index survive eviction',[windowed.firstElementChild.key,Object.keys(entryKeys(windowed)).sort()],['02',['02','03']]);
process.stdout.write(bad.join('\n'));
`)
}
