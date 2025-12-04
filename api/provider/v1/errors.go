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
// This file contains error helper functions for consistent error handling.
package providerv1

import "fmt"

// NewOperationError creates a new OperationError with the given code and message.
// Retryable is set based on the error code.
func NewOperationError(code, message string) *OperationError {
	return &OperationError{
		Code:      code,
		Message:   message,
		Retryable: IsRetryableCode(code),
	}
}

// NewOperationErrorWithDetails creates a new OperationError with additional details.
func NewOperationErrorWithDetails(code, message string, details map[string]any) *OperationError {
	return &OperationError{
		Code:      code,
		Message:   message,
		Retryable: IsRetryableCode(code),
		Details:   details,
	}
}

// NewNotImplementedError creates a NOT_IMPLEMENTED error for a resource operation.
func NewNotImplementedError(resource, operation string) *OperationError {
	return &OperationError{
		Code:      ErrCodeNotImplemented,
		Message:   fmt.Sprintf("operation %q not implemented for resource %q", operation, resource),
		Retryable: false,
	}
}

// NewNotFoundError creates a NOT_FOUND error for a resource.
func NewNotFoundError(resource, name string) *OperationError {
	return &OperationError{
		Code:      ErrCodeNotFound,
		Message:   fmt.Sprintf("%s %q not found", resource, name),
		Retryable: false,
	}
}

// NewAlreadyExistsError creates an ALREADY_EXISTS error for a resource.
func NewAlreadyExistsError(resource, name string) *OperationError {
	return &OperationError{
		Code:      ErrCodeAlreadyExists,
		Message:   fmt.Sprintf("%s %q already exists", resource, name),
		Retryable: false,
	}
}

// NewInvalidSpecError creates an INVALID_SPEC error with a message.
func NewInvalidSpecError(message string) *OperationError {
	return &OperationError{
		Code:      ErrCodeInvalidSpec,
		Message:   message,
		Retryable: false,
	}
}

// NewProviderError creates a PROVIDER_ERROR with optional retryability.
func NewProviderError(message string, retryable bool) *OperationError {
	return &OperationError{
		Code:      ErrCodeProviderError,
		Message:   message,
		Retryable: retryable,
	}
}

// NewTimeoutError creates a TIMEOUT error.
func NewTimeoutError(operation string) *OperationError {
	return &OperationError{
		Code:      ErrCodeTimeout,
		Message:   fmt.Sprintf("operation %q timed out", operation),
		Retryable: true,
	}
}

// NewPermissionDeniedError creates a PERMISSION_DENIED error.
func NewPermissionDeniedError(message string) *OperationError {
	return &OperationError{
		Code:      ErrCodePermissionDenied,
		Message:   message,
		Retryable: false,
	}
}

// NewResourceBusyError creates a RESOURCE_BUSY error.
func NewResourceBusyError(resource, name string) *OperationError {
	return &OperationError{
		Code:      ErrCodeResourceBusy,
		Message:   fmt.Sprintf("%s %q is currently in use", resource, name),
		Retryable: true,
	}
}

// NewDependencyFailedError creates a DEPENDENCY_FAILED error.
func NewDependencyFailedError(resource, dependency string) *OperationError {
	return &OperationError{
		Code:      ErrCodeDependencyFailed,
		Message:   fmt.Sprintf("dependency %q for %s is not satisfied", dependency, resource),
		Retryable: false,
	}
}

// IsRetryable checks if an OperationError is retryable.
func IsRetryable(err *OperationError) bool {
	if err == nil {
		return false
	}
	return err.Retryable
}

// IsRetryableCode checks if an error code is typically retryable.
func IsRetryableCode(code string) bool {
	switch code {
	case ErrCodeTimeout, ErrCodeResourceBusy:
		return true
	default:
		return false
	}
}

// ErrorResult creates an OperationResult with an error.
func ErrorResult(err *OperationError) *OperationResult {
	return &OperationResult{
		Success: false,
		Error:   err,
	}
}

// SuccessResult creates an OperationResult with a resource.
func SuccessResult(resource any) *OperationResult {
	return &OperationResult{
		Success:  true,
		Resource: resource,
	}
}
