package hooks_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/resumemode"
)

// storedValue returns the compacted JSON an event's value holds on disk, so a
// preservation assertion compares the bytes the file carries rather than a
// re-decoded view of them.
func storedValue(t *testing.T, path, key, event string) []byte {
	t.Helper()
	var file map[string]map[string]json.RawMessage
	if err := json.Unmarshal(readFileBytes(t, path), &file); err != nil {
		t.Fatalf("failed to parse %s: %v", path, err)
	}
	events, ok := file[key]
	if !ok {
		t.Fatalf("%s holds no entry for key %q", path, key)
	}
	value, ok := events[event]
	if !ok {
		t.Fatalf("%s holds no %q event for key %q", path, event, key)
	}
	var compacted bytes.Buffer
	if err := json.Compact(&compacted, value); err != nil {
		t.Fatalf("failed to compact the stored value: %v", err)
	}
	return compacted.Bytes()
}

func TestRegistrationDecoding(t *testing.T) {
	t.Run("it loads a string value as a command carrying no mode", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{
			Seed: fmt.Sprintf(`{%q:{"on-resume":"npm start"}}`, hookstest.LiveSeedA),
		})

		h, err := store.Load(hooks.ViaInternal)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}

		got := h[hookstest.LiveSeedA]["on-resume"]
		if got.Command != "npm start" {
			t.Errorf("Command = %q, want %q", got.Command, "npm start")
		}
		if got.Resume != resumemode.Unset {
			t.Errorf("Resume = %q, want unset", got.Resume)
		}
	})

	t.Run("it loads an object value as its command and its mode", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{
			Seed: fmt.Sprintf(`{%q:{"on-resume":{"command":"npm start","resume":"eager"}}}`, hookstest.LiveSeedA),
		})

		h, err := store.Load(hooks.ViaInternal)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}

		got := h[hookstest.LiveSeedA]["on-resume"]
		if got.Command != "npm start" {
			t.Errorf("Command = %q, want %q", got.Command, "npm start")
		}
		if got.Resume != resumemode.Eager {
			t.Errorf("Resume = %q, want %q", got.Resume, resumemode.Eager)
		}
	})

	t.Run("it loads the remaining entries when one value is neither a string nor an object", func(t *testing.T) {
		tests := []struct {
			name  string
			value string
		}{
			{name: "number", value: "42"},
			{name: "array", value: `["x"]`},
			{name: "boolean", value: "true"},
			{name: "null", value: "null"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				store, _ := hookstest.StageStore(t, hookstest.Staging{
					Seed: fmt.Sprintf(`{%q:{"on-resume":%s},%q:{"on-resume":"npm start"}}`,
						hookstest.LiveSeedA, tt.value, hookstest.LiveSeedB),
				})

				h, err := store.Load(hooks.ViaInternal)
				if err != nil {
					t.Fatalf("Load: %v", err)
				}

				if got := h[hookstest.LiveSeedB]["on-resume"].Command; got != "npm start" {
					t.Errorf("neighbour Command = %q, want %q", got, "npm start")
				}
				unmodelled := h[hookstest.LiveSeedA]["on-resume"]
				if unmodelled.Command != "" {
					t.Errorf("Command = %q, want empty", unmodelled.Command)
				}
				if unmodelled.Resume != resumemode.Unset {
					t.Errorf("Resume = %q, want unset", unmodelled.Resume)
				}
			})
		}
	})
}

func TestRegistrationEncoding(t *testing.T) {
	t.Run("it marshals a registration carrying no mode as a plain string", func(t *testing.T) {
		data, err := json.Marshal(hooks.Registration{Command: "npm start"})
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		if got, want := string(data), `"npm start"`; got != want {
			t.Errorf("marshalled = %s, want %s", got, want)
		}
	})

	t.Run("it marshals a registration carrying a mode as an object", func(t *testing.T) {
		data, err := json.Marshal(hooks.Registration{Command: "npm start", Resume: resumemode.Lazy})
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		if got, want := string(data), `{"command":"npm start","resume":"lazy"}`; got != want {
			t.Errorf("marshalled = %s, want %s", got, want)
		}
	})
}

func TestRegistrationPreservationThroughASiblingRewrite(t *testing.T) {
	preserved := func(t *testing.T, value string) {
		t.Helper()
		store, path := hookstest.StageStore(t, hookstest.Staging{
			Seed: fmt.Sprintf(`{%q:{"on-resume":%s},%q:{"on-resume":"old"}}`,
				hookstest.LiveSeedA, value, hookstest.LiveSeedB),
		})

		before := storedValue(t, path, hookstest.LiveSeedA, "on-resume")

		if err := store.Set(hookstest.LiveSeedB, hooks.EventOnResume, "new", hooks.ViaInternal); err != nil {
			t.Fatalf("Set: %v", err)
		}

		if got := storedValue(t, path, hookstest.LiveSeedB, "on-resume"); string(got) != `"new"` {
			t.Errorf("rewritten entry = %s, want %s", got, `"new"`)
		}
		if after := storedValue(t, path, hookstest.LiveSeedA, "on-resume"); !bytes.Equal(after, before) {
			t.Errorf("untouched entry = %s, want %s", after, before)
		}
	}

	t.Run("it preserves an unmodelled attribute on an entry a rewrite did not name", func(t *testing.T) {
		preserved(t, `{"command":"npm start","nickname":"dev server","resume":"lazy"}`)
	})

	t.Run("it preserves an unrecognised resume value on an entry a rewrite did not name", func(t *testing.T) {
		preserved(t, `{"command":"npm start","resume":"LAZY"}`)
	})

	t.Run("it round-trips a value it cannot model through a rewrite of its sibling", func(t *testing.T) {
		for _, value := range []string{"42", `["x"]`, "true", "null"} {
			t.Run(value, func(t *testing.T) { preserved(t, value) })
		}
	})
}

func TestRegistrationRewriteClassification(t *testing.T) {
	t.Run("it treats a bare rewrite of a mode-carrying entry as a modify", func(t *testing.T) {
		store, path := hookstest.StageStore(t, hookstest.Staging{
			Seed: fmt.Sprintf(`{%q:{"on-resume":{"command":"npm start","resume":"eager"}},%q:{"on-resume":"other"}}`,
				hookstest.LiveSeedA, hookstest.LiveSeedB),
		})

		sibling := storedValue(t, path, hookstest.LiveSeedB, "on-resume")

		sink := logtest.Install(t)
		if err := store.Set(hookstest.LiveSeedA, hooks.EventOnResume, "npm start", hooks.ViaInternal); err != nil {
			t.Fatalf("Set: %v", err)
		}

		if op := sink.Records().Matching("hooks", "modify").Only(t, "modify record").AttrString(t, "op"); op != "modify" {
			t.Errorf("op = %q, want %q", op, "modify")
		}
		if got := storedValue(t, path, hookstest.LiveSeedA, "on-resume"); string(got) != `"npm start"` {
			t.Errorf("rewritten entry = %s, want %s", got, `"npm start"`)
		}
		if after := storedValue(t, path, hookstest.LiveSeedB, "on-resume"); !bytes.Equal(after, sibling) {
			t.Errorf("untouched entry = %s, want %s", after, sibling)
		}
	})
}
