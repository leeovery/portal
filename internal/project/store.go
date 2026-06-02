// Package project provides persistence for remembered project directories.
package project

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"time"

	"github.com/leeovery/portal/internal/fileutil"
	"github.com/leeovery/portal/internal/log"
)

// logger is the projects-component logger, bound once at package init. Every
// projects.json mutation that flows through Upsert/Rename/Remove/CleanStale
// emits a single breadcrumb under this component so `grep "projects:"
// portal.log` reconstructs the change history. Importing internal/log
// introduces no cycle — internal/log depends only on the standard library.
//
// Attr-key convention (per the closed attr-key vocabulary): the `project` attr
// carries the project NAME, the `path` attr carries the filesystem PATH (which
// is also the addressable match key Upsert/Rename/Remove all key on), and the
// `value` attr carries the verbatim new value for set/modify.
//
// Message-shape: the op verb is BOTH the slog message (preserving the
// `projects: <verb>` catalog shape and grep idiom) AND a required "op" attr
// drawn from the closed value space (set / modify / rm / clean-stale), so JSON
// output and `grep op=set` filtering both work — see the hooks store for the
// full rationale.
var logger = log.For("projects")

// Project represents a remembered project directory.
type Project struct {
	Path     string    `json:"path"`
	Name     string    `json:"name"`
	LastUsed time.Time `json:"last_used"`
}

// projectsFile is the on-disk JSON structure for projects.json.
type projectsFile struct {
	Projects []Project `json:"projects"`
}

// Store manages persistence of project data to a JSON file.
type Store struct {
	path string
}

// NewStore creates a Store that reads and writes to the given file path.
func NewStore(path string) *Store {
	return &Store{path: path}
}

// Load reads projects from the JSON file.
// Returns an empty slice when the file is missing or contains malformed JSON.
func (s *Store) Load() ([]Project, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []Project{}, nil
		}
		return nil, err
	}

	var f projectsFile
	if err := json.Unmarshal(data, &f); err != nil {
		return []Project{}, nil
	}

	return f.Projects, nil
}

// Save writes projects to the JSON file using atomic write (temp file + rename).
// Creates the parent directory if it does not exist.
func (s *Store) Save(projects []Project) error {
	f := projectsFile{Projects: projects}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal projects: %w", err)
	}

	return fileutil.AtomicWrite(s.path, data)
}

// Upsert adds a new project or updates an existing one matched by path.
// The LastUsed timestamp is set to the current time. If the project already
// exists (matched by Path), its Name and LastUsed are updated.
//
// via records the mutation origin for the audit breadcrumb and is drawn from
// the closed value space cli / internal / migrate (internal for code-driven
// mutations such as the session-creation pipeline).
//
// Upsert emits one audit breadcrumb under the projects component, classified
// from the pre-write Load:
//   - path NOT found -> INFO "set"
//   - path FOUND     -> INFO "modify"
//
// A true set-noop (file unchanged) is effectively UNREACHABLE: Upsert always
// bumps LastUsed=now even when the name matches, so the file always changes.
// "path found" therefore maps to op="modify"; the set-noop op would only become
// reachable if the LastUsed bump were skipped — which we deliberately do NOT do
// (the timestamp behaviour is load-bearing for List ordering).
//
// On a persist failure the breadcrumb is WARN carrying the wrapped error and its
// write-failed-* error_class.
func (s *Store) Upsert(path, name, via string) error {
	projects, err := s.Load()
	if err != nil {
		return fmt.Errorf("failed to load projects: %w", err)
	}

	now := time.Now().UTC()
	found := false

	for i := range projects {
		if projects[i].Path == path {
			projects[i].Name = name
			projects[i].LastUsed = now
			found = true
			break
		}
	}

	// op classified from the pre-write Load: an existing path is op="modify",
	// a brand-new path is op="set". set-noop is unreachable (see doc comment).
	op := "set"
	if found {
		op = "modify"
	}

	if !found {
		projects = append(projects, Project{
			Path:     path,
			Name:     name,
			LastUsed: now,
		})
	}

	if err := s.Save(projects); err != nil {
		logger.Warn(op, "op", op, "project", name, "path", path, "value", name, "via", via,
			"error", err, "error_class", fileutil.ClassifyWriteError(err))
		return err
	}

	logger.Info(op, "op", op, "project", name, "path", path, "value", name, "via", via)
	return nil
}

// List returns all projects sorted by LastUsed in descending order (most recent first).
func (s *Store) List() ([]Project, error) {
	projects, err := s.Load()
	if err != nil {
		return nil, err
	}

	slices.SortFunc(projects, func(a, b Project) int {
		return b.LastUsed.Compare(a.LastUsed)
	})

	return projects, nil
}

// CleanStale removes projects whose directories no longer exist on disk.
// Projects with permission errors are retained. Returns the removed projects.
// The file is only saved if at least one project was removed.
//
// CleanStale is a batch mutation and follows the batch-summary breadcrumb shape:
// one DEBUG per removed project, then exactly one INFO summary (op clean-stale,
// entries=N, via=internal, took=<elapsed>) on a successful whole-batch Save, or
// one WARN carrying the wrapped error and its write-failed-* error_class on a
// Save failure. via is always "internal" because CleanStale is only ever invoked
// by code-driven cleanup, never a user-facing command.
//
// Zero-removal case (decision (a), documented): a clean that removes nothing is
// an idempotent no-op. It emits NO summary and performs NO Save — the
// batch-summary INFO is reserved for batches that did work and must not clutter
// the INFO baseline.
//
// [needs-info, resolved-in-comment] The spec's batch contract mentions a
// per-entry WARN with error_class=unexpected on a mid-loop failure. That has no
// reachable site here: the kept/removed partition is computed entirely in memory
// and persistence is a SINGLE batched Save of the kept slice. There is no point
// at which one entry can fail while the batch continues — the only failure mode
// is the whole-batch Save below, which is write-failed-* (not unexpected). We do
// NOT fabricate a synthetic per-entry failure path. (Same reasoning as the hooks
// store's CleanStale.)
func (s *Store) CleanStale() ([]Project, error) {
	start := time.Now()

	projects, err := s.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load projects: %w", err)
	}

	var kept []Project
	var removed []Project

	for _, p := range projects {
		_, statErr := os.Stat(p.Path)
		switch {
		case statErr == nil:
			kept = append(kept, p)
		case errors.Is(statErr, os.ErrNotExist):
			removed = append(removed, p)
		default:
			// Permission denied or other errors: retain the project
			kept = append(kept, p)
		}
	}

	// Zero-removal case: skip both the Save and the summary (decision (a)).
	if len(removed) == 0 {
		return removed, nil
	}

	for _, p := range removed {
		logger.Debug("clean-stale", "op", "clean-stale", "project", p.Name, "path", p.Path, "via", "internal")
	}

	if err := s.Save(kept); err != nil {
		// Whole-batch persist failure: error_class is a write-failed-* value
		// from the AtomicWrite phase space, NOT "unexpected".
		logger.Warn("clean-stale", "op", "clean-stale", "entries", len(removed), "via", "internal",
			"error", err, "error_class", fileutil.ClassifyWriteError(err), "took", time.Since(start))
		return nil, fmt.Errorf("failed to save after cleaning stale projects: %w", err)
	}

	// entries_failed is omitted: there is no per-entry failure path (see the
	// [needs-info] note above), so M is always 0 and the attr stays absent.
	logger.Info("clean-stale", "op", "clean-stale", "entries", len(removed), "via", "internal", "took", time.Since(start))

	return removed, nil
}

// Rename updates the display name of the project matched by path.
// It does not change the LastUsed timestamp. It is a no-op if the path is not found.
//
// via records the mutation origin for the audit breadcrumb (cli for the
// user-facing TUI rename).
//
// When the path is absent, Rename returns nil WITHOUT a Save and emits NOTHING
// — there is no mutation to audit. When the path is found, after Save it emits
// one breadcrumb under the projects component: INFO "modify" on success, or WARN
// "modify" with the wrapped error and its error_class on a persist failure.
func (s *Store) Rename(path, newName, via string) error {
	projects, err := s.Load()
	if err != nil {
		return fmt.Errorf("failed to load projects: %w", err)
	}

	for i := range projects {
		if projects[i].Path == path {
			projects[i].Name = newName
			if err := s.Save(projects); err != nil {
				logger.Warn("modify", "op", "modify", "project", newName, "path", path, "value", newName, "via", via,
					"error", err, "error_class", fileutil.ClassifyWriteError(err))
				return err
			}
			logger.Info("modify", "op", "modify", "project", newName, "path", path, "value", newName, "via", via)
			return nil
		}
	}

	// Path absent: no-op, no Save, no breadcrumb (nothing was mutated).
	return nil
}

// Remove deletes the project with the given path. It is a no-op if the path
// is not found.
//
// via records the mutation origin for the audit breadcrumb (cli for the
// user-facing TUI delete).
//
// Remove always rewrites the file via Save — even removing an absent path
// re-persists the (unchanged) filtered slice — so it always emits one breadcrumb
// under the projects component: INFO "rm" (no value attr) on success, or WARN
// "rm" with the wrapped error and its error_class on a persist failure. The
// absent-path case still Saves, so it still emits the INFO.
func (s *Store) Remove(path, via string) error {
	projects, err := s.Load()
	if err != nil {
		return fmt.Errorf("failed to load projects: %w", err)
	}

	// Resolve the removed entry's name (for the project attr) before deleting.
	// An absent path has no matching entry, so name stays empty.
	var name string
	for _, p := range projects {
		if p.Path == path {
			name = p.Name
			break
		}
	}

	filtered := slices.DeleteFunc(projects, func(p Project) bool {
		return p.Path == path
	})

	if err := s.Save(filtered); err != nil {
		logger.Warn("rm", "op", "rm", "project", name, "path", path, "via", via,
			"error", err, "error_class", fileutil.ClassifyWriteError(err))
		return err
	}

	logger.Info("rm", "op", "rm", "project", name, "path", path, "via", via)
	return nil
}
