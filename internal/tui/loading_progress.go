package tui

import "fmt"

// Must not import cmd/bootstrap (wrong import direction), so step identity is
// keyed off BootstrapProgressMsg.Index alone.

const (
	LabelCheckingZmx       = "Checking zmx"
	LabelStartingTheSaver  = "Starting the saver"
	LabelRestoringSessions = "Restoring sessions"
	// LabelWaiting leads the labels only once the bootstrap has waited a while
	// for another Portal's to finish.
	LabelWaiting = "Waiting for another Portal"
)

// One increment per distinct completed step, not per label. Keep in lockstep
// with stepLabelTable and cmd/bootstrap's step indices.
const totalBootstrapSteps = 7

type LabelState int

const (
	LabelPending LabelState = iota
	LabelActive
	LabelDone
	LabelFailed
)

var labelOrder = []string{
	LabelCheckingZmx,
	LabelStartingTheSaver,
	LabelRestoringSessions,
}

// The bootstrap's seven steps under their labels: checking zmx; raising the
// restoring flag, the sweep and the saver; then restore, lowering the flag
// and the latch.
var stepLabelTable = map[int]string{
	1: LabelCheckingZmx,
	2: LabelStartingTheSaver,
	3: LabelStartingTheSaver,
	4: LabelStartingTheSaver,
	5: LabelRestoringSessions,
	6: LabelRestoringSessions,
	7: LabelRestoringSessions,
}

// restoreStep's per-session events (RestoreM > 0) count its sessions, n of m,
// rather than completing it.
const restoreStep = 5

// LabelForStep returns "" for an out-of-range index.
func LabelForStep(e BootstrapProgressMsg) string {
	return stepLabelTable[e.Index]
}

// Counter is populated only for LabelRestoringSessions.
type LoadingLabel struct {
	Text    string
	State   LabelState
	Counter string
}

// Message is empty on the normal view; when FailedView populates it, one label
// carries LabelFailed.
type LoadingProgressView struct {
	BarFraction float64
	Labels      []LoadingLabel
	Message     string
}

// LoadingProgress accumulates bootstrap progress: fold each message through
// Apply, then call View. The zero value is ready to use.
type LoadingProgress struct {
	completedSteps map[int]bool
	restoreN       int
	restoreM       int
	waiting        bool
}

// Apply is idempotent per step index, so a duplicate or out-of-order event
// never double-advances the bar. It assumes the producer emits every
// per-session event (RestoreM > 0) before the restore step's completion tick.
func (p LoadingProgress) Apply(e BootstrapProgressMsg) LoadingProgress {
	_, mapped := stepLabelTable[e.Index]
	if !mapped && !e.Waiting {
		return p
	}

	next := p.clone()
	switch {
	case e.Waiting:
		next.waiting = true
	case e.Index == restoreStep && e.RestoreM > 0:
		next.restoreN = e.RestoreN
		next.restoreM = e.RestoreM
	default:
		next.completedSteps[e.Index] = true
	}
	return next
}

func (p LoadingProgress) clone() LoadingProgress {
	steps := make(map[int]bool, len(p.completedSteps)+1)
	for idx := range p.completedSteps {
		steps[idx] = true
	}
	return LoadingProgress{
		completedSteps: steps,
		restoreN:       p.restoreN,
		restoreM:       p.restoreM,
		waiting:        p.waiting,
	}
}

func (p LoadingProgress) View() LoadingProgressView {
	return p.view(0, "")
}

// FailedView leaves no label failed when failedStep is out of range, as a
// fatal before any step leaves it.
func (p LoadingProgress) FailedView(failedStep int, message string) LoadingProgressView {
	return p.view(failedStep, message)
}

func (p LoadingProgress) view(failedStep int, message string) LoadingProgressView {
	failedLabel := LabelForStepIndex(failedStep)
	v := LoadingProgressView{
		BarFraction: float64(len(p.completedSteps)) / float64(totalBootstrapSteps),
		Labels:      make([]LoadingLabel, 0, len(labelOrder)+1),
		Message:     message,
	}
	if p.waiting {
		v.Labels = append(v.Labels, LoadingLabel{Text: LabelWaiting, State: p.waitState()})
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

// LabelForStepIndex returns "" for an out-of-range index.
func LabelForStepIndex(index int) string {
	return stepLabelTable[index]
}

// The waiting label is active until the first step completes: the wait for
// the other Portal is over once this bootstrap runs a step.
func (p LoadingProgress) waitState() LabelState {
	if len(p.completedSteps) == 0 {
		return LabelActive
	}
	return LabelDone
}

// A label is done once every constituent step completed; the active label is the
// first not-yet-done one, so a multi-step label stays active until its last step.
func (p LoadingProgress) labelState(text string) LabelState {
	if p.labelDone(text) {
		return LabelDone
	}
	if text == p.activeLabel() {
		return LabelActive
	}
	return LabelPending
}

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

func (p LoadingProgress) labelDone(text string) bool {
	for idx := 1; idx <= totalBootstrapSteps; idx++ {
		if stepLabelTable[idx] == text && !p.completedSteps[idx] {
			return false
		}
	}
	return true
}

func (p LoadingProgress) counterFor(text string) string {
	if text != LabelRestoringSessions || p.restoreM == 0 {
		return ""
	}
	return fmt.Sprintf("%d/%d", p.restoreN, p.restoreM)
}
