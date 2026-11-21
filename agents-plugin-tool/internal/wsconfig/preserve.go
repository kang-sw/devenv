package wsconfig

import (
	"encoding/json"
)

// preserveConfigMembers projects the edited known fields onto the original
// object. Agent tiers are typed, so preserve future fields inside the
// agents object and beside the fields of each surviving tier leaf.
func preserveConfigMembers(original []byte, cfg Config) ([]byte, error) {
	var members map[string]json.RawMessage
	if len(original) > 0 {
		if err := json.Unmarshal(original, &members); err != nil {
			return nil, err
		}
	}
	if members == nil {
		members = make(map[string]json.RawMessage)
	}
	known, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	var edited map[string]json.RawMessage
	if err := json.Unmarshal(known, &edited); err != nil {
		return nil, err
	}
	agents, err := preserveAgentMembers(members["agents"], edited["agents"])
	if err != nil {
		return nil, err
	}
	edited["agents"] = agents
	for _, key := range []string{"schema_version", "agents", "overrides", "structured_overrides"} {
		delete(members, key)
	}
	for key, value := range edited {
		members[key] = value
	}
	return json.MarshalIndent(members, "", "  ")
}

func preserveAgentMembers(original, edited json.RawMessage) (json.RawMessage, error) {
	return preserveObject(original, edited, 0)
}

func preserveObject(original, edited json.RawMessage, depth int) (json.RawMessage, error) {
	before, after := map[string]json.RawMessage{}, map[string]json.RawMessage{}
	if len(original) > 0 {
		if err := json.Unmarshal(original, &before); err != nil {
			return nil, err
		}
	}
	if err := json.Unmarshal(edited, &after); err != nil {
		return nil, err
	}
	switch depth {
	case 0:
		for key, value := range before {
			if key != "tiers" && key != "model_aliases" {
				after[key] = value
			}
		}
		for key, value := range after {
			if key != "tiers" && key != "model_aliases" {
				continue
			}
			next := 2
			if key == "model_aliases" {
				next = 1
			}
			merged, err := preserveObject(before[key], value, next)
			if err != nil {
				return nil, err
			}
			after[key] = merged
		}
	case 1, 2:
		for key, value := range after {
			merged, err := preserveObject(before[key], value, depth+1)
			if err != nil {
				return nil, err
			}
			after[key] = merged
		}
	case 3:
		for key, value := range before {
			if key != "backend" && key != "model" && key != "effort" {
				after[key] = value
			}
		}
	}
	return json.Marshal(after)
}
