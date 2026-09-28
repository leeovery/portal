package cmd

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// doctorUnhealthy's sole input is the check catalog, so an advisory cannot
// reach the exit decision even by accident; widening the signature stops this
// file compiling.
var _ func([]checkResult) bool = doctorUnhealthy

// themeAdvisories builds n distinct advisory values, each already carrying its
// leading glyph: producers own the whole string and the renderer only indents.
func themeAdvisories(n int) []advisory {
	out := make([]advisory, 0, n)
	for i := range n {
		out = append(out, advisory{
			line: fmt.Sprintf("⚠ theme theme%d: bad colour — token text.primary", i),
		})
	}
	return out
}

// renderedLines drops the trailing newline rather than yielding a phantom empty
// final element, so a line index here is the index of a real rendered line.
func renderedLines(t *testing.T, results []checkResult, advisories []advisory) []string {
	t.Helper()
	var buf bytes.Buffer
	renderDoctorReport(&buf, results, advisories)
	out := buf.String()
	if !strings.HasSuffix(out, "\n") {
		t.Fatalf("report does not end in a newline: %q", out)
	}
	return strings.Split(strings.TrimSuffix(out, "\n"), "\n")
}

func mixedCatalog() []checkResult {
	return []checkResult{
		{name: "daemon", status: checkPass, detail: "running (pid 1, version v1.0.0)"},
		{name: "saver", status: checkFail, detail: "_portal-saver not running"},
		{name: "host terminal", status: checkInfo, detail: "Ghostty (supported)"},
	}
}

func TestAdvisories_CarryOnlyTheRenderedLine(t *testing.T) {
	typ := reflect.TypeFor[advisory]()

	if got := typ.NumField(); got != 1 {
		var names []string
		for i := range got {
			names = append(names, typ.Field(i).Name)
		}
		t.Fatalf("advisory carries %d fields (%s); want only the line the renderer prints", got, strings.Join(names, ", "))
	}
	if got := typ.Field(0).Name; got != "line" {
		t.Errorf("advisory's only field is %q; want %q", got, "line")
	}
}

func TestAdvisories_BlockPositionIsFixed(t *testing.T) {
	results := mixedCatalog()
	advisories := themeAdvisories(2)

	lines := renderedLines(t, results, advisories)

	want := []string{
		"Portal doctor:",
		"  ✓ daemon: running (pid 1, version v1.0.0)",
		"  ✗ saver: _portal-saver not running",
		"    host terminal: Ghostty (supported)",
		"  ⚠ theme theme0: bad colour — token text.primary",
		"  ⚠ theme theme1: bad colour — token text.primary",
		"  1 of 2 checks passed · 2 advisories",
	}
	if len(lines) != len(want) {
		t.Fatalf("rendered %d lines, want %d:\n%s", len(lines), len(want), strings.Join(lines, "\n"))
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Errorf("line[%d] = %q; want %q", i, lines[i], want[i])
		}
	}
}

func TestAdvisories_NeverInterleave(t *testing.T) {
	results := []checkResult{
		{name: "daemon", status: checkPass, detail: "running"},
		{name: "saver", status: checkPass, detail: "_portal-saver up"},
		{name: "hooks", status: checkFail, detail: "hooks not registered on client-attached"},
		{name: "stale hooks", status: checkNotEvaluable, detail: "could not read hooks.json"},
	}
	advisories := themeAdvisories(3)

	lines := renderedLines(t, results, advisories)

	// Index 0 is the header, so the catalog occupies indices 1..len(results).
	for i, r := range results {
		want := fmt.Sprintf("  %s %s: %s", checkMarker(r.status), r.name, r.detail)
		if got := lines[1+i]; got != want {
			t.Fatalf("line[%d] = %q; want the check line %q", 1+i, got, want)
		}
	}

	firstAdvisory := len(results) + 1
	for i, a := range advisories {
		if got, want := lines[firstAdvisory+i], "  "+a.line; got != want {
			t.Errorf("line[%d] = %q; want the advisory %q — the block must be contiguous and follow the whole catalog", firstAdvisory+i, got, want)
		}
	}

	for i, line := range lines {
		if !strings.Contains(line, "⚠") {
			continue
		}
		if i < firstAdvisory {
			t.Errorf("advisory line %q rendered at index %d, inside the catalog region (which ends at %d)", line, i, len(results))
		}
	}
}

func TestAdvisories_HostTerminalStaysInCatalog(t *testing.T) {
	deps := healthyDoctorDeps(t)
	results, err := runDoctorDiagnosis(deps)
	if err != nil {
		t.Fatalf("runDoctorDiagnosis: %v", err)
	}
	if len(results) < 2 {
		t.Fatalf("catalog = %+v; want at least the two informational lines", results)
	}
	host, pending := results[len(results)-2], results[len(results)-1]
	if host.name != "host terminal" || host.status != checkInfo {
		t.Fatalf("second-to-last catalog check = %+v; want the informational host-terminal line", host)
	}
	if pending.name != "pending resumes" || pending.status != checkInfo {
		t.Fatalf("last catalog check = %+v; want the informational pending-resumes line", pending)
	}

	advisories := themeAdvisories(2)
	lines := renderedLines(t, results, advisories)

	// Index 0 is the header, so catalog entry i renders at line i+1.
	hostLine := fmt.Sprintf("  %s %s: %s", checkMarker(host.status), host.name, host.detail)
	if got := lines[len(results)-1]; got != hostLine {
		t.Errorf("line[%d] = %q; want the host-terminal line %q inside the catalog", len(results)-1, got, hostLine)
	}
	pendingLine := fmt.Sprintf("  %s %s: %s", checkMarker(pending.status), pending.name, pending.detail)
	if got := lines[len(results)]; got != pendingLine {
		t.Errorf("line[%d] = %q; want the pending-resumes line %q at the end of the catalog", len(results), got, pendingLine)
	}
	if got, want := lines[len(results)+1], "  "+advisories[0].line; got != want {
		t.Errorf("line[%d] = %q; want the first advisory %q immediately after the catalog", len(results)+1, got, want)
	}
}

func TestAdvisories_SummarySuffix(t *testing.T) {
	allPassed := []checkResult{
		{name: "daemon", status: checkPass},
		{name: "saver", status: checkPass},
	}
	someFailed := []checkResult{
		{name: "daemon", status: checkPass},
		{name: "saver", status: checkFail},
	}

	cases := []struct {
		name       string
		results    []checkResult
		advisories int
		want       string
	}{
		{"all passed, one advisory", allPassed, 1, "2 checks passed · 1 advisory"},
		{"all passed, two advisories", allPassed, 2, "2 checks passed · 2 advisories"},
		{"all passed, five advisories", allPassed, 5, "2 checks passed · 5 advisories"},
		{"some failed, one advisory", someFailed, 1, "1 of 2 checks passed · 1 advisory"},
		{"some failed, three advisories", someFailed, 3, "1 of 2 checks passed · 3 advisories"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			advisories := themeAdvisories(tc.advisories)
			if got := doctorSummaryLine(tc.results, advisories); got != tc.want {
				t.Errorf("doctorSummaryLine = %q; want %q", got, tc.want)
			}

			lines := renderedLines(t, tc.results, advisories)
			if got, want := lines[len(lines)-1], "  "+tc.want; got != want {
				t.Errorf("rendered summary = %q; want %q", got, want)
			}
		})
	}
}

func TestAdvisories_SuffixSuppressedAtZero(t *testing.T) {
	cases := []struct {
		name       string
		advisories []advisory
	}{
		{"nil slice", nil},
		{"empty slice", []advisory{}},
	}

	results := mixedCatalog()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := doctorSummaryLine(results, tc.advisories)
			if want := "1 of 2 checks passed"; got != want {
				t.Errorf("doctorSummaryLine = %q; want %q", got, want)
			}
			if strings.Contains(got, "·") || strings.Contains(got, "advisor") {
				t.Errorf("doctorSummaryLine = %q; the suffix must be suppressed entirely at zero advisories", got)
			}
		})
	}
}

func TestAdvisories_NeverDriveExitCode(t *testing.T) {
	deps := healthyDoctorDeps(t)
	if _, _, err := runDoctorWith(t, deps); err != nil {
		t.Fatalf("Execute err = %v; want nil over an all-passing catalog", err)
	}

	results, err := runDoctorDiagnosis(deps)
	if err != nil {
		t.Fatalf("runDoctorDiagnosis: %v", err)
	}
	if doctorUnhealthy(results) {
		t.Fatalf("precondition: the catalog must be all-passing, got %+v", results)
	}

	lines := renderedLines(t, results, themeAdvisories(5))

	if doctorUnhealthy(results) {
		t.Error("doctorUnhealthy = true after rendering five advisories; an advisory must never reach the exit decision")
	}
	if got, want := lines[len(lines)-1], "  7 checks passed · 5 advisories"; got != want {
		t.Errorf("rendered summary = %q; want %q — five advisories are counted, and the run is still healthy", got, want)
	}
}

func TestAdvisories_FailingCheckAndAdvisoriesShareOneSummary(t *testing.T) {
	deps := healthyDoctorDeps(t)
	deps.SaverPresent = func() (bool, error) { return false, nil }

	if _, _, err := runDoctorWith(t, deps); err != ErrDoctorUnhealthy {
		t.Fatalf("Execute err = %v; want ErrDoctorUnhealthy with a failing saver check", err)
	}

	results, err := runDoctorDiagnosis(deps)
	if err != nil {
		t.Fatalf("runDoctorDiagnosis: %v", err)
	}

	lines := renderedLines(t, results, themeAdvisories(2))

	var summaries []string
	for _, line := range lines {
		if strings.Contains(line, "checks passed") {
			summaries = append(summaries, line)
		}
	}
	if len(summaries) != 1 {
		t.Fatalf("summary line count = %d, want 1:\n%s", len(summaries), strings.Join(lines, "\n"))
	}
	if got, want := summaries[0], "  6 of 7 checks passed · 2 advisories"; got != want {
		t.Errorf("summary = %q; want %q", got, want)
	}
}

func TestAdvisories_GlyphBackedNoMarker(t *testing.T) {
	a := advisory{line: "⚠ themes directory unreadable: /home/u/.config/portal/themes"}

	lines := renderedLines(t, nil, []advisory{a})
	got := lines[1]

	if want := "  " + a.line; got != want {
		t.Errorf("advisory line = %q; want %q — indent only, nothing prepended or appended", got, want)
	}
	if !strings.HasPrefix(got, "  ⚠") {
		t.Errorf("advisory line = %q; want the glyph immediately after the indent, with no marker column", got)
	}
	for _, marker := range []string{"✓", "✗"} {
		if strings.Contains(got, marker) {
			t.Errorf("advisory line = %q; must carry no pass/fail marker (%q)", got, marker)
		}
	}
}

func TestAdvisories_EmptyBlockRendersNothing(t *testing.T) {
	results := mixedCatalog()

	var nilBuf, emptyBuf bytes.Buffer
	renderDoctorReport(&nilBuf, results, nil)
	renderDoctorReport(&emptyBuf, results, []advisory{})

	want := "Portal doctor:\n" +
		"  ✓ daemon: running (pid 1, version v1.0.0)\n" +
		"  ✗ saver: _portal-saver not running\n" +
		"    host terminal: Ghostty (supported)\n" +
		"  1 of 2 checks passed\n"
	if got := nilBuf.String(); got != want {
		t.Errorf("report with no advisories mismatch\n got: %q\nwant: %q", got, want)
	}
	if nilBuf.String() != emptyBuf.String() {
		t.Errorf("nil and empty advisory slices render differently:\n nil: %q\nempty: %q", nilBuf.String(), emptyBuf.String())
	}
}
