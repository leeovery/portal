package runnerreg_test

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/runnerreg"
)

// reply is an answer as it comes over the socket.
type reply struct {
	Version int    `json:"version"`
	OK      bool   `json:"ok"`
	Refusal string `json:"refusal"`
	PID     int    `json:"pid"`
}

// call writes raw to the socket as a request, ending what it writes, and
// reads the answer.
func call(t *testing.T, socket, raw string) reply {
	t.Helper()
	conn, err := net.Dial("unix", socket)
	if err != nil {
		t.Fatalf("dial %s: %v", socket, err)
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn.Write([]byte(raw)); err != nil {
		t.Fatalf("write %q: %v", raw, err)
	}
	if err := conn.(*net.UnixConn).CloseWrite(); err != nil {
		t.Fatalf("end the request: %v", err)
	}
	answer, err := io.ReadAll(conn)
	if err != nil {
		t.Fatalf("read the answer to %q: %v (read %q)", raw, err, answer)
	}
	var r reply
	if err := json.Unmarshal(answer, &r); err != nil {
		t.Fatalf("decode the answer %q: %v", answer, err)
	}
	return r
}

// due is a deadline field still ahead.
func due() string {
	return `"deadline":"` + time.Now().Add(time.Minute).Format(time.RFC3339Nano) + `"`
}

func TestProtocol_Requests(t *testing.T) {
	t.Run("switch hands the handler the session named, by the caller's deadline, and answers with its pid", func(t *testing.T) {
		h := &handler{}
		g := register(t, runnerreg.New(stateDir(t)), "tok123", "api-x7Kd9a", h)
		deadline := time.Now().Add(time.Minute).Round(time.Millisecond)

		r := call(t, g.Socket(), `{"version":1,"request":"switch","session":"blog-c3d4","deadline":"`+deadline.Format(time.RFC3339Nano)+`"}`+"\n")

		switches, detaches := h.asked()
		if !r.OK || r.Version != runnerreg.Version || r.PID != os.Getpid() || !slices.Equal(switches, []string{"blog-c3d4"}) || detaches != 0 {
			t.Errorf("answer = %+v, switches %q, detaches %d; want one switch to blog-c3d4 accepted by pid %d", r, switches, detaches, os.Getpid())
		}
		if len(h.deadlines) != 1 || !h.deadlines[0].Equal(deadline) {
			t.Errorf("the handler's deadlines = %v, want the caller's, %v", h.deadlines, deadline)
		}
	})

	t.Run("detach-to-picker asks the handler to detach", func(t *testing.T) {
		h := &handler{}
		g := register(t, runnerreg.New(stateDir(t)), "tok123", "api-x7Kd9a", h)

		r := call(t, g.Socket(), `{"version":1,"request":"detach-to-picker",`+due()+`}`+"\n")

		switches, detaches := h.asked()
		if !r.OK || len(switches) != 0 || detaches != 1 {
			t.Errorf("answer = %+v, switches %q, detaches %d; want one detach accepted", r, switches, detaches)
		}
	})

	t.Run("a handler that won't is a refusal saying why, with the runner's pid", func(t *testing.T) {
		h := &handler{refuse: errors.New("the runner is not attached")}
		g := register(t, runnerreg.New(stateDir(t)), "tok123", "api-x7Kd9a", h)

		r := call(t, g.Socket(), `{"version":1,"request":"switch","session":"blog-c3d4",`+due()+`}`+"\n")

		if r.OK || r.Version != runnerreg.Version || r.Refusal != "the runner is not attached" || r.PID != os.Getpid() {
			t.Errorf("answer = %+v; want a version 1 refusal from pid %d saying the runner is not attached", r, os.Getpid())
		}
	})

	t.Run("a request past its deadline is refused, the handler never asked", func(t *testing.T) {
		h := &handler{}
		g := register(t, runnerreg.New(stateDir(t)), "tok123", "api-x7Kd9a", h)
		past := time.Now().Add(-time.Millisecond).Format(time.RFC3339Nano)

		r := call(t, g.Socket(), `{"version":1,"request":"switch","session":"blog-c3d4","deadline":"`+past+`"}`+"\n")

		if r.OK || !strings.Contains(r.Refusal, "deadline") {
			t.Errorf("answer = %+v; want a refusal past the deadline", r)
		}
		if switches, _ := h.asked(); len(switches) != 0 {
			t.Errorf("the handler was asked to switch to %q", switches)
		}
	})
}

func TestProtocol_RefusesWhatItDoesNotUnderstand(t *testing.T) {
	d := due()
	for _, tc := range []struct {
		name, raw, says string
	}{
		{"a newer version", `{"version":2,"request":"switch","session":"blog-c3d4",` + d + "}\n", "protocol version 2"},
		{"no version", `{"request":"switch","session":"blog-c3d4",` + d + "}\n", "protocol version 0"},
		{"an unknown request", `{"version":1,"request":"rename","session":"blog-c3d4",` + d + "}\n", `unknown request "rename"`},
		{"a field it doesn't know", `{"version":1,"request":"switch","session":"blog-c3d4","window":2,` + d + "}\n", `unknown field "window"`},
		{"a key in another case", `{"version":1,"request":"switch","Session":"blog-c3d4",` + d + "}\n", `unknown field "Session"`},
		{"a key given twice", `{"version":1,"request":"switch","session":"blog-c3d4","session":"docs-e5f6",` + d + "}\n", `field "session" given twice`},
		{"a switch naming no session", `{"version":1,"request":"switch",` + d + "}\n", "switch names no session"},
		{"a detach-to-picker naming a session", `{"version":1,"request":"detach-to-picker","session":"blog-c3d4",` + d + "}\n", "detach-to-picker takes no session"},
		{"no deadline", `{"version":1,"request":"switch","session":"blog-c3d4"}` + "\n", "names no deadline"},
		{"a deadline that is no time", `{"version":1,"request":"switch","session":"blog-c3d4","deadline":5}` + "\n", `field "deadline"`},
		{"anything after the line", `{"version":1,"request":"switch","session":"blog-c3d4",` + d + "}\n{}\n", "not one line"},
		{"anything after the object", `{"version":1,"request":"switch","session":"blog-c3d4",` + d + "} {}\n", "follows the object"},
		{"no end of line", `{"version":1,"request":"switch","session":"blog-c3d4",` + d + "}", "not one line"},
		{"too long a line", `{"version":1,"request":"switch","session":"` + strings.Repeat("s", 5000) + `",` + d + "}\n", "over"},
		{"no JSON", "switch blog-c3d4\n", "unreadable request"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := &handler{}
			g := register(t, runnerreg.New(stateDir(t)), "tok123", "api-x7Kd9a", h)

			r := call(t, g.Socket(), tc.raw)

			if r.OK || r.Version != runnerreg.Version || !strings.Contains(r.Refusal, tc.says) {
				t.Errorf("answer = %+v; want a version 1 refusal saying %q", r, tc.says)
			}
			if switches, detaches := h.asked(); len(switches) != 0 || detaches != 0 {
				t.Errorf("the handler was asked to switch %q, detach %d times; want nothing", switches, detaches)
			}
		})
	}
}

func TestProtocol_Asking(t *testing.T) {
	t.Run("a request carries a deadline the bound from now, ends with its line, and keys its fields exactly", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		writeEntry(t, r, "tok123", entryFor(os.Getpid(), "api-x7Kd9a"))
		requests := make(chan string, 1)
		answeringSocket(t, r.SocketPath("tok123"), ourAnswer(`"ok":true`), requests)

		before := time.Now()
		if err := r.Switch(t.Context(), "tok123", "api-x7Kd9a", "blog-c3d4"); err != nil {
			t.Fatalf("Switch = %v", err)
		}

		raw := <-requests
		if !strings.HasSuffix(raw, "}\n") || strings.Count(raw, "\n") != 1 {
			t.Errorf("the request was %q, want one line, ended", raw)
		}
		var got map[string]any
		if err := json.Unmarshal([]byte(raw), &got); err != nil {
			t.Fatalf("decode %q: %v", raw, err)
		}
		keys := slices.Sorted(func(yield func(string) bool) {
			for k := range got {
				if !yield(k) {
					return
				}
			}
		})
		if !slices.Equal(keys, []string{"deadline", "request", "session", "version"}) || got["request"] != "switch" || got["session"] != "blog-c3d4" {
			t.Errorf("the request was %q, want a switch to blog-c3d4 with version and deadline", raw)
		}
		deadline, err := time.Parse(time.RFC3339Nano, got["deadline"].(string))
		if err != nil || deadline.Before(before.Add(runnerreg.Timeout())) || deadline.After(time.Now().Add(runnerreg.Timeout())) {
			t.Errorf("deadline = %v, %v; want %v from the ask", deadline, err, runnerreg.Timeout())
		}
	})

	t.Run("a caller's context that ends sooner brings the deadline in, so its answer still comes in time", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		writeEntry(t, r, "tok123", entryFor(os.Getpid(), "api-x7Kd9a"))
		requests := make(chan string, 1)
		answeringSocket(t, r.SocketPath("tok123"), ourAnswer(`"ok":true`), requests)
		ctx, cancel := contextWithin(t, runnerreg.Timeout()/2+runnerreg.AnswerGrace)
		defer cancel()
		end, _ := ctx.Deadline()

		if err := r.Switch(ctx, "tok123", "api-x7Kd9a", "blog-c3d4"); err != nil {
			t.Fatalf("Switch = %v", err)
		}

		var got struct{ Deadline time.Time }
		if err := json.Unmarshal([]byte(<-requests), &got); err != nil || !got.Deadline.Equal(end.Add(-runnerreg.AnswerGrace)) {
			t.Errorf("deadline = %v, %v; want the caller's end less the grace for the answer, %v", got.Deadline, err, end.Add(-runnerreg.AnswerGrace))
		}
	})

	t.Run("an answer in another version is a refusal, though it accepts", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		writeEntry(t, r, "tok123", entryFor(os.Getpid(), "api-x7Kd9a"))
		answeringSocket(t, r.SocketPath("tok123"), strings.Replace(ourAnswer(`"ok":true`), `"version":1`, `"version":2`, 1), nil)

		if err := r.Switch(t.Context(), "tok123", "api-x7Kd9a", "blog-c3d4"); !errors.Is(err, runnerreg.ErrRefused) || !strings.Contains(err.Error(), "protocol version 2") {
			t.Errorf("Switch = %v, want ErrRefused naming protocol version 2", err)
		}
	})

	for name, answer := range map[string]string{
		"a key in another case":  strings.Replace(ourAnswer(`"ok":true`), `"ok"`, `"OK"`, 1),
		"a key given twice":      ourAnswer(`"ok":false,"ok":true`),
		"an acceptance unsigned": `{"version":1,"ok":true}`,
	} {
		t.Run("an answer with "+name+" is no acceptance", func(t *testing.T) {
			r := runnerreg.New(stateDir(t))
			writeEntry(t, r, "tok123", entryFor(os.Getpid(), "api-x7Kd9a"))
			answeringSocket(t, r.SocketPath("tok123"), answer, nil)

			if err := r.Switch(t.Context(), "tok123", "api-x7Kd9a", "blog-c3d4"); err == nil {
				t.Errorf("Switch = nil for the answer %s, want a failure", answer)
			}
		})
	}
}

func TestProtocol_LogsEachRequestServed(t *testing.T) {
	sink := logtest.Install(t)
	g := register(t, runnerreg.New(stateDir(t)), "tok123", "api-x7Kd9a", &handler{})

	call(t, g.Socket(), `{"version":1,"request":"switch","session":"blog-c3d4",`+due()+`}`+"\n")
	call(t, g.Socket(), `{"version":1,"request":"rename",`+due()+`}`+"\n")

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
