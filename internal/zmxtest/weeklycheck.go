package zmxtest

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/portalbintest"
)

// weeklyCheck is the workflow that runs every test that runs a real zmx
// against zmx's latest release and its main branch. A zmx release can break
// any of them, so Binary, which every real zmx a test runs comes from, fails a
// test the workflow doesn't run.
const weeklyCheck = ".github/workflows/zmx-check.yml"

// checkRun is one go test line of the weekly check: the packages it names, as
// ./ patterns, and its -run pattern, nil for none.
type checkRun struct {
	line     string
	packages []string
	run      *regexp.Regexp
}

// weeklyRuns is the weekly check's go test lines, read once per process.
var weeklyRuns = sync.OnceValues(func() ([]checkRun, error) {
	root, err := portalbintest.ProjectRoot()
	if err != nil {
		return nil, err
	}
	workflow, err := os.ReadFile(filepath.Join(root, weeklyCheck))
	if err != nil {
		return nil, err
	}
	return parseCheckRuns(string(workflow))
})

// requireWeeklyCheck fails t unless the weekly check runs it: its package,
// the directory go test runs its binary in, and its top-level test.
func requireWeeklyCheck(t *testing.T) {
	t.Helper()
	runs, err := weeklyRuns()
	if err != nil {
		t.Fatalf("zmxtest: can't tell whether %s runs this test: %v", weeklyCheck, err)
	}
	pkg, err := packageUnderTest()
	if err != nil {
		t.Fatalf("zmxtest: can't tell which package this test is in: %v", err)
	}
	test, _, _ := strings.Cut(t.Name(), "/")
	if err := covered(runs, pkg, test); err != nil {
		t.Fatalf("zmxtest: %v", err)
	}
}

// packageUnderTest is the test binary's directory relative to the module
// root, as a slash path: go test runs each package's binary in its own
// directory.
func packageUnderTest() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	root, err := portalbintest.ProjectRoot()
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, wd)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(rel), nil
}

// covered is why no run of runs runs test in pkg, a slash path relative to
// the module root, naming the line it needs; nil when one does.
func covered(runs []checkRun, pkg, test string) error {
	var unmatched []checkRun
	for _, c := range runs {
		if !c.names(pkg) {
			continue
		}
		if c.run == nil || c.run.MatchString(test) {
			return nil
		}
		unmatched = append(unmatched, c)
	}
	if len(unmatched) > 0 {
		return fmt.Errorf("%s.%s runs a real zmx, but %s runs %s only under -run '%s': name the test to match it (%s)",
			pkg, test, weeklyCheck, pkg, unmatched[0].run, unmatched[0].line)
	}
	return fmt.Errorf("%s.%s runs a real zmx, but %s never runs %s: add the line go test -tags integration -p 1 -timeout 30m ./%s",
		pkg, test, weeklyCheck, pkg, pkg)
}

// names reports whether c's packages take in pkg.
func (c checkRun) names(pkg string) bool {
	return slices.ContainsFunc(c.packages, func(pattern string) bool {
		pattern = strings.TrimPrefix(pattern, "./")
		if pattern == "..." {
			return true
		}
		if tree, ok := strings.CutSuffix(pattern, "/..."); ok {
			return pkg == tree || strings.HasPrefix(pkg, tree+"/")
		}
		return pkg == pattern
	})
}

// parseCheckRuns reads every go test line of workflow, strictly: a line it
// can't read whole is an error naming it, as is a workflow with none, so the
// check never judges a test against a line it misread. It reads no YAML, so
// it refuses what would change a line's run without being on it: GOFLAGS or a
// working-directory anywhere, a go test line anywhere but a run: | block, and
// such a block holding anything but go test lines.
func parseCheckRuns(workflow string) ([]checkRun, error) {
	for _, word := range []string{"GOFLAGS", "working-directory"} {
		if strings.Contains(workflow, word) {
			return nil, fmt.Errorf("can't judge it: it names %s, which changes what its go test lines run", word)
		}
	}
	var runs []checkRun
	lines := strings.Split(workflow, "\n")
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "run: |" || line == "- run: |" {
			block, next := runBlock(lines, i)
			i = next - 1
			blockRuns, err := parseRunBlock(block)
			if err != nil {
				return nil, err
			}
			runs = append(runs, blockRuns...)
			continue
		}
		if strings.Contains(line, "go test") {
			return nil, fmt.Errorf("can't judge %q: a go test line must stand alone in a run: | block", line)
		}
	}
	if len(runs) == 0 {
		return nil, errors.New("it has no go test line")
	}
	return runs, nil
}

// runBlock is the non-blank lines, trimmed, of the block scalar the run: |
// key at lines[at] opens, and the index of the line after it: every line
// indented deeper than the key's, and the blank ones among them.
func runBlock(lines []string, at int) (block []string, next int) {
	depth := indent(lines[at])
	for next = at + 1; next < len(lines); next++ {
		line := lines[next]
		if strings.TrimSpace(line) == "" {
			continue
		}
		if indent(line) <= depth {
			break
		}
		block = append(block, strings.TrimSpace(line))
	}
	return block, next
}

func indent(line string) int {
	return len(line) - len(strings.TrimLeft(line, " \t"))
}

// parseRunBlock reads a run: | block's go test lines. A block holding one
// must hold nothing else: a cd, an exit or any other command could change
// what they run, or whether they run.
func parseRunBlock(block []string) ([]checkRun, error) {
	if !slices.ContainsFunc(block, func(line string) bool { return strings.Contains(line, "go test") }) {
		return nil, nil
	}
	runs := make([]checkRun, 0, len(block))
	for _, line := range block {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != "go" || fields[1] != "test" {
			return nil, fmt.Errorf("can't judge %q: it shares a run block with go test lines, which must stand alone", line)
		}
		c, err := parseCheckRun(line, fields[2:])
		if err != nil {
			return nil, fmt.Errorf("can't judge %q: %w", line, err)
		}
		runs = append(runs, c)
	}
	return runs, nil
}

// parseCheckRun reads one go test line's arguments, which may be only the
// forms the weekly check uses: -tags integration, which every line needs,
// -p 1, -timeout <duration>, -run '<pattern>' with the pattern single-quoted
// and naming no subtest, and ./ package patterns. Each flag may come once.
func parseCheckRun(line string, args []string) (checkRun, error) {
	c := checkRun{line: line}
	seen := map[string]bool{}
	for i := 0; i < len(args); i++ {
		flag := args[i]
		if strings.HasPrefix(flag, "./") {
			c.packages = append(c.packages, flag)
			continue
		}
		if seen[flag] {
			return checkRun{}, fmt.Errorf("%s is given twice", flag)
		}
		seen[flag] = true
		if i+1 == len(args) {
			return checkRun{}, fmt.Errorf("%s has no value", flag)
		}
		i++
		if err := c.set(flag, args[i]); err != nil {
			return checkRun{}, err
		}
	}
	switch {
	case !seen["-tags"]:
		return checkRun{}, errors.New("it lacks -tags integration, so it runs none of the zmx tests")
	case len(c.packages) == 0:
		return checkRun{}, errors.New("it names no package")
	}
	return c, nil
}

func (c *checkRun) set(flag, value string) error {
	switch flag {
	case "-tags":
		if value != "integration" {
			return fmt.Errorf("-tags %s, where the check reads integration alone", value)
		}
	case "-p":
		if value != "1" {
			return fmt.Errorf("-p %s, where the integration lane needs 1", value)
		}
	case "-timeout":
		if _, err := time.ParseDuration(value); err != nil {
			return fmt.Errorf("-timeout %s: %w", value, err)
		}
	case "-run":
		pattern, quoted := strings.CutPrefix(value, "'")
		pattern, closed := strings.CutSuffix(pattern, "'")
		if !quoted || !closed || pattern == "" || strings.Contains(pattern, "'") {
			return fmt.Errorf("-run %s, where the check reads one single-quoted pattern", value)
		}
		if strings.Contains(pattern, "/") {
			return fmt.Errorf("-run %s names subtests, which the check doesn't judge", value)
		}
		run, err := regexp.Compile(pattern)
		if err != nil {
			return fmt.Errorf("-run %s: %w", value, err)
		}
		c.run = run
	default:
		return fmt.Errorf("%s is no argument the check reads", flag)
	}
	return nil
}
