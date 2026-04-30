---
status: complete
created: 2026-04-30
cycle: 1
phase: Traceability Review
topic: Hidden Sessions Showing On Startup
---

# Review Tracking: Hidden Sessions Showing On Startup - Traceability

## Findings

### 1. Release-notes action for legacy `0` sessions on upgrade missing from plan

**Type**: Missing from plan
**Spec Reference**: Out Of Scope / Deferred → "Cleanup Of Pre-Existing `0` Sessions On Upgrade" (final paragraph: "The release notes for the shipping change MUST mention this …")
**Plan Reference**: N/A — neither Phase 1 nor Phase 2 references release notes
**Change Type**: add-task

**Details**:
The specification contains a MUST clause for release-notes content:

> "The accepted resolution is: the legacy `0` session persists until the user restarts their tmux server (machine reboot, manual `tmux kill-server`, `pkill tmux`, etc.). The release notes for the shipping change MUST mention this — the suggested wording is 'After upgrading, restart your tmux server (`tmux kill-server`) once to clear any leftover `0` session created by the previous version.' No code change is required."

This is a MUST-level requirement that survives the "out of scope" labelling — the cleanup itself is out of scope, but the release-notes mention is in scope. The plan currently makes no mention of release notes anywhere across the planning file, phase-1-tasks.md, or phase-2-tasks.md. Without an explicit task, the implementer has no surfaced reminder to author the release-notes line, and the spec's MUST goes unfulfilled.

The natural place for this work is Phase 2's commit because that is the commit that ships the user-visible behavioural change relevant to the upgrade-path note (`_portal-bootstrap` rename plus filter together). It must NOT be in Phase 1 because Phase 1 alone leaves `0` still visible — release-notes wording about "leftover `0` session created by the previous version" is meaningful only once Phase 2 lands.

**Current**:
N/A — no existing task covers this.

**Proposed**:

Add a new task `hidden-sessions-showing-on-startup-2-3` to Phase 2. Append to Phase 2's task table in `planning.md`:

```markdown
| hidden-sessions-showing-on-startup-2-3 | Add release-notes line covering legacy `0` session cleanup on upgrade | Release notes for the shipping version include a one-line note matching the spec's suggested wording or equivalent — "After upgrading, restart your tmux server (`tmux kill-server`) once to clear any leftover `0` session created by the previous version." Note lands with Phase 2's commit (or in the release artefact accompanying Phase 2), not with Phase 1; the wording references the literal name `0` (not `_portal-bootstrap`) because the legacy session pre-dates the rename; no code change is required | release-notes file already exists (append to existing release section), no release-notes file or process exists yet (raise as a release-process question and document the resolution in the commit message; do not block on creating new release infrastructure), wording must reference the literal `0` because Fix A's filter does not match `0` and Fix B's rename only affects new server starts |
```

And append a corresponding task detail block to `phase-2-tasks.md`:

```markdown
## hidden-sessions-showing-on-startup-2-3 | approved

### Task 2-3: Add release-notes line covering legacy `0` session cleanup on upgrade

**Problem**: When a user upgrades to a Portal version that includes Fix B, any tmux server already running was started by an older Portal and therefore already hosts a session named `0`. Fix B's `StartServer` does not run because the server is already running, so the rename never happens for that server's lifetime. Fix A's filter targets `_*` and does not hide the literal name `0`. The accepted resolution is to instruct affected users to restart their tmux server once after upgrading. The specification mandates this guidance MUST appear in the release notes.

**Solution**: Add a one-line release-notes entry alongside the Phase 2 commit (or in the release artefact that ships with Phase 2) instructing users to run `tmux kill-server` once after upgrading to clear any leftover `0` session created by the previous version. No code change is required.

**Outcome**: The release notes for the shipping version contain wording equivalent to the spec's suggested line: "After upgrading, restart your tmux server (`tmux kill-server`) once to clear any leftover `0` session created by the previous version." Affected users have a documented path to clean up the legacy session without Portal needing to attempt automatic cleanup (which is unsafe — a user is free to create a session named `0`).

**Do**:
- Locate the project's release-notes channel (release commit message, `CHANGELOG.md`, GitHub release body, or whichever artefact the goreleaser pipeline ships per `.goreleaser.yaml`). If no dedicated file exists, the release note belongs in the GitHub release body that goreleaser publishes for the tagged version.
- Add a short upgrade note matching the spec's suggested wording (verbatim or equivalent): "After upgrading, restart your tmux server (`tmux kill-server`) once to clear any leftover `0` session created by the previous version."
- The wording MUST reference the literal name `0` (not `_portal-bootstrap`) because the legacy session pre-dates the rename and Fix A's filter does not match `0`.
- Do NOT add automatic cleanup code. The spec is explicit: "Auto-cleanup is **not** added because Portal cannot safely distinguish 'leftover bootstrap session named `0`' from 'user-owned session named `0`' — a user is free to create one. Filtering the literal name `0` carries the same risk."
- The release-notes line ships with Phase 2 (or the release artefact accompanying Phase 2), never with Phase 1. Phase 1 alone leaves `0` still visible, so the note's framing ("leftover `0` session created by the previous version") is only coherent once Phase 2 has landed.

**Acceptance Criteria**:
- [ ] Release notes for the shipping version contain a one-line upgrade note instructing users to run `tmux kill-server` once after upgrading to clear any leftover `0` session.
- [ ] The wording references the literal name `0` (not `_portal-bootstrap`).
- [ ] No automatic-cleanup code is added — the legacy session's persistence is resolved entirely via user action documented in release notes.
- [ ] The note is delivered with Phase 2's commit / release artefact, not Phase 1.

**Tests**:
- No automated tests. Verification is editorial: confirm the release-notes artefact for the shipping tag contains the required line before the release ships.

**Edge Cases**:
- Project has no dedicated release-notes file — use the GitHub release body that goreleaser publishes (per `.goreleaser.yaml`); document the resolution in the commit message rather than blocking on creating new release infrastructure.
- A user-owned session legitimately named `0` exists at upgrade time — the release-notes wording does not commit users to running `tmux kill-server`; it offers it as the accepted cleanup path. Users who have a real `0` session can ignore the note. No automated cleanup is attempted because the ambiguity is unresolvable from Portal's side.
- Wording must use the literal `0`, not `_portal-bootstrap`. The rename only affects new server starts; the legacy session that existed before upgrade is still named `0` for the rest of that server's lifetime.

**Context**:
> From the specification's Out Of Scope / Deferred → "Cleanup Of Pre-Existing `0` Sessions On Upgrade":
>
> "When users upgrade to a Portal version that includes Fix B, tmux servers that were started by an older Portal will already host a session named `0`. The new `StartServer` does not run because the server is already running, so the rename never happens for that server's lifetime. Fix A does not filter `0` (it filters only `_*`)."
>
> "Auto-cleanup is **not** added because Portal cannot safely distinguish 'leftover bootstrap session named `0`' from 'user-owned session named `0`' — a user is free to create one. Filtering the literal name `0` carries the same risk."
>
> "The accepted resolution is: the legacy `0` session persists until the user restarts their tmux server (machine reboot, manual `tmux kill-server`, `pkill tmux`, etc.). The release notes for the shipping change MUST mention this — the suggested wording is 'After upgrading, restart your tmux server (`tmux kill-server`) once to clear any leftover `0` session created by the previous version.' No code change is required."

**Spec Reference**: `.workflows/hidden-sessions-showing-on-startup/specification/hidden-sessions-showing-on-startup/specification.md` § Out Of Scope / Deferred → "Cleanup Of Pre-Existing `0` Sessions On Upgrade".

---
```

Also update Phase 2's task-count metadata in `phase-2-tasks.md` front-matter from `total: 2` to `total: 3`, and update Phase 2's `### Phase 2` Acceptance section in `planning.md` to reference the release-notes deliverable. Suggested addition to Phase 2 Acceptance bullets:

```markdown
- [ ] Release notes for the shipping version include a one-line upgrade note instructing users to run `tmux kill-server` once after upgrading to clear any leftover `0` session created by the previous version (per spec § Out Of Scope / Deferred → "Cleanup Of Pre-Existing `0` Sessions On Upgrade"); the note ships with Phase 2's commit / release artefact, not Phase 1.
```

**Resolution**: Fixed
**Notes**: Applied verbatim. Added Task 2-3 row to Phase 2 task table in `planning.md`, added matching acceptance bullet to Phase 2's Acceptance section, appended Task 2-3 detail block to `phase-2-tasks.md` with status `approved`, updated `phase-2-tasks.md` front-matter `total: 2` → `total: 3`, and created tick task `tick-1687f9` (refs `hidden-sessions-showing-on-startup-2-3`) under parent `tick-0a7576`.

---
