package migrate

import (
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strconv"
	"strings"

	"github.com/leeovery/portal/internal/sessionindex"
)

// Restore is what the bootstrap a run ends with made of the saved sessions.
type Restore struct {
	// Ran reports whether a restore ran: a bootstrap finding none owed runs
	// none.
	Ran bool
	// Restored and Failed count the sessions it made and those it couldn't,
	// which stay for the next restore.
	Restored, Failed int
	// Unfinished is a restore owed and not made: a bootstrap ended by a fatal
	// before it, or a restore that couldn't read sessions.json or list zmx's
	// sessions.
	Unfinished bool
}

// WaitingLine is what portal migrate says as a run starts to wait for the
// tmux version's daemon, pid, to exit.
func WaitingLine(pid int) string { return fmt.Sprintf(waitingCopy, pid) }

// Report is what portal migrate prints of the run o and the restore r after
// it: a summary and the restore's result; for a run that migrated, each
// session it made, as name ← tmux name, and what it generated, dropped,
// skipped, couldn't carry over, moved and removed, each with why; then where
// the tmux version's state and hooks.json's backup are kept. A run that found
// nothing to migrate says so alone. shown is how a path is printed.
func Report(o Outcome, r Restore, shown func(path string) string) string {
	var rep report
	switch o.Course {
	case NoState:
		rep.line(noStateCopy, shown(o.StateDir))
		return rep.String()
	case NotTmux:
		rep.line(notTmuxCopy, shown(o.StateDir))
		return rep.String()
	case Migrated:
		rep.summary(o.Plan)
	case Finished:
		rep.line(finishedCopy, shown(o.StateDir))
	case AlreadyMigrated:
		rep.line(alreadyCopy)
	}
	rep.restore(r, waits(o))
	if o.Course == Migrated {
		rep.detail(o.Plan)
	}
	rep.gap()
	rep.line(tmuxDirCopy, shown(o.TmuxDir))
	if o.Backup != "" {
		rep.line(backupCopy, shown(o.Backup))
	}
	return rep.String()
}

// waits reports whether a restore after o makes sessions that wait on their
// resume panel: a run that migrated them, or finished doing so, has had none
// restored before. A run that migrated no hook makes none wait.
func waits(o Outcome) bool {
	switch o.Course {
	case Migrated:
		return len(o.Plan.Moves) > 0
	case Finished:
		return true
	}
	return false
}

// report builds the report a block at a time, a blank line between blocks.
type report struct {
	strings.Builder
	// open is a block begun and not yet ended by a gap.
	open bool
}

// line writes a line, each character in it a terminal wouldn't show as
// itself escaped: a tmux session's name and a hooks.json key, and the errors
// naming them, hold whatever was typed.
func (r *report) line(format string, args ...any) {
	r.open = true
	for _, c := range fmt.Sprintf(format, args...) {
		if sessionindex.Unprintable(c) {
			quoted := strconv.QuoteRuneToASCII(c)
			r.WriteString(quoted[1 : len(quoted)-1])
			continue
		}
		r.WriteRune(c)
	}
	r.WriteByte('\n')
}

// gap ends the block under way, if one is.
func (r *report) gap() {
	if r.open {
		r.WriteString("\n")
		r.open = false
	}
}

// summary is what a run that migrated made of the tmux version's sessions
// and hooks.
func (r *report) summary(plan Plan) {
	migrated := noSessionsCopy
	if len(plan.Sessions) > 0 {
		migrated = fmt.Sprintf(migratedCopy, count(tmuxSessions(plan), "tmux session"), count(len(plan.Sessions), "session"))
	}
	if len(plan.Skipped) > 0 {
		migrated += fmt.Sprintf(skippedCountCopy, count(len(plan.Skipped), "tmux session"))
	}
	r.line("%s", migrated)
	if len(plan.Moves)+len(plan.Removals) == 0 {
		return
	}
	hooks := fmt.Sprintf(hooksMovedCopy, count(len(plan.Moves), "resume hook"))
	if len(plan.Removals) > 0 {
		hooks += fmt.Sprintf(hooksRemovedCopy, len(plan.Removals))
	}
	r.line("%s", hooks)
}

// restore is the restore's result, and whether the sessions it made wait on
// their resume panel.
func (r *report) restore(res Restore, waits bool) {
	switch {
	case res.Unfinished:
		r.line(unfinishedCopy)
		return
	case !res.Ran:
		r.line(noRestoreCopy)
		return
	}
	restored := fmt.Sprintf(restoredCopy, count(res.Restored, "session"))
	if res.Failed > 0 {
		restored += fmt.Sprintf(unrestoredCopy, count(res.Failed, "session"))
	}
	r.line("%s", restored)
	if waits && res.Restored > 0 {
		r.line(waitsCopy)
	}
}

// detail is each session a run made, then what it changed or couldn't carry
// over, a block each, a block left out when it has nothing to say.
func (r *report) detail(plan Plan) {
	panes := panesBySession(plan)
	section(r, sessionsHeading, plan.Sessions, func(s Session) (string, bool) {
		made := fmt.Sprintf(sessionLine, s.Name, s.From.Session)
		if panes[s.From.Session] > 1 {
			made += fmt.Sprintf(paneSuffix, s.From.Window, s.From.Pane)
		}
		return made, true
	})
	section(r, generatedHeading, plan.Sessions, func(s Session) (string, bool) { return why(s.Name, s.Generated) })
	section(r, labelsHeading, plan.Sessions, func(s Session) (string, bool) { return why(s.Name, s.LabelRefused) })
	section(r, skippedHeading, plan.Skipped, func(s SkippedSession) (string, bool) { return why(s.Name, s.Why) })
	section(r, transcriptsHeading, plan.Sessions, func(s Session) (string, bool) { return why(s.Name, missing(s.Transcript)) })
	section(r, movedHeading, inSessionOrder(plan), func(m HookMove) (string, bool) { return fmt.Sprintf(movedLine, m.From, m.To), true })
	section(r, removedHeading, plan.Removals, func(h HookRemoval) (string, bool) {
		return fmt.Sprintf(whyLine, h.Key, kindCopy(h.Kind)), true
	})
}

// missing is why a session's scrollback wasn't carried over, nil when it was
// or there was none.
func missing(t Transcript) error {
	if errors.Is(t.Missing, fs.ErrNotExist) {
		return fmt.Errorf(transcriptGoneCopy, t.Named)
	}
	return t.Missing
}

// inSessionOrder is the plan's hook moves in the order of the sessions they
// move to, so they read beside the sessions' block.
func inSessionOrder(plan Plan) []HookMove {
	order := make(map[string]int, len(plan.Sessions))
	for i, s := range plan.Sessions {
		order[s.Name] = i
	}
	moves := slices.Clone(plan.Moves)
	slices.SortStableFunc(moves, func(a, b HookMove) int { return cmp.Compare(order[a.To], order[b.To]) })
	return moves
}

// section writes heading, then the line of each item that has one, as its
// own block; with no item to write, nothing.
func section[T any](r *report, heading string, items []T, lineOf func(T) (string, bool)) {
	var lines []string
	for _, item := range items {
		if l, ok := lineOf(item); ok {
			lines = append(lines, l)
		}
	}
	if len(lines) == 0 {
		return
	}
	r.gap()
	r.line("%s", heading)
	for _, l := range lines {
		r.line("%s", l)
	}
}

// why is name's line saying why, when there is a why.
func why(name string, err error) (string, bool) {
	if err == nil {
		return "", false
	}
	return fmt.Sprintf(whyLine, name, err), true
}

// kindCopy says what a removed key was.
func kindCopy(kind HookKeyKind) string {
	switch kind {
	case PaneAddressKey:
		return paneAddressKeyCopy
	case PaneIDKey:
		return paneIDKeyCopy
	case UnsavedTokenKey:
		return unsavedTokenKeyCopy
	}
	return otherKeyCopy
}

// tmuxSessions counts the tmux sessions a plan made sessions of.
func tmuxSessions(plan Plan) int { return len(panesBySession(plan)) }

// panesBySession counts the sessions made of each tmux session's panes.
func panesBySession(plan Plan) map[string]int {
	panes := map[string]int{}
	for _, s := range plan.Sessions {
		panes[s.From.Session]++
	}
	return panes
}

// count is n of noun, "no" for none, its plural ending in s.
func count(n int, noun string) string {
	switch n {
	case 0:
		return "no " + noun + "s"
	case 1:
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
