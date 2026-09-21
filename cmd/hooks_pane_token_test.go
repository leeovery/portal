package cmd

import (
	"bytes"
	"fmt"
	"os"
	"testing"

	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/nanoid"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

func TestHooksSetStampsPaneToken(t *testing.T) {
	t.Run("it stamps and writes under a freshly minted token", func(t *testing.T) {
		_, hooksFile := hooksFileInTempDir(t, nil)
		t.Setenv("TMUX_PANE", "%7")

		stamper := &recordingPaneStamper{}
		withHooksDeps(t, HooksDeps{KeyResolver: &mockKeyResolver{key: ""}, PaneStamper: stamper})

		if _, err := runHookSet(t, "claude --resume abc123"); err != nil {
			t.Fatalf("hook set: %v", err)
		}

		if len(stamper.calls) != 1 {
			t.Fatalf("set-option call count = %d, want 1", len(stamper.calls))
		}
		call := stamper.calls[0]
		if call.target != "%7" {
			t.Errorf("stamp target = %q, want %q", call.target, "%7")
		}
		if call.name != state.PortalPaneIDOption {
			t.Errorf("stamp option = %q, want %q", call.name, state.PortalPaneIDOption)
		}

		data := readHooksJSON(t, hooksFile)
		if len(data) != 1 {
			t.Fatalf("hooks.json entry count = %d, want 1 (%v)", len(data), data)
		}
		if data[call.value]["on-resume"].Command != "claude --resume abc123" {
			t.Errorf("entry under the stamped token %q = %v, want the registered command", call.value, data)
		}
		if !nanoid.IsTokenShaped(call.value) {
			t.Errorf("stamped value %q is not token-shaped", call.value)
		}
	})

	t.Run("it reuses an existing token and issues no set-option", func(t *testing.T) {
		_, hooksFile := hooksFileInTempDir(t, nil)
		t.Setenv("TMUX_PANE", "%7")

		stamper := &recordingPaneStamper{}
		withHooksDeps(t, HooksDeps{KeyResolver: &mockKeyResolver{key: hookstest.SubjectSeedA}, PaneStamper: stamper})

		if _, err := runHookSet(t, "some-cmd"); err != nil {
			t.Fatalf("hook set: %v", err)
		}

		if len(stamper.calls) != 0 {
			t.Errorf("set-option call count = %d, want 0 (a stamped pane must not be re-minted): %+v", len(stamper.calls), stamper.calls)
		}
		data := readHooksJSON(t, hooksFile)
		if len(data) != 1 || data[hookstest.SubjectSeedA]["on-resume"].Command != "some-cmd" {
			t.Errorf("hooks.json = %v, want a single entry under the pane's existing token", data)
		}
	})

	t.Run("it writes nothing when the stamp fails", func(t *testing.T) {
		_, hooksFile := hooksFileInTempDir(t, nil)
		t.Setenv("TMUX_PANE", "%999")

		stamper := &recordingPaneStamper{err: fmt.Errorf("no such pane: %%999")}
		withHooksDeps(t, HooksDeps{KeyResolver: &mockKeyResolver{key: ""}, PaneStamper: stamper})

		_, err := runHookSet(t, "some-cmd")
		if err == nil {
			t.Fatal("expected an error from a failed stamp, got nil")
		}
		if err.Error() != "no such pane: %999" {
			t.Errorf("error = %q, want tmux's own words unaltered", err.Error())
		}
		if _, statErr := os.Stat(hooksFile); statErr == nil {
			t.Error("hooks.json was created despite the stamp failing")
		}
	})

	t.Run("it stamps before it writes", func(t *testing.T) {
		_, hooksFile := hooksFileInTempDir(t, nil)
		t.Setenv("TMUX_PANE", "%7")

		stamper := &recordingPaneStamper{}
		stamper.onCall = func() {
			if _, err := os.Stat(hooksFile); err == nil {
				t.Error("hooks.json already existed when the stamp ran: the write must not precede the stamp")
			}
		}
		withHooksDeps(t, HooksDeps{KeyResolver: &mockKeyResolver{key: ""}, PaneStamper: stamper})

		if _, err := runHookSet(t, "some-cmd"); err != nil {
			t.Fatalf("hook set: %v", err)
		}
		if len(stamper.calls) != 1 {
			t.Fatalf("set-option call count = %d, want 1", len(stamper.calls))
		}
		if _, err := os.Stat(hooksFile); err != nil {
			t.Fatalf("hooks.json was not written: %v", err)
		}
	})

	t.Run("it leaves the token in place when the write fails", func(t *testing.T) {
		denied, _ := hooksFileInTempDir(t, nil)
		t.Setenv("TMUX_PANE", "%7")
		if err := os.Chmod(denied, 0o000); err != nil {
			t.Fatalf("chmod: %v", err)
		}
		t.Cleanup(func() { _ = os.Chmod(denied, 0o700) })

		stamper := &recordingPaneStamper{}
		withHooksDeps(t, HooksDeps{KeyResolver: &mockKeyResolver{key: ""}, PaneStamper: stamper})

		if _, err := runHookSet(t, "some-cmd"); err == nil {
			t.Fatal("expected an error from an unwritable hooks path, got nil")
		}

		if len(stamper.calls) != 1 {
			t.Fatalf("set-option call count = %d, want exactly 1 (the stamp stands; there is no unstamp): %+v", len(stamper.calls), stamper.calls)
		}
		if stamper.calls[0].value == "" {
			t.Error("the pane was stamped with an empty token")
		}
	})

	t.Run("it never writes an empty key", func(t *testing.T) {
		_, hooksFile := hooksFileInTempDir(t, nil)
		t.Setenv("TMUX_PANE", "%7")

		stamper := &recordingPaneStamper{}
		withHooksDeps(t, HooksDeps{
			KeyResolver: &mockKeyResolver{key: ""},
			PaneStamper: stamper,
			TokenMinter: func() (string, error) { return "", fmt.Errorf("entropy source unavailable") },
		})

		_, err := runHookSet(t, "some-cmd")
		if err == nil {
			t.Fatal("expected an error from a failed mint, got nil")
		}
		if len(stamper.calls) != 0 {
			t.Errorf("set-option call count = %d, want 0 when no token was minted", len(stamper.calls))
		}
		if _, statErr := os.Stat(hooksFile); statErr == nil {
			t.Error("hooks.json was created despite the mint failing")
		}
	})

	t.Run("it takes no tmux call on the --pane-key path", func(t *testing.T) {
		_, hooksFile := hooksFileInTempDir(t, map[string]map[string]string{
			hookstest.SubjectSeedA: {"on-resume": "claude --resume xyz"},
		})
		t.Setenv("TMUX_PANE", "")

		resolver, stamper := paneKeyPathSeams()
		withHooksDeps(t, HooksDeps{KeyResolver: resolver, PaneStamper: stamper})

		resetRootCmd()
		rootCmd.SetOut(new(bytes.Buffer))
		rootCmd.SetErr(new(bytes.Buffer))
		rootCmd.SetArgs([]string{"hook", "rm", "--pane-key", hookstest.SubjectSeedA, "--on-resume"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("hook rm --pane-key: %v", err)
		}

		assertNoPaneTmuxCalls(t, resolver, stamper)
		if data := readHooksJSON(t, hooksFile); len(data) != 0 {
			t.Errorf("hooks.json = %v, want the seeded entry removed", data)
		}
	})
}

func TestHooksSetRefusesAnUnresolvablePane(t *testing.T) {
	const stderr = "no such pane: %999"

	t.Run("it mints and stamps nothing when the probe fails", func(t *testing.T) {
		hooksFileInTempDir(t, nil)
		t.Setenv("TMUX_PANE", "%999")

		stamper := &recordingPaneStamper{}
		minted := 0
		withHooksDeps(t, HooksDeps{
			KeyResolver: &mockKeyResolver{err: &tmux.CommandError{Stderr: stderr}},
			PaneStamper: stamper,
			TokenMinter: func() (string, error) { minted++; return hookstest.SubjectSeedB, nil },
		})

		if _, err := runHookSet(t, "some-cmd"); err == nil {
			t.Fatal("expected an error from an unresolvable pane, got nil")
		}
		if minted != 0 {
			t.Errorf("mint count = %d, want 0", minted)
		}
		if len(stamper.calls) != 0 {
			t.Errorf("set-option call count = %d, want 0: %+v", len(stamper.calls), stamper.calls)
		}
	})
}
