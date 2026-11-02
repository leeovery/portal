// Package sessionindex is sessions.json version 2: Portal's own facts about
// each session, keyed by the session's name, which is fixed for its life.
package sessionindex

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// SchemaVersion is bumped on any schema-breaking change.
const SchemaVersion = 2

// ErrLegacyIndex marks a sessions.json written by the tmux version of Portal,
// which `portal migrate` moves aside.
var ErrLegacyIndex = errors.New("sessions.json is a version 1 index")

// VersionError is a sessions.json whose version is not SchemaVersion. A
// version 1 file unwraps to ErrLegacyIndex.
type VersionError struct {
	Found int
}

func (e *VersionError) Error() string {
	return fmt.Sprintf("unsupported sessions.json version: %d (current: %d)", e.Found, SchemaVersion)
}

func (e *VersionError) Unwrap() error {
	if e.Found == 1 {
		return ErrLegacyIndex
	}
	return nil
}

// Index is the root document persisted to sessions.json.
type Index struct {
	Version  int       `json:"version"`
	SavedAt  time.Time `json:"saved_at"`
	Sessions []Entry   `json:"sessions"`

	// changes are the label and removal mutations made since the index was
	// read, logged by Update once its write lands.
	changes []change
}

// Entry is one session. Label is empty while the session shows its name.
// ScrollbackFile is relative to the state directory.
type Entry struct {
	Name           string    `json:"name"`
	Label          string    `json:"label,omitempty"`
	Dir            string    `json:"dir,omitempty"`
	CreatedAt      time.Time `json:"created_at,omitzero"`
	ScrollbackFile string    `json:"scrollback_file,omitempty"`
}

func emptyIndex() Index {
	return Index{Version: SchemaVersion, Sessions: []Entry{}}
}

// Encode serialises idx to indented JSON at SchemaVersion, with no sessions
// encoding as [] rather than null.
func Encode(idx Index) ([]byte, error) {
	idx.Version = SchemaVersion
	if idx.Sessions == nil {
		idx.Sessions = []Entry{}
	}
	return json.MarshalIndent(idx, "", "  ")
}

// Decode errors on malformed JSON, a missing version field, and a version
// other than SchemaVersion (a *VersionError); unknown fields are ignored for
// forward compat.
func Decode(data []byte) (Index, error) {
	var idx Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return Index{}, fmt.Errorf("decode sessions.json: %w", err)
	}
	if idx.Version == 0 {
		return Index{}, errors.New("sessions.json missing version field")
	}
	if idx.Version != SchemaVersion {
		return Index{}, &VersionError{Found: idx.Version}
	}
	if idx.Sessions == nil {
		idx.Sessions = []Entry{}
	}
	return idx, nil
}
