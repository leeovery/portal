package state

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/leeovery/portal/internal/sessionindex"
)

// ErrUnsafeName refuses a session name that is not a single path element, and
// so can't name a file of its own in the state directory: an empty name, "."
// or "..", or one holding a "/" or NUL.
var ErrUnsafeName = errors.New("session name can't name a state file")

// ErrNoCreation refuses a scrollback file for a session whose creation is not
// known: the file is named by its creation, so another creation's never
// stands for it.
var ErrNoCreation = errors.New("session has no recorded creation to name its scrollback by")

// ErrForeignScrollback refuses a scrollback file an index entry names that is
// not one SessionScrollbackFile names for the entry's session, which only a
// hand edit makes. Nothing removes or reads it on the entry's behalf.
var ErrForeignScrollback = errors.New("scrollback file is not one Portal names for the session")

// stemHashLen is how many hex digits of the name's hash a file stem carries.
const stemHashLen = 12

func checkFileName(name string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\x00") {
		return fmt.Errorf("%w: %q", ErrUnsafeName, name)
	}
	return nil
}

// fileStem is the name of a session's files in the state directory: the
// session's name, then a hash of its exact bytes. A filesystem that folds
// case or Unicode normalisation takes Foo and foo for one file name, but their
// stems differ in the hash, so neither session's file stands for the other's.
func fileStem(name string) string {
	sum := sha256.Sum256([]byte(name))
	return name + "-" + hex.EncodeToString(sum[:])[:stemHashLen]
}

// nameOfStem is the session a file stem is fileStem of, and false for a file
// name that is no session's stem.
func nameOfStem(stem string) (string, bool) {
	i := strings.LastIndexByte(stem, '-')
	if i < 0 {
		return "", false
	}
	name := stem[:i]
	if checkFileName(name) != nil || fileStem(name) != stem {
		return "", false
	}
	return name, true
}

// SessionScrollbackFile is the scrollback file an index entry records for the
// session's creation, created, running pid: relative to the state directory,
// forward-slashed, and named by the creation, so each creation of a name
// writes a file of its own and forgetting one removes no other's. A name that
// is not a single path element is refused with ErrUnsafeName, and a zero
// creation with ErrNoCreation.
func SessionScrollbackFile(name string, created time.Time, pid int) (string, error) {
	if err := checkFileName(name); err != nil {
		return "", err
	}
	if created.IsZero() {
		return "", fmt.Errorf("%w: %q", ErrNoCreation, name)
	}
	creation := created.UTC().Format(creationLayout) + "-" + strconv.Itoa(pid)
	return path.Join(scrollbackSubdir, fileStem(name)+"-"+creation+".bin"), nil
}

const creationLayout = "20060102T150405.999999999Z"

// ScrollbackPath is the file in the state directory dir holding the
// scrollback e records. It refuses, with ErrForeignScrollback, a file
// SessionScrollbackFile names for no creation of e's session, and an entry
// recording none.
func ScrollbackPath(dir string, e sessionindex.Entry) (string, error) {
	if err := checkFileName(e.Name); err != nil {
		return "", err
	}
	base, ok := strings.CutPrefix(e.ScrollbackFile, scrollbackSubdir+"/")
	if !ok || checkFileName(base) != nil || !namesCreation(base, fileStem(e.Name)) {
		return "", fmt.Errorf("%w: %q for %q", ErrForeignScrollback, e.ScrollbackFile, e.Name)
	}
	return filepath.Join(dir, scrollbackSubdir, base), nil
}

// IsSessionScrollbackFile reports whether base, a file name in the scrollback
// directory, is one SessionScrollbackFile names for some creation of some
// session. A legacy <name>.bin is not one, nor is anything else.
func IsSessionScrollbackFile(base string) bool {
	for i := range len(base) {
		if base[i] != '-' {
			continue
		}
		if _, ok := nameOfStem(base[:i]); ok && namesCreation(base, base[:i]) {
			return true
		}
	}
	return false
}

// namesCreation reports whether base is the scrollback file name
// SessionScrollbackFile gives some creation of the session whose stem is stem.
func namesCreation(base, stem string) bool {
	rest, ok := strings.CutPrefix(base, stem+"-")
	if !ok {
		return false
	}
	rest, ok = strings.CutSuffix(rest, ".bin")
	at, pid, cut := strings.Cut(rest, "-")
	if !ok || !cut {
		return false
	}
	_, timeErr := time.Parse(creationLayout, at)
	_, pidErr := strconv.Atoi(pid)
	return timeErr == nil && pidErr == nil
}
