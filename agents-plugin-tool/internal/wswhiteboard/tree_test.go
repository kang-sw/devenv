package wswhiteboard

import (
	"encoding/json"
	"strings"
	"testing"
)

// treeLines flattens a tree to "path kind value [long] [open]" lines.
func treeLines(n TreeNode, out *[]string) {
	line := n.Path + " " + n.Kind + " " + n.Value
	if n.Long {
		line += " long"
	}
	if n.Open {
		line += " open"
	}
	*out = append(*out, line)
	for _, c := range n.Children {
		treeLines(c, out)
	}
}

// TestJSONTree pins the tree a JSON value lays out as: fields in stored
// order, nested objects and arrays as folded containers with a preview,
// scalars as leaves, empty containers as leaves, a long or multi-line string
// marked long, paths JSON Pointer escaped, and invalid JSON as one leaf.
func TestJSONTree(t *testing.T) {
	long := strings.Repeat("x", treeLongRunes+1)
	raw := json.RawMessage(`{"zeta":1,"a/b~c":"short","nested":{"k":[true,null,{"deep":2.50}]},"empty":{},"none":[],"long":"` + long + `","multi":"one\ntwo"}`)
	var got []string
	treeLines(JSONTree("agenda/", "plan", raw), &got)
	want := []string{
		"agenda/plan obj {zeta, a/b~c, nested, empty, none, long, multi} open",
		"agenda/plan/zeta num 1",
		"agenda/plan/a~1b~0c str short",
		"agenda/plan/nested obj {k}",
		"agenda/plan/nested/k arr [3]",
		"agenda/plan/nested/k/0 bool true",
		"agenda/plan/nested/k/1 null null",
		"agenda/plan/nested/k/2 obj {deep}",
		"agenda/plan/nested/k/2/deep num 2.50",
		"agenda/plan/empty obj {}",
		"agenda/plan/none arr []",
		"agenda/plan/long str " + long + " long",
		"agenda/plan/multi str one\ntwo long",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("tree:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	if n := JSONTree("agenda/", "bad", json.RawMessage(`{"unterminated":`)); n.Kind != TreeString || n.Value != `{"unterminated":` || n.IsContainer() {
		t.Fatalf("invalid JSON must be one string leaf: %+v", n)
	}
	if n := JSONTree("agenda/", "list", json.RawMessage(`["a"]`)); n.Kind != TreeArray || !n.Open || len(n.Children) != 1 {
		t.Fatalf("a root array must be an open container: %+v", n)
	}
	wide := `{"` + strings.Repeat("k", 30) + `":1,"` + strings.Repeat("m", 30) + `":2}`
	if v := JSONTree("", "w", json.RawMessage(wide)).Value; !strings.HasSuffix(v, "…}") || len([]rune(v)) != treeSummaryRunes+2 {
		t.Fatalf("a long field-name preview must be cut: %q", v)
	}
}

// TestRenderPanelTree pins the tree markup on both layouts: a container is a
// details keyed by its path, open only at the root; a long string folds with
// its whole value in the body; a scalar is a leaf with its kind as class;
// and every value is escaped.
func TestRenderPanelTree(t *testing.T) {
	long := strings.Repeat("y", treeLongRunes+5)
	panel := Panel{ID: "agenda", Title: "Agendas", Icon: "list", Count: "1", Tree: []TreeNode{
		JSONTree("agenda/", "plan", json.RawMessage(`{"step":{"n":3},"note":"<b>x</b>","long":"`+long+`","flag":false}`)),
	}}
	for name, layout := range layouts() {
		out := render(t, BuildView(&State{}, ViewInput{Layout: layout, ReplyID: "r", Panels: []Panel{panel}}))
		for _, want := range []string{
			`<ul class="tree"><li><details class="tnode" data-key="agenda/plan" open><summary><span class="tk">plan</span>`,
			`<details class="tnode" data-key="agenda/plan/step"><summary><span class="tk">step</span><span class="tsum">{n}</span></summary>`,
			`<div class="tleaf"><span class="tk">n</span><span class="tv num">3</span></div>`,
			`<span class="tv str">&lt;b&gt;x&lt;/b&gt;</span>`,
			`<details class="tnode tlong" data-key="agenda/plan/long"><summary><span class="tk">long</span><span class="tv str">` + long + `</span></summary><div class="tfull">` + long + `</div></details>`,
			`<span class="tv bool">false</span>`,
		} {
			if !strings.Contains(out, want) {
				t.Errorf("%s: page lacks %s", name, want)
			}
		}
		if strings.Contains(out, "<b>x</b>") {
			t.Errorf("%s: a tree value was not escaped", name)
		}
	}
}
