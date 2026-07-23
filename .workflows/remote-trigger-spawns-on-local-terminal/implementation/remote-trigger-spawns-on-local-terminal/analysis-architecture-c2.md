AGENT: architecture
FINDINGS:
- FINDING: Mirrored domain type's Activity contract left stale after the semantic inversion
  SEVERITY: medium
  FILES: internal/tmux/clients.go:9-16, internal/spawn/detect_inside.go:9-17
  DESCRIPTION: The `Activity` field is carried by two parallel, byte-identical
    structs across a layer boundary — the tmux-layer source of truth
    (`tmux.ClientInfo`) and the spawn-layer mirror (`spawn.ClientActivity`) that
    `tmuxClientLister` copies into field-for-field. This change correctly rewrote
    the spawn mirror's docstring to the new cross-client semantics ("the
    cross-client winner-selection signal — the most-active client is the burst's
    trigger"), but the tmux-layer struct that actually produces the value still
    documents the pre-fix, inverted-away semantics: "Activity is the local-only
    tiebreak used to choose among 2+ host-local clients" (clients.go:12). After
    this fix that statement is false — Activity now selects the winner across ALL
    clients, local and remote alike, before any locality check. The two mirrored
    definitions now disagree on what the identical field means. Because
    `tmux.ClientInfo` is the source-of-truth domain type (and its only production
    consumer is this detection path), a future maintainer reasoning from its
    contract could legitimately "optimize" `ListClients` to filter or dedupe on a
    local-only assumption (e.g. drop remote clients, or sort/pick among locals) —
    which would silently exclude the remote trigger from `selectTriggeringClient`
    and re-introduce the exact wrong-machine spawn this fix closes. The spec's
    "Owned Behaviour Change" section explicitly required not leaving contract text
    in place describing behaviour the code no longer has; the spawn-side doc was
    updated in lockstep but the mirrored tmux-side contract was not, so the change
    left a documented-but-false contract on the type that owns the data. This is a
    latent re-break vector, not a live defect — low likelihood, high blast radius.
  RECOMMENDATION: Update the `tmux.ClientInfo` doc comment (clients.go:9-16, and
    the matching narrative on `ListClients`) so the `Activity` field is described
    with the same cross-client winner-selection semantics as the spawn mirror —
    it is no longer a "local-only tiebreak among 2+ host-local clients." Keep the
    two mirrored contracts phrased consistently so the field's meaning cannot
    drift again across the boundary. (No structural change to the mirror/adapter
    seam is warranted — it is a deliberate, working testability boundary and
    pre-dates this change; only the stale contract text needs correcting.)
SUMMARY: The locality-gate inversion is a clean, well-composed, fully-tested
  localized change — unchanged public signature, a pure single-responsibility
  winner-selection helper, and symmetric walkToBundle propagation matching
  detectOutsideTmux. The one architectural gap is that the mirrored tmux.ClientInfo
  contract still documents the old local-only-tiebreak semantics for Activity,
  contradicting the updated spawn mirror and leaving a latent re-break vector on
  the source-of-truth type.
