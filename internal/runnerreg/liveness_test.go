package runnerreg_test

import (
	"errors"
	"log/slog"
	"os"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/runnerreg"
)

// askedBy is each call that asks a runner, by the INFO line it logs of the
// result: a switch through the runner's token, and a detach to the picker on
// its session.
var askedBy = map[string]func(t *testing.T, r runnerreg.Registry, token string){
	"switch": func(t *testing.T, r runnerreg.Registry, token string) {
		if err := r.Switch(t.Context(), token, "api-x7Kd9a", "blog-c3d4"); err == nil {
			t.Error("Switch = nil, want a failure")
		}
	},
	"detach to picker": func(t *testing.T, r runnerreg.Registry, _ string) {
		if accepted, err := r.DetachToPicker(t.Context(), "api-x7Kd9a"); err != nil || accepted != 0 {
			t.Errorf("DetachToPicker = %d, %v; want none accepted", accepted, err)
		}
	},
}

// resultOf is the result the call's INFO line logs.
func resultOf(t *testing.T, sink *logtest.Sink, call string) string {
	t.Helper()
	return sink.Records().Matching("attach", call).AtExactLevel(slog.LevelInfo).Only(t, call+" record").AttrString(t, "result")
}

func TestLiveness_RemovesARunnerOnlyWhenItsPidIsNotRunning(t *testing.T) {
	for _, tc := range []struct {
		name  string
		stage func(t *testing.T, r runnerreg.Registry)
	}{
		{"with its socket", func(t *testing.T, r runnerreg.Registry) {
			writeEntry(t, r, "gone01", entryFor(deadPID(t), "api-x7Kd9a"))
			refusingSocket(t, r.SocketPath("gone01"))
		}},
		{"without its socket", func(t *testing.T, r runnerreg.Registry) {
			writeEntry(t, r, "gone01", entryFor(deadPID(t), "api-x7Kd9a"))
		}},
	} {
		for call, ask := range askedBy {
			t.Run(call+": "+tc.name, func(t *testing.T) {
				sink := logtest.Install(t)
				r := runnerreg.New(stateDir(t))
				tc.stage(t, r)

				ask(t, r, "gone01")

				if got := resultOf(t, sink, call); got != "no runner" {
					t.Errorf("%s result = %q, want no runner", call, got)
				}
				if names := files(t, r.Dir()); len(names) != 0 {
					t.Errorf("the registry holds %q, want the entry and its socket gone", names)
				}
				rec := sink.Records().Matching("attach", "stale runner").AtExactLevel(slog.LevelDebug).Only(t, "stale runner record")
				if got := rec.AttrString(t, "reason"); got != "dead" {
					t.Errorf("stale runner reason = %q, want dead", got)
				}
				if got := rec.AttrString(t, "path"); got != r.EntryPath("gone01") {
					t.Errorf("stale runner path = %q, want the entry", got)
				}
			})
		}
	}
}

func TestLiveness_LeavesARunnerWhosePidIsRunning(t *testing.T) {
	// ours stages an entry naming this process, which is running, and returns
	// its pid.
	ours := func(t *testing.T, r runnerreg.Registry) int {
		writeEntry(t, r, "kept01", entryFor(os.Getpid(), "api-x7Kd9a"))
		return os.Getpid()
	}
	for _, tc := range []struct {
		name, reason string
		stage        func(t *testing.T, r runnerreg.Registry) int
	}{
		{"a socket that refuses connections, as a live runner's does with its backlog full", "connection refused", func(t *testing.T, r runnerreg.Registry) int {
			refusingSocket(t, r.SocketPath("kept01"))
			return ours(t, r)
		}},
		{"a missing socket, as when another process took the pid", "missing", ours},
		{"an answer from another pid", "another pid", func(t *testing.T, r runnerreg.Registry) int {
			register(t, r, "kept01", "api-x7Kd9a", &handler{})
			pid := otherPID(t)
			writeEntry(t, r, "kept01", entryFor(pid, "api-x7Kd9a"))
			return pid
		}},
		{"a refusal from another pid", "another pid", func(t *testing.T, r runnerreg.Registry) int {
			pid := otherPID(t)
			writeEntry(t, r, "kept01", entryFor(pid, "api-x7Kd9a"))
			answeringSocket(t, r.SocketPath("kept01"), `{"version":1,"ok":false,"refusal":"busy","pid":1}`, nil)
			return pid
		}},
		{"a socket that never answers in time, as a busy runner's", "timeout", func(t *testing.T, r runnerreg.Registry) int {
			runnerreg.SetTimeout(t, 100*time.Millisecond)
			silentSocket(t, r.SocketPath("kept01"))
			return ours(t, r)
		}},
		{"a refusal, as another version's", "refused", func(t *testing.T, r runnerreg.Registry) int {
			answeringSocket(t, r.SocketPath("kept01"), `{"version":2,"ok":false,"refusal":"protocol version 1, where this runner speaks 2","pid":`+strconv.Itoa(os.Getpid())+`}`, nil)
			return ours(t, r)
		}},
		{"an answer it can't read", "no answer", func(t *testing.T, r runnerreg.Registry) int {
			answeringSocket(t, r.SocketPath("kept01"), `{"Version":1,"ok":true,"pid":`+strconv.Itoa(os.Getpid())+`}`, nil)
			return ours(t, r)
		}},
		{"a hang-up with no answer", "no answer", func(t *testing.T, r runnerreg.Registry) int {
			hangingUpSocket(t, r.SocketPath("kept01"))
			return ours(t, r)
		}},
	} {
		for call, ask := range askedBy {
			t.Run(call+": "+tc.name, func(t *testing.T) {
				sink := logtest.Install(t)
				r := runnerreg.New(stateDir(t))
				if err := os.MkdirAll(r.Dir(), 0o700); err != nil {
					t.Fatal(err)
				}
				pid := tc.stage(t, r)
				staged := files(t, r.Dir())

				ask(t, r, "kept01")

				if got := resultOf(t, sink, call); got == "no runner" || got == "accepted" {
					t.Errorf("%s result = %q, want a refusal or a failure", call, got)
				}
				if got := files(t, r.Dir()); !slices.Equal(got, staged) {
					t.Errorf("the registry holds %q, want what was staged, %q", got, staged)
				}
				rec := sink.Records().Matching("attach", "runner entry left").AtExactLevel(slog.LevelDebug).Only(t, "runner entry left record")
				if got := rec.AttrString(t, "reason"); got != tc.reason {
					t.Errorf("runner entry left reason = %q, want %q", got, tc.reason)
				}
				if got := rec.IntAttr(t, "target_pid"); got != int64(pid) {
					t.Errorf("runner entry left target_pid = %d, want %d", got, pid)
				}
				if recs := sink.Records().Matching("attach", "stale runner"); len(recs) > 0 {
					t.Errorf("logged a stale runner: %+v", recs)
				}
			})
		}
	}
}

func TestLiveness_LeavesWhatItCannotRead(t *testing.T) {
	pid := strconv.Itoa(os.Getpid())
	for name, body := range map[string]string{
		"not json":              "not json",
		"no pid":                `{"session":"api-x7Kd9a"}`,
		"no session":            `{"pid":` + pid + `}`,
		"a field to add":        `{"pid":` + pid + `,"session":"api-x7Kd9a","tty":"/dev/ttys001"}`,
		"a key in another case": `{"PID":` + pid + `,"session":"api-x7Kd9a"}`,
		"a key given twice":     `{"pid":` + pid + `,"session":"blog-c3d4","session":"api-x7Kd9a"}`,
	} {
		t.Run(name, func(t *testing.T) {
			sink := logtest.Install(t)
			r := runnerreg.New(stateDir(t))
			writeEntry(t, r, "odd001", body)
			refusingSocket(t, r.SocketPath("odd001"))

			if err := r.Switch(t.Context(), "odd001", "api-x7Kd9a", "blog-c3d4"); err == nil || errors.Is(err, runnerreg.ErrNoRunner) {
				t.Errorf("Switch = %v, want a failure, and not ErrNoRunner", err)
			}
			if accepted, err := r.DetachToPicker(t.Context(), "api-x7Kd9a"); err != nil || accepted != 0 {
				t.Errorf("DetachToPicker = %d, %v; want none asked", accepted, err)
			}
			if !exists(t, r.EntryPath("odd001")) || !exists(t, r.SocketPath("odd001")) {
				t.Error("an entry the registry could not read was removed")
			}
			for _, rec := range sink.Records().Matching("attach", "runner entry left") {
				if got := rec.AttrString(t, "reason"); got != "unreadable" {
					t.Errorf("runner entry left reason = %q, want unreadable", got)
				}
			}
			if n := len(sink.Records().Matching("attach", "runner entry left")); n != 2 {
				t.Errorf("runner entry left records = %d, want one for each read", n)
			}
		})
	}
}

func TestLiveness_ReadingTheDirectorySweepsEverySession(t *testing.T) {
	sink := logtest.Install(t)
	r := runnerreg.New(stateDir(t))
	writeEntry(t, r, "gone01", entryFor(deadPID(t), "blog-c3d4"))
	refusingSocket(t, r.SocketPath("gone01"))
	writeEntry(t, r, "kept01", entryFor(otherPID(t), "blog-c3d4"))
	refusingSocket(t, r.SocketPath("kept01"))

	if _, err := r.DetachToPicker(t.Context(), "api-x7Kd9a"); err != nil {
		t.Fatalf("DetachToPicker: %v", err)
	}

	if got, want := files(t, r.Dir()), []string{"kept01.json", "kept01.sock"}; !slices.Equal(got, want) {
		t.Errorf("the registry holds %q, want only the runner whose pid is running, %q", got, want)
	}
	rec := sink.Records().Matching("attach", "stale runner").Only(t, "stale runner record")
	if got := rec.AttrString(t, "path"); got != r.EntryPath("gone01") {
		t.Errorf("stale runner path = %q, want the dead runner's entry", got)
	}
}

func TestLiveness_AnOrphanSocketThatRefusesGoesWhenTheDirectoryIsRead(t *testing.T) {
	sink := logtest.Install(t)
	r := runnerreg.New(stateDir(t))
	if err := os.MkdirAll(r.Dir(), 0o700); err != nil {
		t.Fatal(err)
	}
	refusingSocket(t, r.SocketPath("orphan"))
	silentSocket(t, r.SocketPath("listen")) // a runner yet to write its entry

	if _, err := r.DetachToPicker(t.Context(), "api-x7Kd9a"); err != nil {
		t.Fatalf("DetachToPicker: %v", err)
	}

	if exists(t, r.SocketPath("orphan")) {
		t.Error("the orphan socket that refuses survived")
	}
	if !exists(t, r.SocketPath("listen")) {
		t.Error("a socket with a listener behind it was removed")
	}
	rec := sink.Records().Matching("attach", "stale runner").Only(t, "stale runner record")
	if got := rec.AttrString(t, "path"); got != r.SocketPath("orphan") {
		t.Errorf("stale runner path = %q, want the orphan socket", got)
	}
}
