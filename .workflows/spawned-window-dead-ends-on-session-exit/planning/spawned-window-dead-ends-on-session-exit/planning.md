# Plan: Spawned Window Dead-Ends On Session Exit

## Phases

### Phase 1: Ghostty Adapter Shell-Fallback Wrapper
status: draft

**Goal**: Wrap the native Ghostty adapter's window command as `bash -lc '<composed open argv>; exec "$SHELL" -il'` and drop `wait after command` from its osascript, so that when the session inside a burst-spawned (N−1 external) Ghostty window exits or detaches, the exec'd interactive login shell keeps the window visible and usable at the user's normal prompt instead of the "Process exited. Press any key to close the terminal." dead-end. Add unit coverage at the existing command-composition seam. The change is scoped entirely to `internal/spawn/ghostty.go`; both burst entry points benefit automatically through the shared adapter.

**Why this order**: This is a single-root-cause, surgically-contained bugfix. The wrapper and the `wait after command` drop are two halves of one conceptual change (the exec'd shell is precisely what makes the wait-flag redundant), and the regression tests assert both. There is no prior foundation to establish and no independently-valuable intermediate state, so splitting would only add phase-management overhead without a meaningful checkpoint.

**Acceptance**:
- [ ] The native Ghostty adapter's composed window command is the explicit wrapper form `bash -lc '<composed open argv>; exec "$SHELL" -il'`, built as a real 3-element argv rendered through the shared shell-quote helper so the inner argv's single quotes are correctly `'\''`-escaped (not a naive byte concatenation), with the osascript `command:"…"` double-quote/backslash layer wrapping the result as today
- [ ] The Ghostty osascript no longer emits `wait after command`
- [ ] The composed open argv inside the wrapper is byte-identical to today for both surface kinds (attach: `open --session <name> --ack <batch>:<token>`; mint: `open --path <dir> --ack <batch>:<token>`, including an optional `-- <command…>` passthrough), retaining the `/usr/bin/env … PATH=<picker PATH> -u TMUX -u TMUX_PANE` prefix
- [ ] A unit test at the command-composition seam (around the existing `ghosttyEmbed` / template tests) asserts the wrapper shape against the correctly-escaped expected string, the absence of `wait after command`, and the preserved PATH/`-u TMUX` prefix
- [ ] The quote-nesting assertion uses a quote-sensitive fixture — an argv element containing shell-special characters (a single quote, `;`, `$`, `"`), e.g. the mint `-- <command…>` passthrough — and proves the embedded argv round-trips uncorrupted through the added `bash -lc '…'` layer, actually exercising the `'\''` double-escaping path
- [ ] Shared `composeOpenArgv` / `renderCommandString`, the `syscall.Exec` attach path, the `@portal-spawn-<batch>-<token>` ack marker ordering, the trigger path, single-session `portal open`/attach, and custom `terminals.json` adapters are all unchanged in behaviour, and the full test suite passes with no regressions
