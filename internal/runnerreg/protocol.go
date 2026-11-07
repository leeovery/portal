package runnerreg

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/leeovery/portal/internal/runnertoken"
)

// Version is the control protocol's version. A runner refuses a request in
// any other, as a Portal upgraded while attached must never act on a request
// it doesn't understand.
const Version = 1

// The requests a runner takes.
const (
	reqSwitch         = "switch"
	reqDetachToPicker = "detach-to-picker"
	reqConfirmSession = "confirm-session"
	reqTake           = "take"
)

// carries is which of the optional fields each request carries: one it
// carries it needs, not empty, and one it doesn't it refuses, given empty or
// null too. A request missing from it is unknown.
var carries = map[string]carried{
	reqSwitch:         {session: true},
	reqDetachToPicker: {},
	reqConfirmSession: {session: true},
	reqTake:           {session: true, by: true},
}

// carried is which of the optional fields a request carries: the session it
// names, and by, where a session taken went.
type carried struct{ session, by bool }

// maxMessage bounds a request and an answer.
const maxMessage = 4 << 10

// timeout bounds each request: its deadline is this far off, unless the
// caller's context ends sooner, and a runner reads it within this.
var timeout = 2 * time.Second

// answerGrace is how long past a request's deadline its caller still waits
// for the answer. A runner takes a request only before its deadline and
// answers at once, so the caller hears of every request a runner took.
const answerGrace = 500 * time.Millisecond

// AskBound is the longest a request keeps its caller: its deadline, then the
// grace its answer has. A request asked of every runner on a session asks
// them all at once, so it keeps its caller as long.
func AskBound() time.Duration { return timeout + answerGrace }

// errOtherPID is an answer from a process other than the runner registered.
var errOtherPID = errors.New("answered as another process")

type request struct {
	Version  int       `json:"version"`
	Request  string    `json:"request"`
	Session  string    `json:"session,omitempty"`
	By       string    `json:"by,omitempty"`
	Deadline time.Time `json:"deadline"`
	// sessionGiven and byGiven are whether the request read carried each of
	// those fields at all, an empty or null one included.
	sessionGiven, byGiven bool
}

func (q *request) fields() fields {
	return fields{
		"version":  &q.Version,
		"request":  &q.Request,
		"session":  &given{into: &q.Session, given: &q.sessionGiven},
		"by":       &given{into: &q.By, given: &q.byGiven},
		"deadline": &q.Deadline,
	}
}

// given decodes a field into into, recording that the field was given,
// whatever its value: encoding/json decodes a null into a string as nothing
// at all.
type given struct {
	into  any
	given *bool
}

func (g *given) UnmarshalJSON(data []byte) error {
	*g.given = true
	return json.Unmarshal(data, g.into)
}

// check refuses a request the runner would not understand in full.
func (q request) check() error {
	c, known := carries[q.Request]
	switch {
	case q.Version != Version:
		return fmt.Errorf("protocol version %d, where this runner speaks %d", q.Version, Version)
	case !known:
		return fmt.Errorf("unknown request %q", q.Request)
	}
	for _, f := range []optional{
		{name: "session", carried: c.session, value: q.Session, given: q.sessionGiven},
		{name: "by", carried: c.by, value: q.By, given: q.byGiven},
	} {
		if err := f.check(q.Request); err != nil {
			return err
		}
	}
	if q.Deadline.IsZero() {
		return fmt.Errorf("%s names no deadline", q.Request)
	}
	return nil
}

// optional is one of a request's optional fields as read: whether the
// request carries it, its value, and whether it was given at all.
type optional struct {
	name           string
	carried, given bool
	value          string
}

// check refuses the field missing from request, which carries it, or given
// to request, which doesn't.
func (f optional) check(request string) error {
	switch {
	case f.carried && f.value == "":
		return fmt.Errorf("%s names no %s", request, f.name)
	case !f.carried && f.given:
		return fmt.Errorf("%s takes no %s", request, f.name)
	}
	return nil
}

type answer struct {
	Version int  `json:"version"`
	OK      bool `json:"ok"`
	// Refusal says why a request was refused.
	Refusal string `json:"refusal,omitempty"`
	// PID is the runner's own, on every answer.
	PID int `json:"pid"`
}

func (a *answer) fields() fields {
	return fields{"version": &a.Version, "ok": &a.OK, "refusal": &a.Refusal, "pid": &a.PID}
}

// judge is why the answer is no acceptance this version can read from the
// runner registered as pid: an answer from another process, one in another
// version, a refusal, or an acceptance that doesn't say who gave it.
func (a answer) judge(pid int) error {
	switch {
	case a.PID != 0 && a.PID != pid:
		return fmt.Errorf("%w: pid %d", errOtherPID, a.PID)
	case a.Version != Version:
		return fmt.Errorf("%w: it speaks protocol version %d", runnertoken.ErrRefused, a.Version)
	case !a.OK:
		return fmt.Errorf("%w: %s", runnertoken.ErrRefused, a.Refusal)
	case a.PID == 0:
		return errors.New("it accepted without naming its pid")
	}
	return nil
}

// readRequest reads the one request a connection carries, strictly: a field
// it doesn't know is a request it doesn't understand.
func readRequest(r io.Reader) (request, error) {
	var q request
	if err := readMessage(r, q.fields()); err != nil {
		return request{}, fmt.Errorf("unreadable request: %w", err)
	}
	return q, q.check()
}

// send asks the runner listening at socket, registered as pid, to carry out
// req, and returns nil once it accepts.
func send(ctx context.Context, socket string, pid int, req request) error {
	if err := checkSocketPath(socket); err != nil {
		return err
	}
	req.Version, req.Deadline = Version, deadline(ctx)
	ctx, cancel := context.WithDeadline(ctx, req.Deadline.Add(answerGrace))
	defer cancel()
	conn, err := new(net.Dialer).DialContext(ctx, "unix", socket)
	if err != nil {
		return fmt.Errorf("reach runner %d: %w", pid, err)
	}
	defer func() { _ = conn.Close() }()
	stop := context.AfterFunc(ctx, func() { _ = conn.SetDeadline(time.Unix(1, 0)) })
	defer stop()

	a, err := exchange(conn.(*net.UnixConn), req)
	if err == nil {
		err = a.judge(pid)
	}
	if err != nil {
		return fmt.Errorf("ask runner %d to %s: %w", pid, req.Request, err)
	}
	return nil
}

// deadline is when a request sent now is given up on: timeout from now, or
// sooner, so its answer comes before ctx ends.
func deadline(ctx context.Context) time.Time {
	d := time.Now().Add(timeout)
	if end, ok := ctx.Deadline(); ok && end.Add(-answerGrace).Before(d) {
		d = end.Add(-answerGrace)
	}
	return d
}

// exchange writes req and reads its answer. The request's end is the end of
// what the caller writes, so a runner can tell nothing follows it.
func exchange(conn *net.UnixConn, req request) (answer, error) {
	if err := writeMessage(conn, req); err != nil {
		return answer{}, err
	}
	if err := conn.CloseWrite(); err != nil {
		return answer{}, err
	}
	var a answer
	err := readMessage(conn, a.fields())
	return a, err
}

// writeMessage writes v as one JSON line.
func writeMessage(w io.Writer, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = w.Write(append(data, '\n'))
	return err
}

// readMessage reads one JSON line, all r holds, into fields.
func readMessage(r io.Reader, f fields) error {
	data, err := io.ReadAll(io.LimitReader(r, maxMessage+1))
	switch {
	case err != nil:
		return err
	case len(data) > maxMessage:
		return fmt.Errorf("over %d bytes", maxMessage)
	case bytes.IndexByte(data, '\n') != len(data)-1:
		return errors.New("not one line")
	}
	return decodeStrict(data, f)
}

// fields maps each key a JSON object may hold to where its value goes.
type fields map[string]any

// decodeStrict decodes data, one JSON object and nothing after it, into f.
// Unlike encoding/json's own decoding, a key matches exactly, with no
// folding of case, and a key f lacks, or one given twice, is refused.
func decodeStrict(data []byte, f fields) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return errors.New("not a JSON object")
	}
	seen := map[string]bool{}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		key, _ := tok.(string)
		into, known := f[key]
		switch {
		case !known:
			return fmt.Errorf("unknown field %q", key)
		case seen[key]:
			return fmt.Errorf("field %q given twice", key)
		}
		seen[key] = true
		if err := dec.Decode(into); err != nil {
			return fmt.Errorf("field %q: %w", key, err)
		}
	}
	if _, err := dec.Token(); err != nil {
		return err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return errors.New("something follows the object")
	}
	return nil
}
