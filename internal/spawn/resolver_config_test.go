package spawn

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// newTestResolver builds a Resolver over cfg with the recipe runner swapped for
// a fake, so any resolved argv/script adapter is wired to a fabricated runner
// rather than the production execRecipeRunner. Resolve itself never runs the
// runner (only OpenWindow does), so the fake only pins the wiring — no real
// process is ever spawned by these resolver tests.
func newTestResolver(cfg TerminalsConfig) (*Resolver, *fakeRecipeRunner) {
	fake := &fakeRecipeRunner{}
	r := NewResolver(cfg)
	r.runner = fake
	return r, fake
}

// argvOpenEntry is a terminals.json entry declaring a valid argv `open` recipe.
func argvOpenEntry(argv ...string) TerminalEntry {
	return TerminalEntry{Commands: Capabilities{Open: &Recipe{Argv: argv}}}
}

// bothArgvAndScriptEntry is a structurally-invalid entry: it declares BOTH argv
// and script, which validateRecipe rejects (exactly one is required).
func bothArgvAndScriptEntry() TerminalEntry {
	return TerminalEntry{Commands: Capabilities{Open: &Recipe{
		Argv:   []string{"term", "{command}"},
		Script: "/opt/term/open.sh",
	}}}
}

func TestResolverResolve(t *testing.T) {
	t.Run("it resolves a matching config entry ahead of a would-match native adapter", func(t *testing.T) {
		// The identity ALSO matches the native Ghostty registry, yet a matching
		// valid config entry must win — config ahead of native.
		cfg := TerminalsConfig{"com.mitchellh.ghostty*": argvOpenEntry("ghostty", "{command}")}
		r, fake := newTestResolver(cfg)

		adapter, resolution := r.Resolve(NewIdentity("com.mitchellh.ghostty", "Ghostty"))

		if resolution != ResolutionConfig {
			t.Errorf("resolution = %q, want %q (config ahead of native)", resolution, ResolutionConfig)
		}
		argv, ok := adapter.(*argvRecipeAdapter)
		if !ok {
			t.Fatalf("adapter = %T, want *argvRecipeAdapter", adapter)
		}
		if !slices.Equal(argv.template, []string{"ghostty", "{command}"}) {
			t.Errorf("template = %#v, want the entry's recipe argv", argv.template)
		}
		if argv.runner != fake {
			t.Errorf("adapter runner = %p, want the resolver's injected runner %p", argv.runner, fake)
		}
	})

	t.Run("it falls through to native when there is no config entry", func(t *testing.T) {
		r, _ := newTestResolver(TerminalsConfig{})

		adapter, resolution := r.Resolve(NewIdentity("com.mitchellh.ghostty", "Ghostty"))

		if resolution != ResolutionNative {
			t.Errorf("resolution = %q, want %q", resolution, ResolutionNative)
		}
		if _, ok := adapter.(*ghosttyAdapter); !ok {
			t.Errorf("adapter = %T, want *ghosttyAdapter", adapter)
		}
	})

	t.Run("it falls through past an invalid config entry to native then unsupported", func(t *testing.T) {
		// A native-matching identity whose matching config entry is structurally
		// invalid (both argv+script) falls through to native, with the Task 4.2 WARN.
		sink := installSpawnCapture(t)
		cfg := TerminalsConfig{"com.mitchellh.ghostty*": bothArgvAndScriptEntry()}
		r, _ := newTestResolver(cfg)

		adapter, resolution := r.Resolve(NewIdentity("com.mitchellh.ghostty", "Ghostty"))

		if resolution != ResolutionNative {
			t.Errorf("resolution = %q, want %q (invalid config falls through to native)", resolution, ResolutionNative)
		}
		if _, ok := adapter.(*ghosttyAdapter); !ok {
			t.Errorf("adapter = %T, want *ghosttyAdapter", adapter)
		}
		if got := warnRecords(sink); len(got) != 1 {
			t.Errorf("emitted %d WARN records, want exactly 1 rejecting the invalid entry: %+v", len(got), got)
		}
	})

	t.Run("it falls through to unsupported past an invalid matching config entry for a non-native identity", func(t *testing.T) {
		// A non-native identity whose matching config entry is invalid falls all the
		// way through to unsupported (config invalid → native no-match → unsupported).
		sink := installSpawnCapture(t)
		cfg := TerminalsConfig{"com.example.MyTerm": bothArgvAndScriptEntry()}
		r, _ := newTestResolver(cfg)

		adapter, resolution := r.Resolve(NewIdentity("com.example.MyTerm", ""))

		if resolution != ResolutionUnsupported {
			t.Errorf("resolution = %q, want %q", resolution, ResolutionUnsupported)
		}
		if adapter != nil {
			t.Errorf("adapter = %T, want nil", adapter)
		}
		if got := warnRecords(sink); len(got) != 1 {
			t.Errorf("emitted %d WARN records, want exactly 1 rejecting the invalid entry: %+v", len(got), got)
		}
	})

	t.Run("it returns unsupported for an unmatched unknown identity with a non-matching config entry", func(t *testing.T) {
		cfg := TerminalsConfig{"com.mitchellh.ghostty*": argvOpenEntry("ghostty", "{command}")}
		r, _ := newTestResolver(cfg)

		adapter, resolution := r.Resolve(NewIdentity("com.example.MyTerm", ""))

		if resolution != ResolutionUnsupported {
			t.Errorf("resolution = %q, want %q", resolution, ResolutionUnsupported)
		}
		if adapter != nil {
			t.Errorf("adapter = %T, want nil", adapter)
		}
	})

	t.Run("it returns unsupported for a NULL identity even with a catch-all config entry", func(t *testing.T) {
		// The crux: a NULL identity (remote/mosh / no host-local terminal) must skip
		// the config tier entirely — even a `*` catch-all must NOT hijack it.
		cfg := TerminalsConfig{"*": argvOpenEntry("anyterm", "{command}")}
		r, _ := newTestResolver(cfg)

		adapter, resolution := r.Resolve(NewIdentity("", ""))

		if resolution != ResolutionUnsupported {
			t.Errorf("resolution = %q, want %q (config tier skipped for NULL)", resolution, ResolutionUnsupported)
		}
		if adapter != nil {
			t.Errorf("adapter = %T, want nil — a `*` catch-all must not hijack a NULL identity", adapter)
		}
	})

	t.Run("it resolves a valid script config entry to the script adapter with resolution=config", func(t *testing.T) {
		scriptPath := filepath.Join(t.TempDir(), "open.sh")
		if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatalf("write executable script: %v", err)
		}
		cfg := TerminalsConfig{"com.mitchellh.ghostty*": {Commands: Capabilities{Open: &Recipe{Script: scriptPath}}}}
		r, fake := newTestResolver(cfg)

		adapter, resolution := r.Resolve(NewIdentity("com.mitchellh.ghostty", "Ghostty"))

		if resolution != ResolutionConfig {
			t.Errorf("resolution = %q, want %q", resolution, ResolutionConfig)
		}
		script, ok := adapter.(*scriptRecipeAdapter)
		if !ok {
			t.Fatalf("adapter = %T, want *scriptRecipeAdapter", adapter)
		}
		if script.scriptPath != scriptPath {
			t.Errorf("scriptPath = %q, want %q", script.scriptPath, scriptPath)
		}
		if script.runner != fake {
			t.Errorf("adapter runner = %p, want the resolver's injected runner %p", script.runner, fake)
		}
	})
}
