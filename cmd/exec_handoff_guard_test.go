package cmd

import (
	"go/ast"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/sourceguardtest"
)

// The exec marker is the chain's last forensic record and the writer behind it
// is unbuffered, so a marker emitted after the exec is never written at all. A
// hand-off written anywhere else carries that ordering by hand and is proved by
// nothing — every test in the area fakes the exec, so omitting the marker fails
// no test and shows no symptom.
func TestExecSeamsAreCalledOnlyByTheHandOffHelpers(t *testing.T) {
	handOffs := map[string]int{"resumeHandOff": 0, "execHandOff": 0}

	for _, source := range sourceguardtest.ParsePackageSources(t, ".", false) {
		sourceguardtest.ForEachFuncCall(source.File, func(funcName string, call *ast.CallExpr) bool {
			if !isExecSeamCallee(sourceguardtest.CalleeName(call)) {
				return true
			}
			if _, permitted := handOffs[funcName]; !permitted {
				pos := source.Position(call.Pos())
				t.Errorf("%s:%d %s hands the process image over directly; route it through resumeHandOff or execHandOff, which emit the exec marker immediately before the exec",
					pos.Filename, pos.Line, funcName)
				return true
			}
			handOffs[funcName]++
			return true
		})
	}

	for name, calls := range handOffs {
		if calls == 0 {
			t.Errorf("%s makes no exec seam call, so this guard is judging a rule nothing exercises", name)
		}
	}
}

func isExecSeamCallee(name string) bool {
	return strings.EqualFold(name, "execSelf") || strings.EqualFold(name, "execShell")
}
