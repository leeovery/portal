---
status: complete
created: 2026-07-11
cycle: 5
phase: Gap Analysis
topic: restore-host-terminal-windows
---

# Review Tracking: restore-host-terminal-windows - Gap Analysis

All 11 findings resolved via auto (finding_gate_mode = auto). Load-bearing calls are flagged; the full set was summarised to the user at the cycle-5 convergence gate.

## Findings

### 1. Multi-select selection semantics in By-Tag mode (multi-row sessions) undefined

**Priority**: Important
**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Multi-Select Mode (Granularity / Sticky selection) × Session grouping (By Tag = Pattern B)

**Details**: In By Tag mode a multi-tag session renders as multiple rows; the spec never reconciled per-session marking with multi-row rendering (mark the session vs the row; count once vs per row).

**Resolution**: Approved
**Notes**: Marking is keyed on **session identity, not the list row**: toggling any one row marks the session, `●` shows on all its rows, `N selected` counts distinct sessions, marks survive regroup/filter. Added to §Multi-Select Mode.

---

### 2. Intra-config match precedence undefined when multiple `terminals.json` entries match one identity

**Priority**: Important
**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Config Schema (Precedence) × Adapter Contract

**Details**: Precedence between layers (config→native→unsupported) is defined, but not within config when alias/bundle-id/.app/`*`-glob all match one identity.

**Resolution**: Approved
**Notes**: **Judgment call:** most-specific-wins — exact bundle id → exact `.app`/alias → `*`-glob (longer glob beats broader; bare `*` lowest). A specific override always beats a glob fallback. Added to §Config Schema → Precedence.

---

### 3. Config-recipe failure classification into the typed taxonomy unspecified

**Priority**: Important
**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Config Schema × Permissions & Error Quarantine

**Details**: Portal can't read AppleEvent codes from an arbitrary config recipe; how does a recipe outcome map to `permission-required`/`spawn-failed`?

**Resolution**: Approved
**Notes**: Non-zero recipe exit → `spawn-failed`; otherwise the ack decides (timeout → failed). `permission-required` is native-adapter-only — config recipes never trigger the burst-stopping permission path. Added to §Config Schema → recipe execution contract.

---

### 4. Env-injection set beyond PATH, and the TMUX-must-be-absent invariant, unspecified

**Priority**: Important
**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Spawn Architecture (Spawned-window environment) × Trigger-Context Matrix

**Details**: `[any other required vars]` was a literal placeholder; and the "spawned N−1 run out of tmux" invariant silently depends on `TMUX` not being propagated (an inside-tmux picker has `TMUX` set).

**Resolution**: Approved
**Notes**: **Load-bearing:** inject the minimal set (PATH only; additions named explicitly, never a whole-env snapshot); **`TMUX`/`TMUX_PANE` MUST NOT be propagated** — explicitly stripped so the spawned `portal attach` takes the fresh exec-attach path, not `switch-client`. Added to §Spawn Architecture env paragraph.

---

### 5. `portal spawn` CLI reporting and exit-code semantics for the non-`--detect` path unspecified

**Priority**: Important
**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Spawn Architecture (CLI behaviour) × Burst × Testing

**Details**: The CLI (the test seam) had no defined stdout/stderr or exit codes for abort/partial-failure/permission/unsupported.

**Resolution**: Approved
**Notes**: Success self-execs away (no return); pre-flight abort / partial failure / unsupported N≥2 / permission → exit `1` + one-line stderr; usage error → exit `2`. Added a "Reporting & exit codes" block to the CLI subsection.

---

### 6. Input handling during the async in-picker burst underspecified

**Priority**: Important
**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Burst & Partial-Failure Contract (In-picker execution model)

**Details**: Cancel is defined mid-burst, but not what happens to other keys (mark/nav/second Enter) racing the completion handler's selection mutation.

**Resolution**: Approved
**Notes**: During the pending burst the picker is **input-locked to row actions** (m/nav/Space/`/`/`s`/second Enter ignored); only cancel is live — eliminates the race. Added to §In-picker execution model.

---

### 7. N≥2 Enter behaviour when detection is still in-flight (not yet cached) undefined

**Priority**: Important
**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Terminal Identity & Detection (lifecycle) × Unsupported-terminal behaviour

**Details**: Async detection introduces an in-flight state distinct from resolved-NULL; the N≥2 Enter gate had no path for it.

**Resolution**: Approved
**Notes**: The gate **awaits** an in-flight identity (near-instant) then proceeds (supported → spawn; NULL/error → unsupported no-op); never treated as unsupported merely for being unresolved. Added to §Detection lifecycle.

---

### 8. Notice-band single-slot arbiter precedence across all claimants not fully enumerated

**Priority**: Minor
**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Multi-Select Mode × Burst × Detection

**Details**: Many claimants now share the single slot; the unsupported-banner-during-mode case in particular was an implicit decision.

**Resolution**: Approved
**Notes**: Added a precedence list (filter → in-burst progress → error/guidance flash → multi-select banner → unsupported banner → no-tags signpost); on unsupported, the multi-select banner shows and the warning re-asserts at the N≥2 Enter block. Added to §Multi-Select mode affordance.

---

### 9. Ack-marker option name derivation from arbitrary session names unspecified

**Priority**: Minor (raised to load-bearing in resolution — latent bug)
**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Burst & Partial-Failure Contract (Ack channel) × Ack delivery contract

**Details**: `@portal-spawn-<batch>-<session>` derived from a renameable session name could yield an invalid tmux option name → `set-option` fails → false ack-timeout.

**Resolution**: Approved
**Notes**: Fixed a latent bug I introduced (marker keyed on session name). Marker now `@portal-spawn-<batch>-<token>` where `<batch>`/`<token>` are picker-generated **option-name-safe** ids; flag `--spawn-ack <batch>:<token>`; attach writes exactly that (no derivation from the session name). Restores the discussion's original "batch id + per-window token" design. Updated the token-ack bullet, Ack channel, Ack delivery contract, and the composed-command example.

---

### 10. `script` recipe execution mechanism (interpreter, exec bit, tilde) unspecified

**Priority**: Minor
**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Config Schema (Recipe execution contract)

**Details**: `script` recipes: direct-exec vs `sh`, exec bit, tilde expansion all unstated.

**Resolution**: Approved
**Notes**: Portal expands a leading `~` and executes the file directly (exec bit + shebang required); missing/non-executable → invalid entry (skip + WARN). Added to §Config Schema recipe bullet.

---

### 11. `opened` / `total` batch-summary count semantics ambiguous

**Priority**: Minor
**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Observability & State Footprint × Spawn Architecture (N vs N−1)

**Details**: `opened 11/14` ambiguous given the N vs N−1 split and the conditional self-attach.

**Resolution**: Approved
**Notes**: `total` = N (batch, incl. the trigger's self-attach target); `opened` = surfaced windows = acked spawns + the trigger self-attach when it occurs (full success = N/N; on failure the skipped self-attach is not counted). Added to §Observability attr keys.

---
