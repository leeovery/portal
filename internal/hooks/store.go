// Package hooks provides persistence for hook registrations keyed by structural keys.
package hooks

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"

	"github.com/leeovery/portal/internal/fileutil"
)

// Hook represents a single hook entry for list output.
type Hook struct {
	Key     string
	Event   string
	Command string
}

// hooksFile is the on-disk JSON structure: map[structural_key]map[event]command.
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
	data, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal hooks: %w", err)
	}

	return fileutil.AtomicWrite(s.path, data)
}

// Set adds or overwrites a hook for the given key and event.
// Creates the inner map for the key if it does not exist.
func (s *Store) Set(key, event, command string) error {
	h, err := s.Load()
	if err != nil {
		return fmt.Errorf("failed to load hooks: %w", err)
	}

	if h[key] == nil {
		h[key] = make(map[string]string)
	}
	h[key][event] = command

	return s.Save(h)
}

// Remove deletes a hook for the given key and event.
// Removes the outer key if the inner map becomes empty.
// No-op (no error) if the key or event does not exist.
func (s *Store) Remove(key, event string) error {
	h, err := s.Load()
	if err != nil {
		return fmt.Errorf("failed to load hooks: %w", err)
	}

	if events, ok := h[key]; ok {
		delete(events, event)
		if len(events) == 0 {
			delete(h, key)
		}
	}

	return s.Save(h)
}

// List returns a flat slice of Hook structs sorted by key then event type.
func (s *Store) List() ([]Hook, error) {
	h, err := s.Load()
	if err != nil {
		return nil, err
	}

	var list []Hook
	for key, events := range h {
		for event, command := range events {
			list = append(list, Hook{
				Key:     key,
				Event:   event,
				Command: command,
			})
		}
	}

	sort.Slice(list, func(i, j int) bool {
		if list[i].Key != list[j].Key {
			return list[i].Key < list[j].Key
		}
		return list[i].Event < list[j].Event
	})

	return list, nil
}

// CleanStale removes hook entries for keys not present in liveKeys.
// Returns the removed keys. The file is only saved if at least one entry
// was removed.
func (s *Store) CleanStale(liveKeys []string) ([]string, error) {
	h, err := s.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load hooks: %w", err)
	}

	live := make(map[string]struct{}, len(liveKeys))
	for _, k := range liveKeys {
		live[k] = struct{}{}
	}

	kept := make(hooksFile)
	var removed []string

	for key, events := range h {
		if _, ok := live[key]; ok {
			kept[key] = events
		} else {
			removed = append(removed, key)
		}
	}

	if len(removed) > 0 {
		if err := s.Save(kept); err != nil {
			return nil, fmt.Errorf("failed to save after cleaning stale hooks: %w", err)
		}
	}

	return removed, nil
}

// Get returns the event map for a specific key, or an empty map if the key
// has no hooks.
func (s *Store) Get(key string) (map[string]string, error) {
	h, err := s.Load()
	if err != nil {
		return nil, err
	}

	events, ok := h[key]
	if !ok {
		return map[string]string{}, nil
	}

	return events, nil
}
