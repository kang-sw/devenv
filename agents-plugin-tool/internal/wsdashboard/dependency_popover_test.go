package wsdashboard

import (
	"os"
	"strings"
	"testing"
)

func TestDependencyPopoverCSSScope(t *testing.T) {
	data, err := os.ReadFile("../wswhiteboard/assets/page.css")
	if err != nil {
		t.Fatal(err)
	}
	_, rules, ok := strings.Cut(string(data), "/* Dependency popovers")
	if !ok {
		t.Fatal("dependency styles missing")
	}
	rules, _, ok = strings.Cut(rules, ".tbar .tago")
	if !ok {
		t.Fatal("dependency style boundary missing")
	}
	// Retain the opening comment marker for cssClasses' comment stripper.
	rules = "/* Dependency popovers" + rules
	global := cssClasses(rules, "dw")
	if len(global) != 1 || !global["dw"] {
		t.Fatalf("popover selectors must all be scoped by .dw, got globals: %v", global)
	}
	all := cssClasses(rules, "")
	for _, c := range []string{"pop", "sum", "bar", "row", "dot", "sep", "lb", "nm", "st", "ar", "bd", "dbtn"} {
		if !all[c] {
			t.Errorf("missing scoped mockup class .%s", c)
		}
	}
	// The collision scan still sees unscoped rules and lookalike roots.
	probe := cssClasses(".dw .row {} .row {} .dw-other .pop {}", "dw")
	if !probe["row"] || !probe["pop"] || !probe["dw-other"] {
		t.Fatalf("scope handling hid unscoped collisions: %v", probe)
	}
}
