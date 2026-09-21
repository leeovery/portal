package hooks_test

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"testing"

	"github.com/leeovery/portal/internal/fileutil"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/resumemode"
)

func TestSetWritesTheRegistrationWhole(t *testing.T) {
	t.Run("it writes the object form for a registration carrying a mode", func(t *testing.T) {
		store, path := hookstest.StageStore(t, hookstest.Staging{})

		registration := hooks.Registration{Command: "npm start", Resume: resumemode.Lazy}
		if err := store.Set(hookstest.LiveSeedA, hooks.EventOnResume, registration, hooks.ViaCLI); err != nil {
			t.Fatalf("Set: %v", err)
		}

		want := `{"command":"npm start","resume":"lazy"}`
		if got := storedValue(t, path, hookstest.LiveSeedA, "on-resume"); string(got) != want {
			t.Errorf("stored value = %s, want %s", got, want)
		}
	})

	t.Run("it writes the string form for a registration carrying no mode", func(t *testing.T) {
		store, path := hookstest.StageStore(t, hookstest.Staging{})

		registration := hooks.Registration{Command: "npm start"}
		if err := store.Set(hookstest.LiveSeedA, hooks.EventOnResume, registration, hooks.ViaCLI); err != nil {
			t.Fatalf("Set: %v", err)
		}

		if got := storedValue(t, path, hookstest.LiveSeedA, "on-resume"); string(got) != `"npm start"` {
			t.Errorf("stored value = %s, want %s", got, `"npm start"`)
		}
	})

	t.Run("it treats an identical object-form rewrite as a no-op and leaves the file untouched", func(t *testing.T) {
		store, path := hookstest.StageStore(t, hookstest.Staging{
			Seed: fmt.Sprintf(`{%q:{"on-resume":{"command":"npm start","resume":"lazy"}}}`, hookstest.LiveSeedA),
		})
		before := readFileBytes(t, path)
		beforeModTime := modTime(t, path)

		sink := logtest.Install(t)
		registration := hooks.Registration{Command: "npm start", Resume: resumemode.Lazy}
		if err := store.Set(hookstest.LiveSeedA, hooks.EventOnResume, registration, hooks.ViaCLI); err != nil {
			t.Fatalf("Set: %v", err)
		}

		rec := sink.Records().Only(t, "log record")
		logtest.AssertRecord(t, rec, logtest.RecordWant{
			Level:     slog.LevelDebug,
			Msg:       "set-noop",
			Component: "hooks",
			Op:        "set-noop",
			Via:       "cli",
		})

		if after := readFileBytes(t, path); !bytes.Equal(after, before) {
			t.Errorf("file = %s, want it byte-unchanged: %s", after, before)
		}
		if after := modTime(t, path); !after.Equal(beforeModTime) {
			t.Error("file was rewritten on an identical object-form rewrite")
		}
	})

	t.Run("it treats a mode-only change as a modify", func(t *testing.T) {
		tests := []struct {
			name         string
			seeded       string
			registration hooks.Registration
			want         string
		}{
			{
				name:         "none to lazy",
				seeded:       `"npm start"`,
				registration: hooks.Registration{Command: "npm start", Resume: resumemode.Lazy},
				want:         `{"command":"npm start","resume":"lazy"}`,
			},
			{
				name:         "eager to none",
				seeded:       `{"command":"npm start","resume":"eager"}`,
				registration: hooks.Registration{Command: "npm start"},
				want:         `"npm start"`,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				store, path := hookstest.StageStore(t, hookstest.Staging{
					Seed: fmt.Sprintf(`{%q:{"on-resume":%s}}`, hookstest.LiveSeedA, tt.seeded),
				})

				sink := logtest.Install(t)
				if err := store.Set(hookstest.LiveSeedA, hooks.EventOnResume, tt.registration, hooks.ViaCLI); err != nil {
					t.Fatalf("Set: %v", err)
				}

				rec := sink.Records().Only(t, "log record")
				logtest.AssertRecord(t, rec, logtest.RecordWant{
					Level:     slog.LevelInfo,
					Msg:       "modify",
					Component: "hooks",
					Op:        "modify",
					Via:       "cli",
				})

				if got := storedValue(t, path, hookstest.LiveSeedA, "on-resume"); string(got) != tt.want {
					t.Errorf("stored value = %s, want %s", got, tt.want)
				}
			})
		}
	})

	t.Run("it drops an object-form predecessor's unmodelled attributes when the rewrite carries no mode", func(t *testing.T) {
		store, path := hookstest.StageStore(t, hookstest.Staging{
			Seed: fmt.Sprintf(`{%q:{"on-resume":{"command":"npm start","resume":"lazy","nickname":"dev server"}}}`,
				hookstest.LiveSeedA),
		})

		registration := hooks.Registration{Command: "npm start"}
		if err := store.Set(hookstest.LiveSeedA, hooks.EventOnResume, registration, hooks.ViaCLI); err != nil {
			t.Fatalf("Set: %v", err)
		}

		if got := storedValue(t, path, hookstest.LiveSeedA, "on-resume"); string(got) != `"npm start"` {
			t.Errorf("stored value = %s, want %s", got, `"npm start"`)
		}
	})

	t.Run("it leaves every other entry as it found it when writing one", func(t *testing.T) {
		store, path := hookstest.StageStore(t, hookstest.Staging{
			Seed: fmt.Sprintf(`{%q:{"on-resume":{"command":"npm start","nickname":"dev server"}},%q:{"on-resume":"old"}}`,
				hookstest.LiveSeedA, hookstest.LiveSeedB),
		})
		sibling := storedValue(t, path, hookstest.LiveSeedA, "on-resume")

		registration := hooks.Registration{Command: "new", Resume: resumemode.Eager}
		if err := store.Set(hookstest.LiveSeedB, hooks.EventOnResume, registration, hooks.ViaCLI); err != nil {
			t.Fatalf("Set: %v", err)
		}

		if after := storedValue(t, path, hookstest.LiveSeedA, "on-resume"); !bytes.Equal(after, sibling) {
			t.Errorf("untouched entry = %s, want %s", after, sibling)
		}
	})

	t.Run("it carries the command as the value attr out of an object-form write", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{})
		sink := logtest.Install(t)

		registration := hooks.Registration{Command: "npm start", Resume: resumemode.Lazy}
		if err := store.Set(hookstest.LiveSeedA, hooks.EventOnResume, registration, hooks.ViaCLI); err != nil {
			t.Fatalf("Set: %v", err)
		}

		rec := sink.Records().Only(t, "log record")
		logtest.AssertRecord(t, rec, logtest.RecordWant{
			Level:     slog.LevelInfo,
			Msg:       "set",
			Component: "hooks",
			Op:        "set",
			Via:       "cli",
		})
		if got := rec.AttrString(t, "hook_key"); got != hookstest.LiveSeedA {
			t.Errorf("hook_key = %q, want %q", got, hookstest.LiveSeedA)
		}
		if got := rec.AttrString(t, "value"); got != "npm start" {
			t.Errorf("value = %q, want %q", got, "npm start")
		}
	})

	t.Run("it reports a failed save with its error class and writes nothing", func(t *testing.T) {
		store, path := hookstest.StageStore(t, hookstest.Staging{WritesDenied: true})
		sink := logtest.Install(t)

		registration := hooks.Registration{Command: "npm start", Resume: resumemode.Lazy}
		err := store.Set(hookstest.LiveSeedA, hooks.EventOnResume, registration, hooks.ViaCLI)
		if !errors.Is(err, fileutil.ErrWriteTempCreate) {
			t.Fatalf("err = %v, want errors.Is fileutil.ErrWriteTempCreate", err)
		}

		rec := sink.Records().Only(t, "log record")
		logtest.AssertRecord(t, rec, logtest.RecordWant{
			Level:     slog.LevelWarn,
			Msg:       "set",
			Component: "hooks",
			Op:        "set",
			Via:       "cli",
		})
		logtest.AssertWriteFailure(t, rec, "write-failed-temp-create", fileutil.ErrWriteTempCreate)

		if got := hookstest.HooksFileBytes(t, path); got != nil {
			t.Errorf("hooks.json = %s, want no file written", got)
		}
	})
}
