package runnerreg_test

import (
	"bufio"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/runnerreg"
)

// reply is an answer as it comes over the socket.
type reply struct {
	Version   int       `json:"version"`
	OK        bool      `json:"ok"`
	Refusal   string    `json:"refusal"`
	PID       int       `json:"pid"`
	LastInput time.Time `json:"last_input"`
}

// send writes line to the socket as one request and reads its answer.
func send(t *testing.T, socket, line string) reply {
	t.Helper()
	conn, err := net.Dial("unix", socket)
	if err != nil {
		t.Fatalf("dial %s: %v", socket, err)
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn.Write([]byte(line + "\n")); err != nil {
		t.Fatalf("write %q: %v", line, err)
	}
	raw, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil {
		t.Fatalf("read the answer to %q: %v (read %q)", line, err, raw)
	}
	var r reply
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatalf("decode the answer %q: %v", raw, err)
	}
	return r
}

func TestProtocol_Requests(t *testing.T) {
	at := time.Date(2026, 11, 3, 9, 30, 0, 123456789, time.UTC)

	t.Run("last-input answers when the runner last forwarded input, and its pid", func(t *testing.T) {
		h := inputAt(at)
		g := register(t, runnerreg.New(stateDir(t)), "api-x7Kd9a", h)

		r := send(t, g.Socket(), `{"version":1,"request":"last-input"}`)

		if !r.OK || r.Version != runnerreg.Version || r.PID != os.Getpid() || !r.LastInput.Equal(at) {
			t.Errorf("answer = %+v; want version 1 accepting, pid %d, last input %v", r, os.Getpid(), at)
		}
	})

	t.Run("last-input from a runner that has forwarded none answers no time", func(t *testing.T) {
		g := register(t, runnerreg.New(stateDir(t)), "api-x7Kd9a", &handler{})

		r := send(t, g.Socket(), `{"version":1,"request":"last-input"}`)

		if !r.OK || !r.LastInput.IsZero() {
			t.Errorf("answer = %+v; want acceptance with no last input", r)
		}
	})

	t.Run("switch hands the handler the session named", func(t *testing.T) {
		h := &handler{}
		g := register(t, runnerreg.New(stateDir(t)), "api-x7Kd9a", h)

		r := send(t, g.Socket(), `{"version":1,"request":"switch","session":"blog-c3d4"}`)

		switches, detaches := h.asked()
		if !r.OK || r.PID != os.Getpid() || !slices.Equal(switches, []string{"blog-c3d4"}) || detaches != 0 {
			t.Errorf("answer = %+v, switches %q, detaches %d; want one switch to blog-c3d4 accepted", r, switches, detaches)
		}
	})

	t.Run("detach-to-picker asks the handler to detach", func(t *testing.T) {
		h := &handler{}
		g := register(t, runnerreg.New(stateDir(t)), "api-x7Kd9a", h)

		r := send(t, g.Socket(), `{"version":1,"request":"detach-to-picker"}`)

		switches, detaches := h.asked()
		if !r.OK || len(switches) != 0 || detaches != 1 {
			t.Errorf("answer = %+v, switches %q, detaches %d; want one detach accepted", r, switches, detaches)
		}
	})

	t.Run("a handler that won't is a refusal saying why", func(t *testing.T) {
		h := &handler{refuse: errors.New("the runner is not attached")}
		g := register(t, runnerreg.New(stateDir(t)), "api-x7Kd9a", h)

		r := send(t, g.Socket(), `{"version":1,"request":"switch","session":"blog-c3d4"}`)

		if r.OK || r.Version != runnerreg.Version || r.Refusal != "the runner is not attached" {
			t.Errorf("answer = %+v; want a version 1 refusal saying the runner is not attached", r)
		}
	})
}

func TestProtocol_RefusesWhatItDoesNotUnderstand(t *testing.T) {
	for _, tc := range []struct {
		name, line, says string
	}{
		{"a newer version", `{"version":2,"request":"switch","session":"blog-c3d4"}`, "protocol version 2"},
		{"no version", `{"request":"switch","session":"blog-c3d4"}`, "protocol version 0"},
		{"an unknown request", `{"version":1,"request":"rename","session":"blog-c3d4"}`, `unknown request "rename"`},
		{"a field it doesn't know", `{"version":1,"request":"switch","session":"blog-c3d4","window":2}`, "window"},
		{"a switch naming no session", `{"version":1,"request":"switch"}`, "switch names no session"},
		{"a last-input naming a session", `{"version":1,"request":"last-input","session":"blog-c3d4"}`, "last-input takes no session"},
		{"a detach-to-picker naming a session", `{"version":1,"request":"detach-to-picker","session":"blog-c3d4"}`, "detach-to-picker takes no session"},
		{"no JSON", `switch blog-c3d4`, "unreadable request"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := &handler{}
			g := register(t, runnerreg.New(stateDir(t)), "api-x7Kd9a", h)

			r := send(t, g.Socket(), tc.line)

			if r.OK || r.Version != runnerreg.Version || !strings.Contains(r.Refusal, tc.says) {
				t.Errorf("answer = %+v; want a version 1 refusal saying %q", r, tc.says)
			}
			if switches, detaches := h.asked(); len(switches) != 0 || detaches != 0 {
				t.Errorf("the handler was asked to switch %q, detach %d times; want nothing", switches, detaches)
			}
		})
	}
}

func TestProtocol_RunnerCalls(t *testing.T) {
	t.Run("a refusal is ErrRefused, saying why", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		register(t, r, "api-x7Kd9a", &handler{lastInput: time.Now(), refuse: errors.New("the runner is not attached")})
		rn := live(t, r)[0]

		for name, call := range map[string]func() error{
			"switch":           func() error { return rn.Switch(t.Context(), "blog-c3d4") },
			"detach-to-picker": func() error { return rn.DetachToPicker(t.Context()) },
		} {
			if err := call(); !errors.Is(err, runnerreg.ErrRefused) || !strings.Contains(err.Error(), "the runner is not attached") {
				t.Errorf("%s = %v, want ErrRefused saying the runner is not attached", name, err)
			}
		}
	})

	t.Run("an answer in another version is a refusal, though it accepts", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		g := register(t, r, "api-x7Kd9a", &handler{lastInput: time.Now()})
		rn := live(t, r)[0]
		_ = g.Close()
		answeringSocket(t, rn.Socket(), `{"version":2,"ok":true,"pid":`+strconv.Itoa(os.Getpid())+`}`)

		if err := rn.Switch(t.Context(), "blog-c3d4"); !errors.Is(err, runnerreg.ErrRefused) || !strings.Contains(err.Error(), "protocol version 2") {
			t.Errorf("Switch = %v, want ErrRefused naming protocol version 2", err)
		}
	})

	t.Run("a runner gone since it was found fails, and is no refusal", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		g := register(t, r, "api-x7Kd9a", &handler{lastInput: time.Now()})
		rn := live(t, r)[0]
		_ = g.Close()

		if err := rn.Switch(t.Context(), "blog-c3d4"); err == nil || errors.Is(err, runnerreg.ErrRefused) {
			t.Errorf("Switch = %v, want a failure to reach the runner", err)
		}
	})
}

func TestProtocol_LogsEachRequestServed(t *testing.T) {
	sink := logtest.Install(t)
	g := register(t, runnerreg.New(stateDir(t)), "api-x7Kd9a", &handler{})

	send(t, g.Socket(), `{"version":1,"request":"switch","session":"blog-c3d4"}`)
	send(t, g.Socket(), `{"version":1,"request":"rename"}`)

	recs := sink.Records().Matching("attach", "request").AtExactLevel(slog.LevelDebug)
	if len(recs) != 2 {
		t.Fatalf("request records = %d, want one per request", len(recs))
	}
	for i, want := range []struct{ detail, result string }{{"switch", "accepted"}, {"rename", "refused"}} {
		if got := recs[i].AttrString(t, "session"); got != "api-x7Kd9a" {
			t.Errorf("record %d session = %q, want the runner's api-x7Kd9a", i, got)
		}
		if got := recs[i].AttrString(t, "detail"); got != want.detail {
			t.Errorf("record %d detail = %q, want %q", i, got, want.detail)
		}
		if got := recs[i].AttrString(t, "result"); got != want.result {
			t.Errorf("record %d result = %q, want %q", i, got, want.result)
		}
	}
	if got := recs[0].AttrString(t, "target"); got != "blog-c3d4" {
		t.Errorf("switch target = %q, want blog-c3d4", got)
	}
	if !recs[1].HasAttr("error") {
		t.Error("the refusal names no error")
	}
}
