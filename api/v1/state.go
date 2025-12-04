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

// Package v1 provides the API types for testenv-vm configuration.
package v1

// Status constants for environment and resources.
const (
	StatusPending    = "pending"
	StatusCreating   = "creating"
	StatusReady      = "ready"
	StatusFailed     = "failed"
	StatusDestroying = "destroying"
	StatusDestroyed  = "destroyed"
)

// EnvironmentState represents the persisted state of a test environment.
// It is stored as JSON on disk for reliable cleanup across restarts.
type EnvironmentState struct {
	// ID is the unique test environment identifier (testID).
	ID string `json:"id"`
	// Stage is the test stage name.
	Stage string `json:"stage"`
	// Status is the current environment status.
	Status string `json:"status"`
	// CreatedAt is the ISO8601 timestamp of creation.
	CreatedAt string `json:"createdAt"`
	// UpdatedAt is the ISO8601 timestamp of last update.
	UpdatedAt string `json:"updatedAt"`
	// Spec is the original TestenvSpec (for reference during cleanup).
	Spec *TestenvSpec `json:"spec,omitempty"`
	// Resources contains all resource states organized by type.
	Resources ResourceMap `json:"resources"`
	// ExecutionPlan contains the phases for resource creation/deletion.
	ExecutionPlan *ExecutionPlan `json:"executionPlan,omitempty"`
	// Errors tracks errors during execution.
	Errors []ErrorRecord `json:"errors,omitempty"`
	// ArtifactDir is the directory where artifacts are stored.
	ArtifactDir string `json:"artifactDir,omitempty"`
}

// ResourceMap contains all resource states organized by type.
type ResourceMap struct {
	// Keys contains key resource states keyed by resource name.
	Keys map[string]*ResourceState `json:"keys,omitempty"`
	// Networks contains network resource states keyed by resource name.
	Networks map[string]*ResourceState `json:"networks,omitempty"`
	// VMs contains VM resource states keyed by resource name.
	VMs map[string]*ResourceState `json:"vms,omitempty"`
}

// ResourceState represents the state of an individual resource.
type ResourceState struct {
	// Provider is the name of the provider managing this resource.
	Provider string `json:"provider"`
	// Status is the current resource status.
	Status string `json:"status"`
	// State contains provider-returned state (KeyState, NetworkState, VMState).
	State map[string]any `json:"state,omitempty"`
	// CreatedAt is the ISO8601 timestamp of creation.
	CreatedAt string `json:"createdAt,omitempty"`
	// UpdatedAt is the ISO8601 timestamp of last update.
	UpdatedAt string `json:"updatedAt,omitempty"`
	// Error contains the last error message if status is failed.
	Error string `json:"error,omitempty"`
}

// ExecutionPlan contains the phases for resource creation/deletion.
// Resources in the same phase can be executed in parallel.
type ExecutionPlan struct {
	// Phases contains ordered groups of resources to be processed.
	Phases []Phase `json:"phases"`
}

// Phase represents a group of resources to be processed together.
// All resources in a phase have their dependencies satisfied.
type Phase struct {
	// Resources contains the resource references in this phase.
	Resources []ResourceRef `json:"resources"`
}

// ErrorRecord tracks errors during execution.
type ErrorRecord struct {
	// Resource is the reference to the resource that failed.
	Resource ResourceRef `json:"resource"`
	// Operation is the operation that failed: "create", "delete".
	Operation string `json:"operation"`
	// Error is the error message.
	Error string `json:"error"`
	// Timestamp is the ISO8601 timestamp of the error.
	Timestamp string `json:"timestamp"`
}
