package wswhiteboard

import "github.com/kang-sw/devenv/internal/wsxmlcmd"

// The whiteboard.update input is one string of commands in the shared
// XML-like command language (package wsxmlcmd states its rules); this file
// is the whiteboard's grammar for it: the "wb:" prefix, the wb:topic group,
// and the command table.

var itemFieldNames = []string{"title", "option", "recommend", "why", "link"}

// grammar is the whiteboard's command language; the commands follow the
// order of the playbook's table. A wb:topic group names the topic its
// commands (wb:body, wb:edit, wb:recap, wb:add, wb:close) act on, so a topic's long
// name is written once per batch; its optional label attribute makes the
// name a stem and sets the topic's label (applyLabel), and a labelled group
// may hold no commands. Item fields are plain text, so an escaped
// one is decoded; a topic body (raw HTML), a recap (written as a body is),
// wb:edit's old and new (matched against a stored body), and an answer (the
// user's verbatim words) are not.
var grammar = wsxmlcmd.Grammar{
	Prefix: "wb:",
	// wb:topic once took the body itself; the hint shows the form that
	// replaced it to a caller still writing the old one.
	Group: &wsxmlcmd.Group{Name: "topic", Attr: "name", Extra: "label", Hint: `a topic body goes in <wb:body> inside the group, and an empty <wb:body/> clears it: <wb:topic name="..."><wb:body>...</wb:body></wb:topic>`},
	Commands: map[string]wsxmlcmd.Spec{
		"body":      {Kind: wsxmlcmd.Leaf, Scoped: true},
		"edit":      {Kind: wsxmlcmd.Container, Attrs: []string{"replace_all"}, Fields: []string{"old", "new"}, Scoped: true},
		"recap":     {Kind: wsxmlcmd.Leaf, Scoped: true},
		"add":       {Kind: wsxmlcmd.Container, Attrs: []string{"id", "class"}, Fields: itemFieldNames, DecodeFields: itemFieldNames, Scoped: true},
		"close":     {Kind: wsxmlcmd.Empty, Attrs: []string{"result"}, Scoped: true},
		"answer":    {Kind: wsxmlcmd.Leaf, Attrs: []string{"id", "ids", "value", "assumed"}},
		"patch":     {Kind: wsxmlcmd.Container, Attrs: []string{"id", "status", "class", "clear"}, Fields: itemFieldNames, DecodeFields: itemFieldNames},
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
