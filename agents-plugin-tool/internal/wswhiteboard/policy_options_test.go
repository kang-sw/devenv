package wswhiteboard

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestNonPolicyOptionsAddRefusesBatch(t *testing.T) {
	for _, class := range []string{ClassTechnical, ClassAnnounced, ClassCheck} {
		for name, fields := range map[string]string{
			"options":        `<wb:option>x</wb:option><wb:option>y</wb:option>`,
			"recommendation": `<wb:recommend>a: safer</wb:recommend>`,
			"both":           `<wb:option>x</wb:option><wb:recommend>a</wb:recommend>`,
		} {
			t.Run(class+"/"+name, func(t *testing.T) {
				st := &State{}
				commands := addXML(ClassPolicy, "t", "valid first", "x", "y") +
					fmt.Sprintf(`<wb:topic name="t"><wb:add class=%q><wb:title>invalid</wb:title>%s</wb:add></wb:topic>`, class, fields) +
					`<wb:patch id="1"><wb:why>valid later</wb:why></wb:patch>`
				wantProblems(t, refused(t, st, commands), `use class="policy" for a pick`)
			})
		}
	}
}

func TestNonPolicyOptionsPatchRefusesBatch(t *testing.T) {
	for _, class := range []string{ClassTechnical, ClassAnnounced, ClassCheck} {
		for name, patch := range map[string]string{
			"set options":        `<wb:patch id="1"><wb:option>x</wb:option></wb:patch>`,
			"set recommendation": `<wb:patch id="1"><wb:recommend>a</wb:recommend></wb:patch>`,
			"leave options":      `<wb:patch id="2"><wb:why>still invalid</wb:why></wb:patch>`,
			"leave recommend":    `<wb:patch id="3"><wb:title>still invalid</wb:title></wb:patch>`,
			"reclass policy":     fmt.Sprintf(`<wb:patch id="4" class=%q/>`, class),
		} {
			t.Run(class+"/"+name, func(t *testing.T) {
				st := &State{}
				mustApply(t, st, addXML(class, "t", "plain")+addXML(class, "t", "legacy options")+
					addXML(class, "t", "legacy recommendation")+addXML(ClassPolicy, "t", "choice", "x", "y"))
				// Simulate data written by the older accepting daemon, not a new add.
				st.Items[1].Options = []string{"x", "y"}
				st.Items[2].Recommendation = "a"
				commands := `<wb:patch id="1"><wb:why>valid first</wb:why></wb:patch>` + patch + addXML(ClassCheck, "t", "valid later")
				wantProblems(t, refused(t, st, commands), `use class="policy" for a pick`)
			})
		}
	}
}

func TestNonPolicyOptionsRepairPatches(t *testing.T) {
	for _, class := range []string{ClassTechnical, ClassAnnounced, ClassCheck} {
		for name, patch := range map[string]string{
			"reclass": `<wb:patch id="1" class="policy"/>`,
			"clear":   `<wb:patch id="1" clear="options"><wb:recommend></wb:recommend></wb:patch>`,
		} {
			t.Run(class+"/"+name, func(t *testing.T) {
				st := &State{}
				mustApply(t, st, addXML(class, "t", "legacy"))
				st.Items[0].Options = []string{"x", "y"}
				st.Items[0].Recommendation = "b: safer"
				if name == "clear" {
					// Clearing options alone must not silently erase the recommendation.
					wantProblems(t, refused(t, st, `<wb:patch id="1" clear="options"/>`), `use class="policy" for a pick`)
				}
				mustApply(t, st, patch)
				got := item(t, st, 1)
				if name == "reclass" {
					if got.Class != ClassPolicy || !reflect.DeepEqual(got.Options, []string{"x", "y"}) || got.Recommendation != "b: safer" {
						t.Fatalf("reclass lost choices: %+v", got)
					}
				} else if got.Class != class || len(got.Options) != 0 || got.Recommendation != "" {
					t.Fatalf("clear did not repair item: %+v", got)
				}
			})
		}
	}
	st := &State{}
	mustApply(t, st, addXML(ClassCheck, "t", "legacy options only"))
	st.Items[0].Options = []string{"x"}
	mustApply(t, st, `<wb:patch id="1" clear="options"/>`)
	if got := item(t, st, 1); len(got.Options) != 0 || got.Class != ClassCheck {
		t.Fatalf("clear options alone: %+v", got)
	}
	// The same patch may remove choices while reclassing away from policy.
	mustApply(t, st, addXML(ClassPolicy, "t", "choice", "x", "y"))
	mustApply(t, st, `<wb:patch id="2" class="check" clear="options"/>`)

	// A legacy non-policy item that is no longer open keeps accepting edits
	// that leave its shape alone, and clear="options" repairs it without a
	// re-ask; new choices on it are still refused.
	for _, status := range []string{StatusAnswered, StatusAssumed, StatusWithdrawn, StatusSuperseded} {
		t.Run("closed/"+status, func(t *testing.T) {
			legacy := func() *State {
				st := &State{}
				mustApply(t, st, addXML(ClassCheck, "t", "legacy"))
				st.Items[0].Options = []string{"x", "y"}
				st.Items[0].Recommendation = "b: safer"
				st.Items[0].Status = status
				st.Items[0].Answer = "ok"
				return st
			}
			for _, patch := range []string{
				`<wb:patch id="1" status="superseded"/>`,
				`<wb:patch id="1"><wb:why>Context changed</wb:why></wb:patch>`,
				`<wb:patch id="1"><wb:title>Renamed</wb:title></wb:patch>`,
				`<wb:patch id="1"><wb:link>docs/new.md</wb:link></wb:patch>`,
			} {
				st := legacy()
				mustApply(t, st, patch)
				if got := item(t, st, 1); got.Class != ClassCheck || !reflect.DeepEqual(got.Options, []string{"x", "y"}) || got.Recommendation != "b: safer" || got.Answer != "ok" {
					t.Fatalf("%s changed the legacy shape or answer: %+v", patch, got)
				}
			}
			st := legacy()
			wantProblems(t, refused(t, st, `<wb:patch id="1" clear="options"/>`), `use class="policy" for a pick`)
			mustApply(t, st, `<wb:patch id="1" clear="options"><wb:recommend></wb:recommend></wb:patch>`)
			if got := item(t, st, 1); got.Status != status || got.Answer != "ok" || len(got.Options) != 0 || got.Recommendation != "" {
				t.Fatalf("clear did not repair the closed item in place: %+v", got)
			}
			wantProblems(t, refused(t, legacy(), `<wb:patch id="1"><wb:option>z</wb:option></wb:patch>`), "options change only on an open item")
			wantProblems(t, refused(t, legacy(), `<wb:patch id="1" status="open"><wb:option>z</wb:option></wb:patch>`), "only for policy items")
		})
	}
}

func TestStoredNonPolicyOptionsRenderHiddenWithoutWrites(t *testing.T) {
	for _, class := range []string{ClassCheck, ClassTechnical, ClassAnnounced} {
		for _, status := range []string{StatusOpen, StatusAnswered} {
			for _, recommendationOnly := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/recommendationOnly=%t", class, status, recommendationOnly), func(t *testing.T) {
					legacy := &State{Counter: 1, Seq: 1, Items: []Item{{
						ID: 1, Topic: "t", Class: class, Status: status, Title: "Stored item",
						Recommendation: "a: legacy recommendation marker", Rationale: "Keep this context",
					}}}
					if !recommendationOnly {
						legacy.Items[0].Options = []string{"legacy option marker", "other legacy choice"}
					}
					if status == StatusAnswered {
						legacy.Items[0].Answer = "ok"
						legacy.Items[0].AnswerQuote = "confirmed"
					}
					// Open persists the legacy fixture through the existing store path.
					dir := t.TempDir()
					st := folderOpen(t, dir, legacy)
					before := folderTree(t, dir)
					iv := itemView(st.Items[0])
					if len(iv.Options) != 0 || iv.RecLetter != "" || iv.RecText != "" || iv.Answer != legacy.Items[0].Answer {
						t.Fatalf("non-policy view carries choices or changes answer: %+v", iv)
					}
					for name, layout := range layouts() {
						view := BuildView(st, ViewInput{Layout: layout, ReplyID: "r"})
						renderings := map[string]string{"page": render(t, view)}
						// Dashboard regions require a full-layout view, including its timeline.
						if layout == LayoutFull {
							regions, err := RenderRegions(view)
							if err != nil {
								t.Fatal(err)
							}
							renderings["regions"] = string(regions.Board)
						}
						for kind, html := range renderings {
							for _, hidden := range []string{"legacy option marker", "other legacy choice", "legacy recommendation marker", `data-opt=`, `class="opts`, `class="rec-line`} {
								if strings.Contains(html, hidden) {
									t.Fatalf("%s %s renders non-policy choice %q", name, kind, hidden)
								}
							}
							if !strings.Contains(html, "Stored item") {
								t.Fatalf("%s %s lost the stored item", name, kind)
							}
						}
					}
					if !reflect.DeepEqual(before, folderTree(t, dir)) {
						t.Fatal("render changed stored files")
					}
					reopened := folderOpen(t, dir, nil)
					if !reflect.DeepEqual(reopened.Items, legacy.Items) {
						t.Fatalf("reopen changed stored item: %+v", reopened.Items)
					}
					text := queryText(t, reopened, `{"id":1,"depth":"full"}`)
					wantAll(t, text, "legacy recommendation marker")
					if !recommendationOnly {
						wantAll(t, text, "legacy option marker")
					}
				})
			}
		}
	}
}
