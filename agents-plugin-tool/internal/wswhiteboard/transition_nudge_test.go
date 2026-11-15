package wswhiteboard

import (
	"strings"
	"testing"
	"time"
)

const implementingCmd = `<wb:topic name="Work"><wb:state value="implementing" owner="w" eta_minutes="30"/></wb:topic>`

func TestTransitionNudge(t *testing.T) {
	const check = `<wb:add class="check"><wb:title>Verify</wb:title></wb:add>`
	const question = `<wb:add class="policy"><wb:title>pick</wb:title><wb:option>x</wb:option><wb:option>y</wb:option><wb:recommend>a</wb:recommend></wb:add>`
	for _, tc := range []struct {
		name, setup, move string
		want              string // state named in the nudge; "" for none
	}{
		{"awaiting no ask", "", `<wb:topic name="Work"><wb:state value="awaiting-confirmation"/></wb:topic>`, "awaiting-confirmation"},
		{"waiting no ask", "", `<wb:topic name="Work"><wb:state value="waiting"/></wb:topic>`, "waiting"},
		{"awaiting open check", check, `<wb:topic name="Work"><wb:state value="awaiting-confirmation"/></wb:topic>`, ""},
		{"waiting open question", question, `<wb:topic name="Work"><wb:state value="waiting"/></wb:topic>`, ""},
		{"ask added same batch", "", `<wb:topic name="Work">` + check + `<wb:state value="awaiting-confirmation"/></wb:topic>`, ""},
		{"to researching", "", `<wb:topic name="Work"><wb:state value="researching"/></wb:topic>`, ""},
		{"close", "", `<wb:topic name="Work"><wb:close result="done"/></wb:topic>`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := stateBoard(t)
			if tc.setup != "" {
				stateApply(t, s, `<wb:topic name="Work">`+tc.setup+`</wb:topic>`, stateNow)
			}
			stateApply(t, s, implementingCmd, stateNow.Add(time.Minute))
			out := stateApply(t, s, tc.move, stateNow.Add(2*time.Minute))
			got := strings.Contains(out.Info, "topic Work: moved to ")
			if tc.want == "" {
				if got {
					t.Fatalf("unexpected nudge: %s", out.Info)
				}
				return
			}
			if !strings.Contains(out.Info, "topic Work: moved to "+tc.want+" with no open ask — if the work is finished, close it (wb:close)") {
				t.Fatalf("missing nudge: %q", out.Info)
			}
		})
	}
}

func TestTransitionNudgeNotFromImplementing(t *testing.T) {
	s := stateBoard(t)
	out := stateApply(t, s, `<wb:topic name="Work"><wb:state value="waiting"/></wb:topic>`, stateNow.Add(time.Minute))
	if strings.Contains(out.Info, "moved to") {
		t.Fatalf("discussing->waiting nudged: %s", out.Info)
	}
}
