package state

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
)

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
		return Index{}, true, fmt.Errorf("read sessions.json: %w", err)
	}

	idx, err := DecodeIndex(data)
	if err != nil {
		return Index{}, true, fmt.Errorf("parse sessions.json: %w", err)
	}

	return idx, false, nil
}
