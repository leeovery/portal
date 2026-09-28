## Attempt 1

ISSUES:
- `cmd/capturetool/main.go:196`: `deps.NoColor = noColourRequested()` overwrites the fixture's `noColor` unconditionally. Before this task no registered fixture set `noColor`, so this was dormant. `sessions-pending-resume-colourless` is the first fixture that sets it, and it now hits this line.
  - As a result, `go run ./cmd/capturetool --fixture sessions-pending-resume-colourless` with no `NO_COLOR` in the environment renders coloured `●` / `●●` and never `A`/`P`/`AP`.
  - I confirmed this with a `go test -overlay` probe in the scratchpad (no repo file touched). `resolveModel("sessions-pending-resume-colourless", …)` followed by a `SessionsMsg` rendered `3 windows  ●●` and no `AP`, while `fx.Colourless()` reported true.
  - The task's Outcome ties the sibling to this route: "`go run ./cmd/capturetool --fixture sessions-pending-resume` renders …, its colourless sibling shows `A`, `P` and `AP`". The Problem statement says capturetool is the only way to see the row before release.
  - The effect at the phase's visual gate: a user who opens the colourless fixture by name sees the coloured frame. They either judge the colourless geometry broken or sign it off without having seen it. Setting `NO_COLOR` fixes the view, but then the coloured fixture renders colourless too, so the sibling adds nothing on the live route.
  - `TestNoColorIsReadInOnePlace`'s own comment names this failure: it is "how a surface and a fixture come to disagree about NO_COLOR".
  FIX: Change line 196 to `deps.NoColor = deps.NoColor || noColourRequested()`, so the environment can turn colour off but cannot turn a colourless fixture's colour back on. I verified with an overlay that this passes the whole `cmd/capturetool` suite, including `TestNoColorIsReadInOnePlace` (still one `"NO_COLOR"` literal) and `TestResolveModel_NoColorWinsOverTheme`, and that the probe then renders `AP` and no `●`.
  Add a test to `cmd/capturetool/main_test.go` beside `TestResolveModel_NoColorWinsOverTheme`, unsetting `NO_COLOR` the same way its second subtest does:
  - `resolveModel("sessions-pending-resume-colourless", pinned)` gives `View().BackgroundColor == nil` and no `48;2;` in the content.
  - `resolveModel("sessions-pending-resume", pinned)` still paints `pinned.Canvas.Color()`.
  CONFIDENCE: high

COMMENT_CORRECTIONS:
- /Users/leeovery/Code/portal/internal/capture/fixtures.go:128-129 — `fakePendingReader` is a value type, so a typed nil cannot happen here. The alternative this nil avoids is an empty reader value, which Build would wire all the same.
  OLD: // An untyped nil when the fixture declares no pending sessions: a typed nil
// would pass Build's nil check and wire a reader into every other fixture.
  NEW: // Nil rather than an empty reader: Build wires any non-nil one.

NOTES:
- The executor's change to `ModelAt`, beyond the Do list, is justified. `ModelAt` builds its own `SessionsMsg`, so without the pending set neither the swap guard nor the new tests would ever see a pending dot. For every fixture that declares no pending sessions the reader is nil, so the message is byte-identical to before.
- `pendingSessionsOf` (`internal/capture/harness.go:57-68`) restates `tui`'s `readPendingSessions` rule (`internal/tui/pending_resume.go:26-35`). This follows the harness's existing pattern of restating the session read rather than running the model's own fetch. The rule is small and the fake never errors, so I did not raise it.
- "Every existing fixture is unchanged" is covered structurally: `TestFixtures_WithoutPendingSessionsWireNoReader` checks for a nil reader rather than diffing frames. That is sufficient because a nil reader produces exactly the pre-change `SessionsMsg`, and the captured-state and swap guards pass across the whole registry.
- Verified by running: `go test -count=1` on `./internal/capture/` and `./cmd/capturetool/` passes, including `TestPortalBinaryDoesNotImportCapture`, `TestThemeSwapGuard_EnumeratesRegistry`, `TestThemeSwapGuard_ExcludesColourlessFixtures` and `TestFixtureColourless_ReadsDepsNoColor`.
