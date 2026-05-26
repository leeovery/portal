# Move `hooks` to the `skipTmuxCheck` allowlist in `cmd/root.go`

`portal hooks set`, `portal hooks rm`, and `portal hooks list` currently trigger the full 11-step bootstrap on every invocation. `hooks` is intentionally absent from the `skipTmuxCheck` map in `cmd/root.go` — the comment at lines 29-33 explains the reasoning was to keep `CleanStale` and skeleton restoration in the hooks code path "where the user expects it." That reasoning made sense when `portal hooks set` was understood as a rare, human-initiated command. It is actively harmful in the real-world usage pattern where Claude Code's `SessionStart` hook fires `portal hooks set` ~13 times in 3 seconds as each successfully-resumed Claude session in turn registers its new UUID.

Each cascading bootstrap reaches step 7 (`EagerSignalHydrate`), iterates skeleton markers that haven't yet been cleared by other panes' still-completing helpers, and logs ENOENT for FIFOs whose own helpers already consumed them via the `os.Remove` after their successful signal read. That's the 53 ENOENT warnings observed in `portal.log` during post-reboot verification on 2026-05-26 — pure cascade noise, no user-visible failure, but real log pollution plus ~100ms per Claude resume in bootstrap overhead plus an expanded blast radius for the separately-logged `bootstrap-cleanstale-wipes-hooks-on-tmux-transient` defect.

The change is a single map-entry addition in `cmd/root.go`:

```
var skipTmuxCheck = map[string]bool{
    "alias":   true,
    "clean":   true,
    "help":    true,
    "hooks":   true,   // new
    "init":    true,
    "state":   true,
    "version": true,
}
```

The comment block above the map needs rewording. The current text justifies the *exclusion* of `hooks`; the new text needs to justify the *inclusion* — `hooks set`/`rm`/`list` are pure config-file operations that need only `$TMUX_PANE` (already guaranteed because they run inside a tmux pane) and a single `tmux display-message` call to resolve the structural pane key. They do not need daemon orchestration, saver bootstrap, version-upgrade machinery, Restore, EagerSignalHydrate, marker/FIFO cleanup, or `hookStore.CleanStale`. `hooks list` needs nothing tmux-related at all. Auto-cleanup of stale entries moves to bootstrap-triggering commands only (`portal open`, `x`, `attach`).

A brief check is needed at `cmd/hooks.go:47` (`resolveCurrentPaneKey`) to confirm it doesn't pull the tmux client from a context value that bootstrap is responsible for injecting; if it does, the trivial mitigation is to construct a one-off `tmux.DefaultClient()` at the call site (same pattern used by `cmd/state_hydrate.go:385`). The hooks subcommand `RunE` bodies are otherwise unchanged.

`cmd/root_test.go` already exercises the `skipTmuxCheck` behavior for the existing entries; one new sub-test should assert that `portal hooks set …` does not invoke the bootstrap orchestrator. Existing `cmd/hooks_test.go` coverage stays green.

This quickfix does not address the latent `bootstrap-cleanstale-wipes-hooks-on-tmux-transient` defect — that bug can still trigger via `portal open`/`x`/`attach`. The two fixes are complementary: this one eliminates the SessionStart-cascade trigger; the other addresses the root-cause `ListAllPanes` error-swallowing pattern.
