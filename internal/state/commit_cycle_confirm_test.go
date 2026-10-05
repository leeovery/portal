package state_test

import (
	"bytes"
	"errors"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/commandertest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

const confirmRead = "display-message"

var captureReads = []string{"show-options", "list-sessions", "list-panes", "show-environment"}

const ownServerPID = 4242

// exitingServer models a tmux server that begins exiting after one chosen
// read: that read is answered, and every connection after it is refused, or
// answered by successor once one has been started on the same socket. A read
// named in shutdownAnswers is answered with exit status 0 and no output, the
// answer tmux gives an in-flight read once it has begun exiting.
type exitingServer struct {
	pid      int
	sessions string
	panes    string

	exitsAfter      func(args []string) bool
	shutdownAnswers map[string]bool
	successor       *exitingServer

	exited bool
	calls  [][]string
}

func (s *exitingServer) answer(args ...string) (string, error) {
	s.calls = append(s.calls, append([]string(nil), args...))
	if s.exited {
		if s.successor != nil {
			return s.successor.answer(args...)
		}
		return "", &tmux.CommandError{Args: args, Stderr: "no server running", Err: errors.New("exit status 1")}
	}
	if s.exitsAfter != nil && s.exitsAfter(args) {
		s.exited = true
	}
	if s.shutdownAnswers[args[0]] {
		return "", nil
	}
	switch args[0] {
	case "list-sessions":
		return s.sessions, nil
	case "list-panes":
		return s.panes, nil
	case confirmRead:
		return strconv.Itoa(s.pid), nil
	}
	return "", nil
}

func (s *exitingServer) client() *tmux.Client {
	return tmux.NewClient(commandertest.FromFunc(s.answer))
}

func (s *exitingServer) callNames() []string {
	names := make([]string, 0, len(s.calls))
	for _, c := range s.calls {
		names = append(names, c[0])
	}
	return names
}

func exitsAfterRead(prefix ...string) func([]string) bool {
	return func(args []string) bool {
		return len(args) >= len(prefix) && slices.Equal(args[:len(prefix)], prefix)
	}
}

func exitsAfterConfirmation(args []string) bool { return args[0] == confirmRead }

// savedPair is a committed sessions.json naming "work" and "notes", each with
// its transcript on disk.
type savedPair struct {
	dir        string
	index      state.Index
	sessions   []byte
	scrollback map[string]string
}

func seedSavedPair(t *testing.T) savedPair {
	t.Helper()
	dir := t.TempDir()
	idx := state.Index{Version: state.SchemaVersion, Sessions: []state.Session{}}
	for _, name := range []string{"notes", "work"} {
		file := state.SanitizePaneKey(name, 0, 0) + ".bin"
		seedScrollback(t, dir, file, name+"-transcript")
		idx.Sessions = append(idx.Sessions, state.Session{
			Name:        name,
			Environment: map[string]string{},
			Windows: []state.Window{{
				Index: 0, Name: "main", Layout: "tiled", Active: true,
				Panes: []state.Pane{{Index: 0, CWD: "/tmp", Active: true, CurrentCommand: "zsh", ScrollbackFile: "scrollback/" + file}},
			}},
		})
	}
	if err := state.Commit(dir, idx, false, nil); err != nil {
		t.Fatalf("seed sessions.json: %v", err)
	}
	idx.Canonicalize()
	sessions, err := os.ReadFile(state.SessionsJSON(dir))
	if err != nil {
		t.Fatalf("read seeded sessions.json: %v", err)
	}
	return savedPair{dir: dir, index: idx, sessions: sessions, scrollback: scrollbackContents(t, dir)}
}

func (p savedPair) assertUnchanged(t *testing.T) {
	t.Helper()
	sessions, err := os.ReadFile(state.SessionsJSON(p.dir))
	if err != nil {
		t.Fatalf("read sessions.json: %v", err)
	}
	if !bytes.Equal(sessions, p.sessions) {
		t.Errorf("sessions.json rewritten:\nbefore %s\nafter  %s", p.sessions, sessions)
	}
	if got := scrollbackContents(t, p.dir); !maps.Equal(got, p.scrollback) {
		t.Errorf("scrollback = %v, want unchanged %v", got, p.scrollback)
	}
}

// runCycle runs one cycle whose own server is the one at ownServerPID.
func (p savedPair) runCycle(server *exitingServer) error {
	_, err := state.RunCommitCycle(state.CommitCycle{
		Client:    server.client(),
		OwnServer: ownServerPID,
		Dir:       p.dir,
		LoadPrev:  func() *state.Index { return &p.index },
		HashMap:   state.HashMap{},
	})
	return err
}

func panesFor(names ...string) string {
	lines := make([]string, 0, len(names))
	for _, n := range names {
		lines = append(lines, paneLine(n, 0, "main", "tiled", false, true, 0, "/tmp", true, "zsh"))
	}
	return strings.Join(lines, "\n")
}

// onlyWorkLive is a capture in which "notes" has gone: committing it drops
// that session and deletes its transcript.
func onlyWorkLive() *exitingServer {
	return &exitingServer{pid: ownServerPID, sessions: listSessionsFor("work"), panes: panesFor("work")}
}

// newServerOnTheSameSocket is a server started after the committer's own one
// exited and before its restore ran: it holds none of the user's sessions.
func newServerOnTheSameSocket() *exitingServer {
	return &exitingServer{
		pid:      ownServerPID + 1,
		sessions: listSessionsFor(tmux.PortalBootstrapName),
		panes:    panesFor(tmux.PortalBootstrapName),
	}
}

func assertConfirmedAfterCaptureReads(t *testing.T, calls []string) {
	t.Helper()
	confirmAt := slices.Index(calls, confirmRead)
	if confirmAt < 0 {
		t.Fatalf("calls = %v, want a confirmation read", calls)
	}
	for i, name := range calls {
		if slices.Contains(captureReads, name) && i > confirmAt {
			t.Errorf("calls = %v: capture read %q sent after the confirmation", calls, name)
		}
	}
}

func TestRunCommitCycleStandsDownOnARefusedConfirmation(t *testing.T) {
	saved := seedSavedPair(t)
	server := onlyWorkLive()
	server.exitsAfter = func(args []string) bool { return args[0] == "show-environment" }

	err := saved.runCycle(server)

	var cmdErr *tmux.CommandError
	if !errors.As(err, &cmdErr) || cmdErr.Args[0] != confirmRead {
		t.Fatalf("error = %v, want the refused confirmation", err)
	}
	saved.assertUnchanged(t)
}

func TestRunCommitCycleWritesNothingFromAShutdownAnswer(t *testing.T) {
	tests := []struct {
		name   string
		server *exitingServer
	}{
		{
			name: "an empty session listing",
			server: &exitingServer{
				pid:             ownServerPID,
				exitsAfter:      exitsAfterRead("list-sessions"),
				shutdownAnswers: map[string]bool{"list-sessions": true},
			},
		},
		{
			name: "an empty pane listing beside environment reads that still succeed",
			server: &exitingServer{
				pid:             ownServerPID,
				sessions:        listSessionsFor("notes", "work"),
				exitsAfter:      exitsAfterRead("show-environment", "-t", "=work:"),
				shutdownAnswers: map[string]bool{"list-panes": true},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			saved := seedSavedPair(t)

			if err := saved.runCycle(tt.server); err == nil {
				t.Fatal("cycle returned nil, want the refused confirmation")
			}

			saved.assertUnchanged(t)
		})
	}
}

func TestRunCommitCycleWritesNothingWhenTheServerExitsAfterAnyCaptureRead(t *testing.T) {
	tests := []struct {
		name       string
		exitsAfter func([]string) bool
	}{
		{"the skeleton marker read", exitsAfterRead("show-options")},
		{"the session listing", exitsAfterRead("list-sessions")},
		{"the pane listing", exitsAfterRead("list-panes")},
		{"the first session's environment read", exitsAfterRead("show-environment", "-t", "=notes:")},
		{"the last session's environment read", exitsAfterRead("show-environment", "-t", "=work:")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			saved := seedSavedPair(t)
			server := &exitingServer{
				pid:        ownServerPID,
				sessions:   listSessionsFor("notes", "work"),
				panes:      panesFor("work"),
				exitsAfter: tt.exitsAfter,
			}

			if err := saved.runCycle(server); err == nil {
				t.Fatal("cycle returned nil, want it to stand down")
			}

			saved.assertUnchanged(t)
		})
	}
}

func TestRunCommitCycleCommitsOnAConfirmationFromItsOwnServer(t *testing.T) {
	saved := seedSavedPair(t)
	server := &exitingServer{
		pid:      ownServerPID,
		sessions: listSessionsFor(tmux.PortalSaverName, tmux.PortalBootstrapName),
		panes:    panesFor(tmux.PortalSaverName, tmux.PortalBootstrapName),
	}

	if err := saved.runCycle(server); err != nil {
		t.Fatalf("cycle: %v", err)
	}

	if names := sessionNames(onDiskIndex(t, saved.dir)); len(names) != 0 {
		t.Errorf("committed sessions = %v, want none", names)
	}
	if got := scrollbackContents(t, saved.dir); len(got) != 0 {
		t.Errorf("scrollback = %v, want every transcript removed", got)
	}
	assertConfirmedAfterCaptureReads(t, server.callNames())
}

func TestRunCommitCycleCommitsWhenItsOwnServerDropsASession(t *testing.T) {
	saved := seedSavedPair(t)
	server := onlyWorkLive()

	if err := saved.runCycle(server); err != nil {
		t.Fatalf("cycle: %v", err)
	}

	names := sessionNames(onDiskIndex(t, saved.dir))
	if !slices.Equal(names, []string{"work"}) {
		t.Errorf("committed sessions = %v, want [work]", names)
	}
	want := map[string]string{state.SanitizePaneKey("work", 0, 0) + ".bin": "work-transcript"}
	if got := scrollbackContents(t, saved.dir); !maps.Equal(got, want) {
		t.Errorf("scrollback = %v, want %v", got, want)
	}
}

func TestRunCommitCycleStandsDownOnAConfirmationNamingNoServer(t *testing.T) {
	saved := seedSavedPair(t)
	server := onlyWorkLive()
	server.exitsAfter = exitsAfterConfirmation
	server.shutdownAnswers = map[string]bool{confirmRead: true}

	err := saved.runCycle(server)

	if !errors.Is(err, state.ErrNotOwnServer) {
		t.Fatalf("error = %v, want one wrapping ErrNotOwnServer", err)
	}
	saved.assertUnchanged(t)
}

func TestRunCommitCycleStandsDownWithNoOwnServer(t *testing.T) {
	saved := seedSavedPair(t)

	_, err := state.RunCommitCycle(state.CommitCycle{
		Client:   onlyWorkLive().client(),
		Dir:      saved.dir,
		LoadPrev: func() *state.Index { return &saved.index },
		HashMap:  state.HashMap{},
	})

	if !errors.Is(err, state.ErrNotOwnServer) {
		t.Fatalf("error = %v, want one wrapping ErrNotOwnServer", err)
	}
	saved.assertUnchanged(t)
}

func TestRunCommitCycleStandsDownOnANewServerOnTheSameSocket(t *testing.T) {
	t.Run("every capture read and the confirmation answered by the new server", func(t *testing.T) {
		saved := seedSavedPair(t)
		server := newServerOnTheSameSocket()

		err := saved.runCycle(server)

		if !errors.Is(err, state.ErrNotOwnServer) {
			t.Fatalf("error = %v, want one wrapping ErrNotOwnServer", err)
		}
		saved.assertUnchanged(t)
	})

	splits := []struct {
		name       string
		exitsAfter func([]string) bool
	}{
		{"the skeleton marker read", exitsAfterRead("show-options")},
		{"the session listing", exitsAfterRead("list-sessions")},
		{"the pane listing", exitsAfterRead("list-panes")},
		{"the first session's environment read", exitsAfterRead("show-environment", "-t", "=notes:")},
		{"the last capture read, leaving only the confirmation", exitsAfterRead("show-environment", "-t", "=work:")},
	}
	for _, tt := range splits {
		t.Run("its own server exits after "+tt.name+" and a new server answers the rest", func(t *testing.T) {
			saved := seedSavedPair(t)
			server := &exitingServer{
				pid:        ownServerPID,
				sessions:   listSessionsFor("notes", "work"),
				panes:      panesFor("notes", "work"),
				exitsAfter: tt.exitsAfter,
				successor:  newServerOnTheSameSocket(),
			}

			err := saved.runCycle(server)

			if !errors.Is(err, state.ErrNotOwnServer) {
				t.Fatalf("error = %v, want one wrapping ErrNotOwnServer", err)
			}
			if len(server.successor.calls) == 0 {
				t.Fatal("the new server answered nothing; the split never happened")
			}
			saved.assertUnchanged(t)
		})
	}
}

func sessionNames(idx state.Index) []string {
	names := make([]string, 0, len(idx.Sessions))
	for _, s := range idx.Sessions {
		names = append(names, s.Name)
	}
	return names
}
