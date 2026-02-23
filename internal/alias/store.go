// Package alias provides persistence for path aliases in a flat key=value file.
package alias

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Alias represents a single name-to-path mapping.
type Alias struct {
	Name string
	Path string
}

// Store manages persistence of alias data to a flat key=value file.
type Store struct {
	path    string
	aliases map[string]string
}

// NewStore creates a Store that reads and writes to the given file path.
func NewStore(path string) *Store {
	return &Store{
		path:    path,
		aliases: make(map[string]string),
	}
}

// Load reads aliases from the flat key=value file.
// Returns an empty map when the file is missing or empty.
// Duplicate keys are resolved with last-wins semantics.
func (s *Store) Load() (map[string]string, error) {
	f, err := os.Open(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			s.aliases = make(map[string]string)
			return s.aliases, nil
		}
		return nil, fmt.Errorf("failed to open aliases file: %w", err)
	}
	defer func() { _ = f.Close() }()

	aliases := make(map[string]string)
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		name, path, found := strings.Cut(line, "=")
		if !found {
			continue
		}

		aliases[name] = path
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read aliases file: %w", err)
	}

	s.aliases = aliases
	return s.aliases, nil
}

// Save writes all aliases to the file in sorted key=value format.
// Creates the parent directory if it does not exist.
func (s *Store) Save() error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	sorted := s.List()

	var b strings.Builder
	for _, a := range sorted {
		fmt.Fprintf(&b, "%s=%s\n", a.Name, a.Path)
	}

	if err := os.WriteFile(s.path, []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("failed to write aliases file: %w", err)
	}

	return nil
}

// Get returns the path for the given alias name and whether it was found.
func (s *Store) Get(name string) (string, bool) {
	path, ok := s.aliases[name]
	return path, ok
}

// Set adds or overwrites an alias. Each name maps to exactly one path.
func (s *Store) Set(name, path string) {
	s.aliases[name] = path
}

// Delete removes the alias with the given name.
// Returns true if the alias existed, false otherwise.
func (s *Store) Delete(name string) bool {
	_, ok := s.aliases[name]
	if ok {
		delete(s.aliases, name)
	}
	return ok
}

// List returns all aliases sorted by name.
func (s *Store) List() []Alias {
	result := make([]Alias, 0, len(s.aliases))
	for name, path := range s.aliases {
		result = append(result, Alias{Name: name, Path: path})
	}

	slices.SortFunc(result, func(a, b Alias) int {
		return strings.Compare(a.Name, b.Name)
	})

	return result
}
