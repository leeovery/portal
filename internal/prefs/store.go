// Package prefs provides persistence for UI preferences that do not belong in a
// domain store like projects.json. It owns the last-used session list grouping
// mode, persisted to prefs.json.
//
// The package is a pure leaf — it imports only the standard library and
// internal/fileutil — so it is safe to import from internal/tui without an
// import cycle. It deliberately does NOT emit audit/breadcrumb logging:
// prefs.json is not part of the closed state-mutation audit-trail set
// (hooks/aliases/projects), so it must not import internal/log or
// internal/storelog.
package prefs

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/leeovery/portal/internal/fileutil"
)

// SessionListMode is the grouping mode for the TUI session list. It is the
// single source of truth for the three modes; the TUI reuses this type.
type SessionListMode int

const (
	// ModeFlat is the ungrouped session list and the first-run / tolerant-decode
	// default.
	ModeFlat SessionListMode = iota
	// ModeByProject groups sessions by their resolved project directory.
	ModeByProject
	// ModeByTag groups sessions by their project tags.
	ModeByTag
)

// Canonical on-disk strings for each mode. String enum (not int) so prefs.json
// stays human-readable and stable.
const (
	modeFlatString      = "flat"
	modeByProjectString = "by-project"
	modeByTagString     = "by-tag"
)

// String returns the canonical on-disk string for the mode. An out-of-range
// value maps to the flat default so the marshalled form is always one of the
// three canonical tokens.
func (m SessionListMode) String() string {
	switch m {
	case ModeByProject:
		return modeByProjectString
	case ModeByTag:
		return modeByTagString
	default:
		return modeFlatString
	}
}

// parseMode maps a canonical on-disk string to its mode. Any unrecognised value
// collapses to ModeFlat (tolerant decode).
func parseMode(s string) SessionListMode {
	switch s {
	case modeByProjectString:
		return ModeByProject
	case modeByTagString:
		return ModeByTag
	default:
		return ModeFlat
	}
}

// prefsFile is the on-disk JSON structure for prefs.json.
type prefsFile struct {
	SessionListMode string `json:"session_list_mode"`
}

// Store manages persistence of UI preferences to a JSON file.
type Store struct {
	path string
}

// NewStore creates a Store that reads and writes to the given file path.
func NewStore(path string) *Store {
	return &Store{path: path}
}

// Load reads the persisted session list mode from prefs.json.
//
// Every degenerate input collapses to ModeFlat with no hard error: a missing
// file (the normal first-run state), an empty or corrupt/unparseable file, and
// an unrecognised session_list_mode value all return (ModeFlat, nil). Only a
// non-ErrNotExist read error is propagated, alongside ModeFlat.
func (s *Store) Load() (SessionListMode, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ModeFlat, nil
		}
		return ModeFlat, err
	}

	var f prefsFile
	if err := json.Unmarshal(data, &f); err != nil {
		// Tolerant decode: an empty file unmarshals as a JSON error and lands
		// here, as does any corrupt/unparseable content.
		return ModeFlat, nil
	}

	return parseMode(f.SessionListMode), nil
}

// Save persists the given mode to prefs.json via AtomicWrite (temp file +
// rename). The AtomicWrite error is returned verbatim so the caller decides
// non-fatality.
func (s *Store) Save(mode SessionListMode) error {
	data, err := json.MarshalIndent(prefsFile{SessionListMode: mode.String()}, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal prefs: %w", err)
	}

	return fileutil.AtomicWrite(s.path, data)
}
