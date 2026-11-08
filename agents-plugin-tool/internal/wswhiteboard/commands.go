package wswhiteboard

import "github.com/kang-sw/devenv/internal/wsxmlcmd"

// The whiteboard.update input is one string of flat commands in the shared
// XML-like command language (package wsxmlcmd states its rules); this file
// is the whiteboard's grammar for it: the "wb:" prefix and the command table.

var itemFieldNames = []string{"title", "option", "recommend", "why", "link"}

// grammar is the whiteboard's command language; the commands follow the
// order of the playbook's table. Item fields are plain text, so an escaped
// one is decoded; a topic body (raw HTML), wb:edit's old and new (matched
// against a stored body), and an answer (the user's verbatim words) are not.
var grammar = wsxmlcmd.Grammar{
	Prefix: "wb:",
	Commands: map[string]wsxmlcmd.Spec{
		"add":       {Kind: wsxmlcmd.Container, Attrs: []string{"id", "class", "topic"}, Fields: itemFieldNames, DecodeFields: itemFieldNames},
		"answer":    {Kind: wsxmlcmd.Leaf, Attrs: []string{"id", "ids", "value", "assumed"}},
		"patch":     {Kind: wsxmlcmd.Container, Attrs: []string{"id", "status", "clear"}, Fields: itemFieldNames, DecodeFields: itemFieldNames},
		"topic":     {Kind: wsxmlcmd.Leaf, Attrs: []string{"name"}},
		"edit":      {Kind: wsxmlcmd.Container, Attrs: []string{"topic", "replace_all"}, Fields: []string{"old", "new"}},
		"close":     {Kind: wsxmlcmd.Empty, Attrs: []string{"topic"}},
		"floor":     {Kind: wsxmlcmd.Empty, Attrs: []string{"id"}},
		"embed-dir": {Kind: wsxmlcmd.Empty, Attrs: []string{"path"}},
	},
}

// command and problems are the shared parser's types under the names the
// whiteboard's validation uses.
type (
	command  = wsxmlcmd.Command
	problems = wsxmlcmd.Problems
)

// parseCommands reads every whiteboard command in s, recording each problem
// in p.
func parseCommands(p *problems, s string) []command {
	return wsxmlcmd.Parse(p, grammar, s)
}
