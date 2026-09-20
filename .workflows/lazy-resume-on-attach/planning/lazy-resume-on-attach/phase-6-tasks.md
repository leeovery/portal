# Phase 6: Seeing what is waiting — 8 tasks

## lazy-resume-on-attach-6-1

### Task 6.1: The whole-server pending read both surfaces take

**Problem**: Two surfaces have to answer "what is waiting" — `portal doctor` counts pending panes and the picker's session row carries a dot for a session holding one — and nothing outside the daemon can tell them. The marker column exists on the saver's per-tick read, but that read is `state.CaptureStructure`: it enumerates sessions, reads each one's environment, builds the whole structural index and returns pane keys at live addresses, which is the saver's hot path rather than something a diagnosis or a picker can take. Left to themselves, doctor and the picker would each compose an enumeration and each restate "is this pane waiting" — and restating that rule at two call sites is exactly how a diagnosis and a renderer come to disagree about the same server.

**Solution**: One client method beside the existing whole-server hook-key enumeration — `ListPendingResumePanes` — running a single `list-panes -a -F` whose format carries the pending marker and the session name, parsed once into rows plus the two reductions the two surfaces take: the count of pending panes and the set of session names holding one.

**Outcome**: One tmux call answers both questions from one parse; a pane that has never been marked reads unmarked; a failed read is an error rather than an empty set; and no surface composes an enumeration or a presence rule of its own.

**Do**:
- Add to `internal/tmux/tmux.go`, beside `PaneHookRow` and `paneHookRowFormat`: `const resumePendingRowFormat = "#{" + state.ResumePendingOption + "}" + paneHookRowSeparator + "#{session_name}"` — the marker composed from the constant `internal/state` owns, never a restated literal, and taking the **leading** fixed slot so the session name takes the trailing unbounded one.
- Add `type PendingResumeRow struct { Pending bool; Session string }` and `type PendingResumeView struct { Rows []PendingResumeRow; Panes int; Sessions map[string]struct{} }` — `Rows` is one row per live pane (pending or not, exactly as `ListAllPaneHookKeys` returns one per live pane), `Panes` counts the pending ones, and `Sessions` is the set of session names among them, non-nil and empty when none are pending.
- Add `func (c *Client) ListPendingResumePanes() (PendingResumeView, error)` running `ListAllPanesWithFormat(resumePendingRowFormat)` and handing the raw output to an unexported `parsePendingResumeRows`, returning the zero `PendingResumeView` alongside any error.
- In `parsePendingResumeRows`, skip blank lines, `strings.Cut` each line at the **first** separator (a session name may itself carry it, and the marker value never can), reject a line with no separator the way `parsePaneHookRows` rejects one, and lift the marker field through `state.ResumePendingSet` so the presence rule is read from its one home rather than compared against `"1"` here.
- Build both reductions in the same pass over the parsed rows, so the count and the set can never describe different readings.
- Pin the argv and the parse in `internal/tmux/tmux_test.go` through `commandertest.Scripted` (asserting exactly one recorded call), and add `internal/tmux/resume_pending_read_realtmux_test.go` (unit lane, `tmuxtest.SkipIfNoTmux`, a disposable `tmuxtest.New` socket) marking one pane of a multi-pane multi-session fixture through `tmuxtest`'s pending-marker helper and reading it back through the method.
- Edit the `tmux` row of CLAUDE.md's package table: name `ListPendingResumePanes` beside `ListAllPaneHookKeys` as the whole-server pending-resume enumeration returning rows plus the pane count and the session set, consumed by `portal doctor` and the picker's session row.

**Acceptance Criteria**:
- [ ] One `list-panes -a -F` call is issued per invocation and no per-session read is issued, whatever the number of sessions or panes.
- [ ] A pane whose marker field is `1` sets `Pending` on its row; a pane whose field is empty does not, and a pane that has never been marked is indistinguishable from one whose marker was set to the empty string.
- [ ] `Panes` is the number of pending **panes** and `Sessions` is the set of **session names** among them: a session holding two waiting panes contributes 2 to `Panes` and one entry to `Sessions`.
- [ ] A row whose session name contains the field separator parses with the whole name intact, because the cut is taken at the first separator and the marker holds the leading slot.
- [ ] A tmux failure returns the zero view and a wrapped error — never an empty view with a nil error.
- [ ] A server that is not running (the read errors), a server with no panes (`Rows` empty, nil error) and a server with panes but nothing pending (`Rows` non-empty, `Panes` zero, `Sessions` empty) are three distinguishable answers.
- [ ] `Sessions` is non-nil on every successful read, so a caller ranges over it without a nil check.
- [ ] The literal `@portal-resume-pending` appears nowhere in this file — the format is composed from `state.ResumePendingOption`, and the presence rule comes from `state.ResumePendingSet`.
- [ ] Against a real tmux server: one marked pane among several sessions is reported once, with its own session name, and the remaining panes are reported unmarked.

**Tests**:
- `"it issues exactly one list-panes call and no per-session read"`
- `"it reports a marked pane as pending and an unmarked one as not"`
- `"it treats an empty marker field as unmarked"`
- `"it counts panes and sets sessions"` (two pending panes in one session)
- `"it parses a session name carrying the field separator"`
- `"it returns an error and no view when tmux fails"`
- `"it distinguishes no panes from panes with nothing pending"`
- `"it returns a non-nil empty session set when nothing is pending"`
- `"it reads a marked pane back from a real tmux server"` (real tmux, unit lane)

**Edge Cases**:
- A pane that has never been marked reads unmarked, and an empty value reads exactly as an absent one does — tmux reports both as the empty string in a format, so the column carries presence and nothing finer.
- The option name is composed from the marker constant rather than restated: a second spelling of the option would make this read answer for a marker nothing else sets.
- A session name carrying the field separator still parses, so no dot is mis-attributed to a session that is not waiting. The marker takes the leading fixed slot precisely because its value is one of two known shapes, which is the rule the session listing already follows for the directory stamp.
- A tmux failure returns an error and never an empty set: a caller reading a failure as "nothing is pending" would report an install with forty waiting panes as having none, on the one line whose job is to say what is waiting.
- A server that is not running and a server with no panes are both distinguishable from a server with nothing pending — the first errors, the second returns no rows, the third returns rows with no pending flag among them. Doctor's not-evaluable branch (next task) depends on the first being an error rather than a zero.
- The count is panes while the set is sessions, and both are built in one pass, so the two surfaces cannot describe different moments or different rules.
- Portal's own `_portal-saver` and `_portal-bootstrap` panes are enumerated like any other and never carry the marker, so no session filter is needed here and none is added.

**Context**:
> A reboot leaves roughly forty-one panes each holding a pending decision, and the panel exists only inside its own pane. The feature creates a state that previously did not exist and puts it somewhere invisible, so it owes an answer to "what is waiting".
>
> The marker is the pane user-option `@portal-resume-pending`, set to `1` — presence is what is read. A pane option rather than a server option keyed by position: positional keys go stale when tmux renumbers panes, and a pane user-option survives `break-pane`, `move-pane`, a window close under `renumber-windows`, `respawn-pane -k` and a session rename. The whole-server enumeration for pane options already exists, so the picker and doctor each cost one read.
>
> A row carries the pending dot when **any** pane in the session is waiting — the row answers whether the session holds a decision, not how many it holds. Doctor counts panes, so a session holding two waiting panes contributes two to that count and one dot to the list. That is why one read returns both reductions.
>
> Field ordering in the format is load-bearing: a session name may carry the field separator, so the marker takes the leading fixed slot and the session name the trailing unbounded one — the rule the session listing already follows for the directory stamp, and the reason the existing hook-row parse cuts at the first separator only.

**Spec Reference**: `.workflows/lazy-resume-on-attach/specification/lazy-resume-on-attach/specification.md` §8, §8.1, §8.2, §8.3, §7.3

## lazy-resume-on-attach-6-2

### Task 6.2: portal doctor reports the pending-pane count

**Problem**: After a reboot an install comes back with roughly forty-one panes each holding a decision, and each panel lives only inside its own pane — there is no route to "how many are waiting" short of walking every pane by hand. `portal doctor` already reports on this machinery and is the natural home, but every count-bearing line in its catalog fails the moment its count is non-zero (stale hooks, stale projects). Wiring this count the same way would make doctor report failure on success — a correctly functioning install presents forty-one pending panes after a reboot, which is precisely the state this feature exists to produce — and break the scriptable exit code doctor's whole design rests on.

**Solution**: One more line in doctor's catalog carrying the pending-pane count, taking the **informational** status doctor already reserves for a fact that is not a health verdict — never failing, never moving the exit code, excluded from both the passed and the total counts the summary reports — read through a new `DoctorDeps` seam over the whole-server enumeration.

**Outcome**: `portal doctor` states how many panes are waiting, exits zero whatever that number is, and reports a read it could not take as not evaluable rather than as zero.

**Do**:
- Add `PendingResumes func() (tmux.PendingResumeView, error)` to `DoctorDeps` in `cmd/doctor.go`, filled in `resolveDoctorDeps` from `client.ListPendingResumePanes` when the caller left it unset, alongside the other client-backed seams.
- Add `checkPendingResumes(serverUp bool, read func() (tmux.PendingResumeView, error)) checkResult`: a down runtime returns `checkNotEvaluable` — **not** `runtimeDownResult`, which is `checkFail` and would put this line inside the exit code; a read error returns `checkNotEvaluable`; otherwise `checkInfo` carrying `view.Panes` rendered through the existing `pluralCount`.
- Append the result in `runDoctorDiagnosis` after the conditional host-terminal append, so the informational lines trail the pass/fail catalog and a reader can expect a given check at a given place.
- Add no repair: `runDoctorFix` gains nothing, because a pending resume is a decision waiting for the user rather than a fault to reverse.
- Add `cmd/doctor_pending_resume_test.go` driving the command with an injected `PendingResumes` seam over the four cases (a count, zero, a read error, a down runtime) and asserting the rendered line, the exit code and the summary's counts.
- Add a sentence to the `xctl doctor` paragraph of the README, after the one describing the host-terminal check: the report also carries informational lines that state rather than judge — the pending-resume line reporting how many panes are holding a resume decision — and, like the host-terminal line, they never affect the exit code. The paragraph makes no claim today about which lines are informational, so this is an addition rather than a correction of an existing sentence.

**Acceptance Criteria**:
- [ ] The rendered report carries one line for the pending count, after the pass/fail catalog, with the marker `checkInfo` renders.
- [ ] A non-zero count exits zero and does not make `doctorUnhealthy` true — asserted at a count in the low forties, the state a healthy install presents after a reboot.
- [ ] The line is excluded from both the passed and the total in the summary: a run whose only change is a non-zero pending count renders the same `N checks passed` line as one with zero pending.
- [ ] A read error and a down runtime both render as not evaluable, and neither reports zero pending nor changes the exit code.
- [ ] A zero count still renders a line — the fact that nothing is waiting is the answer, not the absence of one.
- [ ] Under `--fix` the line renders in both the pre-repair and the post-repair report, and `runDoctorFix` performs no repair for it.
- [ ] The count is panes: a fixture whose view reports two pending panes in one session renders two, not one.
- [ ] Doctor starts no server on this path and writes nothing — the check's only call is the injected read.

**Tests**:
- `"it renders the pending-pane count as an informational line"`
- `"it exits zero with forty-one pending panes"`
- `"it leaves the pending line out of the passed and total counts"`
- `"it reports a failed read as not evaluable rather than as zero"`
- `"it reports a down runtime as not evaluable rather than as a failure"`
- `"it renders a line when nothing is pending"`
- `"it renders the line in both reports under --fix and repairs nothing"`
- `"it counts panes rather than sessions"`

**Edge Cases**:
- A non-zero count never fails the check and never moves the exit code: the stale-hook and stale-project counts fail the moment their count is non-zero, and this one must not be wired like those neighbours.
- The line is excluded from both the passed and the total the summary reports — rendering it as an always-true passing check would pad `N checks passed` with a line that is not a check.
- A down runtime and a failed enumeration report as not evaluable rather than as zero or as a failure: reporting a failed enumeration as zero pending would assert a fact the read did not establish, on the one line whose job is to say what is waiting. Both statuses already sit outside the exit code and outside the passed/total counts, so the never-fails property holds on either.
- Zero pending still renders a line, so a user cannot read the line's absence as "the check did not run".
- The line renders in both the pre- and post-repair reports under `--fix` because the whole diagnosis is re-run; it has no repair of its own.
- The count is panes and never sessions — a session holding two waiting panes contributes two, which is the reduction the client method already made.
- Doctor is bootstrap-exempt and heals nothing on the read-only path; this check must not start a server or write anything, which is why the down-runtime branch reports rather than probing further.

**Context**:
> Pending panes are visible as a count in `portal doctor`, which already reports on this machinery. A non-zero pending count never fails the check and never changes doctor's exit code. It takes doctor's **informational** status — the one it already reserves for a fact that is not a health verdict.
>
> Doctor's status vocabulary is five members, of which `checkInfo` and `checkNotEvaluable` never drive the exit code and are excluded from both the passed and total counts the summary line reports. `checkInfo` is already used for exactly this kind of fact — the host-terminal line, whose own comment reads "an environmental state, not a Portal-health defect".
>
> A correctly functioning install presents roughly forty-one pending resumes after a reboot, which is precisely the state this feature is built to produce, so wiring the count like its count-bearing neighbours would make `portal doctor` report failure on success and break the scriptable exit code its whole design rests on.
>
> A failed read is reported as not evaluable rather than as a passing zero or as a failure. Both statuses sit outside the exit code and outside the passed/total counts, so the phase's "never fails, never changes the exit code, excluded from both counts" holds either way — but reporting a failed enumeration as zero pending would assert a fact the read did not establish.
>
> The specification states that doctor reports the count on an informational line; it states nothing about the line's wording. The check's name and its detail text are therefore the executor's, within these constraints: they take doctor's existing `name: detail` shape, render the number through `pluralCount` as every other count-bearing line does, say what is waiting rather than what is wrong, and name no particular tool — Portal's resume machinery runs whatever command a registration holds.

**Spec Reference**: `.workflows/lazy-resume-on-attach/specification/lazy-resume-on-attach/specification.md` §8.1, §8, §5.2

## lazy-resume-on-attach-6-3

### Task 6.3: portal doctor reports the install's resume mode

**Problem**: The install-wide `resume_mode` decides whether every restored pane comes back holding a panel or running its command, and it has no UI at all — until a settings screen exists it is changed by hand-editing `prefs.json`. It is also deliberately absent from `portal hook list`: it is one value for the whole install rather than a property of any row, and the only place to put it there is a header or footer line, which breaks naive parsers of a machine interface for a fact that does not vary between rows. So without a report it is configuration you can set and cannot check — and after an upgrade, an install that came back eager is indistinguishable from one that was never configured.

**Solution**: A second informational doctor line carrying the install's resume mode, read through the non-migrating prefs store doctor already holds, rendered as the on-disk spelling `eager` or `lazy`.

**Outcome**: `portal doctor` states which mode the install is in, reports the shipped default whenever the file cannot be read or holds nothing it recognises, and never moves a count or the exit code.

**Do**:
- Add `checkResumeMode(store *prefs.Store) checkResult` to `cmd/doctor.go`: a nil store returns the informational line carrying `resumemode.Default`; otherwise call `store.LoadResumeMode()` and render the returned mode through `Mode.String()`, ignoring the error because the accessor already answers `Default` alongside it. The status is `checkInfo` on every branch.
- Append the result in `runDoctorDiagnosis` after the pending-count line, so the two informational lines sit together at the end of the catalog.
- Take no new seam and no new read: `resolveDoctorDeps` already fills `PrefsStore` from `loadPrefsStoreNoMigrate`, which is the non-migrating route, and this check must not reach for `loadPrefsStore`.
- Add to `cmd/doctor_pending_resume_test.go` (or a sibling) cases over a hand-set `eager`, a hand-set `lazy`, an absent key, an absent file, a corrupt file, an unrecognised value, and a `PrefsStore` left nil.
- Add one case asserting a `prefs.json` carrying an `appearance` key is byte-unchanged after a `portal doctor` run, so the read-only diagnosis is pinned as never dispatching the one-shot translation.
- Extend the README's `xctl doctor` sentence about informational lines to name the resume-mode line beside the pending-resume one.

**Acceptance Criteria**:
- [ ] A `prefs.json` holding `"resume_mode": "eager"` renders `eager`; one holding `"lazy"` renders `lazy`.
- [ ] An install that never set the key renders the shipped default (`lazy`) rather than an empty cell.
- [ ] An unreadable file, a corrupt file, an unrecognised value and a nil `PrefsStore` all render that same default, reached by the accessor's own fallback rather than by a branch of this check's own.
- [ ] A `prefs.json` carrying `appearance` is byte-identical after the run — the diagnosis takes the non-migrating read and dispatches no translation.
- [ ] The line's status is informational: it never fails, never changes the exit code, and is excluded from both the passed and the total the summary reports.
- [ ] The rendered value is the install default alone — a `hooks.json` carrying registrations pinned the other way does not change the line.
- [ ] `portal doctor` writes nothing to `prefs.json` on this path.

**Tests**:
- `"it renders the install's persisted resume mode"` (table: eager, lazy)
- `"it renders the shipped default when the key is absent"`
- `"it renders the shipped default for an unreadable or corrupt prefs.json"` (table)
- `"it renders the shipped default when the prefs store could not be resolved"`
- `"it dispatches no appearance translation"` (prefs.json byte-unchanged)
- `"it moves neither the exit code nor the summary counts"`
- `"it ignores a registration's own mode"` (hooks.json pinned eager under a lazy install)

**Edge Cases**:
- An install that never set the key reports the shipped default rather than an empty cell: the key is `omitempty` on write, so absence is the ordinary state of every install, and a blank there would read as "unknown" for the commonest case.
- An unreadable or corrupt `prefs.json` reports that same default by the same route rather than a second rule — the accessor answers `Default` for every case it cannot make sense of, including a file it could not read at all, and this line simply renders what it answered.
- The read is the non-migrating one so a diagnosis never dispatches the one-shot appearance translation as a side effect. Doctor already resolves its prefs store that way; this check must not reach for the migrating loader, and `loadPrefsStoreNoMigrate` must stay inert.
- The line is informational and moves neither count nor the exit code, exactly as the pending count beside it.
- The value is the install default alone and never a registration's override: the override lives on the registration and is read back from `portal hook list`'s fifth column, which is where a per-row fact belongs.
- An unresolvable prefs path is the unreadable case and not a failure — `resolveDoctorDeps` leaves `PrefsStore` nil when the path will not resolve, and the check renders the default rather than a fault.

**Context**:
> `prefs.json` holds the install's UI preferences and is where the resume mode lives, as the key `resume_mode`, holding `eager` or `lazy`. It decodes tolerantly and independently like every other field there: missing, empty, corrupt or unrecognised gives the shipped default, and a file that cannot be read at all resolves the same way — so an install whose `prefs.json` is unreadable meets panels rather than processes. That is the shipped default arriving by the ordinary route rather than a second rule, and it is the safe direction: a panel can be answered in a keystroke, while a resume the user did not want cannot be taken back.
>
> There is no surface for changing it. The theme picker is the only preference with a UI, so until a settings screen exists this one is changed by hand-editing the file. A settings screen is parked on the roadmap as `preferences-ui`.
>
> The install-wide default is deliberately not in `portal hook list`: it is one value for the whole install rather than a property of any row, and the only place to put it is a header or footer line — which breaks naive parsers of a machine interface for a fact that does not vary between rows. It is reported by `portal doctor` instead, on an informational line carrying the install's resume mode. Like the pending count, it never fails the check and never changes the exit code.
>
> Doctor's prefs store is resolved through `loadPrefsStoreNoMigrate` precisely because doctor heals nothing on the read-only path and the migrating loader dispatches the one-shot `appearance` translation and writes.
>
> The specification states that the line carries the install's resume mode; it states nothing about the line's wording. The check's name and the shape of its detail are therefore the executor's, within these constraints: doctor's existing `name: detail` shape, the value rendered as the on-disk spelling the specification does state (`eager` / `lazy`), and no tool named.

**Spec Reference**: `.workflows/lazy-resume-on-attach/specification/lazy-resume-on-attach/specification.md` §3.1, §3.4, §8.1, §10

## lazy-resume-on-attach-6-4

### Task 6.4: The attached indicator loses its word

**Problem**: The session row's trailing region is sized to the string `● attached` plus the right margin — ten cells of marker and two of margin — and the `session gone` badge is exactly twelve cells wide because it has to replace that whole region without changing the row's width. There is no room for a second indicator, and the word is what makes the region that wide. Dropping it is therefore not a cosmetic edit: it breaks the badge's arithmetic, because with the word gone the indicator needs one cell while the badge still needs twelve, and the row's total width must not move for either.

**Solution**: Replace the marker string with an indicator the row packs hard right — the dot alone, a letter under `NO_COLOR` — and make the row's flex reservation a per-row decision: a gone row reserves the badge's own width and renders it exactly as today, every other row reserves the indicator slot plus the margin, and the name's flex budget takes the cells the word gave back.

**Outcome**: An attached row shows a bare dot at its right edge, an unattached row shows nothing there, a gone row is byte-identical to today, and all three are the same total width.

**Do**:
- In `internal/tui/session_item.go` replace `attachedMarker` / `attachedSlotWidth` with the indicator vocabulary: a `rowIndicatorGlyph` (`●`), an `attachedIndicatorLetter` (`A`), and a renderer that returns the row's packed indicator cluster — the glyph in `Theme.StatePositive` when the session is attached and the delegate is not colourless, the letter when it is, and the empty string when the session is not attached.
- Derive `indicatorSlotWidth` from the widest cluster the renderer can produce rather than restating a number, so the slot moves when an indicator is added rather than being re-stated at each site.
- Introduce the per-row trailing reservation — `lipgloss.Width(goneBadge)` for a gone row, `indicatorSlotWidth + rowRightMargin` otherwise — and feed it into the `used` term the name's flex budget is computed from, so the assembled row is the same total width on either branch.
- Render the non-gone trailing as left-pad to the slot, then the cluster, then the right margin, so a single indicator sits hard right; leave the gone branch rendering the badge exactly as it does today.
- Drop the `goneBadge must not exceed …` width constraint the old arithmetic imposed — the badge now reserves its own width — replacing it with one line stating that a gone row reserves the badge's width instead of the indicator slot, wording the executor's.
- Move the row-anatomy, colourless and burst-abort assertions off the `● attached` / `attachedMarker` strings: `internal/tui/session_row_anatomy_test.go`, `internal/tui/colourless_nocolor_test.go`, `internal/tui/burst_preflight_abort_test.go`, `internal/tui/session_item_test.go`, `internal/tui/row_style_helpers_test.go` and `internal/tui/session_style_consolidation_test.go`.

**Acceptance Criteria**:
- [ ] An attached row renders the dot alone in the trailing region — the word `attached` appears nowhere in a rendered row.
- [ ] An attached row, an unattached row and a gone row rendered at the same list width are all exactly that width.
- [ ] The name's flex budget grows by exactly the cells the word gave back: at a fixed width, a name that truncated before now renders more of itself, and no blank hole is left where the word was.
- [ ] A gone row still replaces the whole trailing region with the badge, at the same right-edge position as today, with the row's width unchanged.
- [ ] The dot keeps `state.positive`, on a selected row as on an unselected one.
- [ ] Under `NO_COLOR` an attached row renders `A` in the indicator cell rather than a bare dot, and an unattached row renders neither.
- [ ] `indicatorSlotWidth` is derived from the rendered cluster, so no call site restates the slot's width.
- [ ] No raw hex appears at the call site — `internal/tui`'s colour-literal guard passes with no exemption added.

**Tests**:
- `"it renders the attached indicator without its word"`
- `"it renders the same row width attached, unattached and gone"`
- `"it gives the reclaimed cells to the name"` (a name that truncated at a fixed width now renders further)
- `"it replaces the whole trailing region with the gone badge"`
- `"it keeps the positive token on the indicator when the row is selected"`
- `"it renders A instead of a dot under NO_COLOR"`
- `"it renders nothing in the indicator cell for an unattached session"`
- `"it packs the indicator hard right"`

**Edge Cases**:
- The trailing region shrinks to the indicator it holds while the row's total width is unchanged — the width invariant is what the delegate's pagination and the gone badge both rest on, so it is asserted directly rather than inferred from the parts.
- The gone badge still replaces the whole trailing region now that the dropped word no longer reserves its cells: the badge reserves its own width out of the row's flex budget, which is what keeps the row width identical while the non-gone rows reclaim the difference.
- Because the reservation is now per row, a row flagged gone reserves more than the same row unflagged, so a long name may truncate one step shorter while the flag is up. The row's total width — the invariant the specification states — is unchanged, and the flag is transient by construction.
- The name's flex budget takes the reclaimed cells rather than leaving a hole: padding the region back out to its old width would keep the row looking identical and defeat the reason the word was dropped.
- An unattached row renders the same width as an attached one, because the slot is padded to a fixed width whether or not it holds an indicator.
- The dot keeps the positive token on a selected row as it does today — the token is resolved through `rowToken` with the row's `selected` flag, not with a literal.
- Under `NO_COLOR` the indicator renders as `A` rather than a bare dot, because Portal's rule for that mode is that state stays glyph-backed and never colour-only; the letter rides with this change rather than arriving later, so no commit renders a bare dot the mode's own rule forbids.
- The row-anatomy assertions move off the old marker string: several suites match on `● attached` literally, and leaving one behind would fail loudly rather than silently, but they are the reason this task touches six test files.

**Context**:
> The session row drops the word `attached` and gains a second dot. The green attached indicator loses its label and stands alone. The dots pack to the right in a fixed order — attached, then pending — rather than holding reserved lanes; a row with one dot puts it hard right whichever it is.
>
> Under `NO_COLOR` each indicator renders as a letter in the same cell — `A` for attached, `P` for pending — so the packing order is untouched and there is no second row geometry. Uppercase rather than lowercase: a lone `a` in a status column reads as a typo, `A` reads as a status code.
>
> A gone row is unaffected: the transient `session gone` badge already replaces the whole trailing region, so it continues to, and a session flagged gone has no live pane to be pending.
>
> Dropping `attached` is pulled into this feature; the rest of the row rework stays parked on `picker-row-redesign`. Removing the word is what makes room for a second indicator, so it cannot wait for the roadmap item — the window count, the session paths and the wider right-hand rework are not this task's business and must not be touched here.
>
> Dropping the word breaks the gone badge's arithmetic, and that is the substance of this task: the trailing region is currently sized to exactly the width of the gone badge, and with the word gone the indicator needs one or two cells while the badge must still replace the region whole without changing the row's width.

**Spec Reference**: `.workflows/lazy-resume-on-attach/specification/lazy-resume-on-attach/specification.md` §8.3, §10

## lazy-resume-on-attach-6-5

### Task 6.5: The pending dot packs beside the attached one

**Problem**: The picker is where a user would notice a new state, and a session holding a waiting pane currently looks exactly like one that does not. Attached and pending-resume are independent facts, so a second meaning cannot be folded into the first indicator — the row needs a second one, packed beside it without either claiming a lane it is not using, and without a second row geometry under `NO_COLOR`, where two identically-shaped circles separated only by hue would not survive.

**Solution**: A pending set on the delegate, keyed on session name, and a second indicator in the cluster the previous task introduced — the attention-token dot after the attached one, `P` after `A` under `NO_COLOR` — with the slot widened to the pair so the row's total width is identical across all four combinations.

**Outcome**: A row whose session holds any waiting pane carries a second dot hard right; a row with both shows the attached dot pushed left to make room; the colourless rendering is `A`, `P` or `AP` in the same cells; and the row's width never moves.

**Do**:
- Add `Pending map[string]struct{}` to `SessionDelegate` in `internal/tui/session_item.go`, keyed on `Session.Name` exactly as `Selected` and `GoneFlagged` already are, with a nil map marking nothing.
- Extend the indicator renderer: the cluster is the attached indicator followed by the pending one, concatenated with no separator, so the coloured form (`●`, `●●`) and the colourless form (`A`, `P`, `AP`) share one geometry; the pending indicator takes `Theme.AccentAttention` and the letter `P`.
- Let `indicatorSlotWidth` re-derive from the widest cluster — now the pair — so the reservation moves with the renderer and no call site restates it.
- Leave the gone branch untouched: a gone row renders the badge and neither indicator.
- Cover the four combinations in `internal/tui/session_row_anatomy_test.go` (or a sibling), including the width-identity assertion across all four and the colourless triple.

**Acceptance Criteria**:
- [ ] A row whose session is in the pending set renders the attention-token dot; one that is not renders nothing in its cell.
- [ ] A row with only a pending dot puts it hard right, at exactly the column an attached-only row puts its dot.
- [ ] A row with both renders attached then pending, with the attached dot pushed left by one cell — neither indicator holds a reserved lane.
- [ ] The row's total width is identical across all four combinations: neither, attached alone, pending alone, both.
- [ ] A session holding several waiting panes carries exactly one dot, because the set is keyed on the session name.
- [ ] A gone row still replaces the whole trailing region with the badge and shows neither indicator, whatever the pending set says.
- [ ] Under `NO_COLOR` the four combinations render as nothing, `A`, `P` and `AP`, in the same cells and the same order, with no second row geometry.
- [ ] A nil pending set marks no row and never panics: every row renders exactly as it does under an empty set.
- [ ] The pending dot is `accent.attention`; no raw hex appears at the call site.

**Tests**:
- `"it renders the pending dot for a session in the pending set"`
- `"it puts a lone pending dot at the same column as a lone attached dot"`
- `"it renders attached then pending when a row carries both"`
- `"it renders the same row width across all four indicator combinations"`
- `"it renders one dot for a session holding several waiting panes"`
- `"it shows neither indicator on a gone row"`
- `"it renders A, P and AP under NO_COLOR"` (table over the four combinations)
- `"it marks nothing for a nil pending set"`
- `"it renders the pending dot in the attention token"`

**Edge Cases**:
- A row with only a pending dot puts it hard right exactly as an attached-only row does — the cluster is right-packed and the padding sits to its left, which is what "no reserved lane" means in cells.
- A row with both pushes the attached dot left: the order is fixed as attached-then-pending whatever the row carries, so the same fact always renders in the same relative position.
- A session holding several waiting panes carries one dot: the row answers whether the session holds a decision, not how many it holds. Doctor is where the pane count lives.
- A gone row still replaces the whole trailing region and shows neither indicator — a session flagged gone has no live pane to be pending, and the badge owns the region.
- Under `NO_COLOR` the pair renders `AP` in the same cells with the packing order untouched and no second row geometry; the letters ride with the indicator that renders them so no commit leaves the mode showing a bare dot.
- The row's total width is identical across all four combinations because the slot is the widest cluster's width and is padded whatever it holds.
- The dot is the attention token and a nil pending set marks nothing — the picker wires the set from a read that may legitimately fail, and a nil map must render a plain row rather than panic.

**Context**:
> The picker is where the user would notice a new state. The row already carries an attached indicator; attached and pending-resume are independent, so it is a second indicator rather than a second meaning for the first.
>
> A row carries that dot when **any** pane in the session is waiting — the row answers whether the session holds a decision, not how many it holds. Doctor counts panes, so a session holding two waiting panes contributes two to that count and one dot to the list.
>
> The dots pack to the right in a fixed order — attached, then pending — rather than holding reserved lanes. A row with one dot puts it hard right whichever it is; a row with both shows the attached dot pushed left to make room. This was built the other way first, with a reserved lane per indicator so the columns aligned down the list, and it was rejected: an indicator should not claim space it is not using.
>
> Under `NO_COLOR` each indicator renders as a letter in the same cell — `A` for attached, `P` for pending — so the packing order is untouched and there is no second row geometry: `A` alone, `P` alone, or `AP` when both. Portal's rule for that mode is that state stays glyph-backed and never colour-only, and two identically-shaped circles separated only by hue would not survive it.
>
> Selection in the picker is already keyed on session identity for the same reason: a multi-tag By-Tag session renders one row per tag, and a fact about the session must show on every one of them.

**Spec Reference**: `.workflows/lazy-resume-on-attach/specification/lazy-resume-on-attach/specification.md` §8.3, §8, §5.2

## lazy-resume-on-attach-6-6

### Task 6.6: The picker resolves pending state from one read

**Problem**: The delegate can render a pending dot but nothing fills its set, and where that set comes from decides whether the indicator can lie. Pending state changes underneath an open picker — a pane resumed in another window while the list is up — so it cannot be cached for the picker's life the way host-terminal detection is, and it cannot be read per row either: a row and its dot would then describe different moments, and a hundred-row list would issue a hundred reads. The session list is already re-fetched at four points (first load, after a kill, after a rename, on preview dismissal), and a dot carried over from a previous list is exactly what a user reads as a stale indicator.

**Solution**: The pending read rides the session-list load: one seam over the whole-server enumeration, taken in the same command as `ListSessions`, delivered on the same message, and applied through the one chokepoint that already replaces the session slice — so the rows and their dots always come from one moment.

**Outcome**: Every list load carries a pending set taken at the same instant as the sessions it describes; a failed read costs the dots and nothing else; a grouped rebuild re-renders from the set already in hand.

**Do**:
- Declare the seam in `internal/tui`: `type PendingResumeReader interface { ListPendingResumePanes() (tmux.PendingResumeView, error) }`, with `Deps.PendingReader`, a `WithPendingResumeReader` option applied by `Build`, and `cmd/open.go` wiring `cfg.pendingReader = client` — the same `*tmux.Client` the session list already reads through.
- Add one unexported helper on the model that performs both reads in the order sessions-then-pending and returns the sessions, the session-name set and the **session** error only; a nil seam and a failed pending read both yield a nil set, and the pending error never reaches `SessionsMsg.Err`.
- Route all four fetch sites through it: `fetchSessionsCmd`, `killAndRefresh`, `renameAndRefresh` and `refreshSessionsAfterPreviewCmd`, carrying the set on `SessionsMsg` and `previewSessionsRefreshedMsg`.
- Extend `applySessions` to take the set alongside the sessions and store it on the model beside `m.sessions`, replacing it wholesale on every load exactly as `m.derivedDirs` is cleared, so nothing survives the list it described.
- Carry the signature through every call site: the two production ones in `internal/tui/model.go` (the sessions-message arm and the preview-refresh arm) and the thirty-six calls across the `internal/tui` suites, which pass a nil set and assert exactly as they do today. A suite left at the old arity stops compiling, which is the intended tripwire rather than a reason to keep a second way into the model's session state.
- Point the delegate's `Pending` at the model's set wherever `Selected` and `GoneFlagged` are already pointed, so `rebuildSessionList` re-renders from the cached set and issues no read of its own.
- Cover in a new `internal/tui` suite over a fake reader: the four refresh paths, the failure and nil-seam degradations, the no-carry-over case, the grouped-rebuild read count, and a multi-tag By-Tag session showing its dot on every row.

**Acceptance Criteria**:
- [ ] A list load takes exactly one pending read, in the same command as the session enumeration, and the delegate renders from that set.
- [ ] A refresh after a kill, after a rename and on preview dismissal each re-read both together; a session that stopped being pending between loads loses its dot on the next list.
- [ ] A pending read that fails renders no dots, renders the session list normally, does not quit the picker and does not block the first paint.
- [ ] An unwired seam marks nothing and panics on nothing.
- [ ] A grouped rebuild — the `s` cycle, a `ProjectsLoadedMsg`, a filter — re-renders from the cached set and issues no second read.
- [ ] A multi-tag session marked pending shows its dot on each of its By-Tag rows, because the keying is on the session name.
- [ ] Flat mode, the filter and the multi-select marks are unaffected by the set's presence or absence.
- [ ] No picker test reaches a real tmux server: the seam is injected wherever a test Executes the open body, and a failed read is the degradation rather than a failure.

**Tests**:
- `"it takes the pending read with the session enumeration on first load"`
- `"it re-reads both after a kill, after a rename and on preview dismissal"` (table over the three)
- `"it drops a dot for a session that is no longer pending"`
- `"it renders the list with no dots when the pending read fails"`
- `"it never carries a pending error into the session-list error"`
- `"it marks nothing when the seam is unwired"`
- `"it issues no second read on a grouped rebuild"`
- `"it shows the dot on every row of a multi-tag pending session"`

**Edge Cases**:
- The rows and their dots come from one moment, so a dot is never carried over from a previous list — the set is replaced wholesale by the same chokepoint that replaces the session slice, never merged into.
- A read that fails renders no dots and never fails the picker or blocks the first paint: `SessionsMsg.Err` quits the TUI, so the pending error must not travel in it. A picker that refused to paint over an unreadable pane option would be worse than one missing an indicator.
- An unwired seam marks nothing rather than panicking — every optional seam in `Deps` is nil-tolerant, and the fixture harness relies on it.
- A grouped rebuild re-renders from the cached set and issues no second read: `rebuildSessionList` is driven by the `s` toggle, a projects load and a filter, and re-reading there would put a tmux call on a keypress.
- Pending state is read on every list load rather than cached for the picker's life, unlike host-terminal detection, which is cached because it cannot change underneath the picker — a pane resumed in another window can.
- Flat mode and the filter are unaffected: the set is consulted by the delegate per row, so no mode does more or less work.
- A test Executing the open body injects the seam, so no picker test gains a new route to a real tmux server; `cmd`'s `TestMain` poisons `TMUX`, which makes a missed injection a loud failure rather than a silent one.

**Context**:
> The picker's pending set rides the session-list load rather than a once-per-picker cache. Host-terminal detection is cached for the picker's life because it cannot change underneath; pending state can — a pane resumed in another window while the picker is open. Taking both reads at the same moment through one path is what stops a row and its dot describing different moments, at one extra enumeration per list load rather than one per row.
>
> The picker takes the opposite degradation to doctor on the same failure and renders no dots, because a picker that refused to paint over an unreadable pane option would be worse than one missing an indicator.
>
> Selection is keyed on session identity, so a multi-tag By-Tag session — one row per tag — marks once and shows on all its rows. The pending set is keyed the same way for the same reason.
>
> `rebuildSessionList` is the single mode-aware re-render chokepoint every entry point routes through, and the existing rule for host-terminal detection is that it must not re-walk there. The same rule applies to this read.

**Spec Reference**: `.workflows/lazy-resume-on-attach/specification/lazy-resume-on-attach/specification.md` §8.3, §8, §8.2

## lazy-resume-on-attach-6-7

### Task 6.7: The help modal's indicator legend

**Problem**: The row now carries up to two bare indicators and nothing says what either means. That is a consequence of dropping the word rather than a separate improvement: `● attached` was self-describing, a dot is not, and a second dot that differs from the first only by hue is less so. The help modal is where the picker explains itself, but it is descriptor-driven — the footer, the help body and the dispatch guard all derive from `keymapEntry` — so expressing an indicator as an entry would demand a key binding for a glyph that dispatches nothing, and would put a non-key in the guard's vocabulary.

**Solution**: A separate legend block in the help modal, rendered as its own compartment beneath the key rows on the Sessions page alone, built from the same indicator renderer the row uses so the legend and the row cannot drift — including under `NO_COLOR`, where both show letters.

**Outcome**: The Sessions `?` help explains both indicators, the Projects and preview help bodies are byte-unchanged, and the keymap descriptor and its dispatch guard are untouched.

**Do**:
- Add a legend vocabulary to `internal/tui/help_modal.go`: a small entry type pairing a rendered indicator with its label, and `sessionsIndicatorLegend()` returning the two entries in the row's own order — attached, then pending.
- Give `renderHelpModalContent` a legend parameter; when it is empty the function renders exactly as today (one title compartment and one body compartment), and when it is non-empty it appends a third compartment of legend rows through `renderJoinedPanel`, so the divider between keys and legend is the frame's own.
- Render each legend row through the existing `keyColumnRow` helper with the indicator in the glyph column, taking the glyph and its token from the **same renderer the delegate uses**, so a `NO_COLOR` legend shows `A` and `P` rather than two dots the mode cannot tell apart.
- Pass the legend at the Sessions call site in `internal/tui/model.go` and pass nothing at the Projects call site and at `internal/tui/pagepreview.go`'s preview-help site.
- Leave `sessionsKeymap()`, `sessionsHelpKeymap()` and the dispatch guards alone — no entry is added, no key is advertised, and `keymap_dispatch_guard_test.go` is neither widened nor exempted.
- Cover in `internal/tui/help_modal_frame_test.go` (or a sibling): the legend's presence on Sessions, its absence from Projects and preview as a byte-identical comparison against the pre-change render, the colourless letters, and the modal's height at the smallest terminal the existing help suites render it at.

**Acceptance Criteria**:
- [ ] The Sessions help modal renders a legend row for each indicator, below the key rows, separated by the panel's own divider.
- [ ] Each legend label says what its indicator means and names no particular tool: the rendered legend rows carry only the indicator forms the row's own renderer produces and the labels this task declares, and no other alphabetic text.
- [ ] The Projects help body and the preview help body are byte-identical to their pre-change renders, in every built-in theme and in colourless mode.
- [ ] Under `NO_COLOR` the legend names the letters `A` and `P` — the same forms the row renders — rather than two indistinguishable dots.
- [ ] The legend's indicators come from the delegate's own renderer, so a change to the row's glyph or token moves the legend with it.
- [ ] `sessionsKeymap()` is unchanged, no new key is advertised on any footer, and the descriptor-to-dispatch guard passes without being widened or exempted.
- [ ] The Sessions help panel with the legend is no taller than the content region at the smallest terminal the existing help-modal suites render it at, so the legend cannot push the panel off the page it is centred on.
- [ ] The `?` self-entry is still skipped from the body, exactly as today.
- [ ] No raw hex appears at any call site — the colour-literal guard passes with no exemption added.

**Tests**:
- `"it renders a legend row for each indicator on the Sessions help"`
- `"it names no tool in either legend label"`
- `"it leaves the Projects help body byte-identical"`
- `"it leaves the preview help body byte-identical"`
- `"it names the letters under NO_COLOR"`
- `"it renders the legend indicators through the row's own renderer"`
- `"it adds no keymap entry and advertises no key"`
- `"it fits the content region at the smallest height the help modal renders at"`
- `"it still skips the ? self-entry"`

**Edge Cases**:
- The legend is not a keymap entry, so the descriptor-to-dispatch guard neither widens nor advertises a key: the help modal is descriptor-driven and the guard derives dispatch from those descriptors, so an indicator expressed as an entry would demand a binding for a glyph that dispatches nothing.
- It renders on the Sessions help alone and the Projects help body is byte-unchanged — the indicators exist only on the sessions row, and a legend on the projects page would explain something that page does not draw. The byte-identity is asserted rather than eyeballed, because the legend parameter passes through the shared renderer.
- The wording states what the indicator means and names no tool. Portal's resume machinery runs whatever command a registration holds, so a pending resume is a pending resume whatever produced it — and this is the one string in the feature whose wording is the executor's and whose surface is the picker's own help, where a tool name would be read as a statement about what Portal is for.
- Under `NO_COLOR` it names the letters rather than two dots the mode cannot tell apart — which is the whole reason the row renders letters there, and the legend inherits it by sharing the renderer rather than by a second rule.
- The modal still fits the smallest height it renders at with the extra rows: the panel is centred on the content region and does not scroll, so rows added to it cost height directly.
- The `?` self-entry skip is unchanged: the legend is a new compartment, not a row in the entries slice, so the body's own filtering is untouched.
- The preview help shares the same renderer and must stay unchanged; it is the third call site and the easiest to forget.

**Context**:
> Dropping `attached` is pulled into this feature, and one consequence rides with it: a bare indicator carries no meaning on its own, so the help modal gains the legend in the same change rather than after it.
>
> Every string this surface renders is tool-agnostic. Portal's resume machinery runs whatever command a registration holds, so nothing names a particular tool — that covers the indicator legend that ships with the picker's pending dot as much as the panel itself.
>
> Under `NO_COLOR` each indicator renders as a letter in the same cell — `A` for attached, `P` for pending. Portal's rule for that mode is that state stays glyph-backed and never colour-only.
>
> The help modal is descriptor-driven: `keymap.go`'s `keymapEntry` is the single source for display, driving the footer, the `?` help body and the theme panel's vertical footer, and `keymap_dispatch_guard_test.go` guards descriptor↔dispatch drift. The legend is a separate block in the modal, and the guard is neither widened nor exempted.
>
> The specification states that the help modal gains a legend for both indicators, worded tool-agnostically; it does not state the wording. The labels are therefore the executor's, within these constraints: they say what each indicator means, they read in the register of the modal's existing labels (sentence case, no trailing period), and they name no particular tool. The wording is settled at this phase's visual check rather than invented here.

**Spec Reference**: `.workflows/lazy-resume-on-attach/specification/lazy-resume-on-attach/specification.md` §8.3, §10, §5.2

## lazy-resume-on-attach-6-8

### Task 6.8: The row on demand for the visual check

**Problem**: The row's trailing region has just been rebuilt — a word removed, a reservation made per-row, two indicators packed right, and a colourless form that must hold the same geometry — and none of that can be judged from an assertion about cell counts. It also cannot be reached by hand: producing a pending dot for real means registering a lazy hook, rebooting and leaving the pane unanswered, and a scratch build of Portal to look at the picker disturbs the running daemon and touches real state. `cmd/capturetool` is the only route to seeing a Portal screen before release, and there is no fixture in which any session is pending.

**Solution**: Two fixtures in the existing registry — one coloured, one colourless — each seeding a session list whose rows cover all four indicator combinations through a faked pending reader, so the frame is produced by the production delegate rather than by a copy of the row.

**Outcome**: `go run ./cmd/capturetool --fixture sessions-pending-resume` renders the real rows at a chosen theme with every combination visible in one frame, its colourless sibling shows `A`, `P` and `AP`, and both are enumerated by the harness's own guards.

**Do**:
- Add a `fakePendingReader` to `internal/capture/fakes.go` alongside `fakeLister`, returning a `tmux.PendingResumeView` built from a declared set of session names, and a `pendingSessions []string` field on `Fixture` wired into `Deps` as `PendingReader` — nil when the fixture declares none, so every existing fixture keeps its current behaviour.
- Add `sessionsPendingResumeFixture()` to the registry: a flat session list in which one session is attached only, one is pending only, one is both, and one is neither, every session carrying a stamped `Dir` so the grouped-render pane-read fallback never fires.
- Add the colourless sibling as a second builder derived from the first with `noColor: true` and its own name, so `Colourless()` reports true and `excludeColourless` keeps it out of the palette-swap diff by its own flag rather than by a name list.
- Seed state, never text: the fixture declares the pending session names and the sessions' attached flags, so no frame can carry a paraphrase of an indicator.
- Assert in `internal/capture` that both names are enumerated by `FixtureNames()` and resolve through `FixtureByName`, that the coloured fixture's frame carries all four combinations, and that the colourless one renders `A`, `P` and `AP`.
- Let the existing guards take the coloured fixture as an ordinary registry member — the swap-and-diff completeness guard grows by it, and the render-size and colourless suites need no exemption.

**Acceptance Criteria**:
- [ ] Both fixture names are listed by `capture.FixtureNames()` and resolve through `FixtureByName`.
- [ ] The coloured fixture's frame carries all four combinations — neither, attached alone, pending alone, both — in one render, so the packing is seen rather than asserted.
- [ ] The rendered rows are produced by the production `SessionDelegate` through `tui.Build`: no layout is restated in the fixture.
- [ ] The colourless sibling renders `A`, `P` and `AP` and reports `Colourless() == true`, matching its `Deps().NoColor`.
- [ ] The palette-swap completeness guard enumerates the coloured fixture and passes over it; the colourless one is excluded by the existing `Colourless()` flag and not by a name list.
- [ ] The pending state reaches the rows through the production seam — the fixture's reader is wired as `Deps.PendingReader` and nothing seeds the delegate directly.
- [ ] No fixture string names a tool, and the fixtures declare state rather than text.
- [ ] `TestPortalBinaryDoesNotImportCapture` still passes — nothing added here reaches the production binary.
- [ ] Every existing fixture is unchanged: a fixture declaring no pending sessions wires a nil reader and renders exactly as before.

**Tests**:
- `"it enumerates both pending-resume fixtures by name"`
- `"it renders all four indicator combinations in one frame"`
- `"it renders the production delegate rather than a copy of the row"`
- `"it renders A, P and AP on the colourless sibling"`
- `"it reports the colourless fixture as colourless"`
- `"it leaves every existing fixture's frame unchanged"`
- `"it keeps internal/capture out of the production binary"` (existing guard, re-run)

**Edge Cases**:
- The fixture seeds pending state rather than text, so no frame can carry a paraphrase: the reader answers with session names and the delegate renders the indicator, which is the same path production takes.
- One frame carries all four combinations so the packing is seen rather than asserted — a frame showing only one combination would prove the dot renders and say nothing about where it sits relative to its sibling.
- The frame renders the production delegate: a fixture that reimplemented the row would sign off a row the picker never draws, which is the one failure a visual gate cannot catch.
- The colourless sibling is excluded from the palette-swap diff by its own flag rather than by a name list — the harness already carries `Colourless()` and `excludeColourless` for exactly this, and a name-based skip would be a second mechanism for the same fact.
- The completeness guard grows by the new fixtures rather than shrinking: the coloured fixture is an ordinary `*Fixture` and joins the guarded set, so enrolling it adds coverage instead of trading it away.
- No fixture string names a tool — every string this feature's surfaces render is tool-agnostic, and a fixture's seeded session names are as visible as any other copy.
- `internal/capture` stays out of the production binary; the existing import guard is the check, and nothing here may give the portal binary a path to it.

**Context**:
> `cmd/capturetool` is a separate offline program that renders a named deterministic fixture of the real TUI with every tmux seam faked; it is the only route to seeing a visual change before release, because a scratch build of Portal itself disturbs the running daemon and touches real state.
>
> The Go fixture definitions in `internal/capture` and the harness itself are permanent — the swap-and-diff completeness guard drives the fixture renderer and its coverage assertion enumerates whatever fixtures exist, so deleting one silently shrinks the guard rather than failing it.
>
> Capture seeds declare state, never text, so a fixture cannot put a paraphrase on a captured frame.
>
> The frame the specification names for this row — **Sessions — pending resume dot (Nord)** — is committed at `testdata/vhs/reference/sessions-pending-resume-dot-nord.png`, and it is the design reference the phase's visual gate reads: the sessions frames already in that directory show the row before the word was dropped, so they say nothing about where the indicators sit. These fixtures exist so the row can be viewed live and held against that frame and against Portal's own existing row grammar; that check is the phase's gate, not a criterion of this task.
>
> That frame diverges from what these fixtures render in one visible way, and the divergence lands on the question it is the sole reference for. Its one two-indicator row (`folio-Jiz4el`) draws the attached dot one blank cell to the left of the pending one; every single-indicator row draws its dot in the same rightmost cell, so the packing order and the right edge match, and only the gap between the pair does not. The specification states the pair as `AP` under `NO_COLOR` — each letter in the cell its own dot occupies, with no second row geometry — which is the adjacent cluster task 6.5 builds and which the row's width-identity criterion pins. The specification's stated form governs, and the frame is read at the gate for layout, structure and colour-role match rather than by pixel diff: a space inserted between the indicators to match it would make the coloured pair three cells wide against a colourless pair of two, so a row carrying both would stop matching the width of a row carrying one.
>
> The `.tape` files and rendered PNGs under `testdata/vhs/` are scaffolding rather than a durable asset: they are written as work proceeds and cleared after sign-off. Nothing in this task commits an image.

**Spec Reference**: `.workflows/lazy-resume-on-attach/specification/lazy-resume-on-attach/specification.md` §8.3, §5.2, §10
