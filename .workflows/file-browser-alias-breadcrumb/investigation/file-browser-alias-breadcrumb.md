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

### Blast Radius

**Directly affected:**
- `internal/ui/browser.go` `handleAliasSave` + `AliasSaver` interface.
- `internal/ui/browser_test.go` `mockAliasSaver` + alias-save tests.

**Potentially affected:**
- Nothing in the current production runtime (flow is unreachable). The fix is a
  correctness/latent-landmine repair, not a behaviour change to a live path.
- If/when the file browser is later wired with a real alias store (the apparent
  original intent of `NewFileBrowserWithAlias`), the fix ensures alias creation
  is audited from day one rather than silently bypassing the trail.

---

## Fix Direction

### Chosen Approach

_(to be confirmed during findings review — see scope question below)_

Leading approach (matches the seed's recommendation): **route the file-browser
caller onto the audited seam, behaviour-preserving for the reachable surface.**

1. Extend the `AliasSaver` interface (`browser.go:44`) to require
   `SetAndSave(name, path, via string) error` (mirroring `tui.AliasEditor` at
   `model.go:106`). Drop `Set`/`Save` from the interface if no longer used
   (narrows the bypass surface); keep `Load` (still needed to populate the map
   before classification + to avoid clobbering on `Save`).
2. Rewrite `handleAliasSave` (`browser.go:263-278`) to `Load()` then
   `store.SetAndSave(name, resolved, "cli")` (`via=cli` — user-facing, matching
   both precedents), replacing the `Set`+`Save` pair. Map a non-nil return to
   `BrowserAliasSaveErrMsg`.
3. Update `mockAliasSaver` (`browser_test.go:716`) to implement `SetAndSave`
   (and drop `Set`/`Save` if removed from the interface); keep the existing
   map-state assertions green.

### Scope question to settle at findings review

Because the flow is **unreachable in production**, there are three coherent
scopes — the user should pick:

- **(A) Fix-in-place (recommended, = seal the latent landmine).** Do the three
  steps above. Removes the audit-bypass without making the dead shortcut live.
  Pure bugfix, behaviour-preserving.
- **(B) Fix-in-place + wire into production.** Also call
  `NewFileBrowserWithAlias` from `internal/tui/model.go:1653` so the `a`-key
  actually works. This is **feature work** (activating a dormant shortcut), not
  a bugfix — likely out of scope for this work unit.
- **(C) Delete the dead shortcut.** If the `a`-key file-browser alias feature is
  not wanted, remove `handleAliasSave` / `NewFileBrowserWithAlias` / the alias
  branch instead of auditing it. Resolves the landmine by deletion.

### Testing Recommendations

- Add a log-capture assertion (via the `logtest.Sink` pattern used in
  `internal/alias/store_logging_test.go`) proving the file-browser save path now
  emits one `aliases: set`/`modify` breadcrumb with `via=cli`.
- Keep existing `mockAliasSaver` map-state tests green after the interface
  change.
- If scope (B): an integration/TUI test that the `a`-key opens the prompt and
  persists + audits.

### Risk Assessment

- **Fix complexity:** Low (scope A) — interface + one call site + mock.
- **Regression risk:** Low — the only consumer is unreachable in prod; the
  change is mechanical and mirrors two existing precedents.
- **Recommended approach:** Regular release.

---

## Notes

Seed flagged this as a clear, narrowly-scoped bugfix. Investigation confirms the
defect (un-audited two-step + `AliasSaver` omitting `SetAndSave`) but corrects
one material fact: the site is **not** currently a live production mutation site
— it is unreachable, so the bug is a **latent landmine** rather than an active
missing-breadcrumb. This sharpens severity to Low and raises the fix-scope
question (A/B/C above) for the user.
