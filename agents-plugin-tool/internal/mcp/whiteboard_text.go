package mcp

import (
	"fmt"
	"strings"
)

// Knob-on workflow text for workflow.whiteboard_enabled. It is applied
// code-side, after printPlaybook, by each reader that resolves the knob with
// its own scopes (the playbook.read handler and the workflow_manual tool), so
// the rsrc mirrors stay byte-identical and the template engine needs no
// conditional. "{{ns}}" stands for the runtime namespace.
// Each variant names the shipped text it replaces; when a prompt override or
// a later edit removed that text, the variant is appended instead of lost.

const (
	lineDiscussionNoCounter = "- Discussion keeps no counter. After compaction, continue from the highest ID\n" +
		"  the summary shows; a collision with an ID the summary dropped is accepted,\n" +
		"  since nothing in discussion persists."
	lineDiscussionStoreCounter = "- Discussion IDs come from the whiteboard store: an item gets its ID when\n" +
		"  `{{ns}}/whiteboard.update` adds it, and the store keeps the counter across\n" +
		"  compaction. When a ticket queue you have read has a last-ID line above the\n" +
		"  store's highest ID, send it as `<wb:floor id=\"...\"/>`."

	lineDiscussNotPersisted = "No block format and no queue;\n  nothing here is persisted."
	// whiteboardCaptureLine is shared word for word by the whiteboard
	// playbook (as `{{.SkillNamespace}}:lead-ticket`) and the knob-on
	// lead-discuss variant; a test pins the two together.
	whiteboardCaptureLine = "Confirmed decisions reach a ticket only through `{{ns}}:lead-ticket`, once the user moves to capture; the board clears with the session key."

	lineTicketQueueStateEnd = "\n\n### Trace a change before writing it"
	lineTicketQueueBoard    = "- When you open or resume a queue, put its open items on the whiteboard:\n" +
		"  `{{ns}}/whiteboard.query` with `ids` set to the queue's item IDs, then in one\n" +
		"  `{{ns}}/whiteboard.update` `wb:add` each open queue item\n" +
		"  the query does not return, under its ticket ID (`id`), with\n" +
		"  a `wb:floor` at the queue's last-ID line so the store never re-mints an ID\n" +
		"  already settled into `## Decisions`. Apply each settled answer to the store\n" +
		"  through `wb:answer` as well as to the ticket."

	whiteboardManualMandate = "### Whiteboard\n\n" +
		"The whiteboard is your primary channel for what you ask the user. Every item\n" +
		"you ask of the user (discussion items and Open Decision Queue items) goes on\n" +
		"the whiteboard through `{{ns}}/whiteboard.update`, and context that outlasts\n" +
		"a reply goes in its topic's body. Progress and explanations stay in the\n" +
		"terminal or chat, which never repeats what the board holds.\n\n" +
		"- Asks a skill or the project reserves for the user (the next-step handoff\n" +
		"  of `{{ns}}:lead-review`, the publish confirmation of `{{ns}}:lead-ship`,\n" +
		"  Approval Protocol asks) remain asks, shown on the whiteboard.\n" +
		"- Output lines a playbook mandates in a fixed format (for example\n" +
		"  `Ticket: <path>`) stay in the terminal.\n\n" +
		"The rest of this section is the `whiteboard` playbook.\n"
)

func withNamespace(text, namespace string) string {
	return strings.ReplaceAll(text, "{{ns}}", namespace)
}

// replaceOrAppend replaces the first old in body with replacement, or
// appends replacement as its own paragraph when old is absent.
func replaceOrAppend(body, old, replacement string) string {
	if strings.Contains(body, old) {
		return strings.Replace(body, old, replacement, 1)
	}
	return strings.TrimRight(body, "\n") + "\n\n" + replacement
}

// applyKnobVariants applies the knob-on variants of the command-language
// tools for one playbook. The workflow manual gets the shared command syntax
// section once (commandSyntaxEnabled), then the whiteboard section, then the
// tracker section; another playbook gets its whiteboard variants. Off knobs
// return body unchanged.
func applyKnobVariants(s *Server, rsrcRoot, name, body string, whiteboardOn, trackerOn bool) (string, error) {
	if name == workflowManualPlaybookName && commandSyntaxEnabled(whiteboardOn, trackerOn) {
		section, err := commandSyntaxManualSection(rsrcRoot)
		if err != nil {
			return "", err
		}
		body = strings.TrimRight(body, "\n") + "\n\n" + section
	}
	body, err := applyWhiteboardVariants(s, rsrcRoot, name, body, whiteboardOn)
	if err != nil {
		return "", err
	}
	if name == workflowManualPlaybookName && trackerOn {
		section, err := trackerManualSection(s, rsrcRoot)
		if err != nil {
			return "", err
		}
		body = strings.TrimRight(body, "\n") + "\n\n" + section
	}
	return body, nil
}

// applyWhiteboardVariants applies the whiteboard's knob-on variants for one
// playbook when enabled (see Server.whiteboardEnabled); a disabled knob or
// another playbook returns body unchanged. The shared command syntax section
// ahead of it is applyKnobVariants'.
func applyWhiteboardVariants(s *Server, rsrcRoot, name, body string, enabled bool) (string, error) {
	if !enabled {
		return body, nil
	}
	switch name {
	case workflowManualPlaybookName:
		body = replaceOrAppend(body, lineDiscussionNoCounter, withNamespace(lineDiscussionStoreCounter, s.runtimeNamespace()))
		section, err := whiteboardManualSection(s, rsrcRoot)
		if err != nil {
			return "", err
		}
		body = strings.TrimRight(body, "\n") + "\n\n" + section
	case "lead-discuss":
		body = replaceOrAppend(body, lineDiscussNotPersisted, "No block format and no queue.\n  "+withNamespace(whiteboardCaptureLine, s.runtimeNamespace()))
	case "lead-ticket":
		bullet := withNamespace(lineTicketQueueBoard, s.runtimeNamespace())
		if strings.Contains(body, lineTicketQueueStateEnd) {
			body = strings.Replace(body, lineTicketQueueStateEnd, "\n"+bullet+lineTicketQueueStateEnd, 1)
		} else {
			body = strings.TrimRight(body, "\n") + "\n\n" + bullet
		}
	}
	return body, nil
}

// whiteboardManualSection is the manual's knob-on section: the mandate, then
// the whiteboard playbook's own content (its title dropped, its headings
// demoted under the section), so the lead needs no extra read.
func whiteboardManualSection(s *Server, rsrcRoot string) (string, error) {
	playbook, _, err := renderPlaybookBody(s, rsrcRoot, whiteboardPlaybookName, nil, s.configOptions(), "", "", "", nil, "")
	if err != nil {
		return "", fmt.Errorf("render %s playbook: %w", whiteboardPlaybookName, err)
	}
	var sb strings.Builder
	sb.WriteString(withNamespace(whiteboardManualMandate, s.runtimeNamespace()))
	inFence := false
	for _, line := range strings.Split(strings.TrimSpace(playbook), "\n") {
		if strings.HasPrefix(line, "```") {
			inFence = !inFence
		}
		if !inFence {
			if strings.HasPrefix(line, "# ") {
				continue
			}
			if strings.HasPrefix(line, "## ") {
				line = "##" + line
			}
		}
		sb.WriteString(line + "\n")
	}
	return strings.TrimRight(sb.String(), "\n"), nil
}
