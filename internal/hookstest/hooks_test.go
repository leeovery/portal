package hookstest_test

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/backend/zmx"
	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/hookstest"
)

// sessionNameSeeds name every seed the vocabulary holds, in the order it
// declares them, so the sweeps below cover the whole of it rather than a
// prefix of it.
var sessionNameSeeds = []struct{ name, key string }{
	{"ReapableSeedA", hookstest.ReapableSeedA},
	{"ReapableSeedB", hookstest.ReapableSeedB},
	{"ReapableSeedC", hookstest.ReapableSeedC},
	{"ReapableSeedD", hookstest.ReapableSeedD},
	{"LiveSeedA", hookstest.LiveSeedA},
	{"LiveSeedB", hookstest.LiveSeedB},
	{"LiveSeedC", hookstest.LiveSeedC},
	{"SubjectSeedA", hookstest.SubjectSeedA},
	{"SubjectSeedB", hookstest.SubjectSeedB},
	{"SubjectSeedC", hookstest.SubjectSeedC},
	{"SubjectSeedD", hookstest.SubjectSeedD},
}

func TestHookKeySeedVocabulary(t *testing.T) {
	t.Run("it names a session zmx can hold with every session-name seed", func(t *testing.T) {
		for _, seed := range sessionNameSeeds {
			if err := zmx.CheckLookup(seed.key); err != nil {
				t.Errorf("%s = %q, want a name a zmx session can have: %v", seed.name, seed.key, err)
			}
		}
	})

	t.Run("it sorts the session-name seeds in the order they are declared", func(t *testing.T) {
		keys := make([]string, 0, len(sessionNameSeeds))
		for _, seed := range sessionNameSeeds {
			keys = append(keys, seed.key)
		}
		if !slices.IsSorted(keys) {
			t.Errorf("seeds %v do not sort in declared order; a fixture listing several would read them out of order", keys)
		}
	})

	t.Run("every named seed is distinct", func(t *testing.T) {
		seen := map[string]string{}
		for _, seed := range sessionNameSeeds {
			assertUnseen(t, seen, seed.name, seed.key)
		}
	})
}

func assertUnseen(t *testing.T, seen map[string]string, name, key string) {
	t.Helper()
	if prev, dup := seen[key]; dup {
		t.Errorf("key %q is shared by %s and %s", key, prev, name)
	}
	seen[key] = name
}

func stageHooksFile(t *testing.T, body string) string {
	t.Helper()
	_, path := hookstest.StageStore(t, hookstest.Staging{Seed: body})
	return path
}

func TestAssertHooksFileUnchanged(t *testing.T) {
	t.Run("it passes when the file is byte-identical before and after", func(t *testing.T) {
		path := stageHooksFile(t, `{"tok01":{"on-resume":"npm start"}}`)
		before := hookstest.HooksFileBytes(t, path)

		rec := &harnesstest.Recorder{}
		rec.Run(func() { hookstest.AssertHooksFileUnchanged(rec, path, before) })

		if len(rec.Errors) != 0 || len(rec.Fatals) != 0 {
			t.Errorf("an unchanged file reported errors %v and fatals %v, want neither", rec.Errors, rec.Fatals)
		}
	})

	t.Run("it fails when a single byte changed", func(t *testing.T) {
		path := stageHooksFile(t, `{"tok01":{"on-resume":"npm start"}}`)
		before := hookstest.HooksFileBytes(t, path)
		if err := os.WriteFile(path, []byte(`{"tok01":{"on-resume":"npm starT"}}`), 0o600); err != nil {
			t.Fatalf("rewrite hooks.json: %v", err)
		}

		rec := &harnesstest.Recorder{}
		rec.Run(func() { hookstest.AssertHooksFileUnchanged(rec, path, before) })

		if len(rec.Errors) != 1 {
			t.Fatalf("got %d errors, want exactly 1: %v", len(rec.Errors), rec.Errors)
		}
		if want := "changed on a failing route"; !strings.Contains(rec.Errors[0], want) {
			t.Errorf("failure message %q does not carry the default wording %q", rec.Errors[0], want)
		}
		if !strings.Contains(rec.Errors[0], "npm starT") {
			t.Errorf("failure message %q does not carry the after bytes", rec.Errors[0])
		}
	})

	t.Run("it uses the caller's context in the failure message", func(t *testing.T) {
		path := stageHooksFile(t, `{"tok01":{"on-resume":"npm start"}}`)
		before := hookstest.HooksFileBytes(t, path)
		if err := os.WriteFile(path, []byte(`{}`), 0o600); err != nil {
			t.Fatalf("rewrite hooks.json: %v", err)
		}

		rec := &harnesstest.Recorder{}
		rec.Run(func() { hookstest.AssertHooksFileUnchanged(rec, path, before, "rewritten on a stand-down") })

		if len(rec.Errors) != 1 {
			t.Fatalf("got %d errors, want exactly 1: %v", len(rec.Errors), rec.Errors)
		}
		if want := "rewritten on a stand-down"; !strings.Contains(rec.Errors[0], want) {
			t.Errorf("failure message %q does not carry the caller's context %q", rec.Errors[0], want)
		}
		if unwanted := "changed on a failing route"; strings.Contains(rec.Errors[0], unwanted) {
			t.Errorf("failure message %q still carries the default wording %q", rec.Errors[0], unwanted)
		}
	})

	t.Run("it fatals when the read fails for a reason other than absence", func(t *testing.T) {
		_, path := hookstest.StageStore(t, hookstest.Staging{Unreadable: true})

		rec := &harnesstest.Recorder{}
		rec.Run(func() { hookstest.HooksFileBytes(rec, path) })

		if len(rec.Fatals) != 1 {
			t.Fatalf("got %d fatals, want exactly 1: %v", len(rec.Fatals), rec.Fatals)
		}
		if !strings.Contains(rec.Fatals[0], path) {
			t.Errorf("fatal message %q does not name the path it could not read", rec.Fatals[0])
		}
	})

	t.Run("it treats an absent file as absent rather than fatalling", func(t *testing.T) {
		path := hookstest.HooksPath(t, t.TempDir())

		before := hookstest.HooksFileBytes(t, path)
		if before != nil {
			t.Fatalf("HooksFileBytes of an absent file = %q, want nil", before)
		}

		rec := &harnesstest.Recorder{}
		rec.Run(func() { hookstest.AssertHooksFileUnchanged(rec, path, before) })

		if len(rec.Fatals) != 0 {
			t.Errorf("an absent file fatalled: %v", rec.Fatals)
		}
		if len(rec.Errors) != 0 {
			t.Errorf("an absent file before and after compared unequal: %v", rec.Errors)
		}
	})
}
