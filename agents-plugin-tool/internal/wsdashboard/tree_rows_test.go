package wsdashboard

import (
	"reflect"
	"strings"
	"testing"
)

func treeTestRow(id, text, children string) string {
	kids := ""
	if children != "" {
		kids = `<div class="kids">` + children + `</div>`
	}
	return `<div class="agent running"><details data-key="agent-` + id + `"><summary>` + text + `</summary></details><ol class="brows live"></ol>` + kids + `<template class="model-source">hover</template></div>`
}

func TestTreeRowsNestedDiffAndParity(t *testing.T) {
	raw := `<div class="thead">header</div><div class="tree">` + treeTestRow("a", "A", treeTestRow("b", "B", "")) + treeTestRow("c", "C", "") + `</div>`
	first := splitTreeRows(raw)
	if first == nil || !reflect.DeepEqual(first.order, []string{"agent-a", "agent-b", "agent-c"}) || !reflect.DeepEqual(first.parents, []int{-1, 0, -1}) {
		t.Fatalf("bad units: %+v", first)
	}
	var render func(string) string
	render = func(markup string) string {
		for i, key := range first.order {
			if strings.Contains(markup, treeRowSlot(i)) {
				markup = strings.ReplaceAll(markup, treeRowSlot(i), render(first.rows[key].HTML))
			}
		}
		return markup
	}
	if got := render(first.shell[0]); got != raw {
		t.Fatalf("changed bytes: %s", got)
	}
	for _, r := range first.rows {
		if r.Hash != newRegion(r.HTML).Hash {
			t.Fatal("partial hash")
		}
	}
	nextRaw := strings.Replace(raw, `<summary>B</summary>`, `<summary>new B</summary>`, 1)
	next := splitTreeRows(nextRaw)
	state := keyedRowState{name: regionTree, previous: first, initialized: true}
	name, data := state.event(newRegion(nextRaw), next)
	patch, ok := data.(keyedRowsEvent)
	if name != "tree-rows" || !ok || len(patch.Rows) != 1 || patch.Rows[0].Key != "agent-b" || !reflect.DeepEqual(patch.Order, first.order) {
		t.Fatalf("nested edit invalidated unrelated rows: %s %+v", name, data)
	}
	if first.rows["agent-a"].Hash != next.rows["agent-a"].Hash {
		t.Fatal("child edit invalidates parent skeleton")
	}
}

func TestTreeRowsFallbacks(t *testing.T) {
	raw := `<div class="tree">` + treeTestRow("a", "A", treeTestRow("b", "B", "")) + `</div>`
	first := splitTreeRows(raw)
	for _, nextRaw := range []string{strings.Replace(raw, "tree", "tree changed", 1), `<div class="tree">` + treeTestRow("a", "A", "") + treeTestRow("b", "B", "") + `</div>`, treeTestRow("b", "B", treeTestRow("a", "A", "")), treeTestRow("a", "A", "")} {
		state := keyedRowState{name: regionTree, previous: first, initialized: true}
		if name, _ := state.event(newRegion(nextRaw), splitTreeRows(nextRaw)); name != "region" {
			t.Fatal("uncertain shell/topology did not fall back")
		}
	}
	state := keyedRowState{name: regionTree}
	if name, _ := state.event(newRegion(raw), first); name != "region" {
		t.Fatal("first must be full")
	}
	state.previous = first
	if name, _ := state.event(newRegion(raw), nil); name != "region" {
		t.Fatal("nil decomposition must be full")
	}
}

func TestTreeRowsRejectUncertainMarkup(t *testing.T) {
	for _, raw := range []string{treeTestRow("a", "A", "") + treeTestRow("a", "duplicate", ""), `<div class="agent"><details><summary>missing key</summary></details></div>`, `<div class="agent"><details data-key="agent-a">broken`, treeTestRow("a", treeRowSlot(0), "")} {
		if splitTreeRows(raw) != nil {
			t.Fatalf("accepted uncertain markup: %s", raw)
		}
	}
}

func TestTreeRowsPeersAndEmpty(t *testing.T) {
	raw := `<div class="tree"><details class="peer live" data-peer="other"><summary>peer</summary><div class="ptree"><div class="kids">` + treeTestRow("a", "A", "") + `</div></div></details><div class="peer compact">unkeyed stays shell</div></div>`
	rows := splitTreeRows(raw)
	if rows == nil || !reflect.DeepEqual(rows.order, []string{"peer-other", "agent-a"}) || !reflect.DeepEqual(rows.parents, []int{-1, 0}) {
		t.Fatalf("bad peer topology: %+v", rows)
	}
	if strings.Contains(rows.rows["peer-other"].HTML, `data-key="agent-a"`) {
		t.Fatal("peer unit includes its child")
	}
	if rows := splitTreeRows(""); rows == nil || len(rows.order) != 0 {
		t.Fatal("empty fallback baseline unavailable")
	}
}
