package zmx

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"unicode"
	"unicode/utf8"

	"github.com/leeovery/portal/internal/backend"
)

// maxSocketPath is the longest unix socket path the platform takes, the
// sockaddr_un path field less its NUL, as zmx computes it: 103 on macOS.
var maxSocketPath = len(syscall.RawSockaddrUnix{}.Path) - 1

// logDir is the one entry zmx keeps beside the sockets in its directory, when
// ZMX_DIR is set, as the Backend always sets it.
const logDir = "logs"

type nameRule struct {
	refuses func(string) bool
	reason  string
}

// runeRule refuses every name holding a character it refuses.
type runeRule struct {
	refuses func(rune) bool
	reason  string
}

// pathRunes are the characters no entry in the directory can carry.
var pathRunes = []runeRule{
	{func(r rune) bool { return r == '/' }, "contains '/'"},
	{func(r rune) bool { return r == 0 }, "contains NUL"},
}

// createRunes are the characters Create refuses as well, though zmx attach
// makes names holding them.
var createRunes = []runeRule{
	{unicode.IsControl, "contains a control character, which zmx's own listing prints raw"},
}

// holding is the name rule of each of rules: a name holding a character it
// refuses.
func holding(rules []runeRule) []nameRule {
	named := make([]nameRule, 0, len(rules))
	for _, rule := range rules {
		named = append(named, nameRule{func(n string) bool { return strings.ContainsFunc(n, rule.refuses) }, rule.reason})
	}
	return named
}

// RefusesRune reports whether Create refuses every name holding r, by the
// rules on single characters that CheckName applies.
func RefusesRune(r rune) bool {
	for _, rule := range slices.Concat(pathRunes, createRunes) {
		if rule.refuses(r) {
			return true
		}
	}
	return false
}

// pathRules refuse the names no entry in the directory can carry, as zmx
// refuses them.
var pathRules = slices.Concat([]nameRule{
	{func(n string) bool { return n == "" }, "is empty"},
	{func(n string) bool { return n == "." || n == ".." }, "names a directory entry"},
}, holding(pathRunes))

// attachRules refuse the names zmx attach reads as its own flags, which it
// takes only before the session name.
var attachRules = []nameRule{
	{func(n string) bool { return n == "-h" || n == "--help" }, "is zmx attach's help flag"},
	{func(n string) bool { return n == "--labels" || strings.HasPrefix(n, "--labels=") }, "is zmx attach's --labels flag"},
}

// createRules refuse as well the names Portal does not make, though zmx
// attach makes some: ones zmx's other commands misread, or its own listing
// cannot carry.
var createRules = slices.Concat([]nameRule{
	{func(n string) bool { return strings.EqualFold(n, logDir) }, "is zmx's log directory"},
	{func(n string) bool { return strings.HasPrefix(n, "-") }, "starts with '-', which zmx reads as a flag"},
	{func(n string) bool { return strings.HasSuffix(n, "*") }, "ends with '*', which zmx kill reads as a prefix match"},
}, holding(createRunes), []nameRule{
	{func(n string) bool { return !utf8.ValidString(n) }, "is not valid UTF-8"},
})

func check(name string, rules ...[]nameRule) error {
	for _, set := range rules {
		for _, rule := range set {
			if rule.refuses(name) {
				return fmt.Errorf("%w: %q %s", backend.ErrInvalidName, name, rule.reason)
			}
		}
	}
	return nil
}

// CheckName refuses a name Create refuses, by every rule but its length,
// which depends on the directory; Backend.ValidateName adds that.
func CheckName(name string) error { return check(name, pathRules, createRules) }

// CheckLookup refuses a name no session in the directory can have, but for
// its length. Every call on a session found by List applies only this, since
// zmx makes sessions under names Create refuses.
func CheckLookup(name string) error { return check(name, pathRules) }

// CheckAttach refuses a name AttachCommand cannot pass to zmx attach, but for
// its length.
func CheckAttach(name string) error { return check(name, pathRules, attachRules) }

// ValidateName refuses a name Create refuses. A session's socket is
// <dir>/<name>, so the room for a name shrinks as the directory grows.
func (b *Backend) ValidateName(name string) error {
	if err := CheckName(name); err != nil {
		return err
	}
	return b.checkLength(name)
}

// checkLookup refuses a name no session in the directory can have.
func (b *Backend) checkLookup(name string) error {
	if err := CheckLookup(name); err != nil {
		return err
	}
	return b.checkLength(name)
}

func (b *Backend) checkLength(name string) error {
	if limit := b.MaxNameLen(); len(name) > limit {
		return fmt.Errorf("%w: %q is %d bytes, more than the %d zmx can hold in %s",
			backend.ErrNameTooLong, name, len(name), max(limit, 0), b.dir)
	}
	return nil
}

// MaxNameLen is the longest name, in bytes, whose socket fits in the
// Backend's directory.
func (b *Backend) MaxNameLen() int {
	return maxSocketPath - len(b.dir) - 1
}

// socket is a session's socket as the directory holds it.
type socket struct {
	// name is the directory entry, which on a filesystem folding case or
	// Unicode normalisation can differ from the name asked for.
	name string
	info fs.FileInfo
}

// held finds the socket that answers to name in the directory, following
// symlinks as zmx does. zmx finds a session by statting <dir>/<name>, so on a
// folding filesystem foo's socket answers for FOO; the entry it really is is
// the one that is the same file. ok is false when no socket answers.
func (b *Backend) held(name string) (s socket, ok bool, err error) {
	info, err := os.Stat(b.path(name))
	if errors.Is(err, fs.ErrNotExist) {
		return socket{}, false, nil
	}
	if err != nil {
		return socket{}, false, err
	}
	if info.Mode().Type() != fs.ModeSocket {
		return socket{}, false, nil
	}
	entries, err := os.ReadDir(b.dir)
	if err != nil {
		return socket{}, false, err
	}
	for _, e := range entries {
		if e.Name() == name {
			return socket{name: name, info: info}, true, nil
		}
	}
	for _, e := range entries {
		if other, err := os.Stat(b.path(e.Name())); err == nil && os.SameFile(info, other) {
			return socket{name: e.Name(), info: info}, true, nil
		}
	}
	return socket{}, false, nil
}

func (b *Backend) path(name string) string { return filepath.Join(b.dir, name) }
