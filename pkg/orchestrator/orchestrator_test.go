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

// Package orchestrator provides resource orchestration and execution.
package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	v1 "github.com/alexandremahdhaoui/testenv-vm/api/v1"
)

func TestNewOrchestrator(t *testing.T) {
	stateDir := t.TempDir()
	config := Config{
		StateDir:         stateDir,
		CleanupOnFailure: true,
	}

	orchestrator, err := NewOrchestrator(config)
	if err != nil {
		t.Fatalf("NewOrchestrator() error = %v", err)
	}
	if orchestrator == nil {
		t.Fatal("NewOrchestrator() returned nil")
	}

	// Verify internal components are initialized
	if orchestrator.manager == nil {
		t.Error("manager is nil")
	}
	if orchestrator.store == nil {
		t.Error("store is nil")
	}
	if orchestrator.executor == nil {
		t.Error("executor is nil")
	}
	if orchestrator.config.StateDir != stateDir {
		t.Errorf("config.StateDir = %s, want %s", orchestrator.config.StateDir, stateDir)
	}
	if !orchestrator.config.CleanupOnFailure {
		t.Error("config.CleanupOnFailure should be true")
	}
}

func TestNewOrchestrator_EmptyConfig(t *testing.T) {
	config := Config{}

	orchestrator, err := NewOrchestrator(config)
	if err != nil {
		t.Fatalf("NewOrchestrator() error = %v", err)
	}
	if orchestrator == nil {
		t.Fatal("NewOrchestrator() returned nil")
	}
}

func TestOrchestrator_Close(t *testing.T) {
	stateDir := t.TempDir()
	config := Config{StateDir: stateDir}

	orchestrator, err := NewOrchestrator(config)
	if err != nil {
		t.Fatalf("NewOrchestrator() error = %v", err)
	}

	// Close should not error when no providers are running
	if err := orchestrator.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestOrchestrator_Create_InvalidSpec(t *testing.T) {
	stateDir := t.TempDir()
	config := Config{StateDir: stateDir}

	orchestrator, err := NewOrchestrator(config)
	if err != nil {
		t.Fatalf("NewOrchestrator() error = %v", err)
	}
	defer orchestrator.Close()

	ctx := context.Background()
	input := &v1.CreateInput{
		TestID: "test-1",
		Stage:  "integration",
		TmpDir: t.TempDir(),
		Spec:   nil, // Invalid spec (nil)
	}

	_, err = orchestrator.Create(ctx, input)
	if err == nil {
		t.Error("expected error for nil spec")
	}
}

func TestOrchestrator_Create_InvalidSpecMap(t *testing.T) {
	stateDir := t.TempDir()
	config := Config{StateDir: stateDir}

	orchestrator, err := NewOrchestrator(config)
	if err != nil {
		t.Fatalf("NewOrchestrator() error = %v", err)
	}
	defer orchestrator.Close()

	ctx := context.Background()
	input := &v1.CreateInput{
		TestID: "test-1",
		Stage:  "integration",
		TmpDir: t.TempDir(),
		Spec: map[string]any{
			"invalid": "spec", // Missing required fields
		},
	}

	_, err = orchestrator.Create(ctx, input)
	if err == nil {
		t.Error("expected error for invalid spec map")
	}
}

func TestOrchestrator_Create_EmptySpec(t *testing.T) {
	stateDir := t.TempDir()
	config := Config{StateDir: stateDir}

	orchestrator, err := NewOrchestrator(config)
	if err != nil {
		t.Fatalf("NewOrchestrator() error = %v", err)
	}
	defer orchestrator.Close()

	ctx := context.Background()
	tmpDir := t.TempDir()
	input := &v1.CreateInput{
		TestID: "test-1",
		Stage:  "integration",
		TmpDir: tmpDir,
		Spec: map[string]any{
			"providers": []any{}, // Empty - will fail validation
		},
	}

	// This should fail because at least one provider is required
	_, err = orchestrator.Create(ctx, input)
	if err == nil {
		t.Error("expected error for empty providers")
	}
}

func TestOrchestrator_Delete_NonExistentState(t *testing.T) {
	stateDir := t.TempDir()
	config := Config{StateDir: stateDir}

	orchestrator, err := NewOrchestrator(config)
	if err != nil {
		t.Fatalf("NewOrchestrator() error = %v", err)
	}
	defer orchestrator.Close()

	ctx := context.Background()
	input := &v1.DeleteInput{
		TestID: "nonexistent-test",
	}

	// Delete should succeed (idempotent) for non-existent state
	err = orchestrator.Delete(ctx, input)
	if err != nil {
		t.Errorf("Delete() error = %v, want nil for non-existent state", err)
	}
}

func TestOrchestrator_Delete_ExistingState(t *testing.T) {
	stateDir := t.TempDir()
	config := Config{StateDir: stateDir}

	orchestrator, err := NewOrchestrator(config)
	if err != nil {
		t.Fatalf("NewOrchestrator() error = %v", err)
	}
	defer orchestrator.Close()

	// Manually create a state file to simulate an existing environment
	// (bypassing Create which requires a working provider)
	stateSubDir := filepath.Join(stateDir, "state")
	if err := os.MkdirAll(stateSubDir, 0755); err != nil {
		t.Fatalf("failed to create state dir: %v", err)
	}

	testID := "test-delete"
	statePath := filepath.Join(stateSubDir, "testenv-"+testID+".json")
	stateContent := `{
		"id": "test-delete",
		"stage": "integration",
		"status": "ready",
		"resources": {
			"keys": {},
			"networks": {},
			"vms": {}
		}
	}`
	if err := os.WriteFile(statePath, []byte(stateContent), 0644); err != nil {
		t.Fatalf("failed to write state file: %v", err)
	}

	// Now delete it
	ctx := context.Background()
	deleteInput := &v1.DeleteInput{
		TestID: testID,
	}

	err = orchestrator.Delete(ctx, deleteInput)
	if err != nil {
		t.Errorf("Delete() error = %v", err)
	}

	// Verify state file is deleted
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Error("state file should be deleted after Delete()")
	}
}

func TestOrchestrator_Delete_RemovesArtifactDir(t *testing.T) {
	stateDir := t.TempDir()
	config := Config{StateDir: stateDir}

	orchestrator, err := NewOrchestrator(config)
	if err != nil {
		t.Fatalf("NewOrchestrator() error = %v", err)
	}
	defer orchestrator.Close()

	// Manually create state and artifact directory
	// (bypassing Create which requires a working provider)
	stateSubDir := filepath.Join(stateDir, "state")
	if err := os.MkdirAll(stateSubDir, 0755); err != nil {
		t.Fatalf("failed to create state dir: %v", err)
	}

	// Create artifact directory
	tmpDir := t.TempDir()
	artifactDir := filepath.Join(tmpDir, "test-artifact-cleanup")
	if err := os.MkdirAll(artifactDir, 0755); err != nil {
		t.Fatalf("failed to create artifact dir: %v", err)
	}

	// Create a test file in artifact directory
	testFile := filepath.Join(artifactDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	testID := "test-artifact-cleanup"
	statePath := filepath.Join(stateSubDir, "testenv-"+testID+".json")
	stateContent := `{
		"id": "test-artifact-cleanup",
		"stage": "integration",
		"status": "ready",
		"artifactDir": "` + artifactDir + `",
		"resources": {
			"keys": {},
			"networks": {},
			"vms": {}
		}
	}`
	if err := os.WriteFile(statePath, []byte(stateContent), 0644); err != nil {
		t.Fatalf("failed to write state file: %v", err)
	}

	// Verify artifact directory exists
	if _, err := os.Stat(artifactDir); os.IsNotExist(err) {
		t.Fatal("artifact directory should exist before Delete()")
	}

	// Now delete it
	ctx := context.Background()
	deleteInput := &v1.DeleteInput{
		TestID: testID,
	}

	err = orchestrator.Delete(ctx, deleteInput)
	if err != nil {
		t.Errorf("Delete() error = %v", err)
	}

	// Verify artifact directory is removed
	if _, err := os.Stat(artifactDir); !os.IsNotExist(err) {
		t.Error("artifact directory should be removed after Delete()")
	}
}

func TestBuildExecutionPlan(t *testing.T) {
	tests := []struct {
		name   string
		phases [][]v1.ResourceRef
		want   *v1.ExecutionPlan
	}{
		{
			name:   "nil phases",
			phases: nil,
			want:   nil,
		},
		{
			name:   "empty phases",
			phases: [][]v1.ResourceRef{},
			want:   nil,
		},
		{
			name: "single phase",
			phases: [][]v1.ResourceRef{
				{{Kind: "key", Name: "key1"}},
			},
			want: &v1.ExecutionPlan{
				Phases: []v1.Phase{
					{Resources: []v1.ResourceRef{{Kind: "key", Name: "key1"}}},
				},
			},
		},
		{
			name: "multiple phases",
			phases: [][]v1.ResourceRef{
				{{Kind: "key", Name: "key1"}},
				{{Kind: "network", Name: "net1"}},
				{{Kind: "vm", Name: "vm1"}},
			},
			want: &v1.ExecutionPlan{
				Phases: []v1.Phase{
					{Resources: []v1.ResourceRef{{Kind: "key", Name: "key1"}}},
					{Resources: []v1.ResourceRef{{Kind: "network", Name: "net1"}}},
					{Resources: []v1.ResourceRef{{Kind: "vm", Name: "vm1"}}},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildExecutionPlan(tt.phases)
			if tt.want == nil {
				if got != nil {
					t.Errorf("buildExecutionPlan() = %v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("buildExecutionPlan() returned nil, want non-nil")
			}
			if len(got.Phases) != len(tt.want.Phases) {
				t.Errorf("len(Phases) = %d, want %d", len(got.Phases), len(tt.want.Phases))
			}
		})
	}
}

func TestToEnvVarName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "simple",
			expected: "SIMPLE",
		},
		{
			input:    "with-hyphen",
			expected: "WITH_HYPHEN",
		},
		{
			input:    "with.dot",
			expected: "WITH_DOT",
		},
		{
			input:    "mixed-hyphen.dot",
			expected: "MIXED_HYPHEN_DOT",
		},
		{
			input:    "already_underscore",
			expected: "ALREADY_UNDERSCORE",
		},
		{
			input:    "UPPERCASE",
			expected: "UPPERCASE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := toEnvVarName(tt.input)
			if result != tt.expected {
				t.Errorf("toEnvVarName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsNotFoundError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "os not exist error",
			err:      os.ErrNotExist,
			expected: true, // os.ErrNotExist contains "file does not exist"
		},
		{
			name:     "generic error",
			err:      os.ErrPermission,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isNotFoundError(tt.err)
			if result != tt.expected {
				t.Errorf("isNotFoundError() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestOrchestrator_buildArtifact_Empty(t *testing.T) {
	stateDir := t.TempDir()
	config := Config{StateDir: stateDir}

	orchestrator, err := NewOrchestrator(config)
	if err != nil {
		t.Fatalf("NewOrchestrator() error = %v", err)
	}

	envState := &v1.EnvironmentState{
		ID:          "test-1",
		Stage:       "integration",
		ArtifactDir: "/tmp/test-1",
		Resources: v1.ResourceMap{
			Keys:     make(map[string]*v1.ResourceState),
			Networks: make(map[string]*v1.ResourceState),
			VMs:      make(map[string]*v1.ResourceState),
		},
	}

	artifact := orchestrator.buildArtifact("test-1", envState)

	if artifact.TestID != "test-1" {
		t.Errorf("TestID = %s, want test-1", artifact.TestID)
	}
	if artifact.Files == nil {
		t.Error("Files map is nil")
	}
	if artifact.Metadata == nil {
		t.Error("Metadata map is nil")
	}
	if artifact.Env == nil {
		t.Error("Env map is nil")
	}
}

func TestOrchestrator_buildArtifact_WithResources(t *testing.T) {
	stateDir := t.TempDir()
	config := Config{StateDir: stateDir}

	orchestrator, err := NewOrchestrator(config)
	if err != nil {
		t.Fatalf("NewOrchestrator() error = %v", err)
	}

	envState := &v1.EnvironmentState{
		ID:          "test-1",
		Stage:       "integration",
		ArtifactDir: "/tmp/test-1",
		Resources: v1.ResourceMap{
			Keys: map[string]*v1.ResourceState{
				"ssh-key": {
					Provider: "test",
					Status:   v1.StatusReady,
					State: map[string]any{
						"privateKeyPath": "/tmp/test-1/ssh-key",
						"publicKeyPath":  "/tmp/test-1/ssh-key.pub",
						"fingerprint":    "SHA256:xxx",
					},
				},
			},
			Networks: map[string]*v1.ResourceState{
				"test-net": {
					Provider: "test",
					Status:   v1.StatusReady,
					State: map[string]any{
						"ip":            "192.168.1.1",
						"interfaceName": "br-test",
					},
				},
			},
			VMs: map[string]*v1.ResourceState{
				"test-vm": {
					Provider: "test",
					Status:   v1.StatusReady,
					State: map[string]any{
						"ip":         "192.168.1.10",
						"mac":        "52:54:00:00:00:01",
						"sshCommand": "ssh user@192.168.1.10",
					},
				},
			},
		},
	}

	artifact := orchestrator.buildArtifact("test-1", envState)

	// Verify key artifact
	if _, ok := artifact.Files["testenv-vm.key.ssh-key"]; !ok {
		t.Error("missing key file in artifact.Files")
	}
	if artifact.Metadata["testenv-vm.key.ssh-key.fingerprint"] != "SHA256:xxx" {
		t.Error("missing or incorrect key fingerprint in metadata")
	}

	// Verify network artifact
	if artifact.Metadata["testenv-vm.network.test-net.ip"] != "192.168.1.1" {
		t.Error("missing or incorrect network IP in metadata")
	}
	if artifact.Metadata["testenv-vm.network.test-net.interface"] != "br-test" {
		t.Error("missing or incorrect network interface in metadata")
	}

	// Verify VM artifact
	if artifact.Metadata["testenv-vm.vm.test-vm.ip"] != "192.168.1.10" {
		t.Error("missing or incorrect VM IP in metadata")
	}
	if artifact.Metadata["testenv-vm.vm.test-vm.mac"] != "52:54:00:00:00:01" {
		t.Error("missing or incorrect VM MAC in metadata")
	}
	if artifact.Env["TESTENV_VM_TEST_VM_IP"] != "192.168.1.10" {
		t.Error("missing or incorrect VM IP in env")
	}
	if artifact.Env["TESTENV_VM_TEST_VM_SSH"] != "ssh user@192.168.1.10" {
		t.Error("missing or incorrect VM SSH command in env")
	}

	// Verify managed resources
	if len(artifact.ManagedResources) == 0 {
		t.Error("ManagedResources should not be empty")
	}
}

func TestOrchestrator_buildArtifact_NilResourceState(t *testing.T) {
	stateDir := t.TempDir()
	config := Config{StateDir: stateDir}

	orchestrator, err := NewOrchestrator(config)
	if err != nil {
		t.Fatalf("NewOrchestrator() error = %v", err)
	}

	envState := &v1.EnvironmentState{
		ID:          "test-1",
		Stage:       "integration",
		ArtifactDir: "/tmp/test-1",
		Resources: v1.ResourceMap{
			Keys: map[string]*v1.ResourceState{
				"ssh-key": {
					Provider: "test",
					Status:   v1.StatusReady,
					State:    nil, // nil state
				},
			},
			Networks: make(map[string]*v1.ResourceState),
			VMs:      make(map[string]*v1.ResourceState),
		},
	}

	// Should not panic
	artifact := orchestrator.buildArtifact("test-1", envState)
	if artifact == nil {
		t.Error("buildArtifact returned nil")
	}
}

func TestOrchestrator_Create_ValidationFailure(t *testing.T) {
	stateDir := t.TempDir()
	config := Config{StateDir: stateDir}

	orchestrator, err := NewOrchestrator(config)
	if err != nil {
		t.Fatalf("NewOrchestrator() error = %v", err)
	}
	defer orchestrator.Close()

	ctx := context.Background()
	tmpDir := t.TempDir()

	// Test with invalid provider (missing engine)
	input := &v1.CreateInput{
		TestID: "test-validation",
		Stage:  "integration",
		TmpDir: tmpDir,
		Spec: map[string]any{
			"providers": []any{
				map[string]any{
					"name": "invalid-provider",
					// Missing "engine" field
				},
			},
		},
	}

	_, err = orchestrator.Create(ctx, input)
	if err == nil {
		t.Error("expected validation error for provider missing engine")
	}
}

func TestOrchestrator_Create_ProviderStartFailure(t *testing.T) {
	stateDir := t.TempDir()
	config := Config{StateDir: stateDir}

	orchestrator, err := NewOrchestrator(config)
	if err != nil {
		t.Fatalf("NewOrchestrator() error = %v", err)
	}
	defer orchestrator.Close()

	ctx := context.Background()
	tmpDir := t.TempDir()
	input := &v1.CreateInput{
		TestID: "test-provider-fail",
		Stage:  "integration",
		TmpDir: tmpDir,
		Spec: map[string]any{
			"providers": []any{
				map[string]any{
					"name":   "nonexistent",
					"engine": "/nonexistent/provider",
				},
			},
		},
	}

	_, err = orchestrator.Create(ctx, input)
	if err == nil {
		t.Error("expected error when provider fails to start")
	}
}

func TestOrchestrator_CleanupOnFailure_Disabled(t *testing.T) {
	stateDir := t.TempDir()
	config := Config{
		StateDir:         stateDir,
		CleanupOnFailure: false,
	}

	orchestrator, err := NewOrchestrator(config)
	if err != nil {
		t.Fatalf("NewOrchestrator() error = %v", err)
	}

	if orchestrator.config.CleanupOnFailure {
		t.Error("CleanupOnFailure should be false")
	}
}

func TestOrchestrator_Config(t *testing.T) {
	tests := []struct {
		name   string
		config Config
	}{
		{
			name:   "empty config",
			config: Config{},
		},
		{
			name: "with state dir",
			config: Config{
				StateDir: "/tmp/test-state",
			},
		},
		{
			name: "with cleanup enabled",
			config: Config{
				StateDir:         "/tmp/test-state",
				CleanupOnFailure: true,
			},
		},
		{
			name: "with cleanup disabled",
			config: Config{
				StateDir:         "/tmp/test-state",
				CleanupOnFailure: false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orch, err := NewOrchestrator(tt.config)
			if err != nil {
				t.Fatalf("NewOrchestrator() error = %v", err)
			}
			if orch.config.StateDir != tt.config.StateDir {
				t.Errorf("StateDir = %s, want %s", orch.config.StateDir, tt.config.StateDir)
			}
			if orch.config.CleanupOnFailure != tt.config.CleanupOnFailure {
				t.Errorf("CleanupOnFailure = %v, want %v", orch.config.CleanupOnFailure, tt.config.CleanupOnFailure)
			}
		})
	}
}
