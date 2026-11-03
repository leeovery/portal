package runnerreg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"time"
)

// Version is the control protocol's version. A runner refuses a request in
// any other, as a Portal upgraded while attached must never act on a request
// it doesn't understand.
const Version = 1

// The requests a runner takes.
const (
	reqLastInput      = "last-input"
	reqSwitch         = "switch"
	reqDetachToPicker = "detach-to-picker"
)

// namesSession is whether each request names a session. A request missing
// from it is unknown.
var namesSession = map[string]bool{
	reqLastInput:      false,
	reqSwitch:         true,
	reqDetachToPicker: false,
}

// maxMessage bounds a request and an answer.
const maxMessage = 4 << 10

// timeout bounds each request, from the dial to the answer.
var timeout = 2 * time.Second

// ErrRefused is a runner's refusal of a request: one it doesn't understand,
// or can't carry out, as when its attach is already ending.
var ErrRefused = errors.New("refused")

type request struct {
	Version int    `json:"version"`
	Request string `json:"request"`
	Session string `json:"session,omitempty"`
}

// check refuses a request the runner would not understand in full.
func (q request) check() error {
	named, known := namesSession[q.Request]
	switch {
	case q.Version != Version:
		return fmt.Errorf("protocol version %d, where this runner speaks %d", q.Version, Version)
	case !known:
		return fmt.Errorf("unknown request %q", q.Request)
	case named && q.Session == "":
		return fmt.Errorf("%s names no session", q.Request)
	case !named && q.Session != "":
		return fmt.Errorf("%s takes no session", q.Request)
	}
	return nil
}

// readRequest reads one request, strictly: a field it doesn't know is a
// request it doesn't understand.
func readRequest(r io.Reader) (request, error) {
	dec := json.NewDecoder(io.LimitReader(r, maxMessage))
	dec.DisallowUnknownFields()
	var q request
	if err := dec.Decode(&q); err != nil {
		return request{}, fmt.Errorf("unreadable request: %w", err)
	}
	return q, q.check()
}

type answer struct {
	Version int  `json:"version"`
	OK      bool `json:"ok"`
	// Refusal says why a request was refused.
	Refusal string `json:"refusal,omitempty"`
	// PID is the runner's own, on every request it accepts.
	PID       int       `json:"pid,omitempty"`
	LastInput time.Time `json:"last_input,omitzero"`
}

func refusal(err error) answer {
	return answer{Version: Version, Refusal: err.Error()}
}

// refused is why the answer is no acceptance this version can read: a
// refusal, or an answer in another version.
func (a answer) refused() error {
	switch {
	case a.Version != Version:
		return fmt.Errorf("%w: it speaks protocol version %d", ErrRefused, a.Version)
	case !a.OK:
		return fmt.Errorf("%w: %s", ErrRefused, a.Refusal)
	}
	return nil
}

// ask sends the runner req and returns its answer, which must accept and come
// from the runner's own pid.
func (rn Runner) ask(ctx context.Context, req request) (answer, error) {
	if err := checkSocketPath(rn.socket); err != nil {
		return answer{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	conn, err := new(net.Dialer).DialContext(ctx, "unix", rn.socket)
	if err != nil {
		return answer{}, fmt.Errorf("reach runner %d: %w", rn.PID, err)
	}
	defer func() { _ = conn.Close() }()
	stop := context.AfterFunc(ctx, func() { _ = conn.SetDeadline(time.Now()) })
	defer stop()

	req.Version = Version
	a, err := exchange(conn, req)
	if err == nil {
		err = a.refused()
	}
	if err == nil && a.PID != rn.PID {
		err = fmt.Errorf("its socket answered as pid %d", a.PID)
	}
	if err != nil {
		return answer{}, fmt.Errorf("ask runner %d to %s: %w", rn.PID, req.Request, err)
	}
	return a, nil
}

// exchange writes req and reads its answer.
func exchange(conn net.Conn, req request) (answer, error) {
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return answer{}, err
	}
	var a answer
	err := json.NewDecoder(io.LimitReader(conn, maxMessage)).Decode(&a)
	return a, err
}
