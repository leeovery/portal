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
)

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
func (s *Store) Upsert(path, name string) error {
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

	if !found {
		projects = append(projects, Project{
			Path:     path,
			Name:     name,
			LastUsed: now,
		})
	}

	return s.Save(projects)
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
func (s *Store) CleanStale() ([]Project, error) {
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

	if len(removed) > 0 {
		if err := s.Save(kept); err != nil {
			return nil, fmt.Errorf("failed to save after cleaning stale projects: %w", err)
		}
	}

	return removed, nil
}

// Rename updates the display name of the project matched by path.
// It does not change the LastUsed timestamp. It is a no-op if the path is not found.
func (s *Store) Rename(path, newName string) error {
	projects, err := s.Load()
	if err != nil {
		return fmt.Errorf("failed to load projects: %w", err)
	}

	for i := range projects {
		if projects[i].Path == path {
			projects[i].Name = newName
			return s.Save(projects)
		}
	}

	return nil
}

// Remove deletes the project with the given path. It is a no-op if the path
// is not found.
func (s *Store) Remove(path string) error {
	projects, err := s.Load()
	if err != nil {
		return fmt.Errorf("failed to load projects: %w", err)
	}

	filtered := slices.DeleteFunc(projects, func(p Project) bool {
		return p.Path == path
	})

	return s.Save(filtered)
}
