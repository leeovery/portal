---
phase: 3
phase_name: Mode toggle, persistence & empty/filter states
total: 9
---

## session-tagging-and-grouping-3-1 | approved

### Task session-tagging-and-grouping-3-1: prefs.json store — read/write session_list_mode with tolerant decode

**Problem**: The feature must remember the user's last-used grouping mode across launches ("if I always open in tag view I don't want to keep switching to it"). UI state does not belong in a domain store like `projects.json`, so a small dedicated prefs file is needed. No store exists for it today.

**Solution**: Add a new leaf package `internal/prefs` with a `Store` that reads and writes a single string-enum key (`session_list_mode`) to `prefs.json`, mirroring the `internal/project` store pattern: JSON file, `fileutil.AtomicWrite` (temp file + rename) on save, and a tolerant decode that collapses every degenerate input (missing file, empty file, corrupt/unparseable JSON, unrecognised enum value) to the Flat default with no hard error. The mode is modelled as a typed enum with canonical string values `flat` / `by-project` / `by-tag`.

**Outcome**: `prefs.NewStore(path).Load()` returns the persisted mode for a valid file, returns Flat for a missing/empty/corrupt/unrecognised file (no error), and `Save(mode)` atomically writes `{"session_list_mode": "<value>"}`; a by-tag value written by `Save` round-trips back through `Load` as the by-tag mode.

**Do**:
- Create `internal/prefs/store.go` in a new package `prefs`. Model the mode as a typed enum, e.g. `type SessionListMode int` with `const ( ModeFlat SessionListMode = iota; ModeByProject; ModeByTag )`, plus a `String()` / canonical-string mapping (`flat` / `by-project` / `by-tag`) and a `parseMode(string) SessionListMode` that returns `ModeFlat` for any unrecognised value. Keep the enum the single source of truth for the three modes — the TUI will reuse this type (task 3-3 imports it), so it must live in `prefs` (a leaf package the TUI may import), not in `cmd`.
- On-disk shape: a struct `prefsFile struct { SessionListMode string \`json:"session_list_mode"\` }`, matching the spec's concrete schema. Marshal with `json.MarshalIndent` for human readability (mirror `project.Store.Save`).
- `func NewStore(path string) *Store` and `func (s *Store) Load() (SessionListMode, error)`:
  - `os.ReadFile`; on `os.ErrNotExist` return `ModeFlat, nil` (first-run is the normal state — never an error).
  - On a read error other than not-exist, return `ModeFlat` plus the error (let the caller decide; the TUI wiring in task 3-9 treats any non-nil as Flat).
  - On `json.Unmarshal` failure (corrupt/unparseable), return `ModeFlat, nil` — tolerant decode, matching `project.Store.Load` which swallows malformed JSON and returns the empty/default state.
  - Map the decoded string through `parseMode`; an unrecognised value yields `ModeFlat`.
- `func (s *Store) Save(mode SessionListMode) error`: marshal `prefsFile{SessionListMode: mode.String()}` and persist via `fileutil.AtomicWrite(s.path, data)` (creates the parent dir, temp+rename). Return the AtomicWrite error verbatim so the caller can decide non-fatality (task 3-4 swallows it).
- Do NOT add audit/breadcrumb logging in v1 — `prefs.json` is not part of the closed state-mutation audit-trail set (`hooks` / `aliases` / `projects`), and adding a new log component requires a spec amendment. Keep the store a pure leaf (imports only `encoding/json`, `errors`, `os`, and `internal/fileutil`); this also keeps it importable from `internal/tui` without an import cycle.

**Acceptance Criteria**:
- [ ] `Load()` on a missing file returns `ModeFlat` and a nil error.
- [ ] `Load()` on an empty file returns `ModeFlat` and a nil error.
- [ ] `Load()` on corrupt/unparseable JSON returns `ModeFlat` and a nil error.
- [ ] `Load()` on a valid file with an unrecognised `session_list_mode` value returns `ModeFlat` and a nil error.
- [ ] `Save(ModeByTag)` then `Load()` round-trips to `ModeByTag`; same for `ModeByProject`.
- [ ] `Save` writes via `fileutil.AtomicWrite` (a temp file is created then renamed; the final file contains `{"session_list_mode": "by-tag"}` for `ModeByTag`).
- [ ] The canonical string mapping is exactly `flat` / `by-project` / `by-tag`.

**Tests**:
- `"it returns Flat for a missing prefs file"`
- `"it returns Flat for an empty prefs file"`
- `"it returns Flat for corrupt unparseable JSON"`
- `"it returns Flat for an unrecognised mode value"`
- `"it round-trips by-tag through Save and Load"`
- `"it round-trips by-project through Save and Load"`
- `"it writes session_list_mode atomically via AtomicWrite"`

**Edge Cases**:
- Missing file → Flat (first-run normal state, not an error).
- Empty file → Flat.
- Corrupt/unparseable JSON → Flat (tolerant decode, no hard error).
- Unrecognised mode value (valid JSON, bad enum) → Flat.
- Valid `by-tag` / `by-project` round-trip.

**Context**:
> Persistence target: a small prefs file under `~/.config/portal/`, using the existing `configFilePath` + `AtomicWrite` pattern. UI state does not belong in domain stores like `projects.json`.
> Format & schema: a JSON object with a single string-enum key, e.g. `{"session_list_mode": "flat" | "by-project" | "by-tag"}`. String enum (not int) so the file stays human-readable and stable.
> Decode-failure behaviour: a missing, empty, corrupt, or unparseable prefs file (or an unrecognised mode value) falls back to Flat and is treated as first-launch — consistent with the other stores' tolerant decode behaviour. No hard error; a missing file is the normal first-run state.

The mirror is `internal/project/store.go` (`Load` swallows malformed JSON → empty state; `Save` → `fileutil.AtomicWrite`).

**Spec Reference**: `.workflows/session-tagging-and-grouping/specification/session-tagging-and-grouping/specification.md` (§ Mode Persistence & Empty States → Remember last mode, Concrete shape)

## session-tagging-and-grouping-3-2 | approved

### Task session-tagging-and-grouping-3-2: Resolve prefs.json via configFilePath + migrateConfigFile

**Problem**: `prefs.json` must resolve through the same path-resolution machinery as the other config files — per-file env-var override → `XDG_CONFIG_HOME/portal/` → `~/.config/portal/` — and participate in the one-shot `migrateConfigFile` move from the old macOS path. Today `configFileComponents` (`cmd/config.go`) knows only `hooks.json` / `aliases` / `projects.json`, so a `prefs.json` resolution would migrate silently with an empty owning component.

**Solution**: Register `prefs.json` in `cmd/config.go`'s `configFileComponents` map and add a `prefsFilePath()` helper plus a `loadPrefsStore()` constructor in the `cmd` package, mirroring `projectsFilePath()` / `loadProjectStore()`. Resolution flows through the existing `configFilePath("PORTAL_PREFS_FILE", "prefs.json")`, which already performs env-override-first, XDG, `~/.config` fallback, and the migrate side-effect.

**Outcome**: `prefsFilePath()` returns the env-var override when `PORTAL_PREFS_FILE` is set, else `$XDG_CONFIG_HOME/portal/prefs.json`, else `~/.config/portal/prefs.json`; a `prefs.json` present only at the old macOS path is moved once to the new path (never overwriting an existing new-path file); `loadPrefsStore()` returns a `*prefs.Store` bound to that path.

**Do**:
- In `cmd/config.go`, add `"prefs.json": "prefs"` to the `configFileComponents` map. NOTE the divergence to flag: the map's doc comment says it is "the closed filename → owning-component mapping for the in-scope user-config files (the state-mutation audit-trail set)". `prefs.json` is NOT part of the audit-trail set (task 3-1 deliberately adds no breadcrumb logging, and `prefs` is not one of the 15 closed log components). Registering it here is purely to give `migrateConfigFile` a non-empty component string so the one-shot migrate emission is attributable — but there is no `prefs` log component. Resolve this explicitly: either (a) leave the component value as `"prefs"` and accept that the migrate breadcrumb would log under a component name not in the closed catalog (a spec gap — flag it, do not silently introduce a new log component), or (b) map `prefs.json` to `""` so `migrateConfigFile` suppresses the migrate emission entirely (the empty-component guard already exists and is the documented behaviour for unmapped filenames). Prefer (b): map `prefs.json` to `""` (or simply leave it unmapped and rely on `configFileComponents[filename]` returning `""`), which keeps the migrate move running best-effort while suppressing a log emission under a non-catalogued component. Document this choice in a code comment at the map so a future reader understands why `prefs.json` is intentionally unmapped/empty.
- Add `func prefsFilePath() (string, error) { return configFilePath("PORTAL_PREFS_FILE", "prefs.json") }` in the `cmd` package (e.g. alongside `projectsFilePath` in `cmd/clean.go`, or in `cmd/open.go` near the other store loaders — pick the file that keeps it close to its sole caller, `loadPrefsStore`).
- Add `func loadPrefsStore() (*prefs.Store, error)` mirroring `loadProjectStore()`: resolve `prefsFilePath()`, return `prefs.NewStore(path), nil`. Import `internal/prefs`.
- Confirm the migrate behaviour is exercised: a `prefs.json` only at `~/Library/Application Support/portal/` and absent at the new path is moved; a `prefs.json` present at both paths is NOT overwritten (the existing `migrateConfigFile` not-exist guard handles this). The env-override path must bypass migration entirely (existing `configFilePath` behaviour — env var returns early before the migrate call).

**Acceptance Criteria**:
- [ ] `prefsFilePath()` returns `$PORTAL_PREFS_FILE` verbatim when that env var is set and non-empty.
- [ ] With no env var and `XDG_CONFIG_HOME` set, `prefsFilePath()` returns `$XDG_CONFIG_HOME/portal/prefs.json`.
- [ ] With neither env var, `prefsFilePath()` returns `$HOME/.config/portal/prefs.json`.
- [ ] A `prefs.json` present only at the old macOS path is migrated to the new path on resolution; a `prefs.json` present at both paths is not overwritten.
- [ ] The env-override path does not trigger migration (returns before the migrate side-effect).
- [ ] `prefs.json` is registered in `configFileComponents` (mapped to `""` / left unmapped per the documented choice) so `migrateConfigFile` does not emit under a non-catalogued log component.
- [ ] `loadPrefsStore()` returns a `*prefs.Store` bound to the resolved path.

**Tests**:
- `"it returns the PORTAL_PREFS_FILE override when set"`
- `"it returns the XDG path for prefs.json when XDG_CONFIG_HOME is set"`
- `"it falls back to ~/.config/portal/prefs.json"`
- `"it migrates prefs.json from the old macOS path when the new path is absent"`
- `"it does not overwrite an existing prefs.json at the new path"`
- `"it does not migrate when the env override is set"`

**Edge Cases**:
- Per-file env-var override wins over XDG and `~/.config`.
- `XDG_CONFIG_HOME` set vs unset.
- Old macOS path present, new path absent → migrate; both present → no overwrite.
- Migrate suppressed (or non-logging) because `prefs` is not an audit-trail component.

**Context**:
> Filename: `prefs.json`, resolved through `configFilePath` exactly like the other config files (per-file env-var override → `XDG_CONFIG_HOME/portal/` → `~/.config/portal/`). It participates in the same `migrateConfigFile` one-shot move convention as `projects.json` / `aliases` / `hooks.json`.

`migrateConfigFile`'s empty-component guard (`cmd/config.go`) already suppresses emission for an unmapped filename while still running the move best-effort — that is the seam used to keep `prefs.json` out of the closed log-component catalogue.

**Spec Reference**: `.workflows/session-tagging-and-grouping/specification/session-tagging-and-grouping/specification.md` (§ Mode Persistence & Empty States → Concrete shape → Filename)

## session-tagging-and-grouping-3-3 | approved

### Task session-tagging-and-grouping-3-3: Mode-aware session re-render core on the model

**Problem**: The Phase 2 grouping builders (`buildByProject` / `buildByTag` in `internal/tui/grouping.go`) and the flat builder (`ToListItems`) exist independently, but nothing on the `Model` selects between them and rebuilds the session list for the active mode. The model has no notion of a current grouping mode, and `applySessions` (`model.go:~796`) unconditionally routes through `ToListItems` (the Flat builder). A single mode-aware re-render core is needed so the `s` toggle (task 3-4) and the `SessionsMsg` refresh path both produce the correct grouped slice.

**Solution**: Add a `sessionListMode prefs.SessionListMode` field to `Model` and a single mode-aware re-render method that, given the live (inside-tmux-filtered) sessions and the loaded project records, dispatches to the correct builder and pushes the resulting `[]list.Item` into `sessionList` via the existing `SetItems` + size-reapply sequence. Route `applySessions` through this core so every ingestion (initial load and `SessionsMsg` refresh) respects the active mode. The builders need project records, so the model must hold (or be able to load) the projects slice at re-render time.

**Outcome**: With `sessionListMode == ModeFlat`, the model's session list is built via `ToListItems` (byte-for-byte today's flat items); with `ModeByProject` it is built via `buildByProject(sessions, projects)`; with `ModeByTag` via `buildByTag(sessions, projects)`; a `SessionsMsg` refresh re-applies the currently-active mode rather than reverting to Flat; re-rendering in the same mode with the same inputs is idempotent (no panic, stable item slice); zero live sessions produces an empty (or signpost-only, per task 3-7) list in every mode without error.

**Do**:
- Add `sessionListMode prefs.SessionListMode` to the `Model` struct (`internal/tui/model.go`). Zero value is `ModeFlat`, so a model constructed without the new option behaves exactly as today (additive, no-regression). Import `internal/prefs` (the enum's home — confirmed leaf, no cycle).
- The grouping builders require `[]project.Project`. The model already holds a `projectStore ProjectStore` (with `List()`). Decide and document the project-records source: either (a) the model loads project records lazily at re-render time via `m.projectStore.List()`, or (b) the model caches the last-loaded projects slice on a field updated by the existing `ProjectsLoadedMsg` handler. Prefer (b) — the model already loads projects (`ProjectsLoadedMsg` populates the projects page), so cache that slice on a `projects []project.Project` field and feed it to the builders. This avoids a synchronous store read inside the render path and keeps the builders pure. Confirm where `ProjectsLoadedMsg` is handled (`model.go:~1077`) and capture the slice there.
- Add `func (m *Model) rebuildSessionList() tea.Cmd` (or fold into a mode-aware variant of `applySessions`). It must:
  - Compute the inside-tmux-filtered sessions via the existing `m.filteredSessions()`.
  - Switch on `m.sessionListMode`: `ModeFlat` → `ToListItems(filtered)`; `ModeByProject` → `buildByProject(filtered, m.projects)`; `ModeByTag` → `buildByTag(filtered, m.projects)`.
  - Call `m.sessionList.SetItems(items)` and re-apply the list size (the existing `if m.termWidth > 0 || m.termHeight > 0 { m.applySessionListSize(...) }` tail from `applySessions`) so pagination accounts for the manual footer.
- Refactor `applySessions` (`model.go:~796`) to store the session slice and then delegate item construction to the mode-aware core (so it is the single ingestion chokepoint for all three modes). Keep the existing `m.sessions = sessions` assignment and the size-reapply. The handler-specific tail logic (e.g. the inside-tmux title rewrite at the `SessionsMsg` call site) stays at the call site, unchanged for now (task 3-5 reconciles the title).
- Do NOT wire the `s` key here (task 3-4) and do NOT read prefs here (task 3-9 injects the initial mode). This task only adds the field + the dispatch core and routes ingestion through it; the field defaults to `ModeFlat`, so behaviour is unchanged until task 3-4/3-9 flip it.
- This is unit-testable by setting `m.sessionListMode` directly (or via a test helper), seeding `m.sessions` and `m.projects`, calling the re-render core, and asserting the resulting `SessionListItems()` shape (flat vs grouped headings via the item `GroupKey`/`GroupHeading` fields). No real tmux is needed.

**Acceptance Criteria**:
- [ ] `Model` has a `sessionListMode prefs.SessionListMode` field whose zero value is `ModeFlat`.
- [ ] The re-render core dispatches to `ToListItems` for Flat, `buildByProject` for By Project, `buildByTag` for By Tag.
- [ ] `applySessions` routes through the mode-aware core (single ingestion chokepoint); Flat-mode output is byte-for-byte today's flat items.
- [ ] A `SessionsMsg` refresh preserves the active mode (does not revert to Flat).
- [ ] The model holds project records (cached from `ProjectsLoadedMsg`) and feeds them to the grouping builders.
- [ ] Zero live sessions produces an empty list in every mode without panic.
- [ ] Re-rendering in the same mode with the same inputs is idempotent.

**Tests**:
- `"it builds flat items when the mode is Flat"`
- `"it builds By Project items when the mode is By Project"`
- `"it builds By Tag items when the mode is By Tag"`
- `"it preserves the active mode across a SessionsMsg refresh"`
- `"it feeds cached project records to the grouping builders"`
- `"it produces an empty list for zero live sessions in every mode"`
- `"it is idempotent when re-rendering the same mode with the same inputs"`

**Edge Cases**:
- Zero live sessions per mode (empty list, no panic).
- Mode-unchanged idempotent re-render.
- Correct builder selected per mode.
- `SessionsMsg` refresh re-applies the active mode (not Flat).

**Context**:
> The session list cycles through three modes: Flat (today's list), By Project, By Tag. Tags are read live from `projects.json` at grouped-render time (no per-session tag cache).
> On the projects-edit → sessions-page transition, dispatch a sessions-list refresh that re-resolves project records and re-groups (Phase 4 leans on this same re-render core).

Phase 2 API the dispatch consumes: `buildByProject(sessions []tmux.Session, projects []project.Project) []list.Item` and `buildByTag(...)` (both in `internal/tui/grouping.go`), and the enriched `SessionItem` (`GroupKey`/`GroupHeading`/`Tag`/`CatchAll`). `applySessions` is the existing chokepoint (`model.go:~796`).

**Spec Reference**: `.workflows/session-tagging-and-grouping/specification/session-tagging-and-grouping/specification.md` (§ Grouping Semantics → Modes; § Item model)

## session-tagging-and-grouping-3-4 | approved

### Task session-tagging-and-grouping-3-4: `s` cycle key handler (Flat → By Project → By Tag → Flat)

**Problem**: The user needs a single key that cycles the session list through Flat → By Project → By Tag → Flat unconditionally (any session count including zero, any tag count) and persists the new mode on each press. The `s` key is verified free in browse mode on the sessions page, but no handler binds it, and while the `/` filter input is focused `s` must remain a literal filter character.

**Solution**: Add an `s` case to the sessions-page browse-mode rune switch in `updateSessionList` that advances `m.sessionListMode` one step in the fixed cycle, re-renders the list via the task 3-3 re-render core, and persists the new mode via an injected persister seam (task 3-9 wires the concrete `prefs.Store`). Crucially, the case goes INSIDE the rune switch that begins at `model.go:~1583` — AFTER the `m.sessionList.SettingFilter()` early-return guard at `~1558` — so that while the filter input is focused, `s` falls through to the list and is typed as a literal filter character.

**Outcome**: Pressing `s` in browse mode advances Flat→By Project→By Tag→Flat (wrapping By Tag→Flat), rebuilds the session list for the new mode, and writes the new mode to `prefs.json` exactly once per press; the cycle fires regardless of session count (including zero) and regardless of tag count; while the `/` filter input is focused, `s` is typed into the filter text and does not cycle; a persist failure does not abort the toggle (the in-memory mode still advances and the list still re-renders).

**Do**:
- Add a method `func (m Model) handleSwitchViewKey() (tea.Model, tea.Cmd)` (mirroring the `handleKillKey` / `handleRenameKey` shape). It must:
  - Advance the mode: `m.sessionListMode = nextSessionListMode(m.sessionListMode)` where `nextSessionListMode` cycles `ModeFlat → ModeByProject → ModeByTag → ModeFlat`. Define `nextSessionListMode` next to the enum or in the model; it wraps unconditionally.
  - Re-render via the task 3-3 core (`m.rebuildSessionList()` or the mode-aware ingestion), capturing the returned `tea.Cmd`.
  - Persist: call the injected persister seam (e.g. `m.modePersister.Save(m.sessionListMode)` — see the seam note below). Swallow a non-nil error (best-effort, non-fatal — the spec pins "persisted on each toggle press"; a write failure must not break the toggle). If the persister is nil (tests that do not wire it), skip the call.
  - Return `m, cmd` (the re-render cmd from `SetItems`).
- Wire the case INSIDE the rune switch at `model.go:~1583`, e.g. `case isRuneKey(msg, "s"): return m.handleSwitchViewKey()`. Place it among the existing rune cases (`q`/`k`/`r`/`n`/`p`/`x`). It MUST NOT be added above the `if m.sessionList.SettingFilter() { break }` guard at `~1558` — that guard is what makes `s` a literal filter character while the filter input is focused. Add a code comment at the case pinning this ordering requirement so a later refactor cannot hoist it above the guard.
- Persister seam: introduce a tiny interface on the model, e.g. `type ModePersister interface { Save(prefs.SessionListMode) error }`, and a `modePersister ModePersister` field + a `WithModePersister(p ModePersister) Option`. The production wiring (task 3-9) passes the `*prefs.Store` (its `Save(SessionListMode) error` already satisfies the interface). The model must NOT import `cmd` or construct the store itself — it only holds the seam. This keeps the model free of config-path/store-construction concerns (the model never imports `internal/prefs` for I/O; it imports `prefs` only for the `SessionListMode` type, which is a pure value type).
- Persist exactly once per press: assert the persister is called exactly once per `s` keystroke (not on every re-render, not on `SessionsMsg`). Only `handleSwitchViewKey` calls `Save`.

**Acceptance Criteria**:
- [ ] `s` in browse mode advances Flat → By Project → By Tag → Flat (wrapping By Tag → Flat).
- [ ] The cycle fires on zero live sessions and on zero tags (unconditional).
- [ ] Each `s` press persists the new mode exactly once via the persister seam.
- [ ] While the `/` filter input is focused (`SettingFilter()` true), `s` is a literal filter character and does NOT cycle the mode.
- [ ] A persist failure does not abort the toggle: the in-memory mode advances and the list re-renders regardless.
- [ ] A nil persister is tolerated (no call, no panic) for tests that do not wire it.
- [ ] The `s` case is inside the rune switch (below the `SettingFilter()` guard), never above it.

**Tests**:
- `"it cycles Flat to By Project to By Tag to Flat on successive s presses"`
- `"it cycles unconditionally with zero live sessions"`
- `"it cycles unconditionally with zero tags"`
- `"it persists the new mode exactly once per s press"`
- `"it treats s as a literal filter character while the filter input is focused"`
- `"it advances the mode even when persistence fails"`
- `"it tolerates a nil persister"`

**Edge Cases**:
- Cycle wraps By Tag → Flat.
- Unconditional on zero sessions / zero tags.
- `s` literal while filter focused (below the `SettingFilter()` guard).
- Persist once per press; persist failure non-fatal.
- Nil persister tolerated.

**Context**:
> A single key cycles the mode: each press advances Flat → By Project → By Tag → Flat. The cycle is unconditional — By Tag is never skipped (even with zero tags it lands on the signposted By-Tag state). The cycle is also unconditional on session count, including zero live sessions. `s` cycles modes and writes the new mode to `prefs.json` regardless of how many sessions are live.
> `s` while the `/` filter input is active: when the filter input is focused and capturing keystrokes, `s` is a literal filter character — it does not cycle the mode mid-typing.
> Write timing: persisted on each toggle press via `AtomicWrite`.

Anchor: the browse-mode handler returns early at `m.sessionList.SettingFilter()` (`model.go:~1558`) before the rune switch (`~1583`). The `s` case goes inside that switch.

**Spec Reference**: `.workflows/session-tagging-and-grouping/specification/session-tagging-and-grouping/specification.md` (§ TUI Rendering & Toggle Behaviour → Toggle key — `s`, single cycle; § Mode Persistence & Empty States → Write timing)

## session-tagging-and-grouping-3-5 | approved

### Task session-tagging-and-grouping-3-5: Mode-aware title via SessionListTitle()

**Problem**: The active grouping mode must be visible in the session list title: `Sessions` (Flat), `Sessions — by project` (By Project), `Sessions — by tag` (By Tag). The title is the `bubbles/list` `Title` field, currently set to the literal `"Sessions"` in `newSessionList` and OVERWRITTEN to `Sessions (current: %s)` at two inside-tmux sites (`model.go:~416` in `WithInsideTmux` and `~1041` in the `SessionsMsg` handler). The spec's title scheme does not account for the existing inside-tmux current-session decoration, so a reconciliation is required rather than a blind overwrite.

**Solution**: Add a single title-computation function that maps the active mode to the spec title and reconciles it with the inside-tmux current-session suffix, then call it from every site that sets `m.sessionList.Title` (the Flat default in `newSessionList`, the `WithInsideTmux` site, and the `SessionsMsg` handler) and after each mode change so the title tracks the mode. The reconciliation: keep the existing `(current: %s)` decoration and append/compose the mode suffix, producing a deterministic combined title (e.g. `Sessions — by tag (current: foo)`), rather than dropping either piece.

**Outcome**: In Flat mode the title is `Sessions` (unchanged from today's baseline when not inside tmux); in By Project it is `Sessions — by project`; in By Tag it is `Sessions — by tag`; the title updates immediately on an `s` mode change and on a `SessionsMsg` refresh; when inside tmux, the current-session decoration is preserved alongside the mode suffix in a single deterministic combined form (the divergence between the spec's flat title scheme and the existing inside-tmux title is reconciled explicitly, not silently overwritten).

**Do**:
- Add `func sessionListTitleForMode(mode prefs.SessionListMode, insideTmux bool, currentSession string) string` (package `tui`). Base title by mode: `ModeFlat` → `"Sessions"`, `ModeByProject` → `"Sessions — by project"`, `ModeByTag` → `"Sessions — by tag"` (use the exact em-dash-spaced strings from the spec). When `insideTmux && currentSession != ""`, append the existing decoration so it reads e.g. `Sessions — by tag (current: foo)`. FLAG THE DIVERGENCE in a code comment: the spec's title scheme (§ Mode indication) only specifies the three base strings and does not mention the pre-existing `(current: %s)` decoration; this function preserves that decoration as a suffix rather than dropping it, which is the minimal reconciliation that satisfies both the spec's mode-indication requirement and the existing inside-tmux behaviour. If the team later wants a different composition, this is the single place to change it.
- Replace the three current title-set sites:
  - `newSessionList` (`model.go:~559`): instead of the literal `l.Title = "Sessions"`, leave the default `"Sessions"` (Flat, not-inside-tmux) — but prefer routing through the helper once the model knows its mode. Since `newSessionList` runs before mode/inside-tmux are known, keep its default `"Sessions"` and let the model set the correct title once construction options apply.
  - `WithInsideTmux` (`model.go:~416`): replace `m.sessionList.Title = fmt.Sprintf("Sessions (current: %s)", currentSession)` with `m.sessionList.Title = sessionListTitleForMode(m.sessionListMode, true, currentSession)`.
  - `SessionsMsg` handler (`model.go:~1041`): replace the inside-tmux `Sessions (current: %s)` rewrite with `m.sessionList.Title = sessionListTitleForMode(m.sessionListMode, m.insideTmux, m.currentSession)` — and call this unconditionally (not only inside-tmux) so a refresh in By Project / By Tag also shows the right base title. Confirm the existing `if m.insideTmux && m.currentSession != ""` block is widened so the title is always recomputed for the active mode.
- In `handleSwitchViewKey` (task 3-4), after advancing the mode, set `m.sessionList.Title = sessionListTitleForMode(m.sessionListMode, m.insideTmux, m.currentSession)` so the title flips on each `s` press. (Alternatively centralise the title set inside the task 3-3 re-render core so both the toggle and the refresh get it for free — prefer this if it keeps the set in one place; document where the canonical set lives.)
- Initial title at construction: ensure the model sets the title for its initial mode (task 3-9 injects the persisted mode). If the model opens in By Tag, the first paint must show `Sessions — by tag`. Apply `sessionListTitleForMode` once after options are applied (e.g. in `New` after the option loop, or in the mode-injection option).
- The existing `SessionListTitle()` accessor (`model.go:~254`) returns `m.sessionList.Title` and is the test seam — assert against it.

**Acceptance Criteria**:
- [ ] Flat mode title is `Sessions` (not inside tmux) — unchanged from today's baseline.
- [ ] By Project title is `Sessions — by project`; By Tag title is `Sessions — by tag`.
- [ ] The title updates on an `s` mode change.
- [ ] The title updates on a `SessionsMsg` refresh (correct base title for the active mode).
- [ ] Inside tmux, the current-session decoration is preserved alongside the mode suffix in a single deterministic combined title (divergence reconciled, not silently overwritten).
- [ ] The reconciliation between the spec title scheme and the pre-existing inside-tmux title is documented in a code comment.

**Tests**:
- `"it shows Sessions for Flat mode"`
- `"it shows Sessions — by project for By Project mode"`
- `"it shows Sessions — by tag for By Tag mode"`
- `"it updates the title on a mode change"`
- `"it updates the title on a SessionsMsg refresh"`
- `"it preserves the current-session decoration alongside the mode suffix inside tmux"`

**Edge Cases**:
- Inside-tmux current-session title interaction (combined form, divergence flagged).
- Title updates on mode change and on `SessionsMsg` refresh.
- Flat title unchanged from baseline.

**Context**:
> Mode string lives in the title (top), via the existing `SessionListTitle()`: Flat → `Sessions`, By Project → `Sessions — by project`, By Tag → `Sessions — by tag`.

DIVERGENCE TO RECONCILE: the existing code sets `Sessions (current: %s)` at `model.go:~416` (`WithInsideTmux`) and `~1041` (`SessionsMsg`). The spec's title scheme does not mention this inside-tmux decoration. Do not silently overwrite it — compose mode suffix + current-session decoration deterministically and document the choice.

**Spec Reference**: `.workflows/session-tagging-and-grouping/specification/session-tagging-and-grouping/specification.md` (§ TUI Rendering & Toggle Behaviour → Mode indication)

## session-tagging-and-grouping-3-6 | approved

### Task session-tagging-and-grouping-3-6: Footer `s switch view` hint on the sessions page

**Problem**: The sessions-page footer must advertise the new toggle with an `s switch view` hint so the feature is discoverable, but only on the sessions page (not on projects, where `s` already means "go to sessions" via `projectHelpKeys`). The sessions footer is built from `sessionHelpKeys()` (`model.go:~524`), which feeds `sessionFooterBindings` and the three-column manual renderer.

**Solution**: Add an `s` → `switch view` `key.Binding` to `sessionHelpKeys()` so it appears in the sessions-page manual keymap footer at all session counts, without touching `projectHelpKeys` (which already binds `s`/`x` → `sessions`). The three-column chunking (`chunkBindingsIntoThreeColumns`) and fixed `keymapFooterColumnSize` absorb the extra entry automatically.

**Outcome**: The sessions-page footer shows an `s switch view` entry alongside the existing session keys (`enter`/`r`/`k`/`p`-`x`/`n`/`space`/`q`); the projects-page footer is unchanged (still shows `s/x sessions`); the footer renders correctly at every session count (including zero); the three-column layout remains intact (no column overflow or misalignment introduced by the new entry).

**Do**:
- In `sessionHelpKeys()` (`model.go:~524`), add `key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "switch view"))`. Choose its position in the slice to read naturally in the manual footer columns — e.g. after `p/x projects` or near the navigation/action cluster. The column split is purely positional (`keymapFooterColumnSize = 5`, three columns in source order), so position determines which column it lands in; pick a position that keeps the three columns balanced and document the reasoning in a brief comment if non-obvious.
- Do NOT modify `projectHelpKeys()` — the projects page keeps `s/x sessions`. The spec explicitly accepts the page-dependent meaning of `s` (projects: go to sessions; sessions: cycle view).
- Do NOT add `s` to `commandPendingHelpKeys()` — command-pending mode lands on the projects page and the toggle is a sessions-page action.
- Confirm the footer still renders at zero sessions: the footer is rendered by `renderKeymapFooter(&m.sessionList, sessionFooterBindings(&m.sessionList))` (`viewSessionList`, `model.go:~1888`) independent of item count, so the hint is present even with an empty list. Add a test asserting the rendered sessions footer contains `switch view` at zero and non-zero session counts.
- Verify the three-column layout is unbroken: `chunkBindingsIntoThreeColumns` filters disabled bindings then splits in source order. The new binding is always enabled, so it adds one visible entry. Assert the rendered footer still produces three columns and includes the new hint (a render-string contains-check on `viewSessionList()` output is sufficient).

**Acceptance Criteria**:
- [ ] The sessions-page footer includes an `s switch view` entry.
- [ ] The projects-page footer is unchanged (still `s/x sessions`).
- [ ] The hint is present at all session counts, including zero.
- [ ] The three-column footer layout remains intact (no overflow/misalignment).
- [ ] `commandPendingHelpKeys` is unchanged (no `s` hint in command-pending mode).

**Tests**:
- `"it shows the s switch view hint in the sessions footer"`
- `"it shows the switch view hint at zero sessions"`
- `"it leaves the projects footer unchanged with s/x sessions"`
- `"it keeps the sessions footer three-column layout intact"`

**Edge Cases**:
- Absent on the projects page (page-dependent `s` meaning).
- Footer column layout unbroken.
- Present at all session counts (including zero).

**Context**:
> Key hint lives in the footer (Portal convention — keys at the bottom). Only the `s switch view` entry is added to the footer. The mode state lives in the title, off the crowded footer.
> Minor accepted wrinkle: `s` already means "go to sessions" on the projects page. Same letter, page-dependent meaning — judged fine.

Anchors: `sessionHelpKeys()` (`model.go:~524`) feeds `sessionFooterBindings` only; the rendered footer comes from `viewSessionList` (`model.go:~1888`).

**Spec Reference**: `.workflows/session-tagging-and-grouping/specification/session-tagging-and-grouping/specification.md` (§ TUI Rendering & Toggle Behaviour → Mode indication, Toggle key)

## session-tagging-and-grouping-3-7 | approved

### Task session-tagging-and-grouping-3-7: By-Tag zero-tags "No tags yet" signpost

**Problem**: When By Tag mode is active but zero tags exist anywhere (the union of all project `tags` is empty), the view must NOT silently flatten — it must render the plain (ungrouped) session list with an explicit "No tags yet" signpost so the user sees they are in tag view, understands why there are no groups, and knows where to add them (the projects page). This is a degrade-to-flat WITH a message, distinct from the catch-all `Untagged` bucket (which appears when tags exist but a session is untagged).

**Solution**: Detect the "zero tags anywhere" condition at By-Tag re-render time (no project record carries any tag) and, when true, build the session list as a plain flat slice (so the list itself is unchanged from Flat) while surfacing a "No tags yet" signpost in the view. The cycle is unaffected — the persisted mode stays `by-tag` and one `s` press advances to Flat as normal; reopening in persisted By-Tag with zero tags shows the signpost identically.

**Outcome**: In By Tag mode with zero tags anywhere, the session list renders as the plain ungrouped list (no group headings) with a visible "No tags yet" signpost; the mode remains `by-tag` (a subsequent `s` advances to Flat); reopening Portal in a persisted By-Tag mode with zero tags shows the signpost; when at least one tag exists anywhere the signpost is NOT shown (normal By-Tag grouping with the `Untagged` catch-all renders instead).

**Do**:
- Add a predicate, e.g. `func anyTagsExist(projects []project.Project) bool`, that returns true if any project's `Tags` slice is non-empty. (Tags are canonical/lower-cased from Phase 1; presence is all that matters here.) This is the "zero tags anywhere" gate.
- In the task 3-3 re-render core, when `m.sessionListMode == ModeByTag` AND `!anyTagsExist(m.projects)`: build the list with the FLAT builder (`ToListItems(filtered)`) — NOT `buildByTag` — so the rendered list is byte-for-byte the plain session list (no `Untagged` heading either, since there is nothing to group). Set a model flag (e.g. `m.byTagSignpost = true`) so the view can render the signpost. When tags exist, clear the flag and use `buildByTag` as normal.
- Render the signpost: surface "No tags yet" in the sessions view. Decide and document the placement — the model already has an inline-flash mechanism rendered between the title/filter row and the list (`renderFlashRow` / `flashText`, `model.go:~1875`), but the flash is transient (cleared on the next actionable key), so it is the WRONG vehicle for a persistent mode signpost. Prefer a dedicated, persistent signpost row: render a styled "No tags yet" line in `viewSessionList` when `m.byTagSignpost` is true (e.g. a dimmed line near the title, or an explicit message that also hints "add tags on the projects page"). Keep it visually distinct from the transient flash and do not reuse `flashText`. Document the exact wording and placement in a code comment; the spec pins the message intent ("No tags yet" plus "knows where to add them — the projects page") so include a short hint to the projects page.
- The signpost must coexist with the normal sessions chrome (title, filter row, footer) without overlaying or replacing them — mirror the additive insertion approach used by the flash row (split the list view's first line and insert the signpost row beneath it), but as a persistent row gated on `m.byTagSignpost`.
- Confirm cycle semantics: the signposted state IS By-Tag mode (`m.sessionListMode == ModeByTag`); the `s` handler (task 3-4) advances it to Flat with one press regardless of the signpost. Add a test: from the signposted By-Tag state, one `s` press yields `ModeFlat` and clears the signpost.
- Reopen-persisted path: when task 3-9 injects an initial `ModeByTag` and zero tags exist, the first re-render must show the signpost. Add a test seeding `ModeByTag` + zero-tag projects and asserting the signpost is shown and the list is the plain flat slice.
- Do NOT trigger the signpost when tags exist but all live sessions happen to be tagged (that is the empty-`Untagged`-suppression case from Phase 2 task 2-4, not the zero-tags-anywhere case). The gate is strictly "no project carries any tag", independent of which sessions are live.

**Acceptance Criteria**:
- [ ] By Tag mode with zero tags anywhere renders the plain ungrouped session list (no group headings) plus a "No tags yet" signpost.
- [ ] It is a degrade-with-message, not a silent flatten (the signpost is visible and points to the projects page).
- [ ] The persisted mode stays `by-tag` in the signposted state; one `s` press advances to Flat and clears the signpost.
- [ ] Reopening in a persisted By-Tag mode with zero tags shows the signpost.
- [ ] When at least one tag exists anywhere, the signpost is NOT shown (normal By-Tag grouping renders).
- [ ] The "tags exist but all live sessions are tagged" case does NOT trigger the signpost (that is empty-`Untagged` suppression, not zero-tags-anywhere).

**Tests**:
- `"it shows the No tags yet signpost in By Tag mode with zero tags anywhere"`
- `"it renders the plain flat session list under the signpost (no Untagged heading)"`
- `"it advances By Tag to Flat with one s press from the signposted state"`
- `"it shows the signpost when reopening in persisted By Tag with zero tags"`
- `"it does not show the signpost when at least one tag exists"`
- `"it does not show the signpost when tags exist but all live sessions are tagged"`

**Edge Cases**:
- Zero tags anywhere → signpost (degrade-with-message, not silent flatten).
- Reopen persisted By-Tag with zero tags → signpost.
- One `s` advances to Flat (cycle unaffected).
- Tags-exist-all-sessions-tagged does NOT trigger the signpost.

**Context**:
> By Tag with zero tags — does not silently flatten. Render the plain (ungrouped) session list with an explicit "No tags yet" message, so the user sees they are in tag view, understands why there are no groups, and knows where to add them (the projects page). This is a degrade-to-flat with a signpost, not a silent one.
> The cycle is unconditional — By Tag is never skipped. Landing on it shows the "No tags yet" signposted state. For cycling purposes the signposted state is the By-Tag mode (the persisted mode is `by-tag`); one `s` press from there advances to Flat. The same holds if Portal reopens in a persisted By-Tag mode with zero tags.

This is distinct from the empty-`Untagged`-suppression case (Phase 2 task 2-4): that fires when tags exist but every live session is tagged; this fires only when no project carries any tag.

**Spec Reference**: `.workflows/session-tagging-and-grouping/specification/session-tagging-and-grouping/specification.md` (§ Mode Persistence & Empty States → Empty states → By Tag with zero tags; § TUI Rendering & Toggle Behaviour → Toggle key)

## session-tagging-and-grouping-3-8 | approved

### Task session-tagging-and-grouping-3-8: Flatten-on-filter and restore-grouping-on-clear

**Problem**: While the `/` filter is active in a grouped mode, the list must flatten to matching sessions with group headers stepping aside; clearing the filter must restore the grouped view. Portal does NOT own the filter — `bubbles/list`'s built-in filter re-ranks matches into a relevance-sorted flat list, which scrambles any grouped layout. The render-layer design (every list item is a session instance; headers are render-layer separators) is what makes flatten-on-filter fall out cheaply, but the header-injecting delegate (Phase 2 task 2-5) injects headings at group-key boundaries — those boundaries must not produce headers while filtering.

**Solution**: Make the header-injecting delegate (and/or the re-render core) suppress group headings while a filter is active, so the built-in filter's relevance-sorted flat result renders as a plain hit-list (headers absent); when the filter clears, the grouped view restores per the current mode. Because the filter only ever sees session instances (no header items), no custom filter-aware item manipulation is needed — the only change is to stop drawing headings while filtering. The exact suppression mechanism must be flagged: Portal does not own the filter, so the delegate must read the list's filter state to decide whether to draw headings.

**Outcome**: In By Project or By Tag mode, activating the `/` filter flattens the list to matching sessions with NO group headers visible; clearing the filter restores the grouped (headed) view for the current mode; in Flat mode the filter behaves exactly as today (no change); the transition is driven by the list's filter state so it tracks both the typing (`Filtering`) and committed (`FilterApplied`) phases and the cleared (`Unfiltered`) phase.

**Do**:
- Identify where the delegate decides to draw a heading (Phase 2 task 2-5's `SessionDelegate.Render`, `internal/tui/session_item.go`). The delegate already has access to the `list.Model` (`m` parameter). Add a guard: when the list is filtering or has a filter applied, suppress heading injection (draw only the session line). FLAG THE MECHANISM: Portal does not own the filter, so the delegate must infer "filter active" from the `list.Model` it is handed — read `m.FilterState()` (`list.Filtering` while typing, `list.FilterApplied` when committed, `list.Unfiltered` when cleared) and suppress headings for `Filtering` and `FilterApplied`. Confirm the delegate's `m` is the live list model (it is — `Render(w, m, index, item)` receives the model), so `m.FilterState()` is readable per-render with no extra plumbing. Document that this is the single point where flatten-on-filter is realised, and that it relies on the render-layer invariant (no header items) so the filter's flat ranked result needs no further massaging.
- Verify the built-in filter sees only session instances: because `FilterValue()` returns the session name (Phase 2 task 2-1) and there are no header items, the filter's relevance-sorted result is already a valid flat session hit-list. No re-grouping or item rebuild is needed while filtering. Assert this: with a filter applied in By Project mode, `SessionListVisibleItems()` contains only `SessionItem`s and the rendered output contains no heading lines.
- Restore on clear: when the filter state returns to `Unfiltered`, the delegate resumes drawing headings, so the grouped view restores automatically against the still-grouped underlying item slice (the slice was never un-grouped — only the heading drawing was suppressed). Confirm no re-render/rebuild is required on clear: the underlying `m.sessionList.Items()` is still the pre-sorted grouped slice from the active mode, so simply ceasing heading-suppression restores the grouped look. Add a test: apply a filter (headers absent), clear it (`ClearFilter`), assert headers return for the active mode.
- Consider the interaction with By-Tag duplicate instances: in By Tag mode the underlying slice has one item per `(session, tag)` pair, so a filtered result may show the same session more than once (once per tag instance that matches). This is acceptable per the spec (flatten-on-filter uses the existing built-in filter unchanged; the item slice is whatever the active mode built). Document this and add a note/test that filtering in By Tag mode may surface duplicate session rows (one per matching instance) — this is expected, not a defect, in v1.
- Flat-mode no-change: in Flat mode the slice has no group keys, so heading injection never fires regardless of filter state — assert filtering in Flat mode is byte-for-byte today's behaviour.
- Do NOT build a custom filter, custom input state, or live re-group per keystroke (the spec explicitly defers "keep groups live while filtering" as a separate feature and forbids Portal owning the filter in v1). The only change is heading suppression keyed off the list's filter state.

**Acceptance Criteria**:
- [ ] In By Project / By Tag mode, an active filter flattens the list to matching sessions with NO group headers visible.
- [ ] Clearing the filter restores the grouped (headed) view for the current mode.
- [ ] Flat-mode filtering is unchanged from today (no group keys, so no headings regardless of filter state).
- [ ] Re-grouping on clear respects the current mode (By Project restores project headings, By Tag restores tag headings).
- [ ] Heading suppression tracks both the `Filtering` (typing) and `FilterApplied` (committed) states, and headings return on `Unfiltered`.
- [ ] The built-in filter sees only session instances (no header items); no custom filter is introduced.
- [ ] By-Tag filtering may surface duplicate session rows (one per matching instance) — documented as expected.

**Tests**:
- `"it suppresses group headers while a filter is active in By Project mode"`
- `"it suppresses group headers while a filter is active in By Tag mode"`
- `"it restores group headers when the filter is cleared"`
- `"it leaves Flat-mode filtering unchanged"`
- `"it restores the current mode's headings on clear (By Tag tags, not By Project)"`
- `"it suppresses headings during both Filtering and FilterApplied states"`
- `"it may surface duplicate session rows when filtering in By Tag mode"`

**Edge Cases**:
- Filter active flattens; headers absent while filtering.
- Clear restores grouping (respecting the current mode).
- Flat-mode filter unchanged.
- `FilterApplied` vs `Filtering` transitions both suppress headings; `Unfiltered` restores them.
- By-Tag duplicate rows under filter (expected).

**Context**:
> While a filter is active, the list flattens to the matching sessions using the existing built-in filter; group headers step aside. Clearing the filter restores the grouped view. There is no behaviour change to filtering as it works today.
> Build note: grouping must be a render-layer concern — every `bubbles/list` item is a session instance, and group headings are injected at render time as visual separators, never as list items. The built-in filter only ever sees session instances (flatten-on-filter is trivial).
> Keeping groups live during filtering would require Portal to own the filter wholesale — deferred as its own separate feature. v1 flattens on filter.

MECHANISM TO FLAG: Portal does not own the filter. The delegate must read the live `list.Model`'s `FilterState()` to decide whether to draw headings — this is the single seam through which flatten-on-filter is realised, and it works only because the render-layer invariant guarantees no header items leak into the filter's view.

**Spec Reference**: `.workflows/session-tagging-and-grouping/specification/session-tagging-and-grouping/specification.md` (§ Filter Composition → Flatten-on-filter, Why not keep groups live while filtering, Build note)

## session-tagging-and-grouping-3-9 | approved

### Task session-tagging-and-grouping-3-9: Wire prefs-backed initial mode + persister into TUI construction (open.go Option)

**Problem**: The TUI must open in the user's persisted grouping mode (Flat on first-ever launch, the last-used mode thereafter) and must persist on each toggle. The model holds the mode field (task 3-3), the toggle handler (task 3-4), and the persister seam (task 3-4), but nothing reads `prefs.json` at construction time or injects the initial mode + a concrete persister. The model must NOT import `internal/prefs` for I/O — the prefs read and the persister live in `cmd/open.go` and are passed in as seams.

**Solution**: In `cmd/open.go`, load the prefs store (task 3-2's `loadPrefsStore`) once at TUI construction, read the persisted initial mode via `Store.Load()` (tolerant — any error or unrecognised value yields Flat), and inject both the initial mode and the store-as-persister into the model via new functional options (`tui.WithInitialMode` + `tui.WithModePersister`). The `*prefs.Store` satisfies the `tui.ModePersister` seam (its `Save(SessionListMode) error` matches), so the same store instance serves both reads and the per-toggle writes.

**Outcome**: A first-ever launch (no `prefs.json`) opens in Flat; after toggling to By Tag and reopening, Portal opens in By Tag; a corrupt/unparseable `prefs.json` opens in Flat (treated as first-launch, no hard error); pressing `s` end-to-end writes the new mode to `prefs.json` via the injected persister; tests that do not wire a persister (nil) run without panicking and default to Flat.

**Do**:
- In `cmd/open.go`'s `openTUI` (near the other store loads, ~`loadProjectStore` at `~416`, and the `stateDir` resolution at `~440`): call `loadPrefsStore()` (task 3-2). On error, log/swallow and proceed with a nil store + Flat default (a prefs path-resolution failure must not block opening the TUI — consistent with the tolerant-decode contract). Then `initialMode := prefs.ModeFlat; if prefsStore != nil { initialMode, _ = prefsStore.Load() }` — `Load` already collapses every degenerate case to `ModeFlat`, so the discarded error is acceptable; add a comment noting the tolerance.
- Thread the values into `tuiConfig` (`cmd/open.go:~340`): add `initialMode prefs.SessionListMode` and `modePersister tui.ModePersister` (the `*prefs.Store`) fields, set them in the `cfg :=` block (`~462`). Guard the persister assignment so a nil store leaves `modePersister` nil.
- In `buildTUIModel` (`cmd/open.go:~359`), append the new options when set: `opts = append(opts, tui.WithInitialMode(cfg.initialMode))` always (Flat is a valid explicit value), and `if cfg.modePersister != nil { opts = append(opts, tui.WithModePersister(cfg.modePersister)) }`.
- Add the two options in `internal/tui/model.go` (next to the other `With…` options, ~`424`–`520`):
  - `func WithInitialMode(mode prefs.SessionListMode) Option` → sets `m.sessionListMode = mode` and sets the initial title via `sessionListTitleForMode` (task 3-5) so the first paint reflects the mode. (If task 3-5 centralises the title set in the re-render core, ensure the initial mode is applied before the first ingestion so the title and item slice agree.)
  - `func WithModePersister(p ModePersister) Option` → sets `m.modePersister = p`.
- Construction ordering: the persisted mode must be applied BEFORE the first session ingestion so the first render groups correctly. Confirm the order in `New` (`model.go:~716`): options apply in slice order in the `for _, opt := range opts` loop; ensure the initial-mode option does not depend on sessions already being loaded (sessions arrive later via `SessionsMsg`, and the task 3-3 re-render core re-applies the mode on every ingestion, so the first `SessionsMsg` will group per the injected mode). Add a test that constructs a model with `WithInitialMode(ModeByTag)` then feeds a `SessionsMsg` and asserts the resulting items are By-Tag grouped (or signposted, per task 3-7).
- End-to-end persister test: construct a model with a fake `ModePersister` (recording `Save` calls), press `s`, and assert `Save` was called once with the advanced mode. Optionally an integration-style test using a real `*prefs.Store` against an isolated temp path (use `portaltest.IsolateStateForTest` / a `PORTAL_PREFS_FILE` override) that presses `s` and re-reads `prefs.json` to confirm the persisted value — but the seam-level fake test is the primary coverage; the model must not import `internal/prefs` for I/O.
- Nil-persister tolerance: tests constructing the model without `WithModePersister` must toggle without panicking (task 3-4 already guards nil) and default to Flat. Assert this.

**Acceptance Criteria**:
- [ ] A first-ever launch (no `prefs.json`) opens the TUI in Flat.
- [ ] After persisting By Tag, a fresh construction opens in By Tag (initial mode read from `prefs.json`).
- [ ] A corrupt/unparseable `prefs.json` opens in Flat (no hard error).
- [ ] `buildTUIModel` injects the initial mode and the persister; a nil persister is tolerated.
- [ ] Pressing `s` end-to-end persists the new mode through the injected persister (verified with a recording fake; optionally a real-store round-trip).
- [ ] The model never imports `internal/prefs` for I/O — only the `SessionListMode` value type; the prefs read and the persister are constructed in `cmd/open.go`.
- [ ] The initial mode is applied before the first session ingestion so the first render groups correctly (title and item slice agree).

**Tests**:
- `"it opens in Flat for a first-ever launch with no prefs file"`
- `"it opens in By Tag after By Tag was persisted"`
- `"it opens in Flat for a corrupt prefs file"`
- `"it injects the initial mode and groups the first SessionsMsg accordingly"`
- `"it persists the new mode through the injected persister on an s press"`
- `"it tolerates a nil persister in tests"`

**Edge Cases**:
- First-ever launch opens Flat.
- Persisted by-tag opens By Tag.
- Corrupt prefs opens Flat.
- Persister writes on toggle end-to-end.
- Nil persister tolerated in tests.

**Context**:
> First-ever launch defaults to Flat (zero surprise), and remembers thereafter.
> Persisted on each toggle press via `AtomicWrite`. Decode-failure behaviour: a missing, empty, corrupt, or unparseable prefs file (or an unrecognised mode value) falls back to Flat and is treated as first-launch.

Anchors: `buildTUIModel` (`cmd/open.go:~359`), the `tuiConfig` struct (`~340`), `cfg :=` block (`~462`), `stateDir` resolution (`~440`), and the model `Option` pattern (`model.go:~424`–`520`, applied in `New` at `~716`). The model holds the persister as a `ModePersister` seam (task 3-4) — `*prefs.Store` satisfies it.

**Spec Reference**: `.workflows/session-tagging-and-grouping/specification/session-tagging-and-grouping/specification.md` (§ Mode Persistence & Empty States → Remember last mode, Concrete shape)
