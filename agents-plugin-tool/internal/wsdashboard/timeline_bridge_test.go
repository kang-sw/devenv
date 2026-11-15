package wsdashboard

import "testing"

func TestBoardTimelineAnswerJumpUsesHistoryOwner(t *testing.T) {
	runNode(t, "timeline answer navigation", dashboardFuncs(t, "boardJump")+`
var bad=[],calls=[],fallback=0,board={},target={id:'e17',parentNode:null,closest:function(){return board;}};
function check(what,got,want){if(JSON.stringify(got)!==JSON.stringify(want))bad.push(what);}
function boardBoxes(){return [board];}
function inBoard(n){return n===target;}
function $(){fallback++;return null;}
var reveal=function(){},window={WsCards:{unfoldTopic:reveal},wbHistory:{loadTarget:function(root,id,owner){calls.push([root===board,id,owner===reveal]);return id==='e17'?target:null;}}};
check('answer entry returned',boardJump('e17')===target,true);
check('existing materializer and scroll owner used',calls,[[true,'e17',true]]);
check('no competing fallback scroll',fallback,0);
['e01','e0','e17[x]','e17,body'].forEach(boardJump);
check('malformed entry locators never materialize',calls.length,1);
process.stdout.write(bad.join('\n'));
`)
}
