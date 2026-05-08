TASK: session-scrollback-preview-1-6 — Enumeration failure and empty-result handling

ACCEPTANCE CRITERIA:
- When Commander.Run returns an error, method returns (nil, non-nil err); error wraps original via %w.
- When Commander.Run returns ("", nil), method returns ([]WindowGroup{}, nil) — empty but non-nil slice.
- Whitespace-only stdout returns ([]WindowGroup{}, nil).
- Error message includes the session name.
- No CapturePane* method signature is modified by this task or task 1-5.
- Callers can use errors.Is(err, anySentinel) against the returned error.

STATUS: Complete

SPEC CONTEXT:
The spec (Multi-pane Rendering Shape > Chrome Floor > Enumeration failure handling, Refresh Semantics > Read Trigger Events > Initial-open ordering, Source of Preview Bytes) requires that enumeration failure (tmux non-zero exit) returns (nil, err) and empty enumeration returns ([]WindowGroup{}, nil) — both shapes treated identically at the TUI call site but distinct at the tmux.Client layer. The "no new capture wrapper" constraint applies only to capture wrappers; this listing method is permitted.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/tmux/tmux.go:458-511 (ListWindowsAndPanesInSession)
  - Error wrap at tmux.go:464: fmt.Errorf("list windows and panes for session %s: %w", session, err) — uses %w, bare %s session name.
  - Empty/whitespace stdout at tmux.go:466-468: if strings.TrimSpace(out) == "" { return []WindowGroup{}, nil } — explicit non-nil empty slice.
  - Diff is additive: no changes to CapturePane (still at tmux.go:624); only WindowGroup type, listWindowsAndPanesFieldSep const, and the new method.
- Notes: Implementation matches the spec contract precisely.

TESTS:
- Status: Adequate
- Coverage in internal/tmux/tmux_test.go:
  - "it returns an error when tmux exits non-zero" (2484-2495).
  - "it returns an empty slice when stdout is empty and exit is zero" (2497-2511).
  - "it returns an empty slice for whitespace-only stdout" (2513-2527).
  - "the wrapped error includes the session name" (2529-2541).
  - "the wrapped error preserves the original via errors.Is" (2543-2555).
  - "the wrapped error uses the spec-mandated prefix without quoting the session name" (2557-2574).
- Notes: Behaviour-focused, not implementation-coupled. The "no capture wrapper" check is by code review (plan explicitly permitted this).

CODE QUALITY:
- Project conventions: Followed.
- SOLID: Good — single responsibility, DI-friendly via Commander.
- Complexity: Low — single linear pass with map-based grouping.
- Modern idioms: Yes — %w wrapping, strings.SplitN, group-by via map[int]int slice position.
- Readability: Good — clear naming, doc comment explains the unit-separator rationale.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] The exact-string error-message test couples to the precise wording. Worth adding an inline comment in the implementation pointing to the test that locks the wrap shape.
- [idea] The empty/whitespace branch and the inner parse-loop blank-line guard are mildly redundant — both would handle whitespace-only inputs. Early-return is a clean fast-path; not worth changing.
- [quickfix] Doc comment partially duplicates the listWindowsAndPanesFieldSep const comment. Cosmetic.
