package tui

import "fmt"

// Task spectrum-tui-design-5-4 — step mapping (11 real bootstrap steps → 5
// friendly labels). §10.4.
//
// The honest loading screen (§10.3) must show progress the user can read, but
// the bootstrap has 11 internal steps with cryptic names (EnsureServer,
// RegisterPortalHooks, SweepOrphanDaemons, …). §10.4 collapses the 11 real steps
// into 5 friendly labels, advancing the progress bar on EVERY real step while
// the active label is the friendly group the current step falls in.
//
// This file is the SINGLE SOURCE OF TRUTH for that contract: the §10.4 table is
// encoded ONCE (stepLabelTable, keyed by the 1-based canonical step index), and
// the bar-advance, the active-label selection, and the done/active/pending
// tick-off all derive from it — no duplicated step lists. It is a pure,
// independently unit-testable accumulator: it has ZERO dependency on the channel
// transport (task 5-2) or on any render code (task 5-5 renders this view).
//
// It deliberately does NOT import cmd/bootstrap (wrong import direction / cycle
// risk — internal/tui must not depend on cmd). The mapping keys off the
// BootstrapProgressMsg.Index (1..11), the stable canonical step number, so the
// closed step* StepName strings live only on the producer side.

// The five §10.4 friendly labels. Mirrored as exported constants so task 5-5's
// render and the tests share one source — never re-typed at call sites.
const (
	LabelStartedTmuxServer     = "Started tmux server"
	LabelRegisteredHooks       = "Registered hooks"
	LabelRestoringSessions     = "Restoring sessions"
	LabelReplayingScrollback   = "Replaying scrollback"
	LabelRunningResumeCommands = "Running resume commands"
)

// totalBootstrapSteps is the count of real bootstrap steps. The bar advances by
// 1/totalBootstrapSteps per distinct completed step index, so it reaches 100%
// only after the last real step — eleven increments, NOT five (one per friendly
// label). Keep this in lockstep with stepLabelTable (the drift guard test
// TestMappingCoversAllElevenStepsNoGaps asserts the table covers exactly 1..11).
const totalBootstrapSteps = 11

// LabelState is the §10.3 tick state of a friendly label: done (✓), active (◐),
// or pending (·). Render glyphs/colours are task 5-5's concern.
type LabelState int

const (
	// LabelPending is the zero value: the label's steps have not started.
	LabelPending LabelState = iota
	// LabelActive: the current step falls in this label's group (its last
	// constituent step has not yet completed).
	LabelActive
	// LabelDone: every constituent step of this label has completed.
	LabelDone
	// LabelFailed: a fatal bootstrap step in this label's group aborted the
	// boot (§10.5). It renders the state.red ✗ marker. Only ever set on the
	// error-frame view (FailedView), never reached by the normal accumulator
	// projection (View).
	LabelFailed
)

// labelOrder is the stable top-to-bottom render order of the five labels.
var labelOrder = []string{
	LabelStartedTmuxServer,
	LabelRegisteredHooks,
	LabelRestoringSessions,
	LabelReplayingScrollback,
	LabelRunningResumeCommands,
}

// stepLabelTable is the §10.4 mapping encoded ONCE: step index (1..11) → friendly
// label. This is the single source of truth — the bar advance, active-label
// selection, and tick-off all derive from it.
//
// Step 6 (Restore) is dual-mapped at runtime, NOT in this table: its
// per-session skeleton events (RestoreM > 0) belong to "Restoring sessions",
// while its completion tick (RestoreM == 0, geometry + scrollback replay)
// belongs to "Replaying scrollback". The table entry below is the completion
// mapping; resolveLabel discriminates on RestoreM for step 6 (see restoreStep).
//
// §10.4 explicitly permits the implementation to adjust WHICH fast cleanup step
// (8–11) sits under WHICH label — the cleanup steps are near-instant and fold
// under the final label — but the bar MUST advance through every real step.
var stepLabelTable = map[int]string{
	1:  LabelStartedTmuxServer,     // EnsureServer
	2:  LabelRegisteredHooks,       // RegisterPortalHooks
	3:  LabelRegisteredHooks,       // SetRestoring (@portal-restoring)
	4:  LabelRegisteredHooks,       // SweepOrphanDaemons
	5:  LabelRegisteredHooks,       // EnsureSaver
	6:  LabelReplayingScrollback,   // Restore — completion (RestoreM==0); skeleton (RestoreM>0) → Restoring sessions
	7:  LabelReplayingScrollback,   // EagerSignalHydrate
	8:  LabelRunningResumeCommands, // ClearRestoring (@portal-restoring) + on-resume hydrate commands fold here
	9:  LabelRunningResumeCommands, // CleanStaleMarkers
	10: LabelRunningResumeCommands, // SweepOrphanFIFOs
	11: LabelRunningResumeCommands, // CleanStale
}

// restoreStep is the index of the Restore step — the only step that dual-maps on
// RestoreM (skeleton per-session vs completion).
const restoreStep = 6

// LabelForStep returns the §10.4 friendly label a single step event maps to —
// the literal "maps each real step to its friendly label" contract, transport-
// free and independently testable. It applies the step-6 RestoreM discriminator:
// a skeleton per-session event (RestoreM > 0) maps to "Restoring sessions";
// every other step (including step 6's completion tick, RestoreM == 0) maps via
// the §10.4 table. An out-of-range index returns "" (no phantom steps). task 5-5
// may use this to label a single event without the accumulator.
func LabelForStep(e BootstrapProgressMsg) string {
	if e.Index == restoreStep && e.RestoreM > 0 {
		return LabelRestoringSessions
	}
	return stepLabelTable[e.Index]
}

// LoadingLabel is one rendered row of the §10.3 tick-list: a friendly label, its
// done/active/pending state, and (for "Restoring sessions" only) an "N/M"
// counter — empty for every other label and for the M=0 degenerate case.
type LoadingLabel struct {
	Text    string
	State   LabelState
	Counter string // "N/M" — only ever populated for LabelRestoringSessions
}

// LoadingProgressView is the render input task 5-5 consumes: the bar fraction
// (0.0..1.0) and the ordered five labels with their states + counter. Pure data
// — no styling, no glyphs.
//
// Message is the §10.5 fatal one-line message: empty on the normal loading view,
// populated by FailedView when a fatal cold-boot step aborts the boot. When set,
// exactly one label carries LabelFailed (the failed step's friendly group) and
// the render layer draws the message line + a quit hint beneath the step-list.
type LoadingProgressView struct {
	BarFraction float64
	Labels      []LoadingLabel
	Message     string
}

// LoadingProgress is the pure accumulator for the §10.4 mapping. Fold each
// BootstrapProgressMsg through Apply (which returns a new value — no mutation of
// the receiver), then call View to produce the render inputs. The zero value is
// ready to use (nothing completed, bar at 0, every label pending).
//
// Idempotency: completion tracks DISTINCT step indices (completedSteps), so a
// duplicate or out-of-order step event never double-advances the bar.
type LoadingProgress struct {
	// completedSteps records which step indices have completed at least once.
	// Tracking the set (not a naive counter) makes the bar advance idempotent
	// per step index.
	completedSteps map[int]bool
	// restoreN / restoreM carry the latest restore per-session counter from the
	// step-6 skeleton events (RestoreM > 0). restoreM == 0 means no per-session
	// events were seen — the M=0 degenerate case (counter suppressed).
	restoreN int
	restoreM int
}

// Apply folds one BootstrapProgressMsg into the accumulator and returns the
// updated value. Out-of-range indices (not in the §10.4 table) are ignored — no
// bar advance, no state change.
//
// Step 6 (Restore) is the one step whose events split on RestoreM. The producer
// (cmd/bootstrap) emits every per-session skeleton event (RestoreM > 0) strictly
// BEFORE the single trailing step-6 completion tick (RestoreM == 0), so a
// RestoreM > 0 event is a reliable "step 6 NOT yet complete" signal — the same
// discriminator LabelForStep uses. Therefore:
//   - A mid-flight skeleton event (RestoreM > 0) updates the (N/M) counter ONLY;
//     it does NOT mark step 6 complete, so "Restoring sessions" stays the active
//     frontier with N/M ticking while restore is genuinely still in flight.
//   - Every other step event — including step 6's trailing completion tick
//     (RestoreM == 0) — marks that step index complete (idempotent set insert).
//     That trailing tick is what finally flips "Restoring sessions" to done and
//     advances the bar through step 6. The counter set on the skeleton path stays
//     sticky into the done state (counterFor still reads the last N/M).
func (p LoadingProgress) Apply(e BootstrapProgressMsg) LoadingProgress {
	if _, mapped := stepLabelTable[e.Index]; !mapped {
		return p
	}

	next := p.clone()
	if e.Index == restoreStep && e.RestoreM > 0 {
		// Mid-flight skeleton event: advance the N/M counter only; step 6 is
		// NOT yet complete.
		next.restoreN = e.RestoreN
		next.restoreM = e.RestoreM
	} else {
		// Every other step, plus step 6's trailing completion tick (RestoreM==0).
		next.completedSteps[e.Index] = true
	}
	return next
}

// clone returns a deep copy of p with an initialised completedSteps set so the
// zero value is usable and Apply never mutates its receiver's map.
func (p LoadingProgress) clone() LoadingProgress {
	steps := make(map[int]bool, len(p.completedSteps)+1)
	for idx := range p.completedSteps {
		steps[idx] = true
	}
	return LoadingProgress{
		completedSteps: steps,
		restoreN:       p.restoreN,
		restoreM:       p.restoreM,
	}
}

// View derives the render inputs from the accumulated state — a pure projection.
// task 5-5 renders this. The bar is (distinct completed steps)/11; each label's
// done/active/pending state follows labelState (done when all its steps
// completed, active when it is the executing frontier, else pending). Only
// "Restoring sessions" carries a counter, and only when at least one skeleton
// event was seen (RestoreM > 0).
func (p LoadingProgress) View() LoadingProgressView {
	v := LoadingProgressView{
		BarFraction: float64(len(p.completedSteps)) / float64(totalBootstrapSteps),
		Labels:      make([]LoadingLabel, 0, len(labelOrder)),
	}
	for _, text := range labelOrder {
		v.Labels = append(v.Labels, LoadingLabel{
			Text:    text,
			State:   p.labelState(text),
			Counter: p.counterFor(text),
		})
	}
	return v
}

// FailedView projects the §10.5 fatal error-frame render input from the
// accumulated state: every step that completed before the fatal stays done (✓),
// the failed step's friendly label (mapped from failedStep via the §10.4 table)
// flips to LabelFailed (✗), and every label after it stays pending (·) — those
// steps never ran. The bar is frozen at the fraction reached at fatal time (the
// completed-step count), NOT advanced past it. message is the
// FatalError.UserMessage rendered beneath the step-list.
//
// failedStep is the 1-based canonical index of the aborting step (1, 2, 3, or 8 —
// the four fatal steps). An out-of-range index leaves no label failed (defensive:
// the render still shows a frozen bar + message), but production always passes a
// valid fatal-step index.
func (p LoadingProgress) FailedView(failedStep int, message string) LoadingProgressView {
	failedLabel := LabelForStepIndex(failedStep)
	v := LoadingProgressView{
		BarFraction: float64(len(p.completedSteps)) / float64(totalBootstrapSteps),
		Labels:      make([]LoadingLabel, 0, len(labelOrder)),
		Message:     message,
	}
	for _, text := range labelOrder {
		state := p.labelState(text)
		if text == failedLabel {
			state = LabelFailed
		}
		v.Labels = append(v.Labels, LoadingLabel{
			Text:    text,
			State:   state,
			Counter: p.counterFor(text),
		})
	}
	return v
}

// LabelForStepIndex maps a 1-based canonical step index to its §10.4 friendly
// label via the static step→label table. It is the index-only sibling of
// LabelForStep (which discriminates the dual-mapped restore step on RestoreM);
// the fatal steps (1, 2, 3, 8) are never the dual-mapped restore step, so the
// static table mapping is exact. An out-of-range index returns "".
func LabelForStepIndex(index int) string {
	return stepLabelTable[index]
}

// labelState computes a label's done/active/pending state. The streamed events
// signal step COMPLETION (the producer emits each StepEvent at its "step
// complete" site), so the §10.4 "active label is the friendly group of the
// current step" resolves to: a label is DONE once every one of its constituent
// real steps has completed; the ACTIVE label is the FIRST not-yet-done label in
// render order (the group whose steps are now executing); all labels after it
// are PENDING. Before any step completes, every label is pending; once all
// eleven steps complete, every label is done (no active frontier remains).
//
// Done deriving from step completion is what makes a multi-step label
// ("Registered hooks", "Running resume commands") stay active until its LAST
// constituent step completes, and what ticks a zero-item label done rather than
// stalled: "Restoring sessions" is done only when step 6 completes, which is the
// trailing RestoreM==0 completion tick — NOT the mid-flight RestoreM>0 skeleton
// events (those advance the N/M counter only and keep it active, see Apply). So
// during a multi-session restore "Restoring sessions" reads active with N/M
// ticking until restore actually finishes. The M=0 case — restore completes
// immediately, its trailing tick already carries RestoreM==0, no per-session
// events at all — marks step 6 done at once, leaving "Restoring sessions"
// reading done, never active and never stalled (the bar still advances through
// step 6). Likewise "Running resume commands" with zero on-resume work ticks
// done once steps 8–11 complete.
func (p LoadingProgress) labelState(text string) LabelState {
	if p.labelDone(text) {
		return LabelDone
	}
	if text == p.activeLabel() {
		return LabelActive
	}
	return LabelPending
}

// activeLabel is the §10.4 frontier: the first not-yet-done label in render
// order (the group whose steps are now executing). It is "" before any step
// completes (nothing started) and "" once every step completes (no frontier
// remains — all labels done).
func (p LoadingProgress) activeLabel() string {
	if len(p.completedSteps) == 0 {
		return ""
	}
	for _, text := range labelOrder {
		if !p.labelDone(text) {
			return text
		}
	}
	return ""
}

// labelDone reports whether every constituent real step of a label has
// completed. "Restoring sessions" has no static stepLabelTable entry (step 6's
// table slot is "Replaying scrollback"); its constituent step is step 6, so it
// is done once step 6 completes — and step 6 completes only on its trailing
// RestoreM==0 tick, never on a mid-flight RestoreM>0 skeleton event (Apply marks
// the step complete only on the non-skeleton path).
func (p LoadingProgress) labelDone(text string) bool {
	if text == LabelRestoringSessions {
		return p.completedSteps[restoreStep]
	}
	for idx := 1; idx <= totalBootstrapSteps; idx++ {
		if stepLabelTable[idx] != text {
			continue
		}
		if !p.completedSteps[idx] {
			return false
		}
	}
	return true
}

// counterFor returns the "N/M" counter for "Restoring sessions" when skeleton
// events were seen (restoreM > 0), and "" for every other label and for the M=0
// degenerate case (no per-session events → counter suppressed, label ticks done
// via labelState because step 6 still completed).
func (p LoadingProgress) counterFor(text string) string {
	if text != LabelRestoringSessions || p.restoreM == 0 {
		return ""
	}
	return fmt.Sprintf("%d/%d", p.restoreN, p.restoreM)
}
