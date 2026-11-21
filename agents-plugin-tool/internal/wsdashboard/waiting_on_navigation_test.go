package wsdashboard

import (
	"encoding/json"
	"testing"
)

func TestWaitingOnTopicFilterReveal(t *testing.T) {
	cards, _ := json.Marshal(mustAsset(t, "cards.js"))
	runNode(t, "waiting-on topic filter reveal", "var CARDS = "+string(cards)+";\n"+cardsDOM+`
var A,B,deferred,landed=0;
boot({},function(){
 A=topic('a',100);A.setAttribute('data-state','waiting');A.setAttribute('data-waiting-ready','true');
 B=topic('b',90);B.setAttribute('data-state','discussing');return [A,B];
},{toolbar:{filter:function(run){deferred=run;}}});
var radio=function(v){return page.W.toolbar().querySelector('.board-state-inline [data-filter="'+v+'"]');};
radio('waiting').fire('click',{});deferred();
check('ready remains in waiting filter',[A.hasAttribute('data-state-filtered'),radio('waiting').querySelector('.board-state-count').textContent],[false,'1']);
radio('discussing').fire('click',{});deferred();
page.board.fire('click',{target:A.querySelector('.t-fold')});
page.W.revealTopic(A,function(){landed++;check('visible and unfolded before geometry',[A.hasAttribute('data-state-filtered'),A.getAttribute('data-size')],[false,'base']);});
check('overview restores before reveal callback',[landed,A.hasAttribute('data-state-filtered')],[0,true]);
deferred();
check('hidden target resets filter to all',[landed,radio('all').getAttribute('aria-checked'),page.store['cards.state-filter']],[1,'true','all']);
page.W.revealTopic(A,function(){landed++;});
check('already visible target reveals synchronously',landed,2);
var saved=page.store['cards.state-filter'];
page.W.revealTopic(topic('outside',1),function(){landed++;});
check('foreign target cannot reset filter or call geometry',[landed,page.store['cards.state-filter']],[2,saved]);
process.stdout.write(bad.join('\n'));
`)
}

func TestWaitingOnTopicFragmentBridge(t *testing.T) {
	src := whiteboardBridgeFunctions(t, string(mustAsset(t, "dashboard.js")), "boardJump")
	runNode(t, "waiting-on fragment bridge", `
var bad=[],loaded=[],scrolled=[],revealed=[],after;
var doc={body:{classList:{contains:function(){return false;}}}},grid=null,tkApi=null;
function check(label,got,want){if(JSON.stringify(got)!==JSON.stringify(want))bad.push(label+': '+JSON.stringify(got));}
var target={id:'topic-6162',tagName:'SECTION',owned:true,hasAttribute:function(k){return k==='data-topic';},closest:function(){return null;},scrollIntoView:function(){scrolled.push(this.id);}};
var targets={'topic-6162':target,'topic-6163':{id:'topic-6163',tagName:'DIV',owned:true},'topic-6164':{id:'topic-6164',tagName:'SECTION',owned:false}};
function $(id){return targets[id];}function inBoard(n){return !!n&&n.owned;}function boardBoxes(){return [{}];}
var window={WsCards:{unfoldTopic:function(n){revealed.push(n.id);},revealTopic:function(n,done){after=done;}},wbHistory:{loadTarget:function(root,id){loaded.push(id);return null;}}};
`+src+`
check('topic returns owned loaded section',boardJump('topic-6162')===target,true);
check('topic skips history materialization and waits for filter owner',[loaded,scrolled],[[],[]]);
after();check('filter owner precedes scroll',[revealed,scrolled],[['topic-6162'],['topic-6162']]);
['topic-6163','topic-6164','topic-ffff','topic-a','topic-61[x]','topic-61,body','topic-61/../','topic-zz'].forEach(boardJump);
check('invalid/missing/off-board/topic-shaped body anchors cannot scroll',[scrolled,loaded],[['topic-6162'],[]]);
process.stdout.write(bad.join('\n'));
`)
}
