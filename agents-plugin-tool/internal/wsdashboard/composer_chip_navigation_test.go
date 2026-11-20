package wsdashboard

import (
	"regexp"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsinbox"
	"golang.org/x/net/html"
)

func TestComposerMentionBodyNavigatesAndOnlyRemoveDeletes(t *testing.T) {
	js := string(mustAsset(t, "dashboard.js"))
	click := regexp.MustCompile(`(?s)  // --- clicks: delegated.*?\n  doc.addEventListener\("click", function \(e\) \{.*?\n  \}\);`).FindString(js)
	if click == "" {
		t.Fatal("missing delegated chip actions")
	}
	runNode(t, "composer mention navigation", `
var bad=[],jumps=[],handler,ibadge=null,gmenu=null,live=false;
function check(label,got,want){if(JSON.stringify(got)!==JSON.stringify(want))bad.push(label+': '+JSON.stringify(got));}
function El(tag){this.tagName=tag.toUpperCase();this.children=[];this.attrs={};this.events={};this.className='';}
El.prototype.setAttribute=function(k,v){this.attrs[k]=String(v);};
El.prototype.getAttribute=function(k){return this.attrs[k]||null;};
El.prototype.appendChild=function(n){n.parentNode=this;this.children.push(n);return n;};
El.prototype.addEventListener=function(k,fn){this.events[k]=fn;};
El.prototype.closest=function(s){var m=/^\[([^\]]+)\]$/.exec(s);for(var n=this;n;n=n.parentNode)if(m&&m[1] in n.attrs)return n;return null;};
El.prototype.click=function(){if(this.events.click)this.events.click();handler({target:this});};
Object.defineProperty(El.prototype,'textContent',{get:function(){return this._text||'';},set:function(v){this._text=v;this.children=[];}});
var doc={createElement:function(t){return new El(t);},createTextNode:function(t){var n=new El('#text');n.textContent=t;return n;},addEventListener:function(k,fn){handler=fn;}};
var cimgs=null,closeRequests=[],bselections=[];
var cchips=new El('div'),bmentions=[{topic:'plan',id:'07',text:'one\n two'},{topic:'계획 "<&',id:'3',entry:'e8',text:'old version'},{topic:'plan',text:'topic only'}],wpicks=[],wconsoles=[];
function chipList(){return [];}function topicNotes(){return [];}function inBoard(){return false;}function wsT(k){return k;}
function inlineEditor(){return null;}
function effectJump(topic,id){jumps.push([topic,id]);}
`+dashboardFuncs(t, "renderChips", "renderSelectionChips")+click+`
var original=JSON.stringify(bmentions);renderChips();
var chip=cchips.children[0],open=chip.children[0],remove=chip.children[1];
check('separate native buttons preserve keyboard activation',[open.tagName,open.type,remove.tagName,remove.type],['BUTTON','button','BUTTON','button']);
check('only small remove carries deletion action',[open.getAttribute('data-unmchip'),remove.getAttribute('data-unmchip'),remove.textContent],[null,'0','×']);
check('full excerpt stays title, compact text stays visible',[chip.title,open.children[1].textContent],['one\n two','one two']);
open.click();cchips.children[1].children[0].click();cchips.children[2].children[0].click();
check('body routes item, immutable entry first, and topic through existing ID-chip path',jumps,[['topic-706c616e','item-7'],['topic-eab384ed9a8d20223c26','e8'],['topic-706c616e','topic-706c616e']]);
check('navigation never mutates held mention payloads',JSON.stringify(bmentions),original);
// Delegation also sees clicks on the body's child text, never a remove.
handler({target:open.children[0]});check('body descendants cannot delete',bmentions.length,3);
remove.click();check('only × removes one mention in order',bmentions.map(function(m){return m.text;}),['old version','topic only']);
check('removal does not navigate',jumps.length,3);
check('remove index follows repaint',cchips.children[0].children[1].getAttribute('data-unmchip'),'0');
process.stdout.write(bad.join('\n'));
`)
}

func TestHistoryAnswerWholeChipTitlePreservesFullOriginalNote(t *testing.T) {
	note := strings.Repeat("long note 한글 ", 60) + "\n\"<&' tail"
	text := wsinbox.AnswerBlock([]wsinbox.Chip{{ID: "7", Answer: "ok", Note: note}}, []wsinbox.TopicNote{{Topic: "plan", Note: note}})
	lines, _, ok := wsinbox.ParseAnswerBlock(text)
	if !ok || len(lines) != 2 || lines[0].Note != note || lines[1].Note != note {
		t.Fatal("answer parser did not retain full original notes")
	}
	out := renderNamed(t, "achips", ConvEntry{Answers: lines})
	if strings.Contains(out, `title="`+note+`"`) || strings.Contains(out, "<script") {
		t.Fatal("note was not escaped in HTML attribute context")
	}
	root, err := html.Parse(strings.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		class, title := "", ""
		for _, a := range n.Attr {
			if a.Key == "class" {
				class = a.Val
			}
			if a.Key == "title" {
				title = a.Val
			}
		}
		if class == "achip" || class == "achip tchip" {
			count++
			if title != note {
				t.Errorf("whole chip title lost note: %q", title)
			}
		}
		if class == "anote" && title != "" {
			t.Error("hover title still belongs only to clipped inner note")
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(root)
	if count != 2 {
		t.Fatalf("answer chips = %d, want 2", count)
	}
}
