package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/resumemode"
	"github.com/leeovery/portal/internal/state"
)

// rawRegistration returns the JSON value hooks.json holds for one key's event,
// compacted. The stored shape is the subject here — a string is a registration
// carrying no mode, an object one that carries settings — and a decode into the
// store's own types answers on content alone.
func rawRegistration(t *testing.T, path, key, event string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read hooks file: %v", err)
	}
	var data map[string]map[string]json.RawMessage
	if err := json.Unmarshal(b, &data); err != nil {
		t.Fatalf("failed to unmarshal hooks JSON: %v", err)
	}
	value, ok := data[key][event]
	if !ok {
		t.Fatalf("hooks.json holds no %q entry under %q: %s", event, key, b)
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, value); err != nil {
		t.Fatalf("failed to compact the stored registration: %v", err)
	}
	return compact.String()
}

func TestHooksSetResumeMode(t *testing.T) {
	t.Run("it writes the object form when the flag names a mode", func(t *testing.T) {
		tests := []struct {
			name string
			want resumemode.Mode
		}{
			{name: "eager", want: resumemode.Eager},
			{name: "lazy", want: resumemode.Lazy},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, hooksFile := hooksFileInTempDir(t, nil)
				stateDir := t.TempDir()
				t.Setenv("PORTAL_STATE_DIR", stateDir)
				t.Setenv("TMUX_PANE", "%7")

				stamper := &recordingPaneStamper{}
				withHooksDeps(t, HooksDeps{KeyResolver: &mockKeyResolver{key: hookstest.SubjectSeedA}, PaneStamper: stamper})

				if _, err := runHookSet(t, "npm start", "--resume-mode", tt.name); err != nil {
					t.Fatalf("hook set: %v", err)
				}

				entry := readHooksJSON(t, hooksFile)[hookstest.SubjectSeedA]["on-resume"]
				if entry.Command != "npm start" {
					t.Errorf("command = %q, want %q", entry.Command, "npm start")
				}
				if entry.Resume != tt.want {
					t.Errorf("resume = %q, want %q", entry.Resume, tt.want)
				}

				want := `{"command":"npm start","resume":"` + tt.name + `"}`
				if got := rawRegistration(t, hooksFile, hookstest.SubjectSeedA, "on-resume"); got != want {
					t.Errorf("stored registration = %s, want %s", got, want)
				}
				if len(stamper.calls) != 0 {
					t.Errorf("set-option call count = %d, want 0 for an already-stamped pane: %+v", len(stamper.calls), stamper.calls)
				}
				if !saveRequestedExists(t, stateDir) {
					t.Error("save.requested was not touched after a pinned registration")
				}
			})
		}
	})

	t.Run("it writes a registration carrying no mode when the flag is not passed", func(t *testing.T) {
		_, hooksFile := hooksFileInTempDir(t, nil)
		t.Setenv("TMUX_PANE", "%7")
		withHooksDeps(t, HooksDeps{KeyResolver: &mockKeyResolver{key: hookstest.SubjectSeedA}})

		if _, err := runHookSet(t, "npm start"); err != nil {
			t.Fatalf("hook set: %v", err)
		}

		if got := readHooksJSON(t, hooksFile)[hookstest.SubjectSeedA]["on-resume"].Resume; got != resumemode.Unset {
			t.Errorf("resume = %q, want no mode", got)
		}
		if got := rawRegistration(t, hooksFile, hookstest.SubjectSeedA, "on-resume"); got != `"npm start"` {
			t.Errorf("stored registration = %s, want the string form", got)
		}
	})

	t.Run("it drops an object-form predecessor's mode when the flag is not passed", func(t *testing.T) {
		_, hooksFile := hookstest.StageStore(t, hookstest.Staging{
			Seed: `{"` + hookstest.SubjectSeedA + `":{"on-resume":{"command":"old-cmd","resume":"lazy","note":"kept"}}}`,
		})
		t.Setenv("PORTAL_HOOKS_FILE", hooksFile)
		t.Setenv("TMUX_PANE", "%7")
		withHooksDeps(t, HooksDeps{KeyResolver: &mockKeyResolver{key: hookstest.SubjectSeedA}})

		if _, err := runHookSet(t, "new-cmd"); err != nil {
			t.Fatalf("hook set: %v", err)
		}

		if got := rawRegistration(t, hooksFile, hookstest.SubjectSeedA, "on-resume"); got != `"new-cmd"` {
			t.Errorf("stored registration = %s, want the string form carrying neither the old mode nor its unmodelled attribute", got)
		}
	})

	t.Run("it refuses --resume-mode with no --on-resume and writes nothing", func(t *testing.T) {
		_, hooksFile := hooksFileInTempDir(t, nil)
		t.Setenv("TMUX_PANE", "%7")

		// Plain seams, so a body that ran would resolve a key and write an
		// empty-command entry: the call counts below are what say it never ran.
		resolver := &mockKeyResolver{key: hookstest.SubjectSeedA}
		stamper := &recordingPaneStamper{}
		withHooksDeps(t, HooksDeps{KeyResolver: resolver, PaneStamper: stamper})

		buf := new(bytes.Buffer)
		resetRootCmd()
		rootCmd.SetOut(buf)
		rootCmd.SetErr(buf)
		rootCmd.SetArgs([]string{"hook", "set", "--resume-mode", "lazy"})
		err := rootCmd.Execute()
		if err == nil {
			t.Fatal("expected a non-zero exit for a mode passed with no command, got nil")
		}
		// The required-flag machinery refuses it before RunE, in cobra's own words.
		if got := err.Error(); got != `required flag(s) "on-resume" not set` {
			t.Errorf("error = %q, want the required-flag refusal", got)
		}

		assertNoPaneTmuxCalls(t, resolver, stamper)
		if _, err := os.Stat(hooksFile); err == nil {
			t.Error("hooks.json was written for a refused registration")
		}
	})

	t.Run("it refuses an unrecognised mode before any tmux read", func(t *testing.T) {
		dir, hooksFile := hooksFileInTempDir(t, map[string]map[string]string{
			hookstest.SubjectSeedA: {"on-resume": "seeded"},
		})
		before, err := os.ReadFile(hooksFile)
		if err != nil {
			t.Fatalf("read the seeded hooks file: %v", err)
		}
		stateDir := dir
		t.Setenv("PORTAL_STATE_DIR", stateDir)
		t.Setenv("TMUX_PANE", "%7")

		resolver := &mockKeyResolver{key: hookstest.SubjectSeedB}
		stamper := &recordingPaneStamper{}
		withHooksDeps(t, HooksDeps{KeyResolver: resolver, PaneStamper: stamper})

		out, err := runHookSet(t, "npm start", "--resume-mode", "eagre")
		if err == nil {
			t.Fatalf("expected a non-zero exit for an unrecognised mode, got nil (output %q)", out)
		}

		if resolver.calls != 0 {
			t.Errorf("hook-key read count = %d, want 0 — the refusal precedes every tmux read", resolver.calls)
		}
		if len(stamper.calls) != 0 {
			t.Errorf("set-option call count = %d, want 0: %+v", len(stamper.calls), stamper.calls)
		}
		assertHooksFileUnchanged(t, hooksFile, before)
		if saveRequestedExists(t, stateDir) {
			t.Error("save.requested was touched by a refused registration")
		}
	})

	t.Run("it refuses an explicitly empty mode", func(t *testing.T) {
		_, hooksFile := hooksFileInTempDir(t, nil)
		t.Setenv("TMUX_PANE", "%7")

		resolver := &mockKeyResolver{key: hookstest.SubjectSeedA}
		withHooksDeps(t, HooksDeps{KeyResolver: resolver})

		if _, err := runHookSet(t, "npm start", "--resume-mode", ""); err == nil {
			t.Fatal("expected a non-zero exit for an explicitly empty mode, got nil")
		}

		if resolver.calls != 0 {
			t.Errorf("hook-key read count = %d, want 0", resolver.calls)
		}
		if _, err := os.Stat(hooksFile); err == nil {
			t.Error("hooks.json was written for a refused registration")
		}
	})

	t.Run("it names both accepted values in the refusal", func(t *testing.T) {
		hooksFileInTempDir(t, nil)
		t.Setenv("TMUX_PANE", "%7")
		withHooksDeps(t, HooksDeps{KeyResolver: &mockKeyResolver{key: hookstest.SubjectSeedA}})

		_, err := runHookSet(t, "npm start", "--resume-mode", "nope")
		if err == nil {
			t.Fatal("expected a non-zero exit, got nil")
		}
		if got := err.Error(); got != `--resume-mode must be "eager" or "lazy"` {
			t.Errorf("error = %q, want the two accepted values named on one line", got)
		}
		// A malformed flag value exits 2 like every other one the CLI refuses.
		if _, ok := errors.AsType[*UsageError](err); !ok {
			t.Errorf("error type = %T, want *UsageError", err)
		}
	})

	t.Run("it still stamps a freshly minted token before writing a pinned registration", func(t *testing.T) {
		_, hooksFile := hooksFileInTempDir(t, nil)
		t.Setenv("TMUX_PANE", "%7")

		stamper := &recordingPaneStamper{}
		stamper.onCall = func() {
			if _, err := os.Stat(hooksFile); err == nil {
				t.Error("hooks.json already existed when the stamp ran: the write must not precede the stamp")
			}
		}
		withHooksDeps(t, HooksDeps{KeyResolver: &mockKeyResolver{key: ""}, PaneStamper: stamper})

		if _, err := runHookSet(t, "npm start", "--resume-mode", "lazy"); err != nil {
			t.Fatalf("hook set: %v", err)
		}

		if len(stamper.calls) != 1 {
			t.Fatalf("set-option call count = %d, want 1", len(stamper.calls))
		}
		if got := stamper.calls[0].name; got != state.PortalPaneIDOption {
			t.Errorf("stamp option = %q, want %q", got, state.PortalPaneIDOption)
		}
		token := stamper.calls[0].value
		entry := readHooksJSON(t, hooksFile)[token]["on-resume"]
		if entry.Command != "npm start" || entry.Resume != resumemode.Lazy {
			t.Errorf("entry under the stamped token %q = %+v, want the pinned registration", token, entry)
		}
	})
}
