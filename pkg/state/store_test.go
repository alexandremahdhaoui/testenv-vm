//go:build unit

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
package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	v1 "github.com/alexandremahdhaoui/testenv-vm/api/v1"
)

// createTestState creates a sample EnvironmentState for testing.
func createTestState(testID string) *v1.EnvironmentState {
	now := time.Now().Format(time.RFC3339)
	return &v1.EnvironmentState{
		ID:        testID,
		Stage:     "unit-test",
		Status:    v1.StatusReady,
		CreatedAt: now,
		UpdatedAt: now,
		Resources: v1.ResourceMap{
			Keys: map[string]*v1.ResourceState{
				"test-key": {
					Provider:  "stub",
					Status:    v1.StatusReady,
					CreatedAt: now,
				},
			},
			Networks: map[string]*v1.ResourceState{
				"test-network": {
					Provider:  "stub",
					Status:    v1.StatusReady,
					CreatedAt: now,
				},
			},
			VMs: map[string]*v1.ResourceState{
				"test-vm": {
					Provider:  "stub",
					Status:    v1.StatusReady,
					CreatedAt: now,
					State: map[string]any{
						"ip": "192.168.100.10",
					},
				},
			},
		},
	}
}

// TestSaveLoadRoundtrip tests that saving and loading state returns identical data.
func TestSaveLoadRoundtrip(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)

	original := createTestState("roundtrip-test")

	// Save the state
	if err := store.Save(original); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Load the state back
	loaded, err := store.Load("roundtrip-test")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Compare the states
	if loaded.ID != original.ID {
		t.Errorf("ID mismatch: got %q, want %q", loaded.ID, original.ID)
	}
	if loaded.Stage != original.Stage {
		t.Errorf("Stage mismatch: got %q, want %q", loaded.Stage, original.Stage)
	}
	if loaded.Status != original.Status {
		t.Errorf("Status mismatch: got %q, want %q", loaded.Status, original.Status)
	}
	if loaded.CreatedAt != original.CreatedAt {
		t.Errorf("CreatedAt mismatch: got %q, want %q", loaded.CreatedAt, original.CreatedAt)
	}
	if loaded.UpdatedAt != original.UpdatedAt {
		t.Errorf("UpdatedAt mismatch: got %q, want %q", loaded.UpdatedAt, original.UpdatedAt)
	}

	// Check resources
	if len(loaded.Resources.Keys) != len(original.Resources.Keys) {
		t.Errorf("Keys count mismatch: got %d, want %d", len(loaded.Resources.Keys), len(original.Resources.Keys))
	}
	if len(loaded.Resources.Networks) != len(original.Resources.Networks) {
		t.Errorf("Networks count mismatch: got %d, want %d", len(loaded.Resources.Networks), len(original.Resources.Networks))
	}
	if len(loaded.Resources.VMs) != len(original.Resources.VMs) {
		t.Errorf("VMs count mismatch: got %d, want %d", len(loaded.Resources.VMs), len(original.Resources.VMs))
	}

	// Check VM state details
	loadedVM := loaded.Resources.VMs["test-vm"]
	originalVM := original.Resources.VMs["test-vm"]
	if loadedVM == nil || originalVM == nil {
		t.Fatal("VM resource is nil")
	}
	if loadedVM.Provider != originalVM.Provider {
		t.Errorf("VM Provider mismatch: got %q, want %q", loadedVM.Provider, originalVM.Provider)
	}
	if loadedVM.Status != originalVM.Status {
		t.Errorf("VM Status mismatch: got %q, want %q", loadedVM.Status, originalVM.Status)
	}
	if loadedVM.State["ip"] != originalVM.State["ip"] {
		t.Errorf("VM IP mismatch: got %v, want %v", loadedVM.State["ip"], originalVM.State["ip"])
	}
}

// TestSaveCreatesDirectories tests that Save creates directories if needed.
func TestSaveCreatesDirectories(t *testing.T) {
	tmpDir := t.TempDir()
	// Create a nested path that doesn't exist
	nestedDir := filepath.Join(tmpDir, "nested", "path", "to", "state")
	store := NewStore(nestedDir)

	state := createTestState("directory-test")

	if err := store.Save(state); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify the directory was created
	stateDir := filepath.Join(nestedDir, "state")
	info, err := os.Stat(stateDir)
	if err != nil {
		t.Fatalf("State directory was not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("State path is not a directory")
	}

	// Verify the file was created
	statePath := filepath.Join(stateDir, "testenv-directory-test.json")
	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("State file was not created: %v", err)
	}
}

// TestSaveUsesAtomicWrites tests that Save uses atomic writes (temp file pattern).
func TestSaveUsesAtomicWrites(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)

	state := createTestState("atomic-test")

	// Save the state
	if err := store.Save(state); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify no temp files remain
	stateDir := filepath.Join(tmpDir, "state")
	entries, err := os.ReadDir(stateDir)
	if err != nil {
		t.Fatalf("Failed to read state directory: %v", err)
	}

	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".tmp" {
			t.Errorf("Temp file found: %s", entry.Name())
		}
	}

	// Verify the target file exists
	expectedFile := "testenv-atomic-test.json"
	found := false
	for _, entry := range entries {
		if entry.Name() == expectedFile {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected file %q not found", expectedFile)
	}
}

// TestLoadNonExistentReturnsError tests that Load returns an error for non-existent state.
func TestLoadNonExistentReturnsError(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)

	_, err := store.Load("non-existent-id")
	if err == nil {
		t.Error("Expected error for non-existent state, got nil")
	}
}

// TestLoadInvalidJSONReturnsError tests that Load returns an error for invalid JSON.
func TestLoadInvalidJSONReturnsError(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)

	// Create the state directory
	stateDir := filepath.Join(tmpDir, "state")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatalf("Failed to create state directory: %v", err)
	}

	// Write invalid JSON
	statePath := filepath.Join(stateDir, "testenv-invalid-json.json")
	if err := os.WriteFile(statePath, []byte("{invalid json"), 0644); err != nil {
		t.Fatalf("Failed to write invalid JSON: %v", err)
	}

	_, err := store.Load("invalid-json")
	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
}

// TestDeleteRemovesFile tests that Delete removes the state file.
func TestDeleteRemovesFile(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)

	// Create a state
	state := createTestState("delete-test")
	if err := store.Save(state); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify the file exists
	if !store.Exists("delete-test") {
		t.Fatal("State file should exist before delete")
	}

	// Delete the state
	if err := store.Delete("delete-test"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify the file is gone
	if store.Exists("delete-test") {
		t.Error("State file should not exist after delete")
	}
}

// TestDeleteNonExistentDoesNotError tests that Delete does not error for non-existent file.
func TestDeleteNonExistentDoesNotError(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)

	// Delete a non-existent state - should not error
	if err := store.Delete("non-existent-id"); err != nil {
		t.Errorf("Delete for non-existent file should not error: %v", err)
	}
}

// TestListReturnsAllTestIDs tests that List returns all testIDs.
func TestListReturnsAllTestIDs(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)

	// Create multiple states
	testIDs := []string{"test-1", "test-2", "test-3"}
	for _, id := range testIDs {
		state := createTestState(id)
		if err := store.Save(state); err != nil {
			t.Fatalf("Save failed for %s: %v", id, err)
		}
	}

	// List all states
	listedIDs, err := store.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(listedIDs) != len(testIDs) {
		t.Errorf("List count mismatch: got %d, want %d", len(listedIDs), len(testIDs))
	}

	// Check all IDs are present
	for _, expectedID := range testIDs {
		found := false
		for _, listedID := range listedIDs {
			if listedID == expectedID {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected ID %q not found in list", expectedID)
		}
	}
}

// TestListEmptyDirectoryReturnsEmptySlice tests that List returns empty slice for empty directory.
func TestListEmptyDirectoryReturnsEmptySlice(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)

	// Create the state directory but don't add any files
	stateDir := filepath.Join(tmpDir, "state")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatalf("Failed to create state directory: %v", err)
	}

	listedIDs, err := store.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(listedIDs) != 0 {
		t.Errorf("Expected empty list, got %d items", len(listedIDs))
	}
}

// TestListNonExistentDirectoryReturnsEmptySlice tests that List returns empty slice if directory doesn't exist.
func TestListNonExistentDirectoryReturnsEmptySlice(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)

	// Don't create the state directory
	listedIDs, err := store.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(listedIDs) != 0 {
		t.Errorf("Expected empty list, got %d items", len(listedIDs))
	}
}

// TestExistsReturnsTrueForExistingState tests that Exists returns true for existing state.
func TestExistsReturnsTrueForExistingState(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)

	state := createTestState("exists-test")
	if err := store.Save(state); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	if !store.Exists("exists-test") {
		t.Error("Exists should return true for existing state")
	}
}

// TestExistsReturnsFalseForNonExistingState tests that Exists returns false for non-existing state.
func TestExistsReturnsFalseForNonExistingState(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)

	if store.Exists("non-existent-id") {
		t.Error("Exists should return false for non-existing state")
	}
}

// TestConcurrentSaveLoad tests basic concurrent save/load operations.
func TestConcurrentSaveLoad(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)

	const numGoroutines = 10

	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines*2)

	// Concurrent saves
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			testID := "concurrent-" + string(rune('0'+id))
			state := createTestState(testID)
			if err := store.Save(state); err != nil {
				errors <- err
			}
		}(i)
	}

	wg.Wait()

	// Concurrent loads
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			testID := "concurrent-" + string(rune('0'+id))
			_, err := store.Load(testID)
			if err != nil {
				errors <- err
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("Concurrent operation failed: %v", err)
	}
}

// TestSaveNilState tests that Save returns error for nil state.
func TestSaveNilState(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)

	if err := store.Save(nil); err == nil {
		t.Error("Expected error for nil state, got nil")
	}
}

// TestSaveEmptyID tests that Save returns error for empty ID.
func TestSaveEmptyID(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)

	state := createTestState("")

	if err := store.Save(state); err == nil {
		t.Error("Expected error for empty ID, got nil")
	}
}

// TestLoadEmptyTestID tests that Load returns error for empty testID.
func TestLoadEmptyTestID(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)

	_, err := store.Load("")
	if err == nil {
		t.Error("Expected error for empty testID, got nil")
	}
}

// TestDeleteEmptyTestID tests that Delete returns error for empty testID.
func TestDeleteEmptyTestID(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)

	if err := store.Delete(""); err == nil {
		t.Error("Expected error for empty testID, got nil")
	}
}

// TestExistsEmptyTestID tests that Exists returns false for empty testID.
func TestExistsEmptyTestID(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)

	if store.Exists("") {
		t.Error("Exists should return false for empty testID")
	}
}

// TestSpecialCharactersInTestID tests that special characters in testID are handled.
func TestSpecialCharactersInTestID(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)

	// Note: Some special characters may not work on all filesystems
	// Using characters that should work on most systems
	testID := "test-with-special_chars.123"

	state := createTestState(testID)
	if err := store.Save(state); err != nil {
		t.Fatalf("Save failed for special characters: %v", err)
	}

	loaded, err := store.Load(testID)
	if err != nil {
		t.Fatalf("Load failed for special characters: %v", err)
	}

	if loaded.ID != testID {
		t.Errorf("ID mismatch: got %q, want %q", loaded.ID, testID)
	}
}

// TestListIgnoresNonMatchingFiles tests that List ignores files that don't match the pattern.
func TestListIgnoresNonMatchingFiles(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)

	// Create the state directory
	stateDir := filepath.Join(tmpDir, "state")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatalf("Failed to create state directory: %v", err)
	}

	// Create a valid state file
	state := createTestState("valid-id")
	if err := store.Save(state); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Create non-matching files
	nonMatchingFiles := []string{
		"other-file.json",
		"testenv-.json",      // Empty testID
		"testenv-test.yaml",  // Wrong extension
		"prefix-testenv.json", // Wrong prefix
	}

	for _, name := range nonMatchingFiles {
		path := filepath.Join(stateDir, name)
		if err := os.WriteFile(path, []byte("{}"), 0644); err != nil {
			t.Fatalf("Failed to create non-matching file %s: %v", name, err)
		}
	}

	// Create a subdirectory (should be ignored)
	subDir := filepath.Join(stateDir, "testenv-subdir.json")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("Failed to create subdirectory: %v", err)
	}

	listedIDs, err := store.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(listedIDs) != 1 {
		t.Errorf("Expected 1 ID, got %d: %v", len(listedIDs), listedIDs)
	}

	if len(listedIDs) > 0 && listedIDs[0] != "valid-id" {
		t.Errorf("Expected ID 'valid-id', got %q", listedIDs[0])
	}
}

// TestStateWithComplexSpec tests saving and loading state with a full spec.
func TestStateWithComplexSpec(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)

	state := createTestState("complex-spec")
	state.Spec = &v1.TestenvSpec{
		Providers: []v1.ProviderConfig{
			{
				Name:    "stub",
				Engine:  "go://./internal/providers/stub",
				Default: true,
			},
		},
		Keys: []v1.KeyResource{
			{
				Name:     "ssh-key",
				Provider: "stub",
				Spec: v1.KeySpec{
					Type: "ed25519",
				},
			},
		},
		Networks: []v1.NetworkResource{
			{
				Name:     "bridge",
				Kind:     "bridge",
				Provider: "stub",
				Spec: v1.NetworkSpec{
					CIDR: "192.168.100.1/24",
				},
			},
		},
		VMs: []v1.VMResource{
			{
				Name:     "test-vm",
				Provider: "stub",
				Spec: v1.VMSpec{
					Memory: 1024,
					VCPUs:  2,
					Disk: v1.DiskSpec{
						Size: "20G",
					},
					Boot: v1.BootSpec{
						Order: []string{"hd"},
					},
				},
			},
		},
	}
	state.ExecutionPlan = &v1.ExecutionPlan{
		Phases: []v1.Phase{
			{
				Resources: []v1.ResourceRef{
					{Kind: "key", Name: "ssh-key"},
					{Kind: "network", Name: "bridge"},
				},
			},
			{
				Resources: []v1.ResourceRef{
					{Kind: "vm", Name: "test-vm"},
				},
			},
		},
	}
	state.Errors = []v1.ErrorRecord{
		{
			Resource:  v1.ResourceRef{Kind: "vm", Name: "test-vm"},
			Operation: "create",
			Error:     "timeout waiting for VM to be ready",
			Timestamp: time.Now().Format(time.RFC3339),
		},
	}

	if err := store.Save(state); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := store.Load("complex-spec")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Verify spec
	if loaded.Spec == nil {
		t.Fatal("Loaded spec is nil")
	}
	if len(loaded.Spec.Providers) != 1 {
		t.Errorf("Providers count mismatch: got %d, want 1", len(loaded.Spec.Providers))
	}
	if loaded.Spec.Providers[0].Name != "stub" {
		t.Errorf("Provider name mismatch: got %q, want 'stub'", loaded.Spec.Providers[0].Name)
	}

	// Verify execution plan
	if loaded.ExecutionPlan == nil {
		t.Fatal("Loaded execution plan is nil")
	}
	if len(loaded.ExecutionPlan.Phases) != 2 {
		t.Errorf("Phases count mismatch: got %d, want 2", len(loaded.ExecutionPlan.Phases))
	}

	// Verify errors
	if len(loaded.Errors) != 1 {
		t.Errorf("Errors count mismatch: got %d, want 1", len(loaded.Errors))
	}
}

// TestSaveOverwritesExisting tests that Save overwrites existing state.
func TestSaveOverwritesExisting(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)

	// Create initial state
	state := createTestState("overwrite-test")
	state.Status = v1.StatusCreating
	if err := store.Save(state); err != nil {
		t.Fatalf("Initial save failed: %v", err)
	}

	// Update and save again
	state.Status = v1.StatusReady
	state.UpdatedAt = time.Now().Add(time.Hour).Format(time.RFC3339)
	if err := store.Save(state); err != nil {
		t.Fatalf("Update save failed: %v", err)
	}

	// Load and verify
	loaded, err := store.Load("overwrite-test")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.Status != v1.StatusReady {
		t.Errorf("Status not updated: got %q, want %q", loaded.Status, v1.StatusReady)
	}
}

// TestStateFileIsValidJSON tests that saved state files are valid JSON.
func TestStateFileIsValidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)

	state := createTestState("json-valid-test")
	if err := store.Save(state); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Read the file directly
	statePath := filepath.Join(tmpDir, "state", "testenv-json-valid-test.json")
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("Failed to read state file: %v", err)
	}

	// Verify it's valid JSON
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Errorf("State file is not valid JSON: %v", err)
	}

	// Verify it's indented (formatted)
	if len(data) > 0 && data[0] == '{' && len(data) > 2 {
		// Check for newline after opening brace (indicates formatting)
		hasNewline := false
		for i := 1; i < len(data) && i < 5; i++ {
			if data[i] == '\n' {
				hasNewline = true
				break
			}
		}
		if !hasNewline {
			t.Error("State file does not appear to be formatted/indented")
		}
	}
}
