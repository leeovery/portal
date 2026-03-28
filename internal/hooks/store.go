// Package hooks provides persistence for pane-level hook registrations.
package hooks

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Hook represents a single hook entry for list output.
type Hook struct {
	PaneID  string
	Event   string
	Command string
}

// hooksFile is the on-disk JSON structure: map[paneID]map[event]command.
type hooksFile = map[string]map[string]string

// Store manages persistence of hook data to a JSON file.
type Store struct {
	path string
}

// NewStore creates a Store that reads and writes to the given file path.
func NewStore(path string) *Store {
	return &Store{path: path}
}

// Load reads hooks from the JSON file.
// Returns an empty map when the file is missing or contains malformed JSON.
func (s *Store) Load() (hooksFile, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return hooksFile{}, nil
		}
		return nil, err
	}

	var h hooksFile
	if err := json.Unmarshal(data, &h); err != nil {
		return hooksFile{}, nil
	}

	return h, nil
}

// Save writes hooks to the JSON file using atomic write (temp file + rename).
// Creates the parent directory if it does not exist.
func (s *Store) Save(h hooksFile) error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal hooks: %w", err)
	}

	tmp, err := os.CreateTemp(dir, "hooks-*.json.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	if err := os.Rename(tmpPath, s.path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

// Set adds or overwrites a hook for the given pane and event.
// Creates the inner map for the pane if it does not exist.
func (s *Store) Set(paneID, event, command string) error {
	h, err := s.Load()
	if err != nil {
		return fmt.Errorf("failed to load hooks: %w", err)
	}

	if h[paneID] == nil {
		h[paneID] = make(map[string]string)
	}
	h[paneID][event] = command

	return s.Save(h)
}

// Remove deletes a hook for the given pane and event.
// Removes the outer key if the inner map becomes empty.
// No-op (no error) if the pane or event does not exist.
func (s *Store) Remove(paneID, event string) error {
	h, err := s.Load()
	if err != nil {
		return fmt.Errorf("failed to load hooks: %w", err)
	}

	if events, ok := h[paneID]; ok {
		delete(events, event)
		if len(events) == 0 {
			delete(h, paneID)
		}
	}

	return s.Save(h)
}

// List returns a flat slice of Hook structs sorted by pane ID then event type.
func (s *Store) List() ([]Hook, error) {
	h, err := s.Load()
	if err != nil {
		return nil, err
	}

	var list []Hook
	for paneID, events := range h {
		for event, command := range events {
			list = append(list, Hook{
				PaneID:  paneID,
				Event:   event,
				Command: command,
			})
		}
	}

	sort.Slice(list, func(i, j int) bool {
		if list[i].PaneID != list[j].PaneID {
			return list[i].PaneID < list[j].PaneID
		}
		return list[i].Event < list[j].Event
	})

	return list, nil
}

// Get returns the event map for a specific pane, or an empty map if the pane
// has no hooks.
func (s *Store) Get(paneID string) (map[string]string, error) {
	h, err := s.Load()
	if err != nil {
		return nil, err
	}

	events, ok := h[paneID]
	if !ok {
		return map[string]string{}, nil
	}

	return events, nil
}
