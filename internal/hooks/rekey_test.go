package hooks_test

import (
	"cmp"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/fileutil"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/resumemode"
)

// rekeyBackup holds the tmux version's key shapes: a token in the string form,
// a token in the object form carrying an attribute the codec doesn't model and
// a second event, coordinates to remove, and a key no rekey names.
var rekeyBackup = fmt.Sprintf(`{
  %q: {"on-resume": "claude --resume token"},
  %q: {"on-resume": {"command": "claude --resume second", "resume": "eager", "nickname": "kept"}, "on-other": "x"},
  %q: {"on-resume": "claude --resume coords"},
  %q: {"on-resume": "untouched"}
}`, hookstest.TmuxEraSeedA, hookstest.ReapableSeedA, hookstest.TmuxEraSeedB, hookstest.LiveSeedA)

// rekeyMoves and rekeyRemove are the rewrite rekeyBackup is put through.
var (
	rekeyMoves  = map[string]string{hookstest.TmuxEraSeedA: hookstest.SubjectSeedA, hookstest.ReapableSeedA: hookstest.SubjectSeedB}
	rekeyRemove = []string{hookstest.TmuxEraSeedB}
)

// stageBackup stages body as the backup a rekey reads, in a directory of its
// own, and returns its path.
func stageBackup(t *testing.T, body string) string {
	t.Helper()
	_, path := hookstest.StageStore(t, hookstest.Staging{Seed: body, SidecarAbsent: true})
	return path
}

// stageLive stages the hooks.json a rekey writes, holding a key the backup
// doesn't, so a rekey that read it would keep that key.
func stageLive(t *testing.T, staging hookstest.Staging) (*hooks.Store, string) {
	t.Helper()
	if staging.Seed == "" {
		staging.Seed = fmt.Sprintf(`{%q: {"on-resume": "live only"}}`, hookstest.SubjectSeedC)
	}
	return hookstest.StageStore(t, staging)
}

func TestRekeyFrom(t *testing.T) {
	t.Run("it writes the backup with each key moved, every value byte for byte, and the removed keys gone, never reading hooks.json", func(t *testing.T) {
		backup := stageBackup(t, rekeyBackup)
		store, path := stageLive(t, hookstest.Staging{})
		before := decodeHooksFile(t, backup)

		if err := store.RekeyFrom(backup, rekeyMoves, rekeyRemove, hooks.ViaCLI); err != nil {
			t.Fatalf("RekeyFrom: %v", err)
		}

		after := decodeHooksFile(t, path)
		if got := slices.Sorted(maps.Keys(after)); !slices.Equal(got, []string{hookstest.LiveSeedA, hookstest.SubjectSeedA, hookstest.SubjectSeedB}) {
			t.Fatalf("keys = %v, want the two moved, the one no rekey names, and nothing of hooks.json's own", got)
		}
		for from, to := range map[string]string{hookstest.TmuxEraSeedA: hookstest.SubjectSeedA, hookstest.ReapableSeedA: hookstest.SubjectSeedB, hookstest.LiveSeedA: hookstest.LiveSeedA} {
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

	t.Run("a key both removed and moved onto holds the moved hooks", func(t *testing.T) {
		backup := stageBackup(t, rekeyBackup)
		store, path := stageLive(t, hookstest.Staging{})

		if err := store.RekeyFrom(backup, map[string]string{hookstest.TmuxEraSeedA: hookstest.TmuxEraSeedB}, []string{hookstest.TmuxEraSeedB}, hooks.ViaCLI); err != nil {
			t.Fatalf("RekeyFrom: %v", err)
		}
		if got := compactJSON(t, decodeHooksFile(t, path)[hookstest.TmuxEraSeedB]["on-resume"]); got != `"claude --resume token"` {
			t.Errorf("%s on-resume = %s, want the moved token's", hookstest.TmuxEraSeedB, got)
		}
	})

	t.Run("a run again writes the same file, whatever hooks.json became meanwhile", func(t *testing.T) {
		backup := stageBackup(t, rekeyBackup)
		store, path := stageLive(t, hookstest.Staging{})
		if err := store.RekeyFrom(backup, rekeyMoves, rekeyRemove, hooks.ViaCLI); err != nil {
			t.Fatalf("RekeyFrom: %v", err)
		}
		first := readFileBytes(t, path)
		if err := store.Set(hookstest.SubjectSeedA, hooks.EventOnResume, hooks.Registration{Command: "changed since", Resume: resumemode.Lazy}, hooks.ViaCLI); err != nil {
			t.Fatalf("Set: %v", err)
		}

		if err := store.RekeyFrom(backup, rekeyMoves, rekeyRemove, hooks.ViaCLI); err != nil {
			t.Fatalf("RekeyFrom again: %v", err)
		}
		hookstest.AssertHooksFileUnchanged(t, path, first, "differs on a second run")
	})

	t.Run("a key the backup doesn't hold is passed over", func(t *testing.T) {
		backup := stageBackup(t, rekeyBackup)
		store, path := stageLive(t, hookstest.Staging{})

		if err := store.RekeyFrom(backup, map[string]string{hookstest.TmuxEraSeedC: hookstest.SubjectSeedC}, []string{hookstest.ReapableSeedB}, hooks.ViaCLI); err != nil {
			t.Fatalf("RekeyFrom: %v", err)
		}
		if got := slices.Sorted(maps.Keys(decodeHooksFile(t, path))); !slices.Equal(got, slices.Sorted(maps.Keys(decodeHooksFile(t, backup)))) {
			t.Errorf("keys = %v, want the backup's", got)
		}
	})

	t.Run("a key moved to itself is no change, neither logged nor counted", func(t *testing.T) {
		backup := stageBackup(t, rekeyBackup)
		store, path := stageLive(t, hookstest.Staging{})
		sink := logtest.Install(t)

		if err := store.RekeyFrom(backup, map[string]string{hookstest.TmuxEraSeedA: hookstest.TmuxEraSeedA}, nil, hooks.ViaCLI); err != nil {
			t.Fatalf("RekeyFrom: %v", err)
		}
		if got := compactJSON(t, decodeHooksFile(t, path)[hookstest.TmuxEraSeedA]["on-resume"]); got != `"claude --resume token"` {
			t.Errorf("%s on-resume = %s, want its own", hookstest.TmuxEraSeedA, got)
		}
		if moved := sink.Records().Matching("hooks", "rekey-move"); len(moved) != 0 {
			t.Errorf("logged %d moves, want none", len(moved))
		}
		if got := sink.Records().Matching("hooks", "rekey").Only(t, "rekey summary").AttrString(t, "entries"); got != "0" {
			t.Errorf("entries = %q, want 0", got)
		}
	})

	t.Run("no backup writes nothing and takes no lock", func(t *testing.T) {
		store, path := stageLive(t, hookstest.Staging{SidecarAbsent: true})
		before := readFileBytes(t, path)
		sink := logtest.Install(t)

		if err := store.RekeyFrom(hookstest.HooksPath(t, t.TempDir()), rekeyMoves, rekeyRemove, hooks.ViaCLI); err != nil {
			t.Fatalf("RekeyFrom: %v", err)
		}
		hookstest.AssertHooksFileUnchanged(t, path, before, "written with no backup")
		assertNoSidecar(t, path)
		sink.Records().Matching("hooks", "rekey-noop").AtExactLevel(slog.LevelDebug).Only(t, "rekey-noop")
	})

	t.Run("it logs each key it moved or removed at DEBUG, and one INFO summary", func(t *testing.T) {
		backup := stageBackup(t, rekeyBackup)
		store, _ := stageLive(t, hookstest.Staging{})
		sink := logtest.Install(t)

		if err := store.RekeyFrom(backup, map[string]string{hookstest.TmuxEraSeedA: hookstest.SubjectSeedA}, []string{hookstest.ReapableSeedA}, hooks.ViaCLI); err != nil {
			t.Fatalf("RekeyFrom: %v", err)
		}
		for _, want := range []struct{ op, key, value string }{
			{"rekey-move", hookstest.TmuxEraSeedA, hookstest.SubjectSeedA},
			{"rekey-rm", hookstest.ReapableSeedA, "on-other=x; on-resume=claude --resume second"},
		} {
			rec := sink.Records().Matching("hooks", want.op).Only(t, want.op+" record")
			logtest.AssertRecord(t, rec, logtest.RecordWant{Level: slog.LevelDebug, Msg: want.op, Component: "hooks", Op: want.op, Via: "cli"})
			if got := rec.AttrString(t, "hook_key"); got != want.key {
				t.Errorf("%s hook_key = %q, want %q", want.op, got, want.key)
			}
			if got := rec.AttrString(t, "value"); got != want.value {
				t.Errorf("%s value = %q, want %q", want.op, got, want.value)
			}
		}
		summary := sink.Records().Matching("hooks", "rekey").Only(t, "rekey summary")
		logtest.AssertRecord(t, summary, logtest.RecordWant{Level: slog.LevelInfo, Msg: "rekey", Component: "hooks", Op: "rekey", Via: "cli"})
		if got := summary.AttrString(t, "entries"); got != "2" {
			t.Errorf("entries = %q, want 2", got)
		}
		summary.RequireDuration(t, "took")
	})

	for name, refused := range map[string]struct {
		backup string
		moves  map[string]string
		remove []string
		want   error
	}{
		"two keys moved onto one": {
			moves: map[string]string{hookstest.TmuxEraSeedA: hookstest.SubjectSeedA, hookstest.ReapableSeedA: hookstest.SubjectSeedA},
		},
		"a key moved and removed": {
			moves: map[string]string{hookstest.TmuxEraSeedA: hookstest.SubjectSeedA}, remove: []string{hookstest.TmuxEraSeedA},
		},
		"a move onto a key the rewrite keeps": {
			moves: map[string]string{hookstest.TmuxEraSeedA: hookstest.LiveSeedA}, want: hooks.ErrKeyHeld,
		},
		"a backup that doesn't parse": {
			backup: "{not json", moves: rekeyMoves, want: hooks.ErrMalformed,
		},
	} {
		t.Run(name+" is refused, writing nothing, with one WARN", func(t *testing.T) {
			backup := stageBackup(t, cmp.Or(refused.backup, rekeyBackup))
			store, path := stageLive(t, hookstest.Staging{})
			before := readFileBytes(t, path)
			sink := logtest.Install(t)

			err := store.RekeyFrom(backup, refused.moves, refused.remove, hooks.ViaCLI)
			if err == nil || refused.want != nil && !errors.Is(err, refused.want) {
				t.Fatalf("RekeyFrom = %v, want a refusal (%v)", err, refused.want)
			}
			hookstest.AssertHooksFileUnchanged(t, path, before, "changed by a refused rekey")
			assertRekeyWarn(t, sink, err, false)
		})
	}

	t.Run("a backup that can't be read is refused, writing nothing", func(t *testing.T) {
		_, unreadable := hookstest.StageStore(t, hookstest.Staging{Unreadable: true})
		store, path := stageLive(t, hookstest.Staging{})
		before := readFileBytes(t, path)
		sink := logtest.Install(t)

		err := store.RekeyFrom(unreadable, rekeyMoves, rekeyRemove, hooks.ViaCLI)
		if !errors.Is(err, hooks.ErrStoreRead) {
			t.Fatalf("RekeyFrom = %v, want ErrStoreRead", err)
		}
		hookstest.AssertHooksFileUnchanged(t, path, before, "changed by a refused rekey")
		assertRekeyWarn(t, sink, err, false)
	})

	t.Run("a lock not granted is refused, writing nothing", func(t *testing.T) {
		hooks.SetLockTimeoutForTest(t, 40*time.Millisecond)
		backup := stageBackup(t, rekeyBackup)
		store, path := stageLive(t, hookstest.Staging{})
		before := readFileBytes(t, path)
		hookstest.HoldHooksSidecar(t, path)
		sink := logtest.Install(t)

		err := store.RekeyFrom(backup, rekeyMoves, rekeyRemove, hooks.ViaCLI)
		if !errors.Is(err, hooks.ErrLockHeld) {
			t.Fatalf("RekeyFrom = %v, want ErrLockHeld", err)
		}
		hookstest.AssertHooksFileUnchanged(t, path, before, "changed by a rekey that never held the lock")
		assertRekeyWarn(t, sink, err, false)
	})

	t.Run("a save that fails writes nothing, with one WARN classing the failure", func(t *testing.T) {
		backup := stageBackup(t, rekeyBackup)
		store, path := stageLive(t, hookstest.Staging{WritesDenied: true})
		before := readFileBytes(t, path)
		sink := logtest.Install(t)

		err := store.RekeyFrom(backup, rekeyMoves, rekeyRemove, hooks.ViaCLI)
		if err == nil {
			t.Fatal("RekeyFrom succeeded with writes denied")
		}
		hookstest.AssertHooksFileUnchanged(t, path, before, "changed by a failed rekey")
		assertRekeyWarn(t, sink, err, true)
	})
}

// assertRekeyWarn pins a refused rekey's one line: a WARN under op rekey
// carrying the error, classed only when a write failed, and nothing logged
// per key.
func assertRekeyWarn(t *testing.T, sink *logtest.Sink, err error, writeFailed bool) {
	t.Helper()
	rec := sink.Records().AtOrAboveLevel(slog.LevelWarn).Only(t, "rekey WARN")
	logtest.AssertRecord(t, rec, logtest.RecordWant{Level: slog.LevelWarn, Msg: "rekey", Component: "hooks", Op: "rekey", Via: "cli"})
	if got := rec.ErrorAttr(t, "error"); got == nil || got.Error() != err.Error() {
		t.Errorf("error = %v, want %v", got, err)
	}
	if writeFailed {
		logtest.AssertWriteFailure(t, rec, "write-failed-temp-create", fileutil.ErrWriteTempCreate)
	} else if rec.HasAttr("error_class") {
		t.Errorf("a refusal before any write carries error_class: %+v", rec.Attrs)
	}
	if n := len(sink.Records().Matching("hooks", "rekey-move")) + len(sink.Records().Matching("hooks", "rekey-rm")); n != 0 {
		t.Errorf("%d per-key lines from a rekey that wrote nothing", n)
	}
}

func assertNoSidecar(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(hookstest.SidecarPath(path)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("sidecar created: %v", err)
	}
}
