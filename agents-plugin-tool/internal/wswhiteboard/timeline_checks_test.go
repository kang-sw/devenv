package wswhiteboard

import (
	"os/exec"
	"strings"
	"testing"
)

func TestTimelineCheckPillRetainsFoldAndLiftControls(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	asset, err := embeddedAsset("rail.js")
	if err != nil {
		t.Fatal(err)
	}
	source := string(asset)
	start, end := strings.Index(source, "  var lifted = null;"), strings.Index(source, "  box.addEventListener(\"click\", pill);")
	if start < 0 || end <= start {
		t.Fatal("check pill owner not found")
	}
	script := `var BOARD='<section class="topic"><div class="tophead"><button class="ckbtn"></button></div><div class="timeline-row"><div class="checks"><div class="ckrow" id="item-1"></div></div></div><div class="timeline-row"><div class="checks"><div class="ckrow" id="item-2"></div></div></div></section>',BODY_CLASS='';` + formPrelude + `
Object.defineProperty(E.prototype,'parentNode',{get:function(){return this.parent;}});
Object.defineProperty(E.prototype,'className',{set:function(v){this.attrs.class=v;}});
Object.defineProperty(E.prototype,'offsetHeight',{get:function(){return 20;}});
E.prototype.removeChild=function(n){this.children.splice(this.children.indexOf(n),1);n.parent=null;};
E.prototype.appendChild=function(n){if(n.parent)n.parent.removeChild(n);this.children.push(n);n.parent=this;};
E.prototype.replaceChild=function(n,old){if(n.parent)n.parent.removeChild(n);this.children[this.children.indexOf(old)]=n;n.parent=this;old.parent=null;};
var query=E.prototype.querySelector;
E.prototype.querySelector=function(s){return s===':scope > .checks'?this.children.find(function(n){return n.cls().includes('checks');})||null:query.call(this,s);};
var d={createElement:function(tag){return new E(tag);}},box=root;
function getComputedStyle(){return {display:'block',marginTop:'0px',marginBottom:'0px'};}
` + source[start:end] + `
var head=root.querySelector('.tophead'),btn=head.querySelector('button'),lists=root.querySelectorAll('.checks'),parents=lists.map(function(n){return n.parent;});
pill({target:btn});check('all chronological check lists fold together',lists.map(function(n){return n.hidden;}),[true,true]);
pill({target:btn});check('all check lists unfold',lists.map(function(n){return n.hidden;}),[false,false]);
head.classList.toggle('wb-stuck',true);pill({target:btn});
check('real controls lifted together',head.querySelectorAll('.ckrow').map(function(n){return n.attrs.id;}),['item-1','item-2']);
check('each chronological position keeps a spacer',parents.map(function(n){return !!n.querySelector('.ckspace');}),[true,true]);
drop();check('drop restores original list identities and positions',lists.map(function(n,i){return n.parent===parents[i];}),[true,true]);
check('popover removed',head.querySelector('.checks'),null);
if(bad.length)throw Error(bad.join('\n'));
`
	if out, err := nodeScript(node, script).CombinedOutput(); err != nil {
		t.Fatalf("check pill: %v\n%s", err, out)
	}
}
