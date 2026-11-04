package state

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

// ErrUnsafeName refuses a session name that is not a single path element, and
// so can't name a file of its own in the state directory: an empty name, "."
// or "..", or one holding a "/" or NUL.
var ErrUnsafeName = errors.New("session name can't name a state file")

func checkFileName(name string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\x00") {
		return fmt.Errorf("%w: %q", ErrUnsafeName, name)
	}
	return nil
}

// SessionScrollbackFile is the session's scrollback file as its index entry
// records it: scrollback/<name>.bin, relative to the state directory and
// forward-slashed.
func SessionScrollbackFile(name string) (string, error) {
	if err := checkFileName(name); err != nil {
		return "", err
	}
	return sessionScrollbackFile(name), nil
}

// SessionScrollbackPath is the session's scrollback file in the state
// directory dir.
func SessionScrollbackPath(dir, name string) (string, error) {
	if err := checkFileName(name); err != nil {
		return "", err
	}
	return sessionScrollbackPath(dir, name), nil
}

func sessionScrollbackFile(name string) string { return path.Join(scrollbackSubdir, name+".bin") }

func sessionScrollbackPath(dir, name string) string {
	return filepath.Join(dir, filepath.FromSlash(sessionScrollbackFile(name)))
}
