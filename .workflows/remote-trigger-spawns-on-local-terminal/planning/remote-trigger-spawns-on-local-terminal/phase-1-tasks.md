---
phase: 1
phase_name: Gate Locality on the Triggering (Most-Active) Client
total: 3
---

## remote-trigger-spawns-on-local-terminal-1-1 | approved

### Task 1-1: Reproduce the bug and invert the locality gate in detectInsideTmux

**Problem**: When a spawn burst is triggered from a **remote** tmux client (e.g. Blink on iPhone/iPad over SSH/mosh) while a **host-local** client (e.g. Ghostty on the Mac) is also attached to the *same session*, `detectInsideTmux` resolves the local terminal as the host and the N−1 spawned windows open on a machine the triggering user is not at. The root cause is an ordering bug: `detectInsideTmux` (`internal/spawn/detect_inside.go`) treats client *locality* as a pre-filter (drop every remote/mosh client whose walk resolves NULL) and client *activity* (`client_activity`) as a tiebreak applied only **among the surviving local clients**. The triggering remote client — which has the highest `client_activity` — is dropped by the NULL walk *before* its activity is ever consulted, so the local client becomes `best` and the burst treats the host terminal as supported. The function answers *"is there any host-local client on the session?"* rather than *"is the client that triggered this burst host-local?"*

**Solution**: Invert the filter-then-tiebreak order into **select-winner-then-locality-check**: pick the triggering client (the max-`client_activity` client across **all** enumerated clients — local and remote alike, first-listed winning an exact tie) over the existing `ListClients(session)` result, then walk **only that winner** and branch on the winner's locality (local `.app` → drive; clean NULL → honest no-op; transient walk failure → NULL + `ErrDetectTransient`). Rewrite the entire `detect_inside.go` docstring contract to describe the new algorithm and its owned fail-safe trade, and land the two codified-bug test transforms in the same commit (they go red under this single code change).

**Outcome**: A remote-triggered burst with a host-local client also on the session resolves NULL (the same atomic no-op as the pure-remote case) instead of driving windows onto the wrong machine, while a legitimate local trigger still drives. All existing pinned invariants stay green. `go test ./internal/spawn/...` and `go test ./...` pass.

**Do**:
- **Reproduce first (red):** Before touching production code, invert the two codified-bug subtests in `internal/spawn/detect_inside_test.go` so they encode the *correct* post-fix behaviour and fail against the current buggy code:
  - The `~:133` subtest `"it drops remote clients but still resolves a mixed local+remote client set"` — seeds a high-activity remote (`PID 601`, mosh → walks NULL, `Activity: 9999`) + a low-activity local (`PID 501`, Ghostty, `Activity: 1`) and currently asserts the **local** wins. Invert it: with the remote as most-active the winner is the remote, whose walk resolves a clean NULL, so the expectation becomes **NULL identity, nil error** (honest no-op). Rename the subtest to describe the new contract (e.g. `"it returns NULL when the most-active client is remote even with a local bystander"`). This subtest doubles as the failing-first regression for the reported bug.
  - The `~:196` subtest `"it resolves a local client despite a transient walk on another client"` — seeds a high-activity client whose walk transiently fails (`PID 601`, `Activity: 100`, `ProcessInfo` returns `psFailure`) + a lower-activity resolvable local (`PID 501`, Ghostty, `Activity: 50`) and currently asserts the local resolves with a nil error. Reframe it: under walk-only-the-winner the flaky high-activity client **is** the winner, so the expectation becomes **NULL identity + an `ErrDetectTransient`-wrapped error** that also wraps `psFailure` (assert `errors.Is(err, ErrDetectTransient)` and `errors.Is(err, psFailure)`, and `got.IsNull()`). Rename to describe the fail-safe (e.g. `"it fails safe to NULL when the most-active winner's walk transiently fails"`).
- **Implement the inversion** in `detectInsideTmux` (`internal/spawn/detect_inside.go`), keeping the `detectInsideTmux(session string, lister clientLister, walker ProcessWalker, reader BundleReader)` signature and the `clientLister` DI seam exactly as-is:
  1. Call `lister.ListClients(session)`; on error return `Identity{}, transient(...)` unchanged (`transient` from `walk.go`).
  2. If the returned client slice is empty, return `Identity{}, nil` (clean NULL — no winner to select).
  3. Otherwise select the winner: iterate the slice, keeping the client with the strictly-greatest `Activity`; on an exact tie the **first-listed** client wins (i.e. only replace the running best when `client.Activity > bestActivity`, seeded from the first element). Do **not** walk during selection.
  4. Walk **only the winner** via `walkToBundle(winner.PID, walker, reader)`:
     - non-nil error → return `Identity{}, werr` (the `ErrDetectTransient`-wrapped walk error, so `Detect()` folds it to a `spawn` WARN);
     - resolved non-NULL identity → return that `Identity, nil` (drive);
     - clean NULL (`id.IsNull()`, nil error) → return `Identity{}, nil` (honest no-op).
- **Rewrite the docstring contract** (roughly current lines 49–72 — the algorithm description AND the "Outcomes" list) to describe the new most-active-winner selection and winner-only walk with fail-safe-to-NULL-on-transient-winner. Remove the two directly-inverted sentences: `"NULL-filtering is the primary signal"` (~line 55) and `"client_activity is used ONLY to disambiguate among host-local clients — never as a cross-client primary signal"` (~lines 70–72). The rewritten docstring must **own** the dropped walk-resilience property: state explicitly that walking only the winner drops the old "one flaky `ps` cannot mask a resolvable local client" guarantee for the winner, and that a transient winner walk now fails safe to NULL + WARN (never spawn on uncertainty) — a deliberate trade of resilience for correctness. Do not leave stale contract text describing behaviour the code no longer has (the per-client scan-all loop and its `firstWalkErr` accumulator are gone).
- **Verify all pinned invariants stay green** (see Acceptance Criteria) — the RETAINED subtests (pure-remote NULL, single-local drive, 2+ all-local highest-activity, exact-tie first-listed, list-clients failure transient, single-client walk failure transient, empty → NULL) must all pass unchanged. In particular do **not** delete the single-client walk-failure subtest (`~:171`, `"it returns a transient error when a walk fails and nothing local resolves"`) as a supposed duplicate of the reframed `:196`: it is the single-client variant of the transient-winner case and its coverage is distinct.

**Acceptance Criteria**:
- [ ] Production change is confined to `internal/spawn/detect_inside.go` — `detect.go`, `detect_outside.go`, and the three `Detect()` consumers (`cmd/open_burst_run.go`, `internal/tui/spawn_detect.go`, `cmd/doctor.go`) are untouched.
- [ ] `detectInsideTmux(session, lister, walker, reader)` signature and the `clientLister` seam are unchanged.
- [ ] Winner selection is max-`Activity` across **all** clients (local and remote); an exact `Activity` tie resolves to the **first-listed** client; only the winner is walked (no all-clients scan loop remains).
- [ ] Mixed: remote most-active + idle local → NULL identity, nil error (bug fixed).
- [ ] Winner walk transient-fails → NULL identity + `ErrDetectTransient`-wrapped error that also wraps the underlying cause.
- [ ] Exact activity tie → first-listed client is the winner.
- [ ] `ListClients` enumeration failure → NULL + `ErrDetectTransient`-wrapped error (winner computed only after a successful enumeration).
- [ ] Empty client list → clean NULL, nil error (no transient).
- [ ] Single-client walk failure → NULL + `ErrDetectTransient`-wrapped error (retained, not deleted).
- [ ] Docstring rewritten: the two inverted sentences are removed, the new most-active-winner algorithm + winner-only walk + fail-safe-to-NULL trade are described, and no stale scan-all/`firstWalkErr` contract text remains.
- [ ] The `~:133` and `~:196` subtests are inverted/reframed in place (or renamed-replaced) to the new expectations; both fail against pre-fix code and pass against post-fix code.
- [ ] `go test ./internal/spawn/...` passes; `go test ./...` (unit lane) passes.

**Tests** (in `internal/spawn/detect_inside_test.go`):
- `"it returns NULL when the most-active client is remote even with a local bystander"` — inverted `~:133`: high-activity remote (`601`, `Activity 9999`) + low-activity local (`501`, `Activity 1`) → `got.IsNull()`, nil error (this is the primary regression for the reported bug).
- `"it fails safe to NULL when the most-active winner's walk transiently fails"` — reframed `~:196`: high-activity flaky client (`601`, `Activity 100`, `ProcessInfo` → `psFailure`) + lower-activity resolvable local (`501`, `Activity 50`) → `got.IsNull()`, `errors.Is(err, ErrDetectTransient)`, `errors.Is(err, psFailure)`.
- Retained green (must not regress): `"it returns NULL when every client is remote or mosh"` (`~:46`), `"it returns the single local client's identity without a tiebreak"` (`~:65`), `"it picks the highest-client_activity local client among 2+ locals"` (`~:83`), `"it picks the highest activity when the higher-activity client is listed first"` (`~:101`), `"it prefers the first local client on an exact activity tie"` (`~:117`), `"it returns a transient error when list-clients fails"` (`~:151`), `"it returns a transient error when a walk fails and nothing local resolves"` (`~:171`), `"it returns clean NULL for zero clients"` (`~:220`).

**Edge Cases**:
- **Empty client list → clean NULL, nil error** — no winner to select; distinct from a transient error.
- **Winner walk transient-fail → NULL + `ErrDetectTransient`** — the deliberately-dropped resilience property; never open windows on uncertainty.
- **Exact activity tie → first-listed wins** — deterministic rule; preserves existing multi-local tie-break behaviour. (The remote/local same-epoch-second tie is explicitly *don't-care* per spec Out of Scope, but the code must still apply the first-listed rule.)
- **`ListClients` enumeration failure → NULL + transient** — unaffected by the inversion (winner is computed only after a successful enumeration).
- **Single-client walk failure → NULL + transient (retained)** — with one client it is the winner, so its walk-failure maps to the winner-walk-transient row; keep this subtest (distinct from the multi-client reframed `:196`).
- **`client_activity` is epoch-seconds-granular** — not a defect and needs no workaround; only the source of the acknowledged same-or-later-second residual edge, which is explicitly out of scope.

**Context**:
> Current buggy loop (to be replaced) walks every client, drops NULL walks, and applies `client_activity` only among the surviving locals (`internal/spawn/detect_inside.go` lines 86–105). The validated mechanism (spec Root Cause): `client_activity` tracks a client's **sent input**, not received mirror redraws, and detection runs immediately after the user's trigger action (picker startup for the TUI, command entry for the CLI), so the just-bumped triggering client is reliably the freshest at detection time — making "most-active client on the session" finger the remote trigger. Delegating to tmux's own best-client resolution (`display-message -p '#{client_pid}'`) was considered and rejected: extra round-trip, no controllable tie-break, would restructure the seam. The behavioural change is "sometimes no-op where it used to drive, never drive where it shouldn't" — no new false-drive is possible.
> Behavioural outcomes by scenario (spec table):
> Pure remote → remote winner → NULL/no-op (unchanged); Single local → local winner → drive (unchanged); Mixed remote most-active + local idle → remote winner → NULL/no-op (bug fixed); Mixed local most-active + remote idle → local winner → drive (preserved); 2+ all-local → highest-activity local, first-listed on tie → drive (unchanged); `ListClients` fails → NULL + transient (unchanged); winner walk transient-fails → NULL + WARN (fail-safe); empty → clean NULL (unchanged).
> The test seams: `fakeClientLister` returns a fabricated `[]ClientActivity` (records sessions), `fakeWalker` is a `map[int]fakeProc` (`{ppid, command, err}`) recording call order, `fakeReader` is a `map[string]fakeBundle`. `localWalkSeams()` wires `501→Ghostty`, `502→Terminal` (both `.app`), `601/602→mosh-server` (walk to clean NULL). `ErrDetectTransient` and `transient(what, cause)` live in `internal/spawn/walk.go`; `walkToBundle(startPID, walker, reader)` has a three-shape return (resolved Identity/nil; NULL/nil; NULL/`ErrDetectTransient`-wrapped).

**Spec Reference**: `.workflows/remote-trigger-spawns-on-local-terminal/specification/remote-trigger-spawns-on-local-terminal/specification.md` — "Root Cause", "The Fix: Gate Locality on the Triggering (Most-Active) Client", "Owned Behaviour Change: Dropped Walk-Resilience Property", "Edge Contracts to Pin", "Testing Requirements".

## remote-trigger-spawns-on-local-terminal-1-2 | approved

### Task 1-2: Guard the fix against over-correction with a local-most-active regression test

**Problem**: The inversion in Task 1-1 could be over-applied so that a legitimate local spawn is refused merely because a remote client happens to be *attached* to the same session. No existing subtest covers the mirror of the reported bug — a **local** client that is the most-active with a **remote idle bystander** also attached — so nothing pins that this legitimate case still drives. This is the one genuinely net-new scenario (the other two target scenarios are the same as Task 1-1's two transforms and need no near-duplicate test).

**Solution**: Add a single net-new, test-only subtest to `internal/spawn/detect_inside_test.go` seeding a local most-active client plus a lower-activity remote bystander and asserting the **local drives** (resolves the local `.app` identity, nil error). It guards against over-correction; the scenario passes against both pre- and post-fix code, so it is a quality/prevention guard rather than the primary regression.

**Outcome**: `internal/spawn/detect_inside_test.go` gains a subtest proving that a local most-active client with an idle remote bystander still resolves the local host terminal, locking the "local trigger still drives" invariant. `go test ./internal/spawn/...` and `go test ./...` pass.

**Do**:
- Add a new subtest to `TestDetectInsideTmux` in `internal/spawn/detect_inside_test.go`, e.g. `"it drives the local client when it is most-active despite an idle remote bystander"`.
- Seed via `fakeClientLister`: a local client that is most-active (e.g. `{PID: 501, Activity: 200}` → Ghostty) and a lower-activity remote bystander (e.g. `{PID: 601, Activity: 50}` → mosh, walks to NULL). Order the slice so a passing test proves max-by-activity selection rather than order-luck (list the remote first, the local second — mirroring the existing `~:83` intent).
- Wire the walk/reader seams via `localWalkSeams()` (already maps `501→Ghostty` and `601→mosh-server`), so no bespoke fakes are needed.
- Assert the resolved identity is the local Ghostty (`got.BundleID == "com.mitchellh.ghostty"`, `got.Name == "Ghostty"`) with a nil error, and (optionally) `!got.IsNull()`.
- This task is **test-only** — no production code changes.

**Acceptance Criteria**:
- [ ] A new subtest exists in `internal/spawn/detect_inside_test.go` for the local-most-active + remote-idle-bystander scenario.
- [ ] It seeds a most-active local client and a lower-activity remote bystander, with the local **not** first in the slice (so the pass proves max-by-activity, not first-listed luck).
- [ ] It asserts the local `.app` identity resolves (`BundleID == "com.mitchellh.ghostty"`) with a nil error.
- [ ] No production files are modified by this task.
- [ ] The subtest passes against the post-fix `detectInsideTmux` (and — being an over-correction guard — also passes against pre-fix code).
- [ ] `go test ./internal/spawn/...` passes; `go test ./...` (unit lane) passes.

**Tests** (in `internal/spawn/detect_inside_test.go`):
- `"it drives the local client when it is most-active despite an idle remote bystander"` — remote bystander (`601`, mosh, `Activity 50`) listed first + local (`501`, Ghostty, `Activity 200`) listed second → winner is the local → resolves `com.mitchellh.ghostty`, nil error.

**Edge Cases**:
- **Remote idle bystander attached but local still drives** — the mirror of the reported bug; confirms the fix does not refuse a legitimate local spawn just because a remote client is attached.

**Context**:
> Spec Testing Requirements: "Local most-active, remote idle bystander → the local drives. This is the only genuinely net-new scenario (no existing subtest covers it) — it guards against an over-correction that would refuse a legitimate local spawn because a remote client is merely attached." The two other target scenarios (remote most-active/local idle → NULL; transient winner walk + resolvable local present → NULL + transient) are covered by Task 1-1's inverted `:133` and reframed `:196` transforms and need no separate near-duplicate. `localWalkSeams()` already provides `501→Ghostty` (`.app`) and `601→mosh-server` (NULL walk); reuse it.

**Spec Reference**: `.workflows/remote-trigger-spawns-on-local-terminal/specification/remote-trigger-spawns-on-local-terminal/specification.md` — "Testing Requirements" (New coverage to add), "Behavioural outcomes by scenario" (Mixed: local most-active, remote idle → drive).

## remote-trigger-spawns-on-local-terminal-1-3 | approved

### Task 1-3: Manually verify the honest no-op end-to-end in the reported reproduction setup

**Problem**: The unit tests in Tasks 1-1 and 1-2 cover the selection / locality-ordering logic via the seeded `clientLister` / `walker` / `reader` fakes, but the **real multi-client end-to-end scenario** — an actual remote SSH/mosh client plus a host-local terminal client on the same tmux session, confirming the N−1 windows genuinely do *not* open on the host machine — is out of unit-test reach and easy to miss. Without a deliberate manual check, a regression in the real client-enumeration + walk path could ship undetected.

**Solution**: Perform a manual end-to-end verification in the reported reproduction setup: with the fix built into a real binary, trigger a spawn burst from a remote tmux client while a host-local terminal is attached to the same session, and confirm the honest no-op (no host windows open). Cover both trigger surfaces (TUI multi-select picker burst and CLI multi-target `portal open` burst). This is a human verification task deliberately not folded into a code task.

**Outcome**: A confirmed, recorded manual verification that a remote-triggered burst with a host-local client on the same session opens **zero** host windows on the local machine and surfaces the honest no-op copy, while a control local-only trigger still drives — documented as a pass/fail note (no code artifact).

**Do**:
- Build the fixed binary: `go build -o portal .` from the project root.
- **Establish the reproduction preconditions** (both must hold at detection time): a **remote** triggering tmux client (SSH or mosh — e.g. Blink on iPhone/iPad, or an `ssh` + `tmux attach` from another machine) **plus** at least one **host-local** terminal client (e.g. Ghostty on the Mac) attached to the **same session**. A bare `tmux attach` (no `-t`) on both lands them on the most-recently-used session, naturally mirroring one session.
- **Verify the TUI picker burst surface:** from the remote client, run `portal` / `portal open` to launch the picker, enter multi-select (`m`), mark 2+ sessions, and press `Enter`. Confirm: **zero** new windows open on the host-local terminal; the remote surface takes the honest no-op; and the reactive no-op copy is the shared NULL message (`spawn.UnsupportedNoopMessage` → "can't open new windows over a remote connection — nothing opened"). Also confirm the proactive `m`-entry block now fires (mixed case resolves NULL → `ResolutionUnsupported` → `DetectUnsupported()` true), so `m` is pre-blocked rather than walking the full multi-select flow into a wrong-machine burst.
- **Verify the CLI multi-target burst surface:** from the remote client, run `portal open <a> <b>` (2+ resolvable targets, or a K≥2-expanding glob). Confirm: **zero** new windows open on the host-local terminal, and the atomic no-op surfaces the same NULL copy.
- **Verify the `portal doctor` line (informational):** from the remote client with the host-local client attached, run `portal doctor` and confirm the host-terminal check reports the NULL branch — "unsupported (remote session)" — rather than a driveable host terminal. (Informational only; never drives the exit code.)
- **Control check (must still drive):** from the **host-local** terminal (a legitimate local trigger, no remote as most-active), trigger a burst and confirm windows **do** open locally as before — the fix must not have regressed the legitimate local-spawn path.
- Record the outcome (surfaces exercised, windows-opened counts, observed copy) as a pass/fail verification note. No production or test code is changed by this task.

**Acceptance Criteria**:
- [ ] The binary under test was built from the branch carrying the Task 1-1 fix (`go build -o portal .`).
- [ ] Reproduction preconditions were established: a remote triggering client AND a host-local client attached to the same session at detection time.
- [ ] TUI picker burst from the remote client opens **zero** host-local windows and shows the honest no-op copy; the proactive `m`-entry block is confirmed active.
- [ ] CLI `portal open <a> <b>` burst from the remote client opens **zero** host-local windows (atomic no-op) with the shared NULL copy.
- [ ] `portal doctor` from the remote client reports "unsupported (remote session)" for the host-terminal line.
- [ ] Control: a local-only trigger from the host-local terminal still drives (windows open locally) — no regression to the legitimate local-spawn path.
- [ ] The verification outcome is recorded (pass/fail with observations).

**Tests**:
- Manual, out of unit-test reach — see the Do steps. No automated test is added by this task (the automated decision-surface coverage lives in Tasks 1-1 and 1-2).

**Edge Cases**:
- None. (This is an end-to-end confirmation of the aggregate behaviour; the boundary/edge contracts are pinned by the unit tests in Task 1-1.)

**Context**:
> Spec Verification scope: "The real multi-client end-to-end scenario — an actual remote SSH/mosh client plus a host-local client on the same session, confirming the N−1 windows genuinely do not open on the host machine — is out of unit-test reach and easy to miss in manual testing. It should be verified manually in the reported reproduction setup once the fix lands." Precondition (spec The Bug): a remote triggering client PLUS at least one host-local client attached to the same session at detection time; a local client on a *different* session isn't enumerated → clean NULL → correct no-op regardless. Coherence (spec): after this fix the mixed case resolves NULL → flows into the NULL/remote branch → no persistent `⚠ unsupported terminal` banner, standard header, and the reactive no-op copy `spawn.UnsupportedNoopMessage`. Release approach: regular release, no feature flag, no hotfix urgency; misplaced windows are recoverable by closing them.

**Spec Reference**: `.workflows/remote-trigger-spawns-on-local-terminal/specification/remote-trigger-spawns-on-local-terminal/specification.md` — "Testing Requirements" (Verification scope), "The Bug" (Precondition), "Scope: Affected Surfaces", "Automatically re-armed safeguard", "Coherence with persistent-no-host-terminal-banner".
