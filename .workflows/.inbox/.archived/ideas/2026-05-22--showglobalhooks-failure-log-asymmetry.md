> **STATUS — folded into `2026-05-25--portal-observability-layer.md` on 2026-05-26.** The "WARN at failure site for diagnostic locality" option was the chosen direction. See that file's Additional concrete gap-closures section for the consolidated atom. Archiving this standalone idea so the inbox doesn't double-count.

# Converge `ShowGlobalHooks` failure logging between hook-migration helpers

`migrateSessionClosedHook` (`internal/tmux/hooks_register.go:304`) emits a WARN log AND returns a wrapped error when `ShowGlobalHooks` fails, whereas `migrateHydrationHooks` (line 232) only wraps and returns. Both behaviours are defensible — the returned error is folded into the `errors.Join` aggregate that `RegisterPortalHooks` ultimately logs at the bootstrap layer — but the asymmetry could confuse a future reader. Either:

- Converge both helpers on "wrap + return; let the caller log once" (drop the WARN inside `migrateSessionClosedHook`), or
- Converge on "WARN at the failure site for diagnostic locality" (add WARN to `migrateHydrationHooks`).

Requires a deliberate logging-discipline decision rather than a mechanical fix.

Source: review of killed-session-resurrects-within-tick-window/killed-session-resurrects-within-tick-window
