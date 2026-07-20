TASK: cli-verb-surface-redesign-3-5 — Spawned-window `open`-grammar argv composition (attach `--session` / mint `--path <literal-dir>`) + Burster surface-spec input

ACCEPTANCE CRITERIA (plan row 3-5 edge cases):
- attach → `env -u TMUX -u TMUX_PANE PATH=… <exe> open --session <name> --ack …`
- mint → `… open --path <literal-dir> --ack …` (reduced literal dir, never alias/zoxide)
- name/dir with spaces = one argv element; `--ack` value = two discrete elements
- TMUX/TMUX_PANE stripped; `os.Executable()` keeps the warm-latch satisfied
- minting at window exec time, not the parent (no pre-mint → no orphan)
- legacy `spawn` CLI + picker burst stay green (all-attach windows converge onto `open --session --ack`)

Also covers phase-level AC: "Each spawned window execs the same `open` grammar a human would: attach targets → `portal open --session <name> --ack <batch>:<token>`; mint targets → `portal open --path <literal-dir> --ack …` (parent reduces alias/zoxide to a literal dir at resolve time so resolution never re-runs in the window); minting happens in each window, not the parent."

STATUS: Complete

SPEC CONTEXT:
Spec § "Burst exec-argv & mint responsibility" governs this task. Each spawned window runs the same `open` grammar a human would — one pinned target + hidden `--ack`, no bespoke burst path. Attach → `open --session <name>`; mint → the parent reduces the target to a literal existing directory at resolve time, then bakes `open --path <literal-dir>`. Alias/zoxide queries never travel to the window (could re-resolve differently mid-burst); only the resolved literal dir does, and `--path` cannot diverge. Minting happens in each window, not the parent — the read-only resolve is the atomic guarantee; a window that never comes up never mints (no orphaned detached session). Command passthrough rides mint windows only, appended after `--ack` in multi-token form, carried byte-identically (no word-splitting); attach windows never carry it. Env composition: `/usr/bin/env PATH=<picker PATH>` with TMUX/TMUX_PANE stripped (the spawned N−1 must run OUT of tmux); `os.Executable()` (not a PATH lookup) keeps the version-gated warm-command latch satisfied so each spawned `open` takes the abridged fast-path.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/spawn/command.go:47 `composeOpenArgv(exePath, path, surface, batch, token, command)` — the pure argv builder. Emits `/usr/bin/env -u TMUX -u TMUX_PANE PATH=<path> <exePath> open <targetFlag> <surface.Value> --ack <batch>:<token>` with `targetFlag` = `--session` (attach) / `--path` (mint, SurfaceMint). Mint-only command appended as `-- <cmd…>` verbatim after `--ack` only when Kind==SurfaceMint && len(command)>0.
  - internal/spawn/command.go:73 `AttachSurfaces(names)` — the picker convergence point mapping all-attach names to `[]Surface{SurfaceAttach}`, so the picker's multi-select burst feeds the same generalized Burster without a name-only fork.
  - internal/spawn/surface.go:5-42 `SurfaceKind`/`Surface` — the two-outcome (attach name / mint literal-dir) value type consumed by the burster.
  - internal/spawn/split.go:39 `SplitTriggerFirst` — leading-trigger split feeding the burster its external set.
  - internal/spawn/burst.go:143 `Burster.Run` — consumes `[]Surface`, resolves exePath ONCE via `b.Exe()`, reads PATH once via `b.Getenv("PATH")`, composes each window argv via `composeOpenArgv`. The burster only SPAWNS each argv — no mint here.
  - cmd/open_surfaces.go:47-49 — literal-dir reduction: a `*resolver.PathResult` becomes `Surface{SurfaceMint, Value: res.Path}` (the resolved literal dir), a `*resolver.SessionResult` becomes `Surface{SurfaceAttach, Value: res.Name}`. This is why alias/zoxide never travel to the window.
  - cmd/spawn_seams.go:56 — production `Exe: os.Executable` (via productionSpawnSeams → NewBurster), satisfying the os.Executable / warm-latch requirement.
  - internal/tui/burst_progress.go:198 — picker burst converges via `spawn.AttachSurfaces(r.external)`, so all-attach windows run `open --session <name> --ack …` (legacy convergence AC).
- Notes: Every fragment of the acceptance grammar is present and load-bearing. The TMUX/TMUX_PANE strip is STRUCTURAL (`-u` unsets + PATH the sole assignment), so an inside-tmux picker cannot leak them into the spawned window. Minting-at-exec is guaranteed by construction: the burster contains no mint call — it only hands the composed `open --path` argv to the adapter, so a window that never opens never mints (no orphan). Literal-dir reduction happens upstream in resolveOpenSurfaces (Task 3-3), consumed here verbatim.

TESTS:
- Status: Adequate
- Coverage:
  - internal/spawn/command_test.go TestComposeOpenArgv — 10 focused subtests, each mapping to a distinct acceptance facet: full attach argv; full mint argv; PATH-only injection + TMUX/TMUX_PANE strip (structural count assertions); session name with spaces = one unquoted element (+ no shell quotes added anywhere); mint dir with spaces = one unquoted element; provided exe path used verbatim (never a bare `portal`); `--ack` as the final TWO discrete elements (not a joined `--ack=value`); mint+command appends `-- <cmd…>`; attach never carries the command even when passed (+ no bare `--`); mint empty-command (nil AND `{}`) appends no bare `--`; single-string command stays ONE element after `--` (no word-split).
  - internal/spawn/command_test.go TestAttachSurfaces — name→SurfaceAttach mapping in order + empty-list case.
  - internal/spawn/surface_test.go — SurfaceKind.String, iota zero-value invariant (SurfaceAttach==0), attach/mint Value carriage.
  - internal/spawn/split_test.go TestSplitTriggerFirst + TestSplitTriggerFirst_DistinctFromSplitNetN — first-trigger split, single-element degenerate, and the opposite-end distinction from the picker's SplitNetN.
  - internal/spawn/burst_test.go TestBurster_Run — surface-spec input at the Burster level: all-attach argv equals composeOpenArgv per window; a mixed attach+mint set composes `--session`/`--path` per surface (asserts mint carries `--path` with the literal dir and NOT `--session`); command rides mint-only through the whole burster (byte-equal to composeOpenArgv with the same command).
- Notes: Well-balanced. Each subtest asserts a distinct behaviour from the acceptance list — no redundant happy-path duplication, no testing of implementation internals. The wire-format is pinned by whole-slice `slices.Equal` where the exact argv matters and by positional/structural assertions where only one property matters. Would fail if the grammar broke (flag name, element splitting, strip, exe substitution, command placement). The warm-latch property itself is a downstream integration concern (abridged-route tests) correctly out of this unit's scope — asserting exePath is passed verbatim is the right unit-level proof here.

CODE QUALITY:
- Project conventions: Followed. Small seam interfaces (ExecutableResolver, Getenv func), pure builders, iota-zero-value convention documented and test-pinned, exhaustive doc comments explaining the load-bearing rationale of each argv fragment. Consistent with golang-code-style / structs-interfaces skills.
- SOLID principles: Good. composeOpenArgv is a single-responsibility pure function; Surface is a clean two-case value type; the burster depends on injectable seams (Exe/Getenv/Ack) not concretes. AttachSurfaces isolates the picker convergence so the generalized burster stays fork-free.
- Complexity: Low. composeOpenArgv is a linear build with one conditional append; burster resolves exe/PATH/ids once up front then a single per-window loop.
- Modern idioms: Yes. `slices` helpers in tests, iota SurfaceKind, table-driven subtests.
- Readability: Good. Intent is self-documenting; the "why" (strip TMUX, os.Executable not PATH lookup, mint-at-exec) is captured in comments without noise.
- Issues: None material.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None. (Considered: `PATH=` when Getenv("PATH") returns empty faithfully carries the picker's own PATH — no concrete corrective action; dropped per the observation floor. The two-line `append("--")` + `append(command...)` is clearer as-is than a nested append — not a change worth making.)
