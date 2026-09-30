package hooks_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/fileutil"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/logtest"
)

// decodeHooksFile parses the file at path into its raw per-key, per-event
// values, so an assertion can compare what an entry holds byte-for-byte.
func decodeHooksFile(t *testing.T, path string) map[string]map[string]json.RawMessage {
	t.Helper()
	var decoded map[string]map[string]json.RawMessage
	if err := json.Unmarshal(readFileBytes(t, path), &decoded); err != nil {
		t.Fatalf("decode hooks.json: %v", err)
	}
	return decoded
}

// compactJSON strips the indentation the store's save applies to the whole
// file, leaving an entry's own content and attribute order to compare.
func compactJSON(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		t.Fatalf("compact %s: %v", raw, err)
	}
	return buf.String()
}

// seededCommand is the command the fixtures register and the confirmation
// showed, so a discard handed it is one the user agreed to.
const seededCommand = "claude --resume abc123"

func assertDiscardRecord(t *testing.T, sink *logtest.Sink, key, command string) {
	t.Helper()
	rec := sink.Records().Only(t, "discard record")
	logtest.AssertRecord(t, rec, logtest.RecordWant{
		Level:     slog.LevelInfo,
		Msg:       "discard",
		Component: "hooks",
		Op:        "discard",
		Via:       "panel",
	})
	if got := rec.AttrString(t, "hook_key"); got != key {
		t.Errorf("hook_key = %q, want %q", got, key)
	}
	if got := rec.AttrString(t, "value"); got != command {
		t.Errorf("value = %q, want %q", got, command)
	}
}

func TestDiscard(t *testing.T) {
	t.Run("it removes the on-resume entry and records the command it destroyed", func(t *testing.T) {
		store, path := hookstest.StageStore(t, hookstest.Staging{
			Entries: map[string]string{hookstest.SubjectSeedA: "claude --resume abc123"},
		})
		sink := logtest.Install(t)

		removed, err := store.Discard(hookstest.SubjectSeedA, hooks.EventOnResume, seededCommand, hooks.ViaPanel)
		if err != nil {
			t.Fatalf("Discard: %v", err)
		}
		if !removed {
			t.Error("removed = false, want true for a seeded entry")
		}
		if _, ok := decodeHooksFile(t, path)[hookstest.SubjectSeedA]; ok {
			t.Error("the discarded key is still on disk")
		}
		assertDiscardRecord(t, sink, hookstest.SubjectSeedA, "claude --resume abc123")
	})

	t.Run("it keeps the key's other events and its key", func(t *testing.T) {
		store, path := hookstest.StageStore(t, hookstest.Staging{
			Body: map[string]map[string]string{
				hookstest.SubjectSeedA: {"on-resume": "claude --resume abc123", "on-start": "npm start"},
			},
		})

		removed, err := store.Discard(hookstest.SubjectSeedA, hooks.EventOnResume, seededCommand, hooks.ViaPanel)
		if err != nil {
			t.Fatalf("Discard: %v", err)
		}
		if !removed {
			t.Error("removed = false, want true for a seeded entry")
		}

		events, ok := decodeHooksFile(t, path)[hookstest.SubjectSeedA]
		if !ok {
			t.Fatal("the key was deleted although it still holds another event")
		}
		if _, ok := events["on-resume"]; ok {
			t.Error("on-resume is still on disk")
		}
		if got := string(events["on-start"]); got != `"npm start"` {
			t.Errorf("on-start = %s, want %q", got, `"npm start"`)
		}
	})

	t.Run("it deletes the key whole when on-resume was its last event", func(t *testing.T) {
		store, path := hookstest.StageStore(t, hookstest.Staging{
			Entries: map[string]string{
				hookstest.SubjectSeedA: "claude --resume abc123",
				hookstest.SubjectSeedB: "npm start",
			},
		})

		if _, err := store.Discard(hookstest.SubjectSeedA, hooks.EventOnResume, seededCommand, hooks.ViaPanel); err != nil {
			t.Fatalf("Discard: %v", err)
		}

		decoded := decodeHooksFile(t, path)
		if _, ok := decoded[hookstest.SubjectSeedA]; ok {
			t.Errorf("key survives with no events left: %v", decoded)
		}
		if len(decoded) != 1 {
			t.Errorf("hooks.json holds %d keys, want 1: %v", len(decoded), decoded)
		}
	})

	t.Run("it records the command out of an object-form entry", func(t *testing.T) {
		const command = `claude --resume "abc 123"`
		cases := []struct {
			name string
			seed string
		}{
			{"string form", `{"` + hookstest.SubjectSeedA + `":{"on-resume":"claude --resume \"abc 123\""}}`},
			{"object form carrying a mode", `{"` + hookstest.SubjectSeedA + `":{"on-resume":{"command":"claude --resume \"abc 123\"","resume":"eager"}}}`},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: tc.seed})
				sink := logtest.Install(t)

				removed, err := store.Discard(hookstest.SubjectSeedA, hooks.EventOnResume, command, hooks.ViaPanel)
				if err != nil {
					t.Fatalf("Discard: %v", err)
				}
				if !removed {
					t.Error("removed = false, want true for a seeded entry")
				}
				assertDiscardRecord(t, sink, hookstest.SubjectSeedA, command)
			})
		}
	})

	t.Run("it removes nothing and writes nothing for a key naming no entry", func(t *testing.T) {
		cases := []struct {
			name string
			key  string
		}{
			{"absent key", hookstest.SubjectSeedB},
			{"key with no on-resume event", hookstest.SubjectSeedA},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				store, path := hookstest.StageStore(t, hookstest.Staging{
					Body: map[string]map[string]string{hookstest.SubjectSeedA: {"on-start": "npm start"}},
				})
				before := readFileBytes(t, path)
				sink := logtest.Install(t)

				removed, err := store.Discard(tc.key, hooks.EventOnResume, seededCommand, hooks.ViaPanel)
				if err != nil {
					t.Fatalf("Discard: %v", err)
				}
				if removed {
					t.Error("removed = true, want false")
				}
				if recs := sink.Records(); len(recs) != 0 {
					t.Errorf("a discard that removed nothing emitted %d records, want 0: %+v", len(recs), recs)
				}
				hookstest.AssertHooksFileUnchanged(t, path, before, "changed on a discard that removed nothing")
			})
		}
	})

	t.Run("it reaches no mutation for an empty hook key", func(t *testing.T) {
		store, path := hookstest.StageStore(t, hookstest.Staging{
			Body:          map[string]map[string]string{"": {"on-resume": "claude --resume abc123"}},
			SidecarAbsent: true,
		})
		before := readFileBytes(t, path)
		sink := logtest.Install(t)

		removed, err := store.Discard("", hooks.EventOnResume, seededCommand, hooks.ViaPanel)
		if err != nil {
			t.Fatalf("Discard: %v", err)
		}
		if removed {
			t.Error("removed = true, want false for an empty key")
		}
		if _, statErr := os.Stat(hookstest.SidecarPath(path)); !errors.Is(statErr, os.ErrNotExist) {
			t.Errorf("sidecar created for an empty key: %v", statErr)
		}
		if recs := sink.Records(); len(recs) != 0 {
			t.Errorf("an empty key emitted %d records, want 0: %+v", len(recs), recs)
		}
		hookstest.AssertHooksFileUnchanged(t, path, before, "changed on an empty key")
	})

	t.Run("it reports no removal when the save fails", func(t *testing.T) {
		store, path := hookstest.StageStore(t, hookstest.Staging{
			Entries:      map[string]string{hookstest.SubjectSeedA: "claude --resume abc123"},
			WritesDenied: true,
		})
		before := readFileBytes(t, path)
		sink := logtest.Install(t)

		removed, err := store.Discard(hookstest.SubjectSeedA, hooks.EventOnResume, seededCommand, hooks.ViaPanel)
		if err == nil {
			t.Fatal("expected an error when the save fails, got nil")
		}
		if removed {
			t.Error("removed = true, want false when the save failed")
		}
		hookstest.AssertHooksFileUnchanged(t, path, before, "changed on a failed save")

		rec := sink.Records().Only(t, "log record")
		logtest.AssertRecord(t, rec, logtest.RecordWant{
			Level:     slog.LevelWarn,
			Msg:       "discard",
			Component: "hooks",
			Op:        "discard",
			Via:       "panel",
		})
		if got := rec.AttrString(t, "hook_key"); got != hookstest.SubjectSeedA {
			t.Errorf("hook_key = %q, want %q", got, hookstest.SubjectSeedA)
		}
		logtest.AssertWriteFailure(t, rec, "write-failed-temp-create", fileutil.ErrWriteTempCreate)
	})

	t.Run("it reports no removal when the mutation lock cannot be taken", func(t *testing.T) {
		hooks.SetLockTimeoutForTest(t, 40*time.Millisecond)
		store, path := hookstest.StageStore(t, hookstest.Staging{
			Entries: map[string]string{hookstest.SubjectSeedA: "claude --resume abc123"},
		})
		before := readFileBytes(t, path)
		hookstest.HoldHooksSidecar(t, path)
		sink := logtest.Install(t)

		removed, err := store.Discard(hookstest.SubjectSeedA, hooks.EventOnResume, seededCommand, hooks.ViaPanel)
		if !errors.Is(err, hooks.ErrLockHeld) {
			t.Errorf("err = %v, want errors.Is ErrLockHeld", err)
		}
		if removed {
			t.Error("removed = true, want false when the lock could not be taken")
		}
		hookstest.AssertHooksFileUnchanged(t, path, before, "changed on a timed-out Discard")
		hookstest.AssertLockWarn(t, sink, "discard", hookstest.SubjectSeedA, "panel")
	})

	t.Run("it keeps discard and rm greppable apart", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{
			Entries: map[string]string{
				hookstest.SubjectSeedA: "claude --resume abc123",
				hookstest.SubjectSeedB: "npm start",
			},
		})
		sink := logtest.Install(t)

		if _, err := store.Discard(hookstest.SubjectSeedA, hooks.EventOnResume, seededCommand, hooks.ViaPanel); err != nil {
			t.Fatalf("Discard: %v", err)
		}
		if _, err := store.Remove(hookstest.SubjectSeedB, hooks.EventOnResume, hooks.ViaCLI); err != nil {
			t.Fatalf("Remove: %v", err)
		}

		discard := sink.Records().WithMessage("discard").Only(t, "discard record")
		rm := sink.Records().WithMessage("rm").Only(t, "rm record")
		if got := discard.AttrString(t, "op"); got != "discard" {
			t.Errorf("discard op = %q, want discard", got)
		}
		if got := rm.AttrString(t, "op"); got != "rm" {
			t.Errorf("rm op = %q, want rm", got)
		}
		if got := discard.AttrString(t, "hook_key"); got != hookstest.SubjectSeedA {
			t.Errorf("discard hook_key = %q, want %q", got, hookstest.SubjectSeedA)
		}
		if got := rm.AttrString(t, "hook_key"); got != hookstest.SubjectSeedB {
			t.Errorf("rm hook_key = %q, want %q", got, hookstest.SubjectSeedB)
		}
		if rm.HasAttr("value") {
			t.Errorf("rm record carries value: %+v", rm.Attrs)
		}
	})

	t.Run("it leaves every other entry byte-unchanged", func(t *testing.T) {
		seed := `{"` + hookstest.SubjectSeedA + `":{"on-resume":"claude --resume abc123"},` +
			`"` + hookstest.SubjectSeedB + `":{"on-resume":{"command":"npm start","resume":"lazy","extra":[1, 2]}},` +
			`"` + hookstest.UnjudgeableSeedA + `":{"on-resume":"vim ."}}`
		store, path := hookstest.StageStore(t, hookstest.Staging{Seed: seed})
		before := decodeHooksFile(t, path)

		if _, err := store.Discard(hookstest.SubjectSeedA, hooks.EventOnResume, seededCommand, hooks.ViaPanel); err != nil {
			t.Fatalf("Discard: %v", err)
		}

		after := decodeHooksFile(t, path)
		if len(after) != 2 {
			t.Fatalf("hooks.json holds %d keys, want 2: %v", len(after), after)
		}
		for _, key := range []string{hookstest.SubjectSeedB, hookstest.UnjudgeableSeedA} {
			if got, want := compactJSON(t, after[key]["on-resume"]), compactJSON(t, before[key]["on-resume"]); got != want {
				t.Errorf("%s on-resume = %s, want %s", key, got, want)
			}
		}
	})
}

func TestDiscard_OnlyTheShownCommand(t *testing.T) {
	t.Run("it leaves an entry rewritten to another command in place", func(t *testing.T) {
		cases := []struct {
			name    string
			rewrite func(t *testing.T, store *hooks.Store, path string)
		}{
			{"re-registered", func(t *testing.T, store *hooks.Store, _ string) {
				if err := store.Set(hookstest.SubjectSeedA, hooks.EventOnResume,
					hooks.Registration{Command: "npm start"}, hooks.ViaCLI); err != nil {
					t.Fatalf("Set: %v", err)
				}
			}},
			{"hand-edited", func(t *testing.T, _ *hooks.Store, path string) {
				body := `{"` + hookstest.SubjectSeedA + `":{"on-resume":"npm start"}}`
				if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
					t.Fatalf("hand-edit hooks.json: %v", err)
				}
			}},
			{"differing only in whitespace", func(t *testing.T, store *hooks.Store, _ string) {
				if err := store.Set(hookstest.SubjectSeedA, hooks.EventOnResume,
					hooks.Registration{Command: seededCommand + " "}, hooks.ViaCLI); err != nil {
					t.Fatalf("Set: %v", err)
				}
			}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				store, path := hookstest.StageStore(t, hookstest.Staging{
					Entries: map[string]string{hookstest.SubjectSeedA: seededCommand},
				})
				tc.rewrite(t, store, path)
				before := readFileBytes(t, path)
				sink := logtest.Install(t)

				removed, err := store.Discard(hookstest.SubjectSeedA, hooks.EventOnResume, seededCommand, hooks.ViaPanel)
				if err != nil {
					t.Fatalf("Discard: %v", err)
				}
				if removed {
					t.Error("removed = true, want false for an entry holding another command")
				}
				if recs := sink.Records(); len(recs) != 0 {
					t.Errorf("a discard that removed nothing emitted %d records, want 0: %+v", len(recs), recs)
				}
				hookstest.AssertHooksFileUnchanged(t, path, before, "changed on a discard of a command it no longer holds")
			})
		}
	})

	t.Run("it removes an entry rewritten to the same command under another mode", func(t *testing.T) {
		store, path := hookstest.StageStore(t, hookstest.Staging{
			Seed: `{"` + hookstest.SubjectSeedA + `":{"on-resume":{"command":"` + seededCommand + `","resume":"eager"}}}`,
		})
		sink := logtest.Install(t)

		removed, err := store.Discard(hookstest.SubjectSeedA, hooks.EventOnResume, seededCommand, hooks.ViaPanel)
		if err != nil {
			t.Fatalf("Discard: %v", err)
		}
		if !removed {
			t.Error("removed = false, want true: only the command is compared")
		}
		if _, ok := decodeHooksFile(t, path)[hookstest.SubjectSeedA]; ok {
			t.Error("the discarded key is still on disk")
		}
		assertDiscardRecord(t, sink, hookstest.SubjectSeedA, seededCommand)
	})

	t.Run("it leaves Remove removing whatever command the key holds", func(t *testing.T) {
		store, path := hookstest.StageStore(t, hookstest.Staging{
			Entries: map[string]string{hookstest.SubjectSeedA: "npm start"},
		})

		removed, err := store.Remove(hookstest.SubjectSeedA, hooks.EventOnResume, hooks.ViaCLI)
		if err != nil {
			t.Fatalf("Remove: %v", err)
		}
		if !removed {
			t.Error("removed = false, want true")
		}
		if _, ok := decodeHooksFile(t, path)[hookstest.SubjectSeedA]; ok {
			t.Error("the removed key is still on disk")
		}
	})
}

func TestDiscard_Refusals(t *testing.T) {
	t.Run("it logs a load it could not complete and reports it as a read failure", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{Unreadable: true})
		sink := logtest.Install(t)

		removed, err := store.Discard(hookstest.SubjectSeedA, hooks.EventOnResume, seededCommand, hooks.ViaPanel)
		if removed {
			t.Error("removed = true, want false when the load failed")
		}
		if !errors.Is(err, hooks.ErrStoreRead) {
			t.Errorf("err = %v, want errors.Is ErrStoreRead", err)
		}

		rec := sink.Records().Only(t, "the failed load's record")
		logtest.AssertRecord(t, rec, logtest.RecordWant{
			Level:     slog.LevelWarn,
			Msg:       "discard",
			Component: "hooks",
			Op:        "discard",
			Via:       "panel",
		})
		if got := rec.AttrString(t, "hook_key"); got != hookstest.SubjectSeedA {
			t.Errorf("hook_key = %q, want %q", got, hookstest.SubjectSeedA)
		}
		if logged := rec.ErrorAttr(t, "error"); logged.Error() != err.Error() {
			t.Errorf("WARN error = %v, want the whole returned chain %v", logged, err)
		}
		if rec.HasAttr("error_class") {
			t.Error("WARN carries error_class, want none: no write phase ran")
		}
	})

	t.Run("it reports malformed JSON as a read failure the caller can name", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: "{not json"})
		sink := logtest.Install(t)

		_, err := store.Discard(hookstest.SubjectSeedA, hooks.EventOnResume, seededCommand, hooks.ViaPanel)
		if !errors.Is(err, hooks.ErrMalformed) || !errors.Is(err, hooks.ErrStoreRead) {
			t.Errorf("err = %v, want errors.Is both ErrMalformed and ErrStoreRead", err)
		}
		sink.Records().AtExactLevel(slog.LevelWarn).Matching("hooks", "discard").Only(t, "the malformed load's record")
	})

	t.Run("it reports a lock it could not take for want of more than time apart from a held one", func(t *testing.T) {
		store, path := hookstest.StageStore(t, hookstest.Staging{
			Entries: map[string]string{hookstest.SubjectSeedA: "claude --resume abc123"},
		})
		if err := os.Chmod(hookstest.SidecarPath(path), 0o000); err != nil {
			t.Fatalf("chmod sidecar: %v", err)
		}
		t.Cleanup(func() { _ = os.Chmod(hookstest.SidecarPath(path), 0o600) })
		sink := logtest.Install(t)

		_, err := store.Discard(hookstest.SubjectSeedA, hooks.EventOnResume, seededCommand, hooks.ViaPanel)
		if !errors.Is(err, hooks.ErrLockFailed) {
			t.Errorf("err = %v, want errors.Is ErrLockFailed", err)
		}
		if errors.Is(err, hooks.ErrLockHeld) {
			t.Errorf("err = %v, want no ErrLockHeld: nothing held the lock", err)
		}
		hookstest.AssertLockWarn(t, sink, "discard", hookstest.SubjectSeedA, "panel")
	})
}
