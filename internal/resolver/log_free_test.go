package resolver

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestResolver_DoesNotEmitLogs guards the spec invariant that internal/resolver
// stays a pure, log-free library: the resolution-decision line is emitted only
// from the open command body (cmd/open.go), where resolution is driven — never
// from the resolver itself.
//
// The guard is precise rather than blanket: it forbids the emission surface (a
// log/slog import or a log.For component binding), NOT every reference to
// internal/log. The resolver legitimately imports internal/log for the
// non-emitting exec.CombinedOutputWithContext boundary helper (gitroot.go); that
// is not logging and stays permitted.
func TestResolver_DoesNotEmitLogs(t *testing.T) {
	fset := token.NewFileSet()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}

		af, err := parser.ParseFile(fset, f, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", f, err)
		}
		for _, imp := range af.Imports {
			if strings.Trim(imp.Path.Value, `"`) == "log/slog" {
				t.Errorf("%s imports \"log/slog\" — internal/resolver must not emit logs; the resolve decision line is emitted only in cmd/open.go", f)
			}
		}

		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		if strings.Contains(string(src), "log.For(") {
			t.Errorf("%s binds a log component via log.For — internal/resolver must not emit logs; the resolve decision line is emitted only in cmd/open.go", f)
		}
	}
}
