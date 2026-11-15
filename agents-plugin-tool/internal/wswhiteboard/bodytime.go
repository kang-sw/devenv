package wswhiteboard

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

// BodyTimeProof binds an actual net body mutation to its final batch metadata.
// It is UX provenance, not a signature: a consistent rollback or recomputed
// digest is not detectable. Old typed writers may drop it; absence is unknown.
type BodyTimeProof struct {
	Version int    `json:"v"`
	At      string `json:"at"`
	Changed int    `json:"changed"`
	Digest  string `json:"digest"`
	// Keep unknown/malformed evidence opaque on round-trip. A bad optional
	// proof must not make a board unreadable, or become trusted on reload.
	raw json.RawMessage
}

type bodyTimeFields BodyTimeProof

func (p *BodyTimeProof) UnmarshalJSON(data []byte) error {
	var fields bodyTimeFields
	var object map[string]json.RawMessage
	fieldErr := json.Unmarshal(data, &fields)
	known := fieldErr == nil && json.Unmarshal(data, &object) == nil && len(object) == 4 && fields.Version == 1
	for _, key := range []string{"v", "at", "changed", "digest"} {
		value, ok := object[key]
		known = known && ok && !bytes.Equal(bytes.TrimSpace(value), []byte("null"))
	}
	if fieldErr != nil {
		fields = bodyTimeFields{}
	}
	*p = BodyTimeProof(fields)
	if !known {
		p.raw = append(json.RawMessage(nil), data...)
	}
	return nil
}

func (p BodyTimeProof) MarshalJSON() ([]byte, error) {
	if p.raw != nil {
		return p.raw, nil
	}
	return json.Marshal(bodyTimeFields(p))
}

// bodyTimeDigest uses ordered [block, kind] pairs, never the mark object's
// encoding. Empty and nil marks have the same [] representation. Body bytes
// are exact; neither a topic name nor any board/root path is part of the proof.
func (d TopicDoc) bodyTimeDigest() string {
	pairs := make([][2]any, 0, len(d.Marks))
	for _, mark := range d.Marks {
		pairs = append(pairs, [2]any{mark.Block, mark.Kind})
	}
	data, err := json.Marshal([]any{"wb-body-time-v1", d.Changed, d.ChangedAt, d.Body, pairs})
	if err != nil {
		panic(err) // Only strings, integers, and arrays above.
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func (d *TopicDoc) mintBodyTimeProof() {
	d.BodyTimeProof = &BodyTimeProof{Version: 1, At: d.ChangedAt, Changed: d.Changed, Digest: d.bodyTimeDigest()}
}

// validatedBodyTime is the sole UI authority for body age. ChangedAt by itself
// may be a legacy first-observation time; even a present proof must bind every
// current component before that time may be shown as an actual edit.
func (d TopicDoc) validatedBodyTime() string {
	p := d.BodyTimeProof
	if p == nil || p.raw != nil || p.Version != 1 || d.Body == "" || d.Changed <= 0 || p.Changed != d.Changed || p.At == "" || p.At != d.ChangedAt {
		return ""
	}
	at, err := time.Parse(time.RFC3339, p.At)
	if err != nil {
		return ""
	}
	if _, offset := at.Zone(); offset != 0 {
		return ""
	}
	if p.Digest != d.bodyTimeDigest() {
		return ""
	}
	return p.At
}

// bodyOrigins is a batch-local logical start snapshot. Adoption moves its
// entry along with the working topic; comparing against the original key
// would mistake an adopted edit-return for a new body mutation.
type bodyOrigins map[string]TopicDoc

func (b bodyOrigins) rename(from, to string) {
	if origin, ok := b[from]; ok {
		b[to] = origin
	} else {
		delete(b, to)
	}
	delete(b, from)
}

// stampBodyTimeProofs runs only after an accepted batch's final timestamps and
// marks are set. Backfill and body-neutral updates preserve evidence verbatim.
func (s *State) stampBodyTimeProofs(origins bodyOrigins) {
	for i := range s.Topics {
		d := &s.Topics[i]
		if d.Body != "" && d.Body != origins[d.Name].Body {
			d.mintBodyTimeProof()
		}
	}
}
