package state

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
)

// Flag is a file in the state directory whose presence is the fact it
// records, its value the file's name. Its vocabulary, like SessionFlag's, is
// closed at the constants declared with it.
type Flag string

// Restoring is present while a bootstrap's restore creates sessions, and
// only then: the bootstrapped latch, not this flag, records a bootstrap
// unfinished.
const Restoring Flag = "restoring"

// SessionFlag is a directory of flags in the state directory, one empty file
// per session, named by the session's file stem.
type SessionFlag string

const (
	// Hydrating is written before restore creates a session, and removed once
	// the session's scrollback replay finishes.
	Hydrating SessionFlag = "hydrating"
	// ResumePending is present while a session's resume panel waits.
	ResumePending SessionFlag = "resume-pending"
)

func (f Flag) Path(dir string) string { return filepath.Join(dir, string(f)) }

// Set raises the flag, making the state directory first. Raising a raised
// flag changes nothing.
func (f Flag) Set(dir string) error { return setFlagFile(dir, f.Path(dir)) }

// Clear lowers the flag, reporting whether it was raised.
func (f Flag) Clear(dir string) (bool, error) { return removeIfPresent(f.Path(dir)) }

// Exists reports whether the flag is raised: whether a regular file stands at
// its path. A flag that can't be looked for is an error, never a lowered flag.
func (f Flag) Exists(dir string) (bool, error) { return flagFileExists(f.Path(dir)) }

// Path is the session's flag file in dir. A name that is not a single path
// element is refused with ErrUnsafeName.
func (f SessionFlag) Path(dir, name string) (string, error) {
	if err := checkFileName(name); err != nil {
		return "", err
	}
	return f.path(dir, name), nil
}

func (f SessionFlag) dir(dir string) string { return filepath.Join(dir, string(f)) }

func (f SessionFlag) path(dir, name string) string {
	return filepath.Join(f.dir(dir), fileStem(name))
}

// Set raises the session's flag, making its directory first. Raising a raised
// flag changes nothing.
func (f SessionFlag) Set(dir, name string) error {
	path, err := f.Path(dir, name)
	if err != nil {
		return err
	}
	return setFlagFile(f.dir(dir), path)
}

// Clear lowers the session's flag, reporting whether it was raised.
func (f SessionFlag) Clear(dir, name string) (bool, error) {
	path, err := f.Path(dir, name)
	if err != nil {
		return false, err
	}
	return removeIfPresent(path)
}

// Exists reports whether the session's flag is raised: whether a regular file
// stands at its path. A flag that can't be looked for is an error, never a
// lowered flag.
func (f SessionFlag) Exists(dir, name string) (bool, error) {
	path, err := f.Path(dir, name)
	if err != nil {
		return false, err
	}
	return flagFileExists(path)
}

// Raised reports whether the session's flag is raised, a flag that can't be
// looked for counting as raised, so what it guards stays guarded on a doubt.
func (f SessionFlag) Raised(dir, name string) bool {
	up, err := f.Exists(dir, name)
	return up || err != nil
}

// List names the sessions whose flag is raised, in name order. A directory
// not made yet raises none; anything in it but a regular file named by a
// session's stem, a .DS_Store or a directory, is no flag.
func (f SessionFlag) List(dir string) ([]string, error) {
	entries, err := os.ReadDir(f.dir(dir))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list %s flags: %w", string(f), err)
	}
	var names []string
	for _, e := range entries {
		if name, ok := nameOfStem(e.Name()); ok && e.Type().IsRegular() {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names, nil
}

// setFlagFile makes parent and creates the empty file path. Creation is
// atomic, and presence is all a flag says, so it needs no rename.
func setFlagFile(parent, path string) error {
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return fmt.Errorf("set flag: %w", err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil {
		return fmt.Errorf("set flag: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("set flag: %w", err)
	}
	return nil
}

// removeIfPresent removes the file at path, reporting whether there was one.
func removeIfPresent(path string) (bool, error) {
	err := os.Remove(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

// flagFileExists counts only a regular file as a raised flag.
func flagFileExists(path string) (bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return info.Mode().IsRegular(), nil
}
