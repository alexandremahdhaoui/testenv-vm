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

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"

	"github.com/alexandremahdhaoui/forge/pkg/engineframework"
	v1 "github.com/alexandremahdhaoui/testenv-vm/api/v1"
	"github.com/alexandremahdhaoui/testenv-vm/pkg/orchestrator"
)

var (
	// orch is the shared orchestrator instance.
	orch     *orchestrator.Orchestrator
	orchOnce sync.Once
	orchErr  error
)

// getOrchestrator returns the shared orchestrator instance, initializing it if necessary.
func getOrchestrator() (*orchestrator.Orchestrator, error) {
	orchOnce.Do(func() {
		// Configure from environment
		stateDir := getEnvOrDefault("TESTENV_VM_STATE_DIR", ".forge/testenv-vm/state")
		cleanupOnFailure := getEnvOrDefault("TESTENV_VM_CLEANUP_ON_FAILURE", "true") == "true"
		imageCacheDir := os.Getenv("TESTENV_VM_IMAGE_CACHE_DIR")

		orch, orchErr = orchestrator.NewOrchestrator(orchestrator.Config{
			StateDir:         stateDir,
			ImageCacheDir:    imageCacheDir,
			CleanupOnFailure: cleanupOnFailure,
		})
	})
	return orch, orchErr
}

// getEnvOrDefault returns the value of the environment variable with the given key,
// or the default value if the environment variable is not set or empty.
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// Create creates a new test environment from the given input.
// This is the main entry point called by the generated MCP server.
func Create(ctx context.Context, input engineframework.CreateInput, spec *v1.Spec) (*engineframework.TestEnvArtifact, error) {
	log.Printf("Handling create request: testID=%s, stage=%s", input.TestID, input.Stage)

	// Propagate spec.StateDir to TESTENV_VM_STATE_DIR env var so both the
	// orchestrator and provider subprocess (which inherits environment) use
	// the same state directory. This prevents key pair mismatches where the
	// orchestrator and provider each create independent key files.
	if spec.StateDir != "" && os.Getenv("TESTENV_VM_STATE_DIR") == "" {
		if err := os.Setenv("TESTENV_VM_STATE_DIR", spec.StateDir); err != nil {
			return nil, fmt.Errorf("failed to set TESTENV_VM_STATE_DIR: %w", err)
		}
		log.Printf("Set TESTENV_VM_STATE_DIR=%s from spec", spec.StateDir)
	}

	o, err := getOrchestrator()
	if err != nil {
		return nil, fmt.Errorf("failed to get orchestrator: %w", err)
	}

	// Convert engineframework.CreateInput to v1.CreateInput
	// The orchestrator expects Spec as map[string]any, so we convert using ToMap()
	v1Input := &v1.CreateInput{
		TestID:   input.TestID,
		Stage:    input.Stage,
		TmpDir:   input.TmpDir,
		RootDir:  input.RootDir,
		Metadata: input.Metadata,
		Spec:     spec.ToMap(),
		Env:      input.Env,
	}

	// Call the orchestrator
	createResult, err := o.Create(ctx, v1Input)
	if err != nil {
		log.Printf("Create failed: %v", err)
		return nil, err
	}

	// Convert v1.TestEnvArtifact to engineframework.TestEnvArtifact
	// Note: Provisioner is not returned in MCP response (not JSON-serializable)
	result := &engineframework.TestEnvArtifact{
		TestID:           createResult.Artifact.TestID,
		Files:            normalizeFilesMap(createResult.Artifact.Files),
		Metadata:         normalizeMetadataMap(createResult.Artifact.Metadata),
		ManagedResources: createResult.Artifact.ManagedResources,
		Env:              normalizeEnvMap(createResult.Artifact.Env),
	}

	log.Printf("Create succeeded: testID=%s", input.TestID)
	return result, nil
}

// normalizeFilesMap ensures the map is non-nil for MCP serialization.
func normalizeFilesMap(m map[string]string) map[string]string {
	if m == nil {
		return make(map[string]string)
	}
	return m
}

// normalizeMetadataMap ensures the map is non-nil for MCP serialization.
func normalizeMetadataMap(m map[string]string) map[string]string {
	if m == nil {
		return make(map[string]string)
	}
	return m
}

// normalizeEnvMap ensures the map is non-nil for MCP serialization.
func normalizeEnvMap(m map[string]string) map[string]string {
	if m == nil {
		return make(map[string]string)
	}
	return m
}
