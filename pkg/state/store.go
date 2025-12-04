// Copyright 2025 Alexandre Mahdhaoui
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package state provides persistent storage for test environment state.
// State files are stored as JSON on disk for reliable cleanup across restarts.
package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	v1 "github.com/alexandremahdhaoui/testenv-vm/api/v1"
)

const (
	// stateSubdir is the subdirectory within baseDir for state files.
	stateSubdir = "state"
	// stateFilePrefix is the prefix for state files.
	stateFilePrefix = "testenv-"
	// stateFileSuffix is the suffix for state files.
	stateFileSuffix = ".json"
)

// Store manages persistent state storage for test environments.
// State files are stored at {baseDir}/state/testenv-{testID}.json.
type Store struct {
	baseDir string
}

// NewStore creates a new Store with the specified base directory.
// The base directory is where all state files will be stored.
func NewStore(baseDir string) *Store {
	return &Store{
		baseDir: baseDir,
	}
}

// stateDir returns the directory path for state files.
func (s *Store) stateDir() string {
	return filepath.Join(s.baseDir, stateSubdir)
}

// statePath returns the file path for a given testID.
func (s *Store) statePath(testID string) string {
	return filepath.Join(s.stateDir(), stateFilePrefix+testID+stateFileSuffix)
}

// Save persists the environment state to disk.
// It uses atomic writes (write to temp file, then rename) to prevent corruption.
// Directories are created if they don't exist.
func (s *Store) Save(state *v1.EnvironmentState) error {
	if state == nil {
		return fmt.Errorf("cannot save nil state")
	}
	if state.ID == "" {
		return fmt.Errorf("cannot save state with empty ID")
	}

	// Ensure the state directory exists
	stateDir := s.stateDir()
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return fmt.Errorf("failed to create state directory %q: %w", stateDir, err)
	}

	// Marshal state to JSON with indentation for readability
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state to JSON: %w", err)
	}

	// Write to a temporary file first for atomic operation
	targetPath := s.statePath(state.ID)
	tempPath := targetPath + ".tmp"

	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write temporary state file %q: %w", tempPath, err)
	}

	// Atomically rename the temp file to the target path
	if err := os.Rename(tempPath, targetPath); err != nil {
		// Clean up temp file on rename failure
		_ = os.Remove(tempPath)
		return fmt.Errorf("failed to rename state file from %q to %q: %w", tempPath, targetPath, err)
	}

	return nil
}

// Load reads the environment state for the given testID from disk.
// It returns an error if the state file does not exist or cannot be parsed.
func (s *Store) Load(testID string) (*v1.EnvironmentState, error) {
	if testID == "" {
		return nil, fmt.Errorf("cannot load state with empty testID")
	}

	statePath := s.statePath(testID)
	data, err := os.ReadFile(statePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("state not found for testID %q: %w", testID, err)
		}
		return nil, fmt.Errorf("failed to read state file %q: %w", statePath, err)
	}

	var state v1.EnvironmentState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to parse state file %q: %w", statePath, err)
	}

	return &state, nil
}

// Delete removes the state file for the given testID.
// It returns an error if the file cannot be deleted, but does not error
// if the file does not exist.
func (s *Store) Delete(testID string) error {
	if testID == "" {
		return fmt.Errorf("cannot delete state with empty testID")
	}

	statePath := s.statePath(testID)
	if err := os.Remove(statePath); err != nil {
		if os.IsNotExist(err) {
			// Not an error if the file doesn't exist
			return nil
		}
		return fmt.Errorf("failed to delete state file %q: %w", statePath, err)
	}

	return nil
}

// List returns all testIDs for which state files exist.
// It returns an empty slice if no state files exist or if the state
// directory does not exist.
func (s *Store) List() ([]string, error) {
	stateDir := s.stateDir()

	entries, err := os.ReadDir(stateDir)
	if err != nil {
		if os.IsNotExist(err) {
			// Return empty slice if directory doesn't exist
			return []string{}, nil
		}
		return nil, fmt.Errorf("failed to read state directory %q: %w", stateDir, err)
	}

	var testIDs []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		// Check if the file matches the expected pattern
		if strings.HasPrefix(name, stateFilePrefix) && strings.HasSuffix(name, stateFileSuffix) {
			// Extract testID from filename
			testID := strings.TrimPrefix(name, stateFilePrefix)
			testID = strings.TrimSuffix(testID, stateFileSuffix)
			if testID != "" {
				testIDs = append(testIDs, testID)
			}
		}
	}

	return testIDs, nil
}

// Exists checks if a state file exists for the given testID.
func (s *Store) Exists(testID string) bool {
	if testID == "" {
		return false
	}

	statePath := s.statePath(testID)
	_, err := os.Stat(statePath)
	return err == nil
}
