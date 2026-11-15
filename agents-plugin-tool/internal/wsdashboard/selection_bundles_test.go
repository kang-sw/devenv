package wsdashboard

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/kang-sw/devenv/internal/wsinbox"
)

func TestSelectionClientCanonicalCodec(t *testing.T) {
	js, err := os.ReadFile("assets/dashboard-bundles.js")
	if err != nil {
		t.Fatal(err)
	}
	var cases []map[string]any
	for _, s := range []wsinbox.Selection{
		{Mentions: []wsinbox.Mention{{Topic: "t", Text: "one", Entry: "e7"}}},
		{Mentions: []wsinbox.Mention{{Topic: "a <b> & \"c\"", ID: "03", Entry: "e8", Text: "a < b [Image #1] &#91; &amp;"}, {Topic: "다음", Entry: "e9", Text: "second\nline"}}, Comment: "memo </wb:comment> [Image #1] &#91; &quot;", Images: []int{1, 8}},
	} {
		block, err := wsinbox.SelectionBlock(s)
		if err != nil {
			t.Fatal(err)
		}
		cases = append(cases, map[string]any{"block": block, "mentions": s.Mentions, "comment": s.Comment, "images": append([]int{}, s.Images...)})
	}
	raw, _ := json.Marshal(cases)
	runNode(t, "canonical selection codec", string(js)+`
var cases = `+string(raw)+`, bad = [];
function check(name, value, want) { if (!require('util').isDeepStrictEqual(value, want)) bad.push(name + ': ' + JSON.stringify(value)); }
cases.forEach(function (c) {
  var parsed = splitSelectionText(c.block + '\n\nordinary [Image #9]');
  check('canonical Go group read', parsed.groups, [{mentions:c.mentions,comment:c.comment,images:c.images}]);
  check('independent text', parsed.text, 'ordinary [Image #9]');
  if (parsed.groups.length) check('same serialization', selectionBlock(parsed.groups[0]), c.block);
  check('ranges shift past prefix', selectionRanges([{start:c.block.length+2,end:c.block.length+10,kind:'topic',id:'x',label:'X'}],parsed.cuts), [{start:0,end:8,kind:'topic',id:'x',label:'X'}]);
});
var block = cases[0].block;
[
  '<wb:selection>\n' + block + '\n</wb:selection>',
  '<wb:selection broken="</wb:selection>">\n' + block + '\n</wb:selection>',
  '<wb:selection>\n<!-- </wb:selection> -->\n' + block + '\n</wb:selection>',
  block.replace('<wb:comment>', '<unknown/>\n<wb:comment>'),
  block.replace('<wb:comment></wb:comment>', ''),
  block.replace('<wb:comment></wb:comment>', '<wb:comment>x</wb:comment>\n<wb:comment>y</wb:comment>')
].forEach(function (raw) { check('failed envelope remains whole', splitSelectionText(raw), {text:raw,groups:[],cuts:[]}); });
var sibling = 'ordinary < broken\n' + block;
check('ordinary less-than does not swallow group', splitSelectionText(sibling).groups.length, 1);
process.stdout.write(bad.join('\n'));
`)
}
