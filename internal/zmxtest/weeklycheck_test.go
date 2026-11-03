package zmxtest

import (
	"regexp"
	"slices"
	"strings"
	"testing"
)

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
		"a workflow with no go test line":   "run: make test\n",
		"a line naming no package":          "go test -tags integration -p 1\n",
		"a line without -tags integration":  "go test -p 1 ./cmd\n",
		"another tag":                       "go test -tags integration,manual ./cmd\n",
		"-p other than 1":                   "go test -tags integration -p 4 ./cmd\n",
		"a timeout that isn't a duration":   "go test -tags integration -timeout soon ./cmd\n",
		"-run=PATTERN":                      "go test -tags integration -run=AgainstZmx ./cmd\n",
		"an unquoted pattern":               "go test -tags integration -run AgainstZmx ./cmd\n",
		"a double-quoted pattern":           `go test -tags integration -run "AgainstZmx" ./cmd` + "\n",
		"a pattern split by a space":        "go test -tags integration -run 'A B' ./cmd\n",
		"a pattern naming a subtest":        "go test -tags integration -run 'A/B' ./cmd\n",
		"a pattern that doesn't compile":    "go test -tags integration -run '(' ./cmd\n",
		"-skip":                             "go test -tags integration -skip 'Slow' ./cmd\n",
		"an unknown flag":                   "go test -tags integration -race ./cmd\n",
		"a flag given twice":                "go test -tags integration -run 'A' -run 'B' ./cmd\n",
		"a flag with no value":              "go test ./cmd -tags\n",
		"a line continued onto the next":    "go test -tags integration \\\n  ./cmd\n",
		"a package that isn't a ./ pattern": "go test -tags integration github.com/leeovery/portal/cmd\n",
	} {
		t.Run("it refuses "+name, func(t *testing.T) {
			if runs, err := parseCheckRuns(workflow); err == nil {
				t.Errorf("parseCheckRuns = %+v, want an error", runs)
			}
		})
	}

	t.Run("a refusal names the line it can't judge", func(t *testing.T) {
		const line = "go test -tags integration -skip 'Slow' ./cmd"
		if _, err := parseCheckRuns(line + "\n"); err == nil || !strings.Contains(err.Error(), line) {
			t.Errorf("parseCheckRuns = %v, want an error naming %q", err, line)
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
