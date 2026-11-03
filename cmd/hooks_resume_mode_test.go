package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/resumemode"
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
				inSession(t, hookSessionA)

				if _, err := runHookSet(t, "npm start", "--resume-mode", tt.name); err != nil {
					t.Fatalf("hook set: %v", err)
				}

				entry := readHooksJSON(t, hooksFile)[hookSessionA]["on-resume"]
				if entry.Command != "npm start" {
					t.Errorf("command = %q, want %q", entry.Command, "npm start")
				}
				if entry.Resume != tt.want {
					t.Errorf("resume = %q, want %q", entry.Resume, tt.want)
				}

				want := `{"command":"npm start","resume":"` + tt.name + `"}`
				if got := rawRegistration(t, hooksFile, hookSessionA, "on-resume"); got != want {
					t.Errorf("stored registration = %s, want %s", got, want)
				}
			})
		}
	})

	t.Run("it writes a registration carrying no mode when the flag is not passed", func(t *testing.T) {
		_, hooksFile := hooksFileInTempDir(t, nil)
		inSession(t, hookSessionA)

		if _, err := runHookSet(t, "npm start"); err != nil {
			t.Fatalf("hook set: %v", err)
		}

		if got := readHooksJSON(t, hooksFile)[hookSessionA]["on-resume"].Resume; got != resumemode.Unset {
			t.Errorf("resume = %q, want no mode", got)
		}
		if got := rawRegistration(t, hooksFile, hookSessionA, "on-resume"); got != `"npm start"` {
			t.Errorf("stored registration = %s, want the string form", got)
		}
	})

	t.Run("it drops an object-form predecessor's mode when the flag is not passed", func(t *testing.T) {
		_, hooksFile := hookstest.StageStore(t, hookstest.Staging{
			Seed: `{"` + hookSessionA + `":{"on-resume":{"command":"old-cmd","resume":"lazy","note":"kept"}}}`,
		})
		t.Setenv("PORTAL_HOOKS_FILE", hooksFile)
		inSession(t, hookSessionA)

		if _, err := runHookSet(t, "new-cmd"); err != nil {
			t.Fatalf("hook set: %v", err)
		}

		if got := rawRegistration(t, hooksFile, hookSessionA, "on-resume"); got != `"new-cmd"` {
			t.Errorf("stored registration = %s, want the string form carrying neither the old mode nor its unmodelled attribute", got)
		}
	})

	t.Run("it refuses --resume-mode with no --on-resume and writes nothing", func(t *testing.T) {
		_, hooksFile := hooksFileInTempDir(t, nil)
		// In a session, so a body that ran would write an empty-command entry.
		inSession(t, hookSessionA)

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

		if _, err := os.Stat(hooksFile); err == nil {
			t.Error("hooks.json was written for a refused registration")
		}
	})

	t.Run("it refuses an unrecognised mode and leaves hooks.json byte-identical", func(t *testing.T) {
		_, hooksFile := hooksFileInTempDir(t, map[string]map[string]string{
			hookSessionA: {"on-resume": "seeded"},
		})
		before := readFileBytes(t, hooksFile)
		inSession(t, hookSessionA)

		out, err := runHookSet(t, "npm start", "--resume-mode", "eagre")
		if err == nil {
			t.Fatalf("expected a non-zero exit for an unrecognised mode, got nil (output %q)", out)
		}

		assertHooksFileUnchanged(t, hooksFile, before)
	})

	t.Run("it refuses an explicitly empty mode", func(t *testing.T) {
		_, hooksFile := hooksFileInTempDir(t, nil)
		inSession(t, hookSessionA)

		if _, err := runHookSet(t, "npm start", "--resume-mode", ""); err == nil {
			t.Fatal("expected a non-zero exit for an explicitly empty mode, got nil")
		}

		if _, err := os.Stat(hooksFile); err == nil {
			t.Error("hooks.json was written for a refused registration")
		}
	})

	t.Run("it names both accepted values in the refusal", func(t *testing.T) {
		hooksFileInTempDir(t, nil)
		inSession(t, hookSessionA)

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
}
