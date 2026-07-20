TASK: cli-verb-surface-redesign-3-1 — `--ack` receiver flag on `open` (hidden) + best-effort marker write before handoff

ACCEPTANCE CRITERIA:
- malformed `<batch>:<token>` → usage error (exit 2) before touching tmux
- write failure still connects/execs (false negative, no orphan)
- rides both `--session` attach and `--path` mint receivers, written as the last act before the handoff
- hidden from `--help`/completion (MarkHidden) but visible in `ps`
- still-present `attach --spawn-ack` left untouched until Phase 5

STATUS: Complete

SPEC CONTEXT: Spec § "Hidden `--ack` flag" (specification.md:141-143). `open --ack <batch>:<token>` is an internal receipt flag, MarkHidden (gone from --help/completion), remains visible in `ps`. The spawned Portal process, as its last act before exec'ing into tmux, writes `@portal-spawn-<batch>-<token>` as a tmux server option — a delivery receipt the parent burst polls. The write is best-effort: the process still execs even if the write fails, so the window attaches regardless (false negative → parent's poll classifies it failed under leave-what-opened; no orphan). Renamed from today's only-labelled-internal `--spawn-ack`. Phase 3 AC (planning.md:89) restates the hidden+MarkHidden+best-effort-last-act contract.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/open.go:191-195 — malformed `--ack` guard at the TOP of RunE, before the -f branch, multi-target gate, and every pin block; returns NewUsageError("open: --ack must be <batch>:<token>").
  - cmd/open.go:345-363 — openResolved: writeAckMarker(cmd) called immediately before openSessionFunc (SessionResult arm) and openPathFunc (PathResult arm); on the attach arm it is placed strictly AFTER the command+attach usage guard.
  - cmd/open.go:365-387 — writeAckMarker: no-op when --ack absent; re-parses (guaranteed to succeed post-RunE-validation); best-effort — a Write error logs DEBUG under the spawn component (batch/detail attrs) and falls through.
  - cmd/open.go:389-399 — buildAckWriter: openDeps.AckWriter seam when injected, else a real @portal-spawn- ServerOptionAckChannel over the shared tmux client.
  - cmd/open.go:1003-1004 — flag registered + MarkHidden("ack").
  - internal/spawn/ackid.go:74-90 — FormatSpawnAckFlag / ParseSpawnAckFlag (colon-delimited, rejects missing colon / empty batch / empty token).
  - internal/spawn/ack.go:37-42, 78-82 — AckWriter seam + ServerOptionAckChannel.Write (sets @portal-spawn-<batch>-<token>="1").
  - main.go:105-108 + cmd/errors.go:3 — *UsageError classifies to exit code 2.
- Notes: The "written as the last act before the handoff" contract holds from RunE's perspective — the marker write is the final statement before the connector/exec handoff (which itself emits the process:exec marker and syscall.Exec). buildAckWriter (and thus tmuxClient) is only reached when --ack is non-empty, so a human `open foo` triggers no spurious tmux call. The malformed guard precedes ListSessionNames and all pin resolution, satisfying "before touching tmux".
- Edge case "attach --spawn-ack untouched until Phase 5": N/A at final state — Phase 5 completed and cmd/attach.go is deleted. This was a temporal phase-ordering constraint, now superseded; no regression.

TESTS:
- Status: Adequate (thorough — near-exemplary)
- Coverage (cmd/open_test.go:3430-3712):
  - TestOpenCommand_Ack_MalformedValue_UsageErrorBeforeTmux — asserts *UsageError type (⇒ exit 2), exact message, countingSessionLister.calls==0 (no tmux), and neither connector fires.
  - TestOpenCommand_Ack_MarkerWrittenBeforeSessionAttach — Write(b,t) once, connect to "dev", order == [write, session].
  - TestOpenCommand_Ack_MarkerWrittenBeforePathMint — Write(b,t) once, mint at dir, order == [write, path].
  - TestOpenCommand_Ack_WriteFailureStillConnects — both session-attach and path-mint subtests: write errors, RunE returns nil, connector still runs.
  - TestOpenCommand_Ack_CommandAttachGuardFiresBeforeWrite — command+attach usage error, zero marker writes.
  - TestOpenCommand_Ack_FlagIsHidden + retired_surface_test.go:149-155 — --ack present AND Hidden.
  - internal/spawn ackid parse/format contract covered in the spawn package.
- Notes: Both attach and mint receivers, best-effort failure on both arms, strict ordering on both arms, before-tmux short-circuit, guard-before-write, and hidden-flag are all covered. Not over-tested — each test pins a distinct behavioural clause of the AC. The malformed test asserts the *UsageError type rather than the literal exit code 2, but that type is the single source of the exit-2 mapping (main.classify), so the coverage is equivalent.

CODE QUALITY:
- Project conventions: Followed. DI via openDeps.AckWriter seam + buildAckWriter fallback (matches the package-wide *Deps idiom); spawn component attr keys (batch/detail) are within the spec-governed closed vocabulary; MarkHidden matches the state-namespace hiding idiom.
- SOLID principles: Good. writeAckMarker / buildAckWriter each single-responsibility; the write is funnelled through openResolved so both receivers share one call site (no duplication across the attach/mint arms).
- Complexity: Low. Guard is a single early-return; writeAckMarker is a linear no-op/parse/write.
- Modern idioms: Yes (strings.Cut for parse, method-value pin table nearby, errors.As classification).
- Readability: Good. Comments precisely justify the top-of-RunE placement ("before any tmux call"), the last-act ordering, the strictly-after-command-guard placement, and the best-effort/false-negative rationale.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
