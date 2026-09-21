// White-box: the seeding and written-file helpers the rest of the write-path
// suites share are unexported.
package prefs

import (
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/leeovery/portal/internal/resumemode"
	"github.com/leeovery/portal/internal/themetest"
)

func TestLoadResumeMode_ReadsThePersistedKey(t *testing.T) {
	t.Run("it reads eager and lazy from the persisted key", func(t *testing.T) {
		cases := []struct {
			name    string
			content string
			want    resumemode.Mode
		}{
			{name: "eager", content: `{"resume_mode":"eager"}`, want: resumemode.Eager},
			{name: "lazy", content: `{"resume_mode":"lazy"}`, want: resumemode.Lazy},
			{name: "eager beside the other keys", content: `{"session_list_mode":"by-tag","theme":"nord","resume_mode":"eager"}`, want: resumemode.Eager},
		}

		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				got, err := NewStore(seedPrefsFile(t, c.content)).LoadResumeMode()
				if err != nil {
					t.Fatalf("unexpected LoadResumeMode error: %v", err)
				}
				if got != c.want {
					t.Errorf("resume mode = %q, want %q", got, c.want)
				}
			})
		}
	})
}

func TestLoadResumeMode_FallsBackToTheShippedDefault(t *testing.T) {
	t.Run("it answers lazy when the key is missing, empty, unrecognised or wrong-typed", func(t *testing.T) {
		cases := []struct {
			name    string
			content string
		}{
			{name: "the key is missing", content: `{"session_list_mode":"by-tag"}`},
			{name: "an empty object", content: `{}`},
			{name: "an empty value", content: `{"resume_mode":""}`},
			{name: "wrong case", content: `{"resume_mode":"LAZY"}`},
			{name: "a leading space", content: `{"resume_mode":" lazy"}`},
			{name: "an unrecognised word", content: `{"resume_mode":"off"}`},
			{name: "a number", content: `{"resume_mode":7}`},
			{name: "a boolean", content: `{"resume_mode":true}`},
			{name: "null", content: `{"resume_mode":null}`},
			{name: "an array", content: `{"resume_mode":["eager"]}`},
			{name: "an object", content: `{"resume_mode":{"mode":"eager"}}`},
		}

		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				got, err := NewStore(seedPrefsFile(t, c.content)).LoadResumeMode()
				if err != nil {
					t.Fatalf("unexpected LoadResumeMode error: %v", err)
				}
				if got != resumemode.Default {
					t.Errorf("resume mode = %q, want the shipped default %q", got, resumemode.Default)
				}
			})
		}
	})

	t.Run("it answers lazy for an absent prefs.json and for a corrupt one", func(t *testing.T) {
		for _, c := range undecodablePrefsCases() {
			t.Run(c.name, func(t *testing.T) {
				got, err := NewStore(seedPrefsFile(t, c.content)).LoadResumeMode()
				if err != nil {
					t.Fatalf("unexpected LoadResumeMode error: %v — a corrupt file is absorbed, not reported", err)
				}
				if got != resumemode.Default {
					t.Errorf("resume mode = %q, want the shipped default %q", got, resumemode.Default)
				}
			})
		}

		for _, c := range absentPathCases() {
			t.Run(c.name, func(t *testing.T) {
				got, err := NewStore(c.path(t.TempDir())).LoadResumeMode()
				if err != nil {
					t.Fatalf("unexpected LoadResumeMode error: %v — an absent file is the normal first-run state", err)
				}
				if got != resumemode.Default {
					t.Errorf("resume mode = %q, want the shipped default %q", got, resumemode.Default)
				}
			})
		}
	})

	t.Run("it answers lazy for a prefs.json that cannot be read at all", func(t *testing.T) {
		path := seedPrefsFile(t, `{"resume_mode":"eager"}`)
		_ = themetest.DenyRead(t, path)

		got, err := NewStore(path).LoadResumeMode()
		if !errors.Is(err, os.ErrPermission) {
			t.Fatalf("LoadResumeMode error = %v, want the OS permission error propagated", err)
		}
		if got != resumemode.Default {
			t.Errorf("resume mode = %q, want the shipped default %q beside the error", got, resumemode.Default)
		}
	})
}

func TestLoadResumeMode_WrongTypeDoesNotZeroTheRecord(t *testing.T) {
	t.Run("it keeps the rest of the record when resume_mode is wrong-typed", func(t *testing.T) {
		store := NewStore(seedPrefsFile(t, `{"session_list_mode":"by-tag","theme":"nord","theme_light":"tokyo-night-day","theme_dark":"tokyo-night","resume_mode":7}`))

		mode, err := store.Load()
		if err != nil {
			t.Fatalf("unexpected Load error: %v", err)
		}
		if mode != ModeByTag {
			t.Errorf("mode = %v, want ModeByTag — the record was zeroed by resume_mode", mode)
		}

		keys, err := store.LoadThemeKeys()
		if err != nil {
			t.Fatalf("unexpected LoadThemeKeys error: %v", err)
		}
		want := ThemeKeys{Theme: "nord", Light: "tokyo-night-day", Dark: "tokyo-night"}
		if keys != want {
			t.Errorf("theme keys = %+v, want %+v — the record was zeroed by resume_mode", keys, want)
		}
	})
}

func TestResumeMode_RoundTripsThroughEveryWriter(t *testing.T) {
	t.Run("it preserves a hand-set resume_mode across a grouping-mode toggle", func(t *testing.T) {
		// The unrecognised value is the load-bearing case: the key is preserved
		// verbatim, never normalised through the recogniser.
		for _, seeded := range []string{"eager", "lazy", "off"} {
			t.Run(seeded, func(t *testing.T) {
				path := seedPrefsFile(t, `{"session_list_mode":"flat","resume_mode":"`+seeded+`"}`)

				if err := NewStore(path).Save(ModeByTag); err != nil {
					t.Fatalf("unexpected Save error: %v", err)
				}

				decoded := decodeWritten(t, path)
				assertWrittenValue(t, decoded, "session_list_mode", "by-tag")
				assertWrittenValue(t, decoded, "resume_mode", seeded)
			})
		}
	})

	t.Run("it drops a wrong-typed resume_mode and keeps its neighbours", func(t *testing.T) {
		path := seedPrefsFile(t, `{"session_list_mode":"flat","theme":"nord","resume_mode":7}`)

		if err := NewStore(path).Save(ModeByTag); err != nil {
			t.Fatalf("unexpected Save error: %v", err)
		}

		decoded := decodeWritten(t, path)
		assertWrittenValue(t, decoded, "session_list_mode", "by-tag")
		assertWrittenValue(t, decoded, "theme", "nord")
		assertKeysAbsent(t, decoded, "resume_mode")
	})

	t.Run("it preserves a hand-set resume_mode across a theme commit", func(t *testing.T) {
		for _, c := range themeSaverCases() {
			t.Run(c.name, func(t *testing.T) {
				path := seedPrefsFile(t, `{"session_list_mode":"flat","resume_mode":"eager"}`)

				if err := c.save(NewStore(path), "nord"); err != nil {
					t.Fatalf("unexpected save error: %v", err)
				}

				decoded := decodeWritten(t, path)
				assertWrittenValue(t, decoded, c.writtenKey, "nord")
				assertWrittenValue(t, decoded, "resume_mode", "eager")
			})
		}
	})

	t.Run("it omits resume_mode from a file that never set it", func(t *testing.T) {
		for _, c := range markerWriterCases() {
			t.Run(c.name, func(t *testing.T) {
				path := seedPrefsFile(t, `{"session_list_mode":"flat"}`)

				if err := c.write(NewStore(path)); err != nil {
					t.Fatalf("unexpected write error: %v", err)
				}

				assertKeysAbsent(t, decodeWritten(t, path), "resume_mode")
			})
		}
	})

	// The field's own decoder absorbs any JSON value; a malformed file must
	// still abort the write rather than merge over it.
	t.Run("it still aborts a write on a malformed prefs.json", func(t *testing.T) {
		cases := []struct {
			name    string
			content string
		}{
			{name: "an unterminated object carrying resume_mode", content: `{"resume_mode":"eager"`},
			{name: "a trailing comma after resume_mode", content: `{"resume_mode":"eager",}`},
			{name: "a wrong-typed resume_mode in an unterminated object", content: `{"resume_mode":7`},
		}

		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				path := seedPrefsFile(t, c.content)
				before := readRaw(t, path)

				err := NewStore(path).Save(ModeByTag)
				if err == nil {
					t.Fatalf("Save returned nil, want an abort error")
				}
				if _, ok := errors.AsType[*json.SyntaxError](err); !ok {
					t.Errorf("error = %v (%T), want the decoder's *json.SyntaxError", err, err)
				}

				assertUntouched(t, path, before)
			})
		}
	})
}
