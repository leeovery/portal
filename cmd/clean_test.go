package cmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCleanCommand(t *testing.T) {
	t.Run("removes stale project and prints removal message", func(t *testing.T) {
		dir := t.TempDir()
		projectsFile := filepath.Join(dir, "projects.json")
		t.Setenv("PORTAL_PROJECTS_FILE", projectsFile)
		t.Setenv("PORTAL_HOOKS_FILE", filepath.Join(t.TempDir(), "hooks.json"))

		stalePath := filepath.Join(dir, "gone")
		content := `{"projects":[{"path":"` + stalePath + `","name":"stale","last_used":"2026-01-01T00:00:00Z"}]}`
		if err := os.WriteFile(projectsFile, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		buf := new(bytes.Buffer)
		resetRootCmd()
		rootCmd.SetOut(buf)
		rootCmd.SetArgs([]string{"clean"})

		err := rootCmd.Execute()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := "Removed stale project: stale (" + stalePath + ")\n"
		if buf.String() != want {
			t.Errorf("output = %q, want %q", buf.String(), want)
		}

		// Verify the stale project was actually removed from the file
		data, err := os.ReadFile(projectsFile)
		if err != nil {
			t.Fatalf("failed to read projects file: %v", err)
		}
		if bytes.Contains(data, []byte("stale")) {
			t.Errorf("stale project should have been removed from the store")
		}
	})

	t.Run("keeps project with existing directory and produces no output for it", func(t *testing.T) {
		dir := t.TempDir()
		projectsFile := filepath.Join(dir, "projects.json")
		t.Setenv("PORTAL_PROJECTS_FILE", projectsFile)
		t.Setenv("PORTAL_HOOKS_FILE", filepath.Join(t.TempDir(), "hooks.json"))

		existingDir := t.TempDir()
		content := `{"projects":[{"path":"` + existingDir + `","name":"exists","last_used":"2026-01-01T00:00:00Z"}]}`
		if err := os.WriteFile(projectsFile, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		buf := new(bytes.Buffer)
		resetRootCmd()
		rootCmd.SetOut(buf)
		rootCmd.SetArgs([]string{"clean"})

		err := rootCmd.Execute()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if buf.String() != "" {
			t.Errorf("output = %q, want empty string", buf.String())
		}

		// Verify project is still in the store
		data, err := os.ReadFile(projectsFile)
		if err != nil {
			t.Fatalf("failed to read projects file: %v", err)
		}
		if !bytes.Contains(data, []byte(existingDir)) {
			t.Errorf("existing project should still be in the store")
		}
	})

	t.Run("keeps project with permission error", func(t *testing.T) {
		dir := t.TempDir()
		projectsFile := filepath.Join(dir, "projects.json")
		t.Setenv("PORTAL_PROJECTS_FILE", projectsFile)
		t.Setenv("PORTAL_HOOKS_FILE", filepath.Join(t.TempDir(), "hooks.json"))

		// Create a parent dir, then a child inside it, then remove perms on parent
		parentDir := filepath.Join(dir, "restricted")
		childDir := filepath.Join(parentDir, "child")
		if err := os.MkdirAll(childDir, 0o755); err != nil {
			t.Fatalf("failed to create child dir: %v", err)
		}
		if err := os.Chmod(parentDir, 0o000); err != nil {
			t.Fatalf("failed to chmod: %v", err)
		}
		t.Cleanup(func() {
			_ = os.Chmod(parentDir, 0o755)
		})

		content := `{"projects":[{"path":"` + childDir + `","name":"restricted","last_used":"2026-01-01T00:00:00Z"}]}`
		if err := os.WriteFile(projectsFile, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		buf := new(bytes.Buffer)
		resetRootCmd()
		rootCmd.SetOut(buf)
		rootCmd.SetArgs([]string{"clean"})

		err := rootCmd.Execute()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// No removal message for permission errors
		if buf.String() != "" {
			t.Errorf("output = %q, want empty string", buf.String())
		}

		// Verify project is still in the store
		data, err := os.ReadFile(projectsFile)
		if err != nil {
			t.Fatalf("failed to read projects file: %v", err)
		}
		if !bytes.Contains(data, []byte("restricted")) {
			t.Errorf("restricted project should still be in the store")
		}
	})

	t.Run("no stale projects produces no output", func(t *testing.T) {
		dir := t.TempDir()
		projectsFile := filepath.Join(dir, "projects.json")
		t.Setenv("PORTAL_PROJECTS_FILE", projectsFile)
		t.Setenv("PORTAL_HOOKS_FILE", filepath.Join(t.TempDir(), "hooks.json"))

		existingDir1 := t.TempDir()
		existingDir2 := t.TempDir()
		content := `{"projects":[
			{"path":"` + existingDir1 + `","name":"first","last_used":"2026-01-01T00:00:00Z"},
			{"path":"` + existingDir2 + `","name":"second","last_used":"2026-02-01T00:00:00Z"}
		]}`
		if err := os.WriteFile(projectsFile, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		buf := new(bytes.Buffer)
		resetRootCmd()
		rootCmd.SetOut(buf)
		rootCmd.SetArgs([]string{"clean"})

		err := rootCmd.Execute()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if buf.String() != "" {
			t.Errorf("output = %q, want empty string", buf.String())
		}
	})

	t.Run("all projects stale removes all and prints each", func(t *testing.T) {
		dir := t.TempDir()
		projectsFile := filepath.Join(dir, "projects.json")
		t.Setenv("PORTAL_PROJECTS_FILE", projectsFile)
		t.Setenv("PORTAL_HOOKS_FILE", filepath.Join(t.TempDir(), "hooks.json"))

		stalePath1 := filepath.Join(dir, "gone1")
		stalePath2 := filepath.Join(dir, "gone2")
		content := `{"projects":[
			{"path":"` + stalePath1 + `","name":"stale1","last_used":"2026-01-01T00:00:00Z"},
			{"path":"` + stalePath2 + `","name":"stale2","last_used":"2026-02-01T00:00:00Z"}
		]}`
		if err := os.WriteFile(projectsFile, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		buf := new(bytes.Buffer)
		resetRootCmd()
		rootCmd.SetOut(buf)
		rootCmd.SetArgs([]string{"clean"})

		err := rootCmd.Execute()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := "Removed stale project: stale1 (" + stalePath1 + ")\nRemoved stale project: stale2 (" + stalePath2 + ")\n"
		if buf.String() != want {
			t.Errorf("output = %q, want %q", buf.String(), want)
		}

		// Verify projects.json is empty (neither stale project remains)
		data, err := os.ReadFile(projectsFile)
		if err != nil {
			t.Fatalf("failed to read projects file: %v", err)
		}
		if bytes.Contains(data, []byte("gone1")) {
			t.Errorf("stale1 should have been removed from the store")
		}
		if bytes.Contains(data, []byte("gone2")) {
			t.Errorf("stale2 should have been removed from the store")
		}
	})

	t.Run("multiple stale projects each printed", func(t *testing.T) {
		dir := t.TempDir()
		projectsFile := filepath.Join(dir, "projects.json")
		t.Setenv("PORTAL_PROJECTS_FILE", projectsFile)
		t.Setenv("PORTAL_HOOKS_FILE", filepath.Join(t.TempDir(), "hooks.json"))

		existingDir := t.TempDir()
		stalePath1 := filepath.Join(dir, "gone1")
		stalePath2 := filepath.Join(dir, "gone2")
		content := `{"projects":[
			{"path":"` + existingDir + `","name":"exists","last_used":"2026-01-01T00:00:00Z"},
			{"path":"` + stalePath1 + `","name":"stale1","last_used":"2026-02-01T00:00:00Z"},
			{"path":"` + stalePath2 + `","name":"stale2","last_used":"2026-03-01T00:00:00Z"}
		]}`
		if err := os.WriteFile(projectsFile, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		buf := new(bytes.Buffer)
		resetRootCmd()
		rootCmd.SetOut(buf)
		rootCmd.SetArgs([]string{"clean"})

		err := rootCmd.Execute()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := "Removed stale project: stale1 (" + stalePath1 + ")\nRemoved stale project: stale2 (" + stalePath2 + ")\n"
		if buf.String() != want {
			t.Errorf("output = %q, want %q", buf.String(), want)
		}

		// Verify only the existing project remains
		data, err := os.ReadFile(projectsFile)
		if err != nil {
			t.Fatalf("failed to read projects file: %v", err)
		}
		if !bytes.Contains(data, []byte(existingDir)) {
			t.Errorf("existing project should still be in the store")
		}
		if bytes.Contains(data, []byte("gone1")) {
			t.Errorf("stale1 should have been removed from the store")
		}
		if bytes.Contains(data, []byte("gone2")) {
			t.Errorf("stale2 should have been removed from the store")
		}
	})

	t.Run("exit code 0 in all cases", func(t *testing.T) {
		// Empty projects file
		dir := t.TempDir()
		projectsFile := filepath.Join(dir, "projects.json")
		t.Setenv("PORTAL_PROJECTS_FILE", projectsFile)
		t.Setenv("PORTAL_HOOKS_FILE", filepath.Join(t.TempDir(), "hooks.json"))

		buf := new(bytes.Buffer)
		resetRootCmd()
		rootCmd.SetOut(buf)
		rootCmd.SetArgs([]string{"clean"})

		err := rootCmd.Execute()

		if err != nil {
			t.Fatalf("expected exit 0 (no error), got: %v", err)
		}
	})

	t.Run("removes stale hooks and prints removal messages", func(t *testing.T) {
		dir := t.TempDir()
		projectsFile := filepath.Join(dir, "projects.json")
		t.Setenv("PORTAL_PROJECTS_FILE", projectsFile)
		hooksFile := filepath.Join(dir, "hooks.json")
		t.Setenv("PORTAL_HOOKS_FILE", hooksFile)

		// Write hooks for two panes; only my-session:0.0 is live
		writeHooksJSON(t, hooksFile, map[string]map[string]string{
			"my-session:0.0":    {"on-resume": "cmd1"},
			"other-session:1.0": {"on-resume": "cmd5"},
		})

		cleanDeps = &CleanDeps{
			AllPaneLister: &mockCleanPaneLister{panes: []string{"my-session:0.0"}},
		}
		t.Cleanup(func() { cleanDeps = nil })

		buf := new(bytes.Buffer)
		resetRootCmd()
		rootCmd.SetOut(buf)
		rootCmd.SetArgs([]string{"clean"})

		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := "Removed stale hook: other-session:1.0\n"
		if buf.String() != want {
			t.Errorf("output = %q, want %q", buf.String(), want)
		}

		// Verify other-session:1.0 was removed from hooks file, my-session:0.0 remains
		data := readHooksJSON(t, hooksFile)
		if _, ok := data["my-session:0.0"]; !ok {
			t.Error("expected live pane my-session:0.0 to remain in hooks file")
		}
		if _, ok := data["other-session:1.0"]; ok {
			t.Error("expected stale pane other-session:1.0 to be removed from hooks file")
		}
	})

	t.Run("zero live panes prunes every hook entry", func(t *testing.T) {
		dir := t.TempDir()
		projectsFile := filepath.Join(dir, "projects.json")
		t.Setenv("PORTAL_PROJECTS_FILE", projectsFile)
		hooksFile := filepath.Join(dir, "hooks.json")
		t.Setenv("PORTAL_HOOKS_FILE", hooksFile)

		// Phase 4: CleanStale runs unconditionally. With no live panes,
		// every hooks.json entry is genuinely orphaned and must be pruned.
		writeHooksJSON(t, hooksFile, map[string]map[string]string{
			"my-session:0.1":    {"on-resume": "cmd-1"},
			"other-session:1.0": {"on-resume": "cmd-2"},
		})

		cleanDeps = &CleanDeps{
			AllPaneLister: &mockCleanPaneLister{panes: []string{}},
		}
		t.Cleanup(func() { cleanDeps = nil })

		buf := new(bytes.Buffer)
		resetRootCmd()
		rootCmd.SetOut(buf)
		rootCmd.SetArgs([]string{"clean"})

		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		out := buf.String()
		if !strings.Contains(out, "Removed stale hook: my-session:0.1") {
			t.Errorf("output = %q, want containing my-session:0.1 removal", out)
		}
		if !strings.Contains(out, "Removed stale hook: other-session:1.0") {
			t.Errorf("output = %q, want containing other-session:1.0 removal", out)
		}

		data := readHooksJSON(t, hooksFile)
		if len(data) != 0 {
			t.Errorf("expected hooks file to be emptied; got %v", data)
		}
	})

	t.Run("ListAllPanes error preserves hooks (safety net)", func(t *testing.T) {
		dir := t.TempDir()
		projectsFile := filepath.Join(dir, "projects.json")
		t.Setenv("PORTAL_PROJECTS_FILE", projectsFile)
		hooksFile := filepath.Join(dir, "hooks.json")
		t.Setenv("PORTAL_HOOKS_FILE", hooksFile)

		writeHooksJSON(t, hooksFile, map[string]map[string]string{
			"my-session:0.1": {"on-resume": "some-cmd"},
		})

		cleanDeps = &CleanDeps{
			AllPaneLister: &mockCleanPaneLister{err: errors.New("tmux dead")},
		}
		t.Cleanup(func() { cleanDeps = nil })

		buf := new(bytes.Buffer)
		resetRootCmd()
		rootCmd.SetOut(buf)
		rootCmd.SetArgs([]string{"clean"})

		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if buf.String() != "" {
			t.Errorf("output = %q, want empty string when ListAllPanes errors", buf.String())
		}

		data := readHooksJSON(t, hooksFile)
		if _, ok := data["my-session:0.1"]; !ok {
			t.Error("expected hook my-session:0.1 to be preserved when ListAllPanes errors")
		}
	})

	t.Run("hooks file missing produces no hook removal output", func(t *testing.T) {
		dir := t.TempDir()
		projectsFile := filepath.Join(dir, "projects.json")
		t.Setenv("PORTAL_PROJECTS_FILE", projectsFile)
		hooksFile := filepath.Join(dir, "hooks.json")
		t.Setenv("PORTAL_HOOKS_FILE", hooksFile)
		// Do NOT create the hooks file

		cleanDeps = &CleanDeps{
			AllPaneLister: &mockCleanPaneLister{panes: []string{"my-session:0.0"}},
		}
		t.Cleanup(func() { cleanDeps = nil })

		buf := new(bytes.Buffer)
		resetRootCmd()
		rootCmd.SetOut(buf)
		rootCmd.SetArgs([]string{"clean"})

		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if buf.String() != "" {
			t.Errorf("output = %q, want empty string", buf.String())
		}
	})

	t.Run("all hooks panes still live produces no hook removal output", func(t *testing.T) {
		dir := t.TempDir()
		projectsFile := filepath.Join(dir, "projects.json")
		t.Setenv("PORTAL_PROJECTS_FILE", projectsFile)
		hooksFile := filepath.Join(dir, "hooks.json")
		t.Setenv("PORTAL_HOOKS_FILE", hooksFile)

		writeHooksJSON(t, hooksFile, map[string]map[string]string{
			"my-session:0.0": {"on-resume": "cmd1"},
			"my-session:0.1": {"on-resume": "cmd3"},
		})

		cleanDeps = &CleanDeps{
			AllPaneLister: &mockCleanPaneLister{panes: []string{"my-session:0.0", "my-session:0.1"}},
		}
		t.Cleanup(func() { cleanDeps = nil })

		buf := new(bytes.Buffer)
		resetRootCmd()
		rootCmd.SetOut(buf)
		rootCmd.SetArgs([]string{"clean"})

		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if buf.String() != "" {
			t.Errorf("output = %q, want empty string", buf.String())
		}
	})

	t.Run("both project and hook removals printed together", func(t *testing.T) {
		dir := t.TempDir()
		projectsFile := filepath.Join(dir, "projects.json")
		t.Setenv("PORTAL_PROJECTS_FILE", projectsFile)
		hooksFile := filepath.Join(dir, "hooks.json")
		t.Setenv("PORTAL_HOOKS_FILE", hooksFile)

		// Stale project
		stalePath := filepath.Join(dir, "gone")
		content := `{"projects":[{"path":"` + stalePath + `","name":"stale","last_used":"2026-01-01T00:00:00Z"}]}`
		if err := os.WriteFile(projectsFile, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		// Stale hook: other-session:1.1 is not live
		writeHooksJSON(t, hooksFile, map[string]map[string]string{
			"other-session:1.1": {"on-resume": "cmd9"},
		})

		cleanDeps = &CleanDeps{
			AllPaneLister: &mockCleanPaneLister{panes: []string{"my-session:0.0"}},
		}
		t.Cleanup(func() { cleanDeps = nil })

		buf := new(bytes.Buffer)
		resetRootCmd()
		rootCmd.SetOut(buf)
		rootCmd.SetArgs([]string{"clean"})

		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := "Removed stale project: stale (" + stalePath + ")\nRemoved stale hook: other-session:1.1\n"
		if buf.String() != want {
			t.Errorf("output = %q, want %q", buf.String(), want)
		}
	})

	// Spec-pinned regression: hook stale-detection is purely structural-key
	// mismatch against `list-panes -a`. Whether the hook command's binary
	// exists on disk is NOT a staleness signal. Per phase-4-tasks.md
	// acceptance bullet "Binary-missing and projects.json-absent are NOT
	// staleness signals". Cycle-1 review remediation gap-fill.
	t.Run("keeps hook with missing-binary command when structural key is live", func(t *testing.T) {
		dir := t.TempDir()
		projectsFile := filepath.Join(dir, "projects.json")
		t.Setenv("PORTAL_PROJECTS_FILE", projectsFile)
		hooksFile := filepath.Join(dir, "hooks.json")
		t.Setenv("PORTAL_HOOKS_FILE", hooksFile)

		// Hook command references a binary that almost certainly does not
		// exist on the test host. The clean path must NOT remove this hook
		// because its structural key matches a live pane.
		writeHooksJSON(t, hooksFile, map[string]map[string]string{
			"my-session:0.0": {"on-resume": "/nonexistent/no-such-binary --resume"},
		})

		cleanDeps = &CleanDeps{
			AllPaneLister: &mockCleanPaneLister{panes: []string{"my-session:0.0"}},
		}
		t.Cleanup(func() { cleanDeps = nil })

		buf := new(bytes.Buffer)
		resetRootCmd()
		rootCmd.SetOut(buf)
		rootCmd.SetArgs([]string{"clean"})

		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if buf.String() != "" {
			t.Errorf("output = %q, want empty (missing-binary is not a staleness signal)", buf.String())
		}

		data := readHooksJSON(t, hooksFile)
		if _, ok := data["my-session:0.0"]; !ok {
			t.Error("hook with missing-binary command must NOT be pruned when its structural key is live")
		}
	})

	// Spec-pinned regression: a hook for a live pane must be retained even
	// when projects.json has no entry for that pane's project. Hooks are
	// keyed by structural pane key, not by project membership. Per
	// phase-4-tasks.md acceptance bullet "Binary-missing and projects.json-
	// absent are NOT staleness signals". Cycle-1 review remediation gap-fill.
	t.Run("keeps hook when projects.json absent and structural key is live", func(t *testing.T) {
		dir := t.TempDir()
		projectsFile := filepath.Join(dir, "projects.json")
		t.Setenv("PORTAL_PROJECTS_FILE", projectsFile)
		hooksFile := filepath.Join(dir, "hooks.json")
		t.Setenv("PORTAL_HOOKS_FILE", hooksFile)

		// projects.json is intentionally NOT created. The clean path must
		// still preserve the hook because its structural key matches a live
		// pane — projects.json membership is not a staleness input.
		writeHooksJSON(t, hooksFile, map[string]map[string]string{
			"my-session:0.0": {"on-resume": "echo hello"},
		})

		cleanDeps = &CleanDeps{
			AllPaneLister: &mockCleanPaneLister{panes: []string{"my-session:0.0"}},
		}
		t.Cleanup(func() { cleanDeps = nil })

		buf := new(bytes.Buffer)
		resetRootCmd()
		rootCmd.SetOut(buf)
		rootCmd.SetArgs([]string{"clean"})

		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if buf.String() != "" {
			t.Errorf("output = %q, want empty (projects.json absence is not a staleness signal)", buf.String())
		}

		data := readHooksJSON(t, hooksFile)
		if _, ok := data["my-session:0.0"]; !ok {
			t.Error("hook for live pane must NOT be pruned when projects.json is absent")
		}
	})
}

// mockCleanPaneLister implements AllPaneLister for clean command tests.
type mockCleanPaneLister struct {
	panes []string
	err   error
}

func (m *mockCleanPaneLister) ListAllPanes() ([]string, error) {
	return m.panes, m.err
}
