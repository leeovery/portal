TASK: restore-host-terminal-windows-1-3 — Outside-tmux detection: env fast-path + walk fallback (tick-0f48c7)

ACCEPTANCE CRITERIA:
1. __CFBundleIdentifier=com.apple.Terminal (no other setup) ⇒ Identity{com.apple.Terminal, Terminal}, walk seam never called.
2. GHOSTTY_RESOURCES_DIR set with __CFBundleIdentifier absent ⇒ Identity{com.mitchellh.ghostty, Ghostty}, walk seam never called.
3. Both env vars absent ⇒ delegates to walkToBundle(selfPID, …) and returns exactly its result (resolved / NULL / transient error).
4. __CFBundleIdentifier set to "" or a malformed value (e.g. "  ") ⇒ falls back to the walk (does not return a bogus identity).

STATUS: Complete

SPEC CONTEXT:
Specification §"Detection model" item 1 (line 250): "Outside tmux (primary flow — fresh terminal → picker): the picker self-walks its own process tree to the terminal (picker → zsh → ghostty), or uses the env fast-path (GHOSTTY_* / __CFBundleIdentifier, accurate outside tmux). Direct — no client list, no tiebreak." The plan pins the two under-specified details: empty/whitespace __CFBundleIdentifier is the clear fallback trigger, and a value with no "." or with internal whitespace is treated as malformed. Bundle-id-first precedence, then GHOSTTY_*, then the walk. Ghostty's bundle id (com.mitchellh.ghostty) is detection-layer knowledge; adapter mapping is Phase 2.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/spawn/detect_outside.go:38 (detectOutsideTmux), :11 (ghosttyBundleID const), :18 (ghosttyEnvKeys finite set), :55 (plausibleBundleID), :67 (ghosttyEnvPresent).
- Notes: Signature matches the plan exactly: detectOutsideTmux(getenv func(string) string, selfPID int, walker ProcessWalker, reader BundleReader) (Identity, error). Precedence is correct and in order — plausible __CFBundleIdentifier → NewIdentity(value, "") direct (no walk); else GHOSTTY_* present → NewIdentity(ghosttyBundleID, "Ghostty") direct (no walk); else walkToBundle propagated verbatim. Plausibility check = contains "." AND no internal whitespace, matching the plan's malformed rule (malformed ⇔ no dot OR internal whitespace). GHOSTTY_* checked against an explicit named finite set (GHOSTTY_RESOURCES_DIR, GHOSTTY_BIN_DIR) rather than a full-environ scan, as required. getenv is an injectable seam. NewIdentity for "com.apple.Terminal" derives Name "Terminal" (identity.go:60 deriveName), satisfying AC1's Name field. ghosttyEnvPresent requires a non-empty post-trim value (a stricter-but-defensible reading of "present"; graceful — an empty GHOSTTY var falls through to the walk, which still resolves). No drift from the plan.

TESTS:
- Status: Adequate
- Location: internal/spawn/detect_outside_test.go
- Coverage: All five planned test cases present and mapped to acceptance criteria:
  - "it resolves directly from __CFBundleIdentifier without walking" (AC1) — asserts BundleID+Name, guarded by failWalker/failReader that t.Fatalf on any invocation (proves no walk).
  - "it resolves to Ghostty from a GHOSTTY_* var when __CFBundleIdentifier is absent" (AC2) — subtests over BOTH keys in the finite set, failWalker/failReader guard proves no walk.
  - "it falls back to the walk when both env vars are absent" (AC3) — asserts resolved identity AND that the walk started at selfPID 777 (walker.calls[0]).
  - "it falls back to the walk for an empty or malformed __CFBundleIdentifier" (AC4) — table of 4: empty, whitespace-only, no-dot, internal-whitespace; each asserts the result came from the walk and the walk was invoked.
  - "it propagates the walk's clean NULL and transient error unchanged" (AC3) — two subtests cover the NULL and ErrDetectTransient shapes, incl. errors.Is against both the sentinel and the underlying ps failure.
  The walk-not-called guard (the plan's explicit requirement) is implemented well via fail seams. Tests reuse the Task 1.2 fakes (fakeWalker/fakeReader/fakeProc/fakeBundle) — no duplication.
- Notes: Not over-tested; each subtest exercises a distinct branch. One behavioural gap (non-blocking): the precedence between a malformed __CFBundleIdentifier AND a present GHOSTTY_* var (malformed CFBundle → GHOSTTY fast-path wins, not the walk) is implemented but untested — AC4 only covers malformed + no GHOSTTY → walk.

CODE QUALITY:
- Project conventions: Followed — 1-method DI seams reused from Task 1.2, injectable getenv seam (no t.Setenv), unit-lane test, package-doc-quality comments, finite named signal set kept auditable per the plan.
- SOLID principles: Good — single-responsibility helpers (plausibleBundleID, ghosttyEnvPresent) extracted cleanly; detectOutsideTmux reads as a linear precedence.
- Complexity: Low — three-branch straight-line function, two trivial predicates.
- Modern idioms: Yes — strings/unicode stdlib usage is idiomatic; IndexFunc(unicode.IsSpace) is the right internal-whitespace check.
- Readability: Good — intent is self-documenting; the precedence doc comment on detectOutsideTmux is accurate.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [quickfix] internal/spawn/detect_outside_test.go:96 — Add a subtest to lock the precedence: a malformed/empty __CFBundleIdentifier WITH a GHOSTTY_* var present must resolve via the Ghostty fast-path (no walk), not fall through to the walk. This behaviour is implemented (step 1's plausibility failure falls to step 2, not step 3) but no test pins it, so a future reordering could regress it silently.
- [idea] internal/spawn/detect_outside.go:18 — The GHOSTTY_* signal set is intentionally narrow (two keys). The plan asks to "flag any real-world GHOSTTY_* key discovery for the build-time residual walk-confirmation"; decide during the build-time residual whether additional stable Ghostty keys warrant inclusion. Degrades gracefully (a miss falls to the walk, which still resolves), so this is a robustness enhancement, not a defect.
