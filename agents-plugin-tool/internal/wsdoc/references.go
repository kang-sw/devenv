package wsdoc

import (
	"fmt"
	"strings"
)

type ReferenceTraceOptions struct {
	TicketStem string
	SpecStem   string
}

type ReferenceTrace struct {
	InputType    string            `json:"input_type"`
	Input        string            `json:"input"`
	Tickets      []TicketInfo      `json:"tickets,omitempty"`
	Specs        []SpecInfo        `json:"specs,omitempty"`
	MentalModels []MentalModelInfo `json:"mental_models,omitempty"`
}

func ReferencesTrace(root string, opts ReferenceTraceOptions) (*ReferenceTrace, error) {
	ticketStem := strings.TrimSpace(opts.TicketStem)
	specStem := strings.TrimSpace(opts.SpecStem)
	if (ticketStem == "") == (specStem == "") {
		return nil, fmt.Errorf("exactly one of ticket_stem or spec_stem is required")
	}
	if ticketStem != "" {
		return traceTicketReferences(root, ticketStem)
	}
	return traceSpecReferences(root, specStem)
}

func traceTicketReferences(root, ticketStem string) (*ReferenceTrace, error) {
	// Resolve: references.trace is a resolution surface end to end — every
	// branch answers "what does this stem point at", so a stem hidden by a
	// worktree scope must still resolve.
	ticket, err := TicketsStatus(root, TicketStatusOptions{TicketStem: ticketStem, IncludeDone: true, IncludeDropped: true, Resolve: true})
	if err != nil {
		return nil, err
	}
	trace := &ReferenceTrace{
		InputType: "ticket",
		Input:     ticketStem,
		Tickets:   []TicketInfo{*ticket},
	}
	specs, err := SpecsFind(root, SpecFindOptions{TicketStem: ticketStem})
	if err != nil {
		return nil, err
	}
	specs = mergeSpecInfos(specs, specsFromTicketFrontmatter(root, *ticket)...)
	trace.Specs = specs
	trace.MentalModels = mentalModelsForSpecs(root, specs)
	return trace, nil
}

func traceSpecReferences(root, specStem string) (*ReferenceTrace, error) {
	status, err := SpecsStatus(root, SpecStatusOptions{SpecStem: specStem})
	if err != nil {
		return nil, err
	}
	trace := &ReferenceTrace{
		InputType: "spec",
		Input:     specStem,
		Specs:     status.Files,
	}
	tickets, err := TicketsFind(root, TicketFindOptions{
		IncludeDone:        true,
		IncludeDropped:     true,
		MentionsTicketStem: "",
		Query:              specStem,
		// Resolve: this query form must supply hidden bodies from the index
		// rather than skip them, or the spec branch stops matching tickets this
		// worktree does not check out.
		Resolve: true,
	})
	if err != nil {
		return nil, err
	}
	trace.Tickets = mergeTicketInfos(tickets, ticketsFromSpecRefs(root, status.Files)...)
	models, err := MentalModelsFind(root, MentalModelFindOptions{SpecStem: specStem})
	if err != nil {
		return nil, err
	}
	trace.MentalModels = models
	return trace, nil
}

// ticketsFromSpecRefs resolves every referenced stem from ONE board scan rather
// than one TicketsStatus call per stem. Each per-stem call constructed a fresh
// ticketScope (there is no cross-call memoization, by design), so under an
// active scope a spec referenced by N tickets cost ~2N git subprocesses on top
// of the N full-board rescans that were already there. The lookup below
// reproduces TicketsStatus's answer exactly: same scan options, same sorted
// order, first-wins per stem — which is what "the first ticket whose stem
// matches" already meant.
func ticketsFromSpecRefs(root string, specs []SpecInfo) []TicketInfo {
	out := []TicketInfo{}
	tickets, err := scanTickets(root, ticketScanOptions{IncludeDone: true, IncludeDropped: true, Resolve: resolveFull})
	if err != nil {
		return out
	}
	byStem := make(map[string]TicketInfo, len(tickets))
	for _, ticket := range tickets {
		if _, ok := byStem[ticket.Stem]; !ok {
			byStem[ticket.Stem] = ticket
		}
	}
	seen := map[string]bool{}
	for _, spec := range specs {
		for _, stem := range spec.TicketRefs {
			if seen[stem] {
				continue
			}
			seen[stem] = true
			// Mirrors TicketsStatus's stem-shape rejection, which returned an
			// error that this loop skipped.
			if !ticketStemRE.MatchString(strings.TrimSpace(stem)) {
				continue
			}
			if ticket, ok := byStem[stem]; ok {
				out = append(out, ticket)
			}
		}
	}
	return out
}

func specsFromTicketFrontmatter(root string, ticket TicketInfo) []SpecInfo {
	out := []SpecInfo{}
	seen := map[string]bool{}
	for _, stem := range ticket.Specs {
		if seen[stem] {
			continue
		}
		seen[stem] = true
		status, err := SpecsStatus(root, SpecStatusOptions{SpecStem: stem})
		if err != nil {
			continue
		}
		out = append(out, status.Files...)
	}
	return out
}

func mentalModelsForSpecs(root string, specs []SpecInfo) []MentalModelInfo {
	merged := []MentalModelInfo{}
	seen := map[string]bool{}
	for _, spec := range specs {
		for _, anchor := range spec.Anchors {
			models, err := MentalModelsFind(root, MentalModelFindOptions{SpecStem: anchor.SpecStem})
			if err != nil {
				continue
			}
			for _, model := range models {
				if seen[model.Path] {
					continue
				}
				seen[model.Path] = true
				merged = append(merged, model)
			}
		}
	}
	return merged
}

func mergeSpecInfos(first []SpecInfo, rest ...SpecInfo) []SpecInfo {
	out := []SpecInfo{}
	seen := map[string]bool{}
	for _, spec := range append(first, rest...) {
		if seen[spec.Path] {
			continue
		}
		seen[spec.Path] = true
		out = append(out, spec)
	}
	return out
}

func mergeTicketInfos(first []TicketInfo, rest ...TicketInfo) []TicketInfo {
	out := []TicketInfo{}
	seen := map[string]bool{}
	for _, ticket := range append(first, rest...) {
		if seen[ticket.Stem] {
			continue
		}
		seen[ticket.Stem] = true
		out = append(out, ticket)
	}
	return out
}
