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

package v1

// CreateInput is the input from Forge to the testenv-vm create tool.
type CreateInput struct {
	// TestID is a unique identifier for this test environment.
	TestID string `json:"testID"`
	// Stage is the test stage name (e.g., "e2e").
	Stage string `json:"stage"`
	// TmpDir is the temporary directory for files (created by Forge).
	TmpDir string `json:"tmpDir"`
	// Metadata from previous subengines.
	Metadata map[string]string `json:"metadata,omitempty"`
	// Spec is the engine-specific config from forge.yaml.
	Spec map[string]any `json:"spec"`
	// Env contains environment variables from previous engines.
	Env map[string]string `json:"env,omitempty"`
	// RootDir is the repository root (for path resolution).
	RootDir string `json:"rootDir"`
}

// DeleteInput is the input for cleanup operations.
type DeleteInput struct {
	// TestID identifies the environment to delete.
	TestID string `json:"testID"`
	// Metadata from the artifact store.
	Metadata map[string]string `json:"metadata,omitempty"`
	// ManagedResources are absolute paths for cleanup.
	ManagedResources []string `json:"managedResources,omitempty"`
}

// TestEnvArtifact is the output from testenv-vm to Forge.
type TestEnvArtifact struct {
	// TestID is the same as the input.
	TestID string `json:"testID"`
	// Files maps engine-namespaced keys to file paths relative to tmpDir.
	// Example: {"testenv-vm.ssh-key": "keys/vm-ssh"}
	Files map[string]string `json:"files,omitempty"`
	// Metadata contains engine-namespaced key-value pairs.
	// Example: {"testenv-vm.vm-ip": "192.168.100.10"}
	Metadata map[string]string `json:"metadata,omitempty"`
	// ManagedResources are absolute paths for cleanup tracking.
	ManagedResources []string `json:"managedResources,omitempty"`
	// Env contains environment variables to pass to next engines and test runners.
	Env map[string]string `json:"env,omitempty"`
}
