// Package legacyindex decodes the tmux version's sessions.json (schema version
// 1), which `portal migrate` reads from state-tmux/. It is decode-only: nothing
// writes this schema any more.
package legacyindex

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// SchemaVersion is the only version DecodeIndex accepts.
const SchemaVersion = 1

// Index is the root document of a version 1 sessions.json.
type Index struct {
	Version  int       `json:"version"`
	SavedAt  time.Time `json:"saved_at"`
	Sessions []Session `json:"sessions"`
}

type Session struct {
	Name        string            `json:"name"`
	Environment map[string]string `json:"environment"`
	Windows     []Window          `json:"windows"`
}

type Window struct {
	Index  int    `json:"index"`
	Name   string `json:"name"`
	Layout string `json:"layout"`
	Zoomed bool   `json:"zoomed"`
	Active bool   `json:"active"`
	Panes  []Pane `json:"panes"`
}

// Pane's ScrollbackFile is relative to the state directory. PortalPaneID is
// the pane's durable identity token, which keyed its hook in hooks.json. It
// decodes to "" for a pane that was never stamped, which is the ordinary case.
type Pane struct {
	Index          int    `json:"index"`
	CWD            string `json:"cwd"`
	Active         bool   `json:"active"`
	CurrentCommand string `json:"current_command"`
	ScrollbackFile string `json:"scrollback_file"`
	PortalPaneID   string `json:"portal_pane_id"`
}

// DecodeIndex errors on malformed JSON, a missing version field, and a version
// other than SchemaVersion; unknown fields are ignored for forward compat.
func DecodeIndex(data []byte) (Index, error) {
	var idx Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return idx, fmt.Errorf("decode sessions.json: %w", err)
	}
	if idx.Version == 0 {
		return idx, errors.New("sessions.json missing version field")
	}
	if idx.Version != SchemaVersion {
		return idx, fmt.Errorf("unsupported sessions.json version: %d (current: %d)", idx.Version, SchemaVersion)
	}
	return idx, nil
}
