package state

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// ErrCorruptIndex sentinels the "sessions.json exists but cannot be used"
// path: malformed JSON, unsupported schema version, an unreadable file
// (permission denied), or any other unparseable content. Bootstrap's
// orchestrator detects this via errors.Is and emits a soft user-facing
// warning (CorruptSessionsJSONWarning) without aborting.
//
// Errors returned for an absent file (clean skip) are NOT wrapped with
// this sentinel — only structural corruption and unreadable-but-present
// files are "the corrupt-index path."
var ErrCorruptIndex = errors.New("sessions.json corrupt")

// ReadIndex loads sessions.json from the given state directory and returns the
// decoded Index along with a skip flag indicating whether the bootstrap caller
// should refrain from proceeding with restoration.
//
// Return contract:
//   - (Index{}, true,  nil)            — sessions.json is absent. Treated as a
//     non-error "nothing to restore" signal; the caller continues normally.
//   - (Index{}, true,  err)            — the file exists but could not be read
//     (e.g. permission denied) or could not be parsed (malformed JSON, missing
//     or unsupported version). The caller logs the error and skips restoration.
//     Both read and parse errors are wrapped with ErrCorruptIndex so a single
//     errors.Is check at consumer sites buckets every "exists-but-unusable"
//     case as a soft warning.
//   - (idx,     false, nil)            — a valid v1 document. The caller may
//     proceed with restoration using idx.
//
// ReadIndex performs no logging or stdout/stderr writes of its own; the caller
// is responsible for surfacing any returned error.
func ReadIndex(dir string) (Index, bool, error) {
	data, err := os.ReadFile(SessionsJSON(dir))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Index{}, true, nil
		}
		return Index{}, true, fmt.Errorf("read sessions.json: %w: %w", ErrCorruptIndex, err)
	}

	idx, err := DecodeIndex(data)
	if err != nil {
		return Index{}, true, fmt.Errorf("parse sessions.json: %w: %w", ErrCorruptIndex, err)
	}

	return idx, false, nil
}

// CountPanes returns the total number of panes across every window in every
// session of idx. The zero-pane case (empty session or window) contributes 0,
// so the count is exact regardless of canonicalisation state. It is the single
// pane-counting implementation shared by `portal doctor`'s sessions.json check.
func CountPanes(idx Index) int {
	total := 0
	for _, s := range idx.Sessions {
		for _, w := range s.Windows {
			total += len(w.Panes)
		}
	}
	return total
}
