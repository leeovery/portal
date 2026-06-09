---
status: complete
created: 2026-06-09
cycle: 2
phase: Gap Analysis
topic: status-recent-warnings-pipe-format
---

# Review Tracking: status-recent-warnings-pipe-format - Gap Analysis

## Findings

### 1. Producer-coupled test seam is unspecified — how the test acquires real-writer output is left to the implementer

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: "Acceptance Criteria & Testing" → "Testing requirements" → "At least one producer-coupled end-to-end regression test" / "Where the producer-coupled assertion lives, and the cmd-layer tests"

**Details**:
The spec names the producer-coupled regression test as "the core requirement" and the guard that "would have caught the original mismatch." It mandates that the test "drive the **real `internal/log` writer** to emit a `WARN` line into the status directory's `portal.log`" and that the migrated cmd-layer tests likewise "source [their] log line from the real `internal/log` writer rather than a hand-authored string." But it never specifies *how* a test in `internal/state` (and `cmd`) is supposed to obtain real-writer output. Inspecting the writer package shows the available seams force a non-trivial, undocumented design decision:

- `newTextHandler` / `textHandler` are **unexported**, so an external test package (`state_test`, `cmd`) cannot construct the real handler directly to render a line.
- The only public entry point that produces real text-format output is `log.Init(stateDir, version, processRole)`, which (a) mutates the **process-wide** handler via the package-global `setHandler` indirection, with no spec-mentioned restore, and (b) writes through the full rotating sink — creating `portal.log.<date>` plus the `portal.log` **symlink** and triggering the first-of-day retention/`process: start` machinery — rather than a plain `portal.log` file. `scanRecentWarnings` opens `PortalLog(dir)` (the symlink), which resolves transparently, but the test author must know this and must isolate the dir and restore global handler state (e.g. via `SetTestHandler` capture or a fresh `Init`) so the swap does not leak across tests.
- `SetTestHandler` restores the prior handler but only accepts an externally-constructed `slog.Handler` — which, for the *real* text format, is precisely the thing an external package cannot build.

Without a stated approach, two implementers could pick materially different mechanisms (drive `log.Init` with all its global/rotation side effects vs. request a new exported render helper from `internal/log`), and the natural choice carries handler-mutation and symlink-resolution mechanics the spec is silent on. This is a planning-readiness gap: the spec's central test cannot be turned into a task without the implementer first making this design call.

**Proposed Addition**:
Add a "Producer-coupled test seam" item to the Testing requirements:

**Producer-coupled test seam.** The real text handler (`textHandler`) is unexported, and `log.Init` mutates the process-wide handler and writes through the rotating sink + `portal.log` symlink — so the producer-coupled test needs an explicit seam to obtain real-writer output. Provide one in `internal/log` that renders a single record to its canonical `portal.log` text line via the **same rendering path `textHandler.Handle` uses in production** (e.g. factor the line-building out of `Handle` into an exported render function, or a test-only render helper — `*testing.T`-first, like `SetTestHandler` — that drives the real handler), never a re-implementation of the format. The test writes the rendered WARN line(s) into the status directory's `portal.log` (a plain file matching `PortalLog(dir)`; no rotation/symlink needed) and runs `CollectStatus`. This keeps fixtures byte-identical to production output — any change to the writer's line format breaks the test (the anti-false-green guarantee) — without leaking process-global handler state. Driving `log.Init` against an isolated dir is an acceptable alternative provided it isolates the dir and restores the handler on cleanup, but the render-path seam is preferred for being side-effect-free.

**Resolution**: Approved
**Notes**: Resolved toward the render-path seam (preferred) with log.Init-against-isolated-dir as an acceptable alternative. Exact shape left to the implementer; the binding constraint is that fixtures derive from the production render path, not a re-implementation.

---

### 2. Empty-message line behaviour is unspecified (benign, but undefined)

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: "Solution Design" → §1 "Shared parse helper" (Parsing rules → Message) and §2 reader "`LastWarning` composition"

**Details**:
The writer can emit a record whose `r.Message` is the empty string (nothing in `internal/log` forbids it). For such a record the real line is `<ts> WARN daemon:  pid=… version=… process_role=…` — i.e. the component colon-space immediately followed by the first attr token, with an empty message between them. The spec's Message rule ("text after `<component>: `, up to … the first `key=value` token") and the `LastWarning` composition rule (`"<LEVEL> <component>: <msg>"`) both still resolve deterministically — `Message == ""`, yielding `LastWarning == "WARN daemon: "` (with a trailing space) — but the spec never states this case or whether the trailing space / empty summary is acceptable. The effect is benign (it only affects the displayed `LastWarning` text for an empty-message warning; the count and health signal are unaffected), so this is a clarity/completeness gap rather than a correctness defect. It is adjacent to the spec's existing "no stray space before the colon" precision for the empty-component case, where the same level of explicitness would be natural.

**Proposed Addition**:
Add a note to §1 (Message rule) / §2 (LastWarning composition):

**Empty message.** A record with an empty `Message` parses with `ok == true` and `Message == ""`. `LastWarning` then renders as `"<LEVEL> <component>:"` (non-empty component) or `"<LEVEL>:"` (empty component) — no trailing space. `RecentWarnings` and the health signal are unaffected. (In practice no closed-catalog message is empty; this is specified only to make the rendering deterministic, consistent with the empty-component "no stray space" rule.)

**Resolution**: Approved
**Notes**: Specified the deterministic rendering (trailing space trimmed) rather than declaring out of scope, mirroring the existing empty-component precision.

---
