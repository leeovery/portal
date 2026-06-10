# Specification: File Browser Alias Breadcrumb

## Specification

## Problem & Chosen Approach

### What was reported

Saving an alias from the shared file-browser "save alias for highlighted dir" flow (the `a`-key, `handleAliasSave` in `internal/ui/browser.go`) emits **no** `aliases: set` audit breadcrumb in `portal.log`. The flow uses the un-audited two-step `store.Set(...)` + `store.Save()` rather than the audited `SetAndSave` chokepoint, and the `AliasSaver` interface it depends on exposes only `Load`/`Set`/`Save` — so the audited path is structurally unreachable from this caller.

### What the investigation established

The flow is **unreachable dead code in production** — the defect is latent, not active:

- The only production file-browser construction (`internal/tui/model.go`) uses the plain `ui.NewFileBrowser(...)` constructor, leaving `aliasStore == nil`.
- The `a`-key handler is gated on `m.aliasStore != nil`, so in the production TUI pressing `a` just appends `a` to the filter text — the alias prompt never opens and `handleAliasSave` never runs.
- `NewFileBrowserWithAlias` (the only constructor that injects an alias store and enables the prompt) is called **only** from a test file.
- The whole file browser is reachable only via the Projects-page `b` key, which the user confirmed they never use.

No alias is created via this flow today, so the missing breadcrumb produces no observable symptom.

### Decision — remove the file browser feature in full

Decided with the user at findings review (2026-06-09). Because the reported bug sits on unreachable dead code, an in-place audit-fix would only polish code that never runs. The user confirmed they never use the file browser and want it gone. **The fix for this bug is to delete the file-browser feature** — this resolves the latent audit-bypass by removal and reclaims two dead packages. No `SetAndSave` rewiring is performed.

Alternatives rejected at findings review:
- **(A) Audit-fix in place** (route `handleAliasSave` onto `SetAndSave`) — polishes code that never executes; leaves unused surface area.
- **(B) Wire it up and finish the feature** — net-new feature work the user doesn't want.
- **(C) Remove the feature entirely** — **chosen**.

### Scope boundary — what must stay green and unchanged

These are independent of the file browser and must not be touched:

- The alias CLI (`cmd/alias.go`; `portal alias set/rm/list`).
- The projects-modal alias editor (`internal/tui/model.go` `aliasEditor` → `SetAndSave`).
- The resolver chain (path → alias → zoxide → TUI filter fallback).
- The Sessions, Projects, and Preview pages.
- `createSession` — survives; it has three non-browser callers. Only the browser→create-session entry point is removed.
- `cfg.cwd` / `m.cwd` — consumed by `WithCWD` and `viewCWD` / `createSession(m.cwd)`, independent of the browser.

---

## Working Notes
