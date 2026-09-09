package tmux_test

import (
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/sourceguardtest"
)

const (
	tmuxImportPath = "github.com/leeovery/portal/internal/tmux"
	cmdImportPath  = "github.com/leeovery/portal/cmd"
)

// targetComposingPackages returns the directory of every package that can
// compose a tmux `-t` argument, sorted so a scan's findings come out in a
// deterministic order.
func targetComposingPackages(t *testing.T) []string {
	t.Helper()
	return slices.Sorted(maps.Values(targetComposingPackageDirs(t)))
}

// targetComposingPackageDirs maps the import path of every package that can
// compose a tmux `-t` argument to its directory: those importing internal/tmux,
// plus internal/tmux itself, which holds the vocabulary. The set is derived from
// the import graph rather than listed, so a package that starts addressing tmux
// joins the scan by construction instead of by someone remembering to add it.
func targetComposingPackageDirs(t *testing.T) map[string]string {
	t.Helper()

	listing, err := modulePackageListing()
	if err != nil {
		t.Fatal(err)
	}
	dirs, err := importersOfTmux(listing)
	if err != nil {
		t.Fatal(err)
	}
	return dirs
}

// modulePackageListing reads the module's packages once per test binary: the
// listing is the same for every caller, and `go list` over the whole module is
// the expensive part of this guard.
//
// The scan parses every non-test source in a package whatever tag gates it, so
// the importer set is resolved with the integration tag in force too: a package
// reaching tmux only from a tagged file composes targets the scan would
// otherwise read from a directory it never visited.
var modulePackageListing = sync.OnceValues(func() (string, error) {
	out, err := exec.Command("go", "list",
		"-tags", "integration",
		"-f", "{{.ImportPath}}\t{{.Dir}}\t{{join .Imports \" \"}}",
		"github.com/leeovery/portal/...",
	).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("go list the module's packages: %w\n%s", err, out)
	}
	return string(out), nil
})

// importersOfTmux reads a `go list` listing of "import path, directory, imports"
// lines into the import path → directory map of the packages addressing tmux.
// Resolving none is an error rather than an empty map: a scan that has stopped
// looking would otherwise report a clean repository forever, which is the
// failure the derivation exists to prevent.
func importersOfTmux(listing string) (map[string]string, error) {
	dirs := map[string]string{}
	for line := range strings.SplitSeq(strings.TrimSpace(listing), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) != 3 {
			continue
		}
		importPath, dir, imports := fields[0], fields[1], strings.Fields(fields[2])
		if importPath == tmuxImportPath || slices.Contains(imports, tmuxImportPath) {
			dirs[importPath] = dir
		}
	}
	if len(dirs) == 0 {
		return nil, errors.New("no package importing internal/tmux was found, so the guard would scan nothing")
	}
	return dirs, nil
}

// exactTargetHelpers is the whole vocabulary a composed target may be drawn
// from. Their bare siblings (PaneTarget, windowTarget) are absent on purpose:
// tmux prefix-matches an unpinned session name, so a target built from one
// reaches a live stranger whose name merely starts the same way.
var exactTargetHelpers = map[string]bool{
	"SessionTargetExact": true,
	"CoordTargetExact":   true,
	"windowTargetExact":  true,
	"PaneTargetExact":    true,
	"PaneIDTarget":       true,
}

// targetTypeNames are the ways the composed-target type is spelled in a
// signature: bare inside internal/tmux, qualified everywhere else. A parameter
// declared with it holds a target the compiler has already vouched for, so the
// scan takes it at its word — which is what lets a parameter be named for what
// it holds rather than for what a source scan will recognise.
var targetTypeNames = map[string]bool{
	"Target":      true,
	"tmux.Target": true,
}

// TestTmuxTargetsAreComposedThroughTheExactnessVocabulary is the standing guard
// over the residue the Target type cannot express. The type refuses a computed
// string where a target is required, and that is the whole of what it refuses:
// an untyped constant converts to a Target implicitly, an explicit conversion
// converts anything at all, and an argv is a []string all the way down, so a
// literal "-t" followed by a concatenation never meets the type in the first
// place. Both rules below are read against that residue.
func TestTmuxTargetsAreComposedThroughTheExactnessVocabulary(t *testing.T) {
	findings := scanBareTargets(t, targetComposingPackages(t))
	for _, finding := range findings {
		t.Errorf("%s: %s", finding.pos, finding.detail)
	}
}

func TestBareTargetGuard_ScansEveryPackageAddressingTmux(t *testing.T) {
	t.Run("it derives the scanned package set from the packages importing internal/tmux", func(t *testing.T) {
		byPath := targetComposingPackageDirs(t)

		for _, want := range []string{cmdImportPath, tmuxImportPath, "github.com/leeovery/portal/internal/restore"} {
			if _, ok := byPath[want]; !ok {
				t.Errorf("derived package set %v holds no entry for %s", slices.Sorted(maps.Keys(byPath)), want)
			}
		}
	})

	t.Run("it flags a bare target composed in cmd", func(t *testing.T) {
		dirs := targetComposingPackages(t)
		cmdDir := targetComposingPackageDirs(t)[cmdImportPath]
		probed := stagePackageWithProbe(t, cmdDir, `package cmd

func probeRun(args ...string) {}

func probeAddressSession(name string) { probeRun("has-session", "-t", name) }
`)
		cmdAt := slices.Index(dirs, cmdDir)
		if cmdAt < 0 {
			t.Fatalf("the derived package set %v holds no cmd directory to stage a probe over", dirs)
		}
		dirs[cmdAt] = probed

		findings := scanBareTargets(t, dirs)
		if len(findings) != 1 {
			t.Fatalf("scan of the derived set with one bare target staged in cmd found %d findings, want 1: %v",
				len(findings), findings)
		}
		if !strings.Contains(findings[0].pos, probeFileName) {
			t.Errorf("finding %v does not name the staged probe", findings[0])
		}
	})
}

const probeFileName = "zz_probe.go"

// stagePackageWithProbe copies src's production sources into a temp directory
// and adds probe alongside them, so the scan reads the package's real sources
// with one authored offender among them. The copy is what keeps the repository
// untouched: nothing is ever written into src.
func stagePackageWithProbe(t *testing.T, src, probe string) string {
	t.Helper()
	paths, err := sourceguardtest.PackageGoFiles(src, false)
	if err != nil {
		t.Fatalf("enumerate %s: %v", src, err)
	}
	dir := t.TempDir()
	for _, path := range paths {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if err := os.WriteFile(filepath.Join(dir, filepath.Base(path)), body, 0o600); err != nil {
			t.Fatalf("stage %s: %v", path, err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, probeFileName), []byte(probe), 0o600); err != nil {
		t.Fatalf("write probe: %v", err)
	}
	return dir
}

func TestBareTargetGuard_FlagsAPackageComposingABareTarget(t *testing.T) {
	t.Run("it fails a package composing a bare -t target", func(t *testing.T) {
		dir := writeFixturePackage(t, `package fixture

import "fmt"

func run(args ...string) {}

type Target string

func SelectPane(target Target) { run("select-pane", "-t", string(target)) }

var stagedArgs = []string{"kill-session", "-t", "some-name"}

func addressSession(name string) {
	bare := name + "-2"
	run("has-session", "-t", bare)
	run("kill-session", "-t", fmt.Sprintf("%s:", name))
	args := []string{"new-window", "-t", name + ":"}
	run(args...)
}
`)

		findings := scanBareTargets(t, []string{dir})
		if len(findings) != 4 {
			t.Fatalf("scan found %d bare targets, want 4: %v", len(findings), findings)
		}
	})

	t.Run("it flags a target split across the end of an argv", func(t *testing.T) {
		dir := writeFixturePackage(t, `package fixture

func run(args ...string) {}

type Target string

func SelectPane(target Target) { run("select-pane", "-t", string(target)) }

func addressSession(name string) {
	args := []string{"kill-session", "-t"}
	run(append(args, name)...)
}
`)

		findings := scanBareTargets(t, []string{dir})
		if len(findings) != 1 {
			t.Fatalf("scan of an argv ending in \"-t\" found %d findings, want 1: %v", len(findings), findings)
		}
		if findings[0].detail != `"-t" ends its argv, so its target is composed apart from the flag` {
			t.Errorf("finding %v does not report the target as composed apart from its flag", findings[0])
		}
	})

	t.Run("it flags a hand-composed target laundered through a local", func(t *testing.T) {
		dir := writeFixturePackage(t, `package fixture

func run(args ...string) {}

type Target string

func SelectPane(target Target) { run("select-pane", "-t", string(target)) }

func addressSession(name string) {
	target := name + ":"
	run("has-session", "-t", target)
}
`)

		findings := scanBareTargets(t, []string{dir})
		if len(findings) != 1 {
			t.Fatalf("scan of a hand-composed target laundered through a local found %d findings, want 1: %v",
				len(findings), findings)
		}
		if findings[0].detail != `the target after "-t" is composed by hand — `+routeItThrough {
			t.Errorf("finding %v does not report the target as composed by hand", findings[0])
		}
	})

	// The vocabulary's own package composes its targets out of string pieces and
	// hands them to the commander as an argv, so nothing there passes through a
	// Target parameter for the type to judge.
	t.Run("it fails the reduced source guard when a target is concatenated inside the tmux package", func(t *testing.T) {
		dir := writeFixturePackage(t, `package tmux

type Target string

func run(args ...string) {}

func CoordTargetExact(session string) Target { return Target("=" + session + ":") }

func SelectPane(target Target) { run("select-pane", "-t", string(target)) }

func addressSession(session string) {
	run("kill-session", "-t", "="+session+":")
}
`)

		findings := scanBareTargets(t, []string{dir})
		if len(findings) != 1 {
			t.Fatalf("scan of a concatenated target inside the tmux package found %d findings, want 1: %v",
				len(findings), findings)
		}
	})

	// The two shapes the type admits: a constant is untyped until it is used, and
	// a conversion converts whatever it is given.
	t.Run("it flags an untyped constant handed to a target parameter", func(t *testing.T) {
		dir := writeFixturePackage(t, `package fixture

type Target string

const SaverName = "_portal-saver"

func CoordTargetExact(session string) Target { return Target("=" + session + ":") }

func RespawnPane(target Target, command string) {}

func armSaver() { RespawnPane(SaverName, "portal state daemon") }
`)

		findings := scanBareTargets(t, []string{dir})
		if len(findings) != 1 {
			t.Fatalf("scan of an untyped constant handed to a target parameter found %d findings, want 1: %v",
				len(findings), findings)
		}
		if findings[0].detail != "the target passed to RespawnPane is composed by hand — "+routeItThrough {
			t.Errorf("finding %v does not name the parameter the constant reached", findings[0])
		}
	})

	t.Run("it flags a hand-composed target converted to the target type", func(t *testing.T) {
		dir := writeFixturePackage(t, `package fixture

type Target string

func CoordTargetExact(session string) Target { return Target("=" + session + ":") }

func SetPaneOption(target Target, name, value string) {}

func stamp(name string) { SetPaneOption(Target(name+":"), "@portal-pane-id", "tok") }
`)

		findings := scanBareTargets(t, []string{dir})
		if len(findings) != 1 {
			t.Fatalf("scan of a conversion handed to a target parameter found %d findings, want 1: %v",
				len(findings), findings)
		}
		if findings[0].detail != "the target passed to SetPaneOption is composed by hand — "+routeItThrough {
			t.Errorf("finding %v does not name the parameter the conversion reached", findings[0])
		}
	})

	// A method and a plain function sharing a name are one entry in the taker
	// map, so their positions are unioned rather than overwritten: reading only
	// the last declaration would drop the other's argument out of the rule.
	t.Run("it flags every position two same-named target takers declare", func(t *testing.T) {
		dir := writeFixturePackage(t, `package fixture

type Target string

type Client struct{}

func CoordTargetExact(session string) Target { return Target("=" + session + ":") }

func (c *Client) stampPaneToken(session string, target Target, token string) {}

func stampPaneToken(paneID Target) {}

func stampTwice(c *Client, name string) {
	c.stampPaneToken(string(CoordTargetExact(name)), Target(name+":"), "tok")
	stampPaneToken(Target(name + ":"))
}
`)

		findings := scanBareTargets(t, []string{dir})
		if len(findings) != 2 {
			t.Fatalf("scan of two same-named target takers found %d findings, want 2 — one at each declaration's position: %v",
				len(findings), findings)
		}
		for _, finding := range findings {
			if finding.detail != "the target passed to stampPaneToken is composed by hand — "+routeItThrough {
				t.Errorf("finding %v does not report an argument of the pooled taker", finding)
			}
		}
	})

	t.Run("it passes a target parameter handed the vocabulary's own output", func(t *testing.T) {
		dir := writeFixturePackage(t, `package fixture

type Target string

func CoordTargetExact(session string) Target { return Target("=" + session + ":") }

func SplitWindow(target Target, cwd string) {}

func splitTwice(name string) {
	SplitWindow(CoordTargetExact(name), "/tmp")
	pinned := CoordTargetExact(name)
	SplitWindow(pinned, "/tmp")
}

func splitThrough(liveTarget Target) { SplitWindow(liveTarget, "/tmp") }
`)

		findings := scanBareTargets(t, []string{dir})
		if len(findings) != 0 {
			t.Errorf("scan flagged %v, want nothing", findings)
		}
	})

	// A multi-valued call hands one right-hand side to several identifiers, so
	// only the target-typed result position may be spent as a target — its
	// siblings are strings like any other, and the position must be read behind
	// unnamed result fields as much as named ones.
	t.Run("it flags the sibling of a multi-valued target result while passing the target itself", func(t *testing.T) {
		const src = `package fixture

type Target string

func run(args ...string) {}

func CoordTargetExact(session string) Target { return Target("=" + session + ":") }

func SelectPane(target Target) { run("select-pane", "-t", string(target)) }

func resolveCurrentPaneKey(name string) (string, Target, error) {
	return "", CoordTargetExact(name), nil
}

func addressSession(name string) {
	key, target, err := resolveCurrentPaneKey(name)
	if err != nil {
		return
	}
	run("has-session", "-t", key)
	run("kill-session", "-t", string(target))
}
`
		dir := writeFixturePackage(t, src)

		findings := scanBareTargets(t, []string{dir})
		if len(findings) != 1 {
			t.Fatalf("scan of a multi-valued target result found %d findings, want 1 (the sibling alone): %v",
				len(findings), findings)
		}
		if findings[0].detail != `the target after "-t" is composed by hand — `+routeItThrough {
			t.Errorf("finding %v does not report the sibling as composed by hand", findings[0])
		}
		wantLine := lineOf(t, src, `run("has-session"`)
		if !strings.Contains(findings[0].pos, fmt.Sprintf("fixture.go:%d:", wantLine)) {
			t.Errorf("finding %v is not the sibling spent at fixture.go:%d", findings[0], wantLine)
		}
	})

	t.Run("it passes a vocabulary target spent as a string on an argv the client does not run", func(t *testing.T) {
		dir := writeFixturePackage(t, `package fixture

type Target string

func run(args ...string) {}

func CoordTargetExact(session string) Target { return Target("=" + session + ":") }

func SelectPane(target Target) { run("select-pane", "-t", string(target)) }

func addressSession(name string) []string {
	pinned := string(CoordTargetExact(name))
	return []string{"tmux", "attach-session", "-t", pinned, "-t", string(CoordTargetExact(name))}
}
`)

		findings := scanBareTargets(t, []string{dir})
		if len(findings) != 0 {
			t.Errorf("scan flagged %v, want nothing", findings)
		}
	})

	t.Run("it passes a target held by a parameter declared as a tmux target, whatever it is named", func(t *testing.T) {
		dir := writeFixturePackage(t, `package fixture

import "github.com/leeovery/portal/internal/tmux"

func run(args ...string) {}

func addressPane(liveTarget tmux.Target) {
	run("respawn-pane", "-k", "-t", string(liveTarget))
}
`)

		findings := scanBareTargets(t, []string{dir})
		if len(findings) != 0 {
			t.Errorf("scan flagged %v, want nothing", findings)
		}
	})

	t.Run("it passes a package composing every target through the vocabulary", func(t *testing.T) {
		dir := writeFixturePackage(t, `package fixture

func run(args ...string) {}

type Target string

func SelectPane(target Target) { run("select-pane", "-t", string(target)) }

func CoordTargetExact(session string) string { return "=" + session + ":" }

func addressSession(name string) {
	run("has-session", "-t", CoordTargetExact(name))
	target := CoordTargetExact(name)
	run("kill-session", "-t", target)
	args := []string{"split-window", "-t", target}
	run(args...)
}
`)

		findings := scanBareTargets(t, []string{dir})
		if len(findings) != 0 {
			t.Errorf("scan flagged %v, want nothing", findings)
		}
	})
}

// The same reasoning one rule up: an import scan resolving no importer is a
// derivation that has stopped looking, not a repository that has stopped
// composing targets.
func TestBareTargetGuard_ErrorsWhenTheImportScanResolvesNothing(t *testing.T) {
	listing := "github.com/leeovery/portal/internal/alias\t/repo/internal/alias\tfmt os\n"

	if _, err := importersOfTmux(listing); err == nil {
		t.Fatal("an import scan resolving no importer of internal/tmux succeeded, want an error")
	}
}

// A guard that stopped finding sources would otherwise report a clean scan
// forever.
func TestBareTargetGuard_FatalsWhenItEnumeratesNoFiles(t *testing.T) {
	rec := &harnesstest.Recorder{}

	rec.Run(func() { scanBareTargets(rec, []string{t.TempDir()}) })

	if len(rec.Fatals) != 1 {
		t.Fatalf("scan of a directory holding no sources reported %d fatals, want 1: %v", len(rec.Fatals), rec.Fatals)
	}
	if !strings.Contains(rec.Fatals[0], "no .go files") {
		t.Errorf("fatal message %q does not say the directory held no sources", rec.Fatals[0])
	}
}

// The same reasoning one rule down: the argument rule is only as wide as the
// function set it derives, so an empty set is a scan that has stopped looking
// rather than a clean one.
func TestBareTargetGuard_FatalsWhenItFindsNoTargetTakingFuncs(t *testing.T) {
	dir := writeFixturePackage(t, `package fixture

func run(args ...string) {}

func addressSession(session string) { run("has-session", session) }
`)

	rec := &harnesstest.Recorder{}

	rec.Run(func() { scanBareTargets(rec, []string{dir}) })

	if len(rec.Fatals) != 1 {
		t.Fatalf("scan of a package declaring no target-taking function reported %d fatals, want 1: %v", len(rec.Fatals), rec.Fatals)
	}
	if !strings.Contains(rec.Fatals[0], "already-composed target") {
		t.Errorf("fatal message %q does not say the function set was empty", rec.Fatals[0])
	}
}

// lineOf returns the 1-based line of the first line in src holding needle, so a
// fixture's expected position is derived from the source it is asserted against
// rather than counted by hand.
func lineOf(t *testing.T, src, needle string) int {
	t.Helper()
	for i, line := range strings.Split(src, "\n") {
		if strings.Contains(line, needle) {
			return i + 1
		}
	}
	t.Fatalf("fixture source holds no line containing %q", needle)
	return 0
}

func writeFixturePackage(t *testing.T, src string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "fixture.go"), []byte(src), 0o600); err != nil {
		t.Fatalf("write fixture package: %v", err)
	}
	return dir
}

type bareTargetFinding struct {
	pos    string
	detail string
}

func (f bareTargetFinding) String() string { return f.pos + ": " + f.detail }

const routeItThrough = "route it through SessionTargetExact, CoordTargetExact, windowTargetExact, PaneTargetExact or PaneIDTarget"

// scanBareTargets reports every place in dirs where a target the exactness
// vocabulary did not produce is spent: composed into an argv after a literal
// "-t", or passed to a parameter declared with the target type. The first rule
// reaches the argv shape the type never sees at all — a client method building
// its own argv, or a tmux command line the client never runs. The second reaches
// the two shapes the type admits into a Target: an untyped constant, and an
// explicit conversion.
func scanBareTargets(t harnesstest.TestingT, dirs []string) []bareTargetFinding {
	t.Helper()

	var sources []sourceguardtest.ParsedSource
	for _, dir := range dirs {
		sources = append(sources, sourceguardtest.ParsePackageSources(t, dir, false)...)
	}
	if len(sources) == 0 {
		return nil
	}

	files := make([]*ast.File, 0, len(sources))
	for _, source := range sources {
		files = append(files, source.File)
	}

	takers := targetTakingFuncs(files)
	if len(takers) == 0 {
		t.Fatalf("no function taking an already-composed target was found, so nothing would be checked at a call site")
		return nil
	}
	producers := targetReturningFuncs(files)

	var findings []bareTargetFinding
	for _, source := range sources {
		findings = append(findings, scanFileForBareTargets(source.Fset, source.File, takers, producers)...)
	}
	return findings
}

// targetReturningFuncs records, per function name, the result positions declared
// with the target type — the mirror of targetTakingFuncs, and how a target
// arriving through a multi-valued call is recognised: there the right-hand side
// is one call for several identifiers, so the position is what says which of
// them holds the target. The declared type is what the position is read from,
// which is narrower than provenance: a declaring body composing its result by
// conversion is a shape no rule here reaches.
func targetReturningFuncs(files []*ast.File) map[string][]int {
	producers := map[string][]int{}
	for _, file := range files {
		for _, decl := range file.Decls {
			fn, isFunc := decl.(*ast.FuncDecl)
			if !isFunc {
				continue
			}
			if positions := targetResultPositions(fn); len(positions) > 0 {
				producers[fn.Name.Name] = unionPositions(producers[fn.Name.Name], positions)
			}
		}
	}
	return producers
}

func targetResultPositions(fn *ast.FuncDecl) []int {
	if fn.Type.Results == nil {
		return nil
	}
	var positions []int
	pos := 0
	for _, field := range fn.Type.Results.List {
		typed := isTargetType(field.Type)
		// An unnamed result occupies its position all the same.
		width := max(len(field.Names), 1)
		for range width {
			if typed {
				positions = append(positions, pos)
			}
			pos++
		}
	}
	return positions
}

// targetTakingFuncs records, per function name, the argument positions declared
// with the target type. Methods and plain functions alike are read, and not only
// the tmux client's: a parameter typed Target holds a composed target wherever it
// is declared, and a wider map only ever checks more call sites.
//
// A call site is matched on the callee's name alone, so two declarations sharing
// a name — a method and a plain function, say — are one entry, and their
// positions are unioned rather than overwritten: keeping only the last read would
// drop the other's positions out of the rule. The pooling is why an unrelated
// function sharing a taker's name has its own arguments checked at those
// positions; give the two operations distinct names rather than widening the
// vocabulary to absorb the report.
func targetTakingFuncs(files []*ast.File) map[string][]int {
	takers := map[string][]int{}
	for _, file := range files {
		for _, decl := range file.Decls {
			fn, isFunc := decl.(*ast.FuncDecl)
			if !isFunc {
				continue
			}
			if positions := targetParamPositions(fn); len(positions) > 0 {
				takers[fn.Name.Name] = unionPositions(takers[fn.Name.Name], positions)
			}
		}
	}
	return takers
}

// unionPositions merges two position sets, deduped and ascending, so a name's
// findings come out in a deterministic order whatever order the declarations
// were read in.
func unionPositions(a, b []int) []int {
	merged := slices.Concat(a, b)
	slices.Sort(merged)
	return slices.Compact(merged)
}

func targetParamPositions(fn *ast.FuncDecl) []int {
	var positions []int
	pos := 0
	for _, field := range fn.Type.Params.List {
		typed := isTargetType(field.Type)
		for range field.Names {
			if typed {
				positions = append(positions, pos)
			}
			pos++
		}
	}
	return positions
}

func scanFileForBareTargets(fset *token.FileSet, file *ast.File, takers, producers map[string][]int) []bareTargetFinding {
	var findings []bareTargetFinding
	check := func(pos token.Pos, call *ast.CallExpr, elems []ast.Expr) {
		bound := map[string]bool{}
		if decl := enclosingDecl(file, pos); decl != nil {
			bound = boundTargets(decl, producers)
		}
		findings = append(findings, bareTargetsIn(fset, elems, bound)...)
		if call != nil {
			findings = append(findings, bareTargetArguments(fset, call, takers, bound)...)
		}
	}

	sourceguardtest.ForEachFuncCall(file, func(_ string, call *ast.CallExpr) bool {
		check(call.Pos(), call, call.Args)
		return true
	})
	// ForEachFuncCall descends into function declarations, and a composed argv
	// slice is not a call in the first place. Both remaining shapes are read
	// here rather than skipped: a guard that declines to look at a node class is
	// the failure it exists to prevent.
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.CompositeLit:
			check(node.Pos(), nil, node.Elts)
		case *ast.CallExpr:
			if enclosingFunc(file, node.Pos()) == nil {
				check(node.Pos(), node, node.Args)
			}
		}
		return true
	})
	return findings
}

// bareTargetsIn reports each "-t" in elems whose following element is neither a
// call to the vocabulary nor an identifier bound to one.
func bareTargetsIn(fset *token.FileSet, elems []ast.Expr, bound map[string]bool) []bareTargetFinding {
	var findings []bareTargetFinding
	for i, elem := range elems {
		if !isStringLit(elem, "-t") {
			continue
		}
		if i+1 == len(elems) {
			findings = append(findings, bareTargetFinding{
				pos:    fset.Position(elem.Pos()).String(),
				detail: `"-t" ends its argv, so its target is composed apart from the flag`,
			})
			continue
		}
		if targetIsExact(elems[i+1], bound) {
			continue
		}
		findings = append(findings, bareTargetFinding{
			pos:    fset.Position(elems[i+1].Pos()).String(),
			detail: `the target after "-t" is composed by hand — ` + routeItThrough,
		})
	}
	return findings
}

// bareTargetArguments reports each argument handed to a target-taking parameter
// that the vocabulary did not produce. The callee is read through CalleeName, so
// a plain function call reaches the rule as much as a method call does.
func bareTargetArguments(fset *token.FileSet, call *ast.CallExpr, takers map[string][]int, bound map[string]bool) []bareTargetFinding {
	callee := sourceguardtest.CalleeName(call)
	if callee == "" {
		return nil
	}
	var findings []bareTargetFinding
	for _, pos := range takers[callee] {
		if pos >= len(call.Args) || targetIsExact(call.Args[pos], bound) {
			continue
		}
		findings = append(findings, bareTargetFinding{
			pos:    fset.Position(call.Args[pos].Pos()).String(),
			detail: "the target passed to " + callee + " is composed by hand — " + routeItThrough,
		})
	}
	return findings
}

// targetIsExact reads through a string conversion first: a Target spent on an
// argv is converted back at that argv, so the conversion is the shape the rule
// meets rather than an exception to it.
func targetIsExact(target ast.Expr, bound map[string]bool) bool {
	target = unwrapStringConversion(target)
	if ident, isIdent := target.(*ast.Ident); isIdent {
		return bound[ident.Name]
	}
	return isExactTargetCall(target)
}

func unwrapStringConversion(expr ast.Expr) ast.Expr {
	call, isCall := expr.(*ast.CallExpr)
	if !isCall || len(call.Args) != 1 {
		return expr
	}
	if ident, isIdent := call.Fun.(*ast.Ident); isIdent && ident.Name == "string" {
		return call.Args[0]
	}
	return expr
}

// boundTargets names the identifiers the declaration may spend as a target:
// those its signatures declare with the target type, and the locals its
// assignments bind (see bindAssignedTargets). A declaration of any kind is read,
// because a command body written as a function literal inside a package-level
// var declares its parameters there rather than in a signature.
func boundTargets(decl ast.Decl, producers map[string][]int) map[string]bool {
	bound := map[string]bool{}
	if fn, isFunc := decl.(*ast.FuncDecl); isFunc {
		bindSignature(bound, fn.Type)
	}
	ast.Inspect(decl, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.FuncLit:
			bindSignature(bound, node.Type)
		case *ast.AssignStmt:
			bindAssignedTargets(bound, node, producers)
		}
		return true
	})
	return bound
}

// A named result carries the target type as a parameter would and is declared in
// the same signature, so it is read the same way.
func bindSignature(bound map[string]bool, sig *ast.FuncType) {
	bindFields(bound, sig.Params)
	bindFields(bound, sig.Results)
}

func bindFields(bound map[string]bool, fields *ast.FieldList) {
	if fields == nil {
		return
	}
	for _, field := range fields.List {
		if !isTargetType(field.Type) {
			continue
		}
		for _, name := range field.Names {
			bound[name.Name] = true
		}
	}
}

func isTargetType(expr ast.Expr) bool {
	switch typ := expr.(type) {
	case *ast.Ident:
		return targetTypeNames[typ.Name]
	case *ast.SelectorExpr:
		pkg, isIdent := typ.X.(*ast.Ident)
		return isIdent && targetTypeNames[pkg.Name+"."+typ.Sel.Name]
	}
	return false
}

// bindAssignedTargets binds the identifiers assigned straight from the
// vocabulary, whatever they are named — the right-hand side says where the
// target came from, so a rename cannot launder a hand-composed one into the set.
// An identifier taking its value from anywhere else is left unbound and reported
// where it is spent.
//
// A multi-valued call has one right-hand side for several identifiers, so there
// the position of a target-typed result is what names the identifier holding a
// target.
func bindAssignedTargets(bound map[string]bool, assign *ast.AssignStmt, producers map[string][]int) {
	if len(assign.Lhs) != len(assign.Rhs) {
		bindMultiValuedTargets(bound, assign, producers)
		return
	}
	for i, lhs := range assign.Lhs {
		ident, isIdent := lhs.(*ast.Ident)
		if !isIdent {
			continue
		}
		if isExactTargetCall(unwrapStringConversion(assign.Rhs[i])) {
			bound[ident.Name] = true
		}
	}
}

func bindMultiValuedTargets(bound map[string]bool, assign *ast.AssignStmt, producers map[string][]int) {
	if len(assign.Rhs) != 1 {
		return
	}
	call, isCall := assign.Rhs[0].(*ast.CallExpr)
	if !isCall {
		return
	}
	for _, pos := range producers[sourceguardtest.CalleeName(call)] {
		if pos >= len(assign.Lhs) {
			continue
		}
		if ident, isIdent := assign.Lhs[pos].(*ast.Ident); isIdent {
			bound[ident.Name] = true
		}
	}
}

func isExactTargetCall(expr ast.Expr) bool {
	call, isCall := expr.(*ast.CallExpr)
	if !isCall {
		return false
	}
	return exactTargetHelpers[sourceguardtest.CalleeName(call)]
}

// enclosingDecl returns the top-level declaration holding pos, whatever kind it
// is; enclosingFunc answers the narrower question of whether a call has already
// been visited by the function walk.
func enclosingDecl(file *ast.File, pos token.Pos) ast.Decl {
	for _, decl := range file.Decls {
		if decl.Pos() <= pos && pos < decl.End() {
			return decl
		}
	}
	return nil
}

func enclosingFunc(file *ast.File, pos token.Pos) *ast.FuncDecl {
	for _, decl := range file.Decls {
		fn, isFunc := decl.(*ast.FuncDecl)
		if isFunc && fn.Pos() <= pos && pos < fn.End() {
			return fn
		}
	}
	return nil
}

func isStringLit(expr ast.Expr, want string) bool {
	lit, isLit := expr.(*ast.BasicLit)
	return isLit && lit.Kind == token.STRING && lit.Value == `"`+want+`"`
}
