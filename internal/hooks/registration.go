package hooks

import (
	"bytes"
	"encoding/json"

	"github.com/leeovery/portal/internal/resumemode"
)

// Registration is an event's stored value: the command to run and the resume
// mode it is pinned to, if any. It is stored as either a JSON string — the
// command alone — or a JSON object carrying command alongside its settings.
// Both shapes are permanently valid and neither converts to the other.
type Registration struct {
	Command string
	Resume  resumemode.Mode
	// raw is the value this registration was decoded from, re-emitted verbatim
	// so a rewrite of one entry returns every other entry carrying what it
	// carried — an attribute this reader does not model included. A
	// registration a mutation builds carries none, so it is written in the
	// shape its own content chooses.
	raw json.RawMessage
}

// UnmarshalJSON never fails: a value of a shape this reader cannot make sense
// of decodes to a registration carrying nothing, so one junk entry never fails
// the file for its neighbours. A JSON string is the command alone; a JSON
// object's command is read when it is a string and its resume when it is one
// the vocabulary admits, and anything else leaves that field unset.
func (r *Registration) UnmarshalJSON(data []byte) error {
	*r = Registration{raw: bytes.Clone(data)}

	var command string
	if err := json.Unmarshal(data, &command); err == nil {
		r.Command = command
		return nil
	}

	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return nil
	}

	r.Command = stringAttribute(object["command"])
	r.Resume, _ = resumemode.Parse(stringAttribute(object["resume"]))
	return nil
}

// MarshalJSON emits the bytes this registration was decoded from when it holds
// them, and otherwise the string form when it carries no mode and the object
// form when it does.
func (r Registration) MarshalJSON() ([]byte, error) {
	if len(r.raw) > 0 {
		return r.raw, nil
	}
	if r.Resume == resumemode.Unset {
		return json.Marshal(r.Command)
	}
	return json.Marshal(struct {
		Command string `json:"command"`
		Resume  string `json:"resume"`
	}{Command: r.Command, Resume: r.Resume.String()})
}

// sameRegistration answers whether two registrations carry the same content,
// disregarding the bytes either was decoded from.
func sameRegistration(a, b Registration) bool {
	return a.Command == b.Command && a.Resume == b.Resume
}

// stringAttribute reads an object attribute that must be a string, answering
// empty for an absent key and for a value of any other shape.
func stringAttribute(value json.RawMessage) string {
	var s string
	if err := json.Unmarshal(value, &s); err != nil {
		return ""
	}
	return s
}
