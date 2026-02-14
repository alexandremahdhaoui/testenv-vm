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
	"fmt"
	"log"
	"sync"
	"time"

	v1 "github.com/alexandremahdhaoui/testenv-vm/api/v1"
)

// Rollback performs cleanup of resources on failure.
// It deletes resources in reverse phase order, continuing on individual failures (best-effort).
// Only resources that were actually created (status != "destroyed") are deleted.
// Errors are logged but do not abort the rollback process.
// Returns a slice of all errors encountered during rollback.
func (e *Executor) Rollback(ctx context.Context, state *v1.EnvironmentState, isoConfig *IsolationConfig) []error {
	if state == nil {
		return []error{fmt.Errorf("state cannot be nil")}
	}

	// Get the execution plan phases
	var phases [][]v1.ResourceRef
	if state.ExecutionPlan != nil {
		for _, phase := range state.ExecutionPlan.Phases {
			phases = append(phases, phase.Resources)
		}
	}

	// If no phases, nothing to roll back
	if len(phases) == 0 {
		return nil
	}

	// Reverse the phases for deletion (last created = first deleted)
	for i, j := 0, len(phases)-1; i < j; i, j = i+1, j-1 {
		phases[i], phases[j] = phases[j], phases[i]
	}

	var allErrors []error

	// Update status to indicate rollback/cleanup in progress
	state.Status = v1.StatusDestroying
	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := e.store.Save(state); err != nil {
		log.Printf("rollback: failed to save destroying state: %v", err)
		allErrors = append(allErrors, fmt.Errorf("failed to save destroying state: %w", err))
	}

	// Execute deletion phases sequentially (in reverse order)
	for _, phase := range phases {
		phaseErrors := e.RollbackPhase(ctx, phase, state, isoConfig)
		allErrors = append(allErrors, phaseErrors...)
	}

	// Update final status based on errors
	if len(allErrors) > 0 {
		state.Status = v1.StatusFailed
		for _, err := range allErrors {
			state.Errors = append(state.Errors, v1.ErrorRecord{
				Operation: "rollback",
				Error:     err.Error(),
				Timestamp: time.Now().UTC().Format(time.RFC3339),
			})
		}
	} else {
		state.Status = v1.StatusDestroyed
	}

	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := e.store.Save(state); err != nil {
		log.Printf("rollback: failed to save final state: %v", err)
		allErrors = append(allErrors, fmt.Errorf("failed to save final state: %w", err))
	}

	return allErrors
}

// RollbackPhase performs cleanup of resources within a single phase.
// Resources within the phase are deleted in parallel (best-effort).
// Only resources that were actually created (status != "destroyed") are deleted.
// Errors are logged but do not abort the rollback process.
// Returns a slice of errors encountered during this phase's rollback.
func (e *Executor) RollbackPhase(ctx context.Context, phase []v1.ResourceRef, state *v1.EnvironmentState, isoConfig *IsolationConfig) []error {
	if len(phase) == 0 {
		return nil
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	var phaseErrors []error

	for _, ref := range phase {
		// Check if resource was actually created (not destroyed)
		resourceState := e.getResourceState(state, ref)
		if resourceState == nil {
			// Resource not in state, nothing to delete
			log.Printf("rollback: skipping %s/%s - not found in state", ref.Kind, ref.Name)
			continue
		}

		if resourceState.Status == v1.StatusDestroyed {
			// Resource already destroyed, skip
			log.Printf("rollback: skipping %s/%s - already destroyed", ref.Kind, ref.Name)
			continue
		}

		wg.Add(1)
		go func(r v1.ResourceRef) {
			defer wg.Done()

			log.Printf("rollback: deleting %s/%s", r.Kind, r.Name)

			if err := e.deleteResource(ctx, r, state, isoConfig); err != nil {
				log.Printf("rollback: failed to delete %s/%s: %v", r.Kind, r.Name, err)
				mu.Lock()
				phaseErrors = append(phaseErrors, fmt.Errorf("failed to delete %s/%s: %w", r.Kind, r.Name, err))
				mu.Unlock()
			} else {
				log.Printf("rollback: successfully deleted %s/%s", r.Kind, r.Name)
			}
		}(ref)
	}

	wg.Wait()

	// Save state after each phase to track progress
	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := e.store.Save(state); err != nil {
		log.Printf("rollback: failed to save state after phase: %v", err)
		phaseErrors = append(phaseErrors, fmt.Errorf("failed to save state after phase: %w", err))
	}

	return phaseErrors
}
