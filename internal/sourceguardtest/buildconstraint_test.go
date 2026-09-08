package sourceguardtest_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/leeovery/portal/internal/sourceguardtest"
)

func TestBuildConstraint(t *testing.T) {
	t.Run("it returns the parsed build constraint for a tagged file and reports none for an untagged one", func(t *testing.T) {
		cases := []struct {
			name   string
			src    string
			want   string
			tagged bool
		}{
			{
				name:   "a file gated on the integration tag",
				src:    "//go:build integration\n\npackage fixture\n",
				want:   "integration",
				tagged: true,
			},
			{
				name:   "a file gated against it",
				src:    "//go:build !integration\n\npackage fixture\n",
				want:   "!integration",
				tagged: true,
			},
			{
				name: "a file with no constraint at all",
				src:  "package fixture\n",
			},
			{
				name: "a file whose only doc comment precedes nothing",
				src:  "// fixture is a package.\npackage fixture\n",
			},
			{
				name: "a constraint stated after the package clause, where it constrains nothing",
				src:  "package fixture\n\n//go:build integration\n",
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				expr, found := sourceguardtest.BuildConstraint(parseFixture(t, tc.src))

				if found != tc.tagged {
					t.Fatalf("BuildConstraint reported found=%t for %q, want %t", found, tc.src, tc.tagged)
				}
				if !found {
					if expr != nil {
						t.Errorf("BuildConstraint returned the expression %s for a file stating no constraint", expr)
					}
					return
				}
				if got := expr.String(); got != tc.want {
					t.Errorf("BuildConstraint returned %s, want %s", got, tc.want)
				}
			})
		}
	})

	t.Run("it classifies an unparseable constraint the same way for every caller", func(t *testing.T) {
		expr, found := sourceguardtest.BuildConstraint(parseFixture(t, "//go:build integration &&\n\npackage fixture\n"))

		if found {
			t.Fatalf("BuildConstraint reported the unparseable constraint %s as read — a caller evaluating it would answer for a constraint nobody can state", expr)
		}
		if expr != nil {
			t.Errorf("BuildConstraint returned the expression %s alongside found=false", expr)
		}
	})
}

func TestSatisfiedWith(t *testing.T) {
	t.Run("it evaluates the integration tag against a parsed constraint", func(t *testing.T) {
		cases := []struct {
			name                string
			src                 string
			satisfiedWithTag    bool
			satisfiedWithoutTag bool
		}{
			{
				name:                "a file gated on the tag",
				src:                 "//go:build integration\n\npackage fixture\n",
				satisfiedWithTag:    true,
				satisfiedWithoutTag: false,
			},
			{
				name:                "a file gated against the tag",
				src:                 "//go:build !integration\n\npackage fixture\n",
				satisfiedWithTag:    false,
				satisfiedWithoutTag: true,
			},
			{
				name:                "a file gated on some other tag",
				src:                 "//go:build darwin\n\npackage fixture\n",
				satisfiedWithTag:    false,
				satisfiedWithoutTag: true,
			},
			{
				name:                "a file gated on the tag alongside another",
				src:                 "//go:build integration && darwin\n\npackage fixture\n",
				satisfiedWithTag:    false,
				satisfiedWithoutTag: false,
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				expr, found := sourceguardtest.BuildConstraint(parseFixture(t, tc.src))
				if !found {
					t.Fatalf("BuildConstraint read no constraint from %q", tc.src)
				}

				if got := sourceguardtest.SatisfiedWith(expr, sourceguardtest.IntegrationTag); got != tc.satisfiedWithTag {
					t.Errorf("SatisfiedWith(%s, %s) = %t, want %t", expr, sourceguardtest.IntegrationTag, got, tc.satisfiedWithTag)
				}
				if got := sourceguardtest.SatisfiedWithout(expr, sourceguardtest.IntegrationTag); got != tc.satisfiedWithoutTag {
					t.Errorf("SatisfiedWithout(%s, %s) = %t, want %t", expr, sourceguardtest.IntegrationTag, got, tc.satisfiedWithoutTag)
				}
			})
		}
	})
}

// parseFixture parses one miniature source under the mode every guard reads its
// files with, so a constraint stated in it is retained as a comment.
func parseFixture(t *testing.T, src string) *ast.File {
	t.Helper()

	file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", src, sourceguardtest.ParseMode)
	if err != nil {
		t.Fatalf("parse fixture source: %v", err)
	}
	return file
}
