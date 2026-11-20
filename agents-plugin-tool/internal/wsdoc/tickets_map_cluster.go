package wsdoc

// mapAnalysis is analyzeTicketMap's output, indexed like its nodes argument.
type mapAnalysis struct {
	cluster      []int                         // per node: its cluster id
	kw           [][]TicketMapTerm             // per node: top-6 vector terms
	clusters     []TicketMapCluster            // ids 0..K-1, Misc last
	keywords     map[string][]TicketMapKeyword // cluster id (decimal) -> soup
	sim          []TicketMapSim                // sorted by (I, J)
	mentionPairs int
	gamma        float64 // raw; the assembler rounds to 1 decimal
	modularity   float64 // raw; the assembler rounds to 3 decimals
	dfCut        int
	dfCutTop     []string
	params       TicketMapParams
}

// analyzeTicketMap runs vectors, similarity, the graph, Louvain, naming and
// the Keyword Soup over the included nodes (open first, then closed refs) and
// their explicit edges. It is deterministic in its inputs.
func analyzeTicketMap(nodes []mapNode, edges []mapEdge) mapAnalysis {
	// TODO(leaf: cluster): implement per the ticket's Vectors through Keyword
	// Soup contracts.
	return mapAnalysis{cluster: make([]int, len(nodes))}
}
