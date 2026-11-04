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

// Entry is one session. Label is empty while the session shows its name. Dir
// is where Portal minted the session; Cwd is where the saver last saw it. A
// creation is CreatedAt, with PID, the process the session runs, zero where
// none was recorded. ScrollbackFile is relative to the state directory.
type Entry struct {
	Name           string    `json:"name"`
	Label          string    `json:"label,omitempty"`
	Dir            string    `json:"dir,omitempty"`
	Cwd            string    `json:"cwd,omitempty"`
	CreatedAt      time.Time `json:"created_at,omitzero"`
	PID            int       `json:"pid,omitempty"`
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

// Decode errors on malformed JSON, a missing version field, a version other
// than SchemaVersion (a *VersionError, judged before the body, whose shape the
// other version may not share), and an entry with an empty or repeated name.
// Unknown fields are ignored.
func Decode(data []byte) (Index, error) {
	var head struct {
		Version *int `json:"version"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return Index{}, fmt.Errorf("decode sessions.json: %w", err)
	}
	if head.Version == nil {
		return Index{}, errors.New("sessions.json missing version field")
	}
	if *head.Version != SchemaVersion {
		return Index{}, &VersionError{Found: *head.Version}
	}
	var idx Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return Index{}, fmt.Errorf("decode sessions.json: %w", err)
	}
	if err := checkNames(idx.Sessions); err != nil {
		return Index{}, err
	}
	if idx.Sessions == nil {
		idx.Sessions = []Entry{}
	}
	return idx, nil
}

func checkNames(entries []Entry) error {
	seen := make(map[string]bool, len(entries))
	for i, e := range entries {
		if e.Name == "" {
			return fmt.Errorf("sessions.json entry %d has no name", i)
		}
		if seen[e.Name] {
			return fmt.Errorf("sessions.json names session %q twice", e.Name)
		}
		seen[e.Name] = true
	}
	return nil
}
