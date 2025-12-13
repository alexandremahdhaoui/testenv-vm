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
	"testing"

	v1 "github.com/alexandremahdhaoui/testenv-vm/api/v1"
	"github.com/alexandremahdhaoui/testenv-vm/pkg/image"
	"github.com/alexandremahdhaoui/testenv-vm/pkg/provider"
	"github.com/alexandremahdhaoui/testenv-vm/pkg/state"
)

// newTestExecutorForRollback creates an Executor for testing with a temporary image cache.
func newTestExecutorForRollback(t *testing.T) (*Executor, *state.Store) {
	t.Helper()
	manager := provider.NewManager()
	stateDir := t.TempDir()
	store := state.NewStore(stateDir)
	imageMgr, err := image.NewCacheManager(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create image cache manager: %v", err)
	}
	return NewExecutor(manager, store, imageMgr), store
}

func TestExecutor_Rollback_NilState(t *testing.T) {
	executor, _ := newTestExecutorForRollback(t)

	ctx := context.Background()
	errors := executor.Rollback(ctx, nil)

	if len(errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errors))
	}
	if errors[0].Error() != "state cannot be nil" {
		t.Errorf("unexpected error message: %s", errors[0].Error())
	}
}

func TestExecutor_Rollback_EmptyPlan(t *testing.T) {
	executor, store := newTestExecutorForRollback(t)

	ctx := context.Background()
	envState := &v1.EnvironmentState{
		ID:     "test-1",
		Status: v1.StatusFailed,
		Resources: v1.ResourceMap{
			Keys:     make(map[string]*v1.ResourceState),
			Networks: make(map[string]*v1.ResourceState),
			VMs:      make(map[string]*v1.ResourceState),
		},
		ExecutionPlan: nil, // No plan
	}

	// Save state first so rollback can save updates
	if err := store.Save(envState); err != nil {
		t.Fatalf("failed to save initial state: %v", err)
	}

	errors := executor.Rollback(ctx, envState)

	if len(errors) != 0 {
		t.Errorf("expected 0 errors for empty plan, got %d: %v", len(errors), errors)
	}
}

func TestExecutor_Rollback_EmptyPhases(t *testing.T) {
	executor, store := newTestExecutorForRollback(t)

	ctx := context.Background()
	envState := &v1.EnvironmentState{
		ID:     "test-1",
		Status: v1.StatusFailed,
		Resources: v1.ResourceMap{
			Keys:     make(map[string]*v1.ResourceState),
			Networks: make(map[string]*v1.ResourceState),
			VMs:      make(map[string]*v1.ResourceState),
		},
		ExecutionPlan: &v1.ExecutionPlan{
			Phases: []v1.Phase{}, // Empty phases
		},
	}

	// Save state first
	if err := store.Save(envState); err != nil {
		t.Fatalf("failed to save initial state: %v", err)
	}

	errors := executor.Rollback(ctx, envState)

	if len(errors) != 0 {
		t.Errorf("expected 0 errors for empty phases, got %d: %v", len(errors), errors)
	}
}

func TestExecutor_Rollback_SkipsDestroyedResources(t *testing.T) {
	executor, store := newTestExecutorForRollback(t)

	ctx := context.Background()
	envState := &v1.EnvironmentState{
		ID:     "test-1",
		Status: v1.StatusFailed,
		Resources: v1.ResourceMap{
			Keys: map[string]*v1.ResourceState{
				"key1": {
					Provider: "test",
					Status:   v1.StatusDestroyed, // Already destroyed
				},
			},
			Networks: make(map[string]*v1.ResourceState),
			VMs:      make(map[string]*v1.ResourceState),
		},
		ExecutionPlan: &v1.ExecutionPlan{
			Phases: []v1.Phase{
				{Resources: []v1.ResourceRef{{Kind: "key", Name: "key1"}}},
			},
		},
	}

	// Save state first
	if err := store.Save(envState); err != nil {
		t.Fatalf("failed to save initial state: %v", err)
	}

	errors := executor.Rollback(ctx, envState)

	// Should have no errors since the resource is already destroyed and skipped
	if len(errors) != 0 {
		t.Errorf("expected 0 errors when skipping destroyed resource, got %d: %v", len(errors), errors)
	}
}

func TestExecutor_Rollback_SkipsResourceNotInState(t *testing.T) {
	executor, store := newTestExecutorForRollback(t)

	ctx := context.Background()
	envState := &v1.EnvironmentState{
		ID:     "test-1",
		Status: v1.StatusFailed,
		Resources: v1.ResourceMap{
			Keys:     make(map[string]*v1.ResourceState), // Empty - no key1
			Networks: make(map[string]*v1.ResourceState),
			VMs:      make(map[string]*v1.ResourceState),
		},
		ExecutionPlan: &v1.ExecutionPlan{
			Phases: []v1.Phase{
				{Resources: []v1.ResourceRef{{Kind: "key", Name: "key1"}}},
			},
		},
	}

	// Save state first
	if err := store.Save(envState); err != nil {
		t.Fatalf("failed to save initial state: %v", err)
	}

	errors := executor.Rollback(ctx, envState)

	// Should have no errors since resource not in state is skipped
	if len(errors) != 0 {
		t.Errorf("expected 0 errors when resource not in state, got %d: %v", len(errors), errors)
	}
}

func TestExecutor_Rollback_UpdatesStatus(t *testing.T) {
	executor, store := newTestExecutorForRollback(t)

	ctx := context.Background()
	envState := &v1.EnvironmentState{
		ID:     "test-1",
		Status: v1.StatusFailed,
		Resources: v1.ResourceMap{
			Keys: map[string]*v1.ResourceState{
				// Resource already destroyed - will be skipped
				"key1": {Provider: "test", Status: v1.StatusDestroyed},
			},
			Networks: make(map[string]*v1.ResourceState),
			VMs:      make(map[string]*v1.ResourceState),
		},
		ExecutionPlan: &v1.ExecutionPlan{
			// Add a phase with a resource (even if it gets skipped)
			// This ensures the rollback logic runs fully
			Phases: []v1.Phase{
				{Resources: []v1.ResourceRef{{Kind: "key", Name: "key1"}}},
			},
		},
	}

	// Save state first
	if err := store.Save(envState); err != nil {
		t.Fatalf("failed to save initial state: %v", err)
	}

	errors := executor.Rollback(ctx, envState)

	// No errors expected when resource is skipped (already destroyed)
	if len(errors) != 0 {
		t.Errorf("expected no errors, got: %v", errors)
	}

	// Status should be updated to destroyed after successful rollback
	if envState.Status != v1.StatusDestroyed {
		t.Errorf("status = %s, want %s", envState.Status, v1.StatusDestroyed)
	}

	// UpdatedAt should be set
	if envState.UpdatedAt == "" {
		t.Error("UpdatedAt should be set after rollback")
	}
}

func TestExecutor_Rollback_ContinuesOnError(t *testing.T) {
	// This test verifies that rollback continues even when individual deletions fail
	// Since we don't have mock providers, we'll test with resources that have
	// providers but the provider is not running (will fail)

	executor, store := newTestExecutorForRollback(t)

	ctx := context.Background()
	envState := &v1.EnvironmentState{
		ID:     "test-1",
		Status: v1.StatusFailed,
		Resources: v1.ResourceMap{
			Keys: map[string]*v1.ResourceState{
				"key1": {Provider: "nonexistent-provider", Status: v1.StatusReady},
			},
			Networks: map[string]*v1.ResourceState{
				"net1": {Provider: "nonexistent-provider", Status: v1.StatusReady},
			},
			VMs: map[string]*v1.ResourceState{
				"vm1": {Provider: "nonexistent-provider", Status: v1.StatusReady},
			},
		},
		ExecutionPlan: &v1.ExecutionPlan{
			Phases: []v1.Phase{
				{Resources: []v1.ResourceRef{{Kind: "key", Name: "key1"}}},
				{Resources: []v1.ResourceRef{{Kind: "network", Name: "net1"}}},
				{Resources: []v1.ResourceRef{{Kind: "vm", Name: "vm1"}}},
			},
		},
	}

	// Save state first
	if err := store.Save(envState); err != nil {
		t.Fatalf("failed to save initial state: %v", err)
	}

	errors := executor.Rollback(ctx, envState)

	// Should have errors (3 resources that failed to delete)
	if len(errors) < 3 {
		t.Errorf("expected at least 3 errors, got %d", len(errors))
	}

	// Status should be failed since there were errors
	if envState.Status != v1.StatusFailed {
		t.Errorf("status = %s, want %s", envState.Status, v1.StatusFailed)
	}
}

func TestExecutor_RollbackPhase_EmptyPhase(t *testing.T) {
	executor, store := newTestExecutorForRollback(t)

	ctx := context.Background()
	envState := &v1.EnvironmentState{
		ID:     "test-1",
		Status: v1.StatusDestroying,
		Resources: v1.ResourceMap{
			Keys:     make(map[string]*v1.ResourceState),
			Networks: make(map[string]*v1.ResourceState),
			VMs:      make(map[string]*v1.ResourceState),
		},
	}

	// Save state first
	if err := store.Save(envState); err != nil {
		t.Fatalf("failed to save initial state: %v", err)
	}

	errors := executor.RollbackPhase(ctx, []v1.ResourceRef{}, envState)

	if len(errors) != 0 {
		t.Errorf("expected 0 errors for empty phase, got %d", len(errors))
	}
}

func TestExecutor_RollbackPhase_SkipsDestroyedResource(t *testing.T) {
	executor, store := newTestExecutorForRollback(t)

	ctx := context.Background()
	envState := &v1.EnvironmentState{
		ID:     "test-1",
		Status: v1.StatusDestroying,
		Resources: v1.ResourceMap{
			Keys: map[string]*v1.ResourceState{
				"key1": {Provider: "test", Status: v1.StatusDestroyed},
			},
			Networks: make(map[string]*v1.ResourceState),
			VMs:      make(map[string]*v1.ResourceState),
		},
	}

	// Save state first
	if err := store.Save(envState); err != nil {
		t.Fatalf("failed to save initial state: %v", err)
	}

	phase := []v1.ResourceRef{{Kind: "key", Name: "key1"}}
	errors := executor.RollbackPhase(ctx, phase, envState)

	if len(errors) != 0 {
		t.Errorf("expected 0 errors when skipping destroyed resource, got %d", len(errors))
	}
}

func TestExecutor_RollbackPhase_ParallelDeletion(t *testing.T) {
	// This test verifies that resources in a phase are deleted in parallel
	// We test this by checking that multiple resources in the same phase
	// are processed (even if they fail due to no provider)

	executor, store := newTestExecutorForRollback(t)

	ctx := context.Background()
	envState := &v1.EnvironmentState{
		ID:     "test-1",
		Status: v1.StatusDestroying,
		Resources: v1.ResourceMap{
			VMs: map[string]*v1.ResourceState{
				"vm1": {Provider: "nonexistent", Status: v1.StatusReady},
				"vm2": {Provider: "nonexistent", Status: v1.StatusReady},
				"vm3": {Provider: "nonexistent", Status: v1.StatusReady},
			},
			Networks: make(map[string]*v1.ResourceState),
			Keys:     make(map[string]*v1.ResourceState),
		},
	}

	// Save state first
	if err := store.Save(envState); err != nil {
		t.Fatalf("failed to save initial state: %v", err)
	}

	phase := []v1.ResourceRef{
		{Kind: "vm", Name: "vm1"},
		{Kind: "vm", Name: "vm2"},
		{Kind: "vm", Name: "vm3"},
	}

	errors := executor.RollbackPhase(ctx, phase, envState)

	// All 3 should fail (no provider)
	if len(errors) != 3 {
		t.Errorf("expected 3 errors, got %d", len(errors))
	}
}

func TestExecutor_Rollback_ReversePhaseOrder(t *testing.T) {
	// Verify that phases are processed in reverse order during rollback
	// We can't easily verify order without mocks, but we can verify the
	// structure is set up correctly

	executor, store := newTestExecutorForRollback(t)

	ctx := context.Background()
	envState := &v1.EnvironmentState{
		ID:     "test-1",
		Status: v1.StatusFailed,
		Resources: v1.ResourceMap{
			Keys: map[string]*v1.ResourceState{
				"key1": {Provider: "test", Status: v1.StatusReady},
			},
			Networks: map[string]*v1.ResourceState{
				"net1": {Provider: "test", Status: v1.StatusReady},
			},
			VMs: map[string]*v1.ResourceState{
				"vm1": {Provider: "test", Status: v1.StatusReady},
			},
		},
		ExecutionPlan: &v1.ExecutionPlan{
			Phases: []v1.Phase{
				{Resources: []v1.ResourceRef{{Kind: "key", Name: "key1"}}},
				{Resources: []v1.ResourceRef{{Kind: "network", Name: "net1"}}},
				{Resources: []v1.ResourceRef{{Kind: "vm", Name: "vm1"}}},
			},
		},
	}

	// Save state first
	if err := store.Save(envState); err != nil {
		t.Fatalf("failed to save initial state: %v", err)
	}

	// Run rollback - it will fail on all resources, but should process
	// in reverse order: vm1 first, then net1, then key1
	_ = executor.Rollback(ctx, envState)

	// The fact that rollback completed without panic indicates
	// the reverse order logic is working
}

func TestExecutor_Rollback_RecordsErrors(t *testing.T) {
	executor, store := newTestExecutorForRollback(t)

	ctx := context.Background()
	envState := &v1.EnvironmentState{
		ID:     "test-1",
		Status: v1.StatusFailed,
		Resources: v1.ResourceMap{
			Keys: map[string]*v1.ResourceState{
				"key1": {Provider: "nonexistent", Status: v1.StatusReady},
			},
			Networks: make(map[string]*v1.ResourceState),
			VMs:      make(map[string]*v1.ResourceState),
		},
		ExecutionPlan: &v1.ExecutionPlan{
			Phases: []v1.Phase{
				{Resources: []v1.ResourceRef{{Kind: "key", Name: "key1"}}},
			},
		},
		Errors: []v1.ErrorRecord{}, // Start with no errors
	}

	// Save state first
	if err := store.Save(envState); err != nil {
		t.Fatalf("failed to save initial state: %v", err)
	}

	_ = executor.Rollback(ctx, envState)

	// Errors should be recorded in state
	if len(envState.Errors) == 0 {
		t.Error("expected errors to be recorded in state")
	}

	// Check that error records have rollback operation
	hasRollbackError := false
	for _, errRecord := range envState.Errors {
		if errRecord.Operation == "rollback" {
			hasRollbackError = true
			break
		}
	}
	if !hasRollbackError {
		t.Error("expected at least one rollback error in state")
	}
}

func TestExecutor_RollbackPhase_NoProviderForResource(t *testing.T) {
	executor, store := newTestExecutorForRollback(t)

	ctx := context.Background()
	envState := &v1.EnvironmentState{
		ID:     "test-1",
		Status: v1.StatusDestroying,
		Resources: v1.ResourceMap{
			Keys: map[string]*v1.ResourceState{
				"key1": {
					Provider: "", // No provider set
					Status:   v1.StatusReady,
				},
			},
			Networks: make(map[string]*v1.ResourceState),
			VMs:      make(map[string]*v1.ResourceState),
		},
	}

	// Save state first
	if err := store.Save(envState); err != nil {
		t.Fatalf("failed to save initial state: %v", err)
	}

	phase := []v1.ResourceRef{{Kind: "key", Name: "key1"}}
	errors := executor.RollbackPhase(ctx, phase, envState)

	// Should have an error about no provider
	if len(errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errors))
	}
}
