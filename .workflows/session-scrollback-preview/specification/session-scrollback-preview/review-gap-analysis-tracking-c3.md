---
status: complete
created: 2026-05-06
cycle: 3
phase: Gap Analysis
topic: session-scrollback-preview
---

# Review Tracking: session-scrollback-preview - Gap Analysis

## Findings

### 1. ScrollbackReader interface contract for placeholder vs error-string branching

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: § Cross-cutting Seams > State Package API Reuse; § Architecture Summary > Test seams; § Read-Failure Handling

**Details**:
The spec defines the seam interface as `ScrollbackReader.Tail(paneKey) ([]byte, error)` and pins three distinct render outcomes for the caller:

1. **Placeholder ("(no saved content)")** — ENOENT, zero-byte file, or zero-line result (file with only an unterminated partial line).
2. **Error string** — OS-level read errors (EACCES, EIO, permissions).
3. **Normal content** — non-empty tail bytes.

What the spec does **not** pin is how the caller distinguishes outcomes (1) and (2) through the `(bytes, err)` signature. Plausible implementations diverge materially:

- **Sentinel-error contract**: helper returns `nil, fs.ErrNotExist` (or a wrapped equivalent) for ENOENT, `nil, nil` for zero-byte / zero-line, and a non-sentinel error for OS-level failures. Caller branches via `errors.Is`.
- **Nil-bytes-no-error contract**: helper returns `nil, nil` for any "no content available" condition (collapsing ENOENT, zero-byte, zero-line into one shape) and `nil, err` only for OS-level failures. Caller branches on `len(bytes) == 0 && err == nil`.
- **Typed-result contract**: helper returns a richer struct (e.g. `TailResult{Bytes, Status}`) that names the three outcomes explicitly, with `error` reserved for unexpected failures.

These have observable consequences — the interface shape, the mockability story for tests, and the placement of the placeholder/error decision (helper vs caller). § Acceptance Criteria asserts side-effect-free behaviour against the mock but does not pin which branching shape the mock must support.

A planner breaking this into tasks would have to choose one of these contracts before a task for the helper or a task for the preview model can be written; the choice cascades into both `internal/state` (helper return shape) and `internal/tui` (placeholder/error decision site).

**Proposed Addition**:
{leave blank — to be discussed}

**Resolution**: Approved
**Notes**: Pinned the ScrollbackReader return contract under § Architecture Summary > Test seams: `(bytes, nil)` = render content; `(nil, nil)` = placeholder (collapses ENOENT, zero-byte, zero-line); `(nil, err)` = error string. Helper unifies "no content" shapes; placeholder/error decision lives in internal/tui at the call site.

---
