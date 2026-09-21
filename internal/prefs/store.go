// Package prefs persists UI preferences to prefs.json. Deliberately a leaf —
// stdlib plus internal/fileutil and internal/resumemode only — so internal/tui
// can import it without a cycle.
package prefs

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/leeovery/portal/internal/fileutil"
	"github.com/leeovery/portal/internal/resumemode"
)

type SessionListMode int

const (
	ModeFlat SessionListMode = iota
	ModeByProject
	ModeByTag
)

// Strings rather than ints keep prefs.json human-readable.
const (
	modeFlatString      = "flat"
	modeByProjectString = "by-project"
	modeByTagString     = "by-tag"
)

// String returns the canonical on-disk string; an out-of-range mode maps to the
// flat default.
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

// Decodes any JSON value without error: a field-level error would zero the
// tolerant load's whole record.
type migrationMarker bool

func (m *migrationMarker) UnmarshalJSON(data []byte) error {
	*m = migrationMarker(bytes.Equal(bytes.TrimSpace(data), []byte("true")))
	return nil
}

func (m migrationMarker) MarshalJSON() ([]byte, error) {
	return strconv.AppendBool(nil, bool(m)), nil
}

// Decodes any JSON value without error: a field-level error would zero the
// tolerant load's whole record. A non-string value carries no mode and is not
// preserved through the next write.
type resumeModeValue string

func (v *resumeModeValue) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		*v = ""
		return nil
	}
	*v = resumeModeValue(s)
	return nil
}

type prefsFile struct {
	SessionListMode string `json:"session_list_mode,omitempty"`
	// Preserved verbatim, never parsed. Do not delete the field: an undeclared
	// key is dropped on re-encode, erasing a downgraded binary's pin.
	Appearance string `json:"appearance,omitempty"`
	Theme      string `json:"theme,omitempty"`
	ThemeLight string `json:"theme_light,omitempty"`
	ThemeDark  string `json:"theme_dark,omitempty"`
	// Must stay declared for the same re-encode reason as Appearance.
	ThemeMigrated migrationMarker `json:"theme_migrated,omitempty"`
	// Must stay declared for the same re-encode reason as Appearance.
	ResumeMode resumeModeValue `json:"resume_mode,omitempty"`
}

// ThemeKeys carries the raw theme slugs from prefs.json verbatim — validation,
// defaulting and the theme-wins tiebreak belong to the resolver.
type ThemeKeys struct {
	Theme string
	Light string
	Dark  string
}

type MigrationState struct {
	Appearance string
	Migrated   bool
}

// ThemeSlot names one half of the adaptive pair; the zero value is deliberately
// invalid so a forgotten argument cannot silently write the light slot.
type ThemeSlot int

const (
	SlotLight ThemeSlot = iota + 1
	SlotDark
)

type Store struct {
	path string
}

func NewStore(path string) *Store {
	return &Store{path: path}
}

func (s *Store) readBytes() (data []byte, present bool, err error) {
	data, err = os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, err
	}

	return data, true, nil
}

// Tolerant load-path decode. Never route a writer through it: corrupt content
// collapses to a zero record with no error, which a merge would then commit.
func (s *Store) readFile() (prefsFile, bool, error) {
	data, present, err := s.readBytes()
	if err != nil {
		return prefsFile{}, false, err
	}
	if !present {
		return prefsFile{}, false, nil
	}

	var f prefsFile
	if err := json.Unmarshal(data, &f); err != nil {
		return prefsFile{}, false, nil
	}

	return f, true, nil
}

// Strict write-path decode: malformed content aborts rather than merges. The
// Field != "" carve-out separates a wrong-typed declared field (rest of the
// record still populated) from a top-level mismatch (wholly zero record).
func (s *Store) readFileStrict() (prefsFile, bool, error) {
	data, present, err := s.readBytes()
	if err != nil {
		return prefsFile{}, false, err
	}
	if !present {
		return prefsFile{}, false, nil
	}

	var f prefsFile
	if err := json.Unmarshal(data, &f); err != nil {
		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &typeErr) && typeErr.Field != "" {
			return f, true, nil
		}
		// Named, not bare: this aborts a commit, and the raw decode error tells
		// the user nothing about which file to open.
		return prefsFile{}, false, fmt.Errorf("prefs: %s is not valid JSON: %w", s.path, err)
	}

	return f, true, nil
}

// The strict re-read happens at write time, not load: a stale snapshot would
// silently revert another instance's commit.
func (s *Store) mutate(fn func(f *prefsFile, existed bool) bool) error {
	f, existed, err := s.readFileStrict()
	if err != nil {
		return err
	}
	if !fn(&f, existed) {
		return nil
	}
	return s.write(f)
}

// Load reads the persisted session list mode; every degenerate file yields
// ModeFlat with no error, and only a non-ErrNotExist read error propagates.
func (s *Store) Load() (SessionListMode, error) {
	f, _, err := s.readFile()
	if err != nil {
		return ModeFlat, err
	}
	return parseMode(f.SessionListMode), nil
}

// LoadResumeMode reads the install-wide resume mode under the same tolerant
// policy as Load: every value it cannot recognise yields resumemode.Default,
// and a non-ErrNotExist read error propagates alongside that default rather
// than leaving the mode undecided.
func (s *Store) LoadResumeMode() (resumemode.Mode, error) {
	f, _, err := s.readFile()
	if err != nil {
		return resumemode.Default, err
	}
	if mode, ok := resumemode.Parse(string(f.ResumeMode)); ok {
		return mode, nil
	}
	return resumemode.Default, nil
}

// LoadThemeKeys reads the raw theme slugs verbatim, under the same tolerant
// policy as Load.
func (s *Store) LoadThemeKeys() (ThemeKeys, error) {
	f, _, err := s.readFile()
	if err != nil {
		return ThemeKeys{}, err
	}
	return ThemeKeys{Theme: f.Theme, Light: f.ThemeLight, Dark: f.ThemeDark}, nil
}

// LoadMigrationState reads the raw appearance value and the marker under the
// same tolerant policy as Load.
func (s *Store) LoadMigrationState() (MigrationState, error) {
	f, _, err := s.readFile()
	if err != nil {
		return MigrationState{}, err
	}
	return MigrationState{Appearance: f.Appearance, Migrated: bool(f.ThemeMigrated)}, nil
}

// LoadThemeState reads the theme keys and the migration state from one file
// read, so the two cannot describe different moments: between two separate
// reads another instance's translation can land, yielding empty keys beside a
// set marker and a launch rendering the shipped pair instead of the pin just
// written.
func (s *Store) LoadThemeState() (ThemeKeys, MigrationState, error) {
	f, _, err := s.readFile()
	if err != nil {
		return ThemeKeys{}, MigrationState{}, err
	}
	return ThemeKeys{Theme: f.Theme, Light: f.ThemeLight, Dark: f.ThemeDark},
		MigrationState{Appearance: f.Appearance, Migrated: bool(f.ThemeMigrated)},
		nil
}

// Save persists the grouping mode, leaving every other key untouched; a
// malformed prefs.json aborts the write rather than overwriting it.
func (s *Store) Save(mode SessionListMode) error {
	return s.mutate(func(f *prefsFile, _ bool) bool {
		f.SessionListMode = mode.String()
		return true
	})
}

// SaveTheme persists slug verbatim as the constant theme, clearing the adaptive
// slots in the same atomic write.
func (s *Store) SaveTheme(slug string) error {
	return s.mutate(func(f *prefsFile, _ bool) bool {
		f.Theme = slug
		f.ThemeLight = ""
		f.ThemeDark = ""
		return true
	})
}

// SaveThemeSlot writes slug into one adaptive slot and clears the constant in
// the same write, leaving the other slot untouched; an invalid slot writes
// nothing.
func (s *Store) SaveThemeSlot(slug string, slot ThemeSlot) error {
	if slot != SlotLight && slot != SlotDark {
		return fmt.Errorf("prefs: invalid theme slot %d", slot)
	}

	return s.mutate(func(f *prefsFile, _ bool) bool {
		switch slot {
		case SlotLight:
			f.ThemeLight = slug
		case SlotDark:
			f.ThemeDark = slug
		}
		f.Theme = ""
		return true
	})
}

// SaveMigrationMarker records that the appearance translation has run. It never
// creates an absent prefs.json — a fresh install has nothing to translate — and
// declining returns nil rather than an error.
func (s *Store) SaveMigrationMarker() error {
	return s.mutate(func(f *prefsFile, existed bool) bool {
		if !existed {
			return false
		}
		f.ThemeMigrated = true
		return true
	})
}

// SaveTranslation records the translated theme key and the migration marker in
// one write, not two: a failure between separate writes would persist the key
// with the marker unset, and the next launch would then record the marker alone.
// One write means either both land or neither does. An absent prefs.json is a
// silent no-op.
func (s *Store) SaveTranslation(slug string) (persisted bool, err error) {
	err = s.mutate(func(f *prefsFile, existed bool) bool {
		if !existed {
			return false
		}
		if bool(f.ThemeMigrated) {
			return false
		}

		f.ThemeMigrated = true

		if slug == "" || f.Theme != "" || f.ThemeLight != "" || f.ThemeDark != "" {
			return true
		}

		f.Theme = slug
		// Already empty by construction; kept to state the mutual exclusion.
		f.ThemeLight = ""
		f.ThemeDark = ""
		persisted = true
		return true
	})
	if err != nil {
		// A failed write persisted nothing, even if the mutator set persisted.
		return false, err
	}

	return persisted, nil
}

// Substitutable so the write can be intercepted without a filesystem;
// production never reassigns it.
var atomicWrite = fileutil.AtomicWrite

func (s *Store) write(f prefsFile) error {
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal prefs: %w", err)
	}

	return atomicWrite(s.path, data)
}
