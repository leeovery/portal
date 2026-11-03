package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/backendtest"
)

// healthyDoctorDeps is an install on which every check passes.
func healthyDoctorDeps(t *testing.T) *DoctorDeps {
	t.Helper()
	projectStore, _ := seedProjectsJSON(t, t.TempDir())
	return staleDeps(t, projectStore)
}

func TestDoctorSummary_AllChecksPassed(t *testing.T) {
	outBuf, _, err := runDoctorWith(t, healthyDoctorDeps(t))
	if err != nil {
		t.Fatalf("Execute err = %v; want nil when every check passes", err)
	}

	const want = "\n  6 checks passed\n"
	if out := outBuf.String(); !strings.HasSuffix(out, want) {
		t.Errorf("report does not end with %q:\n%s", want, out)
	}
}

func TestDoctorSummary_PartialForm(t *testing.T) {
	deps := healthyDoctorDeps(t)
	deps.LookPath = zmxMissing

	outBuf, _, err := runDoctorWith(t, deps)
	if err != ErrDoctorUnhealthy {
		t.Fatalf("Execute err = %v; want ErrDoctorUnhealthy with zmx missing", err)
	}

	const want = "\n  5 of 6 checks passed\n"
	if out := outBuf.String(); !strings.HasSuffix(out, want) {
		t.Errorf("report does not end with %q:\n%s", want, out)
	}
}

func TestDoctorSummary_InfoLineOutsideCounts(t *testing.T) {
	catalog := []checkResult{
		{name: "daemon", status: checkPass, detail: "running"},
		{name: "saver", status: checkPass, detail: "_portal-saver up"},
	}
	withInfo := append(slices.Clone(catalog), checkResult{name: "host terminal", status: checkInfo, detail: "Ghostty (supported)"})

	wantPassed, wantTotal := doctorCheckCounts(catalog)
	gotPassed, gotTotal := doctorCheckCounts(withInfo)
	if gotPassed != wantPassed || gotTotal != wantTotal {
		t.Errorf("doctorCheckCounts(with info line) = (%d, %d); want (%d, %d) — the info line must count for neither", gotPassed, gotTotal, wantPassed, wantTotal)
	}
	if got := doctorSummaryLine(withInfo, nil); got != "2 checks passed" {
		t.Errorf("doctorSummaryLine(with info line) = %q; want %q", got, "2 checks passed")
	}
}

func TestDoctorSummary_NotEvaluableOutsideCounts(t *testing.T) {
	stageDoctorBackend(t).Fail(backendtest.OpList, errors.New("transient"))
	deps := healthyDoctorDeps(t)

	outBuf, _, err := runDoctorWith(t, deps)
	if err != nil {
		t.Fatalf("Execute err = %v; want nil — a not-evaluable check must not drive the exit code", err)
	}

	const want = "\n  5 checks passed\n"
	if out := outBuf.String(); !strings.HasSuffix(out, want) {
		t.Errorf("report does not end with %q:\n%s", want, out)
	}
}

func TestDoctorSummary_UnknownCountsTowardTotalOnly(t *testing.T) {
	results := []checkResult{
		{name: "daemon", status: checkPass, detail: "running"},
		{name: "forgotten"},
	}

	passed, total := doctorCheckCounts(results)
	if passed != 1 || total != 2 {
		t.Errorf("doctorCheckCounts = (%d, %d); want (1, 2) — checkUnknown counts toward the total only", passed, total)
	}
	if !doctorUnhealthy(results) {
		t.Error("doctorUnhealthy = false; the iota-0 sentinel must read as unhealthy")
	}
	if got := doctorSummaryLine(results, nil); got != "1 of 2 checks passed" {
		t.Errorf("doctorSummaryLine = %q; want %q", got, "1 of 2 checks passed")
	}
}

func TestDoctorSummary_MatchesDoctorUnhealthy(t *testing.T) {
	pass := checkResult{name: "pass", status: checkPass, detail: "ok"}
	fail := checkResult{name: "fail", status: checkFail, detail: "broken"}
	unknown := checkResult{name: "forgotten"}
	info := checkResult{name: "host terminal", status: checkInfo, detail: "Ghostty (supported)"}
	notEval := checkResult{name: "stale hooks", status: checkNotEvaluable, detail: "not evaluable"}

	cases := []struct {
		name    string
		results []checkResult
	}{
		{"empty catalog", nil},
		{"pass only", []checkResult{pass, pass, pass}},
		{"one fail", []checkResult{pass, fail, pass}},
		{"one unknown", []checkResult{pass, unknown, pass}},
		{"one info", []checkResult{pass, pass, info}},
		{"one not-evaluable", []checkResult{pass, pass, notEval}},
		{"info and not-evaluable alongside all-pass", []checkResult{pass, pass, info, notEval}},
		{"info and not-evaluable alongside a fail", []checkResult{pass, fail, info, notEval}},
		{"fail and unknown together", []checkResult{fail, unknown}},
		{"every status at once", []checkResult{pass, fail, unknown, info, notEval}},
		{"info only", []checkResult{info}},
		{"not-evaluable only", []checkResult{notEval}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			passed, total := doctorCheckCounts(tc.results)
			healthy := !doctorUnhealthy(tc.results)

			if (passed == total) != healthy {
				t.Errorf("doctorCheckCounts = (%d, %d) so passed==total is %t, but doctorUnhealthy reports healthy=%t — the summary and the exit code disagree about the same run",
					passed, total, passed == total, healthy)
			}

			want := fmt.Sprintf("%d of %d checks passed", passed, total)
			if healthy {
				want = fmt.Sprintf("%d checks passed", passed)
			}
			if got := doctorSummaryLine(tc.results, nil); got != want {
				t.Errorf("doctorSummaryLine = %q; want %q", got, want)
			}
		})
	}
}

func TestDoctorSummary_IsTheLastLine(t *testing.T) {
	results := []checkResult{
		{name: "daemon", status: checkPass, detail: "running (pid 1, version v1.0.0)"},
		{name: "saver", status: checkFail, detail: "_portal-saver not running"},
		{name: "stale hooks", status: checkNotEvaluable, detail: "could not read hooks.json"},
		{name: "host terminal", status: checkInfo, detail: "Ghostty (supported)"},
	}

	var buf bytes.Buffer
	renderDoctorReport(&buf, results, nil)

	want := "Portal doctor:\n" +
		"  ✓ daemon: running (pid 1, version v1.0.0)\n" +
		"  ✗ saver: _portal-saver not running\n" +
		"  · stale hooks: could not read hooks.json\n" +
		"    host terminal: Ghostty (supported)\n" +
		"  1 of 2 checks passed\n"
	if got := buf.String(); got != want {
		t.Errorf("renderDoctorReport output mismatch\n got: %q\nwant: %q", got, want)
	}
}

func TestDoctorSummary_FixPathRendersTwo(t *testing.T) {
	projectStore, _ := seedProjectsJSON(t, filepath.Join(t.TempDir(), "gone"))
	deps := staleDeps(t, projectStore)

	outBuf, _, err := runDoctorWith(t, deps, "--fix")
	if err != nil {
		t.Fatalf("Execute err = %v; want nil (healthy post-repair)", err)
	}

	out := outBuf.String()
	if n := strings.Count(out, " checks passed\n"); n != 2 {
		t.Errorf("summary line count = %d; want 2 (one per renderDoctorReport call):\n%s", n, out)
	}
	if n := strings.Count(out, "\n  5 of 6 checks passed\n"); n != 1 {
		t.Errorf("pre-repair summary missing or repeated (want exactly one \"5 of 6 checks passed\"):\n%s", out)
	}
	if !strings.HasSuffix(out, "\n  6 checks passed\n") {
		t.Errorf("post-repair report does not end with the all-passed summary:\n%s", out)
	}
}

func TestDoctorSummary_NoSingularCarveOut(t *testing.T) {
	cases := []struct {
		name    string
		results []checkResult
		want    string
	}{
		{"one passing check", []checkResult{{name: "daemon", status: checkPass}}, "1 checks passed"},
		{"one failing check", []checkResult{{name: "daemon", status: checkFail}}, "0 of 1 checks passed"},
		{"one passing beside one failing", []checkResult{{name: "daemon", status: checkPass}, {name: "saver", status: checkFail}}, "1 of 2 checks passed"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := doctorSummaryLine(tc.results, nil)
			if got != tc.want {
				t.Errorf("doctorSummaryLine = %q; want %q — the checks count has exactly one form", got, tc.want)
			}
			if strings.Contains(got, "check passed") {
				t.Errorf("doctorSummaryLine = %q; the singular \"check passed\" must never render", got)
			}
		})
	}
}
