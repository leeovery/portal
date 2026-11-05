package hooks_test

import (
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"slices"
	"testing"

	"github.com/leeovery/portal/internal/fileutil"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/logtest"
)

// rekeySeed holds the tmux version's two key shapes, one in the string form
// and one in the object form carrying an attribute the codec doesn't model and
// a second event, beside a third tmux-era key and a key no rekey names.
var rekeySeed = fmt.Sprintf(`{
  %q: {"on-resume": "claude --resume token"},
  %q: {"on-resume": {"command": "claude --resume coords", "resume": "eager", "nickname": "kept"}, "on-other": "x"},
  %q: {"on-resume": "claude --resume gone"},
  %q: {"on-resume": "untouched"}
}`, hookstest.TmuxEraSeedA, hookstest.TmuxEraSeedB, hookstest.TmuxEraSeedC, hookstest.LiveSeedA)

func TestRekey(t *testing.T) {
	moves := map[string]string{hookstest.TmuxEraSeedA: hookstest.SubjectSeedA, hookstest.TmuxEraSeedB: hookstest.SubjectSeedB}
	remove := []string{hookstest.TmuxEraSeedC}

	t.Run("it moves every event of each key to its new key, each value as the file held it, and removes the rest", func(t *testing.T) {
		store, path := hookstest.StageStore(t, hookstest.Staging{Seed: rekeySeed})
		before := decodeHooksFile(t, path)

		if err := store.Rekey(moves, remove, hooks.ViaCLI); err != nil {
			t.Fatalf("Rekey: %v", err)
		}

		after := decodeHooksFile(t, path)
		if got := slices.Sorted(maps.Keys(after)); !slices.Equal(got, []string{hookstest.LiveSeedA, hookstest.SubjectSeedA, hookstest.SubjectSeedB}) {
			t.Fatalf("keys = %v, want the two new keys and the one no rekey names", got)
		}
		for from, to := range map[string]string{hookstest.TmuxEraSeedA: hookstest.SubjectSeedA, hookstest.TmuxEraSeedB: hookstest.SubjectSeedB, hookstest.LiveSeedA: hookstest.LiveSeedA} {
			if len(after[to]) != len(before[from]) {
				t.Errorf("%s holds %d events, want the %d %s held", to, len(after[to]), len(before[from]), from)
			}
			for event, value := range before[from] {
				if got, want := compactJSON(t, after[to][event]), compactJSON(t, value); got != want {
					t.Errorf("%s %s = %s, want %s as %s held it", to, event, got, want, from)
				}
			}
		}
	})

	t.Run("a rekey run again changes nothing and writes no file", func(t *testing.T) {
		store, path := hookstest.StageStore(t, hookstest.Staging{Seed: rekeySeed})
		if err := store.Rekey(moves, remove, hooks.ViaCLI); err != nil {
			t.Fatalf("Rekey: %v", err)
		}
		before, written := readFileBytes(t, path), modTime(t, path)
		sink := logtest.Install(t)

		if err := store.Rekey(moves, remove, hooks.ViaCLI); err != nil {
			t.Fatalf("Rekey again: %v", err)
		}
		hookstest.AssertHooksFileUnchanged(t, path, before, "changed by a rekey run again")
		if !modTime(t, path).Equal(written) {
			t.Error("hooks.json rewritten by a rekey that changed nothing")
		}
		sink.Records().Matching("hooks", "rekey-noop").AtExactLevel(slog.LevelDebug).Only(t, "rekey-noop")
	})

	t.Run("a key the file doesn't hold is passed over", func(t *testing.T) {
		store, path := hookstest.StageStore(t, hookstest.Staging{Seed: rekeySeed})

		if err := store.Rekey(map[string]string{hookstest.TmuxEraSeedA: hookstest.SubjectSeedA, hookstest.ReapableSeedA: hookstest.SubjectSeedC}, []string{hookstest.ReapableSeedB}, hooks.ViaCLI); err != nil {
			t.Fatalf("Rekey: %v", err)
		}
		after := decodeHooksFile(t, path)
		if _, made := after[hookstest.SubjectSeedC]; made {
			t.Errorf("a key made for %s, which the file never held", hookstest.ReapableSeedA)
		}
		if _, moved := after[hookstest.SubjectSeedA]; !moved {
			t.Errorf("keys = %v, want %s moved all the same", slices.Sorted(maps.Keys(after)), hookstest.TmuxEraSeedA)
		}
	})

	t.Run("a move onto a key the file keeps is refused, writing nothing", func(t *testing.T) {
		store, path := hookstest.StageStore(t, hookstest.Staging{Seed: rekeySeed})
		before := readFileBytes(t, path)
		sink := logtest.Install(t)

		err := store.Rekey(map[string]string{hookstest.TmuxEraSeedA: hookstest.SubjectSeedA, hookstest.TmuxEraSeedB: hookstest.LiveSeedA}, remove, hooks.ViaCLI)
		if !errors.Is(err, hooks.ErrKeyHeld) {
			t.Fatalf("Rekey = %v, want ErrKeyHeld", err)
		}
		hookstest.AssertHooksFileUnchanged(t, path, before, "changed by a refused rekey")
		if got := len(sink.Records().Matching("hooks", "rekey").AtExactLevel(slog.LevelWarn)); got != 3 {
			t.Errorf("%d rekey WARNs, want one for each key the refusal kept", got)
		}
	})

	t.Run("a move onto a key the rekey moves away or removes is allowed", func(t *testing.T) {
		store, path := hookstest.StageStore(t, hookstest.Staging{Seed: rekeySeed})

		if err := store.Rekey(map[string]string{hookstest.TmuxEraSeedA: hookstest.TmuxEraSeedB, hookstest.TmuxEraSeedB: hookstest.TmuxEraSeedC}, []string{hookstest.TmuxEraSeedC}, hooks.ViaCLI); err != nil {
			t.Fatalf("Rekey: %v", err)
		}
		after := decodeHooksFile(t, path)
		if got := compactJSON(t, after[hookstest.TmuxEraSeedB]["on-resume"]); got != `"claude --resume token"` {
			t.Errorf("%s on-resume = %s, want the token key's", hookstest.TmuxEraSeedB, got)
		}
		if got := slices.Sorted(maps.Keys(after[hookstest.TmuxEraSeedC])); !slices.Equal(got, []string{"on-other", "on-resume"}) {
			t.Errorf("%s holds %v, want the coordinate key's events in place of its own", hookstest.TmuxEraSeedC, got)
		}
	})

	for name, refused := range map[string]struct {
		moves  map[string]string
		remove []string
	}{
		"two keys moved onto one": {moves: map[string]string{hookstest.TmuxEraSeedA: hookstest.SubjectSeedA, hookstest.TmuxEraSeedB: hookstest.SubjectSeedA}},
		"a key moved and removed": {moves: map[string]string{hookstest.TmuxEraSeedA: hookstest.SubjectSeedA}, remove: []string{hookstest.TmuxEraSeedA}},
	} {
		t.Run(name+" are refused before the file is read", func(t *testing.T) {
			store, path := hookstest.StageStore(t, hookstest.Staging{Seed: rekeySeed, SidecarAbsent: true})
			before := readFileBytes(t, path)

			if err := store.Rekey(refused.moves, refused.remove, hooks.ViaCLI); err == nil {
				t.Fatal("Rekey succeeded")
			}
			hookstest.AssertHooksFileUnchanged(t, path, before, "changed by a refused rekey")
			assertNoSidecar(t, path)
		})
	}

	t.Run("nothing to move or remove takes no lock", func(t *testing.T) {
		store, path := hookstest.StageStore(t, hookstest.Staging{Seed: rekeySeed, SidecarAbsent: true})

		if err := store.Rekey(nil, nil, hooks.ViaCLI); err != nil {
			t.Fatalf("Rekey(nil, nil): %v", err)
		}
		assertNoSidecar(t, path)
	})

	t.Run("it logs each key it moved at INFO with its new key, and each it removed with its commands", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: rekeySeed})
		sink := logtest.Install(t)

		if err := store.Rekey(map[string]string{hookstest.TmuxEraSeedA: hookstest.SubjectSeedA}, []string{hookstest.TmuxEraSeedB}, hooks.ViaCLI); err != nil {
			t.Fatalf("Rekey: %v", err)
		}
		for _, want := range []struct{ op, key, value string }{
			{"rekey", hookstest.TmuxEraSeedA, hookstest.SubjectSeedA},
			{"rekey-rm", hookstest.TmuxEraSeedB, "on-other=x; on-resume=claude --resume coords"},
		} {
			rec := sink.Records().Matching("hooks", want.op).Only(t, want.op+" record")
			logtest.AssertRecord(t, rec, logtest.RecordWant{Level: slog.LevelInfo, Msg: want.op, Component: "hooks", Op: want.op, Via: "cli"})
			if got := rec.AttrString(t, "hook_key"); got != want.key {
				t.Errorf("%s hook_key = %q, want %q", want.op, got, want.key)
			}
			if got := rec.AttrString(t, "value"); got != want.value {
				t.Errorf("%s value = %q, want %q", want.op, got, want.value)
			}
		}
	})

	t.Run("a file that does not parse is refused, not read as empty", func(t *testing.T) {
		store, path := hookstest.StageStore(t, hookstest.Staging{Seed: "{not json"})

		if err := store.Rekey(moves, remove, hooks.ViaCLI); !errors.Is(err, hooks.ErrStoreRead) {
			t.Fatalf("Rekey = %v, want ErrStoreRead", err)
		}
		hookstest.AssertHooksFileUnchanged(t, path, []byte("{not json"), "changed by a rekey of a malformed file")
	})

	t.Run("a save that fails changes nothing and warns for each key", func(t *testing.T) {
		store, path := hookstest.StageStore(t, hookstest.Staging{Seed: rekeySeed, WritesDenied: true})
		before := readFileBytes(t, path)
		sink := logtest.Install(t)

		if err := store.Rekey(map[string]string{hookstest.TmuxEraSeedA: hookstest.SubjectSeedA}, nil, hooks.ViaCLI); err == nil {
			t.Fatal("Rekey succeeded with writes denied")
		}
		hookstest.AssertHooksFileUnchanged(t, path, before, "changed by a failed rekey")
		rec := sink.Records().Matching("hooks", "rekey").Only(t, "rekey WARN")
		logtest.AssertRecord(t, rec, logtest.RecordWant{Level: slog.LevelWarn, Msg: "rekey", Component: "hooks", Op: "rekey", Via: "cli"})
		logtest.AssertWriteFailure(t, rec, "write-failed-temp-create", fileutil.ErrWriteTempCreate)
	})
}

func assertNoSidecar(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(hookstest.SidecarPath(path)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("sidecar created: %v", err)
	}
}
