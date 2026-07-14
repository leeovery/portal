TASK: restore-host-terminal-windows-1-2 — Task 1.2: Process-tree walk to bundle id (internal/spawn/walk.go)

ACCEPTANCE CRITERIA:
- A multi-hop chain picker(100->200) -> zsh(200->300) -> ghostty(300->1, /Applications/Ghostty.app/Contents/MacOS/ghostty) resolves to Identity{BundleID:"com.mitchellh.ghostty", Name:"Ghostty"}.
- An ancestry reaching mosh-server/ppid 1 with no .app returns Identity{} (NULL) and nil error.
- A ps failure mid-walk returns a NULL-ish Identity and an error satisfying errors.Is(err, ErrDetectTransient) — distinct from clean-NULL nil-error.
- A defaults read failure on a found .app returns ErrDetectTransient, not clean NULL.
- A cyclic/over-long ancestry terminates via the hop bound and returns clean NULL (no hang).

STATUS: Complete

SPEC CONTEXT:
Spec "Terminal Identity & Detection → Detection model / Identity resolution": the walk resolves client_pid -> process-tree -> .app bundle via a defaults-read Info.plist read (lsappinfo-free), cleanly separating local Ghostty (-> /Applications/Ghostty.app/.../ghostty -> bundle id) from remote mosh (-> mosh-server at ppid 1 -> NULL). Spec "Detection lifecycle → Error vs clean NULL": a clean NULL and a transient ps/defaults failure both resolve to the unsupported no-op path, but the transient error must be programmatically distinguishable (Task 1.5 emits a WARN for it, not for clean NULL). Spec "Testing Strategy → Detection behind small seams": resolution is unit-testable behind small (1-3 method) interfaces with fabricated data; the real ps/defaults route is manual/integration only.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/spawn/walk.go
  - ProcessWalker seam (walk.go:18-20), BundleReader seam (walk.go:26-28) — both 1-method, Portal DI style.
  - ErrDetectTransient sentinel (walk.go:37); transient() double-%w wrap preserving the underlying ps/defaults cause (walk.go:98-100).
  - walkToBundle three-shape contract (walk.go:60-93): resolved Identity / clean NULL (Identity{}, nil) / NULL + ErrDetectTransient-wrapped. .app check precedes ppid check; ppid<=1 and repeated-pid guard both -> clean NULL; maxWalkHops=32 bound -> clean NULL.
  - appBundlePath prefix extraction on the ".app/" marker (walk.go:107-114), space-safe.
  - realProcessWalker over `ps -o ppid=,comm= -p` + parsePSProcessInfo first-whitespace split preserving embedded spaces (walk.go:118-158); realBundleReader over `defaults read <app>/Contents/Info.plist` with required CFBundleIdentifier + best-effort CFBundleName falling back to appBasename (walk.go:163-201). Both use log.CombinedOutputWithContext, the sanctioned exec boundary.
- Notes: Matches the task's Do list and all 5 acceptance criteria exactly. Real ps/defaults boundary is unexecuted by automated tests, as the task specifies. `defaults read <path>/Info.plist <key>` is the task-dictated verbatim form (manual/integration-validated by the author).

TESTS:
- Status: Adequate
- Coverage: internal/spawn/walk_test.go covers every acceptance criterion and every named test:
  - multi-hop resolve to Ghostty, asserting BundleID, Name, and exactly one reader call to /Applications/Ghostty.app (proves it stops at the .app, reads no other bundle).
  - clean NULL at ppid 1 (/usr/bin/login) with zero reader calls; clean NULL for mosh-server ancestry.
  - ps failure -> errors.Is(ErrDetectTransient) AND errors.Is(underlying psFailure) AND IsNull — pins the distinctness and the wrapped-cause reachability.
  - defaults read failure on a found .app -> same transient assertions with the underlying readFailure preserved.
  - runaway ancestry via monotonicWalker -> exactly maxWalkHops calls (hop-bound stop) and clean NULL; plus an extra self-referential 2-cycle test proving the repeated-pid guard fires well before the bound.
  - Direct unit tests of the helpers otherwise only reachable via the un-automated real boundary: TestAppBundlePath (incl. embedded spaces + non-bundle paths), TestParsePSProcessInfo (right-justified ppid, embedded spaces, empty, non-numeric), TestAppBasename (incl. spaces). These harden the parse logic that the manual ps/defaults route leaves untested — appropriate, not over-testing.
- Notes: The two clean-NULL cases (ppid-1 login vs mosh-server) exercise the same code path with different fixture data, but both are explicitly enumerated in the plan's Tests list, so the slight overlap is deliberate. Tests assert behaviour (return contract, error chain, call counts), not implementation internals. A test would fail if the feature broke.

CODE QUALITY:
- Project conventions: Followed. Small 1-method DI seams, real impls in the same file with `var _ ProcessWalker = ...` assertions, sentinel error wrapped for errors.Is, log.CombinedOutputWithContext at the exec boundary, unit-lane placement (no daemon/binary, no build tag needed).
- SOLID principles: Good. ISP (1-method seams), DIP (walkToBundle depends on interfaces), SRP (parse/appBundlePath/appBasename/transient each isolated).
- Complexity: Low. Bounded loop, linear control flow, named consts (maxWalkHops, appBundleSuffix).
- Modern idioms: Yes — `for range maxWalkHops` (Go 1.22 int range), multi-%w wrap, errors.Is-based sentinel.
- Readability: Good. Doc comments state the three-shape contract, the NULL-vs-transient rationale, and the ps full-path/embedded-space caveat explicitly.
- Security: No injection surface — exec.Command args are passed argv-style (no shell); pid via strconv.Itoa.
- Issues: None material.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [quickfix] internal/spawn/walk_test.go — add a case where a .app is found at an intermediate hop whose ppid > 1 with further ancestors defined above it, asserting the walk returns that bundle immediately and does not ascend. Current coverage has the resolving .app only at ppid==1 (multi-hop test), so the ".app check precedes the ppid check" short-circuit at a non-root hop is inferred, not directly pinned. Low priority.
- [do-now] internal/spawn/walk.go:98 — the transient() helper names its first parameter `context`, shadowing the stdlib package name (not imported here, so harmless, but linters/readers flag it). Rename to `msg` or `what`; zero logic impact.
