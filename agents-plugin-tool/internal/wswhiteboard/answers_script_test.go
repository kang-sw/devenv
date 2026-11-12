package wswhiteboard

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsinbox"
)

// TestAnswersScriptMatchesTheServerBlock holds answers.js (the static page's
// copy-answers and the dashboard composer) and wsinbox.AnswerBlock (the
// delivered message) to one grammar: each case runs through both and the
// blocks must be equal. The cases execute under node when it is installed.
func TestAnswersScriptMatchesTheServerBlock(t *testing.T) {
	cases := []struct {
		Chips  []wsinbox.Chip      `json:"chips"`
		Topics []wsinbox.TopicNote `json:"topics"`
		Want   string              `json:"want"`
	}{
		{},
		{Chips: []wsinbox.Chip{{ID: "1"}, {ID: "2", Note: "  "}}, Topics: []wsinbox.TopicNote{{Topic: "t", Note: " "}}},
		{Chips: []wsinbox.Chip{{ID: "341", Answer: "a"}, {ID: "691", Answer: "b"}, {ID: "123", Answer: "b"}, {ID: "567", Answer: "b"}}},
		{Chips: []wsinbox.Chip{{ID: "5", Answer: "ok"}, {ID: "2", Answer: "a"}, {ID: "1", Answer: "ok"}}},
		{Chips: []wsinbox.Chip{{ID: "126", Answer: "ok"}, {ID: "124", Answer: "ok"}, {ID: "125", Answer: "ok"}, {ID: "127", Answer: "ok"}, {ID: "181", Answer: "ok"}, {ID: "7", Answer: "ok"}, {ID: "8", Answer: "ok"}}},
		{Chips: []wsinbox.Chip{{ID: "01", Answer: "ok"}, {ID: "02", Answer: "ok"}, {ID: "03", Answer: "ok"}, {ID: "05", Answer: "no"}}},
		{Chips: []wsinbox.Chip{{ID: "130", Answer: "ok", Note: " user note "}, {ID: "131", Note: "why?"}, {ID: "4", Answer: "a", Note: "x^y"}}},
		{Chips: []wsinbox.Chip{{ID: "5", Note: "line one\nC:\\path\\n here\r\nend\rlast"}}},
		{Chips: []wsinbox.Chip{{ID: "9", Answer: "c"}}, Topics: []wsinbox.TopicNote{{Topic: "ship-gate", Note: "first\nsecond"}, {Topic: "출시 게이트", Note: "ok?"}}},
		{Topics: []wsinbox.TopicNote{{Topic: "t", Note: "n"}}},
	}
	for i := range cases {
		cases[i].Want = wsinbox.AnswerBlock(cases[i].Chips, cases[i].Topics)
	}
	data, err := json.Marshal(cases)
	if err != nil {
		t.Fatal(err)
	}
	js, err := embeddedAsset("answers.js")
	if err != nil {
		t.Fatal(err)
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; answer block cases not executed")
	}
	script := "var window = {};\n" + string(js) + `
var bad = [];
` + "var cases = " + string(data) + `;
cases.forEach(function (c, i) {
  var items = (c.chips || []).map(function (x) { return { id: x.id, answer: x.answer || "", note: x.note || "" }; });
  var got = window.wbAnswers.block(items, c.topics || []);
  if (got !== c.want) bad.push("case " + i + ": js " + JSON.stringify(got) + ", go " + JSON.stringify(c.want));
});
process.stdout.write(bad.join("\n"));`
	out, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, out)
	}
	if s := strings.TrimSpace(string(out)); s != "" {
		t.Errorf("answers.js and wsinbox.AnswerBlock disagree:\n%s", s)
	}
}
