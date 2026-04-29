---
agent: standards
cycle: 3
findings_count: 4
---
# Standards Analysis (Cycle 3)

## Summary

Implementation tracks spec on load-bearing decisions; drift surface is small (one stale doc-comment, two minor spec ordering inconsistencies post-T8-3, one helper ordering deviation).

---

## Findings

### FINDING: Stale doc-comment claims createSkeleton arms via send-keys
- **Severity**: medium
- **Files**: `internal/restore/session.go:118-119`
- **Description**: createSkeleton's doc comment ends with "Panes are created with no initial command — they default to the user's shell — so that the arm phase can dispatch the hydrate helper via `send-keys` against live indices." This is a fossil from the pre-T7-9 design. The actual arm phase uses `respawn-pane -k` (correctly described elsewhere in the same file at lines 7, 41, 167-174, 476-477). The spec was rewritten in T8-3 to make respawn-pane canonical (spec L633, L734, L759, L1036), and the rest of session.go is consistent. This single hold-out comment misleads any reader trusting it as design intent — directly contradicts the load-bearing semantics the rest of the file (and the spec) document.
- **Recommendation**: Replace the stale `send-keys` clause with respawn-pane wording, e.g. "...so that the arm phase can dispatch the hydrate helper via `respawn-pane -k` against live indices." This brings the file's three doc references to respawn-pane into agreement.

### FINDING: Spec sub-step 1 (mkfifo) ordering inconsistent with respawn-pane pivot
- **Severity**: low
- **Files**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md:1031-1038`, `:767`
- **Description**: Bootstrap Flow step 5's per-session sub-steps list "1. For each pane: compute FIFO path; remove existing; mkfifo" before "2. new-session", "4. new-window/split-window for remaining", "5. Arm each created pane with respawn-pane -k". This pre-T7-9 ordering assumed the FIFO had to exist before pane creation because the helper was the pane's initial command. With the respawn-pane-k pivot (now canonical per spec L633, L734, L759), only the helper (not the pane) needs the FIFO present at open time. Implementation correctly mkfifos inside `armPanes` (session.go:215) — between split-window and respawn-pane — but pairs each FIFO creation with the respawn-pane that immediately consumes it, not with the pane creation. Spec's separate "Signal Mechanism: FIFO Per Pane" section (L767) also says "before creating the pane", another stale fragment.
- **Recommendation**: Move sub-step 1 (mkfifo) so it sits between sub-step 4 (new-window/split-window) and sub-step 5 (arm with respawn-pane), or rephrase sub-step 5 to call out FIFO creation as a precondition the arm phase performs per-pane. Update the L767 "before creating the pane" parenthetical to "before arming the pane" so the two sections align with the actual respawn-pane lifecycle.

### FINDING: Hydrate helper marker-unset / hook-lookup ordering reversed vs spec
- **Severity**: low
- **Files**: `cmd/state_hydrate.go:171-184`, `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md:796-800`
- **Description**: Spec "Helper Behavior on Startup" step 2 lists e (settle sleep) → f (read hooks.json + lookup) → g (unset marker via -su) → h (exec hook-or-shell). Code performs e (sleep) → unset marker → lookup-and-exec inside execShellOrHookAndExit. The two operations are independent (a tmux server-option write vs. a hooks.json read), so the divergence is functionally inert under the happy path; however the spec ordering exists because step f's behaviour is conceptually "load the hook before mutating the system." Reordering risks subtle issues if execShellOrHookAndExit ever grows a code path that relies on the marker still being present (or, conversely, relies on it being absent — neither is currently the case). This is the only e/f/g/h ordering deviation in the helper, and it appears in the file-missing recovery path too (handleHydrateFileMissing → unsetSkeletonMarkerOrLog → caller invokes execShellOrHookAndExit).
- **Recommendation**: Either reorder runHydrate so hook lookup happens before unset (lookup the hook into a local variable, then unset, then exec the chained sh -c), or update the spec's step e/f/g ordering to match the code. Spec-side update is the lower-risk change and just acknowledges that under the current implementation the lookup is independent of the marker state.

### FINDING: Spec L1032 mkfifo step does not name CreateFIFO's actual replace/chmod semantics
- **Severity**: low
- **Files**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md:1032`, `internal/state/fifo.go:32-41`
- **Description**: Spec sub-step 1 says "os.Remove(path) (ignore ENOENT); syscall.Mkfifo(path, 0600)". state.CreateFIFO correctly removes any pre-existing entry — regular file, symlink, or prior FIFO, not just FIFOs — and defends against an unusually-tight umask by chmod'ing post-mkfifo. The spec wording is narrower than the implementation: it only mentions ENOENT tolerance and the 0600 mode passed to mkfifo, leaving the replace-any-inode and umask-defence-chmod aspects undocumented. Implementation is the safer behaviour and matches the section's intent ("guarantees callers a fresh inode every time"), but a future maintainer trusting only the bare spec text could narrow the implementation back to "FIFOs only, no chmod follow-up."
- **Recommendation**: Tighten spec L1032 to acknowledge "remove any existing inode (regular file, symlink, FIFO) before mkfifo, then chmod 0600 to defend against an unusually-tight umask." This makes the contract self-describing and prevents accidental narrowing of the helper.
