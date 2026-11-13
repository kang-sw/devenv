package mcp

import (
	"fmt"
	"strings"

	"github.com/kang-sw/devenv/internal/wsrsrc"
)

// The command language (package wsxmlcmd) is stated once, in the workflow
// manual playbook's command-syntax part, and every tool whose input is that
// language points at it instead of carrying its own copy: the workflow manual
// renders it once ahead of the tools' sections, a refusal points to the
// tool's playbook, and playbook.read of such a tool's playbook appends it.

const commandSyntaxPart = "command-syntax"

// commandSyntaxEnabled is the one condition for the manual's shared command
// syntax section: any enabled tool whose input is the command language. A new
// such tool adds its enable knob here.
func commandSyntaxEnabled(whiteboardOn, trackerOn bool) bool {
	return whiteboardOn || trackerOn
}

// commandSyntaxSection is the shared section as the part holds it, headed
// "## Command syntax".
func commandSyntaxSection(rsrcRoot string) (string, error) {
	section, err := wsrsrc.LoadPart(rsrcRoot, workflowManualPlaybookName, commandSyntaxPart)
	if err != nil {
		return "", fmt.Errorf("read %s/%s: %w", workflowManualPlaybookName, commandSyntaxPart, err)
	}
	return section, nil
}

// commandSyntaxManualSection is the shared section with its headings demoted
// one level, to sit beside the manual's other ### sections.
func commandSyntaxManualSection(rsrcRoot string) (string, error) {
	section, err := commandSyntaxSection(rsrcRoot)
	if err != nil {
		return "", err
	}
	lines := strings.Split(section, "\n")
	inFence := false
	for i, line := range lines {
		if strings.HasPrefix(line, "```") {
			inFence = !inFence
		}
		if !inFence && strings.HasPrefix(line, "#") {
			lines[i] = "#" + line
		}
	}
	return strings.Join(lines, "\n"), nil
}

// appendCommandSyntax appends the shared section to a command-language
// tool's playbook as playbook.read prints it, so the playbook alone states
// the whole input format.
func appendCommandSyntax(rsrcRoot, body string) (string, error) {
	section, err := commandSyntaxSection(rsrcRoot)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(body, "\n") + "\n\n" + section, nil
}

// commandRefusal lists every problem and points to the tool's playbook.
func (s *Server) commandRefusal(tool string, problems []string, playbook string) string {
	var sb strings.Builder
	writeRefusalProblems(&sb, tool, problems)
	fmt.Fprintf(&sb, "\nSyntax and commands: the `%s` playbook (%s/playbook.read).\n", playbook, s.runtimeNamespace())
	return sb.String()
}

// writeRefusalProblems writes a refusal's header and its problem list.
func writeRefusalProblems(sb *strings.Builder, tool string, problems []string) {
	fmt.Fprintf(sb, "%s refused; nothing was applied. Problems:\n", tool)
	for _, problem := range problems {
		sb.WriteString("- " + problem + "\n")
	}
}
