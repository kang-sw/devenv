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
	ticket, err := TicketsStatus(root, TicketStatusOptions{TicketStem: ticketStem, IncludeDone: true, IncludeDropped: true})
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

func ticketsFromSpecRefs(root string, specs []SpecInfo) []TicketInfo {
	out := []TicketInfo{}
	seen := map[string]bool{}
	for _, spec := range specs {
		for _, stem := range spec.TicketRefs {
			if seen[stem] {
				continue
			}
			seen[stem] = true
			ticket, err := TicketsStatus(root, TicketStatusOptions{TicketStem: stem, IncludeDone: true, IncludeDropped: true})
			if err != nil {
				continue
			}
			out = append(out, *ticket)
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
