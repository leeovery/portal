package zmxtest_test

import (
	"fmt"
	"go/ast"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/sourceguardtest"
)

// weeklyCheck is the workflow that runs the zmx-backed tests against zmx's
// latest release and main.
const weeklyCheck = ".github/workflows/zmx-check.yml"

// A zmx release can break any test that runs a real zmx, so the weekly check
// runs every one: each test function reaching zmxtest.New, directly or through
// helpers of its own package, sits in a package the workflow tests, and
// matches its -run pattern when it gives one.
func TestEveryTestThatRunsZmxIsInTheWeeklyCheck(t *testing.T) {
	root := sourceguardtest.ProjectRoot(t)
	workflow, err := os.ReadFile(filepath.Join(root, weeklyCheck))
	if err != nil {
		t.Fatalf("read %s: %v", weeklyCheck, err)
	}
	runs, err := parseCheckRuns(string(workflow))
	if err != nil {
		t.Fatalf("%s: %v", weeklyCheck, err)
	}

	for _, test := range uncheckedZmxTests(t, runs, sourceguardtest.Rooted(root)) {
		t.Errorf("%s runs zmx, but %s never runs it", test, weeklyCheck)
	}
}

func TestWeeklyCheckGuard_Rule(t *testing.T) {
	const helper = "package p\n\nfunc world(t *testing.T) { zmxtest.New(t) }\n"
	runs := []checkRun{
		{packages: []string{"./whole/..."}},
		{packages: []string{"./cmd"}, run: regexp.MustCompile(`AgainstZmx$`)},
	}

	for _, tc := range []struct {
		name  string
		files map[string]string
		want  []string
	}{
		{
			name: "a test in a package the workflow runs whole is checked, in a subpackage too",
			files: map[string]string{
				"whole/a_test.go":     "package p\n\nfunc TestA(t *testing.T) { zmxtest.New(t) }\n",
				"whole/sub/b_test.go": "package p\n\nfunc TestB(t *testing.T) { zmxtest.New(t) }\n",
			},
		},
		{
			name: "a test in a package the workflow never runs is reported",
			files: map[string]string{
				"other/a_test.go": "package p\n\nfunc TestA(t *testing.T) { zmxtest.New(t) }\n",
			},
			want: []string{"other.TestA"},
		},
		{
			name: "a test reaching zmx through its package's helpers must match the run pattern",
			files: map[string]string{
				"cmd/helper_test.go": helper,
				"cmd/a_test.go": "package p\n\nfunc TestOpenAgainstZmx(t *testing.T) { world(t) }\n\n" +
					"func TestOpenBurst(t *testing.T) { t.Run(\"x\", func(t *testing.T) { (&w{}).start(t) }) }\n\n" +
					"type w struct{}\n\nfunc (*w) start(t *testing.T) { world(t) }\n\n" +
					"func TestUnit(t *testing.T) {}\n",
			},
			want: []string{"cmd.TestOpenBurst"},
		},
		{
			name: "zmxtest's own tests call New unqualified",
			files: map[string]string{
				"internal/zmxtest/a_test.go": "package zmxtest\n\nfunc TestTeardown(t *testing.T) { New(t) }\n",
				"cmd/b_test.go":              "package cmd\n\nfunc TestNew(t *testing.T) { New(t) }\n\nfunc New(*testing.T) {}\n",
			},
			want: []string{"internal/zmxtest.TestTeardown"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := stageTree(t, tc.files)

			if got := uncheckedZmxTests(t, runs, sourceguardtest.Rooted(root)); !slices.Equal(got, tc.want) {
				t.Errorf("unchecked = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestWeeklyCheckGuard_FatalsWhenNoTestRunsZmx(t *testing.T) {
	root := stageTree(t, map[string]string{"cmd/a_test.go": "package cmd\n\nfunc TestUnit(t *testing.T) {}\n"})
	recorder := &harnesstest.Recorder{}
	recorder.Run(func() {
		uncheckedZmxTests(recorder, nil, sourceguardtest.Rooted(root))
	})

	if len(recorder.Fatals) != 1 {
		t.Fatalf("a tree with no test running zmx reported %d fatals, want 1: %v", len(recorder.Fatals), recorder.Fatals)
	}
}

func TestParseCheckRuns(t *testing.T) {
	t.Run("it reads each go test line's packages and -run pattern", func(t *testing.T) {
		runs, err := parseCheckRuns(`
      - name: Test
        run: |
          go test -tags integration -p 1 -timeout 30m ./internal/backend/zmx/... ./internal/spawn/...
          go test -tags integration -p 1 -timeout 30m -run 'AgainstZmx$' ./cmd
`)
		if err != nil {
			t.Fatalf("parseCheckRuns: %v", err)
		}
		if len(runs) != 2 ||
			!slices.Equal(runs[0].packages, []string{"./internal/backend/zmx/...", "./internal/spawn/..."}) || runs[0].run != nil ||
			!slices.Equal(runs[1].packages, []string{"./cmd"}) || runs[1].run == nil || runs[1].run.String() != "AgainstZmx$" {
			t.Errorf("runs = %+v, want the two lines' packages, the second's pattern", runs)
		}
	})

	for name, workflow := range map[string]string{
		"no go test line":                "run: make test\n",
		"a go test line with no package": "go test -tags integration\n",
		"a pattern naming a subtest":     "go test -run 'A/B' ./cmd\n",
		"a pattern that doesn't compile": "go test -run '(' ./cmd\n",
	} {
		t.Run("it refuses "+name, func(t *testing.T) {
			if runs, err := parseCheckRuns(workflow); err == nil {
				t.Errorf("parseCheckRuns = %+v, want an error", runs)
			}
		})
	}
}

// checkRun is one go test line of the workflow: the packages it names, as
// ./ patterns, and its -run pattern, nil for none.
type checkRun struct {
	packages []string
	run      *regexp.Regexp
}

// runs reports whether the line runs test, in the package at dir.
func (c checkRun) runs(dir, test string) bool {
	return (c.run == nil || c.run.MatchString(test)) && slices.ContainsFunc(c.packages, func(pattern string) bool {
		pattern = strings.TrimPrefix(pattern, "./")
		if tree, ok := strings.CutSuffix(pattern, "/..."); ok {
			return dir == tree || strings.HasPrefix(dir, tree+"/")
		}
		return dir == pattern
	})
}

// parseCheckRuns reads every go test line of workflow. A workflow with none,
// a line naming no package, and a -run pattern the guard can't judge are
// errors: the guard would check against nothing.
func parseCheckRuns(workflow string) ([]checkRun, error) {
	var runs []checkRun
	for line := range strings.SplitSeq(workflow, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != "go" || fields[1] != "test" {
			continue
		}
		var c checkRun
		for i := 2; i < len(fields); i++ {
			switch {
			case fields[i] == "-run" && i+1 < len(fields):
				i++
				pattern := strings.Trim(fields[i], `'"`)
				if strings.Contains(pattern, "/") {
					return nil, fmt.Errorf("-run %q names subtests, which the guard doesn't judge", pattern)
				}
				run, err := regexp.Compile(pattern)
				if err != nil {
					return nil, fmt.Errorf("-run %q: %w", pattern, err)
				}
				c.run = run
			case strings.HasPrefix(fields[i], "./"):
				c.packages = append(c.packages, fields[i])
			}
		}
		if len(c.packages) == 0 {
			return nil, fmt.Errorf("%q names no package", strings.TrimSpace(line))
		}
		runs = append(runs, c)
	}
	if len(runs) == 0 {
		return nil, fmt.Errorf("no go test line")
	}
	return runs, nil
}

// uncheckedZmxTests is each test function, as <package dir>.<name>, that
// reaches zmxtest.New and that no run of runs runs, sorted. A test reaches it
// through any function of its package's test files that calls it, matched by
// name, which can take in a test that doesn't but never misses one that does.
// A tree in which no test reaches it is fatal: the guard would pass over
// nothing.
func uncheckedZmxTests(t harnesstest.TestingT, runs []checkRun, opts ...sourceguardtest.ScanOption) []string {
	t.Helper()
	_, sources := sourceguardtest.RepoSources(t, sourceguardtest.TestSources, opts...)

	byDir := map[string][]sourceguardtest.ParsedSource{}
	for _, source := range sources {
		dir := path.Dir(filepath.ToSlash(source.Path))
		byDir[dir] = append(byDir[dir], source)
	}
	var unchecked []string
	found := false
	for dir, files := range byDir {
		for _, test := range testsRunningZmx(files) {
			found = true
			if !slices.ContainsFunc(runs, func(c checkRun) bool { return c.runs(dir, test) }) {
				unchecked = append(unchecked, dir+"."+test)
			}
		}
	}
	if !found {
		t.Fatalf("no test reaches zmxtest.New, so the guard judged nothing")
		return nil
	}
	slices.Sort(unchecked)
	return unchecked
}

// testsRunningZmx is the test functions of one package's test files that
// reach zmxtest.New.
func testsRunningZmx(files []sourceguardtest.ParsedSource) []string {
	calls := map[string][]string{}
	reaches := map[string]bool{}
	var tests []string
	for _, source := range files {
		for _, decl := range source.File.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			name := fn.Name.Name
			if fn.Recv == nil && strings.HasPrefix(name, "Test") {
				tests = append(tests, name)
			}
			ast.Inspect(fn, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				if callsNew(source.File, call) {
					reaches[name] = true
				} else if callee := sourceguardtest.CalleeName(call); callee != "" {
					calls[name] = append(calls[name], callee)
				}
				return true
			})
		}
	}
	for grew := true; grew; {
		grew = false
		for fn, callees := range calls {
			if !reaches[fn] && slices.ContainsFunc(callees, func(c string) bool { return reaches[c] }) {
				reaches[fn], grew = true, true
			}
		}
	}
	return slices.DeleteFunc(tests, func(test string) bool { return !reaches[test] })
}

// callsNew reports whether call is zmxtest.New: qualified, or bare inside
// zmxtest's own test package.
func callsNew(file *ast.File, call *ast.CallExpr) bool {
	switch fun := call.Fun.(type) {
	case *ast.SelectorExpr:
		pkg, ok := fun.X.(*ast.Ident)
		return ok && pkg.Name == "zmxtest" && fun.Sel.Name == "New"
	case *ast.Ident:
		return file.Name.Name == "zmxtest" && fun.Name == "New"
	}
	return false
}

func stageTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, body := range files {
		file := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatalf("stage %s: %v", name, err)
		}
		if err := os.WriteFile(file, []byte(body), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return root
}
