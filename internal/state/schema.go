package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// SchemaVersion is the current sessions.json schema version. It is bumped on
// any schema-breaking change; the loader compares this against the version
// field in a decoded payload to apply known-version logic.
const SchemaVersion = 1

// Index is the root document persisted to sessions.json. It captures the
// complete structural topology of all saved tmux sessions plus references to
// per-pane scrollback files.
type Index struct {
	Version  int       `json:"version"`
	SavedAt  time.Time `json:"saved_at"`
	Sessions []Session `json:"sessions"`
}

// Session captures a single tmux session: its name, environment, and windows.
type Session struct {
	Name        string            `json:"name"`
	Environment map[string]string `json:"environment"`
	Windows     []Window          `json:"windows"`
}

// Window captures a single tmux window: layout, zoom and active state, and
// its pane list.
type Window struct {
	Index  int    `json:"index"`
	Name   string `json:"name"`
	Layout string `json:"layout"`
	Zoomed bool   `json:"zoomed"`
	Active bool   `json:"active"`
	Panes  []Pane `json:"panes"`
}

// Pane captures a single tmux pane: its index, working directory, active
// flag, current foreground command, and the relative path to its scrollback
// file under the state directory.
type Pane struct {
	Index          int    `json:"index"`
	CWD            string `json:"cwd"`
	Active         bool   `json:"active"`
	CurrentCommand string `json:"current_command"`
	ScrollbackFile string `json:"scrollback_file"`
}

// Canonicalize normalises the index for stable on-disk encoding:
//   - Version is set to SchemaVersion.
//   - Nil Sessions becomes an empty slice (encodes as [] rather than null).
//   - Each session's nil Environment becomes an empty map (encodes as {}).
//   - Each session's nil Windows becomes an empty slice.
//   - Each window's nil Panes becomes an empty slice.
//
// Canonicalize mutates the receiver. EncodeIndex calls it on a local copy so
// callers' values are never modified by encoding.
func (idx *Index) Canonicalize() {
	idx.Version = SchemaVersion

	if idx.Sessions == nil {
		idx.Sessions = []Session{}
	}
	for i := range idx.Sessions {
		s := &idx.Sessions[i]
		if s.Environment == nil {
			s.Environment = map[string]string{}
		}
		if s.Windows == nil {
			s.Windows = []Window{}
		}
		for j := range s.Windows {
			w := &s.Windows[j]
			if w.Panes == nil {
				w.Panes = []Pane{}
			}
		}
	}
}

// EncodeIndex serialises an Index to its canonical JSON byte form: 2-space
// indent, no null slices or maps, and Version forced to SchemaVersion. The
// supplied Index is not mutated.
func EncodeIndex(idx Index) ([]byte, error) {
	local := idx
	local.Canonicalize()
	return json.MarshalIndent(local, "", "  ")
}

// DecodeIndex parses a sessions.json byte payload into an Index. It returns
// an error if the JSON is malformed, the version field is missing (zero
// after unmarshal), or the version does not match SchemaVersion.
//
// Unknown fields are silently ignored — that is the default json.Unmarshal
// behaviour and is desirable for forward compatibility with future writers.
func DecodeIndex(data []byte) (Index, error) {
	var idx Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return idx, fmt.Errorf("decode sessions.json: %w", err)
	}
	if idx.Version == 0 {
		return idx, errors.New("sessions.json missing version field")
	}
	if idx.Version != SchemaVersion {
		return idx, fmt.Errorf("unsupported sessions.json version: %d", idx.Version)
	}
	return idx, nil
}
