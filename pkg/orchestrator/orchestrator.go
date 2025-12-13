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
	"os"
	"path/filepath"
	"strings"
	"time"

	v1 "github.com/alexandremahdhaoui/testenv-vm/api/v1"
	"github.com/alexandremahdhaoui/testenv-vm/pkg/image"
	"github.com/alexandremahdhaoui/testenv-vm/pkg/provider"
	"github.com/alexandremahdhaoui/testenv-vm/pkg/spec"
	"github.com/alexandremahdhaoui/testenv-vm/pkg/state"
)

// Config contains configuration for the Orchestrator.
type Config struct {
	// StateDir is the directory for state files.
	StateDir string
	// ImageCacheDir is the directory for caching VM base images.
	// If empty, defaults to TESTENV_VM_IMAGE_CACHE_DIR env var or /tmp/testenv-vm/images/.
	ImageCacheDir string
	// CleanupOnFailure indicates whether to rollback on failure.
	CleanupOnFailure bool
}

// Orchestrator coordinates resource creation and deletion.
type Orchestrator struct {
	config   Config
	manager  *provider.Manager
	store    *state.Store
	executor *Executor
}

// NewOrchestrator creates a new Orchestrator with the given configuration.
func NewOrchestrator(config Config) (*Orchestrator, error) {
	// Create provider manager
	manager := provider.NewManager()

	// Create state store with config.StateDir
	store := state.NewStore(config.StateDir)

	// Determine image cache directory
	imageCacheDir := config.ImageCacheDir
	if imageCacheDir == "" {
		imageCacheDir = os.Getenv("TESTENV_VM_IMAGE_CACHE_DIR")
	}
	if imageCacheDir == "" {
		imageCacheDir = "/tmp/testenv-vm/images"
	}

	// Create image cache manager
	imageMgr, err := image.NewCacheManager(imageCacheDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create image cache manager: %w", err)
	}

	// Create executor with manager, store, and image cache manager
	executor := NewExecutor(manager, store, imageMgr)

	return &Orchestrator{
		config:   config,
		manager:  manager,
		store:    store,
		executor: executor,
	}, nil
}

// Create creates a new test environment from the given input.
func (o *Orchestrator) Create(ctx context.Context, input *v1.CreateInput) (*v1.TestEnvArtifact, error) {
	log.Printf("Creating test environment: testID=%s, stage=%s", input.TestID, input.Stage)

	// 1. Parse spec from input.Spec using spec.ParseFromMap
	testenvSpec, err := spec.ParseFromMap(input.Spec)
	if err != nil {
		return nil, fmt.Errorf("failed to parse spec: %w", err)
	}

	// 2. Validate spec using spec.ValidateEarly (Phase 1)
	templatedFields, err := spec.ValidateEarly(testenvSpec)
	if err != nil {
		return nil, fmt.Errorf("spec validation failed: %w", err)
	}

	// 3. Create artifact directory: {input.TmpDir}/{input.TestID}/
	artifactDir := filepath.Join(input.TmpDir, input.TestID)
	if err := os.MkdirAll(artifactDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create artifact directory %q: %w", artifactDir, err)
	}
	log.Printf("Created artifact directory: %s", artifactDir)

	// 4. Start all providers from spec.Providers
	for _, providerCfg := range testenvSpec.Providers {
		if err := o.manager.Start(providerCfg); err != nil {
			return nil, fmt.Errorf("failed to start provider %q: %w", providerCfg.Name, err)
		}
	}

	// 5. Build DAG using BuildDAG
	dag, err := BuildDAG(testenvSpec)
	if err != nil {
		return nil, fmt.Errorf("failed to build DAG: %w", err)
	}

	// Get execution phases from DAG
	phases, err := dag.TopologicalSort()
	if err != nil {
		return nil, fmt.Errorf("failed to compute execution phases: %w", err)
	}

	// 6. Create initial state (EnvironmentState with status=StatusCreating)
	now := time.Now().UTC().Format(time.RFC3339)
	envState := &v1.EnvironmentState{
		ID:          input.TestID,
		Stage:       input.Stage,
		Status:      v1.StatusCreating,
		CreatedAt:   now,
		UpdatedAt:   now,
		Spec:        testenvSpec,
		ArtifactDir: artifactDir,
		Resources: v1.ResourceMap{
			Keys:     make(map[string]*v1.ResourceState),
			Networks: make(map[string]*v1.ResourceState),
			VMs:      make(map[string]*v1.ResourceState),
		},
		ExecutionPlan: buildExecutionPlan(phases),
		Errors:        []v1.ErrorRecord{},
	}

	// 7. Save state
	if err := o.store.Save(envState); err != nil {
		return nil, fmt.Errorf("failed to save initial state: %w", err)
	}

	// 8. Create template context using spec.NewTemplateContext()
	templateCtx := spec.NewTemplateContext()

	// 9. Populate template context Env from input.Env
	if input.Env != nil {
		for k, v := range input.Env {
			templateCtx.Env[k] = v
		}
	}

	// 10. Execute phases using executor.ExecuteCreate (with templated fields for Phase 2 validation)
	result, err := o.executor.ExecuteCreate(ctx, testenvSpec, phases, templateCtx, envState, templatedFields)
	if err != nil {
		return nil, fmt.Errorf("execution error: %w", err)
	}

	// 11. If error and CleanupOnFailure: rollback, update state to failed, return error
	if !result.Success {
		if o.config.CleanupOnFailure {
			log.Printf("Execution failed, performing rollback")
			rollbackErrors := o.executor.Rollback(ctx, envState)
			if len(rollbackErrors) > 0 {
				log.Printf("Rollback completed with %d errors", len(rollbackErrors))
			}
		}

		// Update state to failed
		envState.Status = v1.StatusFailed
		envState.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		if saveErr := o.store.Save(envState); saveErr != nil {
			log.Printf("Failed to save failed state: %v", saveErr)
		}

		// Combine all error messages
		var errMsgs []string
		for _, e := range result.Errors {
			errMsgs = append(errMsgs, e.Error())
		}
		return nil, fmt.Errorf("create failed: %v", errMsgs)
	}

	// 12. Update state to StatusReady
	envState.Status = v1.StatusReady
	envState.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := o.store.Save(envState); err != nil {
		return nil, fmt.Errorf("failed to save ready state: %w", err)
	}

	// 13. Build and return TestEnvArtifact
	artifact := o.buildArtifact(input.TestID, envState)

	log.Printf("Test environment created successfully: testID=%s", input.TestID)
	return artifact, nil
}

// Delete deletes a test environment.
func (o *Orchestrator) Delete(ctx context.Context, input *v1.DeleteInput) error {
	log.Printf("Deleting test environment: testID=%s", input.TestID)

	// 1. Load state from store using input.TestID
	envState, err := o.store.Load(input.TestID)
	if err != nil {
		// 2. If not found, return success (already deleted)
		if os.IsNotExist(err) {
			log.Printf("State not found for testID %s, assuming already deleted", input.TestID)
			return nil
		}
		// Check if the error message indicates "not found"
		if isNotFoundError(err) {
			log.Printf("State not found for testID %s, assuming already deleted", input.TestID)
			return nil
		}
		return fmt.Errorf("failed to load state: %w", err)
	}

	// 3. Update state to StatusDestroying
	envState.Status = v1.StatusDestroying
	envState.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := o.store.Save(envState); err != nil {
		log.Printf("Failed to save destroying state: %v", err)
	}

	// 4. Start providers if needed (from state.Spec.Providers)
	if envState.Spec != nil {
		for _, providerCfg := range envState.Spec.Providers {
			// Check if provider is already running
			if _, exists := o.manager.GetInfo(providerCfg.Name); exists {
				continue
			}
			if err := o.manager.Start(providerCfg); err != nil {
				log.Printf("Failed to start provider %q for deletion: %v", providerCfg.Name, err)
				// Continue anyway - best effort
			}
		}
	}

	// 5. Execute delete in reverse order using executor.ExecuteDelete
	if err := o.executor.ExecuteDelete(ctx, envState); err != nil {
		log.Printf("Delete completed with errors: %v", err)
		// Continue anyway - best effort
	}

	// 6. Delete state file
	if err := o.store.Delete(input.TestID); err != nil {
		log.Printf("Failed to delete state file: %v", err)
		// Continue anyway - best effort
	}

	// 7. Remove artifact directory if exists
	if envState.ArtifactDir != "" {
		if err := os.RemoveAll(envState.ArtifactDir); err != nil {
			log.Printf("Failed to remove artifact directory %q: %v", envState.ArtifactDir, err)
			// Continue anyway - best effort
		}
	}

	// 8. Return nil (best-effort, don't fail on cleanup errors)
	log.Printf("Test environment deleted: testID=%s", input.TestID)
	return nil
}

// Close stops all providers.
func (o *Orchestrator) Close() error {
	return o.manager.StopAll()
}

// buildExecutionPlan converts phases to an ExecutionPlan.
func buildExecutionPlan(phases [][]v1.ResourceRef) *v1.ExecutionPlan {
	if len(phases) == 0 {
		return nil
	}

	plan := &v1.ExecutionPlan{
		Phases: make([]v1.Phase, len(phases)),
	}

	for i, phase := range phases {
		plan.Phases[i] = v1.Phase{
			Resources: phase,
		}
	}

	return plan
}

// buildArtifact builds the TestEnvArtifact from the environment state.
func (o *Orchestrator) buildArtifact(testID string, envState *v1.EnvironmentState) *v1.TestEnvArtifact {
	artifact := &v1.TestEnvArtifact{
		TestID:           testID,
		Files:            make(map[string]string),
		Metadata:         make(map[string]string),
		ManagedResources: []string{},
		Env:              make(map[string]string),
	}

	// Map SSH key paths from state
	for name, keyState := range envState.Resources.Keys {
		if keyState.State != nil {
			// Extract private key path
			if privateKeyPath, ok := keyState.State["privateKeyPath"].(string); ok && privateKeyPath != "" {
				// Use relative path from artifact dir if possible
				relPath := privateKeyPath
				if envState.ArtifactDir != "" {
					if rel, err := filepath.Rel(envState.ArtifactDir, privateKeyPath); err == nil {
						relPath = rel
					}
				}
				artifact.Files[fmt.Sprintf("testenv-vm.key.%s", name)] = relPath
				artifact.ManagedResources = append(artifact.ManagedResources, privateKeyPath)
			}

			// Extract public key path
			if publicKeyPath, ok := keyState.State["publicKeyPath"].(string); ok && publicKeyPath != "" {
				artifact.ManagedResources = append(artifact.ManagedResources, publicKeyPath)
			}

			// Add fingerprint to metadata
			if fingerprint, ok := keyState.State["fingerprint"].(string); ok && fingerprint != "" {
				artifact.Metadata[fmt.Sprintf("testenv-vm.key.%s.fingerprint", name)] = fingerprint
			}
		}

		// Add resource reference to managed resources
		artifact.ManagedResources = append(artifact.ManagedResources,
			fmt.Sprintf("testenv-vm://key/%s", name))
	}

	// Map network info to metadata
	for name, networkState := range envState.Resources.Networks {
		if networkState.State != nil {
			// Extract network IP
			if ip, ok := networkState.State["ip"].(string); ok && ip != "" {
				artifact.Metadata[fmt.Sprintf("testenv-vm.network.%s.ip", name)] = ip
			}

			// Extract interface name
			if ifName, ok := networkState.State["interfaceName"].(string); ok && ifName != "" {
				artifact.Metadata[fmt.Sprintf("testenv-vm.network.%s.interface", name)] = ifName
			}
		}

		// Add resource reference to managed resources
		artifact.ManagedResources = append(artifact.ManagedResources,
			fmt.Sprintf("testenv-vm://network/%s", name))
	}

	// Map VM info to metadata and env
	for name, vmState := range envState.Resources.VMs {
		if vmState.State != nil {
			// Extract VM IP
			if ip, ok := vmState.State["ip"].(string); ok && ip != "" {
				artifact.Metadata[fmt.Sprintf("testenv-vm.vm.%s.ip", name)] = ip
				artifact.Env[fmt.Sprintf("TESTENV_VM_%s_IP", toEnvVarName(name))] = ip
			}

			// Extract SSH command
			if sshCmd, ok := vmState.State["sshCommand"].(string); ok && sshCmd != "" {
				artifact.Env[fmt.Sprintf("TESTENV_VM_%s_SSH", toEnvVarName(name))] = sshCmd
			}

			// Extract MAC address
			if mac, ok := vmState.State["mac"].(string); ok && mac != "" {
				artifact.Metadata[fmt.Sprintf("testenv-vm.vm.%s.mac", name)] = mac
			}
		}

		// Add resource reference to managed resources
		artifact.ManagedResources = append(artifact.ManagedResources,
			fmt.Sprintf("testenv-vm://vm/%s", name))
	}

	return artifact
}

// toEnvVarName converts a resource name to an environment variable name.
// It replaces hyphens and dots with underscores and converts to uppercase.
func toEnvVarName(s string) string {
	s = strings.ToUpper(s)
	s = strings.ReplaceAll(s, "-", "_")
	s = strings.ReplaceAll(s, ".", "_")
	return s
}

// isNotFoundError checks if an error indicates a "not found" condition.
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "not found") || strings.Contains(errStr, "does not exist")
}
