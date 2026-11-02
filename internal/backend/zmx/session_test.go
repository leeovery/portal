package zmx_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backend/zmx"
)

// foldsCase skips a test on a filesystem that does not fold case.
func foldsCase(t *testing.T, dir string) {
	t.Helper()
	probe := filepath.Join(dir, "case-probe")
	if err := os.WriteFile(probe, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(probe) }()
	if _, err := os.Stat(filepath.Join(dir, "CASE-PROBE")); err != nil {
		t.Skip("the filesystem does not fold case")
	}
}

// perSession is each call that acts on one named session, for the rules
// they share.
var perSession = []struct {
	name string
	call func(context.Context, *zmx.Backend, string) error
}{
	{"Kill", func(ctx context.Context, b *zmx.Backend, name string) error { return b.Kill(ctx, name) }},
	{"Detach", func(ctx context.Context, b *zmx.Backend, name string) error { return b.Detach(ctx, name) }},
	{"History", func(ctx context.Context, b *zmx.Backend, name string) error {
		_, err := b.History(ctx, name, 0)
		return err
	}},
	{"AttachCommand", func(ctx context.Context, b *zmx.Backend, name string) error {
		_, err := b.AttachCommand(ctx, name)
		return err
	}},
	{"ProtocolVersion", func(ctx context.Context, b *zmx.Backend, name string) error {
		_, err := b.ProtocolVersion(ctx, name)
		return err
	}},
}

func TestPerSessionCalls(t *testing.T) {
	for _, op := range perSession {
		t.Run(op.name+" reports a missing session as ErrNoSuchSession", func(t *testing.T) {
			if err := op.call(t.Context(), newBackend(t, socketDir(t)), "gone"); !errors.Is(err, backend.ErrNoSuchSession) {
				t.Errorf("%s(gone) = %v, want ErrNoSuchSession", op.name, err)
			}
		})

		t.Run(op.name+" reports a refusing socket from before this boot as ErrNoSuchSession", func(t *testing.T) {
			dir := socketDir(t)
			stale(t, dir, "crashed")

			if err := op.call(t.Context(), newBackend(t, dir), "crashed"); !errors.Is(err, backend.ErrNoSuchSession) {
				t.Errorf("%s(crashed) = %v, want ErrNoSuchSession", op.name, err)
			}
			if _, err := os.Lstat(filepath.Join(dir, "crashed")); err != nil {
				t.Errorf("the stale socket after %s: %v; only zmx removes it", op.name, err)
			}
		})

		t.Run(op.name+" reports a refusing socket from this boot as not answering", func(t *testing.T) {
			dir := socketDir(t)
			refusing(t, dir, "busy")

			err := op.call(t.Context(), newBackend(t, dir), "busy")

			if !errors.Is(err, backend.ErrTimeout) || errors.Is(err, backend.ErrNoSuchSession) {
				t.Errorf("%s(busy) = %v, want ErrTimeout and not ErrNoSuchSession", op.name, err)
			}
			if _, err := os.Lstat(filepath.Join(dir, "busy")); err != nil {
				t.Errorf("the refusing socket after %s: %v", op.name, err)
			}
		})

		t.Run(op.name+" never reaches a session held under the name in another case", func(t *testing.T) {
			dir := socketDir(t)
			foldsCase(t, dir)
			d := serve(t, dir, "foo", silent)

			err := op.call(t.Context(), newBackend(t, dir), "FOO")

			if !errors.Is(err, backend.ErrNoSuchSession) || !strings.Contains(err.Error(), `"foo"`) {
				t.Errorf("%s(FOO) = %v, want ErrNoSuchSession naming foo", op.name, err)
			}
			if reqs := d.received(); len(reqs) != 0 {
				t.Errorf("%s(FOO) sent foo %+v", op.name, reqs)
			}
		})

		t.Run(op.name+" refuses a name no session can have", func(t *testing.T) {
			b := newBackend(t, socketDir(t))
			for _, name := range []string{"", ".", "..", "a/b", "../escape", "a\x00b", strings.Repeat("n", b.MaxNameLen()+1)} {
				if err := op.call(t.Context(), b, name); !errors.Is(err, backend.ErrInvalidName) {
					t.Errorf("%s(%q) = %v, want ErrInvalidName", op.name, name, err)
				}
			}
		})
	}
}

// anyRequest answers every request as a zmx daemon would: Info with the fixture
// record, History with a screen, Kill by unlinking its socket and hanging up,
// DetachAll by hanging up.
func anyRequest(info []byte) func(*daemon, net.Conn) {
	return func(d *daemon, c net.Conn) {
		r, err := d.read(c)
		if err != nil {
			return
		}
		switch r.tag {
		case 6:
			_, _ = c.Write(info)
		case 8:
			_, _ = c.Write(frame(8, 1, []byte("screen")))
		case 5:
			_ = os.Remove(d.path)
			return
		default:
			return
		}
		_, _ = io.Copy(io.Discard, c)
	}
}

func TestNamesZmxHolds(t *testing.T) {
	t.Run("it reaches a session List reports under a name Create refuses, by every call", func(t *testing.T) {
		for _, name := range []string{"-x", "--vt", "x*", "a\nb", "bad\xffname"} {
			t.Run(fmt.Sprintf("%q", name), func(t *testing.T) {
				dir := socketDir(t)
				if err := zmx.CheckName(name); err == nil {
					t.Fatalf("CheckName(%q) accepts it; the case needs a name Create refuses", name)
				}
				l, err := net.Listen("unix", filepath.Join(dir, name))
				if errors.Is(err, syscall.EILSEQ) {
					t.Skip("the filesystem refuses a name that is not UTF-8")
				}
				if err != nil {
					t.Fatal(err)
				}
				_ = l.Close()
				serve(t, dir, name, anyRequest(infoFrame(t)))
				b := newBackend(t, dir)
				ctx := t.Context()

				if got := list(t, b); len(got) != 1 || got[0].Name != name || !got[0].Answered {
					t.Fatalf("List = %+v, want %q answering", got, name)
				}
				if exists, err := b.Exists(ctx, name); err != nil || !exists {
					t.Errorf("Exists = %v, %v; want true", exists, err)
				}
				if _, err := b.History(ctx, name, 0); err != nil {
					t.Errorf("History: %v", err)
				}
				if _, err := b.ProtocolVersion(ctx, name); err != nil {
					t.Errorf("ProtocolVersion: %v", err)
				}
				if cmd, err := b.AttachCommand(ctx, name); err != nil || cmd.Argv[len(cmd.Argv)-1] != name {
					t.Errorf("AttachCommand = %+v, %v; want zmx attach on %q", cmd, err, name)
				}
				if err := b.Detach(ctx, name); err != nil {
					t.Errorf("Detach: %v", err)
				}
				if err := b.Kill(ctx, name); err != nil {
					t.Errorf("Kill: %v", err)
				}
			})
		}
	})

	t.Run("AttachCommand refuses a name zmx attach reads as its own flag, sending nothing", func(t *testing.T) {
		for _, name := range []string{"-h", "--help", "--labels", "--labels=a=1"} {
			dir := socketDir(t)
			d := serve(t, dir, name, anyRequest(infoFrame(t)))

			if _, err := newBackend(t, dir).AttachCommand(t.Context(), name); !errors.Is(err, backend.ErrInvalidName) {
				t.Errorf("AttachCommand(%q) = %v, want ErrInvalidName", name, err)
			}
			if reqs := d.received(); len(reqs) != 0 {
				t.Errorf("AttachCommand(%q) sent %+v", name, reqs)
			}
		}
	})
}

func TestKill(t *testing.T) {
	t.Run("it sends Kill and returns once the daemon hangs up with its socket gone", func(t *testing.T) {
		dir := socketDir(t)
		d := serve(t, dir, "doomed", ending)

		if err := newBackend(t, dir).Kill(t.Context(), "doomed"); err != nil {
			t.Fatalf("Kill: %v", err)
		}
		if reqs := d.received(); len(reqs) != 1 || reqs[0].tag != 5 || len(reqs[0].payload) != 0 {
			t.Errorf("the daemon received %+v, want one empty Kill", reqs)
		}
	})

	t.Run("it reports a daemon that hangs up but keeps its socket as a kill that did not land", func(t *testing.T) {
		dir := socketDir(t)
		serve(t, dir, "survivor", hangingUp())

		err := newBackend(t, dir).Kill(t.Context(), "survivor")

		if err == nil || errors.Is(err, backend.ErrNoSuchSession) || errors.Is(err, backend.ErrTimeout) {
			t.Errorf("Kill of a daemon that hung up and stayed = %v, want a plain failure", err)
		}
	})

	t.Run("it gives up past its ceiling, saying the kill may still land", func(t *testing.T) {
		dir := socketDir(t)
		serve(t, dir, "wedged", silent)

		err := newBackend(t, dir).Kill(t.Context(), "wedged")

		if !errors.Is(err, backend.ErrTimeout) || !strings.Contains(err.Error(), "may still land") {
			t.Errorf("Kill of a silent daemon = %v, want ErrTimeout saying the kill may still land", err)
		}
	})
}

func TestDetach(t *testing.T) {
	t.Run("it sends DetachAll and returns once the daemon hangs up", func(t *testing.T) {
		dir := socketDir(t)
		d := serve(t, dir, "watched", hangingUp())

		if err := newBackend(t, dir).Detach(t.Context(), "watched"); err != nil {
			t.Fatalf("Detach: %v", err)
		}
		if reqs := d.received(); len(reqs) != 1 || reqs[0].tag != 4 || len(reqs[0].payload) != 0 {
			t.Errorf("the daemon received %+v, want one empty DetachAll", reqs)
		}
	})

	t.Run("it gives up past its ceiling, saying the detach may still land", func(t *testing.T) {
		dir := socketDir(t)
		serve(t, dir, "wedged", silent)

		err := newBackend(t, dir).Detach(t.Context(), "wedged")

		if !errors.Is(err, backend.ErrTimeout) || !strings.Contains(err.Error(), "may still land") {
			t.Errorf("Detach of a silent daemon = %v, want ErrTimeout saying the detach may still land", err)
		}
	})
}

func TestHistory(t *testing.T) {
	t.Run("it asks for the whole history as VT with History, and returns it verbatim", func(t *testing.T) {
		dir := socketDir(t)
		vt := []byte("\x1b[31mred\x1b[0m  \r\n\r\n")
		d := serve(t, dir, "s", answering(frame(1, 1, []byte("output")), frame(8, 1, vt)))

		got, err := newBackend(t, dir).History(t.Context(), "s", 0)

		if err != nil || !bytes.Equal(got, vt) {
			t.Errorf("History(0) = %q, %v; want %q", got, err, vt)
		}
		if reqs := d.received(); len(reqs) != 1 || reqs[0].tag != 8 || !bytes.Equal(reqs[0].payload, []byte{1}) {
			t.Errorf("the daemon received %+v, want History of format vt", reqs)
		}
	})

	t.Run("it bounds the history with a Capture of the rows asked for", func(t *testing.T) {
		dir := socketDir(t)
		d := serve(t, dir, "s", answering(frame(22, 1, []byte("screen"))))

		got, err := newBackend(t, dir).History(t.Context(), "s", 300)

		if err != nil || string(got) != "screen" {
			t.Errorf("History(300) = %q, %v; want screen", got, err)
		}
		want := []byte{1, 0, 0, 0, 44, 1, 0, 0}
		if reqs := d.received(); len(reqs) != 1 || reqs[0].tag != 22 || !bytes.Equal(reqs[0].payload, want) {
			t.Errorf("the daemon received %+v, want a Capture of 300 rows as vt", reqs)
		}
	})

	t.Run("it reports an empty answer or a refused request as no history", func(t *testing.T) {
		for _, tt := range []struct {
			name  string
			reply []byte
		}{
			{"empty", frame(8, 1, nil)},
			{"refused", frame(255, 1, nil)},
		} {
			t.Run(tt.name, func(t *testing.T) {
				dir := socketDir(t)
				serve(t, dir, "s", answering(tt.reply))

				if got, err := newBackend(t, dir).History(t.Context(), "s", 0); !errors.Is(err, backend.ErrHistoryUnavailable) {
					t.Errorf("History = %q, %v; want ErrHistoryUnavailable", got, err)
				}
			})
		}
	})

	t.Run("it gives up past its ceiling", func(t *testing.T) {
		dir := socketDir(t)
		serve(t, dir, "wedged", silent)

		if _, err := newBackend(t, dir).History(t.Context(), "wedged", 0); !errors.Is(err, backend.ErrTimeout) {
			t.Errorf("History of a silent daemon = %v, want ErrTimeout", err)
		}
	})

	t.Run("it refuses a negative row count without asking", func(t *testing.T) {
		dir := socketDir(t)
		d := serve(t, dir, "s", silent)

		if _, err := newBackend(t, dir).History(t.Context(), "s", -1); err == nil {
			t.Error("History(-1) = nil error")
		}
		if reqs := d.received(); len(reqs) != 0 {
			t.Errorf("History(-1) sent %+v", reqs)
		}
	})
}

// envValues returns every value env gives key, in order.
func envValues(env []string, key string) []string {
	var values []string
	for _, kv := range env {
		if k, v, _ := strings.Cut(kv, "="); k == key {
			values = append(values, v)
		}
	}
	return values
}

func TestAttachCommand(t *testing.T) {
	t.Run("it runs zmx attach on the live session, pinned to the directory, without the caller's prefix", func(t *testing.T) {
		dir := socketDir(t)
		serve(t, dir, "work", silent)
		b := newBackendRunning(t, "/opt/zmx/bin/zmx", dir,
			"PATH=/usr/bin:/bin", "ZMX_DIR=/Users/lee/.local/state/zmx", "ZMX_SESSION_PREFIX=pre-", "TERM=xterm-ghostty")

		cmd, err := b.AttachCommand(t.Context(), "work")
		if err != nil {
			t.Fatalf("AttachCommand: %v", err)
		}

		if want := []string{"/opt/zmx/bin/zmx", "attach", "work"}; !slices.Equal(cmd.Argv, want) {
			t.Errorf("Argv = %q, want %q", cmd.Argv, want)
		}
		for key, want := range map[string][]string{
			"ZMX_DIR":            {dir},
			"ZMX_SESSION_PREFIX": nil,
			"PATH":               {"/usr/bin:/bin"},
			"TERM":               {"xterm-ghostty"},
		} {
			if got := envValues(cmd.Env, key); !slices.Equal(got, want) {
				t.Errorf("Env %s = %q, want %q", key, got, want)
			}
		}
		if cmd.Switches {
			t.Error("Switches outside any session, want an attach")
		}
	})

	t.Run("it keeps the caller's session, to switch it, only when that session is held in the directory and answers", func(t *testing.T) {
		dir := socketDir(t)
		serve(t, dir, "work", silent)
		home := serve(t, dir, "home", answering(infoFrame(t)))
		serve(t, dir, "wedged", silent)
		refusing(t, dir, "busy")
		for current, switches := range map[string]bool{"home": true, "wedged": false, "busy": false, "gone": false, "HOME": false, "": false} {
			t.Run(fmt.Sprintf("inside %q", current), func(t *testing.T) {
				b := newBackend(t, dir, "PATH=/usr/bin:/bin", "ZMX_SESSION="+current)

				cmd, err := b.AttachCommand(t.Context(), "work")
				if err != nil {
					t.Fatalf("AttachCommand: %v", err)
				}

				want := []string(nil)
				if switches {
					want = []string{current}
				}
				if got := envValues(cmd.Env, "ZMX_SESSION"); !slices.Equal(got, want) || cmd.Switches != switches {
					t.Errorf("Env ZMX_SESSION = %q, Switches %v; want %q, %v", got, cmd.Switches, want, switches)
				}
			})
		}
		if reqs := home.received(); len(reqs) != 1 || reqs[0].tag != 6 {
			t.Errorf("home received %+v, want one Info", reqs)
		}
	})
}

func TestExists(t *testing.T) {
	dir := socketDir(t)
	serve(t, dir, "live", silent)
	stale(t, dir, "crashed")
	b := newBackend(t, dir)

	for name, want := range map[string]bool{"live": true, "crashed": false, "gone": false, "liv": false, "logs": false, "": false} {
		t.Run(name, func(t *testing.T) {
			if got, err := b.Exists(t.Context(), name); err != nil || got != want {
				t.Errorf("Exists(%q) = %v, %v; want %v", name, got, err, want)
			}
		})
	}

	t.Run("it does not count a name held only in another case", func(t *testing.T) {
		foldsCase(t, dir)
		if got, err := b.Exists(t.Context(), "LIVE"); err != nil || got {
			t.Errorf("Exists(LIVE) = %v, %v; want false", got, err)
		}
	})

	t.Run("it reports a refusing socket from this boot as not answering", func(t *testing.T) {
		refusing(t, dir, "busy")
		if got, err := b.Exists(t.Context(), "busy"); !errors.Is(err, backend.ErrTimeout) {
			t.Errorf("Exists(busy) = %v, %v; want ErrTimeout", got, err)
		}
	})
}

func TestCurrent(t *testing.T) {
	t.Run("it reads ZMX_SESSION, the last entry winning", func(t *testing.T) {
		b := newBackend(t, socketDir(t), "ZMX_SESSION=first", "PATH=/bin", "ZMX_SESSION=work")
		if got := b.Current(); got != "work" {
			t.Errorf("Current = %q, want work", got)
		}
	})

	t.Run("it is empty outside a session", func(t *testing.T) {
		if got := newBackend(t, socketDir(t), "PATH=/bin").Current(); got != "" {
			t.Errorf("Current = %q, want empty", got)
		}
	})
}

func TestProtocolVersion(t *testing.T) {
	for _, version := range []byte{1, 7} {
		t.Run(fmt.Sprintf("it reports version %d from the daemon's answer", version), func(t *testing.T) {
			dir := socketDir(t)
			serve(t, dir, "s", answering(frame(6, version, make([]byte, 552))))

			if got, err := newBackend(t, dir).ProtocolVersion(t.Context(), "s"); err != nil || got != int(version) {
				t.Errorf("ProtocolVersion = %d, %v; want %d", got, err, version)
			}
		})
	}

	t.Run("it reports a daemon that cannot read its request as a timeout", func(t *testing.T) {
		dir := socketDir(t)
		serve(t, dir, "old", silent)

		if _, err := newBackend(t, dir).ProtocolVersion(t.Context(), "old"); !errors.Is(err, backend.ErrTimeout) {
			t.Errorf("ProtocolVersion of a silent daemon = %v, want ErrTimeout", err)
		}
	})
}
