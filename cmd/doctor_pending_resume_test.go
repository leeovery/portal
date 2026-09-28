package cmd

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/tmux"
)

const pendingResumeCheckName = "pending resumes"

// pendingResumeRead stands in for the whole-server enumeration, counting the
// calls it answers so a test can tell a read taken from one never attempted.
type pendingResumeRead struct {
	view  tmux.PendingResumeView
	err   error
	calls int
}

func (r *pendingResumeRead) read() (tmux.PendingResumeView, error) {
	r.calls++
	return r.view, r.err
}

func pendingView(sessions ...string) tmux.PendingResumeView {
	view := tmux.PendingResumeView{Rows: []tmux.PendingResumeRow{}, Sessions: map[string]struct{}{}}
	for _, s := range sessions {
		view.Rows = append(view.Rows, tmux.PendingResumeRow{Pending: true, Session: s})
		view.Panes++
		view.Sessions[s] = struct{}{}
	}
	return view
}

func pendingViewOfPanes(n int) tmux.PendingResumeView {
	sessions := make([]string, n)
	for i := range sessions {
		sessions[i] = "project-" + string(rune('a'+i%26))
	}
	return pendingView(sessions...)
}

func reportLines(out string) []string {
	return strings.Split(strings.TrimSuffix(out, "\n"), "\n")
}

// pendingLineIndex returns the index of the one rendered pending line, failing
// the test when there is not exactly one.
func pendingLineIndex(t *testing.T, lines []string) int {
	t.Helper()
	found := -1
	for i, line := range lines {
		if strings.Contains(line, " "+pendingResumeCheckName+": ") {
			if found != -1 {
				t.Fatalf("pending line rendered more than once:\n%s", strings.Join(lines, "\n"))
			}
			found = i
		}
	}
	if found == -1 {
		t.Fatalf("no pending line in report:\n%s", strings.Join(lines, "\n"))
	}
	return found
}

func withoutCheck(results []checkResult, name string) []checkResult {
	return slices.DeleteFunc(slices.Clone(results), func(r checkResult) bool { return r.name == name })
}

func TestDoctorPendingResume(t *testing.T) {
	t.Run("it renders the pending-pane count as an informational line", func(t *testing.T) {
		deps := healthyDoctorDeps(t)
		read := &pendingResumeRead{view: pendingView("alpha", "beta", "gamma")}
		deps.PendingResumes = read.read

		outBuf, _, err := runDoctorWith(t, deps)
		if err != nil {
			t.Fatalf("Execute err = %v; want nil", err)
		}

		lines := reportLines(outBuf.String())
		i := pendingLineIndex(t, lines)
		want := "  " + checkMarker(checkInfo) + " pending resumes: 3 panes waiting to resume"
		if lines[i] != want {
			t.Errorf("pending line = %q; want %q", lines[i], want)
		}
		host := slices.IndexFunc(lines, func(l string) bool { return strings.Contains(l, " host terminal: ") })
		if host == -1 || host > i {
			t.Errorf("pending line must trail the pass/fail catalog and the host-terminal line:\n%s", outBuf.String())
		}
		if i != len(lines)-2 {
			t.Errorf("pending line at %d; want the last line before the summary (%d):\n%s", i, len(lines)-2, outBuf.String())
		}
	})

	t.Run("it exits zero with forty-one pending panes", func(t *testing.T) {
		deps := healthyDoctorDeps(t)
		deps.PendingResumes = (&pendingResumeRead{view: pendingViewOfPanes(41)}).read

		outBuf, _, err := runDoctorWith(t, deps)
		if err != nil {
			t.Fatalf("Execute err = %v; want nil — waiting panes are not a fault", err)
		}
		lines := reportLines(outBuf.String())
		if got, want := lines[pendingLineIndex(t, lines)], "    pending resumes: 41 panes waiting to resume"; got != want {
			t.Errorf("pending line = %q; want %q", got, want)
		}

		results, err := runDoctorDiagnosis(deps)
		if err != nil {
			t.Fatalf("runDoctorDiagnosis: %v", err)
		}
		if doctorUnhealthy(results) {
			t.Errorf("doctorUnhealthy = true with 41 pending panes; want false")
		}
	})

	t.Run("it leaves the pending line out of the passed and total counts", func(t *testing.T) {
		summaryFor := func(read *pendingResumeRead) string {
			deps := healthyDoctorDeps(t)
			deps.PendingResumes = read.read
			outBuf, _, err := runDoctorWith(t, deps)
			if err != nil {
				t.Fatalf("Execute err = %v; want nil", err)
			}
			lines := reportLines(outBuf.String())
			return lines[len(lines)-1]
		}

		zero := summaryFor(&pendingResumeRead{view: pendingView()})
		many := summaryFor(&pendingResumeRead{view: pendingViewOfPanes(41)})
		if zero != "  7 checks passed" {
			t.Errorf("summary with zero pending = %q; want %q", zero, "  7 checks passed")
		}
		if many != zero {
			t.Errorf("summary with 41 pending = %q; want it identical to the zero-pending %q", many, zero)
		}
	})

	t.Run("it reports a failed read as not evaluable rather than as zero", func(t *testing.T) {
		deps := healthyDoctorDeps(t)
		read := &pendingResumeRead{err: errors.New("tmux transient")}
		deps.PendingResumes = read.read

		outBuf, _, err := runDoctorWith(t, deps)
		if err != nil {
			t.Fatalf("Execute err = %v; want nil — a failed read must not move the exit code", err)
		}
		lines := reportLines(outBuf.String())
		want := "  " + checkMarker(checkNotEvaluable) + " pending resumes: could not read pending panes (transient tmux error)"
		if got := lines[pendingLineIndex(t, lines)]; got != want {
			t.Errorf("pending line = %q; want %q", got, want)
		}
		if got := lines[len(lines)-1]; got != "  7 checks passed" {
			t.Errorf("summary = %q; want %q", got, "  7 checks passed")
		}
	})

	t.Run("it reports a down runtime as not evaluable rather than as a failure", func(t *testing.T) {
		deps := healthyDoctorDeps(t)
		deps.ServerRunning = func() bool { return false }
		read := &pendingResumeRead{view: pendingViewOfPanes(5)}
		deps.PendingResumes = read.read

		outBuf, _, _ := runDoctorWith(t, deps)
		lines := reportLines(outBuf.String())
		want := "  " + checkMarker(checkNotEvaluable) + " pending resumes: " + doctorRuntimeNotRunning
		if got := lines[pendingLineIndex(t, lines)]; got != want {
			t.Errorf("pending line = %q; want %q", got, want)
		}
		if read.calls != 0 {
			t.Errorf("pending read called %d times on a down runtime; want 0", read.calls)
		}

		results, err := runDoctorDiagnosis(deps)
		if err != nil {
			t.Fatalf("runDoctorDiagnosis: %v", err)
		}
		catalog := withoutCheck(results, pendingResumeCheckName)
		if doctorUnhealthy(results) != doctorUnhealthy(catalog) {
			t.Errorf("the pending line changed the exit verdict on a down runtime")
		}
		gotPassed, gotTotal := doctorCheckCounts(results)
		wantPassed, wantTotal := doctorCheckCounts(catalog)
		if gotPassed != wantPassed || gotTotal != wantTotal {
			t.Errorf("doctorCheckCounts = (%d, %d); want (%d, %d) — the pending line counts for neither", gotPassed, gotTotal, wantPassed, wantTotal)
		}
	})

	t.Run("it renders a line when nothing is pending", func(t *testing.T) {
		deps := healthyDoctorDeps(t)
		read := &pendingResumeRead{view: pendingView()}
		deps.PendingResumes = read.read

		outBuf, _, err := runDoctorWith(t, deps)
		if err != nil {
			t.Fatalf("Execute err = %v; want nil", err)
		}
		lines := reportLines(outBuf.String())
		if got, want := lines[pendingLineIndex(t, lines)], "    pending resumes: 0 panes waiting to resume"; got != want {
			t.Errorf("pending line = %q; want %q", got, want)
		}
		if read.calls != 1 {
			t.Errorf("pending read called %d times; want exactly the one injected read", read.calls)
		}
	})

	t.Run("it renders the line in both reports under --fix and repairs nothing", func(t *testing.T) {
		deps := healthyDoctorDeps(t)
		read := &pendingResumeRead{view: pendingViewOfPanes(2)}
		deps.PendingResumes = read.read

		outBuf, _, err := runDoctorWith(t, deps, "--fix")
		if err != nil {
			t.Fatalf("Execute err = %v; want nil", err)
		}
		out := outBuf.String()
		const line = "    pending resumes: 2 panes waiting to resume\n"
		if n := strings.Count(out, line); n != 2 {
			t.Errorf("pending line rendered %d times; want 2 (pre- and post-repair):\n%s", n, out)
		}
		if read.calls != 2 {
			t.Errorf("pending read called %d times; want 2 — one per diagnosis, none by a repair", read.calls)
		}
		for _, l := range reportLines(out) {
			if strings.HasPrefix(l, "Pruned ") || strings.HasPrefix(l, "Skipped ") {
				t.Errorf("unexpected repair line %q:\n%s", l, out)
			}
		}
	})

	t.Run("it counts panes rather than sessions", func(t *testing.T) {
		deps := healthyDoctorDeps(t)
		deps.PendingResumes = (&pendingResumeRead{view: pendingView("alpha", "alpha")}).read

		outBuf, _, err := runDoctorWith(t, deps)
		if err != nil {
			t.Fatalf("Execute err = %v; want nil", err)
		}
		lines := reportLines(outBuf.String())
		if got, want := lines[pendingLineIndex(t, lines)], "    pending resumes: 2 panes waiting to resume"; got != want {
			t.Errorf("pending line = %q; want %q", got, want)
		}
	})
}
