## Discussion

## Discussion: Split Oversized Go Files

### Context

Portal is to set its own file-organisation standard, record it in `CLAUDE.md`, and split the oversized Go files along their concern seams — standard and sweep landing in the same release.

**The inherited position from discovery.** The seed's case is that file size here is an active cost, not an aesthetic complaint, and the evidence is agent-read cost: during the theming-system feature `internal/tui/model.go` was the single most-read artifact of the whole implementation — 327 reads across 85 distinct implementation subagents, each full pass roughly three chunked reads at agent tooling's 2,000-line window. Hub-file gravity also concentrates merge risk: any future parallelism in implementation is dead on arrival while one file is the junction for half the feature surface. Go makes the mechanical half free — files inside a package split with no import, caller or test changes — so the work is choosing the seams. The existing `theme_panel.go` / `theme_panel_commit.go` / `theme_row.go` siblings are the working pattern already in the tree.

**The user's framing extended the seed twice.** First, the outcome should align with a recognised convention rather than an arbitrary threshold, because the current sizes serve neither a human nor an agent reader. Second, it must not stop at a refactor: the standard goes into `CLAUDE.md` so the discipline holds for future work, with trimming and refactoring done as far as is practical, all in the same release. The user was explicitly unsure about a size discipline as such while judging it worth having.

**A premise correction was accepted into the framing rather than resolved.** Go has no file-size convention to align with — the standard library ships multi-thousand-line files without apology (`net/http/server.go` ~3,900 lines, `go/types/expr.go` comparable). What Go does conventionalise is *one concern per file within a package*: a cohesion rule, not a line count. The standard this work sets is therefore Portal's own, argued from the measured agent-read cost rather than borrowed from the language, and the honest target is files a reader can hold rather than conformance to an external number. Where exactly that lands — a hard ceiling, a soft guideline, or a cohesion rule with size as a symptom — was left open for this phase.

**Shape was settled in discovery and is not reopened here.** The splits are the same seam-choosing exercise repeated over a handful of files rather than independent concerns needing their own cycles, so no topic multiplication. The work-as-standard reading was considered and rejected: the standard alone would terminate at specification, and the refactor must land in the same release, which needs the work to reach implementation.

#### Current state of the tree (measured 2026-08-15)

The seed's headline numbers have moved — `model.go` was 5,467 when the idea was logged on 2026-08-09 and is 3,448 now, the drop coming from the comment-strip sweep rather than any split. `model_test.go` is 7,116 against the seed's 7,766. No concern split has happened.

Largest files:

| File | Lines |
|---|---|
| `internal/tui/model_test.go` | 7,116 |
| `internal/tmux/portal_saver_test.go` | 3,772 |
| `cmd/open_test.go` | 3,581 |
| `internal/tui/model.go` | 3,448 |
| `internal/tmux/tmux_test.go` | 3,211 |
| `cmd/state_hydrate_test.go` | 1,849 |
| `internal/state/capture_test.go` | 1,626 |
| `cmd/doctor_test.go` | 1,495 |
| `internal/hooks/store_test.go` | 1,425 |

Distribution matters more than the top of the list. **Production: 275 files, 5 over 500 lines, 1 over 800, 1 over 1,000** — `model.go` at 3,448 is a lone outlier, the next largest being `internal/tmux/tmux.go` at 775 and `internal/capture/fixtures.go` at 762. **Tests: 595 files, 68 over 500, 25 over 800, 14 over 1,000.** The production side is already close to whatever standard we set; the test side is where the mass sits.

#### References

- `.workflows/split-oversized-go-files/seeds/2026-08-09-split-oversized-go-files.md` — the originating inbox idea
- `.workflows/split-oversized-go-files/discovery/sessions/session-001.md` — discovery session log (the carrier)
- `CLAUDE.md` — where the standard is to be recorded
- `internal/tui/theme_panel*.go` — the existing in-tree pattern for panel-scoped concern files

---

### Standard Shape

#### Context

Discovery left open what form the standard takes — a hard ceiling, a soft guideline, or a cohesion rule with size as a symptom — having established that Go itself offers nothing to borrow. Everything downstream hangs off this: which files the sweep touches, what goes into `CLAUDE.md`, and whether anything can mechanically enforce it.

#### Options Considered

**A — Line ceiling.** "No Go file over N lines."
- Pros: checkable, guard-testable, unambiguous.
- Cons: blind to *why* size hurts. Flags a table-driven test file (plausibly one coherent concern) identically to `model.go` (nine concerns in one file). Invites the worst fix — splitting mid-concern to satisfy a number.

**B — Pure cohesion rule.** "One concern per file within a package."
- Pros: the actual Go convention; names the real cost driver.
- Cons: judgment-only and un-auditable, so it decays. The evidence is in the tree — `model.go` reached 3,448 lines in a package that already had `theme_panel.go` / `theme_panel_commit.go` / `theme_row.go` demonstrating the pattern.

**C — Cohesion rule, line count as a tripwire.** The rule is cohesion; the number is a review trigger, not a violation. A file past N is presumed to hold more than one concern and has to justify itself.
- Pros: survives the objection discovery accepted (`net/http/server.go` at 3,900 lines is fine because it *would* justify itself) while still giving a reviewer or an agent something mechanical to act on. The only one of the three with an obvious enforcement story in this repo's idiom, given the ~25 source-guard tests already in the tree.
- Cons: two moving parts to state rather than one.

#### Journey

**The distribution reframed the problem before the options were weighed.** Measured at session time: production is 275 files with five over 500 lines and exactly one over 800 — `model.go` at 3,448. The next largest are 775 (`internal/tmux/tmux.go`), 762 (`internal/capture/fixtures.go`), 735 (`cmd/open.go`), 518 (`cmd/doctor.go`), then nothing until 447. **There is no production file between 775 and 3,448.** Tests are 595 files with 68 over 500, 25 over 800, 14 over 1,000 — a smooth decay with no cliff at that scale, but a stark one much higher: 7,116 · 3,772 · 3,581 · 3,211, then a gap down to 1,849 and a smooth tail. Four monsters, then twenty-one merely-large files.

So a line rule binds today on `model.go` plus a dozen-odd test files. The seed's justification — measured agent-read cost on a production hub file — does not transfer unexamined to a 3,200-line test file.

C was taken directly, not as a compromise between A and B: A alone produces bad splits, B alone produces no splits, and the tree is the evidence for the second.

**On the number, the position moved once.** The opening position was a single tripwire at 800 for everything, read off the production cliff. It moved to a split — production 800, tests 2,000 — on the argument that a line of test code carries less information about concern-count than a line of production code, because table-driven cases and fixture setup are repetitive by construction. If the number's job under C is to *predict* "this file probably holds more than one concern", it should be calibrated to how well a line predicts that, and it predicts worse in tests. 2,000 is independently anchored to the agent read window (a file an agent sees whole in one pass) and sits in the test data's own gap; what it selects is exactly the four files the seed named on intuition, before anyone counted.

**The user's framing replaced the rationale with a better one.** The cost of a big file is not line count as such — it is that reading it forces chunked reads and burns context. And the two file kinds are read differently: a production file must usually be understood before it can be edited, so its full cost lands every time; a test file is usually appended to, and can be reached surgically by grep or offset read, so its full cost rarely lands at all. That is a stronger justification for the prod-strict / test-loose split than the line-prediction argument, and it supersedes it — the split now rests on how each file kind is *read*, not on how well a line predicts concern-count.

**The self-healing hypothesis was checked and holds for one file in four.** The user's observation was that splitting a production file drags its test file along, so the test side may self-heal wherever a split is warranted. Against the tree: only `model_test.go` (7,116) has an oversized production counterpart (`model.go`, 3,448). The other three monsters do not — `internal/tmux/portal_saver_test.go` (3,772) pairs with a 386-line production file, `cmd/open_test.go` (3,581) with 735 lines, `internal/tmux/tmux_test.go` (3,211) with 775. Ratios of 10:1, 5:1, 4:1. Self-healing covers `model_test.go` and nothing else, so a test-side rule is not redundant.

**The larger finding: the convention already exists in the tree and is already practised.** `cmd/open_test.go` (3,581 lines, 68 test funcs) sits alongside fifteen-plus purpose-named siblings — `open_burst_run_test.go`, `open_theme_construction_test.go`, `open_surfaces_test.go`, `open_multitarget_test.go`, `open_targets_test.go`, `open_nocolor_test.go`, `open_fatal_test.go`, `open_domain_routing_test.go`, and more. `internal/tmux` is the same: `portal_saver_test.go` (3,772) sits beside `portal_saver_lifecycle_events_test.go`, `portal_saver_integration_test.go`, `portal_saver_endstate_integration_test.go`, and the `hooks_*` family splits six ways. Every recent feature dropped its tests into a purpose-named sibling. **The monsters are pre-convention residue** — the core that predates the habit and that nobody goes back to drain, because nobody edits old tests.

This means the standard is not being invented. It is being written down, and then back-filled onto the files that predate it — which is exactly the "Portal's own standard, derived not borrowed" framing discovery asked for.

It also shows the tree's test convention is *not* a mirror of production files: it is one purpose-named file per behaviour area, and it is finer-grained than production. `cmd/open_targets.go` is 75 lines and carries two test files (`open_targets_test.go`, `open_targets_guard_test.go`). The monsters violate that convention by being named for a *file* rather than for a behaviour.

**The test rule was put as a fork and resolved toward the tree.** Two formulations were available: *a test file mirrors its production file* (simple, predictable, no judgment call — but it would force `open_targets_test.go` to absorb `open_targets_guard_test.go`, undoing a split the tree deliberately made), or *one purpose-named test file per behaviour area* (what the tree actually practises, finer-grained, at the cost of "behaviour area" being a judgment call the mirror rule avoids). The user took the behaviour-area form, on the principle that **file count is cheap** — ten small test files are preferable to one large one. That principle is worth carrying beyond the test rule: it means no lower bound is wanted, and a proposed split is never rejected on grounds of "that's too many files".

**The two-number split was then dropped, and the reason is the strongest empirical finding in the topic.** A background review flagged that the read-cost argument (tests looser) and the convention finding (the tree's test practice is finer-grained than production) point in opposite directions and were never reconciled *(resolves review-001 F5)*. Checking which one the tree supports produced a sharper result than the finding: **every test file over 1,000 lines is named after a production file — all fourteen of them** — and the production files they mirror are tiny. `cmd/theme_test.go` is 1,035 lines against an 87-line `theme.go` (12:1); `portal_saver_test.go` 3,772 against 386 (10:1); `state_commit_now_test.go` 1,163 against 172 (7:1); `resolution_test.go` 1,016 against 220 (5:1); `capture_test.go` 1,626 against 369 (4:1). Meanwhile every behaviour-named test file in the tree — `theme_panel_geometry_test.go`, `open_burst_run_test.go`, `edit_modal_state_machine_test.go`, `theme_panel_commit_load_test.go`, `hooks_register_test.go` — sits in the 300–997 band. Not one has run away.

So the mirror name is not merely a style we prefer against: **it is the mechanism that produces monsters.** A file-mirroring name is unbounded by construction — `theme_test.go` means "tests for theme.go", so everything about themes goes in and the name never signals that the file is full. A behaviour name is self-limiting: new behaviour, new file. `theme.go` stayed at 87 lines precisely because the production side already had a naming discipline the test side lacked.

That kills 2,000 as a standard: it would never trip a single behaviour-named file, because the practised convention peaks at 997. It only selects the four monsters, which makes it a sweep selector wearing a standard's clothes. The split collapses to **one tripwire at 1,000 for both file kinds** — just above the practised band on both sides (production tops at 775, behaviour-named tests at 997), so a well-named file essentially never trips and a trip means something. The read-cost insight is not discarded; it moves out of the number and into the **burden of rebuttal**: a 1,400-line table-driven test file trips and justifies itself in a sentence, a 1,400-line production file trips and probably cannot. Same detector, different burden of proof. Accepted cost: production loosens from 800 to 1,000, so a future 900-line production file will not trip where it would have — affordable given that cohesion is the actual rule and file count is cheap.

**Whether to keep a number at all was reopened and closed.** The user tested dropping the tripwire entirely in favour of naming and behaviour-bucketing rules alone, then argued back to keeping it: cohesion is hard to judge, and without a trigger nobody stops to check. That fixes what the number *is* — a procedural attention device, not a quality metric. It is there to make someone stop; it is never the thing being satisfied.

**A second detector was added because the line tripwire has a false-negative floor** *(resolves review-001 F1)*. The review's remaining objection to the numbers — that a threshold read off the current distribution ratifies the status quo by construction — survived the collapse to a single number, because 1,000 was derived the same descriptive way. The derivation itself holds up: it was calibrated against the *behaviour-named subset* of the tree, the part already following the rule being adopted, and used to catch the part that is not — that is calibration against a validated sample, not circularity. What does not hold up is relying on it alone. The reviewer's worked example: `internal/tmux/tmux.go` is 775 lines, 225 under the line and permanently compliant, and it wraps sessions, windows and panes, hook-key derivation, environment, server lifecycle, options and global hooks — comfortably more than five concerns, invisible to the detector forever. If the tripwire is the only thing that makes anyone stop, the cohesion rule only applies above 1,000 and C slides back toward A with a different number.

The second detector is name-shaped rather than line-shaped, and it generalises the finding that produced the test rule. The same pattern is visible on the production side within one package: every file in `internal/tui` is named for a behaviour or a component — `notice_band.go`, `burst_progress.go`, `loading_view.go`, `session_item.go`, `edit_modal.go`, `pagepreview.go`, all 284–433 lines — except `model.go`, which is named after a *type* and is eight times larger than the next biggest. `internal/tmux/tmux.go` is named after its package. Two production data points, so suggestive rather than proven, but pointing the same way the test-side evidence did.

This also supplies the operative content the cohesion half was missing — the objection that sank option B *(resolves review-001 F2)*. The first formulation was "the file can be named after the one behaviour it owns", and it was **too loose, in the over-indicting direction**: measured against the tree, **217 of 595 test files are mirror-named**, and 173 of those are under 500 lines. `internal/project/pathkey_test.go` beside a small `pathkey.go` is a perfectly good file. Read literally, a rule against mirror-naming indicts 217 files to fix about a dozen.

Mirror-naming is therefore a **correlate, not the property** — it correlates because a production filename is usually the broadest label available for that material. The property actually doing the work is whether the name *excludes* anything:

> A file's name must be specific enough to exclude things. If material that plausibly belongs in the package could be dropped into the file without contradicting its name, the name is a bucket label and the file will grow without bound.

It separates every case examined so far, in both directions:

| File | Could plausible new material go in without contradicting the name? | Verdict |
|---|---|---|
| `cmd/theme_test.go` (1,035) | yes — any theme-command test | bucket |
| `internal/tui/theme_panel_geometry_test.go` (997) | no — commit tests contradict it | bounded |
| `internal/tui/model.go` (3,448) | yes — any TUI code | bucket |
| `internal/tui/notice_band.go` (284) | no | bounded |
| `internal/tmux/tmux.go` (775) | yes — any client method | bucket |
| `internal/project/pathkey_test.go` | no | bounded, and mirror-named |

It also explains the mirror correlation rather than merely observing it: `theme` is a poor name not because it matches a filename but because nothing theme-related contradicts it. Accepted cost: "could this plausibly go in?" remains a judgment call — sharper than "one concern", but not mechanical, and two readers could differ on a borderline name. That is not avoidable; the tripwire is the mechanical half, and this is the half that needs a person.

An implication for `sweep-scope`, not decided here: the sweep's candidate set is no longer "everything over the line" — a file can fail the exclusion test while sitting under 1,000. (`sweep-scope` subsequently settled it: the candidate set *is* everything over the line, plus the two below-the-line failures this discussion actually adjudicated, with unadjudicated ones knowingly left out.)

**The rebuttal was given a durable home, because without one the shape is not C at all** *(resolves review-001 F3)*. Presumption-and-rebuttal is the entire practical difference between this shape and a plain ceiling: remove the ability to record a rebuttal and the tripwire behaves as a hard limit; leave the rebuttal implicit and it behaves as the pure cohesion rule already rejected for decaying. The document had defined the tripwire to the line and left "has to justify itself" undefined.

The worked case: `internal/capture/fixtures.go` is 762 lines and grows by construction — every new TUI screen contributes a fixture. Say it reaches 1,150. It trips, someone establishes it genuinely is one concern (table-shaped fixture definitions, the "repetitive by construction" case), and moves on. Six weeks later an agent opens it, sees 1,150 lines, and redoes the whole analysis. Then the next one does. **That re-litigation is precisely the context cost the standard exists to remove**, so the rebuttal must be durable.

The standard's own rationale picks the location: **in the file**. Any other home — an allow-list, a doc, a commit message — forces a second read to answer a question raised by the first; in the file, the answer rides the read already being made. This fits the surviving convention rather than reversing the comment-strip sweep: 90 of 870 Go files still carry a leading doc comment.

The justification **states the concern the file claims to own**, so a later reader can falsify it by reading rather than having to trust it. A bare "large on purpose" rots silently — the file gains a second concern and the old note keeps vouching for it.

**On drift detection, the governing principle is: record nothing that must be updated when the file changes.** Candidates weighed — a content hash self-invalidates on every edit including a typo in the marker itself (and cannot include itself without excluding its own line), a line count is a poor proxy in both directions (a file can absorb a whole new concern at net-zero line change, or grow 200 lines of the same table and look drifted when nothing changed), and mtime is not committed and is reset by checkout. All three share the fatal property that they must be maintained, and across a set of perhaps three to eight marked files over years they will not be — an unmaintained staleness marker is worse than none, because it lies.

What survives are fields that are true at tagging time and never need touching: the **date** and the **commit verified against**. Both stay honest forever — they assert "this claim was checked against this state", and going stale is information rather than an error. The commit reference is strictly better than a hash: it does not self-invalidate, and it yields the actual diff (`git diff <sha>..HEAD -- <file>`) rather than a boolean. The date is nearly redundant against the commit but is kept because it reads at a glance with no tooling.

Adopted form — a greppable prefix (no `portal:` comment pragma exists in the tree today, so the namespace is free):

```go
// portal:oversized 2026-08-16 @ f238265 — Fixture definitions only; each TUI
// screen contributes one. No logic lives here.
```

**The production/test classification was then removed from the rule altogether** *(resolves review-001 F4)*. The review identified a third category the split missed: code that is test-only in purpose but production-suffixed in filename — `internal/portaltest`, `themetest`, `spawntest`, `restoretest`, `tmuxtest`, `portalbintest`, `transienttest`, `logtest`, `sourceguardtest`, plus `internal/capture` and `cmd/capturetool`. Go can only distinguish the kinds by the `_test.go` suffix, which none of these carry.

Collapsing to one number had already removed most of the classification's work; only the burden of rebuttal still keyed to it. That keying does not survive scrutiny either: what makes a rebuttal easy is the *claim*, not the suffix. "Each TUI screen contributes one fixture, no logic lives here" either holds or it does not, and the filename has no bearing on whether it does. The `_test.go` distinction was only ever a proxy for "probably repetitive by construction", and once a trip is adjudicated on a written claim the proxy has nothing left to do.

The practical stake confirmed the dissolution rather than a new rule: across all eleven of those packages only `internal/capture/fixtures.go` (762) is anywhere near the line — next largest `portaltest/fingerprint.go` at 300, then `capture/swatch.go` at 212 — so a third file class would have existed to govern a single file whose claim already stands on its own. The standard therefore names no categories: one tripwire, one exclusion test, one rebuttal mechanism. Accepted cost: nothing in the rule tells a contributor that a table-driven test file gets an easy ride, so someone may argue harder for one than needed; if that reassurance is wanted it belongs as a line of guidance in `CLAUDE.md`, never as a category in the rule.

A hook for `claude-md-and-enforcement`, not decided here: the marker can serve as its own allow-list — a guard walking `.go` files and flagging anything over 1,000 without the marker needs no separate list, and grep is the enumeration. That also answers the objection that in-file justifications are not enumerable.

#### Decision

- **Shape: C.** The standard is a cohesion rule — one concern per file within a package — with a line count acting as a tripwire that presumes a violation and demands justification, never as a violation in itself.
- **Rationale is context cost, not line count.** A file too large to read in one pass forces chunked reads and burns context — the cost is what a reader must pull in to work with the file, never the line count as such. *(Amended 2026-08-17 — this bullet also carried the production-vs-test reading asymmetry as its rationale; the classification was later removed from the rule altogether, and the asymmetry now lives in the burden-of-claim bullet below.)*
- **Two detectors, not one.** A line tripwire and a name-exclusion test, working at different scales. The tripwire catches files that have grown past what a well-organised file in this tree looks like — it is set just above the band Portal's own well-named files occupy, and its job is to make someone stop and check. The exclusion test catches files holding more than one concern at *any* size — the majority of the tree, and the whole range the tripwire is blind to. *(Amended 2026-08-17 — this bullet described the tripwire as catching files "grown past being readable in one pass", which was its criterion only while the test-side number was 2,000, the measured agent read window. The collapse to a single 1,000 re-derived it off the practised band, and at 1,000 a tripping file is still readable in one pass; "readable in one pass" survives in the context-cost bullet above as why size costs anything at all, never as the tripwire's own test.)*
- **The exclusion test is the operative form of the cohesion rule.** *A file's name must be specific enough to exclude things. If material that plausibly belongs in the package could be dropped into the file without contradicting its name, the name is a bucket label and the file will grow without bound.* This is what makes C more than a ceiling: it bites below the line, where the tripwire is silent (`internal/tmux/tmux.go`, 775 lines, seven-odd concerns, permanently compliant with the number). Mirror-naming is a correlate of failure, never the test itself — 217 of 595 test files are mirror-named and most are perfectly well-bounded.
- **One tripwire, 1,000 lines, every Go file.** No production/test classification exists in the rule at all. Set just above the band Portal's own well-named files already occupy (production tops at 775; behaviour-named test files at 997), so a properly-organised file essentially never trips. The earlier two-number split (production 800 / tests 2,000) is dropped: 2,000 would never fire on anything the tree does, and 200 lines of separation did not earn a second number.
- **The read-cost asymmetry lives in the strength of the written claim, not in a file category.** A repetitive-by-construction file justifies itself in a sentence; a file holding several concerns cannot. That difference is adjudicated on the claim the marker states — it is not keyed to the `_test.go` suffix, or to any classification, and the rule names none.
- **The number is a procedural attention device, not a quality metric.** Its job is to make someone stop and check, because cohesion is hard to judge and otherwise nobody does. It is never the thing being satisfied — a file under 1,000 is not thereby well-organised.
- **Test files are organised per behaviour area, one purpose-named file each** — what the tree already practises, and finer-grained than a 1:1 production mirror (`cmd/open_targets.go` is 75 lines and carries two test files). Naming a test file after the behaviour it covers is **how it passes the exclusion test**, not a separate prohibition: a mirror name is usually a bucket label because a production filename is the broadest label available, which is why every test file over 1,000 lines is mirror-named and no behaviour-named one has run away — but a mirror name that excludes things is fine, and most of the 217 are. *(Amended 2026-08-17 — this bullet previously banned names derived from a production filename outright; mirror-naming was later demoted to a correlate of failure, and a categorical reading would indict 217 files including `internal/project/pathkey_test.go`, which the same subtopic blesses as bounded.)*
- **A trip is rebutted in the file, with a `// portal:oversized {date} @ {commit} — {claim}` marker.** The claim names the single concern the file owns, so a later reader can falsify it by reading. The location follows from the standard's own rationale: a rebuttal stored anywhere else forces a second read to answer a question raised by the first.
- **The marker records only fields that never need updating** — date and verified-at commit. No content hash, line count, or mtime: each must be maintained on every edit, and across a handful of files over years it will not be. A stale marker that lies is worse than no marker; a date and a commit that simply age are honest, and yield the diff on demand.
- **File count is cheap.** Many small files beat one large one; there is no lower bound and no "too many files" objection to a proposed split.

---

### Sweep Scope

#### Context

What this release actually changes. The inherited framing is "as far as is practical, in the same release", which anticipates not finishing — but the standard's own machinery constrains how an unfinished sweep can be expressed.

#### Journey

**The `// portal:oversized` marker cannot be used to defer, and that forces the posture choice.** Its contract is that it states the single concern the file owns, so a later reader can falsify it. A monster cannot make that claim — "this is a bucket and we have not got to it yet" is an admission, not a rebuttal. So there is no honest way to ship the standard while leaving known violators marked-and-standing, and the release must pick between making the tripwire true on day one and shipping the standard as forward-looking with a backlog.

**Posture chosen: make the mechanical detector true on day one.** The scope is everything over the 1,000-line tripwire — `model.go` plus the fourteen test files over the line — **34,370 lines across 15 files in 8 packages** (`internal/tui`, `internal/tmux`, `cmd`, `cmd/bootstrap`, `internal/state`, `internal/hooks`, `internal/resolver`, `internal/theme`). The line falls out of the two detectors: the tripwire is the mechanical half, so it can be made true and then held by a guard; the exclusion test is the human half by construction, so it applies to work as it happens. A guard born with a fifteen-entry exemption list protects nothing.

**The two adjudicated below-the-line failures were then pulled into scope** *(resolves review-002 F3)*. They were initially excluded on the reasoning that the human detector only bites on code someone is already editing, and that the alternative is hand-auditing 275 production files. The review's objection was that the rebuttal mechanism covers only the tripwire: `internal/tmux/tmux.go` (775) and `cmd/open.go` (735) fail the exclusion test, are under the line so cannot carry a `// portal:oversized` marker, and the marker's contract rules out "not got to it yet" — so every agent that opens `tmux.go` re-derives the same verdict indefinitely, which is the exact cost the standard exists to remove.

Half of that does not hold, because **the two detectors are not symmetric**. The tripwire presumes guilt and demands an answer — 1,150 lines obliges a decision. The exclusion test carries no presumption: it is the question asked when deciding where to put code. Nobody is required to adjudicate `tmux.go`, so there is no standing verdict to record; someone adding a client method reads the file, adds the method, and never asks the organisational question. The derivation happens only when someone is about to act on it, which is when it is cheapest, because they are already reading. A second marker (`// portal:split-pending` or similar) was rejected on this basis, and on a second: it would be unbounded in a way the rebuttal marker is not — having declined the 275-file audit, only the failures someone happened to notice would ever be marked, which is arbitrary.

What does hold is the residue: **these two were already adjudicated**, in this discussion, and excluding them discards work already done. The "cannot audit 275 files" reasoning does not apply to them, because the set of already-adjudicated failures is bounded by construction — it is two. Including them costs 1,510 lines against a 34,370-line sweep and buys a materially stronger day-one position: the standard true on *both* detectors for every file anyone has actually examined, rather than true on the mechanical one and knowingly false on the human one.

`cmd/open.go` was flagged as the riskiest single file in the sweep and taken anyway. It is the product's main entry path — resolution grammar, domain pinning, TUI construction, burst dispatch — and the one file where a bad seam changes behaviour rather than merely relocating it; everything else in scope is a test file, `model.go` (clean panel-shaped seams), or `tmux.go` (a method-bag wrapper that splits by method group).

#### The mechanical-safety question, measured

The concern raised was whether changing tests risks production behaviour — and specifically whether the split is genuine cut-and-paste or whether it forces mid-function cuts and helper re-derivation. Measured against the four monsters:

- **Helpers do not move, and that is the load-bearing fact** — *within a package clause*. Go test files sharing a clause share a single scope, so a helper defined in `model_test.go` stays visible to a relocated test **provided the new sibling also declares `package tui_test`**. Nothing is re-derived or duplicated. This matters given the density: `cmd/open_test.go` carries 47 non-test top-level funcs and 23 package-level vars, `portal_saver_test.go` 42 helpers. *(Amended 2026-08-17 — originally stated as a per-directory property. It is per package clause: seven of the eight in-scope directories carry both an internal and an external test package, and a sibling declaring the internal clause sees none of the external clause's helpers and cannot reach an unexported identifier. As written it would have licensed a split that cannot compile.)*
- **No mid-function splitting is required.** Largest test function per file: `portal_saver_test.go` 77 lines, `open_test.go` 234, `tmux_test.go` 274. `model_test.go` is the outlier — `TestCommandPendingMode` 479, `TestProjectsPage` 443, `TestLoadingPage` 415 — but each is already a behaviour area and moves whole. The unit of movement is the entire `func TestX`.
- **No build tags on any of the four**, so the integration-lane hazard (a `//go:build integration` tag failing to ride along to a new sibling, silently moving a test between lanes) does not apply to these files. It still applies to any tagged file a later split touches.

#### The one real risk, now measured rather than hypothesised

Go runs tests in source order, and files in lexical filename order, so **splitting changes execution order**. The `cmd` package injects mocks through package-level mutable state (`openDeps`, `bootstrapDeps`, `doctorDeps`) cleaned via `t.Cleanup()`; a leaked cleanup makes a test order-dependent, and reordering exposes it — arriving mid-refactor looking exactly like sweep damage.

`go test -shuffle=on` measures this in advance. Run across the tree:

- **Every package passes shuffled except `cmd`** — including `internal/tui` and `internal/tmux`, the two largest sweep targets.
- **`cmd` fails intermittently, on roughly one shuffle order in three.** Reproduced: `TestCompletionHidesInternalSurface/top-level completion excludes the hidden state namespace` fails with `candidates=[]` — top-level completion offering nothing at all, meaning an earlier test mutated the root command and did not restore it. Five `cmd` test files mutate the root command, `open_test.go` among them.

This is a **pre-existing latent bug**, present today and independent of any split. The finding converts the sweep's ordering risk from unknown to located: it is confined to one package, and it is visible before a line moves.

**A shuffled-order run becomes the sweep's verification gate** — per package, before and after a split. Shuffle-clean before splitting is the precondition; `cmd` does not currently meet it. *(Amended 2026-08-17 — this originally claimed green-both-sides "proves the move preserved behaviour and introduced no order coupling". It does not prove the second half; see the fixed-seed Decision bullet below.)*

**The gate was then pinned to a fixed seed set, and this session is the evidence for why** *(resolves review-002 F4)*. "Shuffle-clean" left the number of orderings unstated, and a single `-shuffle=on` run samples one permutation out of an enormous space: measuring `cmd` here, run 1 passed, run 2 passed, run 3 failed. A one-run gate would have declared `cmd` clean twice before anyone got unlucky — and catching that failure is the entire reason the gate is trusted. `-shuffle=on` also reseeds randomly per invocation, so a red run cannot be reproduced from the command that produced it, and before/after comparisons are made against different permutations each time.

`go test -shuffle=N` takes an explicit seed, which fixes the reproducibility half: the gate is a fixed set of seeds (working figure: ten, `1`–`10`), and a failure hands back the exact seed to reproduce it against a fixed tree. *(Amended 2026-08-17 — this originally claimed it also made the before/after comparison identical. It does not; see the Decision bullet below.)* Ten is a working figure rather than a derived one — overwhelming against a 1-in-3 dependency, thin against a 1-in-50 — accepted on the reasoning that a rarer order-dependency survives the sweep unchanged rather than being caused by it.

**A third option the review floated was rejected on the record**, so specification does not reopen it: splitting `cmd` under a weakened gate, on the argument that a red-before/red-after run proves nothing either way. A gate that cannot fail is not a gate, and it would put the leak inside the sweep's blast radius instead of ahead of it.

**The build-tag hazard was audited across the full scope and does not apply** *(resolves review-002 F5)*. All **seventeen** in-scope files were checked: **none carries a build tag** — including `internal/tmux/tmux.go` and `cmd/open.go`, re-checked after they entered scope. *(Amended 2026-08-17 — the audit originally ran over fifteen files and claimed full-scope coverage; the scope decision later fixed the set at seventeen. Conclusion unaffected: both added files are core production files and neither is tagged.)* The review's specific suspicion — that `state_commit_now_test.go` (1,163) might be integration-tagged, inferred from `CLAUDE.md` naming "the commit-now suite" — is wrong: that file is plain unit-lane, and the commit-now integration tests live in separate files (`state_commit_now_symptom_integration_test.go` 365, `_reentrancy_` 176, `_daemon_merge_` 129). Tree-wide there are **47 integration-tagged test files and the largest is 710 lines**, so no tagged file is in scope at all.

That is not luck, and it corroborates the naming thesis on the axis where it matters most: the repo already segregates integration tests by *filename*, and `*_integration_test.go` is a name that excludes things — new integration material cannot drift into a unit-lane file without contradicting its name. It is why none of the 47 has grown past 710.

**Pulling on the general case surfaced a worse failure mode neither review caught: silent test loss.** In a 34,370-line relocation, a test function dropped on the floor leaves the suite green — no compiler error, no failing test, and the shuffle gate cannot see it either. It is strictly more likely than a dropped build tag and has no signal at all.

One inventory gate covers both, and is stronger than a tag check because it asserts identity rather than a property:

```
go test -list '.*' ./...            # sorted, before and after — must be identical
```

Measured: **3,706 tests in the unit lane, 3,791 with the integration tag** (85 integration-only). A dropped `//go:build integration` moves a test from integration-only into both lanes, so the unit list *grows*; a lost test makes it *shrink*. Accepted limit: the inventory cannot see a test moved *and* quietly altered — it checks identity, not content. A pure relocation does not alter bodies, and the suite passing covers the rest.

**The `cmd` fork was taken toward fixing the leak rather than dropping the package.** The alternative was excluding `cmd` from the sweep, which would have cut the scope from fifteen files to nine — but `cmd` holds six of the fourteen over-the-line test files (`open_test.go` 3,581, `state_hydrate_test.go` 1,849, `doctor_test.go` 1,495, `state_daemon_run_test.go` 1,217, `state_commit_now_test.go` 1,163, `theme_test.go` 1,035). Leaving them standing breaks the day-one posture, and the marker cannot honestly paper over it, so deferring `cmd` would have reinstated the forward-looking posture by the back door. Deciding factor: dropping `cmd` costs the posture that makes a guard possible at all, whereas the leak is a real bug that will bite regardless of this work.

#### Decision

- **Scope is every Go file over the 1,000-line tripwire, plus the two already-adjudicated exclusion-test failures** — `model.go` and the fourteen over-the-line test files, plus `internal/tmux/tmux.go` (775) and `cmd/open.go` (735): **17 files, 35,880 lines, 8 packages**. At release the tripwire is true of the whole tree and the exclusion test is true of every file anyone has examined.
- **Unadjudicated exclusion-test failures stay out of scope**, explicitly and knowingly. The human detector applies to work as it happens; hand-auditing 275 production files before release is not practical. The two above are in *because* they were already adjudicated — a bounded set, not the start of an audit.
- **`cmd/open.go` is the sweep's highest-risk file** and is understood as such: the product's main entry path, and the only file in scope where a bad seam changes behaviour rather than relocating it.
- **The split is a pure relocation of whole top-level test functions.** Helpers stay where they are and stay visible **within a package clause** (a sibling must declare the same clause as the file it came from); nothing is re-derived or duplicated; no test function is cut mid-body. Verified against all four monsters.
- **A fixed seed set is the verification gate** — `go test -shuffle=N` over seeds `1`–`10`, per package, before and after each split. Never `-shuffle=on`: **reproducibility is the whole of the advantage** — a red run must be replayable from the seed that produced it, which is exactly what this session lacked when `cmd` passed runs 1 and 2 and failed run 3. Shuffle-clean beforehand is a precondition of splitting a package.

  **Settled by derivation** — not discussed. Determined by Go's own implementation (review-004 F3): in `$(go env GOROOT)/src/testing/testing.go`, `-shuffle=N` seeds a PRNG that shuffles `m.tests` **in place**, and `m.tests` arrives in source order — so a split changes the input and the same seed yields a *different* run order afterwards. The gate therefore does **not** put the same permutations on both sides, and it does not prove a split "introduced no order coupling"; it samples ten orderings before and ten different orderings after. *(Amended 2026-08-17 — this bullet claimed "the same ten permutations both sides", and the reasoning that produced it cited differing before/after permutations as the defect of `-shuffle=on`. Fixed seeds do not fix that defect. The gate is unchanged and still worth running; it proves less than was claimed.)* The residual already accepted for the integration lane applies to the unit lane too: an order-dependency the sampled seeds miss was equally present before the split, so it is an undiscovered latent flake rather than damage the sweep caused. `cmd` is where that difference will be felt.
- **The gate runs the full seed set in the unit lane and a single fixed seed in the integration lane** *(resolves review-003 F5)*, the integration run only for packages that carry integration tests — `cmd` (15 tagged files), `cmd/bootstrap` (18), `internal/tmux` (3), `internal/state` (1). The other four in-scope packages have none, `internal/tui` among them, so splits there cannot disturb an integration lane at all. Derivation: the two lanes are asked different questions. The unit lane is asked *is this package order-independent* — a property, which sampling ten orderings probes, and which is cheap enough to run. The integration lane is asked *did this split change anything under a fixed ordering* — a comparison, which one ordering answers. Ten seeded integration runs per package per split, `-p 1`, with no CI and on the repo's most timing-coupled tests, would buy a stronger claim about a property nobody asserted, at a cost high enough that the gate would stop being run — and a gate nobody runs is worse than a cheaper one they do. Accepted residual: an integration test order-dependent in a way the fixed seed misses was equally order-dependent before the split, so it is an undiscovered latent flake rather than damage the sweep caused.
- **The `cmd` order-dependency leak is fixed first, as a precondition** — its own change, independently verifiable, not folded into a split commit. It is a pre-existing test-isolation bug (an unrestored root-command mutation) in a package this work must touch.
- **A test-inventory diff is the second gate** — `go test -list '.*' ./...`, sorted, identical before and after. It catches **silent test loss** (a function dropped during relocation leaves the suite green — no compiler error, no failing test, no signal from the shuffle gate) and a dropped `//go:build integration` tag in the same command, the first shrinking the list and the second growing it. Baseline: 3,706 unit-lane tests, 3,791 with the integration tag.
- Build tags bind on later splits only: **none of the seventeen in-scope files is tagged**, and tree-wide the largest of the 47 integration-tagged files is 710 lines. Any future split that does touch one must carry `//go:build integration` to every new sibling; the inventory gate is what detects a failure to.

---

### Seam Selection

#### Context

Where the files actually split. Go makes the mechanical half free within a package, so this is the judgement half of the work — and the exclusion test is what a proposed seam is judged against.

#### Journey

**The residual problem: draining a bucket-named file does not fix its name** *(resolves review-002 F7)*. `model.go` is indicted by the exclusion test — named after a type, excluding nothing. Splitting it into behaviour-named siblings leaves a residual (the `Model` type, `Init`/`Update`/`View`, whatever else stays) in a file whose name still excludes nothing, so the residual would pass the tripwire and fail the exclusion test on day one, inside the sweep meant to make the standard true. A rename is not free either: `model.go` is one of the 46 filename-pinned guard targets and `CLAUDE.md` names it in two separate sections.

The resolution is that **the exclusion test is evaluated against what a file holds, not against its name in the abstract** — a name can only exclude things relative to a concern. Today's `model.go` fails because it holds ten: the model type, the option constructors, list styling, sizing arithmetic, the grouping glue, the update router, the projects page and its edit modal, the sessions-page key arms, view composition, and roughly 160 lines of ANSI SGR rewriting. Stripped to the `Model` struct, `New`, and the three `tea.Model` methods, the name does exclude things — edit-modal chip logic, canvas backfill and projects-page arms all contradict it.

What makes that legitimate rather than special pleading is a property that also separates the cases already judged: **does the name denote something with a natural boundary, or an open-ended subject area?** `model.go` holding the type and its interface methods is bounded by an external contract — `tea.Model` has exactly three methods and a struct is a struct, so nothing can accrete. `cmd/theme_test.go` is "everything about the theme command", bounded by nothing, which is why it reached 1,035 lines. This is the production-side form of the earlier finding that mirror names are unbounded by construction.

**The two below-the-line production files needed their own residual criteria** *(resolves review-003 F1)*. They are in scope because they fail the exclusion test, not because they trip the tripwire, so "the tripwire is now true" is no completion condition for either — and the residual rule as first written covered only `model.go`.

**`internal/tmux/tmux.go` dissolves entirely**, and that follows from the residual rule rather than being an exception to it. A residual keeps its name only if the split can make that name exclude things; a **package-named file can never reach that state** — "could any client method go in?" is permanently yes. So there is no legitimate residual, and the file ceases to exist. Its 53 functions fall into clean method groups already visible in the outline: the `Commander` seam and command execution; the `Client` type and its constructors; server lifecycle; session operations; window and pane operations; pane-target formatting; the hook-key and structural-key derivation primitives; server and session options; the raw global-hook operations (which join the package's existing `hooks_*` family, distinct from the registration logic in `hooks_register.go`); and environment. Roughly ten files averaging under eighty lines, which "file count is cheap" settles without argument.

**`cmd/open.go` keeps its name and survives as a residual**, because it holds an `init()` and so is governed by the absolute no-move rule. Its residual is the `openCmd` declaration, flag registration, `init()`, and the package-level vars — and that *does* earn the name under the exclusion test: new resolution behaviour, TUI construction or path-opening logic all contradict "the open command's declaration and wiring" and belong in siblings. The package already establishes that family (`open_targets.go`, `open_surfaces.go`, `open_burst.go`, `open_burst_run.go`), so the split extends a convention rather than inventing one. What leaves: the two connectors and TUI-result handling; `openSession`; the path-opening chain (`PathOpener.Open`, `openPath`, the quick-start and execer adapters); the resolution chain (`resolvePinAndOpen`, `openResolved`, `buildQueryResolver`, `emitResolveDecision`, `resolveDecision`); ack-marker writing; command-argument parsing; theme loading and resolution; and TUI construction (`buildTUIModel`, `openTUI`).

These groupings are indicative rather than binding — the decision is the criterion each file must meet, not the particular grouping that satisfies it.

#### Decision

- **A package-named file cannot earn its name and is dissolved, not reduced.** Three in-scope files are package-named and all three cease to exist, leaving no residual, because no amount of stripping makes the package's own name exclude anything inside that package: `internal/tmux/tmux.go`, **`internal/tmux/tmux_test.go`** (3,211 — `package tmux_test`) and **`cmd/bootstrap/bootstrap_test.go`** (1,287 — `package bootstrap`).

  **Settled by derivation** — not discussed. Determined by the dissolution rule already decided for `tmux.go`, plus the removal of the production/test classification, which leaves no test-file exemption to invoke (review-004 F2). The two test files were never named anywhere in this document — they and `internal/resolver/query_test.go` are the over-the-line test files the record never enumerated — which is how 4,498 of the 35,880 in-scope lines came to sit outside a rule that governs them. `query_test.go` is unaffected: its package is `resolver_test`, so its stem is not the package name.
- **`cmd/open.go` keeps its name.** Residual: the command declaration, flag registration, `init()` and package-level vars — bounded, because resolution, path-opening, theme and TUI work all contradict it.
- **The package clause is a boundary seam selection respects, never crosses** *(resolves review-004 F5)*. Seven of the eight in-scope directories are dual-clause (`tui`/`tui_test`, `tmux`/`tmux_test`, `cmd`/`cmd_test`, `bootstrap`/`bootstrap_test`, `state`, `resolver`, `theme`; only `internal/hooks` is single). A sibling **inherits its source file's clause by default** — splitting `model_test.go` yields `package tui_test` siblings. Getting this wrong is a compile error, so it is a constraint on seam choice rather than a runtime hazard, and no gate needs to catch it.
- **A behaviour area may be a *pair* of files where the clause forces one**, and `<behaviour>_internal_test.go` is the established form for the internal half. The behaviour part of the name does the excluding and satisfies the exclusion test; `_internal` is a clause qualifier riding along. The tree already runs this convention in seven files — `exact_target_internal_test.go`, `option_discriminator_internal_test.go`, `capture_internal_test.go`, `union_internal_test.go`, `load_internal_test.go`, `daemon_state_internal_test.go`, `loading_fatal_internal_test.go`. This is the one point where "file count is cheap" meets a language constraint rather than a preference, and the constraint wins because it is not negotiable.
- **Every in-scope file's completion condition is the exclusion test, not the tripwire.** For the two below-the-line files the tripwire was never the reason they are in scope and cannot signal that their split is done.
- **A residual keeps its name; the split has to earn it.** Draining a bucket-named file does not require renaming the file that remains — but the remainder must be reduced until the name genuinely excludes things, judged against contents rather than against the name alone.
- **Acceptance criterion for `model.go`:** post-split it holds the `Model` type, its construction, the three `tea.Model` methods, the `page` enum the router switches on, and the accessors that expose model-level rather than page-level state — and nothing else. If `rebuildSessionList` or the canvas-fill machinery is still there, the split is not finished. The 400–600 line figure is indicative, never a target: the completion condition is the exclusion test, not an arithmetic one.
- **Declaration-only material distributes to the behaviour file that owns it, with no exemption from the exclusion test** *(resolves review-004 F4)*. `model.go` holds roughly 290 lines the nine-concern enumeration never named — thirteen exported DI seam interfaces (`SessionLister`, `PreviewAttacher`, `ProjectStore`, `SessionKiller`, `SessionCreator`, `SessionRenamer`, `ModePersister`, `ThemePersister`, `ProjectEditor`, `AliasEditor` and the theme/preview seams), eight exported message types plus `LoadingMinDuration` and the `page`/`editField`/`editMode` enums, and around thirty-five one-line accessor methods that exist because `model_test.go` is an external `package tui_test`. The obvious destinations are the ones the standard forbids: a bare `seams.go`, `messages.go` or `accessors.go` in package `tui` excludes nothing and is a bucket label by the same reasoning that dissolves `tmux.go`. Exempting declarations from the test was rejected too — declarations accumulating unchecked is part of how `model.go` reached 3,448 lines, and a `messages.go` would grow without bound for exactly the reason the test exists to catch.
  So each declaration goes where its behaviour went: the session seams and `SessionsMsg`/`SessionCreatedMsg` with the sessions code, the project seams and `ProjectsLoadedMsg` with the projects code, the bootstrap and loading messages with the loading page, `editField`/`editMode` with the edit modal, the `With…` options with the concern each configures. `theme_seams.go` is the in-tree proof that this produces bounded names — it excludes every seam that is not a theme seam. Nothing blocks it mechanically: every file in package `tui` can declare methods on `Model`, so distributing accessors costs nothing and the external test package is unaffected.
- **A name is judged on whether its concern has a natural boundary.** Bounded by an external contract (an interface, a struct, a protocol) passes; an open-ended subject area ("everything about X") does not, because it can accrete indefinitely.
- Keeping the name is what preserves the guards and `CLAUDE.md` entries that point at `model.go` — they stay valid because the file still exists and still holds the model.

---

### Refactor Safety

#### Context

The sweep is a large mechanical move: code changes files without changing behaviour. The premise inherited from the seed is that this is free — *"files inside a package split with no import, caller or test changes"* — which is true for the compiler and load-bearing for the whole case that the work is cheap. What that premise misses is anything in the repo that is keyed to a **filename** rather than to a symbol.

#### Journey

**The free-move premise does not hold for this repo's source guards** *(resolves review-001 F6)*. Measured: **46 test files hardcode a `.go` filename**. A share of those are temp-dir fixtures (`alpha.go`, `kept.go`, `thing.go`) and are irrelevant; the rest pin real source files — `model.go`, `tmux.go`, `theme.go`, `open.go`, `doctor.go`, `theme_panel.go`, `theme_panel_commit.go`, `restore.go`, `pagepreview.go`, `setting.go`, `state_daemon.go`, `harness.go`, `modal.go`.

Reading the two the review cited showed they fail in **opposite** ways, which is the operative distinction:

- **Assert-presence self-destructs, safely.** `internal/tui/pagepreview_filter_test.go` reads `model.go`, extracts `updateSessionList`'s body and counts `tea.KeySpace` occurrences. Move that function to a new sibling and the extraction returns empty, and the test does `t.Fatalf("could not locate updateSessionList in model.go")`. It fails loudly — the desired behaviour.
- **Assert-absence goes vacuous, silently.** `internal/tui/theme_panel_commit_test.go` asserts `theme_panel_commit.go` contains *zero* `ApplyTheme` call sites. Move the commit path to a new file and the assertion still passes: it now proves that a file which no longer holds the code does not call the thing. Green suite, dead guard.

**The tree already contains the countermeasure, applied to one side of a pair.** Immediately after the hollow assertion sits a companion asserting `theme_panel.go` holds *at least one* `ApplyTheme` site, commented *"it would pass over the commit path whatever that file held."* The anti-vacuity pattern was already invented here; it was simply not applied to the absence-assertion beside it.

**Production relocation has an ordering mechanism the test-side measurement never covered** *(resolves review-003 F2)*. The mechanical-safety work was scoped to test files (helpers stay visible in one package scope, no mid-function cuts, no build tags) and this subtopic covered filename-pinned guards; neither touches relocation *within production code*, now three of the seventeen in-scope files.

Go presents a package's files to the compiler in sorted filename order. `init()` functions run in that order, and package-level variables initialise in dependency order first, then declaration order across that same file sequence — so splitting a production file changes both. `cmd` is where this is concentrated: **seventeen `init()` functions**, every one registering onto the same root command, with `open.go` carrying one at line 712 plus nine package-level vars (`openCmd`, `pinResolvers`, `openDeps`, `openTUIFunc`/`openPathFunc`/`openSessionFunc`, `resolveLogger`, `themeLogger`, `openDomainPinFlags`). This is almost certainly the unnamed mechanism behind the earlier instinct that a bad seam in `open.go` "changes behaviour rather than merely relocating it", and it is adjacent to the measured leak — `TestCompletionHidesInternalSurface` failing with `candidates=[]` is an unrestored root-command mutation, and the root command is what those seventeen inits build.

Proving each case order-insensitive (cobra sorts commands for display, `log.For` is cached at package init) was rejected as the resolution: the proof would have to be redone by whoever next chooses a seam, and it is the kind of reasoning that is right until it quietly is not.

#### Decision

- **`init()` functions never move.** A file containing one keeps it, and keeps its package-level declarations with it. This makes such a split **init-order-neutral by construction** rather than by argument — nothing to prove, nothing to re-prove, and a reviewer can verify it by looking. Accepted cost: a residual may hold a variable whose only consumer now lives elsewhere. Untidy and safe, taken over tidy and argued.
- **Package-level variables may move with their concern only when the file has no `init()` and their initialisers are side-effect-free** (a sentinel `errors.New`, a literal, a const). Refinement forced by `tmux.go`, which dissolves entirely and so has no residual to hold declarations back: it has no `init()`, and its package-level state is `ErrOptionNotFound`, a literal string slice, and type/const declarations — order-independent by inspection. Where a file has an `init()`, the absolute rule above governs and no inspection is needed: `cmd/open.go` keeps `openCmd`, `pinResolvers`, `openDeps`, `openTUIFunc`/`openPathFunc`/`openSessionFunc`, `resolveLogger`, `themeLogger` and `openDomainPinFlags`.
- **The pure-relocation evidence carries to production files.** It rests on one-package-one-scope, which is identical for production and test code; ordering was the only production-specific hazard, and the rule above removes it.
- **Behaviour-preservation evidence for `tmux.go` and `open.go` is the full suite green in both lanes, plus the no-move rule.** The shuffle and inventory gates stay test-lane instruments and are not asked to cover what they cannot see.
- **The sweep carries an explicit guard-audit obligation.** For every filename-pinned guard whose target file loses code, either repoint it at the file the code moved to, or give it the anti-vacuity companion the tree already demonstrates. **A green suite after the sweep is not evidence the guards still cover anything** — this is the same failure `sourceguardtest.PackageGoFiles` was built to prevent (erroring on an empty match so a guard cannot pass by having stopped looking), one level up.
- **The audit is scoped by the distinction above**, not by the raw count: assert-presence guards announce their own breakage and need only repointing when they fire; assert-absence guards are the silent class and must be checked deliberately.
- Routed to `claude-md-and-enforcement`, not decided here: the review's second angle — that a concern-pinning guard (`applyThemeCallSitesIn(…) == 0` is literally *"this file does not own that concern"*) could be a **mechanism** for the exclusion test rather than only a hazard. It is a real option, but the exclusion test was already settled as a human check, so making concern-boundary guards a general obligation would be out of proportion.

---

### Claude Md And Enforcement

#### Context

`CLAUDE.md` is the standard's destination — the point of the work is that the discipline holds for future contributors and agents, which requires it written down where they read. It is also, uniquely, the one document loaded into **every** session's context unconditionally, which makes its size a permanent cost rather than a per-read one.

#### Journey

**The document is simultaneously the standard's home and the sweep's collateral** *(resolves review-001 F7)*. It names **69 distinct `.go` files across 84 mentions**, and several are claims a `model.go` split falsifies directly: `model.go`'s `rebuildSessionList` is described as "the single mode-aware re-render chokepoint" in two separate sections, and the outer canvas fill as "the last layer in `model.go`'s `View`". The same release therefore writes a file-organisation standard into the document and invalidates file-level claims already in it.

That half was called rather than debated: updating them is in scope. A stale map costs exactly what the standard exists to save — an agent trusts `CLAUDE.md`, goes to `model.go` for `rebuildSessionList`, and does not find it. Leaving it stale contradicts the rationale every other decision here rests on.

**The second half is an own-goal risk that only appeared once the references were counted.** `CLAUDE.md` is organised *by file*. If the sweep turns `model.go` into eight files and the convention is followed, the document gains eight entries; multiplied across the sweep it grows — and it is the one document whose cost is paid unconditionally on every run. A standard justified entirely by reducing context cost would, run through the existing documentation convention, *increase* the only context cost that is never optional.

#### Decision

- **Stale file-level claims are corrected as part of the sweep**, not left to fall out silently. Derivation: a map an agent trusts and that is wrong costs more than the read the standard saves.
- **`CLAUDE.md` is restructured away from file-indexing.** It describes **concerns and invariants**, naming a file only where the file itself is load-bearing — a guard that must not be dropped, a chokepoint everything routes through, an explicit "do not touch this". Filenames stop being the organising key.
- **The naming standard is what makes that affordable.** Once files are named for the behaviour they own, the filename is *predictable* — the notice-band code is in `notice_band.go` — so an index is redundant rather than merely expensive. What is lost is "where does X live?" lookup from the document; grep and gopls answer that better, and without rotting.

**The restructure was then bounded and given a stopping condition** *(resolves review-002 F6)*. As decided it had a direction but no scope and no success criterion, and the two readings differ by an order of magnitude: the package table is **26 rows, lines 58–85, and 31% of the document by word count** (3,413 of 10,969), so a broad reading means rewriting a third of `CLAUDE.md` in the same release as a 35,880-line sweep.

Measuring it also showed the document is closer to the target than the finding assumed. The table is indexed **by package**, not by file, and names files selectively within a row — the `tui` row names four (`grouping.go`, `session_item.go`, `model.go`, `restore.go`) out of roughly thirty in the package. There is no "document every file" convention to dismantle; the wanted convention is largely already the practice.

With one exception, which is the precedent that matters: the theming feature's entry enumerates **ten files for one concern** (`theme_panel.go`, `theme_panel_geometry.go`, `theme_panel_render.go`, `theme_panel_commit.go`, `theme_panel_confirm.go`, `theme_panel_message.go`, `theme_panel_footer.go`, `theme_row.go`, `theme_seams.go`, `theme_state.go`). That is what happened the last time a concern was split into siblings — each new file got documented. Repeated across a 17-file sweep it produces exactly the bloat the restructure exists to prevent. The anti-pattern is already in the tree, and not repeating it is the operative instruction.

#### Decision (restructure scope)

- **Narrow scope with a stated rule**, not a rewrite: correct the claims the sweep falsifies, applying *name a file only where the file itself is load-bearing* — a guard that must not be dropped, a chokepoint everything routes through, an explicit do-not-touch. In practice a broken reference is re-pointed at the **concern**, never replaced by an enumeration of the new siblings implementing it.
- **The budget excludes the standard's own section** *(resolves review-003 F4)*. The no-increase gate governs the rest of the document; the new standard text — cohesion rule, the name-exclusion test, the tripwire and what it is not, the marker form, the behaviour-area naming rule, the guard — is the deliverable and is not funded by shrinking anything. Derivation: a gate that forbids the deliverable is incoherent, and the only other way to pay for it is broadening the restructure past the narrow scope already chosen, which is the scope creep that reading exists to prevent. The standard section is nonetheless written tight — it is read every session like everything else in the file.
- **Stopping condition: `CLAUDE.md`'s word count must not increase**, measured over the document excluding the new standard section. Baseline **10,969 words**. Checkable in one command, targets the actual harm (the unconditional per-session cost), and demands no shrink that would drag unrelated sections into scope. A sweep that adds files and leaves the document no larger is the intended outcome; a shrink from applying the load-bearing rule is better.

#### Journey (enforcement)

**The tripwire is mechanically enforced by a source guard.** The pieces already exist: `internal/log/discard_guard_test.go` is the precedent for a repo-wide source guard (`portalbintest.ProjectRoot()`, walk the source, fail on a forbidden construction), `sourceguardtest.GoSourceFiles(root)` already performs the tree walk with its dot-dir and vendor exclusions, and the root package already carries test files (`main_test.go`, `main_panic_test.go`, `main_theme_fatal_test.go`), so a repo-wide structural guard has a home without inventing a package for it.

One difference from the precedent favours this guard. The discard guard carries a **hardcoded allow-list map** naming the single file permitted to construct a discard logger — remote from the code it exempts, and exactly the kind of list that rots. This guard needs none: the `// portal:oversized` marker in the file *is* the allow-list, so the exemption lives at the site it exempts and can only rot if the file itself does.

The case against was **exemption inflation** — an easy escape hatch teaching a marker-adding reflex, degrading the guard into ceremony. The mitigation is the marker's own contract: silencing the guard requires naming the single concern the file owns, which is a claim a reviewer can read and falsify, the same deterrent a `//nolint` with a required reason relies on. Soft, but not nothing. The no-guard alternative — standard in `CLAUDE.md`, agents read `CLAUDE.md`, sweep proves intent — was rejected because it relies on precisely the discipline that produced a 3,448-line `model.go` in a package already demonstrating the sibling pattern.

#### Decision (enforcement)

**Settled by derivation.** Determined by the tripwire's own stated purpose — a number exists to make someone stop, which only a mechanical check delivers — together with the sweep leaving zero files tripping at release, and the repo's established source-guard idiom.

- **A source guard enforces the tripwire.** It walks every `.go` file and fails on any over 1,000 lines lacking a `// portal:oversized` marker. Taken because it is the only mechanism that fires when someone writes the 1,001st line rather than at an audit nobody schedules — which is the stated reason for having a number at all; because the sweep leaves zero files tripping, making this the cheapest moment a guard will ever be introduced; and because it is the repo's existing idiom, twenty-five guards deep.
- **Raw line count, no refined counting.** Blank-and-comment subtleties buy nothing for an attention device.
- **The marker is the allow-list.** No hardcoded exemption map; grep is the enumeration.
- **Unit lane.** It is a hermetic source walk — no tmux, no daemon, no built binary.
- **Accepted risk:** exemption inflation, mitigated only by the marker's claim contract being reviewable.
- **Concern-pinning guards are not adopted as a general obligation.** A guard encoding "this file does not own that concern" (as `applyThemeCallSitesIn(…) == 0` already does) stays an available tool where a boundary is load-bearing, not a requirement per split. The exclusion test was settled as a human check; mandating a mechanical shadow of it would contradict that and multiply guards across every split file.

#### Open in this subtopic

- The exact wording of the standard as it lands in `CLAUDE.md` is not settled here — only what it must say: the cohesion rule, the exclusion test as its operative form, the 1,000 tripwire and what it is not, the marker form, the behaviour-area naming rule, and the guard.
- Where in a file the `// portal:oversized` marker must sit (leading doc comment, or anywhere the guard's text walk finds it), and whether the guard validates the date/commit fields' shape or merely greps the prefix.
- The marker necessarily records the commit it was *verified against* — the parent of the commit that introduces it. Harmless, but it needs saying wherever the form is written down, or the first author will think the field is wrong.

---

### Summary

#### Key Insights

1. **A file's name is the mechanism, not the symptom.** The finding that reframed the whole topic: every test file in the tree over 1,000 lines is named after a production file, and no behaviour-named file has run away. A name that excludes nothing is unbounded by construction — `theme_test.go` means "tests for theme.go", so everything theme-related goes in and the name never signals the file is full. Size is the visible consequence; naming is the cause. Everything else in the standard descends from this.
2. **A threshold should be calibrated to the practice worth keeping, not to an external anchor.** 2,000 was anchored to the agent read window and would never have fired on anything the tree does — a sweep selector wearing a standard's clothes. 1,000 sits just above the band Portal's own well-named files occupy, so tripping means something. And the number is a procedural attention device, never a bar to clear: a file under it is not thereby well-organised.
3. **Any staleness marker that must be maintained will not be.** Content hashes, line counts and mtimes all fail the same way — they require an edit every time the file changes, which across a handful of files over years will not happen, and an unmaintained marker lies. Only fields true at tagging time and never needing revision survive: a date and the commit verified against. Going stale then becomes information rather than error.
4. **Measurement is cheap here and repeatedly beat plausible reasoning.** The two-number split, the self-healing hypothesis, the build-tag hazard and the shuffle gate's guarantee each turned on a number or a source read rather than on argument: a `-shuffle` probe located a live order-dependency in `cmd` before a line moved, and Go's own `M.Run` showed the gate proves less than its shape suggests. The implication for the work is direct — when a claim about the tree or the toolchain is load-bearing, measure it, because the plausible answer was wrong more often than not.
5. **For ordering hazards, prevention beats detection.** `init()` order and package-level initialisation order both shift when a file splits, and nothing catches it. The rule that `init()` never moves makes the split order-neutral by construction — no proof to write, and none for the next person to redo.
6. **The most dangerous failure mode of a large relocation is silent test loss.** A test function dropped during a 35,880-line move leaves the suite green: no compiler error, no failing test, no signal from any order gate. It is more likely than a dropped build tag and invisible without an explicit inventory diff.

#### Open Threads

- **The standard's exact wording in `CLAUDE.md`**, plus the marker's required placement in a file and whether the guard validates its field shape. What the text must *say* is settled; how it reads is not.
- **`git blame` continuity across the relocation** is never argued anywhere in this document, in a topic that names accepted costs everywhere else. A 35,880-line move degrades line-level history; `blame -C` recovers most of it. Accepted implicitly rather than deliberately.
- **No contingency if the `cmd` order-dependency leak proves deeper** than the one reproduced unrestored root-command mutation. Five `cmd` test files mutate the root command, `cmd` holds seven of the seventeen in-scope files, and dropping `cmd` was explicitly rejected — so a larger leak has no recorded fallback.
- **Whether the exclusion test survives contact with two readers.** It is sharper than "one concern per file" but still a judgment call, accepted as unavoidable: the tripwire is the mechanical half and this is the half that needs a person.

#### Current State

**Settled.** The standard is a cohesion rule with two detectors — a 1,000-line tripwire applying to every Go file with no categories, and a name-exclusion test that bites at any size. A trip is rebutted in the file with a `// portal:oversized {date} @ {commit} — {claim}` marker naming the single concern the file owns, and a repo-wide source guard enforces the tripwire using that marker as its own allow-list. The sweep covers 17 files and 35,880 lines across 8 packages: everything over the line, plus `tmux.go` and `open.go`, the two below-the-line failures this discussion adjudicated. Verification is two gates — seeded shuffle runs (full set in the unit lane, one fixed seed in the integration lane for the four packages that have one) and a test-inventory diff — with the `cmd` order-dependency leak fixed first as a precondition. `init()` functions never move. Package-named files dissolve rather than reduce; `model.go` and `open.go` keep their names and must earn them. `CLAUDE.md` is restructured away from file-indexing under a no-increase word budget that excludes the standard's own section.

**Still uncertain.** How consistently the exclusion test lands between readers, and how large the `cmd` leak turns out to be — the only in-scope work whose size is not yet known.
