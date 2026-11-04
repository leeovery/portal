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
	"github.com/leeovery/portal/internal/storelog"
)

// The op verb is both the slog message and a required "op" attr, so the
// `projects: <verb>` grep idiom and `grep op=set` filtering both work.
var logger = log.For("projects")

type Project struct {
	Path     string    `json:"path"`
	Name     string    `json:"name"`
	LastUsed time.Time `json:"last_used"`
	Tags     []string  `json:"tags,omitempty"`
}

type projectsFile struct {
	Projects []Project `json:"projects"`
}

type Store struct {
	path string
}

func NewStore(path string) *Store {
	return &Store{path: path}
}

// Load reads projects from the JSON file, returning an empty slice when the
// file is missing or holds malformed JSON.
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

// Save atomically writes projects to the JSON file, creating the parent
// directory if it does not exist.
func (s *Store) Save(projects []Project) error {
	f := projectsFile{Projects: projects}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal projects: %w", err)
	}

	return fileutil.AtomicWrite(s.path, data, fileutil.ReportUnsynced(warnUnsynced))
}

// warnUnsynced logs a write that landed though the directory sync after its
// rename failed: the mutation stands, and logs as written.
func warnUnsynced(err error) {
	logger.Warn("unsynced", "op", "unsynced", "error", err, "error_class", fileutil.ClassifyWriteError(err))
}

// Upsert adds a new project or updates the one matched by path, always bumping
// LastUsed to now — List orders on that timestamp, so the bump must not be
// skipped for an unchanged name. via records the mutation origin for the audit
// breadcrumb: cli, internal or migrate.
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

// List returns all projects sorted by LastUsed, most recent first.
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

// StaleEntries returns the records whose directory no longer exists on disk. It
// is read-only, and a stat error other than ErrNotExist retains the entry.
func (s *Store) StaleEntries() ([]Project, error) {
	projects, err := s.Load()
	if err != nil {
		return nil, err
	}
	_, removed := partitionByExistence(projects)
	return removed, nil
}

func partitionByExistence(projects []Project) (kept, removed []Project) {
	for _, p := range projects {
		_, statErr := os.Stat(p.Path)
		switch {
		case statErr == nil:
			kept = append(kept, p)
		case errors.Is(statErr, os.ErrNotExist):
			removed = append(removed, p)
		default:
			// An unreadable directory (permission denied) is not evidence of staleness.
			kept = append(kept, p)
		}
	}
	return kept, removed
}

// CleanStale removes and returns the projects whose directories no longer exist,
// retaining those whose stat failed for another reason. A clean that removes
// nothing writes no file and emits no summary.
func (s *Store) CleanStale() ([]Project, error) {
	start := time.Now()

	projects, err := s.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load projects: %w", err)
	}

	kept, removed := partitionByExistence(projects)

	if len(removed) == 0 {
		return removed, nil
	}

	for _, p := range removed {
		logger.Debug("clean-stale", "op", "clean-stale", "project", p.Name, "path", p.Path, "via", "internal")
	}

	if err := s.Save(kept); err != nil {
		storelog.EmitCleanStaleSummary(logger, len(removed), start, err)
		return nil, fmt.Errorf("failed to save after cleaning stale projects: %w", err)
	}

	storelog.EmitCleanStaleSummary(logger, len(removed), start, nil)

	return removed, nil
}

// Rename updates the display name of the project matched by path, leaving
// LastUsed untouched. An absent path is a silent no-op: no write, no breadcrumb.
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

	return nil
}

// Remove deletes the project with the given path. It rewrites the file even when
// the path is absent, so the breadcrumb is emitted either way.
func (s *Store) Remove(path, via string) error {
	projects, err := s.Load()
	if err != nil {
		return fmt.Errorf("failed to load projects: %w", err)
	}

	// Read the name before the delete; afterwards the entry is gone.
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
