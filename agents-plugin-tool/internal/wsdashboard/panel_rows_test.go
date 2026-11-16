package wsdashboard

import (
	"reflect"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

func panelRowsRebuild(rows *keyedRows) string {
	var out strings.Builder
	for i, key := range rows.order {
		out.WriteString(rows.shell[i])
		out.WriteString(rows.rows[key].HTML)
	}
	out.WriteString(rows.shell[len(rows.order)])
	return out.String()
}

func TestPanelRowsFullHTMLAndHashes(t *testing.T) {
	for _, lang := range []string{"en", "ko"} {
		t.Run(lang, func(t *testing.T) {
			view := wswhiteboard.BuildView(&wswhiteboard.State{}, wswhiteboard.ViewInput{Layout: wswhiteboard.LayoutFull, Panels: []wswhiteboard.Panel{
				{ID: "todo", Title: "Tasks", Rows: []wswhiteboard.PanelRow{{State: "pending", Text: "Keep complete content"}}, Folded: []wswhiteboard.PanelRow{{State: "done", Text: "Retained done"}}},
				{ID: "agenda", Title: "Agendas", Tree: []wswhiteboard.TreeNode{wswhiteboard.JSONTree("agenda/", "plan", []byte(`{"step":{"n":3}}`))}},
				{ID: "tickets", Title: "Tickets", Lanes: []wswhiteboard.PanelLane{{Name: "ready", Rows: []wswhiteboard.PanelRow{{Text: "Ticket", Path: "ai-docs/tickets/ready/fixture.md"}}}}},
				{ID: "mailbox", Title: "Mailbox", Count: "1 unread", Rows: []wswhiteboard.PanelRow{{Label: "peer", Text: "Retained mail"}}},
			}})
			view.Lang = lang
			regions, err := wswhiteboard.RenderRegions(view)
			if err != nil {
				t.Fatal(err)
			}
			raw := string(regions.Panels)
			rows := splitPanelRows(raw)
			if rows == nil || !reflect.DeepEqual(rows.order, []string{"p-todo", "p-agenda", "p-tickets", "p-mailbox"}) {
				t.Fatalf("rows: %#v", rows)
			}
			if panelRowsRebuild(rows) != raw || newRegion(panelRowsRebuild(rows)).Hash != newRegion(raw).Hash {
				t.Fatal("full visible HTML/hash changed")
			}
			for _, key := range rows.order {
				unit := rows.rows[key]
				if unit.Key != key || unit.Hash != newRegion(unit.HTML).Hash {
					t.Fatalf("incomplete hash: %+v", unit)
				}
			}
		})
	}
}

func TestPanelRowsBoundariesAndFallback(t *testing.T) {
	raw := ` <details class="panel" id="p-agenda" open><summary>Agenda</summary><details data-key="agenda/plan"><summary>nested</summary>one</details></details>
<details class="panel" id="p-todo"><summary>Tasks</summary><img src="/image"></details> `
	rows := splitPanelRows(raw)
	if rows == nil || len(rows.order) != 2 {
		t.Fatalf("rows: %#v", rows)
	}
	nextRaw := strings.Replace(raw, "one", "two", 1)
	next := splitPanelRows(nextRaw)
	state := &keyedRowState{name: regionPanels, previous: rows, initialized: true}
	name, payload := state.event(newRegion(nextRaw), next)
	event, ok := payload.(keyedRowsEvent)
	if name != "panel-rows" || !ok || len(event.Rows) != 1 || event.Rows[0].Key != "p-agenda" || event.Hash != newRegion(nextRaw).Hash {
		t.Fatalf("changed card: %s %#v", name, payload)
	}
	for label, changed := range map[string]string{"shell": "\n" + raw, "order": strings.Replace(strings.Replace(raw, `id="p-agenda"`, `id="p-todo"`, 1), `id="p-todo"><summary>Tasks`, `id="p-agenda"><summary>Tasks`, 1), "membership": strings.Replace(raw, `id="p-todo"`, `id="p-mailbox"`, 1)} {
		if name, _ := state.event(newRegion(changed), splitPanelRows(changed)); name != "region" {
			t.Errorf("%s did not use full", label)
		}
	}
	for _, invalid := range []string{
		`<details class="panel"><summary>missing key</summary></details>`,
		`<details class="panel" id="p-"></details>`,
		`<details class="panel" id="p-a" id="p-b"></details>`,
		`<details class="panel" id="p-a"></details><details class="panel" id="p-a"></details>`,
		`<div class="panel" id="p-a"></div>`,
		`<details class="panel" id="p-a"><details class="panel" id="p-b"></details></details>`,
		`<div><details class="panel" id="p-a"></details></div>`,
		`unfamiliar text`,
		`<details class="panel" id="p-a">`,
		`<details class="panel" id="p-a"></div>`,
		`<!--ws-panel-row-0--><details class="panel" id="p-a"></details>`,
	} {
		if splitPanelRows(invalid) != nil {
			t.Errorf("accepted uncertain markup %s", invalid)
		}
	}
	if empty := splitPanelRows(""); empty == nil || len(empty.order) != 0 {
		t.Fatal("empty panel region is a valid baseline")
	}
	if name, _ := (&keyedRowState{name: regionPanels}).event(newRegion(raw), rows); name != "region" {
		t.Fatal("first must be full")
	}
	if name, _ := state.event(newRegion(raw), nil); name != "region" {
		t.Fatal("uncertain must be full")
	}
}
