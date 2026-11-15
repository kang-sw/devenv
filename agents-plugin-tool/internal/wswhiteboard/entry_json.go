package wswhiteboard

import "encoding/json"

// Preserve optional and future Entry members without making them model authority.
// Committed folder log lines are never reserialized; these methods also retain
// unknown members in newly read in-memory entries and explicit retrievals.
type entryJSON Entry

func (e *Entry) UnmarshalJSON(data []byte) error {
	var known entryJSON
	if err := json.Unmarshal(data, &known); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for _, key := range []string{"id", "kind", "panel_id", "item_id", "recap", "text", "at", "imported", "topic_state", "board_notice"} {
		delete(fields, key)
	}
	*e = Entry(known)
	if len(fields) != 0 {
		e.unknown = fields
	}
	return nil
}

func (e Entry) MarshalJSON() ([]byte, error) {
	data, err := json.Marshal(entryJSON(e))
	if err != nil || len(e.unknown) == 0 {
		return data, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	for key, value := range e.unknown {
		if _, known := fields[key]; !known {
			fields[key] = value
		}
	}
	return json.Marshal(fields)
}

func (e Entry) clone() Entry {
	e.TopicState = append(json.RawMessage(nil), e.TopicState...)
	e.BoardNotice = append(json.RawMessage(nil), e.BoardNotice...)
	if e.unknown != nil {
		copy := make(map[string]json.RawMessage, len(e.unknown))
		for key, value := range e.unknown {
			copy[key] = append(json.RawMessage(nil), value...)
		}
		e.unknown = copy
	}
	return e
}
