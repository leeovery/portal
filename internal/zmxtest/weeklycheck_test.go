package zmxtest

import (
	"regexp"
	"slices"
	"strings"
	"testing"
)

// step is a workflow step whose run: | block holds lines.
func step(lines ...string) string {
	return "      - name: Test\n        run: |\n          " + strings.Join(lines, "\n          ") + "\n"
}

const goodLine = "go test -tags integration -p 1 ./cmd"

func TestParseCheckRuns(t *testing.T) {
	t.Run("it reads each go test line's packages and -run pattern, passing over other steps", func(t *testing.T) {
		runs, err := parseCheckRuns(`
      - name: Build
        run: |
          binary=$(scripts/zmx-build "$COMMIT")
          echo "PORTAL_TEST_ZMX_BINARY=$binary" >> "$GITHUB_ENV"

      - name: Test
        run: |
          go test -tags integration -p 1 -timeout 30m ./internal/backend/zmx/... ./internal/spawn/...

          go test -tags integration -p 1 -timeout 30m -run 'AgainstZmx$' ./cmd
      - run: |
          go test -tags integration ./internal/zmxtest
`)
		if err != nil {
			t.Fatalf("parseCheckRuns: %v", err)
		}
		if len(runs) != 3 ||
			!slices.Equal(runs[0].packages, []string{"./internal/backend/zmx/...", "./internal/spawn/..."}) || runs[0].run != nil ||
			!slices.Equal(runs[1].packages, []string{"./cmd"}) || runs[1].run == nil || runs[1].run.String() != "AgainstZmx$" ||
			!slices.Equal(runs[2].packages, []string{"./internal/zmxtest"}) {
			t.Errorf("runs = %+v, want the three lines' packages, the second's pattern", runs)
		}
	})

	for name, workflow := range map[string]string{
		"a workflow with no go test line":                   step("make test"),
		"a line naming no package":                          step("go test -tags integration -p 1"),
		"a line without -tags integration":                  step("go test -p 1 ./cmd"),
		"another tag":                                       step("go test -tags integration,manual ./cmd"),
		"-p other than 1":                                   step("go test -tags integration -p 4 ./cmd"),
		"a timeout that isn't a duration":                   step("go test -tags integration -timeout soon ./cmd"),
		"-run=PATTERN":                                      step("go test -tags integration -run=AgainstZmx ./cmd"),
		"an unquoted pattern":                               step("go test -tags integration -run AgainstZmx ./cmd"),
		"a double-quoted pattern":                           step(`go test -tags integration -run "AgainstZmx" ./cmd`),
		"a pattern split by a space":                        step("go test -tags integration -run 'A B' ./cmd"),
		"a pattern naming a subtest":                        step("go test -tags integration -run 'A/B' ./cmd"),
		"a pattern that doesn't compile":                    step("go test -tags integration -run '(' ./cmd"),
		"-skip":                                             step("go test -tags integration -skip 'Slow' ./cmd"),
		"an unknown flag":                                   step("go test -tags integration -race ./cmd"),
		"a flag given twice":                                step("go test -tags integration -run 'A' -run 'B' ./cmd"),
		"a flag with no value":                              step("go test ./cmd -tags"),
		"a line continued onto the next":                    step("go test -tags integration \\", "  ./cmd"),
		"a package that isn't a ./ pattern":                 step("go test -tags integration github.com/leeovery/portal/cmd"),
		"GOFLAGS set for the job":                           "    env:\n      GOFLAGS: -count=1\n" + step(goodLine),
		"GOFLAGS written to the environment":                step(`echo "GOFLAGS=-run=X" >> "$GITHUB_ENV"`) + step(goodLine),
		"a working-directory":                               "      - working-directory: sub\n" + step(goodLine),
		"a cd in the block":                                 step("cd cmd", goodLine),
		"an exit in the block":                              step("exit 0", goodLine),
		"anything else in the block":                        step(goodLine, "echo done"),
		"an inline run":                                     "      - name: Test\n        run: " + goodLine + "\n",
		"an inline run as a list item":                      "      - run: " + goodLine + "\n",
		"a go test line in a folded block":                  "      - name: Test\n        run: >\n          " + goodLine + "\n",
		"a go test line in a block that strips its newline": "      - name: Test\n        run: |-\n          " + goodLine + "\n",
	} {
		t.Run("it refuses "+name, func(t *testing.T) {
			if runs, err := parseCheckRuns(workflow); err == nil {
				t.Errorf("parseCheckRuns = %+v, want an error", runs)
			}
		})
	}

	t.Run("a refusal names the line it can't judge", func(t *testing.T) {
		for _, c := range []struct{ workflow, line string }{
			{step("go test -tags integration -skip 'Slow' ./cmd"), "go test -tags integration -skip 'Slow' ./cmd"},
			{step("cd cmd", goodLine), "cd cmd"},
			{"      - run: " + goodLine + "\n", "- run: " + goodLine},
		} {
			if _, err := parseCheckRuns(c.workflow); err == nil || !strings.Contains(err.Error(), "can't judge") || !strings.Contains(err.Error(), c.line) {
				t.Errorf("parseCheckRuns = %v, want an error that can't judge %q", err, c.line)
			}
		}
	})

	t.Run("a refusal of GOFLAGS or a working-directory says it can't judge, naming which", func(t *testing.T) {
		for _, word := range []string{"GOFLAGS", "working-directory"} {
			if _, err := parseCheckRuns("# " + word + "\n" + step(goodLine)); err == nil || !strings.Contains(err.Error(), "can't judge") || !strings.Contains(err.Error(), word) {
				t.Errorf("parseCheckRuns = %v, want an error that can't judge it, naming %s", err, word)
			}
		}
	})
}

func TestWeeklyCheckParses(t *testing.T) {
	runs, err := weeklyRuns()
	if err != nil {
		t.Fatalf("%s: %v", weeklyCheck, err)
	}
	if len(runs) == 0 {
		t.Fatalf("%s has no go test line", weeklyCheck)
	}
	if pkg, err := packageUnderTest(); err != nil || pkg != "internal/zmxtest" {
		t.Errorf("packageUnderTest = %q, %v; want internal/zmxtest", pkg, err)
	}
	if err := covered(runs, "internal/zmxtest", "TestNew"); err != nil {
		t.Errorf("the weekly check doesn't run zmxtest's own tests: %v", err)
	}
}

func TestCovered(t *testing.T) {
	runs := []checkRun{
		{line: "go test -tags integration ./internal/backend/...", packages: []string{"./internal/backend/..."}},
		{line: "go test -tags integration -run 'AgainstZmx$' ./cmd", packages: []string{"./cmd"}, run: regexp.MustCompile(`AgainstZmx$`)},
	}
	for _, tc := range []struct {
		pkg, test string
		want      string // a part of the error; empty when covered
	}{
		{"internal/backend", "TestA", ""},
		{"internal/backend/zmx", "TestA", ""},
		{"cmd", "TestOpenAgainstZmx", ""},
		{"internal/backendtest", "TestA", "add the line go test -tags integration -p 1 -timeout 30m ./internal/backendtest"},
		{"cmd/capturetool", "TestOpenAgainstZmx", "never runs cmd/capturetool"},
		{"cmd", "TestOpenBurst", "name the test to match it (go test -tags integration -run 'AgainstZmx$' ./cmd)"},
	} {
		err := covered(runs, tc.pkg, tc.test)
		switch {
		case tc.want == "" && err != nil:
			t.Errorf("covered(%s, %s) = %v, want it run", tc.pkg, tc.test, err)
		case tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), tc.pkg+"."+tc.test)):
			t.Errorf("covered(%s, %s) = %v, want an error naming %s.%s and saying %q", tc.pkg, tc.test, err, tc.pkg, tc.test, tc.want)
		}
	}

	t.Run("a line naming ./... runs every package", func(t *testing.T) {
		if err := covered([]checkRun{{packages: []string{"./..."}}}, "internal/spawn", "TestA"); err != nil {
			t.Errorf("covered = %v, want it run", err)
		}
	})
}
