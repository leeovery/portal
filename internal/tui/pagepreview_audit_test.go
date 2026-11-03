package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPreviewLayerAudit_NoPortalSaverReferences(t *testing.T) {
	const forbidden = "_portal-saver"

	patterns := []string{"pagepreview*.go", "preview_*.go"}
	var sourceFiles []string
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatalf("glob %q: %v", pattern, err)
		}
		for _, path := range matches {
			if strings.HasSuffix(path, "_test.go") {
				continue
			}
			sourceFiles = append(sourceFiles, path)
		}
	}

	if len(sourceFiles) == 0 {
		t.Fatalf("preview-layer audit found zero source files; glob patterns are out of date with the package layout")
	}

	for _, path := range sourceFiles {
		t.Run(path, func(t *testing.T) {
			contents, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			if strings.Contains(string(contents), forbidden) {
				t.Errorf(
					"preview source file %s mentions %q; "+
						"the preview layer must not introduce a name-based blacklist. "+
						"Exclusion belongs to the Sessions-list source (userSessions in cmd/backend.go).",
					path, forbidden,
				)
			}
		})
	}
}

func TestPreviewLayerAudit_ExclusionAppliedAtSource_NotPreviewLayer(t *testing.T) {
	const forbidden = "_portal-saver"

	previewModel := "pagepreview.go"
	previewBytes, err := os.ReadFile(previewModel)
	if err != nil {
		t.Fatalf("read %s: %v", previewModel, err)
	}
	if strings.Contains(string(previewBytes), forbidden) {
		t.Errorf("preview model file %s mentions %q; preview must not introduce name-based suppression", previewModel, forbidden)
	}

	listSource := filepath.Join("..", "..", "cmd", "backend.go")
	listBytes, err := os.ReadFile(listSource)
	if err != nil {
		t.Fatalf("read %s: %v", listSource, err)
	}
	listSrc := string(listBytes)

	if !strings.Contains(listSrc, "slices.DeleteFunc(sessions, backend.Session.PortalOwned)") {
		t.Errorf(
			"canonical Sessions-list filter at %s no longer contains "+
				"slices.DeleteFunc(sessions, backend.Session.PortalOwned) — the "+
				"Portal-owned exclusion has been removed or relocated. Exclusion "+
				"must remain at the list-population source.",
			listSource,
		)
	}
}
