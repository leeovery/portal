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

// Appearance is the TUI colour-scheme preference. It mirrors SessionListMode:
// AppearanceAuto is the iota default (detect with a dark fallback); AppearanceLight
// and AppearanceDark pin the mode and let detection be skipped (spec §2.6).
type Appearance int

const (
	// AppearanceAuto detects the terminal background (with a dark fallback) and is
	// the first-run / tolerant-decode default.
	AppearanceAuto Appearance = iota
	// AppearanceLight pins the light colour scheme and skips detection.
	AppearanceLight
	// AppearanceDark pins the dark colour scheme and skips detection.
	AppearanceDark
)

// Canonical on-disk strings for each appearance. String enum (not int) so
// prefs.json stays human-readable and stable.
const (
	appearanceAutoString  = "auto"
	appearanceLightString = "light"
	appearanceDarkString  = "dark"
)

// String returns the canonical on-disk string for the appearance. An out-of-range
// value maps to the auto default so the marshalled form is always one of the three
// canonical tokens.
func (a Appearance) String() string {
	switch a {
	case AppearanceLight:
		return appearanceLightString
	case AppearanceDark:
		return appearanceDarkString
	default:
		return appearanceAutoString
	}
}

// parseAppearance maps a canonical on-disk string to its appearance. Any
// unrecognised value collapses to AppearanceAuto (tolerant decode), mirroring
// parseMode.
func parseAppearance(s string) Appearance {
	switch s {
	case appearanceLightString:
		return AppearanceLight
	case appearanceDarkString:
		return AppearanceDark
	default:
		return AppearanceAuto
	}
}

// prefsFile is the on-disk JSON structure for prefs.json. Each preference is an
// independent field; a missing field decodes to the empty string, which the
// per-field parsers collapse to their default (tolerant decode).
type prefsFile struct {
	SessionListMode string `json:"session_list_mode"`
	Appearance      string `json:"appearance"`
}

// Store manages persistence of UI preferences to a JSON file.
type Store struct {
	path string
}

// NewStore creates a Store that reads and writes to the given file path.
func NewStore(path string) *Store {
	return &Store{path: path}
}

// readFile reads and decodes prefs.json with the tolerant policy shared by every
// loader: a missing file (the normal first-run state) and an empty or
// corrupt/unparseable file both yield a zero-valued prefsFile with no error (its
// empty-string fields collapse to each preference's default). Only a non-ErrNotExist
// read error is propagated, alongside the zero prefsFile. The bool reports whether
// the file existed and decoded cleanly — callers that read-modify-write use it to
// avoid clobbering a sibling field's value with a default when the file is present.
func (s *Store) readFile() (prefsFile, bool, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return prefsFile{}, false, nil
		}
		return prefsFile{}, false, err
	}

	var f prefsFile
	if err := json.Unmarshal(data, &f); err != nil {
		// Tolerant decode: an empty file unmarshals as a JSON error and lands
		// here, as does any corrupt/unparseable content.
		return prefsFile{}, false, nil
	}

	return f, true, nil
}

// Load reads the persisted session list mode from prefs.json.
//
// Every degenerate input collapses to ModeFlat with no hard error: a missing
// file (the normal first-run state), an empty or corrupt/unparseable file, and
// an unrecognised session_list_mode value all return (ModeFlat, nil). Only a
// non-ErrNotExist read error is propagated, alongside ModeFlat.
func (s *Store) Load() (SessionListMode, error) {
	f, _, err := s.readFile()
	if err != nil {
		return ModeFlat, err
	}
	return parseMode(f.SessionListMode), nil
}

// LoadAppearance reads the persisted appearance preference from prefs.json.
//
// It applies the exact same tolerant policy as Load (a separate loader is the
// lowest-risk option — it leaves Load's (SessionListMode, error) signature
// untouched). Every degenerate input collapses to AppearanceAuto with no hard
// error: a missing file, an empty or corrupt/unparseable file, a missing appearance
// field, and an unrecognised appearance value all return (AppearanceAuto, nil). Only
// a non-ErrNotExist read error is propagated, alongside AppearanceAuto.
func (s *Store) LoadAppearance() (Appearance, error) {
	f, _, err := s.readFile()
	if err != nil {
		return AppearanceAuto, err
	}
	return parseAppearance(f.Appearance), nil
}

// Save persists the given mode to prefs.json via AtomicWrite (temp file +
// rename). It read-modify-writes so a previously-persisted appearance is preserved
// rather than blanked. The AtomicWrite error is returned verbatim so the caller
// decides non-fatality. A non-ErrNotExist read error during the read phase is
// propagated rather than silently overwriting an unreadable-but-present file.
func (s *Store) Save(mode SessionListMode) error {
	// The readFile "file existed" bool is intentionally unused here: the fresh
	// re-read IS the read-modify-write preservation mechanism.
	f, _, err := s.readFile()
	if err != nil {
		return err
	}
	f.SessionListMode = mode.String()
	return s.write(f)
}

// SaveAppearance persists the given appearance to prefs.json via AtomicWrite. Like
// Save, it read-modify-writes so a previously-persisted session_list_mode is
// preserved rather than blanked.
func (s *Store) SaveAppearance(appearance Appearance) error {
	f, _, err := s.readFile()
	if err != nil {
		return err
	}
	f.Appearance = appearance.String()
	return s.write(f)
}

// write marshals the prefsFile and commits it via AtomicWrite (temp file + rename).
func (s *Store) write(f prefsFile) error {
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal prefs: %w", err)
	}

	return fileutil.AtomicWrite(s.path, data)
}
