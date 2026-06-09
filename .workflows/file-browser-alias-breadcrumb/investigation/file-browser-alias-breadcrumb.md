# Investigation: File-Browser Alias Creation Emits No Audit Breadcrumb

## Symptoms

### Problem Description

**Expected behavior:**
Saving an alias from the shared file-browser "save alias for highlighted dir"
flow (the `a`-key, `handleAliasSave` in `internal/ui/browser.go`, wired into
`cmd/open.go`) should emit an `aliases: set` audit breadcrumb in `portal.log`,
exactly like every other production alias-mutation site.

**Actual behavior:**
Aliases created this way leave **no** `aliases: set` breadcrumb in `portal.log`.
The flow uses the un-audited two-step `store.Set(...)` + `store.Save()` rather
than the audited `SetAndSave` seam. The `AliasSaver` interface this flow depends
on exposes only `Load` / `Set` / `Save` — so the audited path is structurally
unreachable from this caller.

### Manifestation

- Missing observability: no `aliases: set` log line for file-browser-created
  aliases.
- Defeats the State-mutation audit-trail spec guarantee of "a single place per
  file where the breadcrumb can't be forgotten."

### Reproduction Steps

1. Launch the file browser (`portal open` → TUI file browser, or `cmd/open.go`
   path).
2. Highlight a directory and press `a` to save an alias for it.
3. Inspect `portal.log` — observe no `aliases: set` breadcrumb is emitted.

**Reproducibility:** Always (structural — the audited seam is not wired in).

### Environment

- **Affected environments:** All (local — this is a CLI/TUI tool).
- **Platform:** n/a
- **User conditions:** Any alias created via the file-browser `a`-key flow.

### Impact

- **Severity:** Low. Observability gap, and — per the reachability finding in
  Analysis — currently **latent**: the file-browser alias-save flow is not wired
  into production, so no alias is created this way today. The defect is a
  landmine that activates the moment the flow is wired with a real alias store.
- **Scope:** Zero live aliases today; every file-browser-created alias if/when
  the flow is activated.
- **Business impact:** Weakens the observability spec's chokepoint guarantee
  ("a single place per file where the breadcrumb can't be forgotten") by leaving
  a reachable-by-construction bypass on the alias store.

### References

- Seed: `seeds/2026-06-02-file-browser-alias-breadcrumb.md` (inbox:bug)
- Source: review of `portal-observability-layer/portal-observability-layer`
- Precedent: Task 3-5 instrumented `cmd/alias.go` + `internal/tui/model.go` via
  the audited `SetAndSave(name, path, "cli")` seam; the file-browser site was
  outside that task's literal "Do" list.

---

## Analysis

### Initial Hypotheses

The file-browser alias-save flow (`handleAliasSave`) is a third production
alias-mutation site still using the un-audited `Set` + `Save` two-step. Its
`AliasSaver` interface omits the audited `SetAndSave` method, so the breadcrumb
cannot be emitted. Suspected fix: extend `AliasSaver` to expose `SetAndSave`,
thread `handleAliasSave` onto it, and update the file-browser mock accordingly.

### Code Trace

**The defect site — `internal/ui/browser.go`:**

- `handleAliasSave` (`browser.go:248`) is the `a`-key alias-save flow. Its
  command closure (`browser.go:263-278`) performs the **un-audited two-step**:
  ```go
  if _, err := store.Load(); err != nil { ... }
  store.Set(name, resolved)        // browser.go:272 — in-memory only
  if err := store.Save(); err != nil { ... }  // browser.go:273 — writes file, no breadcrumb
  ```
- The `AliasSaver` interface (`browser.go:44-49`) exposes only
  `Load() / Set(name,path) / Save() error` — the audited `SetAndSave` seam is
  **structurally unreachable** from this caller.

**The audited seam that should be used — `internal/alias/store.go`:**

- `(*Store).SetAndSave(name, path, via string) error` (`store.go:170`) is the
  chokepoint: classifies the op from the pre-mutation map (`set` / `modify` /
  `set-noop`), performs the in-memory `Set`, runs `Save`, and emits **exactly
  one** breadcrumb under the `aliases` component (INFO on success, WARN with
  `error_class` on persist failure, DEBUG `set-noop` skipping the write). This
  is the spec's "single place per file where the breadcrumb can't be forgotten."
- The raw `(*Store).Set` (`store.go:136`) and `(*Store).Save` (`store.go:109`)
  are deliberately breadcrumb-free; `Save` writes the **entire** in-memory map
  (`s.List()`), so a `Load()` must precede any mutation to avoid clobbering
  other aliases — this is why `handleAliasSave` and the precedent callers all
  `Load` first.

**Precedent callers already on the audited seam (Task 3-5):**

- `cmd/alias.go:81` → `store.SetAndSave(name, normalised, "cli")` (store
  pre-loaded by `loadAliasStore`, which calls `Load` internally at
  `cmd/alias.go:96`).
- `internal/tui/model.go:1951` → `m.aliasEditor.SetAndSave(newAlias,
  m.editProject.Path, "cli")` (projects-edit modal). Its `AliasEditor`
  interface (`model.go:106`) already requires `SetAndSave` — the exact shape to
  mirror onto `AliasSaver`.

**Key files involved:**

- `internal/ui/browser.go` — defect site (`handleAliasSave` + `AliasSaver`).
- `internal/alias/store.go` — the audited `SetAndSave` seam (target).
- `internal/ui/browser_test.go` — `mockAliasSaver` (`:716`) implements
  `Load`/`Set`/`Save`; the only production-adjacent constructor call
  `NewFileBrowserWithAlias` lives here (`:754`), see below.
- `cmd/alias.go`, `internal/tui/model.go` — precedent for the fix.

### Reachability finding (diverges from the seed's framing)

**`handleAliasSave` is NOT currently reachable in production — the defect is
latent, not active.** The seed/discovery called it "a *third* production
alias-mutation site"; the code trace shows it is a *would-be* site that is not
wired in:

1. The only production file-browser construction is
   `internal/tui/model.go:1653` → `ui.NewFileBrowser(m.startPath, m.dirLister)`
   — the **plain** constructor, which leaves `aliasStore == nil`.
2. The `a`-key handler (`browser.go:210`) is gated on `m.aliasStore != nil`, so
   in the production TUI pressing `a` just appends `a` to the filter text; the
   alias prompt never opens and `handleAliasSave` never runs.
3. `NewFileBrowserWithAlias` (`browser.go:102`) — the only constructor that
   injects an alias store and enables the `a`-key prompt — is called **only**
   from `internal/ui/browser_test.go:754`.
4. Git confirms intent: the introducing commit `160d1e9c` ("file browser alias
   shortcut", Feb 2026) touched only `browser.go` + its test; it never wired
   the constructor into the TUI or `cmd/open.go`. `cmd/open.go` loads the alias
   store and passes it as the TUI's `aliasEditor` (the audited projects-modal
   path), not into the file browser.

So today **no alias is created via this flow**, which is exactly why the missing
breadcrumb produced no observable symptom — and why Task 3-5 (which converts
observable mutation sites) had no failing-grep signal pointing here.

### Root Cause

`handleAliasSave` was written (commit `160d1e9c`, Feb 2026) **before** the
observability layer's audited `SetAndSave` seam existed (`portal-observability-layer`,
June 2026), using the then-only-available `Load`→`Set`→`Save` two-step. The
`AliasSaver` interface was shaped around those three methods. When
`portal-observability-layer` Task 3-5 introduced `SetAndSave` as the
audit chokepoint and converted the alias-mutation callers, it migrated the two
**reachable** callers (`cmd/alias.go`, `internal/tui/model.go`) but left this
caller on the un-audited two-step. The interface's omission of `SetAndSave`
makes the audited path structurally impossible to reach from the file browser.

**Why this happens:** the audit-trail guarantee depends on every mutation
flowing through the store's combined `SetAndSave` chokepoint. A caller that
holds an interface exposing the raw `Set`+`Save` primitives can bypass the
chokepoint entirely — which is what `AliasSaver` permits and `handleAliasSave`
does.

### Contributing Factors

- **Interface predates the seam.** `AliasSaver` was minted before `SetAndSave`
  existed; nothing forced it to adopt the audited method when the seam shipped.
- **Out of Task 3-5's literal scope.** Task 3-5's "Do" list named only
  `cmd/alias.go` and `internal/tui/model.go`; this site wasn't enumerated.
- **No production reachability = no symptom.** Because the flow is dead in prod,
  there was no missing-breadcrumb grep failure to catch it.

### Why It Wasn't Caught

- The flow is unreachable in production, so no integration/manual path exercises
  it — the missing breadcrumb is invisible.
- `browser_test.go`'s `mockAliasSaver` mirrors the un-audited interface
  (`Load`/`Set`/`Save`) and asserts only the final map state, not log output —
  so the tests pass while the breadcrumb is absent.
- The observability spec's audit-trail acceptance focused on the store-method
  chokepoint and the named reachable callers, not on auditing every interface
  that fronts the alias store.

### Blast Radius (of the decided fix — full removal)

**Two packages deleted entirely:**
- `internal/ui/` — `browser.go`, `browser_test.go`, `testmain_isolation_test.go`.
  The package contains *nothing but* the file browser, so the whole directory
  goes.
- `internal/browser/` — `listing.go` (+ `listing_test.go`). Its only consumers
  are the file browser (`internal/ui/browser.go`) and `cmd/open.go`'s
  `osDirLister` — which itself exists solely to feed the file browser. With the
  browser gone it has zero consumers → removable.

**`internal/tui/model.go` — excise file-browser integration:**
- `internal/ui` import (l.20).
- `pageFileBrowser` from the page-state iota (l.34) + its two switch arms
  (update l.1544-1545, view l.2269-2270). (Go renumbers the iota automatically;
  no other page constant needs editing.)
- `type DirLister = ui.DirLister` alias (l.120).
- fields `dirLister DirLister` (l.189), `startPath string` (l.190),
  `fileBrowser ui.FileBrowserModel` (l.194).
- `WithDirLister` Option (l.575-579).
- `ui.BrowserDirSelectedMsg` / `ui.BrowserCancelMsg` handlers (l.1363-1365).
- `handleBrowseKey()` (l.1649-1655) + the `b`-key dispatch on the Projects page
  (l.1638).
- `updateFileBrowser()` (l.1971-1977).

**`cmd/open.go` — remove wiring:**
- `osDirLister` type + `ListDirectories` (l.333-339).
- `internal/browser` import (becomes unused).
- `dirLister tui.DirLister` field in `tuiConfig` (l.349).
- `tui.WithDirLister(cfg.dirLister, cfg.cwd)` (l.370).
- `dirLister: &osDirLister{}` (l.505).
- (`cfg.cwd` STAYS — still used by `WithCWD` + the lazy dir-resolution fallback.)

**Tests to remove/update:**
- `internal/ui/*_test.go`, `internal/browser/listing_test.go` (deleted with
  their packages).
- `cmd/open_test.go` — `osDirLister` / `browser.DirEntry` references.
- `internal/tui/model_test.go` — `mockDirLister` / `browser.DirEntry` (l.15-16,
  l.20, l.1671, l.2377), the `b`-key / `pageFileBrowser` paths, and the
  bracket-bindings comment referencing the browser (l.14).

**Docs:**
- `CLAUDE.md` — drop the `ui` and `browser` rows from the internal-packages
  table; update the TUI page state machine line (remove `FileBrowser` from
  `Loading → Sessions → Projects → FileBrowser → Preview`); remove the `b`-to-
  browse mention.

**NOT affected (independent — must stay green):** the alias CLI
(`cmd/alias.go`, `portal alias set/rm/list`), the projects-modal alias editor
(`internal/tui/model.go` `aliasEditor` → `SetAndSave`), the resolver chain
(path → alias → zoxide → TUI filter fallback), and the Sessions/Projects/Preview
pages. None depend on the file browser.

---

## Fix Direction

### Chosen Approach — REMOVE the file browser feature in full

Decided with the user at findings review (2026-06-09). The reported "bug" (alias
save emits no breadcrumb) is on **unreachable dead code**, so a behaviour-
preserving audit-fix would only polish code that never runs. The user confirmed
they never use the file browser (reachable only via Projects-page `b`) and want
it gone. **The fix for this bug is to delete the file-browser feature** — which
resolves the latent audit-bypass by removal and reclaims two dead packages. No
`SetAndSave` rewiring is needed.

Execute the Blast-Radius removal above: delete `internal/ui` + `internal/browser`,
excise the TUI integration and `cmd/open.go` wiring, remove/adjust the affected
tests, and update `CLAUDE.md`.

### Options considered (findings review)

- **(A) Audit-fix in place** — route `handleAliasSave` onto `SetAndSave`. Rejected:
  polishes code that never executes; leaves unused surface area.
- **(B) Wire it up + finish the feature** — swap in `NewFileBrowserWithAlias`,
  handle the saved/error messages (confirmation flash), audited save. Rejected:
  the user doesn't want the feature; this is net-new feature work.
- **(C) Remove the feature entirely — CHOSEN.** Deletes the bug by deletion,
  removes two dead packages, no behaviour change to anything the user uses.

### Work-type categorization (decided)

Keep `work_type: bugfix`. Rationale: valid types are
`epic / feature / bugfix / cross-cutting / quick-fix`; there is no "removal"
type, and **bugfix is the only type with an Investigation phase** — which we have
already completed. Re-typing to quick-fix or feature would orphan this
investigation (those pipelines never read it) and force re-seeding the findings
by hand. A bugfix legitimately concluding "the fix is deletion" is the cleanest,
lowest-friction framing; the removal's blast radius (two packages + TUI state-
machine surgery) also wants the spec/planning/review rigor that quick-fix skips.

### Testing Recommendations

- After removal: `go build ./...` and `go test ./...` green with no dangling
  references to `ui` / `browser` / `pageFileBrowser` / `DirLister`.
- Spot-check that the Projects page no longer reacts to `b` and that the
  Sessions/Projects/Preview pages and alias CLI / projects-modal alias editor
  are unchanged.
- Net test delta is removal, not addition (the deleted packages take their own
  tests with them).

### Risk Assessment

- **Fix complexity:** Low–Medium — mechanical deletion, but spread across two
  packages + the central TUI model + cmd wiring + docs; easy to leave a dangling
  reference, so a compile + full-test pass is the gate.
- **Regression risk:** Low — all removed code is unreachable in production except
  the `b` keybinding, which the user confirmed they never use.
- **Recommended approach:** Regular release.

---

## Notes

Seed flagged this as a clear, narrowly-scoped bugfix. Investigation corrected one
material fact: the alias-save site is **not** a live production mutation site — it
is unreachable dead code, so the original "missing breadcrumb" framing was a
latent landmine, not an active defect. The findings review then broadened: the
user confirmed the whole file browser is unused, and the decided fix is **full
removal** (option C) rather than auditing dead code. Kept as `work_type: bugfix`
because that is the only pipeline with the Investigation phase already done here.

Adjacent facts established during review (not part of this fix, captured for
context): the alias system **is** wired and functional and takes priority over
zoxide in the resolver chain; the user simply had no matching alias for the names
they tested. A noted UX sharp edge (out of scope): an exact-match alias miss
silently degrades to a fuzzy zoxide search, which can open a *different*
directory than intended with no indication the alias was skipped.

The earlier severity note still holds: severity is Low, and the fix-scope
question (A/B/C above) was resolved at findings review in favour of (C) removal.
