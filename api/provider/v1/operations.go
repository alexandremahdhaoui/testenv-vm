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

// Package providerv1 defines resource types for provider communication.
// This file contains operation types for provider request/response handling.
package providerv1

// OperationResult is the standard response for all provider operations.
type OperationResult struct {
	// Success indicates if the operation completed successfully.
	Success bool `json:"success"`
	// Error contains error details if Success is false.
	Error *OperationError `json:"error,omitempty"`
	// Resource contains the resource state if Success is true.
	// This should be one of: *VMState, *NetworkState, *KeyState.
	Resource any `json:"resource,omitempty"`
}

// OperationError provides structured error information.
type OperationError struct {
	// Code is a machine-readable error code.
	Code string `json:"code"`
	// Message is a human-readable error description.
	Message string `json:"message"`
	// Retryable indicates if the operation can be retried.
	Retryable bool `json:"retryable"`
	// Details contains additional error context.
	Details map[string]any `json:"details,omitempty"`
}

// Standard error codes.
const (
	ErrCodeNotImplemented   = "NOT_IMPLEMENTED"   // Provider doesn't support this resource/operation
	ErrCodeNotFound         = "NOT_FOUND"         // Resource doesn't exist
	ErrCodeAlreadyExists    = "ALREADY_EXISTS"    // Resource already exists
	ErrCodeInvalidSpec      = "INVALID_SPEC"      // Invalid specification
	ErrCodeProviderError    = "PROVIDER_ERROR"    // Provider-specific error
	ErrCodeTimeout          = "TIMEOUT"           // Operation timed out
	ErrCodePermissionDenied = "PERMISSION_DENIED" // Insufficient permissions
	ErrCodeResourceBusy     = "RESOURCE_BUSY"     // Resource is in use
	ErrCodeDependencyFailed = "DEPENDENCY_FAILED" // Dependency not satisfied
)

// CapabilitiesResponse describes what a provider supports.
type CapabilitiesResponse struct {
	// ProviderName is the provider identifier.
	ProviderName string `json:"providerName"`
	// Version is the provider version.
	Version string `json:"version"`
	// Resources lists supported resource types and operations.
	Resources []ResourceCapability `json:"resources"`
}

// ResourceCapability describes capabilities for a resource type.
type ResourceCapability struct {
	// Kind is the resource type: vm, network, key.
	Kind string `json:"kind"`
	// Operations lists supported operations: create, get, list, delete.
	Operations []string `json:"operations"`
	// NetworkKinds lists supported network kinds (for network resource).
	NetworkKinds []string `json:"networkKinds,omitempty"`
	// KeyTypes lists supported key algorithms (for key resource).
	KeyTypes []string `json:"keyTypes,omitempty"`
	// VMFeatures lists supported VM features.
	VMFeatures []string `json:"vmFeatures,omitempty"`
}

// GetRequest is the input for get operations.
type GetRequest struct {
	Name string `json:"name"`
}

// ListRequest is the input for list operations.
type ListRequest struct {
	Filter map[string]any `json:"filter,omitempty"`
}

// DeleteRequest is the input for delete operations.
type DeleteRequest struct {
	Name  string `json:"name"`
	Force bool   `json:"force,omitempty"`
}

// ListResult is the response for list operations.
type ListResult struct {
	// Success indicates if the operation completed successfully.
	Success bool `json:"success"`
	// Error contains error details if Success is false.
	Error *OperationError `json:"error,omitempty"`
	// Resources contains the list of resources.
	Resources []any `json:"resources,omitempty"`
}
