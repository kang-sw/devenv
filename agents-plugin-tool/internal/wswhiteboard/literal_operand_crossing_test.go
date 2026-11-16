package wswhiteboard

import (
	"reflect"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsxmlcmd"
)

func TestNamedPanelPatchRefusesPanelToOperandCodeSpan(t *testing.T) {
	testNamedPanelPatchRefusesCrossing(t, "<wb:panel name=\"p\" recap=\"r2\">` <wb:change><wb:old>alpha ` x</wb:old><wb:new>z</wb:new></wb:change></wb:panel>")
}

func TestNamedPanelPatchRefusesPanelToWrapperCodeSpan(t *testing.T) {
	for _, body := range []string{
		"` <wb:change><wb:old>alpha</wb:old> ` <wb:new>z</wb:new></wb:change>",
		"` <wb:change> ` <wb:old>alpha</wb:old><wb:new>z</wb:new></wb:change>",
		"` <wb:change><wb:old>alpha</wb:old><wb:new>z</wb:new> ` </wb:change>",
	} {
		testNamedPanelPatchRefusesCrossing(t, `<wb:panel name="p" recap="r2">`+body+`</wb:panel>`)
	}
}

func testNamedPanelPatchRefusesCrossing(t *testing.T, patch string) {
	t.Helper()
	input := `<wb:topic name="t">` + patch + `</wb:topic>`
	for _, ordered := range []bool{false, true} {
		p := &problems{}
		var cmds []command
		complete := true
		if ordered {
			cmds, complete = wsxmlcmd.ParseOrdered(p, grammar, input)
			cmds = cmds[1:] // topic sentinel
		} else {
			cmds = wsxmlcmd.Parse(p, grammar, input)
		}
		if !complete || len(p.List) == 0 || !strings.Contains(strings.Join(p.List, " "), "ends inside") || len(cmds) != 1 || len(cmds[0].Children) != 1 {
			t.Fatalf("ordered=%v commands=%+v complete=%v problems=%v", ordered, cmds, complete, p.List)
		}
	}
	s := &State{}
	mustApply(t, s, panelCommandXML("t", `name="p" recap="r"`, "alpha beta"))
	before := s.clone()
	out, p := Apply(s, input, testEnv())
	if len(p) == 0 || out.Accepted != 0 || out.Unpartitioned || !strings.Contains(out.Failed, patch) || !reflect.DeepEqual(s, before) {
		t.Fatalf("stray tick patch replaced the named panel: %+v problems=%v", out, p)
	}
	// A soundly delimited bad panel must not block an independent following log.
	mixed(t, s, strings.TrimSuffix(input, `</wb:topic>`)+`<wb:log>keep</wb:log></wb:topic>`, `<wb:topic name="t"><wb:log>keep</wb:log></wb:topic>`)
}
