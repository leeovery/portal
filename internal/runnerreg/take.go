package runnerreg

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// takeFile holds the last take's record, in the registry's directory under a
// name no entry or socket has.
const takeFile = "take"

// selfPID is this process's pid, whose own runner a take never asks: a
// variable so a test's process can stand in for another runner's.
var selfPID = os.Getpid()

// taking is a take as recorded: the session taken, where it went, and when.
type taking struct {
	Session string    `json:"session"`
	By      string    `json:"by"`
	At      time.Time `json:"at"`
}

func (r Registry) takePath() string { return filepath.Join(r.dir, takeFile) }

// RecordTake records that session is being taken, now, to by: for the
// runners a take ends only through the backend's detach, which says nothing
// of where the session went. Each take's record replaces the last one's. A
// read-only registry refuses, recording nothing.
func (r Registry) RecordTake(session, by string) error {
	if r.readOnly {
		return errReadOnly
	}
	data, err := json.Marshal(taking{Session: session, By: by, At: time.Now()})
	if err == nil {
		err = r.makeDir()
	}
	if err == nil {
		err = writeFile(r.takePath(), data)
	}
	if err != nil {
		return fmt.Errorf("record the take of %s: %w", session, err)
	}
	return nil
}

// TakenBy is where session went as the last take recorded it: empty unless
// that take was of session, recorded no earlier than since. A take recorded
// before an attach began is not the one that ended it.
func (r Registry) TakenBy(session string, since time.Time) string {
	path := r.takePath()
	data, err := os.ReadFile(path)
	var tk taking
	if err == nil {
		err = decodeStrict(data, fields{"session": &tk.Session, "by": &tk.By, "at": &tk.At})
	}
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return ""
	case err != nil:
		logger.Debug("take record unreadable", "path", path, "error", err)
		return ""
	case tk.Session != session || tk.At.Before(since):
		return ""
	}
	return tk.By
}
