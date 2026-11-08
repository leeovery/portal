package cmd

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/tui"
	"github.com/leeovery/portal/internal/xdg"
)

// A question no terminal saw is never answered, so a draw asks one only where
// a terminal holds the session, as backend.Held judges it, and where it can't
// tell, asks.
func TestSessionAttached(t *testing.T) {
	session := samplePayload().Session
	cases := []struct {
		name  string
		stage func(f *backendtest.Fake)
		want  bool
	}{
		{"a session a terminal leads", func(f *backendtest.Fake) {
			f.Add(backend.Session{Name: session, Clients: 1})
			f.SetLeaderEnv(session, aLeader)
		}, true},
		{"a session a terminal is attached to", func(f *backendtest.Fake) {
			f.Add(backend.Session{Name: session, Clients: 1})
			f.SetAttachClients(session, aTerminal)
		}, true},
		{"a session only a reader is connected to", func(f *backendtest.Fake) { f.Add(backend.Session{Name: session, Clients: 1}) }, false},
		{"a session no client is connected to", func(f *backendtest.Fake) { f.Add(backend.Session{Name: session}) }, false},
		{"a session that doesn't answer", func(f *backendtest.Fake) {
			f.Add(backend.Session{Name: session})
			f.StopAnswering(session)
		}, true},
		{"a session whose leader the backend can't ask", func(f *backendtest.Fake) {
			f.Add(backend.Session{Name: session})
			f.Fail(backendtest.OpLeaderEnv, errors.New("connection refused"))
		}, true},
		{"a session whose clients the process table can't show", func(f *backendtest.Fake) {
			f.Add(backend.Session{Name: session})
			f.Fail(backendtest.OpAttachClients, backend.ErrUnreadableClient)
		}, true},
		{"no session under the name", func(*backendtest.Fake) {}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := backendtest.New()
			tc.stage(fake)
			withBackendDeps(t, BackendDeps{Backend: fake})

			if got := sessionAttached(session); got != tc.want {
				t.Errorf("sessionAttached() = %v, want %v", got, tc.want)
			}
		})
	}

	t.Run("it asks within its bound", func(t *testing.T) {
		fake := backendtest.New(backend.Session{Name: session, Clients: 1})
		fake.SetAttachClients(session, aTerminal)
		asked := &askBoundBackend{Backend: fake}
		withBackendDeps(t, BackendDeps{Backend: asked})

		sessionAttached(session)

		for _, op := range []backendtest.Op{backendtest.OpLeaderEnv, backendtest.OpSession} {
			if left, ok := asked.left[op]; !ok || left <= 0 || left > resumeAttachedBound {
				t.Errorf("%s had %v left of its deadline (asked %v), want no more than %v", op, left, ok, resumeAttachedBound)
			}
		}
	})
}

// The draw the command runs asks its own session, the one its argv names,
// whether a terminal shows it: another session's terminal answers nothing.
func TestStateResumeDrawCommand_AsksItsOwnSessionWhetherATerminalShowsIt(t *testing.T) {
	session := samplePayload().Session
	const other = "other-x1y2"
	for _, tc := range []struct {
		name string
		led  string
		want bool
	}{
		{"no terminal on it", other, false},
		{"a terminal on it", session, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := backendtest.New(backend.Session{Name: session, Clients: 1}, backend.Session{Name: other, Clients: 1})
			fake.SetLeaderEnv(tc.led, aLeader)
			withBackendDeps(t, BackendDeps{Backend: fake})
			t.Setenv(xdg.PrefsFile.EnvVar, filepath.Join(t.TempDir(), "prefs.json"))
			t.Setenv(xdg.ThemesDir.EnvVar, t.TempDir())
			var draw resumeDrawConfig
			withFuncSeam(t, &resumeDrawRunFunc, func(cfg resumeDrawConfig) error {
				draw = cfg
				return nil
			})
			var attached func() bool
			withFuncSeam(t, &resolvePaneThemeFunc, func(_ theme.Nomination, _ bool, _ int, a func() bool, _ func() (int, error)) (tui.PaneAppearance, error) {
				attached = a
				return tui.PaneAppearance{}, nil
			})

			executeChainArgv(t, resumeChainArgv("portal", resumeDrawSubcommand, samplePayload())[1:])
			if _, err := draw.ResolveTheme(false, 0, nil); err != nil {
				t.Fatalf("ResolveTheme() error = %v", err)
			}

			if got := attached(); got != tc.want {
				t.Errorf("the draw was told a terminal shows the session: %v, want %v", got, tc.want)
			}
		})
	}
}

// askBoundBackend records how long each call to the session's zmx daemon had
// left of its context.
type askBoundBackend struct {
	backend.Backend
	left map[backendtest.Op]time.Duration
}

func (b *askBoundBackend) record(ctx context.Context, op backendtest.Op) {
	if b.left == nil {
		b.left = map[backendtest.Op]time.Duration{}
	}
	if deadline, ok := ctx.Deadline(); ok {
		b.left[op] = time.Until(deadline)
	}
}

func (b *askBoundBackend) LeaderEnv(ctx context.Context, name string) (backend.ClientEnv, error) {
	b.record(ctx, backendtest.OpLeaderEnv)
	return b.Backend.LeaderEnv(ctx, name)
}

func (b *askBoundBackend) Session(ctx context.Context, name string) (backend.Session, error) {
	b.record(ctx, backendtest.OpSession)
	return b.Backend.Session(ctx, name)
}
