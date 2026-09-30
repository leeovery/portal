package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/hooksweep"
	"github.com/leeovery/portal/internal/resumemode"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmuxtest"
)

// A parked pane's registration survives the stale-hook sweep only while some
// live pane carries its token, so these run against a real tmux server: the
// sweep's verdict is taken from what tmux reports, not from a scripted reply.
func TestHydrateLazy_AParkedPaneCarriesTheTokenThatProtectsItsRegistration(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)
	cases := []struct {
		name string
		// restampLanded stages the pane already carrying its saved token, as
		// restore leaves it when its best-effort re-stamp succeeds.
		restampLanded bool
	}{
		{name: "restore's re-stamp failed", restampLanded: false},
		{name: "restore's re-stamp landed", restampLanded: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			token := hookstest.SubjectSeedA
			socket := tmuxtest.New(t, "ptl-lazytok-")
			socket.Run(t, "new-session", "-d", "-s", "work")
			socket.WaitForSession(t, "work", 2*time.Second)
			pane := strings.TrimSpace(socket.Run(t, "display-message", "-p", "-t", "=work:", "#{pane_id}"))
			if tc.restampLanded {
				socket.Run(t, "set-option", "-p", "-t", pane, state.PortalPaneIDOption, token)
			}
			t.Setenv("TMUX_PANE", pane)
			t.Setenv("SHELL", "/bin/zsh")

			store := hydrateStoreWithMode(t, token, "echo hi", resumemode.Lazy)
			exec := &stubExecShell{}
			dir := t.TempDir()
			fifo := makeFIFO(t, dir, "hydrate-work__0.0.fifo")
			scrollback := filepath.Join(dir, "sb")
			if err := os.WriteFile(scrollback, []byte("OLD"), 0o600); err != nil {
				t.Fatalf("seed scrollback: %v", err)
			}
			signalFIFOAsync(t, fifo)
			cfg := hydrateCfg(t, hydrateCfgOpts{
				FIFO:           fifo,
				File:           scrollback,
				OpenFIFO:       openFIFOWithTimeout,
				HookKey:        token,
				HookStore:      store,
				LoadPrefsStore: lazyPrefs(t, ""),
				ResolveExe:     func() (string, error) { return lazyExe, nil },
				ExecShell:      exec.fn(),
			})
			cfg.Client = socket.Client()

			if err := runHydrate(cfg); err != nil {
				t.Fatalf("runHydrate: %v", err)
			}
			if exec.target != "/bin/sh" || len(exec.args) < 3 || !strings.Contains(exec.args[2], resumeDrawSubcommand) {
				t.Fatalf("exec = %q %v, want the pane parked on its panel", exec.target, exec.args)
			}

			if got := strings.TrimSpace(socket.Run(t, "display-message", "-p", "-t", pane, "#{"+state.PortalPaneIDOption+"}")); got != token {
				t.Fatalf("parked pane's %s = %q, want the registration's token %q", state.PortalPaneIDOption, got, token)
			}

			if _, err := hooksweep.Run(socket.Client(), store); err != nil {
				t.Fatalf("hooksweep.Run: %v", err)
			}
			if got, err := store.LookupOnResume(token, hooks.ViaHydrate); err != nil || !got.Found {
				t.Fatalf("after the sweep LookupOnResume(%q) = %+v, %v; want the registration kept", token, got, err)
			}

			probe := resumeWaitProbe{lookup: func(key string) (hooks.OnResume, error) {
				return store.LookupOnResume(key, hooks.ViaHydrate)
			}}
			payload := samplePayload()
			payload.Command, payload.HookKey, payload.Pane = "echo hi", token, pane
			answerEnter(t, &probe, payload)
			assertHookHandOff(t, &probe, "echo hi")
		})
	}
}
