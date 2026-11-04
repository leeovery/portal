package cmd

import (
	"bytes"
	"encoding/json"
	"go/ast"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// The repair breadcrumbs between the two reports are deliberately excluded, so
// an assertion about a report's trailing block cannot be satisfied by a line
// the repairs printed.
func requireTwoReports(t *testing.T, out string) (pre, post string) {
	t.Helper()

	var reports []string
	var current []string
	inReport := false
	for line := range strings.SplitSeq(strings.TrimSuffix(out, "\n"), "\n") {
		if line == "Portal doctor:" {
			inReport = true
			current = nil
			continue
		}
		if !inReport {
			continue
		}
		current = append(current, line)
		if strings.Contains(line, " checks passed") {
			reports = append(reports, strings.Join(current, "\n"))
			inReport = false
		}
	}

	if len(reports) != 2 {
		t.Fatalf("stdout holds %d complete reports, want 2 (the initial diagnosis and the post-repair re-diagnosis):\n%s", len(reports), out)
	}
	return reports[0], reports[1]
}

func requireBothReportsEndWith(t *testing.T, out, want string) {
	t.Helper()

	pre, post := requireTwoReports(t, out)
	for i, report := range []string{pre, post} {
		label := "initial diagnosis"
		if i == 1 {
			label = "post-repair re-diagnosis"
		}
		if !strings.HasSuffix(report, want) {
			t.Errorf("the %s does not close with\n%s\ngot\nPortal doctor:\n%s", label, want, report)
		}
	}
}

func TestDoctorFix_AdvisoriesInBothPasses(t *testing.T) {
	deps := healthyDoctorDeps(t)
	deps.ThemesDir = themesDirWith(t, map[string][]byte{
		"a-missing.theme": sourceMissingTokens(t, "text.primary"),
		"b-colour.theme":  sourceBadColours(t, themeOverride{"canvas", "blue"}),
		"c-valid.theme":   validThemeSource(t),
	})

	outBuf, _, err := runDoctorWith(t, deps, "--fix")
	if err != nil {
		t.Fatalf("Execute err = %v; a broken drop-in must never drive the exit code", err)
	}

	requireBothReportsEndWith(t, outBuf.String(), ""+
		"  ⚠ theme a-missing: missing tokens — missing text.primary\n"+
		"  ⚠ theme b-colour: bad colour — canvas = blue\n"+
		"  10 checks passed · 2 advisories")
}

func TestDoctorFix_SuffixInBothSummaries(t *testing.T) {
	deps := healthyDoctorDeps(t)
	deps.ThemesDir = themesDirWith(t, map[string][]byte{
		"a-missing.theme": sourceMissingTokens(t, "text.primary"),
		"b-colour.theme":  sourceBadColours(t, themeOverride{"canvas", "blue"}),
	})

	outBuf, _, err := runDoctorWith(t, deps, "--fix")
	if err != nil {
		t.Fatalf("Execute err = %v; a broken drop-in must never drive the exit code", err)
	}

	out := outBuf.String()
	if n := strings.Count(out, "\n  10 checks passed · 2 advisories\n"); n != 2 {
		t.Errorf("suffixed summary count = %d; want 2 (one per report render):\n%s", n, out)
	}

	t.Run("the suffix rides both summary forms", func(t *testing.T) {
		// A stale project makes the two summaries carry different checks
		// halves and the same suffix.
		projectStore, _ := seedProjectsJSON(t, filepath.Join(t.TempDir(), "gone"))
		deps := staleDeps(t, projectStore)
		deps.ThemesDir = themesDirWith(t, map[string][]byte{
			"a-missing.theme": sourceMissingTokens(t, "text.primary"),
			"b-colour.theme":  sourceBadColours(t, themeOverride{"canvas", "blue"}),
		})

		outBuf, _, err := runDoctorWith(t, deps, "--fix")
		if err != nil {
			t.Fatalf("Execute err = %v; want nil — the repaired catalog is healthy and an advisory never moves the exit code", err)
		}

		const block = "" +
			"  ⚠ theme a-missing: missing tokens — missing text.primary\n" +
			"  ⚠ theme b-colour: bad colour — canvas = blue\n"
		pre, post := requireTwoReports(t, outBuf.String())
		if want := block + "  9 of 10 checks passed · 2 advisories"; !strings.HasSuffix(pre, want) {
			t.Errorf("the initial diagnosis does not close with\n%s\ngot\nPortal doctor:\n%s", want, pre)
		}
		if want := block + "  10 checks passed · 2 advisories"; !strings.HasSuffix(post, want) {
			t.Errorf("the post-repair re-diagnosis does not close with\n%s\ngot\nPortal doctor:\n%s", want, post)
		}
	})
}

func TestDoctorFix_AdvisoryOnlyExitsZero(t *testing.T) {
	requireDropInSlug(t, "gone")
	setPrefsFile(t, `{"theme":"gone"}`)

	deps := healthyDoctorDeps(t)
	deps.ThemesDir = themesDirWith(t, map[string][]byte{
		"a-missing.theme": sourceMissingTokens(t, "text.primary"),
		"b-colour.theme":  sourceBadColours(t, themeOverride{"canvas", "blue"}),
	})

	outBuf, _, err := runDoctorWith(t, deps, "--fix")
	if err != nil {
		t.Fatalf("Execute err = %v; want nil — a run whose only findings are advisories is a healthy run", err)
	}

	out := outBuf.String()
	if strings.Contains(out, "Pruned ") {
		t.Errorf("--fix printed a prune breadcrumb over a theme-only finding; doctor can prune a stale hook entry, it cannot repair someone's colours:\n%s", out)
	}
	requireBothReportsEndWith(t, out, ""+
		"  ⚠ theme a-missing: missing tokens — missing text.primary\n"+
		"  ⚠ theme b-colour: bad colour — canvas = blue\n"+
		"  ⚠ theme gone does not resolve: not found\n"+
		"  10 checks passed · 3 advisories")
}

// The `appearance` key makes this a file the migrating loader would genuinely
// rewrite. Do not drop syncPersistTranslation as redundant: TestMain no-ops
// persistTranslation package-wide and the migrating loader writes entirely
// inside that seam, so without it even a doctor rewired to the migrating read
// would leave this fixture byte-identical.
func fixThemeFixture(t *testing.T) (root, prefsPath string) {
	t.Helper()

	syncPersistTranslation(t)
	requireBuiltinSlug(t, "nord")
	root = t.TempDir()
	themesDir := themesDirIn(t, root, map[string][]byte{
		"a-missing.theme": sourceMissingTokens(t, "text.primary"),
		"b-colour.theme":  sourceBadColours(t, themeOverride{"canvas", "blue"}),
		"c-valid.theme":   validThemeSource(t),
		"D_bad.theme":     validThemeSource(t),
		"nord.theme":      validThemeSource(t),
		"notes.txt":       []byte("not a theme file\n"),
	})
	t.Setenv("PORTAL_THEMES_DIR", themesDir)

	prefsPath = filepath.Join(root, "prefs.json")
	if err := os.WriteFile(prefsPath, []byte(`{"session_list_mode":"by-tag","appearance":"light"}`), 0o600); err != nil {
		t.Fatalf("seed prefs.json: %v", err)
	}
	t.Setenv("PORTAL_PREFS_FILE", prefsPath)
	return root, prefsPath
}

func TestDoctorFix_ThemeStateUntouched(t *testing.T) {
	root, prefsPath := fixThemeFixture(t)
	before := treeFingerprint(t, root)
	if len(before) < 3 {
		t.Fatalf("snapshot of the config root holds %d entries: %v", len(before), slices.Sorted(maps.Keys(before)))
	}
	prefsBefore, err := os.ReadFile(prefsPath)
	if err != nil {
		t.Fatalf("read prefs.json: %v", err)
	}

	outBuf, _, err := runDoctorWith(t, healthyDoctorDeps(t), "--fix")
	if err != nil {
		t.Fatalf("Execute err = %v; broken drop-ins must never drive the exit code", err)
	}
	if n := strings.Count(outBuf.String(), "⚠"); n != 8 {
		t.Fatalf("report carries %d advisory lines over the two renders, want 8 (4 per pass) — the untouched assertions would be about a scan that found nothing:\n%s", n, outBuf.String())
	}

	assertTreeUnchanged(t, root, before, "--fix changed the theme config tree")

	prefsAfter, err := os.ReadFile(prefsPath)
	if err != nil {
		t.Fatalf("re-read prefs.json: %v", err)
	}
	if !bytes.Equal(prefsBefore, prefsAfter) {
		t.Errorf("--fix rewrote prefs.json\nbefore: %s\nafter:  %s", prefsBefore, prefsAfter)
	}

	t.Run("it writes no theme_migrated marker", func(t *testing.T) {
		// The byte-compare above covers this only incidentally: an idempotent
		// prefs write would satisfy it while still recording the marker.
		if translateAppearance("light") == "" {
			t.Fatal("the fixture's appearance value translates to nothing, so a --fix that ran the migration would write no marker anyway and this assertion would be vacuous")
		}
		var decoded map[string]any
		if err := json.Unmarshal(prefsAfter, &decoded); err != nil {
			t.Fatalf("decode prefs.json: %v", err)
		}
		if got, ok := decoded["theme_migrated"]; ok {
			t.Errorf("prefs.json carries theme_migrated = %#v after --fix; doctor reads prefs through the NON-migrating variant on every path", got)
		}
	})

	t.Run("it creates no themes directory when there is none", func(t *testing.T) {
		absent := filepath.Join(t.TempDir(), "themes")
		t.Setenv("PORTAL_THEMES_DIR", absent)

		if _, _, err := runDoctorWith(t, healthyDoctorDeps(t), "--fix"); err != nil {
			t.Fatalf("Execute err = %v; an absent themes directory is silent, not unhealthy", err)
		}
		if _, err := os.Stat(absent); !os.IsNotExist(err) {
			t.Errorf("os.Stat(%s) = %v; --fix must never create the themes directory", absent, err)
		}
	})
}

func TestDoctorFix_ScanReRunForSecondPass(t *testing.T) {
	t.Run("a second call reflects the directory's current state", func(t *testing.T) {
		dir := themesDirWith(t, map[string][]byte{
			"a-missing.theme": sourceMissingTokens(t, "text.primary"),
			"b-colour.theme":  sourceBadColours(t, themeOverride{"canvas", "blue"}),
		})
		deps := &DoctorDeps{ThemesDir: dir}

		// collectThemeAdvisories, not the union beneath it: it is what both
		// render sites call and what carries the freshness claim.
		missing := advisory{line: "⚠ theme a-missing: missing tokens — missing text.primary"}

		got := collectThemeAdvisories(deps)
		if want := []advisory{missing, {line: "⚠ theme b-colour: bad colour — canvas = blue"}}; !slices.Equal(got, want) {
			t.Fatalf("collected block = %+v; want %+v", got, want)
		}

		if err := os.Remove(filepath.Join(dir, "b-colour.theme")); err != nil {
			t.Fatalf("remove the broken drop-in: %v", err)
		}

		got = collectThemeAdvisories(deps)
		if want := []advisory{missing}; !slices.Equal(got, want) {
			t.Errorf("the second collection = %+v; want %+v — it must re-read the directory rather than replay the first", got, want)
		}
	})

	t.Run("both renders are handed a freshly collected block", func(t *testing.T) {
		source := parsePackageFilesByName(t)["doctor.go"]
		if source == nil {
			t.Fatal("the cmd package declares no doctor.go — the guard has nothing to scan")
		}

		var renders []*ast.CallExpr
		ast.Inspect(source, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == "renderDoctorReport" {
				renders = append(renders, call)
			}
			return true
		})

		if len(renders) != 2 {
			t.Fatalf("doctor.go holds %d renderDoctorReport calls, want 2 (the plain/initial render and the post-repair one)", len(renders))
		}
		for i, call := range renders {
			if len(call.Args) != 3 {
				t.Fatalf("renderDoctorReport call %d takes %d arguments, want 3 — the guard is reading the wrong slot", i, len(call.Args))
			}
			arg, ok := call.Args[2].(*ast.CallExpr)
			if !ok {
				t.Errorf("renderDoctorReport call %d is handed %T as its advisories; want a collectThemeAdvisories(deps) call — a reused variable would make that render a replay", i, call.Args[2])
				continue
			}
			ident, ok := arg.Fun.(*ast.Ident)
			if !ok || ident.Name != "collectThemeAdvisories" {
				t.Errorf("renderDoctorReport call %d is handed a call to something other than collectThemeAdvisories", i)
			}
		}
	})
}

func TestDoctorFix_EmitsNoThemeRecords(t *testing.T) {
	requireDropInSlug(t, "gone")
	requireBuiltinSlug(t, "nord")
	setPrefsFile(t, `{"theme":"gone"}`)
	deps := healthyDoctorDeps(t)
	deps.ThemesDir = themesDirWith(t, map[string][]byte{
		"a-missing.theme": sourceMissingTokens(t, "text.primary"),
		"b-colour.theme":  sourceBadColours(t, themeOverride{"canvas", "blue"}),
		"c-syntax.theme":  sourceDuplicateKeyAt(t, 12, "text.primary"),
		"D_bad.theme":     validThemeSource(t),
		"nord.theme":      validThemeSource(t),
	})

	assertNoThemeRecords(t, func() {
		outBuf, _, err := runDoctorWith(t, deps, "--fix")
		if err != nil {
			t.Fatalf("Execute err = %v; broken drop-ins must never drive the exit code", err)
		}
		if n := strings.Count(outBuf.String(), "⚠"); n != 12 {
			t.Fatalf("report carries %d advisory lines over the two renders, want 12 (6 per pass) — the zero-record assertion would be about the wrong run:\n%s", n, outBuf.String())
		}
	})
}

func TestDoctorFix_RemainsBootstrapExempt(t *testing.T) {
	if !bootstrapExempt["doctor"] {
		t.Error("bootstrapExempt[\"doctor\"] = false; want true (Bootstrap Exemption)")
	}

	t.Run("a --fix run with a full theme surface runs no bootstrap", func(t *testing.T) {
		resetBootstrapOnce(t)
		runner := &recordingRunner{started: true}
		withBootstrapDeps(t, BootstrapDeps{Orchestrator: runner})

		requireDropInSlug(t, "gone")
		setPrefsFile(t, `{"theme":"gone"}`)
		deps := healthyDoctorDeps(t)
		deps.ThemesDir = themesDirWith(t, map[string][]byte{
			"a-missing.theme": sourceMissingTokens(t, "text.primary"),
		})

		outBuf, _, err := runDoctorWith(t, deps, "--fix")
		if err != nil {
			t.Fatalf("Execute err = %v; broken drop-ins must never drive the exit code", err)
		}
		if runner.calls != 0 {
			t.Errorf("the bootstrap orchestrator ran %d times on `doctor --fix`; doctor starts no server, ensures no saver and runs no restore", runner.calls)
		}
		requireBothReportsEndWith(t, outBuf.String(), ""+
			"  ⚠ theme a-missing: missing tokens — missing text.primary\n"+
			"  ⚠ theme gone does not resolve: not found\n"+
			"  10 checks passed · 2 advisories")
	})

	t.Run("the orchestrator seam runs for a non-exempt command", func(t *testing.T) {
		resetBootstrapOnce(t)
		runner := &recordingRunner{started: true}
		withBootstrapDeps(t, BootstrapDeps{Orchestrator: runner})

		probe := &cobra.Command{Use: "doctorfixprobe", RunE: func(*cobra.Command, []string) error { return nil }}
		rootCmd.AddCommand(probe)
		t.Cleanup(func() { rootCmd.RemoveCommand(probe) })

		resetRootCmd()
		rootCmd.SetArgs([]string{"doctorfixprobe"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("probe Execute err = %v", err)
		}
		if runner.calls != 1 {
			t.Fatalf("the bootstrap orchestrator ran %d times for a non-exempt command, want 1 — the exemption assertion above would be vacuous", runner.calls)
		}
	})
}

func TestDoctorFix_ExistingRepairsUnchanged(t *testing.T) {
	t.Run("the project prune and the log sweep still run", func(t *testing.T) {
		liveDir := t.TempDir()
		goneDir := filepath.Join(t.TempDir(), "gone")
		projectStore, projectsPath := seedProjectsJSON(t, liveDir, goneDir)
		deps := staleDeps(t, projectStore)
		deps.ThemesDir = themesDirWith(t, map[string][]byte{
			"a-missing.theme": sourceMissingTokens(t, "text.primary"),
		})
		staleLog := filepath.Join(deps.StateDir, "portal.log.2000-01-01")
		if err := os.WriteFile(staleLog, []byte("old\n"), 0o600); err != nil {
			t.Fatalf("seed stale rotated log: %v", err)
		}

		outBuf, _, err := runDoctorWith(t, deps, "--fix")
		if err != nil {
			t.Fatalf("Execute err = %v; want nil (healthy post-repair)", err)
		}

		out := outBuf.String()
		if after := string(readFileBytes(t, projectsPath)); strings.Contains(after, goneDir) {
			t.Errorf("stale project %q not pruned:\n%s", goneDir, after)
		}
		if !strings.Contains(out, "Pruned stale project: proj1 ("+goneDir+")") {
			t.Errorf("missing pruned-project breadcrumb:\n%s", out)
		}
		if _, statErr := os.Stat(staleLog); !os.IsNotExist(statErr) {
			t.Errorf("stale rotated log not swept (stat err = %v); the log sweep did not run", statErr)
		}

		const advisory = "  ⚠ theme a-missing: missing tokens — missing text.primary\n"
		pre, post := requireTwoReports(t, out)
		if want := advisory + "  9 of 10 checks passed · 1 advisory"; !strings.HasSuffix(pre, want) {
			t.Errorf("the initial diagnosis does not close with\n%s\ngot\nPortal doctor:\n%s", want, pre)
		}
		if want := advisory + "  10 checks passed · 1 advisory"; !strings.HasSuffix(post, want) {
			t.Errorf("the post-repair re-diagnosis does not close with\n%s\ngot\nPortal doctor:\n%s", want, post)
		}
	})
}
