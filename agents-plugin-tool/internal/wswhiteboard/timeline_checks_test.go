package wswhiteboard

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

func TestTimelineChecksUseQuestionAnchorsAtAddPosition(t *testing.T) {
	for _, tc := range []struct {
		name, answer, chip, reading string
		assumed                     bool
	}{
		{name: "open", chip: "open"},
		{name: "ok", answer: "ok", chip: "ok"},
		{name: "no", answer: "no, use A", chip: "no", reading: "use A"},
		{name: "assumed", answer: "ok", chip: "ok", assumed: true},
		{name: "free text", answer: "looks right", chip: "answered", reading: "looks right"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := &State{}
			mustApply(t, st, `<wb:topic name="t"><wb:log>Before check</wb:log><wb:add class="check"><wb:title>held</wb:title></wb:add><wb:log>After check</wb:log></wb:topic>`)
			var added Entry
			var beforeID, afterID string
			for _, e := range st.Topics[0].Entries {
				if e.ItemID == 1 && strings.HasPrefix(e.Text, "Item {{1}} added [check]") {
					added = e
				}
				if e.Text == "Before check" {
					beforeID = e.ID
				}
				if e.Text == "After check" {
					afterID = e.ID
				}
			}
			for i := 0; i < 60; i++ {
				mustApply(t, st, fmt.Sprintf(`<wb:topic name="t"><wb:log>later %d</wb:log></wb:topic>`, i))
			}
			if tc.answer != "" {
				mustApply(t, st, fmt.Sprintf(`<wb:answer id="1" value="%s" assumed="%t">owner words</wb:answer>`, tc.answer, tc.assumed))
			}
			view := BuildView(st, ViewInput{Layout: LayoutFull})
			live, held := timelineRows(view.Topics[0])
			asks := 0
			for _, row := range live {
				if row.Ask {
					asks++
					if row.Key != added.ID || row.Item.ID != 1 || row.Page.Runs[0].Entry.ID != added.ID {
						t.Fatalf("check not at its add event: %+v", row)
					}
				}
			}
			if asks != 1 || len(held) < 50 {
				t.Fatalf("check anchor/paging: asks=%d held=%d", asks, len(held))
			}
			out := markup(render(t, view))
			start := strings.Index(out, `class="srow" id="item-1"`)
			if start < 0 {
				t.Fatal("check lacks the shared question row")
			}
			end := strings.Index(out[start:], `</summary>`)
			if end < 0 {
				t.Fatal("check lacks the shared question summary")
			}
			summary := out[start : start+end]
			wantAll(t, summary, `class="sx" data-key="item:1"`, `<summary><span class="stext"><span class="id">(01)</span>`, `<b>held</b> <span class="pick"`, `<span class="pt">`+tc.chip+`</span></span>`)
			if tc.answer == "" || tc.assumed {
				wantAll(t, summary, `data-key="item:1" open`)
			} else {
				wantNone(t, summary, `data-key="item:1" open`)
			}
			if tc.reading != "" {
				wantAll(t, summary, `<span class="ans"`, `>`+tc.reading+`</span>`)
			}
			wantAll(t, out, `<div class="sbody"><div class="checks"><div class="ckrow`, `id="check-1"`)
			if strings.Count(out, `id="item-1"`) != 1 || strings.Count(out, `id="check-1"`) != 1 {
				t.Fatal("check anchor or real controls duplicated")
			}
			before, event, after := strings.Index(out, `id="`+beforeID+`"`), strings.Index(out, `id="`+added.ID+`"`), strings.Index(out, `id="`+afterID+`"`)
			if !(before >= 0 && before < event && event < start && start < after) {
				t.Fatalf("check moved from its event: before=%d event=%d check=%d after=%d", before, event, start, after)
			}
		})
	}
}

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
	script := `var BOARD='<section class="topic"><div class="tophead"><button class="ckbtn"></button></div><div class="timeline-row"><div class="srow" id="item-1"><details class="sx" open><summary><span class="stext">(1) held <span class="pick">ok</span></span></summary><div class="sbody"><div class="checks"><div class="ckrow" id="check-1"></div></div></div></details></div></div><div class="timeline-row"><div class="srow" id="item-2"><details class="sx" open><summary><span class="stext">(2) next <span class="pick">open</span></span></summary><div class="sbody"><div class="checks"><div class="ckrow" id="check-2"></div></div></div></details></div></div></section>',BODY_CLASS='';` + formPrelude + `
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
check('fold leaves question summaries in place',root.querySelectorAll('.timeline-row .srow summary').length,2);
pill({target:btn});check('all check lists unfold',lists.map(function(n){return n.hidden;}),[false,false]);
head.classList.toggle('wb-stuck',true);pill({target:btn});
check('real controls lifted together',head.querySelectorAll('.ckrow').map(function(n){return n.attrs.id;}),['check-1','check-2']);
check('question anchors stay in their timeline rows',root.querySelectorAll('.timeline-row .srow').map(function(n){return n.attrs.id;}),['item-1','item-2']);
check('each chronological position keeps a spacer',parents.map(function(n){return !!n.querySelector('.ckspace');}),[true,true]);
drop();check('drop restores original list identities and positions',lists.map(function(n,i){return n.parent===parents[i];}),[true,true]);
check('popover removed',head.querySelector('.checks'),null);
if(bad.length)throw Error(bad.join('\n'));
`
	if out, err := nodeScript(node, script).CombinedOutput(); err != nil {
		t.Fatalf("check pill: %v\n%s", err, out)
	}
}
