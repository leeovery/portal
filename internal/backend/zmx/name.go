package zmx

import (
	"fmt"
	"strings"
	"syscall"
	"unicode"

	"github.com/leeovery/portal/internal/backend"
)

// maxSocketPath is the longest unix socket path the platform takes, the
// sockaddr_un path field less its NUL, as zmx computes it: 103 on macOS.
var maxSocketPath = len(syscall.RawSockaddrUnix{}.Path) - 1

// logDir is the one entry zmx v0.8.1 keeps beside the sockets in its
// directory, when ZMX_DIR is set, as the Backend always sets it.
const logDir = "logs"

// nameRules are zmx v0.8.1's own refusals plus the names its argv parsing,
// directory or list output cannot carry.
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
	{func(n string) bool { return strings.ContainsFunc(n, unicode.IsControl) }, "contains a control character"},
}

// ValidateName refuses a name zmx cannot hold. A session's socket is
// <dir>/<name>, so the room for a name shrinks as the directory grows.
func (b *Backend) ValidateName(name string) error {
	for _, rule := range nameRules {
		if rule.refuses(name) {
			return fmt.Errorf("%w: %q %s", backend.ErrInvalidName, name, rule.reason)
		}
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
