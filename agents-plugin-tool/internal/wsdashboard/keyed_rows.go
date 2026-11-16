package wsdashboard

import "slices"

// keyedRows is an immutable decomposition of a shared render. Region-specific
// splitters retain every byte in shell fragments and complete HTML-hashed rows.
// Only trees have parents; their shell contains private descendant slots.
type keyedRows struct {
	order   []string
	rows    map[string]keyedRow
	shell   []string
	parents []int
}

type keyedRow struct {
	Key  string `json:"key"`
	Hash string `json:"hash"`
	HTML string `json:"html"`
}

type keyedRowsEvent struct {
	Hash  string     `json:"hash"`
	Order []string   `json:"order"`
	Rows  []keyedRow `json:"rows"`
}

// Baselines belong to a connection, never the shared page. A successful full
// initializes even an unparseable region; reconnects force a new first full.
type keyedRowState struct {
	name        string
	previous    *keyedRows
	initialized bool
}

// rowRegion pairs a connection's state with this render's immutable input.
// A nil state preserves legacy full-region delivery for that region.
type rowRegion struct {
	state *keyedRowState
	next  *keyedRows
}

func (s *keyedRowState) event(region renderedRegion, next *keyedRows) (string, any) {
	// Rail groups permit row reordering when their shell is unchanged. Other
	// regions conservatively require the same window/order and tree topology.
	if next != nil && s.previous != nil && slices.Equal(next.shell, s.previous.shell) &&
		(s.name == regionRail || (slices.Equal(next.order, s.previous.order) && slices.Equal(next.parents, s.previous.parents))) {
		e := keyedRowsEvent{Hash: region.Hash, Order: next.order, Rows: []keyedRow{}}
		for _, key := range next.order {
			row := next.rows[key]
			if old, ok := s.previous.rows[key]; !ok || old.Hash != row.Hash {
				e.Rows = append(e.Rows, row)
			}
		}
		name := s.name + "-rows"
		if s.name == regionEntries {
			name = "entry-rows"
		} else if s.name == regionPanels {
			name = "panel-rows"
		}
		return name, e
	}
	return "region", regionEvent{Name: s.name, Hash: region.Hash, HTML: string(region.HTML)}
}
