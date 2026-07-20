TASK: cli-verb-surface-redesign-6-4 — Alias-key completion for `open -a`; `open -p` / `open -z` delegated to the shell

ACCEPTANCE CRITERIA (from plan row 6-4):
- alias keys via `alias.Store.Keys()` (Phase 2) loaded from the config path (no tmux client needed)
- empty/missing aliases file → no suggestions gracefully
- `NoFileComp` (finite Portal-owned namespace, no file merge)
- `-p` and `-z` register NO Portal completion func so they fall to shell defaults (`-p` → file completion, `-z` → shell/zoxide's own)
- inherits the completion bootstrap-exemption from task 6-3

STATUS: Complete

SPEC CONTEXT:
Spec § "Tab Completion" (specification.md:225-238) defines the principle "complete every Portal-owned enumerable namespace; leave the rest to the shell." The slot table maps `open -a` → alias keys, `open -p` → (shell — paths), `open -z` → (shell / zoxide's own). Spec § Bootstrap Exemption and plan task 6-3 established that the `__complete` path is bootstrap-exempt (in `skipTmuxCheck`) so a TAB press never spins up the server; 6-4 inherits that exemption and additionally needs NO tmux client at all (pure config-file read for alias keys).

IMPLEMENTATION:
- Status: Implemented (fully aligned with acceptance + spec)
- Location:
  - cmd/completion.go:60-69 — `completionAliasKeys` injectable seam: reads only the aliases config file via `loadAliasStore()` (honours PORTAL_ALIASES_FILE), returns `store.Keys()`; a load error yields nil (missing/empty/unreadable file → zero suggestions).
  - cmd/completion.go:84-92 — `completeAliasKeys`: prefix-filters against `toComplete`, returns `cobra.ShellCompDirectiveNoFileComp`.
  - cmd/open.go:1017-1022 — registers only `session` and `alias` flag completers; `-p/--path` and `-z/--zoxide` are deliberately NOT registered (cobra then emits ShellCompDirectiveDefault → shell/zoxide default completion). Comment block (open.go:1006-1013) documents the deliberate absence.
  - internal/alias/store.go:227-234 — `Store.Keys()` returns sorted alias names (the Phase-2 method reused here).
  - cmd/alias.go:90-102, 107-109 — `loadAliasStore` / `aliasFilePath` resolve the config path via `configFilePath("PORTAL_ALIASES_FILE", "aliases")` — no tmux client.
  - cmd/root.go:58-59 — `skipTmuxCheck["__complete"] = true` (the inherited 6-3 exemption); alias completer needs no client at all (documented at completion.go:52-56).
- Notes: `Store.Load()` returns an empty map (no error) for a missing file, so `loadAliasStore` succeeds and `Keys()` returns an empty slice — the graceful degrade path is correct. `-a` completion never touches tmux; only session-name completion (6-3) builds a DefaultClient.

TESTS:
- Status: Adequate
- Coverage (cmd/completion_test.go):
  - TestCompleteAliasKeys (79-118): all-keys+NoFileComp for empty prefix; prefix-filter; nil seam (missing file) → empty + NoFileComp + no panic.
  - TestCompletionAliasKeysProductionSeam (124-154): exercises the REAL seam through PORTAL_ALIASES_FILE — seeded file yields sorted keys (implicitly verifies Keys() sorting: "work=/w\nblog=/b" → [blog, work]) with NO tmux client; missing-file path yields no suggestions. This directly proves "loaded from the config path, no tmux client needed" and "empty/missing → graceful."
  - TestCompletionWiring (190-217): asserts `--alias` flag completer is registered and routes through `completeAliasKeys`; asserts `--path` and `--zoxide` have NO Portal completion func (GetFlagCompletionFunc returns false) — the exact "-p/-z delegate to shell" acceptance.
- Notes: Every acceptance clause has a matching assertion. NoFileComp is asserted on every alias path (including empty). Not over-tested — the production-seam test is distinct from the mocked-seam test (one proves wiring/logic, the other proves real config-file I/O with no client). No redundant or implementation-detail assertions.

CODE QUALITY:
- Project conventions: Followed. Uses the package-level injectable-seam + t.Cleanup restore pattern (mirrors completionSessionNames / the cmd `*Deps` idiom); small 1-method reads; hermetic tests with no t.Parallel (file header notes the seam mutation). Consistent with golang-cli/cobra conventions.
- SOLID principles: Good. Seam (data source) is cleanly separated from `completeAliasKeys` (filter + directive); DI via the overridable var.
- Complexity: Low. Straight-line prefix filter.
- Modern idioms: Yes (strings.HasPrefix, slices.Sort in Keys, errors.Is in Load).
- Readability: Good. Comments explain the deliberate absence of -p/-z completers and why no client is needed — the non-obvious parts are documented at the call sites.
- Issues: One minor redundancy (see non-blocking).

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [quickfix] cmd/completion.go:64-67 — `completionAliasKeys` calls `store.Load()` a second time, but `loadAliasStore()` (cmd/alias.go:97) already loaded the store. The second `Load()` re-reads the file redundantly (harmless — Load is idempotent — but wasteful and slightly misleading). Concrete change: drop the `if _, err := store.Load(); err != nil { return nil }` block and return `store.Keys()` directly after the `loadAliasStore` error check.
