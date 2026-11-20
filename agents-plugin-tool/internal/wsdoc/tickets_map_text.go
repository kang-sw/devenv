package wsdoc

// termStat is one term's lossless per-channel aggregate in a ticket: w holds
// the weight sums and n the occurrence counts over the channels 0 title,
// 1 heading, 2 code, 3 prose.
type termStat struct {
	w [4]float64
	n [4]int32
}

// mapHeading is one H2-or-deeper body heading, outside fenced code.
type mapHeading struct {
	Level int
	Text  string
}

// ticketDoc is a ticket's feature document for the ticket map. It is
// immutable after buildTicketDoc returns and shared by pointer between the
// cache and every build.
type ticketDoc struct {
	Title    string
	Lines    int
	Headings []mapHeading
	Preview  string
	Mentions []string
	Terms    map[string]*termStat
	// ODQ reports an `## Open Decision Queue` heading outside fenced code
	// (containsOpenDecisionQueue).
	ODQ bool
}

// buildTicketDoc parses one ticket's full text into its feature document.
// closed selects the closed-ref preview budget (700 runes instead of 1500).
func buildTicketDoc(stem, text string, closed bool) *ticketDoc {
	// TODO(leaf: text): implement per the ticket's ticketDoc and Tokenization
	// contracts.
	return &ticketDoc{Title: stem, Terms: map[string]*termStat{}, ODQ: containsOpenDecisionQueue(text)}
}
