package zmx_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backend/zmx"
)

func list(t *testing.T, b *zmx.Backend) []backend.Session {
	t.Helper()
	sessions, err := b.List(t.Context())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	return sessions
}

func TestList(t *testing.T) {
	t.Run("it lists a session with the details its daemon reports", func(t *testing.T) {
		dir := socketDir(t)
		d := serve(t, dir, "work", answering(infoFrame(t)))

		got := list(t, newBackend(t, dir))

		if len(got) != 1 {
			t.Fatalf("List = %+v, want one session", got)
		}
		s := got[0]
		if s.Name != "work" || !s.Answered || s.Clients != 0 || s.PID <= 0 || s.Created.IsZero() || s.Command != fixtureCommand {
			t.Errorf("session = %+v", s)
		}
		if s.Dir != "" {
			t.Errorf("Dir = %q for a cwd on example-host, want none", s.Dir)
		}
		if reqs := d.received(); len(reqs) != 1 || reqs[0].tag != 6 || reqs[0].version != 1 || len(reqs[0].payload) != 0 {
			t.Errorf("the daemon received %+v, want one empty Info request", reqs)
		}
	})

	t.Run("it reads a local cwd as the session's directory", func(t *testing.T) {
		dir := socketDir(t)
		serve(t, dir, "here", answering(withCwd(infoFrame(t), "file://"+hostname+"/srv/my%20app")))

		if got := list(t, newBackend(t, dir)); len(got) != 1 || got[0].Dir != "/srv/my app" {
			t.Errorf("List = %+v, want Dir /srv/my app", got)
		}
	})

	t.Run("it names no directory for a cwd zmx cut at its limit", func(t *testing.T) {
		dir := socketDir(t)
		cwd := "/" + strings.Repeat("d", 255)
		serve(t, dir, "deep", answering(withCwd(infoFrame(t), cwd)))

		if got := list(t, newBackend(t, dir)); len(got) != 1 || !got[0].Answered || got[0].Dir != "" {
			t.Errorf("List = %+v, want an answering session with no Dir", got)
		}
	})

	t.Run("it skips the session's output broadcast ahead of its answer", func(t *testing.T) {
		dir := socketDir(t)
		serve(t, dir, "busy", answering(frame(1, 1, []byte("output")), infoFrame(t)))

		if got := list(t, newBackend(t, dir)); len(got) != 1 || !got[0].Answered {
			t.Errorf("List = %+v, want the session answering", got)
		}
	})

	t.Run("it leaves out a refusing socket from before this boot, and leaves the socket in place", func(t *testing.T) {
		dir := socketDir(t)
		stale(t, dir, "crashed")
		serve(t, dir, "alive", answering(infoFrame(t)))

		got := list(t, newBackend(t, dir))

		if len(got) != 1 || got[0].Name != "alive" {
			t.Errorf("List = %+v, want only alive", got)
		}
		if _, err := os.Lstat(filepath.Join(dir, "crashed")); err != nil {
			t.Errorf("the stale socket after List: %v; only zmx removes it", err)
		}
	})

	t.Run("it keeps a refusing socket from this boot that a process may hold, with only its name, and leaves it in place", func(t *testing.T) {
		dir := socketDir(t)
		refusing(t, dir, "busy")

		if got := list(t, held(newBackend(t, dir), dir, "busy")); len(got) != 1 || got[0] != (backend.Session{Name: "busy"}) {
			t.Errorf("List = %+v, want busy with only its name: a live daemon with a full backlog refuses too", got)
		}
		if _, err := os.Lstat(filepath.Join(dir, "busy")); err != nil {
			t.Errorf("the refusing socket after List: %v", err)
		}
	})

	t.Run("it leaves out a refusing socket from this boot that no process holds, and leaves the socket in place", func(t *testing.T) {
		dir := socketDir(t)
		refusing(t, dir, "crashed")
		serve(t, dir, "alive", answering(infoFrame(t)))

		if got := list(t, newBackend(t, dir)); len(got) != 1 || got[0].Name != "alive" {
			t.Errorf("List = %+v, want only alive", got)
		}
		if _, err := os.Lstat(filepath.Join(dir, "crashed")); err != nil {
			t.Errorf("the stale socket after List: %v; only zmx removes it", err)
		}
	})

	t.Run("it lists a session that answers from a socket made before this boot", func(t *testing.T) {
		dir := socketDir(t)
		d := serve(t, dir, "work", answering(infoFrame(t)))
		backdate(t, d.path)

		if got := list(t, newBackend(t, dir)); len(got) != 1 || !got[0].Answered {
			t.Errorf("List = %+v, want work answering: only a refusing socket can be stale", got)
		}
	})

	t.Run("without the boot time it judges a refusing socket by its holders alone", func(t *testing.T) {
		dir := socketDir(t)
		stale(t, dir, "crashed")
		stale(t, dir, "busy")
		b := mustNew(t, zmx.Config{
			Dir:      dir,
			BootTime: func() (time.Time, error) { return time.Time{}, errors.New("no boot time") },
		})
		held(b, dir, "busy")

		if got := list(t, b); len(got) != 1 || got[0] != (backend.Session{Name: "busy"}) {
			t.Errorf("List = %+v, want busy with only its name, and crashed, which no process holds, left out", got)
		}
	})

	t.Run("it keeps a session that answers wrongly, with only its name", func(t *testing.T) {
		record := infoFrame(t)[8:]
		wrongVersion := append([]byte{}, record...)
		wrongVersion[545] = 2
		for _, tt := range []struct {
			name  string
			reply []byte
		}{
			{"no answer before hanging up", nil},
			{"a short record", frame(6, 1, record[:400])},
			{"a long record", frame(6, 1, append(append([]byte{}, record...), 0))},
			{"a frame cut short", infoFrame(t)[:300]},
			{"a header of another version", frame(6, 2, record)},
			{"a packed header of a tag and a length", append([]byte{6, 0x28, 2, 0, 0, 0, 0, 0}, record...)},
			{"a record of another version", frame(6, 1, wrongVersion)},
			{"a refusal of the request", frame(255, 1, nil)},
		} {
			t.Run(tt.name, func(t *testing.T) {
				dir := socketDir(t)
				serve(t, dir, "odd", hangingUp(tt.reply))

				got := list(t, newBackend(t, dir))

				if len(got) != 1 || got[0] != (backend.Session{Name: "odd"}) {
					t.Errorf("List = %+v, want odd with only its name", got)
				}
			})
		}
	})

	t.Run("it keeps a session that does not answer, with only its name", func(t *testing.T) {
		dir := socketDir(t)
		serve(t, dir, "wedged", silent)

		if got := list(t, newBackend(t, dir)); len(got) != 1 || got[0] != (backend.Session{Name: "wedged"}) {
			t.Errorf("List = %+v, want wedged with only its name", got)
		}
	})

	t.Run("it probes every session at once, so silent ones do not starve those that answer", func(t *testing.T) {
		dir := socketDir(t)
		answers := map[string]bool{}
		for i := range 15 {
			name := fmt.Sprintf("s%02d", i)
			answers[name] = i%5 == 0
			if answers[name] {
				serve(t, dir, name, answering(infoFrame(t)))
			} else {
				serve(t, dir, name, silent)
			}
		}

		got := list(t, newBackend(t, dir))

		if len(got) != len(answers) {
			t.Fatalf("List = %+v, want all %d", got, len(answers))
		}
		for _, s := range got {
			if s.Answered != answers[s.Name] {
				t.Errorf("%s answered %v, want %v", s.Name, s.Answered, answers[s.Name])
			}
		}
	})

	t.Run("it bounds the whole listing by one probe ceiling, however many sessions wait for a slot", func(t *testing.T) {
		dir := socketDir(t)
		const silentSessions = 130
		for i := range silentSessions {
			serve(t, dir, fmt.Sprintf("s%03d", i), silent)
		}

		start := time.Now()
		got := list(t, newBackend(t, dir))

		if took := time.Since(start); took > 2*ceiling {
			t.Errorf("List of %d silent sessions took %s, want about one %s ceiling", silentSessions, took, ceiling)
		}
		answered := slices.IndexFunc(got, func(s backend.Session) bool { return s.Answered })
		if len(got) != silentSessions || answered >= 0 {
			t.Errorf("List = %d sessions, answered at %d; want all %d, none answering", len(got), answered, silentSessions)
		}
	})

	t.Run("it lists in name order what zmx lists, and nothing else", func(t *testing.T) {
		dir := socketDir(t)
		for _, name := range []string{"b", "a", "c"} {
			serve(t, dir, name, answering(infoFrame(t)))
		}
		if err := os.Mkdir(filepath.Join(dir, "logs"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "plain-file"), nil, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(dir, "a"), filepath.Join(dir, "linked")); err != nil {
			t.Fatal(err)
		}
		b := newBackend(t, dir)
		tooLong := strings.Repeat("x", b.MaxNameLen()+1)
		if err := os.Symlink(filepath.Join(dir, "a"), filepath.Join(dir, tooLong)); err != nil {
			t.Fatal(err)
		}

		var names []string
		for _, s := range list(t, b) {
			names = append(names, s.Name)
		}

		if want := "a,b,c,linked"; strings.Join(names, ",") != want {
			t.Errorf("List names = %v, want %s: sockets and links to them, short enough to dial", names, want)
		}
	})

	t.Run("it lists an empty directory as no sessions", func(t *testing.T) {
		got, err := newBackend(t, socketDir(t)).List(t.Context())
		if err != nil || got == nil || len(got) != 0 {
			t.Errorf("List = %#v, %v; want an empty, non-nil list", got, err)
		}
	})

	t.Run("it lists a missing directory as no sessions: zmx makes it with the first", func(t *testing.T) {
		missing := filepath.Join(socketDir(t), "missing")

		got, err := newBackend(t, missing).List(t.Context())
		if err != nil || got == nil || len(got) != 0 {
			t.Errorf("List of a missing directory = %#v, %v; want an empty, non-nil list", got, err)
		}
	})

	t.Run("it fails when the directory cannot be read", func(t *testing.T) {
		locked := filepath.Join(socketDir(t), "locked")
		if err := os.Mkdir(locked, 0o000); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(locked, 0o700) })

		if got, err := newBackend(t, locked).List(t.Context()); err == nil {
			t.Errorf("List of an unreadable directory = %+v, want an error", got)
		}
	})

	t.Run("it returns the listing when every probe finished before the caller's context ended", func(t *testing.T) {
		dir := socketDir(t)
		info := infoFrame(t)
		var cancel atomic.Pointer[context.CancelCauseFunc]
		serve(t, dir, "work", func(d *daemon, c net.Conn) {
			answering(info)(d, c)
			(*cancel.Load())(errors.New("caller gave up once the probe was done"))
		})
		b := newBackend(t, dir)

		for range 50 {
			ctx, end := context.WithCancelCause(t.Context())
			cancel.Store(&end)

			got, err := b.List(ctx)

			if err != nil || len(got) != 1 || !got[0].Answered {
				t.Fatalf("List = %+v, %v; want work answering: its probe finished before the context ended", got, err)
			}
			<-ctx.Done()
		}
	})

	t.Run("it fails with the caller's cause when the caller's deadline comes before its ceiling", func(t *testing.T) {
		dir := socketDir(t)
		serve(t, dir, "wedged", silent)
		ctx, cancel := context.WithTimeout(t.Context(), ceiling/4)
		defer cancel()

		start := time.Now()
		got, err := newBackend(t, dir).List(ctx)

		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("List = %+v, %v; want the caller's deadline", got, err)
		}
		if took := time.Since(start); took >= ceiling {
			t.Errorf("List took %s, past the caller's %s deadline to its own %s ceiling", took, ceiling/4, ceiling)
		}
	})

	t.Run("it fails with the caller's cause once the caller's context ends", func(t *testing.T) {
		dir := socketDir(t)
		serve(t, dir, "wedged", silent)
		cause := errors.New("caller gave up")
		ctx, cancel := context.WithCancelCause(t.Context())
		cancel(cause)

		if got, err := newBackend(t, dir).List(ctx); !errors.Is(err, cause) {
			t.Errorf("List with an ended context = %+v, %v; want its cause", got, err)
		}
	})
}
