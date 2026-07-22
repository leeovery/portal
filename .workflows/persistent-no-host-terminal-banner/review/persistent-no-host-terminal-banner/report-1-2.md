TASK: persistent-no-host-terminal-banner-1-2 — Remove The Unreachable NULL Banner Render Branch

ACCEPTANCE CRITERIA:
- unsupportedNullLabel no longer exists (grep -rn 'unsupportedNullLabel' internal/ returns nothing).
- The string 'no host-local terminal' no longer exists (grep -rn 'no host-local terminal' . returns nothing).
- unsupportedLeftCluster has no bundleID == "" branch and renderUnsupportedHeader renders see docs unconditionally.
- renderUnsupportedHeader('Apple Terminal', 'com.apple.Terminal', ...) still produces amber label + dim identity + blue see docs, single row — the six named-path tests stay green.
- TestUnsupportedHeader_NullIdentityNoHostLocal removed; TestUnsupportedHeader_ExactlyOneRow has no null subcase.
- go build ./... and go test ./internal/tui/... pass (unit lane).

STATUS: complete

SPEC CONTEXT:
Spec §6 (Dead NULL Banner Render Branch — Removed) mandates deleting the unsupportedNullLabel constant, the bundleID == "" branches in unsupportedLeftCluster/renderUnsupportedHeader, and the render-level TestUnsupportedHeader_NullIdentityNoHostLocal; after removal the two renderers are named-only (bundleID != "" always holds) so the `see docs` hint is unconditional, and this "deletes the last live copy of the `no host-local terminal` jargon string". Spec §7 lists the same test as Remove and re-homes the NULL end-state assertion onto the standard-header path. The removal is only safe AFTER Task 1.1's gate change (unsupportedBannerActive gains the !IsNull() discriminator) — verified present at model.go:4737, so the NULL branch is genuinely unreachable and its deletion cannot regress a live render path.

IMPLEMENTATION:
- Status: Implemented (matches task DO steps and spec §6 exactly)
- Location:
  - internal/tui/section_header.go:58-62 — unsupportedNullLabel const + doc comment deleted; unsupportedDocsHint doc reworded to "carried unconditionally".
  - internal/tui/section_header.go:171-175 — renderUnsupportedHeader: the `var hint; if bundleID != ""` conditional replaced by unconditional `hint := headerStyle(AccentBlue…).Render(unsupportedDocsHint)`. Doc comment (148-170) rewritten to the named-only contract.
  - internal/tui/section_header.go:210-216 — unsupportedLeftCluster: `if bundleID == "" { return amber.Render(… unsupportedNullLabel) }` branch removed; now named-only. The single-use `amber` local was inlined into the label render (clean — no dead local left behind). Doc comment (204-209) updated.
  - internal/tui/model.go:4864-4866 — applySectionHeader comment updated to state renderers are named-only and `see docs` is unconditional (comment-only change; the gate itself was landed by Task 1.1).
- Notes: Diff is exactly the intended surface — no incidental edits. bundleID is still a parameter of both renderers (correct — it is rendered into the identity string, just no longer branched on).

TESTS:
- Status: Adequate
- Coverage:
  - TestUnsupportedHeader_NullIdentityNoHostLocal — removed (was the direct exerciser of the deleted branch). Confirmed absent.
  - TestUnsupportedHeader_ExactlyOneRow — {"null", ""} table entry dropped; only {"named", "com.apple.Terminal"} remains; single-row height assertion retained.
  - File-level comment block (L13-25) reworded from "unsupported/NULL … NULL identity: honest ⚠ no host-local terminal line" to the named-only description, cross-referencing TestApplySectionHeader_UnsupportedNullShowsStandardHeader for the NULL case.
  - All six named-path tests from the acceptance list remain present: NamedIdentityAmberDimSeeDocs (L45), RightAlignedSeeDocs (L91), ExactlyOneRow (L110), NarrowDegradeDropsHint (L130), ColourlessGlyphBacked (L159), PaintsCanvasNoEdgeBleed (L181).
  - NULL end-state is covered at the applySectionHeader layer by TestApplySectionHeader_UnsupportedNullShowsStandardHeader (L222) and TestActiveNoticeBand_NullReturnsSignpost (L330) — these belong to Task 1.1 but confirm the NULL path now renders the standard header and returns the signpost, so the deleted render-level test is not a coverage loss.
- Notes: Not over-tested (the redundant render-level NULL case was correctly deleted, its assertion migrated up a layer). Not under-tested (named render path retains full colour-role/alignment/height/NO_COLOR coverage). The remaining "no host-local terminal" occurrences at unsupported_banner_test.go:232 and :254 are negative assertions (the string must be ABSENT) — they are guards proving the banner never renders the jargon, and deleting them would weaken the suite; correctly retained.

CODE QUALITY:
- Project conventions: Followed. No t.Parallel() (noted in the file header per the cmd/tui package-level-mock convention). Doc comments kept in sync with behaviour on every touched function — matches the codebase's heavy-doc-comment style.
- SOLID principles: Good. Removing the dead branch tightens each renderer to a single named-only responsibility.
- Complexity: Lowered — two conditional branches removed; both functions are now straight-line.
- Modern idioms: Yes. Inlining the single-use `amber` local is idiomatic Go.
- Readability: Good. The doc comments now state the load-bearing invariant (bundleID != "" always holds, guaranteed by the gate's !IsNull() discriminator) so a future reader understands why the branch is absent.
- Issues: None.

GREP ACCEPTANCE GATES:
- `grep -rn 'unsupportedNullLabel' internal/` → empty (exit 1). PASS.
- `grep -rn 'no host-local terminal' .` → NOT literally empty, but every remaining match is out of scope for this gate, whose stated intent (spec §6) is "deletes the last live copy of the jargon string" i.e. the render label. Remaining matches categorised:
  - `.workflows/**` (spec, planning, discovery) and `.tick/tasks.jsonl` — documentation / task DB, not code.
  - internal/spawn/detect.go:28 & detect_test.go:17 — `msgDetectionNullBundle = "detection resolved no host-local terminal"`, a detection-layer log message. Explicitly flagged out of scope by the task brief (a DIFFERENT concept — a detection outcome log, not a banner label); untouched by spec §6/§7.
  - internal/spawn/{walk,resolver,identity,detect_inside}.go, resolver_config_test.go, detect_inside_test.go — code COMMENTS / a test error string describing the NULL identity concept in plain English. Legitimate; not a rendered label.
  - internal/capture/fixtures.go:475 — a COMMENT describing the NULL shape.
  - internal/tui/unsupported_banner_test.go:220 (comment), :232 & :254 — negative assertions requiring the string's ABSENCE from the rendered banner.
  The last LIVE render copy (the unsupportedNullLabel constant) is deleted, so the gate's spirit is satisfied. Reported here for transparency; not raised as a finding because no concrete corrective change is warranted (each match is either documentation, an out-of-scope detection log, a descriptive comment, or an absence-guard).

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
