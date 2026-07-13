package spawn

import (
	"encoding/json"
	"errors"
	"os"
)

// Recipe is a single command capability's execution template. Exactly one of
// Argv / Script is expected per recipe (structural validity is enforced in a
// later task); this type only models the shape.
type Recipe struct {
	Argv   []string `json:"argv"`
	Script string   `json:"script"`
}

// Capabilities is the set of command capabilities a terminal entry declares.
// Open is a pointer so an absent `open` sub-key decodes to nil — distinguishable
// from a present-but-empty recipe. Future capabilities (introspect / place) are
// deliberately NOT fields: encoding/json drops unmodeled keys, giving
// forward-compat for free without DisallowUnknownFields.
type Capabilities struct {
	Open *Recipe `json:"open"`
}

// TerminalEntry is one terminal's config record: its command capabilities.
type TerminalEntry struct {
	Commands Capabilities `json:"commands"`
}

// TerminalsConfig maps an identity key (whatever form the user pasted — friendly
// alias / .app name / raw bundle id / *-glob) to its entry. Key matching lives
// in a later task; this type is just the decoded shape.
type TerminalsConfig map[string]TerminalEntry

// TerminalsStore is a read-only store over the user-authored terminals.json
// escape hatch. It mirrors hooks.NewStore: it just holds a path (resolution is
// the cmd layer's job) and never writes.
type TerminalsStore struct {
	path string
}

// NewTerminalsStore returns a store that reads terminals.json from path.
func NewTerminalsStore(path string) *TerminalsStore {
	return &TerminalsStore{path: path}
}

// Load reads and decodes terminals.json, following Portal's tolerant-decode
// convention. It never returns an error and never crashes the picker — every
// failure degrades to an empty (non-nil) config:
//
//   - missing file (ENOENT) → empty config, NO WARN (an unconfigured install is
//     the normal case).
//   - any other read error (unreadable / permission) → empty config + one spawn
//     WARN.
//   - malformed JSON → empty config + one spawn WARN (the whole file is ignored).
//   - a literal JSON null (nil map) → normalised to an empty non-nil config so
//     callers never nil-panic ranging over it.
//
// Load touches the filesystem for reads only — it never creates, truncates, or
// writes the file.
func (s *TerminalsStore) Load() TerminalsConfig {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return TerminalsConfig{}
		}
		detectLogger.Warn("terminals.json unreadable", "detail", err.Error())
		return TerminalsConfig{}
	}

	var cfg TerminalsConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		detectLogger.Warn("terminals.json malformed", "detail", err.Error())
		return TerminalsConfig{}
	}

	if cfg == nil {
		return TerminalsConfig{}
	}
	return cfg
}
