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

// zmx v0.8.1 prints one session per line, tab-separated, behind a two-column
// marker that is "→ " on the caller's own session:
//
//	name=<n>\tpid=<n>\tclients=<n>\tcreated=<unix s>[\tcwd=<osc7>][\tcmd=<c>][\tended=…][\texit_code=…][\t<label>=<v>…]
//	name=<n>\terr=<error>\tstatus=<cleaning up|unreachable>
//
// cwd and cmd are read by position, ahead of any labels: a label may be
// named cwd.
var listMarkers = []string{"  ", "→ "}

// statusGone is the status of a session whose daemon refused the probe; zmx
// deleted its socket during this listing.
const statusGone = "cleaning up"

var errUnparseable = errors.New("unparseable zmx list line")

func parseList(out string) ([]backend.Session, error) {
	sessions := []backend.Session{}
	for line := range strings.Lines(out) {
		line = strings.TrimSuffix(line, "\n")
		if line == "" {
			continue
		}
		s, live, err := parseLine(line)
		if err != nil {
			return nil, fmt.Errorf("%w %q: %w", errUnparseable, line, err)
		}
		if live {
			sessions = append(sessions, s)
		}
	}
	return sessions, nil
}

// parseLine reports false for a session that is already gone.
func parseLine(line string) (backend.Session, bool, error) {
	for _, marker := range listMarkers {
		if rest, ok := strings.CutPrefix(line, marker); ok {
			line = rest
			break
		}
	}
	fields := fieldCursor(strings.Split(line, "\t"))
	name, err := fields.required("name=")
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
