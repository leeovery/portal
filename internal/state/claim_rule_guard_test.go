package state_test

import (
	"go/ast"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"

	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/sourceguardtest"
)

// Every claim judges by the one rule state.ClaimRule builds. A claimer that
// built its own could replace, and so forget, an entry the rule keeps.
func TestNoProductionClaimRuleOutsideState(t *testing.T) {
	for _, finding := range scanForOwnClaimRule(t) {
		t.Errorf("%s %s; take state.ClaimRule(dir)", finding.path, finding.name)
	}
}

func TestClaimRuleGuard_Rule(t *testing.T) {
	const callsNewRule = `package fixture

import "github.com/leeovery/portal/internal/sessionindex"

func rule() sessionindex.Rule { return sessionindex.NewRule(boot, nil) }
`
	const aliasedNewRule = `package fixture

import idx "github.com/leeovery/portal/internal/sessionindex"

var build = idx.NewRule
`
	const literalRule = `package fixture

import "github.com/leeovery/portal/internal/sessionindex"

func claim(idx *sessionindex.Index) { idx.Claim("a", "", at, 0, sessionindex.Rule{}) }
`
	const zeroVarRule = `package fixture

import "github.com/leeovery/portal/internal/sessionindex"

func claim(idx *sessionindex.Index) {
	var rule sessionindex.Rule
	idx.Claim("a", "", at, 0, rule)
}
`
	const passesRule = `package fixture

import (
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/state"
)

func claim(dir string, idx *sessionindex.Index, rule sessionindex.Rule) {
	var taken sessionindex.Rule = state.ClaimRule(dir)
	idx.Claim("a", "", at, 0, taken)
	idx.Claim("b", "", at, 0, rule)
}
`
	const unrelated = `package fixture

import "github.com/leeovery/portal/internal/other/sessionindex"

var rule = sessionindex.NewRule(sessionindex.Rule{})
`
	const clean = "package fixture\n"

	for _, tc := range []struct {
		name         string
		files        map[string]string
		wantFindings []guardFinding
	}{
		{
			name:         "it fails for a production file outside internal/state calling sessionindex.NewRule",
			files:        map[string]string{filepath.Join("cmd", "claim.go"): callsNewRule},
			wantFindings: []guardFinding{{filepath.Join("cmd", "claim.go"), "calls sessionindex.NewRule"}},
		},
		{
			name:         "it fails for a production file taking NewRule as a value under an import alias",
			files:        map[string]string{filepath.Join("internal", "restore", "claim.go"): aliasedNewRule},
			wantFindings: []guardFinding{{filepath.Join("internal", "restore", "claim.go"), "calls sessionindex.NewRule"}},
		},
		{
			name:         "it fails for a production file composing a Rule",
			files:        map[string]string{filepath.Join("cmd", "claim.go"): literalRule},
			wantFindings: []guardFinding{{filepath.Join("cmd", "claim.go"), "composes a sessionindex.Rule"}},
		},
		{
			name:         "it fails for a production file declaring a zero Rule",
			files:        map[string]string{filepath.Join("cmd", "claim.go"): zeroVarRule},
			wantFindings: []guardFinding{{filepath.Join("cmd", "claim.go"), "declares a zero sessionindex.Rule"}},
		},
		{
			name: "it ignores a rule taken from state.ClaimRule or passed in",
			files: map[string]string{
				filepath.Join("cmd", "claim.go"): passesRule,
			},
		},
		{
			name: "it ignores test files, and production files inside internal/state and internal/sessionindex",
			files: map[string]string{
				filepath.Join("cmd", "claim_test.go"):                   callsNewRule,
				filepath.Join("internal", "state", "claim_rule.go"):     callsNewRule,
				filepath.Join("internal", "sessionindex", "claim.go"):   literalRule,
				filepath.Join("internal", "sessionindex", "claim_x.go"): zeroVarRule,
				filepath.Join("cmd", "root.go"):                         clean,
			},
		},
		{
			name:  "it ignores a package of the same name elsewhere",
			files: map[string]string{filepath.Join("cmd", "other.go"): unrelated},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := stageClaimRuleTree(t, tc.files)

			findings := scanForOwnClaimRule(t, sourceguardtest.Rooted(root))

			if !slices.Equal(findings, tc.wantFindings) {
				t.Errorf("findings = %v, want %v", findings, tc.wantFindings)
			}
		})
	}
}

// A guard whose scan came back empty would report a clean tree forever.
func TestClaimRuleGuard_FatalsWhenItScansNothing(t *testing.T) {
	recorder := &harnesstest.Recorder{}
	recorder.Run(func() {
		scanForOwnClaimRule(recorder, sourceguardtest.Rooted(t.TempDir()))
	})

	if len(recorder.Fatals) != 1 {
		t.Fatalf("scan of a tree holding no sources reported %d fatals, want 1: %v", len(recorder.Fatals), recorder.Fatals)
	}
}

const sessionindexPkgPath = "github.com/leeovery/portal/internal/sessionindex"

type guardFinding struct {
	path string
	name string
}

// scanForOwnClaimRule reports every non-test file outside internal/state and
// internal/sessionindex that makes a claim rule itself: by NewRule, called or
// taken as a value, by a Rule literal, or by a Rule variable left zero.
func scanForOwnClaimRule(t harnesstest.TestingT, opts ...sourceguardtest.ScanOption) []guardFinding {
	t.Helper()

	_, sources := sourceguardtest.RepoSources(t, sourceguardtest.NonTestSources, opts...)

	var findings []guardFinding
	for _, source := range sources {
		switch filepath.Dir(source.Path) {
		case filepath.Join("internal", "state"), filepath.Join("internal", "sessionindex"):
			continue
		}
		for _, how := range ownClaimRulesIn(source.File) {
			findings = append(findings, guardFinding{source.Path, how})
		}
	}
	return findings
}

// ownClaimRulesIn says how file makes a claim rule itself, each way once, in
// sorted order.
func ownClaimRulesIn(file *ast.File) []string {
	local, ok := importName(file, sessionindexPkgPath, "sessionindex")
	if !ok {
		return nil
	}
	isSelector := func(e ast.Expr, name string) bool {
		sel, isSel := e.(*ast.SelectorExpr)
		if !isSel || sel.Sel.Name != name {
			return false
		}
		pkg, isIdent := sel.X.(*ast.Ident)
		return isIdent && pkg.Name == local
	}
	var hows []string
	note := func(how string) {
		if !slices.Contains(hows, how) {
			hows = append(hows, how)
		}
	}
	ast.Inspect(file, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.SelectorExpr:
			if isSelector(n, "NewRule") {
				note("calls sessionindex.NewRule")
			}
		case *ast.CompositeLit:
			if isSelector(n.Type, "Rule") {
				note("composes a sessionindex.Rule")
			}
		case *ast.ValueSpec:
			if len(n.Values) == 0 && isSelector(n.Type, "Rule") {
				note("declares a zero sessionindex.Rule")
			}
		}
		return true
	})
	slices.Sort(hows)
	return hows
}

func importName(file *ast.File, pkgPath, name string) (string, bool) {
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil || path != pkgPath {
			continue
		}
		if spec.Name != nil {
			return spec.Name.Name, true
		}
		return name, true
	}
	return "", false
}

func stageClaimRuleTree(t *testing.T, files map[string]string) string {
	t.Helper()

	root := t.TempDir()
	for name, body := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("stage %s: %v", name, err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return root
}
