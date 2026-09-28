package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/resumemode"
	"github.com/leeovery/portal/internal/themetest"
)

const resumeModeCheckName = "resume mode"

func resumeModeLine(detail string) string {
	return "  " + checkMarker(checkInfo) + " " + resumeModeCheckName + ": " + detail
}

// resumeModeLineIndex returns the index of the one rendered resume-mode line,
// failing the test when there is not exactly one.
func resumeModeLineIndex(t *testing.T, lines []string) int {
	t.Helper()
	found := -1
	for i, line := range lines {
		if strings.Contains(line, " "+resumeModeCheckName+": ") {
			if found != -1 {
				t.Fatalf("resume-mode line rendered more than once:\n%s", strings.Join(lines, "\n"))
			}
			found = i
		}
	}
	if found == -1 {
		t.Fatalf("no resume-mode line in report:\n%s", strings.Join(lines, "\n"))
	}
	return found
}

// doctorResumeModeLine runs doctor over a healthy install whose prefs store is
// resolved by doctor itself, and returns the rendered resume-mode line.
func doctorResumeModeLine(t *testing.T, deps *DoctorDeps) string {
	t.Helper()
	outBuf, _, err := runDoctorWith(t, deps)
	if err != nil {
		t.Fatalf("Execute err = %v; want nil", err)
	}
	lines := reportLines(outBuf.String())
	return lines[resumeModeLineIndex(t, lines)]
}

func TestDoctorResumeMode(t *testing.T) {
	defaultDetail := resumemode.Default.String()

	t.Run("it renders the install's persisted resume mode", func(t *testing.T) {
		for _, tc := range []struct {
			stored string
			want   string
		}{
			{stored: "eager", want: "eager"},
			{stored: "lazy", want: "lazy"},
		} {
			t.Run(tc.stored, func(t *testing.T) {
				setPrefsFile(t, `{"resume_mode":"`+tc.stored+`"}`)
				if got, want := doctorResumeModeLine(t, healthyDoctorDeps(t)), resumeModeLine(tc.want); got != want {
					t.Errorf("resume-mode line = %q; want %q", got, want)
				}
			})
		}
	})

	t.Run("it renders the shipped default when the key is absent", func(t *testing.T) {
		setPrefsFile(t, `{"session_list_mode":"by-tag"}`)
		if got, want := doctorResumeModeLine(t, healthyDoctorDeps(t)), resumeModeLine(defaultDetail); got != want {
			t.Errorf("resume-mode line = %q; want %q", got, want)
		}
	})

	t.Run("it renders the shipped default for an unreadable or corrupt prefs.json", func(t *testing.T) {
		for _, tc := range []struct {
			name  string
			stage func(t *testing.T)
		}{
			{name: "absent file", stage: func(t *testing.T) { setPrefsFile(t, "") }},
			{name: "unreadable file", stage: func(t *testing.T) {
				path := setPrefsFile(t, `{"resume_mode":"eager"}`)
				if err := themetest.DenyRead(t, path); err == nil {
					t.Skip("prefs.json stayed readable at mode 0000 (running as root?)")
				}
			}},
			{name: "corrupt file", stage: func(t *testing.T) { setPrefsFile(t, `{"resume_mode":"eager"`) }},
			{name: "unrecognised value", stage: func(t *testing.T) { setPrefsFile(t, `{"resume_mode":"Eager"}`) }},
		} {
			t.Run(tc.name, func(t *testing.T) {
				tc.stage(t)
				if got, want := doctorResumeModeLine(t, healthyDoctorDeps(t)), resumeModeLine(defaultDetail); got != want {
					t.Errorf("resume-mode line = %q; want %q", got, want)
				}
			})
		}
	})

	t.Run("it renders the shipped default when the prefs store could not be resolved", func(t *testing.T) {
		got := checkResumeMode(nil)
		want := checkResult{name: resumeModeCheckName, status: checkInfo, detail: defaultDetail}
		if got != want {
			t.Errorf("checkResumeMode(nil) = %+v; want %+v", got, want)
		}
	})

	t.Run("it dispatches no appearance translation", func(t *testing.T) {
		// Without the synchronous persist, TestMain's no-op seam would leave the
		// file byte-identical even under the migrating loader.
		syncPersistTranslation(t)
		const body = `{"session_list_mode":"by-tag","appearance":"light","resume_mode":"eager"}`
		path := setPrefsFile(t, body)

		if got, want := doctorResumeModeLine(t, healthyDoctorDeps(t)), resumeModeLine("eager"); got != want {
			t.Errorf("resume-mode line = %q; want %q", got, want)
		}

		after, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read prefs.json after doctor: %v", err)
		}
		if !bytes.Equal(after, []byte(body)) {
			t.Errorf("prefs.json changed across a doctor run:\n before %s\n after  %s", body, after)
		}
	})

	t.Run("it writes no prefs.json when there is none", func(t *testing.T) {
		path := setPrefsFile(t, "")

		doctorResumeModeLine(t, healthyDoctorDeps(t))

		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("os.Stat(%s) = %v; doctor must never create prefs.json", path, err)
		}
		if entries, err := os.ReadDir(filepath.Dir(path)); err != nil || len(entries) != 0 {
			t.Errorf("prefs dir holds %v (err %v) after doctor; want nothing written", entries, err)
		}
	})

	t.Run("it moves neither the exit code nor the summary counts", func(t *testing.T) {
		summaryFor := func(stored string) string {
			setPrefsFile(t, `{"resume_mode":"`+stored+`"}`)
			outBuf, _, err := runDoctorWith(t, healthyDoctorDeps(t))
			if err != nil {
				t.Fatalf("Execute err = %v; want nil with resume mode %s", err, stored)
			}
			lines := reportLines(outBuf.String())
			return lines[len(lines)-1]
		}
		for _, stored := range []string{"eager", "lazy"} {
			if got := summaryFor(stored); got != "  7 checks passed" {
				t.Errorf("summary with resume mode %s = %q; want %q", stored, got, "  7 checks passed")
			}
		}

		setPrefsFile(t, `{"resume_mode":"eager"}`)
		results, err := runDoctorDiagnosis(resolveDepsFor(t, healthyDoctorDeps(t)))
		if err != nil {
			t.Fatalf("runDoctorDiagnosis: %v", err)
		}
		catalog := withoutCheck(results, resumeModeCheckName)
		if len(catalog) != len(results)-1 {
			t.Fatalf("diagnosis carries %d resume-mode results; want exactly 1", len(results)-len(catalog))
		}
		if doctorUnhealthy(results) != doctorUnhealthy(catalog) {
			t.Errorf("the resume-mode line changed the exit verdict")
		}
		gotPassed, gotTotal := doctorCheckCounts(results)
		wantPassed, wantTotal := doctorCheckCounts(catalog)
		if gotPassed != wantPassed || gotTotal != wantTotal {
			t.Errorf("doctorCheckCounts = (%d, %d); want (%d, %d) — the resume-mode line counts for neither", gotPassed, gotTotal, wantPassed, wantTotal)
		}
	})

	t.Run("it ignores a registration's own mode", func(t *testing.T) {
		setPrefsFile(t, `{"resume_mode":"lazy"}`)
		deps := healthyDoctorDeps(t)
		seed := `{"` + hookstest.LiveSeedA + `": {"on-resume": {"command": "echo hi", "resume": "eager"}}}`
		deps.HookStore, _ = hookstest.StageStore(t, hookstest.Staging{Seed: seed})

		if got, want := doctorResumeModeLine(t, deps), resumeModeLine("lazy"); got != want {
			t.Errorf("resume-mode line = %q; want %q — the install default alone", got, want)
		}
	})

	t.Run("it trails the pending-resumes line at the end of the catalog", func(t *testing.T) {
		setPrefsFile(t, `{"resume_mode":"eager"}`)
		outBuf, _, err := runDoctorWith(t, healthyDoctorDeps(t))
		if err != nil {
			t.Fatalf("Execute err = %v; want nil", err)
		}
		lines := reportLines(outBuf.String())
		i := resumeModeLineIndex(t, lines)
		if pending := pendingLineIndex(t, lines); pending != i-1 {
			t.Errorf("resume-mode line at %d, pending line at %d; want the resume-mode line directly after it:\n%s", i, pending, outBuf.String())
		}
		if i != len(lines)-2 {
			t.Errorf("resume-mode line at %d; want the last line before the summary (%d):\n%s", i, len(lines)-2, outBuf.String())
		}
	})
}

// resolveDepsFor fills the seams a test left unset the way a doctor run does,
// so a direct diagnosis reads prefs through the same non-migrating route.
func resolveDepsFor(t *testing.T, deps *DoctorDeps) *DoctorDeps {
	t.Helper()
	isolateTerminalsFile(t)
	withDoctorDeps(t, *deps)
	return resolveDoctorDeps()
}
