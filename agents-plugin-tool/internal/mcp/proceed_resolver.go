package mcp

import (
	"encoding/json"
	"fmt"
	"strings"
)

type proceedInput struct {
	Target proceedTargetInput `json:"target"`
	Facts  proceedFactsInput  `json:"facts,omitempty"`
	Format string             `json:"format,omitempty"`
}

type proceedTargetInput struct {
	Kind       string `json:"kind,omitempty"`
	Label      string `json:"label,omitempty"`
	TicketStem string `json:"ticket_stem,omitempty"`
	TicketPath string `json:"ticket_path,omitempty"`
}

type proceedFactsInput struct {
	Ticket proceedTicketFactsInput `json:"ticket,omitempty"`
	Gates  proceedGateFactsInput   `json:"gates,omitempty"`
	Work   proceedWorkFactsInput   `json:"work,omitempty"`
}

type proceedTicketFactsInput struct {
	TicketMissing factString `json:"ticket_missing,omitempty"`
	HasTicket     factString `json:"has_ticket,omitempty"`
	Status        factString `json:"status,omitempty"`
	Category      factString `json:"category,omitempty"`
	Actionable    factString `json:"actionable,omitempty"`
	Freshness     factString `json:"freshness,omitempty"`
	Phase         factString `json:"phase,omitempty"`
}

type proceedGateFactsInput struct {
	DiscussionNeeded factString `json:"discussion_needed,omitempty"`
	NeedsTicket      factString `json:"needs_ticket,omitempty"`
	ScopeBlocked     factString `json:"scope_blocked,omitempty"`
	MigrationAnchor  factString `json:"migration_anchor,omitempty"`
}

type proceedWorkFactsInput struct {
	Category factString `json:"category,omitempty"`
	Slice    factString `json:"slice,omitempty"`
}

type factString struct {
	Value   string
	Present bool
	Null    bool
}

type proceedResult struct {
	Route        string              `json:"route"`
	Next         string              `json:"next"`
	Target       proceedResultTarget `json:"target"`
	Phase        string              `json:"phase"`
	Reason       string              `json:"reason"`
	Conditions   []string            `json:"conditions"`
	Warnings     []string            `json:"warnings"`
	Agenda       proceedAgenda       `json:"agenda"`
	TodoReplaced bool                `json:"todo_replaced"`
	Raw          string              `json:"raw"`
}

type proceedResultTarget struct {
	Kind       string `json:"kind,omitempty"`
	Label      string `json:"label,omitempty"`
	TicketStem string `json:"ticket_stem,omitempty"`
	TicketPath string `json:"ticket_path,omitempty"`
}

type proceedAgenda struct {
	Route      string              `json:"route"`
	Target     proceedResultTarget `json:"target"`
	Ticket     string              `json:"ticket,omitempty"`
	Phase      string              `json:"phase"`
	NextSkill  string              `json:"next_skill"`
	Conditions []string            `json:"conditions"`
	Warnings   []string            `json:"warnings"`
}

type normalizedProceedFacts struct {
	TargetKind       string
	TicketMissing    string
	HasTicket        string
	Status           string
	MigrationAnchor  string
	Actionable       string
	DiscussionNeeded string
	NeedsTicket      string
	Freshness        string
	Category         string
	Slice            string
	ScopeBlocked     string
}

func parseProceedInput(args map[string]any) (proceedInput, error) {
	format, err := parseProceedFormat(args["format"])
	if err != nil {
		return proceedInput{}, err
	}
	targetMap, ok := args["target"].(map[string]any)
	if !ok {
		if _, exists := args["target"]; !exists {
			return proceedInput{}, fmt.Errorf("target is required")
		}
		return proceedInput{}, fmt.Errorf("target must be an object")
	}
	target, err := parseProceedTarget(targetMap)
	if err != nil {
		return proceedInput{}, err
	}
	facts, err := parseProceedFacts(args["facts"])
	if err != nil {
		return proceedInput{}, err
	}
	return proceedInput{Target: target, Facts: facts, Format: format}, nil
}

func parseProceedFormat(raw any) (string, error) {
	if raw == nil {
		return "text", nil
	}
	s, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("format must be a string")
	}
	switch strings.TrimSpace(strings.ToLower(s)) {
	case "", "text":
		return "text", nil
	case "json":
		return "json", nil
	default:
		return "", fmt.Errorf("invalid format %q: want one of text, json", s)
	}
}

func parseProceedTarget(m map[string]any) (proceedTargetInput, error) {
	kindFact, err := parseObjectString(m, "kind")
	if err != nil {
		return proceedTargetInput{}, fmt.Errorf("target.%w", err)
	}
	kind := normalizeToken(kindFact.Value)
	switch kind {
	case "", "unknown":
		kind = "unknown"
	case "ticket-path", "inline":
	default:
		return proceedTargetInput{}, fmt.Errorf("invalid target.kind %q: want one of ticket-path, inline, unknown", kind)
	}
	label, err := parseObjectString(m, "label")
	if err != nil {
		return proceedTargetInput{}, fmt.Errorf("target.%w", err)
	}
	stem, err := parseObjectString(m, "ticket_stem")
	if err != nil {
		return proceedTargetInput{}, fmt.Errorf("target.%w", err)
	}
	path, err := parseObjectString(m, "ticket_path")
	if err != nil {
		return proceedTargetInput{}, fmt.Errorf("target.%w", err)
	}
	out := proceedTargetInput{
		Kind:       kind,
		Label:      strings.TrimSpace(label.Value),
		TicketStem: strings.TrimSpace(stem.Value),
		TicketPath: strings.TrimSpace(path.Value),
	}
	if out.Label == "" {
		out.Label = firstNonEmpty(out.TicketPath, out.TicketStem, out.Kind)
	}
	return out, nil
}

func parseProceedFacts(raw any) (proceedFactsInput, error) {
	if raw == nil {
		return proceedFactsInput{}, nil
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return proceedFactsInput{}, fmt.Errorf("facts must be an object")
	}
	var out proceedFactsInput
	if group, ok := m["ticket"]; ok && group != nil {
		gm, ok := group.(map[string]any)
		if !ok {
			return proceedFactsInput{}, fmt.Errorf("facts.ticket must be an object")
		}
		var err error
		out.Ticket, err = parseProceedTicketFacts(gm)
		if err != nil {
			return proceedFactsInput{}, err
		}
	}
	if group, ok := m["gates"]; ok && group != nil {
		gm, ok := group.(map[string]any)
		if !ok {
			return proceedFactsInput{}, fmt.Errorf("facts.gates must be an object")
		}
		var err error
		out.Gates, err = parseProceedGateFacts(gm)
		if err != nil {
			return proceedFactsInput{}, err
		}
	}
	if group, ok := m["work"]; ok && group != nil {
		gm, ok := group.(map[string]any)
		if !ok {
			return proceedFactsInput{}, fmt.Errorf("facts.work must be an object")
		}
		var err error
		out.Work, err = parseProceedWorkFacts(gm)
		if err != nil {
			return proceedFactsInput{}, err
		}
	}
	return out, nil
}

func parseProceedTicketFacts(m map[string]any) (proceedTicketFactsInput, error) {
	var out proceedTicketFactsInput
	var err error
	if out.TicketMissing, err = parseEnumFact(m, "ticket_missing", []string{"yes", "no", "unknown"}); err != nil {
		return out, fmt.Errorf("facts.ticket.%w", err)
	}
	if out.HasTicket, err = parseEnumFact(m, "has_ticket", []string{"yes", "no", "unknown"}); err != nil {
		return out, fmt.Errorf("facts.ticket.%w", err)
	}
	if out.Status, err = parseEnumFact(m, "status", []string{"idea", "todo", "ready", "done", "dropped", "unknown", "n/a"}); err != nil {
		return out, fmt.Errorf("facts.ticket.%w", err)
	}
	if out.Category, err = parseEnumFact(m, "category", []string{"epic", "workset", "other", "n/a", "unknown"}); err != nil {
		return out, fmt.Errorf("facts.ticket.%w", err)
	}
	if out.Actionable, err = parseEnumFact(m, "actionable", []string{"yes", "no", "unknown"}); err != nil {
		return out, fmt.Errorf("facts.ticket.%w", err)
	}
	if out.Freshness, err = parseEnumFact(m, "freshness", []string{"current", "missing-settled-decisions", "uncertain", "n/a", "unknown"}); err != nil {
		return out, fmt.Errorf("facts.ticket.%w", err)
	}
	if out.Phase, err = parseObjectString(m, "phase"); err != nil {
		return out, fmt.Errorf("facts.ticket.%w", err)
	}
	return out, nil
}

func parseProceedGateFacts(m map[string]any) (proceedGateFactsInput, error) {
	var out proceedGateFactsInput
	var err error
	if out.DiscussionNeeded, err = parseEnumFact(m, "discussion_needed", []string{"yes", "no", "unknown"}); err != nil {
		return out, fmt.Errorf("facts.gates.%w", err)
	}
	if out.NeedsTicket, err = parseEnumFact(m, "needs_ticket", []string{"yes", "no", "n/a", "unknown"}); err != nil {
		return out, fmt.Errorf("facts.gates.%w", err)
	}
	if out.ScopeBlocked, err = parseEnumFact(m, "scope_blocked", []string{"none", "container-ticket", "multiple-explicit-phases", "too-broad", "no-unfinished-phase", "phase-already-complete", "unknown"}); err != nil {
		return out, fmt.Errorf("facts.gates.%w", err)
	}
	if out.MigrationAnchor, err = parseEnumFact(m, "migration_anchor", []string{"loaded", "n/a", "missing", "conflict", "unknown"}); err != nil {
		return out, fmt.Errorf("facts.gates.%w", err)
	}
	return out, nil
}

func parseProceedWorkFacts(m map[string]any) (proceedWorkFactsInput, error) {
	var out proceedWorkFactsInput
	var err error
	if out.Category, err = parseEnumFact(m, "category", []string{"implementation", "ticket_write", "discussion", "status_report", "unknown"}); err != nil {
		return out, fmt.Errorf("facts.work.%w", err)
	}
	if out.Slice, err = parseObjectString(m, "slice"); err != nil {
		return out, fmt.Errorf("facts.work.%w", err)
	}
	return out, nil
}

func parseEnumFact(m map[string]any, name string, allowed []string) (factString, error) {
	f, err := parseObjectString(m, name)
	if err != nil {
		return factString{}, err
	}
	if !f.Present || f.Null || strings.TrimSpace(f.Value) == "" {
		return f, nil
	}
	f.Value = normalizeToken(f.Value)
	for _, value := range allowed {
		if f.Value == value {
			return f, nil
		}
	}
	return factString{}, fmt.Errorf("invalid %s %q: want one of %s", name, f.Value, strings.Join(allowed, ", "))
}

func parseObjectString(m map[string]any, name string) (factString, error) {
	raw, ok := m[name]
	if !ok {
		return factString{}, nil
	}
	if raw == nil {
		return factString{Present: true, Null: true}, nil
	}
	s, ok := raw.(string)
	if !ok {
		return factString{}, fmt.Errorf("%s must be a string or null", name)
	}
	return factString{Value: strings.TrimSpace(s), Present: true}, nil
}

func resolveProceed(input proceedInput) proceedResult {
	n, warnings := normalizeProceedFacts(input)
	route, next, reason := selectProceedRoute(n)
	target := proceedResultTarget{
		Kind:       n.TargetKind,
		Label:      input.Target.Label,
		TicketStem: input.Target.TicketStem,
		TicketPath: input.Target.TicketPath,
	}
	conditions := proceedConditions(n)
	agenda := proceedAgenda{
		Route:      route,
		Target:     target,
		Ticket:     firstNonEmpty(input.Target.TicketPath, input.Target.TicketStem, input.Target.Label),
		Phase:      n.Slice,
		NextSkill:  next,
		Conditions: conditions,
		Warnings:   warnings,
	}
	result := proceedResult{
		Route:        route,
		Next:         next,
		Target:       target,
		Phase:        n.Slice,
		Reason:       reason,
		Conditions:   conditions,
		Warnings:     warnings,
		Agenda:       agenda,
		TodoReplaced: true,
	}
	result.Raw = renderProceedRaw(result)
	return result
}

func normalizeProceedFacts(input proceedInput) (normalizedProceedFacts, []string) {
	warnings := []string{}
	t := input.Facts.Ticket
	g := input.Facts.Gates
	w := input.Facts.Work
	n := normalizedProceedFacts{
		TargetKind:       input.Target.Kind,
		TicketMissing:    factOr(t.TicketMissing, "unknown"),
		HasTicket:        factOr(t.HasTicket, "unknown"),
		Status:           factOr(t.Status, "unknown"),
		MigrationAnchor:  factOr(g.MigrationAnchor, "n/a"),
		Actionable:       factOr(t.Actionable, "unknown"),
		DiscussionNeeded: factOr(g.DiscussionNeeded, "unknown"),
		NeedsTicket:      factOr(g.NeedsTicket, "unknown"),
		Freshness:        factOr(t.Freshness, "unknown"),
		Category:         factOr(t.Category, "unknown"),
		Slice:            factOr(w.Slice, strings.TrimSpace(t.Phase.Value)),
		ScopeBlocked:     factOr(g.ScopeBlocked, "unknown"),
	}
	if n.Slice == "" {
		n.Slice = "unknown"
	}

	switch n.TargetKind {
	case "inline":
		warnIfPresent(&warnings, input.Target.TicketStem != "", "target.ticket_stem ignored for inline target")
		warnIfPresent(&warnings, input.Target.TicketPath != "", "target.ticket_path ignored for inline target")
		warnFactIfMeaningful(&warnings, t.Status, "facts.ticket.status ignored for inline target")
		warnFactIfMeaningful(&warnings, t.Category, "facts.ticket.category ignored for inline target")
		warnFactIfMeaningful(&warnings, t.Freshness, "facts.ticket.freshness ignored for inline target")
		n.TicketMissing = "no"
		n.HasTicket = "no"
		n.Status = "n/a"
		n.Freshness = "n/a"
		n.Category = "n/a"
		if n.Actionable == "unknown" {
			n.NeedsTicket = normalizeNeedsTicket(n.NeedsTicket)
		}
		if n.Actionable != "yes" {
			n.NeedsTicket = "n/a"
		} else {
			n.NeedsTicket = normalizeNeedsTicket(n.NeedsTicket)
		}
	case "ticket-path":
		if n.TicketMissing == "unknown" {
			n.TicketMissing = "no"
		}
		if n.TicketMissing == "yes" {
			if n.HasTicket == "yes" {
				warnings = append(warnings, "has-ticket=yes ignored because ticket-missing=yes")
			}
			warnFactIfMeaningful(&warnings, t.Status, "facts.ticket.status ignored because ticket-missing=yes")
			n.HasTicket = "no"
			n.Status = "n/a"
			n.Freshness = "n/a"
			n.Category = "n/a"
			n.ScopeBlocked = "none"
		} else {
			if n.HasTicket == "unknown" {
				n.HasTicket = "yes"
			}
			if n.HasTicket == "yes" && n.Actionable != "yes" {
				if t.Actionable.Present && t.Actionable.Value != "yes" {
					warnings = append(warnings, "actionable normalized to yes for ticket-path target")
				}
				n.Actionable = "yes"
			}
		}
		n.NeedsTicket = "n/a"
		if n.HasTicket == "no" {
			n.Freshness = "n/a"
		}
	default:
		n.TargetKind = "unknown"
	}

	if n.MigrationAnchor == "conflict" && n.DiscussionNeeded != "yes" {
		warnings = append(warnings, "discussion-needed normalized to yes because migration-anchor=conflict")
		n.DiscussionNeeded = "yes"
	}
	if n.Category == "epic" || n.Category == "workset" {
		if n.ScopeBlocked != "container-ticket" {
			warnings = append(warnings, "scope-blocked normalized to container-ticket for container ticket")
		}
		if n.Slice != "blocked" {
			warnings = append(warnings, "slice normalized to blocked for container ticket")
		}
		n.ScopeBlocked = "container-ticket"
		n.Slice = "blocked"
	}
	if n.ScopeBlocked != "none" && n.ScopeBlocked != "unknown" && n.Slice != "blocked" {
		if n.ScopeBlocked == "container-ticket" {
			n.Slice = "blocked"
		}
	}
	if n.DiscussionNeeded == "unknown" {
		n.DiscussionNeeded = "no"
		warnings = append(warnings, "discussion-needed missing; normalized to no")
	}
	return n, warnings
}

func selectProceedRoute(n normalizedProceedFacts) (route, next, reason string) {
	if n.TargetKind == "inline" && n.Actionable == "no" {
		return "terminal-artifact.non-actionable-inline", "lead-discuss", "target-kind=inline and actionable=no"
	}
	if n.TicketMissing == "yes" {
		return "terminal-artifact.missing-ticket", "stop", "ticket-missing=yes"
	}
	if n.TargetKind == "ticket-path" {
		switch n.Status {
		case "done":
			return "terminal-artifact.done", "stop", "status=done"
		case "dropped":
			return "terminal-artifact.dropped", "stop", "status=dropped"
		case "unknown":
			return "terminal-artifact.unknown-status", "stop", "status=unknown"
		}
	}
	if n.Category == "epic" || n.Category == "workset" {
		return "container-ticket." + n.Category, "stop", "category=" + n.Category
	}
	if n.MigrationAnchor == "missing" {
		return "anchor-discussion.migration-anchor-missing", "stop", "migration-anchor=missing"
	}
	if n.MigrationAnchor == "conflict" {
		return "anchor-discussion.migration-anchor-conflict", "lead-discuss", "migration-anchor=conflict"
	}
	if n.DiscussionNeeded == "yes" {
		return "anchor-discussion.discussion-needed", "lead-discuss", "discussion-needed=yes"
	}
	if n.Status == "idea" || n.Status == "todo" {
		return "ticket-readiness.status-refresh", "lead-write-ticket", "status=" + n.Status
	}
	if n.Freshness == "missing-settled-decisions" {
		return "ticket-readiness.freshness-refresh", "lead-write-ticket", "freshness=missing-settled-decisions"
	}
	if n.ScopeBlocked != "none" && n.ScopeBlocked != "unknown" {
		return "scope-gate." + n.ScopeBlocked, "stop", "scope-blocked=" + n.ScopeBlocked
	}
	if n.TargetKind == "ticket-path" && n.HasTicket == "yes" && n.Status == "ready" && n.Category == "other" && n.Freshness == "current" && n.ScopeBlocked == "none" {
		return "implementation-dispatch.ready-actionable", "lead-implement", "status=ready and category=other and freshness=current and scope-blocked=none"
	}
	if n.TargetKind == "inline" && n.HasTicket == "no" && n.NeedsTicket == "yes" {
		return "ticket-readiness.inline-needs-ticket", "lead-write-ticket", "has-ticket=no and needs-ticket=yes"
	}
	if n.TargetKind == "inline" && n.HasTicket == "no" && n.NeedsTicket == "no" && n.Actionable == "yes" {
		return "implementation-dispatch.inline-direct", "lead-implement", "has-ticket=no and needs-ticket=no"
	}
	return "fallback.insufficient-route-facts", "stop", "route facts are insufficient or inconsistent"
}

func proceedConditions(n normalizedProceedFacts) []string {
	return []string{
		"target-kind=" + n.TargetKind,
		"ticket-missing=" + n.TicketMissing,
		"has-ticket=" + n.HasTicket,
		"status=" + n.Status,
		"migration-anchor=" + n.MigrationAnchor,
		"actionable=" + n.Actionable,
		"discussion-needed=" + n.DiscussionNeeded,
		"needs-ticket=" + n.NeedsTicket,
		"freshness=" + n.Freshness,
		"category=" + n.Category,
		"slice=" + n.Slice,
		"scope-blocked=" + n.ScopeBlocked,
	}
}

func renderProceedRaw(result proceedResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Proceed Verdict\n")
	fmt.Fprintf(&b, "Route: %s\n", result.Route)
	fmt.Fprintf(&b, "NEXT: %s\n\n", result.Next)
	fmt.Fprintf(&b, "Target: %s\n", firstNonEmpty(result.Target.Label, result.Target.TicketPath, result.Target.TicketStem, "n/a"))
	fmt.Fprintf(&b, "Phase: %s\n", result.Phase)
	fmt.Fprintf(&b, "Reason: %s\n\n", result.Reason)
	b.WriteString("Conditions:\n")
	for _, condition := range result.Conditions {
		fmt.Fprintf(&b, "- %s\n", condition)
	}
	b.WriteString("\nWarnings:\n")
	if len(result.Warnings) == 0 {
		b.WriteString("- none\n")
	} else {
		for _, warning := range result.Warnings {
			fmt.Fprintf(&b, "- %s\n", warning)
		}
	}
	b.WriteString("\nAgenda:\n")
	fmt.Fprintf(&b, "- route: %s\n", result.Agenda.Route)
	fmt.Fprintf(&b, "- ticket: %s\n", firstNonEmpty(result.Agenda.Ticket, "n/a"))
	fmt.Fprintf(&b, "- phase: %s\n", result.Agenda.Phase)
	fmt.Fprintf(&b, "- next_skill: %s\n", result.Agenda.NextSkill)
	fmt.Fprintf(&b, "- conditions: %d normalized facts\n", len(result.Agenda.Conditions))
	fmt.Fprintf(&b, "- warnings: %d\n", len(result.Agenda.Warnings))
	return b.String()
}

func proceedResultJSON(result proceedResult) (string, error) {
	raw, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", err
	}
	return string(raw) + "\n", nil
}

func factOr(f factString, fallback string) string {
	if !f.Present || f.Null || strings.TrimSpace(f.Value) == "" {
		return fallback
	}
	return normalizeToken(f.Value)
}

func normalizeNeedsTicket(raw string) string {
	switch raw {
	case "yes", "no", "n/a":
		return raw
	default:
		return "unknown"
	}
}

func normalizeToken(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func warnFactIfMeaningful(warnings *[]string, f factString, message string) {
	if !f.Present || f.Null {
		return
	}
	value := normalizeToken(f.Value)
	if value == "" || value == "unknown" || value == "n/a" {
		return
	}
	*warnings = append(*warnings, message)
}

func warnIfPresent(warnings *[]string, present bool, message string) {
	if present {
		*warnings = append(*warnings, message)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
