package zmx

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unicode"

	"github.com/leeovery/portal/internal/backend"
)

// maxSocketPath is the longest unix socket path the platform takes, the
// sockaddr_un path field less its NUL, as zmx computes it: 103 on macOS.
var maxSocketPath = len(syscall.RawSockaddrUnix{}.Path) - 1

// logDir is the one entry zmx keeps beside the sockets in its directory, when
// ZMX_DIR is set, as the Backend always sets it.
const logDir = "logs"

// nameRules are zmx's own refusals plus the names its argv parsing, its
// directory or its own listing cannot carry.
var nameRules = []struct {
	refuses func(string) bool
	reason  string
}{
	{func(n string) bool { return n == "" }, "is empty"},
	{func(n string) bool { return n == "." || n == ".." }, "names a directory entry"},
	{func(n string) bool { return strings.EqualFold(n, logDir) }, "is zmx's log directory"},
	{func(n string) bool { return strings.ContainsRune(n, '/') }, "contains '/'"},
	{func(n string) bool { return strings.HasPrefix(n, "-") }, "starts with '-', which zmx reads as a flag"},
	{func(n string) bool { return strings.HasSuffix(n, "*") }, "ends with '*', which zmx kill reads as a prefix match"},
	{func(n string) bool { return strings.ContainsFunc(n, unicode.IsControl) }, "contains a control character, which zmx's own listing prints raw"},
}

// CheckName refuses a name by every rule zmx sets but its length, which
// depends on the directory; Backend.ValidateName adds that.
func CheckName(name string) error {
	for _, rule := range nameRules {
		if rule.refuses(name) {
			return fmt.Errorf("%w: %q %s", backend.ErrInvalidName, name, rule.reason)
		}
	}
	return nil
}

// ValidateName refuses a name zmx cannot hold. A session's socket is
// <dir>/<name>, so the room for a name shrinks as the directory grows.
func (b *Backend) ValidateName(name string) error {
	if err := CheckName(name); err != nil {
		return err
	}
	if limit := b.MaxNameLen(); len(name) > limit {
		return fmt.Errorf("%w: %q is %d bytes, more than the %d zmx can hold in %s",
			backend.ErrInvalidName, name, len(name), max(limit, 0), b.dir)
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
