package migrate_test

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/parser"
	"go/token"
	"go/types"
	"maps"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/sourceguardtest"
)

// Only portal migrate knows tmux: its own client runs the binary, and
// tmuxtest runs it for migrate's tests. Nothing else may, so a tmux call
// creeping back in fails here rather than reaching the developer's server.
//
// The guard's reach, exactly:
//
//  1. A string literal holding tmux as a word appears only in the files
//     tmuxNamingFiles lists, read in every source whatever its build tags or
//     platform, tests included. Running tmux takes its name, so a file off the
//     list cannot spell it for any call: a function value, a seam, a wrapper,
//     a dot import or a field set later. It can only borrow a listed file's
//     constant, which item 2 reads.
//  2. Only migrate's client and tmuxtest hand tmux to a runner: os/exec's
//     Command, CommandContext and LookPath, syscall.Exec, os.StartProcess, or
//     an exec.Cmd literal's Path, as its program, or in any string it is
//     handed as a command line a shell may run. It reads a value through
//     parentheses and conversions as a literal, a constant, or a variable its
//     declaration or := sets from one, flagged at that initialiser whatever is
//     assigned to it later; in a source built for another platform, or by no
//     lane, as a literal alone. This holds the listed files to the two
//     owners, and catches a listed file's constant handed straight to a runner
//     elsewhere. Beyond it, in a listed file or through one's constant: a
//     runner held in a function value or a seam, a wrapper handed the program,
//     another runner (syscall.ForkExec, unix.Exec), an exec.Cmd field set
//     after the literal, and a value made at runtime, such as a function's
//     result, a concatenation holding a variable, a field read, a variable set
//     by assignment alone or declared in another package, or an argv slice
//     held in a variable.
//  3. Outside the list, a name that never spells the word escapes both: one
//     assembled at runtime from parts that never spell it, or read from the
//     environment or config.
const (
	tmuxProgram = "tmux"
	// migrateTmuxFile is the one production source that runs tmux.
	migrateTmuxFile = "internal/migrate/tmux.go"
	// tmuxtestDir is the harness that runs tmux for migrate's tests.
	tmuxtestDir = "internal/tmuxtest"
)

// tmuxCommandWord is tmux as a word a shell runs: after the start, whitespace,
// a separator, an opening parenthesis, a path's last slash, a quote, a
// backtick or a redirection, and before the end, whitespace, a separator, a
// closing parenthesis, a quote, a backtick or a redirection.
var tmuxCommandWord = regexp.MustCompile("(^|[\\s;&|(/`'\"<>])" + tmuxProgram + "($|[\\s;&|)`'\"<>])")

// tmuxWord is tmux as a word: no letter, digit or underscore either side.
var tmuxWord = regexp.MustCompile(`(^|[^A-Za-z0-9_])` + tmuxProgram + `($|[^A-Za-z0-9_])`)

// tmuxNamingFiles are the only files a string literal may name tmux in, each
// with why. Anything that runs tmux names it in a string somewhere, so a file
// off this list cannot hand it to a runner however the call is spelled.
var tmuxNamingFiles = map[string]string{
	// portal migrate: its client, its library and its command.
	"internal/migrate/tmux.go":                    "migrate's tmux client, the one production runner of tmux",
	"internal/migrate/tmux_test.go":               "migrate's client against a tmux shim on PATH",
	"internal/migrate/tmux_realtmux_test.go":      "migrate's client against tmuxtest servers",
	"internal/migrate/tmux_owner_guard_test.go":   "this guard: its word, its list and its fixtures",
	"internal/migrate/testmain_isolation_test.go": "poisons TMUX for migrate's tests",
	"internal/migrate/apply.go":                   "opens the tmux version's state dir, saying so on failure",
	"internal/migrate/apply_test.go":              "the hooks.json.tmux backup",
	"internal/migrate/copy.go":                    "the report's copy, which names the tmux version",
	"internal/migrate/daemon.go":                  "the tmux version's daemon.pid, named in errors",
	"internal/migrate/daemon_test.go":             "the wait for the tmux version's daemon",
	"internal/migrate/found.go":                   "what migrate finds beside the state dir, named in errors",
	"internal/migrate/hooksbackup.go":             "the hooks.json.tmux backup",
	"internal/migrate/legacy.go":                  "the reader of the tmux version's state, named in errors",
	"internal/migrate/plan.go":                    "the tmux pane id a hook key is judged by",
	"internal/migrate/plan_test.go":               "the plan from tmux session names",
	"internal/migrate/realistic_test.go":          "the plan from realistic tmux session names",
	"internal/migrate/report.go":                  "the report's count of tmux sessions",
	"internal/migrate/report_test.go":             "the report, which names state-tmux and tmux sessions",
	"internal/migrate/run.go":                     "the run's steps against a tmux server, named in errors",
	"internal/migrate/run_test.go":                "the run against a tmux server, and its report",
	"cmd/migrate.go":                              "the migrate command's help",
	"cmd/migrate_test.go":                         "the migrate command's report",
	"cmd/migrate_integration_test.go":             "the migrate command end to end, with a tmuxtest server",

	// tmuxtest: the tmux servers migrate's tests run.
	"internal/tmuxtest/skip.go":            "skips where tmux isn't installed",
	"internal/tmuxtest/socket.go":          "runs tmux on a test's own socket",
	"internal/tmuxtest/tmux_version.go":    "starts a test's tmux server",
	"internal/tmuxtest/leaf_guard_test.go": "the guard on who imports tmuxtest",

	// The migration's state paths and the stand-downs that read them.
	"internal/state/paths.go":            "state-tmux, the tmux version's state kept beside the state dir",
	"internal/state/paths_test.go":       "state-tmux's path",
	"internal/state/migration.go":        "whether state-tmux exists, named in errors",
	"internal/state/migration_test.go":   "state-tmux and the staging dir",
	"internal/hooksweep/reason.go":       "the tmux-keys stand-down reason",
	"internal/hooksweep/sweep_test.go":   "the sweep standing down over the tmux version's keys",
	"internal/hooks/store_test.go":       "keys the tmux version wrote, judged like any other",
	"internal/hooks/store_shape_test.go": "keys the tmux version wrote, reaped",
	"internal/hookstest/hooks_test.go":   "the tmux-era seed keys",
	"cmd/doctor.go":                      "doctor's legacy copy: the tmux-keys stand-down and the tmux version's index",
	"cmd/doctor_test.go":                 "doctor's legacy copy",
	"cmd/doctor_migration_test.go":       "doctor leaves state-tmux alone",
	"cmd/doctor_stale_hooks_test.go":     "doctor standing down over the tmux version's keys",
	"cmd/session_index.go":               "the refusal of the tmux version's index",
	"cmd/session_index_test.go":          "the refusal of the tmux version's index",
	"cmd/state_hydrate_steps_test.go":    "hydrate refusing the tmux version's index",
	"cmd/state_hydrate_test.go":          "a transcript as the tmux backend saved it",
	"cmd/state_resume_recover_test.go":   "a pane a tmux-era build parked",
	"cmd/state_test.go":                  "the tmux helper's flags, refused",
	"cmd/bootstrap/orphan_sweep_test.go": "the tmux Portal's daemon, never signalled",

	// The TMUX poison and fixtures that keep tmux out.
	"internal/portaltest/isolated_env.go":          "the poisoned TMUX socket",
	"cmd/testmain_isolation_test.go":               "poisons TMUX for cmd's tests",
	"internal/spawn/exec_boundary_test.go":         "a caller's TMUX, stripped from a spawned window",
	"internal/zmxtest/zmxtest_test.go":             "a caller's TMUX, kept from a zmx session",
	"internal/backend/zmx/create_test.go":          "a caller's TMUX, kept from a created session",
	"internal/backend/zmx/older_test.go":           "a caller's TMUX, kept from zmx",
	"internal/backend/zmx/runs_test.go":            "a caller's TMUX, kept from zmx",
	"internal/backend/zmx/session_test.go":         "a caller's TMUX, kept from an attach",
	"internal/backend/zmx/zmx_integration_test.go": "a session told nothing of the caller's TMUX",
	"internal/backend/zmx/holders_test.go":         "a tmux process, which holds no zmx session",
	"internal/capture/swap_harness_test.go":        "screens that must not name tmux",
	"internal/tui/model_test.go":                   "the loading view, which must not name tmux",
	"internal/attachkeys/matcher_fuzz_test.go":     "a terminal's XTVERSION reply in the fuzz corpus",
}

// runner is a function handed a program to run or find: program is the index
// of the argument naming it, and the arguments from there, up to words of them
// (all when words is 0), may hold a command line. An argument that is a
// composite literal, an argv, is read element by element.
type runner struct {
	pkg, name      string
	program, words int
}

var runners = []runner{
	{pkg: "os/exec", name: "Command"},
	{pkg: "os/exec", name: "CommandContext", program: 1},
	{pkg: "os/exec", name: "LookPath"},
	{pkg: "syscall", name: "Exec", words: 2},
	{pkg: "os", name: "StartProcess", words: 2},
}

// The exec.Cmd fields that name the program and hold its argv, in the order an
// unkeyed literal gives them.
const (
	cmdPkg, cmdType      = "os/exec", "Cmd"
	cmdPath, cmdArgs     = "Path", "Args"
	cmdPathAt, cmdArgsAt = 0, 1
)

// tmuxRun is a place a runner is handed tmux.
type tmuxRun struct {
	file string
	line int
}

func (r tmuxRun) String() string { return fmt.Sprintf("%s:%d", r.file, r.line) }

// handed is an expression a runner reads: its program, read as a name or a
// command line, or another argument, read as a command line.
type handed struct {
	expr    ast.Expr
	program bool
}

// sourceReader is how a pass reads a source: the package a qualifier names
// (empty for none), whether a one-argument call is a conversion, and the
// string an expression holds with the expression it is read from, which is
// where a run is reported.
type sourceReader struct {
	importPath   func(qualifier *ast.Ident) string
	isConversion func(*ast.CallExpr) bool
	value        func(ast.Expr) (value string, from ast.Expr, ok bool)
}

func TestOnlyMigrateRunsTmux(t *testing.T) {
	t.Run("it finds tmux run only by migrate's client and tmuxtest", func(t *testing.T) {
		root, sources := sourceguardtest.RepoSources(t, sourceguardtest.AllSources)
		runs, unresolved := tmuxByLiteral(sources)
		for _, lane := range sourceguardtest.Lanes() {
			for _, pkg := range typedPackages(t, root, buildingIn(t, root, unresolved, lane), lane) {
				runs = append(runs, tmuxByType(pkg)...)
			}
		}
		for _, run := range judgeTmuxRuns(t, runs) {
			t.Errorf("%s runs tmux; only portal migrate's client (%s) and %s may", run, migrateTmuxFile, tmuxtestDir)
		}
	})

	t.Run("it finds tmux named by a literal to each runner, under any import name", func(t *testing.T) {
		const src = `package x

import (
	"context"
	"os"
	"os/exec"
	run "os/exec"
	"syscall"
)

func a() { _ = exec.Command("tmux", "ls") }
func b(ctx context.Context) { _ = exec.CommandContext(ctx, "tmux") }
func c() { _, _ = exec.LookPath("tmux") }
func d() { _ = run.Command("/opt/homebrew/bin/tmux") }
func e() { _ = syscall.Exec("/opt/homebrew/bin/tmux", nil, nil) }
func f() { _, _ = os.StartProcess("/opt/homebrew/bin/tmux", nil, nil) }
func g() { _ = &exec.Cmd{Path: "/opt/homebrew/bin/tmux"} }
func h() { _ = exec.Cmd{"tmux", nil} }
func i() { _ = exec.Command("tmuxinator", "my-tmux", "tmux.conf") }
func j(p string) { _ = exec.Command(p) }
`
		runs, unresolved := tmuxByLiteral([]sourceguardtest.ParsedSource{parseFixture(t, "internal/x/x.go", src)})

		want := []string{"internal/x/x.go:11", "internal/x/x.go:12", "internal/x/x.go:13", "internal/x/x.go:14", "internal/x/x.go:15", "internal/x/x.go:16", "internal/x/x.go:17", "internal/x/x.go:18"}
		if got := runStrings(runs); !slices.Equal(got, want) {
			t.Errorf("tmuxByLiteral found %q, want %q", got, want)
		}
		if !slices.Equal(unresolved, []string{"internal/x"}) {
			t.Errorf("tmuxByLiteral left %q to the type checker, want the directory of the call naming a variable", unresolved)
		}
	})

	t.Run("it finds tmux run through a shell, or handed to another program", func(t *testing.T) {
		const src = `package x

import (
	"os"
	"os/exec"
	"syscall"
)

func a() { _ = exec.Command("sh", "-c", "tmux kill-server") }
func b() { _ = exec.Command("sh", "-c", "cd /tmp && tmux") }
func c() { _ = exec.Command("sh", "-c", "true;tmux ls") }
func d() { _ = exec.Command("sh", "-c", "(tmux)") }
func e() { _ = exec.Command("zmx", "run", "s", "tmux") }
func f() { _ = exec.Command("sh", "-c", "/opt/homebrew/bin/tmux ls") }
func g() { _ = syscall.Exec("/bin/sh", []string{"sh", "-c", "tmux ls"}, nil) }
func h() { _, _ = os.StartProcess("/bin/sh", []string{"sh", "-c", "x|tmux"}, nil) }
func i() { _ = exec.Cmd{Path: "/bin/sh", Args: []string{"sh", "-c", "tmux ls"}} }
func j() { _ = syscall.Exec("/bin/sh", nil, []string{"X=tmux"}) }
func k() { _ = exec.Command("sh", "-c", "tmuxinator; my-tmux; echo tmux.conf") }
func l() { _ = exec.Command("sh", "-c", "echo ` + "`tmux ls`" + `") }
func m() { _ = exec.Command("sh", "-c", "tmux>/dev/null") }
func n() { _ = exec.Command("sh", "-c", "tmux<in") }
func o() { _ = exec.Command("sh", "-c", "'tmux' ls") }
func p() { _ = exec.Command("sh", "-c", "\"tmux\" ls") }
`
		runs, _ := tmuxByLiteral([]sourceguardtest.ParsedSource{parseFixture(t, "internal/x/x.go", src)})

		want := []string{"internal/x/x.go:9", "internal/x/x.go:10", "internal/x/x.go:11", "internal/x/x.go:12", "internal/x/x.go:13", "internal/x/x.go:14", "internal/x/x.go:15", "internal/x/x.go:16", "internal/x/x.go:17", "internal/x/x.go:20", "internal/x/x.go:21", "internal/x/x.go:22", "internal/x/x.go:23", "internal/x/x.go:24"}
		if got := runStrings(runs); !slices.Equal(got, want) {
			t.Errorf("tmuxByLiteral found %q, want %q: tmux as a command word, in an argv but not an environment", got, want)
		}
	})

	t.Run("it reads a literal through parentheses and conversions, without types", func(t *testing.T) {
		const src = `package x

import "os/exec"

type name string

func a() { _ = exec.Command(("tmux")) }
func b() { _ = exec.Command(string("tmux")) }
func c() { _ = exec.Command(name("/opt/homebrew/bin/tmux")) }
func d() { _ = (exec.Command)("tmux") }
func e() { _ = exec.Command("sh", "-c", string(("tmux ls"))) }
`
		runs, _ := tmuxByLiteral([]sourceguardtest.ParsedSource{parseFixture(t, "internal/x/x_linux.go", src)})

		want := []string{"internal/x/x_linux.go:7", "internal/x/x_linux.go:8", "internal/x/x_linux.go:9", "internal/x/x_linux.go:10", "internal/x/x_linux.go:11"}
		if got := runStrings(runs); !slices.Equal(got, want) {
			t.Errorf("tmuxByLiteral found %q, want %q", got, want)
		}
	})

	t.Run("it finds no run in a call that is not a runner's", func(t *testing.T) {
		const src = `package x

import exec "example.com/notexec"

func a() { _ = exec.Command("tmux") }
func b() { _ = exec.Cmd{Path: "tmux"} }
`
		if runs, unresolved := tmuxByLiteral([]sourceguardtest.ParsedSource{parseFixture(t, "internal/x/x.go", src)}); len(runs) != 0 || len(unresolved) != 0 {
			t.Errorf("tmuxByLiteral found %v and left %q, want nothing: the package is not os/exec", runs, unresolved)
		}
	})

	t.Run("it leaves a test's variable to the type checker, which reads tests too", func(t *testing.T) {
		const src = `package x

import "os/exec"

func a(p string) { _ = exec.Command(p) }
`
		if _, unresolved := tmuxByLiteral([]sourceguardtest.ParsedSource{parseFixture(t, "internal/x/x_test.go", src)}); !slices.Equal(unresolved, []string{"internal/x"}) {
			t.Errorf("tmuxByLiteral left %q to the type checker, want the test source's directory", unresolved)
		}
	})

	checker := sourceguardtest.NewTypeChecker(t, ".")
	typed := func(t *testing.T, file, src string) []string {
		t.Helper()
		return runStrings(tmuxByType(checker.Sources(t, map[string]string{file: src})))
	}

	t.Run("it finds tmux named by a constant", func(t *testing.T) {
		const src = `package migrate

import (
	"context"
	"os/exec"
)

const program = "tmux"
const shell = "tmux " + "ls"

func a() { _ = exec.Command(program) }
func b(ctx context.Context) { _ = exec.CommandContext(ctx, program) }
func c(p string) { _ = exec.Command(p) }
func d() { _ = exec.Command("sh", "-c", shell) }
func e() { _ = exec.Cmd{Path: program} }
`
		const file = "internal/x/x.go"
		if got, want := typed(t, file, src), []string{file + ":11", file + ":12", file + ":14", file + ":15"}; !slices.Equal(got, want) {
			t.Errorf("tmuxByType found %q, want %q", got, want)
		}
	})

	t.Run("it finds tmux named by a constant a test file declares", func(t *testing.T) {
		const src = `package migrate

import "os/exec"

const program = "/opt/homebrew/bin/tmux"

func a() { _ = exec.Command(program) }
`
		const file = "internal/x/x_test.go"
		if got, want := typed(t, file, src), []string{file + ":7"}; !slices.Equal(got, want) {
			t.Errorf("tmuxByType found %q, want %q", got, want)
		}
	})

	t.Run("it finds tmux named by a variable at the literal or constant it starts from", func(t *testing.T) {
		const src = `package migrate

import (
	"os"
	"os/exec"
)

const program = "tmux"

var bin = "tmux"
var fromConstant, other = program, "zmx"

func a() { _ = exec.Command(bin, "ls") }
func b() { _ = exec.Command(fromConstant) }
func c() {
	local := "/opt/homebrew/bin/tmux"
	_, _ = os.StartProcess(local, nil, nil)
}
func d() {
	var shell = "tmux kill-server"
	_ = exec.Command("sh", "-c", shell)
}
func e() {
	reassigned := "tmux"
	reassigned = "zmx"
	_ = exec.Command(reassigned)
}
func f() { _ = exec.Command(other) }
`
		const file = "internal/x/x_test.go"
		if got, want := typed(t, file, src), []string{file + ":10", file + ":11", file + ":16", file + ":20", file + ":24"}; !slices.Equal(got, want) {
			t.Errorf("tmuxByType found %q, want %q: each initialiser, a later assignment untracked", got, want)
		}
	})

	t.Run("it reads a variable through parentheses and conversions", func(t *testing.T) {
		const src = `package migrate

import "os/exec"

type name string

func a() {
	bin := "tmux"
	_ = exec.Command(string(name((bin))))
}
`
		const file = "internal/x/x.go"
		if got, want := typed(t, file, src), []string{file + ":8"}; !slices.Equal(got, want) {
			t.Errorf("tmuxByType found %q, want %q", got, want)
		}
	})

	t.Run("it leaves a value made at runtime unread", func(t *testing.T) {
		const src = `package migrate

import (
	"os/exec"
	"strings"
)

type config struct{ bin string }

func a() { _ = exec.Command(strings.TrimSpace(" tmux ")) }
func b(dir string) { _ = exec.Command(dir + "/tmux") }
func c(cfg config) { _ = exec.Command(cfg.bin) }
func d() {
	var bin string
	bin = "tmux"
	_ = exec.Command(bin)
}
`
		if got := typed(t, "internal/x/x.go", src); len(got) != 0 {
			t.Errorf("tmuxByType found %q, want nothing: these are beyond what a source can be read for", got)
		}
	})

	t.Run("it checks a directory only in a lane that builds one of its files", func(t *testing.T) {
		root := t.TempDir()
		writeFixtureFile(t, root, "go.mod", "module example.com/m\n\ngo 1.22\n")
		writeFixtureFile(t, root, "plain/p.go", "package plain\n")
		writeFixtureFile(t, root, "tagged/t_test.go", "//go:build integration\n\npackage tagged\n")
		dirs := []string{"plain", "tagged"}

		if got := buildingIn(t, root, dirs, sourceguardtest.WithBuildTags()); !slices.Equal(got, []string{"plain"}) {
			t.Errorf("the unit lane builds %q, want plain alone", got)
		}
		if got := buildingIn(t, root, dirs, sourceguardtest.WithBuildTags(sourceguardtest.IntegrationTag)); !slices.Equal(got, dirs) {
			t.Errorf("the integration lane builds %q, want both", got)
		}
	})

	t.Run("it passes runs in migrate's client and tmuxtest", func(t *testing.T) {
		var rec harnesstest.Recorder
		var found []string
		rec.Run(func() { found = judgeTmuxRuns(&rec, allowedRuns()) })
		if rec.Failed() || len(found) != 0 {
			t.Errorf("judgeTmuxRuns reported %v and %s, want nothing", found, rec.Report())
		}
	})

	t.Run("it reports a run anywhere else, once", func(t *testing.T) {
		stray := tmuxRun{file: "cmd/open.go", line: 7}
		var rec harnesstest.Recorder
		var found []string
		rec.Run(func() { found = judgeTmuxRuns(&rec, append(allowedRuns(), stray, stray)) })
		if rec.Failed() || !slices.Equal(found, []string{stray.String()}) {
			t.Errorf("judgeTmuxRuns reported %v and %s, want %s alone", found, rec.Report(), stray)
		}
	})

	for _, owner := range []string{migrateTmuxFile, tmuxtestDir} {
		t.Run("it fails when it finds no run in "+owner, func(t *testing.T) {
			runs := slices.DeleteFunc(allowedRuns(), func(r tmuxRun) bool { return strings.HasPrefix(r.file, owner) })
			var rec harnesstest.Recorder
			rec.Run(func() { judgeTmuxRuns(&rec, runs) })
			if len(rec.Fatals) != 1 {
				t.Errorf("judgeTmuxRuns without a run in %s reported %s, want one fatal", owner, rec.Report())
			}
		})
	}
}

func TestOnlyListedFilesNameTmux(t *testing.T) {
	t.Run("it finds a string naming tmux only in the files it lists", func(t *testing.T) {
		root, sources := sourceguardtest.RepoSources(t, sourceguardtest.AllSources)
		for _, problem := range judgeTmuxNamers(t, root, tmuxNamers(sources), tmuxNamingFiles) {
			t.Error(problem)
		}
	})

	t.Run("it finds tmux named as a word, in any string literal", func(t *testing.T) {
		const src = `package x

var (
	_ = "state-tmux"
	_ = "tmux 3.5"
	_ = ` + "`tmux`" + `
	_ = "TMUX_PANE"
	_ = "TMUX"
	_ = "tmuxinator"
	_ = "my_tmux"
	_ = "tmux2"
)
`
		got := runStrings(tmuxNamers([]sourceguardtest.ParsedSource{parseFixture(t, "internal/x/x_test.go", src)}))
		if want := []string{"internal/x/x_test.go:4", "internal/x/x_test.go:5", "internal/x/x_test.go:6"}; !slices.Equal(got, want) {
			t.Errorf("tmuxNamers found %q, want %q", got, want)
		}
	})

	root := t.TempDir()
	writeFixtureFile(t, root, "listed.go", "package x\n")
	writeFixtureFile(t, root, "other.go", "package x\n")
	listed := map[string]string{"listed.go": "a reason"}
	judge := func(namers []tmuxRun, listed map[string]string) (problems []string, rec *harnesstest.Recorder) {
		rec = &harnesstest.Recorder{}
		rec.Run(func() { problems = judgeTmuxNamers(rec, root, namers, listed) })
		return problems, rec
	}

	t.Run("it passes a string naming tmux in a listed file", func(t *testing.T) {
		if problems, rec := judge([]tmuxRun{{file: "listed.go", line: 3}}, listed); len(problems) != 0 || rec.Failed() {
			t.Errorf("judgeTmuxNamers reported %q and %s, want nothing", problems, rec.Report())
		}
	})

	t.Run("it reports a string naming tmux in a file off the list", func(t *testing.T) {
		problems, _ := judge([]tmuxRun{{file: "listed.go", line: 3}, {file: "other.go", line: 5}}, listed)
		if len(problems) != 1 || !strings.HasPrefix(problems[0], "other.go:5 ") {
			t.Errorf("judgeTmuxNamers reported %q, want other.go:5 alone", problems)
		}
	})

	t.Run("it reports a listed file that names tmux nowhere", func(t *testing.T) {
		problems, _ := judge([]tmuxRun{{file: "listed.go", line: 3}}, map[string]string{"listed.go": "a reason", "other.go": "a reason"})
		if len(problems) != 1 || !strings.HasPrefix(problems[0], "other.go ") || !strings.Contains(problems[0], "delete it from the list") {
			t.Errorf("judgeTmuxNamers reported %q, want other.go's entry stale", problems)
		}
	})

	t.Run("it reports a listed file that does not exist", func(t *testing.T) {
		problems, _ := judge([]tmuxRun{{file: "listed.go", line: 3}}, map[string]string{"listed.go": "a reason", "gone.go": "a reason"})
		if len(problems) != 1 || !strings.HasPrefix(problems[0], "gone.go ") || !strings.Contains(problems[0], "does not exist") {
			t.Errorf("judgeTmuxNamers reported %q, want gone.go's entry missing", problems)
		}
	})

	t.Run("it fails when it finds no string naming tmux", func(t *testing.T) {
		if _, rec := judge(nil, listed); len(rec.Fatals) != 1 {
			t.Errorf("judgeTmuxNamers over nothing reported %s, want one fatal", rec.Report())
		}
	})
}

func TestOnlyCmdMigrateBuildsTheTmuxClient(t *testing.T) {
	t.Run("it finds migrate.Tmux named outside migrate only by cmd/migrate.go", func(t *testing.T) {
		_, sources := sourceguardtest.RepoSources(t, sourceguardtest.NonTestSources)
		for _, found := range judgeTmuxClientBuilders(t, sources) {
			t.Errorf("%s names migrate.Tmux; only cmd/migrate.go builds portal migrate's tmux client", found)
		}
	})

	const names = `package x

import m "github.com/leeovery/portal/internal/migrate"

var _ m.TmuxServer = m.Tmux{}
`
	t.Run("it reports migrate.Tmux under any import name outside migrate and cmd/migrate.go", func(t *testing.T) {
		var rec harnesstest.Recorder
		var found []string
		rec.Run(func() {
			found = judgeTmuxClientBuilders(&rec, []sourceguardtest.ParsedSource{
				parseFixture(t, tmuxClientBuilder, names),
				parseFixture(t, "cmd/open.go", names),
				parseFixture(t, "internal/migrate/run.go", names),
			})
		})
		if rec.Failed() || !slices.Equal(found, []string{"cmd/open.go:5"}) {
			t.Errorf("judgeTmuxClientBuilders reported %v and %s, want cmd/open.go:5 alone", found, rec.Report())
		}
	})

	t.Run("it fails when cmd/migrate.go no longer names migrate.Tmux", func(t *testing.T) {
		var rec harnesstest.Recorder
		rec.Run(func() {
			judgeTmuxClientBuilders(&rec, []sourceguardtest.ParsedSource{parseFixture(t, "cmd/open.go", names)})
		})
		if len(rec.Fatals) != 1 {
			t.Errorf("judgeTmuxClientBuilders without cmd/migrate.go reported %s, want one fatal", rec.Report())
		}
	})
}

// tmuxNamers returns each place in sources a string literal names tmux as a
// word, whatever the source's build tags or platform.
func tmuxNamers(sources []sourceguardtest.ParsedSource) []tmuxRun {
	var found []tmuxRun
	for _, source := range sources {
		file := filepath.ToSlash(source.Path)
		ast.Inspect(source.File, func(n ast.Node) bool {
			if lit, ok := n.(*ast.BasicLit); ok {
				if value, _, ok := literalValue(lit); ok && tmuxWord.MatchString(value) {
					found = append(found, tmuxRun{file: file, line: source.Position(lit.Pos()).Line})
				}
			}
			return true
		})
	}
	return found
}

// judgeTmuxNamers returns each string naming tmux in a file off listed, and
// each entry of listed whose file is gone or names tmux in no string, so the
// list cannot outlive what it allows. Finding no string naming tmux at all is
// fatal: the scan has stopped seeing the strings the list allows.
func judgeTmuxNamers(t harnesstest.TestingT, root string, namers []tmuxRun, listed map[string]string) []string {
	t.Helper()
	if len(namers) == 0 {
		t.Fatalf("found no string naming tmux; the scan no longer sees the strings tmuxNamingFiles allows")
		return nil
	}
	var problems []string
	naming := map[string]bool{}
	for _, namer := range namers {
		naming[namer.file] = true
		if _, ok := listed[namer.file]; !ok {
			problems = append(problems, fmt.Sprintf("%s names tmux in a string; only the files tmuxNamingFiles lists may, each for a reason", namer))
		}
	}
	for _, file := range slices.Sorted(maps.Keys(listed)) {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(file))); err != nil {
			problems = append(problems, fmt.Sprintf("%s is on tmuxNamingFiles but does not exist (%v); delete it from the list", file, err))
			continue
		}
		if !naming[file] {
			problems = append(problems, fmt.Sprintf("%s is on tmuxNamingFiles but names tmux in no string; delete it from the list", file))
		}
	}
	return problems
}

// tmuxByLiteral returns each place in sources a runner is handed tmux as a
// string literal, and the directory of each source handing one anything else,
// which only the type checker can read. It reads every source, whatever its
// build tags or platform. With no types to tell a conversion from a call, it
// reads through every one-argument call, which fails safe.
func tmuxByLiteral(sources []sourceguardtest.ParsedSource) (runs []tmuxRun, unresolved []string) {
	for _, source := range sources {
		imports := runnerImports(source.File)
		if len(imports) == 0 {
			continue
		}
		file := filepath.ToSlash(source.Path)
		found, unread := tmuxRunsIn(source, sourceReader{
			importPath:   func(qualifier *ast.Ident) string { return imports[qualifier.Name] },
			isConversion: func(*ast.CallExpr) bool { return true },
			value:        literalValue,
		})
		runs = append(runs, found...)
		if unread {
			unresolved = append(unresolved, path.Dir(file))
		}
	}
	slices.Sort(unresolved)
	return runs, slices.Compact(unresolved)
}

// tmuxByType returns each place in pkg a runner is handed tmux as a literal, a
// constant, or a variable set from either where it is declared.
func tmuxByType(pkg sourceguardtest.TypedPackage) []tmuxRun {
	reader := sourceReader{
		importPath: func(qualifier *ast.Ident) string {
			if name, ok := pkg.Info.Uses[qualifier].(*types.PkgName); ok {
				return name.Imported().Path()
			}
			return ""
		},
		isConversion: func(call *ast.CallExpr) bool { return pkg.Info.Types[call.Fun].IsType() },
		value:        typedValue(pkg, initialisers(pkg)),
	}
	var runs []tmuxRun
	for _, source := range pkg.Sources {
		found, _ := tmuxRunsIn(source, reader)
		runs = append(runs, found...)
	}
	return runs
}

// tmuxRunsIn returns each place in source a runner is handed tmux, and whether
// it hands one anything the reader cannot read.
func tmuxRunsIn(source sourceguardtest.ParsedSource, reader sourceReader) (runs []tmuxRun, unread bool) {
	file := filepath.ToSlash(source.Path)
	ast.Inspect(source.File, func(n ast.Node) bool {
		for _, h := range handedTo(n, reader.importPath) {
			text, from, ok := reader.value(unwrapped(h.expr, reader.isConversion))
			switch {
			case !ok:
				unread = true
			case h.program && namesTmux(text), tmuxCommandWord.MatchString(text):
				runs = append(runs, tmuxRun{file: file, line: source.Position(from.Pos()).Line})
			}
		}
		return true
	})
	return runs, unread
}

// handedTo is what n hands a runner, when n is a runner's call or an exec.Cmd
// literal.
func handedTo(n ast.Node, importPath func(*ast.Ident) string) []handed {
	switch n := n.(type) {
	case *ast.CallExpr:
		return handedToCall(n, importPath)
	case *ast.CompositeLit:
		if pkg, name := qualified(n.Type, importPath); pkg == cmdPkg && name == cmdType {
			return handedToCmd(n)
		}
	}
	return nil
}

func handedToCall(call *ast.CallExpr, importPath func(*ast.Ident) string) []handed {
	pkg, name := qualified(call.Fun, importPath)
	at := slices.IndexFunc(runners, func(r runner) bool { return r.pkg == pkg && r.name == name })
	if at < 0 || len(call.Args) <= runners[at].program {
		return nil
	}
	r := runners[at]
	args := call.Args[r.program:]
	if r.words > 0 && len(args) > r.words {
		args = args[:r.words]
	}
	found := []handed{{expr: args[0], program: true}}
	for _, arg := range args[1:] {
		found = append(found, words(arg)...)
	}
	return found
}

func handedToCmd(lit *ast.CompositeLit) []handed {
	var found []handed
	for i, elt := range lit.Elts {
		field, value := cmdField(i, elt)
		switch field {
		case cmdPath:
			found = append(found, handed{expr: value, program: true})
		case cmdArgs:
			found = append(found, words(value)...)
		}
	}
	return found
}

// cmdField names the exec.Cmd field elt sets, at position i of the literal.
func cmdField(i int, elt ast.Expr) (string, ast.Expr) {
	if kv, ok := elt.(*ast.KeyValueExpr); ok {
		if key, ok := kv.Key.(*ast.Ident); ok {
			return key.Name, kv.Value
		}
		return "", nil
	}
	switch i {
	case cmdPathAt:
		return cmdPath, elt
	case cmdArgsAt:
		return cmdArgs, elt
	}
	return "", nil
}

// words are the strings arg hands a runner for a command line: the elements of
// an argv literal, or arg itself.
func words(arg ast.Expr) []handed {
	lit, ok := arg.(*ast.CompositeLit)
	if !ok {
		return []handed{{expr: arg}}
	}
	found := make([]handed, 0, len(lit.Elts))
	for _, elt := range lit.Elts {
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			elt = kv.Value
		}
		found = append(found, handed{expr: elt})
	}
	return found
}

// qualified is the package path and name expr names as pkg.Name, empty when it
// names nothing in another package.
func qualified(expr ast.Expr, importPath func(*ast.Ident) string) (pkg, name string) {
	for paren, ok := expr.(*ast.ParenExpr); ok; paren, ok = expr.(*ast.ParenExpr) {
		expr = paren.X
	}
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return "", ""
	}
	qualifier, ok := sel.X.(*ast.Ident)
	if !ok {
		return "", ""
	}
	return importPath(qualifier), sel.Sel.Name
}

// runnerImports maps each name file imports a runner's package under to its
// path.
func runnerImports(file *ast.File) map[string]string {
	imports := map[string]string{}
	for _, r := range runners {
		for _, name := range importNames(file, r.pkg) {
			imports[name] = r.pkg
		}
	}
	return imports
}

// importNames are the names file imports the package at pkgPath under.
func importNames(file *ast.File, pkgPath string) []string {
	var names []string
	for _, spec := range file.Imports {
		if p, err := strconv.Unquote(spec.Path.Value); err != nil || p != pkgPath {
			continue
		}
		if spec.Name != nil {
			names = append(names, spec.Name.Name)
			continue
		}
		names = append(names, path.Base(pkgPath))
	}
	return names
}

// unwrapped is expr past the parentheses and conversions around it.
func unwrapped(expr ast.Expr, isConversion func(*ast.CallExpr) bool) ast.Expr {
	for {
		switch e := expr.(type) {
		case *ast.ParenExpr:
			expr = e.X
		case *ast.CallExpr:
			if len(e.Args) != 1 || e.Ellipsis.IsValid() || !isConversion(e) {
				return expr
			}
			expr = e.Args[0]
		default:
			return expr
		}
	}
}

func literalValue(expr ast.Expr) (string, ast.Expr, bool) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", nil, false
	}
	value, err := strconv.Unquote(lit.Value)
	return value, lit, err == nil
}

// typedValue reads a constant, or a variable from the constant initialisers
// gives it.
func typedValue(pkg sourceguardtest.TypedPackage, initialisers map[types.Object]ast.Expr) func(ast.Expr) (string, ast.Expr, bool) {
	return func(expr ast.Expr) (string, ast.Expr, bool) {
		if value, ok := constantString(pkg, expr); ok {
			return value, expr, true
		}
		if ident, ok := expr.(*ast.Ident); ok {
			if init := initialisers[pkg.Object(ident)]; init != nil {
				value, ok := constantString(pkg, init)
				return value, init, ok
			}
		}
		return "", nil, false
	}
}

func constantString(pkg sourceguardtest.TypedPackage, expr ast.Expr) (string, bool) {
	value := pkg.Info.Types[expr].Value
	if value == nil || value.Kind() != constant.String {
		return "", false
	}
	return constant.StringVal(value), true
}

// initialisers maps each variable pkg declares with a value, by var or :=, to
// the expression that value is.
func initialisers(pkg sourceguardtest.TypedPackage) map[types.Object]ast.Expr {
	found := map[types.Object]ast.Expr{}
	declare := func(names []ast.Expr, values []ast.Expr) {
		if len(names) != len(values) {
			return
		}
		for i, name := range names {
			if ident, ok := name.(*ast.Ident); ok {
				if v, ok := pkg.Info.Defs[ident].(*types.Var); ok {
					found[v] = values[i]
				}
			}
		}
	}
	for _, source := range pkg.Sources {
		ast.Inspect(source.File, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.ValueSpec:
				names := make([]ast.Expr, len(n.Names))
				for i, name := range n.Names {
					names[i] = name
				}
				declare(names, n.Values)
			case *ast.AssignStmt:
				if n.Tok == token.DEFINE {
					declare(n.Lhs, n.Rhs)
				}
			}
			return true
		})
	}
	return found
}

func namesTmux(program string) bool {
	return program == tmuxProgram || strings.HasSuffix(program, "/"+tmuxProgram)
}

// judgeTmuxRuns returns each run outside migrate's client and tmuxtest, once.
// Finding no run in either is fatal: the scan has stopped seeing the calls
// the rule allows, so it would pass over the ones it forbids too.
func judgeTmuxRuns(t harnesstest.TestingT, runs []tmuxRun) []string {
	t.Helper()
	var inClient, inHarness bool
	var found []string
	for _, run := range runs {
		switch {
		case run.file == migrateTmuxFile:
			inClient = true
		case path.Dir(run.file) == tmuxtestDir:
			inHarness = true
		default:
			found = append(found, run.String())
		}
	}
	if !inClient || !inHarness {
		t.Fatalf("found tmux run in %s: %v, in %s: %v; the scan no longer sees the runs it allows", migrateTmuxFile, inClient, tmuxtestDir, inHarness)
		return nil
	}
	slices.Sort(found)
	return slices.Compact(found)
}

// buildingIn is those of dirs the lane builds a file of. A directory whose
// files the lane's constraints all exclude has nothing to check there, and is
// checked in the lane that includes them.
func buildingIn(t harnesstest.TestingT, root string, dirs []string, lane sourceguardtest.DepsOption) []string {
	t.Helper()
	building := map[string]bool{}
	for _, pkg := range sourceguardtest.ModulePackages(t, root, lane) {
		rel, err := filepath.Rel(root, pkg.Dir)
		if err != nil {
			t.Fatalf("relativise %s: %v", pkg.Dir, err)
			return nil
		}
		building[filepath.ToSlash(rel)] = true
	}
	return slices.DeleteFunc(slices.Clone(dirs), func(dir string) bool { return !building[dir] })
}

// typedPackages type-checks the packages at the module paths dirs under lane
// as their tests compile them, with their external tests, naming the sources
// by module path.
func typedPackages(t *testing.T, root string, dirs []string, lane sourceguardtest.DepsOption) []sourceguardtest.TypedPackage {
	t.Helper()
	if len(dirs) == 0 {
		return nil
	}
	patterns := make([]string, len(dirs))
	for i, dir := range dirs {
		patterns[i] = "./" + dir
	}
	pkgs := sourceguardtest.NewTestTypeChecker(t, root, patterns, lane).Packages(t)
	for _, pkg := range pkgs {
		for i, source := range pkg.Sources {
			rel, err := filepath.Rel(root, source.Path)
			if err != nil {
				t.Fatalf("relativise %s: %v", source.Path, err)
			}
			pkg.Sources[i].Path = rel
		}
	}
	return pkgs
}

// tmuxClientBuilder is the one production source outside migrate that may
// name migrate.Tmux, portal migrate's tmux client.
const tmuxClientBuilder = "cmd/migrate.go"

const migratePkgPath = "github.com/leeovery/portal/internal/migrate"

// judgeTmuxClientBuilders returns each place a production source outside
// migrate and cmd/migrate.go names migrate.Tmux. cmd/migrate.go naming it
// nowhere is fatal: the scan has stopped seeing the one builder it allows.
func judgeTmuxClientBuilders(t harnesstest.TestingT, sources []sourceguardtest.ParsedSource) []string {
	t.Helper()
	var builderSeen bool
	var found []string
	for _, source := range sources {
		file := filepath.ToSlash(source.Path)
		if strings.HasPrefix(file, "internal/migrate/") {
			continue
		}
		for _, pos := range tmuxClientNames(source.File) {
			if file == tmuxClientBuilder {
				builderSeen = true
				continue
			}
			found = append(found, fmt.Sprintf("%s:%d", file, source.Position(pos).Line))
		}
	}
	if !builderSeen {
		t.Fatalf("%s names migrate.Tmux nowhere; the scan no longer sees the builder it allows", tmuxClientBuilder)
		return nil
	}
	return found
}

// tmuxClientNames returns each place file names migrate.Tmux, under whatever
// name it imports migrate.
func tmuxClientNames(file *ast.File) []token.Pos {
	names := importNames(file, migratePkgPath)
	if len(names) == 0 {
		return nil
	}
	var found []token.Pos
	ast.Inspect(file, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok && sel.Sel.Name == "Tmux" {
			if pkg, ok := sel.X.(*ast.Ident); ok && slices.Contains(names, pkg.Name) {
				found = append(found, sel.Pos())
			}
		}
		return true
	})
	return found
}

// allowedRuns is a run in each place the rule allows one.
func allowedRuns() []tmuxRun {
	return []tmuxRun{{file: migrateTmuxFile, line: 143}, {file: tmuxtestDir + "/socket.go", line: 52}}
}

func parseFixture(t *testing.T, file, src string) sourceguardtest.ParsedSource {
	t.Helper()
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, file, src, sourceguardtest.ParseMode)
	if err != nil {
		t.Fatalf("parse fixture %s: %v", file, err)
	}
	return sourceguardtest.ParsedSource{Path: file, Fset: fset, File: parsed}
}

func writeFixtureFile(t *testing.T, root, name, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("make %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func runStrings(runs []tmuxRun) []string {
	out := make([]string, len(runs))
	for i, r := range runs {
		out[i] = r.String()
	}
	return out
}
