AGENT: architecture
FINDINGS:

- FINDING: previewModel uses mixed value/pointer receivers around viewport mutation
  SEVERITY: medium
  FILES: internal/tui/pagepreview.go:193, internal/tui/pagepreview.go:241, internal/tui/pagepreview.go:69, internal/tui/model.go:919
  DESCRIPTION: NewPreviewModel is a value-returning constructor that internally calls `m.readFocusedPaneIntoViewport()` (pointer receiver) on its local value before returning by value. Update is a value receiver that mutates `m.viewport` via the pointer-receiver helper inside cycle branches and returns m by value. It works today because `viewport.Model`'s content survives the value copy, but most lifecycle methods (Update, View, currentGroup, currentPaneKey, chromeLine, degenerate) are value receivers while readFocusedPaneIntoViewport is the lone pointer receiver. Bubble Tea idiom is "all value receivers, return updated value" or "all pointer receivers" — mixing them creates a class of latent bugs where a future field that doesn't survive value-copy (mutex, channel, atomic) would silently break the helper's mutations.
  RECOMMENDATION: Pick one receiver discipline uniformly. Smallest change: make readFocusedPaneIntoViewport return the updated viewport.Model (or the updated previewModel) so the caller assigns it onto m.viewport / m, keeping the surrounding value-receiver style consistent.

- FINDING: previewSessionsRefreshedMsg.PreserveName routes implicitly via previewModel.session before it is zeroed
  SEVERITY: low
  FILES: internal/tui/model.go:882, internal/tui/pagepreview.go:213, internal/tui/model.go:659
  DESCRIPTION: The dismiss handler reads `m.preview.session`, then immediately zeroes m.preview, then dispatches refreshSessionsAfterPreviewCmd(preserveName). The cmd/msg flow is consistent with SessionsMsg/ProjectsLoadedMsg patterns. The only fragility is the read-then-zero ordering: a refactor that flips those two lines would silently send empty PreserveName.
  RECOMMENDATION: Keep current shape; add a one-line comment at the read site noting that preserveName must be captured before m.preview is zeroed.

- FINDING: WithEnumerator / WithScrollbackReader are exported only for cmd/open.go wiring
  SEVERITY: low
  FILES: internal/tui/model.go:458, internal/tui/model.go:469, cmd/open.go:321-324
  DESCRIPTION: The two functional options have a single production caller and zero test callers. They exist for symmetry with WithKiller / WithRenamer / WithProjectStore. Defensible, but they add exported API that no test exercises through the option path; their only purpose is to bridge buildTUIModel to two unexported Model fields.
  RECOMMENDATION: Acceptable as-is for symmetry.

- FINDING: previewModel.session is identity-bearing but no constructor invariant forbids the zero value
  SEVERITY: low
  FILES: internal/tui/pagepreview.go:46, internal/tui/pagepreview.go:124, internal/tui/model.go:884
  DESCRIPTION: The session field is input to currentPaneKey/SanitizePaneKey and the source of preserveName on dismiss. previewModel{} (zero value) is used as a "between opens" sentinel. Calling methods on the zero value would produce SanitizePaneKey("", 0, 0) — silent failure rather than panic. Today benign because methods are only called between Space and Esc, but the type carries no compile-time or runtime guard.
  RECOMMENDATION: Document on previewModel that the zero value is reserved for "between opens, methods must not be called".

- FINDING: openFileForTail / SetOpenFileForTest seam is correctly minimal
  SEVERITY: low
  FILES: internal/state/scrollback_tail.go:20-31
  DESCRIPTION: Confirmation finding. Unexported package var swapped via an exported test helper that returns a restore func. Used only by three test cases to verify the single-fd invariant. Zero production callers redirect file opens. Exported symbol's name signals intent.
  RECOMMENDATION: Keep as-is.

STATUS: findings
