package zmx_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

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

// anyRequest answers every request as a zmx daemon would: Info with info,
// History and Capture with screen, Kill by unlinking its socket and hanging
// up, DetachAll by hanging up.
func anyRequest(info, screen []byte) func(*daemon, net.Conn) {
	return func(d *daemon, c net.Conn) {
		r, err := d.read(c)
		if err != nil {
			return
		}
		switch r.tag {
		case 6:
			_, _ = c.Write(info)
		case 8, 22:
			_, _ = c.Write(frame(r.tag, 1, screen))
		case 5:
			_ = os.Remove(d.path)
			return
		default:
			return
		}
		_, _ = io.Copy(io.Discard, c)
	}
}

func TestAttachFlags(t *testing.T) {
	t.Run("AttachCommand refuses a name zmx attach reads as its own flag, sending nothing", func(t *testing.T) {
		for _, name := range []string{"-h", "--help", "--labels", "--labels=a=1"} {
			dir := socketDir(t)
			d := serve(t, dir, name, anyRequest(infoFrame(t), []byte("screen")))

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

	t.Run("it reports a daemon that hangs up but keeps its socket as not answering", func(t *testing.T) {
		dir := socketDir(t)
		serve(t, dir, "survivor", hangingUp())

		err := newBackend(t, dir).Kill(t.Context(), "survivor")

		if !errors.Is(err, backend.ErrTimeout) || !strings.Contains(err.Error(), "hung up without ending") {
			t.Errorf("Kill of a daemon that hung up and stayed = %v, want ErrTimeout saying it hung up", err)
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
	t.Run("it runs zmx attach on the session, pinned to the directory, without the caller's prefix, tmux or detach-key switch", func(t *testing.T) {
		dir := socketDir(t)
		serve(t, dir, "work", answering(infoFrame(t)))
		b := newBackendRunning(t, "/opt/zmx/bin/zmx", dir,
			"PATH=/usr/bin:/bin", "ZMX_DIR=/Users/lee/.local/state/zmx", "ZMX_SESSION_PREFIX=pre-", "TERM=xterm-ghostty",
			"TMUX=/tmp/tmux-501/default,1,0", "TMUX_PANE=%1", "ZMX_NO_DETACH_KEY=1")

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
			"TMUX":               nil,
			"TMUX_PANE":          nil,
			"ZMX_NO_DETACH_KEY":  nil,
			"PATH":               {"/usr/bin:/bin"},
			"TERM":               {"xterm-ghostty"},
		} {
			if got := envValues(cmd.Env, key); !slices.Equal(got, want) {
				t.Errorf("Env %s = %q, want %q", key, got, want)
			}
		}
	})

	t.Run("it keeps the caller's session, to switch it, only when it answers, held exactly, in the caller's own directory", func(t *testing.T) {
		runtimeDir := socketDir(t)
		dir := filepath.Join(runtimeDir, "zmx")
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		serve(t, dir, "work", answering(infoFrame(t)))
		home := serve(t, dir, "home", answering(infoFrame(t)))
		refusing(t, dir, "busy")
		for _, tt := range []struct {
			inside   string
			env      []string
			switches bool
		}{
			{"home", []string{"ZMX_DIR=" + dir, "ZMX_SESSION=home"}, true},
			{"home, its directory given with a trailing slash", []string{"ZMX_DIR=" + dir + "/", "ZMX_SESSION=home"}, true},
			{"home with no ZMX_DIR, in zmx's default directory, the Backend's", []string{"XDG_RUNTIME_DIR=" + runtimeDir, "ZMX_SESSION=home"}, true},
			{"home in another directory", []string{"ZMX_DIR=/Users/lee/.local/state/zmx", "ZMX_SESSION=home"}, false},
			{"home with no ZMX_DIR, in zmx's default directory, another", []string{"TMPDIR=/tmp", "ZMX_SESSION=home"}, false},
			{"a session that refuses", []string{"ZMX_DIR=" + dir, "ZMX_SESSION=busy"}, false},
			{"home in another case", []string{"ZMX_DIR=" + dir, "ZMX_SESSION=HOME"}, false},
		} {
			t.Run("inside "+tt.inside, func(t *testing.T) {
				env := append(slices.Clone(tt.env), "TMUX=/tmp/tmux-501/default,1,0", "TMUX_PANE=%1")
				cmd, err := newBackend(t, dir, env...).AttachCommand(t.Context(), "work")
				if err != nil {
					t.Fatalf("AttachCommand: %v", err)
				}
				if got := append(envValues(cmd.Env, "TMUX"), envValues(cmd.Env, "TMUX_PANE")...); len(got) != 0 {
					t.Errorf("Env carries the caller's tmux: %q", got)
				}

				var want []string
				if tt.switches {
					want = []string{lookupEnv(tt.env, "ZMX_SESSION")}
				}
				if got := envValues(cmd.Env, "ZMX_SESSION"); !slices.Equal(got, want) || cmd.Switches != tt.switches {
					t.Errorf("Env ZMX_SESSION = %q, Switches %v; want %q, %v", got, cmd.Switches, want, tt.switches)
				}
			})
		}
		if reqs := home.received(); len(reqs) != 3 {
			t.Errorf("home received %+v, want one Info for each attach that could switch it", reqs)
		}
	})

	t.Run("it asks each session within a ceiling of its own, so a slow target does not starve the caller's", func(t *testing.T) {
		dir := socketDir(t)
		const ceiling = time.Second
		serve(t, dir, "work", answeringAfter(7*ceiling/10, infoFrame(t)))
		serve(t, dir, "home", answeringAfter(5*ceiling/10, infoFrame(t)))
		b := newBackend(t, dir, "ZMX_DIR="+dir, "ZMX_SESSION=home")
		zmx.ShortenCeilings(b, ceiling)

		if cmd, err := b.AttachCommand(t.Context(), "work"); err != nil || !cmd.Switches {
			t.Errorf("AttachCommand = %+v, %v; want a switch of home, which answers within its own ceiling", cmd, err)
		}
	})

	t.Run("it fails with the caller's cause when that ends while the caller's session is asked", func(t *testing.T) {
		dir := socketDir(t)
		serve(t, dir, "work", answering(infoFrame(t)))
		cause := errors.New("caller gave up")
		ctx, cancel := context.WithCancelCause(t.Context())
		serve(t, dir, "home", func(d *daemon, c net.Conn) {
			if _, err := d.read(c); err == nil {
				cancel(cause)
			}
			_, _ = io.Copy(io.Discard, c)
		})

		cmd, err := newBackend(t, dir, "ZMX_DIR="+dir, "ZMX_SESSION=home").AttachCommand(ctx, "work")

		if !errors.Is(err, cause) {
			t.Errorf("AttachCommand = %+v, %v; want the caller's cause", cmd, err)
		}
	})

	t.Run("it fails with the caller's cause when that ends after the target answers, with no session to ask", func(t *testing.T) {
		dir := socketDir(t)
		serve(t, dir, "work", answering(infoFrame(t)))
		cause := errors.New("caller gave up")
		ctx, cancel := context.WithCancelCause(t.Context())
		b := mustNew(t, zmx.Config{
			Binary: "/nonexistent/zmx",
			Dir:    dir,
			// Read once the target has answered, to decide whether to switch.
			Environ: func() []string {
				cancel(cause)
				return []string{"PATH=/usr/bin:/bin"}
			},
		})

		if cmd, err := b.AttachCommand(ctx, "work"); !errors.Is(err, cause) {
			t.Errorf("AttachCommand = %+v, %v; want the caller's cause", cmd, err)
		}
	})
}

func lookupEnv(env []string, key string) string {
	values := envValues(env, key)
	if len(values) == 0 {
		return ""
	}
	return values[len(values)-1]
}

func TestExists(t *testing.T) {
	dir := socketDir(t)
	serve(t, dir, "live", answering(infoFrame(t)))
	stale(t, dir, "crashed")
	b := newBackend(t, dir)

	for name, want := range map[string]bool{"live": true, "crashed": false, "gone": false, "liv": false, "logs": false} {
		t.Run(name, func(t *testing.T) {
			if got, err := b.Exists(t.Context(), name); err != nil || got != want {
				t.Errorf("Exists(%q) = %v, %v; want %v", name, got, err, want)
			}
		})
	}

	t.Run("it asks the session for Info, so one that takes the connection and never answers is ErrTimeout", func(t *testing.T) {
		d := serve(t, dir, "wedged", silent)

		if got, err := b.Exists(t.Context(), "wedged"); !errors.Is(err, backend.ErrTimeout) {
			t.Errorf("Exists(wedged) = %v, %v; want ErrTimeout", got, err)
		}
		if reqs := d.received(); len(reqs) != 1 || reqs[0].tag != 6 {
			t.Errorf("wedged received %+v, want one Info", reqs)
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

	t.Run("it reads a session under a name Create refuses", func(t *testing.T) {
		dir := socketDir(t)
		serve(t, dir, "-x", answering(infoFrame(t)))

		if got, err := newBackend(t, dir).ProtocolVersion(t.Context(), "-x"); err != nil || got != zmx.ProtocolVersion {
			t.Errorf("ProtocolVersion(-x) = %d, %v; want %d", got, err, zmx.ProtocolVersion)
		}
	})

	t.Run("it reports a daemon that hangs up before a header as not answering", func(t *testing.T) {
		dir := socketDir(t)
		serve(t, dir, "odd", hangingUp())

		if _, err := newBackend(t, dir).ProtocolVersion(t.Context(), "odd"); !errors.Is(err, backend.ErrTimeout) {
			t.Errorf("ProtocolVersion of a daemon that hung up = %v, want ErrTimeout", err)
		}
	})

	t.Run("it reports a daemon that cannot read its request as a timeout", func(t *testing.T) {
		dir := socketDir(t)
		serve(t, dir, "old", silent)

		if _, err := newBackend(t, dir).ProtocolVersion(t.Context(), "old"); !errors.Is(err, backend.ErrTimeout) {
			t.Errorf("ProtocolVersion of a silent daemon = %v, want ErrTimeout", err)
		}
	})

	t.Run("it skips the session's output before the answer", func(t *testing.T) {
		dir := socketDir(t)
		serve(t, dir, "busy", answering(frame(1, 1, []byte("output")), frame(1, 1, nil), frame(6, 2, make([]byte, 552))))

		if got, err := newBackend(t, dir).ProtocolVersion(t.Context(), "busy"); err != nil || got != 2 {
			t.Errorf("ProtocolVersion = %d, %v; want 2 from the answer, not the output", got, err)
		}
	})

	// A zmx 0.8.1 daemon broadcasts its session's output to every client,
	// a probe included, in its own frame layout: byte 1 is the low byte of the
	// frame's length, not a version.
	t.Run("it reads no version from a zmx 0.8.1 daemon's output", func(t *testing.T) {
		dir := socketDir(t)
		serve(t, dir, "old", broadcastingOldOutput(143))

		if got, err := newBackend(t, dir).ProtocolVersion(t.Context(), "old"); !errors.Is(err, backend.ErrTimeout) {
			t.Errorf("ProtocolVersion of a 0.8.1 daemon broadcasting output = %d, %v; want ErrTimeout", got, err)
		}
	})

	t.Run("it reads no version past an output frame too long to skip", func(t *testing.T) {
		dir := socketDir(t)
		oversize := []byte{1, 1, 0, 0, 0, 0, 0, 0}
		binary.NativeEndian.PutUint32(oversize[4:], 64<<20+1)
		serve(t, dir, "garbled", answering(oversize, frame(6, 1, make([]byte, 552))))

		if got, err := newBackend(t, dir).ProtocolVersion(t.Context(), "garbled"); !errors.Is(err, backend.ErrTimeout) {
			t.Errorf("ProtocolVersion past an oversize frame = %d, %v; want ErrTimeout", got, err)
		}
	})

	t.Run("it reports a refusing socket as refusing", func(t *testing.T) {
		dir := socketDir(t)
		refusing(t, dir, "crashed")

		_, err := newBackend(t, dir).ProtocolVersion(t.Context(), "crashed")
		if !errors.Is(err, zmx.ErrRefusing) || !errors.Is(err, backend.ErrTimeout) {
			t.Errorf("ProtocolVersion of a refusing socket = %v, want ErrRefusing wrapping ErrTimeout", err)
		}
	})

	t.Run("it does not report a silent daemon as refusing", func(t *testing.T) {
		dir := socketDir(t)
		serve(t, dir, "wedged", silent)

		if _, err := newBackend(t, dir).ProtocolVersion(t.Context(), "wedged"); errors.Is(err, zmx.ErrRefusing) {
			t.Errorf("ProtocolVersion of a silent daemon = %v, want no ErrRefusing", err)
		}
	})
}

// broadcastingOldOutput writes, before reading anything, zmx 0.8.1 output
// frames: a 1-byte tag, a u32 length at bytes 1-4, padded to 8, and the
// payload. Then it holds the connection.
func broadcastingOldOutput(length uint32) func(*daemon, net.Conn) {
	return func(_ *daemon, c net.Conn) {
		h := make([]byte, 8)
		h[0] = 1
		binary.NativeEndian.PutUint32(h[1:5], length)
		for range 3 {
			_, _ = c.Write(append(slices.Clone(h), make([]byte, length)...))
		}
		_, _ = io.Copy(io.Discard, c)
	}
}

// asking is each call that asks a session for an answer, for the rules on a
// daemon that gives no well-formed one.
var asking = []struct {
	name string
	call func(context.Context, *zmx.Backend, string) error
}{
	{"Exists", func(ctx context.Context, b *zmx.Backend, name string) error {
		_, err := b.Exists(ctx, name)
		return err
	}},
	{"AttachCommand", func(ctx context.Context, b *zmx.Backend, name string) error {
		_, err := b.AttachCommand(ctx, name)
		return err
	}},
	{"History", func(ctx context.Context, b *zmx.Backend, name string) error {
		_, err := b.History(ctx, name, 0)
		return err
	}},
}

func TestNoWellFormedAnswer(t *testing.T) {
	record := infoFrame(t)[8:]
	for _, op := range asking {
		t.Run(op.name+" reads a daemon that hangs up or answers garbled as not answering", func(t *testing.T) {
			for reply, handle := range map[string]func(*daemon, net.Conn){
				"a hangup":                    hangingUp(),
				"a short record, then hangup": hangingUp(frame(6, 1, record[:400])),
				"a short record":              answering(frame(6, 1, record[:400])),
				"a frame cut short":           hangingUp(frame(8, 1, []byte("scr"))[:10]),
			} {
				dir := socketDir(t)
				serve(t, dir, "odd", handle)

				if err := op.call(t.Context(), newBackend(t, dir), "odd"); !errors.Is(err, backend.ErrTimeout) {
					t.Errorf("%s after %s = %v, want ErrTimeout", op.name, reply, err)
				}
			}
		})
	}

	t.Run("a caller's context ending before the answer is the caller's cause", func(t *testing.T) {
		for _, op := range asking {
			dir := socketDir(t)
			cause := errors.New("caller gave up")
			ctx, cancel := context.WithCancelCause(t.Context())
			serve(t, dir, "slow", func(d *daemon, c net.Conn) {
				if _, err := d.read(c); err == nil {
					cancel(cause)
				}
				_, _ = io.Copy(io.Discard, c)
			})

			if err := op.call(ctx, newBackend(t, dir), "slow"); !errors.Is(err, cause) {
				t.Errorf("%s = %v, want the caller's cause", op.name, err)
			}
		}
	})

	t.Run("an answer in another protocol version keeps its own error", func(t *testing.T) {
		dir := socketDir(t)
		serve(t, dir, "future", answering(frame(6, 2, record), frame(8, 2, []byte("screen"))))
		b := newBackend(t, dir)

		for _, op := range asking {
			if err := op.call(t.Context(), b, "future"); err == nil || errors.Is(err, backend.ErrTimeout) || !strings.Contains(err.Error(), "protocol version") {
				t.Errorf("%s = %v, want the protocol version mismatch", op.name, err)
			}
		}
	})

	t.Run("a session whose socket goes before it answers is gone", func(t *testing.T) {
		for _, op := range asking[1:3] {
			dir := socketDir(t)
			serve(t, dir, "ending", ending)

			if err := op.call(t.Context(), newBackend(t, dir), "ending"); !errors.Is(err, backend.ErrNoSuchSession) {
				t.Errorf("%s = %v, want ErrNoSuchSession", op.name, err)
			}
		}
		dir := socketDir(t)
		serve(t, dir, "ending", ending)
		if exists, err := newBackend(t, dir).Exists(t.Context(), "ending"); err != nil || exists {
			t.Errorf("Exists = %v, %v; want false", exists, err)
		}
	})
}

func TestDirFor(t *testing.T) {
	uid := strconv.Itoa(os.Getuid())
	for _, tt := range []struct {
		name    string
		environ []string
		want    string
	}{
		{"ZMX_DIR when set", []string{"ZMX_DIR=/z", "XDG_RUNTIME_DIR=/run/user/1", "TMPDIR=/t"}, "/z"},
		{"ZMX_DIR when set empty", []string{"ZMX_DIR=", "XDG_RUNTIME_DIR=/run/user/1"}, ""},
		{"zmx in XDG_RUNTIME_DIR", []string{"XDG_RUNTIME_DIR=/run/user/1", "TMPDIR=/t"}, "/run/user/1/zmx"},
		{"zmx-<uid> in TMPDIR, its trailing slashes trimmed", []string{"TMPDIR=/var/folders/x/T//"}, "/var/folders/x/T/zmx-" + uid},
		{"zmx-<uid> in /tmp", []string{"PATH=/bin"}, "/tmp/zmx-" + uid},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := zmx.DirFor(tt.environ); got != tt.want {
				t.Errorf("DirFor(%q) = %q, want %q", tt.environ, got, tt.want)
			}
		})
	}
}
