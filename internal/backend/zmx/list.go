package zmx

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/leeovery/portal/internal/backend"
)

// zmx v0.8.1 prints one entry per session, tab-separated, behind a
// two-column marker that is "→ " on the caller's own session:
//
//	name=<n>\tpid=<n>\tclients=<n>\tcreated=<unix s>[\tcwd=<osc7>][\tcmd=<c>][\tended=…][\texit_code=…][\t<label>=<v>…]
//	name=<n>\terr=<error>\tstatus=<cleaning up|unreachable>
//
// cmd is shell-quoted, but a newline in it is printed raw, so an entry can run
// over several lines; a tab in it ends the field early. cwd and cmd are read
// by position, ahead of any labels. A label may be named cwd, but its value is
// limited to [A-Za-z0-9._-] and so never reads as a directory.
var listMarkers = []string{"  ", "→ "}

const nameField = "name="

// statusGone is the status of a session whose daemon refused the probe; zmx
// deleted its socket during this listing.
const statusGone = "cleaning up"

var errUnparseable = errors.New("unparseable zmx list output")

func parseList(out string) ([]backend.Session, error) {
	entries, err := splitEntries(out)
	if err != nil {
		return nil, err
	}
	sessions := []backend.Session{}
	for _, entry := range entries {
		s, live, err := parseEntry(entry)
		if err != nil {
			return nil, fmt.Errorf("%w: entry %q: %w", errUnparseable, entry, err)
		}
		if live {
			sessions = append(sessions, s)
		}
	}
	return sessions, nil
}

// splitEntries returns each entry without its marker. A line that does not
// open an entry continues the one before it.
func splitEntries(out string) ([]string, error) {
	out = strings.TrimSuffix(out, "\n")
	if out == "" {
		return nil, nil
	}
	var entries []string
	for line := range strings.SplitSeq(out, "\n") {
		if entry, ok := entryStart(line); ok {
			entries = append(entries, entry)
			continue
		}
		if len(entries) == 0 {
			return nil, fmt.Errorf("%w: %q opens no entry", errUnparseable, line)
		}
		entries[len(entries)-1] += "\n" + line
	}
	return entries, nil
}

func entryStart(line string) (string, bool) {
	for _, marker := range listMarkers {
		if entry, ok := strings.CutPrefix(line, marker); ok && strings.HasPrefix(entry, nameField) {
			return entry, true
		}
	}
	return "", false
}

// parseEntry reports false for a session that is already gone.
func parseEntry(entry string) (backend.Session, bool, error) {
	fields := fieldCursor(strings.Split(entry, "\t"))
	name, err := fields.required(nameField)
	if err != nil {
		return backend.Session{}, false, err
	}
	if _, failed := fields.optional("err="); failed {
		status, err := fields.required("status=")
		if err != nil {
			return backend.Session{}, false, err
		}
		return backend.Session{Name: name, Unreachable: true}, status != statusGone, nil
	}
	s, err := parseLive(name, fields)
	return s, true, err
}

func parseLive(name string, fields fieldCursor) (backend.Session, error) {
	pid, err := fields.requiredInt("pid=")
	if err != nil {
		return backend.Session{}, err
	}
	clients, err := fields.requiredInt("clients=")
	if err != nil {
		return backend.Session{}, err
	}
	created, err := fields.requiredInt("created=")
	if err != nil {
		return backend.Session{}, err
	}
	s := backend.Session{Name: name, PID: int(pid), Clients: int(clients)}
	if created > 0 {
		s.Created = time.Unix(created, 0)
	}
	if cwd, ok := fields.optional("cwd="); ok {
		s.Dir = osc7Path(cwd)
	}
	s.Command, _ = fields.optional("cmd=")
	return s, nil
}

// fieldCursor consumes a line's fields in order.
type fieldCursor []string

func (c *fieldCursor) optional(key string) (string, bool) {
	if len(*c) == 0 {
		return "", false
	}
	v, ok := strings.CutPrefix((*c)[0], key)
	if ok {
		*c = (*c)[1:]
	}
	return v, ok
}

func (c *fieldCursor) required(key string) (string, error) {
	v, ok := c.optional(key)
	if !ok {
		return "", fmt.Errorf("missing %s field", strings.TrimSuffix(key, "="))
	}
	return v, nil
}

func (c *fieldCursor) requiredInt(key string) (int64, error) {
	v, err := c.required(key)
	if err != nil {
		return 0, err
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s%q: %w", key, v, err)
	}
	return n, nil
}

// osc7Path reads zmx's cwd, an OSC 7 URI (file:// or kitty-shell-cwd://,
// percent-encoded) or a plain path, as a path; "" when it names none.
func osc7Path(cwd string) string {
	if strings.HasPrefix(cwd, "/") {
		return cwd
	}
	u, err := url.Parse(cwd)
	if err != nil || (u.Scheme != "file" && u.Scheme != "kitty-shell-cwd") || !strings.HasPrefix(u.Path, "/") {
		return ""
	}
	return u.Path
}
