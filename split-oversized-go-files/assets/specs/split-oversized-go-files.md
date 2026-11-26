# Specification: Split Oversized Go Files

## Specification

### 1. The File-Organisation Standard

Portal sets its own file-organisation standard for Go source, recorded in `CLAUDE.md`. It is a **cohesion rule** — one concern per file within a package — carrying **two detectors** that work at different scales.

**Rationale.** The cost a large file imposes is context cost: a file too large to read in one pass forces chunked reads and burns the reader's context. Line count is the visible symptom, never the thing being measured.

#### 1.1 Detector one — the name-exclusion test

The operative form of the cohesion rule:

> A file's name must be specific enough to exclude things. If material that plausibly belongs in the package could be dropped into the file without contradicting its name, the name is a bucket label and the file will grow without bound.

- It bites at **any** size, including below the tripwire, and covers the whole range the tripwire is blind to.
- A name is judged on whether its concern has a **natural boundary**. Bounded by an external contract (an interface, a struct, a protocol) passes; an open-ended subject area — "everything about X" — does not, because it can accrete indefinitely.
- The test is evaluated against **what a file holds**, not against its name in the abstract: a name can only exclude things relative to a concern.
- **Mirror-naming is a correlate of failure, never the test itself.** A test file named after a production file is usually a bucket because a production filename is the broadest label available — but 217 of 595 test files are mirror-named and most are well-bounded.
- It is a **judgment call**, accepted as such: sharper than "one concern per file", not mechanical, and two readers may differ on a borderline name. The tripwire is the mechanical half; this is the half that needs a person.

#### 1.2 Detector two — the line tripwire

**One tripwire, 1,000 lines, every Go file. The rule names no categories** — no production/test classification, no exemption class.

- A file over 1,000 lines is **presumed** to hold more than one concern and must justify itself. Tripping is never itself a violation.
- The number is a **procedural attention device, not a quality metric**. Its job is to make someone stop and check, because cohesion is hard to judge and otherwise nobody does. A file under 1,000 lines is not thereby well-organised.
- Calibration: 1,000 sits just above the band Portal's own well-named files already occupy — production tops at 775, behaviour-named test files at 997 — so a properly-organised file essentially never trips, and a trip means something.
- **Raw line count.** No blank-or-comment refinement: subtlety buys nothing for an attention device.
- The read-cost asymmetry between file kinds lives in the **strength of the written claim**, not in a file category. A repetitive-by-construction file justifies itself in a sentence; a file holding several concerns cannot. Same detector, different burden of proof, adjudicated on the claim rather than on the `_test.go` suffix.

#### 1.3 Test-file organisation

Test files are organised **per behaviour area, one purpose-named file each** — finer-grained than a 1:1 production mirror (`cmd/open_targets.go` is 75 lines and carries two test files, `open_targets_test.go` and `open_targets_guard_test.go`). Naming a test file after the behaviour it covers is **how it passes the exclusion test**, not a separate prohibition.

#### 1.4 File count is cheap

Many small files beat one large one. There is **no lower bound**, and no "that's too many files" objection to a proposed split.

---

### 2. The `// portal:oversized` Marker

A file that trips the tripwire rebuts the presumption **in the file**, with a marker of this form:

```go
// portal:oversized 2026-08-16 @ f238265 — Fixture definitions only; each TUI
// screen contributes one. No logic lives here.
```

#### 2.1 Why in the file

A rebuttal stored anywhere else — an allow-list, a doc, a commit message — forces a second read to answer a question raised by the first. In the file, the answer rides the read already being made. This follows from the standard's own rationale and fits the surviving convention in the tree: 90 of 870 Go files still carry a leading doc comment.

The rebuttal must be **durable**, not re-derived. Without a recorded rebuttal a tripping file is re-adjudicated by every agent that opens it, which is precisely the context cost the standard exists to remove.

#### 2.2 What the claim must say

The claim **states the single concern the file owns**, so a later reader can falsify it by reading. A bare "large on purpose" rots silently — the file gains a second concern and the note keeps vouching for it.

#### 2.3 What the marker records, and what it must not

Only fields **true at tagging time that never need updating**: the **date** and the **commit verified against**.

- **No content hash** — self-invalidates on every edit, including a typo in the marker itself, and cannot include itself without excluding its own line.
- **No line count** — a poor proxy in both directions: a file can absorb a whole new concern at net-zero line change, or grow 200 lines of the same table and look drifted when nothing changed.
- **No mtime** — not committed, and reset by checkout.

All three must be maintained on every edit, and across a handful of files over years they will not be. **A stale marker that lies is worse than no marker.** A date and a commit simply age: they assert "this claim was checked against this state", so going stale is information rather than error. The commit reference is strictly better than a hash — it does not self-invalidate, and it yields the actual diff on demand (`git diff <sha>..HEAD -- <file>`). The date is nearly redundant against the commit and is kept because it reads at a glance with no tooling.

#### 2.4 The commit field's semantics

The commit recorded is the one the claim was **verified against** — the parent of the commit that introduces the marker, not the commit that introduces it. This must be stated wherever the form is written down, or the first author will think the field is wrong.

#### 2.5 Placement

**Decision required — the discussion left this open.** The marker must sit in the file's **leading comment block**, above the `package` clause.

**Derivation:** §2.1 places the rebuttal in the file so it rides the read already being made — which only holds if the reader meets it first. A marker buried at line 900 of a 1,200-line file is discovered by grep, not by reading, and grep is the property §2.6 needs, not the property §2.1 asked for. The leading position is also the one the tree's surviving doc-comment convention already occupies.

**Accepted cost:** a file whose leading comment block already carries a doc comment gains a second paragraph there.

#### 2.6 The prefix is a namespace

`portal:` is free — no comment pragma of that form exists in the tree today. The `// portal:oversized` prefix is greppable, and grep over it is the enumeration of every rebutted file: **the marker is its own allow-list**, so no separate exemption list exists to rot.

---

### 3. The Tripwire Source Guard

A repo-wide source guard mechanically enforces the tripwire.

#### 3.1 Behaviour

It walks every `.go` file in the repository and **fails on any file over 1,000 lines that does not carry a `// portal:oversized` marker**. Raw line count, per §1.2.

#### 3.2 Why a guard exists at all

A number whose stated job is to make someone stop only delivers that if something fires when the 1,001st line is written, rather than at an audit nobody schedules. The no-guard alternative — standard in `CLAUDE.md`, agents read `CLAUDE.md`, the sweep proves intent — relies on precisely the discipline that produced a 3,448-line `model.go` in a package already demonstrating the sibling pattern.

The sweep leaves **zero files tripping** at release, which makes this the cheapest moment a guard will ever be introduced: a guard born with a fifteen-entry exemption list protects nothing.

#### 3.3 Construction

It follows the repo's established source-guard idiom, ~25 guards deep:

- `internal/log/discard_guard_test.go` is the precedent for a repo-wide source guard — `portalbintest.ProjectRoot()`, walk the source, fail on a forbidden construction.
- `sourceguardtest.GoSourceFiles(root)` already performs the tree walk with its dot-dir, `vendor` and `node_modules` exclusions, and is the walk this guard uses.
- The root package already carries test files (`main_test.go`, `main_panic_test.go`, `main_theme_fatal_test.go`), so a repo-wide structural guard has a home without inventing a package for it.

**Unit lane.** It is a hermetic source walk — no tmux, no daemon, no built binary.

#### 3.4 No exemption list

Unlike the discard guard, which carries a hardcoded allow-list map naming the single file permitted to construct a discard logger — remote from the code it exempts, and exactly the kind of list that rots — this guard needs none. The marker in the file **is** the allow-list, so the exemption lives at the site it exempts and can only rot if the file itself does.

#### 3.5 What the guard checks in the marker

**Decision required — the discussion left this open.** The guard **greps the `// portal:oversized` prefix only**. It does not validate the date's or commit's shape, and it does not read the claim.

**Derivation:** every field the guard could validate is one the marker deliberately does not maintain (§2.3) — a shape check on a date or a SHA confirms only that characters were typed in the right pattern, never that the claim was verified or is still true. The thing that makes a marker honest is the claim, which is unvalidatable by construction and is deliberately left to a reviewer (§1.1, §3.6). A validator that checks the two fields it can and skips the one that matters converts a human obligation into a passed test.

**Accepted cost:** `// portal:oversized` with no fields and no claim silences the guard. That is the same escape hatch §3.6 already accepts, not a new one.

#### 3.6 Accepted risk — exemption inflation

An easy escape hatch can teach a marker-adding reflex and degrade the guard into ceremony. The only mitigation is the marker's own contract: silencing the guard requires naming the single concern the file owns, which is a claim a reviewer can read and falsify — the same deterrent a `//nolint` with a required reason relies on. Soft, and accepted as the residual.

#### 3.7 Concern-pinning guards are not a general obligation

A guard encoding "this file does not own that concern" — as `applyThemeCallSitesIn(…) == 0` already does in `internal/tui/theme_panel_commit_test.go` — remains an available tool where a boundary is load-bearing. It is **not** a requirement per split. The exclusion test is settled as a human check; mandating a mechanical shadow of it would contradict that and multiply guards across every split file.

---

### 4. Sweep Scope

#### 4.1 Posture

The release makes the mechanical detector **true on day one**. The alternative — ship the standard as forward-looking with a backlog — is closed off by the marker's own contract: a monster cannot claim to own one concern, and "this is a bucket and we have not got to it yet" is an admission rather than a rebuttal. There is no honest way to ship the standard while leaving known violators marked-and-standing.

The mechanical half can be made true and then held by a guard. The exclusion test is the human half by construction, so it applies to work as it happens.

#### 4.2 The in-scope set — 17 files, 35,880 lines, 8 packages

Everything over the 1,000-line tripwire, plus the two below-the-line exclusion-test failures this work has already adjudicated.

| File | Lines | Package clause | In scope because |
|---|---|---|---|
| `internal/tui/model_test.go` | 7,116 | `tui_test` | over the line |
| `internal/tmux/portal_saver_test.go` | 3,772 | `tmux_test` | over the line |
| `cmd/open_test.go` | 3,581 | `cmd` | over the line |
| `internal/tui/model.go` | 3,448 | `tui` | over the line |
| `internal/tmux/tmux_test.go` | 3,211 | `tmux_test` | over the line |
| `cmd/state_hydrate_test.go` | 1,849 | `cmd` | over the line |
| `internal/state/capture_test.go` | 1,626 | `state_test` | over the line |
| `cmd/doctor_test.go` | 1,495 | `cmd` | over the line |
| `internal/hooks/store_test.go` | 1,425 | `hooks_test` | over the line |
| `cmd/bootstrap/bootstrap_test.go` | 1,287 | `bootstrap` | over the line |
| `cmd/state_daemon_run_test.go` | 1,217 | `cmd` | over the line |
| `cmd/state_commit_now_test.go` | 1,163 | `cmd` | over the line |
| `internal/resolver/query_test.go` | 1,129 | `resolver_test` | over the line |
| `cmd/theme_test.go` | 1,035 | `cmd` | over the line |
| `internal/theme/resolution_test.go` | 1,016 | `theme_test` | over the line |
| `internal/tmux/tmux.go` | 775 | `tmux` | adjudicated exclusion-test failure |
| `cmd/open.go` | 735 | `cmd` | adjudicated exclusion-test failure |

Over-the-line subtotal: 15 files, 34,370 lines. The two adjudicated failures add 1,510.

Packages touched: `internal/tui`, `internal/tmux`, `cmd`, `cmd/bootstrap`, `internal/state`, `internal/hooks`, `internal/resolver`, `internal/theme`.

`cmd` alone holds seven of the seventeen.

#### 4.3 Why the two below-the-line files are in

`internal/tmux/tmux.go` (775) wraps sessions, windows and panes, hook-key derivation, environment, server lifecycle, options and global hooks — comfortably more than five concerns, and permanently invisible to any line detector. `cmd/open.go` (735) fails the same way.

Both were **already adjudicated** in this work. Excluding them would discard work already done and would ship a standard true on the mechanical detector and knowingly false on the human one, for files someone has actually examined. They cost 1,510 lines against a 34,370-line sweep.

Neither can carry a `// portal:oversized` marker to defer — they are under the line, and the marker's contract rules out "not got to it yet" regardless.

#### 4.4 Unadjudicated exclusion-test failures stay out, explicitly

The human detector applies to work as it happens. Hand-auditing 275 production files before release is not practical, and the two above are in *because* they were already adjudicated — a set bounded by construction at two, not the start of an audit.

**No second marker.** A `// portal:split-pending` (or similar) declaring a known-but-deferred failure was rejected on two grounds. First, the two detectors are not symmetric: the tripwire presumes guilt and demands an answer, so 1,150 lines obliges a decision; the exclusion test carries no presumption — it is the question asked when deciding where to put code. Nobody is required to adjudicate `tmux.go`, so there is no standing verdict to record, and the derivation happens only when someone is about to act on it, which is when it is cheapest because they are already reading. Second, having declined the 275-file audit, only the failures someone happened to notice would ever be marked, which is arbitrary.

#### 4.5 `cmd/open.go` is the sweep's highest-risk file

It is the product's main entry path — resolution grammar, domain pinning, TUI construction, burst dispatch — and **the only file in scope where a bad seam changes behaviour rather than merely relocating it**. Everything else in scope is a test file, `model.go` (clean panel-shaped seams), or `tmux.go` (a method-bag wrapper that splits by method group). It is taken anyway, understood as such.

#### 4.6 `cmd` is not deferred

Dropping `cmd` from the sweep would cut the scope from seventeen files to ten, but `cmd` holds six of the fourteen over-the-line test files plus `open.go`. Leaving them standing breaks the day-one posture, and the marker cannot honestly paper over it — so deferring `cmd` reinstates the forward-looking posture by the back door. The deciding factor: dropping `cmd` costs the posture that makes a guard possible at all, whereas the order-dependency leak it carries (§7.1) is a real bug that will bite regardless of this work.

---

### 5. Seam Selection

Go makes the mechanical half of a split free within a package — files split with no import, caller or test changes — so seam choice is the judgment half of the work. A proposed seam is judged against the exclusion test (§1.1).

#### 5.1 Completion condition

**Every in-scope file's completion condition is the exclusion test, not the tripwire.** For the two below-the-line files the tripwire was never the reason they are in scope and cannot signal that their split is done.

#### 5.2 A package-named file is dissolved, not reduced

A residual keeps its name only if the split can make that name exclude things. A **package-named file can never reach that state** — "could any material in this package go in?" is permanently yes. There is therefore no legitimate residual, and the file ceases to exist.

Three in-scope files are package-named and all three dissolve:

| File | Lines | Clause | Stem |
|---|---|---|---|
| `internal/tmux/tmux.go` | 775 | `tmux` | `tmux` |
| `internal/tmux/tmux_test.go` | 3,211 | `tmux_test` | `tmux` |
| `cmd/bootstrap/bootstrap_test.go` | 1,287 | `bootstrap` | `bootstrap` |

`internal/resolver/query_test.go` is **not** package-named — its clause is `resolver_test` and its stem is `query`, so it splits under the ordinary rules.

The stem is what is judged, so an external-clause test file named after the package (`tmux_test.go` in `package tmux_test`) dissolves on the same footing as the production file. There is no test-file exemption to invoke: the rule names no categories (§1.2).

`tmux.go`'s 53 functions fall into method groups already visible in its outline — the `Commander` seam and command execution; the `Client` type and its constructors; server lifecycle; session operations; window and pane operations; pane-target formatting; hook-key and structural-key derivation primitives; server and session options; the raw global-hook operations (which join the package's existing `hooks_*` family, distinct from the registration logic in `hooks_register.go`); and environment. Roughly ten files averaging under eighty lines, which §1.4 settles without argument. **This grouping is indicative, not binding** — the decision is the criterion each file must meet, not the particular grouping that satisfies it.

#### 5.3 A residual keeps its name, and the split has to earn it

Draining a bucket-named file does not require renaming the file that remains — but the remainder must be reduced until the name **genuinely excludes things, judged against contents rather than against the name alone** (§1.1).

Keeping the name is also what preserves the filename-pinned guards and `CLAUDE.md` entries that point at the file — they stay valid because the file still exists and still holds the concern it names.

#### 5.4 `internal/tui/model.go` — acceptance criterion

Post-split `model.go` holds:

- the `Model` type,
- its construction,
- the three `tea.Model` methods (`Init`, `Update`, `View`),
- the `page` enum the router switches on,
- the accessors that expose **model-level** rather than page-level state,

**and nothing else.** If `rebuildSessionList` or the canvas-fill machinery is still there, the split is not finished. Any 400–600 line figure is indicative and never a target: the completion condition is the exclusion test, not an arithmetic one.

Today the file fails because it holds ten concerns — the model type, the option constructors, list styling, sizing arithmetic, the grouping glue, the update router, the projects page and its edit modal, the sessions-page key arms, view composition, and roughly 160 lines of ANSI SGR rewriting. Stripped to the above, the name does exclude things: edit-modal chip logic, canvas backfill and projects-page arms all contradict it.

What makes that legitimate rather than special pleading is §1.1's natural-boundary property. `model.go` holding the type and its interface methods is bounded by an **external contract** — `tea.Model` has exactly three methods and a struct is a struct, so nothing can accrete. `cmd/theme_test.go` is "everything about the theme command", bounded by nothing, which is why it reached 1,035 lines.

#### 5.5 `cmd/open.go` — acceptance criterion

`open.go` **keeps its name** and survives as a residual, because it holds an `init()` and is therefore governed by the absolute no-move rule (§6.1).

Its residual is the `openCmd` declaration, flag registration, `init()`, and the package-level vars. That earns the name: new resolution behaviour, TUI construction or path-opening logic all contradict "the open command's declaration and wiring" and belong in siblings. The package already establishes that family — `open_targets.go`, `open_surfaces.go`, `open_burst.go`, `open_burst_run.go` — so the split extends a convention rather than inventing one.

What leaves, indicatively: the two connectors and TUI-result handling; `openSession`; the path-opening chain (`PathOpener.Open`, `openPath`, the quick-start and execer adapters); the resolution chain (`resolvePinAndOpen`, `openResolved`, `buildQueryResolver`, `emitResolveDecision`, `resolveDecision`); ack-marker writing; command-argument parsing; theme loading and resolution; and TUI construction (`buildTUIModel`, `openTUI`).

#### 5.6 The package clause is a boundary seam selection respects, never crosses

**All eight in-scope directories are dual-clause** — `tui`/`tui_test`, `tmux`/`tmux_test`, `cmd`/`cmd_test`, `bootstrap`/`bootstrap_test`, `state`/`state_test`, `hooks`/`hooks_test`, `resolver`/`resolver_test`, `theme`/`theme_test`.

A sibling **inherits its source file's clause by default** — splitting `model_test.go` yields `package tui_test` siblings. A sibling declaring the internal clause sees none of the external clause's helpers and cannot reach an unexported identifier, and vice versa.

Getting this wrong is a **compile error**, so it is a constraint on seam choice rather than a runtime hazard, and no verification gate needs to catch it.

#### 5.7 `<behaviour>_internal_test.go` — the established paired form

A behaviour area may be a **pair** of files where the clause forces one, and `<behaviour>_internal_test.go` is the established form for the internal half. The behaviour part of the name does the excluding and satisfies the exclusion test; `_internal` is a clause qualifier riding along.

The tree already runs this convention in seven files: `internal/state/capture_internal_test.go`, `internal/state/daemon_state_internal_test.go`, `internal/theme/load_internal_test.go`, `internal/theme/union_internal_test.go`, `internal/tmux/exact_target_internal_test.go`, `internal/tmux/option_discriminator_internal_test.go`, `internal/tui/loading_fatal_internal_test.go`.

This is the one point where §1.4 meets a language constraint rather than a preference, and the constraint wins because it is not negotiable.

#### 5.8 Declaration-only material distributes to the behaviour file that owns it

**No exemption from the exclusion test.** `model.go` holds roughly 290 lines of declarations: thirteen exported DI seam interfaces (`SessionLister`, `PreviewAttacher`, `ProjectStore`, `SessionKiller`, `SessionCreator`, `SessionRenamer`, `ModePersister`, `ThemePersister`, `ProjectEditor`, `AliasEditor` and the theme/preview seams), eight exported message types plus `LoadingMinDuration` and the `page`/`editField`/`editMode` enums, and around thirty-five one-line accessor methods that exist because `model_test.go` is an external `package tui_test`.

The obvious destinations are the ones the standard forbids: a bare `seams.go`, `messages.go` or `accessors.go` in package `tui` excludes nothing and is a bucket label by the same reasoning that dissolves `tmux.go`. Exempting declarations from the test was also rejected — declarations accumulating unchecked is part of how `model.go` reached 3,448 lines, and a `messages.go` would grow without bound for exactly the reason the test exists to catch.

**Each declaration goes where its behaviour went**: the session seams and `SessionsMsg`/`SessionCreatedMsg` with the sessions code, the project seams and `ProjectsLoadedMsg` with the projects code, the bootstrap and loading messages with the loading page, `editField`/`editMode` with the edit modal, the `With…` options with the concern each configures. `theme_seams.go` is the in-tree proof that this produces bounded names — it excludes every seam that is not a theme seam.

Nothing blocks it mechanically: every file in package `tui` can declare methods on `Model`, so distributing accessors costs nothing and the external test package is unaffected.

---

### 6. Refactor Safety

The sweep is a large mechanical move: code changes files without changing behaviour. That is true for the compiler, but not for anything in the repo keyed to a **filename** rather than to a symbol, nor for anything keyed to **file order**.

#### 6.1 `init()` functions never move — absolute

A file containing an `init()` **keeps it, and keeps its package-level declarations with it.**

Go presents a package's files to the compiler in sorted filename order. `init()` functions run in that order, and package-level variables initialise in dependency order first, then declaration order across that same file sequence — so splitting a production file changes both, and nothing catches it.

`cmd` is where this concentrates: **seventeen `init()` functions**, every one registering onto the same root command, with `open.go` carrying one at line 712 plus nine package-level vars (`openCmd`, `pinResolvers`, `openDeps`, `openTUIFunc`, `openPathFunc`, `openSessionFunc`, `resolveLogger`, `themeLogger`, `openDomainPinFlags`).

The rule makes such a split **init-order-neutral by construction** rather than by argument — nothing to prove, nothing for the next person to re-prove, and a reviewer can verify it by looking. Proving each case order-insensitive (cobra sorts commands for display, `log.For` is cached at package init) was rejected as the resolution: the proof would have to be redone by whoever next chooses a seam, and it is the kind of reasoning that is right until it quietly is not.

**Accepted cost:** a residual may hold a variable whose only consumer now lives elsewhere. Untidy and safe, taken over tidy and argued.

#### 6.2 Package-level variables may move only under both conditions

A package-level variable may move with its concern **only when the file has no `init()` and its initialiser is side-effect-free** — a sentinel `errors.New`, a literal, a const.

Where a file has an `init()`, §6.1 governs absolutely and no inspection is needed.

This exists for `tmux.go`, which dissolves entirely (§5.2) and so has no residual to hold declarations back: it has no `init()`, and its package-level state is `ErrOptionNotFound`, a literal string slice, and type/const declarations — order-independent by inspection.

#### 6.3 Filename-pinned source guards — the audit obligation

**46 test files hardcode a `.go` filename.** Some are temp-dir fixtures (`alpha.go`, `kept.go`, `thing.go`) and are irrelevant; the rest pin real source files — `model.go`, `tmux.go`, `theme.go`, `open.go`, `doctor.go`, `theme_panel.go`, `theme_panel_commit.go`, `restore.go`, `pagepreview.go`, `setting.go`, `state_daemon.go`, `harness.go`, `modal.go`.

They fail in **opposite** ways, and that distinction scopes the audit:

- **Assert-presence self-destructs, safely.** `internal/tui/pagepreview_filter_test.go` reads `model.go`, extracts `updateSessionList`'s body and counts `tea.KeySpace` occurrences. Move that function to a new sibling and the extraction returns empty, and the test does `t.Fatalf("could not locate updateSessionList in model.go")`. It fails loudly — the desired behaviour.
- **Assert-absence goes vacuous, silently.** `internal/tui/theme_panel_commit_test.go` asserts `theme_panel_commit.go` contains *zero* `ApplyTheme` call sites. Move the commit path to a new file and the assertion still passes: it now proves that a file which no longer holds the code does not call the thing. Green suite, dead guard.

**The obligation.** For every filename-pinned guard whose target file loses code, either **repoint it at the file the code moved to**, or **give it the anti-vacuity companion the tree already demonstrates**.

The tree already contains that countermeasure, applied to one side of a pair: immediately after the hollow assertion sits a companion asserting `theme_panel.go` holds *at least one* `ApplyTheme` site, commented *"it would pass over the commit path whatever that file held."* The pattern was invented here; it was simply not applied to the absence-assertion beside it.

**The audit is scoped by the presence/absence distinction, not by the raw count of 46.** Assert-presence guards announce their own breakage and need only repointing when they fire. **Assert-absence guards are the silent class and must be checked deliberately.**

**A green suite after the sweep is not evidence the guards still cover anything.** This is the same failure `sourceguardtest.PackageGoFiles` was built to prevent — erroring on an empty match so a guard cannot pass by having stopped looking — one level up.

#### 6.4 Pure relocation, and what carries to production files

**The split is a pure relocation of whole top-level functions.** Helpers stay where they are and stay visible **within a package clause** (§5.6); nothing is re-derived or duplicated; no function is cut mid-body. Verified against the four largest test files:

- Go test files sharing a clause share a single scope, so a helper defined in `model_test.go` stays visible to a relocated test provided the sibling declares the same clause. This matters given the density: `cmd/open_test.go` carries 47 non-test top-level funcs and 23 package-level vars, `portal_saver_test.go` 42 helpers.
- **No mid-function splitting is required.** Largest test function per file: `portal_saver_test.go` 77 lines, `open_test.go` 234, `tmux_test.go` 274. `model_test.go` is the outlier — `TestCommandPendingMode` 479, `TestProjectsPage` 443, `TestLoadingPage` 415 — but each is already a behaviour area and moves whole. The unit of movement is the entire `func TestX`.

The pure-relocation evidence **carries to production files**: it rests on one-package-one-scope, which is identical for production and test code. Ordering was the only production-specific hazard, and §6.1 removes it.

#### 6.5 Build tags

**None of the seventeen in-scope files carries a build tag**, `internal/tmux/tmux.go` and `cmd/open.go` included. Tree-wide there are **47 integration-tagged test files and the largest is 710 lines**, so no tagged file is in scope at all.

That is not luck: the repo already segregates integration tests by *filename*, and `*_integration_test.go` is a name that excludes things — new integration material cannot drift into a unit-lane file without contradicting its name.

`cmd/state_commit_now_test.go` (1,163) is plain **unit-lane** despite `CLAUDE.md` naming "the commit-now suite"; the commit-now integration tests live in separate files (`state_commit_now_symptom_integration_test.go` 365, `_reentrancy_` 176, `_daemon_merge_` 129).

**Build tags bind on later splits only.** Any future split that does touch a tagged file must carry `//go:build integration` to every new sibling; the inventory gate (§7.4) is what detects a failure to.

#### 6.6 Behaviour-preservation evidence for the three production files

For `internal/tui/model.go`, `internal/tmux/tmux.go` and `cmd/open.go`, the evidence is **the full suite green in both lanes, plus the no-move rule of §6.1**. The shuffle and inventory gates stay test-lane instruments and are not asked to cover what they cannot see.

#### 6.7 `git blame` continuity — accepted cost

A 35,880-line relocation degrades line-level history. `git blame -C` recovers most of it. Accepted; no mitigation is built.

---

### 7. Verification Gates

#### 7.1 The `cmd` order-dependency leak is fixed first, as a precondition

Go runs tests in source order, and files in lexical filename order, so **splitting changes execution order**. The `cmd` package injects mocks through package-level mutable state (`openDeps`, `bootstrapDeps`, `doctorDeps`) cleaned via `t.Cleanup()`; a leaked cleanup makes a test order-dependent, and reordering exposes it — arriving mid-refactor looking exactly like sweep damage.

Measured across the tree: **every package passes shuffled except `cmd`**, including `internal/tui` and `internal/tmux`, the two largest sweep targets. `cmd` fails intermittently, on roughly **one shuffle order in three**. Reproduced: `TestCompletionHidesInternalSurface/top-level completion excludes the hidden state namespace` fails with `candidates=[]` — top-level completion offering nothing at all, meaning an earlier test mutated the root command and did not restore it. Five `cmd` test files mutate the root command, `open_test.go` among them.

This is a **pre-existing latent bug**, present today and independent of any split. It is fixed **as its own change, independently verifiable, not folded into a split commit**. Shuffle-clean is a precondition of splitting a package, and `cmd` does not currently meet it.

#### 7.2 Gate one — the seeded shuffle run

**A fixed seed set, `go test -shuffle=N` over seeds `1`–`10`, per package, before and after each split.**

**Never `-shuffle=on`.** Reproducibility is the whole of the advantage: a red run must be replayable from the seed that produced it. `-shuffle=on` reseeds randomly per invocation, so a red run cannot be reproduced from the command that produced it. The measurement that located the `cmd` leak is the evidence — run 1 passed, run 2 passed, run 3 failed. A one-run gate would have declared `cmd` clean twice before anyone got unlucky, and catching that failure is the entire reason the gate is trusted.

Ten is a **working figure, not a derived one** — overwhelming against a 1-in-3 dependency, thin against a 1-in-50. Accepted on the reasoning that a rarer order-dependency survives the sweep unchanged rather than being caused by it.

**What the gate does not prove.** In `$(go env GOROOT)/src/testing/testing.go`, `-shuffle=N` seeds a PRNG that shuffles `m.tests` **in place**, and `m.tests` arrives in source order — so a split changes the input and the same seed yields a *different* run order afterwards. The gate does **not** put the same permutations on both sides, and it does not prove a split "introduced no order coupling". It samples ten orderings before and ten different orderings after. **Accepted residual:** an order-dependency the sampled seeds miss was equally present before the split, so it is an undiscovered latent flake rather than damage the sweep caused. `cmd` is where that difference will be felt.

**A weakened gate for `cmd` was rejected on the record** — splitting under a red-before/red-after run on the argument that it proves nothing either way. A gate that cannot fail is not a gate, and it would put the leak inside the sweep's blast radius instead of ahead of it.

#### 7.3 Lane split for the shuffle gate

**The full seed set runs in the unit lane; a single fixed seed runs in the integration lane**, and only for packages that carry integration tests — `cmd` (15 tagged test files), `cmd/bootstrap` (18), `internal/tmux` (3), `internal/state` (1). The other four in-scope packages have none, `internal/tui` among them, so splits there cannot disturb an integration lane at all.

The two lanes are asked different questions. The unit lane is asked *is this package order-independent* — a property, which sampling ten orderings probes, and which is cheap enough to run. The integration lane is asked *did this split change anything under a fixed ordering* — a comparison, which one ordering answers. Ten seeded integration runs per package per split, at `-p 1`, with no CI and on the repo's most timing-coupled tests, would buy a stronger claim about a property nobody asserted, at a cost high enough that the gate would stop being run — and a gate nobody runs is worse than a cheaper one they do.

**Accepted residual:** an integration test order-dependent in a way the fixed seed misses was equally order-dependent before the split.

#### 7.4 Gate two — the test-inventory diff

```
go test -list '.*' ./...          # sorted, before and after — must be identical
```

**This is a before/after identity check on the same tree, not a comparison against a recorded constant.** For orientation, measured at specification time: **3,707 tests in the unit lane, 3,792 with the integration tag** (85 integration-only).

It catches two failures in one command:

- **Silent test loss** — the most dangerous failure mode of a large relocation. A test function dropped during a 35,880-line move leaves the suite **green**: no compiler error, no failing test, and no signal from the shuffle gate. The list *shrinks*.
- **A dropped `//go:build integration` tag** — which moves a test from integration-only into both lanes, so the unit list *grows*.

It is stronger than a tag check because it asserts **identity** rather than a property.

**Accepted limit:** the inventory cannot see a test moved *and* quietly altered — it checks identity, not content. A pure relocation does not alter bodies (§6.4), and the suite passing covers the rest.

#### 7.5 If the `cmd` leak proves deeper

**Decision required — the discussion named this an open thread and closed nothing around it.**

**The obligation is unbounded: the leak is fixed however deep it goes.** Five `cmd` test files mutate the root command and only one unrestored mutation has been reproduced, so the true size is the one piece of in-scope work whose extent is not yet known.

**Derivation:** the two alternatives are already closed on the record — dropping `cmd` was rejected because it breaks the day-one posture that makes the guard possible, and weakening the gate was rejected because a gate that cannot fail is not a gate. The deciding argument for taking `cmd` on survives the leak being larger: it is a real test-isolation bug that will bite regardless of this work, in the package that already holds the repo's most fragile mock-injection state.

**Sequencing hedge: `cmd` is split last**, after the other seven packages are complete and verified. This is ordering only — not the rejected exclusion — and it is free: it keeps a deeper-than-expected leak from holding seven finished package splits hostage, and it means the decision to cut, if it is ever taken, is taken with the leak's real size known rather than guessed.

**Accepted cost:** the highest-risk file in the sweep (`cmd/open.go`, §4.5) is also the last touched, so the sweep's riskiest work lands when release pressure is highest. Taken because the alternative — leading with `cmd` — puts the unbounded item first and blocks everything behind it.

---

### 8. `CLAUDE.md` Changes

`CLAUDE.md` is the standard's destination — the discipline only holds for future contributors and agents if it is written where they read. It is also the one document loaded into **every** session's context unconditionally, which makes its size a permanent cost rather than a per-read one.

#### 8.1 The document is also the sweep's collateral

It names **68 distinct `.go` files across 83 mentions**, and several are claims the sweep falsifies directly:

- `model.go`'s `rebuildSessionList` is described as "the single mode-aware re-render chokepoint" in **two separate sections** — line 67 (the `tui` package-table row) and line 185 (the session-grouping section). A third mention at line 191 constrains its behaviour rather than locating it.
- The outer canvas fill is described as "the last layer in `model.go`'s `View`" at line 174.

**Stale file-level claims are corrected as part of the sweep**, not left to fall out silently. A map an agent trusts and that is wrong costs more than the read the standard saves: the agent goes to `model.go` for `rebuildSessionList` and does not find it. Leaving it stale contradicts the rationale every other decision here rests on.

#### 8.2 The restructure — direction

`CLAUDE.md` is restructured **away from file-indexing**. It describes **concerns and invariants**, naming a file only where the file itself is load-bearing:

- a guard that must not be dropped,
- a chokepoint everything routes through,
- an explicit "do not touch this".

Filenames stop being the organising key.

**The naming standard is what makes that affordable.** Once files are named for the behaviour they own, the filename is *predictable* — the notice-band code is in `notice_band.go` — so an index is redundant rather than merely expensive.

**What is lost:** "where does X live?" lookup from the document. Grep and gopls answer that better, and without rotting.

#### 8.3 The restructure — scope

**Narrow, with a stated rule. Not a rewrite.**

The document is closer to the target than it first appears. The package table is indexed **by package**, not by file, and names files selectively within a row — the `tui` row names four (`grouping.go`, `session_item.go`, `model.go`, `restore.go`) out of roughly thirty in the package. There is no "document every file" convention to dismantle; the wanted convention is largely already the practice.

A broad reading would mean rewriting **31% of the document** (the table is 3,419 of 10,969 words, 26 rows at lines 58–85) in the same release as a 35,880-line sweep.

**The scope is therefore: correct the claims the sweep falsifies, applying the rule *name a file only where the file itself is load-bearing*.** In practice a broken reference is **re-pointed at the concern**, never replaced by an enumeration of the new siblings implementing it.

**The anti-pattern to not repeat.** The theming feature's entry enumerates **ten files for one concern** — `theme_panel.go`, `theme_panel_geometry.go`, `theme_panel_render.go`, `theme_panel_commit.go`, `theme_panel_confirm.go`, `theme_panel_message.go`, `theme_panel_footer.go`, `theme_row.go`, `theme_seams.go`, `theme_state.go`. That is what happened the last time a concern was split into siblings: each new file got documented. Repeated across a 17-file sweep it produces exactly the bloat the restructure exists to prevent.

#### 8.4 Stopping condition — the word budget

**`CLAUDE.md`'s word count must not increase**, measured over the document **excluding the new standard section**. Baseline: **10,969 words**.

Checkable in one command, targets the actual harm (the unconditional per-session cost), and demands no shrink that would drag unrelated sections into scope. A sweep that adds files and leaves the document no larger is the intended outcome; a shrink from applying the load-bearing rule is better.

**The budget excludes the standard's own section.** The new standard text is the deliverable and is not funded by shrinking anything: a gate that forbids the deliverable is incoherent, and the only other way to pay for it is broadening the restructure past the narrow scope just chosen — the scope creep the narrow reading exists to prevent. The standard section is nonetheless **written tight**; it is read every session like everything else in the file.

#### 8.5 What the standard section must say

The exact prose is not fixed here. Its **required content** is:

1. The cohesion rule — one concern per file within a package (§1).
2. The name-exclusion test as the rule's operative form, stated in full (§1.1).
3. The 1,000-line tripwire, **and what it is not** — a procedural attention device, never a quality metric or a bar to clear (§1.2).
4. The `// portal:oversized {date} @ {commit} — {claim}` marker: its form, its placement in the leading comment block, what the claim must state, and that the commit is the one **verified against** (§2).
5. The behaviour-area naming rule for test files (§1.3).
6. The source guard that enforces the tripwire, and that the marker is its own allow-list (§3).

**Optional, and explicitly not part of the rule:** a line of guidance that a repetitive-by-construction file (a table-driven test, a fixture table) will justify itself easily. The rule names no categories (§1.2), so nothing in it tells a contributor that — someone may argue harder for such a file than needed. If that reassurance is wanted it belongs here as guidance, **never as a category in the rule**.

---

## Working Notes
